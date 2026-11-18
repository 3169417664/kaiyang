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
func (s *Server) generateReport(reportID int64, taskID string, findingIDs []int64, templateID int64, statuses []string, companyOverride string) {
	_ = s.m.pg.UpdateReportRecord(reportID, "running", "", "", "", 0)
	fail := func(err error) {
		_ = s.m.pg.UpdateReportRecord(reportID, "failed", "", "", err.Error(), 0)
	}
	t, ok := s.m.Task(taskID)
	if !ok {
		fail(fmt.Errorf("任务不存在"))
		return
	}
	if len(statuses) == 0 {
		statuses = reportIncludeStatuses
	}
	findings, err := s.m.pg.ListFindingsForExport(db.FindingFilter{TaskID: t.ID, IncludeStatuses: statuses}, findingIDs)
	if err != nil {
		fail(err)
		return
	}
	profile, _ := s.resolveReportProfile(templateID)
	company := strings.TrimSpace(companyOverride)
	if company == "" && profile != nil {
		company = profile.CompanyName
	}
	cvssVersion, locale := "3.1", "zh-CN"
	if profile != nil {
		cvssVersion = firstNonEmpty(profile.CVSSVersion, "3.1")
		locale = firstNonEmpty(profile.Locale, "zh-CN")
	}
	doc := report.NewDocx()
	now := time.Now()
	// 封面（Logo / 委托方 / 项目 / 日期）
	if profile != nil {
		if logo := s.profileLogo(profile); len(logo) > 0 {
			doc.CoverImage(logo)
		}
	}
	doc.CoverLine("开阳 KAIYANG · 渗透测试报告", 36, true)
	doc.CoverLine("", 12, false)
	doc.CoverLine("委托方："+valueOrEmpty(company), 18, false)
	doc.CoverLine("项目："+firstLine(t.Goal, 60), 14, false)
	doc.CoverLine("报告日期："+now.Format("2006-01-02"), 12, false)
	doc.PageBreak()
	doc.TOC()
	doc.PageBreak()

	doc.H1("一、概述")
	doc.P("任务目标：" + t.Goal)
	doc.P("测试时间：" + time.Unix(t.CreatedAt, 0).Format("2006-01-02 15:04") + " 至 " + now.Format("2006-01-02 15:04"))
	doc.P(fmt.Sprintf("本报告共收录 %d 条漏洞（收录状态：%s）。", len(findings), strings.Join(statuses, "/")))
	doc.P("CVSS 版本：" + cvssVersion + "；报告语言：" + locale)
	if profile != nil {
		if names := personnelNames(profile.Personnel); len(names) > 0 {
			rows := make([][]string, 0, len(names))
			for _, n := range names {
				rows = append(rows, []string{n.Name, n.Role, n.Contact})
			}
			doc.H2("测试人员")
			doc.Table([]string{"姓名", "角色", "联系方式"}, rows)
		}
	}
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
			if enr.CVSSScore > 0 || enr.CVSSVector != "" {
				cvss = fmt.Sprintf("%.1f %s", enr.CVSSScore, enr.CVSSVector)
			}
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
			{"CVSS 版本", cvssVersion},
			{"状态", f.Status},
			{"发现者", f.Worker},
		})
		// CVSS metrics 表（v3.1 向量分量逐项展开）
		if enr != nil {
			if rows, ok := cvssMetricRows(enr.CVSSMetrics); ok {
				doc.H3("CVSS 评分要素")
				doc.Table([]string{"指标", "取值"}, rows)
			}
		}
		// 描述：优先 enrichment 结构化字段
		desc := f.Summary
		if enr != nil {
			if s := enrichmentStrings(enr.Description); len(s) > 0 {
				desc = strings.Join(s, "\n")
			}
		}
		doc.H3("漏洞描述")
		doc.P(desc)
		// 测试过程 / 风险分析（enrichment 结构化字段）
		if enr != nil {
			if s := enrichmentStrings(enr.TestProcess); len(s) > 0 {
				doc.H3("测试过程")
				for _, line := range s {
					doc.Bullet(line)
				}
			}
			if s := enrichmentStrings(enr.RiskAnalysis); len(s) > 0 {
				doc.H3("风险分析")
				for _, line := range s {
					doc.Bullet(line)
				}
			}
		}
		doc.H3("复现过程")
		s.writeReproduction(doc, f)
		doc.H3("证据")
		s.writeReportEvidence(doc, t.ID, f)
		if enr != nil {
			if rem := enrichmentStrings(enr.Remediation); len(rem) > 0 {
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
		Provenance    string   `json:"provenance"`
		Steps         []struct {
			Request    string `json:"request"`
			Response   string `json:"response"`
			Expected   string `json:"expected"`
			Proves     string `json:"proves"`
			ExchangeID string `json:"exchange_id"`
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
			label := fmt.Sprintf("步骤 %d", i+1)
			if st.Proves != "" {
				label += "：" + st.Proves
			}
			if st.ExchangeID != "" {
				label += "（流量 #" + st.ExchangeID + "）"
			}
			doc.P(label)
			if st.Request != "" {
				doc.P("请求：")
				doc.Code(st.Request)
			}
			if st.Response != "" {
				doc.P("响应：")
				doc.Code(clipText(st.Response, 2000))
			}
			if st.Expected != "" {
				doc.P("预期：" + st.Expected)
			}
		}
		if rp.Provenance != "" {
			doc.P("证据来源：" + rp.Provenance)
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
		// Phase 5：只取归属本条漏洞的截图。任务级（finding_id=0）与其他漏洞的
		// 图片不再重复塞进每一条漏洞，避免张冠李戴。
		if a.Kind != "image" || a.FindingID != f.ID {
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
	s.writeTrafficEvidenceBodies(doc, f)
}

// writeTrafficEvidenceBodies 输出已绑定流量证据的请求/响应正文（从 evidence.Store
// 读取，截断到 8KB/侧）。快照正文不可读（被 GC / 非文本）时降级为元信息说明。
func (s *Server) writeTrafficEvidenceBodies(doc *report.Docx, f *db.DBFinding) {
	ft, err := s.m.pg.GetFindingTraffic(context.Background(), f.ID)
	if err != nil || ft == nil || len(ft.Bindings) == 0 {
		return
	}
	store := s.evidenceStore()
	doc.P(fmt.Sprintf("关联流量证据 %d 条：", len(ft.Bindings)))
	for i, b := range ft.Bindings {
		doc.P(fmt.Sprintf("%d. [%s] %s %s · 状态码 %d",
			i+1, b.Role, nzStr(b.Snapshot.Method, "GET"), b.Snapshot.URL, b.Snapshot.Status))
		if note := strings.TrimSpace(b.Note); note != "" {
			doc.P("说明：" + note)
		}
		for _, side := range []struct{ key, label string }{{"request", "请求正文"}, {"response", "响应正文"}} {
			preview, err := readEvidencePreview(store, b.Snapshot, side.key, 0, 8192)
			if err != nil {
				doc.P(side.label + "：（正文不可读或已被清理）")
				continue
			}
			text := strings.TrimSpace(preview.Content)
			if text == "" {
				text = "（空）"
			}
			if preview.Truncated {
				text += fmt.Sprintf("\n…（已截断，共 %d 字节）", preview.Total)
			}
			doc.P(side.label + "：")
			doc.Code(text)
		}
	}
}

func nzStr(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
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

// personnelEntry 报告模板里的人员条目（name/role/contact）。
type personnelEntry struct {
	Name    string
	Role    string
	Contact string
}

// personnelNames 解析模板/设置里的人员 JSON；兼容对象数组与纯字符串数组。
func personnelNames(raw json.RawMessage) []personnelEntry {
	if len(raw) == 0 {
		return nil
	}
	var list []map[string]any
	if err := json.Unmarshal(raw, &list); err == nil {
		out := make([]personnelEntry, 0, len(list))
		for _, m := range list {
			e := personnelEntry{
				Name:    strOf(m["name"]),
				Role:    strOf(m["role"]),
				Contact: firstNonEmpty(strOf(m["contact"]), strOf(m["email"])),
			}
			if e.Name == "" && e.Role == "" {
				continue
			}
			out = append(out, e)
		}
		return out
	}
	var strs []string
	if err := json.Unmarshal(raw, &strs); err == nil {
		out := make([]personnelEntry, 0, len(strs))
		for _, s := range strs {
			if strings.TrimSpace(s) != "" {
				out = append(out, personnelEntry{Name: s})
			}
		}
		return out
	}
	return nil
}

func strOf(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// enrichmentStrings 把 enrichment 的结构化数组展开成可读行（字符串原样，
// 对象/数组转紧凑 JSON）。
func enrichmentStrings(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var arr []any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		switch t := v.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				out = append(out, t)
			}
		default:
			if b, err := json.Marshal(v); err == nil && string(b) != "{}" && string(b) != "[]" {
				out = append(out, string(b))
			}
		}
	}
	return out
}

// cvssMetricRows 把 cvss_metrics 渲染成「指标/取值」表行。支持对象
// {"AV":"N",...} 或 [{metric|key, value|score}, ...] 两种形态。
func cvssMetricRows(raw json.RawMessage) ([][]string, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil && len(obj) > 0 {
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		rows := make([][]string, 0, len(keys))
		for _, k := range keys {
			rows = append(rows, []string{k, metricValue(obj[k])})
		}
		return rows, true
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
		rows := make([][]string, 0, len(arr))
		for _, m := range arr {
			k := firstNonEmpty(firstNonEmpty(strOf(m["metric"]), strOf(m["key"])), strOf(m["name"]))
			v := firstNonEmpty(firstNonEmpty(strOf(m["value"]), strOf(m["score"])), strOf(m["selected"]))
			if k == "" {
				continue
			}
			rows = append(rows, []string{k, v})
		}
		if len(rows) > 0 {
			return rows, true
		}
	}
	return nil, false
}

func metricValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
