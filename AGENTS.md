# AGENTS.md

Go アーキテクチャメトリクスの Claude Code plugin。skill 4 本と CLI 2 本を持つ。

## レイアウト

```
.claude-plugin/   plugin.json (version は tagpr が書き換える) と marketplace.json
skills/           setup / measure / evaluate / remediate
cmd/              analyze-arch-lint, analyze-modularity (どちらも package main)
aqua/             registry.yaml (spm-go のローカル定義) と policy.yaml
```

## 検証

```sh
task test                            # go test ./...
task lint                            # 全静的解析。aqua のツールが要る
claude plugin validate --strict .    # plugin / marketplace manifest
```

`task lint` は `AQUA_POLICY_CONFIG` に `aqua/policy.yaml` を渡す。spm-go が
ローカルレジストリ由来で、aqua v2 が標準レジストリ以外を既定で拒否するため。

## skill を書くときの決めごと

- skill 間の参照は相対パスではなく skill 名 (`go-arch-metrics:evaluate`) で書く。
  plugin のインストール先はバージョンごとに変わるので、パスは書けない
- SKILL.md から script を呼ぶときは `"${CLAUDE_PLUGIN_ROOT}/skills/<name>/scripts/..."`
- `baseline.sh` が出す案内も skill 名で書く。相対パスで届く references が
  measure skill 側に無いため
- `analyze-arch-lint` の指標一覧はドキュメントに書かない。`--metrics` が正本

## 責務の分け方

measure は「ツールを回して JSON を出す」まで。数値の判断は evaluate。
しきい値の変更や Zone の追加は evaluate だけを触れば済むようにする。

## リリース

tagpr が CalVer (`YYYY.0M0D.MICRO`) で tag を打つ。`vPrefix = false` が必須。
`v` 付きだと Go module が major 2026 の semver と解釈し、module path に `/v2026` が
無いと拒否する。tag は plugin と GitHub Release のためのもので、Go 側は
default branch の pseudo-version で追う。
