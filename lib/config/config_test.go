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

func TestPortProxyRejectsMalformedMappings(t *testing.T) {
	for _, raw := range []string{`"22"`, `""`, `"666:22:80"`, `"0:22"`, `"-1:22"`, `"70000:22"`, `"22:65536"`, `"22:0"`, `null`, `22`} {
		t.Run(raw, func(t *testing.T) {
			var p PortProxy
			if err := json.Unmarshal([]byte(raw), &p); err == nil {
				t.Fatalf("accepted invalid mapping %s", raw)
			}
		})
	}
}

func TestParseRejectsAmbiguousOrInvalidConfig(t *testing.T) {
	for _, raw := range []string{
		`{"onlyPredefined":true,"OnlyPredefined":false}`,
		`{"allowlist":{"tcp":{"22":[],"22":["10.0.0.0/8"]}}}`,
		`{"predefined":{"tcp":["666:22","666:80"]}}`,
		`{"listenAddress":"localhost"}`, `{"schemaVersion":2}`,
		`{"maxConnections":1025}`, `{"udpIdleSeconds":-1}`,
		`{"ignore":{"udp":[65536]}}`, `{"typo":true}`, `{} {}`,
		`[]`, `null`, ``, `{"onlyPredefined":true,`, `/* unfinished`,
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := Parse([]byte(raw)); err == nil {
				t.Fatal("accepted invalid configuration")
			}
		})
	}
}
func TestLegacyCommentsAndProtocolIsolation(t *testing.T) {
	c, err := Parse([]byte(`{/* old config */ "predefined":{"tcp":["666:22"],"udp":["666:53"]},"allowlist":{"udp":{"666":[]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	roundtrip, err := Parse(data)
	if err != nil || roundtrip.Predefined.Udp[0].Remote != 53 {
		t.Fatalf("roundtrip: %v", err)
	}
	if string(stripComments([]byte(`"literal /* comment */ and \\"`))) != `"literal /* comment */ and \\"` {
		t.Fatal("changed quoted string")
	}
}
func FuzzParseDoesNotPanic(f *testing.F) {
	for _, raw := range []string{`{}`, `{"predefined":{"tcp":["22"]}}`, `{"allowlist":{"udp":{"22":[]}}}`, `{"x":{]`} {
		f.Add([]byte(raw))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		c, err := Parse(raw)
		if err != nil {
			return
		}
		data, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Parse(data); err != nil {
			t.Fatalf("accepted config cannot roundtrip: %v", err)
		}
	})
}
