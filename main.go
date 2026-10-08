package main

import (
	"flag"
	"fmt"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/config"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/proxy"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/service"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/storage"
	"log"
	"os"
	"time"
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
	// config is handed over by the channel, so only this loop touches storage.C
	ready := make(chan configResult)
	// get config interval
	go func() {
		for {
			c, err := config.GetConfig()
			ready <- configResult{c, err}
			time.Sleep(time.Second)
		}
	}()

	configLoaded := false
	wslRunning := true
	for {
		// wait for a config update interval
		res := <-ready
		if res.err != nil {
			log.Printf("error getting config file: %s", res.err)
			if !configLoaded {
				// never start without a valid config, it may restrict access by allowlist
				continue
			}
		} else {
			storage.C = res.c
			configLoaded = true
		}
		proxy.SetAllowlist(storage.C.Allowlist.Tcp)
		// don't touch wsl when it's stopped, `wsl -- <cmd>` below would boot it again
		running, err := service.IsWslRunning()
		if err != nil {
			log.Printf("check wsl state error: %s, retrying", err)
			continue // keep current proxies, a failed check doesn't mean wsl stopped
		}
		if !running {
			if wslRunning {
				log.Println("wsl is not running, stop all proxies and wait for it")
			}
			wslRunning = false
			stopAllProxies()
			continue
		}
		if !wslRunning {
			log.Println("wsl is running")
		}
		wslRunning = true
		// get linux's ip
		wslIp, err := service.GetWslIP()
		if err != nil {
			log.Printf("GetWslIP error: %s, retrying", err)
			continue
		}
		if storage.WslIp != "" && wslIp != storage.WslIp {
			// restart everything here: running proxies are listed by netstat as
			// windows ports, so they are never in needPorts to be updated below
			log.Printf("wsl ip changed from %s to %s, restart all proxies", storage.WslIp, wslIp)
			stopAllProxies()
		}
		storage.WslIp = wslIp
		// get all tcp ports in linux now
		linuxPorts, err := service.GetLinuxHostPorts()
		if err != nil {
			log.Printf("GetLinuxHostPorts error: %s, retrying", err)
			continue // Skipping current loop is Necessary. Otherwise, running port will be stopped.
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
				newProxy := &proxy.Proxy{Port: port.Port, ProxyPort: port.ProxyPort, Type: port.Type, WslIp: storage.WslIp}
				err := newProxy.Start()
				if err != nil {
					log.Printf("start proxy error,%s\n", err)
				}
				storage.ProxyPool = append(storage.ProxyPool, newProxy)
			}
		}
		// check for delete update
		for i := 0; i < len(storage.ProxyPool); {
			needToDelete := true
			for _, port := range linuxPorts {
				if port.Port == storage.ProxyPool[i].Port &&
					port.ProxyPort == storage.ProxyPool[i].ProxyPort {
					needToDelete = false
					break
				}
			}
			if needToDelete {
				_ = storage.ProxyPool[i].Stop()
			}
			// delete
			if !storage.ProxyPool[i].IsRunning {
				storage.ProxyPool = append(storage.ProxyPool[:i], storage.ProxyPool[i+1:]...)
			} else {
				i++
			}
		}
		time.Sleep(time.Second * 1)
	}
}

type configResult struct {
	c   config.Config
	err error
}

func stopAllProxies() {
	for _, p := range storage.ProxyPool {
		if p.IsRunning {
			_ = p.Stop()
		}
	}
	storage.ProxyPool = nil
}
