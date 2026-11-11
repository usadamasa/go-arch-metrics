module github.com/usadamasa/go-arch-metrics

// go1.27.4 で net/http と cmd/go などの脆弱性 (CVE-2026-97054、CVE-2026-97033〜97043) が直った。
// GOTOOLCHAIN=local の手元ビルドでも、これより古い Go では失敗させる
go 1.27.4

require (
	github.com/securego/gosec/v2 v2.30.0
	golang.org/x/crypto/x509roots/fallback v0.0.0-20261111072342-6c966a7517bf
	golang.org/x/tools v0.52.0
	golang.org/x/vuln v1.9.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/ccojocar/zxcvbn-go v1.0.4 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/telemetry v0.0.0-20261010193011-f71dc92da45e // indirect
)

tool (
	github.com/usadamasa/go-arch-metrics/cmd/analyze-arch-lint
	github.com/usadamasa/go-arch-metrics/cmd/analyze-modularity
	github.com/usadamasa/go-arch-metrics/cmd/arch-baseline
	github.com/usadamasa/go-arch-metrics/cmd/vulncheck
)
