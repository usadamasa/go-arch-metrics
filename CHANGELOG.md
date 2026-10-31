# Changelog

## [2026.1031.0](https://github.com/usadamasa/go-arch-metrics/compare/2026.1030.0...2026.1031.0) - 2026-10-31

### Other Changes
- 測定対象から git が ignore するディレクトリとファイルを除く by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/90
- 文書で説明が付かない結果は実装を読まず issue にする方針を evaluate skill に書く by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/92

## [2026.1030.0](https://github.com/usadamasa/go-arch-metrics/compare/2026.1028.0...2026.1030.0) - 2026-10-30

### Other Changes
- baseline.sh を Go の CLI (arch-baseline) で実装し直す by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/85
- --strict が失敗に数える警告を evaluate skill の 1 つの表にまとめる by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/87

## [2026.1028.0](https://github.com/usadamasa/go-arch-metrics/compare/2026.1027.0...2026.1028.0) - 2026-10-28

### Other Changes
- baseline の JSON を測った場所に依らない形にし、日付付きファイルをやめる by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/81
- LOC 外れ値のしきい値を 1 つにそろえ、limit の由来を出す by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/83

## [2026.1027.0](https://github.com/usadamasa/go-arch-metrics/compare/2026.1024.0...2026.1027.0) - 2026-10-27

### Other Changes
- ci: pull_request の types に edited を足し、base の付け替えで CI を走り直す by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/73
- analyze-modularity: Ce を自分の宣言の数で数え、依存先の統合で呼び手が動かないようにする by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/71
- analyze-modularity: --baseline で base から D を悪くしたパッケージだけを落とす by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/72
- docs: 開発者向けの md から「正本」を除き、添削を当てる by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/80

## [2026.1024.0](https://github.com/usadamasa/go-arch-metrics/compare/2026.1021.0...2026.1024.0) - 2026-10-24

### Bug Fixes 🐛
- baseline.sh が golangci-lint と gosec の実行の失敗を握りつぶさないようにする by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/64
### Dependencies
- build(deps): Bump actions/create-github-app-token from 3.2.0 to 3.3.0 by @dependabot[bot] in https://github.com/usadamasa/go-arch-metrics/pull/56
- build(deps): Bump actions/checkout from 6.0.3 to 7.0.1 by @dependabot[bot] in https://github.com/usadamasa/go-arch-metrics/pull/55
- build(deps): Bump Songmu/tagpr from 1.20.3 to 1.21.3 by @dependabot[bot] in https://github.com/usadamasa/go-arch-metrics/pull/54
### Other Changes
- build(go): GO-2026-6809 のため go.mod に toolchain go1.26.10 を足す by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/61
- ci: setup-nix を gh-manage の共有 action へ移行する by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/63
- analyze-modularity: 外から参照されない exported で API surface を判定する by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/67
- analyze-modularity: コメントだけ・空白だけの commit を揮発性に数えない by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/68

## [2026.1021.0](https://github.com/usadamasa/go-arch-metrics/compare/2026.1018.5...2026.1021.0) - 2026-10-21

### Other Changes
- devShell の go を 1.26.10、配布バイナリの go を 1.27.3 に上げる (GO-2026-6809) by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/51
- Dependabot で go.mod・GitHub Actions・flake.lock の更新 PR を出す by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/52
- devShell の Go も Dependabot の nix PR で上がるようにする by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/57
- ci: govulncheck を週次で定期実行する by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/59

## [2026.1018.5](https://github.com/usadamasa/go-arch-metrics/compare/2026.1018.4...2026.1018.5) - 2026-10-18

### Other Changes
- fix(analyze-arch-lint): scope-packages を同じディレクトリに並ぶパッケージ数で数える by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/48

## [2026.1018.4](https://github.com/usadamasa/go-arch-metrics/compare/2026.1018.3...2026.1018.4) - 2026-10-18

### Other Changes
- setup: CLI を plugin の版の tag で取り、tag の commit と照合する by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/44
- flake: package を go_latest でビルドし、go 1.27 の利用側で vulncheck を測れるようにする by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/45

## [2026.1018.3](https://github.com/usadamasa/go-arch-metrics/compare/2026.1018.2...2026.1018.3) - 2026-10-18

### Other Changes
- fix(measure): baseline.sh は PATH に無いツールを go.mod の tool directive から呼ぶ by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/41

## [2026.1018.2](https://github.com/usadamasa/go-arch-metrics/compare/2026.1018.1...2026.1018.2) - 2026-10-18

### Other Changes
- build(nix): CLI 3 本を flake の packages.default として出す by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/38
- fix(analyze-modularity): --strict は actionable な警告だけを失敗に数える by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/37
- feat(analyze-modularity): 非揮発の Zone of Pain を actionable から外す by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/39
- docs(plugin): description から spm-go の名前を外す by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/34

## [2026.1018.1](https://github.com/usadamasa/go-arch-metrics/compare/2026.1018.0...2026.1018.1) - 2026-10-18

### Other Changes
- analyze-modularity を go list で列挙し、git が ignore するパッケージを測らない by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/30
- govulncheck を組み込み、sandbox の中でも脆弱性スキャンが通るようにする by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/32

## [2026.1018.0](https://github.com/usadamasa/go-arch-metrics/compare/2026.0927.0...2026.1018.0) - 2026-10-18

### Other Changes
- fix(measure): spm-go を PATH の go と同じ GOROOT で呼び、0 件を失敗にする by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/18
- chore(aqua): golangci-lint を v2.14.0 に上げる by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/21
- ci: ci-status-check を always() 形に直す by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/22
- chore: 開発ツールの管理を aqua から nix の devShell へ移す by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/23
- chore: nix 移行の後始末と gh-actions-lock の導入 by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/24
- spm-go への依存をやめ、Ca/Ce/A/I/D を analyze-modularity で算出する by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/28

## [2026.0927.0](https://github.com/usadamasa/go-arch-metrics/compare/2026.0919.3...2026.0927.0) - 2026-09-27

### Other Changes
- chore: 配布経路を usadamasa/agents-marketplace へ移す by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/12
- fix(analyze-arch-lint): ルート直下の main パッケージを component に数える by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/13
- chore: tagpr でリリース PR を作るようにする by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/15

## [2026.0919.3](https://github.com/usadamasa/go-arch-metrics/compare/2026.0919.0...2026.0919.3) - 2026-09-19

### Other Changes
- chore: aqua の使い方とツールの入れ方の記述を外す by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/11
- fix(measure): go-arch-lint と spm-go の実行失敗を握りつぶさない by @usadamasa in https://github.com/usadamasa/go-arch-metrics/pull/10
