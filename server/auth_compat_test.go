package server

import "time"

// signJWT 兼容旧测试：TestMain 已确保测试库存在 admin（token_version=0），
// 这里签发一张 admin token，供走完整 Handler/requireAuth 的测试使用。
func signJWT(key []byte) (string, error) {
	return signJWTTTL(key, "admin", "admin", 0, time.Hour)
}
