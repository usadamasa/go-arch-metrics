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
| モジュール性 | Afferent/Efferent Coupling・抽象度・不安定度・距離 | analyze-modularity |
| モジュール性 | Public/Private 関数比率・パッケージサイズ均一性 | analyze-modularity |
| テスト可能性 | 認知的複雑度・循環複雑度・関数長・ネスト深さ | golangci-lint (gocognit, gocyclo, cyclop, funlen, nestif) |
| 保守性 | 保守性指数 | golangci-lint (maintidx) |
| 静的解析 | 高度なバグ検出・未使用コード | golangci-lint (staticcheck) |
| セキュリティ | 既知の脆弱性・セキュリティ問題 | vulncheck (govulncheck の組み込み), gosec |
| テスト品質 | テストカバレッジ | go test -cover |

しきい値は `go-arch-metrics:evaluate` が持つ。ここでは何を入れるかだけを決める。

## Step 1: 必要なコマンド

上の表のツールが、PATH か go.mod の tool directive にあること (`go` と `jq` は PATH に要る)。
入れ方はプロジェクトに任せる。`baseline.sh` は PATH を先に見て、無ければ tool directive から `go tool` で呼ぶ。

`analyze-modularity`・`analyze-arch-lint`・`vulncheck` の 3 本は、この plugin のリポジトリの flake が
`packages.<system>.default` として出す。nix を使うプロジェクトは flake input で参照するだけにする。
nix を使わないプロジェクトは go.mod の tool directive に足せる
(どちらの書き方も `references/tools.md` の「この plugin の CLI を入れる」)。

### 以前の版で buildGoModule を書いたプロジェクト

以前は利用側の `flake.nix` で、この plugin の CLI を `buildGoModule` でソースからビルドしていた。
この形では `vendorHash`・`subPackages`・テスト用の設定を利用側が持つことになり、本体の変更のたびに利用側が壊れる。
`buildGoModule` の定義を消し、flake input の `packages.<system>.default` に置き換える。

### 以前の版で入れた spm-go を外す

以前の版は Afferent/Efferent Coupling・抽象度・不安定度・距離を fdaines/spm-go で測っていた。
今は `analyze-modularity` が `go list` から算出するので spm-go は要らない。
以前の手順で導入したプロジェクトでは、次の 2 箇所から spm-go を消す:

- ツール一覧 (aqua.yaml、flake.nix、`go install` の手順など)
- CI のコマンド (`spm-go all`)

残っていてもエラーにはならないが、使われないツールが残り続ける。

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

## バージョンを上げる

plugin (skill と `baseline.sh`) と CLI 3 本は同じリポジトリから出ていて、JSON のキーや警告の判定は
両方がそろっている前提で書いてある。片方だけ上げない。

1. GitHub Release (または CHANGELOG.md) で、上げる範囲の変更を読む。JSON のキー、しきい値、
   `--strict` の判定が変わっていれば、以前の測定結果とはそのまま比べられない
2. plugin を上げる: `claude plugin update go-arch-metrics@usadamasa` (反映には Claude Code の再起動が要る)
3. CLI を同じ版に上げる。marketplace の登録は ref を指定しておらず、plugin は default branch から入るので、
   CLI も default branch の最新にそろえる
   - flake input で参照している: `nix flake update go-arch-metrics` で flake.lock を進める
   - go.mod の tool directive に入れている: `go get github.com/usadamasa/go-arch-metrics@latest`
     (3 本は同じモジュールなので 1 回で全部上がる)
4. CI を回し、`go-arch-metrics:measure` で測り直す。前回から変わった数値は、手順 1 の変更による差か、
   コードの変化による差かを分けて読む

## リファレンス

| ファイル | 内容 |
|---------|------|
| `references/tools.md` | 各ツールの概要と実行方法 |
| `references/golangci-config.md` | `.golangci.yml` テンプレートとカスタマイズ |
| `references/arch-lint-config.md` | `.go-arch-lint.yml` テンプレートとよくあるエラー |
| `references/ci-integration.md` | CI で回すコマンドと PR ゲート |

## 次のステップ

配置が済んだら `go-arch-metrics:measure` でベースラインを取る。
