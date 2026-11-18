package server

// 报告模板 CRUD（Phase 5）：GET/POST /api/report-templates、
// PUT/DELETE /api/report-templates/{id}。模板字段与 report_templates 表一致；
// 生成报告时可传 template_id 覆盖组织级 report_settings。

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/3169417664/kaiyang/db"
)

type reportTemplateBody struct {
	Name          string          `json:"name"`
	CompanyName   string          `json:"company_name"`
	CompanyNameEn string          `json:"company_name_en"`
	LogoPath      string          `json:"logo_path"`
	Personnel     json.RawMessage `json:"personnel_json"`
	CVSSVersion   string          `json:"cvss_version"`
	Locale        string          `json:"locale"`
}

func (b reportTemplateBody) validate() string {
	if strings.TrimSpace(b.Name) == "" {
		return "模板名称不能为空"
	}
	if len(b.Personnel) > 0 {
		var probe []any
		if err := json.Unmarshal(b.Personnel, &probe); err != nil {
			return "personnel_json 必须是 JSON 数组"
		}
	}
	return ""
}

func (b reportTemplateBody) toTemplate(id int64) *db.ReportTemplate {
	personnel := b.Personnel
	if len(personnel) == 0 {
		personnel = json.RawMessage(`[]`)
	}
	return &db.ReportTemplate{
		ID:            id,
		Name:          strings.TrimSpace(b.Name),
		CompanyName:   strings.TrimSpace(b.CompanyName),
		CompanyNameEn: strings.TrimSpace(b.CompanyNameEn),
		LogoPath:      normalizeReportAssetPath(b.LogoPath),
		Personnel:     personnel,
		CVSSVersion:   strings.TrimSpace(b.CVSSVersion),
		Locale:        strings.TrimSpace(b.Locale),
	}
}

func (s *Server) listReportTemplates(w http.ResponseWriter, r *http.Request) {
	rows, err := s.m.pg.ListReportTemplates()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"templates": rows})
}

func (s *Server) createReportTemplate(w http.ResponseWriter, r *http.Request) {
	var b reportTemplateBody
	if !decodeJSON(w, r, &b, maxMediumJSONBodyBytes) {
		return
	}
	if msg := b.validate(); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	t, err := s.m.pg.CreateReportTemplate(b.toTemplate(0))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, t)
}

func (s *Server) updateReportTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r)
	if err != nil {
		writeErr(w, 400, "bad template id")
		return
	}
	var b reportTemplateBody
	if !decodeJSON(w, r, &b, maxMediumJSONBodyBytes) {
		return
	}
	if msg := b.validate(); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	t := b.toTemplate(id)
	if err := s.m.pg.UpdateReportTemplate(t); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "模板不存在")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	out, err := s.m.pg.GetReportTemplate(id)
	if err != nil || out == nil {
		writeErr(w, 500, "模板更新后读取失败")
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) deleteReportTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r)
	if err != nil {
		writeErr(w, 400, "bad template id")
		return
	}
	if err := s.m.pg.DeleteReportTemplate(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 404, "模板不存在")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// parsePathID 读取 {id} 路径参数。
func parsePathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}
