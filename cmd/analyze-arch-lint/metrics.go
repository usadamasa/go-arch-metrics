package main

import (
	"fmt"
	"strings"
)

// metric は 1 つの指標の定義｡意味と対処をここに持たせ、ドキュメント側で
// 二重に維持しない｡--metrics で一覧を、--strict の違反でその指標の分を出す｡
type metric struct {
	key     string
	label   string
	meaning string
	remedy  string
	// gating が false の指標は報告するだけで exit code を変えない｡
	gating bool
}

// thresholds は --strict の判定値｡
type thresholds struct {
	singletonRatio      float64
	compilerScopedRatio float64
	maxUnusedEdges      int
	maxScopePackages    int
}

var metrics = []metric{
	{
		key:     "singleton-ratio",
		label:   "singleton component 率",
		meaning: "1 パッケージしか含まない component の割合｡1 に近いほど、設定はアーキテクチャの宣言ではなくパッケージツリーの転記になっている｡",
		remedy:  "component をパッケージ単位ではなく役割単位で切り直す (エントリポイント / 各ツール専用の internal / 共有 internal など)｡",
		gating:  true,
	},
	{
		key:     "compiler-scoped-ratio",
		label:   "compiler-scoped edge 率",
		meaning: "Go の internal 規則が既にスコープしている辺の割合｡この辺で arch-lint が足しているのは木の内側の順序だけで、木をまたぐ import はコンパイラが弾き、循環も Go が禁じる｡",
		remedy:  "その辺の両端を 1 つの component へ畳む｡順序を仕様として残したいなら設定ではなくそのツールの doc に書く｡",
		gating:  true,
	},
	{
		key:     "unused-edges",
		label:   "未使用の mayDependOn",
		meaning: "宣言されているが実際の import に裏付けが無い辺｡陳腐化しているか、最初から過剰に許可している｡",
		remedy:  "辺を消す｡消すと落ちるなら、その依存は実在するので消さない｡",
		gating:  true,
	},
	{
		key:     "uncovered-packages",
		label:   "どの component にも属さないパッケージ",
		meaning: "検査対象から漏れているパッケージ｡そこだけ依存方向が野放しになる｡",
		remedy:  "既存 component の in: パターンが拾うようにする｡拾えないなら、そのパッケージの置き場所自体を疑う｡",
		gating:  true,
	},
	{
		key:     "scope-packages",
		label:   "1 つの internal スコープが抱えるパッケージ数",
		meaning: "component を役割単位へ畳むと、1 つのツールの internal がいくら増えても設定は変わらなくなる｡その肥大化はこの指標でしか見えない｡",
		remedy:  "薄いパッケージを隣へ畳むか、共有 internal (cmd/internal) へ引き上げるか、ツール自体を分ける｡数えるのは木の中の全パッケージなので、分割は逆に増やす｡",
		gating:  true,
	},
	{
		key:     "test-only-edges",
		label:   "_test.go だけが使う mayDependOn",
		meaning: "本番のコードには無い依存を、依存方向の宣言として書いている｡",
		remedy:  "テスト用の scaffolding をテスト対象と同じ component へ寄せるか、辺が要ることを doc に残す｡",
	},
	{
		key:     "unused-vendors",
		label:   "未使用の canUse",
		meaning: "import されていない vendor の宣言｡",
		remedy:  "canUse から消す｡どの component からも使われていないなら vendors ごと消す｡",
	},
	{
		key:     "max-fan-out",
		label:   "最大 fan-out",
		meaning: "1 つの component が持つ mayDependOn の最大数｡多いものは他のすべてを知る神 component になっている｡",
		remedy:  "その component が束ねている責務を分けるか、依存先をまとめる中間の component を置く｡",
	},
}

// renderMetrics は --metrics の出力｡
func renderMetrics(th thresholds) string {
	limits := map[string]string{
		"singleton-ratio":       fmt.Sprintf("≤ %.2f", th.singletonRatio),
		"compiler-scoped-ratio": fmt.Sprintf("≤ %.2f", th.compilerScopedRatio),
		"unused-edges":          fmt.Sprintf("≤ %d 件", th.maxUnusedEdges),
		"uncovered-packages":    "0 件",
		"scope-packages":        fmt.Sprintf("≤ %d 個", th.maxScopePackages),
	}

	var b strings.Builder
	for i, m := range metrics {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s (%s)\n", m.label, m.key)
		if limit, ok := limits[m.key]; ok {
			fmt.Fprintf(&b, "  しきい値: %s (--strict で exit 1)\n", limit)
		} else {
			b.WriteString("  しきい値: なし (報告のみ)\n")
		}
		fmt.Fprintf(&b, "  意味: %s\n", m.meaning)
		fmt.Fprintf(&b, "  対処: %s\n", m.remedy)
	}
	return b.String()
}

// violation は 1 つの指標のしきい値超過｡意味と対処を添えて出す｡
type violation struct {
	metric metric
	actual string
}

func (v violation) String() string {
	return fmt.Sprintf("しきい値超過: %s %s\n  意味: %s\n  対処: %s",
		v.metric.label, v.actual, v.metric.meaning, v.metric.remedy)
}

func metricByKey(key string) metric {
	for _, m := range metrics {
		if m.key == key {
			return m
		}
	}
	panic("未定義の metric key: " + key)
}

func checkThresholds(r report, th thresholds) []violation {
	var violations []violation
	add := func(key, format string, args ...any) {
		violations = append(violations, violation{
			metric: metricByKey(key),
			actual: fmt.Sprintf(format, args...),
		})
	}

	if ratio := r.singletonRatio(); ratio > th.singletonRatio {
		add("singleton-ratio", "%.2f > %.2f", ratio, th.singletonRatio)
	}
	if ratio := r.compilerScopedRatio(); ratio > th.compilerScopedRatio {
		add("compiler-scoped-ratio", "%.2f > %.2f", ratio, th.compilerScopedRatio)
	}
	if r.UnusedEdges > th.maxUnusedEdges {
		add("unused-edges", "%d 件 > %d 件", r.UnusedEdges, th.maxUnusedEdges)
	}
	if n := len(r.UncoveredPackages); n > 0 {
		add("uncovered-packages", "%d 件 (%s)", n, strings.Join(r.UncoveredPackages, ", "))
	}
	if r.MaxScopePackages.Count > th.maxScopePackages {
		add("scope-packages", "%s が %d 個 > %d 個",
			r.MaxScopePackages.Root, r.MaxScopePackages.Count, th.maxScopePackages)
	}
	return violations
}
