package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Proxy owns one listening endpoint. Configure fields before Start; mutate live
// admission rules with SetPolicy. Stop cancels dialing and joins every worker.
type Proxy struct {
	Type           string
	Port           int64
	ProxyPort      int64
	WslIp          string
	ListenHost     string
	MaxConnections int
	UDPIdleTimeout time.Duration
	DialTimeout    time.Duration
	op             sync.Mutex
	mu             sync.Mutex
	run            *run
	policy         []*net.IPNet
	restricted     bool
}
type run struct {
	ctx        context.Context
	cancel     context.CancelFunc
	once       sync.Once
	mu         sync.Mutex
	closed     bool
	listener   net.Listener
	packet     *net.UDPConn
	target     string
	limit      int
	idle       time.Duration
	timeout    time.Duration
	clients    map[*session]struct{}
	udp        map[string]*session
	policy     []*net.IPNet
	restricted bool
	wg         sync.WaitGroup
	bytesIn    atomic.Uint64
	bytesOut   atomic.Uint64
	dropped    atomic.Uint64
	lastError  string
}
type session struct {
	client   net.Conn
	upstream net.Conn
	peer     net.Addr
	key      string
	last     time.Time
	queue    chan []byte
	done     chan struct{}
	once     sync.Once
}

func (s *session) close() {
	s.once.Do(func() {
		close(s.done)
		if s.client != nil {
			s.client.Close()
		}
		if s.upstream != nil {
			s.upstream.Close()
		}
	})
}

type Stats struct {
	ActiveConnections int    `json:"activeConnections"`
	BytesIn           uint64 `json:"bytesIn"`
	BytesOut          uint64 `json:"bytesOut"`
	Dropped           uint64 `json:"dropped"`
	Error             string `json:"error,omitempty"`
}

type countingWriter struct {
	writer  io.Writer
	counter *atomic.Uint64
}

func (w countingWriter) Write(data []byte) (int, error) {
	n, err := w.writer.Write(data)
	w.counter.Add(uint64(n))
	return n, err
}

func (p *Proxy) Start() error { return p.StartContext(context.Background()) }
func (p *Proxy) StartContext(parent context.Context) error {
	p.op.Lock()
	defer p.op.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.run != nil {
		p.run.mu.Lock()
		running := !p.run.closed
		p.run.mu.Unlock()
		if running {
			return errors.New("proxy already started")
		}
		p.run.wg.Wait()
	}
	if err := parent.Err(); err != nil {
		return err
	}
	if p.Type != "tcp" && p.Type != "udp" {
		return fmt.Errorf("unsupported protocol %q", p.Type)
	}
	if p.Port <= 0 || p.Port > 65535 || p.ProxyPort < 0 || p.ProxyPort > 65535 {
		return errors.New("invalid proxy port")
	}
	if net.ParseIP(p.WslIp) == nil {
		return errors.New("target must be an IP address")
	}
	host := p.ListenHost
	if host == "" {
		host = "0.0.0.0"
	}
	ctx, cancel := context.WithCancel(parent)
	r := &run{ctx: ctx, cancel: cancel, target: net.JoinHostPort(p.WslIp, strconv.FormatInt(p.Port, 10)), limit: p.MaxConnections, idle: p.UDPIdleTimeout, timeout: p.DialTimeout, clients: map[*session]struct{}{}, udp: map[string]*session{}, policy: cloneNets(p.policy), restricted: p.restricted}
	if r.limit <= 0 {
		r.limit = 128
	}
	if r.idle <= 0 {
		r.idle = 60 * time.Second
	}
	if r.timeout <= 0 {
		r.timeout = 5 * time.Second
	}
	address := net.JoinHostPort(host, strconv.FormatInt(p.ProxyPort, 10))
	if p.Type == "tcp" {
		ln, err := net.Listen("tcp", address)
		if err != nil {
			cancel()
			return err
		}
		r.listener = ln
	} else {
		addr, err := net.ResolveUDPAddr("udp", address)
		if err != nil {
			cancel()
			return err
		}
		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			cancel()
			return err
		}
		r.packet = conn
	}
	p.run = r
	r.wg.Add(1)
	if p.Type == "tcp" {
		go r.acceptTCP()
	} else {
		go r.acceptUDP()
	}
	context.AfterFunc(ctx, r.shutdown)
	return nil
}
func (p *Proxy) Stop() error {
	p.op.Lock()
	defer p.op.Unlock()
	p.mu.Lock()
	r := p.run
	p.mu.Unlock()
	if r != nil {
		r.cancel()
		r.shutdown()
		r.wg.Wait()
	}
	return nil
}
func (p *Proxy) Running() bool {
	p.mu.Lock()
	r := p.run
	p.mu.Unlock()
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.closed
}
func (p *Proxy) Addr() net.Addr {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.run == nil {
		return nil
	}
	if p.run.listener != nil {
		return p.run.listener.Addr()
	}
	return p.run.packet.LocalAddr()
}
func (p *Proxy) Stats() Stats {
	p.mu.Lock()
	r := p.run
	p.mu.Unlock()
	if r == nil {
		return Stats{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return Stats{ActiveConnections: len(r.clients), BytesIn: r.bytesIn.Load(), BytesOut: r.bytesOut.Load(), Dropped: r.dropped.Load(), Error: r.lastError}
}
func (p *Proxy) SetPolicy(nets []*net.IPNet, restricted bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.policy = cloneNets(nets)
	p.restricted = restricted
	r := p.run
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policy = cloneNets(nets)
	r.restricted = restricted
	for s := range r.clients {
		if !r.allowed(s.peer) {
			s.close()
		}
	}
}
func (r *run) allowed(peer net.Addr) bool {
	return allowedBy(r.policy, r.restricted, peer)
}
func (r *run) shutdown() {
	r.once.Do(func() {
		r.mu.Lock()
		r.closed = true
		for s := range r.clients {
			s.close()
		}
		r.mu.Unlock()
		if r.listener != nil {
			r.listener.Close()
		}
		if r.packet != nil {
			r.packet.Close()
		}
	})
}
func (r *run) recordError(err error) {
	if err == nil || r.ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
		return
	}
	r.mu.Lock()
	r.lastError = err.Error()
	r.mu.Unlock()
}
func (r *run) remove(s *session) {
	s.close()
	r.mu.Lock()
	delete(r.clients, s)
	if s.key != "" && r.udp[s.key] == s {
		delete(r.udp, s.key)
	}
	r.mu.Unlock()
}
func (r *run) acceptTCP() {
	defer r.wg.Done()
	defer r.cancel()
	defer r.shutdown()
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			r.recordError(err)
			return
		}
		r.mu.Lock()
		if r.closed || !r.allowed(conn.RemoteAddr()) || len(r.clients) >= r.limit {
			r.dropped.Add(1)
			r.mu.Unlock()
			conn.Close()
			continue
		}
		s := &session{client: conn, peer: conn.RemoteAddr(), done: make(chan struct{})}
		r.clients[s] = struct{}{}
		r.wg.Add(1)
		r.mu.Unlock()
		go r.forwardTCP(s)
	}
}
func (r *run) forwardTCP(s *session) {
	defer r.wg.Done()
	defer r.remove(s)
	upstream, err := (&net.Dialer{Timeout: r.timeout, KeepAlive: 15 * time.Second}).DialContext(r.ctx, "tcp", r.target)
	if err != nil {
		r.recordError(err)
		return
	}
	r.mu.Lock()
	select {
	case <-s.done:
		r.mu.Unlock()
		upstream.Close()
		return
	default:
	}
	s.upstream = upstream
	r.mu.Unlock()
	if c, ok := s.client.(*net.TCPConn); ok {
		c.SetKeepAlive(true)
		c.SetKeepAlivePeriod(15 * time.Second)
	}
	results := make(chan error, 2)
	copyHalf := func(dst, src net.Conn, counter *atomic.Uint64) {
		_, err := io.Copy(countingWriter{dst, counter}, src)
		if c, ok := dst.(interface{ CloseWrite() error }); ok {
			c.CloseWrite()
		}
		results <- err
	}
	go copyHalf(upstream, s.client, &r.bytesIn)
	go copyHalf(s.client, upstream, &r.bytesOut)
	first := <-results
	if first != nil {
		s.close()
		r.recordError(first)
	}
	r.recordError(<-results)
}
