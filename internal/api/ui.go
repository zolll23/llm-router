package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/*
var webFS embed.FS

func UIHandler() (http.Handler, error) {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		return nil, err
	}
	// Serve "/" directly. Rewriting to "/index.html" makes http.FileServer
	// issue a 301 Location: ./ redirect loop with embed/fs.FS.
	return http.FileServer(http.FS(sub)), nil
}
