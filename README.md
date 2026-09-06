# go-arch-metrics

Go プロジェクトのモジュール性とテスト可能性を測り、改善するための Claude Code plugin。
「ソフトウェアアーキテクチャメトリクス (ISBN: 9784814400607)」の観点に基づく。
4 つの skill と、既存ツールがカバーしない領域を測る CLI 2 本を同梱する。

## インストール

```
/plugin marketplace add usadamasa/go-arch-metrics
/plugin install go-arch-metrics@go-arch-metrics
```

測定に使う CLI (`analyze-arch-lint` / `analyze-modularity`) と外部ツール
(go-arch-lint / golangci-lint / gosec / govulncheck / spm-go) は測定対象の
プロジェクト側に入れる。手順は `go-arch-metrics:setup` skill が案内する。

## 5 ステップワークフロー

| Step | やること | skill |
| ---- | -------- | ----- |
| 1 | ツールを入れて `.golangci.yml` / `.go-arch-lint.yml` を置く | `go-arch-metrics:setup` |
| 2 | `baseline.sh` で現状の数値を取る | `go-arch-metrics:measure` |
| 3 | 数値をしきい値と照らして違反を分類する | `go-arch-metrics:evaluate` |
| 4 | 優先順位に従って直す | `go-arch-metrics:remediate` |
| 5 | CI に組み込んで再発を止める | `go-arch-metrics:setup` |

## 同梱する CLI

- **`analyze-arch-lint`** — `.go-arch-lint.yml` 自体の健全性。`go-arch-lint check` は
  コードが設定に違反していないかしか見ず、設定がパッケージツリーの転記へ劣化していく
  方向は検出できない。そこを数値にする。指標は `--metrics` が正本
- **`analyze-modularity`** — `go/ast` によるパッケージの API surface area。spm-go が
  カバーしない exported/unexported 比率とパッケージサイズ分布

フラグと使い方は `go-arch-metrics:setup` の `references/tools.md`。

## 開発

`AGENTS.md` と `develop` skill (`.claude/skills/develop`)。clone したまま `claude --plugin-dir ./` で読める。

## License

MIT
