package gobase

import (
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは ops.go（plugins/go/spec/go.hozo の Requirement.Ports、Requirement.Seams）: Ops、Leaves、<Ns>Ops、Seams、New、構造体 ops、
// ops のメソッドのうち配線の本体でないもの。定義のパッケージのもの（genOps）と、分けた出力の入口の再公開（genEntryOps）。

// コメントの決まった文（plugins/go/spec/go.hozo の Requirement.Comments）。
const (
	commentOpsEntry = "Ops は、この名前空間の外から呼べる op: 仕様で export を付けた op。宣言順。New が返す。"
	commentOpsInner = "Ops は、この名前空間のすべての op（export の有無によらない）。宣言順。New が返す。テストは、内部の op をここから呼ぶ。"
	commentLeaves   = "Leaves は、人が実装する葉の op: 本体のない op。状態には触れない。export の有無によらない。\n" +
		"実装は New の引数 %s に渡す。メソッドが一つでも足りない値は、渡すところでコンパイルできない。"
	commentMissing = "メソッドが一つでも足りない値は、渡すところでコンパイルできない。"
	commentSeams   = "Seams は、継ぎ目: 単位のトランスパイラが継ぎ目を宣言したデコレータの、op への適用ごとに、メソッド一つ。実装は New の引数 %s に渡す。\n" + commentMissing
	commentOps     = "ops は New が返す非公開の構造体。フィールドは New の引数そのまま。メソッドは、この名前空間の op のすべて。"
	// 入口の確かめを持つ単位が、一つでもあるとき（plugins/go/spec/go.hozo の Requirement.Ports の「New」）
	commentOpsChecked = "ops は New が返す非公開の構造体。フィールドは New の引数（入口の確かめを持つ状態の単位は、New が包んだもの）。メソッドは、この名前空間の op のすべて。"
)

func (e *emitter) leavesComment() string {
	return comment(strings.Replace(commentLeaves, "%s", e.fields.leaves, 1))
}

func (e *emitter) seamsComment() string {
	return comment(strings.Replace(commentSeams, "%s", e.fields.seams, 1))
}

// depTypeComment は、別の名前空間の口 `<M>Ops` のコメント。
func depTypeComment(d golang.Dep) string {
	return comment(d.TypeName + " は名前空間 " + d.Namespace + " の op のうち、この名前空間の配線が呼ぶもの。実装は New の引数 " + d.Param + " に渡す。\n" +
		"メソッドの形は相手の op の宣言だけで決まる（どの名前空間から見ても同じ）ので、" + d.Namespace + " の出力の Ops は、そのまま渡せる。")
}

// newComment は New のコメント: 一行目は、ある引数の種類を、この順に「、」でつないだ句。引数が一つもなければ一行だけ。
func (e *emitter) newComment() string {
	var kinds []string
	if len(e.shape.Leaves) > 0 {
		kinds = append(kinds, e.fields.leaves)
	}
	if len(e.shape.States) > 0 {
		kinds = append(kinds, "状態の単位（宣言順）")
	}
	if len(e.shape.Classify) > 0 {
		kinds = append(kinds, "分類の述語（FQN の辞書順）")
	}
	if len(e.shape.Deps) > 0 {
		kinds = append(kinds, "別の名前空間（名前の辞書順）")
	}
	if len(e.shape.Seams) > 0 {
		kinds = append(kinds, e.fields.seams)
	}
	if len(kinds) == 0 {
		return comment("New はこの名前空間の Ops を組み立てる。")
	}
	return comment("New はこの名前空間の Ops を組み立てる。引数の並びは、" + strings.Join(kinds, "、") + "。\n引数が nil かどうかは確かめない。")
}

// signature は、op のメソッドの「名前(引数) 戻り値」。
func (f *fileGen) signature(o *ir.Op) string {
	params, results := golang.Signature(o, f.im, f.e.reserved)
	return method(golang.MethodName(o.Name), params, results)
}

func method(name, params, results string) string {
	s := name + "(" + params + ")"
	if results != "" {
		s += " " + results
	}
	return s
}

// args は、op o を呼ぶときの引数の並び（ctx と、局所の名前）。メソッドの引数を、そのまま実装に渡すのに使う。
// どの op も、最初の引数に ctx を取る（plugins/go/spec/go.hozo の Requirement.OpShape）。
func (e *emitter) args(o *ir.Op) string {
	as := []string{"ctx"}
	for _, p := range o.Params {
		as = append(as, golang.LocalName(p.Name, e.reserved))
	}
	return strings.Join(as, ", ")
}

// newParams は New の引数（`<名前> <型>` の並び）と、その名前の並び。別の名前空間の分類の述語の型は、置き場の入口のパッケージで修飾する。
func (e *emitter) newParams(f *fileGen) (params, names []string) {
	foreign := map[int]string{} // Shape.New の添字 → 分類の FQN
	i := 0
	if len(e.shape.Leaves) > 0 {
		i++
	}
	i += len(e.shape.States)
	for _, fqn := range e.shape.Classify {
		foreign[i] = fqn
		i++
	}
	for i, p := range e.shape.New {
		typ := p.Type
		if fqn, ok := foreign[i]; ok {
			typ = f.im.Qualifier(e.declNS[fqn]) + typ
		}
		params = append(params, p.Name+" "+typ)
		names = append(names, p.Name)
	}
	return params, names
}

// opsInterface は、インターフェース Ops を書く。ops は、並べる op（宣言順）。メソッドの間は一行空ける。
func (e *emitter) opsInterface(f *fileGen, text string, ops []golang.OpShape) {
	f.print(comment(text))
	if len(ops) == 0 {
		f.print("type Ops interface{}\n\n")
		return
	}
	f.print("type Ops interface {\n")
	for i, os := range ops {
		if i > 0 {
			f.print("\n")
		}
		o := e.op(os.FQN)
		f.print(indent(opComment(o)))
		f.printf("\t%s\n", f.signature(o))
	}
	f.print("}\n\n")
}

// genOps は定義のパッケージの ops.go。N の op が一つもなければ nil。
func (e *emitter) genOps() *fileGen {
	if len(e.shape.Ops) == 0 {
		return nil
	}
	f := e.newFile(fileOps, true)
	s := e.shape

	// Ops: 分けた出力の内側は、すべての op。分けない出力は、すべての op が export なので、一つの Ops がどちらでもある
	text := commentOpsEntry
	if e.layout.HasInner {
		text = commentOpsInner
	}
	e.opsInterface(f, text, s.Ops)

	// Leaves
	if len(s.Leaves) > 0 {
		f.print(e.leavesComment())
		f.print("type Leaves interface {\n")
		for i, fqn := range s.Leaves {
			if i > 0 {
				f.print("\n")
			}
			o := e.op(fqn)
			f.print(indent(opComment(o)))
			f.printf("\t%s\n", f.signature(o))
		}
		f.print("}\n\n")
	}

	// <Ns>Ops（名前空間の FQN の辞書順）
	for _, d := range s.Deps {
		f.print(depTypeComment(d))
		f.printf("type %s interface {\n", d.TypeName)
		for i, fqn := range d.Ops {
			if i > 0 {
				f.print("\n")
			}
			o := e.op(fqn)
			f.print(indent(depComment(o, e.view.Signature(fqn))))
			f.printf("\t%s\n", f.signature(o))
		}
		f.print("}\n\n")
	}

	// Seams
	if len(s.Seams) > 0 {
		f.print(e.seamsComment())
		f.print("type Seams interface {\n")
		for i, seam := range s.Seams {
			if i > 0 {
				f.print("\n")
			}
			o := e.op(seam.Op)
			f.print(indent(comment(seam.Method + " は op " + seam.Op + " に付いた @" + seam.Decorator + " の継ぎ目（" + seam.Transpiler +
				"）。body が op の中身で、その前後に置くものを、実装が決める。")))
			params, results := golang.SeamSignature(o, f.im, e.reserved)
			f.printf("\t%s\n", method(seam.Method, params, results))
		}
		f.print("}\n\n")
	}

	// New と、構造体 ops
	params, _ := e.newParams(f)
	f.print(e.newComment())
	// 入口の確かめを持つ単位は、包んで入れる（plugins/go/spec/go.hozo の Requirement.Ports の「効果の入口の確かめ」）
	wrapped := map[string]string{} // New の引数の名前 → 包んだ式
	for _, u := range s.States {
		cs := e.checkedClassifies(u)
		if len(cs) == 0 {
			continue
		}
		parts := []string{u.Param}
		for _, fqn := range cs {
			parts = append(parts, e.fields.classify[fqn])
		}
		wrapped[u.Param] = golang.CheckedUnitName(u.FQN) + "{" + strings.Join(parts, ", ") + "}"
	}
	var inits []string
	for _, p := range s.New {
		if w, ok := wrapped[p.Name]; ok {
			inits = append(inits, p.Name+": "+w)
			continue
		}
		inits = append(inits, p.Name+": "+p.Name)
	}
	f.printf("func New(%s) Ops {\n\treturn &ops{%s}\n}\n\n", strings.Join(params, ", "), strings.Join(inits, ", "))
	if len(wrapped) > 0 {
		f.print(comment(commentOpsChecked))
	} else {
		f.print(comment(commentOps))
	}
	if len(s.New) == 0 {
		f.print("type ops struct{}\n\n")
	} else {
		f.print("type ops struct {\n")
		for _, p := range params {
			f.printf("\t%s\n", p)
		}
		f.print("}\n\n")
	}

	// ops のメソッドのうち、配線の本体でないもの（op の宣言順）
	for _, os := range s.Ops {
		o := e.op(os.FQN)
		if len(os.Layers) > 0 {
			e.genOuter(f, os, o)
			if os.Place == golang.Never {
				f.print(comment(os.Body + " は op " + o.Name + " の中身（" + neverException(o) + " を投げる）。外側の " + os.Method + " が、" + through(os) + "を通して呼ぶ。"))
				params, results := golang.Signature(o, f.im, e.reserved)
				f.printf("func (o *ops) %s {\n\treturn %s\n}\n\n", method(os.Body, params, results), e.neverValue(f, o))
			}
			continue
		}
		switch os.Place {
		case golang.Wiring:
			continue // wiring.go
		case golang.Never:
			f.printf("func (o *ops) %s {\n\treturn %s\n}\n\n", f.signature(o), e.neverValue(f, o))
		default:
			// 葉: 実装を呼んで、そのまま返す（どの op も error を返すので、いつも return）
			f.printf("func (o *ops) %s {\n\treturn %s(%s)\n}\n\n", f.signature(o), e.implExpr(os), e.args(o))
		}
	}
	return f
}

// implExpr は、op の実装（中身）を指す式（plugins/go/spec/go.hozo の Requirement.Seams の「中身」の表の「外側が渡す式」）: 配線と never を返す op は
// `o.<op>Body`（継ぎ目を持つときだけ使う）、葉は、Leaves のメソッドの値（名前は op 名のまま）。
func (e *emitter) implExpr(os golang.OpShape) string {
	switch os.Place {
	case golang.Wiring, golang.Never:
		return "o." + os.Body
	case golang.Leaf:
		return "o." + e.fields.leaves + "." + os.Method
	}
	e.errorf(codeInternal, os.FQN, "op %s の置き場（%d）を知らない（生成器の不具合）", os.FQN, os.Place)
	return "nil"
}

// neverException は、never を返す op が投げる例外の FQN（throws の例外は、ちょうど一つ。THROWS-3）。
func neverException(o *ir.Op) string {
	for _, c := range o.Throws {
		if len(c.Exceptions) > 0 {
			return c.Exceptions[0]
		}
	}
	return ""
}

// neverValue は、never を返す op の中身の式: `<例外の型>{}`。
func (e *emitter) neverValue(f *fileGen, o *ir.Op) string {
	x := neverException(o)
	if x == "" {
		e.errorf(codeInternal, o.Name, "never を返す op %s の throws に例外がない（検査を通った IR には来ないはずの形）", o.Name)
		return "nil"
	}
	return f.goType(ir.Ref(x)) + "{}"
}

// genOuter は、包むデコレータ（継ぎ目か、本体のあるデコレータ）を持つ op の外側: ops の公開のメソッド（plugins/go/spec/go.hozo の Requirement.Seams の「外側」
// 「本体のあるデコレータ」）。層が二つ以上なら入れ子にする: Op.decorators の最後（op に最も近い）のものが最も内側で、
// 中身を直に受け取る。
func (e *emitter) genOuter(f *fileGen, os golang.OpShape, o *ir.Op) {
	var decorators []string
	for _, l := range os.Layers {
		decorators = append(decorators, "@"+l.Decorator)
	}
	impl := e.implExpr(os)
	f.print(comment(os.Method + " は、" + through(os) + "（" + joinJa(decorators) + "）を通して、中身（" + impl + "）を呼ぶ。"))
	params, results := golang.Signature(o, f.im, e.reserved)
	f.printf("func (o *ops) %s {\n\treturn %s\n}\n\n", method(os.Method, params, results), e.layerCall(f, os, o, 0))
}

// layerCall は、op の層 i（0 が、いちばん外。len(os.Layers) が、op の中身）を、op の引数のまま呼ぶ式。
func (e *emitter) layerCall(f *fileGen, os golang.OpShape, o *ir.Op, i int) string {
	args := e.args(o)
	if i >= len(os.Layers) {
		return e.implExpr(os) + "(" + args + ")"
	}
	l := os.Layers[i]
	if l.Bodied {
		return "o." + l.Method + "(" + args + ")"
	}
	return "o." + e.fields.seams + "." + l.Method + "(" + args + ", " + e.layerFunc(f, os, o, i+1) + ")"
}

// layerFunc は、op の層 i を、関数の値として指す式（継ぎ目に、中身として渡す）。
func (e *emitter) layerFunc(f *fileGen, os golang.OpShape, o *ir.Op, i int) string {
	if i >= len(os.Layers) {
		return e.implExpr(os)
	}
	l := os.Layers[i]
	if l.Bodied {
		return "o." + l.Method
	}
	params, results := golang.Signature(o, f.im, e.reserved)
	return "func(" + params + ") " + results + " {\nreturn " + e.layerCall(f, os, o, i) + "\n}"
}

// exportedOps は、N の op のうち export を付けたもの（入口の Ops に並ぶもの）。宣言順。
func (e *emitter) exportedOps() []golang.OpShape {
	var out []golang.OpShape
	for _, o := range e.shape.Ops {
		if o.Exported {
			out = append(out, o)
		}
	}
	return out
}

// genEntryOps は、分けた出力の入口の ops.go: 入口の Ops（export の op だけ）、Leaves と <Ns>Ops と Seams の別名、
// 内側の New を呼ぶ New（plugins/go/spec/go.hozo の Requirement.Layout の「内側のフォルダと入口」）。export の op が一つもなくても、空の Ops と New を出す。
func (e *emitter) genEntryOps() *fileGen {
	f := e.newFile(fileOps, false)
	inner := f.im.Add(e.layout.DefPath, e.layout.DefName)
	s := e.shape
	e.opsInterface(f, commentOpsEntry, e.exportedOps())
	alias := func(c, name string) {
		f.print(c)
		f.printf("type %s = %s.%s\n\n", name, inner, name)
	}
	if len(s.Leaves) > 0 {
		alias(e.leavesComment(), "Leaves")
	}
	for _, d := range s.Deps {
		alias(depTypeComment(d), d.TypeName)
	}
	if len(s.Seams) > 0 {
		alias(e.seamsComment(), "Seams")
	}
	params, names := e.newParams(f)
	f.print(e.newComment())
	f.printf("func New(%s) Ops {\n\treturn %s.New(%s)\n}\n", strings.Join(params, ", "), inner, strings.Join(names, ", "))
	return f
}
