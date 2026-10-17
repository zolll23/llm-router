package router

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"router-app/internal/classifier"
	"router-app/internal/provider"
)

type DecideInput struct {
	Class        classifier.Result
	Models       []provider.ModelInfo
	SessionKey   string
	Sticky       *StickyStore
	Now          time.Time
	ForceTier    string // optional override complexity
	EscalateMin  string
	LocalOnly    bool
	ExternalOK   func(providerID string) bool
}

type DecideOutput struct {
	Model        provider.ModelInfo
	Reason       string
	StickyHit    bool
	StickyBreak  bool
	UserOverride bool
}

func SessionKeyFromSystem(system string) string {
	sum := sha256.Sum256([]byte(system))
	return hex.EncodeToString(sum[:16])
}

func Decide(ctx context.Context, in DecideInput) (DecideOutput, error) {
	_ = ctx
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	class := in.Class
	userOverride := false
	if in.ForceTier != "" {
		class.Complexity = in.ForceTier
		userOverride = true
	}

	opts := ScoreOpts{
		Escalate:   ShouldEscalate(class.Complexity, in.EscalateMin),
		LocalOnly:  in.LocalOnly,
		ExternalOK: in.ExternalOK,
	}

	if in.Sticky != nil && in.SessionKey != "" && !userOverride && !in.LocalOnly {
		if id, ok := in.Sticky.Get(in.SessionKey, now); ok {
			for _, m := range in.Models {
				if m.ID() != id || !m.Routable {
					continue
				}
				// Break sticky when escalation wants external but pin is local (or vice versa under cooldown)
				if opts.Escalate && m.CostTier == "local" {
					extOK := false
					if in.ExternalOK != nil {
						for _, cand := range in.Models {
							if cand.CostTier == "metered" && cand.Routable && in.ExternalOK(cand.ProviderID) {
								extOK = true
								break
							}
						}
					}
					if extOK {
						in.Sticky.Clear(in.SessionKey)
						out, err := decideFresh(class, in.Models, opts)
						if err != nil {
							return DecideOutput{}, err
						}
						out.StickyBreak = true
						out.Reason = "sticky-break-escalate"
						if in.Sticky != nil && in.SessionKey != "" {
							in.Sticky.Set(in.SessionKey, out.Model.ID(), now)
						}
						return out, nil
					}
				}
				return DecideOutput{Model: m, Reason: "sticky", StickyHit: true}, nil
			}
			in.Sticky.Clear(in.SessionKey)
			out, err := decideFresh(class, in.Models, opts)
			if err != nil {
				return DecideOutput{}, err
			}
			out.StickyBreak = true
			out.Reason = "sticky-miss-rescore"
			if in.Sticky != nil && in.SessionKey != "" {
				in.Sticky.Set(in.SessionKey, out.Model.ID(), now)
			}
			return out, nil
		}
	}

	out, err := decideFresh(class, in.Models, opts)
	if err != nil {
		return DecideOutput{}, err
	}
	out.UserOverride = userOverride
	if userOverride {
		out.Reason = "override:" + class.Complexity
	}
	if in.Sticky != nil && in.SessionKey != "" && !in.LocalOnly {
		in.Sticky.Set(in.SessionKey, out.Model.ID(), now)
	}
	return out, nil
}

func decideFresh(class classifier.Result, models []provider.ModelInfo, opts ScoreOpts) (DecideOutput, error) {
	m, ok := pickBest(models, class, opts)
	if !ok {
		return DecideOutput{}, fmt.Errorf("no routable models for complexity %s", class.Complexity)
	}
	reason := "score:" + class.Complexity + ":" + m.CostTier
	if opts.Escalate {
		reason += ":escalate"
	}
	if opts.LocalOnly {
		reason += ":local-only"
	}
	return DecideOutput{Model: m, Reason: reason}, nil
}
