// arch-baseline は Go プロジェクトのアーキテクチャメトリクスを一通り測り、標準出力にサマリ、
// JSON に各ツールの生の結果を書く。数値の判断はしない (go-arch-metrics:evaluate の担当)。
//
// 各ツールの「指摘あり」では測定を続け、「実行そのものの失敗」では理由を stderr に出して非 0 で終わる。
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/usadamasa/go-arch-metrics/internal/schema"
)

const usage = "使用方法: arch-baseline <project-root> [<output-json>]"

// baseline は出力の JSON。キーは measure skill の baseline.schema.json が決める
type baseline = schema.Baseline

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New(usage)
	}
	root, output, err := resolvePaths(args)
	if err != nil {
		return err
	}
	info, err := readModule(root)
	if err != nil {
		return err
	}

	fmt.Println("=== Go アーキテクチャメトリクス ベースライン測定 ===")
	fmt.Printf("プロジェクト: %s\n", root)
	fmt.Printf("測定日時: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	tools, err := resolveTools(info)
	if err != nil {
		return err
	}
	result, err := measureAll(tools, root)
	if err != nil {
		return err
	}
	result.Project = info.Module.Path

	if err := writeJSON(output, result); err != nil {
		return err
	}
	fmt.Printf("詳細結果を保存しました: %s\n\n", output)
	fmt.Println("=== 測定完了 ===")
	fmt.Println()
	fmt.Println("次のステップ:")
	fmt.Println("  1. 数値をしきい値と照らす: go-arch-metrics:evaluate skill")
	fmt.Println("  2. 違反を是正する: go-arch-metrics:remediate skill")
	fmt.Println("  3. 設定ファイルの調整と CI 統合: go-arch-metrics:setup skill")
	return nil
}

// measureAll は各ツールと参考値の測定を並列に回し、出力を決まった順に書き出す。
// 最初に失敗した measure の出力までを書いて、その error を返す
func measureAll(tools map[string]tool, root string) (baseline, error) {
	var b baseline
	// 各 step は b の別のフィールドだけに書くので、ロックは要らない
	steps := []func(*section) error{
		func(w *section) (err error) {
			b.GolangciLint, err = measureGolangci(w, tools["golangci-lint"], root)
			return err
		},
		func(w *section) (err error) {
			b.GoArchLint, err = measureGoArchLint(w, tools["go-arch-lint"], root)
			return err
		},
		func(w *section) (err error) {
			b.Govulncheck, err = measureVulncheck(w, tools["vulncheck"], root)
			return err
		},
		func(w *section) (err error) {
			b.Gosec, err = measureGosec(w, tools["gosec"], root)
			return err
		},
		func(w *section) (err error) {
			b.Modularity, err = measureModularity(w, tools["analyze-modularity"], root)
			return err
		},
		func(w *section) error {
			measureReference(w, root)
			return nil
		},
	}

	sections := make([]section, len(steps))
	errs := make([]error, len(steps))
	var wg sync.WaitGroup
	for i, step := range steps {
		wg.Go(func() { errs[i] = step(&sections[i]) })
	}
	wg.Wait()

	for i := range sections {
		sections[i].flush()
		if errs[i] != nil {
			return b, errs[i]
		}
		fmt.Println()
	}
	return b, nil
}

// resolvePaths はプロジェクトのルートと出力先を絶対パスにする。出力先の相対パスは呼び出したディレクトリから
// 解決する。測り終えてから書けないと分かるのを避け、先に確かめる
func resolvePaths(args []string) (string, string, error) {
	root, err := filepath.Abs(args[0])
	if err != nil {
		return "", "", err
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() { // #nosec G703 -- CLI ツール: パスは引数由来
		return "", "", fmt.Errorf("ディレクトリが存在しません: %s", args[0])
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil { // #nosec G703 -- CLI ツール: パスは引数由来
		return "", "", missingGoMod(root)
	}
	if len(args) < 2 {
		return root, filepath.Join(root, "baseline.json"), nil
	}
	output, err := filepath.Abs(args[1])
	if err != nil {
		return "", "", err
	}
	if fi, err := os.Stat(filepath.Dir(output)); err != nil || !fi.IsDir() { // #nosec G703 -- CLI ツール: パスは引数由来
		return "", "", fmt.Errorf("出力先のディレクトリが存在しません: %s", filepath.Dir(args[1]))
	}
	return root, output, nil
}

// missingGoMod は 1 つ下のディレクトリにある go.mod を候補として挙げる。
func missingGoMod(root string) error {
	found, _ := filepath.Glob(filepath.Join(root, "*", "go.mod"))
	var dirs []string
	for _, f := range found {
		if filepath.Base(filepath.Dir(f)) != "vendor" {
			dirs = append(dirs, filepath.Dir(f))
		}
	}
	if len(dirs) == 0 {
		return fmt.Errorf("go.mod が見つかりません。Go プロジェクトルートを指定してください: %s", root)
	}
	return fmt.Errorf(`%s/go.mod が見つかりません。
以下のサブディレクトリに go.mod があります:
  %s
使用方法: arch-baseline %s`, root, strings.Join(dirs, "\n  "), dirs[0])
}

// writeJSON は jq と同じく HTML の文字をエスケープせずに書く。ツールのメッセージの -> や < を読めるまま残す
func writeJSON(path string, b baseline) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(b); err != nil {
		return fmt.Errorf("JSON を組み立てられません: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil { // #nosec G306 G703 -- 出力先は引数で渡され、リポジトリに commit して共有するレポート
		return fmt.Errorf("JSON を書けません: %w", err)
	}
	return nil
}
