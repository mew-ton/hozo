package gobase

import (
	"reflect"

	"github.com/mew-ton/hozo/ir"
)

// checkLiteralTypes は GO-9（docs/plugin-go.md「診断」）: 名前空間 N の宣言が、基底が、まだ Go の形を持たない、リテラルの型の
// 使い方をしていれば、その宣言を断る。形を当て推量して、コードを出さない。断るのは、次の二つ。
//
//   - リテラルの型（ir.LiteralType）を、型の位置（フィールド、引数、戻り値、union のアーム、状態、分類の対象）に使う。
//     配線の本体の中の型（アームの中の対象の型が、as に書いたリテラルの型になる）は、見ない: Go の型は、もとの型と同じ。
//   - literal の絞り込み（ir.NarrowByLiteral）を持つ分類の対象が、型 boolean の値でない（boolean? と、状態の分類）。
//   - op かデコレータの配線が、union の位置に、アームの型の値を入れている（IR の arm。docs/types.md「union に入れる」）。
//
// 対象の型が boolean の、値の分類と op の分類の literal の絞り込みは、出せる（同「match」）。
func (e *emitter) checkLiteralTypes() {
	const tail = "。トランスパイラ go は、まだ、その Go の形を持たない"
	for _, d := range e.ns.Decls {
		name := d.Common().Name
		if c, ok := d.(*ir.Classify); ok && hasLiteralNarrowing(c) && !isBoolean(c.SubjectType()) {
			e.errorf(codeLiteralType, name, "%s %s は、リテラルの型で決まる絞り込み（literal）を持つが、対象が、型 boolean の値でない（boolean? と、状態の分類は、まだ）%s", d.Kind(), name, tail)
			continue
		}
		if usesLiteralType(reflect.ValueOf(d)) {
			e.errorf(codeLiteralType, name, "%s %s は、リテラルの型を、型の位置に使う%s", d.Kind(), name, tail)
			continue
		}
		if entersUnion(reflect.ValueOf(d)) {
			e.errorf(codeLiteralType, name, "%s %s は、配線で、union の位置に、アームの型の値を入れる%s", d.Kind(), name, tail)
		}
	}
}

// hasLiteralNarrowing は、分類 c が、literal の絞り込みのケースを持つか。
func hasLiteralNarrowing(c *ir.Classify) bool {
	for _, cs := range c.Cases {
		for _, as := range cs.As {
			if as.By == ir.NarrowByLiteral {
				return true
			}
		}
	}
	return false
}

// isBoolean は、型 t が、核の型 boolean そのものか（optional でも、絞り込んだ型でもない）。
func isBoolean(t ir.Type) bool {
	c, ok := t.(ir.CoreType)
	return ok && c.Name == ir.CoreBoolean
}

var (
	literalTypeType = reflect.TypeOf(ir.LiteralType{})
	caseTypingType  = reflect.TypeOf(ir.CaseTyping{})
	wiringType      = reflect.TypeOf(ir.Wiring{})
)

// usesLiteralType は、値 v の中（構造体の項目、並び、ポインタとインターフェースの先）に、リテラルの型があるか。配線の本体
// （ir.Wiring）と、literal の絞り込みの as の型（ir.CaseTyping。型を作らない）は、見ない。
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
		switch v.Type() {
		case literalTypeType:
			return true
		case wiringType:
			return false
		case caseTypingType:
			if v.Interface().(ir.CaseTyping).By == ir.NarrowByLiteral {
				return false
			}
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() && usesLiteralType(v.Field(i)) {
				return true
			}
		}
	}
	return false
}

var armRefType = reflect.TypeOf((*ir.ArmRef)(nil))

// entersUnion は、値 v の中（配線の本体を含む）に、union に入れる値（nil でない *ir.ArmRef）があるか。
func entersUnion(v reflect.Value) bool {
	if v.Type() == armRefType {
		return !v.IsNil()
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		return !v.IsNil() && entersUnion(v.Elem())
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if entersUnion(v.Index(i)) {
				return true
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() && entersUnion(v.Field(i)) {
				return true
			}
		}
	}
	return false
}
