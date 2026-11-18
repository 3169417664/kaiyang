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
	return s.runCustomExecutorFor(ctx, t, strings.TrimSpace(t.PreAgent), intent)
}

// runCustomExecutorFor 显式接收执行器 Agent key：pre_agent 阶段传任务配置，
// 审核批准的 target_executor 传意图 payload 里的 executor。没有 pre_agent 的
// 任务也走这里，不再依赖 t.PreAgent 非空。
func (s *Server) runCustomExecutorFor(ctx context.Context, t *Task, key string, intent *db.Node) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("executor key 为空")
	}
	if intent == nil {
		return fmt.Errorf("执行器 %q 没有待执行意图", key)
	}
	a, err := s.m.pg.GetAgentByKey(key)
	if err != nil || a == nil {
		return fmt.Errorf("执行器 %q 不存在: %w", key, err)
	}
	if !a.Enabled {
		return fmt.Errorf("执行器 %q 已停用", key)
	}
	if a.Builtin {
		return fmt.Errorf("执行器 %q 是内置 Agent，不能作为自定义执行器", key)
	}
	prompt, _ := s.m.pg.CurrentPrompt(a.ID)
	prov := s.llmDirect
	if a.LLMProfileID != nil {
		if p, _, ok := s.providerForProfile(*a.LLMProfileID); ok {
			prov = p
		}
	}
	if prov == nil {
		return fmt.Errorf("执行器 %q 没有可用 LLM 配置", key)
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
