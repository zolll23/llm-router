package classifier

import "strings"

func NormalizeCategory(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "coding", "code":
		return "coding"
	case "reasoning", "reason":
		return "reasoning"
	case "creative":
		return "creative"
	case "chat", "conversation", "":
		return "chat"
	default:
		return "chat"
	}
}

func NormalizeComplexity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trivial", "simple", "medium", "hard":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "simple"
	}
}
