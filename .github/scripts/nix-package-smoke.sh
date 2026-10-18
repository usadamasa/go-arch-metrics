#!/usr/bin/env bash
# flake の packages.default でビルドした CLI を実際に動かす。
# buildGoModule は -trimpath でビルドするので、標準ライブラリを読む処理が空の結果を exit 0 で
# 返す壊れ方をしうる。exit code だけでなく出力の中身まで確かめる。
# 使い方: nix-package-smoke.sh <bin ディレクトリの絶対パス>
# 前提: go と go-arch-lint が PATH にある (devShell)。カレントディレクトリはリポジトリのルート
set -euo pipefail

if [ $# -ne 1 ]; then
    printf '%s\n' "使い方: $0 <bin ディレクトリの絶対パス>" >&2
    exit 1
fi
BIN_DIR=$1
case "$BIN_DIR" in
    /*) ;;
    *)
        printf '%s\n' "エラー: bin ディレクトリは絶対パスで渡す: ${BIN_DIR}" >&2
        exit 1
        ;;
esac

printf '%s\n' "--- analyze-modularity ---"
"${BIN_DIR}/analyze-modularity" --strict . | jq -e '.summary.total_packages > 0'

printf '%s\n' "--- analyze-arch-lint ---"
"${BIN_DIR}/analyze-arch-lint" --strict .

printf '%s\n' "--- vulncheck ---"
# fmt.Println を呼ぶモジュールを testdata の DB で測り、標準ライブラリの脆弱性 (GO-9999-0001) を拾えるかを見る
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
printf '%s\n' "module example.com/fixture" "" "go 1.26" >"${fixture}/go.mod"
printf '%s\n' "package main" "" 'import "fmt"' "" 'func main() { fmt.Println("hello") }' >"${fixture}/main.go"

rc=0
out=$("${BIN_DIR}/vulncheck" -db "file://${PWD}/cmd/vulncheck/testdata/vulndb" -C "$fixture" ./... 2>&1) || rc=$?
printf '%s\n' "$out"
if [ "$rc" -ne 3 ]; then
    printf '%s\n' "エラー: vulncheck の exit code が ${rc} (期待は 3)" >&2
    exit 1
fi
if [[ "$out" != *"GO-9999-0001"* ]]; then
    printf '%s\n' "エラー: vulncheck の出力に GO-9999-0001 が無い" >&2
    exit 1
fi
