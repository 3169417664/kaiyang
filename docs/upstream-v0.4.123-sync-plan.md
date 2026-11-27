# 上游 Pojun v0.4.90–v0.4.123 同步计划

基线：KAIYANG `v0.2.1`（已同步到上游 v0.4.89 + 代理池修复）。
上游：`chu0119/Pojun` v0.4.123，34 个 Release。

## 批次 1：验证可信度 + 请求头安全（本次）

- [x] `validate_finding` + `agent/validators`（RCE/XSS/SSRF）
- [x] `audit_finding` + verification_strategy 确定性复核
- [x] `auth_request` / `diff_responses` header CRLF、`@file` 防护
- [x] `http_request` User-Agent 统一校验
- [x] 工具注册进 worker/planner/mainagent

## 批次 2：上下文与经验（建议下一批）

- [x] 覆盖率矩阵（asset × dimension）同源到 planner/autopilot/报告
- [x] 终态 reflection + `intent_reflections`，planner/worker 注入
- [x] 上下文分层 Index/Summary/Raw，Summary 12KB 预算
- [x] 长意图摘要窗口化（v0.4.106）
- [ ] `search_experience` / playbook 武器库 / 资产 SimHash

## 批次 3：预算与回归

- [x] 任务级 token/USD/墙钟预算网关
- [ ] 确定性回归库 `finding_regressions` + 回放
- [ ] 离线评测 harness `bench score/validate/compare` + pricing

## 批次 4：运维与部署

- [x] MCP 启动健康检查与错误脱敏
- [x] 附件配额/落盘清理闭环
- [x] 任务多角色测试凭据池
- [x] 未引用流量 TTL 清理
- [x] 任务删除异步清理队列
- [x] 资源自适应（opt-in）
- [x] 内置 HTTPS/证书管理（opt-in）

## 批次 5：LLM 与前端

- [x] LLM 配置独立提示词、思考强度覆盖、路由可视化
- [ ] 全链路装配视图
- [x] 工具可见性/BM25 工具搜索
- [ ] 微信小程序渗透能力

## 原则

- 按模块移植，不整仓合并。
- 每个新路由必须补 `permissionForRequest`。
- 新工具必须进三张工具清单；纯读工具用 `readTool/readExpTool`。
- 新表/列只追加到 `schema.sql`，迁移幂等。
