package service

import "strings"

type Port struct {
	Type      string
	Port      int64
	ProxyPort int64
}

func GetNeededProxyPorts(linuxPorts []Port, windowsPorts []Port) []Port {
	var result []Port
	for _, linuxPort := range linuxPorts {
		omitted := false
		for _, windowsPort := range windowsPorts {
			if linuxPort.ProxyPort == windowsPort.Port && strings.EqualFold(linuxPort.Type, windowsPort.Type) {
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
