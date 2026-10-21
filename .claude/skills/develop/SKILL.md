---
name: develop
description: >-
  Use when working on the go-arch-metrics repository itself: preparing the local toolchain
  and running the checks that CI runs (task test / task lint / bats / claude plugin validate).
  Triggers for requests like "task lint が落ちる", "CI と同じチェックをローカルで回したい",
  "このリポジトリの開発環境を用意して", "PR を出す前に検証して".
---

# このリポジトリの開発

`go-arch-metrics` 自体を触るときの環境準備と検証を扱う。ディレクトリのレイアウト、
skill を書くときの決めごと、リリース手順はリポジトリルートの `AGENTS.md`。

測定対象プロジェクト側にツールを入れる話は `go-arch-metrics:setup`。ここは
このリポジトリの開発者向け。

## 環境

Go と lint・テストのツール (`bats`、`shellcheck` を含む) は `flake.nix` の devShell が揃え、
`.envrc` (nix-direnv の `use flake`) が読み込む。
版は `flake.lock` が固定する nixpkgs で決まり、上げるときは `nix flake update`。
`flake.lock`・`go.mod`・GitHub Actions の更新は Dependabot が週次で PR を出す (`.github/dependabot.yml`)。
devShell の go は `go.mod` の `go` 行と同じ系列 (`go_1_26`) にし、`GOTOOLCHAIN=local` でずれを即座に落とす。

`analyze-modularity` / `analyze-arch-lint` は devShell に入れない。`task lint` が `go run ./cmd/...` で
このリポジトリのソースから動かす。

## 検証

task が何を回すかは `task --list` を見る (`Taskfile.yml` の `desc` が正本)。
ここでは CI のどの job に対応するかだけを持つ。

| コマンド | CI job |
|---------|--------|
| `task test` | go-test / bats |
| `task lint` | go-lint |
| `task build` | — |
| `shellcheck skills/*/scripts/*.sh` | shellcheck |
| `claude plugin validate --strict .` | plugin-validate |

PR を出す前にこの全部を通す。job の定義は `.github/workflows/ci.yaml`。

## つまずきどころ

- **`analyze-arch-lint --strict` が落ちたとき**: 指標の意味としきい値と対処は
  `analyze-arch-lint --metrics` が出す。ドキュメント側に表を書き写さない
- **配布用の skill (`skills/`) を足したとき**: `.claude-plugin/plugin.json` の `skills`
  に追記する。追記しないと `claude plugin validate --strict skills` は通っても plugin
  からは見えない。このリポジトリ自身のための skill (`.claude/skills/`) は plugin の
  外なので追記しない
