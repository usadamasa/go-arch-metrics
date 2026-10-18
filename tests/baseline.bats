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
    for tool in golangci-lint go-arch-lint gosec vulncheck \
                analyze-modularity analyze-arch-lint; do
        printf '%s\n' '#!/bin/sh' 'exit 0' > "${STUB_BIN}/${tool}"
        chmod +x "${STUB_BIN}/${tool}"
    done
    PATH="${STUB_BIN}:${PATH}"
    # go-arch-lint は違反なしの JSON を返す (--json 以外のフラグでは何も出さず落ちる)
    stub_go_arch_lint 0 '{"Type":"models.Check","Payload":{"ArchHasWarnings":false,"ArchWarningsDeps":[],"ArchWarningsNotMatched":[],"ArchWarningsDeepScan":[]}}'
    # analyze-modularity は警告なしのレポートを返す。summary が無いと baseline.sh は実行失敗とみなす
    stub_analyze_modularity 0 '{"packages":[],"summary":{"mean_loc":0,"stddev_loc":0,"mean_public_ratio":0,"total_packages":0,"total_warnings":0,"main_sequence":{"excluded_packages":null,"structurally_constrained_count":0,"actionable_count":0}}}'

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

# stub_vulncheck <exit-code> <stdout>
# 実物 (govulncheck の組み込み) は脆弱性なしで 0、脆弱性ありで 3、実行できなければそれ以外を返す。
stub_vulncheck() {
    local rc="$1" out="$2"
    printf '%s\n' '#!/bin/sh' \
        "printf '%s\n' '${out}'" \
        "exit ${rc}" > "${STUB_BIN}/vulncheck"
    chmod +x "${STUB_BIN}/vulncheck"
}

# stub_analyze_modularity <exit-code> <stdout> [stderr]
# 実物は失敗の理由を stderr に出し、stdout には JSON だけを出す。
stub_analyze_modularity() {
    local rc="$1" out="$2" err="${3:-}"
    printf '%s\n' '#!/bin/sh' \
        "printf '%s' '${out}'" \
        "printf '%s' '${err}' >&2" \
        "exit ${rc}" > "${STUB_BIN}/analyze-modularity"
    chmod +x "${STUB_BIN}/analyze-modularity"
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
    # 既定のスタブは exit 0 なので脆弱性なし
    [[ "$output" == *"govulncheck: 脆弱性なし"* ]]
    [ "$(jq -r '.govulncheck.status' "${PROJECT}"/baseline-*.json)" = "ok" ]
}

@test "govulncheck が脆弱性を見つけたら出力を残して測定を続ける" {
    write_passing_test
    stub_vulncheck 3 'Vulnerability #1: GO-2024-0001'

    run bash "$BASELINE" "$PROJECT"

    [ "$status" -eq 0 ]
    [[ "$output" == *"govulncheck: 脆弱性あり"* ]]
    [[ "$output" == *"GO-2024-0001"* ]]
    [ "$(jq -r '.govulncheck.status' "${PROJECT}"/baseline-*.json)" = "vulnerable" ]
}

@test "govulncheck が実行できなければ未測定として理由を出し、測定を続ける" {
    write_passing_test
    # DB に届かないと脆弱性の有無と無関係に落ちる
    stub_vulncheck 1 'govulncheck: fetching vulnerabilities: Get "https://vuln.go.dev/index/modules.json.gz": dial tcp: lookup vuln.go.dev: no such host'

    run bash "$BASELINE" "$PROJECT"

    [ "$status" -eq 0 ]
    [[ "$output" == *"govulncheck: 未測定"* ]]
    [[ "$output" == *"no such host"* ]]
    [[ "$output" != *"脆弱性なし"* ]]
    [ "$(jq -r '.govulncheck.status' "${PROJECT}"/baseline-*.json)" = "unmeasured" ]
    [ "$(jq -r '.govulncheck.exit_code' "${PROJECT}"/baseline-*.json)" = "1" ]
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

@test "analyze-modularity が実行に失敗したら理由を出して止まる" {
    write_passing_test
    # Ca/Ce の算出は go list に頼るので、go list が落ちると stdout は空で理由は stderr に出る
    stub_analyze_modularity 1 '' 'エラー: go list の実行に失敗 (exit status 1)'

    run bash "$BASELINE" "$PROJECT"

    [ "$status" -ne 0 ]
    [[ "$output" == *"go list の実行に失敗"* ]]
    run ls "${PROJECT}"/baseline-*.json
    [ "$status" -ne 0 ]
}

@test "analyze-modularity が JSON を出さなければ止まる" {
    write_passing_test
    stub_analyze_modularity 0 ''

    run bash "$BASELINE" "$PROJECT"

    [ "$status" -ne 0 ]
    [[ "$output" == *"analyze-modularity の実行に失敗しました"* ]]
}

@test "analyze-modularity の Main Sequence 集計をサマリに出す" {
    write_passing_test
    stub_analyze_modularity 0 '{"packages":[],"summary":{"mean_loc":0,"stddev_loc":0,"mean_public_ratio":0,"total_packages":3,"total_warnings":1,"main_sequence":{"excluded_packages":["cmd/app"],"structurally_constrained_count":1,"actionable_count":1,"actionable_max_distance":0.7,"actionable_mean_distance":0.7}}}'

    run bash "$BASELINE" "$PROJECT"

    [ "$status" -eq 0 ]
    [[ "$output" == *"アクション可能 Distance > 0.5: 1 件"* ]]
    [[ "$output" == *"構造制約パッケージ数: 1"* ]]
    [ "$(jq '.modularity.summary.main_sequence.actionable_count' "${PROJECT}"/baseline-*.json)" -eq 1 ]
}
