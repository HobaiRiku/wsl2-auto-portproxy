package app

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/service"
)

type stoppedScanner struct{}

func (stoppedScanner) Scan(context.Context) (service.Snapshot, error) {
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
	response, err := http.Get(endpoint + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	token, err := os.ReadFile(filepath.Join(home, "token"))
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", endpoint+"/api/status", nil)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	response, err = http.DefaultClient.Do(req)
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
