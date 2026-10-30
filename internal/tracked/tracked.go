// Package tracked は測定の対象を構成管理の対象にそろえる。go list が列挙するパッケージのファイルを
// git check-ignore にかけ、git が ignore するファイルを対象から除く。arch-baseline・analyze-modularity・
// analyze-arch-lint はこの判定だけを使い、各ツールへ同じ対象を渡す。
//
// コンパイルの入力は変えない。追跡されるパッケージの中で ignore されたファイルも go build はコンパイルするので、
// 型検査はそのファイルを含めて行い、測定の結果からだけ除く。
package tracked

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
)

// Package は追跡されるファイルを 1 つ以上持つパッケージ。ファイルの一覧は ignore されたものを除いたもの。
type Package struct {
	ImportPath   string
	Name         string
	Dir          string
	GoFiles      []string
	CgoFiles     []string
	TestGoFiles  []string
	XTestGoFiles []string
}

func (p *Package) fileLists() []*[]string {
	return []*[]string{&p.GoFiles, &p.CgoFiles, &p.TestGoFiles, &p.XTestGoFiles}
}

// Selection は測定の対象。
type Selection struct {
	// Packages は go list の順に並ぶ
	Packages []Package
	// NotRepository はモジュールが git リポジトリの外にあり、何も除いていないこと
	NotRepository bool

	root    string
	ignored map[string]bool
}

// List は moduleRoot で go list ./... を回し、git が ignore するファイルを除いた測定の対象を返す。
// go list が除くもの (入れ子のモジュール、vendor、testdata、. と _ で始まるディレクトリ) は最初から入らない。
// env は go list に足す環境変数。
func List(moduleRoot string, env []string) (Selection, error) {
	root, err := canonical(moduleRoot)
	if err != nil {
		return Selection{}, fmt.Errorf("モジュールルートを解決できません: %w", err)
	}
	listed, err := goList(moduleRoot, env)
	if err != nil {
		return Selection{}, err
	}

	var files []string
	for i := range listed {
		for _, list := range listed[i].fileLists() {
			for _, name := range *list {
				files = append(files, filepath.Join(listed[i].Dir, name))
			}
		}
	}
	ignored, inRepository, err := checkIgnore(root, files)
	if err != nil {
		return Selection{}, err
	}

	s := Selection{NotRepository: !inRepository, root: root, ignored: ignored}
	for _, p := range listed {
		kept := false
		for _, list := range p.fileLists() {
			*list = slices.DeleteFunc(*list, func(name string) bool { return s.Ignored(filepath.Join(p.Dir, name)) })
			kept = kept || len(*list) > 0
		}
		if kept {
			s.Packages = append(s.Packages, p)
		}
	}
	return s, nil
}

// Ignored は path (絶対パス) のファイルを git が ignore するかを返す。存在しないパスは ignore しないとみなす。
func (s Selection) Ignored(path string) bool {
	resolved, err := canonical(path)
	if err != nil {
		return false
	}
	return s.ignored[resolved]
}

// Patterns は Packages をモジュールルートからの相対パス (./a、ルートは .) で返す。
// go のツールにパッケージの引数として渡す。
func (s Selection) Patterns() []string {
	patterns := make([]string, 0, len(s.Packages))
	for _, p := range s.Packages {
		dir, err := canonical(p.Dir)
		if err != nil {
			dir = p.Dir
		}
		rel, err := filepath.Rel(s.root, dir)
		if err != nil || rel == "." {
			patterns = append(patterns, ".")
			continue
		}
		patterns = append(patterns, "./"+filepath.ToSlash(rel))
	}
	return patterns
}

func goList(moduleRoot string, env []string) ([]Package, error) {
	cmd := exec.Command("go", "list", "-json=ImportPath,Name,Dir,GoFiles,CgoFiles,TestGoFiles,XTestGoFiles", "./...")
	cmd.Dir = moduleRoot
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list の実行に失敗 (%s): %w\n%s", moduleRoot, err, bytes.TrimSpace(stderr.Bytes()))
	}
	var pkgs []Package
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p Package
		if err := dec.Decode(&p); err != nil {
			if errors.Is(err, io.EOF) {
				return pkgs, nil
			}
			return nil, fmt.Errorf("go list の出力のパースに失敗: %w", err)
		}
		pkgs = append(pkgs, p)
	}
}

// checkIgnore は files (絶対パス) のうち git が ignore するものを、実体の絶対パスの集合で返す。
// 追跡されるファイルは .gitignore に当たっても ignore しない (git check-ignore の既定)。
// root が git リポジトリの外なら何も ignore せず、inRepository を false で返す。
func checkIgnore(root string, files []string) (ignored map[string]bool, inRepository bool, err error) {
	ignored = make(map[string]bool)
	// git が無いのは前提の欠如なので止める。git リポジトリでないのは正常な状態として続ける
	probe := exec.Command("git", "rev-parse", "--show-toplevel")
	probe.Dir = root
	if err := probe.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, false, fmt.Errorf("git が PATH に見つかりません: %w", err)
		}
		return ignored, false, nil
	}
	if len(files) == 0 {
		return ignored, true, nil
	}

	var input bytes.Buffer
	for _, f := range files {
		resolved, err := canonical(f)
		if err != nil {
			return nil, false, fmt.Errorf("go list が返したファイルを解決できません: %w", err)
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil {
			return nil, false, fmt.Errorf("%s をモジュールルートからの相対パスにできません: %w", f, err)
		}
		input.WriteString(rel)
		input.WriteByte(0)
	}

	cmd := exec.Command("git", "check-ignore", "-z", "--stdin")
	cmd.Dir = root
	cmd.Stdin = &input
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// exit 1 は「どれも ignore されない」
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && exitErr.ExitCode() == 1 {
			return ignored, true, nil
		}
		return nil, false, fmt.Errorf("git check-ignore の実行に失敗: %w\n%s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	for rel := range bytes.SplitSeq(out, []byte{0}) {
		if len(rel) > 0 {
			ignored[filepath.Join(root, string(rel))] = true
		}
	}
	return ignored, true, nil
}

// canonical は symlink を解決した絶対パスを返す。
func canonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
