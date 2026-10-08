package proxy

import (
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"time"
)

type Proxy struct {
	Type      string
	Port      int64
	ProxyPort int64
	Listener  *net.TCPListener
	IsRunning bool
	WslIp     string
}

func (p *Proxy) Start() error {
	localAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf(":%d", p.ProxyPort))
	if err != nil {
		log.Printf("resove local Addr error,%s\n", err)
		return err
	}
	ln, err := net.ListenTCP("tcp", localAddr)
	if err != nil {
		log.Printf("Could not start proxy server on %d: %v\n", p.Port, err)
		return err
	}
	p.Listener = ln
	// fixed for this listener's lifetime, a new wsl ip means Stop and Start again
	target := net.JoinHostPort(p.WslIp, strconv.FormatInt(p.Port, 10))
	log.Printf("new proxy start in port:%d->%d", p.ProxyPort, p.Port)
	go func() {
		for {
			conn, err := ln.AcceptTCP()
			if err != nil {
				log.Println("Could not accept client connection:", err)
				break
			}
			go p.handleTCPConn(conn, target, 5)
		}
	}()
	p.IsRunning = true
	return nil
}

func (p *Proxy) Stop() error {
	p.IsRunning = false
	log.Printf("proxy stop, port:%d->%d", p.ProxyPort, p.Port)
	return p.Listener.Close()
}

func (p *Proxy) handleTCPConn(conn *net.TCPConn, targetAddr string, timeout int64) {
	// close the client connection on every return path, including dial failures
	defer conn.Close()
	log.Printf("Client '%v' connected!\n", conn.RemoteAddr())

	if !Allowed(p.ProxyPort, conn.RemoteAddr()) {
		log.Printf("Client '%v' rejected by allowlist of port %d\n", conn.RemoteAddr(), p.ProxyPort)
		return
	}

	_ = conn.SetKeepAlive(true)
	_ = conn.SetKeepAlivePeriod(time.Second * 15)
	c, err := net.DialTimeout("tcp", targetAddr, time.Duration(timeout)*time.Second)
	if err != nil {
		log.Println("Could not connect to remote server:", err)
		return
	}
	client := c.(*net.TCPConn)
	defer client.Close()
	log.Printf("Connection to server '%v' established!\n", client.RemoteAddr())

	_ = client.SetKeepAlive(true)
	_ = client.SetKeepAlivePeriod(time.Second * 15)

	// Copy each direction until EOF, then forward the FIN with CloseWrite so a
	// half-closed peer still gets the rest of the other side's data. Both
	// connections are closed only once both directions finish, or as soon as
	// either one fails. Buffered so the second goroutine can exit after an
	// early return.
	errc := make(chan error, 2)
	pipe := func(dst, src *net.TCPConn) {
		_, err := io.Copy(dst, src)
		if err == nil {
			err = dst.CloseWrite()
		}
		errc <- err
	}
	go pipe(client, conn)
	go pipe(conn, client)

	for i := 0; i < 2; i++ {
		if err := <-errc; err != nil {
			log.Println(err)
			return
		}
	}
}
