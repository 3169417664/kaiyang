package db

import "strings"

// 自定义执行器领取原语。executor_key 来自 agents.executor_key / tasks.pre_agent。
// 默认 worker 的 Frontier 已排除带 executor 的 open 意图，避免两条执行路径抢单。

// DispatchFrontier 返回可被指定自定义执行器领取的意图：
//   - state=open 且 payload.executor=key（审核批准后转正的执行器意图）
//   - state=proposed 且 payload.executor=key（种子意图/pre_agent 尚未执行）
func (s *ExplorationStore) DispatchFrontier(limit int, executorKey string) ([]*Node, error) {
	if limit <= 0 {
		limit = 10
	}
	executorKey = strings.TrimSpace(executorKey)
	if executorKey == "" {
		return nil, nil
	}
	rows, err := s.db.Query(`SELECT `+nodeCols+` FROM exploration_nodes
WHERE exploration_id=$1 AND kind='intent'
  AND state IN ('open','proposed')
  AND payload->>'executor'=$2
ORDER BY priority DESC, id ASC LIMIT $3`, s.expID, executorKey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNodes(rows)
}

// ClaimExecutorIntent 由自定义执行器 CAS 领取：open/proposed → running。
func (s *ExplorationStore) ClaimExecutorIntent(id int64, executorKey, owner string) (bool, error) {
	executorKey = strings.TrimSpace(executorKey)
	if executorKey == "" {
		return false, nil
	}
	res, err := s.db.Exec(`UPDATE exploration_nodes
SET state='running', owner=$1
WHERE id=$2 AND exploration_id=$3 AND kind='intent'
  AND state IN ('open','proposed') AND payload->>'executor'=$4`, owner, id, s.expID, executorKey)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// HasCustomExecutorIntentActive 报告指定执行器是否还有 running 的意图。
func (s *ExplorationStore) HasCustomExecutorIntentActive(executorKey string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(
SELECT 1 FROM exploration_nodes
WHERE exploration_id=$1 AND kind='intent' AND state='running' AND owner=$2)`,
		s.expID, "exec:"+strings.TrimSpace(executorKey)).Scan(&exists)
	return exists, err
}

// FinishExecutorIntent 由自定义执行器在 run 结束后落终态（done/blocked）。
func (s *ExplorationStore) FinishExecutorIntent(id int64, state, reason string) error {
	if state != "done" && state != "blocked" {
		state = "blocked"
	}
	_, err := s.db.Exec(`UPDATE exploration_nodes SET state=$2, blocked_reason=NULLIF($3,''), completed_at=now()
		WHERE id=$1 AND exploration_id=$4 AND kind='intent'`, id, state, reason, s.expID)
	return err
}
