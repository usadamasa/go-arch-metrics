package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Report はレポートの最上位構造｡
type Report struct {
	Packages []PackageResult `json:"packages"`
	Summary  Summary         `json:"summary"`
	Warnings []Warning       `json:"warnings,omitempty"`
}

// MainSequenceSummary は Ca/Ce/A/I/D の全パッケージ集計｡
// actionable_* は構造制約と非揮発の Zone of Pain を除いて数える (isMainSequenceActionable と同じ基準)｡
type MainSequenceSummary struct {
	ExcludedPackages             []string `json:"excluded_packages"`
	StructurallyConstrainedCount int      `json:"structurally_constrained_count"`
	NonVolatileZoneOfPainCount   int      `json:"non_volatile_zone_of_pain_count"`
	ActionableCount              int      `json:"actionable_count"`
	ActionableMaxDistance        *float64 `json:"actionable_max_distance,omitempty"`
	ActionableMeanDistance       *float64 `json:"actionable_mean_distance,omitempty"`
	VolatilityWindowCommits      int      `json:"volatility_window_commits"`
	VolatileCommitThreshold      int      `json:"volatile_commit_threshold"`
	VolatilityUnmeasuredReason   string   `json:"volatility_unmeasured_reason,omitempty"`
}

// Summary は全パッケージのサマリ統計｡
// ActionableWarnings は warnings のうち actionable なものの件数で、--strict はこれを見る｡
type Summary struct {
	MeanLOC            float64              `json:"mean_loc"`
	StddevLOC          float64              `json:"stddev_loc"`
	MeanPublicRatio    float64              `json:"mean_public_ratio"`
	OutlierPackages    []string             `json:"outlier_packages,omitempty"`
	TotalPackages      int                  `json:"total_packages"`
	TotalWarnings      int                  `json:"total_warnings"`
	ActionableWarnings int                  `json:"actionable_warnings"`
	MainSequence       *MainSequenceSummary `json:"main_sequence"`
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
	maxPublicRatio := flag.Float64("max-public-ratio", 0.6, "high_public_ratio (参考情報) を出すしきい値。--strict の判定には効かない")
	sigma := flag.Float64("sigma", 2.0, "LOC 外れ値検出の σ 倍数")
	minSymbols := flag.Int("min-symbols", defaultMinSymbols, "high_public_ratio (参考情報) を出す最小シンボル数。--strict の判定には効かない")
	strict := flag.Bool("strict", false, "actionable な警告 (コードを直して消せるもの) がある場合に exit code 1 を返す")
	volatilityWindow := flag.Int("volatility-window", defaultVolatilityWindow, "揮発性を数える直近の commit 数 (merge を除く)")
	volatileCommits := flag.Int("volatile-commits", defaultVolatileCommits, "窓の中でこの数以上の commit に触られたパッケージを揮発とみなす")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "使用方法: analyze-modularity [flags] <directory>...\n")
		fmt.Fprintf(os.Stderr, "  各ディレクトリ内の Go パッケージを解析します｡\n")
		os.Exit(1)
	}
	// 窓 0 だと全パッケージが 0 commit になり、Zone of Pain がすべて非揮発に倒れて --strict が緩む
	if *volatilityWindow < 1 || *volatileCommits < 1 {
		fmt.Fprintf(os.Stderr, "エラー: --volatility-window と --volatile-commits は 1 以上を指定する\n")
		os.Exit(1)
	}

	coupling, packages, err := collectPackages(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
		os.Exit(1)
	}

	history, err := recentCommitCounts(inferModuleRoot(args), *volatilityWindow)
	if err != nil {
		fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
		os.Exit(1)
	}
	if history.unmeasuredReason != "" {
		fmt.Fprintf(os.Stderr, "注意: 揮発性を測りません (%s)｡Zone of Pain はすべて揮発として扱います\n", history.unmeasuredReason)
	}

	if err := enrichMainSequence(coupling, packages, history, *volatileCommits); err != nil {
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
		allWarnings = append(allWarnings, GenerateExportedReferenceWarnings(p)...)
		allWarnings = append(allWarnings, mainSequenceWarnings...)
		allWarnings = append(allWarnings, crossWarnings...)
	}

	actionableWarnings := 0
	for _, w := range allWarnings {
		if w.Actionable {
			actionableWarnings++
		}
	}

	summary := Summary{
		MeanLOC:            locStats.Mean,
		StddevLOC:          locStats.Stddev,
		MeanPublicRatio:    ratioStats.Mean,
		OutlierPackages:    outliers,
		TotalPackages:      len(packages),
		TotalWarnings:      len(allWarnings),
		ActionableWarnings: actionableWarnings,
		MainSequence:       buildMainSequenceSummary(packages, history, *volatilityWindow, *volatileCommits),
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

	if *strict && report.Summary.ActionableWarnings > 0 {
		os.Exit(1)
	}
}

// collectPackages は測る対象を決めて読む｡測るのは構成管理の対象だけ: go list が列挙するパッケージ
// (入れ子のモジュールと vendor を含まない) から git が ignore するものを落とし、args の下にあるものを
// 解析する｡返す Ca/Ce はディレクトリをキーにし、ignore されたパッケージからの参照も向けた参照も数えない｡
func collectPackages(args []string) (map[string]PackageMetrics, []PackageResult, error) {
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
	loaded, err := loadTypedPackages(moduleRoot)
	if err != nil {
		return nil, nil, err
	}
	usages := analyzeExportedReferences(loaded, tracked)

	var packages []PackageResult
	for _, s := range selected {
		result, err := AnalyzeDir(s.Dir, s.GoFiles)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", s.Path, err)
		}
		result.Path = s.Path
		// main はどこからも import できず、exported でも変更の影響が外へ広がらないので判定しない
		if s.Name != "main" {
			usage, ok := usages[s.ImportPath]
			if !ok {
				return nil, nil, fmt.Errorf("型情報の読み込み結果に %s が無い", s.ImportPath)
			}
			result.UnreferencedExported = new(len(usage.Unreferenced))
			result.UnreferencedExportedSymbols = usage.Unreferenced
			result.TestOnlyExported = new(len(usage.TestOnly))
			result.TestOnlyExportedSymbols = usage.TestOnly
			result.internal = slices.Contains(strings.Split(s.ImportPath, "/"), "internal")
		}
		packages = append(packages, result)
	}
	if len(packages) == 0 {
		return nil, nil, fmt.Errorf("解析可能な Go パッケージが見つかりません")
	}
	return computeCoupling(loaded, tracked), packages, nil
}

// enrichMainSequence は coupling (computeCoupling の Ca/Ce) と AST の型カウントから出す A で
// 各パッケージの MainSequence を埋める｡揮発性は history を volatileCommits で切って決める｡
func enrichMainSequence(coupling map[string]PackageMetrics, packages []PackageResult, history commitHistory, volatileCommits int) error {
	// go list の Dir は絶対パスで、報告側の Path は引数のまま (相対もある)。symlink の解決度も
	// 違いうるので、両方を実体の絶対パスにそろえてから突き合わせる
	byDir := make(map[string]PackageMetrics, len(coupling))
	for dir, m := range coupling {
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
		volatility, recentCommits := history.volatilityOf(dir, volatileCommits)
		packages[i].MainSequence = &MainSequenceData{
			Zone:                    classifyZone(m),
			Distance:                m.Distance,
			StructurallyConstrained: isStructurallyConstrained(m),
			Abstractness:            m.Abstractness,
			Instability:             m.Instability,
			AfferentCoupling:        m.AfferentCoupling,
			EfferentCoupling:        m.EfferentCoupling,
			RecentCommitCount:       recentCommits,
			Volatility:              volatility,
		}
	}
	return nil
}

// buildMainSequenceSummary はパッケージの Ca/Ce/A/I/D からサマリを生成する｡
// 揮発性を決めた窓・しきい値・測れなかった理由も、actionable_* の前提として一緒に載せる｡
func buildMainSequenceSummary(packages []PackageResult, history commitHistory, volatilityWindow, volatileCommits int) *MainSequenceSummary {
	summary := &MainSequenceSummary{
		VolatilityWindowCommits:    volatilityWindow,
		VolatileCommitThreshold:    volatileCommits,
		VolatilityUnmeasuredReason: history.unmeasuredReason,
	}
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
		if !isMainSequenceActionable(p.MainSequence) {
			summary.NonVolatileZoneOfPainCount++
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
