package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/usadamasa/go-arch-metrics/internal/tracked"
)

// writeReferenceFixtureModule は exported の参照のされ方を一通り持つモジュールを作り、ルートを返す｡
func writeReferenceFixtureModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/ref\n\ngo 1.26\n",
		"internal/lib/lib.go": `package lib

import "fmt"

func Used() int        { return 1 }
func Unused()          {}
func OnlyTests()       {}
func OnlyOtherTests()  {}
func OnlyOwnTests()    {}
func ownUse()          { Unused() }

type Widget struct{ Inner }

func (Widget) Called()             {}
func (Widget) Uncalled()           {}
func (Widget) String() string      { return fmt.Sprint(1) }
func (Widget) Visit(n int) bool    { return n > 0 }
func (Widget) Visit2(s string) int { return len(s) }

type Inner struct{}

func (Inner) Promoted() {}

type Box[T any] struct{ v T }

func (b Box[T]) Get() T { return b.v }

type Unseen struct{}

// Made は型名を外から書かれないが、外から呼ばれる Make の戻り値として外へ渡る
type Made struct {
	Base
	Part  Piece
	inner Hidden
}

type Base struct{}
type Piece struct{}
type Hidden struct{}
type Info struct{}
type Orphan struct{}

func Make() Made              { return Made{} }
func (Made) Detail() Info     { return Info{} }
func (Made) Unused2() Orphan  { return Orphan{} }

type Constraint interface{ ~int }

func Pick[T Constraint](v T) T { return v }

type Target struct{}
type Alias = Target

func GetAlias() Alias { return Alias{} }

type XOut struct{}

func ForXTest() XOut { return XOut{} }
`,
		"internal/lib/lib_internal_test.go": "package lib\n\nimport \"testing\"\n\nfunc TestOwn(t *testing.T) { OnlyOwnTests() }\n",
		"internal/lib/lib_test.go":          "package lib_test\n\nimport (\n\t\"testing\"\n\n\t\"example.com/ref/internal/lib\"\n)\n\nfunc TestX(t *testing.T) {\n\tlib.OnlyTests()\n\t_ = lib.ForXTest()\n}\n",
		"internal/dotted/dotted.go":         "package dotted\n\nfunc DotFunc() {}\n\nfunc DotUnused() {}\n",
		"app/app.go": `package app

import (
	"example.com/ref/internal/lib"
	. "example.com/ref/internal/dotted"
)

// visitor は lib.Widget.Visit と名前も型も同じメソッドを持つ。引数の名前は比べない
type visitor interface{ Visit(depth int) bool }

// visitor2 は名前だけ同じで型が違う
type visitor2 interface{ Visit2(n int) int }

func Run() int {
	var w lib.Widget
	w.Called()
	w.Promoted()
	b := lib.Box[int]{}
	DotFunc()
	m := lib.Make()
	_ = m.Detail()
	_ = lib.GetAlias()
	return lib.Used() + b.Get() + lib.Pick(1)
}
`,
		"other/other.go":      "package other\n",
		"other/other_test.go": "package other\n\nimport (\n\t\"testing\"\n\n\t\"example.com/ref/internal/lib\"\n)\n\nfunc TestO(t *testing.T) { lib.OnlyOtherTests() }\n",
		"cmd/tool/main.go":    "package main\n\nfunc Exported() {}\n\nfunc main() { Exported() }\n",
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

func Test_analyzeExportedReferences(t *testing.T) {
	root := writeReferenceFixtureModule(t)
	sel, err := tracked.List(root, nil)
	if err != nil {
		t.Fatalf("tracked.List: %v", err)
	}

	loaded, err := loadTypedPackages(root)
	if err != nil {
		t.Fatalf("loadTypedPackages: %v", err)
	}
	got := analyzeExportedReferences(loaded, sel)

	t.Run("lib", func(t *testing.T) {
		lib, ok := got["example.com/ref/internal/lib"]
		if !ok {
			t.Fatalf("lib の結果が無い: %v", got)
		}
		// 自パッケージ (internal test を含む) からの参照は外からの参照に数えない。
		// 参照される func・method のシグネチャと、参照される型の exported フィールドに出てくる型は参照済み
		// (Made・Base・Piece・Info・Constraint・Alias・Target・Inner)。unexported フィールドだけの Hidden と、
		// 参照されないメソッドのシグネチャだけの Orphan は未参照
		wantUnreferenced := []string{
			"Hidden", "Made.Unused2", "OnlyOwnTests", "Orphan", "Unseen", "Unused", "Widget.Uncalled", "Widget.Visit2",
		}
		if !slices.Equal(lib.Unreferenced, wantUnreferenced) {
			t.Errorf("Unreferenced = %v, want %v", lib.Unreferenced, wantUnreferenced)
		}
		// 自パッケージの外部テストからだけ参照されるもの。シグネチャを辿った XOut も同じ種類を引き継ぐ。
		// 他パッケージのテストからの参照 (OnlyOtherTests) は参照済み
		wantTestOnly := []string{"ForXTest", "OnlyTests", "XOut"}
		if !slices.Equal(lib.TestOnly, wantTestOnly) {
			t.Errorf("TestOnly = %v, want %v", lib.TestOnly, wantTestOnly)
		}
	})

	t.Run("dot import の参照を解く", func(t *testing.T) {
		dotted := got["example.com/ref/internal/dotted"]
		if !slices.Equal(dotted.Unreferenced, []string{"DotUnused"}) {
			t.Errorf("Unreferenced = %v, want [DotUnused]", dotted.Unreferenced)
		}
	})

	t.Run("internal でないパッケージも数える", func(t *testing.T) {
		app, ok := got["example.com/ref/app"]
		if !ok || !slices.Equal(app.Unreferenced, []string{"Run"}) {
			t.Errorf("app = %+v (ok=%v), want Unreferenced [Run]", app, ok)
		}
	})

	t.Run("main は判定しない", func(t *testing.T) {
		if _, ok := got["example.com/ref/cmd/tool"]; ok {
			t.Error("main パッケージの結果がある")
		}
	})
}
