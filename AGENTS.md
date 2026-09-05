# AGENTS.md

Go アーキテクチャメトリクスの Claude Code plugin。skill 4 本と CLI 2 本を持つ。

## レイアウト

```
.claude-plugin/   plugin.json (version の実体はここ 1 箇所) と marketplace.json
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

CalVer (`YYYY.0M0D.MICRO`) の tag を手で打つ。**`v` を付けない。**
`v2026.0905.0` は Go module が major 2026 の semver と解釈し、module path に
`/v2026` が無いと拒否する。`v` 無しなら Go にとって semver tag ではないので無視され、
`@latest` は default branch の pseudo-version に解決される。tag は plugin と
GitHub Release のためのもので、Go 側は commit で追う。

手順は 3 つ。`.claude-plugin/plugin.json` の `version` を先に上げて merge する。

```sh
git tag 2026.0905.0 && git push origin 2026.0905.0
gh release create 2026.0905.0 --generate-notes
```

release notes の分類は `.github/release.yml` が持つ。

tagpr は使わない。`GITHUB_TOKEN` では release PR を作れず
("GitHub Actions is not permitted to create or approve pull requests")、
GitHub App のトークンを置く運用に見合う頻度でもないため。
