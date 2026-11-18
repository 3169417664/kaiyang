package db

import (
	"database/sql"
	"strings"
	"time"
)

// 服务端会话表：把 JWT 的 jti 与用户、token_version 绑定。
//
// 为什么需要它：HS256 密钥一旦被读走（Agent 的 Bash/Read 面、同机进程），
// 攻击者可以离线签发任意 sub/ver 的令牌，签名校验完全形同虚设。要求令牌
// 对应一条真实登录产生的会话记录后，光有密钥不够——还需要创建一行会话
// （需要数据库写权限）或窃取一个仍在有效期内的 jti。

// CreateAuthSession 记录一次成功登录（或续期）产生的令牌。
func (d *DB) CreateAuthSession(jti, userID string, tokenVersion int, expiresAt time.Time, ip, userAgent string) error {
	jti = strings.TrimSpace(jti)
	userID = strings.TrimSpace(userID)
	if jti == "" || userID == "" {
		return sql.ErrNoRows
	}
	_, err := d.Exec(`INSERT INTO auth_sessions(jti,user_id,token_version,expires_at,last_seen,ip,user_agent)
		VALUES ($1,$2,$3,$4,now(),$5,$6)
		ON CONFLICT (jti) DO UPDATE SET expires_at=EXCLUDED.expires_at,last_seen=now(),ip=EXCLUDED.ip,user_agent=EXCLUDED.user_agent`,
		jti, userID, tokenVersion, expiresAt, strings.TrimSpace(ip), truncateRunes(userAgent, 512))
	return err
}

// AuthSessionValid 校验 jti 是否存在、属于该用户、token_version 未过期。
func (d *DB) AuthSessionValid(jti, userID string, tokenVersion int) (bool, error) {
	jti = strings.TrimSpace(jti)
	userID = strings.TrimSpace(userID)
	if jti == "" || userID == "" {
		return false, nil
	}
	var n int
	err := d.QueryRow(`SELECT COUNT(*) FROM auth_sessions
		WHERE jti=$1 AND user_id=$2 AND token_version=$3 AND expires_at > now()`, jti, userID, tokenVersion).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// AuthSessionRenewable 限制滑动续期频率：同一 jti 最多每小时续期一次，
// 避免 Bearer 客户端在到期前半段每个请求都新建一条会话记录。
func (d *DB) AuthSessionRenewable(jti string) bool {
	if strings.TrimSpace(jti) == "" {
		return false
	}
	var ok bool
	if err := d.QueryRow(`SELECT COALESCE(last_seen < now() - interval '1 hour', false)
		FROM auth_sessions WHERE jti=$1 AND expires_at > now()`, jti).Scan(&ok); err != nil {
		return false
	}
	return ok
}

func (d *DB) TouchAuthSession(jti string) {
	if strings.TrimSpace(jti) == "" {
		return
	}
	_, _ = d.Exec(`UPDATE auth_sessions SET last_seen=now() WHERE jti=$1`, jti)
}

func (d *DB) DeleteAuthSession(jti string) {
	if strings.TrimSpace(jti) == "" {
		return
	}
	_, _ = d.Exec(`DELETE FROM auth_sessions WHERE jti=$1`, jti)
}

// DeleteAuthSessionsForUser 在停用账号/改密时清掉该用户全部在线会话。
func (d *DB) DeleteAuthSessionsForUser(userID string) {
	if strings.TrimSpace(userID) == "" {
		return
	}
	_, _ = d.Exec(`DELETE FROM auth_sessions WHERE user_id=$1`, userID)
}

// PruneExpiredAuthSessions 清理过期记录，避免表无限增长。
func (d *DB) PruneExpiredAuthSessions() {
	_, _ = d.Exec(`DELETE FROM auth_sessions WHERE expires_at < now() - interval '1 day'`)
}

// RBACUserIsAdminByUsername 供任务/会话运行期判断创建者是否为管理员。
// 空用户名视为系统/历史数据（管理员语义），避免旧任务失去能力。
func (d *DB) RBACUserIsAdminByUsername(username string) (bool, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return true, nil
	}
	var n int
	err := d.QueryRow(`SELECT COUNT(*) FROM rbac_users u
		JOIN rbac_user_roles ur ON ur.user_id=u.id
		JOIN rbac_roles r ON r.id=ur.role_id
		WHERE u.username=$1 AND u.enabled=true AND r.id=$2`, username, RBACRoleAdmin).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
