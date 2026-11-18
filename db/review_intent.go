package db

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// 意图审核（proposed 待审池）的数据库原语。
//
// 状态机：
//   proposed + executor=review + review_status=pending
//     approve → state=open，executor=pending_executor（或删除占位）
//     reject  → state=proposed，review_status=rejected
//     revise  → review_status=pending，content_version+1
//   所有处置都带 expected_version CAS，冲突返回 ErrStaleVersion。

var (
	ErrNotPendingReview = errors.New("intent is not pending review")
	ErrStaleVersion     = errors.New("intent content version changed")
)

const proposedPendingWhere = `kind='intent' AND state='proposed'
  AND payload->>'executor'='review' AND COALESCE(payload->>'review_status','pending')='pending'`

func (s *ExplorationStore) HasProposedIntent() (bool, error) {
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(
		SELECT 1 FROM exploration_nodes WHERE exploration_id=$1 AND `+proposedPendingWhere+`)`, s.expID).Scan(&exists)
	return exists, err
}

func (s *ExplorationStore) ProposedIntents(limit int) ([]*Node, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT `+nodeCols+` FROM exploration_nodes
		WHERE exploration_id=$1 AND `+proposedPendingWhere+`
		ORDER BY priority DESC, id ASC LIMIT $2`, s.expID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNodes(rows)
}

func (s *ExplorationStore) RejectedIntents(limit int) ([]*Node, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT `+nodeCols+` FROM exploration_nodes
		WHERE exploration_id=$1 AND kind='intent' AND state='proposed'
		  AND payload->>'review_status'='rejected'
		ORDER BY id DESC LIMIT $2`, s.expID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNodes(rows)
}

// AddProposedIntent 落一条待审意图（state=proposed，worker Frontier 不会领走）。
func (s *ExplorationStore) AddProposedIntent(payload map[string]any, priority int, anchors []int64, origin string) (int64, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["executor"] = "review"
	payload["review_status"] = "pending"
	if origin == "" {
		origin = "planner"
	}
	return s.AddNode("intent", payload, priority, "proposed", origin, anchors)
}

// ApproveIntentAt 把待审意图转为 open；pending_executor 转正为 executor。
func (s *ExplorationStore) ApproveIntentAt(id int64, expectedVersion int) (bool, error) {
	return s.reviewTransition(id, expectedVersion, "open", "approved")
}

// RejectIntentAt 保持 proposed，只把 review_status 置为 rejected；planner 修订后重送审。
func (s *ExplorationStore) RejectIntentAt(id int64, expectedVersion int, note string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var payload []byte
	var curVersion int
	var state string
	if err := tx.QueryRow(`SELECT payload::text, content_version, state FROM exploration_nodes
		WHERE id=$1 AND exploration_id=$2 FOR UPDATE`, id, s.expID).Scan(&payload, &curVersion, &state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, sql.ErrNoRows
		}
		return false, err
	}
	m := map[string]any{}
	_ = json.Unmarshal(payload, &m)
	if state != "proposed" || strOf(m["executor"]) != "review" || reviewStatusOf(m) != "pending" {
		return false, ErrNotPendingReview
	}
	if curVersion != expectedVersion {
		return false, ErrStaleVersion
	}
	m["review_status"] = "rejected"
	if note != "" {
		m["review_note"] = note
	}
	m["reviewed_version"] = curVersion
	raw, _ := json.Marshal(m)
	if _, err := tx.Exec(`UPDATE exploration_nodes SET payload=$1::jsonb,
		content_version=content_version+1 WHERE id=$2 AND exploration_id=$3`, string(raw), id, s.expID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// SetReviewNote 写入 AI 预审意见（只标注，不改变状态）。
func (s *ExplorationStore) SetReviewNote(id int64, note string, reviewedVersion int) error {
	_, err := s.db.Exec(`UPDATE exploration_nodes
		SET payload = payload || jsonb_build_object('review_note',$1::text,'reviewed_version',$2::int)
		WHERE id=$3 AND exploration_id=$4 AND state='proposed'`,
		note, reviewedVersion, id, s.expID)
	return err
}

// UpdateProposedIntent 修订待审/被驳回意图的正文与优先级，并自动重新送审。
func (s *ExplorationStore) UpdateProposedIntent(id int64, expectedVersion int, summary string, priority int) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var payload []byte
	var curVersion int
	var state string
	if err := tx.QueryRow(`SELECT payload::text, content_version, state FROM exploration_nodes
		WHERE id=$1 AND exploration_id=$2 FOR UPDATE`, id, s.expID).Scan(&payload, &curVersion, &state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, sql.ErrNoRows
		}
		return false, err
	}
	if state != "proposed" {
		return false, ErrNotPendingReview
	}
	if curVersion != expectedVersion {
		return false, ErrStaleVersion
	}
	m := map[string]any{}
	_ = json.Unmarshal(payload, &m)
	if summary != "" {
		m["summary"] = summary
	}
	m["review_status"] = "pending"
	m["executor"] = "review"
	delete(m, "review_note")
	raw, _ := json.Marshal(m)
	if priority <= 0 {
		priority = 5
	}
	if priority > 10 {
		priority = 10
	}
	if _, err := tx.Exec(`UPDATE exploration_nodes SET payload=$1::jsonb, priority=$2,
		content_version=content_version+1 WHERE id=$3 AND exploration_id=$4`,
		string(raw), priority, id, s.expID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *ExplorationStore) reviewTransition(id int64, expectedVersion int, nextState, reviewStatus string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var payload []byte
	var curVersion int
	var state string
	if err := tx.QueryRow(`SELECT payload::text, content_version, state FROM exploration_nodes
		WHERE id=$1 AND exploration_id=$2 FOR UPDATE`, id, s.expID).Scan(&payload, &curVersion, &state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, sql.ErrNoRows
		}
		return false, err
	}
	m := map[string]any{}
	_ = json.Unmarshal(payload, &m)
	if state != "proposed" || strOf(m["executor"]) != "review" || reviewStatusOf(m) != "pending" {
		return false, ErrNotPendingReview
	}
	if curVersion != expectedVersion {
		return false, ErrStaleVersion
	}
	if nextState == "open" {
		if pe := strOf(m["pending_executor"]); pe != "" {
			m["executor"] = pe
		} else {
			delete(m, "executor")
		}
		delete(m, "pending_executor")
		m["review_status"] = reviewStatus
		m["reviewed_version"] = curVersion
	}
	raw, _ := json.Marshal(m)
	if _, err := tx.Exec(`UPDATE exploration_nodes SET state=$1, payload=$2::jsonb,
		content_version=content_version+1 WHERE id=$3 AND exploration_id=$4`,
		nextState, string(raw), id, s.expID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func reviewStatusOf(m map[string]any) string {
	if v, ok := m["review_status"].(string); ok && v != "" {
		return v
	}
	return "pending"
}

func strOf(v any) string {
	s, _ := v.(string)
	return s
}
