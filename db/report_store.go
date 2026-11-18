package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// FindingEnrichment 结构化漏洞富化层（人工/AI 编辑），以 findings.id 为键。
type FindingEnrichment struct {
	FindingID    int64           `json:"finding_id"`
	NodeID       *int64          `json:"node_id,omitempty"`
	Name         string          `json:"name"`
	Target       string          `json:"target"`
	Module       string          `json:"module"`
	Severity     string          `json:"severity"`
	Status       string          `json:"status"`
	CVSSScore    float64         `json:"cvss_score"`
	CVSSVector   string          `json:"cvss_vector"`
	CVSSMetrics  json.RawMessage `json:"cvss_metrics"`
	Description  json.RawMessage `json:"description"`
	TestProcess  json.RawMessage `json:"test_process"`
	RiskAnalysis json.RawMessage `json:"risk_analysis"`
	Remediation  json.RawMessage `json:"remediation"`
	Source       string          `json:"source"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

func (d *DB) GetFindingReproduction(findingID int64) (json.RawMessage, error) {
	var raw []byte
	err := d.QueryRow(`SELECT COALESCE(reproduction::text,'{}') FROM findings WHERE id=$1`, findingID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

func (d *DB) SetFindingReproduction(findingID int64, raw json.RawMessage) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	_, err := d.Exec(`UPDATE findings SET reproduction=$2::jsonb WHERE id=$1`, findingID, string(raw))
	return err
}

func (d *DB) GetFindingEnrichment(findingID int64) (*FindingEnrichment, error) {
	var e FindingEnrichment
	var node sql.NullInt64
	var cm, desc, tp, risk, rem []byte
	err := d.QueryRow(`SELECT finding_id,node_id,name,target,module,severity,status,cvss_score,cvss_vector,
		cvss_metrics::text,description::text,test_process::text,risk_analysis::text,remediation::text,source,updated_at
		FROM finding_enrichment WHERE finding_id=$1`, findingID).
		Scan(&e.FindingID, &node, &e.Name, &e.Target, &e.Module, &e.Severity, &e.Status, &e.CVSSScore, &e.CVSSVector,
			&cm, &desc, &tp, &risk, &rem, &e.Source, &e.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if node.Valid {
		v := node.Int64
		e.NodeID = &v
	}
	e.CVSSMetrics, e.Description, e.TestProcess = json.RawMessage(cm), json.RawMessage(desc), json.RawMessage(tp)
	e.RiskAnalysis, e.Remediation = json.RawMessage(risk), json.RawMessage(rem)
	return &e, nil
}

func (d *DB) UpsertFindingEnrichment(e *FindingEnrichment) error {
	if e == nil || e.FindingID <= 0 {
		return errors.New("finding_id 必填")
	}
	if len(e.CVSSMetrics) == 0 {
		e.CVSSMetrics = json.RawMessage(`{}`)
	}
	if len(e.Description) == 0 {
		e.Description = json.RawMessage(`[]`)
	}
	if len(e.TestProcess) == 0 {
		e.TestProcess = json.RawMessage(`[]`)
	}
	if len(e.RiskAnalysis) == 0 {
		e.RiskAnalysis = json.RawMessage(`[]`)
	}
	if len(e.Remediation) == 0 {
		e.Remediation = json.RawMessage(`[]`)
	}
	if e.Source == "" {
		e.Source = "manual"
	}
	_, err := d.Exec(`INSERT INTO finding_enrichment(finding_id,node_id,name,target,module,severity,status,
		cvss_score,cvss_vector,cvss_metrics,description,test_process,risk_analysis,remediation,source)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11::jsonb,$12::jsonb,$13::jsonb,$14::jsonb,$15)
		ON CONFLICT (finding_id) DO UPDATE SET
		  node_id=EXCLUDED.node_id, name=EXCLUDED.name, target=EXCLUDED.target, module=EXCLUDED.module,
		  severity=EXCLUDED.severity, status=EXCLUDED.status, cvss_score=EXCLUDED.cvss_score,
		  cvss_vector=EXCLUDED.cvss_vector, cvss_metrics=EXCLUDED.cvss_metrics,
		  description=EXCLUDED.description, test_process=EXCLUDED.test_process,
		  risk_analysis=EXCLUDED.risk_analysis, remediation=EXCLUDED.remediation,
		  source=EXCLUDED.source, updated_at=now()`,
		e.FindingID, e.NodeID, e.Name, e.Target, e.Module, e.Severity, e.Status,
		e.CVSSScore, e.CVSSVector, string(e.CVSSMetrics), string(e.Description),
		string(e.TestProcess), string(e.RiskAnalysis), string(e.Remediation), e.Source)
	return err
}

// ReportSettings 组织级报告设置（单例）。
type ReportSettings struct {
	CompanyName   string          `json:"company_name"`
	CompanyNameEn string          `json:"company_name_en"`
	LogoPath      string          `json:"logo_path"`
	Personnel     json.RawMessage `json:"personnel"`
	CVSSVersion   string          `json:"cvss_version"`
}

func (d *DB) GetReportSettings() (*ReportSettings, error) {
	var s ReportSettings
	var p []byte
	err := d.QueryRow(`SELECT company_name,company_name_en,logo_path,personnel_json::text,cvss_version
		FROM report_settings WHERE id=1`).Scan(&s.CompanyName, &s.CompanyNameEn, &s.LogoPath, &p, &s.CVSSVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return &ReportSettings{Personnel: json.RawMessage(`[]`), CVSSVersion: "3.1"}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(p) == 0 {
		p = []byte(`[]`)
	}
	s.Personnel = json.RawMessage(p)
	return &s, nil
}

func (d *DB) SaveReportSettings(s *ReportSettings) error {
	if len(s.Personnel) == 0 {
		s.Personnel = json.RawMessage(`[]`)
	}
	if s.CVSSVersion == "" {
		s.CVSSVersion = "3.1"
	}
	_, err := d.Exec(`INSERT INTO report_settings(id,company_name,company_name_en,logo_path,personnel_json,cvss_version)
		VALUES (1,$1,$2,$3,$4::jsonb,$5)
		ON CONFLICT (id) DO UPDATE SET company_name=EXCLUDED.company_name, company_name_en=EXCLUDED.company_name_en,
		  logo_path=EXCLUDED.logo_path, personnel_json=EXCLUDED.personnel_json, cvss_version=EXCLUDED.cvss_version,
		  updated_at=now()`, s.CompanyName, s.CompanyNameEn, s.LogoPath, string(s.Personnel), s.CVSSVersion)
	return err
}

type ReportRecord struct {
	ID        int64      `json:"id"`
	TaskID    *int64     `json:"task_id,omitempty"`
	TaskDesc  string     `json:"task_desc"`
	Filename  string     `json:"filename"`
	Path      string     `json:"-"`
	Size      int64      `json:"size"`
	Company   string     `json:"company"`
	Status    string     `json:"status"`
	Error     string     `json:"error,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

func (d *DB) CreateReportRecord(taskID *int64, taskDesc, company string) (*ReportRecord, error) {
	var r ReportRecord
	err := d.QueryRow(`INSERT INTO reports(task_id,task_desc,company,status)
		VALUES ($1,$2,$3,'queued') RETURNING id,status,created_at`, taskID, taskDesc, company).
		Scan(&r.ID, &r.Status, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	r.TaskID, r.TaskDesc, r.Company = taskID, taskDesc, company
	return &r, nil
}

func (d *DB) UpdateReportRecord(id int64, status, filename, path, errMsg string, size int64) error {
	if status == "running" {
		_, err := d.Exec(`UPDATE reports SET status=$2, error=$3, started_at=now() WHERE id=$1`, id, status, errMsg)
		return err
	}
	_, err := d.Exec(`UPDATE reports SET status=$2, filename=$3, path=$4, size=$5, error=$6, finished_at=now() WHERE id=$1`,
		id, status, filename, path, size, errMsg)
	return err
}

func (d *DB) GetReportRecord(id int64) (*ReportRecord, error) {
	var r ReportRecord
	var task, started, finished sql.NullTime
	var taskID sql.NullInt64
	err := d.QueryRow(`SELECT id,task_id,task_desc,filename,path,size,company,status,error,created_at,started_at,finished_at
		FROM reports WHERE id=$1`, id).
		Scan(&r.ID, &taskID, &r.TaskDesc, &r.Filename, &r.Path, &r.Size, &r.Company, &r.Status, &r.Error, &r.CreatedAt, &started, &finished)
	if err != nil {
		return nil, err
	}
	if taskID.Valid {
		v := taskID.Int64
		r.TaskID = &v
	}
	if started.Valid {
		r.StartedAt = &started.Time
	}
	if finished.Valid {
		r.FinishedAt = &finished.Time
	}
	_ = task
	return &r, nil
}

func (d *DB) ListReportRecords(limit int) ([]ReportRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := d.Query(`SELECT id,task_id,task_desc,filename,path,size,company,status,error,created_at,started_at,finished_at
		FROM reports ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReportRecord
	for rows.Next() {
		var r ReportRecord
		var taskID sql.NullInt64
		var started, finished sql.NullTime
		if err := rows.Scan(&r.ID, &taskID, &r.TaskDesc, &r.Filename, &r.Path, &r.Size, &r.Company, &r.Status, &r.Error, &r.CreatedAt, &started, &finished); err != nil {
			return nil, err
		}
		if taskID.Valid {
			v := taskID.Int64
			r.TaskID = &v
		}
		if started.Valid {
			r.StartedAt = &started.Time
		}
		if finished.Valid {
			r.FinishedAt = &finished.Time
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) DeleteReportRecord(id int64) (string, error) {
	var path string
	if err := d.QueryRow(`SELECT path FROM reports WHERE id=$1`, id).Scan(&path); err != nil {
		return "", err
	}
	_, err := d.Exec(`DELETE FROM reports WHERE id=$1`, id)
	return path, err
}
