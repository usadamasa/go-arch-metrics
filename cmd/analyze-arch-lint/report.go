package main

import (
	"fmt"
	"sort"
	"strings"
)

// findingLabels は finding の kind ごとの見出し｡出力順もこの並びに従う｡
var findingLabels = []struct {
	kind  string
	label string
}{
	{kindUncoveredPkg, "どの component にも属さないパッケージ"},
	{kindEmptyComponent, "パッケージにマッチしない component"},
	{kindUnusedEdge, "未使用の mayDependOn"},
	{kindUnusedVendor, "未使用の canUse"},
	{kindTestOnlyEdge, "_test.go だけが使う mayDependOn"},
	{kindCompilerScoped, "Go の internal 規則が既にスコープしている mayDependOn"},
}

// renderReport は人間向けのレポートを組み立てる｡
// strings.Builder の Write はエラーを返さないので、書き出しは呼び出し側の 1 回だけになる｡
func renderReport(r report) string {
	var b strings.Builder

	fmt.Fprintf(&b, "component: %d 個 (うち 1 パッケージのみ: %d 個 / %.0f%%)\n",
		r.Components, r.SingletonComponents, r.singletonRatio()*100)
	fmt.Fprintf(&b, "パッケージ: %d 個 (どの component にも属さない: %d 個)\n",
		r.Packages, len(r.UncoveredPackages))
	fmt.Fprintf(&b, "mayDependOn: %d 辺 (未使用 %d / _test.go のみ %d / internal 規則でスコープ済み %d = %.0f%%)\n",
		r.DeclaredEdges, r.UnusedEdges, r.TestOnlyEdges, r.CompilerScopedEdges, r.compilerScopedRatio()*100)
	fmt.Fprintf(&b, "最大 fan-out: %s (%d 辺)\n", r.MaxFanOut.Component, r.MaxFanOut.Count)
	fmt.Fprintf(&b, "最大の internal スコープ: %s (%d パッケージ)\n",
		r.MaxScopePackages.Root, r.MaxScopePackages.Count)

	byKind := map[string][]finding{}
	for _, f := range r.Findings {
		byKind[f.Kind] = append(byKind[f.Kind], f)
	}
	for _, fl := range findingLabels {
		items := byKind[fl.kind]
		if len(items) == 0 {
			continue
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].Component != items[j].Component {
				return items[i].Component < items[j].Component
			}
			return items[i].Target < items[j].Target
		})
		fmt.Fprintf(&b, "\n%s (%d 件):\n", fl.label, len(items))
		for _, f := range items {
			if f.Target == "" {
				fmt.Fprintf(&b, "  %s\n", f.Component)
				continue
			}
			fmt.Fprintf(&b, "  %s -> %s\n", f.Component, f.Target)
		}
	}
	return b.String()
}
