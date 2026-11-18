package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// FindingInvocation 记录 planner 显式把一条 Finding 派发给自定义 Agent 的调用。
type FindingInvocation struct {
	AgentKey       string    `json:"agent_key"`
	FindingID      int64     `json:"finding_id"`
	TaskID         int64     `json:"task_id"`
	Reason         string    `json:"reason"`
	ConversationID *int64    `json:"conversation_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// RecordFindingInvocation 幂等记录一次派发；已存在时返回首次会话 id。
func (d *DB) RecordFindingInvocation(agentKey string, findingID, taskID int64, reason string) (created bool, convID *int64, err error) {
	tx, err := d.Begin()
	if err != nil {
		return false, nil, err
	}
	defer tx.Rollback()
	var inserted bool
	if err := tx.QueryRow(`INSERT INTO finding_agent_invocations(agent_key,finding_id,task_id,reason)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (agent_key,finding_id) DO NOTHING
		RETURNING true`, agentKey, findingID, taskID, reason).Scan(&inserted); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return false, nil, err
		}
	}
	var cid sql.NullInt64
	if err := tx.QueryRow(`SELECT conversation_id FROM finding_agent_invocations
		WHERE agent_key=$1 AND finding_id=$2`, agentKey, findingID).Scan(&cid); err != nil {
		return false, nil, err
	}
	if cid.Valid {
		v := cid.Int64
		convID = &v
	}
	if err := tx.Commit(); err != nil {
		return false, nil, err
	}
	return inserted, convID, nil
}

func (d *DB) SetFindingInvocationConversation(agentKey string, findingID, convID int64) error {
	_, err := d.Exec(`UPDATE finding_agent_invocations SET conversation_id=$3
		WHERE agent_key=$1 AND finding_id=$2 AND conversation_id IS NULL`, agentKey, findingID, convID)
	return err
}

// PendingFindingInvocations 返回 10 分钟前创建、仍未建会话的调用，供补发扫描。
func (d *DB) PendingFindingInvocations(olderThan time.Duration, limit int) ([]FindingInvocation, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := d.Query(`SELECT agent_key,finding_id,task_id,reason,conversation_id,created_at
		FROM finding_agent_invocations
		WHERE conversation_id IS NULL AND created_at < now() - $1::interval
		ORDER BY created_at LIMIT $2`, olderThan.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FindingInvocation
	for rows.Next() {
		var it FindingInvocation
		var cid sql.NullInt64
		if err := rows.Scan(&it.AgentKey, &it.FindingID, &it.TaskID, &it.Reason, &cid, &it.CreatedAt); err != nil {
			return nil, err
		}
		if cid.Valid {
			v := cid.Int64
			it.ConversationID = &v
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (d *DB) GetTaskDispatchTarget(assetKey string) (taskID, findingID int64, ok bool, err error) {
	var tID, fID int64
	err = d.QueryRow(`SELECT task_id, finding_id FROM task_dispatch_targets WHERE asset_key=$1`, assetKey).Scan(&tID, &fID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	return tID, fID, true, nil
}

func (d *DB) RecordTaskDispatchTarget(assetKey string, taskID, findingID int64) error {
	_, err := d.Exec(`INSERT INTO task_dispatch_targets(asset_key,task_id,finding_id)
		VALUES ($1,$2,$3) ON CONFLICT (asset_key) DO NOTHING`, assetKey, taskID, findingID)
	return err
}

// ErrTaskDispatchTargetExists 表示资产键已被其他任务登记（跨父任务/并发派发）。
var ErrTaskDispatchTargetExists = errors.New("dispatch asset key already registered")

// insertTaskDispatchTargets 在创建任务的同一事务里登记内网派发资产键。
// 唯一键是 asset_key：同一个 IP/根域/域/URL host 无论被多少父任务看到，都只允许
// 一个内网子任务。任一键冲突 → 整个创建回滚并返回 ErrTaskDispatchTargetExists，
// 由调用方返回已有任务 id（先查）或冲突错误（并发）。
func insertTaskDispatchTargets(tx *sql.Tx, taskID, findingID int64, assetKeys []string) error {
	seen := map[string]struct{}{}
	for _, key := range assetKeys {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		res, err := tx.Exec(`INSERT INTO task_dispatch_targets(asset_key,task_id,finding_id)
			VALUES ($1,$2,$3) ON CONFLICT (asset_key) DO NOTHING`, key, taskID, findingID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("%w: %s", ErrTaskDispatchTargetExists, key)
		}
	}
	return nil
}
