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

1. **spawn_task 祖先链**：`finding_id` 已做资产去重与派发登记，但未复制 Finding 完整祖先子图。
2. **报告多语言**：模板 `locale` 已存储，正文仍以中文为主，未做完整翻译。
3. **领域模板正文**：6 个场景模板可编辑，初始正文仍共用 Pojun 默认。
4. **拓扑自动布局**：画布已接入 React Flow，当前为简单网格布局，未引入 dagre 分层。
5. **发布链路**：尚无正式 Release/tag，`install.sh` 的下载链路不可用。
6. **测试覆盖**：新模块缺少端到端测试、空库/旧库迁移回归与 Docker Compose 冒烟。
7. **依赖升级**：15 个 Dependabot PR 仍 open。

## 验证

- GitHub CI：Go build/vet、真实 PostgreSQL `go test -p 1`、前端 tsc/static build、gitleaks、CodeQL。
- 每个 Phase 独立 PR 合并，CI 全绿后进入下一阶段。
