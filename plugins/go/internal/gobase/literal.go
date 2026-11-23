package gobase

import (
	"reflect"

	"github.com/mew-ton/hozo/ir"
)

// checkLiteralTypes は GO-9（docs/plugin-go.md「診断」）: 名前空間 N の宣言が、リテラルの型（ir.LiteralType）か、literal の
// 絞り込み（ir.NarrowByLiteral）を使っていれば、その宣言を断る。基底は、まだ、その Go の形を決めていない
// （docs/open.md「聞かずに決めて、確かめていない決定」）ので、形を当て推量して、コードを出さない。
func (e *emitter) checkLiteralTypes() {
	for _, d := range e.ns.Decls {
		if usesLiteralType(reflect.ValueOf(d)) {
			name := d.Common().Name
			e.errorf(codeLiteralType, name, "%s %s は、リテラルの型（か、リテラルの型で決まる分類）を使う。トランスパイラ go は、まだ、リテラルの型を出せない", d.Kind(), name)
		}
	}
}

var (
	literalTypeType = reflect.TypeOf(ir.LiteralType{})
	caseTypingType  = reflect.TypeOf(ir.CaseTyping{})
)

// usesLiteralType は、値 v の中（構造体の項目、並び、ポインタとインターフェースの先）に、リテラルの型か、literal の絞り込みがあるか。
// IR は木（宣言は、ほかの宣言を、FQN の文字列で指す）なので、辿り切れる。
func usesLiteralType(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		return !v.IsNil() && usesLiteralType(v.Elem())
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if usesLiteralType(v.Index(i)) {
				return true
			}
		}
	case reflect.Map:
		for it := v.MapRange(); it.Next(); {
			if usesLiteralType(it.Value()) {
				return true
			}
		}
	case reflect.Struct:
		if v.Type() == literalTypeType {
			return true
		}
		if v.Type() == caseTypingType && v.Interface().(ir.CaseTyping).By == ir.NarrowByLiteral {
			return true
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() && usesLiteralType(v.Field(i)) {
				return true
			}
		}
	}
	return false
}
