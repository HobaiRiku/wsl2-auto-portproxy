package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/controller"
	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/logbuffer"
	"github.com/HobaiRiku/wsl2-auto-portproxy/internal/registry"
)

type Options struct {
	Registry   *registry.Registry
	Controller *controller.Controller
	Logs       *logbuffer.Buffer
	Token      string
	Version    string
	Started    time.Time
	Port       int
	DevUI      bool
	Web        http.Handler
}
type credentials struct {
	mu       sync.Mutex
	tickets  map[string]time.Time
	sessions map[string]time.Time
}

func secret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
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
func New(o Options) http.Handler {
	credentials := &credentials{tickets: map[string]time.Time{}, sessions: map[string]time.Time{}}
	auth := func(r *http.Request) bool {
		bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if o.Token != "" && subtle.ConstantTimeCompare([]byte(bearer), []byte(o.Token)) == 1 {
			return true
		}
		cookie, err := r.Cookie("wslpp_session")
		if err != nil {
			return false
		}
		credentials.mu.Lock()
		defer credentials.mu.Unlock()
		expires, exists := credentials.sessions[cookie.Value]
		return exists && time.Now().Before(expires)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("POST /api/connect", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Code string `json:"code"`
		}
		if !decode(w, r, &body) {
			return
		}
		credentials.mu.Lock()
		expires, ok := credentials.tickets[body.Code]
		delete(credentials.tickets, body.Code)
		credentials.mu.Unlock()
		if !ok || time.Now().After(expires) {
			problem(w, 401, "authorization link expired; run wslpp ui again")
			return
		}
		token, err := secret()
		if err != nil {
			problem(w, 500, "cannot create session")
			return
		}
		credentials.mu.Lock()
		for key, expires := range credentials.sessions {
			if time.Now().After(expires) {
				delete(credentials.sessions, key)
			}
		}
		if len(credentials.sessions) >= 64 {
			credentials.mu.Unlock()
			problem(w, 429, "session limit reached")
			return
		}
		credentials.sessions[token] = time.Now().Add(12 * time.Hour)
		credentials.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "wslpp_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
		reply(w, 200, map[string]bool{"ok": true})
	})
	secured := http.NewServeMux()
	secured.HandleFunc("POST /api/session", func(w http.ResponseWriter, r *http.Request) {
		ticket, err := secret()
		if err != nil {
			problem(w, 500, "cannot create authorization link")
			return
		}
		credentials.mu.Lock()
		for key, expires := range credentials.tickets {
			if time.Now().After(expires) {
				delete(credentials.tickets, key)
			}
		}
		if len(credentials.tickets) >= 16 {
			credentials.mu.Unlock()
			problem(w, 429, "authorization link limit reached")
			return
		}
		credentials.tickets[ticket] = time.Now().Add(time.Minute)
		credentials.mu.Unlock()
		reply(w, 200, map[string]string{"code": ticket})
	})
	secured.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		status := o.Controller.Status()
		reply(w, 200, struct {
			controller.Status
			Version string    `json:"version"`
			Started time.Time `json:"startedAt"`
		}{status, o.Version, o.Started})
	})
	secured.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) { doc, _, _ := o.Registry.Snapshot(); reply(w, 200, doc) })
	secured.HandleFunc("PUT /api/config", func(w http.ResponseWriter, r *http.Request) {
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
	secured.HandleFunc("GET /api/logs", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, o.Logs.Entries()) })
	mux.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auth(r) {
			problem(w, 401, "authorization required; run wslpp ui")
			return
		}
		secured.ServeHTTP(w, r)
	}))
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
