package server

import (
	"net/http"
	"testing"
)

func TestPermissionForRequestMapsCoreRoutes(t *testing.T) {
	cases := []struct {
		method, path string
		want         string
		mapped       bool
	}{
		{http.MethodGet, "/api/tasks", "task:read", true},
		{http.MethodPost, "/api/tasks", "task:write", true},
		{http.MethodDelete, "/api/tasks/12", "task:delete", true},
		{http.MethodGet, "/api/tasks/12/review/proposed", "review:read", true},
		{http.MethodPost, "/api/tasks/12/review/33/approve", "review:approve", true},
		{http.MethodGet, "/api/tasks/12/topology", "topology:read", true},
		{http.MethodPost, "/api/tasks/12/topology/nodes", "topology:write", true},
		{http.MethodGet, "/api/tasks/12/llm/resolution", "llm:read", true},
		{http.MethodGet, "/api/rbac/me", "auth:self", true},
		{http.MethodGet, "/api/rbac/users", "rbac:read", true},
		{http.MethodGet, "/api/audit/logs", "log:read", true},
		{http.MethodGet, "/api/hostenv", "hostenv:read", true},
		{http.MethodGet, "/api/listeners", "c2:read", true},
		{http.MethodGet, "/api/reports", "report:read", true},
		{http.MethodGet, "/api/knowledge/search", "knowledge:read", true},
		{http.MethodGet, "/api/credentials/intranet", "credential:read", true},
		{http.MethodGet, "/api/workspace/list", "workspace:read", true},
		{http.MethodPost, "/api/workspace/write", "workspace:write", true},
		{http.MethodGet, "/api/assets", "asset:read", true},
		{http.MethodGet, "/api/traffic", "traffic:read", true},
		{http.MethodGet, "/api/exploration/findings", "finding:read", true},
		{http.MethodGet, "/api/settings", "config:read", true},
		{http.MethodPost, "/api/update/apply", "config:write", true},
		{http.MethodGet, "/api/unknown-module", "", false},
	}
	for _, tc := range cases {
		got, mapped := permissionForRequest(tc.method, tc.path)
		if mapped != tc.mapped || got != tc.want {
			t.Errorf("%s %s -> (%q,%v), want (%q,%v)", tc.method, tc.path, got, mapped, tc.want, tc.mapped)
		}
	}
}

func TestUnmappedRouteIsDenyByDefault(t *testing.T) {
	if _, mapped := permissionForRequest(http.MethodGet, "/api/not-in-the-map"); mapped {
		t.Fatal("未映射路由必须返回 mapped=false，由 requireAuth 限制为 admin-only")
	}
}
