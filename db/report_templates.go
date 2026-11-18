package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// ReportTemplate 是可复用的报告模板：公司名、Logo、人员、CVSS 版本与语言。
// report_settings 仍是组织级单例默认值；模板用于按客户/项目切换不同交付口径。
type ReportTemplate struct {
	ID            int64           `json:"id"`
	Name          string          `json:"name"`
	CompanyName   string          `json:"company_name"`
	CompanyNameEn string          `json:"company_name_en"`
	LogoPath      string          `json:"logo_path"`
	Personnel     json.RawMessage `json:"personnel_json"`
	CVSSVersion   string          `json:"cvss_version"`
	Locale        string          `json:"locale"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

func normalizeReportTemplate(t *ReportTemplate) {
	if len(t.Personnel) == 0 {
		t.Personnel = json.RawMessage(`[]`)
	}
	if t.CVSSVersion == "" {
		t.CVSSVersion = "3.1"
	}
	if t.Locale == "" {
		t.Locale = "zh-CN"
	}
}

func (d *DB) ListReportTemplates() ([]*ReportTemplate, error) {
	rows, err := d.Query(`SELECT id,name,company_name,company_name_en,logo_path,personnel_json::text,
		cvss_version,locale,created_at,updated_at FROM report_templates ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*ReportTemplate
	for rows.Next() {
		var t ReportTemplate
		var personnel []byte
		if err := rows.Scan(&t.ID, &t.Name, &t.CompanyName, &t.CompanyNameEn, &t.LogoPath, &personnel,
			&t.CVSSVersion, &t.Locale, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		if len(personnel) == 0 {
			personnel = []byte(`[]`)
		}
		t.Personnel = json.RawMessage(personnel)
		out = append(out, &t)
	}
	return out, rows.Err()
}

func (d *DB) GetReportTemplate(id int64) (*ReportTemplate, error) {
	var t ReportTemplate
	var personnel []byte
	err := d.QueryRow(`SELECT id,name,company_name,company_name_en,logo_path,personnel_json::text,
		cvss_version,locale,created_at,updated_at FROM report_templates WHERE id=$1`, id).
		Scan(&t.ID, &t.Name, &t.CompanyName, &t.CompanyNameEn, &t.LogoPath, &personnel,
			&t.CVSSVersion, &t.Locale, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(personnel) == 0 {
		personnel = []byte(`[]`)
	}
	t.Personnel = json.RawMessage(personnel)
	return &t, nil
}

func (d *DB) CreateReportTemplate(t *ReportTemplate) (*ReportTemplate, error) {
	normalizeReportTemplate(t)
	err := d.QueryRow(`INSERT INTO report_templates(name,company_name,company_name_en,logo_path,personnel_json,cvss_version,locale)
		VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7) RETURNING id,created_at,updated_at`,
		t.Name, t.CompanyName, t.CompanyNameEn, t.LogoPath, string(t.Personnel), t.CVSSVersion, t.Locale).
		Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (d *DB) UpdateReportTemplate(t *ReportTemplate) error {
	normalizeReportTemplate(t)
	res, err := d.Exec(`UPDATE report_templates SET name=$2,company_name=$3,company_name_en=$4,logo_path=$5,
		personnel_json=$6::jsonb,cvss_version=$7,locale=$8,updated_at=now() WHERE id=$1`,
		t.ID, t.Name, t.CompanyName, t.CompanyNameEn, t.LogoPath, string(t.Personnel), t.CVSSVersion, t.Locale)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *DB) DeleteReportTemplate(id int64) error {
	res, err := d.Exec(`DELETE FROM report_templates WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
