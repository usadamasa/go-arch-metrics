package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// 各 measure は JSON に残すツールの生の出力を返す。指摘があっても測定は続け、実行そのものが
// 失敗したとき (指摘の数を信用できないとき) だけ error を返す。失敗の理由はツールが stderr に出している

// section は 1 つの measure の出力。measure は並列に回すので、終わってから決まった順に書き出す
type section struct {
	out bytes.Buffer // サマリ。stdout へ出す
	err bytes.Buffer // ツールの stderr と警告。stderr へ出す
}

func (s *section) printf(format string, a ...any) { fmt.Fprintf(&s.out, format, a...) }
func (s *section) println(a ...any)               { fmt.Fprintln(&s.out, a...) }
func (s *section) errorf(format string, a ...any) { fmt.Fprintf(&s.err, format, a...) }

func (s *section) flush() {
	fmt.Print(s.out.String())
	fmt.Fprint(os.Stderr, s.err.String())
}

// hasKeys は raw が keys をすべて持つ JSON object かを返す。
func hasKeys(raw []byte, keys ...string) bool {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return false
	}
	for _, k := range keys {
		if _, ok := obj[k]; !ok {
			return false
		}
	}
	return true
}

// countEntry はキーごとの件数。
type countEntry struct {
	key   string
	count int
}

// countByKey は件数の多い順に並べる。同数はキーの順
func countByKey(keys []string) []countEntry {
	counts := map[string]int{}
	for _, k := range keys {
		counts[k]++
	}
	entries := make([]countEntry, 0, len(counts))
	for k, n := range counts {
		entries = append(entries, countEntry{k, n})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		return entries[i].key < entries[j].key
	})
	return entries
}

func printCounts(w *section, title string, keys []string) {
	w.printf("%s: %d\n", title, len(keys))
	for _, e := range countByKey(keys) {
		w.printf("  %s: %d\n", e.key, e.count)
	}
}

// measureGolangci は golangci-lint を回す。exit 0 は指摘なし、1 は指摘ありで、どちらも stdout に JSON が出る。
// 3 以上は実行できていない (pkg/exitcodes/exitcodes.go)。go tool 経由のツールのビルド失敗も 1 で stdout が
// 空なので、JSON の有無でも見分ける。v2 では --out-format json は廃止。--show-stats=false で JSON の後ろに
// 件数の行を出させない
func measureGolangci(w *section, t tool, root string) (json.RawMessage, error) {
	w.println("--- golangci-lint (テスト可能性・保守性) ---")
	out, rc, err := t.run(root, &w.err,"run", "--output.json.path", "stdout", "--show-stats=false", "--timeout", "5m", "./...")
	if err != nil {
		return nil, err
	}
	var report struct {
		Issues []struct {
			FromLinter string
			Text       string
		}
	}
	if rc > 1 || !hasKeys(out, "Issues") || json.Unmarshal(out, &report) != nil {
		return nil, fmt.Errorf("golangci-lint の実行に失敗しました (exit %d)。理由は上の出力を参照", rc)
	}
	// コンパイルできないパッケージは typecheck の指摘になり、ほかの linter は解析しない。違反数が少なく見えるので止める
	var typecheck, linters []string
	for _, issue := range report.Issues {
		if issue.FromLinter == "typecheck" {
			typecheck = append(typecheck, "  "+issue.Text)
		}
		linters = append(linters, issue.FromLinter)
	}
	if len(typecheck) > 0 {
		return nil, fmt.Errorf("golangci-lint が対象のコードをコンパイルできませんでした。理由:\n%s", strings.Join(typecheck, "\n"))
	}
	printCounts(w, "合計違反数", linters)
	return out, nil
}

// measureGoArchLint は go-arch-lint を回す。違反があっても実行エラーでも exit 1 するので、終了コードでは
// 見分けられない。違反なら stdout に check の JSON が出て、実行エラーなら stdout は空で理由が stderr に出る
func measureGoArchLint(w *section, t tool, root string) (json.RawMessage, error) {
	w.println("--- go-arch-lint (モジュール性・依存方向) ---")
	if _, err := os.Stat(filepath.Join(root, ".go-arch-lint.yml")); err != nil { // #nosec G703 -- CLI ツール: root は引数で渡された測定対象
		return nil, errors.New(`.go-arch-lint.yml が見つかりません
  テンプレート: go-arch-metrics:setup skill の references/arch-lint-config.md`)
	}
	out, rc, err := t.run(root, &w.err,"check", "--json")
	if err != nil {
		return nil, err
	}
	var report struct {
		Type    string
		Payload struct {
			ArchWarningsDeps []struct {
				ComponentName      string
				ResolvedImportName string
			}
			ArchWarningsNotMatched []json.RawMessage
			ArchWarningsDeepScan   []json.RawMessage
		}
	}
	if json.Unmarshal(out, &report) != nil || report.Type != "models.Check" {
		return nil, fmt.Errorf("go-arch-lint の実行に失敗しました (exit %d)。理由は上の出力を参照", rc)
	}
	deps := report.Payload.ArchWarningsDeps
	w.printf("依存方向違反数: %d\n", len(deps))
	for _, d := range deps[:min(len(deps), 10)] {
		w.printf("  %s -> %s\n", d.ComponentName, d.ResolvedImportName)
	}
	if len(deps) > 10 {
		w.printf("  ... 他 %d 件\n", len(deps)-10)
	}
	w.printf("どの component にも属さないファイル: %d\n", len(report.Payload.ArchWarningsNotMatched))
	w.printf("deepScan 違反数: %d\n", len(report.Payload.ArchWarningsDeepScan))
	return out, nil
}

// vulnResult は govulncheck の結果。未測定を「なし」に見せないよう、3 つの状態を分けて残す
type vulnResult struct {
	Status   string `json:"status"`
	ExitCode int    `json:"exit_code"`
}

// measureVulncheck は vulncheck (この plugin が govulncheck を組み込んだ CLI) を回す。exit 0 は脆弱性なし、
// 3 は脆弱性あり、それ以外は実行できていない (脆弱性 DB に届かなかった等)。未測定でも測定は続ける
func measureVulncheck(w *section, t tool, root string) (vulnResult, error) {
	w.println("--- govulncheck (脆弱性スキャン) ---")
	out, rc, err := t.runCombined(root, "./...")
	if err != nil {
		return vulnResult{}, err
	}
	switch rc {
	case 0:
		w.println("govulncheck: 脆弱性なし")
		return vulnResult{"ok", rc}, nil
	case 3:
		w.println("govulncheck: 脆弱性あり")
		w.println(strings.TrimRight(string(out), "\n"))
		return vulnResult{"vulnerable", rc}, nil
	default:
		w.errorf("govulncheck: 未測定 (exit %d)。実行に失敗しているので脆弱性の有無は分からない。理由:\n%s\n", rc, tailLines(string(out), 5))
		return vulnResult{"unmeasured", rc}, nil
	}
}

// measureGosec は gosec を回す。指摘ありでも実行の失敗でも exit 1 を返す (cmd/gosec/main.go)。
// 走り切れば stdout に JSON のレポートが出るので、その有無で見分ける。ログは stderr に出る
func measureGosec(w *section, t tool, root string) (json.RawMessage, error) {
	w.println("--- gosec (セキュリティ解析) ---")
	out, rc, err := t.run(root, &w.err,"-fmt", "json", "./...")
	if err != nil {
		return nil, err
	}
	var report struct {
		GolangErrors map[string][]struct {
			Line   json.Number `json:"line"`
			Column json.Number `json:"column"`
			Error  string      `json:"error"`
		} `json:"Golang errors"`
		Issues []struct {
			RuleID string `json:"rule_id"`
		}
	}
	if rc > 1 || !hasKeys(out, "Issues", "Stats") || json.Unmarshal(out, &report) != nil {
		return nil, fmt.Errorf("gosec の実行に失敗しました (exit %d)。理由は上の出力を参照", rc)
	}
	// コンパイルできないパッケージは "Golang errors" に入り、解析されない。指摘数が少なく見えるので止める
	if len(report.GolangErrors) > 0 {
		var lines []string
		for _, file := range sortedKeys(report.GolangErrors) {
			for _, e := range report.GolangErrors[file] {
				lines = append(lines, fmt.Sprintf("  %s:%s:%s: %s", file, e.Line, e.Column, e.Error))
			}
		}
		return nil, fmt.Errorf("gosec が対象のコードをコンパイルできませんでした。理由:\n%s", strings.Join(lines, "\n"))
	}
	rules := make([]string, len(report.Issues))
	for i, issue := range report.Issues {
		rules[i] = issue.RuleID
	}
	printCounts(w, "セキュリティ指摘数", rules)
	return out, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// modularitySummary は analyze-modularity の summary のうち、サマリに出すもの。
type modularitySummary struct {
	TotalPackages      int      `json:"total_packages"`
	MeanLOC            float64  `json:"mean_loc"`
	MeanPublicRatio    float64  `json:"mean_public_ratio"`
	TotalWarnings      int      `json:"total_warnings"`
	ActionableWarnings int      `json:"actionable_warnings"`
	OutlierPackages    []string `json:"outlier_packages"`
	MainSequence       struct {
		ActionableCount              int      `json:"actionable_count"`
		StructurallyConstrainedCount int      `json:"structurally_constrained_count"`
		NonVolatileZoneOfPainCount   int      `json:"non_volatile_zone_of_pain_count"`
		VolatilityWindowCommits      int      `json:"volatility_window_commits"`
		VolatileCommitThreshold      int      `json:"volatile_commit_threshold"`
		VolatilityUnmeasuredReason   string   `json:"volatility_unmeasured_reason"`
		ActionableMaxDistance        *float64 `json:"actionable_max_distance"`
		ActionableMeanDistance       *float64 `json:"actionable_mean_distance"`
	} `json:"main_sequence"`
}

// measureModularity は analyze-modularity を回す。Ca/Ce の算出は go list に頼る。失敗すると stdout は空で
// 理由が stderr に出るので、summary の有無で見分ける
func measureModularity(w *section, t tool, root string) (json.RawMessage, error) {
	w.println("--- analyze-modularity (モジュール性) ---")
	out, rc, err := t.run(root, &w.err,root)
	if err != nil {
		return nil, err
	}
	var report struct {
		Summary *modularitySummary `json:"summary"`
	}
	if json.Unmarshal(out, &report) != nil || report.Summary == nil {
		return nil, fmt.Errorf("analyze-modularity の実行に失敗しました (exit %d)。理由は上の出力を参照", rc)
	}
	printModularity(w, report.Summary)
	return out, nil
}

func printModularity(w *section, s *modularitySummary) {
	ms := s.MainSequence
	w.printf("パッケージ数: %d\n", s.TotalPackages)
	w.printf("平均 LOC: %d\n", int(s.MeanLOC))
	w.printf("平均 public ratio: %d%%\n", int(s.MeanPublicRatio*100))
	w.printf("警告数: %d (うち --strict が失敗に数えるもの: %d)\n", s.TotalWarnings, s.ActionableWarnings)
	if len(s.OutlierPackages) > 0 {
		w.printf("外れ値パッケージ: %s\n", strings.Join(s.OutlierPackages, ", "))
	}
	w.println("Main Sequence:")
	w.printf("  アクション可能 Distance > 0.5: %d 件\n", ms.ActionableCount)
	w.printf("  構造制約パッケージ数: %d\n", ms.StructurallyConstrainedCount)
	w.printf("  非揮発の Zone of Pain (直近 %d commit で %d 回未満): %d 件\n",
		ms.VolatilityWindowCommits, ms.VolatileCommitThreshold, ms.NonVolatileZoneOfPainCount)
	if ms.VolatilityUnmeasuredReason != "" {
		w.printf("  揮発性は未測定: %s\n", ms.VolatilityUnmeasuredReason)
	}
	w.printf("  最大 Distance (アクション可能): %s\n", formatDistance(ms.ActionableMaxDistance))
	w.printf("  平均 Distance (アクション可能): %s\n", formatDistance(ms.ActionableMeanDistance))
}

func formatDistance(d *float64) string {
	if d == nil {
		return "N/A"
	}
	return strconv.FormatFloat(*d, 'g', -1, 64)
}
