package server

import (
	"time"

	"github.com/3169417664/kaiyang/db"
)

// signJWT 兼容旧测试：TestMain 已确保测试库存在 admin（token_version=0），
// 这里签发一张 admin token，并落一条服务端会话记录（requireAuth 会校验 jti），
// 供走完整 Handler/requireAuth 的测试使用。
func signJWT(key []byte) (string, error) {
	jti, err := randomTokenID()
	if err != nil {
		return "", err
	}
	// TTL 必须大于 sessionTTL/2，否则第一次请求就触发滑动续期并把测试仍在使用
	// 的旧 jti 删掉，后续请求会 401。
	tok, err := signJWTWithID(key, "admin", "admin", 0, jti, 24*time.Hour)
	if err != nil {
		return "", err
	}
	expires := time.Now().Add(24 * time.Hour)
	// 优先复用 New 注入的连接；全量测试里前一个用例可能已把该连接关掉，
	// 因此失败时按 DSN 自建一次。
	if sink := authSessionTestSink; sink != nil {
		if err := sink.CreateAuthSession(jti, "admin", 0, expires, "test", "test"); err == nil {
			return tok, nil
		}
	}
	dsn, _, derr := db.DSN()
	if derr != nil {
		return "", derr
	}
	d, derr := db.Open(dsn)
	if derr != nil {
		return "", derr
	}
	defer d.Close()
	if err := d.CreateAuthSession(jti, "admin", 0, expires, "test", "test"); err != nil {
		return "", err
	}
	return tok, nil
}
