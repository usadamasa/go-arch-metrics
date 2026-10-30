# AGENTS.md

Go アーキテクチャメトリクスの Claude Code plugin。skill 4 本と CLI 4 本を持つ。

## レイアウト

```
.claude-plugin/   plugin.json (version の実体はここ 1 箇所)。marketplace.json は usadamasa/agents-marketplace が持つ
skills/           setup / measure / evaluate / remediate
cmd/              analyze-arch-lint, analyze-modularity, vulncheck, arch-baseline (いずれも package main)。
                  vulncheck は govulncheck の組み込みで、埋め込みの roots で TLS を検証する。
                  arch-baseline は measure skill が呼ぶ、全ツールを一通り回す CLI
flake.nix         CLI 4 本の packages.default と開発ツールの devShell (版は flake.lock が固定する)
.claude/skills/   develop (このリポジトリ自身の開発環境と検証)
```

## 検証

```sh
task test                            # go test ./...
task lint                            # 全静的解析。devShell のツールが要る
claude plugin validate --strict .    # plugin manifest
```

環境の用意、各コマンドが何を回しているか、つまずきどころは `develop` skill
(`.claude/skills/develop`) にある。

## skill を書くときの決めごと

- skill 間の参照は相対パスではなく skill 名 (`go-arch-metrics:evaluate`) で書く。
  plugin のインストール先はバージョンごとに変わるので、パスは書けない
- SKILL.md からは script を `"${CLAUDE_PLUGIN_ROOT}/skills/<name>/scripts/..."` のパスで呼ぶ
- `arch-baseline` が出す案内も skill 名で書く。相対パスで届く references が
  measure skill 側に無いため
- `analyze-arch-lint` の指標一覧はドキュメントに書かない。一覧は `--metrics` の出力で見る

## 責務の分け方

measure は「ツールを回して JSON を出す」まで。数値の判断は evaluate。
しきい値の変更や Zone の追加は evaluate だけを触れば済むようにする。

## リリース

tagpr がリリース PR を作り、merge すると CalVer (`YYYY.0M0D.MICRO`) の tag と GitHub Release を打つ。
`.claude-plugin/plugin.json` の `version` は手で上げない。

tag に `v` を付けない (`.tagpr` の `vPrefix = false`)。
`v2026.0905.0` は Go module が major 2026 の semver と解釈し、module path に
`/v2026` が無いと拒否する。`v` 無しなら Go にとって semver tag ではないので無視され、
`@latest` は default branch の pseudo-version に解決される。tag は plugin と
GitHub Release のためのもので、Go 側は commit で追う。

release notes の分類は `.github/release.yml` が持つ。
