package service

import (
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type Port struct {
	Type string
	Port int64
}

func GetWslIP() (string, error) {
	cmd := exec.Command("wsl", "--", "bash", "-c", "ip -4 a show eth0 | grep -oP '(?<=inet\\s)\\d+(\\.\\d+){3}'")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	ip := strings.Replace(string(output), "\n", "", -1)
	reg := regexp.MustCompile("^\\d{1,3}.\\d{1,3}.\\d{1,3}.\\d{1,3}$")
	if !reg.MatchString(ip) {
		return "", errors.New("invalid ip")
	}
	return ip, nil
}
func GetLinuxHostPorts() ([]Port, error) {
	cmd := exec.Command("wsl", "--exec", "netstat", "-tunlp")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	reg := regexp.MustCompile("(tcp|udp)(\\d+)\\s+\\d+\\s+\\d+\\s+(:::|0.0.0.0:)(\\d{2,5})")
	rets := reg.FindAllStringSubmatch(string(output), -1)
	var linuxPorts []Port
	for _, ret := range rets {
		duplicated := false
		p, _ := strconv.ParseInt(ret[4], 10, 0)
		for _, find := range linuxPorts {
			if find.Port == p {
				duplicated = true
				break
			}
		}
		if !duplicated {
			port := Port{Type: ret[1], Port: p}
			linuxPorts = append(linuxPorts, port)
		}
	}
	return linuxPorts, nil
}

func GetWindowsHostPorts() ([]Port, error) {
	cmd := exec.Command("cmd", "/c", "Netstat", "-ano", "|", "findstr", "LISTENING")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	reg := regexp.MustCompile("(TCP|UDP)\\s+(\\[::\\]:|0.0.0.0:)(\\d{2,5})")
	rets := reg.FindAllStringSubmatch(string(output), -1)
	var windowsPorts []Port
	for _, ret := range rets {
		duplicated := false
		p, _ := strconv.ParseInt(ret[3], 10, 0)
		for _, find := range windowsPorts {
			if find.Port == p {
				duplicated = true
				break
			}
		}
		if !duplicated {
			port := Port{Type: ret[1], Port: p}
			if port.Type == "TCP" {
				port.Type = "tcp"
			} else {
				port.Type = "udp"
			}
			windowsPorts = append(windowsPorts, port)
		}
	}
	return windowsPorts, nil
}

func GetNeededProxyPorts(linuxPorts []Port, windowsPorts []Port) []Port {
	var result []Port
	for _, linuxPort := range linuxPorts {
		omitted := false
		for _, windowsPort := range windowsPorts {
			if linuxPort.Port == windowsPort.Port {
				omitted = true
				break
			}
		}
		if !omitted {
			result = append(result, linuxPort)
		}
	}
	return result
}