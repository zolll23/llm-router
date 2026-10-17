package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"router/internal/classifier"
	"router/internal/config"
	"router/internal/eval"
	"router/internal/provider"
	"router/internal/proxy"
	"router/internal/router"
	"router/internal/store"
)

type Server struct {
	Cfg      *config.Config
	Reg      *provider.Registry
	DB       *store.DB
	Sticky   *router.StickyStore
	Settings *SettingsStore
	UI       http.Handler
}

func (s *Server) classifier() classifier.Classifier {
	st := s.Settings.Get()
	switch st.Classifier {
	case "heuristic":
		return &classifier.Heuristic{}
	default:
		base := st.NimbleBaseURL
		if base == "" {
			base = "http://127.0.0.1:11434"
		}
		return classifier.NewNimble(base, st.NimbleModel, &classifier.Heuristic{})
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("POST /v1/chat/completions", s.handleChat)
	mux.HandleFunc("GET /api/inventory", s.handleInventory)
	mux.HandleFunc("POST /api/inventory/sync", s.handleSync)
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handlePutSettings)
	mux.HandleFunc("GET /api/stats/overview", s.handleOverview)
	mux.HandleFunc("GET /api/stats/models/{id...}", s.handleModelDetail)
	mux.HandleFunc("GET /api/eval/summary", s.handleQuality)
	mux.HandleFunc("GET /api/eval/confusion", s.handleConfusion)
	mux.HandleFunc("POST /api/eval/judge-run", s.handleJudgeRun)
	mux.HandleFunc("POST /api/eval/export-finetune", s.handleExportFinetune)
	if s.UI != nil {
		mux.Handle("GET /{$}", s.UI)
		mux.Handle("GET /app.js", s.UI)
		mux.Handle("GET /styles.css", s.UI)
	}
	return s.wrapAuth(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	data := []map[string]any{
		{"id": s.Cfg.VirtualModel, "object": "model", "owned_by": "router"},
	}
	for _, m := range s.Reg.Models() {
		data = append(data, map[string]any{
			"id": m.ID(), "object": "model", "owned_by": m.ProviderID,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (s *Server) handleInventory(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"models": s.Reg.Models()})
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	_ = s.Reg.Sync(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"models": s.Reg.Models()})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Settings.Get())
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var st RuntimeSettings
	if err := json.NewDecoder(r.Body).Decode(&st); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.Settings.Put(st); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ttl := st.StickyTTLMinutes
	if ttl <= 0 {
		ttl = 30
	}
	s.Sticky = router.NewStickyStore(time.Duration(ttl) * time.Minute)
	writeJSON(w, http.StatusOK, s.Settings.Get())
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	ov, err := s.DB.Overview(time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, ov)
}

func (s *Server) handleModelDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	since := time.Now().Add(-7 * 24 * time.Hour).UnixMilli()
	lat, series, err := s.DB.ModelDetail(id, since)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"model_id": id, "latency": lat, "series": series})
}

func (s *Server) handleQuality(w http.ResponseWriter, r *http.Request) {
	q, err := s.DB.QualitySince(time.Now().Add(-7 * 24 * time.Hour).UnixMilli())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, q)
}

func (s *Server) handleExportFinetune(w http.ResponseWriter, r *http.Request) {
	path, n, err := eval.ExportGLiNERJSONL(s.DB, s.Cfg.DataDir, 500)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "rows": n})
}

func (s *Server) handleConfusion(w http.ResponseWriter, r *http.Request) {
	c, err := eval.ConfusionMatrix(s.DB, time.Now().Add(-30*24*time.Hour).UnixMilli())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleJudgeRun(w http.ResponseWriter, r *http.Request) {
	st := s.Settings.Get()
	n, err := eval.RunOnce(r.Context(), s.DB, eval.JudgeConfig{
		BaseURL: st.NimbleBaseURL,
		Model:   st.NimbleModel,
	}, 25)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"judged": n})
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	st := s.Settings.Get()
	deps := proxy.Deps{
		Reg:        s.Reg,
		Classifier: s.classifier(),
		Sticky:     s.Sticky,
		DB:         s.DB,
		KeepAlive:  st.KeepAlive,
	}
	res, err := proxy.ProxyChat(r.Context(), deps, body, r.Header.Clone())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	user := lastUserText(body)
	_ = proxy.CopyAndRecord(w, deps, res, user)
}

func lastUserText(body []byte) string {
	var root struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(body, &root)
	user := ""
	for _, m := range root.Messages {
		if m.Role != "user" {
			continue
		}
		if s, ok := m.Content.(string); ok {
			user = s
		}
	}
	return user
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
