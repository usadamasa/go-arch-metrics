package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"io"
	"log"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/securego/gosec/v2"
	"github.com/securego/gosec/v2/analyzers"
	"github.com/securego/gosec/v2/issue"
	"github.com/securego/gosec/v2/rules"
	"golang.org/x/tools/go/packages"
)

const gosecModule = "github.com/securego/gosec/v2"

// measureGosec は gosec を組み込みで回し、gosec -fmt json と同じ形のレポートを返す。設定は CLI の既定と
// 同じで、テストファイルは解析しない。
//
// 型検査は ignore されたファイルも含めて行い、AST のルールには追跡されるファイルだけを渡す。Stats の files・
// lines・nosec はそのファイルだけを数える。SSA の解析はパッケージ全体が要るので全体で回し、指摘を
// ignore されたファイルの分だけ除く。AST のルールのうち、パッケージのほかのファイルから定数を探すもの
// (G201、G202) は、ignore されたファイルにある定数を見ない
func measureGosec(w *section, target target) (json.RawMessage, error) {
	w.println("--- gosec (セキュリティ解析) ---")
	cfg := &packages.Config{Mode: gosec.LoadMode, Dir: target.root}
	pkgs, err := packages.Load(cfg, target.packages()...)
	if err != nil {
		return nil, fmt.Errorf("gosec の対象のパッケージを読み込めません: %w", err)
	}

	analyzer := gosec.NewAnalyzer(gosec.NewConfig(), false, false, false, 1, log.New(io.Discard, "", 0))
	analyzer.LoadRules(rules.Generate(false).RulesInfo())
	analyzer.LoadAnalyzers(analyzers.Generate(false).AnalyzersInfo())

	// コンパイルできないパッケージは解析されず、指摘数が少なく見えるので止める。ignore されたファイルの
	// エラーでも、同じパッケージの追跡されるファイルが解析されないので止める
	compileErrors := map[string][]gosec.Error{}
	for _, pkg := range pkgs {
		errs, err := gosec.ParseErrors(pkg)
		if err != nil {
			return nil, fmt.Errorf("gosec が %s のエラーを読めません: %w", pkg.PkgPath, err)
		}
		for file, e := range errs {
			compileErrors[file] = append(compileErrors[file], e...)
		}
	}
	if len(compileErrors) > 0 {
		var lines []string
		for _, file := range sortedKeys(compileErrors) {
			for _, e := range compileErrors[file] {
				lines = append(lines, fmt.Sprintf("  %s:%d:%d: %s", file, e.Line, e.Column, e.Err))
			}
		}
		return nil, fmt.Errorf("gosec が対象のコードをコンパイルできませんでした。理由:\n%s", strings.Join(lines, "\n"))
	}

	for _, pkg := range pkgs {
		trackedFiles := *pkg
		trackedFiles.Syntax = slices.DeleteFunc(slices.Clone(pkg.Syntax), func(f *ast.File) bool {
			return target.selected.Ignored(pkg.Fset.File(f.Pos()).Name())
		})
		analyzer.CheckRules(&trackedFiles)
		analyzer.CheckAnalyzers(pkg)
	}
	issues, metrics, errs := analyzer.Report()
	issues = slices.DeleteFunc(issues, func(i *issue.Issue) bool { return target.ignored(i.File) })
	// CLI と同じく、絞り込んだ後の件数にそろえる
	metrics.NumFound = len(issues)

	// gosec -fmt json は ReportInfo をそのまま JSON にする (report/json)。sarif などの依存を持ち込まないよう、
	// report パッケージを通さずに書く
	info := gosec.NewReportInfo(issues, metrics, errs).WithVersion(moduleVersion(gosecModule))
	out, err := marshal(info)
	if err != nil {
		return nil, fmt.Errorf("gosec のレポートを組み立てられません: %w", err)
	}

	ids := make([]string, len(issues))
	for i, is := range issues {
		ids[i] = is.RuleID
	}
	printCounts(w, "セキュリティ指摘数", ids)
	return out, nil
}

// moduleVersion は組み込んだ module の版を返す。分からなければ空
func moduleVersion(path string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, dep := range info.Deps {
		if dep.Path == path {
			return dep.Version
		}
	}
	return ""
}
