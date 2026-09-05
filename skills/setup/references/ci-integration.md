# CI 統合ガイド (GitHub Actions)

## ツールバージョン管理: aqua

CI でも同じ `aqua.yaml` を使ってツールバージョンを固定する。

### aqua.yaml (プロジェクトルート)

> **注意**: aqua は go-arch-lint の管理に主に使う。
> golangci-lint は GitHub Actions では `golangci/golangci-lint-action` が自前でインストールするため aqua 不要。
> ローカル環境では `go.mod` の `tool` directive でも可 (`references/tools.md`)。

```yaml
---
# yaml-language-server: $schema=https://raw.githubusercontent.com/aquaproj/aqua/main/json-schema/aqua-yaml.json
aqua_version: ">=2.0.0"

registries:
  - type: standard
    ref: v4.227.0  # 定期的に更新する

packages:
  - name: golangci/golangci-lint@v2.13.1   # golangci-lint v2 系を使う (v1 と設定非互換)
  - name: fe3dback/go-arch-lint@v1.18.0
```

spm-go は標準レジストリに存在しないため、ローカルレジストリの定義が別に要る。
このプラグインのリポジトリの `aqua/registry.yaml` と `aqua/policy.yaml` がそのまま使える。
`policy.yaml` は `AQUA_POLICY_CONFIG` 環境変数で渡す (端末ごとの `aqua policy allow` を
使わないのは、端末ローカルの許可状態を作らないため)。

go-arch-lint v1.18.0 は `filepath.Walk` でプロジェクトツリー全体を lstat するため、
サンドボックス環境ではプロジェクト直下の `.env` の lstat が拒否されて
`failed to walk project tree: lstat .../.env: operation not permitted` で落ちる。
修正は upstream の PR #90 (`Walk` → `WalkDir`) で、リリースされるまでは
同じ `aqua/registry.yaml` にある `usadamasa/go-arch-lint` の `go_build` 定義を
そのまま使ってタグを固定できる。

---

## GitHub Actions ワークフロー

### 方法 1: aqua でツールを統一管理 (推奨)

```yaml
# .github/workflows/arch-metrics.yml
name: Architecture Metrics

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  golangci-lint:
    name: golangci-lint (Testability & Maintainability)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6

      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod  # go.mod がサブディレクトリにある場合: go-version-file: cmd/go.mod
          cache: true

      - name: Setup aqua
        uses: aquaproj/aqua-installer@v3.1.2   # major だけの浮動タグは無いので patch まで書く
        with:
          aqua_version: v2.53.3

      - name: Run golangci-lint
        run: golangci-lint run --timeout 5m ./...

  go-arch-lint:
    name: go-arch-lint (Modularity)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6

      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
          cache: true

      - name: Setup aqua
        uses: aquaproj/aqua-installer@v3.1.2   # major だけの浮動タグは無いので patch まで書く
        with:
          aqua_version: v2.53.3

      - name: Check architecture rules
        run: go-arch-lint check ./...
```

aqua は lazy install なので、`aqua install` を明示的に呼ばなくてもコマンド起動時に入る。
先に全部入れておきたいなら `run: aqua install -l` のステップを足す。

### 方法 2: golangci-lint 公式 Action を使う (golangci-lint のみ)

```yaml
# .github/workflows/golangci-lint.yml
name: golangci-lint

on:
  push:
    branches: [main]
  pull_request:

jobs:
  golangci-lint:
    name: lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6

      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
          cache: false

      - name: golangci-lint
        uses: golangci/golangci-lint-action@v7
        with:
          version: v2.13.1   # v2 系を指定する。v1 を指定すると .golangci.yml が読めない
          # .golangci.yml を自動で読み込む
          args: --timeout 5m
```

---

## 全 linter を 1 ジョブにまとめる

`go.mod` の `tool` directive を使うと `go install` の対象を CI に列挙せずに済む:

```yaml
  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6

      - uses: actions/setup-go@v6
        with:
          go-version-file: go.mod
          cache: false

      - uses: aquaproj/aqua-installer@v3.1.2   # major だけの浮動タグは無いので patch まで書く
        with:
          aqua_version: v2.53.3

      - uses: arduino/setup-task@v2
        with:
          version: 3.x

      # 対象は go.mod の tool directive が持つので、ここで名前を列挙しない
      - name: Install repo-owned linters
        run: go install tool

      - name: Run all linters
        run: task lint
```

## カバレッジ計測

```yaml
      - name: go test with coverage
        run: go test -coverprofile=coverage.out ./...

      - name: Upload coverage
        uses: actions/upload-artifact@v7
        with:
          name: coverage
          path: coverage.out
```

---

## PR ゲートとしての活用

### 段階的導入

golangci-lint v2 では `issues.new` / `issues.new-from-rev` の設定キーが廃止された。
既存違反が多いプロジェクトでの段階的導入は、**しきい値を実測最大値の直上に置いて
CI を緑にし、そこから絞る** 方式で行う (`references/golangci-config.md` の
「しきい値の調整」)。差分だけを見たい場合は `golangci/golangci-lint-action` の
差分チェック機能を使う。

### 違反数レポートをコメントとして投稿

```yaml
- name: Run golangci-lint with JSON output
  run: |
    golangci-lint run --output.json.path stdout ./... 2>/dev/null > lint-result.json || true

- name: Post lint summary
  uses: actions/github-script@v7
  with:
    script: |
      const fs = require('fs');
      const result = JSON.parse(fs.readFileSync('lint-result.json', 'utf8'));
      const issues = result.Issues || [];
      const summary = issues.reduce((acc, issue) => {
        acc[issue.FromLinter] = (acc[issue.FromLinter] || 0) + 1;
        return acc;
      }, {});
      const body = Object.entries(summary)
        .map(([linter, count]) => `- ${linter}: ${count} violations`)
        .join('\n');
      github.rest.issues.createComment({
        issue_number: context.issue.number,
        owner: context.repo.owner,
        repo: context.repo.repo,
        body: `## Architecture Metrics\n\n${body || 'No violations found!'}`
      });
```

---

## Taskfile への統合

```yaml
version: '3'

tasks:
  lint:
    desc: "全静的解析を実行"
    cmds:
      - golangci-lint run --timeout 5m ./...
      - go-arch-lint check
      - analyze-arch-lint --strict .
      - govulncheck ./...
      - gosec ./...
      - analyze-modularity --strict .
      - spm-go all
```

ベースライン測定 (`baseline.sh`) はプラグインのディレクトリに置かれ、パスが
プラグインのバージョンによって変わるため Taskfile には書かない。
`go-arch-metrics:measure` から実行する。
