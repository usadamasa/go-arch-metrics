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

## 判断に使う情報

結果の読み方は、次の情報だけで決める。

- JSON の値。`analyze-modularity` の警告は `message` と `actionable` を読む
- この skill の本文 (下の表) と `references/` の判定条件
- `analyze-arch-lint --metrics` の出力
- `go-arch-metrics:measure` の `references/baseline.schema.json` と `references/output-examples.md`

この plugin の実装 (`cmd/` の Go ソース。module cache や plugin cache にあるものを含む) は読まない。
測定対象のプロジェクトのコードを読むことは、この制約の対象外。
実装から得た理解は版が変わると古くなり、memory に残ると後の判断を誤らせる。
文書に無い条件は memory に書かない。

上の情報で説明が付かない結果は、`usadamasa/go-arch-metrics` へ issue を立てる。
評価そのものは止めず、その結果は「文書で説明できない」と報告に書いて、ほかの結果の評価を続ける。

```bash
gh issue create -R usadamasa/go-arch-metrics --title "<ツール名>: <説明の付かない出力を 1 行で>" --body-file <path>
```

issue の本文に書くもの:

- plugin の版 (`claude plugin list` の `go-arch-metrics@usadamasa` の `version`)
- 説明の付かない出力。JSON ならキーのパスと値 (`modularity.warnings[].message` など)、標準出力なら
  その行
- 照らした文書の場所 (skill 名とファイル名、節の名前) と、そこに書かれた判定条件
- 出力と文書がどう合わないか (出るはずの警告が無い、無いはずの警告が出る、値の説明が付かない、
  文書に無いキーがある)

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
| **Medium** | 外から参照されない exported (`unreferenced_exported`) / LOC 外れ値 | API surface 過大 / 責務過多の兆候 |
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
  ├─ zone == "zone_of_pain" かつ volatility == "non_volatile" → 対処不要 (変更されない具象安定パッケージ)
  └─ それ以外 → アクション可能: Zone 別対処へ (remediate skill)
```

`analyze-modularity` の警告もこの分け方にそろえてある。警告ごとの扱いは次の節の表。

Zone の定義と構造的制約の根拠は `references/thresholds.md`。

## `--strict` で exit 1 になる警告

`analyze-modularity --strict` は、下の表で「なる」に当たる警告が 1 件でもあれば exit 1 で終わる。
JSON ではその警告に `actionable: true` が付き、件数が `summary.actionable_warnings` に入る。
「ならない」に当たる警告は JSON の `warnings` に載るだけで、終了コードを変えない。

| type | 警告が出るとき | `--strict` で exit 1 になるか |
|------|---------------|------------------------------|
| `unreferenced_exported` | `internal` のパッケージに、外から参照されない exported かテストだけが参照する exported がある | なる |
| `loc_outlier` | パッケージの LOC が `max(mean + 2σ, 1000)` を超える | なる |
| `constrained_loc_outlier` | 構造制約のパッケージに `loc_outlier` が出る。同じパッケージに 2 件並ぶ | なる |
| `zone_of_pain` | A < 0.5 かつ I < 0.5 | なる。ただし構造制約、非揮発、base から悪化していない、のどれかに当たればならない |
| `zone_of_uselessness` | A > 0.5 かつ I > 0.5 | なる。ただし base から悪化していなければならない。変更の多さは見ない |
| `high_distance` | D > 0.5 で、同じパッケージの Zone の警告が exit 1 になる | なる |
| `structurally_constrained` | パッケージが構造制約に当たる | ならない |
| `high_public_ratio` | public ratio が `--max-public-ratio` を超える | ならない |

Zone の警告が exit 1 にならないパッケージには `high_distance` を出さない。D は Zone の警告の `value` で見る。

表の「ただし」に出てくる条件は次のとおり。

- **構造制約**: ほかのパッケージを import せず (Ce=0)、ほかから import される (Ca>0) パッケージ。
  JSON では `structurally_constrained: true` になる。I が 0 に固まるので、A を上げる以外に D を動かせない。
  根拠は `references/thresholds.md` の「構造的制約」
- **非揮発**: 変更の少ないパッケージ。HEAD から遡った直近 200 commit (merge を除く) のうち、
  そのパッケージのコードを変えた commit が 5 未満なら非揮発 (`volatility: "non_volatile"`)、
  5 以上なら揮発 (`"volatile"`) になる。200 と 5 は `--volatility-window` と `--volatile-commits` で変える。
  shallow clone などで履歴を数えられない (`"unmeasured"`) ときは揮発として扱う。
  数え方は `references/thresholds.md` の「非揮発」
- **base から悪化していない**: `--baseline` を渡し、base にもあるパッケージの D が base 以下。
  `--baseline` を渡さないときと、base に無いか base で excluded のパッケージには効かない。
  規則は `references/thresholds.md` の「base との比較」

main パッケージと孤立パッケージ (`zone: "excluded"`) には Zone と Distance の警告を出さない。

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

指標の一覧はドキュメント側に置かない。一覧は `--metrics` の出力で見る。

## リファレンス

| ファイル | 内容 |
|---------|------|
| `references/thresholds.md` | 各しきい値の根拠、Zone 分類と構造的制約、非揮発と base との比較の規則 |
| `references/arch-lint-health.md` | 依存ルール設定の劣化の見つけ方、compiler-scoped edge |

## 次のステップ

直す順番が決まったら `go-arch-metrics:remediate`。
しきい値そのものを設定へ反映するなら `go-arch-metrics:setup`。
