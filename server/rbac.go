package server

import (
	"net/http"
	"sort"
	"strings"

	"github.com/3169417664/kaiyang/db"
	"golang.org/x/crypto/bcrypt"
)

// 平台权限管理 API（/api/rbac/*）。角色固定三角色，不提供自定义角色编辑。

type rbacUserView struct {
	db.RBACUser
	RoleIDs []string `json:"role_ids"`
}

type rbacRoleView struct {
	db.RBACRole
	Permissions []string `json:"permissions"`
}

// GET /api/rbac/me
func (s *Server) rbacMe(w http.ResponseWriter, r *http.Request) {
	acc := CurrentAccess(r)
	if acc == nil {
		writeErr(w, 401, "会话无效")
		return
	}
	perms := make([]string, 0, len(acc.Permissions))
	for k, v := range acc.Permissions {
		if v {
			perms = append(perms, k)
		}
	}
	sort.Strings(perms)
	writeJSON(w, 200, map[string]any{
		"user":        acc.User,
		"roles":       acc.Roles,
		"permissions": perms,
		"scope":       acc.Scope,
	})
}

// GET /api/rbac/metadata
func (s *Server) rbacMetadata(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	roles, err := pg.ListRBACRoles()
	if err != nil {
		writeErr(w, 500, "获取角色失败")
		return
	}
	writeJSON(w, 200, map[string]any{
		"permissions": db.RBACPermissions,
		"roles":       roles,
		"scopes":      []string{db.RBACScopeAll},
	})
}

// GET /api/rbac/users
func (s *Server) rbacListUsers(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	users, err := pg.ListRBACUsers()
	if err != nil {
		writeErr(w, 500, "获取用户失败")
		return
	}
	out := make([]rbacUserView, 0, len(users))
	for _, u := range users {
		ids, _ := pg.ListRBACUserRoleIDs(u.ID)
		if ids == nil {
			ids = []string{}
		}
		out = append(out, rbacUserView{RBACUser: u, RoleIDs: ids})
	}
	writeJSON(w, 200, map[string]any{"users": out})
}

// POST /api/rbac/users
func (s *Server) rbacCreateUser(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req struct {
		Username    string   `json:"username"`
		DisplayName string   `json:"display_name"`
		Password    string   `json:"password"`
		Enabled     *bool    `json:"enabled"`
		RoleIDs     []string `json:"role_ids"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "参数无效")
		return
	}
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if username == "" {
		writeErr(w, 400, "用户名必填")
		return
	}
	if msg := validatePassword(req.Password); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, "密码处理失败")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	u, err := pg.CreateRBACUser(username, req.DisplayName, string(hash), enabled, req.RoleIDs)
	if err != nil {
		msg := "创建用户失败"
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(err.Error(), "duplicate") {
			msg = "用户名已存在"
		}
		writeErr(w, 400, msg)
		return
	}
	pg.RecordAudit(auditActor(r), "rbac", "create_user", "success", clientIP(r), "创建用户 "+username, map[string]any{"user_id": u.ID})
	writeJSON(w, 200, map[string]any{"user": u})
}

// PUT /api/rbac/users/{id}
func (s *Server) rbacUpdateUser(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeErr(w, 400, "缺少用户 ID")
		return
	}
	var req struct {
		DisplayName string    `json:"display_name"`
		Enabled     *bool     `json:"enabled"`
		RoleIDs     *[]string `json:"role_ids"`
		Password    string    `json:"password"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, "参数无效")
		return
	}
	if err := pg.UpdateRBACUser(id, req.DisplayName, req.Enabled, req.RoleIDs); err != nil {
		writeErr(w, 500, "更新用户失败")
		return
	}
	if pw := strings.TrimSpace(req.Password); pw != "" {
		if msg := validatePassword(pw); msg != "" {
			writeErr(w, 400, msg)
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		if err != nil {
			writeErr(w, 500, "密码处理失败")
			return
		}
		if err := pg.UpdateRBACUserPassword(id, string(hash)); err != nil {
			writeErr(w, 500, "重置密码失败")
			return
		}
	}
	pg.RecordAudit(auditActor(r), "rbac", "update_user", "success", clientIP(r), "更新用户 "+id, nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/rbac/users/{id}
func (s *Server) rbacDeleteUser(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if err := pg.DeleteRBACUser(id); err != nil {
		writeErr(w, 400, "删除失败: "+err.Error())
		return
	}
	pg.RecordAudit(auditActor(r), "rbac", "delete_user", "success", clientIP(r), "删除用户 "+id, nil)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/rbac/roles
func (s *Server) rbacListRoles(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	roles, err := pg.ListRBACRoles()
	if err != nil {
		writeErr(w, 500, "获取角色失败")
		return
	}
	out := make([]rbacRoleView, 0, len(roles))
	for _, role := range roles {
		keys, _ := pg.ListRBACRolePermissionKeys(role.ID)
		if keys == nil {
			keys = []string{}
		}
		out = append(out, rbacRoleView{RBACRole: role, Permissions: keys})
	}
	writeJSON(w, 200, map[string]any{"roles": out})
}
