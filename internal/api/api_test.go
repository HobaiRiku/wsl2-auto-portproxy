package api

import (
	"bytes"
	"encoding/json"
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
	server.Config.Handler = New(Options{Registry: r, Controller: c, Logs: &logbuffer.Buffer{}, Token: "test-token", Port: port, Web: http.NotFoundHandler()})
	server.Start()
	t.Cleanup(server.Close)
	return server, r
}
func request(t *testing.T, server *httptest.Server, method, path, body string, auth bool, cookie *http.Cookie) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if auth {
		req.Header.Set("Authorization", "Bearer test-token")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}
func TestAuthorizationLinkSingleUseAndCookie(t *testing.T) {
	server, _ := testServer(t)
	if response := request(t, server, "GET", "/api/status", "", false, nil); response.StatusCode != 401 {
		t.Fatal(response.StatusCode)
	}
	ticket := request(t, server, "POST", "/api/session", "{}", true, nil)
	var code struct{ Code string }
	if err := json.NewDecoder(ticket.Body).Decode(&code); err != nil || code.Code == "" {
		t.Fatalf("ticket: %v", err)
	}
	body, _ := json.Marshal(map[string]string{"code": code.Code})
	connected := request(t, server, "POST", "/api/connect", string(body), false, nil)
	if connected.StatusCode != 200 {
		t.Fatal(connected.StatusCode)
	}
	cookies := connected.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("invalid session cookie")
	}
	if response := request(t, server, "GET", "/api/status", "", false, cookies[0]); response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	if response := request(t, server, "POST", "/api/connect", string(body), false, nil); response.StatusCode != 401 {
		t.Fatal("ticket replay accepted")
	}
}
func TestConfigValidationAndRevisionConflict(t *testing.T) {
	server, r := testServer(t)
	doc, _, _ := r.Snapshot()
	bad := `{"revision":"` + doc.Revision + `","config":{"predefined":{"tcp":["22"]}}}`
	if response := request(t, server, "PUT", "/api/config", bad, true, nil); response.StatusCode != 400 {
		t.Fatal(response.StatusCode)
	}
	body := `{"revision":"` + doc.Revision + `","config":{"onlyPredefined":true}}`
	if response := request(t, server, "PUT", "/api/config", body, true, nil); response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	if response := request(t, server, "PUT", "/api/config", body, true, nil); response.StatusCode != 409 {
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
