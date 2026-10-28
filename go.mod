module github.com/usadamasa/go-arch-metrics

// GO-2026-4602 (FileInfo can escape from a Root in os) は go1.26.1 で修正された｡
// os.ReadDir を呼ぶので、go1.26.0 でビルドすると govulncheck が拾う｡
go 1.26.1

// GO-2026-6809 (crypto/x509) は go1.26.10 で修正された。clone してビルドするときに
// GOTOOLCHAIN=auto なら古い go がこの版へ切り替える。go 行は利用側の要求になるので上げない
toolchain go1.26.10

require (
	golang.org/x/crypto/x509roots/fallback v0.0.0-20261020153306-9f2b28d4331a
	golang.org/x/tools v0.52.0
	golang.org/x/vuln v1.9.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/telemetry v0.0.0-20261010193011-f71dc92da45e // indirect
)

tool (
	github.com/usadamasa/go-arch-metrics/cmd/analyze-arch-lint
	github.com/usadamasa/go-arch-metrics/cmd/analyze-modularity
	github.com/usadamasa/go-arch-metrics/cmd/vulncheck
)
