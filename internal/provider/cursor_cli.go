package provider

import (
	"bytes"
	"context"
	"net/http"
	"os/exec"
	"strings"
)

type CursorCLI struct {
	id      string
	command string
}

func NewCursorCLI(id, command string) *CursorCLI {
	return &CursorCLI{id: id, command: command}
}

func (c *CursorCLI) ID() string { return c.id }

func (c *CursorCLI) ListModels(ctx context.Context) ([]ModelInfo, error) {
	cmd := exec.CommandContext(ctx, c.command, "--list-models")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	lines := strings.Split(stdout.String(), "\n")
	var out []ModelInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Available") {
			continue
		}
		// format: "id - Display Name"
		id := line
		if i := strings.Index(line, " - "); i > 0 {
			id = strings.TrimSpace(line[:i])
		}
		if id == "" {
			continue
		}
		out = append(out, ModelInfo{
			ProviderID:  c.id,
			UpstreamID:  id,
			DisplayName: line,
			Routable:    false, // chat path spike later
			Tools:       true,
			CostTier:    "metered",
			Context:     128000,
		})
	}
	return out, nil
}

func (c *CursorCLI) ChatCompletions(ctx context.Context, rawBody []byte, hdr http.Header) (*http.Response, error) {
	return nil, ErrNotRoutable
}
