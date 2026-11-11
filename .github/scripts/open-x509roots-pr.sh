#!/usr/bin/env bash
# update-x509roots.sh の変更を固定のブランチへ push し、更新 PR を開く。開いた PR があれば push で更新するだけにする。
# GH_TOKEN は PR の CI を起動できるトークン (GITHUB_TOKEN で開いた PR では ci が走らない) を渡す。
set -euo pipefail

readonly BRANCH="x509roots-fallback-update"

for name in GH_TOKEN GITHUB_REPOSITORY; do
  if [ -z "${!name:-}" ]; then
    printf 'ERROR: %s が空です\n' "$name" >&2
    exit 1
  fi
done

version=$(go list -m -f '{{.Version}}' golang.org/x/crypto/x509roots/fallback)
readonly version
readonly title="chore(deps): x509roots/fallback を ${version} に上げる"

git config user.name "github-actions[bot]"
git config user.email "41898283+github-actions[bot]@users.noreply.github.com"
git switch -C "$BRANCH"
git add go.mod go.sum flake.nix
git commit -m "$title"
git push --force "https://x-access-token:${GH_TOKEN}@github.com/${GITHUB_REPOSITORY}.git" "HEAD:refs/heads/${BRANCH}"

open_pr=$(gh pr list --repo "$GITHUB_REPOSITORY" --head "$BRANCH" --state open --json number --jq '.[0].number // empty')
if [ -n "$open_pr" ]; then
  gh pr edit "$open_pr" --repo "$GITHUB_REPOSITORY" --title "$title"
  printf '開いている PR #%s を更新しました\n' "$open_pr"
  exit 0
fi

gh pr create --repo "$GITHUB_REPOSITORY" --base main --head "$BRANCH" \
  --title "$title" --assignee usadamasa --label dependencies \
  --body "vulncheck が埋め込む roots (golang.org/x/crypto/x509roots/fallback) を ${version} に上げ、flake.nix の vendorHash を付け直す。.github/workflows/update-x509roots.yaml が開いた PR。"
