// analyze-arch-lint は .go-arch-lint.yml 自体の健全性を評価する｡
//
// go-arch-lint check が見るのは「コードが設定に違反していないか」だけで、
// 設定そのものが劣化していく方向 (パッケージを 1 つ足すたびに component を 1 つ
// 足す運用) は検出できない｡そこを数値にする｡
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	archFile := flag.String("arch-file", ".go-arch-lint.yml", "arch ファイルのパス (プロジェクトディレクトリからの相対)")
	asJSON := flag.Bool("json", false, "JSON で出力する")
	strict := flag.Bool("strict", false, "しきい値を超えたら exit code 1 を返す")
	showMetrics := flag.Bool("metrics", false, "指標の一覧・しきい値・意味・対処を出力して終わる")
	th := thresholds{}
	flag.Float64Var(&th.singletonRatio, "max-singleton-ratio", 0.5, "1 パッケージしか含まない component の割合の上限")
	flag.Float64Var(&th.compilerScopedRatio, "max-compiler-scoped-ratio", 0.3, "Go の internal 規則が既にスコープしている辺の割合の上限")
	flag.IntVar(&th.maxUnusedEdges, "max-unused-edges", 0, "未使用の mayDependOn の数の上限")
	flag.IntVar(&th.maxScopePackages, "max-scope-packages", 10, "1 つの internal スコープが抱えるパッケージ数の上限")
	flag.Parse()

	if *showMetrics {
		if _, err := os.Stdout.WriteString(renderMetrics(th)); err != nil {
			fail(err)
		}
		return
	}

	dir := "."
	if args := flag.Args(); len(args) > 0 {
		dir = args[0]
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		fail(err)
	}

	cfg, err := loadConfig(filepath.Join(absDir, *archFile))
	if err != nil {
		fail(err)
	}
	g, err := collectGraph(absDir, *archFile)
	if err != nil {
		fail(err)
	}

	r := analyze(cfg, g)

	var out []byte
	if *asJSON {
		encoded, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			fail(err)
		}
		out = append(encoded, '\n')
	} else {
		out = []byte(renderReport(r))
	}
	if _, err := os.Stdout.Write(out); err != nil {
		fail(err)
	}

	if *strict {
		if violations := checkThresholds(r, th); len(violations) > 0 {
			for _, v := range violations {
				fmt.Fprintln(os.Stderr, v)
			}
			os.Exit(1)
		}
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
	os.Exit(1)
}

func loadConfig(path string) (archConfig, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- CLIツール: パスはフラグ引数由来
	if err != nil {
		return archConfig{}, fmt.Errorf("arch ファイルを読めません: %w", err)
	}
	var cfg archConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return archConfig{}, fmt.Errorf("arch ファイルの解釈に失敗: %w", err)
	}
	if len(cfg.Components) == 0 {
		return archConfig{}, fmt.Errorf("%s に components がありません", path)
	}
	return cfg, nil
}

// mappingPayload は go-arch-lint mapping --json の必要部分｡
type mappingPayload struct {
	Payload struct {
		ProjectDirectory string
		ModuleName       string
		MappingGrouped   []struct {
			ComponentName string
			FileNames     []string
		}
	}
}

// collectGraph は component とパッケージの対応を go-arch-lint mapping から、
// 実際の import を go list から集める｡glob の解釈は go-arch-lint に任せる｡
func collectGraph(absDir, archFile string) (graph, error) {
	g := graph{
		componentPkgs: map[string][]string{},
		imports:       map[string][]string{},
		testImports:   map[string][]string{},
	}

	raw, err := run(absDir, "go-arch-lint", "mapping", "--json", "--output-color=false",
		"--project-path", absDir, "--arch-file", archFile)
	if err != nil {
		return g, err
	}
	var m mappingPayload
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return g, fmt.Errorf("go-arch-lint mapping の出力を解釈できません: %w", err)
	}

	for _, grp := range m.Payload.MappingGrouped {
		seen := map[string]bool{}
		for _, f := range grp.FileNames {
			rel, err := filepath.Rel(m.Payload.ProjectDirectory, filepath.Dir(f))
			if err != nil {
				return g, fmt.Errorf("パッケージパスを相対化できません: %w", err)
			}
			if !seen[rel] {
				seen[rel] = true
				g.componentPkgs[grp.ComponentName] = append(g.componentPkgs[grp.ComponentName], rel)
			}
		}
	}

	const sep = "\x1f"
	format := strings.Join([]string{
		"{{.ImportPath}}", "{{join .Imports \",\"}}",
		"{{join .TestImports \",\"}}", "{{join .XTestImports \",\"}}",
	}, sep)
	listed, err := run(absDir, "go", "list", "-f", format, "./...")
	if err != nil {
		return g, err
	}

	prefix := m.Payload.ModuleName + "/"
	trim := func(csv string) []string {
		var out []string
		for _, imp := range strings.Split(csv, ",") {
			if imp != "" {
				out = append(out, strings.TrimPrefix(imp, prefix))
			}
		}
		return out
	}
	for _, line := range strings.Split(strings.TrimSpace(listed), "\n") {
		fields := strings.Split(line, sep)
		if len(fields) != 4 {
			continue
		}
		pkg := strings.TrimPrefix(fields[0], prefix)
		g.imports[pkg] = trim(fields[1])
		g.testImports[pkg] = append(trim(fields[2]), trim(fields[3])...)
	}
	if len(g.imports) == 0 {
		return g, fmt.Errorf("%s に Go パッケージが見つかりません", absDir)
	}
	return g, nil
}

func run(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...) // #nosec G204 -- 呼び出し側は "go" / "go-arch-lint" のリテラルのみ
	cmd.Dir = dir
	// go.work があると cmd/ 単体のモジュールとして go list が通らない｡
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		return "", fmt.Errorf("%s の実行に失敗: %w: %s", name, err, stderr)
	}
	return string(out), nil
}
