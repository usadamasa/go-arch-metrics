package main

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Volatility はパッケージの変更頻度の分類｡
type Volatility string

const (
	VolatilityVolatile    Volatility = "volatile"
	VolatilityNonVolatile Volatility = "non_volatile"
	VolatilityUnmeasured  Volatility = "unmeasured"
)

// 揮発性の既定値｡Martin (Agile PPP ch.28) は「非揮発」の数値を示していないので目安｡
// 日数ではなく commit 数で窓を切る: HEAD が同じなら結果も同じになり、日がたつだけで CI の判定が変わらない｡
// agents-daemon の実例では、非揮発の dreamio が 263 commit 中 1 回、揮発の sessionstate が 30 回で、
// この値で両側に分かれる｡
const (
	defaultVolatilityWindow = 200
	defaultVolatileCommits  = 5
)

// commitSeparator は git log の出力で commit の区切りに置く印 (RS)｡パス名には現れない｡
// --format には制御文字を直接渡せないので、git の %x1e で出させる
const commitSeparator = "\x1e"

// commitHistory は直近の commit がどのパッケージを触ったかの集計｡
type commitHistory struct {
	// counts は実体の絶対パスのディレクトリから、窓の中でそこを触った commit 数への対応
	counts map[string]int
	// unmeasuredReason は測れなかった理由｡空なら測れている
	unmeasuredReason string
}

// volatilityOf は dir のパッケージの揮発性と、窓の中の commit 数を返す｡測れていなければ数は nil｡
func (h commitHistory) volatilityOf(dir string, threshold int) (Volatility, *int) {
	if h.unmeasuredReason != "" {
		return VolatilityUnmeasured, nil
	}
	count := h.counts[dir]
	if count >= threshold {
		return VolatilityVolatile, &count
	}
	return VolatilityNonVolatile, &count
}

// recentCommitCounts は moduleRoot のリポジトリで HEAD から直近 window commit (merge を除く) を見て、
// ディレクトリ直下の _test.go でない .go を触った commit をディレクトリごとに数える｡
// 履歴が欠けていて数が信用できないとき (shallow clone)、git リポジトリでないとき、commit が無いときは
// unmeasuredReason に理由を入れて返す｡git が無いのは前提の欠如なのでエラーにする｡
func recentCommitCounts(moduleRoot string, window int) (commitHistory, error) {
	toplevel, err := gitOutput(moduleRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return commitHistory{}, fmt.Errorf("git が PATH に見つかりません: %w", err)
		}
		return commitHistory{unmeasuredReason: "git リポジトリではない"}, nil
	}

	shallow, err := gitOutput(moduleRoot, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return commitHistory{}, err
	}
	if shallow == "true" {
		return commitHistory{unmeasuredReason: "shallow clone で履歴が欠けている"}, nil
	}

	if _, err := gitOutput(moduleRoot, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		return commitHistory{unmeasuredReason: "commit がまだ無い"}, nil
	}

	root, err := canonicalDir(toplevel)
	if err != nil {
		return commitHistory{}, fmt.Errorf("リポジトリのルートを解決できません: %w", err)
	}

	out, err := gitOutput(root, "-c", "core.quotePath=off", "log", "--no-merges", "-n", fmt.Sprint(window),
		"--name-only", "--format=%x1e", "HEAD")
	if err != nil {
		return commitHistory{}, err
	}

	counts := make(map[string]int)
	for commit := range strings.SplitSeq(out, commitSeparator) {
		touched := make(map[string]bool)
		for line := range strings.SplitSeq(commit, "\n") {
			path := strings.TrimSpace(line)
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				continue
			}
			// 削除済みのディレクトリもあるので symlink は解かず、ルートに連結するだけにする
			touched[filepath.Join(root, filepath.Dir(path))] = true
		}
		for dir := range touched {
			counts[dir]++
		}
	}
	return commitHistory{counts: counts}, nil
}

// gitOutput は dir で git を実行し、前後の空白を除いた stdout を返す｡
func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...) // #nosec G204 -- 呼び出し側は git のサブコマンドのリテラルと数値の窓だけを渡す
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s の実行に失敗: %w\n%s", strings.Join(args, " "), err, bytes.TrimSpace(stderr.Bytes()))
	}
	return strings.TrimSpace(string(out)), nil
}
