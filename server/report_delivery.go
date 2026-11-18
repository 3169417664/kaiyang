package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/3169417664/kaiyang/db"
	"github.com/3169417664/kaiyang/report"
)

// generateReport 异步生成中文 docx。数据源：findings 表 + enrichment + reproduction +
// task_artifacts 截图 + finding_traffic_bindings 流量证据。
func (s *Server) generateReport(reportID int64, taskID string, findingIDs []int64) {
	_ = s.m.pg.UpdateReportRecord(reportID, "running", "", "", "", 0)
	fail := func(err error) {
		_ = s.m.pg.UpdateReportRecord(reportID, "failed", "", "", err.Error(), 0)
	}
	t, ok := s.m.Task(taskID)
	if !ok {
		fail(fmt.Errorf("任务不存在"))
		return
	}
	findings, err := s.m.pg.ListFindingsForExport(db.FindingFilter{TaskID: t.ID, ExcludeStatuses: exportExcludeStatuses}, findingIDs)
	if err != nil {
		fail(err)
		return
	}
	settings, _ := s.m.pg.GetReportSettings()
	doc := report.NewDocx()
	now := time.Now()
	// 封面
	doc.CoverLine("开阳 KAIYANG · 渗透测试报告", 36, true)
	doc.CoverLine("", 12, false)
	doc.CoverLine("委托方："+valueOrEmpty(settings.CompanyName), 18, false)
	doc.CoverLine("项目："+firstLine(t.Goal, 60), 14, false)
	doc.CoverLine("报告日期："+now.Format("2006-01-02"), 12, false)
	doc.PageBreak()
	doc.TOC()
	doc.PageBreak()

	doc.H1("一、概述")
	doc.P("任务目标：" + t.Goal)
	doc.P("测试时间：" + time.Unix(t.CreatedAt, 0).Format("2006-01-02 15:04") + " 至 " + now.Format("2006-01-02 15:04"))
	doc.P(fmt.Sprintf("本报告共收录 %d 条已确认漏洞。", len(findings)))
	doc.Table([]string{"严重度", "数量"}, severityRows(findings))

	doc.H1("二、漏洞明细")
	for i, f := range findings {
		enr, _ := s.m.pg.GetFindingEnrichment(f.ID)
		title := f.Name
		if title == "" && enr != nil {
			title = enr.Name
		}
		if title == "" {
			title = f.VulnClass
		}
		doc.H2(fmt.Sprintf("2.%d %s", i+1, title))
		cvss := ""
		target := ""
		if enr != nil {
			cvss = fmt.Sprintf("%.1f %s", enr.CVSSScore, enr.CVSSVector)
			target = enr.Target
		}
		if target == "" && len(f.AssetIDs) > 0 {
			target = s.reportAssetNames([]*db.DBFinding{f})[f.AssetIDs[0]]
		}
		doc.Table([]string{"字段", "内容"}, [][]string{
			{"漏洞类别", f.VulnClass},
			{"严重程度", f.Severity},
			{"影响目标", target},
			{"CVSS", cvss},
			{"状态", f.Status},
			{"发现者", f.Worker},
		})
		desc := f.Summary
		if enr != nil && len(enr.Description) > 2 {
			var arr []string
			if json.Unmarshal(enr.Description, &arr) == nil && len(arr) > 0 {
				desc = strings.Join(arr, "\n")
			}
		}
		doc.H3("漏洞描述")
		doc.P(desc)
		doc.H3("复现过程")
		s.writeReproduction(doc, f)
		doc.H3("证据")
		s.writeReportEvidence(doc, t.ID, f)
		if enr != nil {
			var rem []string
			if json.Unmarshal(enr.Remediation, &rem) == nil && len(rem) > 0 {
				doc.H3("修复建议")
				for _, r := range rem {
					doc.Bullet(r)
				}
			}
		}
		if f.Evidence != "" {
			doc.H3("原始证据")
			doc.Code(clipText(f.Evidence, 4000))
		}
	}

	// 保存到 keyDir/report_assets/reports，不进入可浏览 dataDir。
	dir := filepath.Join(s.keyDir, "report_assets", "reports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fail(err)
		return
	}
	name := fmt.Sprintf("task-%s-%d-%s.docx", safeFileToken(t.ID), reportID, now.Format("20060102-150405"))
	out := filepath.Join(dir, name)
	fh, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		fail(err)
		return
	}
	if err := doc.Encode(fh); err != nil {
		_ = fh.Close()
		fail(err)
		return
	}
	_ = fh.Close()
	st, _ := os.Stat(out)
	size := int64(0)
	if st != nil {
		size = st.Size()
	}
	_ = s.m.pg.UpdateReportRecord(reportID, "done", name, out, "", size)
}

func (s *Server) writeReproduction(doc *report.Docx, f *db.DBFinding) {
	raw, _ := s.m.pg.GetFindingReproduction(f.ID)
	var rp struct {
		EntryPoint    string   `json:"entry_point"`
		Prerequisites []string `json:"prerequisites"`
		Steps         []struct {
			Request  string `json:"request"`
			Response string `json:"response"`
			Expected string `json:"expected"`
			Proves   string `json:"proves"`
		} `json:"steps"`
	}
	if len(raw) > 0 && json.Unmarshal(raw, &rp) == nil && len(rp.Steps) > 0 {
		if rp.EntryPoint != "" {
			doc.P("入口点：" + rp.EntryPoint)
		}
		for _, p := range rp.Prerequisites {
			doc.Bullet("前置条件：" + p)
		}
		for i, st := range rp.Steps {
			doc.P(fmt.Sprintf("步骤 %d：%s", i+1, st.Proves))
			if st.Request != "" {
				doc.Code(st.Request)
			}
			if st.Response != "" {
				doc.Code(clipText(st.Response, 2000))
			}
		}
		return
	}
	// 回退：evidence 原文。
	if f.Evidence != "" {
		doc.P("（未记录结构化复现步骤，以下为原始证据）")
		doc.Code(clipText(f.Evidence, 3000))
	}
}

func (s *Server) writeReportEvidence(doc *report.Docx, taskID string, f *db.DBFinding) {
	arts, _ := s.m.pg.ListTaskArtifacts(taskID)
	imgN := 0
	for _, a := range arts {
		if a.Kind != "image" {
			continue
		}
		if a.FindingID != 0 && a.FindingID != f.ID {
			continue
		}
		if data, ok := s.artifactImageData(&a); ok {
			if doc.Image(data, a.Note) {
				imgN++
			}
		}
	}
	if imgN == 0 {
		doc.P("（无截图附件）")
	}
	// 流量证据：列出已绑定快照的请求摘要。
	if ft, err := s.m.pg.GetFindingTraffic(context.Background(), f.ID); err == nil && ft != nil && len(ft.Bindings) > 0 {
		doc.P(fmt.Sprintf("关联流量证据 %d 条：", len(ft.Bindings)))
		for _, b := range ft.Bindings {
			doc.Bullet(fmt.Sprintf("%s · %s", b.Role, b.Note))
		}
	}
}

func severityRows(fs []*db.DBFinding) [][]string {
	m := map[string]int{}
	for _, f := range fs {
		m[f.Severity]++
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return severityRank(keys[i]) < severityRank(keys[j]) })
	out := make([][]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, []string{k, strconv.Itoa(m[k])})
	}
	return out
}

func severityRank(s string) int {
	switch strings.ToLower(s) {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	}
	return 4
}

func valueOrEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func clipText(s string, max int) string {
	if len([]rune(s)) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "\n…（已截断）"
}

func safeFileToken(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "report"
	}
	return b.String()
}
