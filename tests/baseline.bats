#!/usr/bin/env bats
# baseline.sh の「参考値の失敗で測定全体を落とさない」性質を守るテスト。
# 外部ツールはスタブに差し替え、go と jq だけ本物を使う。

setup() {
    BASELINE="${BATS_TEST_DIRNAME}/../skills/measure/scripts/baseline.sh"
    STUB_BIN="${BATS_TEST_TMPDIR}/bin"
    PROJECT="${BATS_TEST_TMPDIR}/project"
    mkdir -p "$STUB_BIN" "$PROJECT"

    # baseline.sh が require_tool で要求する外部ツール。中身は問わないので
    # 何も出さずに成功するスタブでよい。go と jq は本物を使う。
    for tool in golangci-lint go-arch-lint gosec govulncheck spm-go \
                analyze-modularity analyze-arch-lint; do
        printf '%s\n' '#!/bin/sh' 'exit 0' > "${STUB_BIN}/${tool}"
        chmod +x "${STUB_BIN}/${tool}"
    done
    PATH="${STUB_BIN}:${PATH}"
    # go-arch-lint は違反なしの JSON を返す (--json 以外のフラグでは何も出さず落ちる)
    stub_go_arch_lint 0 '{"Type":"models.Check","Payload":{"ArchHasWarnings":false,"ArchWarningsDeps":[],"ArchWarningsNotMatched":[],"ArchWarningsDeepScan":[]}}'

    printf '%s\n' 'module baseline-fixture' 'go 1.26.1' > "${PROJECT}/go.mod"
    # .go-arch-lint.yml が無いと baseline.sh は測定前に exit 1 する
    printf '%s\n' 'version: 3' 'workdir: .' > "${PROJECT}/.go-arch-lint.yml"
    printf '%s\n' 'package fixture' '' 'func Answer() int { return 42 }' \
        > "${PROJECT}/fixture.go"
}

# stub_go_arch_lint <exit-code> <stdout> [stderr]
# 引数に --json があるときだけ stdout を出す。実物の v1.19.0 は --json-output を知らない。
stub_go_arch_lint() {
    local rc="$1" out="$2" err="${3:-}"
    printf '%s\n' '#!/bin/sh' \
        'for a in "$@"; do [ "$a" = --json ] && json=1; done' \
        '[ -n "$json" ] || { echo "unknown flag" >&2; exit 1; }' \
        "printf '%s' '${out}'" \
        "printf '%s' '${err}' >&2" \
        "exit ${rc}" > "${STUB_BIN}/go-arch-lint"
    chmod +x "${STUB_BIN}/go-arch-lint"
}

write_passing_test() {
    printf '%s\n' 'package fixture' '' 'import "testing"' '' \
        'func TestAnswer(t *testing.T) {' \
        '	if Answer() != 42 {' \
        '		t.Fatal("unexpected")' \
        '	}' \
        '}' > "${PROJECT}/fixture_test.go"
}

write_failing_test() {
    printf '%s\n' 'package fixture' '' 'import "testing"' '' \
        'func TestAnswer(t *testing.T) {' \
        '	t.Fatal("deliberate failure")' \
        '}' > "${PROJECT}/fixture_test.go"

    # go test は失敗したパッケージの後も ok 行を出し続ける。パッケージ順で後ろに
    # 並ぶ成功パッケージを置き、FAIL 行が出力の末尾に来ない状況を作る。
    mkdir -p "${PROJECT}/zzlast"
    printf '%s\n' 'package zzlast' '' 'func Noop() {}' > "${PROJECT}/zzlast/zzlast.go"
    printf '%s\n' 'package zzlast' '' 'import "testing"' '' \
        'func TestNoop(t *testing.T) { Noop() }' > "${PROJECT}/zzlast/zzlast_test.go"
}

@test "go test が落ちても JSON サマリを出して exit 0 する" {
    write_failing_test

    run bash "$BASELINE" "$PROJECT"

    [ "$status" -eq 0 ]
    [[ "$output" == *"カバレッジ: テスト失敗のため測定不可"* ]]
    # 後ろの成功パッケージの ok 行に埋もれず、失敗した関数名まで届く
    [[ "$output" == *"--- FAIL: TestAnswer"* ]]
    # 参考値が取れなくても測定結果そのものは残る
    ls "${PROJECT}"/baseline-*.json
}

@test "go test が通れば平均カバレッジを出す" {
    write_passing_test

    run bash "$BASELINE" "$PROJECT"

    [ "$status" -eq 0 ]
    [[ "$output" == *"平均カバレッジ:"* ]]
    ls "${PROJECT}"/baseline-*.json
}

@test "go-arch-lint の依存方向違反を件数と中身で出し、JSON に残す" {
    write_passing_test
    # 違反があると go-arch-lint は JSON を出したうえで exit 1 する
    stub_go_arch_lint 1 '{"Type":"models.Check","Payload":{"ArchHasWarnings":true,"ArchWarningsDeps":[{"ComponentName":"domain","ResolvedImportName":"example.com/infra"}],"ArchWarningsNotMatched":[],"ArchWarningsDeepScan":[]}}'

    run bash "$BASELINE" "$PROJECT"

    [ "$status" -eq 0 ]
    [[ "$output" == *"依存方向違反数: 1"* ]]
    [[ "$output" == *"domain -> example.com/infra"* ]]
    [ "$(jq '.go_arch_lint.Payload.ArchWarningsDeps | length' "${PROJECT}"/baseline-*.json)" -eq 1 ]
}

@test "go-arch-lint が実行に失敗したら理由を出して止まる" {
    write_passing_test
    # 実行エラーのときは stdout が空で、理由は stderr に出る
    stub_go_arch_lint 1 '' 'failed to walk project tree'

    run bash "$BASELINE" "$PROJECT"

    [ "$status" -ne 0 ]
    [[ "$output" == *"failed to walk project tree"* ]]
    [[ "$output" != *"依存方向違反なし"* ]]
}
