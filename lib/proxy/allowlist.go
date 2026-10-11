package proxy

import "net"

func cloneNets(nets []*net.IPNet) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(nets))
	for _, n := range nets {
		if n != nil {
			out = append(out, &net.IPNet{IP: append(net.IP(nil), n.IP...), Mask: append(net.IPMask(nil), n.Mask...)})
		}
	}
	return out
}
func allowedBy(nets []*net.IPNet, restricted bool, peer net.Addr) bool {
	if !restricted {
		return true
	}
	var ip net.IP
	switch a := peer.(type) {
	case *net.TCPAddr:
		ip = a.IP
	case *net.UDPAddr:
		ip = a.IP
	default:
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
