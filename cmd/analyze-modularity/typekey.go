package main

import (
	"go/types"
	"strconv"
	"strings"
)

// writeTypeKey は型を、書き方によらず同じ型なら同じになる文字列で書く｡
// alias (any を含む) はどの深さでも解き、byte・rune は uint8・int32 として書く｡
// 名前付きの型はパッケージのパスで書くので、ソースと export data から読んだ同じ型も一致する｡
func writeTypeKey(b *strings.Builder, t types.Type) {
	switch t := types.Unalias(t).(type) {
	case *types.Basic:
		b.WriteString(types.Typ[t.Kind()].Name())
	case *types.Named:
		if pkg := t.Obj().Pkg(); pkg != nil {
			b.WriteString(pkg.Path())
			b.WriteString(".")
		}
		b.WriteString(t.Obj().Name())
		if args := t.TypeArgs(); args.Len() > 0 {
			b.WriteString("[")
			for i := range args.Len() {
				if i > 0 {
					b.WriteString(", ")
				}
				writeTypeKey(b, args.At(i))
			}
			b.WriteString("]")
		}
	case *types.Pointer:
		b.WriteString("*")
		writeTypeKey(b, t.Elem())
	case *types.Slice:
		b.WriteString("[]")
		writeTypeKey(b, t.Elem())
	case *types.Array:
		b.WriteString("[")
		b.WriteString(strconv.FormatInt(t.Len(), 10))
		b.WriteString("]")
		writeTypeKey(b, t.Elem())
	case *types.Map:
		b.WriteString("map[")
		writeTypeKey(b, t.Key())
		b.WriteString("]")
		writeTypeKey(b, t.Elem())
	case *types.Chan:
		b.WriteString(chanPrefix(t.Dir()))
		writeTypeKey(b, t.Elem())
	case *types.Signature:
		b.WriteString("func")
		writeSignatureKey(b, t)
	case *types.Struct:
		writeStructKey(b, t)
	case *types.Interface:
		writeInterfaceKey(b, t)
	default:
		// 型パラメータとメソッドのシグネチャに出てこない型 (union など) は、名前で書けば足りる
		b.WriteString(types.TypeString(t, func(p *types.Package) string { return p.Path() }))
	}
}

func chanPrefix(dir types.ChanDir) string {
	switch dir {
	case types.SendOnly:
		return "chan<- "
	case types.RecvOnly:
		return "<-chan "
	}
	return "chan "
}

// writeSignatureKey は「(引数) (戻り値)」を書く｡レシーバと引数の名前は含めない｡
func writeSignatureKey(b *strings.Builder, sig *types.Signature) {
	writeTupleKey(b, sig.Params())
	if sig.Variadic() {
		b.WriteString(" variadic")
	}
	b.WriteString(" ")
	writeTupleKey(b, sig.Results())
}

func writeTupleKey(b *strings.Builder, tuple *types.Tuple) {
	b.WriteString("(")
	for i := range tuple.Len() {
		if i > 0 {
			b.WriteString(", ")
		}
		writeTypeKey(b, tuple.At(i).Type())
	}
	b.WriteString(")")
}

func writeStructKey(b *strings.Builder, s *types.Struct) {
	b.WriteString("struct{")
	for i := range s.NumFields() {
		if i > 0 {
			b.WriteString("; ")
		}
		field := s.Field(i)
		if field.Embedded() {
			b.WriteString("embedded ")
		} else {
			b.WriteString(field.Name())
			b.WriteString(" ")
		}
		writeTypeKey(b, field.Type())
		if tag := s.Tag(i); tag != "" {
			b.WriteString(" ")
			b.WriteString(strconv.Quote(tag))
		}
	}
	b.WriteString("}")
}

// writeInterfaceKey は埋め込みを展開したメソッドの集合で書く｡Method は名前の順に並ぶ｡
func writeInterfaceKey(b *strings.Builder, iface *types.Interface) {
	b.WriteString("interface{")
	for i := range iface.NumMethods() {
		if i > 0 {
			b.WriteString("; ")
		}
		method := iface.Method(i)
		b.WriteString(method.Name())
		writeSignatureKey(b, method.Signature())
	}
	b.WriteString("}")
}
