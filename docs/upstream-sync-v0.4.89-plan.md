# 上游 v0.4.89 全量能力同步计划

目标：把上游 Pojun v0.4.83–v0.4.89 的新功能全部移植进 KAIYANG，同时保留
KAIYANG 的 RBAC、审核、报告、知识库、拓扑与凭据改造。

基线：`main` @ `59c3dd4`（已包含 v0.4.87 审计修复、去品牌、#21.2 审查脚本）。
上游源：`chu0119/Pojun` tag `v0.4.89`。

## 任务分解

### P1 · 导入与隔离（先做，低耦合）

- [x] 导入 `vault/`：XChaCha20-Poly1305 凭据保险库 + 测试
- [x] 导入 `mihomo/`：内核安装、配置生成、订阅解析、启动/停止、测速
- [x] 导入 `egress/`：SSH 跳板最小 SOCKS5
- [x] 模块路径替换为 `github.com/3169417664/kaiyang`，编译通过

### P2 · 代理池与出口后端

- [x] `server/manager.go`：出口模式、严格出口、代理池/mihomo 状态接线
- [x] `server/proxypool_api.go` / `proxypool_health.go`：升级为上游语义
- [x] 新增 `proxypool_local.go`（本机 Clash 端口接管）、`proxypool_mihomo.go`、
      `proxypool_subscription.go`
- [x] 新增 `server/egress_api.go`：Clash / SSH 跳板 / 严格出口 API
- [x] `traffic/pool.go` / `traffic.go`：捕获关闭时池仍生效、严格出口哨兵
- [x] 环境变量：新增 `EGRESS_PROXY`，兼容读取旧 `POJUN_PROXY`
- [x] Agent 侧：`worker.go`、`tools_http.go`、`tools_jobs.go`、`vulnintel/nuclei.go`
      统一注入平台出口（含 Bash/job/原生 HTTP/自定义工具/任务登录自检）

### P3 · 诊断包

- [x] `server/diagnostics.go` + `diagnostics_collect.go`
- [x] `traffic/diagnostics.go`
- [x] 代理 URL `user:pass` 掩码；新增 `/api/diagnostics/export`

### P4 · 宿主安装源

- [x] `hostd/install_sources.go` + hostd `/jobs` 镜像参数
- [x] `server/hostenv_api.go` 透传预设 ID
- [x] 前端「系统 → 宿主环境 → 安装源」

### P5 · 会话吊销

- [x] `server/auth_revoke.go`：吊销全部 / 当前会话，复用现有 `auth_sessions`
- [x] 路由 `POST /api/auth/revoke-all|revoke-current`
- [x] 侧边栏「吊销全部会话」确认弹窗

### P6 · 前端

- [x] `/system/egress`：Clash 节点、订阅、SSH 跳板、严格出口
- [x] `/system/proxypool/clash`：节点/分组/测速
- [x] `/system/diagnostics` 或设置页导出入口
- [x] `/system/hostenv` 安装源选择
- [x] `api.ts` / `types.ts` / 侧边栏

### P7 · 回归与发布

- [x] `go build ./...`、`go vet ./...`
- [x] 真实 PG `go test -p 1 ./...`
- [x] 前端 `tsc` + `NEXT_EXPORT=1 next build`
- [x] 代理链路：mihomo 安装 → 订阅 → 启动 → 严格出口失败 → 接管本机 Clash
- [x] 诊断包脱敏、SSH 跳板、安装源、会话吊销、RBAC 路由矩阵
- [x] PR + CI + WSL 冒烟；按需发 `v0.2.0`

## 风险

- 上游 `server/proxypool_*` 与 KAIYANG 现有文件差异大，按文件移植而不是覆盖。
- 路由新增必须补 `permissionForRequest`，默认 admin-only。
- `EGRESS_PROXY` 改名保留旧名兼容，避免已有 skills/脚本失效。
- mihomo 内核下载受网络影响；保留「本机 Clash 端口接管」与「服务器文件安装」两条路。
- SSH 跳板凭据只能经 vault 引用，API 不回传明文。
