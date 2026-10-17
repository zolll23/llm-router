package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"router/internal/api"
	"router/internal/config"
	"router/internal/provider"
)

func main() {
	cfgPath := flag.String("config", "configs/config.example.yaml", "config path")
	flag.Parse()
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatal(err)
	}

	reg := provider.NewRegistry()
	for _, pc := range cfg.Providers {
		p, err := provider.NewFromConfig(pc)
		if err != nil {
			log.Fatalf("provider %s: %v", pc.ID, err)
		}
		reg.Register(p)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if err := reg.Sync(ctx); err != nil {
		log.Printf("initial sync: %v", err)
	}
	reg.StartPeriodicSync(ctx, 60*time.Second)

	srv := &api.Server{Cfg: cfg, Reg: reg}
	httpSrv := &http.Server{Addr: cfg.Listen, Handler: srv.Handler()}
	go func() {
		<-ctx.Done()
		shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()
	log.Printf("listening on %s (virtual model %q, %d models)", cfg.Listen, cfg.VirtualModel, len(reg.Models()))
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
