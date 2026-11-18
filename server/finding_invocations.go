package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/3169417664/kaiyang/agent"
	"github.com/3169417664/kaiyang/db"
	actool "github.com/Autumn-27/norma/tool"
)

// planner 显式把一条 Finding 派发给已启用、非内置的自定义 Agent。
func (s *Server) toolDispatchFindingToAgent() actool.CoreTool {
	return wrTool("dispatch_finding_to_agent",
		"把当前任务的一条独立漏洞记录（findings.id，不是 finding_node_id）显式派发给一个已启用、非内置的自定义 Agent。"+
			"同一 agent_key+finding_id 只记录一次；已派发时返回首次会话 ID。适用于 reporter / vuln_dispatcher / trace_cleaner 等编排 Agent。",
		objSchema(map[string]any{
			"agent_key":  strParam("目标自定义 Agent 的 key（必须已启用且非内置）"),
			"finding_id": map[string]any{"type": "integer", "description": "findings.id（独立漏洞记录 ID）"},
			"reason":     strParam("可选：派发原因/上下文"),
		}, "agent_key", "finding_id"),
		func(ctx context.Context, in json.RawMessage) (actool.Result, error) {
			var a struct {
				AgentKey  string `json:"agent_key"`
				FindingID int64  `json:"finding_id"`
				Reason    string `json:"reason"`
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return actool.Errorf("参数无效"), nil
			}
			if strings.TrimSpace(a.AgentKey) == "" || a.FindingID <= 0 {
				return actool.Errorf("agent_key 与 finding_id 必填"), nil
			}
			ri := agent.RunInfoFrom(ctx)
			if ri.TaskID <= 0 {
				return actool.Errorf("当前不在任务上下文中，不能派发 Finding"), nil
			}
			t, ok := s.m.Task(strconv.FormatInt(ri.TaskID, 10))
			if !ok {
				return actool.Errorf("任务不存在"), nil
			}
			f, err := s.m.pg.GetFinding(a.FindingID)
			if err != nil || f == nil {
				return actool.Errorf("finding 不存在"), nil
			}
			if f.TaskID == nil || *f.TaskID != ri.TaskID {
				return actool.Errorf("finding 不属于当前任务"), nil
			}
			target, err := s.m.pg.GetAgentByKey(a.AgentKey)
			if err != nil || target == nil {
				return actool.Errorf("目标 Agent 不存在"), nil
			}
			if !target.Enabled || target.Builtin {
				return actool.Errorf("目标 Agent 必须已启用且非内置"), nil
			}
			created, convID, err := s.m.pg.RecordFindingInvocation(a.AgentKey, a.FindingID, ri.TaskID, a.Reason)
			if err != nil {
				return actool.Errorf(fmt.Sprintf("记录派发失败: %v", err)), nil
			}
			if convID != nil {
				return jsonResult(map[string]any{"ok": true, "already_invoked": true, "conversation_id": *convID})
			}
			if !created {
				return jsonResult(map[string]any{"ok": true, "already_invoked": true})
			}
			conv, err := s.m.pg.CreateConversation(a.AgentKey, firstLine("Finding: "+f.Summary, 60), nil)
			if err != nil {
				return actool.Errorf(fmt.Sprintf("创建会话失败: %v", err)), nil
			}
			if err := s.m.pg.SetFindingInvocationConversation(a.AgentKey, a.FindingID, conv.ID); err != nil {
				log.Printf("[finding-dispatch] set conversation: %v", err)
			}
			msg := buildFindingDispatchMessage(t, f, a.Reason)
			go s.runFindingInvocation(conv, msg)
			return jsonResult(map[string]any{"ok": true, "enqueued": true, "conversation_id": conv.ID})
		})
}

func buildFindingDispatchMessage(t *Task, f *db.DBFinding, reason string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "任务 #%s 新登记一条漏洞，请按你的职责处理。\n", t.ID)
	nodeID := int64(0)
	if f.NodeID != nil {
		nodeID = *f.NodeID
	}
	fmt.Fprintf(&b, "finding_id=%d finding_node_id=%d vulnclass=%s severity=%s\n", f.ID, nodeID, f.VulnClass, f.Severity)
	fmt.Fprintf(&b, "标题：%s\n摘要：%s\n", f.Name, f.Summary)
	if f.Evidence != "" {
		fmt.Fprintf(&b, "证据：\n%s\n", f.Evidence)
	}
	if reason != "" {
		fmt.Fprintf(&b, "派发原因：%s\n", reason)
	}
	b.WriteString("\n注意：所有清理/删除动作必须逐项获得人工确认；不要扩大任务授权范围。")
	return b.String()
}

func (s *Server) runFindingInvocation(c *db.Conversation, msg string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[finding-dispatch] conv %d panic: %v", c.ID, r)
		}
	}()
	pg := s.m.pg
	if _, err := pg.AppendConvActivity(c.ID, db.Activity{Worker: c.AgentKey, Kind: "user", Summary: firstLine(msg, 200), Detail: msg}); err != nil {
		log.Printf("[finding-dispatch] append msg: %v", err)
	}
	busyKey := s.convBusyKey(c.ID)
	s.chatMu.Lock()
	s.chatBusy[busyKey] = true
	s.chatMu.Unlock()
	s.runConversationSync(c, msg, busyKey)
}

func (s *Server) findingInvocationRetryLoop() {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			s.retryFindingInvocations()
		}
	}
}

// 10 分钟补发：会话创建成功但 invocation 未回填时由后台循环调用。
func (s *Server) retryFindingInvocations() {
	items, err := s.m.pg.PendingFindingInvocations(10*time.Minute, 20)
	if err != nil {
		return
	}
	for _, it := range items {
		f, err := s.m.pg.GetFinding(it.FindingID)
		if err != nil || f == nil {
			continue
		}
		t, ok := s.m.Task(strconv.FormatInt(it.TaskID, 10))
		if !ok {
			continue
		}
		conv, err := s.m.pg.CreateConversation(it.AgentKey, firstLine("Finding: "+f.Summary, 60), nil)
		if err != nil {
			continue
		}
		_ = s.m.pg.SetFindingInvocationConversation(it.AgentKey, it.FindingID, conv.ID)
		go s.runFindingInvocation(conv, buildFindingDispatchMessage(t, f, it.Reason))
	}
}
