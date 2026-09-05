module github.com/usadamasa/go-arch-metrics

// GO-2026-4602 (FileInfo can escape from a Root in os) は go1.26.1 で修正された｡
// os.ReadDir を呼ぶので、go1.26.0 でビルドすると govulncheck が拾う｡
go 1.26.1

require gopkg.in/yaml.v3 v3.0.1
