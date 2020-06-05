package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
	"wsl2-auto-portproxy/lib/proxy"
	"wsl2-auto-portproxy/lib/service"
	"wsl2-auto-portproxy/lib/storage"
)

var version string

func main() {
	// 输出版本命令
	var showVersion bool
	flag.BoolVar(&showVersion, "v", false, "show version")
	flag.Parse()
	if showVersion {
		fmt.Println(version)
		os.Exit(1)
	}
	for {
		// step 1: get linux's ip
		storage.WslIp, _ = service.GetWslIP()
		// step 2: get all tcp ports in linux now
		linuxPorts, err := service.GetLinuxHostPorts()
		if err != nil {
			log.Fatal(err)
		}
		// step 3: get all tcp ports in local windows now
		windowsPorts, err := service.GetWindowsHostPorts()
		if err != nil {
			log.Println(err)
		}
		// step 4: calculate which port need to proxy
		needPorts := service.GetNeededProxyPorts(linuxPorts, windowsPorts)
		// create proxy
		for _, port := range needPorts {
			omitted := false
			for _, p := range storage.ProxyPool {
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
				newProxy := proxy.Proxy{Port: port.Port, Type: port.Type, WslIp: storage.WslIp}
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
				if port.Port == p.Port {
					needToDelete = false
					break
				}
			}
			if needToDelete {
				_ = p.Stop()
			}
			// clean not running proxy
			if !p.IsRunning {
				for i, one := range storage.ProxyPool {
					if one.Port == p.Port {
						storage.ProxyPool = append(storage.ProxyPool[:i], storage.ProxyPool[i+1:]...)
					}
				}
			}
		}
		time.Sleep(time.Second * 1)
	}
}
