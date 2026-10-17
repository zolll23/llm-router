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
	Category   string  // coding|reasoning|chat|creative
	NeedsTools bool
	Confidence float64
	Source     string // gliner|nimble|heuristic
}

type Classifier interface {
	Classify(ctx context.Context, in Input) (Result, error)
}
