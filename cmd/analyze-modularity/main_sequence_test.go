package main

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usadamasa/go-arch-metrics/internal/tracked"
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

// couplingOf は root のモジュールを読み、import path の末尾をキーにした Ca/Ce を返す｡
func couplingOf(t *testing.T, root string) map[string]PackageMetrics {
	t.Helper()
	sel, err := tracked.List(root, nil)
	if err != nil {
		t.Fatalf("tracked.List 失敗: %v", err)
	}
	loaded, err := loadTypedPackages(root)
	if err != nil {
		t.Fatalf("loadTypedPackages 失敗: %v", err)
	}
	byBase := make(map[string]PackageMetrics)
	for _, m := range computeCoupling(loaded, sel) {
		byBase[filepath.Base(m.Path)] = m
	}
	return byBase
}

func Test_computeCoupling(t *testing.T) {
	t.Run("Ce は自分の宣言、Ca は依存元のパッケージを数える", func(t *testing.T) {
		root := writeFixtureModule(t)

		got := couplingOf(t, root)

		// a は b と c を参照するが、参照する宣言は A の 1 つなので Ce=1
		want := map[string]struct{ ca, ce int }{
			"a":   {0, 1},
			"b":   {2, 1},
			"c":   {3, 0},
			"d":   {0, 1},
			"e":   {0, 1},
			"iso": {0, 0},
		}
		if len(got) != len(want) {
			t.Fatalf("パッケージ数: got %d, want %d", len(got), len(want))
		}
		for base, w := range want {
			m, ok := got[base]
			if !ok {
				t.Errorf("%s がない", base)
				continue
			}
			if m.AfferentCoupling != w.ca || m.EfferentCoupling != w.ce {
				t.Errorf("%s: Ca=%d Ce=%d, want Ca=%d Ce=%d", base, m.AfferentCoupling, m.EfferentCoupling, w.ca, w.ce)
			}
		}
		if got["b"].Name != "b" || got["b"].Path != "example.com/fx/b" {
			t.Errorf("Name/Path が引き継がれていない: %+v", got["b"])
		}
	})

	t.Run("Ce: 型とメソッドは 1 単位、var は spec ごと、標準ライブラリとテストファイルは数えない", func(t *testing.T) {
		root := writeModule(t, map[string]string{
			"go.mod": "module example.com/unit\n\ngo 1.26\n",
			"c/c.go": "package c\n\nfunc C() int { return 1 }\n\nfunc D() int { return 2 }\n",
			// S とメソッド 2 つで 1 単位、V と W で 2 単位、fmt だけを使う F は数えない
			"p/p.go": "package p\n\nimport (\n\t\"fmt\"\n\n\t\"example.com/unit/c\"\n)\n\n" +
				"type S struct{}\n\nfunc (S) M() int { return c.C() }\n\nfunc (*S) N() int { return c.D() }\n\n" +
				"var (\n\tV = c.C()\n\tW = c.D()\n)\n\nfunc F() { fmt.Println() }\n",
			"p/p_test.go": "package p\n\nimport (\n\t\"testing\"\n\n\t\"example.com/unit/c\"\n)\n\nfunc TestP(t *testing.T) { _ = c.C() }\n",
		})

		got := couplingOf(t, root)

		if ce := got["p"].EfferentCoupling; ce != 3 {
			t.Errorf("p.Ce = %d, want 3 (S, V, W)", ce)
		}
		// p の宣言 3 つが c を参照しても、c に依存するパッケージは p の 1 つ
		if ca := got["c"].AfferentCoupling; ca != 1 {
			t.Errorf("c.Ca = %d, want 1 (p)", ca)
		}
	})

	t.Run("gitignore 済みのパッケージは数えず、そこへの辺もそこからの辺も数えない", func(t *testing.T) {
		root := writeFixtureModule(t)
		initGitRepo(t, root, "c/\n")

		got := couplingOf(t, root)

		if _, ok := got["c"]; ok || len(got) != 5 {
			t.Errorf("c を除いた 5 件のはず: %v", got)
		}
		// b の Ce は c への参照が落ちて 0 になる
		if ce := got["b"].EfferentCoupling; ce != 0 {
			t.Errorf("b.Ce = %d, want 0 (ignore された c への辺は数えない)", ce)
		}
	})
}

func Test_enrichMainSequence(t *testing.T) {
	t.Run("go list のディレクトリで突き合わせ、cwd に依らない", func(t *testing.T) {
		root := writeFixtureModule(t)
		t.Chdir(root)
		packages := []PackageResult{
			{Path: "b", dir: filepath.Join(root, "b"), ExportedTypes: 0},
			{Path: "c", dir: filepath.Join(root, "c"), ExportedTypes: 1, InterfaceTypes: 1},
		}

		sel, err := tracked.List(".", nil)
		if err != nil {
			t.Fatalf("tracked.List 失敗: %v", err)
		}
		loaded, err := loadTypedPackages(".")
		if err != nil {
			t.Fatalf("loadTypedPackages 失敗: %v", err)
		}
		if err := enrichMainSequence(computeCoupling(loaded, sel), packages, commitHistory{unmeasuredReason: "test"}, defaultVolatileCommits, nil); err != nil {
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

// initGitRepo は dir を git リポジトリにし、.gitignore を置く｡
func initGitRepo(t *testing.T, dir, gitignore string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init 失敗: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		t.Fatalf(".gitignore 作成失敗: %v", err)
	}
}

func Test_selectPackages(t *testing.T) {
	root := writeFixtureModule(t)
	sel, err := tracked.List(root, nil)
	if err != nil {
		t.Fatalf("tracked.List 失敗: %v", err)
	}
	listed := sel.Packages

	wantAll := "a,b,c,d,e,iso"
	pathsOf := func(selected []selectedPackage) string {
		var paths []string
		for _, s := range selected {
			paths = append(paths, s.Path)
		}
		return strings.Join(paths, ",")
	}

	t.Run("絶対パスの引数でも Path はモジュールルートからの相対", func(t *testing.T) {
		selected, err := selectPackages(listed, root, []string{root})
		if err != nil {
			t.Fatalf("selectPackages 失敗: %v", err)
		}
		if got := pathsOf(selected); got != wantAll {
			t.Errorf("paths = %v, want %v", got, wantAll)
		}
		if len(selected[0].GoFiles) != 1 || selected[0].GoFiles[0] != "a.go" {
			t.Errorf("a.GoFiles = %v, want [a.go]", selected[0].GoFiles)
		}
	})

	t.Run("サブディレクトリを渡せばその下だけ", func(t *testing.T) {
		selected, err := selectPackages(listed, root, []string{filepath.Join(root, "b")})
		if err != nil {
			t.Fatalf("selectPackages 失敗: %v", err)
		}
		if got := pathsOf(selected); got != "b" {
			t.Errorf("paths = %v, want b だけ", got)
		}
	})

	t.Run("相対パスの引数でも絶対パスと同じ Path", func(t *testing.T) {
		t.Chdir(root)
		selected, err := selectPackages(listed, root, []string{"."})
		if err != nil {
			t.Fatalf("selectPackages 失敗: %v", err)
		}
		if got := pathsOf(selected); got != wantAll {
			t.Errorf("paths = %v, want %v", got, wantAll)
		}
	})

	t.Run("サブディレクトリから相対で渡してもモジュールルートからの相対", func(t *testing.T) {
		t.Chdir(filepath.Join(root, "b"))
		selected, err := selectPackages(listed, root, []string{"."})
		if err != nil {
			t.Fatalf("selectPackages 失敗: %v", err)
		}
		if got := pathsOf(selected); got != "b" {
			t.Errorf("paths = %v, want b", got)
		}
	})
}
