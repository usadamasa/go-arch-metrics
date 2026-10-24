package main

import (
	"fmt"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

// exportedUsage はパッケージの exported のうち、外から参照されないものの名前｡
// メソッドは「型名.メソッド名」で表す｡
type exportedUsage struct {
	// Unreferenced はどこからも参照されない (自パッケージとその internal test を除く)
	Unreferenced []string
	// TestOnly は外部テストパッケージと他パッケージのテストからだけ参照される
	TestOnly []string
}

// referenceKind は exported が外のどこから参照されたか｡
type referenceKind int

const (
	referencedFromTest referenceKind = iota + 1
	referencedFromProduction
)

// analyzeExportedReferences は listed の main でないパッケージごとに、外から参照されない exported を返す｡
// 判定の軸は「unexport しても他の挙動が変わらないか」で、次のものを参照済みとみなす:
//   - 他パッケージ (テストを含む) の識別子が型情報の上で指すもの。dot import と埋め込みで昇格したメソッドを含む
//   - 読み込んだどこか (標準ライブラリを含む) の interface に、名前と型が同じメソッドがあるメソッド
//
// リフレクションと go:linkname からの参照は静的に見えないので数えない｡
func analyzeExportedReferences(moduleRoot string, listed []listedPackage) (map[string]exportedUsage, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedImports | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedSyntax,
		Dir:   moduleRoot,
		Tests: true,
	}
	loaded, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("パッケージの読み込みに失敗 (%s): %w", moduleRoot, err)
	}
	// 型検査に失敗したパッケージの参照は欠けるので、未参照を多く見積もらないよう止める
	for _, p := range loaded {
		if len(p.Errors) > 0 {
			return nil, fmt.Errorf("型検査に失敗 (%s): %v", p.ID, p.Errors[0])
		}
	}

	tracked := make(map[string]bool, len(listed))
	for _, p := range listed {
		tracked[p.ImportPath] = true
	}

	needed := interfaceMethods(loaded)
	references := make(map[string]map[string]referenceKind)
	for _, p := range loaded {
		// p.test は go test が生成する main で、利用者のコードではない
		if strings.HasSuffix(p.ID, ".test") || !tracked[strings.TrimSuffix(p.PkgPath, "_test")] {
			continue
		}
		kind := referencedFromProduction
		if p.ID != p.PkgPath {
			kind = referencedFromTest
		}
		for _, obj := range p.TypesInfo.Uses {
			if obj.Pkg() == nil || obj.Pkg().Path() == p.PkgPath {
				continue
			}
			key, ok := symbolKey(obj)
			if !ok {
				continue
			}
			target := obj.Pkg().Path()
			if references[target] == nil {
				references[target] = make(map[string]referenceKind)
			}
			references[target][key] = max(references[target][key], kind)
		}
	}

	result := make(map[string]exportedUsage)
	for _, p := range loaded {
		if p.ID != p.PkgPath || p.Name == "main" || !tracked[p.PkgPath] {
			continue
		}
		var usage exportedUsage
		for key, method := range exportedSymbols(p.Types) {
			if method != nil && needed[methodSignature(method)] {
				continue
			}
			switch references[p.PkgPath][key] {
			case referencedFromProduction:
			case referencedFromTest:
				usage.TestOnly = append(usage.TestOnly, key)
			default:
				usage.Unreferenced = append(usage.Unreferenced, key)
			}
		}
		slices.Sort(usage.Unreferenced)
		slices.Sort(usage.TestOnly)
		result[p.PkgPath] = usage
	}
	return result, nil
}

// exportedSymbols は pkg の package scope にある exported な func と type、および exported なメソッドを返す｡
// 値はメソッドならその *types.Func で、func と type なら nil｡const と var と struct のフィールドは数えない｡
func exportedSymbols(pkg *types.Package) map[string]*types.Func {
	symbols := make(map[string]*types.Func)
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		switch obj := scope.Lookup(name).(type) {
		case *types.Func:
			if obj.Exported() {
				symbols[name] = nil
			}
		case *types.TypeName:
			if obj.Exported() {
				symbols[name] = nil
			}
			named, ok := obj.Type().(*types.Named)
			if !ok || obj.IsAlias() {
				continue
			}
			// unexported な型の exported メソッドも、外へ返した値から呼べるので数える
			for method := range named.Methods() {
				if method.Exported() {
					symbols[name+"."+method.Name()] = method
				}
			}
		}
	}
	return symbols
}

// symbolKey は参照先のオブジェクトを exportedSymbols と同じ形の名前にする｡
func symbolKey(obj types.Object) (string, bool) {
	switch obj := obj.(type) {
	case *types.TypeName:
		return obj.Name(), true
	case *types.Func:
		recv := obj.Origin().Signature().Recv()
		if recv == nil {
			return obj.Name(), true
		}
		recvType := recv.Type()
		if ptr, ok := recvType.(*types.Pointer); ok {
			recvType = ptr.Elem()
		}
		named, ok := recvType.(*types.Named)
		if !ok {
			return "", false
		}
		return named.Obj().Name() + "." + obj.Name(), true
	}
	return "", false
}

// methodSignature はメソッドを「名前 + 引数と戻り値の型」の文字列にする｡レシーバと引数の名前は含めない｡
// ソースから読んだ型と export data から読んだ型は別のオブジェクトになるので、パッケージのパスで書いた文字列で比べる｡
func methodSignature(method *types.Func) string {
	qualifier := func(p *types.Package) string { return p.Path() }
	typesOf := func(tuple *types.Tuple) string {
		names := make([]string, 0, tuple.Len())
		for v := range tuple.Variables() {
			names = append(names, types.TypeString(v.Type(), qualifier))
		}
		return strings.Join(names, ", ")
	}
	sig := method.Signature()
	variadic := ""
	if sig.Variadic() {
		variadic = " variadic"
	}
	return method.Name() + "(" + typesOf(sig.Params()) + variadic + ") (" + typesOf(sig.Results()) + ")"
}

// interfaceMethods は読み込んだパッケージとその import 先 (標準ライブラリを含む) にある interface のメソッドを集める｡
// ソースを読んだパッケージでは、関数の中や型リテラルに書いた interface も拾う｡
func interfaceMethods(loaded []*packages.Package) map[string]bool {
	methods := make(map[string]bool)
	add := func(t types.Type) {
		iface, ok := t.Underlying().(*types.Interface)
		if !ok {
			return
		}
		for method := range iface.Methods() {
			methods[methodSignature(method)] = true
		}
	}

	visited := make(map[*types.Package]bool)
	var walk func(pkg *types.Package)
	walk = func(pkg *types.Package) {
		if visited[pkg] {
			return
		}
		visited[pkg] = true
		scope := pkg.Scope()
		for _, name := range scope.Names() {
			if tn, ok := scope.Lookup(name).(*types.TypeName); ok {
				add(tn.Type())
			}
		}
		for _, imported := range pkg.Imports() {
			walk(imported)
		}
	}

	for _, p := range loaded {
		walk(p.Types)
		for _, tv := range p.TypesInfo.Types {
			add(tv.Type)
		}
	}
	return methods
}
