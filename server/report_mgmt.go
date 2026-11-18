package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/3169417664/kaiyang/db"
)

// 报告管理 / 富化 API（中文 docx）。

func (s *Server) getReportSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.m.pg.GetReportSettings()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) putReportSettings(w http.ResponseWriter, r *http.Request) {
	var st db.ReportSettings
	if !decodeJSON(w, r, &st, maxMediumJSONBodyBytes) {
		return
	}
	st.LogoPath = normalizeReportAssetPath(st.LogoPath)
	if err := s.m.pg.SaveReportSettings(&st); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) listReports(w http.ResponseWriter, r *http.Request) {
	rows, err := s.m.pg.ListReportRecords(100)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"reports": rows})
}

func (s *Server) generateReportHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskID          string   `json:"task_id"`
		FindingIDs      []int64  `json:"finding_ids"`
		Company         string   `json:"company"`
		TemplateID      int64    `json:"template_id"`
		IncludeStatuses []string `json:"include_statuses"`
	}
	if !decodeJSON(w, r, &req, maxSmallJSONBodyBytes) {
		return
	}
	t, ok := s.m.Task(strings.TrimSpace(req.TaskID))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	statuses := req.IncludeStatuses
	for i := range statuses {
		statuses[i] = strings.ToLower(strings.TrimSpace(statuses[i]))
	}
	profile, err := s.resolveReportProfile(req.TemplateID)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	company := strings.TrimSpace(req.Company)
	if company == "" && profile != nil {
		company = profile.CompanyName
	}
	tid, _ := strconv.ParseInt(t.ID, 10, 64)
	rec, err := s.m.pg.CreateReportRecord(&tid, firstLine(t.Description, 80), company)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	go s.generateReport(rec.ID, t.ID, req.FindingIDs, req.TemplateID, parseReportStatuses(strings.Join(statuses, ",")), company)
	writeJSON(w, 202, map[string]any{"report_id": rec.ID, "status": rec.Status})
}

func (s *Server) getReportStatus(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	rec, err := s.m.pg.GetReportRecord(id)
	if err != nil {
		writeErr(w, 404, "报告不存在")
		return
	}
	writeJSON(w, 200, rec)
}

func (s *Server) downloadReport(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	rec, err := s.m.pg.GetReportRecord(id)
	if err != nil || rec.Status != "done" {
		writeErr(w, 404, "报告未生成或不存在")
		return
	}
	base := filepath.Join(s.keyDir, "report_assets", "reports")
	abs := filepath.Clean(rec.Path)
	if !strings.HasPrefix(abs, base+string(os.PathSeparator)) {
		writeErr(w, 403, "非法报告路径")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+rec.Filename+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, abs)
}

func (s *Server) deleteReport(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	path, err := s.m.pg.DeleteReportRecord(id)
	if err != nil {
		writeErr(w, 404, "报告不存在")
		return
	}
	if path != "" {
		base := filepath.Join(s.keyDir, "report_assets", "reports")
		if abs := filepath.Clean(path); strings.HasPrefix(abs, base+string(os.PathSeparator)) {
			_ = os.Remove(abs)
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---- 富化 / 复现 ----

func (s *Server) getFindingEnrichment(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	e, err := s.m.pg.GetFindingEnrichment(id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	repro, _ := s.m.pg.GetFindingReproduction(id)
	writeJSON(w, 200, map[string]any{"enrichment": e, "reproduction": json.RawMessage(repro)})
}

func (s *Server) putFindingEnrichment(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var e db.FindingEnrichment
	if !decodeJSON(w, r, &e, maxMediumJSONBodyBytes) {
		return
	}
	e.FindingID = id
	if err := s.m.pg.UpsertFindingEnrichment(&e); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) putFindingReproduction(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var raw json.RawMessage
	if !decodeJSON(w, r, &raw, maxMediumJSONBodyBytes) {
		return
	}
	if err := s.m.pg.SetFindingReproduction(id, raw); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
