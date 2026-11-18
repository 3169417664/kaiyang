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
