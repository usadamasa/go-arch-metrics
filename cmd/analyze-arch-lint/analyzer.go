package main

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// flexStrings は go-arch-lint の `in:` を受ける｡スカラーとシーケンスの両方が来る｡
type flexStrings []string

func (f *flexStrings) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		*f = flexStrings{node.Value}
		return nil
	}
	var list []string
	if err := node.Decode(&list); err != nil {
		return fmt.Errorf("in: の解釈に失敗: %w", err)
	}
	*f = list
	return nil
}

type vendorSpec struct {
	In flexStrings `yaml:"in"`
}

type depSpec struct {
	MayDependOn []string `yaml:"mayDependOn"`
	CanUse      []string `yaml:"canUse"`
}

// archConfig は .go-arch-lint.yml のうち健全性評価に使う部分だけを持つ｡
// component の `in:` は使わない (glob の解釈は go-arch-lint mapping に任せる)｡
type archConfig struct {
	Components map[string]any        `yaml:"components"`
	Vendors    map[string]vendorSpec `yaml:"vendors"`
	Deps       map[string]depSpec    `yaml:"deps"`
}

// graph は設定の外から集めた実態｡パッケージ名はすべてモジュール相対で、
// vendor と標準ライブラリの import だけが完全なパスのまま入る｡
type graph struct {
	componentPkgs map[string][]string
	imports       map[string][]string
	testImports   map[string][]string
}

const (
	kindUnusedEdge     = "unused-edge"
	kindTestOnlyEdge   = "test-only-edge"
	kindCompilerScoped = "compiler-scoped-edge"
	kindUnusedVendor   = "unused-vendor"
	kindEmptyComponent = "empty-component"
	kindUncoveredPkg   = "uncovered-package"
)

type finding struct {
	Kind      string `json:"kind"`
	Component string `json:"component"`
	Target    string `json:"target,omitempty"`
	Detail    string `json:"detail"`
}

type fanOut struct {
	Component string `json:"component"`
	Count     int    `json:"count"`
}

// scopeSize は 1 つの internal スコープ (= 1 つの cmd の木) が抱えるパッケージ数｡
// component を役割単位へ畳むと、木の中がいくら増えても設定は変わらなくなる｡
// その肥大化はここで見る｡
type scopeSize struct {
	Root  string `json:"root"`
	Count int    `json:"count"`
}

type report struct {
	Components          int       `json:"components"`
	SingletonComponents int       `json:"singleton_components"`
	Packages            int       `json:"packages"`
	UncoveredPackages   []string  `json:"uncovered_packages,omitempty"`
	DeclaredEdges       int       `json:"declared_edges"`
	UnusedEdges         int       `json:"unused_edges"`
	TestOnlyEdges       int       `json:"test_only_edges"`
	CompilerScopedEdges int       `json:"compiler_scoped_edges"`
	MaxFanOut           fanOut    `json:"max_fan_out"`
	MaxScopePackages    scopeSize `json:"max_scope_packages"`

	Findings []finding `json:"findings,omitempty"`
}

// singletonRatio は 1 パッケージしか含まない component の割合｡
// 1 に近いほど、設定がアーキテクチャではなくパッケージツリーの転記になっている｡
func (r report) singletonRatio() float64 {
	if r.Components == 0 {
		return 0
	}
	return float64(r.SingletonComponents) / float64(r.Components)
}

// compilerScopedRatio は Go の internal 規則が既にスコープしている辺の割合｡
// 1 に近いほど、設定はコンパイラの再実装になっている｡
func (r report) compilerScopedRatio() float64 {
	if r.DeclaredEdges == 0 {
		return 0
	}
	return float64(r.CompilerScopedEdges) / float64(r.DeclaredEdges)
}

// internalScopeRoot は Go の internal 規則が pkg の import 元を閉じ込める木の根を返す｡
// internal 要素が複数あるときは最も深いものが最も強い制約になる｡
// 制約が無い (モジュール全体から import できる) 場合は空文字列｡
func internalScopeRoot(pkg string) string {
	parts := strings.Split(pkg, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "internal" {
			return strings.Join(parts[:i], "/")
		}
	}
	return ""
}

// isCompilerScoped は「C -> D の辺が Go の internal 規則ですでにスコープ済みか」を返す｡
// D のすべてのパッケージが同じ木に閉じており、C のすべてのパッケージがその木の中に
// あるなら、arch-lint が追加しているのは木の内側の順序だけになる｡
func isCompilerScoped(fromPkgs, toPkgs []string) bool {
	if len(fromPkgs) == 0 || len(toPkgs) == 0 {
		return false
	}
	for _, to := range toPkgs {
		root := internalScopeRoot(to)
		if root == "" {
			return false
		}
		for _, from := range fromPkgs {
			if from != root && !strings.HasPrefix(from, root+"/") {
				return false
			}
		}
	}
	return true
}

// importsAnyOf は from の各パッケージの import に targets のいずれかが現れるかを返す｡
func importsAnyOf(importsBy map[string][]string, fromPkgs, targets []string) bool {
	for _, from := range fromPkgs {
		for _, imp := range importsBy[from] {
			if slices.Contains(targets, imp) {
				return true
			}
		}
	}
	return false
}

// usesVendor は from の各パッケージが prefixes のいずれかで始まる import を持つかを返す｡
func usesVendor(importsBy map[string][]string, fromPkgs []string, prefixes []string) bool {
	for _, from := range fromPkgs {
		for _, imp := range importsBy[from] {
			for _, p := range prefixes {
				if imp == p || strings.HasPrefix(imp, p+"/") {
					return true
				}
			}
		}
	}
	return false
}

// largestInternalScope は最も多くのパッケージを抱える internal スコープを返す｡
// 同数のときは名前順で安定させる｡
func largestInternalScope(pkgs []string) scopeSize {
	counts := map[string]int{}
	for _, p := range pkgs {
		if root := internalScopeRoot(p); root != "" {
			counts[root]++
		}
	}
	var largest scopeSize
	for _, root := range slices.Sorted(maps.Keys(counts)) {
		if counts[root] > largest.Count {
			largest = scopeSize{Root: root, Count: counts[root]}
		}
	}
	return largest
}

func analyze(cfg archConfig, g graph) report {
	r := report{
		Components:       len(cfg.Components),
		Packages:         len(g.imports),
		MaxScopePackages: largestInternalScope(sortedKeys(g.imports)),
	}

	covered := map[string]bool{}
	names := make([]string, 0, len(cfg.Components))
	for name := range cfg.Components {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		pkgs := g.componentPkgs[name]
		for _, p := range pkgs {
			covered[p] = true
		}
		switch len(pkgs) {
		case 0:
			r.Findings = append(r.Findings, finding{
				Kind: kindEmptyComponent, Component: name,
				Detail: "どのパッケージにもマッチしない",
			})
		case 1:
			r.SingletonComponents++
		}
	}

	for _, p := range sortedKeys(g.imports) {
		if !covered[p] {
			r.UncoveredPackages = append(r.UncoveredPackages, p)
			r.Findings = append(r.Findings, finding{
				Kind: kindUncoveredPkg, Component: p,
				Detail: "どの component にも属さない",
			})
		}
	}

	for _, name := range names {
		spec := cfg.Deps[name]
		fromPkgs := g.componentPkgs[name]

		if n := len(spec.MayDependOn); n > r.MaxFanOut.Count {
			r.MaxFanOut = fanOut{Component: name, Count: n}
		}

		for _, target := range spec.MayDependOn {
			r.DeclaredEdges++
			toPkgs := g.componentPkgs[target]

			if isCompilerScoped(fromPkgs, toPkgs) {
				r.CompilerScopedEdges++
				r.Findings = append(r.Findings, finding{
					Kind: kindCompilerScoped, Component: name, Target: target,
					Detail: "Go の internal 規則が既に同じ木へスコープしている",
				})
			}

			switch {
			case importsAnyOf(g.imports, fromPkgs, toPkgs):
			case importsAnyOf(g.testImports, fromPkgs, toPkgs):
				r.TestOnlyEdges++
				r.Findings = append(r.Findings, finding{
					Kind: kindTestOnlyEdge, Component: name, Target: target,
					Detail: "_test.go の import だけが使う",
				})
			default:
				r.UnusedEdges++
				r.Findings = append(r.Findings, finding{
					Kind: kindUnusedEdge, Component: name, Target: target,
					Detail: "実際の import に裏付けが無い",
				})
			}
		}

		for _, v := range spec.CanUse {
			prefixes := cfg.Vendors[v].In
			if usesVendor(g.imports, fromPkgs, prefixes) || usesVendor(g.testImports, fromPkgs, prefixes) {
				continue
			}
			r.Findings = append(r.Findings, finding{
				Kind: kindUnusedVendor, Component: name, Target: v,
				Detail: "canUse に宣言された vendor を import していない",
			})
		}
	}

	return r
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
