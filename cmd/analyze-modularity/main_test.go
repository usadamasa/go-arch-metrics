package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usadamasa/go-arch-metrics/internal/schema"
)

// strictTestDir は --strict フラグのテスト用に Go ファイルを含む一時ディレクトリを作成する｡
// withWarnings が true の場合、どこからも参照されない exported を持つ internal パッケージを作る｡
func strictTestDir(t *testing.T, withWarnings bool) string {
	t.Helper()
	dir := t.TempDir()

	var content string
	file := "pkg.go"
	if withWarnings {
		// exported 10 個がどこからも参照されない internal パッケージ → unreferenced_exported が actionable
		file = filepath.Join("internal", "highpublic", "pkg.go")
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

	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, file)), 0755); err != nil {
		t.Fatalf("ディレクトリ作成失敗: %v", err)
	}
	err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0644)
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

		byBase := make(map[string]schema.Package, len(report.Packages))
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

		if report.Summary.MainSequence.StructurallyConstrainedCount != 1 {
			t.Errorf("structurally_constrained_count = %d, want 1 (c)", report.Summary.MainSequence.StructurallyConstrainedCount)
		}
	})

	t.Run("gitignore 済みのパッケージと入れ子のモジュールは報告に出ない", func(t *testing.T) {
		root := writeFixtureModule(t)
		initGitRepo(t, root, "tmp/\n")
		extra := map[string]string{
			// .gitignore に当たる (go list には含まれる)。c を import するが、構成管理の外なので c の Ca には数えない
			"tmp/x/x.go": "package x\n\nimport \"example.com/fx/c\"\n\nfunc X() int { return c.C() }\n",
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
			if strings.HasPrefix(p.Path, "tmp/") {
				t.Errorf("tmp 配下が報告に混ざっている: %s", p.Path)
			}
			if filepath.Base(p.Path) == "c" && p.MainSequence.AfferentCoupling != 3 {
				t.Errorf("c.Ca = %d, want 3 (a, b, d。gitignore 済みの tmp/x は数えない)", p.MainSequence.AfferentCoupling)
			}
		}
	})

	t.Run("絶対パスで渡しても相対で渡しても同じ JSON を出す", func(t *testing.T) {
		root := writeFixtureModule(t)
		bin := filepath.Join(t.TempDir(), "analyze-modularity")
		if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
			t.Fatalf("build 失敗: %v\n%s", err, out)
		}
		run := func(cwd, arg string) []byte {
			cmd := exec.Command(bin, arg)
			cmd.Dir = cwd
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("実行失敗 (cwd=%s arg=%s): %v", cwd, arg, err)
			}
			return out
		}

		absolute := run(t.TempDir(), root)
		relative := run(root, ".")
		if string(absolute) != string(relative) {
			t.Errorf("引数の渡し方で出力が変わる\nabsolute:\n%s\nrelative:\n%s", absolute, relative)
		}

		var report Report
		if err := json.Unmarshal(absolute, &report); err != nil {
			t.Fatalf("JSON パース失敗: %v", err)
		}
		var paths []string
		for _, p := range report.Packages {
			paths = append(paths, p.Path)
		}
		if got := strings.Join(paths, ","); got != "a,b,c,d,e,iso" {
			t.Errorf("paths = %s, want モジュールルートからの相対 (a,b,c,d,e,iso)", got)
		}
		excluded := report.Summary.MainSequence.ExcludedPackages
		if excluded == nil {
			t.Fatal("excluded_packages が null")
		}
		if got := strings.Join(*excluded, ","); got != "iso" {
			t.Errorf("excluded_packages = %s, want iso", got)
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

// writeModule は files をモジュールとして書き、ルートを返す｡
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
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

// mainSequenceOf は root を解析し、末尾が base のパッケージの main_sequence を返す｡
func mainSequenceOf(t *testing.T, root, base string) *MainSequenceData {
	t.Helper()
	out, err := exec.Command("go", "run", ".", root).Output()
	if err != nil {
		t.Fatalf("実行失敗: %v", err)
	}
	var report Report
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("JSON パース失敗: %v", err)
	}
	for _, p := range report.Packages {
		if filepath.Base(p.Path) == base {
			return p.MainSequence
		}
	}
	t.Fatalf("%s が報告に無い", base)
	return nil
}

func TestMainSequence_依存先を統合しても呼び手のIとDは変わらない(t *testing.T) {
	// agents-daemon#280: parking が使う gitrepo を domain へ統合すると、parking のコードは同じなのに I が下がった
	split := writeModule(t, map[string]string{
		"go.mod": "module example.com/merge\n\ngo 1.26\n",
		"x/x.go": "package x\n\nfunc X() int { return 1 }\n",
		"y/y.go": "package y\n\nfunc Y() int { return 2 }\n",
		"p/p.go": "package p\n\nimport (\n\t\"example.com/merge/x\"\n\t\"example.com/merge/y\"\n)\n\n" +
			"func P() int { return x.X() + y.Y() }\n\nfunc Local() int { return 3 }\n",
		"q/q.go": "package q\n\nimport \"example.com/merge/p\"\n\nfunc Q() int { return p.P() }\n",
	})
	merged := writeModule(t, map[string]string{
		"go.mod":   "module example.com/merge\n\ngo 1.26\n",
		"xy/xy.go": "package xy\n\nfunc X() int { return 1 }\n\nfunc Y() int { return 2 }\n",
		"p/p.go": "package p\n\nimport \"example.com/merge/xy\"\n\n" +
			"func P() int { return xy.X() + xy.Y() }\n\nfunc Local() int { return 3 }\n",
		"q/q.go": "package q\n\nimport \"example.com/merge/p\"\n\nfunc Q() int { return p.P() }\n",
	})

	before := mainSequenceOf(t, split, "p")
	after := mainSequenceOf(t, merged, "p")

	if before.Instability != after.Instability || before.Distance != after.Distance {
		t.Errorf("統合で p が動いた: 分割 I=%.2f D=%.2f (Ca=%d Ce=%d), 統合 I=%.2f D=%.2f (Ca=%d Ce=%d)",
			before.Instability, before.Distance, before.AfferentCoupling, before.EfferentCoupling,
			after.Instability, after.Distance, after.AfferentCoupling, after.EfferentCoupling)
	}
}

func TestMainSequence_既存の呼び手に関数を足しても呼ばれる側のIとDは変わらない(t *testing.T) {
	// 呼び手のパッケージの中で p を使う関数が増えるのは、p と呼び手の間の新しい依存ではない
	p := "package p\n\nimport \"example.com/grow/x\"\n\nfunc P() int { return x.X() }\n"
	before := writeModule(t, map[string]string{
		"go.mod": "module example.com/grow\n\ngo 1.26\n",
		"x/x.go": "package x\n\nfunc X() int { return 1 }\n",
		"p/p.go": p,
		"q/q.go": "package q\n\nimport \"example.com/grow/p\"\n\nfunc Q() int { return p.P() }\n",
	})
	after := writeModule(t, map[string]string{
		"go.mod": "module example.com/grow\n\ngo 1.26\n",
		"x/x.go": "package x\n\nfunc X() int { return 1 }\n",
		"p/p.go": p,
		"q/q.go": "package q\n\nimport \"example.com/grow/p\"\n\nfunc Q() int { return p.P() }\n\nfunc Q2() int { return p.P() + 1 }\n",
	})

	b := mainSequenceOf(t, before, "p")
	a := mainSequenceOf(t, after, "p")

	if b.Instability != a.Instability || b.Distance != a.Distance {
		t.Errorf("q に関数を足すと p が動いた: 前 I=%.2f D=%.2f (Ca=%d Ce=%d), 後 I=%.2f D=%.2f (Ca=%d Ce=%d)",
			b.Instability, b.Distance, b.AfferentCoupling, b.EfferentCoupling,
			a.Instability, a.Distance, a.AfferentCoupling, a.EfferentCoupling)
	}
}

func Test_buildMainSequenceSummary_非揮発のパッケージはactionableから除く(t *testing.T) {
	packages := []PackageResult{
		{Path: "cold", MainSequence: &MainSequenceData{Zone: ZoneOfPain, Distance: 0.75, Volatility: VolatilityNonVolatile}},
		{Path: "hot", MainSequence: &MainSequenceData{Zone: ZoneOfPain, Distance: 0.86, Volatility: VolatilityVolatile}},
		{Path: "unknown", MainSequence: &MainSequenceData{Zone: ZoneOfPain, Distance: 0.6, Volatility: VolatilityUnmeasured}},
		{Path: "abstract", MainSequence: &MainSequenceData{Zone: ZoneOfUselessness, Distance: 0.7, Volatility: VolatilityNonVolatile}},
	}

	summary := buildMainSequenceSummary(packages, commitHistory{}, defaultVolatilityWindow, defaultVolatileCommits)

	if summary.NonVolatileZoneOfPainCount != 1 {
		t.Errorf("non_volatile_zone_of_pain_count = %d, want 1 (cold。zone_of_uselessness は数えない)", summary.NonVolatileZoneOfPainCount)
	}
	if summary.ActionableCount != 3 {
		t.Errorf("actionable_count = %d, want 3 (hot, unknown, abstract)", summary.ActionableCount)
	}
	if summary.ActionableMaxDistance == nil || *summary.ActionableMaxDistance != 0.86 {
		t.Errorf("actionable_max_distance = %v, want 0.86", summary.ActionableMaxDistance)
	}
}

func Test_buildMainSequenceSummary_baselineで判定が変わったものだけを数える(t *testing.T) {
	base := func(d float64) *float64 { return &d }
	packages := []PackageResult{
		// baseline が無くても actionable でない: 数えない
		{Path: "fine", MainSequence: &MainSequenceData{Zone: ZoneMainSequence, Distance: 0.1, Volatility: VolatilityVolatile, BaselineDistance: base(0.1)}},
		{Path: "cold", MainSequence: &MainSequenceData{Zone: ZoneOfPain, Distance: 0.75, Volatility: VolatilityNonVolatile, BaselineDistance: base(0.75)}},
		// baseline があるから actionable でない: 数える
		{Path: "hot", MainSequence: &MainSequenceData{Zone: ZoneOfPain, Distance: 0.75, Volatility: VolatilityVolatile, BaselineDistance: base(0.80)}},
	}

	summary := buildMainSequenceSummary(packages, commitHistory{}, defaultVolatilityWindow, defaultVolatileCommits)

	if summary.BaselineNotWorsenedCount != 1 {
		t.Errorf("baseline_not_worsened_count = %d, want 1 (hot)", summary.BaselineNotWorsenedCount)
	}
	if summary.NonVolatileZoneOfPainCount != 1 {
		t.Errorf("non_volatile_zone_of_pain_count = %d, want 1 (cold)", summary.NonVolatileZoneOfPainCount)
	}
	if summary.ActionableCount != 0 {
		t.Errorf("actionable_count = %d, want 0", summary.ActionableCount)
	}
}

// runStrict は analyze-modularity --strict を dir に対して走らせ、exit code と stderr を返す｡
func runStrict(t *testing.T, dir string) (int, string) {
	t.Helper()
	cmd := exec.Command("go", "run", ".", "--strict", dir)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return 0, stderr.String()
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("予期しないエラー: %v", err)
	}
	return exitErr.ExitCode(), stderr.String()
}

func TestStrictFlag_揮発性(t *testing.T) {
	// writeFixtureModule で直せる警告を出すのは b だけ (構造制約でない zone_of_pain、D≈0.67)
	t.Run("b が非揮発なら exit 0", func(t *testing.T) {
		root := committedFixtureModule(t)

		code, stderr := runStrict(t, root)
		if code != 0 {
			t.Errorf("exit code = %d, want 0\n%s", code, stderr)
		}
	})

	t.Run("b が揮発なら exit 1", func(t *testing.T) {
		root := committedFixtureModule(t)
		touchCommits(t, root, "b/b.go", defaultVolatileCommits)

		code, stderr := runStrict(t, root)
		if code != 1 {
			t.Errorf("exit code = %d, want 1\n%s", code, stderr)
		}
	})

	t.Run("窓やしきい値が 1 未満なら測る前に止める", func(t *testing.T) {
		// 窓 0 だとすべてのパッケージが 0 commit になり、Zone of Pain がすべて非揮発に倒れて gate が緩む
		root := committedFixtureModule(t)
		for _, flagArg := range []string{"--volatility-window=0", "--volatile-commits=0"} {
			cmd := exec.Command("go", "run", ".", "--strict", flagArg, root)
			var stderr strings.Builder
			cmd.Stderr = &stderr
			err := cmd.Run()
			exitErr, ok := err.(*exec.ExitError)
			if !ok || exitErr.ExitCode() != 1 {
				t.Errorf("%s: err = %v, want exit 1", flagArg, err)
			}
			if !strings.Contains(stderr.String(), "1 以上") {
				t.Errorf("%s: stderr に理由が無い:\n%s", flagArg, stderr.String())
			}
		}
	})

	t.Run("shallow clone では揮発性を測らず、b を揮発として扱って exit 1", func(t *testing.T) {
		root := committedFixtureModule(t)
		clone := filepath.Join(t.TempDir(), "clone")
		runGit(t, root, "clone", "-q", "--depth", "1", "file://"+root, clone)

		code, stderr := runStrict(t, clone)
		if code != 1 {
			t.Errorf("exit code = %d, want 1\n%s", code, stderr)
		}
		if !strings.Contains(stderr, "shallow") || !strings.Contains(stderr, "fetch-depth") {
			t.Errorf("stderr に shallow と fetch-depth の案内が無い:\n%s", stderr)
		}
	})
}

// addDependent は root に b を import するパッケージ name を足す｡b の Ca が 1 増え、D が大きくなる｡
func addDependent(t *testing.T, root, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, name), 0755); err != nil {
		t.Fatalf("ディレクトリ作成失敗: %v", err)
	}
	writeGoFile(t, filepath.Join(root, name), name+".go",
		"package "+name+"\n\nimport \"example.com/fx/b\"\n\nfunc F() int { return b.B() }\n")
}

// writeBaseline は root の今の測定結果をファイルに書き、パスを返す｡CI が base の commit で測る代わり｡
func writeBaseline(t *testing.T, root string) string {
	t.Helper()
	out, err := exec.Command("go", "run", ".", root).Output()
	if err != nil {
		t.Fatalf("baseline の測定に失敗: %v", err)
	}
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, out, 0644); err != nil {
		t.Fatalf("baseline の書き込みに失敗: %v", err)
	}
	return path
}

// runStrictWithBaseline は --strict --baseline で root を測り、exit code と stderr を返す｡
func runStrictWithBaseline(t *testing.T, root, baseline string) (int, string) {
	t.Helper()
	cmd := exec.Command("go", "run", ".", "--strict", "--baseline", baseline, root)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return 0, stderr.String()
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("予期しないエラー: %v", err)
	}
	return exitErr.ExitCode(), stderr.String()
}

func TestStrictFlag_baseline(t *testing.T) {
	// agents-daemon#217: 窓の中で 4 commit のパッケージは、何を変えても 5 commit 目で落ちていた
	t.Run("D を悪くせずに 5 commit 目で揮発になっても exit 0", func(t *testing.T) {
		root := committedFixtureModule(t)
		touchCommits(t, root, "b/b.go", defaultVolatileCommits-1)
		baseline := writeBaseline(t, root)
		touchCommits(t, root, "b/b.go", 1)

		if code, stderr := runStrict(t, root); code != 1 {
			t.Fatalf("前提: baseline なしでは exit 1 のはず (got %d)\n%s", code, stderr)
		}
		if code, stderr := runStrictWithBaseline(t, root, baseline); code != 0 {
			t.Errorf("exit code = %d, want 0\n%s", code, stderr)
		}
	})

	t.Run("base で揮発のパッケージの D を小さくする変更は exit 0", func(t *testing.T) {
		root := committedFixtureModule(t)
		addDependent(t, root, "f")
		addDependent(t, root, "g")
		commitAll(t, root, "add dependents")
		touchCommits(t, root, "b/b.go", defaultVolatileCommits)
		baseline := writeBaseline(t, root)
		// Ca 4 → 3 で D 0.80 → 0.75。まだ Zone of Pain で揮発
		if err := os.RemoveAll(filepath.Join(root, "g")); err != nil {
			t.Fatalf("g の削除に失敗: %v", err)
		}
		commitAll(t, root, "drop g")

		if code, stderr := runStrictWithBaseline(t, root, baseline); code != 0 {
			t.Errorf("exit code = %d, want 0\n%s", code, stderr)
		}
	})

	t.Run("揮発のパッケージの D を悪くする変更は exit 1", func(t *testing.T) {
		root := committedFixtureModule(t)
		touchCommits(t, root, "b/b.go", defaultVolatileCommits)
		baseline := writeBaseline(t, root)
		addDependent(t, root, "f")
		commitAll(t, root, "add f")

		if code, stderr := runStrictWithBaseline(t, root, baseline); code != 1 {
			t.Errorf("exit code = %d, want 1\n%s", code, stderr)
		}
	})

	t.Run("どのパッケージとも突き合わない baseline は stderr で知らせる", func(t *testing.T) {
		root := committedFixtureModule(t)
		// 別のモジュールを測ったレポート。x は c を import して Zone of Pain に入る
		other := writeModule(t, map[string]string{
			"go.mod": `module example.com/other

go 1.26
`,
			"c/c.go": `package c

func C() int { return 1 }
`,
			"x/x.go": `package x

import "example.com/other/c"

func X() int { return c.C() }
`,
		})
		baseline := writeBaseline(t, other)

		_, stderr := runStrictWithBaseline(t, root, baseline)
		if !strings.Contains(stderr, "突き合いません") {
			t.Errorf("stderr に注意が無い:\n%s", stderr)
		}
	})

	t.Run("import_path の無い baseline (別の版の出力) は測る前に止める", func(t *testing.T) {
		root := committedFixtureModule(t)
		baseline := writeBaseline(t, root)
		// 今のレポートから import_path だけを落とし、それを持たなかった版の出力を作る
		var report map[string]any
		data, err := os.ReadFile(baseline)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &report); err != nil {
			t.Fatal(err)
		}
		for _, p := range report["packages"].([]any) {
			delete(p.(map[string]any), "import_path")
		}
		if data, err = json.Marshal(report); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(baseline, data, 0644); err != nil {
			t.Fatalf("baseline の書き込みに失敗: %v", err)
		}

		code, stderr := runStrictWithBaseline(t, root, baseline)
		// テスト名が一時ディレクトリの名前に入るので、パスを除いてから見る
		if code != 1 || !strings.Contains(strings.ReplaceAll(stderr, baseline, ""), "import_path") {
			t.Errorf("exit code = %d, want 1 と import_path の案内\n%s", code, stderr)
		}
	})
}

// edgeFixtureModule は agents-daemon#184 の internal/edge を写したモジュールを作る｡
// edge の exported 10 個はすべて app から参照され、unexported は unexported 個だけ持つ｡
func edgeFixtureModule(t *testing.T, unexported int) string {
	t.Helper()
	var edge strings.Builder
	edge.WriteString("package edge\n\n")
	for _, name := range []string{"A", "B", "C", "D", "E", "F"} {
		fmt.Fprintf(&edge, "func %s() int { return 1 }\n", name)
	}
	for _, name := range []string{"G", "H", "I", "J"} {
		fmt.Fprintf(&edge, "type %s struct{}\n", name)
	}
	for i := range unexported {
		fmt.Fprintf(&edge, "func helper%d() {}\n", i)
	}

	root := t.TempDir()
	files := map[string]string{
		"go.mod":                "module example.com/daemon\n\ngo 1.26\n",
		"internal/edge/edge.go": edge.String(),
		"app/app.go": "package app\n\nimport \"example.com/daemon/internal/edge\"\n\n" +
			"func Run() int {\n\t_ = edge.G{}\n\t_ = edge.H{}\n\t_ = edge.I{}\n\t_ = edge.J{}\n" +
			"\treturn edge.A() + edge.B() + edge.C() + edge.D() + edge.E() + edge.F()\n}\n",
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

func TestStrictFlag_使われているexportedだけのパッケージ(t *testing.T) {
	// exported がすべて外から使われていれば、unexported の重複を消しても直す対象にならない
	for _, unexported := range []int{7, 6} {
		t.Run(fmt.Sprintf("unexported %d 個", unexported), func(t *testing.T) {
			root := edgeFixtureModule(t, unexported)

			cmd := exec.Command("go", "run", ".", "--strict", root)
			var stderr strings.Builder
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if err != nil {
				t.Errorf("--strict が落ちた: %v\n%s\n%s", err, out, stderr.String())
			}

			var report Report
			if err := json.Unmarshal(out, &report); err != nil {
				t.Fatalf("JSON パース失敗: %v", err)
			}
			for _, w := range report.Warnings {
				if w.Actionable {
					t.Errorf("actionable な警告が出た: %+v", w)
				}
			}
		})
	}
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

	t.Run("未参照の exported を数えて JSON に出し、actionable は unreferenced_exported だけ", func(t *testing.T) {
		dir := strictTestDir(t, true)

		out, err := exec.Command("go", "run", ".", dir).Output()
		if err != nil {
			t.Fatalf("実行失敗: %v", err)
		}
		var report Report
		if err := json.Unmarshal(out, &report); err != nil {
			t.Fatalf("JSON パース失敗: %v", err)
		}
		if len(report.Packages) != 1 {
			t.Fatalf("packages = %d, want 1", len(report.Packages))
		}
		p := report.Packages[0]
		if p.UnreferencedExported == nil || *p.UnreferencedExported != 10 || len(p.UnreferencedExportedSymbols) != 10 {
			t.Errorf("unreferenced_exported = %v %v, want 10 個", p.UnreferencedExported, p.UnreferencedExportedSymbols)
		}
		got := map[string]bool{}
		for _, w := range report.Warnings {
			got[w.Type] = w.Actionable
		}
		if actionable, ok := got[WarningTypeUnreferencedExported]; !ok || !actionable {
			t.Errorf("unreferenced_exported が actionable で出ていない: %+v", report.Warnings)
		}
		if actionable, ok := got[WarningTypeHighPublicRatio]; !ok || actionable {
			t.Errorf("high_public_ratio は actionable=false で残るはず: %+v", report.Warnings)
		}
	})

	t.Run("--strict あり、構造制約の警告だけ → exit code 0", func(t *testing.T) {
		// a → c の 2 パッケージ。c は Ce=0, Ca=1, A=0 の葉で zone_of_pain と structurally_constrained を出す。
		// a は I=1, A=0 で D=0 なので警告を出さない
		dir := t.TempDir()
		files := map[string]string{
			"go.mod": "module strictleaf\n",
			"a/a.go": "package a\n\nimport \"strictleaf/c\"\n\nfunc A() int { return c.C() }\n",
			"c/c.go": "package c\n\nfunc C() int { return 1 }\n",
		}
		for name, content := range files {
			path := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatalf("ディレクトリ作成失敗: %v", err)
			}
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				t.Fatalf("ファイル作成失敗: %v", err)
			}
		}

		cmd := exec.Command("go", "run", ".", "--strict", dir)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("構造制約の警告だけなら --strict でも exit 0 のはず: %v\n%s", err, out)
		}

		var report Report
		if err := json.Unmarshal(out, &report); err != nil {
			t.Fatalf("JSON パース失敗: %v", err)
		}
		if report.Summary.TotalWarnings != 2 {
			t.Errorf("total_warnings = %d, want 2 (警告は JSON に残す)", report.Summary.TotalWarnings)
		}
		if report.Summary.ActionableWarnings != 0 {
			t.Errorf("actionable_warnings = %d, want 0", report.Summary.ActionableWarnings)
		}
	})
}
