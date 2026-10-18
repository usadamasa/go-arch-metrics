package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// strictTestDir は --strict フラグのテスト用に Go ファイルを含む一時ディレクトリを作成する｡
// withWarnings が true の場合、public ratio が高い警告を意図的に発生させる｡
func strictTestDir(t *testing.T, withWarnings bool) string {
	t.Helper()
	dir := t.TempDir()

	var content string
	if withWarnings {
		// public ratio = 1.0 (> デフォルト閾値 0.6) かつシンボル数 10 (>= defaultMinSymbols) → 警告が発生する
		content = `package highpublic

func FuncA() {}
func FuncB() {}
func FuncC() {}
func FuncD() {}
func FuncE() {}
func FuncF() {}
func FuncG() {}
func FuncH() {}

type TypeA struct{}
type TypeB struct{}
`
	} else {
		// public ratio = 0.0 (< デフォルト閾値 0.6) → 警告なし
		content = `package lowpublic

// internalA は非公開関数 A｡
func internalA() {}

// internalB は非公開関数 B｡
func internalB() {}

// internalC は非公開関数 C｡
func internalC() {}

// internalType は非公開型｡
type internalType struct{}
`
	}

	err := os.WriteFile(filepath.Join(dir, "pkg.go"), []byte(content), 0644)
	if err != nil {
		t.Fatalf("テスト用ファイルの作成に失敗: %v", err)
	}
	// Ca/Ce は go list で取るので、go.mod が無いと解析自体が止まる
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module strictfixture\n"), 0644); err != nil {
		t.Fatalf("go.mod 作成失敗: %v", err)
	}
	return dir
}

func TestInferModuleRoot(t *testing.T) {
	t.Run("go.mod ありディレクトリ → そのディレクトリ", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0644); err != nil {
			t.Fatalf("go.mod 作成失敗: %v", err)
		}

		got := inferModuleRoot([]string{dir})
		if got != dir {
			t.Errorf("inferModuleRoot = %q, want %q", got, dir)
		}
	})

	t.Run("サブディレクトリ → 親の go.mod ディレクトリ", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0644); err != nil {
			t.Fatalf("go.mod 作成失敗: %v", err)
		}
		subDir := filepath.Join(dir, "sub", "pkg")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("サブディレクトリ作成失敗: %v", err)
		}

		got := inferModuleRoot([]string{subDir})
		if got != dir {
			t.Errorf("inferModuleRoot = %q, want %q", got, dir)
		}
	})

	t.Run("go.mod なし → abs(args[0])", func(t *testing.T) {
		dir := t.TempDir()
		got := inferModuleRoot([]string{dir})
		abs, _ := filepath.Abs(dir)
		if got != abs {
			t.Errorf("inferModuleRoot = %q, want %q", got, abs)
		}
	})
}

func TestMainSequenceIntegration(t *testing.T) {
	t.Run("fixture モジュールの Ca/Ce/A/I/D と Zone を JSON に出す", func(t *testing.T) {
		root := writeFixtureModule(t)

		cmd := exec.Command("go", "run", ".", root)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("実行失敗: %v", err)
		}

		var report Report
		if err := json.Unmarshal(out, &report); err != nil {
			t.Fatalf("JSON パース失敗: %v", err)
		}

		byBase := make(map[string]PackageResult, len(report.Packages))
		for _, p := range report.Packages {
			byBase[filepath.Base(p.Path)] = p
		}

		b := byBase["b"].MainSequence
		if b == nil {
			t.Fatal("b の main_sequence が nil")
		}
		// b は a と e から import され、c を import する
		if b.AfferentCoupling != 2 || b.EfferentCoupling != 1 {
			t.Errorf("b: Ca=%d Ce=%d, want Ca=2 Ce=1", b.AfferentCoupling, b.EfferentCoupling)
		}
		if b.Zone != ZoneOfPain {
			t.Errorf("b.Zone = %q, want %q (A=0, I=1/3)", b.Zone, ZoneOfPain)
		}

		c := byBase["c"].MainSequence
		if c == nil {
			t.Fatal("c の main_sequence が nil")
		}
		// c は a, b, d から import され、interface 1 つが型宣言の全部
		if c.AfferentCoupling != 3 || c.EfferentCoupling != 0 || c.Abstractness != 1 {
			t.Errorf("c: Ca=%d Ce=%d A=%v, want Ca=3 Ce=0 A=1", c.AfferentCoupling, c.EfferentCoupling, c.Abstractness)
		}
		if !c.StructurallyConstrained {
			t.Error("c は Ce=0, Ca>0 なので structurally_constrained のはず")
		}

		iso := byBase["iso"].MainSequence
		if iso == nil || iso.Zone != ZoneExcluded {
			t.Errorf("iso は孤立なので excluded のはず: %+v", iso)
		}

		if report.Summary.MainSequence == nil {
			t.Fatal("summary.main_sequence が nil")
		}
		if report.Summary.MainSequence.StructurallyConstrainedCount != 1 {
			t.Errorf("structurally_constrained_count = %d, want 1 (c)", report.Summary.MainSequence.StructurallyConstrainedCount)
		}
	})

	t.Run("gitignore 済みのパッケージと入れ子のモジュールは報告に出ない", func(t *testing.T) {
		root := writeFixtureModule(t)
		initGitRepo(t, root, "tmp/\n")
		extra := map[string]string{
			// .gitignore に当たる (go list には含まれる)
			"tmp/x/x.go": "package x\n\nfunc X() {}\n",
			// 入れ子のモジュール (go list ./... に含まれない)
			"tmp/probe/go.mod":  "module example.com/probe\n\ngo 1.26\n",
			"tmp/probe/main.go": "package main\n\nfunc main() {}\n",
		}
		for name, content := range extra {
			path := filepath.Join(root, name)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatalf("ディレクトリ作成失敗: %v", err)
			}
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				t.Fatalf("ファイル作成失敗: %v", err)
			}
		}

		cmd := exec.Command("go", "run", ".", root)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("実行失敗: %v", err)
		}
		var report Report
		if err := json.Unmarshal(out, &report); err != nil {
			t.Fatalf("JSON パース失敗: %v", err)
		}

		if report.Summary.TotalPackages != 6 {
			t.Errorf("total_packages = %d, want 6 (a b c d e iso)", report.Summary.TotalPackages)
		}
		for _, p := range report.Packages {
			if strings.Contains(p.Path, "tmp") {
				t.Errorf("tmp 配下が報告に混ざっている: %s", p.Path)
			}
		}
	})

	t.Run("go.mod が無い → exit code 1", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "x.go"), []byte("package x\n"), 0644); err != nil {
			t.Fatalf("x.go 作成失敗: %v", err)
		}

		cmd := exec.Command("go", "run", ".", dir)
		err := cmd.Run()
		if err == nil {
			t.Fatal("go.mod が無ければ exit code 1 を返すべき")
		}
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("予期しないエラー: %v", err)
		}
		if exitErr.ExitCode() != 1 {
			t.Errorf("exit code: got %d, want 1", exitErr.ExitCode())
		}
	})
}

func TestStrictFlag(t *testing.T) {
	// サブプロセステストには cmd/ ディレクトリから go run を使用する｡
	// os.Exit を含む main() は直接呼び出せないため、サブプロセスで実行する｡

	tests := []struct {
		name         string
		withWarnings bool
		strictFlag   bool
		wantExitCode int
	}{
		{
			name:         "--strict あり、警告あり → exit code 1",
			withWarnings: true,
			strictFlag:   true,
			wantExitCode: 1,
		},
		{
			name:         "--strict あり、警告なし → exit code 0",
			withWarnings: false,
			strictFlag:   true,
			wantExitCode: 0,
		},
		{
			name:         "--strict なし、警告あり → exit code 0",
			withWarnings: true,
			strictFlag:   false,
			wantExitCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := strictTestDir(t, tt.withWarnings)

			args := []string{"run", "."}
			if tt.strictFlag {
				args = append(args, "--strict")
			}
			args = append(args, dir)

			cmd := exec.Command("go", args...)

			err := cmd.Run()

			var gotExitCode int
			if err != nil {
				exitErr, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("予期しないエラー: %v", err)
				}
				gotExitCode = exitErr.ExitCode()
			}

			if gotExitCode != tt.wantExitCode {
				t.Errorf("exit code: got %d, want %d", gotExitCode, tt.wantExitCode)
			}
		})
	}
}
