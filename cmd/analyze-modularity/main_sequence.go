package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// PackageMetrics は Robert C. Martin の Package Metrics (Ca/Ce/A/I/D) の 1 パッケージ分｡
type PackageMetrics struct {
	Name             string
	Path             string
	AfferentCoupling int
	EfferentCoupling int
	Abstractness     float64
	Instability      float64
	Distance         float64
}

// Zone は Martin の Main Sequence に基づくパッケージの Zone 分類｡
type Zone string

const (
	ZoneExcluded      Zone = "excluded"
	ZoneOfPain        Zone = "zone_of_pain"
	ZoneOfUselessness Zone = "zone_of_uselessness"
	ZoneMainSequence  Zone = "main_sequence"
)

// classifyZone はパッケージの Zone を分類する｡
// Martin (Agile PPP ch.28) の volatility caveat に基づく:
//
//	main パッケージ or 孤立 (Ca=0, Ce=0) → excluded
//	A < 0.5 かつ I < 0.5 → zone_of_pain
//	A > 0.5 かつ I > 0.5 → zone_of_uselessness
//	上記以外 → main_sequence
func classifyZone(pkg PackageMetrics) Zone {
	if pkg.Name == "main" {
		return ZoneExcluded
	}
	if pkg.AfferentCoupling == 0 && pkg.EfferentCoupling == 0 {
		return ZoneExcluded
	}
	if pkg.Abstractness < 0.5 && pkg.Instability < 0.5 {
		return ZoneOfPain
	}
	if pkg.Abstractness > 0.5 && pkg.Instability > 0.5 {
		return ZoneOfUselessness
	}
	return ZoneMainSequence
}

// isStructurallyConstrained は Ce=0 かつ Ca>0 の構造制約を判定する｡
// I=0 がトポロジーにより強制されている状態を示す｡
func isStructurallyConstrained(pkg PackageMetrics) bool {
	return pkg.EfferentCoupling == 0 && pkg.AfferentCoupling > 0
}

// instability は I = Ce / (Ca + Ce) を返す｡
// 孤立パッケージ (Ca = Ce = 0) は依存元が無く、変えても何も壊れないので最大限に不安定とみなして 1 を返す｡
func instability(ca, ce int) float64 {
	if ca+ce == 0 {
		return 1
	}
	return float64(ce) / float64(ca+ce)
}

// distance は Main Sequence からの距離 D = |A + I - 1| を返す｡
func distance(a, i float64) float64 {
	return math.Abs(a + i - 1)
}

// listedPackage は go list -json の出力のうち使うフィールド｡
type listedPackage struct {
	ImportPath string
	Name       string
	Dir        string
	GoFiles    []string
}

// listModulePackages は moduleRoot で go list -json ./... を実行し、モジュール内の全パッケージを返す｡
// 入れ子のモジュールと vendor は go list が除く｡GoFiles はテストファイルを含まない｡
func listModulePackages(moduleRoot string) ([]listedPackage, error) {
	cmd := exec.Command("go", "list", "-json=ImportPath,Name,Dir,GoFiles", "./...")
	cmd.Dir = moduleRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list の実行に失敗 (%s): %w\n%s", moduleRoot, err, bytes.TrimSpace(stderr.Bytes()))
	}

	dec := json.NewDecoder(bytes.NewReader(out))
	var pkgs []listedPackage
	for {
		var p listedPackage
		if err := dec.Decode(&p); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("go list の出力のパースに失敗: %w", err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// gitIgnoredDirs は dirs のうち git が ignore するものを、実体の絶対パスの集合で返す｡
// moduleRoot が git リポジトリでなければ ignore するものは無いとみなし、stderr にその旨を出す｡
func gitIgnoredDirs(moduleRoot string, dirs []string) (map[string]bool, error) {
	ignored := make(map[string]bool)
	if len(dirs) == 0 {
		return ignored, nil
	}

	// git が無いのは前提の欠如なので止める。git リポジトリでないのは正常な状態として続ける
	probe := exec.Command("git", "rev-parse", "--show-toplevel")
	probe.Dir = moduleRoot
	if err := probe.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("git が PATH に見つかりません: %w", err)
		}
		fmt.Fprintf(os.Stderr, "注意: %s は git リポジトリではないので .gitignore は見ません\n", moduleRoot)
		return ignored, nil
	}

	var input bytes.Buffer
	for _, dir := range dirs {
		rel, err := filepath.Rel(moduleRoot, dir)
		if err != nil {
			return nil, fmt.Errorf("%s をモジュールルートからの相対パスにできません: %w", dir, err)
		}
		input.WriteString(rel)
		input.WriteByte(0)
	}

	cmd := exec.Command("git", "check-ignore", "-z", "--stdin")
	cmd.Dir = moduleRoot
	cmd.Stdin = &input
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// exit 1 は「どれも ignore されない」
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return ignored, nil
		}
		return nil, fmt.Errorf("git check-ignore の実行に失敗: %w\n%s", err, bytes.TrimSpace(stderr.Bytes()))
	}

	for rel := range bytes.SplitSeq(out, []byte{0}) {
		if len(rel) == 0 {
			continue
		}
		dir, err := canonicalDir(filepath.Join(moduleRoot, string(rel)))
		if err != nil {
			return nil, fmt.Errorf("git check-ignore が返したパスを解決できません: %w", err)
		}
		ignored[dir] = true
	}
	return ignored, nil
}

// dropIgnored は listed から git が ignore するパッケージを除いて返す｡
// 構成管理の外にあるパッケージは、一覧にも Ca/Ce の辺にも入れない｡
func dropIgnored(moduleRoot string, listed []listedPackage) ([]listedPackage, error) {
	dirs := make([]string, 0, len(listed))
	for _, p := range listed {
		dirs = append(dirs, p.Dir)
	}
	ignored, err := gitIgnoredDirs(moduleRoot, dirs)
	if err != nil {
		return nil, err
	}
	tracked := make([]listedPackage, 0, len(listed))
	for _, p := range listed {
		dir, err := canonicalDir(p.Dir)
		if err != nil {
			return nil, fmt.Errorf("go list が返したディレクトリを解決できません: %w", err)
		}
		if !ignored[dir] {
			tracked = append(tracked, p)
		}
	}
	return tracked, nil
}

// selectedPackage は報告の対象に選ばれたパッケージ｡Path は「引数 + 引数からの相対パス」で、
// 引数に . を渡せば cmd/analyze-modularity のような形になる｡
type selectedPackage struct {
	Path       string
	ImportPath string
	Name       string
	Dir        string
	GoFiles    []string
}

// selectPackages は listed のうち args のどれかの下にあり、解析する Go ファイルを持つパッケージを選ぶ｡
func selectPackages(listed []listedPackage, args []string) ([]selectedPackage, error) {
	var selected []selectedPackage
	for _, arg := range args {
		root, err := canonicalDir(arg)
		if err != nil {
			return nil, fmt.Errorf("パスが存在しません: %s", arg)
		}
		for _, p := range listed {
			dir, err := canonicalDir(p.Dir)
			if err != nil {
				return nil, fmt.Errorf("go list が返したディレクトリを解決できません: %w", err)
			}
			if dir != root && !strings.HasPrefix(dir, root+string(filepath.Separator)) {
				continue
			}
			if len(p.GoFiles) == 0 {
				continue
			}
			rel, err := filepath.Rel(root, dir)
			if err != nil {
				return nil, err
			}
			selected = append(selected, selectedPackage{Path: filepath.Join(arg, rel), ImportPath: p.ImportPath, Name: p.Name, Dir: p.Dir, GoFiles: p.GoFiles})
		}
	}
	return selected, nil
}

// canonicalDir は symlink を解決した絶対パスを返す｡
func canonicalDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// computeCoupling は tracked のパッケージどうしの参照から Ca/Ce を数え、パッケージのディレクトリをキーに返す｡
// Ce はほかのパッケージを参照する自分の宣言の単位 (declarationUnits) の数 (Martin)、
// Ca は自分を参照する宣言を持つほかのパッケージの数 (JDepend)｡
// どちらも、自分のコードが変わらない限り動かない数え方を選ぶ｡Ce を依存先のパッケージの数にすると依存先の統合で、
// Ca を依存元の宣言の数にすると既存の呼び手に関数を足すだけで値が動く｡
// 標準ライブラリ・外部モジュール・テストファイルは数えない｡
func computeCoupling(loaded []*packages.Package, tracked []listedPackage) map[string]PackageMetrics {
	inModule := make(map[string]bool, len(tracked))
	for _, p := range tracked {
		inModule[p.ImportPath] = true
	}

	efferent := make(map[string]int, len(tracked))
	dependents := make(map[string]map[string]bool, len(tracked))
	for _, p := range loaded {
		// テストを含む変種 (p [p.test]、p_test、p.test) は数えない
		if p.ID != p.PkgPath || !inModule[p.PkgPath] {
			continue
		}
		for _, targets := range declarationUnits(p) {
			referenced := false
			for target := range targets {
				if !inModule[target] {
					continue
				}
				if dependents[target] == nil {
					dependents[target] = make(map[string]bool)
				}
				dependents[target][p.PkgPath] = true
				referenced = true
			}
			if referenced {
				efferent[p.PkgPath]++
			}
		}
	}

	byDir := make(map[string]PackageMetrics, len(tracked))
	for _, p := range tracked {
		byDir[p.Dir] = PackageMetrics{
			Name:             p.Name,
			Path:             p.ImportPath,
			AfferentCoupling: len(dependents[p.ImportPath]),
			EfferentCoupling: efferent[p.ImportPath],
		}
	}
	return byDir
}

// declarationUnits は p のトップレベル宣言を単位に分け、単位ごとに参照するほかのパッケージのパスを返す｡
// 単位は Martin の Ce が数えるクラスに当たるもの: 型とそのメソッドで 1 つ、それ以外の関数は 1 つずつ、
// var/const/type は spec ごとに 1 つ｡メソッドを型へ寄せるのは、メソッドの多い型で Ce が膨らまないようにするため｡
func declarationUnits(p *packages.Package) map[any]map[string]bool {
	units := make(map[any]map[string]bool)
	add := func(unit any, node ast.Node) {
		if units[unit] == nil {
			units[unit] = make(map[string]bool)
		}
		ast.Inspect(node, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			if obj := p.TypesInfo.Uses[id]; obj != nil && obj.Pkg() != nil && obj.Pkg().Path() != p.PkgPath {
				units[unit][obj.Pkg().Path()] = true
			}
			return true
		})
	}

	for _, file := range p.Syntax {
		for _, decl := range file.Decls {
			switch decl := decl.(type) {
			case *ast.FuncDecl:
				add(funcUnit(p.TypesInfo, decl), decl)
			case *ast.GenDecl:
				for _, spec := range decl.Specs {
					switch spec := spec.(type) {
					case *ast.ImportSpec:
					case *ast.TypeSpec:
						add(p.TypesInfo.Defs[spec.Name], spec)
					default:
						add(spec, spec)
					}
				}
			}
		}
	}
	return units
}

// funcUnit は関数宣言が属する単位を返す｡メソッドはレシーバの型 (*types.TypeName)、それ以外は宣言そのもの｡
func funcUnit(info *types.Info, decl *ast.FuncDecl) any {
	fn, ok := info.Defs[decl.Name].(*types.Func)
	if !ok {
		return decl
	}
	recv := fn.Signature().Recv()
	if recv == nil {
		return decl
	}
	recvType := recv.Type()
	if ptr, ok := recvType.(*types.Pointer); ok {
		recvType = ptr.Elem()
	}
	named, ok := recvType.(*types.Named)
	if !ok {
		return decl
	}
	return named.Origin().Obj()
}
