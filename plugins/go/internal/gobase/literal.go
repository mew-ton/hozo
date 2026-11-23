package gobase

import (
	"reflect"

	"github.com/mew-ton/hozo/ir"
)

// checkLiteralTypes は GO-9（docs/plugin-go.md「診断」）: 名前空間 N の宣言が、基底が、まだ Go の形を持たない、リテラルの型の
// 使い方をしていれば、その宣言を断る。形を当て推量して、コードを出さない。断るのは、次の二つ。
//
//   - リテラルの型（ir.LiteralType）を、Go の形のない位置に使う: op・read・効果・デコレータの引数と戻り値、例外の引数、状態の型、
//     optional と array の中、インデックスの model のキーと値、分類の対象。
//   - literal の絞り込み（ir.NarrowByLiteral）を持つ分類の対象が、型 boolean か boolean? の値でない（状態の分類）。
//
// 断らないのは、Go の形のあるもの: model のフィールドの型そのもの（同「model」）、union のアームの型そのもの（同「union」）、
// 分類のケースの as の型（同「match」。型を作らない）。配線の本体の中の型は、見ない。
func (e *emitter) checkLiteralTypes() {
	const tail = "。トランスパイラ go は、まだ、その Go の形を持たない"
	for _, d := range e.ns.Decls {
		name := d.Common().Name
		if c, ok := d.(*ir.Classify); ok && hasLiteralNarrowing(c) && !isBooleanOrOptional(c.SubjectType()) {
			e.errorf(codeLiteralType, name, "%s %s は、リテラルの型で決まる絞り込み（literal）を持つが、対象が、型 boolean か boolean? の値でない（状態の分類は、まだ）%s", d.Kind(), name, tail)
			continue
		}
		if usesLiteralType(reflect.ValueOf(d)) {
			e.errorf(codeLiteralType, name, "%s %s は、リテラルの型を、Go の形のない位置に使う（形があるのは、model のフィールドの型そのものと、union のアームの型そのもの）%s", d.Kind(), name, tail)
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

// isBooleanOrOptional は、型 t が、核の型 boolean か、その optional か（絞り込んだ型でない）。値の分類と op の分類でなければ、t は nil。
func isBooleanOrOptional(t ir.Type) bool {
	if o, ok := t.(ir.OptionalType); ok {
		t = o.Of
	}
	c, ok := t.(ir.CoreType)
	return ok && c.Name == ir.CoreBoolean
}

var (
	literalTypeType = reflect.TypeOf(ir.LiteralType{})
	caseTypingType  = reflect.TypeOf(ir.CaseTyping{})
	wiringType      = reflect.TypeOf(ir.Wiring{})
	fieldType       = reflect.TypeOf(ir.Field{})
	unionArmType    = reflect.TypeOf(ir.UnionArm{})
)

// usesLiteralType は、値 v の中（構造体の項目、並び、ポインタとインターフェースの先）に、Go の形のない位置の、リテラルの型があるか。
// 見ないもの: 配線の本体（ir.Wiring）、分類のケースの as の型（ir.CaseTyping。型を作らない）、型がリテラルの型そのものの、
// model のフィールド（ir.Field）と union のアーム（ir.UnionArm）。
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
		case wiringType, caseTypingType:
			return false
		case fieldType:
			if isLiteralType(v.Interface().(ir.Field).Type) {
				return false
			}
		case unionArmType:
			if isLiteralType(v.Interface().(ir.UnionArm).Type) {
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
