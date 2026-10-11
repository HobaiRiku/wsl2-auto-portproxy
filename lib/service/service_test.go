package service

import "testing"

func TestNeededPortsUsesProtocolAndLocalMapping(t *testing.T) {
	linux := []Port{{Type: "tcp", Port: 22, ProxyPort: 666}}
	for _, tc := range []struct {
		name     string
		occupied Port
		want     int
	}{
		{"remote port is irrelevant", Port{Type: "tcp", Port: 22}, 1},
		{"mapped local port conflicts", Port{Type: "tcp", Port: 666}, 0},
		{"different protocol can coexist", Port{Type: "udp", Port: 666}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := GetNeededProxyPorts(linux, []Port{tc.occupied}); len(got) != tc.want {
				t.Fatalf("got %v, want %d routes", got, tc.want)
			}
		})
	}
}
