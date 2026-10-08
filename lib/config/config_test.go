package config

import (
	"encoding/json"
	"net"
	"testing"
)

func TestAllowlistUnmarshal(t *testing.T) {
	var c Config
	err := json.Unmarshal([]byte(`{
		"allowlist": {"tcp": {"22": ["192.168.1.0/24", "10.0.0.5", "fe80::1"]}}
	}`), &c)
	if err != nil {
		t.Fatal(err)
	}
	nets := c.Allowlist.Tcp[22]
	if len(nets) != 3 {
		t.Fatalf("got %d rules, want 3", len(nets))
	}
	if !nets[0].Contains(net.ParseIP("192.168.1.200")) {
		t.Error("CIDR rule should contain 192.168.1.200")
	}
	if !nets[1].Contains(net.ParseIP("10.0.0.5")) || nets[1].Contains(net.ParseIP("10.0.0.6")) {
		t.Error("single IP rule should match only itself")
	}
	if !nets[2].Contains(net.ParseIP("fe80::1")) {
		t.Error("IPv6 rule should match itself")
	}
}

func TestAllowlistUnmarshalInvalid(t *testing.T) {
	for _, data := range []string{
		`{"allowlist": {"tcp": {"abc": ["10.0.0.1"]}}}`,
		`{"allowlist": {"tcp": {"70000": ["10.0.0.1"]}}}`,
		`{"allowlist": {"tcp": {"22": ["10.0.0.300"]}}}`,
		`{"allowlist": {"tcp": {"22": ["10.0.0.0/33"]}}}`,
	} {
		var c Config
		if err := json.Unmarshal([]byte(data), &c); err == nil {
			t.Errorf("expected error for %s", data)
		}
	}
}

func TestPortProxyMarshal(t *testing.T) {
	b, err := json.Marshal(PortProxy{Local: 666, Remote: 22})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"666:22"` {
		t.Errorf("got %s", b)
	}
}

func TestAllowlistUnmarshalDuplicatePort(t *testing.T) {
	var c Config
	err := json.Unmarshal([]byte(`{"allowlist": {"tcp": {"666": ["10.0.0.0/8"], "0666": ["192.168.1.5"]}}}`), &c)
	if err == nil {
		t.Error("expected error for duplicate port")
	}
}
