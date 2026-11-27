package server

import (
	"testing"

	"github.com/3169417664/kaiyang/agent"
)

func TestPreviewTailOptionsNeverCarryRealEgress(t *testing.T) {
	opt := previewTailOptions("worker", false)
	if opt.ProxyAddr != "<出口代理>" || opt.CAEnabled {
		t.Fatalf("worker preview must be a credential-free placeholder: %+v", opt)
	}
	if opt = previewTailOptions("worker", true); !opt.CAEnabled || opt.RunDir == "" {
		t.Fatalf("traffic capture on should surface the traffic tail gate: %+v", opt)
	}
	if opt = previewTailOptions("planner", false); opt.WorkDir == "" {
		t.Fatal("planner preview must carry a visible work-dir placeholder")
	}
	if opt = previewTailOptions("unknown_chat", false); opt.WorkDir != "<会话工作目录>" {
		t.Fatalf("chat preview work dir: %q", opt.WorkDir)
	}
}

func TestPromptChainKindsAndLabels(t *testing.T) {
	if got := promptChainKind(agent.PromptSegment{Key: "body", Source: "db_template"}); got != "body" {
		t.Fatalf("body kind = %q", got)
	}
	if got := promptChainKind(agent.PromptSegment{Key: "artifact_spec", Source: "code"}); got != "code_tail" {
		t.Fatalf("code tail kind = %q", got)
	}
	if got := promptChainKind(agent.PromptSegment{Key: "host_tools", Source: "host"}); got != "host" {
		t.Fatalf("host tools kind = %q", got)
	}
	if got := promptChainKind(agent.PromptSegment{Key: "deferred_tools", Source: "deferred"}); got != "deferred_tools" {
		t.Fatalf("deferred kind = %q", got)
	}

	pp := agent.ProfilePrompt{Source: "agent_binding"}
	if got := promptChainSegmentLabel(agent.PromptSegment{Key: "profile_prompt"}, pp, ""); got != "LLM 配置 · 显式绑定" {
		t.Fatalf("profile label = %q", got)
	}
	if got := promptChainSegmentLabel(agent.PromptSegment{Key: "finding_guidance", Source: "finding_guidance"}, pp, ""); got != "按最终工具集派生" {
		t.Fatalf("finding guidance label = %q", got)
	}
}

func TestPromptChainRuntimeInjectionPoints(t *testing.T) {
	keys := map[string]bool{}
	for _, seg := range promptChainRuntimeSegments("worker") {
		keys[seg.Key] = true
		if seg.Present || seg.Available || seg.Text != "" {
			t.Fatalf("runtime injection point %s must stay body-less in the static view", seg.Key)
		}
	}
	for _, want := range []string{"runtime_task_auth", "runtime_launch", "runtime_intent"} {
		if !keys[want] {
			t.Fatalf("missing runtime injection point %q", want)
		}
	}
	if promptChainRuntimeSegments("goals") != nil {
		t.Fatal("goals must not advertise runtime injection points")
	}
}
