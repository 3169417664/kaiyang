# 贡献指南 · KAIYANG

## 分支模型（Git Flow-lite）

| 分支 | 用途 | 来源 | 合并到 |
|---|---|---|---|
| `main` | 可发布版本 | — | — |
| `develop` | 集成分支 | `main` | — |
| `feature/<issue>-<slug>` | 新功能 | `develop` | `develop` |
| `fix/<issue>-<slug>` | 缺陷修复 | `develop` | `develop` |
| `chore/<slug>` / `docs/<slug>` | 工程/文档 | `develop` | `develop` |
| `release/vX.Y.Z` | 发布准备 | `develop` | `main` + `develop` |
| `hotfix/<issue>-<slug>` | 线上紧急修复 | `main` | `main` + `develop` |

- `main`、`develop` 禁止直接 push；必须 PR + 至少 1 个 approval + CI 全绿。
- 合并方式统一 squash；合并后删除源分支。
- 一个 PR 只做一个模块/一个阶段子项；不要把多个阶段混在一个 PR。

## 提交信息（Conventional Commits）

```
feat(review): add proposed intent state
fix(auth): reject legacy username
chore(repo): add GitHub Actions CI
docs(plan): update phase 3 checklist
```

- scope 用模块名：`auth`、`rbac`、`review`、`planner`、`worker`、`report`、`knowledge`、`credentials`、`topology`、`web`、`ci`。
- 中文描述可用；首行不超过 72 字符。

## PR 检查清单

- [ ] 变更点与影响面写清楚（含新增/修改的表、路由、工具）。
- [ ] 数据库迁移只追加 `§FUSION-*` 段；CHECK 变更使用 DO 块；一次性迁移有 settings flag。
- [ ] 新工具已注册到 `PlannerTools` / `WorkerTools` / `MainAgentTools` / `BuiltinToolSeeds`。
- [ ] 权限映射已补齐；未映射路由默认 admin-only。
- [ ] 提示词改动保留 Pojun 红队硬尾；模板覆盖链有测试。
- [ ] 没有任何 key/token/密码/客户数据进入仓库。
- [ ] 本地通过：`go build ./...`、`go test -p 1 ./...`、`cd web && npx tsc --noEmit`、`NEXT_EXPORT=1 npx next build`。
- [ ] 前端变更附截图；后端变更附 curl/测试输出。

## 测试

```bash
# 后端（真实 PostgreSQL，-p 1 不能去）
docker run -d --name kaiyang-ci-pg -e POSTGRES_PASSWORD=pojuntest \
  -e POSTGRES_USER=kaiyang -e POSTGRES_DB=kaiyang_test -p 15432:5432 postgres:16-alpine
POJUN_PG_DSN="postgres://kaiyang:pojuntest@127.0.0.1:15432/kaiyang_test?sslmode=disable" \
  go test -p 1 ./... -count=1

# 前端
cd web && npx tsc --noEmit
cd web && NEXT_EXPORT=1 npx next build
```

> 环境变量暂时保留 `POJUN_*` 前缀以兼容存量部署；后续版本再提供 `KAIYANG_*` 别名。

## 发布

1. 从 `develop` 切 `release/vX.Y.Z`，只修发布问题。
2. 更新 `CHANGELOG.md` 与版本号。
3. PR 到 `main`，CI 全绿后 squash 合并。
4. `git tag vX.Y.Z && git push origin vX.Y.Z`，Release workflow 自动出多平台包与 GHCR 镜像。
5. 反向合并 `main` 到 `develop`。

## 安全与合规

- 仅用于自有系统、隔离靶场或已获得明确书面授权的目标。
- 禁止提交真实客户数据、扫描结果、凭据、API key。
- 漏洞报告见 `SECURITY.md`。
