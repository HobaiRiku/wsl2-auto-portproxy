package app

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/service"
)

type stoppedScanner struct{}

func (stoppedScanner) Scan(context.Context, string) (service.Snapshot, error) {
	now := time.Now()
	return service.Snapshot{State: "stopped", Distro: "Test", NetworkMode: "unknown", LastScan: &now}, nil
}
func TestAppReservesEndpointAndShutsDown(t *testing.T) {
	home := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, Options{Home: home, Listen: "127.0.0.1:0", Scanner: stoppedScanner{}}) }()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	var endpoint string
	for endpoint == "" {
		select {
		case err := <-done:
			t.Fatalf("app exited: %v", err)
		case <-deadline.C:
			t.Fatal("endpoint not ready")
		case <-ticker.C:
			if data, err := os.ReadFile(filepath.Join(home, "endpoint")); err == nil {
				endpoint = string(data)
			}
		}
	}
	// Without keep-alives the transport cannot leave a spare, request-less
	// connection open, which http.Server.Shutdown waits up to 5s for.
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	response, err := client.Get(endpoint + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	response, err = client.Get(endpoint + "/api/status")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown blocked")
	}
	if _, err := os.Stat(filepath.Join(home, "endpoint")); !os.IsNotExist(err) {
		t.Fatal("endpoint retained after shutdown")
	}
}
