package main

import (
	"os"
	"path/filepath"
	"testing"
)

func Test_classifyZone(t *testing.T) {
	tests := []struct {
		name string
		pkg  SPMPackage
		want SPMZone
	}{
		{
			name: "main パッケージ → excluded",
			pkg:  SPMPackage{Name: "main", Abstractness: 0.0, Instability: 0.0},
			want: SPMZoneExcluded,
		},
		{
			name: "cmd/main パッケージ → excluded",
			pkg:  SPMPackage{Name: "cmd/main", Abstractness: 0.1, Instability: 0.1},
			want: SPMZoneExcluded,
		},
		{
			name: "孤立パッケージ (Ca=0, Ce=0) → excluded",
			pkg:  SPMPackage{Name: "pkg/isolated", AfferentCoupling: 0, EfferentCoupling: 0},
			want: SPMZoneExcluded,
		},
		{
			name: "A < 0.5, I < 0.5 → zone_of_pain",
			pkg:  SPMPackage{Name: "pkg/stable", Abstractness: 0.1, Instability: 0.1, AfferentCoupling: 5, EfferentCoupling: 1},
			want: SPMZoneOfPain,
		},
		{
			name: "A > 0.5, I > 0.5 → zone_of_uselessness",
			pkg:  SPMPackage{Name: "pkg/abstract", Abstractness: 0.8, Instability: 0.8, AfferentCoupling: 1, EfferentCoupling: 5},
			want: SPMZoneOfUselessness,
		},
		{
			name: "A=0.1, I=0.9 → main_sequence",
			pkg:  SPMPackage{Name: "pkg/normal", Abstractness: 0.1, Instability: 0.9, AfferentCoupling: 1, EfferentCoupling: 5},
			want: SPMZoneMainSequence,
		},
		{
			name: "境界値 A=0.5, I=0.5 → main_sequence",
			pkg:  SPMPackage{Name: "pkg/boundary", Abstractness: 0.5, Instability: 0.5, AfferentCoupling: 3, EfferentCoupling: 3},
			want: SPMZoneMainSequence,
		},
		{
			name: "A=0.5, I=0.3 → main_sequence (A が境界値で zone_of_pain にならない)",
			pkg:  SPMPackage{Name: "pkg/edge1", Abstractness: 0.5, Instability: 0.3, AfferentCoupling: 3, EfferentCoupling: 1},
			want: SPMZoneMainSequence,
		},
		{
			name: "A=0.3, I=0.5 → main_sequence (I が境界値で zone_of_pain にならない)",
			pkg:  SPMPackage{Name: "pkg/edge2", Abstractness: 0.3, Instability: 0.5, AfferentCoupling: 1, EfferentCoupling: 3},
			want: SPMZoneMainSequence,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyZone(tt.pkg)
			if got != tt.want {
				t.Errorf("classifyZone(%+v) = %q, want %q", tt.pkg, got, tt.want)
			}
		})
	}
}

func Test_isStructurallyConstrained(t *testing.T) {
	tests := []struct {
		name string
		pkg  SPMPackage
		want bool
	}{
		{
			name: "Ce=0, Ca=5 → 構造制約あり",
			pkg:  SPMPackage{Name: "pkg/leaf", EfferentCoupling: 0, AfferentCoupling: 5},
			want: true,
		},
		{
			name: "Ce=0, Ca=0 → 孤立 (構造制約なし)",
			pkg:  SPMPackage{Name: "pkg/isolated", EfferentCoupling: 0, AfferentCoupling: 0},
			want: false,
		},
		{
			name: "Ce=3, Ca=5 → 構造制約なし",
			pkg:  SPMPackage{Name: "pkg/normal", EfferentCoupling: 3, AfferentCoupling: 5},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStructurallyConstrained(tt.pkg)
			if got != tt.want {
				t.Errorf("isStructurallyConstrained(%+v) = %v, want %v", tt.pkg, got, tt.want)
			}
		})
	}
}

func Test_loadSPMReport(t *testing.T) {
	t.Run("正常な JSON", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "spm.json")
		content := `{
  "packages": [
    {"name": "pkg/a", "afferent_coupling": 3, "efferent_coupling": 2, "abstractness": 0.1, "instability": 0.4, "distance": 0.5},
    {"name": "pkg/b", "afferent_coupling": 0, "efferent_coupling": 5, "abstractness": 0.0, "instability": 1.0, "distance": 0.0}
  ]
}`
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("テストファイル作成失敗: %v", err)
		}

		report, err := loadSPMReport(path)
		if err != nil {
			t.Fatalf("loadSPMReport 失敗: %v", err)
		}
		if len(report.Packages) != 2 {
			t.Errorf("パッケージ数: got %d, want 2", len(report.Packages))
		}
		if report.Packages[0].Name != "pkg/a" {
			t.Errorf("Name: got %q, want %q", report.Packages[0].Name, "pkg/a")
		}
		if report.Packages[0].AfferentCoupling != 3 {
			t.Errorf("AfferentCoupling: got %d, want 3", report.Packages[0].AfferentCoupling)
		}
	})

	t.Run("不正な JSON → error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.json")
		if err := os.WriteFile(path, []byte("{invalid json"), 0644); err != nil {
			t.Fatalf("テストファイル作成失敗: %v", err)
		}

		_, err := loadSPMReport(path)
		if err == nil {
			t.Error("不正な JSON ではエラーを返すべき")
		}
	})

	t.Run("存在しないファイル → error", func(t *testing.T) {
		_, err := loadSPMReport("/nonexistent/path/spm.json")
		if err == nil {
			t.Error("存在しないファイルではエラーを返すべき")
		}
	})
}

func Test_buildSPMIndex(t *testing.T) {
	report := &SPMReport{
		Packages: []SPMPackage{
			{Name: "main", Path: "example.com/mod/cmd/app", Distance: 0.5},
			{Name: "main", Path: "example.com/mod/cmd/cli", Distance: 0.3},
			{Name: "pkg", Path: "example.com/mod/internal/pkg", Distance: 0.1},
		},
	}

	index := buildSPMIndex(report)

	// Path でインデックスされていること (Name が重複していても全て残る)
	if len(index) != 3 {
		t.Fatalf("インデックスサイズ: got %d, want 3", len(index))
	}
	if _, ok := index["example.com/mod/cmd/app"]; !ok {
		t.Error("キー example.com/mod/cmd/app がインデックスに存在しない")
	}
	if _, ok := index["example.com/mod/cmd/cli"]; !ok {
		t.Error("キー example.com/mod/cmd/cli がインデックスに存在しない")
	}
	if _, ok := index["example.com/mod/internal/pkg"]; !ok {
		t.Error("キー example.com/mod/internal/pkg がインデックスに存在しない")
	}
	// Name ("main") で引くとマッチしないこと
	if _, ok := index["main"]; ok {
		t.Error("Name でマッチすべきでない")
	}
}

func Test_readModuleName(t *testing.T) {
	t.Run("正常な go.mod → モジュール名取得", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "go.mod")
		if err := os.WriteFile(path, []byte("module example.com/mymod\n\ngo 1.21\n"), 0644); err != nil {
			t.Fatalf("go.mod 作成失敗: %v", err)
		}

		got, err := readModuleName(path)
		if err != nil {
			t.Fatalf("readModuleName 失敗: %v", err)
		}
		if got != "example.com/mymod" {
			t.Errorf("readModuleName = %q, want %q", got, "example.com/mymod")
		}
	})

	t.Run("コメント付き go.mod → module 行を正しく抽出", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "go.mod")
		content := "// some comment\nmodule example.com/commented\n\ngo 1.21\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("go.mod 作成失敗: %v", err)
		}

		got, err := readModuleName(path)
		if err != nil {
			t.Fatalf("readModuleName 失敗: %v", err)
		}
		if got != "example.com/commented" {
			t.Errorf("readModuleName = %q, want %q", got, "example.com/commented")
		}
	})

	t.Run("module 行なし → エラー", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "go.mod")
		if err := os.WriteFile(path, []byte("go 1.21\n"), 0644); err != nil {
			t.Fatalf("go.mod 作成失敗: %v", err)
		}

		_, err := readModuleName(path)
		if err == nil {
			t.Error("module 行がない場合はエラーを返すべき")
		}
	})

	t.Run("存在しないファイル → エラー", func(t *testing.T) {
		_, err := readModuleName("/nonexistent/go.mod")
		if err == nil {
			t.Error("存在しないファイルではエラーを返すべき")
		}
	})
}
