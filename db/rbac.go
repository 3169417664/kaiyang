package db

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// KAIYANG 多用户 RBAC 数据层。
//
// 设计边界（与产品决策一致）：
//   - 三个系统角色：admin / operator / viewer；
//   - 权限表 + 路由级强制，但不做任务/资产归属隔离；
//   - 不提供自定义角色编辑器，角色集合固定；
//   - scope 列保留给未来资源隔离，当前一律 all。

const (
	RBACRoleAdmin    = "admin"
	RBACRoleOperator = "operator"
	RBACRoleViewer   = "viewer"

	RBACScopeAll = "all"
)

// RBACPermissions 是 KAIYANG 的权限目录（module:action）。
var RBACPermissions = map[string]string{
	"auth:self":         "管理自己的会话和密码",
	"dashboard:read":    "查看仪表盘总览",
	"task:read":         "查看任务 / 探索链路 / 会话",
	"task:write":        "创建 / 控制任务、发送对话引导",
	"task:delete":       "删除任务 / 归档",
	"finding:read":      "查看漏洞发现",
	"finding:write":     "创建 / 更新漏洞发现、富化、证据",
	"finding:delete":    "删除漏洞发现",
	"traffic:read":      "查看流量录制",
	"traffic:delete":    "删除流量证据（破坏性，仅管理员）",
	"asset:read":        "查看资产 / 公司 / 资产同步",
	"asset:write":       "创建 / 导入 / 同步资产",
	"asset:delete":      "删除资产",
	"credential:read":   "查看资产凭据（不含 secret）",
	"review:read":       "查看待审意图 / 审核会话",
	"review:approve":    "批准 / 驳回意图",
	"topology:read":     "查看内网拓扑",
	"topology:write":    "编辑内网拓扑",
	"intercept:read":    "查看拦截规则 / 审批",
	"intercept:approve": "处理待审批的拦截请求（不能改规则/开关）",
	"intercept:write":   "管理拦截规则 / 全局审批配置（仅管理员）",
	"report:read":       "查看 / 下载报告与模板",
	"report:write":      "生成报告、管理模板与报告设置",
	"report:delete":     "删除报告 / 模板",
	"knowledge:read":    "查看知识库 / 检索",
	"knowledge:write":   "创建 / 更新知识库文档",
	"knowledge:delete":  "删除知识库文档",
	"notify:read":       "查看通知渠道与投递",
	"notify:write":      "管理通知渠道",
	"proxy:read":        "查看代理池 / 出口",
	"proxy:write":       "管理代理池 / 出口",
	"vulnintel:read":    "查看漏洞情报",
	"vulnintel:write":   "同步 / 配置漏洞情报",
	"log:read":          "查看后端日志 / 审计",
	"llm:read":          "查看 LLM 配置",
	"llm:write":         "管理 LLM 配置与模型",
	"agent:read":        "查看 Agent 配置",
	"agent:write":       "管理 Agent / 提示词 / 触发器",
	"mcp:read":          "查看 MCP 服务",
	"mcp:write":         "管理 MCP 服务",
	"skill:read":        "查看技能",
	"skill:write":       "创建 / 更新技能",
	"skill:delete":      "删除技能",
	"tool:read":         "查看工具",
	"tool:write":        "管理工具 / 自定义工具",
	"config:read":       "查看系统配置",
	"config:write":      "修改系统配置 / 更新 / 宿主环境",
	"rbac:read":         "查看用户 / 角色 / 权限",
	"rbac:write":        "管理用户 / 角色",
	"c2:read":           "查看监听器 / C2",
	"c2:write":          "启停监听器 / 管理 C2",
	"hostenv:read":      "查看宿主环境",
	"hostenv:write":     "安装宿主工具 / 维护宿主环境",
	"workspace:read":    "查看 / 下载工作空间文件",
	"workspace:write":   "编辑 / 上传 / 删除工作空间文件",
}

var operatorPermissions = []string{
	"auth:self", "dashboard:read",
	"task:read", "task:write", "task:delete",
	"finding:read", "finding:write", "finding:delete",
	"traffic:read",
	"asset:read", "asset:write", "asset:delete",
	"credential:read",
	"review:read", "review:approve",
	"topology:read", "topology:write",
	// 审批请求可以处理，但规则/工具门控/LLM judge 配置属于安全控制面，仅 admin。
	"intercept:read", "intercept:approve",
	"report:read", "report:write", "report:delete",
	"knowledge:read", "knowledge:write", "knowledge:delete",
	"notify:read", "notify:write",
	"proxy:read", "proxy:write",
	"vulnintel:read", "vulnintel:write",
	"log:read",
	"skill:read",
	"tool:read",
	"workspace:read", "workspace:write",
}

var viewerPermissions = []string{
	"auth:self", "dashboard:read",
	"task:read",
	"finding:read",
	"traffic:read",
	"asset:read",
	"review:read",
	"topology:read",
	"intercept:read",
	"report:read",
	"knowledge:read",
	"vulnintel:read",
	"skill:read",
	"workspace:read",
}

// RBACUser 平台账号。
type RBACUser struct {
	ID                 string    `json:"id"`
	Username           string    `json:"username"`
	DisplayName        string    `json:"display_name"`
	PasswordHash       string    `json:"-"`
	Enabled            bool      `json:"enabled"`
	IsBuiltin          bool      `json:"is_builtin"`
	TokenVersion       int       `json:"-"`
	MustChangePassword bool      `json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// RBACRole 系统角色。scope 当前固定 all，保留给未来资源隔离。
type RBACRole struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Scope       string    `json:"scope"`
	IsSystem    bool      `json:"is_system"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RBACAccess 某用户解析后的授权画像。
type RBACAccess struct {
	User        RBACUser        `json:"user"`
	Roles       []RBACRole      `json:"roles"`
	Permissions map[string]bool `json:"permissions"`
	Scope       string          `json:"scope"`
}

func newRBACID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// RBACNeedsBootstrap 报告是否还没有任何平台用户。
func (d *DB) RBACNeedsBootstrap() (bool, error) {
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM rbac_users`).Scan(&n); err != nil {
		return false, err
	}
	return n == 0, nil
}

// BootstrapRBAC 幂等播种权限目录 + 三角色 + 角色授权；无用户且 adminHash 非空时建 admin。
func (d *DB) BootstrapRBAC(adminHash string, mustChange bool) error {
	_, err := d.BootstrapRBACWithAdmin(adminHash, mustChange)
	return err
}

// BootstrapRBACWithAdmin 与 BootstrapRBAC 相同，但返回“本次调用是否真正创建了
// admin”。首次初始化端点必须依据该返回值签发 token：并发 init 中只有真正写入
// 密码的那个请求可以拿到会话，其余请求即使看到 userCount==0 也不能签发。
func (d *DB) BootstrapRBACWithAdmin(adminHash string, mustChange bool) (bool, error) {
	now := time.Now()
	tx, err := d.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	for key, desc := range RBACPermissions {
		if _, err := tx.Exec(`INSERT INTO rbac_permissions(key, description) VALUES ($1,$2)
			ON CONFLICT (key) DO UPDATE SET description=EXCLUDED.description`, key, desc); err != nil {
			return false, err
		}
	}
	// 清理已下线的权限 key，避免旧库残留。
	rows, err := tx.Query(`SELECT key FROM rbac_permissions`)
	if err != nil {
		return false, err
	}
	var stale []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			rows.Close()
			return false, err
		}
		if _, ok := RBACPermissions[k]; !ok {
			stale = append(stale, k)
		}
	}
	rows.Close()
	for _, k := range stale {
		tx.Exec(`DELETE FROM rbac_role_permissions WHERE permission_key=$1`, k) //nolint:errcheck
		tx.Exec(`DELETE FROM rbac_permissions WHERE key=$1`, k)                 //nolint:errcheck
	}

	sysRoles := []RBACRole{
		{ID: RBACRoleAdmin, Name: "管理员", Description: "全局管理权限", Scope: RBACScopeAll, IsSystem: true},
		{ID: RBACRoleOperator, Name: "操作员", Description: "日常测试、审核、报告与知识库；不能管理系统与高危能力", Scope: RBACScopeAll, IsSystem: true},
		{ID: RBACRoleViewer, Name: "只读用户", Description: "只读查看业务数据", Scope: RBACScopeAll, IsSystem: true},
	}
	for _, r := range sysRoles {
		if _, err := tx.Exec(`INSERT INTO rbac_roles(id,name,description,scope,is_system,created_at,updated_at)
			VALUES ($1,$2,$3,$4,true,$5,$5)
			ON CONFLICT (id) DO UPDATE SET name=EXCLUDED.name, description=EXCLUDED.description, scope=EXCLUDED.scope, is_system=true, updated_at=EXCLUDED.updated_at`,
			r.ID, r.Name, r.Description, r.Scope, now); err != nil {
			return false, err
		}
	}

	// 系统角色授权确定性重建。
	if _, err := tx.Exec(`DELETE FROM rbac_role_permissions WHERE role_id IN ($1,$2,$3)`,
		RBACRoleAdmin, RBACRoleOperator, RBACRoleViewer); err != nil {
		return false, err
	}
	grant := func(role, key string) error {
		if _, ok := RBACPermissions[key]; !ok {
			return nil
		}
		_, err := tx.Exec(`INSERT INTO rbac_role_permissions(role_id,permission_key) VALUES ($1,$2) ON CONFLICT DO NOTHING`, role, key)
		return err
	}
	for key := range RBACPermissions {
		if err := grant(RBACRoleAdmin, key); err != nil {
			return false, err
		}
	}
	for _, key := range operatorPermissions {
		if err := grant(RBACRoleOperator, key); err != nil {
			return false, err
		}
	}
	for _, key := range viewerPermissions {
		if err := grant(RBACRoleViewer, key); err != nil {
			return false, err
		}
	}

	var userCount int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM rbac_users`).Scan(&userCount); err != nil {
		return false, err
	}
	created := false
	if userCount == 0 && strings.TrimSpace(adminHash) != "" {
		// ON CONFLICT 是竞态防护的最后一层：两个并发 init 都读到 count=0 时，
		// 只有先写入者的 RowsAffected=1；后者 0 行，不能继续签发会话。
		res, ierr := tx.Exec(`INSERT INTO rbac_users(id,username,display_name,password_hash,enabled,is_builtin,must_change_password,created_at,updated_at)
			VALUES ('admin','admin','管理员',$1,true,true,$3,$2,$2)
			ON CONFLICT (id) DO NOTHING`, adminHash, now, mustChange)
		if ierr != nil {
			return false, ierr
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			if _, err := tx.Exec(`INSERT INTO rbac_user_roles(user_id,role_id) VALUES ('admin',$1) ON CONFLICT DO NOTHING`, RBACRoleAdmin); err != nil {
				return false, err
			}
			created = true
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return created, nil
}

func scanRBACUser(row interface{ Scan(...any) error }) (*RBACUser, error) {
	var u RBACUser
	if err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.PasswordHash, &u.Enabled, &u.IsBuiltin, &u.TokenVersion, &u.MustChangePassword, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

const rbacUserCols = `id,username,display_name,password_hash,enabled,is_builtin,token_version,must_change_password,created_at,updated_at`

func (d *DB) GetRBACUserByUsername(username string) (*RBACUser, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return nil, sql.ErrNoRows
	}
	return scanRBACUser(d.QueryRow(`SELECT `+rbacUserCols+` FROM rbac_users WHERE username=$1`, username))
}

func (d *DB) GetRBACUserByID(id string) (*RBACUser, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, sql.ErrNoRows
	}
	return scanRBACUser(d.QueryRow(`SELECT `+rbacUserCols+` FROM rbac_users WHERE id=$1`, id))
}

// ResolveRBACAccess 解析用户角色 + 权限。
func (d *DB) ResolveRBACAccess(userID string) (*RBACAccess, error) {
	u, err := d.GetRBACUserByID(userID)
	if err != nil {
		return nil, err
	}
	acc := &RBACAccess{User: *u, Permissions: map[string]bool{}, Scope: RBACScopeAll}
	rows, err := d.Query(`SELECT r.id,r.name,r.description,r.scope,r.is_system,r.created_at,r.updated_at
		FROM rbac_roles r JOIN rbac_user_roles ur ON ur.role_id=r.id WHERE ur.user_id=$1 ORDER BY r.is_system DESC, r.name`, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r RBACRole
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Scope, &r.IsSystem, &r.CreatedAt, &r.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		acc.Roles = append(acc.Roles, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	prows, err := d.Query(`SELECT DISTINCT rp.permission_key FROM rbac_role_permissions rp
		JOIN rbac_user_roles ur ON ur.role_id=rp.role_id WHERE ur.user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer prows.Close()
	for prows.Next() {
		var k string
		if err := prows.Scan(&k); err != nil {
			return nil, err
		}
		acc.Permissions[k] = true
	}
	return acc, prows.Err()
}

func (d *DB) ListRBACUsers() ([]RBACUser, error) {
	rows, err := d.Query(`SELECT ` + rbacUserCols + ` FROM rbac_users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RBACUser
	for rows.Next() {
		u, err := scanRBACUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

func (d *DB) ListRBACUserRoleIDs(userID string) ([]string, error) {
	rows, err := d.Query(`SELECT role_id FROM rbac_user_roles WHERE user_id=$1 ORDER BY role_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (d *DB) CreateRBACUser(username, displayName, passwordHash string, enabled bool, roleIDs []string) (*RBACUser, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" || passwordHash == "" {
		return nil, errors.New("用户名与密码必填")
	}
	id := newRBACID()
	now := time.Now()
	tx, err := d.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`INSERT INTO rbac_users(id,username,display_name,password_hash,enabled,is_builtin,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,false,$6,$6)`, id, username, strings.TrimSpace(displayName), passwordHash, enabled, now); err != nil {
		return nil, err
	}
	for _, rid := range roleIDs {
		if rid = strings.TrimSpace(rid); rid != "" {
			if _, err := tx.Exec(`INSERT INTO rbac_user_roles(user_id,role_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, id, rid); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetRBACUserByID(id)
}

func (d *DB) UpdateRBACUser(userID, displayName string, enabled *bool, roleIDs *[]string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return errors.New("缺少用户 ID")
	}
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if enabled != nil {
		bump := ""
		if !*enabled {
			bump = ",token_version=token_version+1"
		}
		if _, err := tx.Exec(`UPDATE rbac_users SET display_name=$1,enabled=$2`+bump+`,updated_at=now() WHERE id=$3`, strings.TrimSpace(displayName), *enabled, userID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(`UPDATE rbac_users SET display_name=$1,updated_at=now() WHERE id=$2`, strings.TrimSpace(displayName), userID); err != nil {
			return err
		}
	}
	if roleIDs != nil {
		if _, err := tx.Exec(`DELETE FROM rbac_user_roles WHERE user_id=$1`, userID); err != nil {
			return err
		}
		for _, rid := range *roleIDs {
			if rid = strings.TrimSpace(rid); rid != "" {
				if _, err := tx.Exec(`INSERT INTO rbac_user_roles(user_id,role_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, userID, rid); err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}

func (d *DB) UpdateRBACUserPassword(userID, passwordHash string) error {
	if strings.TrimSpace(userID) == "" || passwordHash == "" {
		return errors.New("参数缺失")
	}
	_, err := d.Exec(`UPDATE rbac_users SET password_hash=$1,token_version=token_version+1,must_change_password=false,updated_at=now() WHERE id=$2`, passwordHash, userID)
	return err
}

func (d *DB) DeleteRBACUser(userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" || userID == "admin" {
		return errors.New("该用户不可删除")
	}
	_, err := d.Exec(`DELETE FROM rbac_users WHERE id=$1 AND is_builtin=false`, userID)
	return err
}

func (d *DB) ListRBACRoles() ([]RBACRole, error) {
	rows, err := d.Query(`SELECT id,name,description,scope,is_system,created_at,updated_at FROM rbac_roles ORDER BY is_system DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RBACRole
	for rows.Next() {
		var r RBACRole
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Scope, &r.IsSystem, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) ListRBACRolePermissionKeys(roleID string) ([]string, error) {
	rows, err := d.Query(`SELECT permission_key FROM rbac_role_permissions WHERE role_id=$1 ORDER BY permission_key`, roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
