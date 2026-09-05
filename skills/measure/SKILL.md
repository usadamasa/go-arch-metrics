---
name: measure
description: >-
  Use when asked to measure the current architecture quality of a Go project — running the
  metrics tools and collecting the numbers without judging them.
  Triggers for requests like "measure code quality", "ベースラインを取って",
  "アーキテクチャメトリクスを測って", "今の複雑度どれくらい".
---

# ベースライン測定

Go プロジェクトの現状のメトリクスを取る。数値の解釈と違反の優先順位付けは
`go-arch-metrics:evaluate` が持つ。ここは「ツールを回して数値を出す」までを扱う。

## エージェント行動制約

> ツールが未インストールの場合、手動計算や代替手段で回避してはならない。
> baseline.sh のエラー出力に表示されるインストールコマンドをユーザーに提示し、
> インストール後に再実行すること。
>
> 禁止: wc -l で LOC 判定、spm-go なしで Distance 推定、
> analyze-modularity なしで public ratio 集計、ツールエラーの無視

ツールが入っていないときの導入手順は `go-arch-metrics:setup`。

## 実行

```bash
bash "${CLAUDE_PLUGIN_ROOT}/skills/measure/scripts/baseline.sh" ./
```

引数は Go プロジェクトのルート (`go.mod` があるディレクトリ)。
全ツールがインストール済みであることが前提で、未インストール時はエラー終了する。

出力は 2 つ:

- 標準出力のサマリ (違反件数と代表的な違反)
- `baseline-YYYYMMDD_HHMMSS.json` (全ツールの生の結果)

JSON は `go-arch-metrics:evaluate` がそのまま読む。測定日時が入るので、
改善の前後を比べるときは消さずに残す。

## 対象メトリクス早見表

| カテゴリ | メトリクス | ツール | しきい値 |
|---------|-----------|--------|---------|
| モジュール性 | パッケージ依存方向 | go-arch-lint | 違反 = 0 |
| モジュール性 | 依存ルール設定自体の健全性 | analyze-arch-lint | `analyze-arch-lint --metrics` が出す |
| モジュール性 | import 禁止リスト | golangci-lint/depguard | 指定パッケージ = 0 |
| テスト可能性 | 認知的複雑度 | gocognit | ≤ 20 |
| テスト可能性 | 循環複雑度 | gocyclo | ≤ 20 |
| テスト可能性 | 関数の長さ | funlen | ≤ 100行 / 60文 |
| テスト可能性 | ネストの深さ | nestif | ≤ 8 (導入時推奨) → 目標 ≤ 5 |
| 保守性 | 保守性指数 | maintidx | ≥ 20 |
| 静的解析 | 高度なバグ検出・未使用コード | staticcheck | デフォルト有効 (U1000 系で未使用コードをカバー) |
| セキュリティ | 既知の脆弱性 | govulncheck | 違反 = 0 |
| セキュリティ | セキュリティ問題 | gosec | 違反 = 0 (CI ゲート) |
| テスト品質 | テストカバレッジ | go test -cover | ≥ 60% (段階的に引き上げ) |
| モジュール性 | Afferent/Efferent Coupling | spm-go | 参考値 (しきい値なし) |
| モジュール性 | 抽象度・不安定度・距離 | spm-go | アクション可能 Distance ≤ 0.5 |
| モジュール性 | Public/Private 関数比率 | analyze-modularity | public ratio ≤ 0.6 |
| モジュール性 | パッケージサイズ均一性 | analyze-modularity | LOC ≤ mean + 2σ |

しきい値の根拠と、Distance の「アクション可能」の定義は `go-arch-metrics:evaluate`。

## 個別に測る

baseline.sh を通さず 1 つのツールだけ回したいときは
`go-arch-metrics:setup` の `references/tools.md` に各ツールの実行方法がある。
出力の読み方は `references/output-examples.md`。

## 次のステップ

数値が出たら `go-arch-metrics:evaluate` でしきい値と照らす。
