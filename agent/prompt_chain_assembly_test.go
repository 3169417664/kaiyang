package agent

import (
	"strings"
	"testing"

	"github.com/3169417664/kaiyang/hosttools"
)

// These tests pin the single-assembly contract: the projection the read-only
// chain page renders must be built by the same RuntimeSystemSegments the
// runtime builders call, so the two cannot drift. A failure here means either a
// builder bypassed RuntimeSystem or a tail was added/removed on one side only.

func TestRuntimeSystemSegmentsMatchRuntimeBuilders(t *testing.T) {
	withSkillIndex(t, "SKILLS")
	prevP, prevW := PlannerTemplateDefault, WorkerTemplateDefault
	PlannerTemplateDefault, WorkerTemplateDefault = nil, nil
	t.Cleanup(func() { PlannerTemplateDefault, WorkerTemplateDefault = prevP, prevW })

	pp := ProfilePrompt{Text: "OVERLAY", Mode: "prepend"}
	const goal, dataDir, workDir = "target.example.com", "/data", "/data"

	t.Run("planner", func(t *testing.T) {
		segs, warn := RuntimeSystemSegments("planner", DefaultPromptFor("planner"), "builtin_default",
			PlannerVars{Goal: goal, DataDir: dataDir, Now: nowStr()}, "", pp, PlannerTailOptions(goal, workDir))
		if warn != "" {
			t.Fatalf("unexpected warning: %s", warn)
		}
		got := plannerSystemWith(goal, dataDir, workDir, "", pp)
		if want := JoinPromptSegments(segs); got != want {
			t.Fatalf("planner runtime and projection diverged")
		}
		assertTailSuffix(t, got, "planner", PlannerTailOptions(goal, workDir))
		assertKeys(t, segs, []string{"profile_prompt", "body", "skill_index",
			"exec_mode", "artifact_spec", "coverage_matrix", "scope_verify", "exec_face_planner",
			"engagement_framing", "impact_orchestration", "blackboard"})
	})

	t.Run("worker", func(t *testing.T) {
		opt := WorkerTailOptions("127.0.0.1:8080", "/ca.pem", "/data/run")
		segs, warn := RuntimeSystemSegments("worker", DefaultPromptFor("worker"), "builtin_default",
			WorkerVars{ProxyAddr: "127.0.0.1:8080", WorkerName: "w", DataDir: dataDir, Now: nowStr()}, "", pp, opt)
		if warn != "" {
			t.Fatalf("unexpected warning: %s", warn)
		}
		got := workerSystemWith("worker", "127.0.0.1:8080", "/ca.pem", dataDir, "/data/run", "", pp)
		if want := JoinPromptSegments(segs); got != want {
			t.Fatalf("worker runtime and projection diverged")
		}
		assertTailSuffix(t, got, "worker", opt)
		keys := []string{"profile_prompt", "body", "skill_index", "traffic_tools", "egress_proxy",
			"artifact_spec", "scanner_first", "skill_routing", "exec_face_worker",
			"engagement_framing", "impact_verification", "foothold_fact", "host_tools"}
		assertKeys(t, segs, keys)
		if (hosttools.SummaryText() != "") != segByKey(segs, "host_tools").Present {
			t.Fatal("host_tools presence must follow the host inventory")
		}
	})

	t.Run("mainagent", func(t *testing.T) {
		segs, warn := RuntimeSystemSegments("mainagent", mainAgentDefaultTmpl, "builtin_default",
			MainVars{Goal: goal, DataDir: dataDir, Now: nowStr()}, "", pp, MainAgentTailOptions(workDir))
		if warn != "" {
			t.Fatalf("unexpected warning: %s", warn)
		}
		got := mainAgentSystemWith(goal, dataDir, workDir, pp)
		if want := JoinPromptSegments(segs); got != want {
			t.Fatalf("mainagent runtime and projection diverged")
		}
		assertTailSuffix(t, got, "mainagent", MainAgentTailOptions(workDir))
	})

	t.Run("chat variants", func(t *testing.T) {
		for _, key := range []string{"pentest", "reporter", "retester", "auto", "custom_x"} {
			opt := ChatTailOptions(workDir)
			segs, warn := RuntimeSystemSegments(key, DefaultAssistantPrompt, "builtin_default",
				chatVars{DataDir: dataDir, Now: nowStr()}, "", pp, opt)
			if warn != "" {
				t.Fatalf("%s unexpected warning: %s", key, warn)
			}
			got := chatSystemWith(key, dataDir, workDir, pp)
			if want := JoinPromptSegments(segs); got != want {
				t.Fatalf("%s runtime and projection diverged", key)
			}
			assertTailSuffix(t, got, key, opt)
			wantKeys := []string{"profile_prompt", "body", "skill_index", "chat_workdir"}
			switch key {
			case "pentest":
				wantKeys = append(wantKeys, "engagement_framing", "impact_verification", "impact_orchestration", "src_report_annex")
			case "reporter":
				wantKeys = append(wantKeys, "engagement_framing", "src_report_annex")
			case "retester":
				wantKeys = append(wantKeys, "engagement_framing")
			}
			assertKeys(t, segs, wantKeys)
		}
	})

	t.Run("goals scope gate", func(t *testing.T) {
		without := StaticTailSegments("goals", GoalsTailOptions("", false))
		with := StaticTailSegments("goals", GoalsTailOptions("", true))
		if segByKey(without, "goals_scope").Present {
			t.Fatal("goals_scope must stay gated without an asset store")
		}
		scope := segByKey(with, "goals_scope")
		if !scope.Present || scope.Text != goalsScopeTail {
			t.Fatal("goals_scope must be appended when the runtime has scope context")
		}
		noExec := segByKey(without, "goals_exec_mode")
		if noExec.Present {
			t.Fatal("goals_exec_mode must stay gated when the goal is not vulnerability-only")
		}
	})
}

// assertTailSuffix verifies the runtime output ends with exactly the code-owned
// tails for that agent: the shared StaticTailSegments is the only source.
func assertTailSuffix(t *testing.T, got, agentKey string, o TailOptions) {
	t.Helper()
	tails := JoinPromptSegments(StaticTailSegments(agentKey, o))
	if tails == "" {
		return
	}
	if !strings.HasSuffix(got, tails) {
		t.Fatalf("%s output does not end with the shared static tails", agentKey)
	}
}

func assertKeys(t *testing.T, segs []PromptSegment, want []string) {
	t.Helper()
	got := make([]string, 0, len(segs))
	for _, s := range segs {
		got = append(got, s.Key)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("segment keys:\n got %v\nwant %v", got, want)
	}
}

func segByKey(segs []PromptSegment, key string) PromptSegment {
	for _, s := range segs {
		if s.Key == key {
			return s
		}
	}
	return PromptSegment{}
}

func TestRuntimeBodyForCustomExecutorAndChat(t *testing.T) {
	def, src, key := RuntimeBodyFor("planner", false)
	if key != "planner" || def != DefaultPromptFor("planner") || src == "" {
		t.Fatalf("planner mapping wrong: key=%q src=%q", key, src)
	}
	// A custom executor agent runs the worker assembly but keeps its own key for
	// the DB prompt override.
	if _, _, key := RuntimeBodyFor("custom_exec", true); key != "worker" {
		t.Fatalf("custom executor must use worker assembly, got %q", key)
	}
	if def, _, _ := RuntimeBodyFor("custom_chat", false); def != DefaultAssistantPrompt {
		t.Fatal("custom chat agent must fall back to the assistant starter prompt")
	}
}

// TestPreviewSystemStaysReadOnly pins that the preview inputs never expose a
// real proxy address and that every segment carries a source the UI can label.
func TestPreviewSystemStaysReadOnly(t *testing.T) {
	withSkillIndex(t, "")
	opt := WorkerTailOptions("<出口代理>", "", "/data/run")
	segs := StaticTailSegments("worker", opt)
	for _, s := range segs {
		if s.Key == "egress_proxy" && strings.Contains(s.Text, "127.0.0.1") {
			t.Fatal("preview must not render a real egress proxy")
		}
		if s.Source == "" {
			t.Fatalf("segment %s has no source", s.Key)
		}
	}
}
