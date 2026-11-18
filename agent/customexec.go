package agent

import (
	"context"
	"strings"
	"time"

	"github.com/3169417664/kaiyang/db"
	"github.com/Autumn-27/norma/harness"
	"github.com/Autumn-27/norma/llm"
)

// CustomExecutorConfig 自定义执行器输入。
type CustomExecutorConfig struct {
	Agent            *db.Agent
	Provider         llm.Provider
	TaskID           int64
	Intent           *db.Node
	Assets           *db.AssetStore
	Store            *db.ExplorationStore
	Goal             string
	Prompt           string // 自定义 Agent 的当前提示词正文
	Hooks            harness.HookRunner
	Emit             func(db.Activity)
	Enrich           EnrichTrigger
	NotifyFinding    func(int64, string)
	FindingRecorder  FindingRecorder
	ArtifactRecorder ArtifactRecorder
	WorkDir          string
	ProxyAddr        string
	ProxyCACert      string
}

// RunCustomExecutor 用声明 executor_key 的自定义 Agent 执行一条已领取的意图。
//
// 复用 Pojun Worker 的执行链（执行期安全、证据、审批、录制全部保留），只替换：
//   - agent key → 自定义 Agent（提示词/技能/MCP 装配按它走）；
//   - 提示词 → 该 Agent 的当前模板；
//   - 工具底座 → WorkerTools + list_intents + claim_executor_intent；
//   - 执行者名 → exec:<key>，活动流可追溯。
func RunCustomExecutor(ctx context.Context, cfg CustomExecutorConfig) (harness.TerminalReason, WriteCounts, error) {
	key := "exec:" + cfg.Agent.Key
	wk := NewWorker(cfg.Provider, cfg.Agent.Key, cfg.WorkDir, nil, 0, cfg.Agent.MaxTurns)
	wk.SetAgentKey(cfg.Agent.Key)
	wk.SetTaskPromptOverride(strings.TrimSpace(cfg.Prompt))
	wk.SetRunTimeout(time.Duration(cfg.Agent.RunSecs) * time.Second)
	wk.SetProxy(cfg.ProxyAddr, cfg.ProxyCACert)
	if cfg.FindingRecorder != nil {
		wk.SetFindingRecorder(cfg.FindingRecorder)
	}
	if cfg.ArtifactRecorder != nil {
		wk.SetArtifactRecorder(cfg.ArtifactRecorder)
	}
	// 额外工具（list_intents / claim_executor_intent）由 Worker.execute 检测
	// agentKey != "worker" 时自动换用 CustomExecutorTools。
	return wk.Execute(ctx, key, cfg.TaskID, cfg.Assets, cfg.Store, cfg.Intent, cfg.Hooks, cfg.Emit, cfg.Enrich, cfg.NotifyFinding)
}
