package server

import (
	"log"

	"github.com/3169417664/kaiyang/agent"
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
		"由 planner 显式派发：核对 finding 与执行模式，按需派生/追加内网渗透子任务。",
		"你是授权渗透测试系统里的漏洞派发 Agent。先读任务图、Finding、操作约束与执行模式；"+
			"只有证据满足当前 execution mode 的处置前提时才 spawn_task 或追加子任务；"+
			"同一资产必须先去重，不扩大任务授权范围。",
		[]string{"spawn_task", "get_task_graph", "list_task_findings", "get_task_node_detail", "list_llm_profiles",
			"check_internal_asset_task", "append_finding_to_task"})
}

// seedOptionalAgent 幂等播种预置自定义 Agent：已存在则保留用户修改，不覆盖。
func (s *Server) seedOptionalAgent(key, name, desc, prompt string, tools []string) {
	if existing, _ := s.m.pg.GetAgentByKey(key); existing != nil {
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
