package proxy

import (
	"net"
	"testing"
	"time"
)

func mustCIDR(t *testing.T, s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAllowed(t *testing.T) {
	rules := map[int64][]*net.IPNet{
		22: {mustCIDR(t, "192.168.1.0/24"), mustCIDR(t, "10.0.0.5/32")},
	}

	cases := []struct {
		port int64
		ip   string
		want bool
	}{
		{22, "192.168.1.77", true},
		{22, "10.0.0.5", true},
		{22, "10.0.0.6", false},
		{22, "::ffff:192.168.1.8", true},
		{22, "127.0.0.1", true},
		{22, "::1", true},
		{80, "8.8.8.8", true},
	}
	for _, c := range cases {
		addr := &net.TCPAddr{IP: net.ParseIP(c.ip), Port: 50000}
		nets, restricted := rules[c.port]
		if got := allowedBy(nets, restricted, addr); got != c.want {
			t.Errorf("Allowed(%d, %s) = %v, want %v", c.port, c.ip, got, c.want)
		}
	}
}

// startProxy starts a proxy on a free local port forwarding to 127.0.0.1:remotePort.
func startProxy(t *testing.T, remotePort int64) *Proxy {
	p := &Proxy{Type: "tcp", Port: remotePort, WslIp: "127.0.0.1", ListenHost: "127.0.0.1"}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Stop() })
	return p
}

// expectClosed fails unless the server closes conn within a short time.
func expectClosed(t *testing.T, conn net.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("expected connection to be closed")
	} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("connection was left open")
	}
}

func TestClientClosedWhenRemoteUnreachable(t *testing.T) {
	// reserve a port with nothing listening on it
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadPort := int64(ln.Addr().(*net.TCPAddr).Port)
	ln.Close()

	p := startProxy(t, deadPort)
	conn, err := net.Dial("tcp", p.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	expectClosed(t, conn)
}

func TestLoopbackClientBypassesAllowlist(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	p := startProxy(t, int64(backend.Addr().(*net.TCPAddr).Port))

	// the rules exclude 127.0.0.1, but loopback clients are always allowed
	p.SetPolicy([]*net.IPNet{mustCIDR(t, "203.0.113.0/24")}, true)

	go func() {
		c, err := backend.Accept()
		if err == nil {
			_, _ = c.Write([]byte("ok"))
			c.Close()
		}
	}()
	conn, err := net.Dial("tcp", p.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 2)
	if _, err := conn.Read(buf); err != nil || string(buf) != "ok" {
		t.Fatalf("loopback client should pass the allowlist, got %q, %v", buf, err)
	}
}
