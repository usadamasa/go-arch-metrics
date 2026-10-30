package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// binary は TestMain がビルドした arch-baseline のパス｡go run は子の exit code を 1 に潰すので、直接叩く｡
var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "arch-baseline-test")
	if err != nil {
		panic(err)
	}
	binary = filepath.Join(dir, "arch-baseline")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		panic(fmt.Sprintf("go build 失敗: %v\n%s", err, out))
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

const (
	emptyGolangci   = `{"Issues":[],"Report":{}}`
	emptyGosec      = `{"Golang errors":{},"Issues":[],"Stats":{"files":1,"lines":3,"nosec":0,"found":0}}`
	emptyGoArchLint = `{"Type":"models.Check","Payload":{"ArchHasWarnings":false,"ArchWarningsDeps":[],"ArchWarningsNotMatched":[],"ArchWarningsDeepScan":[]}}`
)

// modularityReport は analyze-modularity の警告なしのレポートで、total_packages だけを差し替える｡
func modularityReport(totalPackages int) string {
	return fmt.Sprintf(`{"packages":[],"summary":{"mean_loc":0,"stddev_loc":0,"mean_public_ratio":0,"total_packages":%d,"total_warnings":0,"actionable_warnings":0,"main_sequence":{"excluded_packages":null,"structurally_constrained_count":0,"non_volatile_zone_of_pain_count":0,"actionable_count":0,"volatility_window_commits":200,"volatile_commit_threshold":5}}}`, totalPackages)
}

// fixture は測定対象のモジュールと、外部ツールを差し替えるスタブの置き場｡
type fixture struct {
	t       *testing.T
	stubBin string
	project string
}

// newFixture は全ツールのスタブを「指摘なし」で置き、測定対象のモジュールを作る｡
// PATH はスタブと go と標準コマンドだけに絞る｡開発機の PATH にある本物の vulncheck などを拾わないため｡
func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{t: t, stubBin: filepath.Join(root, "bin"), project: filepath.Join(root, "project")}
	for _, dir := range []string{f.stubBin, f.project} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.stub("golangci-lint", 0, emptyGolangci, "")
	f.stub("gosec", 0, emptyGosec, "")
	f.stubGoArchLint(0, emptyGoArchLint, "")
	f.stub("vulncheck", 0, "", "")
	f.stub("analyze-modularity", 0, modularityReport(0), "")
	f.stub("analyze-arch-lint", 0, "", "")

	f.write("go.mod", "module baseline-fixture\ngo 1.26.1\n")
	f.write(".go-arch-lint.yml", "version: 3\nworkdir: .\n")
	f.write("fixture.go", "package fixture\n\nfunc Answer() int { return 42 }\n")
	return f
}

func (f *fixture) write(rel, content string) {
	f.t.Helper()
	path := filepath.Join(f.project, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

// stub は name のスタブを置く｡stdout と stderr の中身はファイルに書き、スタブはそれを cat する｡
// 引用符のエスケープを考えずに任意の JSON を返せる｡
func (f *fixture) stub(name string, rc int, stdout, stderr string) {
	f.writeStub(name, "", rc, stdout, stderr)
}

// stubGoArchLint は引数に --json があるときだけ stdout を出す｡実物の v1.19.0 は --json-output を知らない｡
func (f *fixture) stubGoArchLint(rc int, stdout, stderr string) {
	guard := `json=; for a in "$@"; do [ "$a" = --json ] && json=1; done
[ -n "$json" ] || { echo "unknown flag" >&2; exit 1; }
`
	f.writeStub("go-arch-lint", guard, rc, stdout, stderr)
}

func (f *fixture) writeStub(name, guard string, rc int, stdout, stderr string) {
	f.t.Helper()
	base := filepath.Join(f.stubBin, name)
	for ext, content := range map[string]string{".out": stdout, ".err": stderr} {
		if err := os.WriteFile(base+ext, []byte(content), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
	script := fmt.Sprintf("#!/bin/sh\n%scat '%s.out'\ncat '%s.err' >&2\nexit %d\n", guard, base, base, rc)
	if err := os.WriteFile(base, []byte(script), 0o755); err != nil { // #nosec G306 -- テストのスタブは実行権限が要る
		f.t.Fatal(err)
	}
}

// removeStub は PATH からスタブを消す｡
func (f *fixture) removeStub(name string) {
	f.t.Helper()
	if err := os.Remove(filepath.Join(f.stubBin, name)); err != nil {
		f.t.Fatal(err)
	}
}

// addModuleTool は測定対象のモジュールに cmd/<name> を作って go.mod の tool directive に足し、PATH のスタブは消す｡
func (f *fixture) addModuleTool(name, body string) {
	f.t.Helper()
	f.removeStub(name)
	f.write(filepath.Join("cmd", name, "main.go"), "package main\n\n"+body+"\n")
	gomod, err := os.ReadFile(filepath.Join(f.project, "go.mod"))
	if err != nil {
		f.t.Fatal(err)
	}
	f.write("go.mod", string(gomod)+"tool baseline-fixture/cmd/"+name+"\n")
}

func (f *fixture) writePassingTest() {
	f.write("fixture_test.go", `package fixture

import "testing"

func TestAnswer(t *testing.T) {
	if Answer() != 42 {
		t.Fatal("unexpected")
	}
}
`)
}

func (f *fixture) writeFailingTest() {
	f.write("fixture_test.go", `package fixture

import "testing"

func TestAnswer(t *testing.T) {
	t.Fatal("deliberate failure")
}
`)
	// go test は失敗したパッケージの後も ok 行を出し続ける｡パッケージ順で後ろに
	// 並ぶ成功パッケージを置き、FAIL 行が出力の末尾に来ない状況を作る｡
	f.write("zzlast/zzlast.go", "package zzlast\n\nfunc Noop() {}\n")
	f.write("zzlast/zzlast_test.go", "package zzlast\n\nimport \"testing\"\n\nfunc TestNoop(t *testing.T) { Noop() }\n")
}

// run は cwd でビルド済みの arch-baseline に引数を渡し、exit code と stdout+stderr を返す｡
func (f *fixture) run(cwd string, args ...string) (int, string) {
	f.t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		f.t.Fatal(err)
	}
	cmd := exec.Command(binary, args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "PATH="+strings.Join([]string{f.stubBin, filepath.Dir(goBin), "/usr/bin", "/bin"}, ":"))
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		f.t.Fatalf("実行失敗: %v\n%s", err, out)
	}
	return exitErr.ExitCode(), string(out)
}

// measure は出力先を省いて project を測る｡
func (f *fixture) measure() (int, string) {
	f.t.Helper()
	return f.run(f.project, f.project)
}

// result は path の JSON を読み、dotted の key をたどった値を返す｡
func (f *fixture) result(path string, keys ...string) any {
	f.t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		f.t.Fatalf("JSON が読めない: %v", err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		f.t.Fatalf("JSON として読めない: %v\n%s", err, data)
	}
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			f.t.Fatalf("%v をたどれない: %v", keys, v)
		}
		v = m[k]
	}
	return v
}

func (f *fixture) defaultJSON() string {
	return filepath.Join(f.project, "baseline.json")
}

func (f *fixture) assertNoDefaultJSON() {
	f.t.Helper()
	if _, err := os.Stat(f.defaultJSON()); !errors.Is(err, os.ErrNotExist) {
		f.t.Errorf("baseline.json を書いてはいけない (stat: %v)", err)
	}
}

func assertCode(t *testing.T, got, want int, out string) {
	t.Helper()
	if got != want {
		t.Fatalf("exit code = %d, want %d\n%s", got, want, out)
	}
}

func assertFailed(t *testing.T, got int, out string) {
	t.Helper()
	if got == 0 {
		t.Fatalf("exit 0 で終わってはいけない\n%s", out)
	}
}

func assertContains(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("出力に %q が無い\n%s", w, out)
		}
	}
}

func assertNotContains(t *testing.T, out, unwanted string) {
	t.Helper()
	if strings.Contains(out, unwanted) {
		t.Errorf("出力に %q があってはいけない\n%s", unwanted, out)
	}
}

func TestGoTestが落ちてもJSONを出してexit0する(t *testing.T) {
	f := newFixture(t)
	f.writeFailingTest()

	code, out := f.measure()

	assertCode(t, code, 0, out)
	// 後ろの成功パッケージの ok 行に埋もれず、失敗した関数名まで届く
	assertContains(t, out, "カバレッジ: テスト失敗のため測定不可", "--- FAIL: TestAnswer")
	// 参考値が取れなくても測定結果そのものは残る
	f.result(f.defaultJSON())
}

func TestGoTestが通れば平均カバレッジを出す(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()

	code, out := f.measure()

	assertCode(t, code, 0, out)
	assertContains(t, out, "平均カバレッジ:", "govulncheck: 脆弱性なし")
	if got := f.result(f.defaultJSON(), "govulncheck", "status"); got != "ok" {
		t.Errorf("govulncheck.status = %v, want ok", got)
	}
}

func TestJSONのprojectは測った場所ではなくmodulePath(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()

	code, out := f.measure()

	assertCode(t, code, 0, out)
	if got := f.result(f.defaultJSON(), "project"); got != "baseline-fixture" {
		t.Errorf("project = %v, want baseline-fixture", got)
	}
}

func Test同じコードを測り直せばJSONは変わらない(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	outDir := t.TempDir()
	first, second := filepath.Join(outDir, "first.json"), filepath.Join(outDir, "second.json")

	code, out := f.run(f.project, f.project, first)
	assertCode(t, code, 0, out)
	code, out = f.run(f.project, f.project, second)
	assertCode(t, code, 0, out)

	a, _ := os.ReadFile(first)
	b, _ := os.ReadFile(second)
	if !bytes.Equal(a, b) {
		t.Errorf("2 回の測定で JSON が違う\n%s\n---\n%s", a, b)
	}
	if got := f.result(first, "timestamp"); got != nil {
		t.Errorf("測定日時を入れてはいけない: %v", got)
	}
}

func Test出力先を省くとルートのbaselineJSONに書き測り直すと上書きする(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()

	code, out := f.measure()
	assertCode(t, code, 0, out)
	f.stub("analyze-modularity", 0, modularityReport(5), "")
	code, out = f.measure()

	assertCode(t, code, 0, out)
	assertContains(t, out, "詳細結果を保存しました: "+f.defaultJSON())
	if got := f.result(f.defaultJSON(), "modularity", "summary", "total_packages"); got != 5.0 {
		t.Errorf("total_packages = %v, want 5", got)
	}
	if matches, _ := filepath.Glob(filepath.Join(f.project, "baseline-*.json")); len(matches) > 0 {
		t.Errorf("日付付きのファイルを書いてはいけない: %v", matches)
	}
}

func Test2つめの引数で出力先を指定できルートには書かない(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	cwd := t.TempDir()
	if err := os.Mkdir(filepath.Join(cwd, "out"), 0o755); err != nil {
		t.Fatal(err)
	}

	// 相対パスは呼び出したディレクトリから解決する
	code, out := f.run(cwd, f.project, "out/before.json")

	assertCode(t, code, 0, out)
	if got := f.result(filepath.Join(cwd, "out", "before.json"), "govulncheck", "status"); got != "ok" {
		t.Errorf("govulncheck.status = %v, want ok", got)
	}
	f.assertNoDefaultJSON()
}

func Test出力先のディレクトリが無ければ測る前に止まる(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()

	code, out := f.run(f.project, f.project, filepath.Join(t.TempDir(), "missing", "out.json"))

	assertFailed(t, code, out)
	assertContains(t, out, "出力先のディレクトリが存在しません")
	assertNotContains(t, out, "golangci-lint")
}

func TestGoModが無ければサブディレクトリの候補を挙げて止まる(t *testing.T) {
	f := newFixture(t)
	parent := filepath.Dir(f.project)

	code, out := f.run(parent, parent)

	assertFailed(t, code, out)
	assertContains(t, out, "go.mod が見つかりません", f.project)
}

func TestGovulncheckが脆弱性を見つけたら出力を残して測定を続ける(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	f.stub("vulncheck", 3, "Vulnerability #1: GO-2024-0001\n", "")

	code, out := f.measure()

	assertCode(t, code, 0, out)
	assertContains(t, out, "govulncheck: 脆弱性あり", "GO-2024-0001")
	if got := f.result(f.defaultJSON(), "govulncheck", "status"); got != "vulnerable" {
		t.Errorf("govulncheck.status = %v, want vulnerable", got)
	}
}

func TestGovulncheckが実行できなければ未測定として理由を出し測定を続ける(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	// DB に届かないと脆弱性の有無と無関係に落ちる
	f.stub("vulncheck", 1, `govulncheck: fetching vulnerabilities: Get "https://vuln.go.dev/index/modules.json.gz": dial tcp: lookup vuln.go.dev: no such host`+"\n", "")

	code, out := f.measure()

	assertCode(t, code, 0, out)
	assertContains(t, out, "govulncheck: 未測定", "no such host")
	assertNotContains(t, out, "脆弱性なし")
	if got := f.result(f.defaultJSON(), "govulncheck", "status"); got != "unmeasured" {
		t.Errorf("govulncheck.status = %v, want unmeasured", got)
	}
	if got := f.result(f.defaultJSON(), "govulncheck", "exit_code"); got != 1.0 {
		t.Errorf("govulncheck.exit_code = %v, want 1", got)
	}
}

func TestGoArchLintの依存方向違反を件数と中身で出しJSONに残す(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	// 違反があると go-arch-lint は JSON を出したうえで exit 1 する
	f.stubGoArchLint(1, `{"Type":"models.Check","Payload":{"ArchHasWarnings":true,"ArchWarningsDeps":[{"ComponentName":"domain","ResolvedImportName":"example.com/infra"}],"ArchWarningsNotMatched":[],"ArchWarningsDeepScan":[]}}`, "")

	code, out := f.measure()

	assertCode(t, code, 0, out)
	assertContains(t, out, "依存方向違反数: 1", "domain -> example.com/infra")
	deps, _ := f.result(f.defaultJSON(), "go_arch_lint", "Payload", "ArchWarningsDeps").([]any)
	if len(deps) != 1 {
		t.Errorf("ArchWarningsDeps = %v, want 1 件", deps)
	}
}

func TestGoArchLintの設定が無ければテンプレートを案内して止まる(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	if err := os.Remove(filepath.Join(f.project, ".go-arch-lint.yml")); err != nil {
		t.Fatal(err)
	}

	code, out := f.measure()

	assertFailed(t, code, out)
	assertContains(t, out, ".go-arch-lint.yml が見つかりません", "go-arch-metrics:setup")
	f.assertNoDefaultJSON()
}

func TestGoArchLintが実行に失敗したら理由を出して止まる(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	// 実行エラーのときは stdout が空で、理由は stderr に出る
	f.stubGoArchLint(1, "", "failed to walk project tree")

	code, out := f.measure()

	assertFailed(t, code, out)
	assertContains(t, out, "go-arch-lint の実行に失敗しました", "failed to walk project tree")
	f.assertNoDefaultJSON()
}

func TestAnalyzeModularityが実行に失敗したら理由を出して止まる(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	// Ca/Ce の算出は go list に頼るので、go list が落ちると stdout は空で理由は stderr に出る
	f.stub("analyze-modularity", 1, "", "エラー: go list の実行に失敗 (exit status 1)")

	code, out := f.measure()

	assertFailed(t, code, out)
	assertContains(t, out, "go list の実行に失敗")
	f.assertNoDefaultJSON()
}

func TestAnalyzeModularityがJSONを出さなければ止まる(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	f.stub("analyze-modularity", 0, "", "")

	code, out := f.measure()

	assertFailed(t, code, out)
	assertContains(t, out, "analyze-modularity の実行に失敗しました")
}

func TestGolangciLintの指摘は件数で出しJSONに残して測定を続ける(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	// 指摘があると golangci-lint は JSON を出したうえで exit 1 する
	f.stub("golangci-lint", 1, `{"Issues":[{"FromLinter":"gocognit","Text":"cognitive complexity 31"}],"Report":{}}`, "")

	code, out := f.measure()

	assertCode(t, code, 0, out)
	assertContains(t, out, "合計違反数: 1", "gocognit: 1")
	issues, _ := f.result(f.defaultJSON(), "golangci_lint", "Issues").([]any)
	if len(issues) != 1 {
		t.Errorf("golangci_lint.Issues = %v, want 1 件", issues)
	}
}

func TestGolangciLintが実行に失敗したら理由を出して止まる(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	// 設定の誤りなどで実行できないと exit 3 で、stdout は空、理由は stderr に出る
	f.stub("golangci-lint", 3, "", "Error: unknown linters: 'nosuchlinter'")

	code, out := f.measure()

	assertFailed(t, code, out)
	assertContains(t, out, "golangci-lint の実行に失敗しました (exit 3)", "unknown linters")
	assertNotContains(t, out, "違反なし")
	f.assertNoDefaultJSON()
}

func TestGolangciLintがJSONを出してもexit0と1以外なら止まる(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	// エラーをログに出したまま走り切ると、JSON を出したうえで exit 7 する
	f.stub("golangci-lint", 7, `{"Issues":[],"Report":{"Error":"typechecking error"}}`, "")

	code, out := f.measure()

	assertFailed(t, code, out)
	assertContains(t, out, "golangci-lint の実行に失敗しました (exit 7)")
}

func TestGolangciLintが対象コードをコンパイルできなければ理由を出して止まる(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	// コンパイルできないと typecheck の Issue を出して exit 1 する｡ほかの linter はそのパッケージを解析しない
	f.stub("golangci-lint", 1, `{"Issues":[{"FromLinter":"typecheck","Text":"./a.go:2:23: undefined: undefinedThing"}],"Report":{}}`, "")

	code, out := f.measure()

	assertFailed(t, code, out)
	assertContains(t, out, "golangci-lint が対象のコードをコンパイルできませんでした", "undefined: undefinedThing")
	f.assertNoDefaultJSON()
}

func TestGosecが対象コードをコンパイルできなければ理由を出して止まる(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	// コンパイルできないと "Golang errors" に理由を入れて exit 1 する｡そのパッケージは解析しない
	f.stub("gosec", 1, `{"Golang errors":{"/p/a.go":[{"line":2,"column":23,"error":"undefined: undefinedThing"}]},"Issues":[],"Stats":{"files":1,"lines":2,"nosec":0,"found":0}}`, "")

	code, out := f.measure()

	assertFailed(t, code, out)
	assertContains(t, out, "gosec が対象のコードをコンパイルできませんでした", "/p/a.go:2:23: undefined: undefinedThing")
	f.assertNoDefaultJSON()
}

func TestGosecの指摘は件数で出しJSONに残して測定を続ける(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	// 指摘があると gosec は JSON を出したうえで exit 1 する
	f.stub("gosec", 1, `{"Golang errors":{},"Issues":[{"rule_id":"G204","details":"Subprocess launched with variable"}],"Stats":{"files":1,"lines":3,"nosec":0,"found":1}}`, "")

	code, out := f.measure()

	assertCode(t, code, 0, out)
	assertContains(t, out, "セキュリティ指摘数: 1", "G204: 1")
	issues, _ := f.result(f.defaultJSON(), "gosec", "Issues").([]any)
	if len(issues) != 1 {
		t.Errorf("gosec.Issues = %v, want 1 件", issues)
	}
}

func TestGosecが実行に失敗したら理由を出して止まる(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	// 指摘ありと同じ exit 1 だが、stdout にレポートが出ず理由は stderr に出る
	f.stub("gosec", 1, "", "[gosec] No packages found")

	code, out := f.measure()

	assertFailed(t, code, out)
	assertContains(t, out, "gosec の実行に失敗しました (exit 1)", "No packages found")
	f.assertNoDefaultJSON()
}

func TestGoToolで呼ぶツールのビルドが失敗したら理由を出して止まる(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	f.addModuleTool("golangci-lint", "func main() { undefinedThing() }")

	code, out := f.measure()

	assertFailed(t, code, out)
	assertContains(t, out, "golangci-lint の実行に失敗しました", "undefined: undefinedThing")
}

func TestPATHに無いツールはGoModのToolDirectiveからGoToolで呼ぶ(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	f.addModuleTool("vulncheck", "import \"fmt\"\n\nfunc main() { fmt.Println(\"vulncheck via go tool\") }")
	f.addModuleTool("analyze-modularity", "import \"fmt\"\n\nfunc main() {\n\tfmt.Print(`"+modularityReport(7)+"`)\n}")
	f.addModuleTool("analyze-arch-lint", "func main() {}")

	code, out := f.measure()

	assertCode(t, code, 0, out)
	assertContains(t, out, "govulncheck: 脆弱性なし")
	if got := f.result(f.defaultJSON(), "modularity", "summary", "total_packages"); got != 7.0 {
		t.Errorf("total_packages = %v, want 7", got)
	}
}

func TestPATHにもToolDirectiveにも無いツールは両方を挙げて止まる(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	f.removeStub("vulncheck")

	code, out := f.measure()

	assertFailed(t, code, out)
	assertContains(t, out, "vulncheck が PATH にも go.mod の tool directive にも見つかりません")
}

func TestAnalyzeModularityのMainSequence集計をサマリに出す(t *testing.T) {
	f := newFixture(t)
	f.writePassingTest()
	f.stub("analyze-modularity", 0, `{"packages":[],"summary":{"mean_loc":0,"stddev_loc":0,"mean_public_ratio":0,"total_packages":3,"total_warnings":3,"actionable_warnings":1,"main_sequence":{"excluded_packages":["cmd/app"],"structurally_constrained_count":1,"non_volatile_zone_of_pain_count":2,"actionable_count":1,"actionable_max_distance":0.7,"actionable_mean_distance":0.7,"volatility_window_commits":200,"volatile_commit_threshold":5}}}`, "")

	code, out := f.measure()

	assertCode(t, code, 0, out)
	assertContains(t, out,
		"アクション可能 Distance > 0.5: 1 件",
		"構造制約パッケージ数: 1",
		"非揮発の Zone of Pain (直近 200 commit で 5 回未満): 2 件",
		"警告数: 3 (うち --strict が失敗に数えるもの: 1)",
		"最大 Distance (アクション可能): 0.7",
	)
	if got := f.result(f.defaultJSON(), "modularity", "summary", "main_sequence", "actionable_count"); got != 1.0 {
		t.Errorf("actionable_count = %v, want 1", got)
	}
}
