package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fixtureRunner struct {
	mode    string
	modeErr error
	running string
	fail    bool
	calls   []string
}

func (r *fixtureRunner) Output(ctx context.Context, args ...string) ([]byte, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch strings.Join(args, " ") {
	case "--list --verbose":
		return []byte(" NAME STATE VERSION\n* Ubuntu Test Running 2\n"), nil
	case "--list --running --quiet":
		if r.fail {
			return nil, errors.New("failed state query")
		}
		return []byte(r.running), nil
	case "--distribution Ubuntu Test --exec /usr/lib/wsl/wslinfo --networking-mode":
		if r.modeErr != nil {
			return nil, r.modeErr
		}
		if r.mode != "" {
			return []byte(r.mode), nil
		}
		return []byte("nat\n"), nil
	case "--distribution Ubuntu Test --exec ip -j -4 address show dev eth0":
		return []byte(`[{"addr_info":[{"family":"inet","local":"172.20.1.2","scope":"global"}]}]`), nil
	case "--distribution Ubuntu Test --exec ss -H -l -n -t -u":
		return []byte("tcp LISTEN 0 128 0.0.0.0:22 0.0.0.0:*\n"), nil
	}
	return nil, errors.New("unexpected command")
}
func TestStoppedWSLDoesNotExecuteLinux(t *testing.T) {
	r := &fixtureRunner{}
	state, err := (Scanner{Runner: r}).Scan(context.Background())
	if err != nil || state.State != "stopped" {
		t.Fatalf("%+v %v", state, err)
	}
	for _, call := range r.calls {
		if strings.Contains(call, "--exec") {
			t.Fatalf("stopped WSL would be booted: %s", call)
		}
	}
}
func TestStateErrorIsNotStopped(t *testing.T) {
	r := &fixtureRunner{fail: true}
	state, err := (Scanner{Runner: r}).Scan(context.Background())
	if err == nil || state.State != "unknown" {
		t.Fatalf("%+v %v", state, err)
	}
}
func TestScannerUsesExplicitDefaultDistribution(t *testing.T) {
	r := &fixtureRunner{running: "Ubuntu Test\r\n"}
	state, err := (Scanner{Runner: r}).Scan(context.Background())
	if err != nil || state.IP != "172.20.1.2" || len(state.Ports) != 1 {
		t.Fatalf("%+v %v", state, err)
	}
}
func TestParseLinuxPortsProtocolAndWildcards(t *testing.T) {
	out := "tcp LISTEN 0 128 0.0.0.0:7 0.0.0.0:*\nudp UNCONN 0 0 *:7 *:*\ntcp LISTEN 0 128 [::]:7 [::]:*\ntcp LISTEN 0 128 127.0.0.1:80 0.0.0.0:*\n"
	got, err := ParseLinuxPorts(out)
	want := []Port{{Type: "tcp", Port: 7, ProxyPort: 7}, {Type: "udp", Port: 7, ProxyPort: 7}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%v %v", got, err)
	}
	for _, bad := range []string{"truncated", "tcp LISTEN 0 128 *:70000 *:*"} {
		if _, err := ParseLinuxPorts(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestMirroredDoesNotDiscoverNATTargets(t *testing.T) {
	r := &fixtureRunner{running: "Ubuntu Test", mode: "mirrored"}
	snapshot, err := (Scanner{Runner: r}).Scan(context.Background())
	if err != nil || snapshot.NetworkMode != "mirrored" || snapshot.IP != "" || len(snapshot.Ports) != 0 {
		t.Fatalf("%+v: %v", snapshot, err)
	}
	for _, call := range r.calls {
		if strings.Contains(call, "--exec ip ") || strings.Contains(call, "--exec ss ") {
			t.Fatal("probed NAT target in mirrored mode")
		}
	}
}
func TestLegacyNATRequiresExplicitOptIn(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		r := &fixtureRunner{running: "Ubuntu Test", modeErr: errors.New("wslinfo missing")}
		snapshot, err := (Scanner{Runner: r, LegacyNAT: legacy}).Scan(context.Background())
		if legacy {
			if err != nil || snapshot.NetworkMode != "nat" || snapshot.IP == "" {
				t.Fatalf("%+v: %v", snapshot, err)
			}
		} else if err == nil || snapshot.NetworkMode != "unknown" || snapshot.IP != "" {
			t.Fatal("guessed NAT without explicit opt-in")
		}
	}
}
