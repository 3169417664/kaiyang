package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/3169417664/kaiyang/db"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	jwtKeyFilename = "jwt.key"
	authPassKey    = "auth.password_hash" // 旧单管理员密码，仅用于首启迁移
	// 480 分钟滑动续期；空闲超过该时长需重新登录。设置键可覆盖。
	defaultSessionTTL = 480 * time.Minute
	sessionTTLKey     = "auth.session_ttl_minutes"
	keyChars          = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	minPasswordRunes = 8
	maxPasswordBytes = 72 // bcrypt 硬限制

	errDataSourceUnavailable = "数据源暂时不可用，请稍后重试"
)

type ctxKey int

const (
	accessCtxKey ctxKey = 1
	adminRunCtxKey ctxKey = 2
)

func ctxIsAdmin(ctx context.Context) bool {
	v, _ := ctx.Value(adminRunCtxKey).(bool)
	return v
}

// authClaims 携带用户 id(Subject) + 用户名 + token_version。
type authClaims struct {
	Username string `json:"usr"`
	Ver      int    `json:"ver"`
	jwt.RegisteredClaims
}

// CurrentAccess 取当前请求的 RBAC 授权画像。
func CurrentAccess(r *http.Request) *db.RBACAccess {
	acc, _ := r.Context().Value(accessCtxKey).(*db.RBACAccess)
	return acc
}

func (s *Server) sessionTTL() time.Duration {
	if s.m != nil && s.m.pg != nil {
		if m := s.m.pg.GetInt(sessionTTLKey, 0); m > 0 {
			return time.Duration(m) * time.Minute
		}
	}
	return defaultSessionTTL
}

func validatePassword(pw string) string {
	if utf8.RuneCountInString(pw) < minPasswordRunes {
		return fmt.Sprintf("密码长度至少 %d 位", minPasswordRunes)
	}
	if len(pw) > maxPasswordBytes {
		return fmt.Sprintf("密码长度不能超过 %d 字节", maxPasswordBytes)
	}
	return ""
}

// loadOrCreateJWTKey 读取/生成 32 字节 HS256 签名密钥。
func loadOrCreateJWTKey(keyDir, dataDir string) ([]byte, error) {
	path := filepath.Join(keyDir, jwtKeyFilename)
	if legacy := filepath.Join(dataDir, jwtKeyFilename); legacy != path {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if data, rerr := os.ReadFile(legacy); rerr == nil {
				if werr := os.WriteFile(path, data, 0o600); werr == nil {
					_ = os.Remove(legacy)
					log.Printf("[auth] JWT key 已从 %s 迁移到 %s", legacy, path)
				}
			}
		}
	}
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) >= 32 {
		return []byte(strings.TrimSpace(string(data))), nil
	}
	buf := make([]byte, 32)
	for i := range buf {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(keyChars))))
		if err != nil {
			return nil, fmt.Errorf("generate jwt key: %w", err)
		}
		buf[i] = keyChars[n.Int64()]
	}
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		return nil, fmt.Errorf("write jwt key: %w", err)
	}
	log.Printf("[auth] 新 JWT key 已写入 %s", path)
	return buf, nil
}

func (s *Server) currentJWTKey() []byte {
	if v, ok := s.jwtKey.Load().([]byte); ok {
		return v
	}
	return nil
}

// rotateJWTKey 仅供维护场景使用；日常改密只递增该用户的 token_version，不踢全员。
func (s *Server) rotateJWTKey() error {
	buf := make([]byte, 32)
	for i := range buf {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(keyChars))))
		if err != nil {
			return fmt.Errorf("generate jwt key: %w", err)
		}
		buf[i] = keyChars[n.Int64()]
	}
	if err := os.WriteFile(filepath.Join(s.keyDir, jwtKeyFilename), buf, 0o600); err != nil {
		return fmt.Errorf("write jwt key: %w", err)
	}
	s.jwtKey.Store(buf)
	log.Printf("[auth] JWT key 已轮换，全部旧会话失效")
	return nil
}

func signJWTTTL(key []byte, userID, username string, ver int, ttl time.Duration) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, authClaims{
		Username: username,
		Ver:      ver,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}).SignedString(key)
}

func parseJWT(tokenStr string, key []byte) (*authClaims, bool) {
	var c authClaims
	t, err := jwt.ParseWithClaims(tokenStr, &c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return key, nil
	})
	if err != nil || !t.Valid {
		return nil, false
	}
	return &c, true
}

// sseQueryTokenPaths 是唯一允许 ?token= 的 SSE 端点白名单。
var sseQueryTokenPaths = map[string]bool{
	"/api/exploration/activity/stream": true,
	"/api/logs/stream":                 true,
	"/api/update/stream":               true,
}

func queryTokenAllowed(r *http.Request) bool {
	if sseQueryTokenPaths[r.URL.Path] {
		return true
	}
	return strings.HasPrefix(r.URL.Path, "/api/side-questions/") && strings.HasSuffix(r.URL.Path, "/events")
}

func extractToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	for _, name := range []string{"kaiyang_token", "pojun_token"} {
		if c, err := r.Cookie(name); err == nil && c.Value != "" {
			return c.Value
		}
	}
	if queryTokenAllowed(r) {
		return r.URL.Query().Get("token")
	}
	return ""
}

func accessIsAdmin(acc *db.RBACAccess) bool {
	if acc == nil {
		return false
	}
	for _, role := range acc.Roles {
		if role.ID == db.RBACRoleAdmin {
			return true
		}
	}
	return false
}

// requireAuth 校验 JWT、解析 RBAC、注入 context，并执行路由级权限。
func (s *Server) requireAuth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/api/auth/") || p == "/api/health" {
			h.ServeHTTP(w, r)
			return
		}
		claims, ok := parseJWT(extractToken(r), s.currentJWTKey())
		if !ok {
			writeErr(w, 401, "未授权或 token 已过期")
			return
		}
		if strings.TrimSpace(claims.Subject) == "" {
			writeErr(w, 401, "登录状态已失效，请重新登录")
			return
		}
		if s.m == nil || s.m.pg == nil {
			writeErr(w, 503, "授权服务暂时不可用")
			return
		}
		acc, err := s.m.pg.ResolveRBACAccess(claims.Subject)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeErr(w, 401, "登录状态已失效，请重新登录")
			} else {
				log.Printf("[auth] 授权查询失败: %v", err)
				writeErr(w, 503, "授权服务暂时不可用")
			}
			return
		}
		if acc == nil {
			writeErr(w, 503, "授权服务暂时不可用")
			return
		}
		if !acc.User.Enabled {
			writeErr(w, 403, "账号已停用")
			return
		}
		if claims.Ver != acc.User.TokenVersion {
			writeErr(w, 401, "登录状态已失效，请重新登录")
			return
		}
		if acc.User.MustChangePassword {
			writeErr(w, 403, "首次登录需先修改密码")
			return
		}
		// 滑动续期：token 走过半程仍在活跃使用时补发新 token。
		if claims.ExpiresAt != nil {
			ttl := s.sessionTTL()
			if rem := time.Until(claims.ExpiresAt.Time); rem > 0 && rem < ttl/2 {
				if fresh, err := signJWTTTL(s.currentJWTKey(), claims.Subject, claims.Username, acc.User.TokenVersion, ttl); err == nil {
					w.Header().Set("Access-Control-Expose-Headers", "X-Refresh-Token")
					w.Header().Set("X-Refresh-Token", fresh)
				}
			}
		}
		admin := accessIsAdmin(acc)
		ctx := context.WithValue(r.Context(), accessCtxKey, acc)
		ctx = context.WithValue(ctx, adminRunCtxKey, admin)
		r = r.WithContext(ctx)

		if s.m.pg.GetBool("rbac.enforce", true) {
			perm, mapped := permissionForRequest(r.Method, r.URL.Path)
			if !mapped {
				// deny-by-default：未映射路由仅 admin 可访问。
				if !admin {
					writeErr(w, 403, "权限不足：未映射路由")
					return
				}
			} else if perm != "" && !acc.Permissions[perm] {
				writeErr(w, 403, "权限不足："+perm)
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

// bootstrapRBAC 启动时把旧单管理员密码迁移成 admin，并播种三角色/权限。
func (s *Server) bootstrapRBAC() {
	if s.m == nil || s.m.pg == nil {
		return
	}
	legacyHash, _, _ := s.m.pg.GetSetting(authPassKey)
	mustChange := true
	if os.Getenv("POJUN_FUSION_NO_FORCE_PASSWORD_CHANGE") == "1" || os.Getenv("KAIYANG_NO_FORCE_PASSWORD_CHANGE") == "1" {
		mustChange = false
	}
	if err := s.m.pg.BootstrapRBAC(legacyHash, mustChange); err != nil {
		log.Printf("[rbac] bootstrap 失败: %v", err)
		return
	}
	if strings.TrimSpace(legacyHash) != "" {
		log.Printf("[rbac] 多用户权限就绪（旧密码已迁移为 admin）")
	} else {
		log.Printf("[rbac] 多用户权限就绪")
	}
}

// GET /api/auth/status
func (s *Server) authStatus(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	needs, err := pg.RBACNeedsBootstrap()
	if err != nil {
		// 读失败必须 fail-closed：不能回答 initialized:false，否则前端会把用户
		// 送到 /setup 覆盖已有管理员密码。
		writeErr(w, 503, errDataSourceUnavailable)
		return
	}
	writeJSON(w, 200, map[string]any{"initialized": !needs})
}

// POST /api/auth/init — 首启创建 admin。
func (s *Server) authInit(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	ip := clientIP(r)
	needs, err := pg.RBACNeedsBootstrap()
	if err != nil {
		pg.RecordAudit("", "auth", "init", "fail", ip, "初始化守卫查询失败（已拒绝）", nil)
		writeErr(w, 503, "服务暂时不可用")
		return
	}
	if !needs {
		pg.RecordAudit("", "auth", "init", "fail", ip, "平台已初始化，拒绝重复设置密码", nil)
		writeErr(w, 403, "已初始化")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req, maxAuthJSONBodyBytes) {
		return
	}
	if msg := validatePassword(req.Password); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, "密码加密失败")
		return
	}
	if err := pg.BootstrapRBAC(string(hash), false); err != nil {
		writeErr(w, 500, "初始化失败: "+err.Error())
		return
	}
	_ = pg.SetSetting(authPassKey, string(hash)) // 兼容回滚
	u, err := pg.GetRBACUserByID("admin")
	if err != nil {
		writeErr(w, 500, "管理员创建失败")
		return
	}
	tok, err := signJWTTTL(s.currentJWTKey(), u.ID, u.Username, u.TokenVersion, s.sessionTTL())
	if err != nil {
		writeErr(w, 500, "token 生成失败")
		return
	}
	pg.RecordAudit("admin", "auth", "init", "success", ip, "首启初始化管理员密码", nil)
	writeJSON(w, 200, map[string]any{"token": tok})
}

// POST /api/auth/change-password
func (s *Server) authChangePassword(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	claims, ok := parseJWT(extractToken(r), s.currentJWTKey())
	if !ok {
		writeErr(w, 401, "未授权")
		return
	}
	u, err := pg.GetRBACUserByID(claims.Subject)
	if err != nil {
		writeErr(w, 401, "用户不存在")
		return
	}
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if !decodeJSON(w, r, &req, maxAuthJSONBodyBytes) {
		return
	}
	if msg := validatePassword(req.NewPassword); msg != "" {
		writeErr(w, 400, msg)
		return
	}
	ip := clientIP(r)
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.OldPassword)); err != nil {
		pg.RecordAudit(u.Username, "auth", "change_password", "fail", ip, "当前密码错误", nil)
		writeErr(w, 401, "当前密码错误")
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, "密码加密失败")
		return
	}
	if err := pg.UpdateRBACUserPassword(u.ID, string(newHash)); err != nil {
		writeErr(w, 500, "保存失败: "+err.Error())
		return
	}
	pg.RecordAudit(u.Username, "auth", "change_password", "success", ip, "修改密码成功", nil)
	fresh := ""
	if nu, e := pg.GetRBACUserByID(u.ID); e == nil && nu != nil {
		fresh, _ = signJWTTTL(s.currentJWTKey(), nu.ID, nu.Username, nu.TokenVersion, s.sessionTTL())
	}
	writeJSON(w, 200, map[string]any{"ok": true, "token": fresh})
}

// POST /api/auth/login
func (s *Server) authLogin(w http.ResponseWriter, r *http.Request) {
	pg := s.pg(w)
	if pg == nil {
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req, maxAuthJSONBodyBytes) {
		return
	}
	ip := clientIP(r)
	uname := strings.ToLower(strings.TrimSpace(req.Username))
	if locked, retry := s.loginLimiter.locked(ip, uname); locked {
		w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())))
		pg.RecordAudit(uname, "auth", "login_locked", "fail", ip,
			fmt.Sprintf("触发暴力破解锁定，剩余冷却 %ds", int(retry.Seconds())), nil)
		writeErr(w, 429, fmt.Sprintf("尝试过于频繁，账号/来源已临时锁定，请 %d 秒后再试", int(retry.Seconds())))
		return
	}
	u, err := pg.GetRBACUserByUsername(uname)
	if err != nil || u == nil || !u.Enabled {
		reason := "用户名或密码错误"
		if u != nil && !u.Enabled {
			reason = "账号已停用"
		}
		s.registerLoginFailure(pg, ip, uname)
		pg.RecordAudit(uname, "auth", "login_failed", "fail", ip, reason, nil)
		writeErr(w, 401, reason)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		s.registerLoginFailure(pg, ip, u.Username)
		pg.RecordAudit(u.Username, "auth", "login_failed", "fail", ip, "密码错误", nil)
		writeErr(w, 401, "用户名或密码错误")
		return
	}
	tok, err := signJWTTTL(s.currentJWTKey(), u.ID, u.Username, u.TokenVersion, s.sessionTTL())
	if err != nil {
		writeErr(w, 500, "token 生成失败")
		return
	}
	s.loginLimiter.reset(ip, u.Username)
	pg.RecordAudit(u.Username, "auth", "login", "success", ip, "登录成功", nil)
	log.Printf("[auth] 登录成功 用户=%q ip=%s", u.Username, ip)
	writeJSON(w, 200, map[string]any{"token": tok, "must_change_password": u.MustChangePassword})
}

func (s *Server) registerLoginFailure(pg *db.DB, ip, username string) {
	if s.loginLimiter.fail(ip, username, s.loginPolicy()) {
		pg.RecordAudit(username, "auth", "login_locked", "fail", ip, "失败次数超阈值，已临时锁定", nil)
		log.Printf("[auth] 暴力破解锁定 用户=%q ip=%s", username, ip)
	}
}

// clientIP 只在直连来源是可信代理时采信转发头，避免伪造 XFF 绕过锁定。
func clientIP(r *http.Request) string {
	remote := remoteAddrIP(r.RemoteAddr)
	if isTrustedProxyIP(remote) {
		if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
			return xr
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	return remote
}

func remoteAddrIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr)); err == nil {
		return host
	}
	return strings.Trim(strings.TrimSpace(remoteAddr), "[]")
}

var trustedProxyNets = loopbackProxyNets()

func loopbackProxyNets() []*net.IPNet {
	var nets []*net.IPNet
	for _, c := range []string{"127.0.0.0/8", "::1/128"} {
		if _, n, err := net.ParseCIDR(c); err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}

func SetTrustedProxies(list string) {
	nets := loopbackProxyNets()
	for _, f := range strings.FieldsFunc(list, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
		if f == "" {
			continue
		}
		if !strings.Contains(f, "/") {
			if ip := net.ParseIP(f); ip != nil {
				if ip.To4() != nil {
					f += "/32"
				} else {
					f += "/128"
				}
			}
		}
		if _, n, err := net.ParseCIDR(f); err == nil {
			nets = append(nets, n)
		}
	}
	trustedProxyNets = nets
}

func isTrustedProxyIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, n := range trustedProxyNets {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}
