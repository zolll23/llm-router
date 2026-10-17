package proxy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRepairToolCallsFromContentJSON(t *testing.T) {
	body := []byte(`{
	  "choices":[{
	    "message":{
	      "role":"assistant",
	      "content":"{\"name\":\"grep\",\"arguments\":{\"pattern\":\"flow\"}}"
	    }
	  }]
	}`)
	allowed := map[string]bool{"grep": true, "bash": true}
	out, ok := RepairToolCallsInChatJSON(body, allowed)
	if !ok {
		t.Fatal("expected repair")
	}
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatal(err)
	}
	msg := root["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	tcs := msg["tool_calls"].([]any)
	if len(tcs) != 1 {
		t.Fatalf("tool_calls=%v", tcs)
	}
	fn := tcs[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != "grep" {
		t.Fatalf("name=%v", fn["name"])
	}
	if msg["content"] != "" {
		t.Fatalf("content should be cleared: %v", msg["content"])
	}
}

func TestRepairMapsExecuteAlias(t *testing.T) {
	content := "```json\n" + `{"name":"execute","arguments":{"command":"ls"}}` + "\n```"
	payload := map[string]any{
		"choices": []any{
			map[string]any{"message": map[string]any{"content": content}},
		},
	}
	body, _ := json.Marshal(payload)
	allowed := map[string]bool{"bash": true}
	out, ok := RepairToolCallsInChatJSON(body, allowed)
	if !ok {
		t.Fatal("expected repair")
	}
	var root map[string]any
	_ = json.Unmarshal(out, &root)
	fn := root["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != "bash" {
		t.Fatalf("got %v", fn["name"])
	}
}

func TestRepairSkipsUnknownTool(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"content":"{\"name\":\"execute\",\"arguments\":{}}"}}]}`)
	allowed := map[string]bool{"read_file": true}
	_, ok := RepairToolCallsInChatJSON(body, allowed)
	if ok {
		t.Fatal("should not repair unknown/unaliased tool")
	}
}

func TestRepairBacktickCodeArgument(t *testing.T) {
	content := "```json\n{\n  \"name\": \"execute\",\n  \"arguments\": {\n    \"code\": `\n      const x = `<div>${y}</div>`;\n      await tools.grep({pattern: \"flow\"});\n    `\n  }\n}\n```"
	payload := map[string]any{
		"choices": []any{
			map[string]any{"message": map[string]any{"role": "assistant", "content": content}},
		},
	}
	body, _ := json.Marshal(payload)
	allowed := map[string]bool{"execute": true}
	out, ok := RepairToolCallsInChatJSON(body, allowed)
	if !ok {
		t.Fatal("expected repair of backtick execute payload")
	}
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatal(err)
	}
	msg := root["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "" {
		t.Fatalf("content should be cleared, got %q", msg["content"])
	}
	fn := msg["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != "execute" {
		t.Fatalf("name=%v", fn["name"])
	}
	args := fn["arguments"].(string)
	if !strings.Contains(args, "tools.grep") {
		t.Fatalf("arguments missing code: %s", args)
	}
}
