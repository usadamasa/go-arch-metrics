package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// writeFixtureModule は a → b, c / b → c / d → c / e → b の依存を持つモジュールを作り、ルートを返す｡
// iso は何にも依存されず何にも依存しない｡c だけが interface を持つ｡
func writeFixtureModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod":     "module example.com/fx\n\ngo 1.26\n",
		"a/a.go":     "package a\n\nimport (\n\t\"example.com/fx/b\"\n\t\"example.com/fx/c\"\n)\n\nfunc A() int { return b.B() + c.C() }\n",
		"b/b.go":     "package b\n\nimport \"example.com/fx/c\"\n\nfunc B() int { return c.C() }\n",
		"c/c.go":     "package c\n\ntype Doer interface{ Do() }\n\nfunc C() int { return 1 }\n",
		"d/d.go":     "package d\n\nimport \"example.com/fx/c\"\n\nfunc D() int { return c.C() }\n",
		"e/e.go":     "package e\n\nimport \"example.com/fx/b\"\n\nfunc E() int { return b.B() }\n",
		"iso/iso.go": "package iso\n\nfunc Iso() {}\n",
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("ディレクトリ作成失敗: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("ファイル作成失敗: %v", err)
		}
	}
	return root
}

func Test_classifyZone(t *testing.T) {
	tests := []struct {
		name string
		pkg  PackageMetrics
		want Zone
	}{
		{
			name: "main パッケージ → excluded",
			pkg:  PackageMetrics{Name: "main", Abstractness: 0.0, Instability: 0.0},
			want: ZoneExcluded,
		},
		{
			name: "孤立パッケージ (Ca=0, Ce=0) → excluded",
			pkg:  PackageMetrics{Name: "isolated", AfferentCoupling: 0, EfferentCoupling: 0},
			want: ZoneExcluded,
		},
		{
			name: "A < 0.5, I < 0.5 → zone_of_pain",
			pkg:  PackageMetrics{Name: "stable", Abstractness: 0.1, Instability: 0.1, AfferentCoupling: 5, EfferentCoupling: 1},
			want: ZoneOfPain,
		},
		{
			name: "A > 0.5, I > 0.5 → zone_of_uselessness",
			pkg:  PackageMetrics{Name: "abstract", Abstractness: 0.8, Instability: 0.8, AfferentCoupling: 1, EfferentCoupling: 5},
			want: ZoneOfUselessness,
		},
		{
			name: "A=0.1, I=0.9 → main_sequence",
			pkg:  PackageMetrics{Name: "normal", Abstractness: 0.1, Instability: 0.9, AfferentCoupling: 1, EfferentCoupling: 5},
			want: ZoneMainSequence,
		},
		{
			name: "境界値 A=0.5, I=0.5 → main_sequence",
			pkg:  PackageMetrics{Name: "boundary", Abstractness: 0.5, Instability: 0.5, AfferentCoupling: 3, EfferentCoupling: 3},
			want: ZoneMainSequence,
		},
		{
			name: "A=0.5, I=0.3 → main_sequence (A が境界値で zone_of_pain にならない)",
			pkg:  PackageMetrics{Name: "edge1", Abstractness: 0.5, Instability: 0.3, AfferentCoupling: 3, EfferentCoupling: 1},
			want: ZoneMainSequence,
		},
		{
			name: "A=0.3, I=0.5 → main_sequence (I が境界値で zone_of_pain にならない)",
			pkg:  PackageMetrics{Name: "edge2", Abstractness: 0.3, Instability: 0.5, AfferentCoupling: 1, EfferentCoupling: 3},
			want: ZoneMainSequence,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyZone(tt.pkg)
			if got != tt.want {
				t.Errorf("classifyZone(%+v) = %q, want %q", tt.pkg, got, tt.want)
			}
		})
	}
}

func Test_isStructurallyConstrained(t *testing.T) {
	tests := []struct {
		name string
		pkg  PackageMetrics
		want bool
	}{
		{name: "Ce=0, Ca=5 → 構造制約あり", pkg: PackageMetrics{EfferentCoupling: 0, AfferentCoupling: 5}, want: true},
		{name: "Ce=0, Ca=0 → 孤立 (構造制約なし)", pkg: PackageMetrics{EfferentCoupling: 0, AfferentCoupling: 0}, want: false},
		{name: "Ce=3, Ca=5 → 構造制約なし", pkg: PackageMetrics{EfferentCoupling: 3, AfferentCoupling: 5}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStructurallyConstrained(tt.pkg)
			if got != tt.want {
				t.Errorf("isStructurallyConstrained(%+v) = %v, want %v", tt.pkg, got, tt.want)
			}
		})
	}
}

func Test_instability(t *testing.T) {
	tests := []struct {
		name   string
		ca, ce int
		want   float64
	}{
		{name: "Ca=0, Ce=0 (孤立) → 1", ca: 0, ce: 0, want: 1},
		{name: "Ca=3, Ce=0 (葉) → 0", ca: 3, ce: 0, want: 0},
		{name: "Ca=0, Ce=3 → 1", ca: 0, ce: 3, want: 1},
		{name: "Ca=2, Ce=1 → 1/3", ca: 2, ce: 1, want: 1.0 / 3.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := instability(tt.ca, tt.ce)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("instability(%d, %d) = %v, want %v", tt.ca, tt.ce, got, tt.want)
			}
		})
	}
}

func Test_distance(t *testing.T) {
	tests := []struct {
		name string
		a, i float64
		want float64
	}{
		{name: "A=0, I=1 → 0", a: 0, i: 1, want: 0},
		{name: "A=0, I=0 → 1", a: 0, i: 0, want: 1},
		{name: "A=1, I=1 → 1", a: 1, i: 1, want: 1},
		{name: "A=0.5, I=1/3 → 1/6", a: 0.5, i: 1.0 / 3.0, want: 1.0 / 6.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := distance(tt.a, tt.i)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("distance(%v, %v) = %v, want %v", tt.a, tt.i, got, tt.want)
			}
		})
	}
}

func Test_computeCoupling(t *testing.T) {
	// 標準ライブラリと外部モジュールへの import はモジュール内の辺ではないので数えない
	pkgs := []listedPackage{
		{ImportPath: "m/a", Name: "a", Dir: "/m/a", Imports: []string{"fmt", "m/b", "m/c"}},
		{ImportPath: "m/b", Name: "b", Dir: "/m/b", Imports: []string{"m/c", "github.com/x/y"}},
		{ImportPath: "m/c", Name: "c", Dir: "/m/c", Imports: []string{"os"}},
		{ImportPath: "m/d", Name: "d", Dir: "/m/d", Imports: []string{"m/c"}},
		{ImportPath: "m/e", Name: "e", Dir: "/m/e", Imports: []string{"m/b"}},
		{ImportPath: "m/iso", Name: "iso", Dir: "/m/iso", Imports: nil},
	}

	got := computeCoupling(pkgs)

	want := map[string]struct{ ca, ce int }{
		"/m/a":   {0, 2},
		"/m/b":   {2, 1},
		"/m/c":   {3, 0},
		"/m/d":   {0, 1},
		"/m/e":   {0, 1},
		"/m/iso": {0, 0},
	}
	if len(got) != len(want) {
		t.Fatalf("パッケージ数: got %d, want %d", len(got), len(want))
	}
	for dir, w := range want {
		m, ok := got[dir]
		if !ok {
			t.Errorf("%s がない", dir)
			continue
		}
		if m.AfferentCoupling != w.ca || m.EfferentCoupling != w.ce {
			t.Errorf("%s: Ca=%d Ce=%d, want Ca=%d Ce=%d", dir, m.AfferentCoupling, m.EfferentCoupling, w.ca, w.ce)
		}
	}
	if got["/m/b"].Name != "b" || got["/m/b"].Path != "m/b" {
		t.Errorf("Name/Path が引き継がれていない: %+v", got["/m/b"])
	}
}

func Test_enrichMainSequence(t *testing.T) {
	t.Run("相対パスで渡したパッケージにも go list の結果を突き合わせる", func(t *testing.T) {
		root := writeFixtureModule(t)
		t.Chdir(root)
		packages := []PackageResult{
			{Path: "b", ExportedTypes: 0},
			{Path: filepath.Join(".", "c"), ExportedTypes: 1, InterfaceTypes: 1},
		}

		if err := enrichMainSequence(".", packages); err != nil {
			t.Fatalf("enrichMainSequence 失敗: %v", err)
		}

		if packages[0].MainSequence == nil || packages[0].MainSequence.AfferentCoupling != 2 {
			t.Errorf("b: %+v, want Ca=2", packages[0].MainSequence)
		}
		if packages[1].MainSequence == nil || packages[1].MainSequence.Abstractness != 1 {
			t.Errorf("c: %+v, want A=1", packages[1].MainSequence)
		}
	})
}

func Test_listModulePackages(t *testing.T) {
	t.Run("fixture モジュールの import 辺を返す", func(t *testing.T) {
		root := writeFixtureModule(t)

		pkgs, err := listModulePackages(root)
		if err != nil {
			t.Fatalf("listModulePackages 失敗: %v", err)
		}

		byPath := make(map[string]listedPackage, len(pkgs))
		for _, p := range pkgs {
			byPath[p.ImportPath] = p
		}
		if len(byPath) != 6 {
			t.Fatalf("パッケージ数: got %d, want 6 (%v)", len(byPath), pkgs)
		}
		a := byPath["example.com/fx/a"]
		if a.Name != "a" || len(a.Imports) != 2 {
			t.Errorf("a: %+v", a)
		}
		if filepath.Base(a.Dir) != "a" {
			t.Errorf("a.Dir: %q", a.Dir)
		}
		if len(byPath["example.com/fx/iso"].Imports) != 0 {
			t.Errorf("iso は import を持たないはず: %+v", byPath["example.com/fx/iso"])
		}
	})

	t.Run("go.mod が無いディレクトリ → エラー", func(t *testing.T) {
		dir := t.TempDir()
		writeGoFile(t, dir, "x.go", "package x\n")

		_, err := listModulePackages(dir)
		if err == nil {
			t.Fatal("go.mod が無ければエラーを返すべき")
		}
	})
}
