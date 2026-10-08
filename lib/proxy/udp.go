package proxy

import (
	"errors"
	"net"
	"time"
)

// One connected upstream socket per client keeps replies and source ports
// isolated. A bounded send queue prevents a slow upstream blocking all clients.
func (r *run) acceptUDP() {
	defer r.wg.Done()
	defer r.cancel()
	defer r.shutdown()
	target, err := net.ResolveUDPAddr("udp", r.target)
	if err != nil {
		r.recordError(err)
		return
	}
	r.wg.Add(1)
	go r.expireUDP()
	buffer := make([]byte, 65536)
	for {
		n, peer, err := r.packet.ReadFromUDP(buffer)
		if err != nil {
			if r.ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			r.recordError(err)
			r.dropped.Add(1)
			continue
		}
		r.mu.Lock()
		if r.closed || !r.allowed(peer) {
			r.dropped.Add(1)
			r.mu.Unlock()
			continue
		}
		key := peer.String()
		s := r.udp[key]
		if s != nil {
			select {
			case <-s.done:
				delete(r.udp, key)
				delete(r.clients, s)
				s = nil
			default:
			}
		}
		if s == nil {
			if len(r.clients) >= r.limit {
				r.dropped.Add(1)
				r.mu.Unlock()
				continue
			}
			upstream, err := net.DialUDP("udp", nil, target)
			if err != nil {
				r.lastError = err.Error()
				r.dropped.Add(1)
				r.mu.Unlock()
				continue
			}
			s = &session{upstream: upstream, peer: peer, key: key, last: time.Now(), queue: make(chan []byte, 8), done: make(chan struct{})}
			r.udp[key] = s
			r.clients[s] = struct{}{}
			r.wg.Add(2)
			go r.sendUDP(s)
			go r.receiveUDP(s)
		}
		s.last = time.Now()
		packet := append([]byte{}, buffer[:n]...)
		select {
		case s.queue <- packet:
		default:
			r.dropped.Add(1)
		}
		r.mu.Unlock()
	}
}
func (r *run) sendUDP(s *session) {
	defer r.wg.Done()
	defer r.remove(s)
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-s.done:
			return
		case packet := <-s.queue:
			s.upstream.SetWriteDeadline(time.Now().Add(r.timeout))
			n, err := s.upstream.Write(packet)
			if err != nil || n != len(packet) {
				r.recordError(err)
				r.dropped.Add(1)
				return
			}
			r.bytesIn.Add(uint64(n))
		}
	}
}
func (r *run) receiveUDP(s *session) {
	defer r.wg.Done()
	defer r.remove(s)
	buffer := make([]byte, 65536)
	for {
		n, err := s.upstream.Read(buffer)
		if err != nil {
			r.recordError(err)
			return
		}
		r.mu.Lock()
		if r.closed || !r.allowed(s.peer) {
			r.mu.Unlock()
			return
		}
		s.last = time.Now()
		r.mu.Unlock()
		written, err := r.packet.WriteTo(buffer[:n], s.peer)
		if err != nil || written != n {
			r.recordError(err)
			r.dropped.Add(1)
			return
		}
		r.bytesOut.Add(uint64(written))
	}
}
func (r *run) expireUDP() {
	defer r.wg.Done()
	period := r.idle / 2
	if period > time.Second {
		period = time.Second
	}
	if period < time.Millisecond {
		period = time.Millisecond
	}
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case now := <-ticker.C:
			r.mu.Lock()
			for _, s := range r.udp {
				if now.Sub(s.last) >= r.idle {
					s.close()
				}
			}
			r.mu.Unlock()
		}
	}
}
