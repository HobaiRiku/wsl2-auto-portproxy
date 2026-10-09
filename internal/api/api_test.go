package api

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/controller"
	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/logbuffer"
	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/registry"
)

func testServer(t *testing.T) (*httptest.Server, *registry.Registry) {
	t.Helper()
	r := registry.New(filepath.Join(t.TempDir(), "config.json"))
	c := controller.New(r, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), 0)
	server := httptest.NewUnstartedServer(nil)
	port := server.Listener.Addr().(*net.TCPAddr).Port
	server.Config.Handler = New(Options{Registry: r, Controller: c, Logs: &logbuffer.Buffer{}, Port: port, Web: http.NotFoundHandler()})
	server.Start()
	t.Cleanup(server.Close)
	return server, r
}
func request(t *testing.T, server *httptest.Server, method, path, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}
func TestLoopbackNeedsNoLogin(t *testing.T) {
	server, _ := testServer(t)
	for _, path := range []string{"/api/status", "/api/config", "/api/logs", "/api/distros"} {
		if response := request(t, server, "GET", path, ""); response.StatusCode != 200 {
			t.Fatalf("%s: %d", path, response.StatusCode)
		}
	}
	if response := request(t, server, "POST", "/api/session", "{}"); response.StatusCode != 404 && response.StatusCode != 405 {
		t.Fatalf("session endpoint still served: %d", response.StatusCode)
	}
}
func TestConfigValidationAndRevisionConflict(t *testing.T) {
	server, r := testServer(t)
	doc, _, _ := r.Snapshot()
	bad := `{"revision":"` + doc.Revision + `","config":{"predefined":{"tcp":["22"]}}}`
	if response := request(t, server, "PUT", "/api/config", bad); response.StatusCode != 400 {
		t.Fatal(response.StatusCode)
	}
	body := `{"revision":"` + doc.Revision + `","config":{"onlyPredefined":true}}`
	if response := request(t, server, "PUT", "/api/config", body); response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	if response := request(t, server, "PUT", "/api/config", body); response.StatusCode != 409 {
		t.Fatal("stale update accepted")
	}
	current, _, _ := r.Snapshot()
	if !current.Config.OnlyPredefined {
		t.Fatal("configuration not saved")
	}
}
func TestOriginHostAndPeerChecks(t *testing.T) {
	server, _ := testServer(t)
	for _, kind := range []string{"origin", "host"} {
		req, _ := http.NewRequest("GET", server.URL+"/api/health", nil)
		if kind == "origin" {
			req.Header.Set("Origin", "http://evil.example")
		} else {
			req.Host = "evil.example:47831"
		}
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 403 {
			t.Fatalf("accepted %s", kind)
		}
	}
	port := server.Listener.Addr().(*net.TCPAddr).Port
	req := httptest.NewRequest("GET", "http://127.0.0.1:"+strconv.Itoa(port)+"/api/health", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	recorder := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(recorder, req)
	if recorder.Code != 403 {
		t.Fatal("trusted spoofed proxy header")
	}
}
