package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeGoFile はテスト用の Go ソースファイルを作成する｡
func writeGoFile(t *testing.T, dir, filename, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644)
	if err != nil {
		t.Fatalf("テスト用ファイルの作成に失敗: %v", err)
	}
}

func TestPackageResult_Abstractness(t *testing.T) {
	tests := []struct {
		name string
		r    PackageResult
		want float64
	}{
		{name: "型なし → 0", r: PackageResult{}, want: 0},
		{name: "interface 1 / 型 4 → 0.25", r: PackageResult{InterfaceTypes: 1, ExportedTypes: 2, UnexportedTypes: 2}, want: 0.25},
		{name: "全部 interface → 1", r: PackageResult{InterfaceTypes: 2, ExportedTypes: 1, UnexportedTypes: 1}, want: 1},
		{name: "struct だけ → 0 (struct は抽象ではない)", r: PackageResult{ExportedTypes: 3}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.Abstractness(); got != tt.want {
				t.Errorf("Abstractness() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnalyzeDir(t *testing.T) {
	t.Run("exported と unexported の関数・型をカウントする", func(t *testing.T) {
		dir := t.TempDir()
		writeGoFile(t, dir, "sample.go", `package sample

// PublicFunc は公開関数｡
func PublicFunc() {}

// privateFunc は非公開関数｡
func privateFunc() {}

// AnotherPublic は公開関数｡
func AnotherPublic() int { return 0 }

// PublicType は公開型｡
type PublicType struct{}

// privateType は非公開型｡
type privateType struct{}

// PublicInterface は公開インタフェース｡
type PublicInterface interface {
	DoSomething()
}

// PublicMethod は公開メソッド｡
func (p *PublicType) PublicMethod() {}

// privateMethod は非公開メソッド｡
func (p *PublicType) privateMethod() {}
`)

		result, err := AnalyzeDir(dir, []string{"sample.go"})
		if err != nil {
			t.Fatalf("AnalyzeDir 失敗: %v", err)
		}

		if result.ExportedFuncs != 2 {
			t.Errorf("ExportedFuncs: got %d, want 2", result.ExportedFuncs)
		}
		if result.UnexportedFuncs != 1 {
			t.Errorf("UnexportedFuncs: got %d, want 1", result.UnexportedFuncs)
		}
		if result.ExportedMethods != 1 {
			t.Errorf("ExportedMethods: got %d, want 1", result.ExportedMethods)
		}
		if result.UnexportedMethods != 1 {
			t.Errorf("UnexportedMethods: got %d, want 1", result.UnexportedMethods)
		}
		if result.ExportedTypes != 2 {
			t.Errorf("ExportedTypes: got %d, want 2 (PublicType + PublicInterface)", result.ExportedTypes)
		}
		if result.UnexportedTypes != 1 {
			t.Errorf("UnexportedTypes: got %d, want 1", result.UnexportedTypes)
		}
		if result.InterfaceTypes != 1 {
			t.Errorf("InterfaceTypes: got %d, want 1 (PublicInterface)", result.InterfaceTypes)
		}
		if result.Files != 1 {
			t.Errorf("Files: got %d, want 1", result.Files)
		}
	})

	t.Run("型の宣言が無ければ Abstractness は 0", func(t *testing.T) {
		dir := t.TempDir()
		writeGoFile(t, dir, "funcs.go", "package funcs\n\nfunc F() {}\n")

		result, err := AnalyzeDir(dir, []string{"funcs.go"})
		if err != nil {
			t.Fatalf("AnalyzeDir 失敗: %v", err)
		}
		if got := result.Abstractness(); got != 0 {
			t.Errorf("Abstractness: got %v, want 0", got)
		}
	})

	t.Run("渡されたファイルだけを読む (go list の GoFiles にテストファイルは含まれない)", func(t *testing.T) {
		dir := t.TempDir()
		writeGoFile(t, dir, "lib.go", `package lib

func ExportedFunc() {}
func unexported() {}
`)
		writeGoFile(t, dir, "lib_test.go", `package lib

func TestExported(t *testing.T) {}
func helperFunc() {}
`)

		result, err := AnalyzeDir(dir, []string{"lib.go"})
		if err != nil {
			t.Fatalf("AnalyzeDir 失敗: %v", err)
		}

		if result.ExportedFuncs != 1 {
			t.Errorf("ExportedFuncs: got %d, want 1", result.ExportedFuncs)
		}
		if result.UnexportedFuncs != 1 {
			t.Errorf("UnexportedFuncs: got %d, want 1", result.UnexportedFuncs)
		}
		if result.Files != 1 {
			t.Errorf("Files: got %d, want 1", result.Files)
		}
	})

	t.Run("ファイルが無ければエラー", func(t *testing.T) {
		dir := t.TempDir()
		_, err := AnalyzeDir(dir, nil)
		if err == nil {
			t.Error("ファイルが無ければエラーを返すべき")
		}
	})

	t.Run("読めないファイルはエラー", func(t *testing.T) {
		dir := t.TempDir()
		_, err := AnalyzeDir(dir, []string{"missing.go"})
		if err == nil {
			t.Error("存在しないファイルではエラーを返すべき")
		}
	})
}

func TestPublicRatio(t *testing.T) {
	tests := []struct {
		name    string
		result  PackageResult
		wantMin float64
		wantMax float64
	}{
		{
			name: "半分が public",
			result: PackageResult{
				ExportedFuncs:     2,
				UnexportedFuncs:   2,
				ExportedMethods:   1,
				UnexportedMethods: 1,
				ExportedTypes:     1,
				UnexportedTypes:   1,
			},
			wantMin: 0.49,
			wantMax: 0.51,
		},
		{
			name: "全部 public",
			result: PackageResult{
				ExportedFuncs: 5,
				ExportedTypes: 3,
			},
			wantMin: 0.99,
			wantMax: 1.01,
		},
		{
			name: "全部 private",
			result: PackageResult{
				UnexportedFuncs: 5,
				UnexportedTypes: 3,
			},
			wantMin: -0.01,
			wantMax: 0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.PublicRatio()
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("PublicRatio() = %f, want between %f and %f", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestCalcStats(t *testing.T) {
	t.Run("正常な統計計算", func(t *testing.T) {
		values := []float64{100, 200, 300, 400, 500}
		stats := CalcStats(values)

		if stats.Mean != 300 {
			t.Errorf("Mean: got %f, want 300", stats.Mean)
		}
		// stddev of [100,200,300,400,500] = sqrt(20000) ≈ 141.42
		if stats.Stddev < 140 || stats.Stddev > 143 {
			t.Errorf("Stddev: got %f, want ~141.42", stats.Stddev)
		}
	})

	t.Run("空のスライス", func(t *testing.T) {
		stats := CalcStats(nil)
		if stats.Mean != 0 || stats.Stddev != 0 {
			t.Errorf("空スライスでは Mean=0, Stddev=0 を返すべき: got Mean=%f, Stddev=%f", stats.Mean, stats.Stddev)
		}
	})

	t.Run("要素1つ", func(t *testing.T) {
		stats := CalcStats([]float64{42})
		if stats.Mean != 42 {
			t.Errorf("Mean: got %f, want 42", stats.Mean)
		}
		if stats.Stddev != 0 {
			t.Errorf("Stddev: got %f, want 0", stats.Stddev)
		}
	})
}

func TestDetectOutliers(t *testing.T) {
	t.Run("外れ値の検出", func(t *testing.T) {
		// データポイントを増やして外れ値が統計を支配しないようにする
		packages := []PackageResult{
			{Path: "pkg/a", LOC: 100},
			{Path: "pkg/b", LOC: 120},
			{Path: "pkg/c", LOC: 110},
			{Path: "pkg/d", LOC: 105},
			{Path: "pkg/e", LOC: 115},
			{Path: "pkg/f", LOC: 108},
			{Path: "pkg/g", LOC: 112},
			{Path: "pkg/h", LOC: 103},
			{Path: "pkg/outlier", LOC: 2000}, // 外れ値
		}

		outliers := DetectOutliers(packages, 2.0)
		if len(outliers) != 1 {
			t.Fatalf("外れ値の数: got %d, want 1", len(outliers))
		}
		if outliers[0] != "pkg/outlier" {
			t.Errorf("外れ値パッケージ: got %s, want pkg/outlier", outliers[0])
		}
	})

	t.Run("外れ値なし", func(t *testing.T) {
		packages := []PackageResult{
			{Path: "pkg/a", LOC: 100},
			{Path: "pkg/b", LOC: 110},
			{Path: "pkg/c", LOC: 105},
		}

		outliers := DetectOutliers(packages, 2.0)
		if len(outliers) != 0 {
			t.Errorf("外れ値の数: got %d, want 0", len(outliers))
		}
	})
}

func TestTotalSymbols(t *testing.T) {
	tests := []struct {
		name   string
		result PackageResult
		want   int
	}{
		{
			name: "全フィールドの合計",
			result: PackageResult{
				ExportedFuncs:     2,
				UnexportedFuncs:   3,
				ExportedMethods:   1,
				UnexportedMethods: 2,
				ExportedTypes:     1,
				UnexportedTypes:   1,
			},
			want: 10,
		},
		{
			name:   "ゼロ値",
			result: PackageResult{},
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.TotalSymbols()
			if got != tt.want {
				t.Errorf("TotalSymbols() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestGenerateWarnings(t *testing.T) {
	t.Run("public ratio 超過の警告 (シンボル数十分)", func(t *testing.T) {
		pkg := PackageResult{
			Path:            "pkg/api",
			ExportedFuncs:   8,
			UnexportedFuncs: 2,
			ExportedTypes:   5,
			UnexportedTypes: 1,
		}

		warnings := GenerateWarnings(pkg, 0.6, 2.0, Stats{Mean: 100, Stddev: 50}, defaultMinSymbols)
		found := false
		for _, w := range warnings {
			if w.Type == "high_public_ratio" {
				found = true
			}
		}
		if !found {
			t.Error("public ratio 超過の警告が生成されていない")
		}
	})

	t.Run("小規模パッケージは public ratio チェックをスキップ", func(t *testing.T) {
		// シンボル数 6 (< defaultMinSymbols=10) → public ratio 1.0 でも警告なし
		pkg := PackageResult{
			Path:          "internal/category",
			ExportedFuncs: 3,
			ExportedTypes: 3,
		}

		warnings := GenerateWarnings(pkg, 0.6, 2.0, Stats{Mean: 100, Stddev: 50}, defaultMinSymbols)
		for _, w := range warnings {
			if w.Type == "high_public_ratio" {
				t.Error("小規模パッケージ (シンボル数 < 10) では high_public_ratio 警告を生成すべきでない")
			}
		}
	})

	t.Run("シンボル数ちょうど閾値で public ratio チェックが有効", func(t *testing.T) {
		// シンボル数 10 (= defaultMinSymbols) → チェック有効
		pkg := PackageResult{
			Path:            "pkg/medium",
			ExportedFuncs:   8,
			UnexportedFuncs: 1,
			ExportedTypes:   1,
		}

		warnings := GenerateWarnings(pkg, 0.6, 2.0, Stats{Mean: 100, Stddev: 50}, defaultMinSymbols)
		found := false
		for _, w := range warnings {
			if w.Type == "high_public_ratio" {
				found = true
			}
		}
		if !found {
			t.Error("シンボル数 >= 10 では public ratio チェックが有効であるべき")
		}
	})

	t.Run("小規模パッケージでも LOC outlier は検出する", func(t *testing.T) {
		// シンボル数 3 (< defaultMinSymbols) でも LOC 外れ値は検出
		pkg := PackageResult{
			Path:          "internal/small",
			LOC:           1500,
			ExportedFuncs: 3,
		}

		warnings := GenerateWarnings(pkg, 0.6, 2.0, Stats{Mean: 100, Stddev: 50}, defaultMinSymbols)
		found := false
		for _, w := range warnings {
			if w.Type == "loc_outlier" {
				found = true
			}
		}
		if !found {
			t.Error("小規模パッケージでも LOC 外れ値の警告は生成されるべき")
		}
	})

	t.Run("警告なし (しきい値以内)", func(t *testing.T) {
		pkg := PackageResult{
			Path:            "pkg/ok",
			LOC:             100,
			ExportedFuncs:   3,
			UnexportedFuncs: 7,
		}

		warnings := GenerateWarnings(pkg, 0.6, 2.0, Stats{Mean: 100, Stddev: 50}, defaultMinSymbols)
		if len(warnings) != 0 {
			t.Errorf("警告数: got %d, want 0", len(warnings))
		}
	})

	t.Run("LOC 外れ値の警告", func(t *testing.T) {
		pkg := PackageResult{
			Path:            "pkg/big",
			LOC:             1500,
			ExportedFuncs:   1,
			UnexportedFuncs: 5,
		}

		warnings := GenerateWarnings(pkg, 0.6, 2.0, Stats{Mean: 100, Stddev: 50}, defaultMinSymbols)
		found := false
		for _, w := range warnings {
			if w.Type == "loc_outlier" {
				found = true
			}
		}
		if !found {
			t.Error("LOC 外れ値の警告が生成されていない (1500 > max(100 + 2*50, minLOCLimit) = 1000)")
		}
	})

	t.Run("loc_outlier は actionable、high_public_ratio は参考情報", func(t *testing.T) {
		pkg := PackageResult{
			Path:          "pkg/big-api",
			LOC:           1500,
			ExportedFuncs: 10,
		}

		warnings := GenerateWarnings(pkg, 0.6, 2.0, Stats{Mean: 100, Stddev: 50}, defaultMinSymbols)
		if len(warnings) != 2 {
			t.Fatalf("警告数: got %d, want 2 (high_public_ratio, loc_outlier)", len(warnings))
		}
		want := map[string]bool{WarningTypeHighPublicRatio: false, WarningTypeLocOutlier: true}
		for _, w := range warnings {
			if w.Actionable != want[w.Type] {
				t.Errorf("%s: actionable=%v, want %v", w.Type, w.Actionable, want[w.Type])
			}
		}
	})
}

func TestGenerateExportedReferenceWarnings(t *testing.T) {
	t.Run("internal の未参照は actionable で、シンボル名を並べる", func(t *testing.T) {
		pkg := PackageResult{
			Path:                        "internal/edge",
			internal:                    true,
			UnreferencedExported:        new(2),
			UnreferencedExportedSymbols: []string{"Prune", "Walker.Skip"},
			TestOnlyExported:            new(0),
		}

		warnings := GenerateExportedReferenceWarnings(pkg)
		if len(warnings) != 1 {
			t.Fatalf("警告数: got %d, want 1", len(warnings))
		}
		w := warnings[0]
		if w.Type != WarningTypeUnreferencedExported || !w.Actionable || w.Value != 2 {
			t.Errorf("got %+v, want unreferenced_exported, actionable, value 2", w)
		}
		for _, name := range []string{"Prune", "Walker.Skip"} {
			if !strings.Contains(w.Message, name) {
				t.Errorf("Message に %s が無い: %s", name, w.Message)
			}
		}
	})

	t.Run("テストからだけ参照される exported も数え、種類を分けて書く", func(t *testing.T) {
		pkg := PackageResult{
			Path:                    "internal/edge",
			internal:                true,
			UnreferencedExported:    new(0),
			TestOnlyExported:        new(1),
			TestOnlyExportedSymbols: []string{"Fixture"},
		}

		warnings := GenerateExportedReferenceWarnings(pkg)
		if len(warnings) != 1 || !warnings[0].Actionable {
			t.Fatalf("got %+v, want actionable な警告 1 件", warnings)
		}
		if !strings.Contains(warnings[0].Message, "テストからだけ") || !strings.Contains(warnings[0].Message, "Fixture") {
			t.Errorf("Message にテストだけの分が無い: %s", warnings[0].Message)
		}
	})

	t.Run("internal でなければ報告だけで actionable にしない", func(t *testing.T) {
		pkg := PackageResult{
			Path:                        "pkg/api",
			UnreferencedExported:        new(1),
			UnreferencedExportedSymbols: []string{"Client"},
			TestOnlyExported:            new(0),
		}

		warnings := GenerateExportedReferenceWarnings(pkg)
		if len(warnings) != 1 || warnings[0].Actionable {
			t.Fatalf("got %+v, want actionable=false の警告 1 件", warnings)
		}
	})

	t.Run("すべて参照されていれば警告なし", func(t *testing.T) {
		pkg := PackageResult{Path: "internal/edge", internal: true, UnreferencedExported: new(0), TestOnlyExported: new(0)}
		if warnings := GenerateExportedReferenceWarnings(pkg); len(warnings) != 0 {
			t.Errorf("got %+v, want 0 件", warnings)
		}
	})

	t.Run("判定しないパッケージ (main) は警告なし", func(t *testing.T) {
		if warnings := GenerateExportedReferenceWarnings(PackageResult{Path: "cmd/app"}); len(warnings) != 0 {
			t.Errorf("got %+v, want 0 件", warnings)
		}
	})
}

func TestGenerateMainSequenceWarnings(t *testing.T) {
	t.Run("MainSequence=nil → 空", func(t *testing.T) {
		pkg := PackageResult{Path: "pkg/none"}
		warnings := GenerateMainSequenceWarnings(pkg)
		if len(warnings) != 0 {
			t.Errorf("MainSequence nil で警告数: got %d, want 0", len(warnings))
		}
	})

	t.Run("zone_of_pain → 警告", func(t *testing.T) {
		pkg := PackageResult{
			Path: "pkg/pain",
			MainSequence: &MainSequenceData{
				Zone:         ZoneOfPain,
				Distance:     0.8,
				Abstractness: 0.1,
				Instability:  0.1,
			},
		}
		warnings := GenerateMainSequenceWarnings(pkg)
		found := false
		for _, w := range warnings {
			if w.Type == "zone_of_pain" {
				found = true
			}
		}
		if !found {
			t.Error("zone_of_pain 警告が生成されていない")
		}
	})

	t.Run("zone_of_uselessness → 警告", func(t *testing.T) {
		pkg := PackageResult{
			Path: "pkg/useless",
			MainSequence: &MainSequenceData{
				Zone:         ZoneOfUselessness,
				Distance:     0.6,
				Abstractness: 0.8,
				Instability:  0.8,
			},
		}
		warnings := GenerateMainSequenceWarnings(pkg)
		found := false
		for _, w := range warnings {
			if w.Type == "zone_of_uselessness" {
				found = true
			}
		}
		if !found {
			t.Error("zone_of_uselessness 警告が生成されていない")
		}
	})

	t.Run("excluded → 空", func(t *testing.T) {
		pkg := PackageResult{
			Path:         "main",
			MainSequence: &MainSequenceData{Zone: ZoneExcluded},
		}
		warnings := GenerateMainSequenceWarnings(pkg)
		if len(warnings) != 0 {
			t.Errorf("excluded で警告数: got %d, want 0", len(warnings))
		}
	})

	t.Run("構造制約 → 情報警告", func(t *testing.T) {
		pkg := PackageResult{
			Path: "pkg/leaf",
			MainSequence: &MainSequenceData{
				Zone:                    ZoneOfPain,
				Distance:                1.0,
				StructurallyConstrained: true,
				AfferentCoupling:        5,
				EfferentCoupling:        0,
			},
		}
		warnings := GenerateMainSequenceWarnings(pkg)
		found := false
		for _, w := range warnings {
			if w.Type == "structurally_constrained" {
				found = true
			}
		}
		if !found {
			t.Error("structurally_constrained 警告が生成されていない")
		}
	})

	t.Run("構造制約の zone_of_pain と structurally_constrained は actionable=false", func(t *testing.T) {
		pkg := PackageResult{
			Path: "pkg/leaf",
			MainSequence: &MainSequenceData{
				Zone:                    ZoneOfPain,
				Distance:                1.0,
				StructurallyConstrained: true,
				AfferentCoupling:        5,
			},
		}
		warnings := GenerateMainSequenceWarnings(pkg)
		if len(warnings) != 2 {
			t.Fatalf("警告数: got %d, want 2 (zone_of_pain, structurally_constrained)", len(warnings))
		}
		for _, w := range warnings {
			if w.Actionable {
				t.Errorf("%s: actionable=true, want false (トポロジー由来で直す手が無い)", w.Type)
			}
			if !strings.Contains(w.Message, "--strict") {
				t.Errorf("%s: Message に --strict で数えない旨が無い: %q", w.Type, w.Message)
			}
		}
	})

	t.Run("非揮発の zone_of_pain は actionable=false で、high_distance は出さない", func(t *testing.T) {
		count := 1
		pkg := PackageResult{
			Path: "pkg/cold",
			MainSequence: &MainSequenceData{
				Zone: ZoneOfPain, Distance: 0.75, Abstractness: 0, Instability: 0.25,
				AfferentCoupling: 3, EfferentCoupling: 1,
				Volatility: VolatilityNonVolatile, RecentCommitCount: &count,
			},
		}
		warnings := GenerateMainSequenceWarnings(pkg)
		if len(warnings) != 1 || warnings[0].Type != WarningTypeZoneOfPain {
			t.Fatalf("警告: %+v, want zone_of_pain だけ", warnings)
		}
		if warnings[0].Actionable {
			t.Error("非揮発の zone_of_pain: actionable=true, want false")
		}
		if !strings.Contains(warnings[0].Message, "非揮発") || !strings.Contains(warnings[0].Message, "--strict") {
			t.Errorf("Message に非揮発と --strict で数えない旨が無い: %q", warnings[0].Message)
		}
	})

	t.Run("揮発または未測定の zone_of_pain は actionable=true", func(t *testing.T) {
		for _, v := range []Volatility{VolatilityVolatile, VolatilityUnmeasured} {
			pkg := PackageResult{
				Path: "pkg/hot",
				MainSequence: &MainSequenceData{
					Zone: ZoneOfPain, Distance: 0.75, Abstractness: 0, Instability: 0.25,
					AfferentCoupling: 3, EfferentCoupling: 1, Volatility: v,
				},
			}
			warnings := GenerateMainSequenceWarnings(pkg)
			if len(warnings) != 2 {
				t.Fatalf("%s: 警告数 %d, want 2 (zone_of_pain, high_distance)", v, len(warnings))
			}
			for _, w := range warnings {
				if !w.Actionable {
					t.Errorf("%s %s: actionable=false, want true", v, w.Type)
				}
			}
		}
	})

	t.Run("非揮発でも zone_of_uselessness は actionable=true", func(t *testing.T) {
		pkg := PackageResult{
			Path: "pkg/abstract",
			MainSequence: &MainSequenceData{
				Zone: ZoneOfUselessness, Distance: 0.6, Abstractness: 0.8, Instability: 0.8,
				Volatility: VolatilityNonVolatile,
			},
		}
		for _, w := range GenerateMainSequenceWarnings(pkg) {
			if !w.Actionable {
				t.Errorf("%s: actionable=false, want true (使われていない抽象は変更頻度と関係ない)", w.Type)
			}
		}
	})

	t.Run("直せる Main Sequence の警告は actionable=true", func(t *testing.T) {
		pkgs := []PackageResult{
			{Path: "pkg/pain", MainSequence: &MainSequenceData{Zone: ZoneOfPain, Distance: 0.8, Abstractness: 0.1, Instability: 0.1}},
			{Path: "pkg/useless", MainSequence: &MainSequenceData{Zone: ZoneOfUselessness, Distance: 0.6, Abstractness: 0.8, Instability: 0.8}},
		}
		for _, pkg := range pkgs {
			warnings := GenerateMainSequenceWarnings(pkg)
			if len(warnings) == 0 {
				t.Fatalf("%s: 警告が生成されていない", pkg.Path)
			}
			for _, w := range warnings {
				if !w.Actionable {
					t.Errorf("%s %s: actionable=false, want true", pkg.Path, w.Type)
				}
			}
		}
	})

	t.Run("high_distance (非構造制約, D > 0.5) → 警告", func(t *testing.T) {
		pkg := PackageResult{
			Path: "pkg/far",
			MainSequence: &MainSequenceData{
				Zone:                    ZoneMainSequence,
				Distance:                0.7,
				StructurallyConstrained: false,
			},
		}
		warnings := GenerateMainSequenceWarnings(pkg)
		found := false
		for _, w := range warnings {
			if w.Type == "high_distance" {
				found = true
			}
		}
		if !found {
			t.Error("high_distance 警告が生成されていない")
		}
	})

	t.Run("main_sequence, D <= 0.5 → high_distance なし", func(t *testing.T) {
		pkg := PackageResult{
			Path: "pkg/close",
			MainSequence: &MainSequenceData{
				Zone:     ZoneMainSequence,
				Distance: 0.3,
			},
		}
		warnings := GenerateMainSequenceWarnings(pkg)
		for _, w := range warnings {
			if w.Type == "high_distance" {
				t.Error("D <= 0.5 では high_distance 警告を生成すべきでない")
			}
		}
	})
}

func TestGenerateCrossWarnings(t *testing.T) {
	t.Run("構造制約 + loc_outlier → constrained_loc_outlier", func(t *testing.T) {
		pkg := PackageResult{
			Path: "pkg/big-leaf",
			MainSequence: &MainSequenceData{
				Zone:                    ZoneOfPain,
				StructurallyConstrained: true,
			},
		}
		astWarnings := []Warning{
			{Type: "loc_outlier", Package: "pkg/big-leaf", Value: 500, Limit: 200},
		}
		warnings := GenerateCrossWarnings(pkg, astWarnings)
		if len(warnings) != 1 {
			t.Fatalf("警告数: got %d, want 1", len(warnings))
		}
		if warnings[0].Type != "constrained_loc_outlier" {
			t.Errorf("Type: got %q, want %q", warnings[0].Type, "constrained_loc_outlier")
		}
		// 肥大化した葉は分割で直せる
		if !warnings[0].Actionable {
			t.Error("constrained_loc_outlier: actionable=false, want true")
		}
	})

	t.Run("構造制約のみ (loc_outlier なし) → 空", func(t *testing.T) {
		pkg := PackageResult{
			Path: "pkg/small-leaf",
			MainSequence: &MainSequenceData{
				Zone:                    ZoneOfPain,
				StructurallyConstrained: true,
			},
		}
		warnings := GenerateCrossWarnings(pkg, nil)
		if len(warnings) != 0 {
			t.Errorf("警告数: got %d, want 0", len(warnings))
		}
	})

	t.Run("loc_outlier のみ (構造制約なし) → 空", func(t *testing.T) {
		pkg := PackageResult{
			Path: "pkg/big-normal",
			MainSequence: &MainSequenceData{
				Zone:                    ZoneMainSequence,
				StructurallyConstrained: false,
			},
		}
		astWarnings := []Warning{
			{Type: "loc_outlier", Package: "pkg/big-normal", Value: 500, Limit: 200},
		}
		warnings := GenerateCrossWarnings(pkg, astWarnings)
		if len(warnings) != 0 {
			t.Errorf("警告数: got %d, want 0", len(warnings))
		}
	})

	t.Run("MainSequence=nil → 空", func(t *testing.T) {
		pkg := PackageResult{Path: "pkg/none"}
		astWarnings := []Warning{
			{Type: "loc_outlier", Package: "pkg/none", Value: 500, Limit: 200},
		}
		warnings := GenerateCrossWarnings(pkg, astWarnings)
		if len(warnings) != 0 {
			t.Errorf("警告数: got %d, want 0", len(warnings))
		}
	})
}

// loc_outlier は相対指標なので、パッケージを細かく分割すると平均が下がり、
// それにつれて閾値も下がる｡結果、1 行も変わっていない無関係なパッケージが
// 突然「外れ値」として現れる (実際にこれを踏んだ)｡minLOCLimit はその巻き込みを
// 止めるための下限で、この振る舞いを固定しておく｡
func TestGenerateWarnings_LOC閾値には下限がある(t *testing.T) {
	// 分割が進んで平均・分散が小さくなった状況を模す。
	// 統計だけなら閾値は 100 + 2*50 = 200 になるが、下限 1000 が優先される。
	tightStats := Stats{Mean: 100, Stddev: 50}

	t.Run("統計上は外れ値でも下限未満なら警告しない", func(t *testing.T) {
		pkg := PackageResult{Path: "pkg/unrelated", LOC: 899, ExportedFuncs: 1, UnexportedFuncs: 5}

		for _, w := range GenerateWarnings(pkg, 0.6, 2.0, tightStats, defaultMinSymbols) {
			if w.Type == WarningTypeLocOutlier {
				t.Errorf("LOC %d は下限 %d 未満なので警告すべきでない (limit=%v)", pkg.LOC, minLOCLimit, w.Limit)
			}
		}
	})

	t.Run("下限を超えていれば警告する", func(t *testing.T) {
		pkg := PackageResult{Path: "pkg/huge", LOC: minLOCLimit + 1, ExportedFuncs: 1, UnexportedFuncs: 5}

		found := false
		for _, w := range GenerateWarnings(pkg, 0.6, 2.0, tightStats, defaultMinSymbols) {
			if w.Type == WarningTypeLocOutlier {
				found = true
				if w.Limit != minLOCLimit {
					t.Errorf("limit = %v, want %d (統計値より下限が優先される)", w.Limit, minLOCLimit)
				}
			}
		}
		if !found {
			t.Errorf("LOC %d は下限を超えているので警告すべき", pkg.LOC)
		}
	})

	t.Run("統計値が下限より大きければ統計値が使われる", func(t *testing.T) {
		wideStats := Stats{Mean: 2000, Stddev: 500} // 閾値 3000
		pkg := PackageResult{Path: "pkg/mid", LOC: 2500, ExportedFuncs: 1, UnexportedFuncs: 5}

		for _, w := range GenerateWarnings(pkg, 0.6, 2.0, wideStats, defaultMinSymbols) {
			if w.Type == WarningTypeLocOutlier {
				t.Errorf("LOC %d は統計閾値 3000 未満なので警告すべきでない (limit=%v)", pkg.LOC, w.Limit)
			}
		}
	})
}
