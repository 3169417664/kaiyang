package agent

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/3169417664/kaiyang/hosttools"
)

// PromptSegment is one ordered piece of a system prompt, exposed for the
// read-only assembly view. Text is the exact chunk joined into the final prompt
// (including its leading separator); JoinPromptSegments concatenates chunks
// verbatim, so the runtime builders and the preview stay byte-identical.
//
// The runtime side builds its system prompt through RuntimeSystemSegments, and
// the server's GET /api/agents/{key}/prompt/chain projects the same function's
// output. There is exactly one assembly implementation; the preview cannot
// silently drift from what a run actually sends.
type PromptSegment struct {
	Key       string `json:"key"`
	Title     string `json:"title"`
	Source    string `json:"source"`
	Editable  bool   `json:"editable"`
	Gate      string `json:"gate,omitempty"`
	Present   bool   `json:"present"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Text      string `json:"text"`
	// OriginalChars is the content length when Text was dropped from the request
	// (replace mode). Zero when Text is the content.
	OriginalChars int `json:"-"`
}

// JoinPromptSegments concatenates segment texts without inserting separators:
// code-owned tails already carry their own "\n" / "\n\n" prefix, matching the
// historical string concatenation byte for byte.
func JoinPromptSegments(parts []PromptSegment) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Text)
	}
	return b.String()
}

// TailOptions supplies the few runtime values the static code-owned tails
// interpolate. The preview passes visible placeholders, so it never claims a
// path or proxy that the production run does not use.
type TailOptions struct {
	WorkDir string
	RunDir  string
	// ProxyAddr may be a masked/placeholder value in read-only previews; the
	// runtime passes the real egress address.
	ProxyAddr string
	CAEnabled bool
	// IncludeHostTools adds the host tool inventory when the runtime does.
	IncludeHostTools bool
	// Goal drives the vulnerability-only execution-mode code tail (planner).
	Goal string
	// GoalExecMode carries the goals-decomposer execution-mode tail computed from
	// the task's goal/description (runtime-only input; empty in the preview).
	GoalExecMode string
	// GoalScope appends the scope-extraction tail (goals) only when the runtime
	// has an asset store + task to write to.
	GoalScope bool
}

// DefaultPromptFor returns the built-in body a runtime builder falls back to
// when an agent has no saved DB prompt. Custom chat agents fall back to the
// assistant starter prompt.
func DefaultPromptFor(agentKey string) string {
	if t, ok := BuiltinPromptSeeds()[agentKey]; ok {
		return t
	}
	switch agentKey {
	case "reporter":
		return ReporterDefaultPrompt
	case "retester":
		return RetesterDefaultPrompt
	}
	return DefaultAssistantPrompt
}

// RuntimeBodyFor returns the default body, its source label, and the assembly
// key (planner | worker | mainagent | goals | <chat key>) the runtime would use
// for an agent. executor=true means a custom agent that claims intents through
// executor_key: it runs the worker assembly while its DB prompt stays keyed by
// the custom agent key.
func RuntimeBodyFor(agentKey string, executor bool) (def, defSource, assemblyKey string) {
	if executor {
		def, defSource = DefaultBodyFor("worker")
		return def, defSource, "worker"
	}
	switch agentKey {
	case "planner", "worker", "mainagent", "goals":
		def, defSource = DefaultBodyFor(agentKey)
		return def, defSource, agentKey
	}
	return DefaultAssistantPrompt, "builtin_default", agentKey
}

// DefaultBodyFor returns the body a runtime builder would use absent a DB
// override, plus the source label the projection should show for it. Planner and
// worker prefer the template-library default (四级链第③级) when it is set.
func DefaultBodyFor(agentKey string) (string, string) {
	switch agentKey {
	case "planner":
		if PlannerTemplateDefault != nil {
			if b := PlannerTemplateDefault(); strings.TrimSpace(b) != "" {
				return b, "template_default"
			}
		}
	case "worker":
		if WorkerTemplateDefault != nil {
			if b := WorkerTemplateDefault(); strings.TrimSpace(b) != "" {
				return b, "template_default"
			}
		}
	}
	return DefaultPromptFor(agentKey), "builtin_default"
}

// PreviewVarsFor returns the sample template variables for an agent. It is
// exported so tests can rebuild a runtime assembly with exactly the values the
// preview shows.
func PreviewVarsFor(agentKey string) any {
	switch agentKey {
	case "goals":
		return GoalsVars{EngagementDescription: "", DataDir: "<数据目录>", Now: nowStr()}
	case "planner":
		return PlannerVars{Goal: "<任务目标>", Scope: "", AssetSummary: "", DataDir: "<数据目录>", Now: nowStr()}
	case "worker":
		return WorkerVars{ProxyAddr: "<出口代理>", WorkerName: "", DataDir: "<数据目录>", Now: nowStr()}
	case "mainagent":
		return MainVars{Goal: "<任务目标>", AssetSummary: "", FindingsSummary: "", DataDir: "<数据目录>", Now: nowStr()}
	default:
		return chatVars{DataDir: "<数据目录>", Now: nowStr()}
	}
}

// RuntimeSystemSegments is the single assembly implementation: the editable
// body (task snapshot / DB override / template default / built-in default, in
// that order), any LLM-profile overlay, the host-injected skill index, and the
// code-owned static tails. Runtime-only injections (constraints, Program
// Profile, task auth, intent/facts/lessons, autopilot, launch instruction,
// finding guidance, deferred tools) are appended by callers and represented in
// the projection as gated, body-less injection points.
func RuntimeSystemSegments(agentKey, def, defSource string, vars any, taskOverride string, pp ProfilePrompt, o TailOptions) ([]PromptSegment, string) {
	segs, warning := renderSystemSegments(agentKey, def, defSource, vars, taskOverride, pp)
	return append(segs, StaticTailSegments(agentKey, o)...), warning
}

// RuntimeSystem is RuntimeSystemSegments' joined form — the system prompt before
// any runtime-only injection.
func RuntimeSystem(agentKey, def, defSource string, vars any, taskOverride string, pp ProfilePrompt, o TailOptions) (string, string) {
	segs, warning := RuntimeSystemSegments(agentKey, def, defSource, vars, taskOverride, pp, o)
	return JoinPromptSegments(segs), warning
}

// renderSystemSegments renders the body portion and reports a non-empty warning
// when the selected template failed and a fallback was used. The runtime text is
// unchanged by the warning.
func renderSystemSegments(agentKey, def, defSource string, vars any, taskOverride string, pp ProfilePrompt) ([]PromptSegment, string) {
	if defSource == "" {
		defSource = "builtin_default"
	}
	source := defSource
	rendered := ""
	warning := ""

	// 第①级：任务级提示词快照（最高优先级）。
	if strings.TrimSpace(taskOverride) != "" {
		if out, err := renderTmpl(taskOverride, vars); err == nil {
			rendered = out
			source = "task_override"
		} else {
			warning = "任务级提示词快照渲染失败，已回退下一级"
		}
	}
	// 第②级起：全局 Agent 提示词覆盖（DB）→ 模板库默认 → 内置默认。
	if rendered == "" {
		tmpl := def
		if PromptOverride != nil {
			if t, ok := PromptOverride(agentKey); ok && t != "" {
				tmpl = t
				source = "db_template"
			}
		}
		out, err := renderTmpl(tmpl, vars)
		if err == nil {
			rendered = out
		} else {
			switch source {
			case "db_template":
				alt, altErr := renderTmpl(def, vars)
				if altErr == nil {
					rendered = alt
					warning = "DB 提示词模板渲染失败，已回退模板/内置默认正文"
				} else {
					rendered = def
					warning = "DB 提示词模板与默认正文均渲染失败，已回退默认原文"
				}
				source = defSource
			default:
				rendered = def
				warning = "默认提示词模板渲染失败，已回退原文"
			}
		}
	}

	skill := ""
	if SkillIndexBlock != nil {
		skill = SkillIndexBlock(agentKey)
	}

	var out []PromptSegment
	appendProfile := func(text string) {
		out = append(out, PromptSegment{
			Key: "profile_prompt", Title: "独立提示词（LLM 配置）", Source: "profile_prompt",
			Present: true, Available: true, Text: text,
		})
	}
	bodySeg := PromptSegment{
		Key: "body", Title: "提示词正文", Source: source, Editable: true,
		Present: true, Available: true, Text: rendered,
	}
	skillSeg := PromptSegment{
		Key: "skill_index", Title: "技能速查（宿主注入）", Source: "host",
		Gate: "该 Agent 绑定了 Skill", Present: skill != "", Available: true,
	}
	if skill != "" {
		skillSeg.Text = "\n\n" + skill
	} else {
		skillSeg.Reason = "该 Agent 未绑定 Skill"
	}

	switch {
	case pp.Active() && pp.Mode == "replace":
		appendProfile(pp.Text)
		bodySeg.Present = false
		bodySeg.Text = ""
		bodySeg.OriginalChars = utf8.RuneCountInString(rendered)
		bodySeg.Reason = fmt.Sprintf("已被独立提示词替换（原正文 %d 字符；技能索引与代码硬尾巴仍保留）", bodySeg.OriginalChars)
		out = append(out, bodySeg, skillSeg)
	case pp.Active() && pp.Mode == "prepend":
		appendProfile(pp.Text + "\n\n")
		out = append(out, bodySeg, skillSeg)
	default:
		out = append(out, bodySeg, skillSeg)
		if pp.Active() && pp.Mode == "append" {
			appendProfile("\n\n" + pp.Text)
		}
	}
	return out, warning
}

// PlannerTailOptions / WorkerTailOptions / MainAgentTailOptions / ChatTailOptions /
// GoalsTailOptions are the constructors both the runtime builders and the
// read-only projection call, so the set of inputs a tail interpolates is
// declared once.
func PlannerTailOptions(goal, workDir string) TailOptions {
	return TailOptions{WorkDir: workDir, Goal: goal}
}

func WorkerTailOptions(proxyAddr, caCert, runDir string) TailOptions {
	return TailOptions{ProxyAddr: proxyAddr, CAEnabled: caCert != "", RunDir: runDir, IncludeHostTools: true}
}

func MainAgentTailOptions(workDir string) TailOptions {
	return TailOptions{WorkDir: workDir, IncludeHostTools: true}
}

func ChatTailOptions(workDir string) TailOptions {
	return TailOptions{WorkDir: workDir}
}

func GoalsTailOptions(execMode string, scope bool) TailOptions {
	return TailOptions{GoalExecMode: execMode, GoalScope: scope}
}

// StaticTailSegments returns the code-owned, non-editable tail segments for an
// agent, in the exact order the runtime builders append them. The runtime
// builders consume this through RuntimeSystem, so preview and run cannot drift.
func StaticTailSegments(agentKey string, o TailOptions) []PromptSegment {
	workDir := o.WorkDir
	if workDir == "" {
		workDir = "<任务工作目录>"
	}
	runDir := o.RunDir
	if runDir == "" {
		runDir = "<本次意图工作目录>"
	}
	code := func(key, title, text string) PromptSegment {
		return PromptSegment{Key: key, Title: title, Source: "code", Present: text != "", Available: true, Text: text}
	}
	conditional := func(key, title, gate, text, reason string) PromptSegment {
		return PromptSegment{Key: key, Title: title, Source: "code", Gate: gate, Present: text != "", Available: text != "", Text: text, Reason: reason}
	}

	switch agentKey {
	case "planner":
		return []PromptSegment{
			conditional("exec_mode", "执行模式（仅漏洞路径）", "任务目标识别为仅漏洞路径", plannerExecutionModeSpec(o.Goal), "任务目标未识别为仅漏洞路径时不注入"),
			code("artifact_spec", "中间产物输出规约", artifactSpec(workDir)),
			code("coverage_matrix", "漏洞覆盖矩阵", coverageMatrixPlannerGuidance),
			code("scope_verify", "授权边界与复核优先", scopeVerifyPlannerGuidance),
			code("exec_face_planner", "执行面编排", execFacePlannerGuidance),
			code("engagement_framing", "交战框定", engagementFraming),
			code("impact_orchestration", "影响编排", impactOrchestrationGuidance),
			code("blackboard", "规划黑板", blackboardPlannerGuidance),
		}

	case "worker":
		return []PromptSegment{
			conditional("traffic_tools", "流量工具说明", "需开启流量捕获", workerTrafficBlock(o.CAEnabled), "流量捕获未开启，运行时不会出现 traffic_* 工具"),
			conditional("egress_proxy", "出口代理说明", "需配置出口代理", workerEgressBlock(o.ProxyAddr), "未配置出口代理"),
			code("artifact_spec", "中间产物输出规约", workerArtifactSpec(runDir)),
			code("scanner_first", "扫描器优先", scannerFirstGuidance),
			code("skill_routing", "技能路由", skillRoutingGuidance),
			code("exec_face_worker", "执行面", execFaceWorkerGuidance),
			code("engagement_framing", "交战框定", engagementFraming),
			code("impact_verification", "影响验证", impactVerificationGuidance),
			code("foothold_fact", "立足点事实口径", footholdFactGuidance),
			hostToolsSegment(o),
		}

	case "mainagent":
		return []PromptSegment{
			code("artifact_spec", "中间产物输出规约", artifactSpec(workDir)),
			code("engagement_framing", "交战框定", engagementFraming),
			code("impact_orchestration", "影响编排", impactOrchestrationGuidance),
			code("src_task_book", "SRC 任务书", srcTaskBookGuidance),
			hostToolsSegment(o),
		}

	case "goals":
		out := []PromptSegment{
			conditional("goals_exec_mode", "执行模式（仅漏洞路径）", "任务目标识别为仅漏洞路径", o.GoalExecMode, "任务未识别为仅漏洞路径时不注入"),
			code("goals_verification", "验收标准", goalsVerificationTail),
		}
		scope := conditional("goals_scope", "范围提取", "存在资产库与任务上下文", "", "无资产库/任务上下文时不注入")
		if o.GoalScope {
			scope.Present, scope.Available, scope.Reason, scope.Text = true, true, "", goalsScopeTail
		}
		return append(out, scope)

	default: // conversational agents: pentest / reporter / retester / auto / custom
		out := []PromptSegment{
			code("chat_workdir", "文件输出规约", chatWorkDirSpec(workDir)),
		}
		switch agentKey {
		case "pentest":
			out = append(out,
				code("engagement_framing", "交战框定", engagementFraming),
				code("impact_verification", "影响验证", impactVerificationGuidance),
				code("impact_orchestration", "影响编排", impactOrchestrationGuidance),
				code("src_report_annex", "SRC 报告附录", srcReportAnnex),
			)
		case "reporter":
			out = append(out,
				code("engagement_framing", "交战框定", engagementFraming),
				code("src_report_annex", "SRC 报告附录", srcReportAnnex),
			)
		case "retester":
			out = append(out, code("engagement_framing", "交战框定", engagementFraming))
		}
		return out
	}
}

func hostToolsSegment(o TailOptions) PromptSegment {
	seg := PromptSegment{
		Key: "host_tools", Title: "本机可用工具", Source: "host",
		Gate: "宿主工具清单非空", Available: true,
	}
	if !o.IncludeHostTools {
		seg.Available = false
		seg.Reason = "该 Agent 不注入宿主工具清单"
		return seg
	}
	if t := hosttools.SummaryText(); t != "" {
		seg.Present = true
		seg.Text = "\n\n【本机可用工具】\n" + t
		return seg
	}
	seg.Reason = "宿主未安装额外工具"
	return seg
}
