package server

import (
	"net/http"
)

// 会话吊销。KAIYANG 的令牌本身已经绑定服务端 auth_sessions（jti），所以
// 「吊销」就是删除会话记录：比上游的 settings 时间戳哨兵更直接，也不依赖
// 缓存 TTL。
//
// 路由在 /api/auth/* 下，requireAuth 不拦截；这里自行校验 JWT 与会话。

func (s *Server) currentClaims(w http.ResponseWriter, r *http.Request) (*authClaims, bool) {
	claims, ok := parseJWT(extractToken(r), s.currentJWTKey())
	if !ok || claims.Subject == "" || claims.ID == "" {
		clearAuthCookie(w, r)
		writeErr(w, 401, "登录状态已失效，请重新登录")
		return nil, false
	}
	if s.m == nil || s.m.pg == nil {
		writeErr(w, 503, "授权服务暂时不可用")
		return nil, false
	}
	active, err := s.m.pg.AuthSessionValid(claims.ID, claims.Subject, claims.Ver)
	if err != nil {
		writeInternalError(w, "auth revoke session check", err)
		return nil, false
	}
	if !active {
		clearAuthCookie(w, r)
		writeErr(w, 401, "登录状态已失效，请重新登录")
		return nil, false
	}
	return claims, true
}

// POST /api/auth/revoke-current — 只失效当前设备的会话。
func (s *Server) authRevokeCurrent(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(w, r)
	if !ok {
		return
	}
	s.m.pg.DeleteAuthSession(claims.ID)
	clearAuthCookie(w, r)
	writeJSON(w, 200, map[string]any{"ok": true, "revoked": "current"})
}

// POST /api/auth/revoke-all — 失效该用户全部会话（含当前），需要显式确认。
func (s *Server) authRevokeAll(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(w, r)
	if !ok {
		return
	}
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if !decodeJSON(w, r, &req, maxAuthJSONBodyBytes) {
		return
	}
	if !req.Confirm {
		writeErr(w, 400, "需要显式确认：请求体传 {\"confirm\":true}")
		return
	}
	s.m.pg.DeleteAuthSessionsForUser(claims.Subject)
	clearAuthCookie(w, r)
	s.m.pg.RecordAudit(claims.Username, "auth", "revoke_all_sessions", "success", clientIP(r), "吊销全部会话", nil)
	writeJSON(w, 200, map[string]any{"ok": true, "revoked": "all"})
}
