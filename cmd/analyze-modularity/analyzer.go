package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// SPMData は spm-go のメトリクスと Zone 分類結果を保持する｡
// analyze-modularity に --spm-json が渡された場合のみ設定される｡
type SPMData struct {
	Zone                    SPMZone `json:"zone"`
	Distance                float64 `json:"distance"`
	StructurallyConstrained bool    `json:"structurally_constrained"`
	Abstractness            float64 `json:"abstractness"`
	Instability             float64 `json:"instability"`
	AfferentCoupling        int     `json:"afferent_coupling"`
	EfferentCoupling        int     `json:"efferent_coupling"`
}

// PackageResult はパッケージごとの解析結果を保持する｡
type PackageResult struct {
	Path              string  `json:"path"`
	ExportedFuncs     int     `json:"exported_funcs"`
	UnexportedFuncs   int     `json:"unexported_funcs"`
	ExportedMethods   int     `json:"exported_methods"`
	UnexportedMethods int     `json:"unexported_methods"`
	ExportedTypes     int     `json:"exported_types"`
	UnexportedTypes   int     `json:"unexported_types"`
	PublicRatioValue  float64 `json:"public_ratio"`
	LOC               int     `json:"loc"`
	Files             int     `json:"files"`

	// SPM (--spm-json 指定時のみ)
	SPM *SPMData `json:"spm,omitempty"`
}

// PublicRatio は exported/(exported+unexported) の比率を返す｡
func (r PackageResult) PublicRatio() float64 {
	exported := r.ExportedFuncs + r.ExportedMethods + r.ExportedTypes
	total := exported + r.UnexportedFuncs + r.UnexportedMethods + r.UnexportedTypes
	if total == 0 {
		return 0
	}
	return float64(exported) / float64(total)
}

// Stats は統計サマリを保持する｡
type Stats struct {
	Mean   float64 `json:"mean"`
	Stddev float64 `json:"stddev"`
}

// Warning は閾値超過の警告を表す｡
type Warning struct {
	Type    string  `json:"type"`
	Package string  `json:"package"`
	Value   float64 `json:"value"`
	Limit   float64 `json:"limit"`
	Message string  `json:"message"`
}

// 警告タイプ定数｡
const (
	WarningTypeHighPublicRatio         = "high_public_ratio"
	WarningTypeLocOutlier              = "loc_outlier"
	WarningTypeZoneOfPain              = "zone_of_pain"
	WarningTypeZoneOfUselessness       = "zone_of_uselessness"
	WarningTypeStructurallyConstrained = "structurally_constrained"
	WarningTypeHighDistance            = "high_distance"
	WarningTypeConstrainedLocOutlier   = "constrained_loc_outlier"
)

// mainSequenceDistanceThreshold は Main Sequence からの許容最大距離｡
// Martin, Clean Architecture, Chapter 14 に基づく｡
const mainSequenceDistanceThreshold = 0.5

// AnalyzeDir はディレクトリ内の Go ソースファイルを解析する｡
// テストファイル (*_test.go) は除外する｡
func AnalyzeDir(dir string) (PackageResult, error) {
	goFiles, err := listGoSourceFiles(dir)
	if err != nil {
		return PackageResult{}, err
	}

	if len(goFiles) == 0 {
		return PackageResult{}, fmt.Errorf("go ソースファイルが見つかりません: %s", dir)
	}

	result := PackageResult{
		Path:  dir,
		Files: len(goFiles),
	}

	fset := token.NewFileSet()
	for _, name := range goFiles {
		fullPath := filepath.Join(dir, name)
		if err := analyzeFile(fset, fullPath, &result); err != nil {
			return PackageResult{}, err
		}
	}

	result.PublicRatioValue = result.PublicRatio()
	return result, nil
}

// listGoSourceFiles はディレクトリ内の Go ソースファイル名を返す (テストファイル除外)｡
func listGoSourceFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("ディレクトリの読み込みに失敗: %w", err)
	}

	var goFiles []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			goFiles = append(goFiles, name)
		}
	}
	return goFiles, nil
}

// analyzeFile は単一の Go ファイルを解析し、結果を result に加算する｡
func analyzeFile(fset *token.FileSet, fullPath string, result *PackageResult) error {
	src, err := os.ReadFile(fullPath) // #nosec G304 -- listGoSourceFiles で列挙した既知ファイル
	if err != nil {
		return fmt.Errorf("ファイルの読み込みに失敗: %w", err)
	}

	f, err := parser.ParseFile(fset, fullPath, src, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("パースに失敗: %w", err)
	}

	file := fset.File(f.Pos())
	if file != nil {
		result.LOC += file.LineCount()
	}

	countDeclarations(f, result)
	return nil
}

// countDeclarations は AST から関数・メソッド・型の宣言をカウントする｡
func countDeclarations(f *ast.File, result *PackageResult) {
	ast.Inspect(f, func(n ast.Node) bool {
		switch decl := n.(type) {
		case *ast.FuncDecl:
			countFuncDecl(decl, result)
		case *ast.GenDecl:
			countTypeDecl(decl, result)
		}
		return true
	})
}

// countFuncDecl は関数/メソッド宣言をカウントする｡
func countFuncDecl(decl *ast.FuncDecl, result *PackageResult) {
	exported := ast.IsExported(decl.Name.Name)
	isMethod := decl.Recv != nil

	switch {
	case isMethod && exported:
		result.ExportedMethods++
	case isMethod:
		result.UnexportedMethods++
	case exported:
		result.ExportedFuncs++
	default:
		result.UnexportedFuncs++
	}
}

// countTypeDecl は型宣言をカウントする｡
func countTypeDecl(decl *ast.GenDecl, result *PackageResult) {
	if decl.Tok != token.TYPE {
		return
	}
	for _, spec := range decl.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		if ast.IsExported(ts.Name.Name) {
			result.ExportedTypes++
		} else {
			result.UnexportedTypes++
		}
	}
}

// CalcStats は値のスライスから平均と標準偏差を計算する｡
func CalcStats(values []float64) Stats {
	n := len(values)
	if n == 0 {
		return Stats{}
	}

	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(n)

	if n == 1 {
		return Stats{Mean: mean}
	}

	var sumSqDiff float64
	for _, v := range values {
		diff := v - mean
		sumSqDiff += diff * diff
	}
	stddev := math.Sqrt(sumSqDiff / float64(n))

	return Stats{Mean: mean, Stddev: stddev}
}

// DetectOutliers は LOC が mean + sigma*stddev を超えるパッケージを検出する｡
func DetectOutliers(packages []PackageResult, sigma float64) []string {
	locs := make([]float64, len(packages))
	for i, p := range packages {
		locs[i] = float64(p.LOC)
	}

	stats := CalcStats(locs)
	threshold := stats.Mean + sigma*stats.Stddev

	var outliers []string
	for _, p := range packages {
		if float64(p.LOC) > threshold {
			outliers = append(outliers, p.Path)
		}
	}
	return outliers
}

// TotalSymbols はパッケージ内の宣言シンボル総数を返す｡
func (r PackageResult) TotalSymbols() int {
	return r.ExportedFuncs + r.UnexportedFuncs +
		r.ExportedMethods + r.UnexportedMethods +
		r.ExportedTypes + r.UnexportedTypes
}

// defaultMinSymbols は public ratio チェックを適用する最小シンボル数のデフォルト値｡
//
// シンボル数 n が小さいとき、比率 p = exported/total は統計的に不安定になる｡
// n=3, p=1.0 の Wilson スコア区間 (95%) は約 [0.29, 1.00] であり、
// 真の比率が 29%～100% のどこにあるか判別できない｡
//
// 閾値 10 の根拠:
//   - 統計: n ≥ 10 で Wilson 区間の幅が ±0.3 以下に収まり、比率推定が安定する
//     (Agresti & Coull, 1998, "Approximate Is Better than 'Exact' for
//     Interval Estimation of Binomial Proportions")
//   - Martin の Zone of Pain 除外: 小規模・具象・非可変のユーティリティパッケージは
//     メトリクス警告の対象外とすべき
//     (Robert C. Martin, Clean Architecture, Chapter 14)
//   - CRP (Common Reuse Principle): 意味のあるパッケージは協調する複数のシンボルを持つ｡
//     10 未満のシンボル群では凝集度/カプセル化パターンの測定が困難
//     (Martin & Martin, Agile Principles, Patterns, and Practices in C#, Chapter 28)
const defaultMinSymbols = 10

// GenerateSPMWarnings は SPM データに基づく警告を生成する｡
// SPM データがない場合は空を返す｡
func GenerateSPMWarnings(pkg PackageResult) []Warning {
	if pkg.SPM == nil {
		return nil
	}
	spm := pkg.SPM
	var warnings []Warning

	switch spm.Zone {
	case SPMZoneOfPain:
		warnings = append(warnings, Warning{
			Type:    WarningTypeZoneOfPain,
			Package: pkg.Path,
			Value:   spm.Distance,
			Message: fmt.Sprintf("Zone of Pain: 具象的 (A=%.2f) で安定 (I=%.2f)", spm.Abstractness, spm.Instability),
		})
	case SPMZoneOfUselessness:
		warnings = append(warnings, Warning{
			Type:    WarningTypeZoneOfUselessness,
			Package: pkg.Path,
			Value:   spm.Distance,
			Message: fmt.Sprintf("Zone of Uselessness: 抽象的 (A=%.2f) で不安定 (I=%.2f)", spm.Abstractness, spm.Instability),
		})
	case SPMZoneExcluded:
		return nil
	}

	if spm.StructurallyConstrained {
		warnings = append(warnings, Warning{
			Type:    WarningTypeStructurallyConstrained,
			Package: pkg.Path,
			Value:   spm.Distance,
			Message: fmt.Sprintf("構造制約: Ce=0, Ca=%d のため D=%.2f はトポロジー由来", spm.AfferentCoupling, spm.Distance),
		})
	}

	// Excluded は上の switch で早期リターン済みなので Zone チェック不要
	if !spm.StructurallyConstrained && spm.Distance > mainSequenceDistanceThreshold {
		warnings = append(warnings, Warning{
			Type:    WarningTypeHighDistance,
			Package: pkg.Path,
			Value:   spm.Distance,
			Limit:   mainSequenceDistanceThreshold,
			Message: fmt.Sprintf("Main Sequence から大きく外れています (D=%.2f > %.1f)", spm.Distance, mainSequenceDistanceThreshold),
		})
	}

	return warnings
}

// GenerateCrossWarnings は AST 警告と SPM データを照合して複合警告を生成する｡
// 構造制約パッケージが LOC 外れ値でもある場合、constrained_loc_outlier 警告を追加する｡
func GenerateCrossWarnings(pkg PackageResult, astWarnings []Warning) []Warning {
	if pkg.SPM == nil || !pkg.SPM.StructurallyConstrained {
		return nil
	}

	for _, w := range astWarnings {
		if w.Type == WarningTypeLocOutlier {
			return []Warning{{
				Type:    WarningTypeConstrainedLocOutlier,
				Package: pkg.Path,
				Value:   w.Value,
				Limit:   w.Limit,
				Message: fmt.Sprintf("構造制約 leaf パッケージが肥大化しています (LOC %.0f > %.0f)", w.Value, w.Limit),
			}}
		}
	}
	return nil
}

// minLOCLimit は loc_outlier が使う閾値の下限｡
//
// loc_outlier は「平均 + sigma * 標準偏差」という相対的な指標なので、パッケージを
// 細かく分割すると平均が下がり、それにつれて閾値も下がる｡結果、分割したのとは
// 無関係なパッケージが 1 行も変わっていないのに外れ値として現れる (実際に、ある
// コマンドを internal へ分割した際、無関係な既存パッケージが 899 行 > 870 行で
// 警告になった)｡
//
// 絶対値として妥当な大きさのパッケージまで「大きすぎる」と言わないよう、閾値には
// 下限を設ける｡高々この行数なら Go のパッケージとして普通の範囲であり、統計的に
// どれだけ外れていても指摘する意味が無い｡high_public_ratio が minSymbols 未満の
// パッケージを免除しているのと同じ考え方｡
const minLOCLimit = 1000

// GenerateWarnings はパッケージの警告を生成する｡
// minSymbols 未満のシンボル数のパッケージでは比率メトリクスが統計的に不安定なため
// high_public_ratio チェックをスキップする｡同じ理由で loc_outlier にも下限
// (minLOCLimit) を設けている — 詳細は minLOCLimit の宣言を参照｡
func GenerateWarnings(pkg PackageResult, maxPublicRatio, sigma float64, locStats Stats, minSymbols int) []Warning {
	var warnings []Warning

	ratio := pkg.PublicRatio()
	if ratio > maxPublicRatio && pkg.TotalSymbols() >= minSymbols {
		warnings = append(warnings, Warning{
			Type:    WarningTypeHighPublicRatio,
			Package: pkg.Path,
			Value:   ratio,
			Limit:   maxPublicRatio,
			Message: fmt.Sprintf("API surface が大きすぎます (%.2f > %.2f)", ratio, maxPublicRatio),
		})
	}

	locThreshold := math.Max(locStats.Mean+sigma*locStats.Stddev, minLOCLimit)
	if float64(pkg.LOC) > locThreshold {
		warnings = append(warnings, Warning{
			Type:    WarningTypeLocOutlier,
			Package: pkg.Path,
			Value:   float64(pkg.LOC),
			Limit:   locThreshold,
			Message: fmt.Sprintf("パッケージが統計的外れ値です (LOC %d > %.0f)", pkg.LOC, locThreshold),
		})
	}

	return warnings
}
