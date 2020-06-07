package storage

import (
	"wsl2-auto-portproxy/lib/config"
	"wsl2-auto-portproxy/lib/proxy"
)

var ProxyPool []proxy.Proxy

var WslIp string

var C config.Config
