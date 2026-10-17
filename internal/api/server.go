package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"router-app/internal/benchmark"
	"router-app/internal/classifier"
	"router-app/internal/config"
	"router-app/internal/eval"
	"router-app/internal/provider"
	"router-app/internal/proxy"
	"router-app/internal/router"
	"router-app/internal/scheduler"
	"router-app/internal/store"
)

type Server struct {
	Cfg        *config.Config
	Reg        *provider.Registry
	DB         *store.DB
	Sticky     *router.StickyStore
	Settings   *SettingsStore
	UI         http.Handler
	Benchmarks *benchmark.BenchmarkStore
	Quotas     *provider.QuotaStore
	Perf       *provider.PerfStore
	Schedulers *scheduler.Registry
}

func (s *Server) classifier() classifier.Classifier {
	st := s.Settings.Get()
	switch st.Classifier {
	case "heuristic":
		return &classifier.Heuristic{}
	case "nimble":
		base := st.NimbleBaseURL
		if base == "" {
			base = "http://127.0.0.1:11434"
		}
		return classifier.NewNimble(base, st.NimbleModel, &classifier.Heuristic{})
	default: // gliner
		base := st.GLiNERBaseURL
		if base == "" {
			base = "http://127.0.0.1:8090"
		}
		return classifier.NewGLiNER(base, &classifier.Heuristic{})
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("POST /v1/chat/completions", s.handleChat)
	mux.HandleFunc("GET /api/inventory", s.handleInventory)
	mux.HandleFunc("GET /api/models", s.handleModelsRich)
	mux.HandleFunc("GET /api/quotas", s.handleQuotas)
	mux.HandleFunc("POST /api/inventory/sync", s.handleSync)
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handlePutSettings)
	mux.HandleFunc("GET /api/stats/overview", s.handleOverview)
	mux.HandleFunc("GET /api/stats/sankey", s.handleSankey)
	mux.HandleFunc("GET /api/stats/models/{id...}", s.handleModelDetail)
	mux.HandleFunc("GET /api/eval/summary", s.handleQuality)
	mux.HandleFunc("GET /api/eval/confusion", s.handleConfusion)
	mux.HandleFunc("POST /api/eval/judge-run", s.handleJudgeRun)
	mux.HandleFunc("POST /api/eval/export-finetune", s.handleExportFinetune)
	mux.HandleFunc("GET /api/benchmarks/host", s.handleHostHardware)
	mux.HandleFunc("GET /api/benchmarks/profiles", s.handleHardwareProfiles)
	mux.HandleFunc("POST /api/benchmarks/recommend", s.handleRecommend)
	if s.UI != nil {
		mux.Handle("GET /{$}", s.UI)
		mux.Handle("GET /usage", s.UI)
		mux.Handle("GET /quality", s.UI)
		mux.Handle("GET /models", s.UI)
		mux.Handle("GET /settings", s.UI)
		mux.Handle("GET /recommendations", s.UI)
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
	if s.Quotas != nil {
		s.Quotas.Refresh(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": s.Reg.Models()})
}

func (s *Server) handleModelsRich(w http.ResponseWriter, r *http.Request) {
	models := s.Reg.Models()
	out := make([]map[string]any, 0, len(models))
	for _, m := range models {
		row := map[string]any{
			"id":           m.ID(),
			"provider_id":  m.ProviderID,
			"upstream_id":  m.UpstreamID,
			"display_name": m.DisplayName,
			"cost_tier":    m.CostTier,
			"routable":     m.Routable,
			"tools":        m.Tools,
			"params":       m.Params,
			"metrics":      m.Metrics,
			"benchmark_url": "https://huggingface.co/models?search=" +
				strings.ReplaceAll(m.UpstreamID, ":", "+"),
		}
		if m.CostTier == "local" {
			base := m.UpstreamID
			if i := strings.Index(base, ":"); i >= 0 {
				base = base[:i]
			}
			row["ollama_url"] = "https://ollama.com/library/" + base
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": out})
}

func (s *Server) handleQuotas(w http.ResponseWriter, r *http.Request) {
	var list []provider.QuotaSnapshot
	if s.Quotas != nil {
		list = s.Quotas.All()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"providers":  list,
		"fetched_at": time.Now().UTC(),
	})
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

func (s *Server) handleSankey(w http.ResponseWriter, r *http.Request) {
	days := 7
	if v := r.URL.Query().Get("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}
	diag, err := s.DB.SankeySince(days)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, diag)
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

func (s *Server) handleHostHardware(w http.ResponseWriter, r *http.Request) {
	hw := benchmark.DetectHost()
	writeJSON(w, http.StatusOK, hw)
}

func (s *Server) handleHardwareProfiles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"profiles": benchmark.Profiles(),
		"default":  benchmark.PresetMacM536,
	})
}

func (s *Server) handleRecommend(w http.ResponseWriter, r *http.Request) {
	var hw benchmark.Hardware
	if err := json.NewDecoder(r.Body).Decode(&hw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if hw.Source == "" {
		hw.Source = "request"
	}
	var inv []provider.ModelInfo
	if s.Reg != nil {
		inv = s.Reg.Models()
	}
	var load []store.LoadCell
	var total int64
	if s.DB != nil {
		var err error
		load, total, err = s.DB.LoadProfile(7)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	res := s.Benchmarks.RecommendWithLoad(hw, inv, load, total)
	writeJSON(w, http.StatusOK, res)
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
		Quotas:     s.Quotas,
		Perf:       s.Perf,
		Schedulers: s.Schedulers,
		KeepAlive:  st.KeepAlive,
		Settings: func() proxy.RoutingSettings {
			cur := s.Settings.Get()
			return proxy.RoutingSettings{
				EscalateMin:              cur.EscalateMinComplexity,
				QuotaThresholdRollingPct: cur.QuotaThresholdRollingPct,
				QuotaThresholdWeeklyPct:  cur.QuotaThresholdWeeklyPct,
				QuotaThresholdMonthlyPct: cur.QuotaThresholdMonthlyPct,
				QuotaCooldownSeconds:     cur.QuotaCooldownSeconds,
				KeepAlive:                cur.KeepAlive,
				LocalQueueMax:            cur.LocalQueueMax,
				LocalQueueRedirect:       cur.LocalQueueRedirect,
			}
		},
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
