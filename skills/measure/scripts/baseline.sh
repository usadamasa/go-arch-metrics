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

# PATH 上のツール存在チェック (未インストール時はエラー終了)
require_tool() {
    local tool="$1"
    local install_hint="$2"
    if ! command -v "$tool" &>/dev/null; then
        printf '%s\n' "エラー: $tool が見つかりません" >&2
        if [[ -n "$install_hint" ]]; then
            printf '%s\n' "  インストール: ${install_hint}" >&2
        fi
        exit 1
    fi
}

# 前提ツールの一括チェック。インストール手段はツールごとに書き分けず、
# go.mod の tool directive へ寄せる (SKILL.md「前提: ツールのインストール」節)。
go_tool_hint="go.mod の tool directive に宣言して 'go install tool' (SKILL.md 参照)"
require_tool go ""
require_tool jq "brew install jq (macOS) / apt install jq (Linux)"
for tool in golangci-lint go-arch-lint gosec govulncheck spm-go analyze-modularity analyze-arch-lint; do
    require_tool "$tool" "$go_tool_hint"
done

# 全ツールが PROJECT_ROOT で実行されるため、一度だけ cd する
cd "$PROJECT_ROOT"

# golangci-lint の実行
GOLANGCI_RESULT='null'
printf '%s\n' "--- golangci-lint (テスト可能性・保守性) ---"

# JSON 形式で実行 (違反検出時の非ゼロ終了は許容)
# v2 では --out-format json は廃止。--output.json.path stdout を使う
GOLANGCI_JSON=$(golangci-lint run --output.json.path stdout --timeout 5m ./... 2>/dev/null | head -1 || true)

if [[ -n "$GOLANGCI_JSON" ]]; then
    GOLANGCI_SUMMARY=$(printf '%s' "$GOLANGCI_JSON" | jq -r '
      (.Issues // []) | group_by(.FromLinter) |
      map({linter: .[0].FromLinter, count: length}) |
      sort_by(-.count) |
      "合計違反数: \(map(.count) | add // 0)",
      (.[] | "  \(.linter): \(.count)")
    ' 2>/dev/null || printf '%s\n' "  集計スキップ (jq が必要)")
    printf '%s\n' "$GOLANGCI_SUMMARY"
    GOLANGCI_RESULT="$GOLANGCI_JSON"
else
    printf '%s\n' "  違反なし (または実行エラー)"
fi
printf '%s\n' ""

# go-arch-lint の実行
ARCH_RESULT='null'
printf '%s\n' "--- go-arch-lint (モジュール性・依存方向) ---"

if [[ ! -f ".go-arch-lint.yml" ]]; then
    printf '%s\n' "エラー: .go-arch-lint.yml が見つかりません" >&2
    printf '%s\n' "  テンプレート: go-arch-metrics:setup skill の references/arch-lint-config.md" >&2
    exit 1
else
    # 違反検出時の非ゼロ終了は許容
    ARCH_JSON=$(go-arch-lint check --json-output ./... 2>/dev/null || true)
    if [[ -n "$ARCH_JSON" ]]; then
        ARCH_VIOLATIONS=$(printf '%s' "$ARCH_JSON" | jq -r '
          (.violations // []) |
          "依存方向違反数: \(length)",
          (.[0:10][] | "  \(.packageName // "?") -> \(.dependencyName // "?")"),
          if length > 10 then "  ... 他 \(length - 10) 件" else empty end
        ' 2>/dev/null || printf '%s\n' "  集計スキップ (jq が必要)")
        printf '%s\n' "$ARCH_VIOLATIONS"
        ARCH_RESULT="$ARCH_JSON"
    else
        printf '%s\n' "  依存方向違反なし"
    fi
fi
printf '%s\n' ""

# govulncheck の実行
printf '%s\n' "--- govulncheck (脆弱性スキャン) ---"
# 脆弱性検出時の非ゼロ終了は許容
govulncheck ./... 2>&1 || true
printf '%s\n' ""

# gosec の実行
printf '%s\n' "--- gosec (セキュリティ解析) ---"
# セキュリティ警告検出時の非ゼロ終了は許容
gosec ./... 2>&1 || true
printf '%s\n' ""

# spm-go → 一時ファイルに保存 (analyze-modularity に --spm-json で渡す)
SPM_TMP=$(mktemp "${TMPDIR:-/tmp}/spm.XXXXXX")
readonly SPM_TMP
trap 'rm -f "$SPM_TMP"' EXIT

printf '%s\n' "--- spm-go + analyze-modularity (統合メトリクス) ---"

# spm-go は -f json でも進捗メッセージ (先頭) と Time 行 (末尾) を stdout に出すため、JSON 部分のみ抽出
if ! spm-go all -f json 2>/dev/null | awk '/^\{$/{found=1} found{print} /^\}$/ && found{exit}' > "$SPM_TMP"; then
    printf '%s\n' "  spm-go 実行失敗" >&2
    : > "$SPM_TMP"
fi

# analyze-modularity の実行 (spm-go 統合)
MODULARITY_RESULT='null'

SPM_ARGS=()
if [[ -s "$SPM_TMP" ]]; then
    SPM_ARGS=(--spm-json "$SPM_TMP")
fi

MOD_JSON=$(analyze-modularity "${SPM_ARGS[@]}" "$PROJECT_ROOT" || { printf '%s\n' "  analyze-modularity 実行失敗" >&2; true; })
if [[ -n "$MOD_JSON" ]]; then
    printf '%s' "$MOD_JSON" | jq -r '
        "パッケージ数: \(.summary.total_packages)",
        "平均 LOC: \(.summary.mean_loc | floor)",
        "平均 public ratio: \(.summary.mean_public_ratio | . * 100 | floor)%",
        "警告数: \(.summary.total_warnings)",
        if (.summary.outlier_packages | length) > 0
        then "外れ値パッケージ: \(.summary.outlier_packages | join(", "))"
        else empty end,
        if .summary.spm then
            "SPM 統合:",
            "  アクション可能 Distance > 0.5: \(.summary.spm.actionable_count) 件",
            "  構造制約パッケージ数: \(.summary.spm.structurally_constrained_count)",
            "  最大 Distance (アクション可能): \(.summary.spm.actionable_max_distance // "N/A")",
            "  平均 Distance (アクション可能): \(.summary.spm.actionable_mean_distance // "N/A")"
        else empty end
    ' 2>/dev/null || printf '%s\n' "  集計スキップ (jq が必要)"
    MODULARITY_RESULT="$MOD_JSON"
else
    printf '%s\n' "  実行エラー"
fi
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
  --arg project "$PROJECT_ROOT" \
  --argjson golangci "$GOLANGCI_RESULT" \
  --argjson arch "$ARCH_RESULT" \
  --argjson modularity "$MODULARITY_RESULT" \
  '{
    timestamp: $ts,
    project: $project,
    golangci_lint: $golangci,
    go_arch_lint: $arch,
    modularity: $modularity
  }' > "$OUTPUT_JSON"
printf '%s\n' "詳細結果を保存しました: $OUTPUT_JSON"

printf '%s\n' ""
printf '%s\n' "=== 測定完了 ==="
printf '%s\n' ""
printf '%s\n' "次のステップ:"
printf '%s\n' "  1. 数値をしきい値と照らす: go-arch-metrics:evaluate skill"
printf '%s\n' "  2. 違反を是正する: go-arch-metrics:remediate skill"
printf '%s\n' "  3. 設定ファイルの調整と CI 統合: go-arch-metrics:setup skill"
