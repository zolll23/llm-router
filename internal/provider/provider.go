package provider

import (
	"context"
	"errors"
	"net/http"
)

var ErrNotRoutable = errors.New("provider cannot chat")

type ModelInfo struct {
	ProviderID  string
	UpstreamID  string
	DisplayName string
	Routable    bool
	Tools       bool
	Vision      bool
	Context     int
	CostTier    string // local | metered | unknown
	Capabilities []string // coding, reasoning, general, creative
	Params      int      // size in billions (e.g. 7, 70)
}

func CanonicalID(providerID, upstreamID string) string {
	return providerID + "/" + upstreamID
}

func (m ModelInfo) ID() string {
	return CanonicalID(m.ProviderID, m.UpstreamID)
}

type Provider interface {
	ID() string
	ListModels(ctx context.Context) ([]ModelInfo, error)
	ChatCompletions(ctx context.Context, rawBody []byte, hdr http.Header) (*http.Response, error)
}
