package proxy

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

func localProxy(t *testing.T, backend net.Listener) *Proxy {
	t.Helper()
	p := &Proxy{Type: "tcp", Port: int64(backend.Addr().(*net.TCPAddr).Port), ProxyPort: 0, WslIp: "127.0.0.1"}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Stop() })
	return p
}
func dialProxy(t *testing.T, p *Proxy) *net.TCPConn {
	t.Helper()
	addr := p.Addr().(*net.TCPAddr)
	c, err := net.DialTCP("tcp", nil, &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: addr.Port})
	if err != nil {
		t.Fatal(err)
	}
	c.SetDeadline(time.Now().Add(3 * time.Second))
	t.Cleanup(func() { c.Close() })
	return c
}
func TestHalfClosePreservesResponse(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	request := bytes.Repeat([]byte("request"), 32768)
	response := bytes.Repeat([]byte("response after EOF"), 32768)
	result := make(chan error, 1)
	go func() {
		c, err := backend.Accept()
		if err != nil {
			result <- err
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(3 * time.Second))
		body, readErr := io.ReadAll(c)
		err = readErr
		if err == nil && !bytes.Equal(body, request) {
			err = io.ErrUnexpectedEOF
		}
		if err == nil {
			_, err = c.Write(response)
		}
		result <- err
	}()
	c := dialProxy(t, localProxy(t, backend))
	if _, err := c.Write(request); err != nil {
		t.Fatal(err)
	}
	c.CloseWrite()
	body, err := io.ReadAll(c)
	if err != nil || !bytes.Equal(body, response) {
		t.Fatalf("response length=%d error=%v", len(body), err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
func TestStopClosesActiveConnectionsAndCanRestart(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, err := backend.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(3 * time.Second))
		io.Copy(c, c)
	}()
	p := localProxy(t, backend)
	c := dialProxy(t, p)
	c.Write([]byte("ready"))
	buf := make([]byte, 5)
	if _, err := io.ReadFull(c, buf); err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	expectClosed(t, c)
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("backend connection did not close")
	}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
}
func TestStopBeforeStart(t *testing.T) {
	if err := new(Proxy).Stop(); err != nil {
		t.Fatal(err)
	}
}
