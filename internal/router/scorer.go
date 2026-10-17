package router

import (
	"strings"

	"router-app/internal/classifier"
	"router-app/internal/provider"
)

type ScoreOpts struct {
	Escalate   bool
	LocalOnly  bool
	ExternalOK func(providerID string) bool
	// AgentOK when set and class.NeedsTools: only keep models that return true.
	AgentOK func(m provider.ModelInfo) bool
	// LocalAllow when non-empty: CostTier==local models must be in this set (by ModelInfo.ID()).
	LocalAllow map[string]bool
	// LocalLoaded when set with non-empty LocalAllow: local models must be loaded (e.g. in ollama ps).
	LocalLoaded func(m provider.ModelInfo) bool
	// NoLocal drops all CostTier==local candidates (force cloud path).
	NoLocal bool
}

func filterModels(models []provider.ModelInfo, class classifier.Result, opts ScoreOpts) []provider.ModelInfo {
	var out []provider.ModelInfo
	for _, m := range models {
		if !m.Routable {
			continue
		}
		if class.NeedsTools && !m.Tools {
			continue
		}
		if class.NeedsTools && opts.AgentOK != nil && !opts.AgentOK(m) {
			continue
		}
		if opts.LocalOnly && m.CostTier != "local" {
			continue
		}
		if opts.NoLocal && m.CostTier == "local" {
			continue
		}
		if m.CostTier == "local" && len(opts.LocalAllow) > 0 {
			if !opts.LocalAllow[m.ID()] {
				continue
			}
			if opts.LocalLoaded != nil && !opts.LocalLoaded(m) {
				continue
			}
		}
		out = append(out, m)
	}
	return out
}

// AllowlistForCategory returns the set of allowed local model IDs for a category.
// nil/empty means "no allowlist" (legacy scoring).
func AllowlistForCategory(lists map[string][]string, category string) map[string]bool {
	if len(lists) == 0 {
		return nil
	}
	ids := lists[category]
	if len(ids) == 0 {
		return nil
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			out[id] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func scoreModel(m provider.ModelInfo, class classifier.Result, opts ScoreOpts) float64 {
	score := 0.0

	for _, cap := range m.Capabilities {
		if cap == class.Category {
			score += 100.0
		}
	}

	switch m.CostTier {
	case "local":
		score += 20.0
	case "metered":
		score += 10.0
	default:
		score += 5.0
	}

	// Escalation: prefer external when eligible and headroom OK;
	// without headroom, heavily demote metered so local wins.
	if opts.Escalate && !opts.LocalOnly {
		ok := opts.ExternalOK == nil || opts.ExternalOK(m.ProviderID)
		if m.CostTier == "metered" {
			if ok {
				score += 80.0
			} else {
				score -= 200.0
			}
		} else if m.CostTier == "local" {
			score += 5.0
		}
	}

	switch class.Complexity {
	case "trivial", "simple":
		if m.Params > 0 && m.Params <= 7 {
			score += 50.0
		} else if m.Params == 0 {
			score += 25.0
		} else {
			score += 10.0
		}
	case "medium":
		if m.Params > 0 && m.Params <= 32 {
			score += 40.0
		} else if m.Params == 0 {
			score += 25.0
		} else {
			score += 20.0
		}
	case "hard":
		if m.Params >= 32 {
			score += 50.0
		} else if m.Params == 0 {
			score += 25.0
		} else {
			score += 10.0
		}
	}

	if tps, ok := m.Metrics["tps"]; ok {
		score += tps * 0.5
	}
	if p50, ok := m.Metrics["latency_p50"]; ok && p50 > 0 {
		// Prefer faster models (p50 in ms)
		bonus := 50.0 - p50/100.0
		if bonus < 0 {
			bonus = 0
		}
		if bonus > 50 {
			bonus = 50
		}
		score += bonus
	}

	return score
}

func complexityRank(c string) int {
	switch c {
	case "trivial":
		return 0
	case "simple":
		return 1
	case "medium":
		return 2
	case "hard":
		return 3
	default:
		return 1
	}
}

func ShouldEscalate(complexity, escalateMin string) bool {
	if escalateMin == "" {
		escalateMin = "hard"
	}
	return complexityRank(complexity) >= complexityRank(escalateMin)
}

func pickBest(models []provider.ModelInfo, class classifier.Result, opts ScoreOpts) (provider.ModelInfo, bool) {
	cands := filterModels(models, class, opts)
	if len(cands) == 0 {
		return provider.ModelInfo{}, false
	}
	best := cands[0]
	bestScore := scoreModel(best, class, opts)
	for _, m := range cands[1:] {
		s := scoreModel(m, class, opts)
		if s > bestScore {
			best = m
			bestScore = s
		}
	}
	return best, true
}
