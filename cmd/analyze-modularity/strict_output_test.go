package main

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestWriteActionableWarnings(t *testing.T) {
	warnings := []Warning{
		{Package: "internal/a", Type: WarningTypeUnreferencedExported, Message: "外から参照されない exported がある", Actionable: true},
		{Package: "internal/a", Type: WarningTypeHighPublicRatio, Message: "public ratio が高い", Actionable: false},
		{Package: "b", Type: WarningTypeZoneOfPain, Message: "Zone of Pain", Actionable: true},
	}

	var b strings.Builder
	if err := writeActionableWarnings(&b, warnings); err != nil {
		t.Fatal(err)
	}

	want := "internal/a: 外から参照されない exported がある (unreferenced_exported)\n" +
		"b: Zone of Pain (zone_of_pain)\n"
	if b.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

// runCommand は analyze-modularity を args で走らせ、exit code と stdout と stderr を返す｡
func runCommand(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return 0, stdout.String(), stderr.String()
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("予期しないエラー: %v", err)
	}
	return exitErr.ExitCode(), stdout.String(), stderr.String()
}

func TestStrictOutput(t *testing.T) {
	t.Run("--strict --quiet で落ちたら stdout は空で、actionable な警告だけを stderr に出す", func(t *testing.T) {
		dir := strictTestDir(t, true)

		code, stdout, stderr := runCommand(t, "--strict", "--quiet", dir)
		if code != 1 {
			t.Errorf("exit code = %d, want 1\n%s", code, stderr)
		}
		if stdout != "" {
			t.Errorf("stdout が空でない:\n%s", stdout)
		}
		if !strings.Contains(stderr, "internal/highpublic: ") || !strings.Contains(stderr, "(unreferenced_exported)") {
			t.Errorf("stderr に actionable な警告が無い:\n%s", stderr)
		}
		if strings.Contains(stderr, "(high_public_ratio)") {
			t.Errorf("actionable でない警告が stderr に出た:\n%s", stderr)
		}
	})

	t.Run("--strict --quiet で通ったら何も出さない", func(t *testing.T) {
		// git リポジトリでないと揮発性と .gitignore の注意が stderr に出るので、commit 済みのモジュールで測る。
		// b は非揮発の Zone of Pain なので actionable な警告は無い
		root := committedFixtureModule(t)

		code, stdout, stderr := runCommand(t, "--strict", "--quiet", root)
		if code != 0 {
			t.Errorf("exit code = %d, want 0\n%s", code, stderr)
		}
		if stdout != "" || stderr != "" {
			t.Errorf("出力がある:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
		}
	})

	t.Run("--strict だけなら stdout にレポートを出し、落ちた理由を stderr に出す", func(t *testing.T) {
		dir := strictTestDir(t, true)

		code, stdout, stderr := runCommand(t, "--strict", dir)
		if code != 1 {
			t.Errorf("exit code = %d, want 1\n%s", code, stderr)
		}
		var report Report
		if err := json.Unmarshal([]byte(stdout), &report); err != nil {
			t.Fatalf("stdout が JSON のレポートでない: %v\n%s", err, stdout)
		}
		if !strings.Contains(stderr, "(unreferenced_exported)") {
			t.Errorf("stderr に actionable な警告が無い:\n%s", stderr)
		}
	})

	t.Run("--strict なしの --quiet は測る前に止める", func(t *testing.T) {
		dir := strictTestDir(t, true)

		code, stdout, stderr := runCommand(t, "--quiet", dir)
		if code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
		if stdout != "" {
			t.Errorf("stdout が空でない:\n%s", stdout)
		}
		if !strings.Contains(stderr, "--strict") {
			t.Errorf("stderr に理由が無い:\n%s", stderr)
		}
	})
}
