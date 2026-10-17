package classifier

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

type GLiNER struct {
	BaseURL  string
	Client   *http.Client
	Fallback Classifier
}

func NewGLiNER(baseURL string, fallback Classifier) *GLiNER {
	if fallback == nil {
		fallback = &Heuristic{}
	}
	return &GLiNER{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Client:   &http.Client{Timeout: 30 * time.Second},
		Fallback: fallback,
	}
}

type glinerReq struct {
	Text  string                `json:"text"`
	Tasks map[string]glinerTask `json:"tasks"`
}

type glinerTask struct {
	Labels []string `json:"labels"`
}

type glinerResp struct {
	Results map[string]struct {
		Label         string             `json:"label"`
		Confidence    float64            `json:"confidence"`
		Probabilities map[string]float64 `json:"probabilities"`
	} `json:"results"`
}

func (g *GLiNER) Classify(ctx context.Context, in Input) (Result, error) {
	reqBody := glinerReq{
		Text: in.UserText,
		Tasks: map[string]glinerTask{
			"complexity":  {Labels: []string{"trivial", "simple", "medium", "hard"}},
			"needs_tools": {Labels: []string{"yes", "no"}},
		},
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return g.Fallback.Classify(ctx, in)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.BaseURL+"/v1/classify", bytes.NewReader(b))
	if err != nil {
		return g.Fallback.Classify(ctx, in)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := g.Client.Do(httpReq)
	if err != nil {
		return g.Fallback.Classify(ctx, in)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return g.Fallback.Classify(ctx, in)
	}
	var parsed glinerResp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return g.Fallback.Classify(ctx, in)
	}
	comp, ok := parsed.Results["complexity"]
	if !ok || comp.Label == "" {
		return g.Fallback.Classify(ctx, in)
	}
	c := strings.ToLower(comp.Label)
	switch c {
	case "trivial", "simple", "medium", "hard":
	default:
		return g.Fallback.Classify(ctx, in)
	}
	needs := in.ToolsPresent
	if nt, ok := parsed.Results["needs_tools"]; ok {
		switch strings.ToLower(nt.Label) {
		case "yes", "true", "1":
			needs = true
		case "no", "false", "0":
			needs = false
		}
	}
	conf := comp.Confidence
	if conf == 0 {
		conf = 0.7
	}
	return Result{Complexity: c, NeedsTools: needs, Confidence: conf, Source: "gliner"}, nil
}
