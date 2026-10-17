package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"router-app/internal/classifier"
	"router-app/internal/provider"
	"router-app/internal/router"
	"router-app/internal/store"
)

type RoutingSettings struct {
	EscalateMin              string
	QuotaThresholdRollingPct float64
	QuotaThresholdWeeklyPct  float64
	QuotaThresholdMonthlyPct float64
	QuotaCooldownSeconds     int
	KeepAlive                string
}

type Deps struct {
	Reg        *provider.Registry
	Classifier classifier.Classifier
	Sticky     *router.StickyStore
	DB         *store.DB
	Quotas     *provider.QuotaStore
	Perf       *provider.PerfStore
	Settings   func() RoutingSettings
	KeepAlive  string
}

type ChatResult struct {
	Response *http.Response
	Model    provider.ModelInfo
	Reason   string
	Decision router.DecideOutput
	Class    classifier.Result
	Session  string
	Usage    store.UsageEvent
}

type chatRequestMeta struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	Messages []struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	} `json:"messages"`
	Tools  []json.RawMessage `json:"tools"`
	Router *struct {
		Tier string `json:"tier"`
	} `json:"router"`
}

func extractText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var b strings.Builder
		for _, part := range v {
			if m, ok := part.(map[string]any); ok {
				if m["type"] == "text" {
					if s, ok := m["text"].(string); ok {
						b.WriteString(s)
					}
				}
			}
		}
		return b.String()
	default:
		return ""
	}
}

func parseMeta(raw []byte) (chatRequestMeta, string, string, int) {
	var meta chatRequestMeta
	_ = json.Unmarshal(raw, &meta)
	var system, user string
	for _, m := range meta.Messages {
		t := extractText(m.Content)
		switch m.Role {
		case "system":
			if system == "" {
				system = t
			}
		case "user":
			user = t
		}
	}
	approx := len(raw) / 4
	return meta, system, user, approx
}

func rewriteModel(raw []byte, upstreamID string, keepAlive string) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	m["model"] = upstreamID
	if stream, ok := m["stream"].(bool); ok && stream {
		so, _ := m["stream_options"].(map[string]any)
		if so == nil {
			so = map[string]any{}
		}
		so["include_usage"] = true
		m["stream_options"] = so
	}
	if keepAlive != "" {
		// Ollama OpenAI compat may ignore; also set options-like field harmlessly
		m["keep_alive"] = keepAlive
	}
	return json.Marshal(m)
}

func ProxyChat(ctx context.Context, d Deps, body []byte, hdr http.Header) (*ChatResult, error) {
	start := time.Now()
	meta, system, user, approx := parseMeta(body)
	session := hdr.Get("X-Router-Session")
	if session == "" {
		session = router.SessionKeyFromSystem(system)
	}
	force := hdr.Get("X-Router-Tier")
	if force == "" && meta.Router != nil {
		force = meta.Router.Tier
	}

	classIn := classifier.Input{
		UserText:     user,
		ToolsPresent: len(meta.Tools) > 0,
		ApproxTokens: approx,
		MessageCount: len(meta.Messages),
	}
	class, err := d.Classifier.Classify(ctx, classIn)
	if err != nil {
		return nil, err
	}
	if class.Category == "" {
		if fb, ferr := (&classifier.Heuristic{}).Classify(ctx, classIn); ferr == nil {
			class.Category = fb.Category
		}
		if class.Category == "" {
			class.Category = "chat"
		}
	}

	rs := RoutingSettings{EscalateMin: "hard", QuotaCooldownSeconds: 600}
	if d.Settings != nil {
		rs = d.Settings()
	}
	thr := provider.QuotaThresholds{
		Rolling: rs.QuotaThresholdRollingPct,
		Weekly:  rs.QuotaThresholdWeeklyPct,
		Monthly: rs.QuotaThresholdMonthlyPct,
	}
	if thr.Rolling <= 0 {
		thr = provider.DefaultQuotaThresholds()
	}
	extOK := func(pid string) bool {
		if d.Quotas == nil {
			return true
		}
		return d.Quotas.ExternalOK(pid, thr, time.Now())
	}

	dec, err := router.Decide(ctx, router.DecideInput{
		Class:       class,
		Models:      d.Reg.Models(),
		SessionKey:  session,
		Sticky:      d.Sticky,
		ForceTier:   force,
		EscalateMin: rs.EscalateMin,
		ExternalOK:  extOK,
	})
	if err != nil {
		return nil, err
	}

	resp, model, dec, err := forwardChat(ctx, d, body, hdr, dec, rs.KeepAlive)
	if err != nil {
		return nil, err
	}

	// On rate limit: cooldown + one retry on best local
	if resp != nil && resp.StatusCode == http.StatusTooManyRequests && d.Quotas != nil {
		cd := time.Duration(rs.QuotaCooldownSeconds) * time.Second
		if cd <= 0 {
			cd = 10 * time.Minute
		}
		d.Quotas.MarkCooldown(model.ProviderID, time.Now().Add(cd))
		_ = resp.Body.Close()
		localDec, lerr := router.Decide(ctx, router.DecideInput{
			Class:       class,
			Models:      d.Reg.Models(),
			SessionKey:  session,
			Sticky:      d.Sticky,
			ForceTier:   force,
			EscalateMin: rs.EscalateMin,
			LocalOnly:   true,
			ExternalOK:  func(string) bool { return false },
		})
		if lerr == nil {
			resp2, model2, dec2, err2 := forwardChat(ctx, d, body, hdr, localDec, rs.KeepAlive)
			if err2 == nil {
				resp, model, dec = resp2, model2, dec2
			}
		}
	}

	res := &ChatResult{
		Response: resp,
		Model:    model,
		Reason:   dec.Reason,
		Decision: dec,
		Class:    class,
		Session:  session,
		Usage: store.UsageEvent{
			ModelID:          model.ID(),
			ProviderID:       model.ProviderID,
			SessionID:        session,
			Category:         class.Category,
			Complexity:       class.Complexity,
			Pool:             store.PoolLabel(model.ProviderID, model.CostTier),
			Confidence:       class.Confidence,
			ClassifierSource: class.Source,
			HTTPStatus:       resp.StatusCode,
			LatencyMS:        int(time.Since(start).Milliseconds()),
		},
	}
	return res, nil
}

func forwardChat(ctx context.Context, d Deps, body []byte, hdr http.Header, dec router.DecideOutput, keepAlive string) (*http.Response, provider.ModelInfo, router.DecideOutput, error) {
	_, prov, ok := d.Reg.Get(dec.Model.ID())
	if !ok {
		return nil, provider.ModelInfo{}, dec, fmt.Errorf("provider missing for %s", dec.Model.ID())
	}
	ka := keepAlive
	if ka == "" {
		ka = d.KeepAlive
	}
	if ka == "" && dec.Model.CostTier == "local" {
		ka = "60m"
	}
	rewritten, err := rewriteModel(body, dec.Model.UpstreamID, ka)
	if err != nil {
		return nil, provider.ModelInfo{}, dec, err
	}
	resp, err := prov.ChatCompletions(ctx, rewritten, hdr)
	if err != nil {
		return nil, provider.ModelInfo{}, dec, err
	}
	return resp, dec.Model, dec, nil
}

func CopyAndRecord(w http.ResponseWriter, d Deps, res *ChatResult, stateText string) error {
	resp := res.Response
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("X-Router-Model", res.Model.ID())
	w.Header().Set("X-Router-Provider", res.Model.ProviderID)
	w.Header().Set("X-Router-Tier", res.Class.Complexity)
	w.Header().Set("X-Router-Reason", res.Reason)
	w.WriteHeader(resp.StatusCode)

	ct := resp.Header.Get("Content-Type")
	var bodyBuf bytes.Buffer
	tee := io.TeeReader(resp.Body, &bodyBuf)
	if strings.Contains(ct, "text/event-stream") {
		if err := copySSE(w, tee); err != nil {
			return err
		}
		usage := parseUsageFromSSE(bodyBuf.Bytes())
		applyUsage(&res.Usage, usage)
	} else {
		if _, err := io.Copy(w, tee); err != nil {
			return err
		}
		usage := parseUsageFromJSON(bodyBuf.Bytes())
		applyUsage(&res.Usage, usage)
	}

	if d.Perf != nil && res.Usage.LatencyMS > 0 {
		d.Perf.Observe(res.Model.ID(), int64(res.Usage.LatencyMS), res.Usage.CompletionTokens)
	}
	if d.DB != nil {
		_ = d.DB.InsertUsage(res.Usage)
		_ = d.DB.InsertRouting(store.RoutingEvent{
			RequestID:        fmt.Sprintf("%d", time.Now().UnixNano()),
			SessionID:        res.Session,
			StateText:        stateText,
			PredCategory:     res.Class.Category,
			PredComplexity:   res.Class.Complexity,
			PredNeedsTools:   res.Class.NeedsTools,
			PredConfidence:   res.Class.Confidence,
			ClassifierSource: res.Class.Source,
			ChosenModel:      res.Model.ID(),
			CostTier:         res.Model.CostTier,
			Pool:             store.PoolLabel(res.Model.ProviderID, res.Model.CostTier),
			Reason:           res.Reason,
			HTTPStatus:       resp.StatusCode,
			StickyBreak:      res.Decision.StickyBreak,
			UserOverride:     res.Decision.UserOverride,
			LocalSuccess:     res.Model.CostTier == "local" && resp.StatusCode < 400,
		})
	}
	return nil
}

func copySSE(w http.ResponseWriter, r io.Reader) error {
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

type usageBits struct {
	Prompt, Cached, Completion int
	OK                         bool
}

func applyUsage(dst *store.UsageEvent, u usageBits) {
	if !u.OK {
		dst.Estimated = true
		return
	}
	dst.PromptTokens = u.Prompt
	dst.CachedTokens = u.Cached
	dst.CompletionTokens = u.Completion
}

func parseUsageFromJSON(b []byte) usageBits {
	var root struct {
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(b, &root); err != nil {
		return usageBits{}
	}
	if root.Usage.PromptTokens == 0 && root.Usage.CompletionTokens == 0 {
		return usageBits{}
	}
	return usageBits{
		Prompt: root.Usage.PromptTokens, Cached: root.Usage.PromptTokensDetails.CachedTokens,
		Completion: root.Usage.CompletionTokens, OK: true,
	}
}

func parseUsageFromSSE(b []byte) usageBits {
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	var last usageBits
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			continue
		}
		u := parseUsageFromJSON([]byte(payload))
		if u.OK {
			last = u
		}
	}
	return last
}
