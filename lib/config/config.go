package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

type Config struct {
	SchemaVersion  int             `json:"schemaVersion,omitempty"`
	Distro         string          `json:"distro,omitempty"`
	OnlyPredefined bool            `json:"onlyPredefined"`
	Predefined     PredefinedPorts `json:"predefined"`
	Ignore         IgnorePorts     `json:"ignore"`
	Allowlist      AllowlistRules  `json:"allowlist"`
	ListenAddress  string          `json:"listenAddress,omitempty"`
	UDPEnabled     bool            `json:"udpEnabled"`
	MaxConnections int             `json:"maxConnections,omitempty"`
	UDPIdleSeconds int             `json:"udpIdleSeconds,omitempty"`
}
type PredefinedPorts struct {
	Tcp []PortProxy `json:"tcp"`
	Udp []PortProxy `json:"udp"`
}
type IgnorePorts struct {
	Tcp []int64 `json:"tcp"`
	Udp []int64 `json:"udp"`
}
type PortProxy struct {
	Local  int64
	Remote int64
}

func (p PortProxy) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("%d:%d", p.Local, p.Remote))
}
func (p *PortProxy) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}
	parts := strings.Split(text, ":")
	if len(parts) != 2 {
		return fmt.Errorf("mapping %q must be local:remote", text)
	}
	local, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || !validPort(local) {
		return fmt.Errorf("invalid local port in %q", text)
	}
	remote, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || !validPort(remote) {
		return fmt.Errorf("invalid remote port in %q", text)
	}
	*p = PortProxy{Local: local, Remote: remote}
	return nil
}
func validPort(port int64) bool { return port > 0 && port <= 65535 }

type AllowlistRules struct {
	Tcp map[int64][]*net.IPNet
	Udp map[int64][]*net.IPNet
}

func (a *AllowlistRules) UnmarshalJSON(data []byte) error {
	var raw struct {
		Tcp map[string][]string `json:"tcp"`
		Udp map[string][]string `json:"udp"`
	}
	if err := decode(data, &raw); err != nil {
		return err
	}
	tcp, err := parseRules(raw.Tcp)
	if err != nil {
		return err
	}
	udp, err := parseRules(raw.Udp)
	if err != nil {
		return err
	}
	*a = AllowlistRules{Tcp: tcp, Udp: udp}
	return nil
}
func parseRules(raw map[string][]string) (map[int64][]*net.IPNet, error) {
	out := make(map[int64][]*net.IPNet, len(raw))
	for key, entries := range raw {
		port, err := strconv.ParseInt(key, 10, 64)
		if err != nil || !validPort(port) {
			return nil, fmt.Errorf("allowlist: invalid port %q", key)
		}
		if _, exists := out[port]; exists {
			return nil, fmt.Errorf("allowlist: duplicate port %d", port)
		}
		nets := make([]*net.IPNet, 0, len(entries))
		for _, entry := range entries {
			n, err := parseIPOrCIDR(entry)
			if err != nil {
				return nil, fmt.Errorf("allowlist port %d: %w", port, err)
			}
			nets = append(nets, n)
		}
		out[port] = nets
	}
	return out, nil
}
func (a AllowlistRules) MarshalJSON() ([]byte, error) {
	raw := struct {
		Tcp map[string][]string `json:"tcp"`
		Udp map[string][]string `json:"udp"`
	}{formatRules(a.Tcp), formatRules(a.Udp)}
	return json.Marshal(raw)
}
func formatRules(rules map[int64][]*net.IPNet) map[string][]string {
	out := make(map[string][]string, len(rules))
	for port, nets := range rules {
		items := make([]string, 0, len(nets))
		for _, n := range nets {
			items = append(items, n.String())
		}
		out[strconv.FormatInt(port, 10)] = items
	}
	return out
}
func parseIPOrCIDR(s string) (*net.IPNet, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q", s)
		}
		return n, nil
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP %q", s)
	}
	if ip4 := ip.To4(); ip4 != nil {
		return &net.IPNet{IP: ip4, Mask: net.CIDRMask(32, 32)}, nil
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}, nil
}
func decode(data []byte, out any) error {
	if err := checkKeys(json.NewDecoder(bytes.NewReader(data)), 0); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("configuration must contain one JSON value")
	}
	return nil
}

// encoding/json otherwise accepts repeated keys, including differently cased
// struct fields. Reject ambiguous edits before publishing an access policy.
func checkKeys(d *json.Decoder, depth int) error {
	if depth > 64 {
		return fmt.Errorf("configuration is nested too deeply")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name := strings.ToLower(key.(string))
			if seen[name] {
				return fmt.Errorf("duplicate JSON field %q", key)
			}
			seen[name] = true
			if err := checkKeys(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := checkKeys(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter")
	}
	_, err = d.Token()
	return err
}
func Parse(data []byte) (Config, error) {
	data = stripComments(data)
	var c Config
	if len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '{' {
		return c, fmt.Errorf("configuration must be a JSON object")
	}
	if err := decode(data, &c); err != nil {
		return c, err
	}
	return c, c.Validate()
}
func (c Config) Validate() error {
	if c.SchemaVersion != 0 && c.SchemaVersion != 1 {
		return fmt.Errorf("unsupported schemaVersion %d", c.SchemaVersion)
	}
	if strings.TrimSpace(c.Distro) != c.Distro || strings.IndexFunc(c.Distro, unicode.IsControl) >= 0 {
		return fmt.Errorf("distro must be a WSL distribution name")
	}
	if c.ListenAddress != "" && net.ParseIP(c.ListenAddress) == nil {
		return fmt.Errorf("listenAddress must be an IP address")
	}
	if c.MaxConnections < 0 || c.MaxConnections > 1024 {
		return fmt.Errorf("maxConnections must be 0 (default) or 1..1024")
	}
	if c.UDPIdleSeconds < 0 || c.UDPIdleSeconds > 86400 {
		return fmt.Errorf("udpIdleSeconds must be 0 (default) or 1..86400")
	}
	for protocol, ports := range map[string][]PortProxy{"tcp": c.Predefined.Tcp, "udp": c.Predefined.Udp} {
		seen := map[int64]bool{}
		for _, p := range ports {
			if !validPort(p.Local) || !validPort(p.Remote) {
				return fmt.Errorf("%s: invalid mapping", protocol)
			}
			if seen[p.Local] {
				return fmt.Errorf("%s: duplicate local port %d", protocol, p.Local)
			}
			seen[p.Local] = true
		}
	}
	for _, ports := range [][]int64{c.Ignore.Tcp, c.Ignore.Udp} {
		for _, port := range ports {
			if !validPort(port) {
				return fmt.Errorf("invalid ignored port %d", port)
			}
		}
	}
	return nil
}

// Legacy block comments are accepted, without corrupting quoted string contents.
func stripComments(data []byte) []byte {
	out := make([]byte, 0, len(data))
	quoted, escaped := false, false
	for i := 0; i < len(data); i++ {
		b := data[i]
		if quoted {
			out = append(out, b)
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == '"' {
				quoted = false
			}
			continue
		}
		if b == '"' {
			quoted = true
		}
		if b == '/' && i+1 < len(data) && data[i+1] == '*' {
			end := bytes.Index(data[i+2:], []byte("*/"))
			if end < 0 {
				return data
			}
			i += end + 3
			out = append(out, ' ')
			continue
		}
		out = append(out, b)
	}
	return out
}
func LoadFile(file string) (Config, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return Config{}, err
	}
	return Parse(data)
}
func GetConfig() (Config, error) {
	home := os.Getenv("WSLPP_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return Config{}, err
		}
		home = filepath.Join(userHome, ".wslpp")
	}
	c, err := LoadFile(filepath.Join(home, "config.json"))
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	return c, err
}
