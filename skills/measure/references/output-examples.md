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

## govulncheck

```
Scanning your code and 42 packages across 3 dependent modules for known vulnerabilities...

No vulnerabilities found.
```

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

## spm-go

```
+----+-----------+-------+----------+----------+--------------+-----------------+-------------+--------------+----------+
|  # | PACKAGE   | FILES | AFFERENT | EFFERENT | ABSTRACTIONS | IMPLEMENTATIONS | INSTABILITY | ABSTRACTNESS | DISTANCE |
+----+-----------+-------+----------+----------+--------------+-----------------+-------------+--------------+----------+
|  1 | main      |     5 |        0 |        4 |           18 |              20 |           1 |         0.47 |     0.47 |
|  2 | pathutil  |     1 |        1 |        0 |            0 |               3 |           0 |            0 |        1 |
+----+-----------+-------+----------+----------+--------------+-----------------+-------------+--------------+----------+
```

`pathutil` の D=1.0 は Ce=0 の葉パッケージによる構造的制約で、設計上の問題とは限らない。
判定は `go-arch-metrics:evaluate` の `references/thresholds.md`。

## analyze-modularity (JSON)

```json
{
  "packages": [
    {
      "path": "analyze-permissions",
      "exported_funcs": 9, "unexported_funcs": 11,
      "exported_types": 11, "unexported_types": 3,
      "public_ratio": 0.59, "loc": 974, "files": 5
    }
  ],
  "summary": {
    "mean_loc": 308, "stddev_loc": 287,
    "mean_public_ratio": 0.62,
    "outlier_packages": ["analyze-permissions"],
    "total_packages": 11, "total_warnings": 7
  },
  "warnings": []
}
```

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
