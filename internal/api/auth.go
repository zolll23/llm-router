package api

import (
	"net"
	"net/http"
	"strings"

	"router-app/internal/config"
)

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	return host == "localhost"
}

func tokenForAPI(cfg *config.Config) string {
	if cfg.APIToken != "" {
		return cfg.APIToken
	}
	return cfg.AdminToken
}

func isPublicPath(path string) bool {
	switch path {
	case "/health", "/", "/index.html", "/usage", "/quality", "/models",
		"/settings", "/recommendations", "/app.js", "/styles.css":
		return true
	default:
		return false
	}
}

func (s *Server) wrapAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isLoopbackAddr(s.Cfg.Listen) {
			next.ServeHTTP(w, r)
			return
		}
		path := r.URL.Path
		if isPublicPath(path) {
			next.ServeHTTP(w, r)
			return
		}
		need := ""
		if strings.HasPrefix(path, "/v1/") {
			need = tokenForAPI(s.Cfg)
		} else {
			need = s.Cfg.AdminToken
		}
		if need == "" {
			http.Error(w, "admin_token/api_token required for non-loopback listen", http.StatusUnauthorized)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+need {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
