# go-arch-metrics

Go プロジェクトのモジュール性とテスト可能性を測り、改善するための Claude Code plugin。
「ソフトウェアアーキテクチャメトリクス (ISBN: 9784814400607)」の観点に基づく。

plugin としての 4 skill と、既存ツールがカバーしない領域を測る CLI 2 本を同梱する。

## インストール

### plugin

```
/plugin marketplace add usadamasa/go-arch-metrics
/plugin install go-arch-metrics@go-arch-metrics
```

開発中は `claude --plugin-dir ./` でも読める。

### CLI

`analyze-arch-lint` と `analyze-modularity` は測定対象のプロジェクト側に入れる。

```sh
go get -tool github.com/usadamasa/go-arch-metrics/cmd/analyze-arch-lint@latest
go get -tool github.com/usadamasa/go-arch-metrics/cmd/analyze-modularity@latest
go install tool
```

プロジェクトの `go.mod` を触りたくない場合は
`go install github.com/usadamasa/go-arch-metrics/cmd/analyze-arch-lint@latest` でも入る。

go-arch-lint / golangci-lint / gosec / govulncheck / spm-go は別途必要。
入れ方は `go-arch-metrics:setup` skill が案内する。

## 5 ステップワークフロー

| Step | やること | skill |
| ---- | -------- | ----- |
| 1 | ツールを入れて `.golangci.yml` / `.go-arch-lint.yml` を置く | `go-arch-metrics:setup` |
| 2 | `baseline.sh` で現状の数値を取る | `go-arch-metrics:measure` |
| 3 | 数値をしきい値と照らして違反を分類する | `go-arch-metrics:evaluate` |
| 4 | 優先順位に従って直す | `go-arch-metrics:remediate` |
| 5 | CI に組み込んで再発を止める | `go-arch-metrics:setup` |

measure と evaluate を分けているのは、measure が「ツールを回して JSON を出す」までで
判断を含まず、evaluate が「その JSON をどう読むか」を持つため。しきい値の変更や
新しい Zone の追加は evaluate だけを触れば済む。

## 同梱する CLI

### analyze-arch-lint

`.go-arch-lint.yml` 自体の健全性を測る。`go-arch-lint check` が見るのは
「コードが設定に違反していないか」だけで、設定がパッケージツリーの転記へ
劣化していく方向は検出できない。そこを数値にする。

```sh
analyze-arch-lint --metrics          # 指標の一覧・しきい値・意味・対処
analyze-arch-lint --strict <dir>     # CI ゲート (しきい値超過で exit 1)
analyze-arch-lint --json <dir>       # 機械可読
```

`go-arch-lint mapping --json` と `go list` を呼ぶので、どちらも PATH に要る。

### analyze-modularity

`go/ast` ベースでパッケージの API surface area を測る。spm-go がカバーしない
exported/unexported 比率とパッケージサイズ分布を算出する。
`--spm-json` で spm-go の出力を渡すと Zone 分類と構造的制約の判定まで行う。

```sh
analyze-modularity --max-public-ratio 0.6 --sigma 2.0 --min-symbols 10 .
analyze-modularity --strict .        # CI ゲート
```

## 開発

```sh
task test    # go test ./...
task lint    # golangci-lint, go-arch-lint, analyze-arch-lint, govulncheck, gosec, analyze-modularity, spm-go
task build   # bin/ へビルド
```

`task lint` は aqua で固定したツールを使う。`aqua install -l` で先に入れておく。

## License

MIT
