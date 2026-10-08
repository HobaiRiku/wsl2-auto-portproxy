package controller

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/registry"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/config"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/proxy"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/service"
)

type Scanner interface {
	Scan(context.Context) (service.Snapshot, error)
}
type Route struct {
	Distro   string
	Protocol string
	Listen   string
	Target   string
	Remote   int64
	Local    int64
	Host     string
	Max      int
	Idle     time.Duration
}

func (r Route) ID() string { return r.Protocol + "/" + r.Listen }

type ProxyStatus struct {
	ID       string `json:"id"`
	Protocol string `json:"protocol"`
	Listen   string `json:"listen"`
	Target   string `json:"target"`
	State    string `json:"state"`
	proxy.Stats
}
type Status struct {
	WSL         service.Snapshot `json:"wsl"`
	Proxies     []ProxyStatus    `json:"proxies"`
	ConfigError string           `json:"configError,omitempty"`
}
type resource struct {
	route   Route
	proxy   *proxy.Proxy
	err     string
	retry   time.Time
	backoff time.Duration
}
type Controller struct {
	mu             sync.Mutex
	registry       *registry.Registry
	scanner        Scanner
	logger         *slog.Logger
	resources      map[string]*resource
	observed       service.Snapshot
	last           service.Snapshot
	lastOK         time.Time
	problem        string
	managementPort int
}

func New(r *registry.Registry, s Scanner, logger *slog.Logger, managementPort int) *Controller {
	return &Controller{registry: r, scanner: s, logger: logger, resources: map[string]*resource{}, observed: service.Snapshot{State: "unknown", NetworkMode: "unknown"}, managementPort: managementPort}
}
func Plan(c config.Config, s service.Snapshot) []Route {
	out := map[string]Route{}
	host := c.ListenAddress
	if host == "" {
		host = "0.0.0.0"
	}
	ignored := func(protocol string, port int64) bool {
		ports := c.Ignore.Tcp
		if protocol == "udp" {
			ports = c.Ignore.Udp
		}
		for _, p := range ports {
			if p == port {
				return true
			}
		}
		return false
	}
	route := func(p service.Port, local int64) Route {
		return Route{Distro: s.Distro, Protocol: p.Type, Listen: net.JoinHostPort(host, strconv.FormatInt(local, 10)), Target: net.JoinHostPort(s.IP, strconv.FormatInt(p.Port, 10)), Host: host, Remote: p.Port, Local: local, Max: c.MaxConnections, Idle: time.Duration(c.UDPIdleSeconds) * time.Second}
	}
	// Explicit mappings take precedence over auto-discovered same-number ports.
	for _, p := range s.Ports {
		if p.Type == "udp" && !c.UDPEnabled || ignored(p.Type, p.Port) {
			continue
		}
		mappings := c.Predefined.Tcp
		if p.Type == "udp" {
			mappings = c.Predefined.Udp
		}
		for _, m := range mappings {
			if m.Remote == p.Port {
				r := route(p, m.Local)
				out[r.ID()] = r
			}
		}
	}
	if !c.OnlyPredefined {
		for _, p := range s.Ports {
			if p.Type == "udp" && !c.UDPEnabled || ignored(p.Type, p.Port) {
				continue
			}
			mapped := false
			mappings := c.Predefined.Tcp
			if p.Type == "udp" {
				mappings = c.Predefined.Udp
			}
			for _, m := range mappings {
				if m.Remote == p.Port {
					mapped = true
				}
			}
			if mapped {
				continue
			}
			r := route(p, p.Port)
			if _, exists := out[r.ID()]; !exists {
				out[r.ID()] = r
			}
		}
	}
	routes := make([]Route, 0, len(out))
	for _, r := range out {
		routes = append(routes, r)
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].ID() < routes[j].ID() })
	return routes
}
func (c *Controller) Run(ctx context.Context) {
	defer c.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		c.registry.Reload()
		snapshot, err := c.scanner.Scan(ctx)
		if ctx.Err() != nil {
			return
		}
		c.Reconcile(ctx, snapshot, err, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-c.registry.Wake():
		}
	}
}
func (c *Controller) Reconcile(ctx context.Context, s service.Snapshot, scanErr error, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	doc, ready, problem := c.registry.Snapshot()
	c.problem = problem
	if scanErr != nil {
		if c.observed.Error != scanErr.Error() {
			c.logger.Warn("WSL discovery failed", "error", scanErr)
		}
		if s.Distro != "" || c.lastOK.IsZero() {
			c.observed = s
		}
		c.observed.State = "unknown"
		c.observed.Error = scanErr.Error()
		if c.lastOK.IsZero() || now.Sub(c.lastOK) > 15*time.Second {
			c.stopLocked()
			return
		}
		s = c.last
	} else {
		c.observed = s
		c.observed.Error = ""
		c.last = s
		c.lastOK = now
	}
	if !ready || s.State != "running" || s.NetworkMode != "nat" || net.ParseIP(s.IP) == nil {
		c.stopLocked()
		return
	}
	wanted := map[string]Route{}
	for _, r := range Plan(doc.Config, s) {
		wanted[r.ID()] = r
	}
	for id, entry := range c.resources {
		next, exists := wanted[id]
		if !exists || entry.route != next {
			if entry.proxy != nil {
				entry.proxy.Stop()
			}
			delete(c.resources, id)
		}
	}
	for id, route := range wanted {
		entry := c.resources[id]
		if entry == nil {
			entry = &resource{route: route}
			c.resources[id] = entry
		}
		rules := doc.Config.Allowlist.Tcp
		if route.Protocol == "udp" {
			rules = doc.Config.Allowlist.Udp
		}
		nets, restricted := rules[route.Local]
		if entry.proxy != nil {
			entry.proxy.SetPolicy(nets, restricted)
			if entry.proxy.Running() {
				continue
			}
			entry.proxy.Stop()
			entry.proxy = nil
		}
		if now.Before(entry.retry) {
			continue
		}
		if route.Protocol == "tcp" && int(route.Local) == c.managementPort {
			entry.err = "port reserved for the management API"
			continue
		}
		p := &proxy.Proxy{Type: route.Protocol, Port: route.Remote, ProxyPort: route.Local, WslIp: s.IP, ListenHost: route.Host, MaxConnections: route.Max, UDPIdleTimeout: route.Idle}
		p.SetPolicy(nets, restricted)
		if err := p.StartContext(ctx); err != nil {
			entry.err = err.Error()
			if entry.backoff == 0 {
				entry.backoff = time.Second
			} else {
				entry.backoff *= 2
			}
			if entry.backoff > 30*time.Second {
				entry.backoff = 30 * time.Second
			}
			entry.retry = now.Add(entry.backoff)
			c.logger.Warn("proxy blocked", "listen", route.Listen, "protocol", route.Protocol, "error", err)
			continue
		}
		entry.proxy = p
		entry.err = ""
		entry.backoff = 0
		c.logger.Info("proxy listening", "protocol", route.Protocol, "listen", route.Listen, "target", route.Target)
	}
}
func (c *Controller) stopLocked() {
	for id, entry := range c.resources {
		if entry.proxy != nil {
			entry.proxy.Stop()
		}
		delete(c.resources, id)
	}
}
func (c *Controller) Stop() { c.mu.Lock(); defer c.mu.Unlock(); c.stopLocked() }
func (c *Controller) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := Status{WSL: c.observed, Proxies: make([]ProxyStatus, 0, len(c.resources)), ConfigError: c.problem}
	for id, entry := range c.resources {
		state := "blocked"
		stats := proxy.Stats{Error: entry.err}
		if entry.proxy != nil {
			stats = entry.proxy.Stats()
			if entry.proxy.Running() {
				state = "active"
			} else {
				state = "error"
			}
			if c.observed.State == "unknown" {
				state = "stale"
			}
		}
		out.Proxies = append(out.Proxies, ProxyStatus{ID: id, Protocol: entry.route.Protocol, Listen: entry.route.Listen, Target: entry.route.Target, State: state, Stats: stats})
	}
	sort.Slice(out.Proxies, func(i, j int) bool { return out.Proxies[i].ID < out.Proxies[j].ID })
	return out
}
func (c *Controller) String() string {
	s := c.Status()
	return fmt.Sprintf("WSL %s (%s), %d proxies", s.WSL.Distro, s.WSL.State, len(s.Proxies))
}
