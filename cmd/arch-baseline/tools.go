package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// requiredTools は測定に使う外部ツール。analyze-arch-lint はここでは呼ばないが、
// CI に組み込むツールとして揃っていることを確かめる
var requiredTools = []string{"golangci-lint", "go-arch-lint", "gosec", "vulncheck", "analyze-modularity", "analyze-arch-lint"}

// moduleInfo は go mod edit -json から取る測定対象のモジュールの情報。
type moduleInfo struct {
	Module struct {
		Path string
	}
	Tool []struct {
		Path string
	}
}

func readModule(root string) (moduleInfo, error) {
	var info moduleInfo
	out, err := exec.Command("go", "mod", "edit", "-json", filepath.Join(root, "go.mod")).Output() // #nosec G204 G702 -- 引数で渡された測定対象の go.mod を読むだけ
	if err != nil {
		return info, fmt.Errorf("go mod edit -json に失敗しました: %w", withStderr(err))
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return info, fmt.Errorf("go mod edit -json の出力を読めません: %w", err)
	}
	return info, nil
}

// tool は外部ツールの呼び出し方。PATH のコマンドか、go tool <package path>。
type tool struct {
	name string
	argv []string
}

// resolveTools は requiredTools をすべて解決する。PATH を先に見て、無ければ go.mod の tool directive から
// go tool で呼ぶ。入れ方は利用者に任せるので案内しない
func resolveTools(info moduleInfo) (map[string]tool, error) {
	tools := make(map[string]tool, len(requiredTools))
	for _, name := range requiredTools {
		t, err := resolveTool(name, info)
		if err != nil {
			return nil, err
		}
		tools[name] = t
	}
	return tools, nil
}

func resolveTool(name string, info moduleInfo) (tool, error) {
	if _, err := exec.LookPath(name); err == nil {
		return tool{name: name, argv: []string{name}}, nil
	}
	for _, t := range info.Tool {
		if goToolName(t.Path) == name {
			return tool{name: name, argv: []string{"go", "tool", t.Path}}, nil
		}
	}
	return tool{}, fmt.Errorf("%s が PATH にも go.mod の tool directive にも見つかりません", name)
}

var majorVersion = regexp.MustCompile(`^v[0-9]+$`)

// goToolName は go tool での名前を返す。パスの末尾の要素で、末尾が v2 のようなメジャーバージョンなら
// 1 つ前の要素 (go tool の規則)
func goToolName(pkgPath string) string {
	elems := strings.Split(pkgPath, "/")
	last := elems[len(elems)-1]
	if majorVersion.MatchString(last) && len(elems) > 1 {
		return elems[len(elems)-2]
	}
	return last
}

// run は dir で args を付けて呼び、stdout と exit code を返す。stderr は stderr へ流す
// (失敗の理由はツール自身が stderr に出す)。起動できなかったときだけ error を返す
func (t tool) run(dir string, stderr io.Writer, args ...string) ([]byte, int, error) {
	var stdout bytes.Buffer
	rc, err := t.invoke(dir, &stdout, stderr, args)
	return stdout.Bytes(), rc, err
}

// runCombined は stdout と stderr を 1 つにまとめて返す。
func (t tool) runCombined(dir string, args ...string) ([]byte, int, error) {
	var out bytes.Buffer
	rc, err := t.invoke(dir, &out, &out, args)
	return out.Bytes(), rc, err
}

func (t tool) invoke(dir string, stdout, stderr io.Writer, args []string) (int, error) {
	argv := append(append([]string{}, t.argv[1:]...), args...)
	cmd := exec.Command(t.argv[0], argv...) // #nosec G204 G702 -- argv は PATH か go.mod の tool directive で解決したツールと固定の引数
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitErr.ExitCode(), nil
	}
	if err != nil {
		return 0, fmt.Errorf("%s を起動できません: %w", t.name, err)
	}
	return 0, nil
}

// withStderr は exec の失敗に子の stderr を添える。
func withStderr(err error) error {
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && len(exitErr.Stderr) > 0 {
		return fmt.Errorf("%w\n%s", err, bytes.TrimSpace(exitErr.Stderr))
	}
	return err
}
