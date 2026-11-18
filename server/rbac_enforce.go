package server

import (
	"net/http"
	"strings"

	"github.com/3169417664/kaiyang/db"
)

// RBAC 路由 → 权限映射。
//
// 与 AntiFox 的关键差异：**未映射路由默认 admin-only**，而不是放行。
// 新增路由必须在这里补映射，并补测试；否则 operator/viewer 会被 403。

func isAdmin(r *http.Request) bool { return accessIsAdmin(CurrentAccess(r)) }

func hostExecToolKind(kind string) bool {
	switch kind {
	case "command", "script", "shell":
		return true
	}
	return false
}

func crudPerm(method, module string) string {
	switch method {
	case http.MethodGet, http.MethodHead:
		return module + ":read"
	case http.MethodDelete:
		if _, ok := db.RBACPermissions[module+":delete"]; ok {
			return module + ":delete"
		}
		return module + ":write"
	default:
		return module + ":write"
	}
}

// permissionForRequest 返回 (权限键, 是否已映射)。mapped=false 表示未映射，
// requireAuth 会把它限制为 admin-only。
func permissionForRequest(method, fullPath string) (string, bool) {
	p := strings.TrimPrefix(fullPath, "/api")
	if p == "" {
		p = "/"
	}

	// 身份与审计
	switch {
	case p == "/rbac/me":
		return "auth:self", true
	case strings.HasPrefix(p, "/rbac"):
		if method == http.MethodGet || method == http.MethodHead {
			return "rbac:read", true
		}
		return "rbac:write", true
	case p == "/audit/logs":
		return "log:read", true
	case strings.HasPrefix(p, "/logs"):
		return "log:read", true
	}

	// 审核：读与审批分离
	if strings.Contains(p, "/review") {
		if method == http.MethodGet || method == http.MethodHead {
			return "review:read", true
		}
		return "review:approve", true
	}

	// 拓扑
	if strings.Contains(p, "/topology") {
		if method == http.MethodGet || method == http.MethodHead {
			return "topology:read", true
		}
		return "topology:write", true
	}

	// 任务级 LLM 配置属于 LLM 管理，不随 task:write 下放
	if strings.HasPrefix(p, "/tasks/") && strings.Contains(p, "/llm") {
		return crudPerm(method, "llm"), true
	}

	// 高危能力：admin-only 模块
	switch {
	case strings.HasPrefix(p, "/update"):
		return crudPerm(method, "config"), true
	case strings.HasPrefix(p, "/hostenv"), p == "/tools/inventory":
		return crudPerm(method, "hostenv"), true
	case strings.HasPrefix(p, "/listeners"), strings.HasPrefix(p, "/c2"):
		return crudPerm(method, "c2"), true
	case strings.HasPrefix(p, "/settings"):
		return crudPerm(method, "config"), true
	case strings.HasPrefix(p, "/llm"):
		return crudPerm(method, "llm"), true
	case strings.HasPrefix(p, "/agents"), strings.HasPrefix(p, "/triggers"),
		strings.HasPrefix(p, "/visibility"), strings.HasPrefix(p, "/prompt-vars"):
		return crudPerm(method, "agent"), true
	case strings.HasPrefix(p, "/mcp"):
		return crudPerm(method, "mcp"), true
	case strings.HasPrefix(p, "/tools"):
		return crudPerm(method, "tool"), true
	}

	// 业务模块
	switch {
	case p == "/stats" || strings.HasPrefix(p, "/tokens"), p == "/version":
		return "dashboard:read", true
	case strings.HasPrefix(p, "/workspace"):
		if method == http.MethodGet || method == http.MethodHead {
			return "workspace:read", true
		}
		return "workspace:write", true
	case strings.HasPrefix(p, "/exploration/findings"), strings.HasPrefix(p, "/findings"):
		return crudPerm(method, "finding"), true
	case strings.HasPrefix(p, "/exploration"), strings.HasPrefix(p, "/tasks"),
		strings.HasPrefix(p, "/task-categories"), strings.HasPrefix(p, "/task-templates"),
		strings.HasPrefix(p, "/task-archives"), strings.HasPrefix(p, "/active"),
		strings.HasPrefix(p, "/chat"), strings.HasPrefix(p, "/conversations"),
		strings.HasPrefix(p, "/sessions"), strings.HasPrefix(p, "/side-questions"),
		strings.HasPrefix(p, "/commands"), strings.HasPrefix(p, "/llm-records"):
		return crudPerm(method, "task"), true
	case strings.HasPrefix(p, "/traffic"):
		return "traffic:read", true
	case strings.HasPrefix(p, "/credentials"):
		return "credential:read", true
	case strings.HasPrefix(p, "/assets"), strings.HasPrefix(p, "/companies"), strings.HasPrefix(p, "/sync"):
		return crudPerm(method, "asset"), true
	case strings.HasPrefix(p, "/intercept"), strings.HasPrefix(p, "/asset-intercept"):
		return crudPerm(method, "intercept"), true
	case strings.HasPrefix(p, "/report"), strings.HasPrefix(p, "/reports"):
		return crudPerm(method, "report"), true
	case strings.HasPrefix(p, "/knowledge"):
		if p == "/knowledge/search" || p == "/knowledge/categories" || p == "/knowledge/logs" {
			return "knowledge:read", true
		}
		return crudPerm(method, "knowledge"), true
	case strings.HasPrefix(p, "/notify"):
		return crudPerm(method, "notify"), true
	case strings.HasPrefix(p, "/proxypool"):
		return crudPerm(method, "proxy"), true
	case strings.HasPrefix(p, "/vulnintel"):
		return crudPerm(method, "vulnintel"), true
	case strings.HasPrefix(p, "/skills"):
		return crudPerm(method, "skill"), true
	}

	return "", false
}
