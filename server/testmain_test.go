package server

import (
	"database/sql"
	"os"
	"testing"

	"github.com/3169417664/kaiyang/db"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
)

// TestMain acquires a PostgreSQL advisory lock (7337741002) for the entire
// server test suite so cross-package DELETE cleanup races with db/agent
// packages are avoided when running `go test ./...`.
func TestMain(m *testing.M) {
	dsn, _, err := db.DSN()
	if err != nil {
		os.Exit(m.Run())
	}
	conn, err := sql.Open("pgx", dsn)
	if err != nil || conn.Ping() != nil {
		os.Exit(m.Run())
	}
	defer conn.Close()
	if _, err := conn.Exec(`SELECT pg_advisory_lock(7337741002)`); err != nil {
		os.Exit(m.Run())
	}
	defer conn.Exec(`SELECT pg_advisory_unlock(7337741002)`) //nolint:errcheck

	// 测试用 admin：让旧测试的 signJWT(key) 兼容 token 能解析出 RBAC 画像。
	// 只影响测试库；生产 bootstrap 仍走真实 setup/迁移。
	if d, err := db.Open(dsn); err == nil {
		hash, _ := bcrypt.GenerateFromPassword([]byte("kaiyang-test-password"), bcrypt.MinCost)
		_ = d.BootstrapRBAC(string(hash), false)
		_, _ = d.Exec(`UPDATE rbac_users SET token_version=0 WHERE id='admin'`)
		d.Close()
	}
	os.Exit(m.Run())
}
