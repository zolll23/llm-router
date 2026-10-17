package provider

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var paramsFromName = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)b`)

// PerfStore keeps rolling latency/tps hints for scoring.
type PerfStore struct {
	mu   sync.RWMutex
	byID map[string]*perfEntry
}

type perfEntry struct {
	latencies []float64 // ms, ring of last N
	tpsEMA    float64
	hasTPS    bool
}

func NewPerfStore() *PerfStore {
	return &PerfStore{byID: map[string]*perfEntry{}}
}

func (p *PerfStore) Observe(modelID string, latencyMS int64, completionTokens int) {
	if modelID == "" || latencyMS <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.byID[modelID]
	if e == nil {
		e = &perfEntry{}
		p.byID[modelID] = e
	}
	e.latencies = append(e.latencies, float64(latencyMS))
	if len(e.latencies) > 40 {
		e.latencies = e.latencies[len(e.latencies)-40:]
	}
	if completionTokens > 0 && latencyMS > 0 {
		tps := float64(completionTokens) / (float64(latencyMS) / 1000.0)
		if !e.hasTPS {
			e.tpsEMA = tps
			e.hasTPS = true
		} else {
			e.tpsEMA = e.tpsEMA*0.7 + tps*0.3
		}
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := int(math.Round(p * float64(len(sorted)-1)))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func (p *PerfStore) Apply(m *ModelInfo) {
	if m == nil {
		return
	}
	p.mu.RLock()
	e := p.byID[m.ID()]
	p.mu.RUnlock()
	if e == nil || len(e.latencies) == 0 {
		return
	}
	cp := append([]float64(nil), e.latencies...)
	// insertion sort small
	for i := 1; i < len(cp); i++ {
		j := i
		for j > 0 && cp[j-1] > cp[j] {
			cp[j-1], cp[j] = cp[j], cp[j-1]
			j--
		}
	}
	if m.Metrics == nil {
		m.Metrics = map[string]float64{}
	}
	m.Metrics["latency_p50"] = percentile(cp, 0.50)
	if e.hasTPS {
		m.Metrics["tps"] = e.tpsEMA
	}
}

// EstimateParamsFromName parses "7b", "14b" from model tags.
func EstimateParamsFromName(name string) int {
	m := paramsFromName.FindStringSubmatch(strings.ToLower(name))
	if len(m) < 2 {
		return 0
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	return int(f + 0.5)
}
