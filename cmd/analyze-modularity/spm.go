package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// SPMPackage は spm-go v0.11.1 JSON の1パッケージ｡
type SPMPackage struct {
	Name             string  `json:"name"`
	Path             string  `json:"path"`
	AfferentCoupling int     `json:"afferent_coupling"`
	EfferentCoupling int     `json:"efferent_coupling"`
	Abstractness     float64 `json:"abstractness"`
	Instability      float64 `json:"instability"`
	Distance         float64 `json:"distance"`
}

// SPMReport は spm-go JSON の最上位構造｡
type SPMReport struct {
	Packages []SPMPackage `json:"packages"`
}

// SPMZone は Martin の Main Sequence に基づくパッケージの Zone 分類｡
type SPMZone string

const (
	SPMZoneExcluded      SPMZone = "excluded"
	SPMZoneOfPain        SPMZone = "zone_of_pain"
	SPMZoneOfUselessness SPMZone = "zone_of_uselessness"
	SPMZoneMainSequence  SPMZone = "main_sequence"
)

// mainPkgPattern は main パッケージ名にマッチする正規表現｡
var mainPkgPattern = regexp.MustCompile(`(^|/)main$`)

// classifyZone は SPM パッケージの Zone を分類する｡
// Martin (Agile PPP ch.28) の volatility caveat に基づく:
//
//	main パッケージ or 孤立 (Ca=0, Ce=0) → excluded
//	A < 0.5 かつ I < 0.5 → zone_of_pain
//	A > 0.5 かつ I > 0.5 → zone_of_uselessness
//	上記以外 → main_sequence
func classifyZone(pkg SPMPackage) SPMZone {
	if mainPkgPattern.MatchString(pkg.Name) {
		return SPMZoneExcluded
	}
	if pkg.AfferentCoupling == 0 && pkg.EfferentCoupling == 0 {
		return SPMZoneExcluded
	}
	if pkg.Abstractness < 0.5 && pkg.Instability < 0.5 {
		return SPMZoneOfPain
	}
	if pkg.Abstractness > 0.5 && pkg.Instability > 0.5 {
		return SPMZoneOfUselessness
	}
	return SPMZoneMainSequence
}

// isStructurallyConstrained は Ce=0 かつ Ca>0 の構造制約を判定する｡
// I=0 がトポロジーにより強制されている状態を示す｡
func isStructurallyConstrained(pkg SPMPackage) bool {
	return pkg.EfferentCoupling == 0 && pkg.AfferentCoupling > 0
}

// loadSPMReport は spm-go JSON ファイルをストリーミングで読み込みパースする｡
func loadSPMReport(path string) (*SPMReport, error) {
	f, err := os.Open(path) // #nosec G304 -- ユーザー指定のフラグ値
	if err != nil {
		return nil, fmt.Errorf("SPM JSON の読み込みに失敗: %w", err)
	}
	defer func() { _ = f.Close() }()

	var report SPMReport
	if err := json.NewDecoder(f).Decode(&report); err != nil {
		return nil, fmt.Errorf("SPM JSON のパースに失敗: %w", err)
	}
	return &report, nil
}

// buildSPMIndex は SPMReport からインポートパス→SPMPackage のマップを構築する｡
// キーは spm-go JSON の path フィールド (フルインポートパス) をそのまま使用する｡
func buildSPMIndex(report *SPMReport) map[string]SPMPackage {
	index := make(map[string]SPMPackage, len(report.Packages))
	for _, pkg := range report.Packages {
		index[pkg.Path] = pkg
	}
	return index
}

// readModuleName は go.mod ファイルからモジュール名を読み取る｡
func readModuleName(gomodPath string) (string, error) {
	f, err := os.Open(gomodPath) // #nosec G304 -- inferModuleRoot で発見したパス
	if err != nil {
		return "", fmt.Errorf("go.mod の読み込みに失敗: %w", err)
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 && fields[0] == "module" {
			return strings.Trim(fields[1], `"`), nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("go.mod の読み取りエラー: %w", err)
	}
	return "", fmt.Errorf("module directive が見つかりません: %s", gomodPath)
}
