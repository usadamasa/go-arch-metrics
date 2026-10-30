package main

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// パッケージ統計とテストカバレッジは参考値なので、失敗しても測定全体は止めない。
// 失敗の理由は stderr に残す

// measureReference は測定対象のパッケージ数と go test のパッケージごとのカバレッジの平均を出す。
func measureReference(w *section, target target) {
	w.println("--- パッケージ統計 ---")
	w.printf("  パッケージ数: %d\n", len(target.selected.Packages))

	w.println("  テストカバレッジを測定中...")
	profile, err := os.CreateTemp("", "arch-baseline-cover-*.out")
	if err != nil {
		w.println("  カバレッジ: 測定不可 (プロファイルの置き場を作れない)")
		w.errorf("警告: %v\n", err)
		return
	}
	defer func() { _ = os.Remove(profile.Name()) }()
	if err := profile.Close(); err != nil {
		w.errorf("警告: %v\n", err)
	}

	out, err := goCombined(target.root, append([]string{"test", "-coverprofile=" + profile.Name()}, target.packages()...)...)
	if err != nil {
		w.println("  カバレッジ: テスト失敗のため測定不可")
		w.errorf("警告: go test が失敗したためカバレッジを測定できません。\n%s\n", testFailure(out))
		return
	}
	data, err := os.ReadFile(profile.Name())
	if err != nil {
		w.println("  カバレッジ: 測定不可 (プロファイルを読めない)")
		w.errorf("警告: %v\n", err)
		return
	}
	if mean, ok := target.meanCoverage(string(data)); ok {
		w.printf("  平均カバレッジ: %.1f%%\n", mean)
	} else {
		w.println("  カバレッジ: 測定不可 (カバレッジの対象になる文が無い)")
	}
}

func goCombined(root string, args ...string) (string, error) {
	cmd := exec.Command("go", args...) // #nosec G204 G702 -- 呼び出し側は go test の固定の引数と go list が返したパッケージだけを渡す
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// meanCoverage は coverprofile から、パッケージごとの文のカバレッジ (go test -cover が出す値と同じ) を
// git が追跡するファイルだけで計算し、その単純平均を返す。パッケージの大きさで重み付けしない。
// 文を持たないパッケージは平均に入れない
func (t target) meanCoverage(profile string) (float64, bool) {
	dirs := make(map[string]string, len(t.selected.Packages))
	for _, p := range t.selected.Packages {
		dirs[p.ImportPath] = p.Dir
	}
	type stmts struct{ total, covered int }
	byPackage := map[string]*stmts{}
	seen := map[string]bool{}
	for line := range strings.SplitSeq(profile, "\n") {
		// 例: example.com/m/pkg/file.go:12.34,15.2 3 1
		block, counts, ok := strings.Cut(line, " ")
		if !ok || seen[line] {
			continue
		}
		seen[line] = true
		file, _, _ := strings.Cut(block, ":")
		pkg, name := path.Dir(file), path.Base(file)
		dir, ok := dirs[pkg]
		if !ok || t.selected.Ignored(filepath.Join(dir, name)) {
			continue
		}
		numStmt, count, _ := strings.Cut(counts, " ")
		n, err1 := strconv.Atoi(numStmt)
		c, err2 := strconv.Atoi(count)
		if err1 != nil || err2 != nil {
			continue
		}
		s := byPackage[pkg]
		if s == nil {
			s = &stmts{}
			byPackage[pkg] = s
		}
		s.total += n
		if c > 0 {
			s.covered += n
		}
	}
	var sum float64
	var packages int
	for _, s := range byPackage {
		if s.total > 0 {
			sum += 100 * float64(s.covered) / float64(s.total)
			packages++
		}
	}
	if packages == 0 {
		return 0, false
	}
	return sum / float64(packages), true
}

var failLine = regexp.MustCompile(`^\s*(--- )?FAIL`)

// testFailure は go test の失敗の理由を切り出す。go test は失敗したパッケージの後も ok 行を出し続けるので、
// 末尾を切り出すと原因の行に届かないことがある。FAIL 行を拾い、無いときだけ末尾に落とす
func testFailure(out string) string {
	var fails []string
	for line := range strings.SplitSeq(out, "\n") {
		if failLine.MatchString(line) {
			fails = append(fails, line)
		}
	}
	if len(fails) == 0 {
		return "出力の末尾:\n" + tailLines(out, 5)
	}
	return strings.Join(fails[:min(len(fails), 10)], "\n")
}

func nonEmptyLines(s string) []string {
	var lines []string
	for line := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func tailLines(s string, n int) string {
	lines := nonEmptyLines(s)
	return strings.Join(lines[max(len(lines)-n, 0):], "\n")
}
