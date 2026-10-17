package provider

import (
	"fmt"
	"regexp"
	"strings"
)

// Override values for agent readiness.
const (
	OverrideAuto  = "auto"
	OverrideAllow = "allow"
	OverrideDeny  = "deny"
)

var agentNameDeny = regexp.MustCompile(`(?i)(vl|vision|abliterated)`)

// AgentOKResult is the effective OpenCode/agent readiness for a model.
type AgentOKResult struct {
	OK     bool   `json:"agent_ok"`
	Source string `json:"agent_ok_source"` // auto|allow|deny
	Reason string `json:"agent_ok_reason"`
}

// EvalAgentOK applies override then auto rules (min params, tools, name deny).
func EvalAgentOK(m ModelInfo, minParams int, override string) AgentOKResult {
	if minParams <= 0 {
		minParams = 14
	}
	switch strings.ToLower(strings.TrimSpace(override)) {
	case OverrideDeny:
		return AgentOKResult{OK: false, Source: OverrideDeny, Reason: "manual deny"}
	case OverrideAllow:
		return AgentOKResult{OK: true, Source: OverrideAllow, Reason: "manual allow"}
	}

	params := m.Params
	if params <= 0 {
		params = EstimateParamsFromName(m.UpstreamID)
		if params <= 0 {
			params = EstimateParamsFromName(m.DisplayName)
		}
	}
	name := m.UpstreamID + " " + m.DisplayName
	switch {
	case !m.Tools:
		return AgentOKResult{OK: false, Source: OverrideAuto, Reason: "no tools"}
	case params <= 0:
		return AgentOKResult{OK: false, Source: OverrideAuto, Reason: "unknown params"}
	case params < minParams:
		return AgentOKResult{OK: false, Source: OverrideAuto, Reason: fmt.Sprintf("params %d < %d", params, minParams)}
	case agentNameDeny.MatchString(name):
		return AgentOKResult{OK: false, Source: OverrideAuto, Reason: "name deny (vl/vision/abliterated)"}
	default:
		return AgentOKResult{OK: true, Source: OverrideAuto, Reason: fmt.Sprintf("params %d ≥ %d, tools", params, minParams)}
	}
}
