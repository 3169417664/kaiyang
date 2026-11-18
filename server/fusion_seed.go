package server

import (
	"log"
	"strings"

	"github.com/3169417664/kaiyang/agent"
	"github.com/3169417664/kaiyang/db"
)

const (
	vulnDispatcherFindingMessage = "Planner 已确认以下 finding 需要评估后续处置：[vulnclass=__VULNCLASS__/severity=__SEVERITY__] __SUMMARY__。先读显式派发上下文的 execution_mode：" +
		"foothold 或缺失时，仅 vulnclass ∈ {getshell,webshell,reverse_shell} 且 severity ∈ {critical,high}，并有 WebShell 可连接、交互命令回显或 C2/反弹会话上线证据才评估派发；" +
		"vulnerability_only 时，vulnclass ∈ {rce,file_upload,deserialization,privesc,获取目标权限} 且 severity ∈ {critical,high}、已有可复现确认证据，可评估派发内网渗透子任务，不要求立足点。" +
		"其他漏洞档不派。派发前按提示词查同资产内网任务去重。"
	traceCleanerFindingMessage = "Planner 已确认以下 finding 满足痕迹清理前提：[vulnclass=__VULNCLASS__/severity=__SEVERITY__] __SUMMARY__。" +
		"先读 execution_mode：只有已取得可继续操作的立足点、且当前任务存在 [投放] 事实时才进入清理审计；" +
		"用 get_task_graph / list_task_findings 核对投放事实，逐项列出清理计划，等待真实用户明确回复“同意清理”后再执行。"
)

// seedFusion 首启幂等播种 KAIYANG 场景模板：
//   - 6 个场景分类：外网渗透 / 内网渗透 / 权限提升 / 漏洞利用 / 漏洞挖掘 / 代码审计；
//   - planner/worker 模板库：默认正文取 Pojun 内置默认，内置行可编辑不可删；
//   - 内网渗透/权限提升/漏洞利用默认开启意图审核。
func (s *Server) seedFusion() {
	if s.m == nil || s.m.pg == nil {
		return
	}
	names := []string{"外网渗透", "内网渗透", "权限提升", "漏洞利用", "漏洞挖掘", "代码审计"}
	reviewDefault := map[string]bool{
		"内网渗透": true, "权限提升": true, "漏洞利用": true,
	}
	plannerBody := agent.PlannerDefaultTemplateBody()
	workerBody := agent.WorkerDefaultTemplateBody()
	for _, name := range names {
		nkey := name
		if err := s.m.pg.SeedPromptTemplate("planner", name, nkey, plannerBody, name == "外网渗透"); err != nil {
			log.Printf("[seed] planner template %s: %v", name, err)
		}
		if err := s.m.pg.SeedPromptTemplate("worker", name, nkey, workerBody, name == "外网渗透"); err != nil {
			log.Printf("[seed] worker template %s: %v", name, err)
		}
		if err := s.m.pg.SeedTaskCategoryPreset(name, reviewDefault[name]); err != nil {
			log.Printf("[seed] task category %s: %v", name, err)
		}
	}
	// 把模板 id 关联到同名分类（只在分类尚为 0 时写，不覆盖用户改配）。
	for _, name := range names {
		pt, err1 := s.m.pg.GetPromptTemplateByNKey("planner", name)
		wt, err2 := s.m.pg.GetPromptTemplateByNKey("worker", name)
		if err1 != nil || err2 != nil || pt == nil || wt == nil {
			continue
		}
		_, _ = s.m.pg.Exec(`UPDATE task_categories
			SET default_planner_template_id=$1, default_worker_template_id=$2
			WHERE nkey=$3 AND COALESCE(default_planner_template_id,0)=0`, pt.ID, wt.ID, name)
	}
}

// seedFusionAgents 必须在 orchestration tools seed 之后调用，否则工具绑定找不到行。
func (s *Server) seedFusionAgents() {
	s.seedOptionalAgent("trace_cleaner", "痕迹清理",
		"由 planner 对符合清理条件的 finding 显式派发：核对 [投放] 事实，列出清理计划并在人工确认后逐项执行。",
		agent.TraceCleanerDefaultPrompt,
		[]string{"get_task_graph", "get_task_node_detail", "list_task_findings", "record_fact", "save_evidence"})
	s.seedOptionalAgent("vuln_dispatcher", "漏洞派发",
		"由 planner 确认 finding 满足当前 execution mode 的处置前提后显式调用：执行约束检查、资产去重，并按漏洞类型派发或追加内网渗透子任务。",
		agent.VulnDispatcherDefaultPrompt,
		[]string{"spawn_task", "get_task_graph", "list_task_findings", "get_task_node_detail", "list_llm_profiles",
			"check_internal_asset_task", "append_finding_to_task"})
	// 旧版 seed 的短提示词不包含 execution_mode 分支：只替换逐字等于旧默认的
	// 记录，用户编辑过的不动。
	s.upgradeAgentPromptIfEqual("vuln_dispatcher", oldVulnDispatcherPrompt, agent.VulnDispatcherDefaultPrompt)
	// 显式派发触发器：OnFinding=false，不自动消费全量 finding；触发器消息只在
	// planner 通过 dispatch_finding_to_agent 唤起时作为上下文模板使用。serial+by_task
	// 合并同一任务的瞬时突发，减少重复派发。
	s.seedOptionalAgentTrigger("vuln_dispatcher", vulnDispatcherFindingMessage)
	s.seedOptionalAgentTrigger("trace_cleaner", traceCleanerFindingMessage)
}

// oldVulnDispatcherPrompt 是 Phase 4 初版 seed 的短提示词（用于逐字比对升级）。
const oldVulnDispatcherPrompt = "你是授权渗透测试系统里的漏洞派发 Agent。先读任务图、Finding、操作约束与执行模式；" +
	"只有证据满足当前 execution mode 的处置前提时才 spawn_task 或追加子任务；" +
	"同一资产必须先去重，不扩大任务授权范围。"

// upgradeAgentPromptIfEqual 仅当当前提示词与旧默认逐字相等时替换为新默认。
func (s *Server) upgradeAgentPromptIfEqual(agentKey, oldPrompt, newPrompt string) {
	a, err := s.m.pg.GetAgentByKey(agentKey)
	if err != nil || a == nil {
		return
	}
	cur, _ := s.m.pg.CurrentPrompt(a.ID)
	if strings.TrimSpace(cur) != strings.TrimSpace(oldPrompt) || strings.TrimSpace(cur) == strings.TrimSpace(newPrompt) {
		return
	}
	if _, err := s.m.pg.SavePrompt(a.ID, newPrompt, "内置默认升级（execution_mode 分支）", "system"); err != nil {
		log.Printf("[seed] upgrade %s prompt: %v", agentKey, err)
	}
}

// seedOptionalAgentTrigger 幂等补一条显式派发触发配置；已有触发器则保留用户修改。
func (s *Server) seedOptionalAgentTrigger(agentKey, findingMessage string) {
	triggers, err := s.m.pg.ListTriggersFor(agentKey)
	if err != nil {
		log.Printf("[seed] %s triggers: %v", agentKey, err)
		return
	}
	if len(triggers) > 0 {
		return
	}
	if _, err := s.m.pg.CreateTrigger(&db.AgentTrigger{
		AgentKey: agentKey, Enabled: true, OnFinding: false, FindingMessage: findingMessage,
	}); err != nil {
		log.Printf("[seed] %s trigger: %v", agentKey, err)
		return
	}
	if err := s.m.pg.SetAgentTriggerBehavior(agentKey, "serial", "by_task", 0); err != nil {
		log.Printf("[seed] %s trigger behavior: %v", agentKey, err)
	}
}

// seedOptionalAgent 幂等播种预置自定义 Agent：已存在则保留用户修改与提示词，
// 但仍补齐工具绑定（升级路径：上一版 seed 的 Agent 缺新绑定的工具）。
func (s *Server) seedOptionalAgent(key, name, desc, prompt string, tools []string) {
	if existing, _ := s.m.pg.GetAgentByKey(key); existing != nil {
		if err := s.m.pg.AddAgentToToolBinding(key, tools); err != nil {
			log.Printf("[seed] agent %s tools (existing): %v", key, err)
		}
		return
	}
	a, err := s.m.pg.CreateAgent(key, name, desc)
	if err != nil {
		log.Printf("[seed] agent %s: %v", key, err)
		return
	}
	if err := s.m.pg.SeedPromptIfEmpty(a.ID, prompt); err != nil {
		log.Printf("[seed] agent %s prompt: %v", key, err)
	}
	if err := s.m.pg.AddAgentToToolBinding(key, tools); err != nil {
		log.Printf("[seed] agent %s tools: %v", key, err)
	}
}
