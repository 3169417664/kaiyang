package server

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/3169417664/kaiyang/agent"
	"github.com/3169417664/kaiyang/db"
	"github.com/Autumn-27/norma/harness"
)

// runCustomExecutor 由 Engine 的 pre_agent 阶段调用：加载自定义 Agent 的提示词与
// 模型，复用 Worker 执行链跑一条已领取的意图，结束后落 done/blocked 终态。
func (s *Server) runCustomExecutor(ctx context.Context, t *Task, intent *db.Node) error {
	key := strings.TrimSpace(t.PreAgent)
	if key == "" {
		return fmt.Errorf("pre_agent 为空")
	}
	a, err := s.m.pg.GetAgentByKey(key)
	if err != nil || a == nil {
		return fmt.Errorf("pre_agent %q 不存在: %w", key, err)
	}
	if !a.Enabled {
		return fmt.Errorf("pre_agent %q 已停用", key)
	}
	prompt, _ := s.m.pg.CurrentPrompt(a.ID)
	prov := s.llmDirect
	if a.LLMProfileID != nil {
		if p, _, ok := s.providerForProfile(*a.LLMProfileID); ok {
			prov = p
		}
	}
	if prov == nil {
		return fmt.Errorf("pre_agent %q 没有可用 LLM 配置", key)
	}
	taskID, _ := strconv.ParseInt(t.ID, 10, 64)
	reason, _, err := agent.RunCustomExecutor(ctx, agent.CustomExecutorConfig{
		Agent: a, Provider: prov, TaskID: taskID, Intent: intent,
		Assets: s.m.assets, Store: t.Store, Goal: t.Goal, Prompt: prompt,
		Emit:             func(act db.Activity) { s.engine.emitActivity(t, act) },
		Enrich:           s.m.enrich,
		NotifyFinding:    t.NotifyFinding,
		FindingRecorder:  s.evidenceStore(),
		ArtifactRecorder: s,
		WorkDir:          s.m.dir,
		ProxyAddr:        s.m.ProxyAddr(),
		ProxyCACert:      s.m.ProxyCACert(),
	})
	state := "blocked"
	if reason == harness.ReasonCompleted {
		state = "done"
	}
	_ = t.Store.FinishExecutorIntent(intent.ID, state, fmt.Sprint(reason))
	if err != nil {
		return err
	}
	return nil
}
