package classifier

import "context"

type Input struct {
	UserText     string
	ToolsPresent bool
	ApproxTokens int
	MessageCount int
}

type Result struct {
	Complexity string  // trivial|simple|medium|hard
	NeedsTools bool
	Confidence float64
	Source     string // nimble|heuristic
}

type Classifier interface {
	Classify(ctx context.Context, in Input) (Result, error)
}
