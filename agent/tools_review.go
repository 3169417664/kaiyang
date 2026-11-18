package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/3169417664/kaiyang/db"
	actool "github.com/Autumn-27/norma/tool"
)

// 意图审核工具：planner 送审 / 查询 / 修订 / 取消。
// 与 add_intent（直投 open）互斥：审核开启时 add_intent 会被硬拒绝。

type reviewIntentItem struct {
	Summary        string            `json:"summary"`
	AssetIDs       []json.RawMessage `json:"asset_ids"`
	ParentIDs      []json.RawMessage `json:"parent_ids"`
	Priority       int               `json:"priority"`
	TargetExecutor string            `json:"target_executor"`
	Reason         string            `json:"reason"`
}

func (t *ToolSet) addReviewIntent() actool.CoreTool {
	return t.writeExpTool("add_review_intent",
		"【送审】提交意图进入人工审核：落 proposed + review_status=pending + executor='review' 占位，不会直接执行；"+
			"审核通过后同一节点转 open。与 add_intent（直投 open）不同，本工具只送审。支持单条或 intents 数组批量。",
		obj(map[string]any{
			"summary":         str("意图描述：做什么+为什么"),
			"priority":        intp("优先级 1-10，默认 5"),
			"asset_ids":       map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "可选：目标资产 id"},
			"parent_ids":      map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "可选：上游 fact/finding 节点 id"},
			"target_executor": str("可选：建议执行器 executor_key（必须是已启用自定义 agent）"),
			"reason":          str("可选：送审理由/给审核人的说明"),
			"intents":         map[string]any{"type": "array", "items": map[string]any{"type": "object"}, "description": "批量模式：每条 {summary,priority?,asset_ids?,parent_ids?,target_executor?,reason?}"},
		}),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			if t.reviewEnabledFn == nil || !t.reviewEnabledFn() {
				return actool.Errorf("该任务未开启意图审核（review_enabled=false），add_review_intent 不可用；请改用 add_intent 直投"), nil
			}
			var req struct {
				reviewIntentItem
				Intents []reviewIntentItem `json:"intents"`
			}
			if err := json.Unmarshal(in, &req); err != nil {
				return actool.Errorf(fmt.Sprintf("参数无效: %v", err)), nil
			}
			items := req.Intents
			if len(items) == 0 {
				items = []reviewIntentItem{req.reviewIntentItem}
			}
			ids := make([]int64, len(items))
			errs := map[string]string{}
			for i, it := range items {
				summary := strings.TrimSpace(it.Summary)
				if summary == "" {
					errs[fmt.Sprint(i)] = "summary 不能为空"
					continue
				}
				priority := it.Priority
				if priority <= 0 {
					priority = 5
				}
				if priority > 10 {
					priority = 10
				}
				anchors := pidList(it.AssetIDs)
				payload := map[string]any{
					"summary":       summary,
					"executor":      "review",
					"review_status": "pending",
				}
				if len(anchors) > 0 {
					payload["asset_ids"] = anchors
				}
				if te := strings.TrimSpace(it.TargetExecutor); te != "" {
					payload["pending_executor"] = te
				}
				if reason := strings.TrimSpace(it.Reason); reason != "" {
					payload["review_reason"] = reason
				}
				id, err := t.ts.AddProposedIntent(payload, priority, anchors, t.worker)
				if err != nil {
					errs[fmt.Sprint(i)] = err.Error()
					continue
				}
				ids[i] = id
			}
			if len(errs) > 0 {
				return jsonResult(map[string]any{"ids": ids, "errors": errs})
			}
			return jsonResult(map[string]any{"ids": ids})
		})
}

func (t *ToolSet) listIntentsTool() actool.CoreTool {
	return t.readExpTool("list_intents",
		"列出本任务的意图（默认 proposed 待审；可选 open/rejected/all），带 content_version 供 update_intent 做乐观锁。",
		obj(map[string]any{
			"state": str("proposed（默认）| open | rejected | all"),
			"limit": intp("返回上限，默认 50"),
		}),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				State string `json:"state"`
				Limit int    `json:"limit"`
			}
			_ = json.Unmarshal(in, &a)
			if a.Limit <= 0 || a.Limit > 200 {
				a.Limit = 50
			}
			var nodes []*db.Node
			var err error
			switch strings.ToLower(strings.TrimSpace(a.State)) {
			case "", "proposed":
				nodes, err = t.ts.ProposedIntents(a.Limit)
			case "rejected":
				nodes, err = t.ts.RejectedIntents(a.Limit)
			case "open":
				nodes, err = t.ts.Frontier(a.Limit)
			case "all":
				nodes, err = t.ts.ListByKind("intent", a.Limit)
			default:
				return actool.Errorf("state 只支持 proposed/open/rejected/all"), nil
			}
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			out := make([]map[string]any, 0, len(nodes))
			for _, n := range nodes {
				var p map[string]any
				_ = json.Unmarshal(n.Payload, &p)
				out = append(out, map[string]any{
					"id": n.ID, "state": n.State, "priority": n.Priority,
					"summary": p["summary"], "executor": p["executor"],
					"review_status": p["review_status"], "review_note": p["review_note"],
					"content_version": n.ContentVersion, "created_at": n.CreatedAt.Unix(),
				})
			}
			return jsonResult(map[string]any{"intents": out})
		})
}

func (t *ToolSet) updateIntentTool() actool.CoreTool {
	return t.writeExpTool("update_intent",
		"修订 proposed / 被驳回意图的正文与优先级（expected_version 乐观锁）；修订后自动重新送审。",
		obj(map[string]any{
			"intent_id":        idp("意图节点 id（必填）"),
			"expected_version": intp("当前 content_version（必填）"),
			"summary":          str("新的意图描述"),
			"priority":         intp("新的优先级 1-10（可选）"),
		}, "intent_id", "expected_version"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				IntentID        int64  `json:"intent_id"`
				ExpectedVersion int    `json:"expected_version"`
				Summary         string `json:"summary"`
				Priority        int    `json:"priority"`
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return actool.Errorf("参数无效"), nil
			}
			ok, err := t.ts.UpdateProposedIntent(a.IntentID, a.ExpectedVersion, strings.TrimSpace(a.Summary), a.Priority)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			if !ok {
				return actool.Errorf("修订未生效（状态或版本冲突）"), nil
			}
			return jsonResult(map[string]any{"ok": true})
		})
}

func (t *ToolSet) cancelIntentTool() actool.CoreTool {
	return t.writeExpTool("cancel_intent",
		"软删除 open/proposed 意图（留血缘，原因必填）；running/paused 请改用 kill/control。",
		obj(map[string]any{
			"intent_id": idp("意图节点 id（必填）"),
			"reason":    str("取消原因（必填）"),
		}, "intent_id", "reason"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				IntentID int64  `json:"intent_id"`
				Reason   string `json:"reason"`
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return actool.Errorf("参数无效"), nil
			}
			if strings.TrimSpace(a.Reason) == "" {
				return actool.Errorf("reason 必填"), nil
			}
			summary, err := t.ts.SoftDeleteIntent(a.IntentID, a.Reason)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return jsonResult(map[string]any{"ok": true, "summary": summary})
		})
}
