package db

import "fmt"

// fusionSchemaVersionKey 记录 KAIYANG 融合迁移的进度。
//
// 注意：这不是替代 schema.sql 的迁移账本。schema.sql 仍然每次启动幂等执行；
// 本文件只承载“需要读旧数据、做一次写回”的迁移。新增迁移必须：
//  1. 用 settings flag 守卫，保证重启/重跑幂等；
//  2. 在 migrateFusion 里显式调用；
//  3. 只做数据迁移，不在这里建表/改 CHECK（那些放 schema.sql）。
const fusionSchemaVersionKey = "fusion.schema_version"

func (d *DB) migrateFusion() error {
	if _, err := d.Exec(`
INSERT INTO settings(key, value) VALUES ($1, '1')
ON CONFLICT (key) DO NOTHING`, fusionSchemaVersionKey); err != nil {
		return fmt.Errorf("mark fusion schema version: %w", err)
	}
	return nil
}
