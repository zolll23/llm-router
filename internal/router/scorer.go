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
	switch m.CostTier {
	case "local":
		score += 10
	case "metered":
		score += 2
	default:
		score += 1
	}
	switch class.Complexity {
	case "trivial", "simple":
		if m.CostTier == "local" {
			score += 20
		}
	case "medium":
		if m.CostTier == "local" {
			score += 8
		} else {
			score += 5
		}
	case "hard":
		if m.CostTier != "local" {
			score += 12
		} else {
			score += 3
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
