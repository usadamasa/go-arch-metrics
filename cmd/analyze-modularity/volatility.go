package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strconv"
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
		return commitHistory{unmeasuredReason: "shallow clone で履歴が欠けている。CI では actions/checkout に fetch-depth: 0 を付ける"}, nil
	}

	if _, err := gitOutput(moduleRoot, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		return commitHistory{unmeasuredReason: "commit がまだ無い"}, nil
	}

	root, err := canonicalDir(toplevel)
	if err != nil {
		return commitHistory{}, fmt.Errorf("リポジトリのルートを解決できません: %w", err)
	}

	out, err := gitOutput(root, "-c", "core.quotePath=off", "log", "--no-merges", "-n", fmt.Sprint(window),
		"--name-only", "--format=%x1e%H", "HEAD")
	if err != nil {
		return commitHistory{}, err
	}
	commits := parseTouchedFiles(out)

	versions, err := readFileVersions(root, commits)
	if err != nil {
		return commitHistory{}, err
	}

	counts := make(map[string]int)
	for _, c := range commits {
		touched := make(map[string]bool)
		for _, path := range c.paths {
			if !codeChanged(versions[c.hash+"^:"+path], versions[c.hash+":"+path]) {
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

// touchedCommit は 1 commit と、それが触った _test.go でない .go のパス (リポジトリのルートから)｡
type touchedCommit struct {
	hash  string
	paths []string
}

// parseTouchedFiles は git log --name-only --format=%x1e%H の出力を commit ごとに分ける｡
func parseTouchedFiles(out string) []touchedCommit {
	var commits []touchedCommit
	for block := range strings.SplitSeq(out, commitSeparator) {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if lines[0] == "" {
			continue
		}
		c := touchedCommit{hash: lines[0]}
		for _, line := range lines[1:] {
			path := strings.TrimSpace(line)
			if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				c.paths = append(c.paths, path)
			}
		}
		commits = append(commits, c)
	}
	return commits
}

// readFileVersions は各 commit が触ったファイルの、親の版と commit の版を 1 回の git cat-file --batch で読む｡
// キーは "<hash>^:<path>" と "<hash>:<path>"｡追加・削除で片側が無いときはキーを入れない｡
func readFileVersions(root string, commits []touchedCommit) (map[string][]byte, error) {
	var names []string
	for _, c := range commits {
		for _, path := range c.paths {
			names = append(names, c.hash+"^:"+path, c.hash+":"+path)
		}
	}
	versions := make(map[string][]byte, len(names))
	if len(names) == 0 {
		return versions, nil
	}

	cmd := exec.Command("git", "cat-file", "--batch")
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(strings.Join(names, "\n") + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git cat-file --batch の実行に失敗: %w\n%s", err, bytes.TrimSpace(stderr.Bytes()))
	}

	rest := out
	for _, name := range names {
		header, body, ok := bytes.Cut(rest, []byte("\n"))
		if !ok {
			return nil, fmt.Errorf("git cat-file --batch の出力が途中で切れている (%s)", name)
		}
		if bytes.HasSuffix(header, []byte(" missing")) {
			rest = body
			continue
		}
		// header は "<oid> blob <size>"
		fields := strings.Fields(string(header))
		if len(fields) != 3 || fields[1] != "blob" {
			return nil, fmt.Errorf("git cat-file --batch の出力を読めない (%s): %q", name, header)
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || len(body) < size+1 {
			return nil, fmt.Errorf("git cat-file --batch の出力を読めない (%s): %q", name, header)
		}
		versions[name] = body[:size]
		rest = body[size+1:]
	}
	return versions, nil
}

// codeChanged は before から after への変更がコードを変えたかを返す｡無い側は nil｡
// コメントを除いて parse し、位置の情報を持たない FileSet で go/format にかけて比べる｡
// 元の改行位置に左右されないので、コメントと空白だけの変更は一致する｡parse できなければ変更とみなす｡
func codeChanged(before, after []byte) bool {
	if before == nil || after == nil {
		return true
	}
	a, okA := normalizedSource(before)
	b, okB := normalizedSource(after)
	return !okA || !okB || !bytes.Equal(a, b)
}

func normalizedSource(src []byte) ([]byte, bool) {
	file, err := parser.ParseFile(token.NewFileSet(), "", src, parser.SkipObjectResolution)
	if err != nil {
		return nil, false
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), file); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
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
