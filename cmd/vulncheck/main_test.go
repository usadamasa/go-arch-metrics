package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeModule は main.go 1 つのモジュールを作り、ルートを返す｡
func writeModule(t *testing.T, mainGo string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":  "module example.com/fixture\n\ngo 1.26\n",
		"main.go": mainGo,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatalf("%s 作成失敗: %v", name, err)
		}
	}
	return dir
}

// fixtureDB は testdata の脆弱性 DB を file:// URL で返す｡
// stdlib の fmt.Println に当たる GO-9999-0001 だけが入っている｡
func fixtureDB(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "vulndb"))
	if err != nil {
		t.Fatalf("testdata の絶対パス取得失敗: %v", err)
	}
	return "file://" + abs
}

// run は go run . に引数を渡し、exit code と stdout+stderr を返す｡
func run(t *testing.T, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("実行失敗: %v\n%s", err, out)
	}
	return exitErr.ExitCode(), string(out)
}

const affectedMain = `package main

import "fmt"

func main() { fmt.Println("hello") }
`

const cleanMain = `package main

func main() {}
`

func TestVulncheck(t *testing.T) {
	t.Run("脆弱な symbol を呼んでいれば exit 3", func(t *testing.T) {
		dir := writeModule(t, affectedMain)

		code, out := run(t, "-db", fixtureDB(t), "-C", dir, "./...")

		if code != 3 {
			t.Errorf("exit code = %d, want 3\n%s", code, out)
		}
		if !strings.Contains(out, "GO-9999-0001") {
			t.Errorf("GO-9999-0001 が出力に無い:\n%s", out)
		}
	})

	t.Run("脆弱な symbol を呼んでいなければ exit 0", func(t *testing.T) {
		dir := writeModule(t, cleanMain)

		code, out := run(t, "-db", fixtureDB(t), "-C", dir, "./...")

		if code != 0 {
			t.Errorf("exit code = %d, want 0\n%s", code, out)
		}
		if !strings.Contains(out, "No vulnerabilities found") {
			t.Errorf("No vulnerabilities found が出力に無い:\n%s", out)
		}
	})

	t.Run("DB に届かなければ 0 でも 3 でもない exit code (未測定)", func(t *testing.T) {
		dir := writeModule(t, cleanMain)
		missing := "file://" + filepath.Join(t.TempDir(), "nonexistent")

		code, out := run(t, "-db", missing, "-C", dir, "./...")

		if code == 0 || code == 3 {
			t.Errorf("exit code = %d, want neither 0 nor 3\n%s", code, out)
		}
	})

	t.Run("本物の DB (vuln.go.dev) に埋め込みの roots で届く", func(t *testing.T) {
		if testing.Short() {
			t.Skip("ネットワークが要る")
		}
		dir := writeModule(t, cleanMain)

		code, out := run(t, "-C", dir, "./...")

		if code != 0 {
			t.Errorf("exit code = %d, want 0\n%s", code, out)
		}
	})
}

