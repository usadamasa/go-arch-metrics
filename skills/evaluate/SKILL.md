---
name: evaluate
description: >-
  Use when interpreting Go architecture metrics: comparing measured numbers against thresholds,
  classifying violations, and deciding what to fix first. Also covers the health of the
  `.go-arch-lint.yml` itself — when the config has grown long or components have become 1:1
  with packages. Triggers for requests like "依存ルールの健全性を評価して",
  ".go-arch-lint.yml が長大化した", "どこから直すべき", "この数値はまずいのか".
---

# 測定結果の評価

`go-arch-metrics:measure` が出した数値をしきい値と照らし、違反を分類して
優先順位を付ける。具体的な直し方は `go-arch-metrics:remediate`。

## 違反の分類

| カテゴリ | 影響 | 代表的な違反 |
|---------|------|------------|
| モジュール性 | 変更の伝播リスク | 依存方向の逆転, 循環参照 |
| テスト可能性 | テスト難易度の上昇 | 認知的複雑度 > 20, 関数長 > 100行 |
| 保守性 | 長期的な腐敗 | 保守性指数 < 20, 到達不能コード |

## 優先順位の基準

| 優先度 | 条件 | 理由 |
|--------|------|------|
| **High** | 依存方向の逆転 / 循環参照 | アーキテクチャ崩壊の根本原因。放置すると全体に波及 |
| **Medium** | 認知的複雑度 > 30 / 関数長 > 200行 | テストが実質書けない状態 |
| **Medium** | Public ratio > 0.6 / LOC 外れ値 | API surface 過大 / 責務過多の兆候 |
| **Medium** | Distance > 0.5 (アクション可能パッケージのみ) | パッケージの責務バランスが不明瞭 |
| **Low** | 保守性指数 15-19 / ネスト深さ 6-8 | 徐々に劣化するが即座の破綻はない |

初期導入時は既存コードのベースライン値を記録し、段階的に目標値へ近づける。
最初から全違反を 0 にしようとするのは逆効果。

## Distance を読む前に確認すること

D = |A + I - 1| > 0.5 のパッケージは、そのまま違反として数えない。
`zone` と `structurally_constrained` を先に見る:

```
D > 0.5 のパッケージ
  ├─ zone == "excluded" → 対処不要 (main / 孤立パッケージ)
  ├─ structurally_constrained == true → 長期的評価へ (remediate skill)
  └─ それ以外 → アクション可能: Zone 別対処へ (remediate skill)
```

Zone の定義と構造的制約の根拠は `references/thresholds.md`。

## `.go-arch-lint.yml` 自体の健全性

`go-arch-lint check` が答えるのは「コードが設定に違反していないか」だけで、
設定そのものが劣化していく方向は検出できない。設定がパッケージツリーの転記に
なっていないかは `analyze-arch-lint` で測る。詳細は `references/arch-lint-health.md`。

```bash
analyze-arch-lint <project-dir>            # 人が読む形
analyze-arch-lint --json <project-dir>     # 機械可読
analyze-arch-lint --strict <project-dir>   # しきい値超過で exit 1 (CI ゲート)
analyze-arch-lint --metrics                # 指標の一覧・しきい値・意味・対処
```

指標の一覧はドキュメント側に置かない。`--metrics` が正本。

## リファレンス

| ファイル | 内容 |
|---------|------|
| `references/thresholds.md` | 各しきい値の根拠、Zone 分類と構造的制約 |
| `references/arch-lint-health.md` | 依存ルール設定の劣化の見つけ方、compiler-scoped edge |

## 次のステップ

直す順番が決まったら `go-arch-metrics:remediate`。
しきい値そのものを設定へ反映するなら `go-arch-metrics:setup`。
