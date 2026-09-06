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

Go の版は `go.mod`。lint が呼ぶ外部ツールは `aqua.yaml` が固定する。

```sh
brew install aquaproj/aqua/aqua   # 未導入なら
export PATH="${AQUA_ROOT_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/aquaproj-aqua}/bin:$PATH"
```

aqua は lazy install なので `aqua install` は要らない。`task` を起動した時点で必要な
ものだけ入る (CI も `aqua install` を呼んでいない)。先に全部入れておきたいときだけ
`aqua install -l`。

`bats` は aqua の管理外。`apt-get install bats` または `brew install bats-core`。

## 検証

| コマンド | 中身 | CI job |
|---------|------|--------|
| `task test` | `go test ./...` と `bats tests/` | go-test / bats |
| `task test-cover` | カバレッジ付き `go test` | go-test |
| `task lint` | golangci-lint, go-arch-lint, analyze-arch-lint --strict, govulncheck, gosec, analyze-modularity --strict, spm-go | go-lint |
| `task build` | `cmd/` の全バイナリを `bin/` へ | — |
| `shellcheck skills/*/scripts/*.sh` | skill の script | shellcheck |
| `claude plugin validate --strict .` | plugin manifest (`.claude-plugin/plugin.json` と `skills` も個別に通す) | plugin-validate |

PR を出す前にこの全部を通す。job の定義は `.github/workflows/ci.yaml`。

## つまずきどころ

- **`task lint` の `AQUA_POLICY_CONFIG`**: spm-go と go-arch-lint がローカルレジストリ
  (`aqua/registry.yaml`) 由来で、aqua v2 は標準レジストリ以外を既定で拒否する (code 002)。
  そのため Taskfile が `aqua/policy.yaml` を渡している。**絶対パスでなければならない** —
  go-arch-lint は各パッケージのディレクトリを cwd にして go を起動するので、相対パスだと
  `cmd/<pkg>/../aqua` を探しに行く。端末ごとの `aqua policy allow` は使わない
  (端末ローカルの状態を作らないため)
- **go-arch-lint は fork を使う**: `usadamasa/go-arch-lint@v1.18.1-walkdir.1`。upstream の
  v1.18.0 は `filepath.Walk` で全エントリを lstat するため、strict sandbox 内では `.env`
  の lstat 拒否で `failed to walk project tree` と落ちる。upstream の PR #90
  (Walk -> WalkDir) がリリースされたら `aqua.yaml` を標準レジストリ側へ戻す
- **`analyze-arch-lint --strict` が落ちたとき**: 指標の意味としきい値と対処は
  `analyze-arch-lint --metrics` が出す。ドキュメント側に表を書き写さない
- **skill を足したとき**: `.claude-plugin/plugin.json` の `skills` に追記する。
  追記しないと `claude plugin validate --strict .` は通っても plugin から見えない
