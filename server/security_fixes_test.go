package server

import (
	"net/http"
	"testing"

	"github.com/3169417664/kaiyang/db"
)

// F-02：viewer 只有 traffic:read，删除必须映射到 traffic:delete。
func TestTrafficDeleteRequiresDeletePermission(t *testing.T) {
	perm, mapped := permissionForRequest(http.MethodDelete, "/api/traffic/all")
	if !mapped || perm != "traffic:delete" {
		t.Fatalf("DELETE /api/traffic/all -> (%q,%v), want traffic:delete", perm, mapped)
	}
	if db.RBACPermissions[perm] == "" {
		t.Fatal("traffic:delete 未登记到权限目录")
	}
}

// F-03：ScopeSentry 数据源写入等价于 mcp:write，不能借 asset:write 下放。
func TestScopeSentryDatasourceIsMCPWrite(t *testing.T) {
	perm, mapped := permissionForRequest(http.MethodPost, "/api/sync/scopesentry/datasource")
	if !mapped || perm != "mcp:write" {
		t.Fatalf("datasource -> (%q,%v), want mcp:write", perm, mapped)
	}
}

// F-06：审批可下放，规则/工具门控/judge 配置仅 admin。
func TestInterceptApprovalAndConfigPermissions(t *testing.T) {
	cases := []struct {
		method, path, want string
	}{
		{http.MethodPost, "/api/intercept/pending/1/decide", "intercept:approve"},
		{http.MethodPut, "/api/intercept/tool-config", "intercept:write"},
		{http.MethodPut, "/api/intercept/judge", "intercept:write"},
		{http.MethodPost, "/api/intercept/rules", "intercept:write"},
		{http.MethodPost, "/api/tasks/1/intercept-rules", "intercept:write"},
	}
	for _, tc := range cases {
		got, mapped := permissionForRequest(tc.method, tc.path)
		if !mapped || got != tc.want {
			t.Errorf("%s %s -> (%q,%v), want %q", tc.method, tc.path, got, mapped, tc.want)
		}
	}
}

// F-04：只有真正的位图允许 inline；HTML/SVG 即使被标成 image 也必须走下载。
func TestSafeInlineImageMime(t *testing.T) {
	cases := []struct {
		name, mime string
		want       string
	}{
		{"shot.png", "image/png", "image/png"},
		{"shot.jpg", "image/jpeg", "image/jpeg"},
		{"shot.PNG", "", "image/png"},
		{"report.html", "text/html", ""},
		{"report.html", "image/png", ""},
		{"logo.svg", "image/svg+xml", ""},
		{"x.png", "text/html", ""},
	}
	for _, tc := range cases {
		if got := safeInlineImageMime(tc.name, tc.mime); got != tc.want {
			t.Errorf("safeInlineImageMime(%q,%q)=%q, want %q", tc.name, tc.mime, got, tc.want)
		}
	}
}

// F-03：SSRF 守卫拒绝回环/链路本地 URL。
func TestValidateMCPURLBlocksLocalTargets(t *testing.T) {
	bad := []string{
		"http://127.0.0.1:9200/",
		"http://localhost:8787/",
		"http://[::1]:80/",
		"http://169.254.169.254/latest/meta-data/",
		"file:///etc/passwd",
	}
	for _, u := range bad {
		if err := validateMCPURL(u); err == nil {
			t.Errorf("validateMCPURL(%q) 应拒绝", u)
		}
	}
	if err := validateMCPURL("https://asm.example.com/mcp"); err != nil {
		t.Errorf("公网 https URL 不应被拒绝: %v", err)
	}
}
