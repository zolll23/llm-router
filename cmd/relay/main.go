package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"sync"
	"time"
)

type Proxy struct {
	mu         sync.RWMutex
	targetPort string
	stateFile  string
}

func (p *Proxy) updateTarget() {
	data, err := os.ReadFile(p.stateFile)
	if err != nil {
		log.Printf("failed to read state file: %v", err)
		return
	}
	p.mu.Lock()
	p.targetPort = string(data)
	p.mu.Unlock()
	log.Printf("proxy target updated to: %s", p.targetPort)
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	target := p.targetPort
	p.mu.RUnlock()

	if target == "" {
		http.Error(w, "no target available", http.StatusServiceUnavailable)
		return
	}

	proxy := httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "127.0.0.1:" + target
		},
	}
	proxy.ServeHTTP(w, r)
}

func main() {
	stateFile := ".current_port"
	listenPort := "8080"
	
	p := &Proxy{
		stateFile: stateFile,
	}
	
	// Initial state
	p.updateTarget()
	
	// Periodically check for state changes
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		for range ticker.C {
			p.updateTarget()
		}
	}()

	log.Printf("relay proxy listening on :%s", listenPort)
	if err := http.ListenAndServe(":"+listenPort, p); err != nil {
		log.Fatal(err)
	}
}
