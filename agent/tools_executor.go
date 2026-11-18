package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	actool "github.com/Autumn-27/norma/tool"
)

// 自定义执行器工具：在 Worker 工具底座上增加「查看可领取意图」与「CAS 领取」。
func (t *ToolSet) claimExecutorIntent() actool.CoreTool {
	return t.writeExpTool("claim_executor_intent",
		"兼容入口：按 intent_id 领取一条 payload.executor 与自身 executor_key 匹配的意图（open/proposed → running）。"+
			"正常 pre_agent 与审核批准路径已由服务端 CAS 领取并把意图直接交给你执行——收到「执行器领取契约」提示时不要重复调用本工具；"+
			"只有提示词明确要求自领、且意图尚未被服务端领取时才使用。领取失败（不匹配/已被领/状态不对）时不要执行该意图。",
		obj(map[string]any{
			"intent_id": idp("意图节点 id（先用 list_intents 查）"),
		}, "intent_id"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			if strings.TrimSpace(t.executorKey) == "" {
				return actool.Errorf("当前会话没有绑定 executor_key，不能领取执行器意图"), nil
			}
			var a struct {
				IntentID int64 `json:"intent_id"`
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return actool.Errorf("参数无效"), nil
			}
			owner := "exec:" + t.executorKey
			ok, err := t.ts.ClaimExecutorIntent(a.IntentID, t.executorKey, owner)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			if !ok {
				return actool.Errorf(fmt.Sprintf("意图 #%d 无法领取（不匹配或已被处理）", a.IntentID)), nil
			}
			return jsonResult(map[string]any{"ok": true, "intent_id": a.IntentID, "owner": owner})
		})
}

// CustomExecutorTools = Worker 工具底座 + 意图查询/领取。
func (t *ToolSet) CustomExecutorTools() []actool.CoreTool {
	base := t.WorkerTools()
	return append(base, t.listIntentsTool(), t.claimExecutorIntent())
}
