# .go-arch-lint.yml の是正

設定がパッケージツリーの転記へ劣化していることの検出は
`go-arch-metrics:evaluate` の `references/arch-lint-health.md` と
`analyze-arch-lint --metrics`。ここはその直し方を扱う。

## 是正の方向

component をパッケージ単位ではなく **役割** 単位で切る。cmd 群 + 共有 internal という
構成なら 3 つで足りる:

```yaml
components:
  cmd_main:
    in: "*"
  cmd_internal:
    in: "*/internal/**"
  shared_internal:
    in: "internal/**"

deps:
  cmd_main:
    mayDependOn:
      - cmd_internal
      - shared_internal
  cmd_internal:
    mayDependOn:
      - cmd_internal
      - shared_internal
  # shared_internal は entry を持たない = 標準ライブラリ以外に依存できない
```

`in:` のパターンはモジュールルートからの相対で書く。`go.mod` がリポジトリ直下に
あって cmd が `cmd/` にあるなら `cmd/*` / `cmd/*/internal/**` になる。

書くのは Go のコンパイラが強制しないルールだけにする。この構成で言えば:

| 書かない | 理由 |
| -------- | ---- |
| ツール専用 internal を他のツールから使わせない | Go の `internal` 規則が既に閉じ込める |
| ツール内部の internal 同士の階層 | Go が import 循環を禁じるので一方向性は保たれる |
| 各ツールが使ってよい vendor の細かい割り当て | 主目的は「宣言せずに外部依存を増やせない」ことで、割り当ての粒度ではない |

| 書く | 理由 |
| ---- | ---- |
| 共有 internal は何にも依存しない | コンパイラが許すので arch-lint が唯一の防御線 |
| 共有 internal 同士も依存しない | 同上。`deps` に entry を置かないことで表現する |

**go-arch-lint は同一 component 内の import も検査する。** だから共有 internal を
1 component にまとめても「共有 internal 同士は依存しない」は維持される。
畳む前にこの挙動を捨て置きの設定 (`--arch-file` で差し替え) で確認しておくと、
何が失われるかを取り違えない。

## 畳んだ後に見えなくなるもの

役割単位にすると、パッケージを足しても設定が変わらない。それが狙いだが、
裏返すと **1 つのツールの internal がいくつに割れても設定に痕跡が残らない**。
`analyze-arch-lint` の `scope-packages` 指標はそこを見る。
