# KAIYANG 实施状态（Phase 1–7）

| 阶段 | 状态 | 说明 |
|---|---|---|
| Phase 1 多用户 RBAC + 审计 | ✅ 已合并 | 三角色、旧 admin 迁移、强制改密、登录锁定、审计 |
| Phase 2 轨迹 + 可观测性 | ✅ 已合并 | session=all 轨迹、accesslog、xlog、请求体限长、工作区敏感文件保护 |
| Phase 3 审核 + 场景模板 | ✅ 已合并 | proposed 状态机、人工批准/驳回、6 场景、模板库四级链 |
| Phase 4 自定义执行器 + Finding 编排 | ✅ 已合并 | executor_key/pre_agent、dispatch_finding_to_agent、内网派发工具、trace_cleaner |
| Phase 5 报告交付 | ✅ 已合并 | 中文 docx、报告管理、富化、复现、截图/流量证据 |
| Phase 6 知识库 | ✅ 已合并 | 全局+任务双层 RAG、pgvector 守卫、关键词降级 |
| Phase 7 凭据 + 拓扑 | ✅ 已合并 | asset_credentials/links、内网凭据 API、任务拓扑 API/基础 UI |

## 已知缺口

1. **审核 AI 预审/自动处置**：`review_auto` 已落库，但尚未接入 LLM 预审；当前为纯人工审核。
2. **领域模板正文**：6 个场景模板可编辑，但初始正文都取 Pojun 默认；尚未按内网/提权/漏洞利用差异化。
3. **建任务模板选择**：走分类默认模板 + API；表单还未暴露模板下拉。
4. **拓扑画布**：后端节点/边/分组/便签 API 完整，前端目前是节点管理表格，尚未接入力导向/React Flow 画布。
5. **报告 AI 叙述**：报告使用结构化字段与原始证据生成，AI 章节叙述/E2E 自动富化尚未接入。
6. **内部派发**：`vuln_dispatcher` 工具已 seed，`append_finding_to_task` 会写 hint；完整的「自动识别内网资产→派生/追加子任务」仍需按实际场景调优。
7. **凭据 UI**：后端与工具已启用；资产页凭据 Tab 尚未恢复展示（参考 artex-ld 的 feature flag 方案）。

## 验证

- GitHub CI：Go build/vet、真实 PostgreSQL `go test -p 1`、前端 tsc/static build、gitleaks、CodeQL。
- 每个 Phase 独立 PR 合并，CI 全绿后进入下一阶段。
