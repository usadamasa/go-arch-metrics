package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runGit は dir で git を実行する｡開発機の設定 (署名、既定の作者) に左右されないよう -c で上書きする｡
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	base := []string{"-C", dir, "-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}
	out, err := exec.Command("git", append(base, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v 失敗: %v\n%s", args, err, out)
	}
	return string(out)
}

// commitAll は dir の変更をすべて 1 commit にする｡
func commitAll(t *testing.T, dir, message string) {
	t.Helper()
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "--allow-empty", "-m", message)
}

// touchCommits は rel のファイルに 1 行ずつ足して n 回 commit する｡
func touchCommits(t *testing.T, dir, rel string, n int) {
	t.Helper()
	path := filepath.Join(dir, rel)
	for i := range n {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatalf("%s を開けない: %v", rel, err)
		}
		if _, err := fmt.Fprintf(f, "\n// touch %d\n", i); err != nil {
			t.Fatalf("%s に書けない: %v", rel, err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("%s を閉じられない: %v", rel, err)
		}
		commitAll(t, dir, fmt.Sprintf("touch %s %d", rel, i))
	}
}

// committedFixtureModule は writeFixtureModule を git リポジトリにして 1 commit 積んだものを返す｡
func committedFixtureModule(t *testing.T) string {
	t.Helper()
	root := writeFixtureModule(t)
	initGitRepo(t, root, "")
	commitAll(t, root, "initial")
	return root
}

func canonicalOf(t *testing.T, path string) string {
	t.Helper()
	dir, err := canonicalDir(path)
	if err != nil {
		t.Fatalf("canonicalDir(%s) 失敗: %v", path, err)
	}
	return dir
}

func Test_recentCommitCounts(t *testing.T) {
	t.Run("パッケージのディレクトリ直下の .go を触った commit を数える", func(t *testing.T) {
		root := committedFixtureModule(t)
		touchCommits(t, root, "b/b.go", 3)

		history, err := recentCommitCounts(root, 200)
		if err != nil {
			t.Fatalf("recentCommitCounts 失敗: %v", err)
		}
		if history.unmeasuredReason != "" {
			t.Fatalf("測れるはず: %s", history.unmeasuredReason)
		}
		if got := history.counts[canonicalOf(t, filepath.Join(root, "b"))]; got != 4 {
			t.Errorf("b = %d, want 4 (initial + 3)", got)
		}
		if got := history.counts[canonicalOf(t, filepath.Join(root, "c"))]; got != 1 {
			t.Errorf("c = %d, want 1 (initial)", got)
		}
	})

	t.Run("_test.go と .go 以外のファイルだけを触った commit は数えない", func(t *testing.T) {
		root := committedFixtureModule(t)
		for _, name := range []string{"b/b_test.go", "b/README.md"} {
			if err := os.WriteFile(filepath.Join(root, name), []byte("package b\n"), 0644); err != nil {
				t.Fatalf("%s 作成失敗: %v", name, err)
			}
		}
		commitAll(t, root, "add test and doc")

		history, err := recentCommitCounts(root, 200)
		if err != nil {
			t.Fatalf("recentCommitCounts 失敗: %v", err)
		}
		if got := history.counts[canonicalOf(t, filepath.Join(root, "b"))]; got != 1 {
			t.Errorf("b = %d, want 1 (initial だけ)", got)
		}
	})

	t.Run("1 commit で同じパッケージの複数ファイルを触っても 1 回", func(t *testing.T) {
		root := committedFixtureModule(t)
		if err := os.WriteFile(filepath.Join(root, "b", "b2.go"), []byte("package b\n"), 0644); err != nil {
			t.Fatalf("b2.go 作成失敗: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "b", "b.go"), []byte("package b\n\nfunc B() int { return 2 }\n"), 0644); err != nil {
			t.Fatalf("b.go 書き換え失敗: %v", err)
		}
		commitAll(t, root, "touch two files")

		history, err := recentCommitCounts(root, 200)
		if err != nil {
			t.Fatalf("recentCommitCounts 失敗: %v", err)
		}
		if got := history.counts[canonicalOf(t, filepath.Join(root, "b"))]; got != 2 {
			t.Errorf("b = %d, want 2", got)
		}
	})

	t.Run("窓より古い commit は数えない", func(t *testing.T) {
		root := committedFixtureModule(t)
		touchCommits(t, root, "b/b.go", 3)

		history, err := recentCommitCounts(root, 2)
		if err != nil {
			t.Fatalf("recentCommitCounts 失敗: %v", err)
		}
		if got := history.counts[canonicalOf(t, filepath.Join(root, "b"))]; got != 2 {
			t.Errorf("b = %d, want 2 (直近 2 commit)", got)
		}
		if got := history.counts[canonicalOf(t, filepath.Join(root, "c"))]; got != 0 {
			t.Errorf("c = %d, want 0 (initial は窓の外)", got)
		}
	})

	t.Run("shallow clone は測らない", func(t *testing.T) {
		root := committedFixtureModule(t)
		touchCommits(t, root, "b/b.go", 3)
		clone := filepath.Join(t.TempDir(), "clone")
		runGit(t, root, "clone", "-q", "--depth", "1", "file://"+root, clone)

		history, err := recentCommitCounts(clone, 200)
		if err != nil {
			t.Fatalf("shallow でもエラーにしない: %v", err)
		}
		if !strings.Contains(history.unmeasuredReason, "shallow") {
			t.Errorf("unmeasuredReason = %q, want shallow を含む", history.unmeasuredReason)
		}
	})

	t.Run("git リポジトリでなければ測らない", func(t *testing.T) {
		root := writeFixtureModule(t)

		history, err := recentCommitCounts(root, 200)
		if err != nil {
			t.Fatalf("git リポジトリでなくてもエラーにしない: %v", err)
		}
		if history.unmeasuredReason == "" {
			t.Error("unmeasuredReason が空")
		}
	})

	t.Run("commit が無ければ測らない", func(t *testing.T) {
		root := writeFixtureModule(t)
		initGitRepo(t, root, "")

		history, err := recentCommitCounts(root, 200)
		if err != nil {
			t.Fatalf("commit が無くてもエラーにしない: %v", err)
		}
		if history.unmeasuredReason == "" {
			t.Error("unmeasuredReason が空")
		}
	})
}

func Test_commitHistory_volatilityOf(t *testing.T) {
	history := commitHistory{counts: map[string]int{"/m/hot": 5, "/m/cold": 4}}

	if got, count := history.volatilityOf("/m/hot", 5); got != VolatilityVolatile || count == nil || *count != 5 {
		t.Errorf("hot: %q %v, want volatile 5 (しきい値ちょうどは揮発)", got, count)
	}
	if got, count := history.volatilityOf("/m/cold", 5); got != VolatilityNonVolatile || count == nil || *count != 4 {
		t.Errorf("cold: %q %v, want non_volatile 4", got, count)
	}
	if got, count := history.volatilityOf("/m/never", 5); got != VolatilityNonVolatile || count == nil || *count != 0 {
		t.Errorf("never: %q %v, want non_volatile 0", got, count)
	}

	unmeasured := commitHistory{unmeasuredReason: "shallow clone"}
	if got, count := unmeasured.volatilityOf("/m/hot", 5); got != VolatilityUnmeasured || count != nil {
		t.Errorf("unmeasured: %q %v, want unmeasured nil", got, count)
	}
}
