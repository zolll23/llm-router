package provider

import "testing"

func TestEvalAgentOKAuto(t *testing.T) {
	ok7 := EvalAgentOK(ModelInfo{UpstreamID: "qwen:7b", Tools: true, Params: 7}, 14, "")
	if ok7.OK {
		t.Fatalf("7b should fail: %+v", ok7)
	}
	ok31 := EvalAgentOK(ModelInfo{UpstreamID: "gemma:31b", Tools: true, Params: 31}, 14, "")
	if !ok31.OK {
		t.Fatalf("31b should pass: %+v", ok31)
	}
	vl := EvalAgentOK(ModelInfo{UpstreamID: "qwen2.5-vl:14b", Tools: true, Params: 14}, 14, "")
	if vl.OK {
		t.Fatalf("vl should fail: %+v", vl)
	}
}

func TestEvalAgentOKOverride(t *testing.T) {
	allow := EvalAgentOK(ModelInfo{UpstreamID: "x:7b", Tools: true, Params: 7}, 14, OverrideAllow)
	if !allow.OK || allow.Source != OverrideAllow {
		t.Fatalf("%+v", allow)
	}
	deny := EvalAgentOK(ModelInfo{UpstreamID: "x:31b", Tools: true, Params: 31}, 14, OverrideDeny)
	if deny.OK || deny.Source != OverrideDeny {
		t.Fatalf("%+v", deny)
	}
}
