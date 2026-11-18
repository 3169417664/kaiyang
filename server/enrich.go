package server

// 漏洞富化（Phase 5 AI）：用当前激活的 LLM 生成结构化 enrichment，写入
// finding_enrichment。POST /api/findings/{id}/enrich 为手动触发；
// 漏洞状态改为 confirmed 时后台静默触发一次（已有 enrichment 则跳过）。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/3169417664/kaiyang/db"
	"github.com/Autumn-27/norma/llm"
)

const enrichSystemPrompt = `你是渗透测试报告编辑。根据给定的漏洞记录，输出一个 JSON 对象（不要 Markdown 代码围栏、不要解释），字段如下：
{
  "name": "漏洞名称（简洁）",
  "target": "影响目标（域名/URL/IP）",
  "module": "漏洞类别",
  "severity": "critical|high|medium|low",
  "cvss_score": 0.0,
  "cvss_vector": "CVSS:3.1/...",
  "cvss_metrics": {"AV":"N","AC":"L","PR":"N","UI":"N","S":"U","C":"H","I":"H","A":"H"},
  "description": ["漏洞描述段落"],
  "test_process": ["复现/测试步骤（可逐条）"],
  "risk_analysis": ["风险与影响"],
  "remediation": ["修复建议"]
}
要求：只依据记录中已有证据，不臆造未出现的结果；描述使用中文；数组元素为字符串。`

// enrichFinding 调 LLM 生成富化并落库。调用方负责超时/错误处理。
func (s *Server) enrichFinding(ctx context.Context, findingID int64) (*db.FindingEnrichment, error) {
	f, err := s.m.pg.GetFinding(findingID)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, fmt.Errorf("漏洞不存在")
	}
	cfg, ok := s.loadLLMConfig()
	if !ok {
		return nil, fmt.Errorf("LLM 未配置或不可用")
	}
	prov, err := cfg.NewProvider()
	if err != nil {
		return nil, fmt.Errorf("构建 LLM provider 失败: %w", err)
	}
	repro, _ := s.m.pg.GetFindingReproduction(f.ID)
	user := strings.Join([]string{
		"漏洞类别: " + f.VulnClass,
		"漏洞名称: " + f.Name,
		"严重程度: " + f.Severity,
		"状态: " + f.Status,
		"摘要: " + clipText(f.Summary, 2000),
		"证据/PoC:\n" + clipText(f.Evidence, 4000),
		"详细报告:\n" + clipText(f.Report, 3000),
		"结构化复现:\n" + clipText(string(repro), 3000),
	}, "\n\n")
	req := llm.CompletionRequest{
		System:    []string{enrichSystemPrompt},
		Messages:  []llm.Message{llm.UserText(user)},
		MaxTokens: 1500,
	}
	msg, _, _, err := prov.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM 调用失败: %w", err)
	}
	out, err := parseEnrichmentJSON(msg.Text())
	if err != nil {
		return nil, err
	}
	out.FindingID = f.ID
	out.Source = "ai"
	if out.Severity == "" {
		out.Severity = f.Severity
	}
	if !db.ValidSeverity(out.Severity) {
		out.Severity = f.Severity
	}
	if out.Module == "" {
		out.Module = f.VulnClass
	}
	if out.Name == "" {
		out.Name = f.Name
	}
	if err := s.m.pg.UpsertFindingEnrichment(out); err != nil {
		return nil, err
	}
	return s.m.pg.GetFindingEnrichment(f.ID)
}

// parseEnrichmentJSON 宽容解析模型输出：去代码围栏、截取第一个完整 JSON 对象。
func parseEnrichmentJSON(text string) (*db.FindingEnrichment, error) {
	s := strings.TrimSpace(text)
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		if j := strings.Index(s, "\n"); j >= 0 {
			s = s[j+1:]
		}
		if k := strings.Index(s, "```"); k >= 0 {
			s = s[:k]
		}
	}
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("LLM 未返回 JSON 对象")
	}
	s = s[start : end+1]
	var raw struct {
		Name         string          `json:"name"`
		Target       string          `json:"target"`
		Module       string          `json:"module"`
		Severity     string          `json:"severity"`
		CVSSScore    float64         `json:"cvss_score"`
		CVSSVector   string          `json:"cvss_vector"`
		CVSSMetrics  json.RawMessage `json:"cvss_metrics"`
		Description  json.RawMessage `json:"description"`
		TestProcess  json.RawMessage `json:"test_process"`
		RiskAnalysis json.RawMessage `json:"risk_analysis"`
		Remediation  json.RawMessage `json:"remediation"`
	}
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, fmt.Errorf("解析 LLM JSON 失败: %w", err)
	}
	return &db.FindingEnrichment{
		Name: raw.Name, Target: raw.Target, Module: raw.Module, Severity: raw.Severity,
		CVSSScore: raw.CVSSScore, CVSSVector: raw.CVSSVector, CVSSMetrics: raw.CVSSMetrics,
		Description: raw.Description, TestProcess: raw.TestProcess,
		RiskAnalysis: raw.RiskAnalysis, Remediation: raw.Remediation,
	}, nil
}

// enrichFindingHandler POST /api/findings/{id}/enrich
func (s *Server) enrichFindingHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r)
	if err != nil || id <= 0 {
		writeErr(w, 400, "bad finding id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	out, err := s.enrichFinding(ctx, id)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "enrichment": out})
}

// triggerAutoEnrich 状态改 confirmed 后的后台富化：已有富化则跳过，失败静默。
func (s *Server) triggerAutoEnrich(findingID int64) {
	if findingID <= 0 {
		return
	}
	go func() {
		if existing, _ := s.m.pg.GetFindingEnrichment(findingID); existing != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		_, _ = s.enrichFinding(ctx, findingID)
	}()
}
