package config

import (
	"encoding/json"
	"fmt"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/util"
	"github.com/pkg/errors"
	"io/ioutil"
	"log"
	"net"
	"os/user"
	"path"
	"regexp"
	"strconv"
	"strings"
)

type Config struct {
	OnlyPredefined bool
	Predefined     PredefinedPorts
	Ignore         IgnorePorts
	Allowlist      AllowlistRules
}

type PredefinedPorts struct {
	Tcp []PortProxy `json:"tcp"`
	Udp []PortProxy `json:"udp"`
}

type PortProxy struct {
	Local  int64
	Remote int64
}

func (pp PortProxy) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", fmt.Sprintf("%d:%d", pp.Local, pp.Remote))), nil
}

func (pp *PortProxy) UnmarshalJSON(data []byte) error {
	var ppStr string
	err := json.Unmarshal(data, &ppStr)
	if err != nil {
		return err
	}
	ppPorts := strings.Split(ppStr, ":")
	pp.Local, err = strconv.ParseInt(ppPorts[0], 10, 64)
	if err != nil {
		return err
	}
	pp.Remote, err = strconv.ParseInt(ppPorts[1], 10, 64)
	if err != nil {
		return err
	}
	return nil
}

type IgnorePorts struct {
	Tcp []int64 `json:"tcp"`
	Udp []int64 `json:"udp"`
}

// AllowlistRules restricts which client IPs may connect, keyed by windows listen port.
// Ports without rules are open to everyone.
type AllowlistRules struct {
	Tcp map[int64][]*net.IPNet
}

func (ar *AllowlistRules) UnmarshalJSON(data []byte) error {
	var raw struct {
		Tcp map[string][]string `json:"tcp"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	ar.Tcp = make(map[int64][]*net.IPNet, len(raw.Tcp))
	for portStr, entries := range raw.Tcp {
		port, err := strconv.ParseInt(portStr, 10, 64)
		if err != nil || port <= 0 || port > 65535 {
			return fmt.Errorf("allowlist: invalid port %q", portStr)
		}
		nets := make([]*net.IPNet, 0, len(entries))
		for _, entry := range entries {
			n, err := parseIPOrCIDR(entry)
			if err != nil {
				return fmt.Errorf("allowlist: port %d: %s", port, err)
			}
			nets = append(nets, n)
		}
		ar.Tcp[port] = nets
	}
	return nil
}

// parseIPOrCIDR accepts "192.168.1.5" or "192.168.1.0/24" (IPv6 as well).
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

// JsonFile is a struct to unmarshal config file
type JsonFile struct {
	OnlyPredefined bool            `json:"onlyPredefined"`
	Predefined     PredefinedPorts `json:"predefined"`
	Ignore         IgnorePorts     `json:"ignore"`
	Allowlist      AllowlistRules  `json:"allowlist"`
}

var jsonCommentRegexp = regexp.MustCompile(`/\*([\s\S]*?)\*/`)

func init() {
	// create config dir
	userHome, _ := user.Current()
	_, err := util.CreatePathIfNotExist(path.Join(userHome.HomeDir, ".wslpp"))
	if err != nil {
		log.Fatalf("config init error: %s", err)
	}
}

// GetConfig return the config object read from %HOMEPATH%/.wslpp/config.json
func GetConfig() (Config, error) {
	var out Config
	userHome, _ := user.Current()
	configFilePath := path.Join(userHome.HomeDir, ".wslpp/config.json")
	exists, _ := util.PathExists(configFilePath)
	if !exists {
		out.OnlyPredefined = false
		return out, nil
	}
	b, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		return out, errors.Wrap(err, "read file error")
	}
	b = jsonCommentRegexp.ReplaceAll(b, []byte{})
	if err = json.Unmarshal(b, &out); err != nil {
		return out, errors.Wrap(err, "unmarshal config json error")
	}
	return out, nil
}
