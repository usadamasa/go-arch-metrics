package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestCollectGraphIgnoresGitIgnored は git が ignore するディレクトリとファイルを component の対応にも
// import の集計にも入れないことを確かめる。go-arch-lint mapping はスタブで、ignore されたファイルも返す。
func TestCollectGraphIgnoresGitIgnored(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":     "module example.com/fx\n\ngo 1.26\n",
		"a/a.go":     "package a\n\nimport _ \"example.com/fx/b\"\n",
		"a/extra.go": "package a\n\nimport _ \"example.com/fx/c\"\n",
		"b/b.go":     "package b\n",
		"c/c.go":     "package c\n",
		"work/w.go":  "package work\n\nimport _ \"example.com/fx/a\"\n",
		".gitignore": "/work/\n/a/extra.go\n",
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init 失敗: %v\n%s", err, out)
	}

	mapping := fmt.Sprintf(`{"Payload":{"ProjectDirectory":%q,"ModuleName":"example.com/fx","MappingGrouped":[`+
		`{"ComponentName":"app","FileNames":[%q,%q]},`+
		`{"ComponentName":"lib","FileNames":[%q]},`+
		`{"ComponentName":"work","FileNames":[%q]},`+
		`{"ComponentName":"extra","FileNames":[%q]}]}}`,
		root, filepath.Join(root, "a", "extra.go"), filepath.Join(root, "a", "a.go"),
		filepath.Join(root, "b", "b.go"), filepath.Join(root, "work", "w.go"), filepath.Join(root, "a", "extra.go"))
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "mapping.json"), []byte(mapping), 0o600); err != nil {
		t.Fatal(err)
	}
	stub := "#!/bin/sh\ncat '" + filepath.Join(bin, "mapping.json") + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "go-arch-lint"), []byte(stub), 0o700); err != nil { // #nosec G306 -- テストのスタブは実行権限が要る
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	g, err := collectGraph(root, ".go-arch-lint.yml")
	if err != nil {
		t.Fatalf("collectGraph: %v", err)
	}

	want := map[string][]string{"app": {"a"}, "lib": {"b"}}
	if !reflect.DeepEqual(g.componentPkgs, want) {
		t.Errorf("componentPkgs = %v, want %v (ignore されたファイルだけの component は空)", g.componentPkgs, want)
	}
	if _, ok := g.imports["work"]; ok {
		t.Errorf("ignore された work が import の集計に入っている: %v", g.imports)
	}
	if !slices.Equal(g.imports["a"], []string{"b"}) {
		t.Errorf("imports[a] = %v, want [b] (ignore された extra.go の c は数えない)", g.imports["a"])
	}
}

func writeArchFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".go-arch-lint.yml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("テスト用 arch ファイルを書けません: %v", err)
	}
	return path
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantErr    bool
		wantVendor []string
		wantDeps   []string
	}{
		{
			name: "in: がスカラー",
			body: `
components:
  cmd_main:
    in: "*"
vendors:
  cobra:
    in: github.com/spf13/cobra
deps:
  cmd_main:
    canUse:
      - cobra
`,
			wantVendor: []string{"github.com/spf13/cobra"},
		},
		{
			name: "in: がシーケンス",
			body: `
components:
  cmd_main:
    in: "*"
vendors:
  yaml:
    in:
      - gopkg.in/yaml.v3
      - sigs.k8s.io/yaml
deps:
  cmd_main:
    mayDependOn:
      - cmd_main
    canUse:
      - yaml
`,
			wantVendor: []string{"gopkg.in/yaml.v3", "sigs.k8s.io/yaml"},
			wantDeps:   []string{"cmd_main"},
		},
		{
			name:    "components が無い",
			body:    "deps:\n  cmd_main:\n    mayDependOn: []\n",
			wantErr: true,
		},
		{
			name:    "YAML として壊れている",
			body:    "components:\n  - [\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := loadConfig(writeArchFile(t, tt.body))
			if tt.wantErr {
				if err == nil {
					t.Fatal("エラーを期待したが nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("予期しないエラー: %v", err)
			}
			for name, v := range cfg.Vendors {
				if !slices.Equal([]string(v.In), tt.wantVendor) {
					t.Errorf("vendors[%s].In = %v, want %v", name, v.In, tt.wantVendor)
				}
			}
			if got := cfg.Deps["cmd_main"].MayDependOn; !slices.Equal(got, tt.wantDeps) {
				t.Errorf("deps[cmd_main].mayDependOn = %v, want %v", got, tt.wantDeps)
			}
		})
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	if _, err := loadConfig(filepath.Join(t.TempDir(), "absent.yml")); err == nil {
		t.Fatal("存在しないファイルでエラーを期待したが nil")
	}
}

// TestNormalizeImportsRootPackage はモジュールルート直下のパッケージを go-arch-lint mapping と
// 同じ "." で表すことを確かめる｡キーが食い違うと、ルートの main は component に属さない
// ことになり、その import も辺の裏付けに数えられない｡
func TestNormalizeImportsRootPackage(t *testing.T) {
	const module = "example.com/app"
	pkgs := []packageImports{
		{importPath: module, imports: []string{module + "/cmd", module + "/internal/version", "fmt"}},
		{importPath: module + "/cmd", imports: []string{"github.com/spf13/cobra"}, testImports: []string{module}},
		{importPath: module + "/internal/version"},
	}

	imports, testImports := normalizeImports(pkgs, module)

	wantImports := map[string][]string{
		".":                {"cmd", "internal/version", "fmt"},
		"cmd":              {"github.com/spf13/cobra"},
		"internal/version": nil,
	}
	if !reflect.DeepEqual(imports, wantImports) {
		t.Errorf("imports = %v, want %v", imports, wantImports)
	}
	if got := testImports["cmd"]; !slices.Equal(got, []string{"."}) {
		t.Errorf("testImports[cmd] = %v, want [.]", got)
	}

	cfg := archConfig{
		Components: map[string]any{"cli": nil, "shared_internal": nil},
		Deps: map[string]depSpec{
			"cli": {MayDependOn: []string{"cli", "shared_internal"}},
		},
	}
	g := graph{
		componentPkgs: map[string][]string{
			"cli":             {".", "cmd"},
			"shared_internal": {"internal/version"},
		},
		imports:     imports,
		testImports: testImports,
	}
	r := analyze(cfg, g)
	if len(r.UncoveredPackages) != 0 || r.UnusedEdges != 0 {
		t.Errorf("uncovered = %v, unused edges = %d, want none", r.UncoveredPackages, r.UnusedEdges)
	}
}

func TestCheckThresholds(t *testing.T) {
	th := thresholds{
		singletonRatio: 0.5, compilerScopedRatio: 0.3,
		maxUnusedEdges: 0, maxScopePackages: 10,
	}

	tests := []struct {
		name     string
		r        report
		wantKeys []string
	}{
		{
			name: "しきい値内",
			r: report{
				Components: 3, SingletonComponents: 0,
				DeclaredEdges: 4, CompilerScopedEdges: 0, UnusedEdges: 0,
				MaxScopePackages: scopeSize{Root: "app", Count: 10},
			},
		},
		{
			name: "すべて超過",
			r: report{
				Components: 4, SingletonComponents: 4,
				DeclaredEdges: 10, CompilerScopedEdges: 8, UnusedEdges: 2,
				UncoveredPackages: []string{"stray"},
				MaxScopePackages:  scopeSize{Root: "app", Count: 11},
			},
			wantKeys: []string{
				"singleton-ratio", "compiler-scoped-ratio",
				"unused-edges", "uncovered-packages", "scope-packages",
			},
		},
		{
			name: "internal スコープの肥大化だけ超過",
			r: report{
				Components: 3, SingletonComponents: 0,
				DeclaredEdges: 4, CompilerScopedEdges: 0, UnusedEdges: 0,
				MaxScopePackages: scopeSize{Root: "app", Count: 11},
			},
			wantKeys: []string{"scope-packages"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := checkThresholds(tt.r, th)
			if len(got) != len(tt.wantKeys) {
				t.Fatalf("違反 %d 件 (%v), want %d 件", len(got), got, len(tt.wantKeys))
			}
			for i, key := range tt.wantKeys {
				if got[i].metric.key != key {
					t.Errorf("違反[%d] の metric = %q, want %q", i, got[i].metric.key, key)
				}
				// 意味と対処が添えられていないと、出力を見ただけでは直せない｡
				if !strings.Contains(got[i].String(), "対処: ") {
					t.Errorf("違反[%d] に対処が無い: %q", i, got[i].String())
				}
			}
		})
	}
}

// --metrics は全指標を意味と対処つきで並べる｡ドキュメント側の表を置き換えるので、
// gating な指標にはしきい値も出す｡
func TestRenderMetrics(t *testing.T) {
	out := renderMetrics(thresholds{
		singletonRatio: 0.5, compilerScopedRatio: 0.3,
		maxUnusedEdges: 0, maxScopePackages: 10,
	})
	for _, m := range metrics {
		if !strings.Contains(out, m.label) {
			t.Errorf("--metrics に %q が無い", m.label)
		}
		if !strings.Contains(out, m.remedy) {
			t.Errorf("--metrics に %q の対処が無い", m.key)
		}
	}
	if !strings.Contains(out, "しきい値: なし (報告のみ)") {
		t.Error("gating でない指標のしきい値表示が無い")
	}
}
