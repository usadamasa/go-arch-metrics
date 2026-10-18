package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
)

// PackageMetrics は Robert C. Martin の Package Metrics (Ca/Ce/A/I/D) の 1 パッケージ分｡
type PackageMetrics struct {
	Name             string
	Path             string
	AfferentCoupling int
	EfferentCoupling int
	Abstractness     float64
	Instability      float64
	Distance         float64
}

// Zone は Martin の Main Sequence に基づくパッケージの Zone 分類｡
type Zone string

const (
	ZoneExcluded      Zone = "excluded"
	ZoneOfPain        Zone = "zone_of_pain"
	ZoneOfUselessness Zone = "zone_of_uselessness"
	ZoneMainSequence  Zone = "main_sequence"
)

// classifyZone はパッケージの Zone を分類する｡
// Martin (Agile PPP ch.28) の volatility caveat に基づく:
//
//	main パッケージ or 孤立 (Ca=0, Ce=0) → excluded
//	A < 0.5 かつ I < 0.5 → zone_of_pain
//	A > 0.5 かつ I > 0.5 → zone_of_uselessness
//	上記以外 → main_sequence
func classifyZone(pkg PackageMetrics) Zone {
	if pkg.Name == "main" {
		return ZoneExcluded
	}
	if pkg.AfferentCoupling == 0 && pkg.EfferentCoupling == 0 {
		return ZoneExcluded
	}
	if pkg.Abstractness < 0.5 && pkg.Instability < 0.5 {
		return ZoneOfPain
	}
	if pkg.Abstractness > 0.5 && pkg.Instability > 0.5 {
		return ZoneOfUselessness
	}
	return ZoneMainSequence
}

// isStructurallyConstrained は Ce=0 かつ Ca>0 の構造制約を判定する｡
// I=0 がトポロジーにより強制されている状態を示す｡
func isStructurallyConstrained(pkg PackageMetrics) bool {
	return pkg.EfferentCoupling == 0 && pkg.AfferentCoupling > 0
}

// instability は I = Ce / (Ca + Ce) を返す｡
// 孤立パッケージ (Ca = Ce = 0) は依存元が無く、変えても何も壊れないので最大限に不安定とみなして 1 を返す｡
func instability(ca, ce int) float64 {
	if ca+ce == 0 {
		return 1
	}
	return float64(ce) / float64(ca+ce)
}

// distance は Main Sequence からの距離 D = |A + I - 1| を返す｡
func distance(a, i float64) float64 {
	return math.Abs(a + i - 1)
}

// listedPackage は go list -json の出力のうち使うフィールド｡
type listedPackage struct {
	ImportPath string
	Name       string
	Dir        string
	Imports    []string
}

// listModulePackages は moduleRoot で go list -json ./... を実行し、モジュール内の全パッケージを返す｡
// Imports はテストファイルの import を含まない (それは TestImports)｡
func listModulePackages(moduleRoot string) ([]listedPackage, error) {
	cmd := exec.Command("go", "list", "-json=ImportPath,Name,Dir,Imports", "./...")
	cmd.Dir = moduleRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list の実行に失敗 (%s): %w\n%s", moduleRoot, err, bytes.TrimSpace(stderr.Bytes()))
	}

	dec := json.NewDecoder(bytes.NewReader(out))
	var pkgs []listedPackage
	for {
		var p listedPackage
		if err := dec.Decode(&p); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("go list の出力のパースに失敗: %w", err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// computeCoupling はモジュール内の import 辺から Ca/Ce を数え、パッケージのディレクトリをキーに返す｡
// 標準ライブラリと外部モジュールへの import は数えない｡
func computeCoupling(pkgs []listedPackage) map[string]PackageMetrics {
	inModule := make(map[string]bool, len(pkgs))
	for _, p := range pkgs {
		inModule[p.ImportPath] = true
	}

	afferent := make(map[string]int, len(pkgs))
	byDir := make(map[string]PackageMetrics, len(pkgs))
	for _, p := range pkgs {
		efferent := 0
		for _, imp := range p.Imports {
			if inModule[imp] {
				efferent++
				afferent[imp]++
			}
		}
		byDir[p.Dir] = PackageMetrics{Name: p.Name, Path: p.ImportPath, EfferentCoupling: efferent}
	}
	for dir, m := range byDir {
		m.AfferentCoupling = afferent[m.Path]
		byDir[dir] = m
	}
	return byDir
}
