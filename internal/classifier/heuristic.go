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
	category := "chat"
	conf := 0.55

	codingHints := []string{"code", "python", "go", "rust", "javascript", "typescript", "sql", "html", "css", "api", "implement", "debug", "fix", "test", "код", "программирование", "реализ", "исправ", "тест"}
	reasoningHints := []string{"prove", "logic", "math", "solve", "why", "reason", "analyze", "докажи", "логика", "почему", "анализ"}
	creativeHints := []string{"write a story", "poem", "creative", "imagine", "напиши рассказ", "стихотворение", "придумай"}

	for _, h := range codingHints {
		if strings.Contains(text, h) {
			category = "coding"
			break
		}
	}
	if category == "chat" {
		for _, h := range reasoningHints {
			if strings.Contains(text, h) {
				category = "reasoning"
				break
			}
		}
	}
	if category == "chat" {
		for _, h := range creativeHints {
			if strings.Contains(text, h) {
				category = "creative"
				break
			}
		}
	}

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
	return Result{Complexity: complexity, Category: category, NeedsTools: needs, Confidence: conf, Source: "heuristic"}, nil
}
