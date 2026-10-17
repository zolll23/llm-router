package api

import (
	"encoding/json"
	"io"
	"net/http"

	"router/internal/config"
	"router/internal/provider"
	"router/internal/proxy"
)

type Server struct {
	Cfg *config.Config
	Reg *provider.Registry
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("POST /v1/chat/completions", s.handleChat)
	mux.HandleFunc("GET /api/inventory", s.handleInventory)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	data := []map[string]any{
		{
			"id":       s.Cfg.VirtualModel,
			"object":   "model",
			"owned_by": "router",
		},
	}
	for _, m := range s.Reg.Models() {
		data = append(data, map[string]any{
			"id":       m.ID(),
			"object":   "model",
			"owned_by": m.ProviderID,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (s *Server) handleInventory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"models": s.Reg.Models()})
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := proxy.ProxyChat(r.Context(), s.Reg, body, r.Header.Clone())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := proxy.CopyResponse(w, res.Response, res.Model, res.Reason); err != nil {
		return
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
