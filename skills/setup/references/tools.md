# ツール詳細リファレンス

出力例は `go-arch-metrics:measure` の `references/output-examples.md`、
しきい値の根拠は `go-arch-metrics:evaluate` の `references/thresholds.md` にある。

## ツール管理方針

- **原則: `go.mod` の `tool` directive** に宣言し `go install tool` で入れる
- **オプション: aqua** で go 以外のものも含めて固定したい場合に使用

`go get -tool <パス>` と `go install tool` は各ツールの節に書いてある。
一覧はどこにも置かない (置くと `go.mod` とずれる)。

### aqua (オプション: バージョン固定)

バージョンを厳密に固定したい場合は aqua で管理できる。

```bash
brew install aquaproj/aqua/aqua
export PATH="${AQUA_ROOT_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/aquaproj-aqua}/bin:$PATH"
aqua install
```

`aqua.yaml` の書き方は `references/ci-integration.md`。
spm-go は aqua の標準レジストリに存在しないため、ローカルレジストリの定義が要る
(このプラグインのリポジトリの `aqua/registry.yaml` がそのまま使える)。

---

## golangci-lint

### 概要

複数の linter をまとめて実行する Go 静的解析ツール。
アーキテクチャメトリクス向けには以下の linter を有効化する:

| Linter | 計測するメトリクス |
|--------|------------------|
| `gocognit` | 認知的複雑度 |
| `gocyclo` | 循環複雑度 (McCabe) |
| `cyclop` | 循環複雑度 (パッケージ全体も対象) |
| `funlen` | 関数の行数・文数 |
| `nestif` | ネストの深さ |
| `maintidx` | 保守性指数 |
| `staticcheck` | 高度なバグ検出・未使用コード (U1000 系) |
| `depguard` | import 禁止リスト |

しきい値と根拠は `go-arch-metrics:evaluate` の `references/thresholds.md`。

> **注意**: `deadcode` は golangci-lint v2 に存在しない。`golangci-lint run --enable deadcode` を実行すると `Error: unknown linters: 'deadcode'` になる。未使用コードの検出は `staticcheck` の U1000 系チェックが代替する。

### インストールと実行

```bash
# go.mod へ宣言して入れる
go get -tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint
go install tool

# 実行
golangci-lint run ./...

# 特定 linter のみ
golangci-lint run --enable-only gocognit,gocyclo ./...

# JSON 出力 (v2)
golangci-lint run --output.json.path stdout ./... 2>/dev/null
```

---

## go-arch-lint

### 概要

パッケージ間の依存方向を `.go-arch-lint.yml` で宣言し、違反を検出するツール。
レイヤードアーキテクチャ・クリーンアーキテクチャの依存方向を強制できる。

### インストールと実行

```bash
# go.mod へ宣言して入れる
go get -tool github.com/fe3dback/go-arch-lint
go install tool

# 実行 (プロジェクトルートから)
go-arch-lint check ./...

# JSON 出力
go-arch-lint check --json-output ./...

# 依存関係のグラフ生成 (graphviz 要)
go-arch-lint graph ./... | dot -Tsvg > arch.svg
```

---

## govulncheck

### 概要

Go の既知の脆弱性データベース (vuln.go.dev) に基づき、プロジェクトの依存関係に
脆弱性がないかスキャンするツール。Go チーム公式。

### インストールと実行

```bash
# go.mod へ宣言して入れる
go get -tool golang.org/x/vuln/cmd/govulncheck
go install tool

# 実行
govulncheck ./...
```

---

## gosec

### 概要

Go ソースコードのセキュリティ問題を検出する静的解析ツール。
SQL インジェクション、ハードコードされた認証情報、弱い暗号等を検出する。

### インストールと実行

```bash
# go.mod へ宣言して入れる
go get -tool github.com/securego/gosec/v2/cmd/gosec
go install tool

# 実行
gosec ./...
```

---

## spm-go

### 概要

Robert C. Martin の Package Metrics (Ca/Ce/A/I/D) を Go パッケージに適用するツール。
パッケージの安定性・抽象度・Main Sequence からの距離を定量評価する。

| メトリクス | 説明 |
|-----------|------|
| Afferent Coupling (Ca) | このパッケージに依存するパッケージ数 |
| Efferent Coupling (Ce) | このパッケージが依存するパッケージ数 |
| Abstractness (A) | 抽象型 / (抽象型 + 具象型) |
| Instability (I) | Ce / (Ca + Ce) |
| Distance (D) | \|A + I - 1\| (Main Sequence からの距離) |

Zone 分類と構造的制約の扱いは `go-arch-metrics:evaluate`。

### インストールと実行

```bash
# go.mod へ宣言して入れる
go get -tool github.com/fdaines/spm-go
go install tool

# 全メトリクスを表示
spm-go all

# 個別サブコマンド
spm-go instability
spm-go abstractness
spm-go distance
spm-go packages
spm-go dependencies
```

---

## analyze-modularity

### 概要

`go/ast` ベースでパッケージの API surface area を測定するカスタム CLI。
spm-go がカバーしない exported/unexported 比率とパッケージサイズ分布を算出する。

| メトリクス | 説明 |
|-----------|------|
| Public ratio | exported / (exported + unexported) |
| LOC 外れ値 | パッケージ LOC > mean + 2σ |
| Exported funcs/methods | 公開関数・メソッド数 |
| Exported types | 公開型 (struct/interface) 数 |

### インストールと実行

```bash
# go.mod へ宣言して入れる
go get -tool github.com/usadamasa/go-arch-metrics/cmd/analyze-modularity@latest
go install tool

# プロジェクトの go.mod を触りたくない場合
go install github.com/usadamasa/go-arch-metrics/cmd/analyze-modularity@latest

# 実行
analyze-modularity <directory>...

# フラグ
analyze-modularity --max-public-ratio 0.6 --sigma 2.0 --min-symbols 10 .

# JSON 出力を jq で整形
analyze-modularity . | jq '.summary'
analyze-modularity . | jq '.warnings'
```

---

## analyze-arch-lint

### 概要

`.go-arch-lint.yml` 自体の健全性を測るカスタム CLI。go-arch-lint check が見るのは
「コードが設定に違反していないか」だけで、設定がパッケージツリーの転記へ劣化していく
方向は検出できない。そこを数値にする。観点は `go-arch-metrics:evaluate`。

**指標の一覧・しきい値・意味・対処は `analyze-arch-lint --metrics` が出す。**
ここにもドキュメント側にも表を置かない。

### インストールと実行

```bash
# go.mod へ宣言して入れる (analyze-modularity と同じモジュールにある)
go get -tool github.com/usadamasa/go-arch-metrics/cmd/analyze-arch-lint@latest
go install tool

# プロジェクトの go.mod を触りたくない場合
go install github.com/usadamasa/go-arch-metrics/cmd/analyze-arch-lint@latest

# 実行
analyze-arch-lint <project-dir>

# 指標の一覧・しきい値・意味・対処
analyze-arch-lint --metrics

# CI ゲート (しきい値超過で exit 1、違反ごとに意味と対処を stderr へ)
analyze-arch-lint --strict <project-dir>

# JSON 出力
analyze-arch-lint --json <project-dir> | jq '.findings'
```

`go-arch-lint mapping --json` と `go list` を呼ぶので、どちらも PATH に要る。
