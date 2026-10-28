#!/usr/bin/env bash
# baseline.sh - Go プロジェクトのアーキテクチャメトリクス ベースライン測定スクリプト
# 使用方法: baseline.sh <project-root>
# 出力: メトリクスサマリと JSON 詳細ファイル (baseline-YYYYMMDD_HHMMSS.json)
set -euo pipefail

PROJECT_ROOT="${1:?使用方法: baseline.sh <project-root>}"

if [[ ! -d "$PROJECT_ROOT" ]]; then
    printf '%s\n' "エラー: ディレクトリが存在しません: $PROJECT_ROOT" >&2
    exit 1
fi

# cd 後も参照するため絶対パスに変換
PROJECT_ROOT=$(cd "$PROJECT_ROOT" && pwd)
readonly PROJECT_ROOT

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
OUTPUT_JSON="${PROJECT_ROOT}/baseline-${TIMESTAMP}.json"

if [[ ! -f "${PROJECT_ROOT}/go.mod" ]]; then
    # サブディレクトリに go.mod があるか検索
    FOUND_MODS=$(find "$PROJECT_ROOT" -maxdepth 2 -name "go.mod" -not -path "*/vendor/*" 2>/dev/null)
    if [[ -n "$FOUND_MODS" ]]; then
        printf '%s\n' "エラー: ${PROJECT_ROOT}/go.mod が見つかりません。" >&2
        printf '%s\n' "以下のサブディレクトリに go.mod があります:" >&2
        printf '%s\n' "$FOUND_MODS" | while read -r f; do
            printf '  %s\n' "$(dirname "$f")" >&2
        done
        FIRST_MOD=$(printf '%s\n' "$FOUND_MODS" | head -1)
        printf '%s\n' "使用方法: baseline.sh $(dirname "$FIRST_MOD")" >&2
    else
        printf '%s\n' "エラー: go.mod が見つかりません。Go プロジェクトルートを指定してください: $PROJECT_ROOT" >&2
    fi
    exit 1
fi

printf '%s\n' "=== Go アーキテクチャメトリクス ベースライン測定 ==="
printf '%s\n' "プロジェクト: $PROJECT_ROOT"
printf '%s\n' "測定日時: $(date '+%Y-%m-%d %H:%M:%S')"
printf '%s\n' ""

# 前提ツールを解決する。入れ方は利用者に任せるので案内しない。
# go と jq は PATH に要る。ほかは PATH を先に見て、無ければ go.mod の tool directive から go tool で呼ぶ
for tool in go jq; do
    if ! command -v "$tool" &>/dev/null; then
        printf '%s\n' "エラー: $tool が PATH に見つかりません" >&2
        exit 1
    fi
done

GO_MOD_JSON=$(go mod edit -json "${PROJECT_ROOT}/go.mod")
# JSON の project には測った場所に依らない module path を入れる
MODULE_PATH=$(printf '%s' "$GO_MOD_JSON" | jq -r '.Module.Path')
# go.mod の tool directive に載るパッケージパス (1 行に 1 つ)
MODULE_TOOLS=$(printf '%s' "$GO_MOD_JSON" | jq -r '.Tool // [] | .[].Path')
readonly GO_MOD_JSON MODULE_PATH MODULE_TOOLS

# tool_package <name>
# tool directive のうち、go tool での名前が name のパッケージパスを出す。無ければ 1 を返す。
# 名前はパスの末尾の要素で、末尾が v2 のようなメジャーバージョンなら 1 つ前の要素 (go tool の規則)
tool_package() {
    local name="$1" path base
    for path in $MODULE_TOOLS; do
        base=${path##*/}
        if [[ "$base" =~ ^v[0-9]+$ ]]; then
            base=${path%/*}
            base=${base##*/}
        fi
        if [[ "$base" == "$name" ]]; then
            printf '%s\n' "$path"
            return 0
        fi
    done
    return 1
}

# resolve_tool <name>
# 呼び出し方を配列 RESOLVED に入れる。どちらにも無ければ止まる
resolve_tool() {
    local name="$1" pkg
    if command -v "$name" &>/dev/null; then
        RESOLVED=("$name")
        return 0
    fi
    if pkg=$(tool_package "$name"); then
        RESOLVED=(go tool "$pkg")
        return 0
    fi
    printf '%s\n' "エラー: $name が PATH にも go.mod の tool directive にも見つかりません" >&2
    exit 1
}

resolve_tool golangci-lint
GOLANGCI_LINT=("${RESOLVED[@]}")
resolve_tool go-arch-lint
GO_ARCH_LINT=("${RESOLVED[@]}")
resolve_tool gosec
GOSEC=("${RESOLVED[@]}")
resolve_tool vulncheck
VULNCHECK=("${RESOLVED[@]}")
resolve_tool analyze-modularity
ANALYZE_MODULARITY=("${RESOLVED[@]}")
# analyze-arch-lint はここでは呼ばないが、CI に組み込むツールとして揃っていることを確かめる
resolve_tool analyze-arch-lint
readonly GOLANGCI_LINT GO_ARCH_LINT GOSEC VULNCHECK ANALYZE_MODULARITY

# 全ツールが PROJECT_ROOT で実行されるため、一度だけ cd する
cd "$PROJECT_ROOT"

# golangci-lint の実行
printf '%s\n' "--- golangci-lint (テスト可能性・保守性) ---"

# exit 0 は指摘なし、1 は指摘ありで、どちらも stdout に JSON が出る。3 以上は実行できていない
# (pkg/exitcodes/exitcodes.go)。go tool 経由のツールのビルド失敗も 1 で stdout が空なので、JSON の有無でも見分ける。
# v2 では --out-format json は廃止。--show-stats=false で JSON の後ろに件数の行を出させない
GOLANGCI_RC=0
GOLANGCI_JSON=$("${GOLANGCI_LINT[@]}" run --output.json.path stdout --show-stats=false --timeout 5m ./...) ||
    GOLANGCI_RC=$?
if ((GOLANGCI_RC > 1)) || ! printf '%s' "$GOLANGCI_JSON" | jq -e 'has("Issues")' >/dev/null; then
    printf '%s\n' "エラー: golangci-lint の実行に失敗しました (exit ${GOLANGCI_RC})。理由は上の出力を参照" >&2
    exit 1
fi
# コンパイルできないパッケージは typecheck の指摘になり、ほかの linter は解析しない。違反数が少なく見えるので止める
GOLANGCI_TYPECHECK=$(printf '%s' "$GOLANGCI_JSON" |
    jq -r '(.Issues // [])[] | select(.FromLinter == "typecheck") | "  \(.Text)"')
if [[ -n "$GOLANGCI_TYPECHECK" ]]; then
    printf '%s\n' "エラー: golangci-lint が対象のコードをコンパイルできませんでした。理由:" >&2
    printf '%s\n' "$GOLANGCI_TYPECHECK" >&2
    exit 1
fi
printf '%s' "$GOLANGCI_JSON" | jq -r '
  (.Issues // []) | group_by(.FromLinter) |
  map({linter: .[0].FromLinter, count: length}) |
  sort_by(-.count) |
  "合計違反数: \(map(.count) | add // 0)",
  (.[] | "  \(.linter): \(.count)")
'
GOLANGCI_RESULT="$GOLANGCI_JSON"
printf '%s\n' ""

# go-arch-lint の実行
ARCH_RESULT='null'
printf '%s\n' "--- go-arch-lint (モジュール性・依存方向) ---"

if [[ ! -f ".go-arch-lint.yml" ]]; then
    printf '%s\n' "エラー: .go-arch-lint.yml が見つかりません" >&2
    printf '%s\n' "  テンプレート: go-arch-metrics:setup skill の references/arch-lint-config.md" >&2
    exit 1
else
    # go-arch-lint は違反があっても実行エラーでも exit 1 するので、終了コードでは見分けられない。
    # 違反なら stdout に check の JSON が出て、実行エラーなら stdout は空で理由が stderr に出る。
    ARCH_RC=0
    ARCH_JSON=$("${GO_ARCH_LINT[@]}" check --json) || ARCH_RC=$?
    if ! printf '%s' "$ARCH_JSON" | jq -e '.Type == "models.Check"' >/dev/null 2>&1; then
        printf '%s\n' "エラー: go-arch-lint の実行に失敗しました (exit ${ARCH_RC})。理由は上の出力を参照" >&2
        exit 1
    fi
    # Go の nil slice は null で出るので // [] で受ける
    printf '%s' "$ARCH_JSON" | jq -r '
      .Payload |
      (.ArchWarningsDeps // []) as $deps |
      "依存方向違反数: \($deps | length)",
      ($deps[0:10][] | "  \(.ComponentName) -> \(.ResolvedImportName)"),
      if ($deps | length) > 10 then "  ... 他 \(($deps | length) - 10) 件" else empty end,
      "どの component にも属さないファイル: \(.ArchWarningsNotMatched // [] | length)",
      "deepScan 違反数: \(.ArchWarningsDeepScan // [] | length)"
    '
    ARCH_RESULT="$ARCH_JSON"
fi
printf '%s\n' ""

# govulncheck の実行 (vulncheck はこの plugin が govulncheck を組み込んだ CLI)
printf '%s\n' "--- govulncheck (脆弱性スキャン) ---"
# exit 0 は脆弱性なし、3 は脆弱性あり、それ以外は実行できていない (脆弱性 DB に届かなかった等)。
# 未測定を「なし」に見せないよう、3 つを分けて出力と JSON に残す。
VULN_RC=0
VULN_OUT=$("${VULNCHECK[@]}" ./... 2>&1) || VULN_RC=$?
case "$VULN_RC" in
    0)
        VULN_STATUS=ok
        printf '%s\n' "govulncheck: 脆弱性なし"
        ;;
    3)
        VULN_STATUS=vulnerable
        printf '%s\n' "govulncheck: 脆弱性あり"
        printf '%s\n' "$VULN_OUT"
        ;;
    *)
        VULN_STATUS=unmeasured
        printf '%s\n' "govulncheck: 未測定 (exit ${VULN_RC})。実行に失敗しているので脆弱性の有無は分からない。理由:" >&2
        printf '%s\n' "$VULN_OUT" | tail -5 >&2
        ;;
esac
readonly VULN_STATUS VULN_RC
printf '%s\n' ""

# gosec の実行
printf '%s\n' "--- gosec (セキュリティ解析) ---"
# gosec は指摘ありでも実行の失敗でも exit 1 を返す (cmd/gosec/main.go)。
# 走り切れば stdout に JSON のレポートが出るので、その有無で見分ける。ログは stderr に出る
GOSEC_RC=0
GOSEC_JSON=$("${GOSEC[@]}" -fmt json ./...) || GOSEC_RC=$?
if ((GOSEC_RC > 1)) || ! printf '%s' "$GOSEC_JSON" | jq -e 'has("Issues") and has("Stats")' >/dev/null; then
    printf '%s\n' "エラー: gosec の実行に失敗しました (exit ${GOSEC_RC})。理由は上の出力を参照" >&2
    exit 1
fi
# コンパイルできないパッケージは "Golang errors" に入り、解析されない。指摘数が少なく見えるので止める
GOSEC_BUILD_ERRORS=$(printf '%s' "$GOSEC_JSON" |
    jq -r '(."Golang errors" // {}) | to_entries[] | .key as $file |
      .value[] | "  \($file):\(.line):\(.column): \(.error)"')
if [[ -n "$GOSEC_BUILD_ERRORS" ]]; then
    printf '%s\n' "エラー: gosec が対象のコードをコンパイルできませんでした。理由:" >&2
    printf '%s\n' "$GOSEC_BUILD_ERRORS" >&2
    exit 1
fi
printf '%s' "$GOSEC_JSON" | jq -r '
  (.Issues // []) | group_by(.rule_id) |
  map({rule: .[0].rule_id, count: length}) |
  sort_by(-.count) |
  "セキュリティ指摘数: \(map(.count) | add // 0)",
  (.[] | "  \(.rule): \(.count)")
'
GOSEC_RESULT="$GOSEC_JSON"
printf '%s\n' ""

# analyze-modularity の実行 (API surface と Ca/Ce/A/I/D)
printf '%s\n' "--- analyze-modularity (モジュール性) ---"

# Ca/Ce の算出は go list に頼る。失敗すると stdout は空で理由が stderr に出るので、summary の有無で見分ける
MOD_RC=0
MOD_JSON=$("${ANALYZE_MODULARITY[@]}" "$PROJECT_ROOT") || MOD_RC=$?
if ! printf '%s' "$MOD_JSON" | jq -e '.summary' >/dev/null 2>&1; then
    printf '%s\n' "エラー: analyze-modularity の実行に失敗しました (exit ${MOD_RC})。理由は上の出力を参照" >&2
    exit 1
fi
printf '%s' "$MOD_JSON" | jq -r '
    "パッケージ数: \(.summary.total_packages)",
    "平均 LOC: \(.summary.mean_loc | floor)",
    "平均 public ratio: \(.summary.mean_public_ratio | . * 100 | floor)%",
    "警告数: \(.summary.total_warnings) (うち --strict が失敗に数えるもの: \(.summary.actionable_warnings))",
    if (.summary.outlier_packages | length) > 0
    then "外れ値パッケージ: \(.summary.outlier_packages | join(", "))"
    else empty end,
    "Main Sequence:",
    "  アクション可能 Distance > 0.5: \(.summary.main_sequence.actionable_count) 件",
    "  構造制約パッケージ数: \(.summary.main_sequence.structurally_constrained_count)",
    "  非揮発の Zone of Pain (直近 \(.summary.main_sequence.volatility_window_commits) commit で \(.summary.main_sequence.volatile_commit_threshold) 回未満): \(.summary.main_sequence.non_volatile_zone_of_pain_count) 件",
    if .summary.main_sequence.volatility_unmeasured_reason
    then "  揮発性は未測定: \(.summary.main_sequence.volatility_unmeasured_reason)"
    else empty end,
    "  最大 Distance (アクション可能): \(.summary.main_sequence.actionable_max_distance // "N/A")",
    "  平均 Distance (アクション可能): \(.summary.main_sequence.actionable_mean_distance // "N/A")"
'
MODULARITY_RESULT="$MOD_JSON"
printf '%s\n' ""

# パッケージ統計とテストカバレッジは参考値なので、失敗しても測定全体は止めない。
# どちらも pipefail 下でコマンド置換に直接パイプを書くと、go の非ゼロ終了が
# そのまま set -e に拾われ、JSON サマリを出す前に script が黙って死ぬ。
# stderr は捨てず、失敗の理由を残す。

# パッケージ統計 (go list)
printf '%s\n' "--- パッケージ統計 ---"
if PKG_LIST=$(go list ./... 2>&1); then
    printf '%s\n' "  パッケージ数: $(printf '%s\n' "$PKG_LIST" | wc -l | tr -d ' ')"
else
    printf '%s\n' "  パッケージ数: 測定不可 (go list が失敗)"
    printf '%s\n' "警告: go list が失敗しました。出力の末尾:" >&2
    printf '%s\n' "$PKG_LIST" | tail -5 >&2
fi

# テストカバレッジ (参考値)
printf '%s\n' "  テストカバレッジを測定中..."
if TEST_OUTPUT=$(go test ./... -cover 2>&1); then
    if COVERAGE_LINES=$(printf '%s\n' "$TEST_OUTPUT" | grep -oE '[0-9]+\.[0-9]+%'); then
        printf '%s\n' "$COVERAGE_LINES" |
            awk -F'%' '{ sum += $1; n++ } END { printf "  平均カバレッジ: %.1f%%\n", sum/n }'
    else
        printf '%s\n' "  カバレッジ: 測定不可 (カバレッジ行が出力にない)"
    fi
else
    printf '%s\n' "  カバレッジ: テスト失敗のため測定不可"
    printf '%s\n' "警告: go test が失敗したためカバレッジを測定できません。" >&2
    # go test は失敗したパッケージの後も ok 行を出し続けるので、末尾を切り出すと
    # 原因の行に届かないことがある。FAIL 行を拾い、無いときだけ末尾に落とす。
    if FAIL_LINES=$(printf '%s\n' "$TEST_OUTPUT" | grep -E '^[[:space:]]*(--- )?FAIL'); then
        printf '%s\n' "$FAIL_LINES" | head -10 >&2
    else
        printf '%s\n' "出力の末尾:" >&2
        printf '%s\n' "$TEST_OUTPUT" | tail -5 >&2
    fi
fi

printf '%s\n' ""

# JSON サマリファイルの出力
jq -n \
  --arg ts "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg project "$MODULE_PATH" \
  --argjson golangci "$GOLANGCI_RESULT" \
  --argjson arch "$ARCH_RESULT" \
  --argjson gosec "$GOSEC_RESULT" \
  --argjson modularity "$MODULARITY_RESULT" \
  --arg vuln_status "$VULN_STATUS" \
  --argjson vuln_rc "$VULN_RC" \
  '{
    timestamp: $ts,
    project: $project,
    golangci_lint: $golangci,
    go_arch_lint: $arch,
    gosec: $gosec,
    modularity: $modularity,
    govulncheck: {status: $vuln_status, exit_code: $vuln_rc}
  }' > "$OUTPUT_JSON"
printf '%s\n' "詳細結果を保存しました: $OUTPUT_JSON"

printf '%s\n' ""
printf '%s\n' "=== 測定完了 ==="
printf '%s\n' ""
printf '%s\n' "次のステップ:"
printf '%s\n' "  1. 数値をしきい値と照らす: go-arch-metrics:evaluate skill"
printf '%s\n' "  2. 違反を是正する: go-arch-metrics:remediate skill"
printf '%s\n' "  3. 設定ファイルの調整と CI 統合: go-arch-metrics:setup skill"
