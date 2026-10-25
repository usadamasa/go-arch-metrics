package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// loadJudgmentInputs は Main Sequence の警告を actionable にするかを決める、コードの外の材料を読む:
// git の履歴から数える揮発性と、baselinePath (空なら読まない) の base の D｡
// どちらもパッケージを解析する前に読み、baseline の誤りで解析の時間を無駄にしない｡
func loadJudgmentInputs(moduleRoot string, window int, baselinePath string) (commitHistory, map[string]float64, error) {
	var baseline map[string]float64
	if baselinePath != "" {
		var err error
		if baseline, err = loadBaseline(baselinePath); err != nil {
			return commitHistory{}, nil, err
		}
	}
	history, err := recentCommitCounts(moduleRoot, window)
	if err != nil {
		return commitHistory{}, nil, err
	}
	if history.unmeasuredReason != "" {
		fmt.Fprintf(os.Stderr, "注意: 揮発性を測りません (%s)｡Zone of Pain はすべて揮発として扱います\n", history.unmeasuredReason)
	}
	return history, baseline, nil
}

// loadBaseline は base の commit で測ったレポートを読み、import path から D への対応を返す｡
// Zone が excluded のパッケージと main_sequence の無いパッケージは入れない (base に無いものとして判定する)｡
func loadBaseline(path string) (map[string]float64, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- 利用者が --baseline で指定したレポート
	if err != nil {
		return nil, fmt.Errorf("baseline を読めません: %w", err)
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("baseline の JSON を読めません (%s): %w", path, err)
	}

	distances := make(map[string]float64, len(report.Packages))
	for _, p := range report.Packages {
		// 測り方の違う版のレポートと比べると、同じコードでも D が動く
		if p.ImportPath == "" {
			return nil, fmt.Errorf("baseline の %s に import_path が無い。base も同じ版の analyze-modularity で測る", p.Path)
		}
		if p.MainSequence == nil || p.MainSequence.Zone == ZoneExcluded {
			continue
		}
		distances[p.ImportPath] = p.MainSequence.Distance
	}
	return distances, nil
}

// notWorsenedFromBaseline は D が base から大きくなっていないかを返す｡base に無ければ false｡
func notWorsenedFromBaseline(ms *MainSequenceData) bool {
	return ms.BaselineDistance != nil && ms.Distance <= *ms.BaselineDistance
}
