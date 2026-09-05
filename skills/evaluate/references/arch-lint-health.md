# .go-arch-lint.yml 自体の健全性

`go-arch-lint check` が答えるのは「コードが設定に違反していないか」だけで、
設定そのものが劣化していく方向は検出できない。実際に起きる劣化はこの形をとる:

> パッケージを 1 つ足すたびに component を 1 つ足し、その import を `mayDependOn` に
> 書き足す。違反は常に 0 になる。設定はアーキテクチャの宣言ではなく、
> import グラフの転記になっている。

転記になった設定は何も禁じない。コードが先で設定が後から追いかけるため、
「この依存は書いてよいか」を設定に問うても、答えは常に「書けば通る」になる。

## 測る

`analyze-arch-lint <project-dir>`。`--json` で機械可読、`--strict` で
しきい値超過時に exit 1。

**指標の一覧・しきい値・意味・対処は `analyze-arch-lint --metrics` が出す。**
ここには一覧を置かない (置くと必ずコマンド側とずれる)。`--strict` の違反出力にも
その指標の意味と対処が付く。

データ源は 3 つ。glob の解釈は自前で書かず `go-arch-lint mapping --json` に任せる。

- 設定: `.go-arch-lint.yml`
- component ↔ パッケージ: `go-arch-lint mapping --json`
- 実際の import: `go list -f`

## compiler-scoped edge という考え方

Go の `internal` 規則は、`<root>/internal/...` にあるパッケージの import 元を
`<root>/...` の木の中へ閉じ込める。`internal` 要素が複数あるときは最も深いものが効く。

component C → D の辺は、D の全パッケージがある木に閉じており、C の全パッケージが
その木の中にあるとき **compiler-scoped** と呼ぶ。この辺で arch-lint が追加している
制約は「木の内側での順序」だけで、木をまたぐ import はコンパイラが既に弾く。
さらに Go は import 循環を禁じるので、木の内側でも一方向性は自動的に保たれる。
残るのは「どちら向きか」の宣言だけで、これはパッケージが増えるたびに書き換えが要る。

共有 `internal/*` (モジュール直下) はスコープがモジュール全体なので compiler-scoped
にならない。ここへの依存規則は arch-lint が唯一の防御線であり、残す価値がある。

## 是正

component を役割単位へ切り直す手順は `go-arch-metrics:remediate` の
`references/arch-lint-remediation.md`。
