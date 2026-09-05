package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
	return dir
}

func TestFindGoPackageDirs(t *testing.T) {
	t.Run("Walk 中の権限エラーはスキップして続行する", func(t *testing.T) {
		dir := t.TempDir()

		// 正常なサブディレクトリ (Go ファイルあり)
		goodDir := filepath.Join(dir, "good")
		if err := os.MkdirAll(goodDir, 0755); err != nil {
			t.Fatalf("ディレクトリ作成に失敗: %v", err)
		}
		if err := os.WriteFile(filepath.Join(goodDir, "main.go"), []byte("package main\n"), 0644); err != nil {
			t.Fatalf("ファイル作成に失敗: %v", err)
		}

		// 権限エラーのサブディレクトリ
		badDir := filepath.Join(dir, "bad")
		if err := os.MkdirAll(badDir, 0755); err != nil {
			t.Fatalf("ディレクトリ作成に失敗: %v", err)
		}
		if err := os.Chmod(badDir, 0000); err != nil {
			t.Skipf("chmod 0000 に失敗 (root ユーザーの可能性): %v", err)
		}
		t.Cleanup(func() { os.Chmod(badDir, 0755) })

		dirs, err := findGoPackageDirs(dir)
		if err != nil {
			t.Fatalf("findGoPackageDirs がエラーを返した: %v", err)
		}

		if len(dirs) != 1 {
			t.Errorf("ディレクトリ数: got %d, want 1", len(dirs))
		}
	})

	t.Run("ReadDir の権限エラーはスキップして続行する", func(t *testing.T) {
		dir := t.TempDir()

		// 正常なサブディレクトリ (Go ファイルあり)
		goodDir := filepath.Join(dir, "good")
		if err := os.MkdirAll(goodDir, 0755); err != nil {
			t.Fatalf("ディレクトリ作成に失敗: %v", err)
		}
		if err := os.WriteFile(filepath.Join(goodDir, "main.go"), []byte("package main\n"), 0644); err != nil {
			t.Fatalf("ファイル作成に失敗: %v", err)
		}

		// Walk では入れるが ReadDir で失敗するディレクトリ (実行権限のみ)
		noReadDir := filepath.Join(dir, "noread")
		if err := os.MkdirAll(noReadDir, 0755); err != nil {
			t.Fatalf("ディレクトリ作成に失敗: %v", err)
		}
		if err := os.Chmod(noReadDir, 0111); err != nil {
			t.Skipf("chmod 0111 に失敗: %v", err)
		}
		t.Cleanup(func() { os.Chmod(noReadDir, 0755) })

		dirs, err := findGoPackageDirs(dir)
		if err != nil {
			t.Fatalf("findGoPackageDirs がエラーを返した: %v", err)
		}

		if len(dirs) != 1 {
			t.Errorf("ディレクトリ数: got %d, want 1", len(dirs))
		}
	})
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

func TestSPMJSONIntegration(t *testing.T) {
	t.Run("有効な spm-json → enriched 出力", func(t *testing.T) {
		// モジュールルートとなるディレクトリに go.mod を配置
		moduleDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte("module test-module\n"), 0644); err != nil {
			t.Fatalf("go.mod 作成失敗: %v", err)
		}
		// Go ソースをサブディレクトリに配置
		pkgDir := filepath.Join(moduleDir, "lowpublic")
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			t.Fatalf("ディレクトリ作成失敗: %v", err)
		}
		content := `package lowpublic

func internalA() {}
func internalB() {}
func internalC() {}
type internalType struct{}
`
		if err := os.WriteFile(filepath.Join(pkgDir, "pkg.go"), []byte(content), 0644); err != nil {
			t.Fatalf("pkg.go 作成失敗: %v", err)
		}

		spmFile := filepath.Join(t.TempDir(), "spm.json")
		spmJSON := `{"packages": [{"name": "lowpublic", "path": "test-module/lowpublic", "afferent_coupling": 0, "efferent_coupling": 0, "abstractness": 0.0, "instability": 0.0, "distance": 1.0}]}`
		if err := os.WriteFile(spmFile, []byte(spmJSON), 0644); err != nil {
			t.Fatalf("spm.json 作成失敗: %v", err)
		}

		cmd := exec.Command("go", "run", ".", "--spm-json", spmFile, moduleDir)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("実行失敗: %v", err)
		}

		var report Report
		if err := json.Unmarshal(out, &report); err != nil {
			t.Fatalf("JSON パース失敗: %v", err)
		}

		if report.Summary.SPM == nil {
			t.Fatal("SPM サマリが nil")
		}
	})

	t.Run("不正 JSON → exit code 1", func(t *testing.T) {
		goDir := strictTestDir(t, false)

		spmFile := filepath.Join(t.TempDir(), "bad.json")
		if err := os.WriteFile(spmFile, []byte("{invalid"), 0644); err != nil {
			t.Fatalf("bad.json 作成失敗: %v", err)
		}

		cmd := exec.Command("go", "run", ".", "--spm-json", spmFile, goDir)
		err := cmd.Run()
		if err == nil {
			t.Fatal("不正 JSON では exit code 1 を返すべき")
		}
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("予期しないエラー: %v", err)
		}
		if exitErr.ExitCode() != 1 {
			t.Errorf("exit code: got %d, want 1", exitErr.ExitCode())
		}
	})

	t.Run("存在しないパス → exit code 1", func(t *testing.T) {
		goDir := strictTestDir(t, false)

		cmd := exec.Command("go", "run", ".", "--spm-json", "/nonexistent/spm.json", goDir)
		err := cmd.Run()
		if err == nil {
			t.Fatal("存在しないパスでは exit code 1 を返すべき")
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
