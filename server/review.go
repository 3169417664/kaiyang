package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/3169417664/kaiyang/db"
)

// 意图审核 HTTP 面（KAIYANG Phase 3）。
// AI 预审与 review 对话后续接入；当前先提供人工批准/驳回与待审列表，
// 保证「审核开启 → 意图必须进 proposed → 人工放行」的闸门闭环。

// reviewTargetExecutor 从批准后的意图 payload 读取执行器 key。
// 审核批准会把 pending_executor 转正为 executor；没有预选时 executor 被删除，
// 返回空串（由默认 worker 执行）。
func reviewTargetExecutor(payload json.RawMessage) string {
	var p struct {
		Executor string `json:"executor"`
	}
	if json.Unmarshal(payload, &p) != nil {
		return ""
	}
	key := strings.TrimSpace(p.Executor)
	if key == "review" {
		return ""
	}
	return key
}

// PUT /api/tasks/{id}/review/config — 运行时开关审核。
func (s *Server) reviewConfig(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
		Auto    bool `json:"auto"`
	}
	if !decodeJSON(w, r, &req, maxSmallJSONBodyBytes) {
		return
	}
	id, err := strconv.ParseInt(t.ID, 10, 64)
	if err != nil {
		writeErr(w, 400, "invalid task id")
		return
	}
	if err := s.m.pg.SetTaskReviewConfig(id, req.Enabled, req.Auto); err != nil {
		writeErr(w, 500, "保存失败: "+err.Error())
		return
	}
	t.ReviewEnabled = req.Enabled
	t.ReviewAuto = req.Auto
	t.Notify()
	writeJSON(w, 200, map[string]any{"ok": true, "enabled": req.Enabled, "auto": req.Auto})
}

func (s *Server) reviewListProposed(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	if !t.ReviewEnabled {
		writeErr(w, 409, "该任务未开启意图审核（review_enabled=false）")
		return
	}
	ns, err := t.Store.ProposedIntents(100)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(ns))
	for _, n := range ns {
		var p map[string]any
		_ = json.Unmarshal(n.Payload, &p)
		out = append(out, map[string]any{
			"id": n.ID, "summary": p["summary"], "priority": n.Priority, "origin": n.Origin,
			"pending_executor": p["pending_executor"], "review_note": p["review_note"],
			"review_reason": p["review_reason"], "content_version": n.ContentVersion,
			"created_at": n.CreatedAt.Unix(),
		})
	}
	writeJSON(w, 200, map[string]any{"intents": out})
}

func (s *Server) reviewApprove(w http.ResponseWriter, r *http.Request) {
	s.reviewAction(w, r, true)
}

func (s *Server) reviewReject(w http.ResponseWriter, r *http.Request) {
	s.reviewAction(w, r, false)
}

func (s *Server) reviewAction(w http.ResponseWriter, r *http.Request, approve bool) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	if !t.ReviewEnabled {
		writeErr(w, 409, "该任务未开启意图审核（review_enabled=false）")
		return
	}
	intentID, err := strconv.ParseInt(r.PathValue("intentID"), 10, 64)
	if err != nil || intentID <= 0 {
		writeErr(w, 400, "invalid intent id")
		return
	}
	var body struct {
		ExpectedVersion *int   `json:"expected_version"`
		Note            string `json:"note"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	version := 0
	if body.ExpectedVersion != nil {
		version = *body.ExpectedVersion
	} else {
		// 兼容旧客户端：不传版本时取当前版本，CAS 仍会在并发处置时失败。
		n, e := t.Store.GetNode(intentID)
		if e != nil {
			if errors.Is(e, sql.ErrNoRows) {
				writeErr(w, 404, "意图不存在")
				return
			}
			writeErr(w, 500, e.Error())
			return
		}
		version = n.ContentVersion
	}
	var ok2 bool
	if approve {
		ok2, err = t.Store.ApproveIntentAt(intentID, version)
	} else {
		ok2, err = t.Store.RejectIntentAt(intentID, version, strings.TrimSpace(body.Note))
	}
	if err != nil {
		if errors.Is(err, db.ErrNotPendingReview) || errors.Is(err, db.ErrStaleVersion) || errors.Is(err, sql.ErrNoRows) {
			writeErr(w, 409, "意图已被处理（并发冲突）")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	if !ok2 {
		writeErr(w, 409, "意图已被处理")
		return
	}
	s.m.pg.RecordAudit(auditActor(r), "review", map[bool]string{true: "approve", false: "reject"}[approve], "success", clientIP(r),
		"任务 "+t.ID+" 意图 "+strconv.FormatInt(intentID, 10), nil)
	// 批准后唤醒 planner 继续规划；驳回也让 planner 看到驳回稿。
	t.Notify()
	// 批准的 target_executor：没有 pre_agent 的任务也要真正启动自定义执行器，
	// 否则 open + payload.executor=<key> 的意图既不会被默认 worker 领取
	// （Frontier 排除 executor），也不会进 pre_agent 阶段，只能永久滞留。
	if approve {
		if n, e := t.Store.GetNode(intentID); e == nil && n != nil {
			if key := reviewTargetExecutor(n.Payload); key != "" {
				runCtx := s.ctx
				if runCtx == nil {
					runCtx = context.Background()
				}
				go func() {
					if err := s.engine.RunApprovedExecutor(runCtx, t, key, n); err != nil {
						log.Printf("[review] task %s 意图 %d 目标执行器 %q 启动失败: %v", t.ID, intentID, key, err)
						t.Notify()
					}
				}()
			}
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
