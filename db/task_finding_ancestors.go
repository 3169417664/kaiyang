package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
)

// 内网派发子任务时的 Finding 祖先子图复制。
//
// 当 spawn_task 携带 finding_id 创建子任务时，子任务的探索图不应该只剩一个
// origin fact——它需要继承父任务里「这个 Finding 是怎么被发现的」这一条推理链，
// 否则内网 planner 开局没有任何前置事实，只能重复侦察。
//
// 复制范围严格限定为：从该 Finding 节点出发，沿 yields/derived_from 反向可达
// （即祖先方向）的 intent/fact/finding 节点，以及这些节点之间的同类型边。
// 明确不复制：
//   - 触发派发的 Finding 本身（"父 Finding"）——它是父任务的漏洞记录，子任务
//     会通过 task_dispatch_targets / append_finding_to_task 获得自己的上下文；
//   - 父任务的 origin fact——复制它会在子任务里制造第二个 state='origin' 的
//     根事实，破坏「每个 exploration 只有一个 root fact」的不变量。被复制子图
//     的根节点改挂到子任务自己的 origin fact 上；
//   - 无关分支——兄弟 Finding / 下游 fact 不在反向可达集合内，天然排除。
//
// 复制后的 intent 一律固定为 done：它们是已经发生过的历史事实，不能被调度器
// 重新执行。fact/finding 保留原状态（confirmed/dismissed）。
//
// 幂等：每个复制节点在 payload 里写入 imported_from_node_id。同一目标
// exploration 重复调用时复用已复制节点，不产生重复子图，边用 ON CONFLICT
// DO NOTHING 兜底。

const importedFromNodeField = "imported_from_node_id"

// copyFindingAncestorsTx copies the ancestor subgraph of findings.id to dstExpID.
// It must run inside the same transaction that creates the destination task so a
// copy failure rolls the task back instead of leaving a half-inherited graph.
// Findings without a live exploration node (standalone/manual records) are a
// no-op: there is no lineage to copy.
func copyFindingAncestorsTx(tx *sql.Tx, findingID, dstExpID, dstOriginID int64) error {
	if findingID <= 0 || dstExpID <= 0 {
		return nil
	}

	// 解析触发的 Finding：它必须同时有 findings 行、所属任务和仍存活的探索节点。
	var srcExpID, srcNodeID int64
	err := tx.QueryRow(`
SELECT t.exploration_id, n.id
FROM findings f
JOIN tasks t ON t.id = f.task_id
JOIN exploration_nodes n ON n.id = f.node_id AND n.exploration_id = t.exploration_id
WHERE f.id = $1 AND n.kind = 'finding'`, findingID).Scan(&srcExpID, &srcNodeID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read finding ancestor root: %w", err)
	}

	// 反向可达祖先集。递归项只穿过可复制的 reasoning 节点，因此：
	//   - deleted 的 intent 与其上游分支不会被拖进来；
	//   - 父任务 origin fact 不会被拖进来；
	//   - 目标/提示等下游或旁支种类不会被穿过。
	type sourceNode struct {
		id       int64
		kind     string
		payload  []byte
		priority int
		state    string
		origin   string
	}
	nodes := make([]sourceNode, 0, 16)
	rows, err := tx.Query(`
WITH RECURSIVE anc(id) AS (
    SELECT $2::bigint
  UNION
    SELECT e.src_id
    FROM exploration_edges e
    JOIN anc ON e.dst_id = anc.id
    JOIN exploration_nodes sn ON sn.id = e.src_id AND sn.exploration_id = e.exploration_id
    WHERE e.exploration_id = $1
      AND e.rel IN ($3, $4)
      AND sn.kind IN ('intent','fact','finding')
      AND NOT (sn.kind = 'intent' AND sn.state = 'deleted')
      AND NOT (sn.kind = 'fact' AND sn.state = 'origin')
)
SELECT n.id, n.kind, n.payload, n.priority, n.state, COALESCE(n.origin, '')
FROM exploration_nodes n
WHERE n.exploration_id = $1
  AND n.id IN (SELECT id FROM anc)
  AND n.id <> $2
ORDER BY n.id`, srcExpID, srcNodeID, RelYields, RelDerivedFrom)
	if err != nil {
		return fmt.Errorf("read finding ancestors: %w", err)
	}
	for rows.Next() {
		var n sourceNode
		if err := rows.Scan(&n.id, &n.kind, &n.payload, &n.priority, &n.state, &n.origin); err != nil {
			rows.Close()
			return fmt.Errorf("scan finding ancestor: %w", err)
		}
		nodes = append(nodes, n)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(nodes) == 0 {
		return nil
	}

	// 幂等：目标 exploration 里已从同一源节点复制过的节点直接复用。
	imported := map[int64]int64{}
	existing, err := tx.Query(`
SELECT payload->>'`+importedFromNodeField+`', id
FROM exploration_nodes
WHERE exploration_id = $1 AND payload ? $2`, dstExpID, importedFromNodeField)
	if err != nil {
		return fmt.Errorf("read imported ancestor nodes: %w", err)
	}
	for existing.Next() {
		var raw string
		var dstID int64
		if err := existing.Scan(&raw, &dstID); err != nil {
			existing.Close()
			return fmt.Errorf("scan imported ancestor node: %w", err)
		}
		srcID, perr := strconv.ParseInt(raw, 10, 64)
		if perr == nil && srcID > 0 {
			imported[srcID] = dstID
		}
	}
	if err := existing.Close(); err != nil {
		return err
	}
	if err := existing.Err(); err != nil {
		return err
	}

	type nodePair struct{ src, dst int64 }
	pairs := make([]nodePair, 0, len(nodes))
	for _, sn := range nodes {
		if dstID, ok := imported[sn.id]; ok {
			pairs = append(pairs, nodePair{src: sn.id, dst: dstID})
			continue
		}
		payload := map[string]any{}
		if len(sn.payload) > 0 {
			if err := json.Unmarshal(sn.payload, &payload); err != nil {
				return fmt.Errorf("decode ancestor node %d payload: %w", sn.id, err)
			}
		}
		if payload == nil {
			payload = map[string]any{}
		}
		// 来源标记既用于幂等，也让 planner/审计能看出这是跨任务导入的历史节点。
		payload[importedFromNodeField] = sn.id
		payload["imported_from_exploration_id"] = srcExpID
		payload["imported_from_finding_id"] = findingID
		raw, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode ancestor node %d payload: %w", sn.id, err)
		}
		state := sn.state
		if sn.kind == KindIntent {
			state = "done" // 历史事实：固定终态，调度器不会再执行
		}
		var dstID int64
		if err := tx.QueryRow(`
INSERT INTO exploration_nodes(exploration_id, kind, payload, priority, state, origin)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			dstExpID, sn.kind, string(raw), sn.priority, state, sn.origin).Scan(&dstID); err != nil {
			return fmt.Errorf("copy ancestor node %d: %w", sn.id, err)
		}
		// 资产是全局共享的，锚点只需把 node_id 重映射到副本；资产 id 原样保留。
		if _, err := tx.Exec(`
INSERT INTO exploration_anchors(node_id, asset_id)
SELECT $1, asset_id FROM exploration_anchors WHERE node_id = $2
ON CONFLICT DO NOTHING`, dstID, sn.id); err != nil {
			return fmt.Errorf("copy ancestor node %d anchors: %w", sn.id, err)
		}
		imported[sn.id] = dstID
		pairs = append(pairs, nodePair{src: sn.id, dst: dstID})
	}

	// 只复制副本集合内部的 yields/derived_from 边，避免把断头边带进子任务。
	srcIDs := make([]int64, 0, len(pairs))
	for _, p := range pairs {
		srcIDs = append(srcIDs, p.src)
	}
	edges, err := tx.Query(`
SELECT src_id, rel, dst_id
FROM exploration_edges
WHERE exploration_id = $1
  AND src_id = ANY($2::bigint[])
  AND dst_id = ANY($2::bigint[])
  AND rel IN ($3, $4)`, srcExpID, srcIDs, RelYields, RelDerivedFrom)
	if err != nil {
		return fmt.Errorf("read ancestor edges: %w", err)
	}
	type sourceEdge struct {
		from int64
		rel  string
		to   int64
	}
	copiedEdges := make([]sourceEdge, 0, len(pairs))
	for edges.Next() {
		var e sourceEdge
		if err := edges.Scan(&e.from, &e.rel, &e.to); err != nil {
			edges.Close()
			return fmt.Errorf("scan ancestor edge: %w", err)
		}
		copiedEdges = append(copiedEdges, e)
	}
	if err := edges.Close(); err != nil {
		return err
	}
	if err := edges.Err(); err != nil {
		return err
	}

	hasIncoming := map[int64]bool{}
	for _, e := range copiedEdges {
		from, okFrom := imported[e.from]
		to, okTo := imported[e.to]
		if !okFrom || !okTo || from == to {
			continue
		}
		if _, err := tx.Exec(`
INSERT INTO exploration_edges(exploration_id, src_id, rel, dst_id)
VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, dstExpID, from, e.rel, to); err != nil {
			return fmt.Errorf("copy ancestor edge %d-%s->%d: %w", e.from, e.rel, e.to, err)
		}
		hasIncoming[to] = true
	}

	// 子图根节点（没有副本内父节点）挂到子任务自己的 origin fact 上，保持
	// 「每个 intent 都能回溯到任务根」的血缘不变量。
	if dstOriginID > 0 {
		for _, p := range pairs {
			if hasIncoming[p.dst] || p.dst == dstOriginID {
				continue
			}
			if _, err := tx.Exec(`
INSERT INTO exploration_edges(exploration_id, src_id, rel, dst_id)
VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, dstExpID, dstOriginID, RelDerivedFrom, p.dst); err != nil {
				return fmt.Errorf("anchor imported ancestor %d to origin: %w", p.dst, err)
			}
		}
	}
	return nil
}
