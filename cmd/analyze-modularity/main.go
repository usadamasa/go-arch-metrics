package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// Report はレポートの最上位構造｡
type Report struct {
	Packages []PackageResult `json:"packages"`
	Summary  Summary         `json:"summary"`
	Warnings []Warning       `json:"warnings,omitempty"`
}

// MainSequenceSummary は Ca/Ce/A/I/D の全パッケージ集計｡
type MainSequenceSummary struct {
	ExcludedPackages             []string `json:"excluded_packages"`
	StructurallyConstrainedCount int      `json:"structurally_constrained_count"`
	ActionableCount              int      `json:"actionable_count"`
	ActionableMaxDistance        *float64 `json:"actionable_max_distance,omitempty"`
	ActionableMeanDistance       *float64 `json:"actionable_mean_distance,omitempty"`
}

// Summary は全パッケージのサマリ統計｡
type Summary struct {
	MeanLOC         float64              `json:"mean_loc"`
	StddevLOC       float64              `json:"stddev_loc"`
	MeanPublicRatio float64              `json:"mean_public_ratio"`
	OutlierPackages []string             `json:"outlier_packages,omitempty"`
	TotalPackages   int                  `json:"total_packages"`
	TotalWarnings   int                  `json:"total_warnings"`
	MainSequence    *MainSequenceSummary `json:"main_sequence"`
}

// inferModuleRoot は引数ディレクトリから親方向に go.mod を探索してモジュールルートを返す｡
func inferModuleRoot(args []string) string {
	dir, err := filepath.Abs(args[0])
	if err != nil {
		return args[0]
	}

	current := dir
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return dir
}

func main() {
	maxPublicRatio := flag.Float64("max-public-ratio", 0.6, "public ratio の警告しきい値")
	sigma := flag.Float64("sigma", 2.0, "LOC 外れ値検出の σ 倍数")
	minSymbols := flag.Int("min-symbols", defaultMinSymbols, "public ratio チェックの最小シンボル数")
	strict := flag.Bool("strict", false, "警告がある場合に exit code 1 を返す")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "使用方法: analyze-modularity [flags] <directory>...\n")
		fmt.Fprintf(os.Stderr, "  各ディレクトリ内の Go パッケージを解析します｡\n")
		os.Exit(1)
	}

	tracked, packages, err := collectPackages(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
		os.Exit(1)
	}

	if err := enrichMainSequence(tracked, packages); err != nil {
		fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
		os.Exit(1)
	}

	// 統計計算
	locs := make([]float64, len(packages))
	ratios := make([]float64, len(packages))
	for i, p := range packages {
		locs[i] = float64(p.LOC)
		ratios[i] = p.PublicRatio()
	}

	locStats := CalcStats(locs)
	ratioStats := CalcStats(ratios)
	outliers := DetectOutliers(packages, *sigma)

	// 警告生成
	var allWarnings []Warning
	for _, p := range packages {
		astWarnings := GenerateWarnings(p, *maxPublicRatio, *sigma, locStats, *minSymbols)
		mainSequenceWarnings := GenerateMainSequenceWarnings(p)
		crossWarnings := GenerateCrossWarnings(p, astWarnings)
		allWarnings = append(allWarnings, astWarnings...)
		allWarnings = append(allWarnings, mainSequenceWarnings...)
		allWarnings = append(allWarnings, crossWarnings...)
	}

	summary := Summary{
		MeanLOC:         locStats.Mean,
		StddevLOC:       locStats.Stddev,
		MeanPublicRatio: ratioStats.Mean,
		OutlierPackages: outliers,
		TotalPackages:   len(packages),
		TotalWarnings:   len(allWarnings),
		MainSequence:    buildMainSequenceSummary(packages),
	}

	report := Report{
		Packages: packages,
		Summary:  summary,
		Warnings: allWarnings,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		fmt.Fprintf(os.Stderr, "JSON 出力エラー: %v\n", err)
		os.Exit(1)
	}

	if *strict && report.Summary.TotalWarnings > 0 {
		os.Exit(1)
	}
}

// collectPackages は測る対象を決めて読む｡測るのは構成管理の対象だけ: go list が列挙するパッケージ
// (入れ子のモジュールと vendor を含まない) から git が ignore するものを落とし、args の下にあるものを
// 解析する｡返す tracked は Ca/Ce の辺を数える集合で、ignore されたものを含まない｡
func collectPackages(args []string) ([]listedPackage, []PackageResult, error) {
	moduleRoot := inferModuleRoot(args)
	listed, err := listModulePackages(moduleRoot)
	if err != nil {
		return nil, nil, err
	}
	tracked, err := dropIgnored(moduleRoot, listed)
	if err != nil {
		return nil, nil, err
	}
	selected, err := selectPackages(tracked, args)
	if err != nil {
		return nil, nil, err
	}

	var packages []PackageResult
	for _, s := range selected {
		result, err := AnalyzeDir(s.Dir, s.GoFiles)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", s.Path, err)
		}
		result.Path = s.Path
		packages = append(packages, result)
	}
	if len(packages) == 0 {
		return nil, nil, fmt.Errorf("解析可能な Go パッケージが見つかりません")
	}
	return tracked, packages, nil
}

// enrichMainSequence は listed の import 辺から Ca/Ce を数え、AST の型カウントから A を出して
// 各パッケージの MainSequence を埋める｡
func enrichMainSequence(listed []listedPackage, packages []PackageResult) error {
	// go list の Dir は絶対パスで、報告側の Path は引数のまま (相対もある)。symlink の解決度も
	// 違いうるので、両方を実体の絶対パスにそろえてから突き合わせる
	byDir := make(map[string]PackageMetrics, len(listed))
	for dir, m := range computeCoupling(listed) {
		canonical, err := canonicalDir(dir)
		if err != nil {
			return fmt.Errorf("go list が返したディレクトリを解決できません: %w", err)
		}
		byDir[canonical] = m
	}

	for i := range packages {
		dir, err := canonicalDir(packages[i].Path)
		if err != nil {
			return fmt.Errorf("パッケージのディレクトリを解決できません: %w", err)
		}
		m, ok := byDir[dir]
		if !ok {
			return fmt.Errorf("go list の結果に %s が無い", packages[i].Path)
		}
		m.Abstractness = packages[i].Abstractness()
		m.Instability = instability(m.AfferentCoupling, m.EfferentCoupling)
		m.Distance = distance(m.Abstractness, m.Instability)
		packages[i].MainSequence = &MainSequenceData{
			Zone:                    classifyZone(m),
			Distance:                m.Distance,
			StructurallyConstrained: isStructurallyConstrained(m),
			Abstractness:            m.Abstractness,
			Instability:             m.Instability,
			AfferentCoupling:        m.AfferentCoupling,
			EfferentCoupling:        m.EfferentCoupling,
		}
	}
	return nil
}

// buildMainSequenceSummary はパッケージの Ca/Ce/A/I/D からサマリを生成する｡
func buildMainSequenceSummary(packages []PackageResult) *MainSequenceSummary {
	summary := &MainSequenceSummary{}
	var maxDist, sumDist float64
	firstActionable := true

	for _, p := range packages {
		if p.MainSequence == nil {
			continue
		}
		if p.MainSequence.Zone == ZoneExcluded {
			summary.ExcludedPackages = append(summary.ExcludedPackages, p.Path)
			continue
		}
		if p.MainSequence.StructurallyConstrained {
			summary.StructurallyConstrainedCount++
			continue
		}
		if p.MainSequence.Distance > mainSequenceDistanceThreshold {
			summary.ActionableCount++
			sumDist += p.MainSequence.Distance
			if firstActionable || p.MainSequence.Distance > maxDist {
				maxDist = p.MainSequence.Distance
				firstActionable = false
			}
		}
	}

	if summary.ActionableCount > 0 {
		meanDist := sumDist / float64(summary.ActionableCount)
		summary.ActionableMaxDistance = &maxDist
		summary.ActionableMeanDistance = &meanDist
	}

	return summary
}
