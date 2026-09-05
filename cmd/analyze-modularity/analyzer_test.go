package main

import (
	"os"
	"path/filepath"
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

		result, err := AnalyzeDir(dir)
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
		if result.Files != 1 {
			t.Errorf("Files: got %d, want 1", result.Files)
		}
	})

	t.Run("テストファイルを除外する", func(t *testing.T) {
		dir := t.TempDir()
		writeGoFile(t, dir, "lib.go", `package lib

func ExportedFunc() {}
func unexported() {}
`)
		writeGoFile(t, dir, "lib_test.go", `package lib

func TestExported(t *testing.T) {}
func helperFunc() {}
`)

		result, err := AnalyzeDir(dir)
		if err != nil {
			t.Fatalf("AnalyzeDir 失敗: %v", err)
		}

		// テストファイルの関数はカウントされないこと
		if result.ExportedFuncs != 1 {
			t.Errorf("ExportedFuncs: got %d, want 1 (テストファイル除外)", result.ExportedFuncs)
		}
		if result.UnexportedFuncs != 1 {
			t.Errorf("UnexportedFuncs: got %d, want 1 (テストファイル除外)", result.UnexportedFuncs)
		}
	})

	t.Run("空ディレクトリ", func(t *testing.T) {
		dir := t.TempDir()
		_, err := AnalyzeDir(dir)
		if err == nil {
			t.Error("空ディレクトリではエラーを返すべき")
		}
	})

	t.Run("権限エラーのディレクトリ", func(t *testing.T) {
		dir := t.TempDir()
		err := os.Chmod(dir, 0000)
		if err != nil {
			t.Skipf("chmod 0000 に失敗 (root ユーザーの可能性): %v", err)
		}
		t.Cleanup(func() { os.Chmod(dir, 0755) })

		_, err = AnalyzeDir(dir)
		if err == nil {
			t.Error("権限エラーのディレクトリではエラーを返すべき")
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
}

func TestGenerateSPMWarnings(t *testing.T) {
	t.Run("SPM=nil → 空", func(t *testing.T) {
		pkg := PackageResult{Path: "pkg/nospm"}
		warnings := GenerateSPMWarnings(pkg)
		if len(warnings) != 0 {
			t.Errorf("SPM nil で警告数: got %d, want 0", len(warnings))
		}
	})

	t.Run("zone_of_pain → 警告", func(t *testing.T) {
		pkg := PackageResult{
			Path: "pkg/pain",
			SPM: &SPMData{
				Zone:         SPMZoneOfPain,
				Distance:     0.8,
				Abstractness: 0.1,
				Instability:  0.1,
			},
		}
		warnings := GenerateSPMWarnings(pkg)
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
			SPM: &SPMData{
				Zone:         SPMZoneOfUselessness,
				Distance:     0.6,
				Abstractness: 0.8,
				Instability:  0.8,
			},
		}
		warnings := GenerateSPMWarnings(pkg)
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
			Path: "main",
			SPM:  &SPMData{Zone: SPMZoneExcluded},
		}
		warnings := GenerateSPMWarnings(pkg)
		if len(warnings) != 0 {
			t.Errorf("excluded で警告数: got %d, want 0", len(warnings))
		}
	})

	t.Run("構造制約 → 情報警告", func(t *testing.T) {
		pkg := PackageResult{
			Path: "pkg/leaf",
			SPM: &SPMData{
				Zone:                    SPMZoneOfPain,
				Distance:                1.0,
				StructurallyConstrained: true,
				AfferentCoupling:        5,
				EfferentCoupling:        0,
			},
		}
		warnings := GenerateSPMWarnings(pkg)
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

	t.Run("high_distance (非構造制約, D > 0.5) → 警告", func(t *testing.T) {
		pkg := PackageResult{
			Path: "pkg/far",
			SPM: &SPMData{
				Zone:                    SPMZoneMainSequence,
				Distance:                0.7,
				StructurallyConstrained: false,
			},
		}
		warnings := GenerateSPMWarnings(pkg)
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
			SPM: &SPMData{
				Zone:     SPMZoneMainSequence,
				Distance: 0.3,
			},
		}
		warnings := GenerateSPMWarnings(pkg)
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
			SPM: &SPMData{
				Zone:                    SPMZoneOfPain,
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
	})

	t.Run("構造制約のみ (loc_outlier なし) → 空", func(t *testing.T) {
		pkg := PackageResult{
			Path: "pkg/small-leaf",
			SPM: &SPMData{
				Zone:                    SPMZoneOfPain,
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
			SPM: &SPMData{
				Zone:                    SPMZoneMainSequence,
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

	t.Run("SPM=nil → 空", func(t *testing.T) {
		pkg := PackageResult{Path: "pkg/nospm"}
		astWarnings := []Warning{
			{Type: "loc_outlier", Package: "pkg/nospm", Value: 500, Limit: 200},
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
