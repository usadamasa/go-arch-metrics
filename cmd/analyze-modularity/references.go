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
	// TestOnly は自パッケージの外部テスト (package p_test) からだけ参照される
	TestOnly []string
}

// referenceKind は exported が外のどこから参照されたか｡大きいほど強い参照｡
type referenceKind int

const (
	// referencedFromOwnXTest は自パッケージの外部テストからの参照｡export_test.go へ移せば unexport できる
	referencedFromOwnXTest referenceKind = iota + 1
	// referencedFromOutside は他パッケージ (テストを含む) からの参照｡unexport すると他パッケージが壊れる
	referencedFromOutside
)

// analyzeExportedReferences は listed の main でないパッケージごとに、外から参照されない exported を返す｡
// 判定の軸は「unexport しても他の挙動が変わらないか」で、規則とその理由は
// go-arch-metrics:evaluate の references/thresholds.md に置く｡
func analyzeExportedReferences(moduleRoot string, listed []listedPackage) (map[string]exportedUsage, error) {
	loaded, err := loadTypedPackages(moduleRoot)
	if err != nil {
		return nil, err
	}
	tracked := make(map[string]bool, len(listed))
	for _, p := range listed {
		tracked[p.ImportPath] = true
	}

	c := referenceCollector{
		needed:     interfaceMethods(loaded),
		references: make(map[string]map[string]referenceKind),
	}
	for _, p := range loaded {
		// p.test は go test が生成する main で、利用者のコードではない
		if strings.HasSuffix(p.ID, ".test") || !tracked[strings.TrimSuffix(p.PkgPath, "_test")] {
			continue
		}
		c.collect(p)
	}

	result := make(map[string]exportedUsage)
	for _, p := range loaded {
		if p.ID != p.PkgPath || p.Name == "main" || !tracked[p.PkgPath] {
			continue
		}
		result[p.PkgPath] = c.usageOf(p.Types)
	}
	return result, nil
}

// loadTypedPackages は moduleRoot のパッケージをテストを含めて型検査して返す｡
func loadTypedPackages(moduleRoot string) ([]*packages.Package, error) {
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
	return loaded, nil
}

// referenceCollector はパッケージのパスごとに、exported が外からどの強さで参照されたかを集める｡
type referenceCollector struct {
	// needed は読み込んだどこかの interface にあるメソッドの methodSignature
	needed     map[string]bool
	references map[string]map[string]referenceKind
}

// collect は p の識別子が指す、他パッケージのオブジェクトを参照として記録する｡
func (c *referenceCollector) collect(p *packages.Package) {
	kind := referencedFromOutside
	if p.ID != p.PkgPath && strings.HasSuffix(p.PkgPath, "_test") {
		kind = referencedFromOwnXTest
	}
	for _, obj := range p.TypesInfo.Uses {
		if obj.Pkg() == nil || obj.Pkg().Path() == p.PkgPath {
			continue
		}
		// 外部テストが参照する自パッケージ以外のものは、他パッケージのテストからの参照になる
		if kind == referencedFromOwnXTest && obj.Pkg().Path()+"_test" != p.PkgPath {
			c.mark(obj, referencedFromOutside)
			continue
		}
		c.mark(obj, kind)
	}
}

// mark は obj を kind で参照済みにし、unexport すると外から型名を書けなくなる型を同じ kind で辿る｡
func (c *referenceCollector) mark(obj types.Object, kind referenceKind) {
	key, ok := symbolKey(obj)
	if !ok {
		return
	}
	path := obj.Pkg().Path()
	if c.references[path] == nil {
		c.references[path] = make(map[string]referenceKind)
	}
	if c.references[path][key] >= kind {
		return
	}
	c.references[path][key] = kind

	switch obj := obj.(type) {
	case *types.Func:
		c.walk(path, obj.Origin().Signature(), kind)
	case *types.TypeName:
		c.markTypeName(path, obj, kind)
	}
}

// markTypeName は参照される型から辿れる型を辿る: 別名の右辺、型パラメータの制約、exported なフィールド、
// 外から呼ばれるか interface のために残すメソッドのシグネチャ｡
func (c *referenceCollector) markTypeName(path string, obj *types.TypeName, kind referenceKind) {
	if alias, ok := obj.Type().(*types.Alias); ok {
		c.walk(path, alias.Rhs(), kind)
		return
	}
	named, ok := obj.Type().(*types.Named)
	if !ok {
		return
	}
	for tp := range named.TypeParams().TypeParams() {
		c.walk(path, tp.Constraint(), kind)
	}
	c.walk(path, named.Underlying(), kind)
	for method := range named.Methods() {
		if method.Exported() && c.needed[methodSignature(method)] {
			c.walk(path, method.Signature(), kind)
		}
	}
}

// walk は t に現れる path のパッケージの型を mark する｡他パッケージの名前付き型の中へは入らない｡
func (c *referenceCollector) walk(path string, t types.Type, kind referenceKind) {
	switch t := t.(type) {
	case *types.Alias:
		c.markIfIn(path, t.Obj(), kind)
		c.walkTypeArgs(path, t.TypeArgs(), kind)
	case *types.Named:
		c.markIfIn(path, t.Origin().Obj(), kind)
		c.walkTypeArgs(path, t.TypeArgs(), kind)
	case *types.Pointer:
		c.walk(path, t.Elem(), kind)
	case *types.Slice:
		c.walk(path, t.Elem(), kind)
	case *types.Array:
		c.walk(path, t.Elem(), kind)
	case *types.Chan:
		c.walk(path, t.Elem(), kind)
	case *types.Map:
		c.walk(path, t.Key(), kind)
		c.walk(path, t.Elem(), kind)
	case *types.Signature:
		c.walkSignature(path, t, kind)
	case *types.Struct:
		for field := range t.Fields() {
			if field.Exported() {
				c.walk(path, field.Type(), kind)
			}
		}
	case *types.Interface:
		for method := range t.ExplicitMethods() {
			if method.Exported() {
				c.walk(path, method.Signature(), kind)
			}
		}
		for embedded := range t.EmbeddedTypes() {
			c.walk(path, embedded, kind)
		}
	case *types.Union:
		for term := range t.Terms() {
			c.walk(path, term.Type(), kind)
		}
	case *types.TypeParam:
		c.walk(path, t.Constraint(), kind)
	}
}

func (c *referenceCollector) walkSignature(path string, sig *types.Signature, kind referenceKind) {
	for tp := range sig.TypeParams().TypeParams() {
		c.walk(path, tp.Constraint(), kind)
	}
	for v := range sig.Params().Variables() {
		c.walk(path, v.Type(), kind)
	}
	for v := range sig.Results().Variables() {
		c.walk(path, v.Type(), kind)
	}
}

func (c *referenceCollector) walkTypeArgs(path string, args *types.TypeList, kind referenceKind) {
	for arg := range args.Types() {
		c.walk(path, arg, kind)
	}
}

// markIfIn は obj が path のパッケージのものなら mark する｡
func (c *referenceCollector) markIfIn(path string, obj *types.TypeName, kind referenceKind) {
	if obj.Pkg() != nil && obj.Pkg().Path() == path {
		c.mark(obj, kind)
	}
}

// usageOf は pkg の exported を、集めた参照の強さで分類する｡interface のために残すメソッドは数えない｡
func (c *referenceCollector) usageOf(pkg *types.Package) exportedUsage {
	var usage exportedUsage
	for key, method := range exportedSymbols(pkg) {
		if method != nil && c.needed[methodSignature(method)] {
			continue
		}
		switch c.references[pkg.Path()][key] {
		case referencedFromOutside:
		case referencedFromOwnXTest:
			usage.TestOnly = append(usage.TestOnly, key)
		default:
			usage.Unreferenced = append(usage.Unreferenced, key)
		}
	}
	slices.Sort(usage.Unreferenced)
	slices.Sort(usage.TestOnly)
	return usage
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
