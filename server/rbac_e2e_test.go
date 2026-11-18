package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestRBACE2E 走真实 Handler + requireAuth：
// admin 登录 → 建 operator/viewer → 角色边界 → 审计可读 → 清理。
//
// 依赖 TestMain 在测试库中播种的 admin（密码 kaiyang-test-password）。
func TestRBACE2E(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v)", err)
	}
	defer m.Close()
	td := t.TempDir()
	s := New(context.Background(), m, td, td, td)
	h := s.Handler()

	do := func(token, method, path string, body any) (int, map[string]any) {
		var r *http.Request
		if body != nil {
			b, _ := json.Marshal(body)
			r = httptest.NewRequest(method, path, bytes.NewReader(b))
		} else {
			r = httptest.NewRequest(method, path, nil)
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}

	login := func(username, password string) string {
		code, out := do("", http.MethodPost, "/api/auth/login", map[string]any{"username": username, "password": password})
		if code != 200 {
			t.Fatalf("login %s: status=%d body=%v", username, code, out)
		}
		tok, _ := out["token"].(string)
		if tok == "" {
			t.Fatalf("login %s: no token", username)
		}
		return tok
	}

	adminTok := login("admin", "kaiyang-test-password")

	suffix := time.Now().UnixNano()
	opName := fmt.Sprintf("op%d", suffix)
	viewName := fmt.Sprintf("view%d", suffix)
	password := "kaiyang-test-password"

	create := func(name, role string) string {
		code, out := do(adminTok, http.MethodPost, "/api/rbac/users", map[string]any{
			"username": name, "display_name": name, "password": password, "role_ids": []string{role},
		})
		if code != 200 {
			t.Fatalf("create %s: status=%d body=%v", name, code, out)
		}
		u, _ := out["user"].(map[string]any)
		id, _ := u["id"].(string)
		if id == "" {
			t.Fatalf("create %s: no id: %v", name, out)
		}
		t.Cleanup(func() { do(adminTok, http.MethodDelete, "/api/rbac/users/"+id, nil) })
		return id
	}
	create(opName, "operator")
	create(viewName, "viewer")

	opTok := login(opName, password)
	viewTok := login(viewName, password)

	// admin 能读用户；operator/viewer 不能。
	if code, _ := do(adminTok, http.MethodGet, "/api/rbac/users", nil); code != 200 {
		t.Fatalf("admin list users = %d, want 200", code)
	}
	if code, _ := do(opTok, http.MethodGet, "/api/rbac/users", nil); code != 403 {
		t.Fatalf("operator list users = %d, want 403", code)
	}
	if code, _ := do(viewTok, http.MethodGet, "/api/rbac/users", nil); code != 403 {
		t.Fatalf("viewer list users = %d, want 403", code)
	}

	// 业务只读：operator/viewer 都能看任务。
	if code, _ := do(opTok, http.MethodGet, "/api/tasks", nil); code != 200 {
		t.Fatalf("operator list tasks = %d, want 200", code)
	}
	if code, _ := do(viewTok, http.MethodGet, "/api/tasks", nil); code != 200 {
		t.Fatalf("viewer list tasks = %d, want 200", code)
	}
	// 写：viewer 403，operator 允许（用不会产生副作用的批量控制空列表验证）。
	if code, _ := do(viewTok, http.MethodPost, "/api/tasks/control/batch", map[string]any{"ids": []string{}, "action": "pause"}); code != 403 {
		t.Fatalf("viewer batch control = %d, want 403", code)
	}
	if code, _ := do(opTok, http.MethodPost, "/api/tasks/control/batch", map[string]any{"ids": []string{}, "action": "pause"}); code == 403 {
		t.Fatalf("operator batch control = 403, want permission (other 4xx/2xx acceptable)")
	}

	// 日志/审计：operator 可读，viewer 不可。
	if code, _ := do(opTok, http.MethodGet, "/api/audit/logs", nil); code != 200 {
		t.Fatalf("operator audit = %d, want 200", code)
	}
	if code, _ := do(viewTok, http.MethodGet, "/api/audit/logs", nil); code != 403 {
		t.Fatalf("viewer audit = %d, want 403", code)
	}
	if code, _ := do(opTok, http.MethodGet, "/api/logs", nil); code != 200 {
		t.Fatalf("operator logs = %d, want 200", code)
	}

	// 未映射路由：admin 放行（404/200 均可），operator 403。
	if code, _ := do(opTok, http.MethodGet, "/api/definitely-not-mapped", nil); code != 403 {
		t.Fatalf("operator unmapped = %d, want 403", code)
	}
}
