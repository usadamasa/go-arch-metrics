# go-arch-metrics

Go プロジェクトのモジュール性とテスト可能性を測り、改善するための Claude Code plugin。
「ソフトウェアアーキテクチャメトリクス (ISBN: 9784814400607)」の観点に基づく。
skill と、既存ツールがカバーしない領域を測る CLI を同梱する。

## インストール

[usadamasa/agents-marketplace](https://github.com/usadamasa/agents-marketplace) から配布する。

```sh
claude plugin marketplace add usadamasa/agents-marketplace
claude plugin install go-arch-metrics@usadamasa
```

測定に使うツールは測定対象のプロジェクト側に入れる。何が要るかと入れ方は
`go-arch-metrics:setup` skill が案内する。

## 5 ステップワークフロー

| Step | やること | skill |
| ---- | -------- | ----- |
| 1 | ツールを入れて `.golangci.yml` / `.go-arch-lint.yml` を置く | `go-arch-metrics:setup` |
| 2 | `arch-baseline` で現状の数値を取る | `go-arch-metrics:measure` |
| 3 | 数値をしきい値と照らして違反を分類する | `go-arch-metrics:evaluate` |
| 4 | 優先順位に従って直す | `go-arch-metrics:remediate` |
| 5 | CI に組み込んで再発を止める | `go-arch-metrics:setup` |

## 同梱する CLI

- **`analyze-arch-lint`**: `.go-arch-lint.yml` 自体の健全性。`go-arch-lint check` は
  コードが設定に違反していないかしか見ず、設定がパッケージツリーの転記へ劣化していく
  方向は検出できない。そこを数値にする。指標の一覧は `--metrics` の出力にある
- **`analyze-modularity`**: パッケージの API surface area (exported/unexported 比率、
  サイズ分布) を測る。宣言どうしの参照から Martin の Ca/Ce/A/I/D と Zone 分類も出す
- **`vulncheck`**: govulncheck をライブラリとして組み込んだもの。引数と exit code は govulncheck と同じ。
  違いは証明書の検証だけで、OS の証明書ストアに頼らず埋め込みの roots を使うので、trustd に届かない
  sandbox (Claude Code など) の中でも vuln.go.dev に届く
- **`arch-baseline`**: 測定に使うツールを一通り回し、サマリと JSON を出す。
  ツールの「指摘あり」では測定を続け、実行そのものの失敗では理由を出して非 0 で終わる

フラグと使い方は `go-arch-metrics:setup` の `references/tools.md`。

## 開発

`AGENTS.md` と `develop` skill (`.claude/skills/develop`)。clone したまま `claude --plugin-dir ./` で読める。

## License

MIT
