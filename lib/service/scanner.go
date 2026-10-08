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
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
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
type Scanner struct {
	Runner    Runner
	LegacyNAT bool
}

func (s Scanner) State(ctx context.Context) (Snapshot, error) {
	r := s.Runner
	if r == nil {
		r = CommandRunner{}
	}
	state := Snapshot{State: "unknown", NetworkMode: "unknown"}
	all, err := r.Output(ctx, "--list", "--verbose")
	if err != nil {
		return state, err
	}
	state.Distro = parseDefaultDistro(decodeWslOutput(all))
	if state.Distro == "" {
		return state, errors.New("no default WSL distribution visible to the service account")
	}
	version := defaultVersion(decodeWslOutput(all))
	if version != "2" {
		return state, fmt.Errorf("default distribution %q must use WSL2", state.Distro)
	}
	running, err := r.Output(ctx, "--list", "--running", "--quiet")
	if err != nil {
		return state, err
	}
	state.State = "stopped"
	if containsFold(parseLines(decodeWslOutput(running)), state.Distro) {
		state.State = "running"
	}
	return state, nil
}
func (s Scanner) Scan(ctx context.Context) (Snapshot, error) {
	state, err := s.State(ctx)
	if err != nil {
		return state, err
	}
	if state.State != "running" {
		now := time.Now()
		state.LastScan = &now
		return state, nil
	}
	r := s.Runner
	if r == nil {
		r = CommandRunner{}
	}
	args := []string{"--distribution", state.Distro, "--exec"}
	mode, err := r.Output(ctx, append(args, "/usr/lib/wsl/wslinfo", "--networking-mode")...)
	if err != nil {
		if ctx.Err() != nil {
			return state, ctx.Err()
		}
		if !s.LegacyNAT {
			return state, fmt.Errorf("cannot determine WSL networking mode: %w; use --legacy-nat only for verified older NAT installations", err)
		}
		state.NetworkMode = "nat"
	} else {
		state.NetworkMode = strings.ToLower(strings.TrimSpace(decodeWslOutput(mode)))
	}
	if state.NetworkMode != "nat" {
		now := time.Now()
		state.LastScan = &now
		return state, nil
	}
	addresses, err := r.Output(ctx, append(args, "ip", "-j", "-4", "address", "show", "dev", "eth0")...)
	if err != nil {
		return state, err
	}
	state.IP, err = parseAddress(addresses)
	if err != nil {
		return state, err
	}
	// Recheck immediately before commands that may start a distribution. WSL has
	// no atomic 'execute only if running'; the remaining OS race is documented.
	latest, err := s.State(ctx)
	if err != nil {
		return state, err
	}
	if latest.State != "running" || latest.Distro != state.Distro {
		return latest, nil
	}
	ports, err := r.Output(ctx, append(args, "ss", "-H", "-l", "-n", "-t", "-u")...)
	if err != nil {
		return state, err
	}
	state.Ports, err = ParseLinuxPorts(string(ports))
	if err != nil {
		return state, err
	}
	now := time.Now()
	state.LastScan = &now
	return state, nil
}
func defaultVersion(out string) string {
	for _, line := range parseLines(out) {
		if strings.HasPrefix(line, "*") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				return fields[len(fields)-1]
			}
		}
	}
	return ""
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
		return "", err
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
func ParseLinuxPorts(out string) ([]Port, error) {
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
		if host != "*" && host != "0.0.0.0" && host != "::" {
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
