package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	var packages []PackageResult
	for _, arg := range args {
		dirs, err := findGoPackageDirs(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
			os.Exit(1)
		}
		for _, dir := range dirs {
			result, err := AnalyzeDir(dir)
			if err != nil {
				if !strings.Contains(err.Error(), "go ソースファイルが見つかりません") {
					fmt.Fprintf(os.Stderr, "警告: %s: %v\n", dir, err)
				}
				continue
			}
			packages = append(packages, result)
		}
	}

	if len(packages) == 0 {
		fmt.Fprintf(os.Stderr, "エラー: 解析可能な Go パッケージが見つかりません\n")
		os.Exit(1)
	}

	if err := enrichMainSequence(inferModuleRoot(args), packages); err != nil {
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

// enrichMainSequence は go list の import 辺から Ca/Ce を数え、AST の型カウントから A を出して
// 各パッケージの MainSequence を埋める｡go list に含まれないパッケージは nil のまま残し、stderr に知らせる｡
func enrichMainSequence(moduleRoot string, packages []PackageResult) error {
	listed, err := listModulePackages(moduleRoot)
	if err != nil {
		return err
	}

	// go list の Dir は絶対パスで、走査したディレクトリは引数のまま (相対もある)。symlink の解決度も
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
			fmt.Fprintf(os.Stderr, "警告: go list に含まれないため Ca/Ce を算出できません: %s\n", packages[i].Path)
			continue
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

// canonicalDir は symlink を解決した絶対パスを返す｡
func canonicalDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
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

// findGoPackageDirs はディレクトリを再帰的に走査し、Go ファイルを含むディレクトリを返す｡
func findGoPackageDirs(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("パスが存在しません: %s", root)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("ディレクトリではありません: %s", root)
	}

	var dirs []string
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告: Walk エラー: %s: %v\n", path, err)
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		// vendor, testdata, .git 等を除外
		base := filepath.Base(path)
		if base == "vendor" || base == "testdata" || base == ".git" || base == ".worktrees" {
			return filepath.SkipDir
		}
		// Go ファイルがあるかチェック
		entries, err := os.ReadDir(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告: ReadDir エラー: %s: %v\n", path, err)
			return nil
		}
		for _, e := range entries {
			name := e.Name()
			if filepath.Ext(name) == ".go" && !isTestFile(name) {
				dirs = append(dirs, path)
				break
			}
		}
		return nil
	})
	return dirs, err
}

// isTestFile はファイル名がテストファイルかどうか判定する｡
func isTestFile(name string) bool {
	return len(name) > 8 && name[len(name)-8:] == "_test.go"
}
