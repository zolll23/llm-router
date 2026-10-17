package router

import (
	"router/internal/classifier"
	"router/internal/provider"
)

func filterModels(models []provider.ModelInfo, class classifier.Result) []provider.ModelInfo {
	var out []provider.ModelInfo
	for _, m := range models {
		if !m.Routable {
			continue
		}
		if class.NeedsTools && !m.Tools {
			continue
		}
		out = append(out, m)
	}
	return out
}

func scoreModel(m provider.ModelInfo, class classifier.Result) float64 {
	score := 0.0

	// Capability alignment
	for _, cap := range m.Capabilities {
		if cap == class.Category {
			score += 30.0
		}
	}

	// Cost and efficiency
	switch m.CostTier {
	case "local":
		score += 10
	case "metered":
		score += 2
	default:
		score += 1
	}

	// Balance size and complexity
	switch class.Complexity {
	case "trivial", "simple":
		// Prefer small local models
		if m.CostTier == "local" && m.Params <= 7 {
			score += 20
		} else if m.CostTier == "local" {
			score += 5
		}
	case "medium":
		// Prefer mid-size local or metered
		if m.CostTier == "local" && m.Params <= 32 {
			score += 15
		} else if m.CostTier == "metered" {
			score += 10
		}
	case "hard":
		// Prefer large models, prefer metered for ultimate quality
		if m.Params >= 70 || m.CostTier == "metered" {
			score += 20
		} else {
			score += 5
		}
	}

	if m.Context >= 64000 {
		score += 2
	}
	return score
}

func pickBest(models []provider.ModelInfo, class classifier.Result) (provider.ModelInfo, bool) {
	cands := filterModels(models, class)
	if len(cands) == 0 {
		return provider.ModelInfo{}, false
	}
	best := cands[0]
	bestScore := scoreModel(best, class)
	for _, m := range cands[1:] {
		s := scoreModel(m, class)
		if s > bestScore {
			best = m
			bestScore = s
		}
	}
	return best, true
}
