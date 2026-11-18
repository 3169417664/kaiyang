# KAIYANG 缝合决策记录

来源：`C:\Users\Administrator\.claude\plans\virtual-roaming-petal.md`（已批准）。

## 基线与来源

- 主干：Pojun 0.4.82（ARTEX 0.3.15 血脉）。
- 审核/模板/执行器/凭据/拓扑/轨迹：artex-ld 0.1.6。
- 报告/富化/RBAC/知识库：AntiFox 2.0.3。
- 不改上游 module：KAIYANG module 为 `github.com/3169417664/kaiyang`。

## 产品决策

- 名称：开阳 KAIYANG / MULTI-STAR OPS。
- 权限：admin / operator / viewer；权限表 + 路由级强制；不做资源归属隔离。
- 审计：admin + operator 可读。
- 用户名：只保留 `admin`。
- 高危能力：C2/listener、hostd、系统配置、LLM/Agent/MCP/工具、用户管理、更新，仅 admin。
- 审核：内网渗透/权限提升/漏洞利用默认开；审核开启时 `add_intent` 硬拒绝。
- 报告：中文 docx；Pojun OOXML + AntiFox 富化/管理；AI 叙述 + 确认即自动富化；异步生成 + 保留同步导出。
- 知识库：双层 RAG；自动检测 pgvector/embedder，缺失降级关键词。
- trace_cleaner 与内网 Finding 派发：本期实现；trace_cleaner 采用「原版对话确认 + 每个动作 intercept ask」双闸。
- 部署：Docker Compose + 裸机二进制。
- 存量数据：全新部署，仅迁移旧管理员密码。
- 会话：480 分钟滑动续期；迁移 admin 强制改密。

## 数据模型

- 规范键：`findings.id`；`node_id` 仅兼容。
- 证据复用 `task_artifacts` + `finding_traffic_bindings` + `traffic_evidence_snapshots`。
- 新增 `findings.reproduction`、`finding_enrichment(finding_id PK)`。
- schema 只追加 `§FUSION-*` 段；CHECK 用 DO 块；一次性迁移用 settings flag。

## 阶段

P0 仓库/基线 → P1 RBAC → P2 低风险 → P3 审核+模板 → P4 执行器+编排 → P5 报告 → P6 知识库 → P7 凭据+拓扑 → P8 加固。

## 仓库

- GitHub：`3169417664/kaiyang`（private）。
- 分支：Git Flow-lite（main/develop/feature/fix/release/hotfix）。
- CI：Go build/vet/test（PostgreSQL）、前端 tsc/static build、CodeQL、gitleaks、Dependabot。
- 大文件：`report/fonts/*.ttf` 走 Git LFS。
