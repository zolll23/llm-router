package router

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"router/internal/classifier"
	"router/internal/provider"
)

type DecideInput struct {
	Class      classifier.Result
	Models     []provider.ModelInfo
	SessionKey string
	Sticky     *StickyStore
	Now        time.Time
	ForceTier  string // optional override complexity
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

	if in.Sticky != nil && in.SessionKey != "" && !userOverride {
		if id, ok := in.Sticky.Get(in.SessionKey, now); ok {
			for _, m := range in.Models {
				if m.ID() == id && m.Routable {
					return DecideOutput{Model: m, Reason: "sticky", StickyHit: true}, nil
				}
			}
			in.Sticky.Clear(in.SessionKey)
			out, err := decideFresh(class, in.Models)
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

	out, err := decideFresh(class, in.Models)
	if err != nil {
		return DecideOutput{}, err
	}
	out.UserOverride = userOverride
	if userOverride {
		out.Reason = "override:" + class.Complexity
	}
	if in.Sticky != nil && in.SessionKey != "" {
		in.Sticky.Set(in.SessionKey, out.Model.ID(), now)
	}
	return out, nil
}

func decideFresh(class classifier.Result, models []provider.ModelInfo) (DecideOutput, error) {
	m, ok := pickBest(models, class)
	if !ok {
		return DecideOutput{}, fmt.Errorf("no routable models for complexity %s", class.Complexity)
	}
	return DecideOutput{Model: m, Reason: "score:" + class.Complexity + ":" + m.CostTier}, nil
}
