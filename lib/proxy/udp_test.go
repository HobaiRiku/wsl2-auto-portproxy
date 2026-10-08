package proxy

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"
)

func udpBackend(t *testing.T) *net.UDPConn {
	t.Helper()
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}
func udpClient(t *testing.T, p *Proxy) *net.UDPConn {
	t.Helper()
	c, err := net.DialUDP("udp", nil, p.Addr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	c.SetDeadline(time.Now().Add(3 * time.Second))
	t.Cleanup(func() { c.Close() })
	return c
}
func TestUDPDatagramsAndClientIsolation(t *testing.T) {
	backend := udpBackend(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 65536)
		for {
			n, peer, err := backend.ReadFromUDP(buf)
			if err != nil {
				return
			}
			backend.WriteToUDP(buf[:n], peer)
		}
	}()
	p := &Proxy{Type: "udp", Port: int64(backend.LocalAddr().(*net.UDPAddr).Port), WslIp: "127.0.0.1", ListenHost: "127.0.0.1"}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Stop(); backend.Close(); <-done })
	a, b := udpClient(t, p), udpClient(t, p)
	for _, body := range [][]byte{nil, []byte("small"), bytes.Repeat([]byte("x"), 60000)} {
		for _, c := range []*net.UDPConn{a, b} {
			if _, err := c.Write(body); err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, 65536)
			n, err := c.Read(buf)
			if err != nil || !bytes.Equal(buf[:n], body) {
				t.Fatalf("n=%d want=%d err=%v", n, len(body), err)
			}
		}
	}
	if count := p.Stats().ActiveConnections; count != 2 {
		t.Fatalf("got %d sessions", count)
	}
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	if count := p.Stats().ActiveConnections; count != 0 {
		t.Fatalf("Stop retained %d sessions", count)
	}
}
func TestUDPStableSourceAndMultipleReplies(t *testing.T) {
	backend := udpBackend(t)
	p := &Proxy{Type: "udp", Port: int64(backend.LocalAddr().(*net.UDPAddr).Port), WslIp: "127.0.0.1", ListenHost: "127.0.0.1"}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	c := udpClient(t, p)
	buf := make([]byte, 32)
	backend.SetDeadline(time.Now().Add(3 * time.Second))
	c.Write([]byte("one"))
	_, first, err := backend.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	backend.WriteToUDP([]byte("reply1"), first)
	backend.WriteToUDP([]byte("reply2"), first)
	for _, want := range []string{"reply1", "reply2"} {
		n, err := c.Read(buf)
		if err != nil || string(buf[:n]) != want {
			t.Fatalf("got %q err=%v", buf[:n], err)
		}
	}
	c.Write([]byte("two"))
	_, second, err := backend.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	if first.String() != second.String() {
		t.Fatal("upstream source port changed")
	}
}
func TestUDPIdleExpiryAndContextStop(t *testing.T) {
	backend := udpBackend(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &Proxy{Type: "udp", Port: int64(backend.LocalAddr().(*net.UDPAddr).Port), WslIp: "127.0.0.1", ListenHost: "127.0.0.1", UDPIdleTimeout: 20 * time.Millisecond, MaxConnections: 1}
	if err := p.StartContext(ctx); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	c := udpClient(t, p)
	c.Write([]byte("first"))
	backend.SetDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := backend.ReadFromUDP(make([]byte, 8)); err != nil {
		t.Fatal(err)
	}
	limit := time.After(3 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for p.Stats().ActiveConnections != 0 {
		select {
		case <-ticker.C:
		case <-limit:
			t.Fatal("idle session did not expire")
		}
	}
	cancel()
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	if p.Running() {
		t.Fatal("proxy still running")
	}
}
