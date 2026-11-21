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
//  4. Validate を持つ型を、フィールドの型に持つ model。フィールドの型は、optional、array と、インデックスの model の値を通して見る。
//     インデックスの model そのものは、Validate を持たない。
//
// 継承した model は、展開済みのフィールド（受け継いだデコレータを含む）で判定する。union と exception は持たない（例外の引数の型が
// Validate を持つ型でも、例外の構造体には出さない）。
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
					if r, ok := Through(p, f.Type).(ir.RefType); ok && has[r.Target] {
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

// Through は、フィールドの型を、optional、array と、インデックスの model の値を通して見た型（`T?`、`T[]`、
// `model Items { [name: string]: T; }` の Items から T。docs/plugin-go-valid.md「Validate を持つ型」の 4）。
// インデックスの model だけを辿って、同じ model に戻ったら（値を持たない循環）、その ref を返す。
func Through(p *ir.Program, t ir.Type) ir.Type {
	seen := map[string]bool{}
	for {
		switch x := t.(type) {
		case ir.OptionalType:
			t = x.Of
		case ir.ArrayType:
			t = x.Of
		case ir.CaseType:
			t = x.Of
		case ir.RefType:
			idx := p.IndexOf(x)
			if idx == nil || idx.Value == nil || seen[x.Target] {
				return t
			}
			seen[x.Target] = true
			t = idx.Value
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

// Transpiler は、Validate を出すトランスパイラの名前（docs/plugin-go-valid.md「マニフェスト」）。
const Transpiler = "go-valid"

// Unlisted は、view の単位から見た、名前空間 namespace の型の置き場（docs/config.md「型の置き場」）の単位が、transpilers に
// go-valid を並べていなければ、その単位が何番目の [[emit]] か（1 始まり）と true を返す。並べていれば、または置き場がなければ
// （CFG-22 が通したはずのもの）、0 と false。
//
// Validate は、型の定義のパッケージにしか書けないので、別の名前空間の型の Validate を呼ぶコードは、その型の置き場の単位が go-valid を
// 並べているときにだけコンパイルできる。呼ぶ側（go-valid の GOVALID-3、go-http の GOHTTP-11、go-cli の CLI-9）は、これが true なら
// 診断にする。置き場は View だけから求まる（docs/ir.md「その名前空間から見た IR」: View.namespaces の要素のうち、index が
// config.index と同じもの、なければ base が config.base と同じ最初のもの）。namespace が view.Viewer なら、View.config を見る。
func Unlisted(view *ir.View, namespace string) (unit int, ok bool) {
	if view == nil {
		return 0, false
	}
	home := &view.Config
	if namespace != view.Viewer {
		home = nil
		list := view.Namespaces[namespace]
		for i := range list {
			if list[i].Index == view.Config.Index {
				home = &list[i]
				break
			}
		}
		for i := range list {
			if home == nil && list[i].Base == view.Config.Base {
				home = &list[i]
			}
		}
	}
	if home == nil {
		return 0, false
	}
	for _, t := range home.Transpilers {
		if t == Transpiler {
			return 0, false
		}
	}
	return home.Index, true
}
