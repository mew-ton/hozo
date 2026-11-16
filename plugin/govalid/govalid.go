// Package govalid は、検証のトランスパイラ go-valid（docs/plugin-go-valid.md）の形の規則の写し。go-valid と、Validate を呼ぶ相乗り
// （go-http、go-cli）が共有する: 呼ぶ側は、go-valid の出力を見ずに、ここの関数で、どの型が Validate を持つかを求める
// （docs/plugin-protocol.md「相乗りが基底の形を知る方法」）。
//
// 規則の持ち主は docs/plugin-go-valid.md「Validate を持つ型」で、API の形は docs/packages.md「`plugin/govalid`（公開）」。
package govalid

import (
	"strings"

	"github.com/mew-ton/hozo/ir"
)

// vocabulary は、制約のデコレータを持つ語彙の名前空間（docs/vocab.md「同梱する語彙」の valid）。
const vocabulary = "valid"

// standard は valid の標準の制約（docs/plugin-go-valid.md「マニフェスト」「制約」）。
var standard = map[string]bool{
	vocabulary + ".min":     true,
	vocabulary + ".max":     true,
	vocabulary + ".length":  true,
	vocabulary + ".pattern": true,
	vocabulary + ".format":  true,
}

// Standard は、decorator（汎用デコレータの FQN。Application.Decorator）が valid の標準の制約
// （min / max / length / pattern / format）か。利用者が valid に足した制約（設計書 §6.5）は false。
func Standard(decorator string) bool { return standard[decorator] }

// constraint は、decorator が valid の語彙のデコレータ（`@valid.*`）か: 名前空間 valid に宣言されたもの。標準のものも、
// 利用者が足したものも数える。子の名前空間（valid.sub.x）のものは数えない。
func constraint(decorator string) bool {
	name, ok := strings.CutPrefix(decorator, vocabulary+".")
	return ok && name != "" && !strings.Contains(name, ".")
}

// HasValidate は、宣言 typeFQN（scalar か model）が Validate を持つか（docs/plugin-go-valid.md「Validate を持つ型」）。
// 仕様だけから決まり、設定によらない。次のどれかに当たるものが持ち、当たるものがなくなるまで繰り返して決める
// （互いを参照する model があってもよい）:
//
//  1. `@valid.*` が付いた scalar。
//  2. 元の型（Scalar.base）が、Validate を持つ scalar である scalar。
//  3. `@valid.*` が付いたフィールドを持つ model。
//  4. Validate を持つ型を、フィールドの型に持つ model。フィールドの型は、optional、array、record の値を通して見る。
//
// 継承した model は、展開済みのフィールド（受け継いだデコレータを含む）で判定する。union と exception は持たない。
// View が見る側のために合成した model（synthesizedFrom が op の FQN。Go の型にならない。docs/plugin-go.md「出力の対象」）も持たない。
// scalar でも model でもない宣言、Program にない FQN は false。
func HasValidate(p *ir.Program, typeFQN string) bool {
	if p == nil {
		return false
	}
	switch p.Lookup(typeFQN).(type) {
	case *ir.Scalar, *ir.Model:
		return validated(p)[typeFQN]
	}
	return false
}

// validated は、Validate を持つ scalar と model の FQN の集合。
func validated(p *ir.Program) map[string]bool {
	decls := map[string]ir.Decl{}
	var order []ir.Decl
	for i := range p.Namespaces {
		for _, d := range p.Namespaces[i].Decls {
			if d == nil {
				continue
			}
			if _, dup := decls[d.Common().Name]; dup {
				continue
			}
			decls[d.Common().Name] = d
			order = append(order, d)
		}
	}
	has := map[string]bool{}
	// 1 と 3: 自分に付いた制約で決まるもの
	for _, d := range order {
		switch x := d.(type) {
		case *ir.Scalar:
			has[x.Name] = constrained(x.Decorators)
		case *ir.Model:
			if viewSynthesized(x, decls) {
				continue
			}
			for _, f := range x.Fields {
				has[x.Name] = has[x.Name] || constrained(f.Decorators)
			}
		}
	}
	// 2 と 4: 当たるものがなくなるまで繰り返す
	for changed := true; changed; {
		changed = false
		for _, d := range order {
			name := d.Common().Name
			if has[name] {
				continue
			}
			switch x := d.(type) {
			case *ir.Scalar:
				if r, ok := x.Base.(ir.RefType); ok && has[r.Target] {
					has[name], changed = true, true
				}
			case *ir.Model:
				if viewSynthesized(x, decls) {
					continue
				}
				for _, f := range x.Fields {
					if r, ok := through(f.Type).(ir.RefType); ok && has[r.Target] {
						has[name], changed = true, true
						break
					}
				}
			}
		}
	}
	return has
}

// constrained は、decorators に `@valid.*` があるか。
func constrained(decorators []ir.Application) bool {
	for _, app := range decorators {
		if constraint(app.Decorator) {
			return true
		}
	}
	return false
}

// through は、フィールドの型を、optional、array、record の値を通して見た型（`T?`、`T[]`、`Record<K, T>` の T）。
func through(t ir.Type) ir.Type {
	for {
		switch x := t.(type) {
		case ir.OptionalType:
			t = x.Of
		case ir.ArrayType:
			t = x.Of
		case ir.RecordType:
			t = x.Value
		case ir.CaseType:
			t = x.Of
		default:
			return t
		}
	}
}

// viewSynthesized は、m が、View が見る側のために合成した model か（synthesizedFrom が op の FQN）。
func viewSynthesized(m *ir.Model, decls map[string]ir.Decl) bool {
	if m.SynthesizedFrom == "" {
		return false
	}
	_, isOp := decls[m.SynthesizedFrom].(*ir.Op)
	return isOp
}
