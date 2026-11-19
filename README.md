<div align="center">

<img src="web/public/brand/kaiyang-mark.svg" width="96" alt="KAIYANG"/>

# 开阳 KAIYANG

**MULTI-STAR OPS · AI 自主渗透测试平台**

> 开阳，北斗第六星，多星合而为一。<br/>
> 以 Pojun 为执行底座，缝合 artex-ld 的可控编排与 AntiFox 的交付能力。

[![CI](https://github.com/3169417664/kaiyang/actions/workflows/ci.yml/badge.svg)](https://github.com/3169417664/kaiyang/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-AGPL--3.0-blue)](LICENSE)
![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go&logoColor=white)
![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Docker-lightgrey)

**Go 单体后端** · **Next.js 内嵌前端** · **PostgreSQL** · **多 Agent 自主推进** · **全程证据留痕**

</div>

> 来源与许可：本项目基于 Pojun 0.4.82（ARTEX 0.3.15 血脉）缝合 artex-ld 与 AntiFox 的增强能力。完整归属见 [NOTICE](NOTICE)，许可证为 AGPL-3.0。

---

## 当前状态

KAIYANG 已完成 Phase 1–8 的主体缝合，`main` 为最新可运行版本：

| 阶段 | 状态 | 能力 |
|---|---|---|
| Phase 1 | ✅ | 多用户 RBAC（admin/operator/viewer）、审计、旧 admin 迁移、强制改密、登录锁定 |
| Phase 2 | ✅ | 任务轨迹合并时间线、minimap、步骤抽屉、accesslog、xlog、请求体限长、工作区敏感文件保护 |
| Phase 3 | ✅ | 意图审核 proposed 状态机、人工批准/驳回、6 场景分类、planner/worker 模板库、模板管理页 |
| Phase 4 | ✅ | 自定义执行器（`executor_key` / `pre_agent`）、Finding 派发、内网派发去重工具、trace_cleaner |
| Phase 5 | ✅ | 中文 docx 交付报告、报告管理、漏洞富化、结构化复现、截图/流量证据 |
| Phase 6 | ✅ | 全局+任务双层知识库、pgvector 守卫、关键词降级、Agent 检索工具 |
| Phase 7 | ✅ | 资产凭据模型、内网凭据 API、任务拓扑 API 与基础节点管理 |
| Phase 8 | ✅ | 实施状态文档、README、`main`/`develop` 发布同步 |

详细状态与已知缺口：[docs/stitch/implementation-status.md](docs/stitch/implementation-status.md)。

### 尚未完成（按优先级）

- 报告模板的完整多语言正文翻译（模板 locale 已存储，正文仍以中文为主）
- 新模块的端到端测试、空库/旧库迁移回归脚本、Docker Compose 冒烟
- 15 个 Dependabot 依赖升级 PR 仍 open

---

## 核心能力

### 多 Agent 自主推进

planner 是唯一意图生成者，worker 领取意图执行并写回双图（全局资产图 + 每任务探索图），mainagent 负责人机对话与即时任务。图一变就唤醒 planner，形成闭环。

### 多用户与审核

- 账号 + 固定三角色：`admin` / `operator` / `viewer`
- 路由级权限；未映射路由默认仅 admin
- 登录失败 IP + 账号双维度锁定；JWT `token_version` 改密/停用即失效
- 审计日志：登录、用户管理、审批、配置变更、高危操作
- 意图审核：`proposed` 待审池，人工批准后转 `open`；审核开启时 `add_intent` 硬拒绝

### 执行与编排

- 原生 `http_request` / `session_login` / cookie jar / `poc_scaffold` / 后台作业 / 共享脚本
- 自定义执行器：Agent 声明 `executor_key`，任务用 `pre_agent` 指定首个执行器
- Finding 派发：`dispatch_finding_to_agent` + `finding_agent_invocations`
- 内网派发：`task_dispatch_targets`、`check_internal_asset_task`、`append_finding_to_task`
- 痕迹清理：`trace_cleaner` 按 `[投放]` 事实工作，保留人工确认

### 证据与交付

- MITM 流量录制，worker 全部 HTTP/S 请求留痕
- 任务附件：截图、数据、日志、文档预览
- 中文 docx 报告：封面、目录、概述、漏洞明细、复现步骤、截图、流量证据、修复建议
- 报告文件存放在 `keyDir/report_assets/reports`，不进入可浏览工作区

### 知识库

- 全局知识库 + 任务级知识库
- pgvector 语义检索；扩展或 embedder 缺失时自动降级关键词检索
- Agent 工具：`search_knowledge`、`list_knowledge_categories`

### 出口与代理

- Clash / mihomo 内核管理：订阅拉取、节点分组/测速/五种出口模式、断点续传安装
- 代理池：总开关、导入验活、熔断跨重启持久化、本机 Clash 端口接管
- 严格出口：池已配置但全部熔断时失败关闭，不允许真实 IP 直连泄漏
- SSH 跳板：凭据加密存保险库，本地 SOCKS5 一键入池
- 统一出口变量：`EGRESS_PROXY` / `EGRESS_PROXY_CA`，Bash/job/nuclei/原生 HTTP 同源

### 资产与拓扑

- 资产凭据：`discovered` / `verified` 关系，secret 不下发
- 内网凭据：`GET /api/credentials/intranet`
- 任务拓扑：节点 / 边 / 分组 / 便签 API，任务详情「拓扑」Tab

### 运维与诊断

- 诊断包导出：配置、运行指标、任务/意图/轨迹、日志聚合、流量/代理/LLM 统计
- 宿主安装源：APT / PyPI 腾讯云、清华 TUNA 镜像
- 会话吊销：吊销当前或全部设备，复用服务端 `auth_sessions`

---

## 技术架构

```
cmd/kaiyang         主程序（Go 单体，内嵌 Next.js 静态前端）
agent/              planner / worker / mainagent / 自定义执行器
server/             HTTP API、引擎调度、审核、报告、知识库、拓扑
db/                 PostgreSQL 唯一存储；schema.sql 幂等迁移 + §FUSION 区
knowledge/          分块、embedding、检索、文档抽取
report/             手写 OOXML 报告渲染（不依赖 python-docx）
web/                Next.js App Router 前端（静态导出）
skills/             内置技能库
```

---

## 快速开始

> 依赖 **PostgreSQL 16+**；探索需配置 LLM（Anthropic / OpenAI / 兼容网关）。
> 已发布 **v0.2.0**（上游代理/诊断/运维能力全量同步版，Linux / macOS，含 `install.sh` 与 SHA256SUMS）。

### 一键安装（Linux）

```bash
curl -fsSL https://github.com/3169417664/kaiyang/releases/latest/download/install.sh | sudo bash
```

macOS 使用同一脚本（不需要 sudo）：

```bash
curl -fsSL https://github.com/3169417664/kaiyang/releases/latest/download/install.sh | bash
```

### 源码构建

### 1. 构建前端

```bash
cd web
npm ci
NEXT_EXPORT=1 npx next build
cd ..
mkdir -p server/webui/dist
cp -r web/out/. server/webui/dist/
```

### 2. 构建后端

```bash
CGO_ENABLED=0 go build -tags embedui -o kaiyang ./cmd/kaiyang
```

### 3. 准备数据库并启动

```bash
export POJUN_PG_DSN='postgres://kaiyang:password@127.0.0.1:5432/kaiyang?sslmode=disable'
./kaiyang -addr :8787 -proxy :8788
```

打开 `http://localhost:8787/setup` 设置管理员密码；首次登录后创建团队账号。

> 环境变量暂时保留 `POJUN_*` 前缀以兼容 Pojun 部署；后续版本再提供 `KAIYANG_*` 别名。

### Docker

```bash
# 先准备 dist/amd64/kaiyang 与 dist/amd64/kaiyang-hostd（见 build.sh）
docker compose up -d
```

当前 compose 默认镜像 `ghcr.io/3169417664/kaiyang:latest` 需要发布后可用；源码构建请使用 `build.sh` 生成 dist 后再构建镜像。

---

## 仓库与协作

- 仓库：`https://github.com/3169417664/kaiyang`
- 分支：`main`（可发布）、`develop`（集成）、`feature/*`、`fix/*`、`release/*`、`hotfix/*`
- `main` / `develop` 均启用分支保护：PR + CI（backend / frontend / gitleaks）+ 线性历史
- 提交规范、PR 检查清单见 [CONTRIBUTING.md](CONTRIBUTING.md)
- 漏洞报告见 [SECURITY.md](SECURITY.md)

### CI

- Go build / vet / `go test -p 1`（真实 PostgreSQL）
- 前端 `tsc --noEmit` + `NEXT_EXPORT=1 next build`
- gitleaks、CodeQL 默认扫描、Dependabot

---

## 配置

- 数据库：`config.json` 或 `POJUN_PG_DSN`
- LLM：UI「系统 → LLM」或 `POJUN_LLM_PROVIDER` / `POJUN_LLM_MODEL` / `POJUN_LLM_BASE_URL` / `POJUN_LLM_PROXY`
- 日志：`KAIYANG_LOG_LEVEL` / `POJUN_LOG_LEVEL` / `ARTEX_LOG_LEVEL`（debug/info/warn/error）
- 报告目录：`keyDir/report_assets`（不在 `dataDir` 工作区）
- 知识库 embedder：UI「系统 → 知识库」配置 OpenAI 兼容 `/embeddings` 端点

---

## 安全边界

KAIYANG 仅可用于你拥有、受你控制或已获得明确书面授权的系统与隔离靶场。禁止对未授权目标进行扫描、探测、利用或任何破坏性操作。使用前请阅读 [SECURITY.md](SECURITY.md) 与 [LICENSE](LICENSE)。

- **宿主命令执行仅限管理员发起的运行**：operator 的任务/会话可使用联网测试与图谱工具，
  但拿不到 Bash/Read/Write/job/MCP 等宿主访问面。
- **会话令牌使用 HttpOnly Cookie**，前端不落 localStorage；JWT 额外绑定服务端会话，
  仅有签名密钥无法伪造管理员。
- **流量删除、拦截规则、工具门控、LLM judge、MCP/ScopeSentry 数据源**为管理面，
  operator 只能审批待办，不能修改安全控制本身。
- 2026-11-18 审计修复与残余风险见 [docs/security/2026-11-18-审计修复记录.md](docs/security/2026-11-18-审计修复记录.md)。

---

## 致谢

- [ARTEX](https://github.com/Autumn-27/ARTEX)：双图架构与多 Agent 基座
- [Pojun](https://github.com/chu0119/Pojun)：执行面、情报、C2、宿主工具链，本项目主干
- [artex-ld](https://github.com/IcebergN2026/artex-ld)：审核、模板、自定义执行器、凭据、拓扑、轨迹
- [AntiFox](https://github.com/AntiFox-AI/antifox)：报告交付、富化、RBAC、知识库
