package db

import (
	"database/sql"
	"errors"
	"fmt"
)

// KAIYANG 数据模型约定：findings.id 是跨报告/富化/证据/流量的规范键。
// exploration_nodes.id（node_id）只作为兼容/回溯字段，不得作为新表主键。
//
// 旧数据里存在 node_id 为空、或同一 node 被合并到另一条 finding 的情况，
// 因此这里提供双向解析，并在找不到时明确返回 ok=false，而不是猜。

// FindingNodeIDByID 返回 findings.id 对应的探索图节点 id；node_id 为空时返回 nil。
// node_id → findings.id 的解析复用 finding_traffic.go 里已有的 FindingIDByNodeID。
func (d *DB) FindingNodeIDByID(findingID int64) (*int64, error) {
	if findingID <= 0 {
		return nil, nil
	}
	var nodeID sql.NullInt64
	err := d.QueryRow(`SELECT node_id FROM findings WHERE id = $1`, findingID).Scan(&nodeID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding node for %d: %w", findingID, err)
	}
	if !nodeID.Valid {
		return nil, nil
	}
	v := nodeID.Int64
	return &v, nil
}
