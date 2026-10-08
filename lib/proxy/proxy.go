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
	p.Listener, err = net.ListenTCP("tcp", localAddr)
	if err != nil {
		log.Printf("Could not start proxy server on %d: %v\n", p.Port, err)
		return err
	}
	log.Printf("new proxy start in port:%d->%d", p.ProxyPort, p.Port)
	go func() {
		for {
			conn, err := p.Listener.AcceptTCP()
			if err != nil {
				log.Println("Could not accept client connection:", err)
				break
			}
			go p.handleTCPConn(conn, 5)
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

func (p *Proxy) handleTCPConn(conn *net.TCPConn, timeout int64) {
	// close the client connection on every return path, including dial failures
	defer conn.Close()
	log.Printf("Client '%v' connected!\n", conn.RemoteAddr())

	if !Allowed(p.ProxyPort, conn.RemoteAddr()) {
		log.Printf("Client '%v' rejected by allowlist of port %d\n", conn.RemoteAddr(), p.ProxyPort)
		return
	}

	_ = conn.SetKeepAlive(true)
	_ = conn.SetKeepAlivePeriod(time.Second * 15)
	targetAddr := net.JoinHostPort(p.WslIp, strconv.FormatInt(p.Port, 10))
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

	// buffered so the second copy goroutine can exit after the first one ends the connection
	stop := make(chan bool, 2)

	go func() {
		_, err := io.Copy(client, conn)
		if err != nil {
			log.Println(err)
		}
		stop <- true
	}()

	go func() {
		_, err := io.Copy(conn, client)
		if err != nil {
			log.Println(err)
		}
		stop <- true
	}()

	<-stop
}
