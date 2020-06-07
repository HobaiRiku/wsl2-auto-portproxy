package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
	"wsl2-auto-portproxy/lib/config"
	"wsl2-auto-portproxy/lib/proxy"
	"wsl2-auto-portproxy/lib/service"
	"wsl2-auto-portproxy/lib/storage"
)

var version string

func main() {
	// print version
	var showVersion bool
	flag.BoolVar(&showVersion, "v", false, "show version")
	flag.Parse()
	if showVersion {
		fmt.Println(version)
		os.Exit(1)
	}
	// get config interval
	go func() {
		for {
			c, err := config.GetConfig()
			if err != nil {
				log.Printf("error getting config file: %s", err)
			} else {
				storage.C = c
			}
			time.Sleep(time.Second)
		}
	}()
	for {
		// get linux's ip
		storage.WslIp, _ = service.GetWslIP()
		// get all tcp ports in linux now
		linuxPorts, err := service.GetLinuxHostPorts()
		if err != nil {
			log.Fatal(err)
		}
		// change proxy port by config "predefined"
		for i, p := range linuxPorts {
			for _, predefinedTcpPort := range storage.C.Predefined.Tcp {
				if p.Port == predefinedTcpPort.Remote {
					linuxPorts[i].ProxyPort = predefinedTcpPort.Local
				}
			}
		}
		// filter by config "ignore"
		for i := 0; i < len(linuxPorts); {
			needToDelete := false
			for _, ignorePort := range storage.C.Ignore.Tcp {
				if ignorePort == linuxPorts[i].Port {
					needToDelete = true
				}
			}
			if needToDelete {
				linuxPorts = append(linuxPorts[:i], linuxPorts[i+1:]...)
			} else {
				i++
			}
		}
		// filter by config "OnlyPredefined"
		if storage.C.OnlyPredefined {
			for i := 0; i < len(linuxPorts); {
				needToDelete := true
				for _, predefinedTcpPort := range storage.C.Predefined.Tcp {
					if predefinedTcpPort.Remote == linuxPorts[i].Port {
						needToDelete = false
					}
				}
				if needToDelete {
					linuxPorts = append(linuxPorts[:i], linuxPorts[i+1:]...)
				} else {
					i++
				}
			}
		}
		// get all tcp ports in local windows now
		windowsPorts, err := service.GetWindowsHostPorts()
		if err != nil {
			log.Println(err)
		}
		// calculate which port need to proxy
		needPorts := service.GetNeededProxyPorts(linuxPorts, windowsPorts)
		// create proxy
		for _, port := range needPorts {
			omitted := false
			for _, p := range storage.ProxyPool {
				// update WslIp
				p.WslIp = storage.WslIp
				if p.Port == port.Port {
					omitted = true
					if !p.IsRunning {
						err := p.Start()
						if err != nil {
							log.Printf("start proxy error,%s\n", err)
						}
					}
					break
				}
			}
			if !omitted {
				newProxy := proxy.Proxy{Port: port.Port, ProxyPort: port.ProxyPort, Type: port.Type, WslIp: storage.WslIp}
				err := newProxy.Start()
				if err != nil {
					log.Printf("start proxy error,%s\n", err)
				}
				storage.ProxyPool = append(storage.ProxyPool, newProxy)
			}
		}
		// check for delete update
		for _, p := range storage.ProxyPool {
			needToDelete := true
			for _, port := range linuxPorts {
				if port.Port == p.Port && port.ProxyPort == p.ProxyPort {
					needToDelete = false
					break
				}
			}
			if needToDelete {
				_ = p.Stop()
			}
			// clean not running proxy
			if !p.IsRunning {
				for i := 0; i < len(storage.ProxyPool); {
					if storage.ProxyPool[i].Port == p.Port && storage.ProxyPool[i].ProxyPort == p.ProxyPort {
						storage.ProxyPool = append(storage.ProxyPool[:i], storage.ProxyPool[i+1:]...)
					} else {
						i++
					}
				}
			}
		}
		time.Sleep(time.Second * 1)
	}
}
