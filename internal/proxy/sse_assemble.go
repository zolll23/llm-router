package proxy

import (
	"bytes"
	"encoding/json"
	"strings"
)

// assembleChatJSONFromSSE merges streamed chunks into a single chat.completion JSON.
func assembleChatJSONFromSSE(raw []byte) []byte {
	var id, model string
	var role, content strings.Builder
	toolArgs := map[int]*strings.Builder{}
	toolMeta := map[int]map[string]any{}
	finish := "stop"

	for _, block := range bytes.Split(raw, []byte("\n\n")) {
		line := strings.TrimSpace(string(block))
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if id == "" {
			id, _ = chunk["id"].(string)
		}
		if model == "" {
			model, _ = chunk["model"].(string)
		}
		choices, _ := chunk["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		ch, _ := choices[0].(map[string]any)
		if fr, ok := ch["finish_reason"].(string); ok && fr != "" {
			finish = fr
		}
		delta, _ := ch["delta"].(map[string]any)
		if delta == nil {
			if msg, ok := ch["message"].(map[string]any); ok {
				delta = msg
			}
		}
		if delta == nil {
			continue
		}
		if r, ok := delta["role"].(string); ok && r != "" && role.Len() == 0 {
			role.WriteString(r)
		}
		if c, ok := delta["content"].(string); ok {
			content.WriteString(c)
		}
		if tcs, ok := delta["tool_calls"].([]any); ok {
			for _, tc := range tcs {
				m, _ := tc.(map[string]any)
				if m == nil {
					continue
				}
				idx := 0
				if f, ok := m["index"].(float64); ok {
					idx = int(f)
				}
				if toolMeta[idx] == nil {
					toolMeta[idx] = map[string]any{"type": "function"}
					toolArgs[idx] = &strings.Builder{}
				}
				if idv, ok := m["id"].(string); ok && idv != "" {
					toolMeta[idx]["id"] = idv
				}
				if fn, ok := m["function"].(map[string]any); ok {
					cur, _ := toolMeta[idx]["function"].(map[string]any)
					if cur == nil {
						cur = map[string]any{}
					}
					if n, ok := fn["name"].(string); ok && n != "" {
						cur["name"] = n
					}
					if a, ok := fn["arguments"].(string); ok {
						toolArgs[idx].WriteString(a)
					}
					toolMeta[idx]["function"] = cur
				}
			}
		}
	}

	msg := map[string]any{"role": "assistant"}
	if role.Len() > 0 {
		msg["role"] = role.String()
	}
	if content.Len() > 0 {
		msg["content"] = content.String()
	} else {
		msg["content"] = ""
	}
	if len(toolMeta) > 0 {
		var tcs []any
		for i := 0; i < len(toolMeta); i++ {
			m := toolMeta[i]
			if m == nil {
				continue
			}
			fn, _ := m["function"].(map[string]any)
			if fn == nil {
				fn = map[string]any{}
			}
			fn["arguments"] = toolArgs[i].String()
			m["function"] = fn
			if m["id"] == nil {
				m["id"] = "call_sse_" + string(rune('a'+i))
			}
			tcs = append(tcs, m)
		}
		msg["tool_calls"] = tcs
		if finish == "stop" {
			finish = "tool_calls"
		}
	}
	out := map[string]any{
		"id": id, "object": "chat.completion", "model": model,
		"choices": []any{map[string]any{"index": 0, "message": msg, "finish_reason": finish}},
	}
	b, _ := json.Marshal(out)
	return b
}
