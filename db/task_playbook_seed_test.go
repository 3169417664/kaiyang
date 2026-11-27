package db

import (
	"strings"
	"testing"
)

// v0.4.60：内置任务 Playbook 种子——三批模板入库、幂等、可检索。
func TestSeedBuiltinTaskPlaybooks(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()
	// 干净批次：清掉本测试的历史种子标记与同名模板（共享测试库）。
	_, _ = d.Exec(`DELETE FROM task_templates WHERE name LIKE 'Playbook：%'`)
	_, _ = d.Exec(`DELETE FROM settings WHERE key IN ('task_playbooks_v1','task_playbooks_v2','task_playbooks_v4')`)

	if err := d.seedBuiltinTaskPlaybooks(); err != nil {
		t.Fatalf("seed v1: %v", err)
	}
	if err := d.seedBuiltinTaskPlaybooksV2(); err != nil {
		t.Fatalf("seed v2: %v", err)
	}
	if err := d.seedBuiltinTaskPlaybooksV4(); err != nil {
		t.Fatalf("seed v4: %v", err)
	}
	list, err := d.ListTaskTemplates()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, tpl := range list {
		if strings.HasPrefix(tpl.Name, "Playbook：") {
			count++
			if !strings.Contains(tpl.Goal, "【内部工作流") && !strings.Contains(tpl.Goal, "【工作流") && !strings.Contains(tpl.Goal, "【产出】") {
				t.Errorf("playbook %q 缺少结构化工作流段", tpl.Name)
			}
		}
	}
	want := len(builtinTaskPlaybooks) + len(builtinTaskPlaybooksV2) + len(builtinTaskPlaybooksV4)
	if count != want {
		t.Fatalf("playbook 数=%d want %d", count, want)
	}
	// 幂等：再跑一次不新增、不报错（flag 已 done）。
	before := len(list)
	if err := d.seedBuiltinTaskPlaybooks(); err != nil {
		t.Fatalf("second seed v1: %v", err)
	}
	if err := d.seedBuiltinTaskPlaybooksV2(); err != nil {
		t.Fatalf("second seed v2: %v", err)
	}
	if err := d.seedBuiltinTaskPlaybooksV4(); err != nil {
		t.Fatalf("second seed v4: %v", err)
	}
	after, _ := d.ListTaskTemplates()
	if len(after) != before {
		t.Fatalf("重复种子不应新增：before=%d after=%d", before, len(after))
	}
}

// 老库迁移：v1 已 done 时，v2 仍必须插入（这是第一批 flag 设计的已知坑）。
func TestSeedBuiltinTaskPlaybooksV2OnOldDB(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()
	_, _ = d.Exec(`DELETE FROM task_templates WHERE name LIKE 'Playbook：%'`)
	_, _ = d.Exec(`DELETE FROM settings WHERE key IN ('task_playbooks_v1','task_playbooks_v2','task_playbooks_v4')`)
	// 模拟老库：v1 已执行完毕，v2 从未执行。
	if _, err := d.Exec(`INSERT INTO settings(key,value) VALUES ('task_playbooks_v1','done') ON CONFLICT (key) DO UPDATE SET value='done'`); err != nil {
		t.Fatalf("set v1 done: %v", err)
	}
	if err := d.seedBuiltinTaskPlaybooks(); err != nil {
		t.Fatalf("seed v1: %v", err)
	}
	if err := d.seedBuiltinTaskPlaybooksV2(); err != nil {
		t.Fatalf("seed v2: %v", err)
	}
	list, err := d.ListTaskTemplates()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tpl := range list {
		if strings.Contains(tpl.Name, "微信小程序") {
			found = true
		}
	}
	if !found {
		t.Fatalf("v2 模板未插入（v1=done 不应阻塞 v2）")
	}
}

// 老库迁移：v1/v2 均已 done 的老库，v4 批次仍必须插入。
func TestSeedBuiltinTaskPlaybooksV4OnOldDB(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()
	_, _ = d.Exec(`DELETE FROM task_templates WHERE name LIKE 'Playbook：%'`)
	_, _ = d.Exec(`DELETE FROM settings WHERE key IN ('task_playbooks_v1','task_playbooks_v2','task_playbooks_v4')`)
	// 模拟老库：v1/v2 已执行完毕，v4 从未执行。
	if _, err := d.Exec(`INSERT INTO settings(key,value) VALUES ('task_playbooks_v1','done'),('task_playbooks_v2','done')
ON CONFLICT (key) DO UPDATE SET value='done'`); err != nil {
		t.Fatalf("set v1/v2 done: %v", err)
	}
	if err := d.seedBuiltinTaskPlaybooks(); err != nil {
		t.Fatalf("seed v1: %v", err)
	}
	if err := d.seedBuiltinTaskPlaybooksV2(); err != nil {
		t.Fatalf("seed v2: %v", err)
	}
	if err := d.seedBuiltinTaskPlaybooksV4(); err != nil {
		t.Fatalf("seed v4: %v", err)
	}
	list, err := d.ListTaskTemplates()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tpl := range list {
		if strings.Contains(tpl.Name, "API 安全全流程") {
			found = true
		}
	}
	if !found {
		t.Fatalf("v4 模板未插入（v1/v2=done 不应阻塞 v4）")
	}
	if v, _, _ := d.GetSetting("task_playbooks_v4"); v != "done" {
		t.Fatalf("task_playbooks_v4 flag=%q, want done", v)
	}
}

// 用户改过的同名模板不被 v4 种子复活（插过即跳过）。
func TestSeedBuiltinTaskPlaybooksV4KeepsUserEdits(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	defer d.Close()
	const name = "Playbook：AI/LLM 应用安全"
	_, _ = d.Exec(`DELETE FROM task_templates WHERE name = $1`, name)
	_, _ = d.Exec(`DELETE FROM settings WHERE key = 'task_playbooks_v4'`)
	custom, err := d.CreateTaskTemplate(TaskTemplateInput{
		Name:        name,
		Description: "用户自定义",
		Goal:        "USER-EDITED-GOAL",
	})
	if err != nil {
		t.Fatalf("create custom template: %v", err)
	}
	defer func() { _, _ = d.Exec(`DELETE FROM task_templates WHERE id = $1`, custom.ID) }()

	if err := d.seedBuiltinTaskPlaybooksV4(); err != nil {
		t.Fatalf("seed v4: %v", err)
	}
	list, err := d.ListTaskTemplates()
	if err != nil {
		t.Fatal(err)
	}
	for _, tpl := range list {
		if tpl.Name == name {
			if tpl.Goal != "USER-EDITED-GOAL" {
				t.Fatalf("用户修改被种子覆盖：%q", tpl.Goal)
			}
			return
		}
	}
	t.Fatalf("用户模板 %q 丢失", name)
}
