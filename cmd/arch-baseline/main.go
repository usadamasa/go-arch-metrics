// arch-baseline は Go プロジェクトのアーキテクチャメトリクスを一通り測り、標準出力にサマリ、
// JSON に各ツールの生の結果を書く。数値の判断はしない (go-arch-metrics:evaluate の担当)。
//
// 各ツールの「指摘あり」では測定を続け、「実行そのものの失敗」では理由を stderr に出して非 0 で終わる。
// 指摘の数を信用できない JSON を残さないため。
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const usage = "使用方法: arch-baseline <project-root> [<output-json>]"

// baseline は出力の JSON。キーは evaluate / remediate の references が読む
type baseline struct {
	Project      string          `json:"project"`
	GolangciLint json.RawMessage `json:"golangci_lint"`
	GoArchLint   json.RawMessage `json:"go_arch_lint"`
	Gosec        json.RawMessage `json:"gosec"`
	Modularity   json.RawMessage `json:"modularity"`
	Govulncheck  vulnResult      `json:"govulncheck"`
}

func main() {
	if err := run(os.Stdout, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "エラー: %v\n", err)
		os.Exit(1)
	}
}

func run(w io.Writer, args []string) error {
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

	fmt.Fprintln(w, "=== Go アーキテクチャメトリクス ベースライン測定 ===")
	fmt.Fprintf(w, "プロジェクト: %s\n", root)
	fmt.Fprintf(w, "測定日時: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	tools, err := resolveTools(info)
	if err != nil {
		return err
	}
	result, err := measureAll(w, tools, root)
	if err != nil {
		return err
	}
	result.Project = info.Module.Path
	measureReference(w, root)
	fmt.Fprintln(w)

	if err := writeJSON(output, result); err != nil {
		return err
	}
	fmt.Fprintf(w, "詳細結果を保存しました: %s\n\n", output)
	fmt.Fprintln(w, "=== 測定完了 ===")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "次のステップ:")
	fmt.Fprintln(w, "  1. 数値をしきい値と照らす: go-arch-metrics:evaluate skill")
	fmt.Fprintln(w, "  2. 違反を是正する: go-arch-metrics:remediate skill")
	fmt.Fprintln(w, "  3. 設定ファイルの調整と CI 統合: go-arch-metrics:setup skill")
	return nil
}

// measureAll は JSON に残すツールを順に回す。
func measureAll(w io.Writer, tools map[string]tool, root string) (baseline, error) {
	var b baseline
	var err error
	if b.GolangciLint, err = measureGolangci(w, tools["golangci-lint"], root); err != nil {
		return b, err
	}
	fmt.Fprintln(w)
	if b.GoArchLint, err = measureGoArchLint(w, tools["go-arch-lint"], root); err != nil {
		return b, err
	}
	fmt.Fprintln(w)
	if b.Govulncheck, err = measureVulncheck(w, tools["vulncheck"], root); err != nil {
		return b, err
	}
	fmt.Fprintln(w)
	if b.Gosec, err = measureGosec(w, tools["gosec"], root); err != nil {
		return b, err
	}
	fmt.Fprintln(w)
	if b.Modularity, err = measureModularity(w, tools["analyze-modularity"], root); err != nil {
		return b, err
	}
	fmt.Fprintln(w)
	return b, nil
}

// resolvePaths はプロジェクトのルートと出力先を絶対パスにする。出力先の相対パスは呼び出したディレクトリから
// 解決する。測り終えてから書けないと分かるのを避け、先に確かめる
func resolvePaths(args []string) (string, string, error) {
	root, err := filepath.Abs(args[0])
	if err != nil {
		return "", "", err
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return "", "", fmt.Errorf("ディレクトリが存在しません: %s", args[0])
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", "", missingGoMod(root)
	}
	if len(args) < 2 {
		return root, filepath.Join(root, "baseline.json"), nil
	}
	output, err := filepath.Abs(args[1])
	if err != nil {
		return "", "", err
	}
	if fi, err := os.Stat(filepath.Dir(output)); err != nil || !fi.IsDir() {
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
	return fmt.Errorf("%s/go.mod が見つかりません。\n以下のサブディレクトリに go.mod があります:\n  %s\n使用方法: arch-baseline %s",
		root, strings.Join(dirs, "\n  "), dirs[0])
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
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil { // #nosec G306 -- リポジトリに commit して共有するレポート
		return fmt.Errorf("JSON を書けません: %w", err)
	}
	return nil
}
