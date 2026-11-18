#!/usr/bin/env bash
# 一次性初始化 GitHub 仓库、推送 main/develop、配置分支保护。
# 前置：gh auth login -h github.com
set -euo pipefail

OWNER="3169417664"
REPO="kaiyang"
FULL="$OWNER/$REPO"

if ! gh auth status >/dev/null 2>&1; then
  echo "错误：gh 未登录或 token 失效。请先执行： gh auth login -h github.com" >&2
  exit 1
fi

if gh repo view "$FULL" >/dev/null 2>&1; then
  echo "仓库 $FULL 已存在，跳过创建"
  git remote get-url origin >/dev/null 2>&1 || git remote add origin "https://github.com/$FULL.git"
else
  echo "创建私有仓库 $FULL"
  gh repo create "$FULL" --private --source=. --remote=origin --push
fi

git push -u origin main
git push -u origin develop

protect() {
  local branch="$1"
  echo "配置 $branch 分支保护"
  local payload
  payload=$(cat <<JSON
{
  "required_status_checks": {"strict": true, "contexts": ["backend", "frontend", "gitleaks"]},
  "enforce_admins": true,
  "required_pull_request_reviews": {
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": true,
    "required_approving_review_count": 1
  },
  "restrictions": null,
  "required_linear_history": true,
  "allow_force_pushes": false,
  "allow_deletions": false
}
JSON
)
  printf '%s' "$payload" | gh api -X PUT "repos/$FULL/branches/$branch/protection" \
    -H "Accept: application/vnd.github+json" --input - >/dev/null
}

protect main
protect develop

gh api -X PATCH "repos/$FULL" -f has_wiki=false -f has_projects=false >/dev/null || true

echo "完成：https://github.com/$FULL"
