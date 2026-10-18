# 各ツールの出力例

数値の解釈は `go-arch-metrics:evaluate`、実行方法は `go-arch-metrics:setup` の
`references/tools.md`。ここは「何が出てくるか」だけを並べる。

## golangci-lint

```
cmd/server/handler.go:42:1: Function 'HandleRequest' has too high cognitive complexity (25 > 20) (gocognit)
cmd/server/handler.go:42:1: Function 'HandleRequest' is too long (145 > 100) (funlen)
internal/usecase/order.go:89:5: nestif: nesting depth 6 > 5 (nestif)
```

## go-arch-lint

```
[ARCHITECTURE ERROR] package "github.com/example/app/infrastructure/db"
  imports "github.com/example/app/domain/usecase"
  which violates rule: infrastructure -> domain is not allowed
```

## vulncheck (govulncheck の組み込み)

```
=== Symbol Results ===

No vulnerabilities found.

Your code is affected by 0 vulnerabilities.
This scan also found 1 vulnerability in packages you import and 12
vulnerabilities in modules you require, but your code doesn't appear to call
these vulnerabilities.
```

exit code が 0 なら脆弱性なし、3 なら呼んでいるシンボルに脆弱性あり、それ以外は DB に届かず未測定。
baseline.sh はこれを `govulncheck.status` (`ok` / `vulnerable` / `unmeasured`) として JSON に残す。

## gosec

```
[gosec] 2024/01/01 Results:

Golang errors in file: [/path/to/file.go]

Summary:
  Gosec: dev
  Files: 15
  Lines: 1234
  Nosec: 0
  Issues: 0
```

## analyze-modularity (JSON)

```json
{
  "packages": [
    {
      "path": "analyze-permissions",
      "exported_funcs": 9, "unexported_funcs": 11,
      "exported_types": 11, "unexported_types": 3, "interface_types": 2,
      "public_ratio": 0.59, "loc": 974, "files": 5,
      "main_sequence": {
        "zone": "zone_of_pain", "distance": 0.53, "structurally_constrained": false,
        "abstractness": 0.14, "instability": 0.33,
        "afferent_coupling": 2, "efferent_coupling": 1
      }
    }
  ],
  "summary": {
    "mean_loc": 308, "stddev_loc": 287,
    "mean_public_ratio": 0.62,
    "outlier_packages": ["analyze-permissions"],
    "total_packages": 11, "total_warnings": 7, "actionable_warnings": 2,
    "main_sequence": {
      "excluded_packages": ["cmd/app"],
      "structurally_constrained_count": 3,
      "actionable_count": 1,
      "actionable_max_distance": 0.53, "actionable_mean_distance": 0.53
    }
  },
  "warnings": [
    {
      "type": "zone_of_pain", "package": "analyze-permissions",
      "value": 0.53, "limit": 0,
      "message": "Zone of Pain: 具象的 (A=0.14) で安定 (I=0.33)",
      "actionable": true
    },
    {
      "type": "high_distance", "package": "analyze-permissions",
      "value": 0.53, "limit": 0.5,
      "message": "Main Sequence から大きく外れています (D=0.53 > 0.5)",
      "actionable": true
    },
    {
      "type": "structurally_constrained", "package": "internal/config",
      "value": 1, "limit": 0,
      "message": "構造制約: Ce=0, Ca=4 のため D=1.00 はトポロジー由来 (情報のみ。--strict の失敗には数えない)",
      "actionable": false
    }
  ]
}
```

`structurally_constrained: true` (Ce=0 の葉パッケージ) の D=1.0 は構造的制約で、設計上の問題とは限らない。
このパッケージに付く `zone_of_pain` と `structurally_constrained` の警告は `actionable: false` で、
`summary.actionable_warnings` と `--strict` の判定に入らない。判定は `go-arch-metrics:evaluate` の
`references/thresholds.md`。

## analyze-arch-lint

```
component: 3 個 (うち 1 パッケージのみ: 0 個 / 0%)
パッケージ: 24 個 (どの component にも属さない: 0 個)
mayDependOn: 4 辺 (未使用 0 / _test.go のみ 0 / internal 規則でスコープ済み 1 = 25%)
最大 fan-out: cmd_main (2 辺)
最大の internal スコープ: cmd/analyze-latency (5 パッケージ)
```

指標の意味・しきい値・対処は `analyze-arch-lint --metrics` が出す。
ドキュメント側には表を置かない (置くと必ずコマンド側とずれる)。
