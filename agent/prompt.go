package agent

import (
	"bytes"
	"strings"
	"text/template"
	"time"
)

// PromptOverride, if set, returns the stored system-prompt template for an agent
// key and whether one exists. The server wires it to the PG agent_prompts table.
// When nil or no override exists, agents use their built-in default prompt — so
// behavior is identical until a user edits a prompt in the UI.
var PromptOverride func(agentKey string) (string, bool)

// Prompt-variable structs — fields mirror each agent's catalog (docs §5a) so a
// user template referencing a catalog variable renders; referencing anything else
// fails template execution and falls back to the built-in default.
type PlannerVars struct{ Goal, Scope, AssetSummary, DataDir, Now string }
type WorkerVars struct{ ProxyAddr, WorkerName, DataDir, Now string }
type MainVars struct{ Goal, AssetSummary, FindingsSummary, DataDir, Now string }
type GoalsVars struct{ EngagementDescription, DataDir, Now string }

// nowStr is the server-local wall-clock string exposed as the universal {{.Now}}
// prompt variable. renderSystem runs on every agent turn/round, so this is fresh
// each run — a prompt can subtract it from a fixed start stamp to reason about
// elapsed time (e.g. a timed benchmark's "last N hours" window).
func nowStr() string { return time.Now().Format("2006-01-02 15:04:05 MST") }

// renderSystem returns the rendered system-prompt BODY (段 [A]) for agentKey.
// Precedence: the DB-stored template (if any) over the built-in default template
// (def). BOTH are Go templates now — the built-in default is seeded into the DB
// verbatim, so the two paths render identically until a user edits the prompt.
// Rendering always runs (def used to be pre-substituted plain text; it is now a
// {{.Var}} template like the DB one). On any render error we fall back to the
// default template, then to the raw default string — an agent never starts with a
// half-rendered prompt. Callers append the code-owned tail (trafficTool / 中间产物
// 输出规约) AFTER this, so those can't be edited away via the DB body.
// SkillIndexBlock（v0.4.60，宿主注入）：把该 agent 已绑定的 skill 索引块追加进
// system prompt。为什么必须进 prompt 而不只是工具描述：Skill 是元工具，模型不
// 被显式提醒就几乎不会主动调（实测 56k 次调用里 Skill 触发 8 次，任务 18 零次
// ——与当年宿主工具"装了没用起来"同一病根，解法也同款：清单进 system prompt）。
// 返回空串 = 该 agent 没绑技能，不加块。
var SkillIndexBlock func(agentKey string) string

// PlannerTemplateDefault / WorkerTemplateDefault 由 host 注入：模板库中
// is_default 的正文。任务级 override 为空时，它作为第四级回退的“模板默认”。
var (
	PlannerTemplateDefault func() string
	WorkerTemplateDefault  func() string
)

// renderSystemWithOverride 在 renderSystem 之外增加最高优先级的任务级快照：
// 任务快照 > 全局 Agent 提示词覆盖 > 模板库默认 > 内置默认。
func renderSystemWithOverride(agentKey, def string, vars any, taskOverride string) string {
	if strings.TrimSpace(taskOverride) != "" {
		if out, err := renderTmpl(taskOverride, vars); err == nil {
			body := out
			if SkillIndexBlock != nil {
				if blk := SkillIndexBlock(agentKey); blk != "" {
					body += "\n\n" + blk
				}
			}
			return body
		}
	}
	return renderSystem(agentKey, def, vars)
}

func renderSystem(agentKey, def string, vars any) string {
	tmpl := def
	if PromptOverride != nil {
		if t, ok := PromptOverride(agentKey); ok && t != "" {
			tmpl = t
		}
	}
	body := def
	if out, err := renderTmpl(tmpl, vars); err == nil {
		body = out
	} else if out, err := renderTmpl(def, vars); err == nil {
		body = out
	}
	if SkillIndexBlock != nil {
		if blk := SkillIndexBlock(agentKey); blk != "" {
			body += "\n\n" + blk
		}
	}
	return body
}

func renderTmpl(tmpl string, vars any) (string, error) {
	t, err := template.New("p").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	if err := t.Execute(&b, vars); err != nil {
		return "", err
	}
	return b.String(), nil
}
