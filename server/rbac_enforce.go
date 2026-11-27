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
	// 任务级多角色凭据池 / SRC 众测 profile：跟随 task 读写权限。
	// 凭据秘密只在 DB 与 agent 解析层使用，接口只回 DTO（无 secret 字段）。
	if strings.HasPrefix(p, "/tasks/") && (strings.Contains(p, "/credentials") || strings.Contains(p, "/program-profile")) {
		return crudPerm(method, "task"), true
	}
	// 任务级拦截规则是安全控制面的一部分：读可下放，写仅 admin。
	if strings.Contains(p, "/intercept-rules") {
		if method == http.MethodGet || method == http.MethodHead {
			return "intercept:read", true
		}
		return "intercept:write", true
	}
	// ScopeSentry 数据源写入 = 配置一个会被服务端连接的 MCP URL，等价于 mcp:write；
	// 不能借用 asset:write 让 operator 绕过 admin-only 的 MCP 管理。
	if p == "/sync/scopesentry/datasource" {
		return "mcp:write", true
	}

	// 高危能力：admin-only 模块
	switch {
	case strings.HasPrefix(p, "/update"):
		return crudPerm(method, "config"), true
	case strings.HasPrefix(p, "/hostenv"), p == "/tools/inventory":
		return crudPerm(method, "hostenv"), true
	case strings.HasPrefix(p, "/listeners"), strings.HasPrefix(p, "/c2"):
		return crudPerm(method, "c2"), true
	case strings.HasPrefix(p, "/settings"), strings.HasPrefix(p, "/resource"),
		strings.HasPrefix(p, "/tls"):
		// 资源计划与 TLS/证书都属于管理面：读写都归 config 权限（默认 admin）。
		return crudPerm(method, "config"), true
	case strings.HasPrefix(p, "/cleanup"):
		// 删除后清理队列是运维控制面：读列表可给 config:read，重试/消费仅 admin。
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
	case strings.HasPrefix(p, "/planner-templates"), strings.HasPrefix(p, "/worker-templates"):
		return crudPerm(method, "task"), true
	case strings.HasPrefix(p, "/exploration"), strings.HasPrefix(p, "/tasks"),
		strings.HasPrefix(p, "/task-categories"), strings.HasPrefix(p, "/task-templates"),
		strings.HasPrefix(p, "/task-archives"), strings.HasPrefix(p, "/active"),
		strings.HasPrefix(p, "/chat"), strings.HasPrefix(p, "/conversations"),
		strings.HasPrefix(p, "/sessions"), strings.HasPrefix(p, "/side-questions"),
		strings.HasPrefix(p, "/commands"), strings.HasPrefix(p, "/llm-records"):
		return crudPerm(method, "task"), true
	case p == "/traffic/retention":
		// 留存执行等价于删除：POST 即使 dry_run 也属于删除控制面，不能给 viewer。
		if method == http.MethodGet || method == http.MethodHead {
			return "traffic:read", true
		}
		return "traffic:delete", true
	case strings.HasPrefix(p, "/traffic"):
		// 读与删必须分开：viewer 只有 traffic:read，不能清空证据。
		if method == http.MethodDelete {
			return "traffic:delete", true
		}
		return "traffic:read", true
	case strings.HasPrefix(p, "/credentials"):
		return "credential:read", true
	case strings.HasPrefix(p, "/assets"), strings.HasPrefix(p, "/companies"), strings.HasPrefix(p, "/sync"):
		return crudPerm(method, "asset"), true
	case strings.HasPrefix(p, "/intercept"), strings.HasPrefix(p, "/asset-intercept"):
		if method == http.MethodGet || method == http.MethodHead {
			return "intercept:read", true
		}
		// 审批待办可交给 operator；改规则 / 工具门控 / judge 配置仅 admin。
		if strings.HasPrefix(p, "/intercept/pending/") && strings.HasSuffix(p, "/decide") {
			return "intercept:approve", true
		}
		return "intercept:write", true
	case strings.HasPrefix(p, "/report-templates"), strings.HasPrefix(p, "/report-assets"):
		// Phase 5：模板 CRUD 与 Logo 上传/回放归入报告权限；未映射路由默认
		// admin-only，所以这里必须显式列出。
		return crudPerm(method, "report"), true
	case strings.HasPrefix(p, "/report"), strings.HasPrefix(p, "/reports"):
		return crudPerm(method, "report"), true
	case strings.HasPrefix(p, "/knowledge"):
		if p == "/knowledge/search" || p == "/knowledge/categories" || p == "/knowledge/logs" {
			return "knowledge:read", true
		}
		return crudPerm(method, "knowledge"), true
	case strings.HasPrefix(p, "/notify"):
		return crudPerm(method, "notify"), true
	case strings.HasPrefix(p, "/diagnostics"), strings.HasPrefix(p, "/egress"),
		strings.HasPrefix(p, "/proxypool/mihomo"):
		// 内核安装、SSH 跳板凭据、诊断包都属于管理面，不随 proxy:write 下放。
		return crudPerm(method, "config"), true
	case strings.HasPrefix(p, "/proxypool"):
		return crudPerm(method, "proxy"), true
	case strings.HasPrefix(p, "/vulnintel"):
		return crudPerm(method, "vulnintel"), true
	case strings.HasPrefix(p, "/skills"):
		return crudPerm(method, "skill"), true
	}

	return "", false
}
