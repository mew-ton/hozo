package gobase

import (
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは types.go（plugins/go/spec/go.hozo の Requirement.Types）: model、scalar、union、exception、effect を、宣言の順に一つずつ。
// 定義のパッケージのもの（genTypes）と、分けた出力の入口の再公開（genEntryTypes）。

// typeDecl は、types.go に出す宣言か: N の model（見る側のために合成したものを除く）、scalar、union（arms）、exception、effect。
func (e *emitter) typeDecl(d ir.Decl) bool {
	switch x := d.(type) {
	case *ir.Model:
		// View が見る側のために合成した model（synthesizedFrom が op の FQN）は出さない（plugins/go/spec/go.hozo の Requirement.Targets）
		return x.SynthesizedFrom == "" || e.op(x.SynthesizedFrom) == nil
	case *ir.Union:
		return x.Variant == ir.UnionArms
	case *ir.Scalar, *ir.Exception, *ir.Effect:
		return true
	}
	return false
}

// modelComment は model のコメント: 「<型名> は<説明>」と、継承した model なら、継承の一行（plugins/go/spec/go.hozo の Requirement.Comments）。
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

// effectComment は「<定数名> は<説明>」。宛先が分類の遷移なら、末尾に `effects <宛先の FQN> as <前> -> <後>` の一行を足す。
// 行の遷移を持つ効果（Effect.row）なら、末尾に `[<絞り込んだ引数の表示>、]effects <状態の FQN>.<操作> <行の遷移の表示>` の一行を足す。
func (e *emitter) effectComment(x *ir.Effect) string {
	text := describe(golang.EffectConst(x.Name), x.Description, "effect "+x.Name+"。")
	if _, ok := x.Target.(ir.TransitionTarget); ok || x.Row != nil {
		narrowed := ""
		if n := golang.NarrowedParamsDisplay(x); n != "" && x.Row != nil {
			narrowed = n + "、"
		}
		text += "\n" + narrowed + "effects " + effectTarget(x)
	}
	return comment(text)
}

// arm は union のアーム一つの、Go での型。
type armType struct {
	name    string  // アーム名
	typ     ir.Type // アームの型
	goName  string  // 修飾なしの Go の型の名前（N の model の構造体か、包む型）
	wrapped bool    // 包む型（`type <Union><アーム名> struct{ Value <型> }`）か
	// literal は、アームの型が、リテラルの型そのものなら、そのリテラル（包む型は、値を持たない: `type <Union><アーム名> struct{}`）。
	// そうでなければ nil。
	literal ir.Literal
}

// armTypes は union u のアームの Go の型（plugins/go/spec/go.hozo の Requirement.Types の「union」）: アームの型が、u を宣言した名前空間の model への ref なら、
// その構造体。それ以外（別の名前空間の model、scalar、核の型、optional、array、union）は、包む型 `<Union><アーム名>`。
// リテラルの型のアームは、値を持たない、包む型（armType.literal）。
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
		if lt, ok := a.Type.(ir.LiteralType); ok {
			at.literal = lt.Literal
		}
		out = append(out, at)
	}
	return out
}

// genTypes は定義のパッケージの types.go。中身がなければ nil。
func (e *emitter) genTypes() *fileGen {
	f := e.newTypesFile(fileTypes)
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
			// 引数ごとにフィールド一つ（Exception.params の順）。名前とタグは model のフィールドと同じ。引数のない例外は struct{}
			if len(x.Params) == 0 {
				f.printf("type %s struct{}\n\n", name)
			} else {
				f.printf("type %s struct {\n", name)
				for _, p := range x.Params {
					f.printf("\t%s\n", f.structField(p.Name, p.Type))
				}
				f.print("}\n\n")
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
	return f
}

// genModel は model 一つ: 構造体だけ（plugins/go/spec/go.hozo の Requirement.Types の「model」: 値を作る関数は出さない。フィールドの既定値は、言語にない）。
// インデックスの model は、名前付きのマップの型。リテラルの型のフィールドは、項目を持たず、コメントの行になる（同「model」）。
func (e *emitter) genModel(f *fileGen, m *ir.Model) {
	name := e.typeName(m.Name)
	f.print(e.modelComment(m))
	if m.Index != nil {
		f.printf("type %s map[%s]%s\n\n", name, f.goType(m.Index.Key), f.goType(m.Index.Value))
		return
	}
	stored := 0
	for _, fd := range m.Fields {
		if !isLiteralType(fd.Type) {
			stored++
		}
	}
	if stored == 0 {
		// フィールドが、リテラルの型のものだけなら、型の前の行に、同じコメント
		for _, fd := range m.Fields {
			f.print(comment(body(fd.Description)))
			f.print(literalFieldComment(fd))
		}
		f.printf("type %s struct{}\n\n", name)
		return
	}
	f.printf("type %s struct {\n", name)
	for _, fd := range m.Fields {
		f.print(indent(comment(body(fd.Description))))
		if isLiteralType(fd.Type) {
			f.print(indent(literalFieldComment(fd)))
			continue
		}
		f.printf("\t%s\n", f.structField(fd.Name, fd.Type))
	}
	f.print("}\n\n")
}

// isLiteralType は、型 t が、リテラルの型そのものか（optional や array の中のものは、数えない）。
func isLiteralType(t ir.Type) bool {
	_, ok := t.(ir.LiteralType)
	return ok
}

// literalFieldComment は、リテラルの型のフィールド fd の、項目の代わりの、コメントの行（plugins/go/spec/go.hozo の Requirement.Types の「model」）。
func literalFieldComment(fd ir.Field) string {
	return comment(fd.Name + " は、いつも " + golang.TypeDisplay(fd.Type) + "（Go の項目は、ない）")
}

// structField は、構造体のフィールド一つ（model のフィールドと、例外の引数。plugins/go/spec/go.hozo の Requirement.Names）: 名前は先頭を大文字に、
// タグは `json:"<元の名前>"`。型が省略可能なら `,omitempty`。
func (f *fileGen) structField(name string, t ir.Type) string {
	tag := name
	if _, optional := t.(ir.OptionalType); optional {
		tag += ",omitempty"
	}
	return golang.FieldName(name) + " " + f.goType(t) + " `json:" + strconv.Quote(tag) + "`"
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
			if a.literal != nil {
				// リテラルの型のアーム: 値は、型で決まっているので、持たせない
				f.printf("type %s struct{}\n\n", a.goName)
				continue
			}
			f.printf("type %s struct{ Value %s }\n\n", a.goName, f.goType(a.typ))
		}
	}
}

func wrapperComment(goName, union, arm string, t ir.Type) string {
	return comment(goName + " は union " + union + " のアーム " + arm + "（" + golang.TypeDisplay(t) + "）を包む型。")
}

// genEntryTypes は、分けた出力の入口の types.go: 内側の型の別名と、effect の定数（plugins/go/spec/go.hozo の Requirement.Layout の「内側のフォルダと入口」の
// 再公開の表）。コメントは定義と同じ。再公開するものがなければ nil。
// toDef なら、定義のパッケージの types.go: 型のパッケージの再公開（同「型のパッケージ」。型のパッケージを分ける出力だけ）。
func (e *emitter) genReexportTypes(toDef bool) *fileGen {
	f := e.newFile(fileTypes, toDef)
	var inner string
	if toDef {
		inner = f.im.Add(e.layout.TypesPath, e.layout.TypesName)
	} else {
		inner = f.im.Add(e.layout.DefPath, e.layout.DefName)
	}
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
			alias(e.modelComment(x), e.typeName(x.Name))
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
