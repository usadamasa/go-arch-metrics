# 是正ガイド

優先順位の基準は `go-arch-metrics:evaluate`。ここは「どう直すか」だけを扱う。

## カテゴリ別 是正手順

### モジュール性違反の是正

#### 依存方向の逆転 (go-arch-lint 違反)

**問題**: `domain` パッケージが `infra` パッケージを直接 import している

```go
// ❌ Bad: domain が infra に依存
package domain

import "github.com/example/app/internal/infra/db"

type OrderRepository struct {
    db *db.Client  // domain が infra に依存 → 逆転!
}
```

**是正**: インタフェースを domain に定義し、infra で実装する (DIP)

```go
// ✅ Good: domain はインタフェースのみ定義
package domain

// Repository インタフェースを domain に定義
type OrderRepository interface {
    FindByID(ctx context.Context, id OrderID) (*Order, error)
    Save(ctx context.Context, order *Order) error
}

// ✅ Good: infra でインタフェースを実装
package infra

import "github.com/example/app/internal/domain"

type OrderRepositoryImpl struct {
    db *sql.DB
}

func (r *OrderRepositoryImpl) FindByID(ctx context.Context, id domain.OrderID) (*domain.Order, error) {
    // DB アクセス実装
}
```

#### 循環参照の解消

**問題**: パッケージ A が B を import し、B が A を import している

**是正手順**:
1. `go list -f '{{.ImportPath}} -> {{.Imports}}' ./...` で循環を特定
2. 共有される型・ロジックを第3の共通パッケージ (`pkg/` や `internal/shared/`) に移動
3. または一方を interface に抽象化

---

### テスト可能性違反の是正

#### 認知的複雑度が高い関数の分割

**問題**: 認知的複雑度 > 20

```go
// ❌ Bad: 複雑度 28 (if/else/switch/for の組み合わせ)
func ProcessOrder(order Order) error {
    if order.Status == "pending" {
        if order.Amount > 1000 {
            for _, item := range order.Items {
                if item.Stock > 0 {
                    // ...
                } else {
                    // ...
                }
            }
        } else {
            // ...
        }
    } else if order.Status == "processing" {
        // ...
    }
    return nil
}
```

**是正**: 早期リターンと関数分割

```go
// ✅ Good: 各責務を独立した関数に分割
func ProcessOrder(order Order) error {
    if err := validateOrder(order); err != nil {
        return err
    }
    return processOrderByStatus(order)
}

func validateOrder(order Order) error {
    if order.Status == "" {
        return ErrInvalidStatus
    }
    return nil
}

func processOrderByStatus(order Order) error {
    switch order.Status {
    case "pending":
        return processPendingOrder(order)
    case "processing":
        return processRunningOrder(order)
    default:
        return ErrUnknownStatus
    }
}
```

#### 関数が長すぎる場合

**是正パターン**:

1. **ステップの抽出**: 処理の各フェーズを独立した関数に分割
2. **ヘルパーの抽出**: 繰り返しロジックをまとめる
3. **構造体メソッド化**: 状態を持つロジックはレシーバメソッドにする

```go
// ❌ Bad: 150行の巨大関数
func HandleRequest(req *http.Request, resp http.ResponseWriter) {
    // 認証チェック (30行)
    // バリデーション (40行)
    // ビジネスロジック (50行)
    // レスポンス組み立て (30行)
}

// ✅ Good: 責務を分割
func HandleRequest(req *http.Request, resp http.ResponseWriter) {
    user, err := h.authenticate(req)
    if err != nil {
        h.writeError(resp, err)
        return
    }
    input, err := h.validate(req)
    if err != nil {
        h.writeError(resp, err)
        return
    }
    result, err := h.usecase.Execute(req.Context(), user, input)
    if err != nil {
        h.writeError(resp, err)
        return
    }
    h.writeSuccess(resp, result)
}
```

#### ネストが深すぎる場合

**是正**: ガード節 (早期リターン) パターン

```go
// ❌ Bad: ネスト深さ 6
func processItem(item Item) error {
    if item != nil {
        if item.IsValid() {
            if item.Stock > 0 {
                if item.Price > 0 {
                    // 実際の処理
                }
            }
        }
    }
    return nil
}

// ✅ Good: ガード節で早期リターン
func processItem(item Item) error {
    if item == nil {
        return ErrNilItem
    }
    if !item.IsValid() {
        return ErrInvalidItem
    }
    if item.Stock <= 0 {
        return ErrOutOfStock
    }
    if item.Price <= 0 {
        return ErrInvalidPrice
    }
    // 実際の処理
    return nil
}
```

---

### コード重複の是正

#### package main 間のユーティリティ重複

**問題**: `cmd/` 配下の複数コマンドが同一の関数を丸ごとコピーしている

```go
// ❌ Bad: cmd/foo/main.go と cmd/bar/realpath.go に同一の resolveRealpath()
package main

func resolveRealpath(path string) (string, error) {
    // 完全に同一の実装が2箇所に存在
}
```

**是正**: `internal/` パッケージに共通ユーティリティとして抽出する

```go
// ✅ Good: internal/pathutil/realpath.go に共通化
package pathutil

func ResolveRealpath(path string) (string, error) {
    // 一箇所で管理
}

// cmd/server/main.go
import "example.com/app/internal/pathutil"
resolved, err := pathutil.ResolveRealpath(home)
```

**判断基準**: 2箇所以上で同一実装が存在し、それぞれ独立に修正される可能性がある場合は抽出する。テストヘルパー (`writeTestFile` 等) も同様。

#### 標準ライブラリの再実装

**問題**: 標準ライブラリに既存の関数を自前で実装している

```go
// ❌ Bad: slices.Equal が標準ライブラリにあるのに独自実装
func sliceEqual(a, b []string) bool {
    if len(a) != len(b) { return false }
    for i := range a {
        if a[i] != b[i] { return false }
    }
    return true
}

// ✅ Good: 標準ライブラリを使う
import "slices"
if !slices.Equal(got, tt.want) { ... }
```

**チェックポイント**: 新しい関数を書く前に `go doc` や pkg.go.dev で同名・同目的の関数がないか確認する。特に `slices`, `maps`, `cmp` パッケージ (Go 1.21+)。

#### 冗長な条件分岐

**問題**: bool を返すだけの冗長な if-else

```go
// ❌ Bad: 冗長な返却
func isRedirect(tok string) bool {
    if strings.Contains(tok, ">/") || strings.Contains(tok, ">&") {
        return true
    }
    return false
}

// ✅ Good: 条件式をそのまま返す
func isRedirect(tok string) bool {
    return strings.Contains(tok, ">/") || strings.Contains(tok, ">&")
}
```

#### switch case の不必要な分離

**問題**: 同一処理の case を分けて書いている

```go
// ❌ Bad: du と tree で同じ処理なのに case を分離
case "du":
    paths := parseGenericPaths(tokens, home)
    targets = append(targets, paths...)
case "tree":
    paths := parseGenericPaths(tokens, home)
    targets = append(targets, paths...)

// ✅ Good: 同一処理はまとめる
case "du", "tree":
    paths := parseGenericPaths(tokens, home)
    targets = append(targets, paths...)
```

---

### 保守性違反の是正

#### 保守性指数が低い (maintidx < 20)

保守性指数は複合指標なので、以下を組み合わせて改善する:

1. **コメントを追加** (Halstead の可読性コンポーネントを改善)
2. **関数を分割** (行数コンポーネントを改善)
3. **複雑度を下げる** (循環複雑度コンポーネントを改善)

#### 到達不能コード (deadcode)

```bash
# deadcode で検出
deadcode ./...

# 出力例
cmd/server/main.go:45: unreachable function "oldHandler"
internal/usecase/order.go:78: unreachable function "legacyCalculate"
```

**是正**: 単純に削除する。「将来使うかも」は Git の履歴に残っているため不要。

---

## パッケージ責務の是正

### Public ratio が高すぎる場合 (analyze-modularity)

**問題**: `public_ratio > 0.6` は API surface が過大であることを示す

**是正パターン**:

1. **unexported にできる関数・型を特定**: `package main` 内の関数は外部から参照されないため、全て unexported でよい
2. **internal パッケージの場合**: 他パッケージから実際に使われていない exported シンボルを unexported に変更
3. **型の公開範囲を縮小**: constructor パターン (`NewXxx()`) を使い、struct フィールドを unexported にする

```go
// ❌ Bad: 全フィールドが公開
type Config struct {
    Host     string
    Port     int
    Timeout  time.Duration
}

// ✅ Good: constructor で制御
type Config struct {
    host    string
    port    int
    timeout time.Duration
}

func NewConfig(host string, port int) *Config {
    return &Config{host: host, port: port, timeout: 30 * time.Second}
}
```

### パッケージが統計的外れ値の場合 (analyze-modularity)

**問題**: LOC > mean + 2σ は単一パッケージに過大な責務が集中している兆候

**是正パターン**:

1. **責務の分離**: 1パッケージ内の異なる責務を別パッケージに分割
2. **ファイル分割**: 同一パッケージ内でも機能ごとにファイルを分ける
3. **共通ロジックの抽出**: `internal/` パッケージに共通部分を移動

### Distance from Main Sequence が高い場合 (spm-go)

**問題**: D = |A + I - 1| > 0.5 はパッケージが Main Sequence から遠い

#### 1. まず zone と structurally_constrained を確認する

baseline.sh の出力 JSON で各パッケージの `zone` と `structurally_constrained` を確認し、以下のフローで対処を判断する:

```
D > 0.5 のパッケージ
  ├─ zone == "excluded" → 対処不要 (main/孤立パッケージ)
  ├─ structurally_constrained == true → 長期的評価へ (下記参照)
  └─ それ以外 → アクション可能: Zone 別対処へ
```

#### 2. Zone 別 解釈と対処

| パターン | Zone | 構造制約 | 解釈 | 対処 |
|---------|------|---------|------|------|
| アクション要 | zone_of_pain | false | 具象的で安定だが Ce>0。依存を持つのに抽象度が低い | インタフェースを導入して A を上げる |
| 構造制約 | zone_of_pain | true | Ce=0 の葉パッケージ。D=1.0 は構造的必然 | 通常は harmless。長期的評価基準で判断 |
| アクション要 | zone_of_uselessness | false | 抽象的で不安定。使われていない抽象が多い | 不要な抽象を削除するか、具象実装を追加 |
| 正常 | main_sequence | - | A + I ≈ 1 でバランス良好 | 対処不要 |
| 除外 | excluded | - | main パッケージまたは孤立パッケージ | D 集計対象外。対処不要 |

#### 3. 構造制約パッケージの長期的評価基準

`structurally_constrained: true` のパッケージは通常 harmless だが、以下の場合はリファクタリングを検討する:

| 評価軸 | 要注意サイン | 対処 |
|--------|------------|------|
| 変更頻度 (churn) | `git log --oneline <pkg-dir> \| wc -l` が高い | 揮発性が高いなら Zone of Pain のリスクあり。インタフェース抽出を検討 |
| 依存数 (Ca) | Ca が非常に大きい (例: >5) | 多くのパッケージから依存されている安定コア。変更時の影響が大きいため、インタフェース layer の追加を検討 |
| 責務の範囲 | LOC が外れ値、または public ratio が高い | パッケージ分割を検討 |

Zone の定義と構造的制約の根拠は `go-arch-metrics:evaluate` の
`references/thresholds.md`。
