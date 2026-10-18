module github.com/usadamasa/go-arch-metrics

// GO-2026-4602 (FileInfo can escape from a Root in os) は go1.26.1 で修正された｡
// os.ReadDir を呼ぶので、go1.26.0 でビルドすると govulncheck が拾う｡
go 1.26.1

require (
	golang.org/x/crypto/x509roots/fallback v0.0.0-20261016151342-0ae6809b7559
	golang.org/x/vuln v1.9.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/telemetry v0.0.0-20261010193011-f71dc92da45e // indirect
	golang.org/x/tools v0.52.0 // indirect
)
