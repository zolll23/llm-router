package classifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Nimble struct {
	BaseURL string
	Model   string
	Client  *http.Client
	Fallback Classifier
}

func NewNimble(baseURL, model string, fallback Classifier) *Nimble {
	if model == "" {
		model = "nimble"
	}
	if fallback == nil {
		fallback = &Heuristic{}
	}
	return &Nimble{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Model:    model,
		Client:   &http.Client{Timeout: 30 * time.Second},
		Fallback: fallback,
	}
}

type systemOneReq struct {
	Model     string         `json:"model"`
	State     any            `json:"state"`
	Questions map[string]any `json:"questions"`
}

type systemOneResp struct {
	Answers map[string]struct {
		Type          string             `json:"type"`
		Choice        string             `json:"choice"`
		Noul          float64            `json:"noul"`
		Confidence    float64            `json:"confidence"`
		Probabilities map[string]float64 `json:"probabilities"`
	} `json:"answers"`
}

func (n *Nimble) Classify(ctx context.Context, in Input) (Result, error) {
	reqBody := systemOneReq{
		Model: n.Model,
		State: map[string]any{
			"user_text":      in.UserText,
			"tools_present":  in.ToolsPresent,
			"approx_tokens":  in.ApproxTokens,
			"message_count":  in.MessageCount,
		},
		Questions: map[string]any{
			"complexity": map[string]any{
				"type":         "choice",
				"instructions": "How complex is this coding/agent request?",
				"criteria": map[string]string{
					"trivial": "Greeting or one-line factual answer",
					"simple":  "Small local edit, rename, typo, short explanation",
					"medium":  "Multi-step coding, debugging, tests",
					"hard":    "Architecture, large refactor, deep reasoning",
				},
			},
			"needs_tools": map[string]any{
				"type":         "noul",
				"instructions": "Does fulfilling this request require tools (file edits, shell, search)?",
			},
		},
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return n.Fallback.Classify(ctx, in)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, n.BaseURL+"/v1/systemone", bytes.NewReader(b))
	if err != nil {
		return n.Fallback.Classify(ctx, in)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := n.Client.Do(httpReq)
	if err != nil {
		return n.Fallback.Classify(ctx, in)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return n.Fallback.Classify(ctx, in)
	}
	var parsed systemOneResp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return n.Fallback.Classify(ctx, in)
	}
	compAns, ok := parsed.Answers["complexity"]
	if !ok || compAns.Choice == "" {
		return n.Fallback.Classify(ctx, in)
	}
	needs := in.ToolsPresent
	if nt, ok := parsed.Answers["needs_tools"]; ok {
		needs = nt.Noul >= 0.5
	}
	conf := compAns.Confidence
	if conf == 0 {
		conf = 0.7
	}
	c := strings.ToLower(compAns.Choice)
	switch c {
	case "trivial", "simple", "medium", "hard":
	default:
		return Result{}, fmt.Errorf("unexpected complexity %q", c)
	}
	return Result{Complexity: c, NeedsTools: needs, Confidence: conf, Source: "nimble"}, nil
}
