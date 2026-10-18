# Changelog

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
