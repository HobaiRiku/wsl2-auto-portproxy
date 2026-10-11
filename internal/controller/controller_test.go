package controller

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/registry"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/config"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/service"
)

func TestPlanMappingsProtocolsAndPriority(t *testing.T) {
	c := config.Config{UDPEnabled: true, Predefined: config.PredefinedPorts{Tcp: []config.PortProxy{{Local: 666, Remote: 22}, {Local: 667, Remote: 22}}}, Ignore: config.IgnorePorts{Udp: []int64{53}}}
	s := service.Snapshot{IP: "172.20.1.2", Ports: []service.Port{{Type: "tcp", Port: 22}, {Type: "udp", Port: 22}, {Type: "tcp", Port: 666}, {Type: "udp", Port: 53}}}
	got := Plan(c, s)
	if len(got) != 3 {
		t.Fatalf("routes: %+v", got)
	}
	for _, r := range got {
		if r.Protocol == "tcp" && r.Remote != 22 {
			t.Fatal("auto port overrode explicit mapping")
		}
	}
	c.OnlyPredefined = true
	if got := Plan(c, s); len(got) != 2 {
		t.Fatalf("predefined: %+v", got)
	}
	c.OnlyPredefined = false
	c.UDPEnabled = false
	if got := Plan(c, s); len(got) != 2 {
		t.Fatal("UDP enabled implicitly")
	}
}
func TestBindConflictRecoveryAndStaleShutdown(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port
	registry := registry.New(filepath.Join(t.TempDir(), "config.json"))
	doc, _, _ := registry.Snapshot()
	raw := []byte(`{"onlyPredefined":true,"listenAddress":"127.0.0.1","predefined":{"tcp":["` + strconv.Itoa(port) + `:22"]}}`)
	if _, err := registry.Replace(doc.Revision, raw); err != nil {
		t.Fatal(err)
	}
	c := New(registry, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), 0)
	defer c.Stop()
	now := time.Now()
	state := service.Snapshot{State: "running", NetworkMode: "nat", IP: "127.0.0.1", Ports: []service.Port{{Type: "tcp", Port: 22}}}
	c.Reconcile(context.Background(), state, nil, now)
	if s := c.Status(); len(s.Proxies) != 1 || s.Proxies[0].State != "blocked" || s.Discovered[0].Decision != "blocked" {
		t.Fatalf("%+v", s)
	}
	occupied.Close()
	c.Reconcile(context.Background(), state, nil, now.Add(2*time.Second))
	if s := c.Status(); s.Proxies[0].State != "active" || s.Discovered[0].Decision != "forwarded" {
		t.Fatal("conflict did not recover")
	}
	var original net.Addr
	for _, entry := range c.resources {
		original = entry.proxy.Addr()
	}
	c.Reconcile(context.Background(), state, nil, now.Add(3*time.Second))
	for _, entry := range c.resources {
		if entry.proxy.Addr() != original {
			t.Fatal("unchanged listener restarted")
		}
	}
	c.Reconcile(context.Background(), service.Snapshot{}, errors.New("scan failed"), now.Add(4*time.Second))
	if c.Status().Proxies[0].State != "stale" {
		t.Fatal("scan failure removed valid proxy immediately")
	}
	c.Reconcile(context.Background(), service.Snapshot{}, errors.New("scan failed"), now.Add(20*time.Second))
	if len(c.Status().Proxies) != 0 {
		t.Fatal("stale proxy was kept indefinitely")
	}
}
func TestExplainDecisions(t *testing.T) {
	c := config.Config{Predefined: config.PredefinedPorts{Tcp: []config.PortProxy{{Local: 80, Remote: 8080}, {Local: 2222, Remote: 22}}}, Ignore: config.IgnorePorts{Tcp: []int64{445}}}
	s := service.Snapshot{IP: "172.20.1.2", Ports: []service.Port{{Type: "tcp", Port: 80}, {Type: "tcp", Port: 445}, {Type: "tcp", Port: 3000}, {Type: "tcp", Port: 8080}, {Type: "udp", Port: 53}}}
	decisions := func() map[string]string {
		out := map[string]string{}
		for _, d := range Explain(c, s) {
			out[d.Protocol+"/"+strconv.FormatInt(d.Port, 10)] = d.Decision
		}
		return out
	}
	want := map[string]string{"tcp/80": "conflict", "tcp/445": "ignored", "tcp/3000": "forwarded", "tcp/8080": "forwarded", "tcp/22": "not-listening", "udp/53": "udp-disabled"}
	if got := decisions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	c.OnlyPredefined = true
	if got := decisions()["tcp/3000"]; got != "not-predefined" {
		t.Fatalf("onlyPredefined: %s", got)
	}
}
