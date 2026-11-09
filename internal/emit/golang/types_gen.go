package golang

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	gol "github.com/mew-ton/hozo/plugin/golang"
)

// genTypes は types.go（docs/emit-go.md「model」「union」「exception」「effect」と scalar の型）。
// 宣言は IR の順（種類ごとにまとめない）。複製した宣言はその後ろに Program の順で。
// 最後に、配線が必要とした射影の変換関数と ptr ヘルパー。
func (e *emitter) genTypes() *genFile {
	g := e.newFile("types.go")
	emitted := map[projection]bool{}
	for _, d := range e.ns.Decls {
		e.genTypeDecl(g, d, emitted)
	}
	for i := range e.prog.Namespaces {
		n := &e.prog.Namespaces[i]
		if n.Name == e.viewer {
			continue
		}
		for _, d := range n.Decls {
			if d != nil && e.copied[d.Common().Name] {
				e.copyNote = fmt.Sprintf("名前空間 %s の宣言の複製（Go のパッケージを import できないため。docs/emit-go.md「配置」）。", n.Name)
				e.genTypeDecl(g, d, emitted)
				e.copyNote = ""
			}
		}
	}
	for _, p := range e.projections {
		if !emitted[p] {
			emitted[p] = true
			e.genProjection(g, p)
		}
	}
	if e.needPtr {
		g.println("// ptr は値のポインタを作る（optional の位置に値を渡すとき）。")
		g.println("func ptr[T any](v T) *T { return &v }\n")
	}
	return g
}

func (e *emitter) genTypeDecl(g *genFile, d ir.Decl, emitted map[projection]bool) {
	switch x := d.(type) {
	case *ir.Model:
		e.genModel(g, x)
		if x.DerivedFrom != nil && e.model(x.DerivedFrom.Base) != nil {
			p := projection{x.DerivedFrom.Base, x.Name}
			e.projectionFunc(p.from, p.to)
			if !emitted[p] {
				emitted[p] = true
				e.genProjection(g, p)
			}
		}
	case *ir.Scalar:
		e.genScalar(g, x)
	case *ir.Union:
		e.genUnion(g, x)
	case *ir.Exception:
		e.genException(g, x)
	case *ir.Effect:
		e.genEffect(g, x)
	}
}

// declDoc は宣言のコメント。複製した宣言ならその旨を足す。
func (e *emitter) declDoc(goName string, d *ir.Doc, fallback string) string {
	c := declComment(goName, d, fallback)
	if e.copyNote != "" {
		c += "// " + e.copyNote + "\n"
	}
	return c
}

// genModel は構造体と、既定値があれば New<Model>（docs/emit-go.md「model」）。
func (e *emitter) genModel(g *genFile, m *ir.Model) {
	name := e.typeName(m.Name)
	fallback := m.Name + " の model。"
	if m.SynthesizedFrom != "" {
		if strings.Contains(m.Name, "$") {
			fallback = fmt.Sprintf("op %s の引数のうち bind で埋まる位置を除いたもの（docs/ir.md「その名前空間から見た IR」）。", m.SynthesizedFrom)
		} else {
			fallback = m.SynthesizedFrom + " のその場で書いた model。"
		}
	}
	g.body.WriteString(e.declDoc(name, m.Description, fallback))
	if m.DerivedFrom != nil {
		g.printf("// %s から %s を除いたもの（is ... omit）。\n", m.DerivedFrom.Base, strings.Join(m.DerivedFrom.Omit, ", "))
	}
	if len(m.Fields) == 0 {
		g.printf("type %s struct{}\n\n", name)
		return
	}
	g.printf("type %s struct {\n", name)
	hasDefault := false
	for _, f := range m.Fields {
		if f.Default != nil {
			hasDefault = true
		}
		if f.Description != nil {
			g.body.WriteString(comment(f.Description.Body))
		}
		if hasCase(f.Type) {
			g.printf("// 前提: %s\n", f.Type.String())
		}
		tag := f.Name
		if _, opt := stripOptional(f.Type); opt {
			tag += ",omitempty"
		}
		g.printf("\t%s %s `json:%s`\n", gol.FieldName(f.Name), gol.GoType(f.Type, &g.imports), strconv.Quote(tag))
	}
	g.println("}\n")
	if hasDefault {
		e.genNewModel(g, m)
	}
}

// hasCase は型のどこかに case があるか。
func hasCase(t ir.Type) bool {
	switch x := t.(type) {
	case ir.CaseType:
		return true
	case ir.OptionalType:
		return hasCase(x.Of)
	case ir.ArrayType:
		return hasCase(x.Of)
	case ir.RecordType:
		return hasCase(x.Key) || hasCase(x.Value)
	}
	return false
}

// genNewModel は New<Model>（docs/emit-go.md「model」）。既定値は GOEMIT-4 で確かめる。
func (e *emitter) genNewModel(g *genFile, m *ir.Model) {
	name := e.typeName(m.Name)
	g.printf("// New%s は既定値を入れた %s を返す。\n", name, name)
	g.printf("func New%s() %s {\n", name, name)
	g.printf("\tm := %s{}\n", name)
	for _, f := range m.Fields {
		if f.Default == nil {
			continue
		}
		e.checkDefault(m, &f)
		g.printf("\tm.%s = %s\n", gol.FieldName(f.Name), e.defaultExpr(f.Default, f.Type, &g.imports))
	}
	g.println("\treturn m\n}\n")
}

// genProjection は model から model への変換関数（docs/emit-go.md「model」の derivedFrom と
// 「配線の本体」の射影）。docs/types.md「射影」: 同じ名前のフィールドを写し、ないものは既定値で埋める。
func (e *emitter) genProjection(g *genFile, p projection) {
	from, to := e.model(p.from), e.model(p.to)
	if from == nil || to == nil {
		return
	}
	fromT, toT := gol.GoType(ir.Ref(p.from), &g.imports), gol.GoType(ir.Ref(p.to), &g.imports)
	fn := e.typeName(p.from) + "To" + e.typeName(p.to)
	g.printf("// %s は %s を %s に射影する（docs/types.md「射影」）。\n", fn, p.from, p.to)
	g.printf("func %s(v %s) %s {\n", fn, fromT, toT)
	g.printf("\treturn %s{\n", toT)
	for _, tf := range to.Fields {
		var expr string
		if ft, ok := e.fieldType(p.from, tf.Name); ok {
			expr = e.convert("v."+gol.FieldName(tf.Name), ft, tf.Type, &g.imports)
		} else if tf.Default != nil {
			expr = e.defaultExpr(tf.Default, tf.Type, &g.imports)
		} else {
			continue
		}
		g.printf("\t\t%s: %s,\n", gol.FieldName(tf.Name), expr)
	}
	g.println("\t}\n}\n")
}

// genScalar は名前付きの型（docs/emit-go.md「scalar」）。Validate は validate.go。
func (e *emitter) genScalar(g *genFile, s *ir.Scalar) {
	name := e.typeName(s.Name)
	g.body.WriteString(e.declDoc(name, s.Description, fmt.Sprintf("%s の scalar（元の型 %s）。", s.Name, s.Base.String())))
	g.printf("type %s %s\n\n", name, gol.GoType(s.Base, &g.imports))
}

// armWrapper は、ユニオンのアームの型が別のパッケージの型なら、N のパッケージに作る包む型の名前
// （docs/emit-go.md「union」）。
func (e *emitter) armWrapper(u *ir.Union, a ir.UnionArm) (string, bool) {
	if r, ok := stripCase(a.Type).(ir.RefType); ok && e.isLocal(r.Target) {
		return "", false
	}
	return e.typeName(u.Name) + gol.Capitalize(a.Name), true
}

// genUnion はマーカーのインターフェースと、アームごとのマーカーメソッド（docs/emit-go.md「union」）。
// set のユニオンは出さない。
func (e *emitter) genUnion(g *genFile, u *ir.Union) {
	if u.Variant != ir.UnionArms {
		return
	}
	name := e.typeName(u.Name)
	marker := "is" + name
	g.body.WriteString(e.declDoc(name, u.Description, u.Name+" の union。アームは "+armNames(u)+"。"))
	g.printf("type %s interface{ %s() }\n\n", name, marker)
	done := map[string]bool{}
	for _, a := range u.Arms {
		if w, ok := e.armWrapper(u, a); ok {
			if a.Description != nil {
				g.body.WriteString(comment(a.Description.Body))
			}
			g.printf("// %s は %s のアーム %s（型 %s は別のパッケージなので包む）。\n", w, u.Name, a.Name, a.Type.String())
			if _, isRef := stripCase(a.Type).(ir.RefType); isRef {
				g.printf("type %s struct{ %s }\n\n", w, gol.GoType(a.Type, &g.imports))
			} else {
				g.printf("type %s struct{ Value %s }\n\n", w, gol.GoType(a.Type, &g.imports))
			}
			g.printf("func (%s) %s() {}\n\n", w, marker)
			continue
		}
		t := gol.GoType(a.Type, &g.imports)
		if done[t] {
			continue
		}
		done[t] = true
		g.printf("func (%s) %s() {}\n\n", t, marker)
	}
}

func armNames(u *ir.Union) string {
	names := make([]string, len(u.Arms))
	for i, a := range u.Arms {
		names[i] = a.Name
	}
	return strings.Join(names, ", ")
}

// genException は runtime.Exception を満たす型（docs/emit-go.md「exception」）。
func (e *emitter) genException(g *genFile, x *ir.Exception) {
	name := e.typeName(x.Name)
	rt := g.imports.Add("github.com/mew-ton/hozo/runtime", "runtime")
	g.body.WriteString(comment(name + ": " + docText(x.Description, x.Name+" の exception。")))
	if e.copyNote != "" {
		g.printf("// %s\n", e.copyNote)
	}
	if x.Value != nil {
		g.printf("type %s struct{ Value %s }\n\n", name, gol.GoType(x.Value, &g.imports))
	} else {
		g.printf("type %s struct{}\n\n", name)
	}
	g.printf("func (%s) Error() string         { return %q }\n", name, x.Name)
	g.printf("func (%s) HozoException() string { return %q }\n\n", name, x.Name)
	g.printf("var _ %s.Exception = %s{}\n\n", rt, name)
}

// genEffect は効果の定数（docs/emit-go.md「effect」）。
func (e *emitter) genEffect(g *genFile, x *ir.Effect) {
	rt := g.imports.Add("github.com/mew-ton/hozo/runtime", "runtime")
	c := gol.EffectConst(localName(x.Name))
	g.body.WriteString(declComment(c, x.Description, fmt.Sprintf("効果 %s（状態 %s への %s）。", x.Name, x.State, x.Operation)))
	g.printf("const %s %s.Effect = %q\n\n", c, rt, x.Name)
}

// genDoc は doc.go（名前空間の description）。description がなければ nil。
func (e *emitter) genDoc() *genFile {
	if e.ns.Description == nil || strings.TrimSpace(e.ns.Description.Body) == "" {
		return nil
	}
	g := e.newFile("doc.go")
	g.pkgDoc = comment(joinJa("Package "+e.pkg+" は", strings.TrimSpace(e.ns.Description.Body)))
	return g
}
