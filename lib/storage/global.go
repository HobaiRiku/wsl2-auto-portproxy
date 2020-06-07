package storage

import (
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/config"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/proxy"
)

var ProxyPool []proxy.Proxy

var WslIp string

var C config.Config
