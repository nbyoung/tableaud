package main

import (
	"io/fs"
	"net/http"

	"github.com/nbyoung/tableaud/internal/web"
)

// NewDaemon is the tiny handler that serves the same views by route with a
// query. It renders through the App that the export uses.
func NewDaemon(a *App) http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(web.Static, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	serve := func(w http.ResponseWriter, r Req) {
		out, err := a.Render(r, DaemonLinker{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(out)
	}
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) { serve(w, Req{Kind: "index"}) })
	mux.HandleFunc("GET /tableau", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		serve(w, Req{Kind: "tableau", ID: q.Get("id"), State: q.Get("s"), Part: q.Get("part")})
	})
	for _, k := range []string{"task", "gate", "assignment", "queue"} {
		mux.HandleFunc("GET /"+k, func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			id := q.Get("id")
			if k == "assignment" || k == "queue" {
				id = q.Get("as")
			}
			serve(w, Req{Kind: k, ID: id, Level: q.Get("level")})
		})
	}
	mux.HandleFunc("GET /fragment", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		id := q.Get("id")
		if q.Get("kind") == "assignment" {
			id = q.Get("as")
		}
		serve(w, Req{Kind: q.Get("kind"), ID: id, Part: q.Get("part")})
	})
	return mux
}
