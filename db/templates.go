package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Planner / Worker 提示词模板库（与 task_templates 的任务描述模板不同）。
// 四级提示词链：任务快照 > 全局覆盖 > 模板库默认 > 内置默认。

type PromptTemplate struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	NKey      string    `json:"nkey"`
	Body      string    `json:"body"`
	Builtin   bool      `json:"builtin"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (d *DB) ListPromptTemplates(kind string) ([]PromptTemplate, error) {
	table, err := templateTable(kind)
	if err != nil {
		return nil, err
	}
	rows, err := d.Query(`SELECT id,name,nkey,body,builtin,is_default,created_at,updated_at
		FROM ` + table + ` ORDER BY builtin DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PromptTemplate
	for rows.Next() {
		var t PromptTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.NKey, &t.Body, &t.Builtin, &t.IsDefault, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (d *DB) GetPromptTemplate(kind string, id int64) (*PromptTemplate, error) {
	table, err := templateTable(kind)
	if err != nil {
		return nil, err
	}
	var t PromptTemplate
	err = d.QueryRow(`SELECT id,name,nkey,body,builtin,is_default,created_at,updated_at
		FROM `+table+` WHERE id=$1`, id).
		Scan(&t.ID, &t.Name, &t.NKey, &t.Body, &t.Builtin, &t.IsDefault, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (d *DB) GetPromptTemplateByNKey(kind, nkey string) (*PromptTemplate, error) {
	table, err := templateTable(kind)
	if err != nil {
		return nil, err
	}
	var t PromptTemplate
	err = d.QueryRow(`SELECT id,name,nkey,body,builtin,is_default,created_at,updated_at
		FROM `+table+` WHERE nkey=$1`, strings.TrimSpace(nkey)).
		Scan(&t.ID, &t.Name, &t.NKey, &t.Body, &t.Builtin, &t.IsDefault, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// DefaultPromptTemplateBody 返回模板库中 is_default 的正文；没有则空串。
func (d *DB) DefaultPromptTemplateBody(kind string) string {
	table, err := templateTable(kind)
	if err != nil {
		return ""
	}
	var body string
	if err := d.QueryRow(`SELECT body FROM `+table+` WHERE is_default ORDER BY id LIMIT 1`).Scan(&body); err != nil {
		return ""
	}
	return body
}

func (d *DB) CreatePromptTemplate(kind, name, nkey, body string) (*PromptTemplate, error) {
	table, err := templateTable(kind)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	nkey = strings.TrimSpace(nkey)
	body = strings.TrimSpace(body)
	if name == "" || nkey == "" || body == "" {
		return nil, errors.New("名称、nkey、正文必填")
	}
	if len(body) > 32<<10 {
		return nil, errors.New("模板正文超过 32KB")
	}
	var id int64
	if err := d.QueryRow(`INSERT INTO `+table+`(name,nkey,body) VALUES ($1,$2,$3) RETURNING id`, name, nkey, body).Scan(&id); err != nil {
		return nil, err
	}
	return d.GetPromptTemplate(kind, id)
}

func (d *DB) UpdatePromptTemplate(kind string, id int64, name, body string) error {
	table, err := templateTable(kind)
	if err != nil {
		return err
	}
	t, err := d.GetPromptTemplate(kind, id)
	if err != nil {
		return err
	}
	if t.Builtin && strings.TrimSpace(name) != "" && strings.TrimSpace(name) != t.Name {
		return errors.New("内置模板名称不可修改")
	}
	if len(body) > 32<<10 {
		return errors.New("模板正文超过 32KB")
	}
	if strings.TrimSpace(name) == "" {
		name = t.Name
	}
	_, err = d.Exec(`UPDATE `+table+` SET name=$1, body=$2, updated_at=now() WHERE id=$3`, strings.TrimSpace(name), strings.TrimSpace(body), id)
	return err
}

func (d *DB) DeletePromptTemplate(kind string, id int64) error {
	table, err := templateTable(kind)
	if err != nil {
		return err
	}
	var builtin bool
	if err := d.QueryRow(`SELECT builtin FROM `+table+` WHERE id=$1`, id).Scan(&builtin); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if builtin {
		return errors.New("内置模板不可删除")
	}
	_, err = d.Exec(`DELETE FROM `+table+` WHERE id=$1`, id)
	return err
}

func (d *DB) SetDefaultPromptTemplate(kind string, id int64) error {
	table, err := templateTable(kind)
	if err != nil {
		return err
	}
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE `+table+` SET is_default=false`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE `+table+` SET is_default=true, updated_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// SeedPromptTemplate 幂等播种内置模板（只插入，不覆盖用户修改）。
func (d *DB) SeedPromptTemplate(kind, name, nkey, body string, isDefault bool) error {
	table, err := templateTable(kind)
	if err != nil {
		return err
	}
	_, err = d.Exec(`INSERT INTO `+table+`(name,nkey,body,builtin,is_default)
		VALUES ($1,$2,$3,true,$4) ON CONFLICT (nkey) DO NOTHING`, name, nkey, body, isDefault)
	return err
}

func templateTable(kind string) (string, error) {
	switch kind {
	case "planner":
		return "planner_templates", nil
	case "worker":
		return "worker_templates", nil
	}
	return "", errors.New("unknown template kind")
}
