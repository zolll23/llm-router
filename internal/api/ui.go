package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/*
var webFS embed.FS

var spaPaths = map[string]bool{
	"/":                true,
	"/index.html":      true,
	"/usage":           true,
	"/quality":         true,
	"/models":          true,
	"/settings":        true,
	"/recommendations": true, // alias → models
}

func isSPAPath(path string) bool {
	return spaPaths[path]
}

func UIHandler() (http.Handler, error) {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return nil, err
	}
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return nil, err
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSPAPath(r.URL.Path) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(index)
			return
		}
		files.ServeHTTP(w, r)
	}), nil
}
