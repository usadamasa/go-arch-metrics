---
name: remediate
description: >-
  Use when fixing Go architecture metric violations: inverted dependencies, high cognitive
  complexity, deep nesting, oversized public API surface, packages far from the Main Sequence,
  and `.go-arch-lint.yml` configs that have degraded into a copy of the package tree.
  Triggers for requests like "improve testability", "public ratio を下げたい",
  "モジュール化の目的から逸脱している", "循環参照を解消したい".
---

# 違反の是正

`go-arch-metrics:evaluate` が付けた優先順位に従って、カテゴリ別に直す。

## カテゴリ別の入口

| 違反 | 参照先 |
|------|--------|
| 依存方向の逆転 / 循環参照 | `references/remediation.md`「モジュール性違反の是正」 |
| 認知的複雑度 / 関数長 / ネスト深さ | `references/remediation.md`「テスト可能性違反の是正」 |
| コード重複 | `references/remediation.md`「コード重複の是正」 |
| 保守性指数 / 到達不能コード | `references/remediation.md`「保守性違反の是正」 |
| Public ratio / LOC 外れ値 / Distance | `references/remediation.md`「パッケージ責務の是正」 |
| `.go-arch-lint.yml` が長大化した | `references/arch-lint-remediation.md` |

## 段階的改善のロードマップ

```
Sprint 1: ベースライン測定と High 優先度の解消
  → 依存方向の逆転をすべて解消
  → 循環複雑度 > 50 の関数を優先的にリファクタリング

Sprint 2: テスト可能性の改善
  → 認知的複雑度 > 30 の関数を分割
  → テストカバレッジを測定し、複雑な関数のテストを追加

Sprint 3: しきい値の段階的引き下げ
  → golangci-lint の設定を 30 → 25 → 20 と段階的に厳しくする
  → CI でのチェックを有効化

Sprint 4: 保守性の安定化
  → maintidx 違反の解消
  → deadcode の削除
```

しきい値を設定ファイルへ反映する手順は `go-arch-metrics:setup`。

## 次のステップ

直したら `go-arch-metrics:measure` でもう一度測り、ベースラインと比べる。
