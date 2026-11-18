package agent

import (
	"context"
	"testing"

	actool "github.com/Autumn-27/norma/tool"
)

func buildNamed(names ...string) []actool.CoreTool {
	out := make([]actool.CoreTool, 0, len(names))
	for _, n := range names {
		out = append(out, actool.Build(actool.Spec{Name: n, Description: "d"}))
	}
	return out
}

// 非管理员运行必须同时摘掉命令执行与宿主文件读取：只摘 Bash 时，Read 仍可
// 读取 jwt.key / config.json，F-01 的密钥泄漏路径没有关闭。
func TestDropHostAccessToolsCoversReadAndExec(t *testing.T) {
	tools := buildNamed("Bash", "Read", "LS", "Glob", "Grep", "Write", "job_start", "shell_open", "http_request", "save_evidence")
	out := DropHostAccessTools(tools)
	kept := map[string]bool{}
	for _, tl := range out {
		kept[tl.Name()] = true
	}
	for _, banned := range []string{"Bash", "Read", "LS", "Glob", "Grep", "Write", "job_start", "shell_open"} {
		if kept[banned] {
			t.Errorf("非管理员运行不应拿到 %s", banned)
		}
	}
	for _, want := range []string{"http_request", "save_evidence"} {
		if !kept[want] {
			t.Errorf("联网测试/证据工具应保留: %s", want)
		}
	}
}

func TestRunAdminDefaultsTrueAndCanBeDisabled(t *testing.T) {
	if !RunAdminFrom(context.Background()) {
		t.Fatal("未标注的运行应按管理员语义（系统内部运行）")
	}
	if RunAdminFrom(WithRunAdmin(context.Background(), false)) {
		t.Fatal("WithRunAdmin(false) 必须生效")
	}
}

func TestFilterSensitiveEnv(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"POJUN_PG_DSN=postgres://u:p@db/x",
		"ANTHROPIC_API_KEY=sk-secret",
		"GITHUB_TOKEN=ghp_x",
		"AWS_SECRET_ACCESS_KEY=x",
		"HTTP_PROXY=http://127.0.0.1:7890",
	}
	out := FilterSensitiveEnv(env)
	kept := map[string]bool{}
	for _, kv := range out {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				kept[kv[:i]] = true
				break
			}
		}
	}
	for _, banned := range []string{"POJUN_PG_DSN", "ANTHROPIC_API_KEY", "GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY"} {
		if kept[banned] {
			t.Errorf("敏感变量未剔除: %s", banned)
		}
	}
	for _, want := range []string{"PATH", "HTTP_PROXY"} {
		if !kept[want] {
			t.Errorf("非敏感变量不应被删除: %s", want)
		}
	}
}
