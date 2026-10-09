package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fixtureRunner struct {
	probe   string
	running string
	fail    bool
	calls   []string
}

const natProbe = "@@mode nat\n@@ip\n" +
	`[{"addr_info":[{"family":"inet","local":"172.20.1.2","scope":"global"}]}]` + "\n" +
	"@@ports\ntcp LISTEN 0 128 0.0.0.0:22 0.0.0.0:*\nudp UNCONN 0 0 172.20.1.2:53 0.0.0.0:*\n"

func (r *fixtureRunner) Output(ctx context.Context, args ...string) ([]byte, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch strings.Join(args[:min(len(args), 4)], " ") {
	case "--list --verbose":
		return []byte(" NAME STATE VERSION\n* Ubuntu Test Running 2\n  Debian Stopped 2\n  Legacy Stopped 1\n"), nil
	case "--list --running --quiet":
		if r.fail {
			return nil, errors.New("failed state query")
		}
		return []byte(r.running), nil
	case "--distribution Ubuntu Test --exec sh", "--distribution Debian --exec sh":
		if args[len(args)-1] != probeScript {
			return nil, errors.New("unexpected probe")
		}
		if r.probe != "" {
			return []byte(r.probe), nil
		}
		return []byte(natProbe), nil
	}
	return nil, errors.New("unexpected command: " + strings.Join(args, " "))
}
func TestStoppedWSLDoesNotExecuteLinux(t *testing.T) {
	r := &fixtureRunner{}
	state, err := (Scanner{Runner: r}).Scan(context.Background(), "")
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
	state, err := (Scanner{Runner: r}).Scan(context.Background(), "")
	if err == nil || state.State != "unknown" {
		t.Fatalf("%+v %v", state, err)
	}
}
func TestScannerUsesDefaultDistributionInOneProbe(t *testing.T) {
	r := &fixtureRunner{running: "Ubuntu Test\r\n"}
	state, err := (Scanner{Runner: r}).Scan(context.Background(), "")
	want := []Port{{Type: "tcp", Port: 22, ProxyPort: 22}, {Type: "udp", Port: 53, ProxyPort: 53}}
	if err != nil || state.Distro != "Ubuntu Test" || state.IP != "172.20.1.2" || state.NetworkMode != "nat" || !reflect.DeepEqual(state.Ports, want) {
		t.Fatalf("%+v %v", state, err)
	}
	if len(r.calls) != 3 {
		t.Fatalf("expected 3 wsl.exe calls, got %q", r.calls)
	}
}
func TestScannerUsesConfiguredDistribution(t *testing.T) {
	r := &fixtureRunner{running: "Ubuntu Test\nDebian\n"}
	state, err := (Scanner{Runner: r}).Scan(context.Background(), "debian")
	if err != nil || state.Distro != "Debian" || state.IP == "" {
		t.Fatalf("%+v %v", state, err)
	}
	if _, err := (Scanner{Runner: r}).Scan(context.Background(), "Missing"); err == nil || !strings.Contains(err.Error(), "Ubuntu Test, Debian") {
		t.Fatalf("missing distro: %v", err)
	}
	if _, err := (Scanner{Runner: r}).Scan(context.Background(), "Legacy"); err == nil || !strings.Contains(err.Error(), "WSL2") {
		t.Fatalf("WSL1 distro: %v", err)
	}
}
func TestParseLinuxPortsProtocolAndWildcards(t *testing.T) {
	out := "tcp LISTEN 0 128 0.0.0.0:7 0.0.0.0:*\nudp UNCONN 0 0 *:7 *:*\ntcp LISTEN 0 128 [::]:7 [::]:*\ntcp LISTEN 0 128 127.0.0.1:80 0.0.0.0:*\ntcp LISTEN 0 128 10.0.0.2:81 0.0.0.0:*\n"
	got, err := ParseLinuxPorts(out, "10.0.0.2")
	want := []Port{{Type: "tcp", Port: 7, ProxyPort: 7}, {Type: "tcp", Port: 81, ProxyPort: 81}, {Type: "udp", Port: 7, ProxyPort: 7}}
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
	r := &fixtureRunner{running: "Ubuntu Test", probe: "@@mode mirrored\n"}
	snapshot, err := (Scanner{Runner: r}).Scan(context.Background(), "")
	if err != nil || snapshot.NetworkMode != "mirrored" || snapshot.IP != "" || len(snapshot.Ports) != 0 {
		t.Fatalf("%+v: %v", snapshot, err)
	}
}
func TestProbeCommandFailureIsReported(t *testing.T) {
	r := &fixtureRunner{running: "Ubuntu Test", probe: "@@mode nat\n@@ip\nsh: ip: not found\n@@error ip\n"}
	_, err := (Scanner{Runner: r}).Scan(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "ip: not found") {
		t.Fatalf("got %v", err)
	}
}
