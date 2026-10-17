package classifier

import (
	"context"
	"strings"
)

type Heuristic struct{}

func (h *Heuristic) Classify(ctx context.Context, in Input) (Result, error) {
	_ = ctx
	text := strings.ToLower(in.UserText)
	complexity := "simple"
	conf := 0.55

	hardHints := []string{"architect", "refactor", "migrate", "prove", "design system", "distributed", "оптимиз", "архитектур", "рефактор"}
	mediumHints := []string{"implement", "debug", "fix", "test", "explain", "реализ", "исправ", "тест"}
	for _, h := range hardHints {
		if strings.Contains(text, h) {
			complexity = "hard"
			conf = 0.6
			break
		}
	}
	if complexity != "hard" {
		for _, h := range mediumHints {
			if strings.Contains(text, h) {
				complexity = "medium"
				conf = 0.58
				break
			}
		}
	}
	if in.ApproxTokens > 32000 || in.MessageCount > 40 {
		if complexity == "simple" {
			complexity = "medium"
		}
		conf = 0.5
	}
	needs := in.ToolsPresent
	if !needs {
		needs = strings.Contains(text, "tool") || strings.Contains(text, "edit file") || strings.Contains(text, "файл")
	}
	return Result{Complexity: complexity, NeedsTools: needs, Confidence: conf, Source: "heuristic"}, nil
}
