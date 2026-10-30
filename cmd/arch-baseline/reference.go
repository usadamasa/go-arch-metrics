package main

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// パッケージ統計とテストカバレッジは参考値なので、失敗しても測定全体は止めない。
// 失敗の理由は stderr に残す

// measureReference は go list のパッケージ数と go test -cover の平均カバレッジを出す。
// カバレッジはパッケージごとの値なので、追跡パッケージの中で ignore されたファイルも含む
func measureReference(w *section, target target) {
	w.println("--- パッケージ統計 ---")
	root := target.root
	if out, err := goCombined(root, append([]string{"list"}, target.packages()...)...); err == nil {
		w.printf("  パッケージ数: %d\n", len(nonEmptyLines(out)))
	} else {
		w.println("  パッケージ数: 測定不可 (go list が失敗)")
		w.errorf("警告: go list が失敗しました。出力の末尾:\n%s\n", tailLines(out, 5))
	}

	w.println("  テストカバレッジを測定中...")
	out, err := goCombined(root, append([]string{"test", "-cover"}, target.packages()...)...)
	if err != nil {
		w.println("  カバレッジ: テスト失敗のため測定不可")
		w.errorf("警告: go test が失敗したためカバレッジを測定できません。\n%s\n", testFailure(out))
		return
	}
	if mean, ok := meanCoverage(out); ok {
		w.printf("  平均カバレッジ: %.1f%%\n", mean)
	} else {
		w.println("  カバレッジ: 測定不可 (カバレッジ行が出力にない)")
	}
}

func goCombined(root string, args ...string) (string, error) {
	cmd := exec.Command("go", args...) // #nosec G204 -- 呼び出し側は go list / go test の固定の引数と go list が返したパッケージだけを渡す
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

var coveragePattern = regexp.MustCompile(`([0-9]+\.[0-9]+)%`)

// meanCoverage は出力に現れるカバレッジの単純平均を返す。パッケージの大きさで重み付けしない
func meanCoverage(out string) (float64, bool) {
	matches := coveragePattern.FindAllStringSubmatch(out, -1)
	if len(matches) == 0 {
		return 0, false
	}
	var sum float64
	for _, m := range matches {
		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			return 0, false
		}
		sum += v
	}
	return sum / float64(len(matches)), true
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
