# しきい値の根拠と Zone 分類

## しきい値の根拠

| しきい値 | 根拠 |
|---------|------|
| 認知的複雑度 ≤ 20 | Sonar 社の研究: 20 超えで理解コストが急上昇 |
| 循環複雑度 ≤ 20 | McCabe の原論文: 10 が理想、20 が実用上限 |
| 関数長 ≤ 100行 | Clean Code: 関数は画面に収まる長さが理想 |
| ネストの深さ ≤ 5 | 目標値。既存コードへの一括適用は 5-8 から始め、段階的に絞る |
| 保守性指数 ≥ 20 | Microsoft の定義: 0-9 は保守困難、10-19 は低品質 |
| Distance ≤ 0.5 (アクション可能パッケージのみ) | Robert C. Martin の D' = \|A+I-1\|。Ce=0 の葉パッケージ (構造的制約)、非揮発の Zone of Pain、main パッケージは除外して集計。Martin (Agile PPP ch.28): 非揮発性の具象安定パッケージは Zone of Pain でも harmless |
| Public ratio ≤ 0.6 | API surface が大きすぎると変更影響範囲が拡大する。シンボル数 < `--min-symbols` (デフォルト 10) のパッケージは比率メトリクスが統計的に不安定なためスキップ |
| LOC ≤ mean + 2σ | 統計的外れ値は過大な責務の兆候 (正規分布の 95% 信頼区間外) |

初期導入時は既存コードのベースライン値を記録し、段階的に目標値に近づけること。
最初から全違反を 0 にしようとするのは逆効果。

---

## Zone 分類と構造的制約

Robert C. Martin の Distance メトリクス D = |A + I - 1| を解釈する際、
パッケージの (A, I) 座標に基づく Zone 分類と構造的制約を考慮する必要がある。

### Zone 分類

| Zone | 条件 | 解釈 |
|------|------|------|
| **Zone of Pain** | A < 0.5 かつ I < 0.5 | 具象的で安定。変更コストが高い |
| **Zone of Uselessness** | A > 0.5 かつ I > 0.5 | 抽象的で不安定。使われていない抽象 |
| **Main Sequence** | 上記以外 (A + I ≈ 1) | バランス良好。理想状態 |
| **Excluded** | main パッケージ or Ca=0 かつ Ce=0 (孤立) | D 集計対象外 |

### 構造的制約 (Structurally Constrained)

Ce=0 かつ Ca>0 の葉パッケージは、トポロジー上 I=0 に固定される。
このとき A=0 なら D=1.0 になるが、これは設計上の問題ではなく構造的制約による必然である。

Martin (Agile PPP ch.28) は、非揮発性 (変更頻度が低い) の具象安定パッケージは
Zone of Pain にあっても **harmless** だと述べている。`internal/cache`, `internal/config`
のようなユーティリティパッケージがこれに該当する。

baseline.sh はこれらのパッケージを `structurally_constrained: true` としてマークし、
D 集計 (`actionable_*`) から除外する。同じ基準で、構造制約のパッケージに付く `zone_of_pain` と
`structurally_constrained` の警告は `actionable: false` になり、`analyze-modularity --strict` の失敗に数えない。

構造制約パッケージでもリファクタリングを検討すべきケースの判断基準は
`go-arch-metrics:remediate`。

### 非揮発 (Non-volatile)

Martin の「非揮発」は、構造制約でない Zone of Pain のパッケージにも当てはまる。
`analyze-modularity` は git の履歴から変更頻度を測り、非揮発の Zone of Pain を actionable から外す。

- 窓: HEAD から辿れる直近 200 commit (merge を除く)。`--volatility-window` で変える
- 数え方: パッケージのディレクトリ直下の、`_test.go` でない `.go` を触った commit を 1 回と数える
- しきい値: 窓の中で 5 commit 以上触られたら揮発 (`volatile`)、それ未満は非揮発 (`non_volatile`)。
  `--volatile-commits` で変える
- 測れないとき (`unmeasured`): shallow clone、git リポジトリでない、commit が無い。このときは揮発として扱い、
  `--strict` の判定を緩めない。CI では `actions/checkout` に `fetch-depth: 0` を付ける

Martin は数値を示していないので、200 と 5 は目安。agents-daemon の実例では、ほぼ触られない `dreamio`
(263 commit 中 1 回) と頻繁に変わる `sessionstate` (30 回) が、この値で両側に分かれる。

日数ではなく commit 数で窓を切るのは、HEAD が同じなら結果も同じにするため。日数で切ると、コードが同じでも
日がたつだけで CI の判定が変わる。PR のブランチでは窓に PR 自身の commit も入るので、PR の中で同じパッケージを
何度も触ると揮発に変わり、その PR の `--strict` が落ちうる。

非揮発の Zone of Pain に付く `zone_of_pain` は `actionable: false` になり、`high_distance` は出さない
(D は `zone_of_pain` の `value` が持つ)。`summary.main_sequence.non_volatile_zone_of_pain_count` に数え、`actionable_*` からは除く。
このため、この扱いが入る前と比べて `actionable_count` の値が下がることがある。
Zone of Uselessness は使われていない抽象で、変更頻度と関係ないので外さない。

### Go 固有の考慮事項

- **main パッケージ**: Go では `main` パッケージは常に Ca=0 (他パッケージから import されない) であり、I=1 に固定される。エントリポイントとしての性質上、D 集計に含めるのは不適切
- **孤立パッケージ**: Ca=0 かつ Ce=0 のパッケージは、プロジェクト内の他パッケージとの結合がなく、メトリクスの意味をなさないため除外する
