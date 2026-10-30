// Package schema は arch-baseline と analyze-modularity が出す JSON の型。
// skills/measure/references/baseline.schema.json から生成する。キーを変えるときは schema を直して
// task generate で生成し直す。
package schema

//go:generate go run github.com/atombender/go-jsonschema@v0.24.1 --tags json -t --capitalization LOC -p schema -o schema.go ../../skills/measure/references/baseline.schema.json
