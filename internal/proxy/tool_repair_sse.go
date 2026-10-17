package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func jsonBodyResponse(status int, body []byte, contentType string) *http.Response {
	if contentType == "" {
		contentType = "application/json"
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{contentType}},
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}
}

// chatJSONToSSE wraps a full chat.completion JSON into a short SSE stream
// so stream-expecting clients still accept repaired tool_calls.
func chatJSONToSSE(body []byte) []byte {
	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return body
	}
	id, _ := root["id"].(string)
	if id == "" {
		id = "chatcmpl-repair"
	}
	model, _ := root["model"].(string)
	choices, _ := root["choices"].([]any)
	if len(choices) == 0 {
		return body
	}
	ch, _ := choices[0].(map[string]any)
	msg, _ := ch["message"].(map[string]any)
	if msg == nil {
		return body
	}
	delta := map[string]any{"role": "assistant"}
	if c, ok := msg["content"].(string); ok && c != "" {
		delta["content"] = c
	}
	if tcs, ok := msg["tool_calls"]; ok {
		delta["tool_calls"] = tcs
	}
	chunk1 := map[string]any{
		"id": id, "object": "chat.completion.chunk", "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}},
	}
	fr, _ := ch["finish_reason"].(string)
	if fr == "" {
		fr = "stop"
	}
	chunk2 := map[string]any{
		"id": id, "object": "chat.completion.chunk", "model": model,
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": fr}},
	}
	b1, _ := json.Marshal(chunk1)
	b2, _ := json.Marshal(chunk2)
	return []byte(fmt.Sprintf("data: %s\n\ndata: %s\n\ndata: [DONE]\n\n", b1, b2))
}

func readAllBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
