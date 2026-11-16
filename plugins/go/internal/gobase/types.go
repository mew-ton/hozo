package gobase

import (
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは types.go（docs/plugin-go.md「型」）: model、scalar、union、exception、effect を、宣言の順に一つずつ。
// 定義のパッケージのもの（genTypes）と、分けた出力の入口の再公開（genEntryTypes）。

// typeDecl は、types.go に出す宣言か: N の model（見る側のために合成したものを除く）、scalar、union（arms）、exception、effect。
func (e *emitter) typeDecl(d ir.Decl) bool {
	switch x := d.(type) {
	case *ir.Model:
		// View が見る側のために合成した model（synthesizedFrom が op の FQN）は出さない（docs/plugin-go.md「出力の対象」）
		return x.SynthesizedFrom == "" || e.op(x.SynthesizedFrom) == nil
	case *ir.Union:
		return x.Variant == ir.UnionArms
	case *ir.Scalar, *ir.Exception, *ir.Effect:
		return true
	}
	return false
}

// modelComment は model のコメント: 「<型名> は<説明>」と、継承した model なら、継承の一行（docs/plugin-go.md「コメント」）。
func (e *emitter) modelComment(m *ir.Model) string {
	text := describe(e.typeName(m.Name), m.Description, "model "+m.Name+"。")
	if x := m.Extends; x != nil {
		var clauses []string
		if len(x.Removed) > 0 {
			clauses = append(clauses, joinJa(x.Removed)+" をなくした")
		}
		if len(x.Overridden) > 0 {
			clauses = append(clauses, joinJa(x.Overridden)+" を変えた")
		}
		var added []string
		if b, ok := e.lookup(x.Base).(*ir.Model); ok {
			inherited := map[string]bool{}
			for _, f := range b.Fields {
				inherited[f.Name] = true
			}
			for _, f := range m.Fields {
				if !inherited[f.Name] {
					added = append(added, f.Name)
				}
			}
		}
		if len(added) > 0 {
			clauses = append(clauses, joinJa(added)+" を足した")
		}
		clause := "何も変えていない"
		if len(clauses) > 0 {
			clause = strings.Join(clauses, "、")
		}
		text += "\n" + x.Base + " を継承し（extends）、" + clause + "もの。" + x.Base + " とは別の型で、暗黙には渡せない。"
	}
	return comment(text)
}

func (e *emitter) scalarComment(s *ir.Scalar) string {
	return comment(describe(e.typeName(s.Name), s.Description, "scalar "+s.Name+"。"))
}

func (e *emitter) unionComment(u *ir.Union) string {
	return comment(describe(e.typeName(u.Name), u.Description, "union "+u.Name+"。"))
}

// exceptionComment は「<型名>: <説明>」。説明がなければ「<型名> は exception <FQN>。」。
func (e *emitter) exceptionComment(x *ir.Exception) string {
	if b := body(x.Description); b != "" {
		return comment(e.typeName(x.Name) + ": " + b)
	}
	return comment(e.typeName(x.Name) + " は exception " + x.Name + "。")
}

// effectComment は「<定数名> は<説明>」。宛先が分類の遷移なら、末尾に `on <宛先の FQN>: <前> -> <後>` の一行を足す。
func (e *emitter) effectComment(x *ir.Effect) string {
	text := describe(golang.EffectConst(x.Name), x.Description, "effect "+x.Name+"。")
	if _, ok := x.Target.(ir.TransitionTarget); ok {
		text += "\non " + effectTarget(x)
	}
	return comment(text)
}

func newModelComment(name string) string {
	return comment("New" + name + " は既定値を入れた " + name + " を返す。")
}

// hasDefaults は、既定値を持つフィールドが一つでもあるか。IR にフィールドの既定値はなくなった（版 0.5。docs/ir.md「版」）ので、
// いつも false（New<Model> は出ない。出す仕組みそのものは、基底を作り直す段でなくす）。
func hasDefaults(m *ir.Model) bool {
	return false
}

// arm は union のアーム一つの、Go での型。
type armType struct {
	name    string  // アーム名
	typ     ir.Type // アームの型
	goName  string  // 修飾なしの Go の型の名前（N の model の構造体か、包む型）
	wrapped bool    // 包む型（`type <Union><アーム名> struct{ Value <型> }`）か
}

// armTypes は union u のアームの Go の型（docs/plugin-go.md「union」）: アームの型が、u を宣言した名前空間の model への ref なら、
// その構造体。それ以外（別の名前空間の model、scalar、核の型、optional、array、record、union）は、包む型 `<Union><アーム名>`。
func (e *emitter) armTypes(u *ir.Union) []armType {
	ns := e.declNS[u.Name]
	var out []armType
	for _, a := range u.Arms {
		at := armType{name: a.Name, typ: a.Type}
		if r, ok := a.Type.(ir.RefType); ok {
			if m, isModel := e.lookup(r.Target).(*ir.Model); isModel && e.declNS[m.Name] == ns {
				at.goName = e.typeName(m.Name)
				out = append(out, at)
				continue
			}
		}
		at.goName, at.wrapped = e.typeName(u.Name)+golang.Capitalize(a.Name), true
		out = append(out, at)
	}
	return out
}

// genTypes は定義のパッケージの types.go。中身がなければ nil。
func (e *emitter) genTypes() *fileGen {
	f := e.newFile(fileTypes, true)
	any := false
	for _, d := range e.ns.Decls {
		if !e.typeDecl(d) {
			continue
		}
		any = true
		switch x := d.(type) {
		case *ir.Model:
			e.genModel(f, x)
		case *ir.Scalar:
			f.print(e.scalarComment(x))
			f.printf("type %s %s\n\n", e.typeName(x.Name), f.goType(x.Base))
		case *ir.Union:
			e.genUnion(f, x)
		case *ir.Exception:
			name := e.typeName(x.Name)
			f.print(e.exceptionComment(x))
			if x.Value != nil {
				f.printf("type %s struct{ Value %s }\n\n", name, f.goType(x.Value))
			} else {
				f.printf("type %s struct{}\n\n", name)
			}
			f.printf("func (%s) Error() string { return %s }\n", name, strconv.Quote(x.Name))
			f.printf("func (%s) HozoException() string { return %s }\n\n", name, strconv.Quote(x.Name))
			f.printf("var _ %s.Exception = %s{}\n\n", f.im.Add(runtimePath, "runtime"), name)
		case *ir.Effect:
			f.print(e.effectComment(x))
			f.printf("const %s %s.Effect = %s\n\n", golang.EffectConst(x.Name), f.im.Add(runtimePath, "runtime"), strconv.Quote(x.Name))
		}
	}
	if !any {
		return nil
	}
	if e.usesPtr[f.key] {
		f.print(ptrHelper)
	}
	return f
}

// ptrHelper は、値をポインタにする補助（docs/plugin-go.md「値」）。使ったファイルの属するパッケージに一つだけ出す。
const ptrHelper = "func ptr[T any](v T) *T { return &v }\n"

// genModel は model 一つ: 構造体と、既定値を持つフィールドがあれば New<Model>。
func (e *emitter) genModel(f *fileGen, m *ir.Model) {
	name := e.typeName(m.Name)
	f.print(e.modelComment(m))
	if len(m.Fields) == 0 {
		f.printf("type %s struct{}\n\n", name)
	} else {
		f.printf("type %s struct {\n", name)
		for _, fd := range m.Fields {
			f.print(indent(comment(body(fd.Description))))
			tag := fd.Name
			if _, optional := fd.Type.(ir.OptionalType); optional {
				tag += ",omitempty"
			}
			f.printf("\t%s %s `json:%s`\n", golang.FieldName(fd.Name), f.goType(fd.Type), strconv.Quote(tag))
		}
		f.print("}\n\n")
	}
	if !hasDefaults(m) {
		return
	}
	f.print(newModelComment(name))
	f.printf("func New%s() %s {\n\tm := %s{}\n", name, name, name)
	f.print("\treturn m\n}\n\n")
}

// genUnion は union（arms）一つ: インターフェース、マーカーメソッド（アームの順）、包む型（アームの順）。
func (e *emitter) genUnion(f *fileGen, u *ir.Union) {
	name := e.typeName(u.Name)
	marker := "is" + name
	f.print(e.unionComment(u))
	f.printf("type %s interface{ %s() }\n\n", name, marker)
	arms := e.armTypes(u)
	for _, a := range arms {
		f.printf("func (%s) %s() {}\n", a.goName, marker)
	}
	if len(arms) > 0 {
		f.print("\n")
	}
	for _, a := range arms {
		if a.wrapped {
			f.print(wrapperComment(a.goName, u.Name, a.name, a.typ))
			f.printf("type %s struct{ Value %s }\n\n", a.goName, f.goType(a.typ))
		}
	}
}

func wrapperComment(goName, union, arm string, t ir.Type) string {
	return comment(goName + " は union " + union + " のアーム " + arm + "（" + golang.TypeDisplay(t) + "）を包む型。")
}

// literal は、リテラル lit を、型 target の位置に書く Go の式（docs/plugin-go.md「値」の literal の行と「型をそろえる」）。
// target が省略可能で、Go の型がポインタなら `ptr(<型>(<リテラル>))`。target が nil（位置の型が決まらない）なら、リテラルの型の値。
func (f *fileGen) literal(lit ir.Literal, target ir.Type) string {
	if opt, ok := target.(ir.OptionalType); ok {
		inner := f.literal(lit, opt.Of)
		if golang.IsNillable(opt.Of, f.e.p) {
			return inner
		}
		if _, isDuration := lit.(ir.DurationLiteral); isDuration {
			return f.ptr(inner) // すでに型の付いた式
		}
		return f.ptr(f.goType(opt.Of) + "(" + inner + ")")
	}
	if c, ok := target.(ir.CaseType); ok {
		return f.literal(lit, c.Of)
	}
	switch x := lit.(type) {
	case ir.StringLiteral:
		return strconv.Quote(x.Value)
	case ir.IntLiteral:
		if target == nil {
			return "int64(" + strconv.FormatInt(x.Value, 10) + ")"
		}
		return strconv.FormatInt(x.Value, 10)
	case ir.FloatLiteral:
		s := strconv.FormatFloat(x.Value, 'g', -1, 64)
		if !strings.ContainsAny(s, ".e") {
			s += ".0"
		}
		return s
	case ir.BooleanLiteral:
		return strconv.FormatBool(x.Value)
	case ir.DurationLiteral:
		typ := ""
		if _, isRef := target.(ir.RefType); isRef {
			typ = f.goType(target) // Duration が元の scalar
		} else {
			typ = f.goType(ir.Duration())
		}
		return typ + "(" + strconv.FormatInt(int64(x.Value), 10) + ")"
	}
	return "nil"
}

// genEntryTypes は、分けた出力の入口の types.go: 内側の型の別名、effect の定数、New<Model>（docs/plugin-go.md「内側のフォルダと入口」の
// 再公開の表）。コメントは定義と同じ。再公開するものがなければ nil。
func (e *emitter) genEntryTypes() *fileGen {
	f := e.newFile(fileTypes, false)
	inner := f.im.Add(e.layout.DefPath, e.layout.DefName)
	any := false
	alias := func(c, name string) {
		f.print(c)
		f.printf("type %s = %s.%s\n\n", name, inner, name)
	}
	for _, d := range e.ns.Decls {
		if !e.typeDecl(d) {
			continue
		}
		any = true
		switch x := d.(type) {
		case *ir.Model:
			name := e.typeName(x.Name)
			alias(e.modelComment(x), name)
			if hasDefaults(x) {
				f.print(newModelComment(name))
				f.printf("func New%s() %s { return %s.New%s() }\n\n", name, name, inner, name)
			}
		case *ir.Scalar:
			alias(e.scalarComment(x), e.typeName(x.Name))
		case *ir.Union:
			alias(e.unionComment(x), e.typeName(x.Name))
			for _, a := range e.armTypes(x) {
				if a.wrapped {
					alias(wrapperComment(a.goName, x.Name, a.name, a.typ), a.goName)
				}
			}
		case *ir.Exception:
			alias(e.exceptionComment(x), e.typeName(x.Name))
		case *ir.Effect:
			f.print(e.effectComment(x))
			f.printf("const %s = %s.%s\n\n", golang.EffectConst(x.Name), inner, golang.EffectConst(x.Name))
		}
	}
	if !any {
		return nil
	}
	return f
}
