---
name: setup
description: >-
  Use when introducing Go architecture metrics tooling to a project: listing the required
  tools, writing `.golangci.yml` / `.go-arch-lint.yml`, and wiring the checks into CI.
  Triggers for requests like "set up go-arch-lint", "configure golangci-lint for metrics",
  "メトリクスを CI に入れたい", "アーキテクチャメトリクスを導入したい".
---

# ツール導入と設定ファイルの配置

「ソフトウェアアーキテクチャメトリクス (ISBN: 9784814400607)」の観点で Go プロジェクトを
測るための、必要なツールと設定ファイルの配置を扱う。

測定そのものは `go-arch-metrics:measure`、数値の解釈は `go-arch-metrics:evaluate`。

## 対象メトリクスとツールの対応

| カテゴリ | メトリクス | ツール |
|---------|-----------|--------|
| モジュール性 | パッケージ依存方向 | go-arch-lint |
| モジュール性 | 依存ルール設定自体の健全性 | analyze-arch-lint |
| モジュール性 | import 禁止リスト | golangci-lint/depguard |
| モジュール性 | Afferent/Efferent Coupling・抽象度・不安定度・距離 | spm-go |
| モジュール性 | Public/Private 関数比率・パッケージサイズ均一性 | analyze-modularity |
| テスト可能性 | 認知的複雑度・循環複雑度・関数長・ネスト深さ | golangci-lint (gocognit, gocyclo, cyclop, funlen, nestif) |
| 保守性 | 保守性指数 | golangci-lint (maintidx) |
| 静的解析 | 高度なバグ検出・未使用コード | golangci-lint (staticcheck) |
| セキュリティ | 既知の脆弱性・セキュリティ問題 | govulncheck, gosec |
| テスト品質 | テストカバレッジ | go test -cover |

しきい値は `go-arch-metrics:evaluate` が持つ。ここでは何を入れるかだけを決める。

## Step 1: 必要なコマンド

上の表のツールと `jq` が PATH にあること。入れ方はプロジェクトに任せる。

## Step 2: 設定ファイルを配置する

2 種類の設定ファイルをプロジェクトルートに作成する:

- **`.golangci.yml`** → テスト可能性・保守性メトリクス
  → テンプレートは `references/golangci-config.md`
- **`.go-arch-lint.yml`** → パッケージ依存方向ルール (モジュール性)
  → テンプレートは `references/arch-lint-config.md`
  → component はパッケージ単位ではなく役割単位で切る。既存設定の健全性評価は
    `go-arch-metrics:evaluate`

> **重要**: `.go-arch-lint.yml` のコンポーネント deps を設計する前に、実際の import を確認すること。
> プランと実態が乖離していると arch-lint 違反が大量に出る。
>
> ```bash
> # 各パッケージが実際に何を import しているか確認
> go list -f '{{.ImportPath}}: {{.Imports}}' ./...
>
> # main パッケージの import を確認 (エントリポイントは多くのパッケージを直接 import しがち)
> go list -f '{{.ImportPath}}: {{.Imports}}' .
> ```

### golangci-lint のバージョン

> **重要**: golangci-lint v1 と v2 は設定ファイルが非互換。**必ず v2 を使うこと。**
>
> | 項目 | v1 (旧) | v2 (現行) |
> |------|---------|----------|
> | linter 設定 | `linters-settings:` (トップレベル) | `linters.settings:` (linters 内) |
> | テスト除外 | `issues.exclude-rules:` | `linters.exclusions.rules:` |
> | JSON 出力 | `--out-format json` | `--output.json.path stdout` |
> | バージョン宣言 | `version: "1"` or なし | `version: "2"` |
>
> v1 形式のキーは v2 では **警告なしに無視される** ため、設定が効いていないことに気づきにくい。

## Step 3: CI に組み込む

CI で回すコマンドは `references/ci-integration.md`。

初期導入時はしきい値を既存コードの実測最大値の直上に置き、CI を緑にしてから
段階的に絞る。最初から全違反を 0 にしようとするのは逆効果。

## リファレンス

| ファイル | 内容 |
|---------|------|
| `references/tools.md` | 各ツールの概要と実行方法 |
| `references/golangci-config.md` | `.golangci.yml` テンプレートとカスタマイズ |
| `references/arch-lint-config.md` | `.go-arch-lint.yml` テンプレートとよくあるエラー |
| `references/ci-integration.md` | CI で回すコマンドと PR ゲート |

## 次のステップ

配置が済んだら `go-arch-metrics:measure` でベースラインを取る。
