package main

import (
	"reflect"
	"testing"
)

// mirrorInput は「設定がパッケージツリーを転記している」状態を模したデータ｡
// 3 component のうち 2 つが 1 パッケージしか持たず、cmd -> cmd/internal の辺は
// Go の internal 規則が既にスコープしている｡
func mirrorInput() (archConfig, graph) {
	cfg := archConfig{
		Components: map[string]any{"app": nil, "app_foo": nil, "app_bar": nil, "shared": nil},
		Vendors:    map[string]vendorSpec{"cobra": {In: flexStrings{"github.com/spf13/cobra"}}},
		Deps: map[string]depSpec{
			"app": {
				MayDependOn: []string{"app_foo", "app_bar", "shared"},
				CanUse:      []string{"cobra"},
			},
			"app_foo": {MayDependOn: []string{"app_bar"}},
		},
	}
	g := graph{
		componentPkgs: map[string][]string{
			"app":     {"app"},
			"app_foo": {"app/internal/foo"},
			"app_bar": {"app/internal/bar"},
			"shared":  {"internal/pathutil", "internal/settings"},
		},
		imports: map[string][]string{
			"app":               {"app/internal/foo", "github.com/spf13/cobra"},
			"app/internal/foo":  {},
			"app/internal/bar":  {},
			"internal/pathutil": {},
			"internal/settings": {},
		},
		testImports: map[string][]string{
			"app/internal/foo": {"app/internal/bar"},
		},
	}
	return cfg, g
}

// collapsedInput は役割ごとに畳んだ状態｡singleton も compiler-scoped も無い｡
func collapsedInput() (archConfig, graph) {
	cfg := archConfig{
		Components: map[string]any{"cmd_main": nil, "cmd_internal": nil, "shared_internal": nil},
		Deps: map[string]depSpec{
			"cmd_main":     {MayDependOn: []string{"cmd_internal", "shared_internal"}},
			"cmd_internal": {MayDependOn: []string{"cmd_internal", "shared_internal"}},
		},
	}
	g := graph{
		componentPkgs: map[string][]string{
			"cmd_main":        {"app", "other"},
			"cmd_internal":    {"app/internal/foo", "app/internal/bar", "other/internal/baz"},
			"shared_internal": {"internal/pathutil", "internal/settings"},
		},
		imports: map[string][]string{
			"app":                {"app/internal/foo", "internal/pathutil"},
			"other":              {"other/internal/baz"},
			"app/internal/foo":   {"app/internal/bar"},
			"app/internal/bar":   {},
			"other/internal/baz": {},
			"internal/pathutil":  {},
			"internal/settings":  {},
		},
		testImports: map[string][]string{},
	}
	return cfg, g
}

func TestAnalyzeCounts(t *testing.T) {
	tests := []struct {
		name            string
		input           func() (archConfig, graph)
		components      int
		singletons      int
		packages        int
		edges           int
		unused          int
		testOnly        int
		compilerScoped  int
		maxFanOutName   string
		maxFanOutCount  int
		uncoveredLength int
		maxScopeRoot    string
		maxScopeCount   int
	}{
		{
			name:       "パッケージツリーの転記",
			input:      mirrorInput,
			components: 4,
			// app / app_foo / app_bar が 1 パッケージずつ
			singletons: 3,
			packages:   5,
			// app の 3 辺 + app_foo の 1 辺
			edges: 4,
			// app -> app_bar と app -> shared に裏付けが無い
			unused: 2,
			// app_foo -> app_bar は _test.go だけが使う
			testOnly: 1,
			// app -> app_foo / app_bar と app_foo -> app_bar は
			// Go の internal 規則が app/ 配下へスコープ済み
			compilerScoped: 3,
			maxFanOutName:  "app",
			maxFanOutCount: 3,
			// app/ 配下の internal は foo と bar の 2 つ
			maxScopeRoot:  "app",
			maxScopeCount: 2,
		},
		{
			name:           "役割ごとに畳んだ状態",
			input:          collapsedInput,
			components:     3,
			singletons:     0,
			packages:       7,
			edges:          4,
			unused:         1, // cmd_internal -> shared_internal は未使用
			testOnly:       0,
			compilerScoped: 0,
			maxFanOutName:  "cmd_internal", // 同数のときは名前順で安定させる
			maxFanOutCount: 2,
			// app/ が 2、other/ が 1
			maxScopeRoot:  "app",
			maxScopeCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, g := tt.input()
			got := analyze(cfg, g)

			checks := []struct {
				label string
				got   int
				want  int
			}{
				{"components", got.Components, tt.components},
				{"singletons", got.SingletonComponents, tt.singletons},
				{"packages", got.Packages, tt.packages},
				{"edges", got.DeclaredEdges, tt.edges},
				{"unused", got.UnusedEdges, tt.unused},
				{"testOnly", got.TestOnlyEdges, tt.testOnly},
				{"compilerScoped", got.CompilerScopedEdges, tt.compilerScoped},
				{"maxFanOut", got.MaxFanOut.Count, tt.maxFanOutCount},
				{"uncovered", len(got.UncoveredPackages), tt.uncoveredLength},
				{"maxScope", got.MaxScopePackages.Count, tt.maxScopeCount},
			}
			for _, c := range checks {
				if c.got != c.want {
					t.Errorf("%s = %d, want %d", c.label, c.got, c.want)
				}
			}
			if got.MaxFanOut.Component != tt.maxFanOutName {
				t.Errorf("maxFanOut.Component = %q, want %q", got.MaxFanOut.Component, tt.maxFanOutName)
			}
			if got.MaxScopePackages.Root != tt.maxScopeRoot {
				t.Errorf("maxScopePackages.Root = %q, want %q", got.MaxScopePackages.Root, tt.maxScopeRoot)
			}
		})
	}
}

func TestAnalyzeFindings(t *testing.T) {
	cfg, g := mirrorInput()
	got := analyze(cfg, g)

	var unused, testOnly []string
	for _, f := range got.Findings {
		switch f.Kind {
		case kindUnusedEdge:
			unused = append(unused, f.Component+" -> "+f.Target)
		case kindTestOnlyEdge:
			testOnly = append(testOnly, f.Component+" -> "+f.Target)
		}
	}

	wantUnused := []string{"app -> app_bar", "app -> shared"}
	if !reflect.DeepEqual(unused, wantUnused) {
		t.Errorf("unused findings = %v, want %v", unused, wantUnused)
	}
	wantTestOnly := []string{"app_foo -> app_bar"}
	if !reflect.DeepEqual(testOnly, wantTestOnly) {
		t.Errorf("test-only findings = %v, want %v", testOnly, wantTestOnly)
	}
}

// canUse に宣言した vendor が実際には import されていない場合も未使用として拾う｡
func TestAnalyzeUnusedVendor(t *testing.T) {
	cfg, g := mirrorInput()
	g.imports["app"] = []string{"app/internal/foo"} // cobra を使わなくする
	got := analyze(cfg, g)

	found := false
	for _, f := range got.Findings {
		if f.Kind == kindUnusedVendor && f.Component == "app" && f.Target == "cobra" {
			found = true
		}
	}
	if !found {
		t.Errorf("未使用 vendor の finding が無い: %+v", got.Findings)
	}
}

func TestInternalScopeRoot(t *testing.T) {
	tests := []struct {
		pkg  string
		want string
	}{
		{"app", ""},
		{"internal/pathutil", ""},
		{"app/internal/foo", "app"},
		{"app/internal/foo/bar", "app"},
		{"a/internal/b/internal/c", "a/internal/b"},
	}
	for _, tt := range tests {
		if got := internalScopeRoot(tt.pkg); got != tt.want {
			t.Errorf("internalScopeRoot(%q) = %q, want %q", tt.pkg, got, tt.want)
		}
	}
}
