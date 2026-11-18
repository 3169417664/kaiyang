package agent

import (
	"context"
	"strconv"
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
	// 服务端（pre_agent 阶段 / 审核批准路径）已在启动本次运行前 CAS 领取意图；
	// 把领取契约写进提示词，避免 Agent 误以为还要自己调 claim_executor_intent。
	prompt := strings.TrimSpace(cfg.Prompt)
	if prompt != "" {
		prompt += "\n\n"
	}
	intentID := int64(0)
	if cfg.Intent != nil {
		intentID = cfg.Intent.ID
	}
	prompt += "【执行器领取契约】本次意图 #" + strconv.FormatInt(intentID, 10) +
		" 已由服务端 CAS 领取并交给你执行（owner=exec:" + cfg.Agent.Key + "）。不要调用 claim_executor_intent 重复领取；直接执行当前意图。"
	wk.SetTaskPromptOverride(prompt)
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
