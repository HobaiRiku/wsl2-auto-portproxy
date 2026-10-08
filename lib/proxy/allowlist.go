package proxy

import (
	"net"
	"sync"
)

// allowlist holds the client IP restrictions per windows listen port,
// it is replaced by the main loop on every config reload and read by
// every incoming connection.
var allowlist = struct {
	sync.RWMutex
	rules map[int64][]*net.IPNet
}{}

// SetAllowlist replaces the client IP restrictions, keyed by windows listen port.
func SetAllowlist(rules map[int64][]*net.IPNet) {
	allowlist.Lock()
	allowlist.rules = rules
	allowlist.Unlock()
}

// Allowed reports whether a client may use the proxy listening on port.
// Ports without rules are open to everyone, loopback clients are always allowed.
func Allowed(port int64, addr net.Addr) bool {
	allowlist.RLock()
	nets, restricted := allowlist.rules[port]
	allowlist.RUnlock()
	if !restricted {
		return true
	}
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return false
	}
	if tcpAddr.IP.IsLoopback() {
		return true
	}
	for _, n := range nets {
		if n.Contains(tcpAddr.IP) {
			return true
		}
	}
	return false
}
