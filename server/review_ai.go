package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/3169417664/kaiyang/db"
	"github.com/Autumn-27/norma/llm"
)

// prescreenIntent 用任务/全局 LLM 对一条待审意图做只读预审，写入 review_note；
// t.ReviewAuto=true 时按预审结论自动批准/驳回（CAS）。
func (s *Server) prescreenIntent(t *Task, n *db.Node) {
	prov := s.llmDirect
	if t.ActiveLLMProfileID != nil {
		if p, _, ok := s.providerForProfile(*t.ActiveLLMProfileID); ok {
			prov = p
		}
	}
	if prov == nil {
		return
	}
	var p map[string]any
	_ = json.Unmarshal(n.Payload, &p)
	summary, _ := p["summary"].(string)
	if strings.TrimSpace(summary) == "" {
		return
	}
	reason, _ := p["review_reason"].(string)
	sys := "你是授权渗透测试平台的意图审核助手。只做只读预审，不执行任何操作。"
	ask := fmt.Sprintf("任务目标：%s\n待审意图：%s\n送审理由：%s\n\n请判断这条意图是否具体、可执行、在授权范围内。只输出 JSON：{\"verdict\":\"pass\"|\"revise\",\"note\":\"不超过120字的中文理由\"}", t.Goal, summary, reason)
	tmp := 0.0
	msg, _, _, err := prov.Complete(context.Background(), llm.CompletionRequest{
		System: []string{sys}, Messages: []llm.Message{llm.UserText(ask)}, MaxTokens: 300, Temperature: &tmp,
	})
	if err != nil {
		log.Printf("[review-ai] task %s intent %d 预审失败: %v", t.ID, n.ID, err)
		return
	}
	var out struct {
		Verdict string `json:"verdict"`
		Note    string `json:"note"`
	}
	text := strings.TrimSpace(msg.Text())
	if i, j := strings.Index(text, "{"), strings.LastIndex(text, "}"); i >= 0 && j > i {
		text = text[i : j+1]
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		log.Printf("[review-ai] task %s intent %d 解析失败: %v (%s)", t.ID, n.ID, err, clipText(text, 120))
		return
	}
	if strings.TrimSpace(out.Note) == "" {
		out.Note = "AI 预审未给出理由"
	}
	if err := t.Store.SetReviewNote(n.ID, out.Note, n.ContentVersion); err != nil {
		log.Printf("[review-ai] task %s intent %d 写预审意见失败: %v", t.ID, n.ID, err)
		return
	}
	if !t.ReviewAuto {
		return
	}
	switch strings.ToLower(strings.TrimSpace(out.Verdict)) {
	case "pass":
		if ok, err := t.Store.ApproveIntentAt(n.ID, n.ContentVersion); err == nil && ok {
			s.m.pg.RecordAudit("review_ai", "review", "approve", "success", "", "AI 预审通过自动批准", nil)
			t.Notify()
		}
	case "revise":
		if ok, err := t.Store.RejectIntentAt(n.ID, n.ContentVersion, out.Note); err == nil && ok {
			s.m.pg.RecordAudit("review_ai", "review", "reject", "success", "", "AI 预审建议修订", nil)
			t.Notify()
		}
	}
}

// maybePrescreen 在待审列表接口被调用时为缺少预审意见的意图异步触发一次。
func (s *Server) maybePrescreen(t *Task, n *db.Node) {
	var p map[string]any
	_ = json.Unmarshal(n.Payload, &p)
	if rv, ok := p["reviewed_version"].(float64); ok && int(rv) >= n.ContentVersion {
		return
	}
	go s.prescreenIntent(t, n)
}
