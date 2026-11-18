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

// HasCustomExecutorIntentActive 报告指定执行器是否仍有活动中的意图。
// active = proposed/open/running/paused；done/blocked/exhausted/stopped/deleted
// 都不再算。pre_agent 阶段用它决定「执行器阶段是否结束」。
func (s *ExplorationStore) HasCustomExecutorIntentActive(executorKey string) (bool, error) {
	executorKey = strings.TrimSpace(executorKey)
	if executorKey == "" || executorKey == "review" {
		return false, nil
	}
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(
SELECT 1 FROM exploration_nodes
WHERE exploration_id=$1 AND kind='intent'
  AND COALESCE(payload->>'executor','')=$2
  AND state IN ('proposed','open','running','paused'))`, s.expID, executorKey).Scan(&exists)
	return exists, err
}

// HasPendingUncontrolledCustomIntent 是「不受 Planner 控制」意图的单一门控查询：
// kind='intent'、payload.executor 非空且非 'review'、该 executor 对应 Agent 的
// 当前 planner_controlled=false，且 state 尚未放行（done 放行；deleted/物理删除
// 无行即放行）。Agent 缺失按默认 true（不受门控）处理。
//
// 语义：自定义 Agent 的首个 hold_until_done 意图执行期间，planner 不自动唤醒、
// 不改派；只有真实 done 或人工删除才解除。开关按 Agent 当前配置即时生效。
func (s *ExplorationStore) HasPendingUncontrolledCustomIntent() (bool, error) {
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM exploration_nodes
LEFT JOIN agents a ON a.executor_key=exploration_nodes.payload->>'executor'
  OR a.key=exploration_nodes.payload->>'executor'
WHERE exploration_nodes.exploration_id=$1 AND exploration_nodes.kind='intent'
  AND COALESCE(exploration_nodes.payload->>'executor','') NOT IN ('','review')
  AND COALESCE(a.planner_controlled,true)=false
  AND exploration_nodes.state NOT IN ('done','deleted'))`, s.expID).Scan(&exists)
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
