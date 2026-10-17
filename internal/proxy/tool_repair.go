package proxy

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	jsonFenceRe = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\}|\\[.*?\\])\\s*```")
	jsonObjRe   = regexp.MustCompile(`(?s)\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}`)
)

// RepairToolCallsInChatJSON rewrites assistant content that embeds tool JSON
// into proper message.tool_calls. allowed is the set of tool names from the request.
// Returns repaired body and true if a repair was applied.
func RepairToolCallsInChatJSON(body []byte, allowed map[string]bool) ([]byte, bool) {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return body, false
	}
	choices, _ := root["choices"].([]any)
	if len(choices) == 0 {
		return body, false
	}
	ch, _ := choices[0].(map[string]any)
	if ch == nil {
		return body, false
	}
	msg, _ := ch["message"].(map[string]any)
	if msg == nil {
		return body, false
	}
	if tcs, ok := msg["tool_calls"].([]any); ok && len(tcs) > 0 {
		return body, false
	}
	content, _ := msg["content"].(string)
	if strings.TrimSpace(content) == "" {
		return body, false
	}
	calls := extractToolCalls(content, allowed)
	if len(calls) == 0 {
		return body, false
	}
	msg["tool_calls"] = calls
	msg["content"] = ""
	ch["finish_reason"] = "tool_calls"
	ch["message"] = msg
	choices[0] = ch
	root["choices"] = choices
	out, err := json.Marshal(root)
	if err != nil {
		return body, false
	}
	return out, true
}

func extractToolCalls(content string, allowed map[string]bool) []any {
	var blobs []string
	for _, m := range jsonFenceRe.FindAllStringSubmatch(content, -1) {
		if len(m) > 1 {
			blobs = append(blobs, m[1])
		}
	}
	if len(blobs) == 0 {
		for _, m := range jsonObjRe.FindAllString(content, -1) {
			blobs = append(blobs, m)
		}
	}
	var out []any
	for _, raw := range blobs {
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			// Weak models often emit JS-ish JSON with backtick strings.
			if fixed, ok := normalizeBacktickJSON(raw); ok {
				if err2 := json.Unmarshal([]byte(fixed), &v); err2 != nil {
					continue
				}
			} else {
				continue
			}
		}
		out = append(out, toolCallsFromValue(v, allowed)...)
	}
	if len(out) == 0 {
		out = append(out, extractToolCallsPseudo(content, allowed)...)
	}
	return out
}

var toolNameRe = regexp.MustCompile(`(?i)"name"\s*:\s*"([^"]+)"`)

// normalizeBacktickJSON rewrites "key": `...` (incl. nested backticks) into valid JSON strings.
func normalizeBacktickJSON(raw string) (string, bool) {
	var b strings.Builder
	changed := false
	i := 0
	for i < len(raw) {
		// look for :" or : " then optional space and backtick
		if raw[i] == '`' {
			// only treat as string if preceded by : (with whitespace)
			j := i - 1
			for j >= 0 && (raw[j] == ' ' || raw[j] == '\t' || raw[j] == '\n' || raw[j] == '\r') {
				j--
			}
			if j >= 0 && raw[j] == ':' {
				if val, end, ok := readBacktickString(raw, i); ok {
					enc, _ := json.Marshal(val)
					b.Write(enc)
					i = end
					changed = true
					continue
				}
			}
		}
		b.WriteByte(raw[i])
		i++
	}
	if !changed {
		return "", false
	}
	return b.String(), true
}

// readBacktickString reads from opening ` and finds the closing ` that is followed by } or ,.
func readBacktickString(s string, open int) (string, int, bool) {
	if open >= len(s) || s[open] != '`' {
		return "", open, false
	}
	for end := len(s) - 1; end > open; end-- {
		if s[end] != '`' {
			continue
		}
		after := strings.TrimSpace(s[end+1:])
		if after == "" || after[0] == '}' || after[0] == ',' {
			return s[open+1 : end], end + 1, true
		}
	}
	return "", open, false
}

func extractToolCallsPseudo(content string, allowed map[string]bool) []any {
	m := toolNameRe.FindStringSubmatch(content)
	if m == nil {
		return nil
	}
	name := resolveToolName(m[1], allowed)
	if name == "" {
		return nil
	}
	args := map[string]any{}
	if code, ok := extractBacktickField(content, "code"); ok {
		args["code"] = code
	} else if cmd, ok := extractBacktickField(content, "command"); ok {
		args["command"] = cmd
	} else if script, ok := extractBacktickField(content, "script"); ok {
		args["code"] = script
	} else {
		return nil
	}
	return toolCallsFromValue(map[string]any{"name": name, "arguments": args}, allowed)
}

func extractBacktickField(s, field string) (string, bool) {
	re := regexp.MustCompile(`(?i)"` + regexp.QuoteMeta(field) + `"\s*:\s*` + "`")
	loc := re.FindStringIndex(s)
	if loc == nil {
		return "", false
	}
	val, _, ok := readBacktickString(s, loc[1]-1)
	return val, ok
}

func toolCallsFromValue(v any, allowed map[string]bool) []any {
	switch t := v.(type) {
	case []any:
		var out []any
		for _, el := range t {
			out = append(out, toolCallsFromValue(el, allowed)...)
		}
		return out
	case map[string]any:
		if arr, ok := t["tool_calls"].([]any); ok {
			return toolCallsFromValue(arr, allowed)
		}
		name := stringField(t, "name")
		if name == "" {
			if fn, ok := t["function"].(map[string]any); ok {
				name = stringField(fn, "name")
			}
		}
		if name == "" {
			return nil
		}
		name = resolveToolName(name, allowed)
		if name == "" {
			return nil
		}
		args := t["arguments"]
		if args == nil {
			args = t["parameters"]
		}
		if args == nil {
			if fn, ok := t["function"].(map[string]any); ok {
				args = fn["arguments"]
			}
		}
		argStr := "{}"
		switch a := args.(type) {
		case string:
			argStr = a
		case nil:
			argStr = "{}"
		default:
			b, _ := json.Marshal(a)
			argStr = string(b)
		}
		id := stringField(t, "id")
		if id == "" {
			id = "call_repair_" + name
		}
		return []any{
			map[string]any{
				"id":   id,
				"type": "function",
				"function": map[string]any{
					"name":      name,
					"arguments": argStr,
				},
			},
		}
	default:
		return nil
	}
}

func stringField(m map[string]any, k string) string {
	if s, ok := m[k].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func resolveToolName(name string, allowed map[string]bool) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if len(allowed) == 0 {
		return name
	}
	if allowed[name] {
		return name
	}
	lower := strings.ToLower(name)
	for a := range allowed {
		if strings.ToLower(a) == lower {
			return a
		}
	}
	// common aliases from weak models inventing names
	aliases := map[string][]string{
		"execute": {"bash", "shell", "run", "terminal"},
		"run":     {"bash", "shell", "execute"},
		"shell":   {"bash", "execute"},
		"read":    {"read_file", "Read", "read"},
		"write":   {"write_file", "Edit", "edit", "write"},
		"edit":    {"Edit", "write_file", "write"},
		"grep":    {"Grep", "grep", "search"},
		"search":  {"grep", "Grep"},
	}
	for _, cand := range aliases[lower] {
		if allowed[cand] {
			return cand
		}
		for a := range allowed {
			if strings.EqualFold(a, cand) {
				return a
			}
		}
	}
	return ""
}

func toolNamesFromRequest(body []byte) map[string]bool {
	var meta struct {
		Tools []struct {
			Type     string `json:"type"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
			Name string `json:"name"`
		} `json:"tools"`
	}
	_ = json.Unmarshal(body, &meta)
	out := map[string]bool{}
	for _, t := range meta.Tools {
		n := t.Function.Name
		if n == "" {
			n = t.Name
		}
		if n != "" {
			out[n] = true
		}
	}
	return out
}

func forceStreamFalse(body []byte) []byte {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	m["stream"] = false
	delete(m, "stream_options")
	b, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return b
}
