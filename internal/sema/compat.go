package sema

import (
	"fmt"
	"strings"

	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/ir"
)

// assignFailure は渡せない理由。空なら渡せる。
type assignFailure string

const (
	assignOK assignFailure = ""
	// failScalar は核の型（リテラルを含む）の値を scalar の位置に渡した（TYPE-1）。
	failScalar assignFailure = "TYPE-1"
	// failType はそれ以外（呼び出しの引数なら TYPE-4、return なら TYPE-3）。
	failType assignFailure = "TYPE-4"
	// failAmbiguous は、union の位置に渡した値が、アームの二つ以上の型に渡せる（TYPE-20。assignInto が、報告を済ませている）。
	failAmbiguous assignFailure = "TYPE-20"
)

// assignInto は、式の型 s の値を、型 t の位置に置けるか（docs/types.md「互換」の規則 1〜14）。値を、型の決まった位置に置く所
// （同「union に入れる」の「入れられる位置」）が呼ぶ。規則 14 で入るなら、どのアームに入るかを返す（IR の arm）。
// 二つ以上のアームに当たれば、span に TYPE-20 を出して、failAmbiguous を返す（呼び元は、重ねて報告しない）。
// dry なら、診断を出さず、IR にも書かない（アームを数えるための、試し）。
func (a *analyzer) assignInto(s exprType, t ir.Type, span source.Span, dry bool) (assignFailure, *ir.ArmRef) {
	un := a.unionArms(stripOptional(t))
	switch s.kind {
	case etUnknown, etNever, etNoValue, etNone:
		un = nil
	}
	if un == nil {
		return a.direct(s, t, dry), nil
	}
	if s.kind == etValue && (s.typ == nil || a.assignType(s.typ, t)) {
		return assignOK, nil // 規則 1〜13 で、そのまま渡せる（union の値そのもの、など）
	}
	// 規則 14: アームを、宣言の順に見て、規則 1〜13 で渡せる型のアームを数える（重ねない: アームの中では、規則 14 を使わない）
	var hits []ir.UnionArm
	for _, arm := range un.Arms {
		if arm.Type != nil && a.direct(s, arm.Type, true) == assignOK {
			hits = append(hits, arm)
		}
	}
	switch len(hits) {
	case 0:
		return failType, nil
	case 1:
		if !dry {
			a.direct(s, hits[0].Type, false) // レコードリテラルは、決まったアームの model に、当てはめる（中の診断と、中の arm）
		}
		return assignOK, &ir.ArmRef{Union: stripOptional(t).(ir.RefType).Target, Arm: hits[0].Name}
	}
	if !dry {
		names := make([]string, len(hits))
		for i, h := range hits {
			names[i] = h.Name
		}
		a.errorf("TYPE-20", span, "%s は、union %s のアーム %s の、どの型にも渡せて、どのアームに入るかが、決まらない", exprName(s), un.Name, strings.Join(names, "、"))
	}
	return failAmbiguous, nil
}

// direct は、規則 1〜13（assign）。dry なら、レコードリテラルの当てはめの診断を出さず、当てはまらなければ failType を返す。
func (a *analyzer) direct(s exprType, t ir.Type, dry bool) assignFailure {
	if !dry || s.kind != etRecord {
		return a.assign(s, t)
	}
	m := a.modelOf(stripOptional(t))
	if m == nil || !a.fitRecord(s.rec, m, true) {
		return failType
	}
	return assignOK
}

// assign は式の型 s の値を型 t の位置に渡せるか（docs/types.md「互換」）。レコードリテラルは当てはめ（fitRecord）、
// その診断（TYPE-5 / TYPE-6 と中の値）はここで出す。
func (a *analyzer) assign(s exprType, t ir.Type) assignFailure {
	switch s.kind {
	case etUnknown:
		return assignOK
	case etNever:
		// 「返らない」式は、どの型の位置にも書ける（docs/types.md「式の型」）
		return assignOK
	case etNoValue:
		return failType
	case etNone:
		// 規則 3: none は optional に
		if _, ok := t.(ir.OptionalType); ok {
			return assignOK
		}
		return failType
	case etLiteral:
		return a.assignLiteral(s, t)
	case etRecord:
		return a.assignRecord(s.rec, t)
	}
	if s.typ == nil || t == nil {
		return assignOK
	}
	if (isCore(s.typ) || isLiteralType(s.typ)) && a.isScalarRef(stripOptional(t)) {
		return failScalar // 明示: 核の型（とリテラルの型）の値を scalar の位置に渡す（TYPE-1）
	}
	if a.assignType(s.typ, t) {
		return assignOK
	}
	return failType
}

// assignLiteral は、リテラルの式を渡す規則: 規則 10（対応する核の型に）、規則 13（同じ値の、リテラルの型に）と、規則 2 を通した
// optional。scalar の位置は TYPE-1。
func (a *analyzer) assignLiteral(s exprType, t ir.Type) assignFailure {
	target := stripOptional(t)
	if a.isScalarRef(target) {
		return failScalar
	}
	switch c := target.(type) {
	case ir.CoreType:
		if literalAssignable(s.typ, c) {
			return assignOK
		}
	case ir.LiteralType:
		if s.lit != nil && literalFits(s.lit, c.Literal) {
			return assignOK
		}
	}
	return failType
}

// literalFits は、リテラルの値 v を、リテラルの型 want の位置に渡せるか（docs/types.md「互換」の 13）: 種類と値が同じ。
// 整数のリテラルは、同じ値の、小数のリテラルの型にも渡せる（規則 10 の int → float と、同じ向き）。
func literalFits(v, want ir.Literal) bool {
	if ir.LiteralEqual(v, want) {
		return true
	}
	i, isInt := v.(ir.IntLiteral)
	f, isFloat := want.(ir.FloatLiteral)
	return isInt && isFloat && float64(i.Value) == f.Value
}

// literalAssignable は規則 10: string→string、int→int か float、float→float、boolean→boolean、duration→Duration。
func literalAssignable(lit ir.Type, c ir.CoreType) bool {
	l, ok := lit.(ir.CoreType)
	if !ok {
		return false
	}
	if l.Name == ir.CoreInt {
		return c.Name == ir.CoreInt || c.Name == ir.CoreFloat
	}
	return l.Name == c.Name
}

// assignRecord は規則 9: T が model への ref（か model の optional）で、リテラルが当てはまる。
func (a *analyzer) assignRecord(r *recordVal, t ir.Type) assignFailure {
	m := a.modelOf(stripOptional(t))
	if m == nil {
		return failType
	}
	a.fitRecord(r, m, false)
	return assignOK
}

// assignType は型 S の値を型 T の位置に渡せるか（docs/types.md「互換」の規則 1、2、4〜7、11、12。規則 8 は欠番）。
func (a *analyzer) assignType(s, t ir.Type) bool {
	if s == nil || t == nil {
		return false
	}
	// 1. 等しい
	if ir.TypeEqual(s, t) {
		return true
	}
	// 2. T が optional(T') で、S が T' に渡せる。渡せなければ、残りの規則（絞り込みを捨てた S が T と等しい、など）を見る
	if o, ok := t.(ir.OptionalType); ok && a.assignType(s, o.Of) {
		return true
	}
	switch sv := s.(type) {
	case ir.LiteralType:
		// 10. リテラルの型は、対応する核の型に。13. 整数のリテラルの型は、同じ値の、小数のリテラルの型に
		switch tv := t.(type) {
		case ir.CoreType:
			return literalAssignable(ir.LiteralCore(sv.Literal), tv)
		case ir.LiteralType:
			return literalFits(sv.Literal, tv.Literal)
		}
		return false
	case ir.CaseType:
		// 5. 同じ classify の絞り込みで、ケースが部分集合
		if tv, ok := t.(ir.CaseType); ok && tv.Classify == sv.Classify && ir.TypeEqual(sv.Of, tv.Of) && subset(sv.Cases, tv.Cases) {
			return true
		}
		// 4. 絞り込みを捨てる
		return a.assignType(sv.Of, t)
	case ir.OptionalType:
		// 12. 中の型が渡せる
		tv, ok := t.(ir.OptionalType)
		return ok && a.assignType(sv.Of, tv.Of)
	case ir.ArrayType:
		// 6. 要素が渡せる
		tv, ok := t.(ir.ArrayType)
		return ok && a.assignType(sv.Of, tv.Of)
		// 7 は欠番（Record<K, V> どうしの規則だった。インデックスの model は、名前で比べる: 規則 1）
	case ir.RefType:
		// 11. scalar の値は元の型の位置に渡せる。model の値は、別の model の位置には渡せない
		// （フィールドが同じでも、一方がもう一方を継承していても。docs/types.md「互換」の「渡せないもの」）
		if sc := a.scalarSym(sv.Target); sc != nil {
			if base := a.scalarBaseOf(sc); base != nil {
				return a.assignType(base, t)
			}
		}
	}
	return false
}

// fitRecord は docs/types.md「当てはまる（レコードリテラルを model T に）」。TYPE-5 / TYPE-6 と、各値の互換を確かめる。
// 必須のフィールドが埋まっているか（BIND-4）は bind を要するので internal/check が行う。
// フィールドの値は、規則 14 の位置で、入ったアームを、IR のフィールド（recordFieldVal.out）に書く。
// dry は、union のアームを数えるための、試し（docs/types.md「union に入れる」の「アームの決め方」）: 診断を出さず、IR に書かない。
// 二度あるフィールド（TYPE-6）と、アームの決まらない値（TYPE-20）は、どの model に当てはめても同じなので、試しでは、当てはまる方に数える
// （アームが決まった後の、本番の当てはめが、報告する）。
func (a *analyzer) fitRecord(r *recordVal, m *ir.Model, dry bool) bool {
	ok := true
	seen := map[string]bool{}
	for i := range r.fields {
		f := &r.fields[i]
		if seen[f.name] {
			if !dry {
				a.errorf("TYPE-6", f.span, "レコードリテラルにフィールド %s が二度ある", f.name)
				ok = false
			}
			continue
		}
		seen[f.name] = true
		mf := findField(m, f.name)
		if mf == nil {
			if !dry {
				a.errorf("TYPE-5", f.span, "model %s にフィールド %s はない", m.Name, f.name)
			}
			ok = false
			continue
		}
		fail, arm := a.assignInto(f.typ, mf.Type, f.valueSpan, dry)
		if !dry && f.out != nil {
			f.out.Arm = arm
		}
		switch fail {
		case assignOK:
		case failAmbiguous:
			ok = ok && dry
		default:
			if !dry {
				a.errorf(string(fail), f.valueSpan, "フィールド %s.%s の型 %s に %s は渡せない", m.Name, f.name, typeName(mf.Type), exprName(f.typ))
			}
			ok = false
		}
	}
	return ok
}

// recordVal は評価したレコードリテラル。
type recordVal struct {
	span   source.Span
	fields []recordFieldVal
}

type recordFieldVal struct {
	name      string
	span      source.Span
	valueSpan source.Span
	typ       exprType
	// out は、IR のフィールド（入ったアームを書く先）。
	out *ir.RecordField
}

// --- 型の補助 ---

func stripOptional(t ir.Type) ir.Type {
	if o, ok := t.(ir.OptionalType); ok {
		return o.Of
	}
	return t
}

func isCore(t ir.Type) bool {
	_, ok := t.(ir.CoreType)
	return ok
}

func isLiteralType(t ir.Type) bool {
	_, ok := t.(ir.LiteralType)
	return ok
}

// modelOf は model への ref ならその model。合成した model も含む。
func (a *analyzer) modelOf(t ir.Type) *ir.Model {
	r, ok := t.(ir.RefType)
	if !ok {
		return nil
	}
	return a.models[r.Target]
}

// scalarSym は FQN が scalar の宣言ならその symbol。
func (a *analyzer) scalarSym(fqn string) *symbol {
	s := a.syms[fqn]
	if s == nil || s.kind != kindScalar {
		return nil
	}
	return s
}

func (a *analyzer) isScalarRef(t ir.Type) bool {
	r, ok := t.(ir.RefType)
	return ok && a.scalarSym(r.Target) != nil
}

// scalarBaseOf は scalar の継承を辿った核の型。循環や未解決なら nil。
func (a *analyzer) scalarBaseOf(s *symbol) ir.Type {
	seen := map[string]bool{}
	for s != nil && !seen[s.fqn] {
		seen[s.fqn] = true
		sc, ok := a.lower(s).(*ir.Scalar)
		if !ok || sc.Base == nil {
			return nil
		}
		r, ok := sc.Base.(ir.RefType)
		if !ok {
			return sc.Base
		}
		s = a.scalarSym(r.Target)
	}
	return nil
}

// scalarBase は t が scalar への ref なら継承を辿った核の型、そうでなければ t。
func (a *analyzer) scalarBase(t ir.Type) ir.Type {
	if r, ok := t.(ir.RefType); ok {
		if s := a.scalarSym(r.Target); s != nil {
			return a.scalarBaseOf(s)
		}
	}
	return t
}

// subset は a のすべての要素が b にあるか。
func subset(a, b []string) bool {
	for _, x := range a {
		if !contains(b, x) {
			return false
		}
	}
	return true
}

func contains(list []string, x string) bool {
	for _, y := range list {
		if y == x {
			return true
		}
	}
	return false
}

// failMessage は TYPE-4 / TYPE-1 の文言。
func failMessage(fail assignFailure, what string, got exprType, want ir.Type) string {
	if fail == failScalar {
		return fmt.Sprintf("%s: 核の型の値 %s を scalar %s の位置に渡せない（検証を通していない）", what, exprName(got), typeName(want))
	}
	return fmt.Sprintf("%s: %s に %s は渡せない", what, typeName(want), exprName(got))
}
