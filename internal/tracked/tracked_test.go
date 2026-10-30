package tracked

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeModule は root に go.mod と files を書く。
func writeModule(t *testing.T, root string, files map[string]string) {
	t.Helper()
	files["go.mod"] = "module example.com/fx\n\ngo 1.26\n"
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func initGitRepo(t *testing.T, dir, gitignore string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init 失敗: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(gitignore), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fixture は追跡するパッケージ a と root、ignore されたディレクトリ tmp/x、
// a の中で ignore されたファイル extra.go と extra_test.go を持つモジュール。
func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeModule(t, root, map[string]string{
		"root.go":             "package fx\n",
		"a/a.go":              "package a\n\nfunc A() int { return Extra() }\n",
		"a/a_test.go":         "package a\n",
		"a/extra.go":          "package a\n\nfunc Extra() int { return 1 }\n",
		"a/extra_test.go":     "package a\n",
		"a/a_ext_test.go":     "package a_test\n",
		"a/sub/sub.go":        "package sub\n",
		"a/sub/sub_test.go":   "package sub\n",
		"tmp/x/x.go":          "package x\n",
		"tmp/x/x_test.go":     "package x\n",
		"onlyignored/o.go":    "package onlyignored\n",
		"onlytests/t_test.go": "package onlytests\n",
	})
	initGitRepo(t, root, "/tmp/\n/a/extra.go\n/a/extra_test.go\n/onlyignored/o.go\n/onlytests/\n")
	return root
}

func importPaths(s Selection) []string {
	var paths []string
	for _, p := range s.Packages {
		paths = append(paths, p.ImportPath)
	}
	return paths
}

func TestList(t *testing.T) {
	t.Run("ignore されたファイルしか持たないパッケージを除き、残りのファイルから ignore されたものを除く", func(t *testing.T) {
		root := fixture(t)

		s, err := List(root, nil)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if s.NotRepository {
			t.Error("git リポジトリなので NotRepository は false")
		}
		want := []string{"example.com/fx", "example.com/fx/a", "example.com/fx/a/sub"}
		if got := importPaths(s); !slices.Equal(got, want) {
			t.Fatalf("packages = %v, want %v", got, want)
		}
		a := s.Packages[1]
		if !slices.Equal(a.GoFiles, []string{"a.go"}) {
			t.Errorf("a.GoFiles = %v, want [a.go]", a.GoFiles)
		}
		if !slices.Equal(a.TestGoFiles, []string{"a_test.go"}) {
			t.Errorf("a.TestGoFiles = %v, want [a_test.go]", a.TestGoFiles)
		}
		if !slices.Equal(a.XTestGoFiles, []string{"a_ext_test.go"}) {
			t.Errorf("a.XTestGoFiles = %v, want [a_ext_test.go]", a.XTestGoFiles)
		}
		if a.Name != "a" || filepath.Base(a.Dir) != "a" {
			t.Errorf("a = %+v", a)
		}
	})

	t.Run("Ignored はファイル単体と ignore されたディレクトリの下のファイルを返す", func(t *testing.T) {
		root := fixture(t)
		s, err := List(root, nil)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		for name, want := range map[string]bool{
			"a/extra.go":      true,
			"a/extra_test.go": true,
			"tmp/x/x.go":      true,
			"tmp/x/x_test.go": true,
			"a/a.go":          false,
			"a/sub/sub.go":    false,
			"root.go":         false,
		} {
			if got := s.Ignored(filepath.Join(root, name)); got != want {
				t.Errorf("Ignored(%s) = %v, want %v", name, got, want)
			}
		}
	})

	t.Run("Ignored は symlink を挟んだパスでも同じ答えを返す", func(t *testing.T) {
		root := fixture(t)
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(root, link); err != nil {
			t.Fatal(err)
		}
		s, err := List(link, nil)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if !s.Ignored(filepath.Join(root, "a", "extra.go")) || !s.Ignored(filepath.Join(link, "a", "extra.go")) {
			t.Error("実体のパスでも symlink 経由のパスでも ignore と判定するはず")
		}
	})

	t.Run("Patterns はモジュールルートからの相対パスで、ルートは .", func(t *testing.T) {
		root := fixture(t)
		s, err := List(root, nil)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		want := []string{".", "./a", "./a/sub"}
		if got := s.Patterns(); !slices.Equal(got, want) {
			t.Errorf("Patterns = %v, want %v", got, want)
		}
	})

	t.Run("git リポジトリでなければ何も除かず NotRepository", func(t *testing.T) {
		root := t.TempDir()
		writeModule(t, root, map[string]string{
			"a/a.go":     "package a\n",
			"tmp/x/x.go": "package x\n",
		})
		if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/tmp/\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		s, err := List(root, nil)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if !s.NotRepository {
			t.Error("NotRepository は true")
		}
		if got := importPaths(s); !slices.Equal(got, []string{"example.com/fx/a", "example.com/fx/tmp/x"}) {
			t.Errorf("packages = %v", got)
		}
	})

	t.Run("git が PATH に無ければエラー (リポジトリでないのとは区別する)", func(t *testing.T) {
		root := t.TempDir()
		writeModule(t, root, map[string]string{"a/a.go": "package a\n"})
		goBin, err := exec.LookPath("go")
		if err != nil {
			t.Fatal(err)
		}
		bin := t.TempDir()
		if err := os.Symlink(goBin, filepath.Join(bin, "go")); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin)

		_, err = List(root, nil)
		if err == nil || !strings.Contains(err.Error(), "git が PATH に見つかりません") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("go.mod が無ければエラー", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "x.go"), []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := List(root, nil); err == nil {
			t.Error("go.mod が無ければエラー")
		}
	})

	t.Run("env を go list に渡す", func(t *testing.T) {
		root := fixture(t)
		_, err := List(root, []string{"GOFLAGS=-mod=bogus"})
		if err == nil || !strings.Contains(err.Error(), "bogus") {
			t.Errorf("env が go list に届いていない: %v", err)
		}
	})
}
