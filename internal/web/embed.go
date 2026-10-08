//go:build !embedui

package web

import "net/http"

func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(503)
		w.Write([]byte("UI not embedded. Build with make build, or use npm run dev in ui/.\n"))
	})
}
