package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/controller"
	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/logbuffer"
	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/registry"
	"github.com/HobaiRiku/wsl2-auto-portproxy/lib/service"
)

type Options struct {
	Registry   *registry.Registry
	Controller *controller.Controller
	Logs       *logbuffer.Buffer
	Version    string
	Started    time.Time
	Port       int
	DevUI      bool
	Web        http.Handler
	// Distros lists installed WSL distributions; it must not start WSL.
	Distros func(context.Context) ([]service.Distro, error)
}

func reply(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
func problem(w http.ResponseWriter, status int, err string) {
	reply(w, status, map[string]string{"error": err})
}
func decode(w http.ResponseWriter, r *http.Request, out any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		problem(w, 415, "application/json required")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		problem(w, 400, err.Error())
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		problem(w, 400, "one JSON value required")
		return false
	}
	return true
}

// New serves the management API and UI. There is no login: the server only
// listens on loopback, and every request must come from a loopback peer with
// a loopback Host and same-origin Origin, which blocks remote access, DNS
// rebinding and cross-site requests from web pages.
func New(o Options) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		status := o.Controller.Status()
		reply(w, 200, struct {
			controller.Status
			Version string    `json:"version"`
			Started time.Time `json:"startedAt"`
		}{status, o.Version, o.Started})
	})
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) { doc, _, _ := o.Registry.Snapshot(); reply(w, 200, doc) })
	mux.HandleFunc("PUT /api/config", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Revision string          `json:"revision"`
			Config   json.RawMessage `json:"config"`
		}
		if !decode(w, r, &body) {
			return
		}
		doc, err := o.Registry.Replace(body.Revision, body.Config)
		if errors.Is(err, registry.ErrConflict) {
			problem(w, 409, err.Error())
			return
		}
		if errors.Is(err, registry.ErrPersistence) {
			problem(w, 500, err.Error())
			return
		}
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, doc)
	})
	mux.HandleFunc("GET /api/distros", func(w http.ResponseWriter, r *http.Request) {
		if o.Distros == nil {
			reply(w, 200, []service.Distro{})
			return
		}
		distros, err := o.Distros(r.Context())
		if err != nil {
			problem(w, 502, err.Error())
			return
		}
		if distros == nil {
			distros = []service.Distro{}
		}
		reply(w, 200, distros)
	})
	mux.HandleFunc("GET /api/logs", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, o.Logs.Entries()) })
	mux.Handle("/", o.Web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		host, port, err := net.SplitHostPort(r.Host)
		ip := net.ParseIP(host)
		if err != nil || port != strconv.Itoa(o.Port) || (host != "localhost" && (ip == nil || !ip.IsLoopback())) {
			problem(w, 403, "invalid management host")
			return
		}
		peer, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || net.ParseIP(peer) == nil || !net.ParseIP(peer).IsLoopback() {
			problem(w, 403, "management API is loopback only")
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" && origin != "http://"+r.Host && !(o.DevUI && (origin == "http://127.0.0.1:5173" || origin == "http://localhost:5173")) {
			problem(w, 403, "cross-origin request rejected")
			return
		}
		mux.ServeHTTP(w, r)
	})
}
