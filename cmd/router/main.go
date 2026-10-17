package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"router-app/internal/api"
	"router-app/internal/benchmark"
	"router-app/internal/config"
	"router-app/internal/eval"
	"router-app/internal/provider"
	"router-app/internal/router"
	"router-app/internal/store"
)

func main() {
	cfgPath := flag.String("config", "configs/config.example.yaml", "config path")
	portOverride := flag.String("port", "", "override port")
	flag.Parse()
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	if *portOverride != "" {
		cfg.Listen = *portOverride
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatal(err)
	}

	db, err := store.Open(cfg.DataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	settings, err := api.NewSettingsStore(cfg.DataDir)
	if err != nil {
		log.Fatal(err)
	}
	st := settings.Get()
	// seed nimble URL from first ollama provider if unset default
	for _, pc := range cfg.Providers {
		if pc.Type == "ollama" && pc.BaseURL != "" && (st.NimbleBaseURL == "" || st.NimbleBaseURL == "http://127.0.0.1:11434") {
			st.NimbleBaseURL = pc.BaseURL
			_ = settings.Put(st)
			break
		}
	}

	reg := provider.NewRegistry()
	quotas := provider.NewQuotaStore()
	perf := provider.NewPerfStore()
	reg.SetPerfStore(perf)
	for _, pc := range cfg.Providers {
		p, err := provider.NewFromConfig(pc)
		if err != nil {
			log.Fatalf("provider %s: %v", pc.ID, err)
		}
		reg.Register(p)
		if q := provider.NewQuotaFetcherFromConfig(pc); q != nil {
			quotas.Register(q)
			log.Printf("quota adapter %s (%s)", pc.ID, pc.Quota)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	_ = reg.Sync(ctx)
	reg.StartPeriodicSync(ctx, 60*time.Second)
	quotas.StartPeriodicRefresh(ctx, 60*time.Second)

	stNow := settings.Get()
	eval.StartScheduler(ctx, db, eval.JudgeConfig{
		BaseURL: stNow.NimbleBaseURL,
		Model:   stNow.NimbleModel,
	}, 6*time.Hour)

	ttl := time.Duration(settings.Get().StickyTTLMinutes) * time.Minute
	ui, err := api.UIHandler()
	if err != nil {
		log.Fatal(err)
	}

	benchs := benchmark.NewStore("") // Use a real URL in production
	if err := benchs.Update(); err != nil {
		log.Printf("initial benchmark update failed: %v", err)
	}
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := benchs.Update(); err != nil {
					log.Printf("periodic benchmark update failed: %v", err)
				}
			}
		}
	}()

	srv := &api.Server{
		Cfg:        cfg,
		Reg:        reg,
		DB:         db,
		Sticky:     router.NewStickyStore(ttl),
		Settings:   settings,
		UI:         ui,
		Benchmarks: benchs,
		Quotas:     quotas,
		Perf:       perf,
	}
	httpSrv := &http.Server{Addr: cfg.Listen, Handler: srv.Handler()}
	go func() {
		<-ctx.Done()
		shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()
	log.Printf("listening on http://%s (ui /, virtual=%q, models=%d)", cfg.Listen, cfg.VirtualModel, len(reg.Models()))
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

