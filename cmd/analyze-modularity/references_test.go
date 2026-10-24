package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
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
`,
		"internal/lib/lib_internal_test.go": "package lib\n\nimport \"testing\"\n\nfunc TestOwn(t *testing.T) { OnlyOwnTests() }\n",
		"internal/lib/lib_test.go":          "package lib_test\n\nimport (\n\t\"testing\"\n\n\t\"example.com/ref/internal/lib\"\n)\n\nfunc TestX(t *testing.T) { lib.OnlyTests() }\n",
		"internal/dotted/dotted.go":         "package dotted\n\nfunc DotFunc() {}\n\nfunc DotUnused() {}\n",
		"app/app.go": `package app

import (
	"example.com/ref/internal/lib"
	. "example.com/ref/internal/dotted"
)

// visitor は lib.Widget.Visit と名前も型も同じメソッドを持つ
type visitor interface{ Visit(n int) bool }

// visitor2 は名前だけ同じで型が違う
type visitor2 interface{ Visit2(n int) int }

func Run() int {
	var w lib.Widget
	w.Called()
	w.Promoted()
	b := lib.Box[int]{}
	DotFunc()
	return lib.Used() + b.Get()
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
	listed, err := listModulePackages(root)
	if err != nil {
		t.Fatalf("listModulePackages: %v", err)
	}

	got, err := analyzeExportedReferences(root, listed)
	if err != nil {
		t.Fatalf("analyzeExportedReferences: %v", err)
	}

	t.Run("lib", func(t *testing.T) {
		lib, ok := got["example.com/ref/internal/lib"]
		if !ok {
			t.Fatalf("lib の結果が無い: %v", got)
		}
		// 自パッケージ (internal test を含む) からの参照は外からの参照に数えない。
		// Inner は埋め込みでメソッドが昇格するだけで、型名は外から参照されない
		wantUnreferenced := []string{"Inner", "OnlyOwnTests", "Unseen", "Unused", "Widget.Uncalled", "Widget.Visit2"}
		if !slices.Equal(lib.Unreferenced, wantUnreferenced) {
			t.Errorf("Unreferenced = %v, want %v", lib.Unreferenced, wantUnreferenced)
		}
		// 外部テストパッケージと他パッケージのテストからだけ参照されるもの
		wantTestOnly := []string{"OnlyOtherTests", "OnlyTests"}
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
