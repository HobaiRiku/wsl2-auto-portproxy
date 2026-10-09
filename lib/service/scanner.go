package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Runner interface {
	Output(context.Context, ...string) ([]byte, error)
}
type CommandRunner struct{ Timeout time.Duration }

func (r CommandRunner) Output(ctx context.Context, args ...string) ([]byte, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wsl.exe", args...)
	cmd.Env = append(os.Environ(), "WSL_UTF8=1")
	cmd.WaitDelay = time.Second
	hideWindow(cmd)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		detail := ""
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			detail = strings.TrimSpace(decodeWslOutput(exit.Stderr))
		}
		if detail == "" {
			detail = strings.TrimSpace(decodeWslOutput(out))
		}
		if detail != "" {
			return nil, fmt.Errorf("wsl %s: %w: %s", strings.Join(args, " "), err, detail)
		}
		return nil, fmt.Errorf("wsl %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

type Snapshot struct {
	State       string     `json:"state"`
	Distro      string     `json:"distro"`
	IP          string     `json:"ip"`
	NetworkMode string     `json:"networkMode"`
	LastScan    *time.Time `json:"lastScan,omitempty"`
	Error       string     `json:"error,omitempty"`
	Ports       []Port     `json:"-"`
}

// Distro is one row of `wsl --list --verbose`.
type Distro struct {
	Name    string `json:"name"`
	Default bool   `json:"default"`
	Version string `json:"version"`
}

type Scanner struct {
	Runner Runner
}

func (s Scanner) runner() Runner {
	if s.Runner == nil {
		return CommandRunner{}
	}
	return s.Runner
}

// Distros lists installed distributions without starting any of them.
func (s Scanner) Distros(ctx context.Context) ([]Distro, error) {
	out, err := s.runner().Output(ctx, "--list", "--verbose")
	if err != nil {
		return nil, err
	}
	return parseDistros(decodeWslOutput(out)), nil
}

// State resolves the target distribution (the configured one, or the WSL
// default when distro is empty) and whether it is running. It never executes
// a Linux command, so it cannot boot a stopped distribution.
func (s Scanner) State(ctx context.Context, distro string) (Snapshot, error) {
	state := Snapshot{State: "unknown", NetworkMode: "unknown"}
	distros, err := s.Distros(ctx)
	if err != nil {
		return state, err
	}
	var target *Distro
	for i := range distros {
		if (distro == "" && distros[i].Default) || (distro != "" && strings.EqualFold(distros[i].Name, distro)) {
			target = &distros[i]
			break
		}
	}
	if target == nil {
		if distro == "" {
			return state, errors.New("no default WSL distribution visible to this Windows account")
		}
		names := make([]string, len(distros))
		for i, d := range distros {
			names[i] = d.Name
		}
		return state, fmt.Errorf("WSL distribution %q not found (installed: %s)", distro, strings.Join(names, ", "))
	}
	state.Distro = target.Name
	if target.Version != "2" {
		return state, fmt.Errorf("distribution %q must use WSL2", state.Distro)
	}
	running, err := s.runner().Output(ctx, "--list", "--running", "--quiet")
	if err != nil {
		return state, err
	}
	state.State = "stopped"
	if containsFold(parseLines(decodeWslOutput(running)), state.Distro) {
		state.State = "running"
	}
	return state, nil
}

// probeScript gathers everything from inside the distribution in a single
// wsl.exe call. wslinfo lives in /usr/bin on current WSL (a link to /init);
// releases without it predate mirrored networking, so they are NAT.
const probeScript = `m=$(wslinfo --networking-mode 2>/dev/null || /usr/lib/wsl/wslinfo --networking-mode 2>/dev/null); m=${m:-nat}; echo "@@mode $m"; [ "$m" = nat ] || exit 0; echo @@ip; ip -j -4 address show dev eth0 2>&1 || echo "@@error ip"; echo @@ports; ss -H -l -n -t -u 2>&1 || echo "@@error ss"`

func (s Scanner) Scan(ctx context.Context, distro string) (Snapshot, error) {
	state, err := s.State(ctx, distro)
	if err != nil {
		return state, err
	}
	if state.State != "running" {
		now := time.Now()
		state.LastScan = &now
		return state, nil
	}
	// WSL has no atomic 'execute only if running'; a distribution stopping
	// between State and this call is the remaining, documented race.
	out, err := s.runner().Output(ctx, "--distribution", state.Distro, "--exec", "sh", "-c", probeScript)
	if err != nil {
		return state, err
	}
	sections, err := parseProbe(decodeWslOutput(out))
	if err != nil {
		return state, err
	}
	state.NetworkMode = strings.ToLower(strings.TrimSpace(sections["mode"]))
	if state.NetworkMode != "nat" {
		now := time.Now()
		state.LastScan = &now
		return state, nil
	}
	state.IP, err = parseAddress([]byte(sections["ip"]))
	if err != nil {
		return state, err
	}
	state.Ports, err = ParseLinuxPorts(sections["ports"], state.IP)
	if err != nil {
		return state, err
	}
	now := time.Now()
	state.LastScan = &now
	return state, nil
}

// parseProbe splits probeScript output into its "@@name" sections.
func parseProbe(out string) (map[string]string, error) {
	sections := map[string]string{}
	current := ""
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "@@") {
			name, value, _ := strings.Cut(strings.TrimPrefix(line, "@@"), " ")
			if name == "error" {
				return nil, fmt.Errorf("WSL %s command failed: %s", value, strings.TrimSpace(sections[current]))
			}
			current = name
			sections[name] = strings.TrimSpace(value)
			continue
		}
		if current != "" {
			sections[current] += line + "\n"
		}
	}
	if _, ok := sections["mode"]; !ok {
		return nil, fmt.Errorf("unexpected WSL probe output: %q", strings.TrimSpace(out))
	}
	return sections, nil
}
func parseAddress(data []byte) (string, error) {
	var devices []struct {
		Addresses []struct {
			Family string `json:"family"`
			Local  string `json:"local"`
			Scope  string `json:"scope"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal(data, &devices); err != nil {
		return "", fmt.Errorf("cannot parse WSL eth0 address: %w", err)
	}
	for _, device := range devices {
		for _, a := range device.Addresses {
			ip := net.ParseIP(a.Local)
			if a.Family == "inet" && a.Scope == "global" && ip != nil && ip.To4() != nil {
				return ip.String(), nil
			}
		}
	}
	return "", errors.New("no global IPv4 address on WSL eth0")
}

// ParseLinuxPorts returns listeners reachable through the WSL NAT address:
// wildcard binds plus binds on any of the given addresses.
func ParseLinuxPorts(out string, addresses ...string) ([]Port, error) {
	ports := make([]Port, 0)
	seen := map[string]bool{}
	for _, line := range parseLines(out) {
		f := strings.Fields(line)
		if len(f) < 5 {
			return nil, fmt.Errorf("invalid ss output: %q", line)
		}
		protocol := f[0]
		if protocol != "tcp" && protocol != "udp" {
			return nil, fmt.Errorf("unknown ss protocol %q", protocol)
		}
		endpoint := f[4]
		colon := strings.LastIndex(endpoint, ":")
		if colon < 0 {
			return nil, fmt.Errorf("invalid ss endpoint %q", endpoint)
		}
		host := strings.Trim(endpoint[:colon], "[]")
		port, err := strconv.ParseInt(endpoint[colon+1:], 10, 64)
		if err != nil || port <= 0 || port > 65535 {
			return nil, fmt.Errorf("invalid ss port %q", endpoint)
		}
		if host != "*" && host != "0.0.0.0" && host != "::" && !containsFold(addresses, host) {
			continue
		}
		key := protocol + ":" + strconv.FormatInt(port, 10)
		if seen[key] {
			continue
		}
		seen[key] = true
		ports = append(ports, Port{Type: protocol, Port: port, ProxyPort: port})
	}
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Type != ports[j].Type {
			return ports[i].Type < ports[j].Type
		}
		return ports[i].Port < ports[j].Port
	})
	return ports, nil
}
