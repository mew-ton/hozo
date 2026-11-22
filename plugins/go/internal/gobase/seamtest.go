package gobase

import (
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは、デコレータのテストの口（docs/plugin-go.md「デコレータのテストの口」）: 単位の継ぎ目にあるデコレータの、結果ごとの
// ケース（<D>Cases、<D>Case）、まとめの SeamCases、走らせる RunSeams。例外を受けるデコレータ（catches）の継ぎ目には、RunSeams が、
// 受ける例外ごとのテストを足す（同「受けるケース」）。

const fileSeams = "seams.go"

// seamBodyNames は、seams.go の関数の本体が、決まった名前で使う識別子。import の名前に使わない。
var seamBodyNames = []string{"t", "c", "k", "cases", "calls", "err", "want", "ctx", "caught"}

// seamRelation は、デコレータ d の throws の節が、例外 x に付けた関係: "prevents" か "keeps"。関係がない、両方ある、は ""。
func seamRelation(d *ir.Decorator, x string) string {
	prevents, keeps := false, false
	for _, c := range d.Throws {
		for _, y := range c.Exceptions {
			if y != x {
				continue
			}
			for _, r := range c.Relations {
				switch r.Kind {
				case ir.Prevents:
					prevents = true
				case ir.Keeps:
					keeps = true
				}
			}
		}
	}
	switch {
	case prevents && !keeps:
		return "prevents"
	case keeps && !prevents:
		return "keeps"
	}
	return ""
}

// seamResults は、デコレータ d の結果: 正常（OK）と、宣言の throws の例外ごと（書かれた順。同じ例外は、一度だけ。引数の名前で
// 書いた例外は、入れない）。名前の決まりは、葉の op の結果と同じ（leafResults）。
func (e *emitter) seamResults(d *ir.Decorator) []leafResult {
	var clauses []ir.ThrowsClause
	for _, c := range d.Throws {
		var xs []string
		for _, x := range c.Exceptions {
			if !strings.HasPrefix(x, "$param:") {
				xs = append(xs, x)
			}
		}
		clauses = append(clauses, ir.ThrowsClause{Exceptions: xs})
	}
	return e.leafResults(&ir.Op{Throws: clauses})
}

// seamCatch は、継ぎ目一つが、受ける例外一つ: 例外の FQN と、それを受けた節の扱い。
type seamCatch struct {
	exception string
	mode      ir.CatchMode
}

// argTargets は、デコレータの適用の引数の値が指す宣言の FQN の列（ref は一つ、refs と list は、複数。ユニオンは、IR で展開済み）。
// op の引数名（"$param:"）と、リテラルなどは、含めない。
func argTargets(v ir.ArgValue) []string {
	var out []string
	switch x := v.(type) {
	case ir.RefValue:
		if !strings.HasPrefix(x.Target, "$param:") {
			out = append(out, x.Target)
		}
	case ir.RefsValue:
		for _, t := range x.Targets {
			if !strings.HasPrefix(t, "$param:") {
				out = append(out, t)
			}
		}
	case ir.ListValue:
		for _, item := range x.Items {
			out = append(out, argTargets(item)...)
		}
	}
	return out
}

// seamCatches は、op o への、デコレータ d（FQN は fqn）の適用のすべてについて、受ける例外（catches の節の例外。節の順。
// 同じ例外は、一度だけで、扱いは、最初に出てきた節のもの）と、デコレータが投げうる例外（throws の節の例外。書かれた順。
// 同じ例外は、一度だけ）。デコレータの引数の名前（"$param:名前"）は、その適用が渡した例外で置き換える
// （docs/plugin-go.md「デコレータのテストの口」の「受けるケース」）。Program にない例外は、入れない。
func (e *emitter) seamCatches(o *ir.Op, fqn string, d *ir.Decorator) (catches []seamCatch, throws []string) {
	seenCatch, seenThrow := map[string]bool{}, map[string]bool{}
	for _, app := range o.Decorators {
		if app.Decorator != fqn {
			continue
		}
		subst := func(name string) []string {
			param, ok := strings.CutPrefix(name, "$param:")
			if !ok {
				return []string{name}
			}
			for _, a := range app.Args {
				if a.Param == param {
					return argTargets(a.Value)
				}
			}
			return nil
		}
		for _, c := range d.Catches {
			for _, raw := range c.Exceptions {
				for _, x := range subst(raw) {
					if seenCatch[x] || e.lookup(x) == nil {
						continue
					}
					seenCatch[x] = true
					catches = append(catches, seamCatch{exception: x, mode: c.Mode})
				}
			}
		}
		for _, c := range d.Throws {
			for _, raw := range c.Exceptions {
				for _, x := range subst(raw) {
					if seenThrow[x] || e.lookup(x) == nil {
						continue
					}
					seenThrow[x] = true
					throws = append(throws, x)
				}
			}
		}
	}
	return catches, throws
}

// genTestSeams はテストのパッケージの seams.go。対象のデコレータが一つもなければ nil。
func (e *emitter) genTestSeams() *fileGen {
	decorators := golang.SeamDecorators(e.shape)
	if len(decorators) == 0 {
		return nil
	}
	// 受けるケースの、投げうる例外の変数（want0、want1、…）も、import の名前に使わない
	names, catching := append([]string{}, seamBodyNames...), false
	for _, sm := range e.shape.Seams {
		o, _ := e.lookup(sm.Op).(*ir.Op)
		d, _ := e.lookup(sm.Decorator).(*ir.Decorator)
		if o == nil || d == nil {
			continue
		}
		catches, throws := e.seamCatches(o, sm.Decorator, d)
		catching = catching || len(catches) > 0
		for i := len(names) - len(seamBodyNames); i < len(throws); i++ {
			names = append(names, "want"+strconv.Itoa(i))
		}
	}
	f := e.newTestFile(fileSeams, names)
	testing := f.im.Add("testing", "testing")
	entry := f.im.Qualifier(e.view.Viewer)
	decs := map[string]*ir.Decorator{}
	for _, fqn := range decorators {
		d, _ := e.lookup(fqn).(*ir.Decorator)
		if d == nil {
			e.errorf(codeInternal, fqn, "継ぎ目のデコレータ %s が Program にない（生成器の不具合）", fqn)
			return nil
		}
		decs[fqn] = d
		name := golang.SeamCasesName(fqn)
		f.print(comment(name + "Cases は、デコレータ " + fqn + " の、結果ごとのケース。結果の数だけ、メソッドがある。"))
		f.printf("type %sCases interface {\n", name)
		for _, r := range e.seamResults(d) {
			if r.exception == "" {
				f.printf("\t// OK は、op の中身を、ちょうど一度呼んで、その結果を、そのまま返すケース。\n\tOK() %sCase\n", name)
				continue
			}
			note := ""
			switch seamRelation(d, r.exception) {
			case "prevents":
				note = "（prevents effects: op の中身を、呼ばない）"
			case "keeps":
				note = "（keeps effects: op の中身を、呼んでいる）"
			}
			f.printf("\t// %s は、例外 %s を投げるケース%s。\n\t%s() %sCase\n", r.name, r.exception, note, r.name, name)
		}
		f.print("}\n\n")
		f.print(comment(name + "Case は、デコレータ " + fqn + " の、ケース一つ: その結果になる状態の、継ぎ目の実装。"))
		f.printf("type %sCase struct {\n\t// Seams は、継ぎ目の実装を返す。走らせる側が、付けた op ごとに、一度呼ぶ。\n\tSeams func() %sSeams\n}\n\n", name, entry)
	}

	f.print(comment("SeamCases は、継ぎ目を持つデコレータの、テストのケース。デコレータごとに、メソッド一つ。\n" +
		"メソッドが一つでも足りない値は、RunSeams に渡すところでコンパイルできない。"))
	f.print("type SeamCases interface {\n")
	for _, fqn := range decorators {
		name := golang.SeamCasesName(fqn)
		f.printf("\t%s() %sCases\n", name, name)
	}
	f.print("}\n\n")

	doc := "RunSeams は、継ぎ目ごと（付けた op と、デコレータの組）、結果ごとに、テストを一つ走らせる。テストの名前は、\n" +
		"<op の FQN>@<デコレータの FQN>/<結果>。ケースごとに、継ぎ目を、にせものの中身（呼ばれた回数を数え、零値を返す）と一緒に呼び、\n" +
		"結果が、そのケースのものであることを確かめる。"
	if catching {
		doc += "\n例外を受けるデコレータの継ぎ目には、受ける例外ごとのテスト（<op の FQN>@<デコレータの FQN>/catches/<例外の名前>）を足す:\n" +
			"正常のケースの継ぎ目を、その例外を返す、にせものの中身と一緒に呼び、デコレータの宣言のとおりに終わることを確かめる。"
	}
	f.print(comment(doc))
	f.printf("func RunSeams(t *%s.T, c SeamCases) {\n", testing)
	ctxPkg := f.im.Add("context", "context")
	reserved := map[string]bool{}
	for k, v := range e.reserved {
		reserved[k] = v
	}
	for _, n := range names {
		reserved[n] = true
	}
	for _, sm := range e.shape.Seams {
		o, d := e.op(sm.Op), decs[sm.Decorator]
		if o == nil || d == nil {
			continue
		}
		name := golang.SeamCasesName(sm.Decorator)
		params, results := golang.Signature(o, f.im, reserved)
		args := []string{ctxPkg + ".Background()"}
		for _, p := range o.Params {
			args = append(args, f.zero(p.Type))
		}
		ret, lhs := "return nil", "err"
		if hasValue(o) {
			ret, lhs = "return "+f.zero(o.Returns)+", nil", "_, err"
		}
		head := lhs + " := k.Seams()." + sm.Method + "(" + strings.Join(args, ", ") + ", func(" + params + ") " + results + " {\ncalls++\n"
		call := head + ret + "\n})"
		f.printf("t.Run(%s, func(t *%s.T) {\ncases := c.%s()\n", strconv.Quote(sm.Op+"@"+sm.Decorator), testing, name)
		for _, r := range e.seamResults(d) {
			f.printf("t.Run(%s, func(t *%s.T) {\nk := cases.%s()\ncalls := 0\n%s\n", strconv.Quote(r.name), testing, r.name, call)
			if r.exception == "" {
				f.print("if err != nil {\nt.Fatalf(\"want no error, got %v\", err)\n}\n")
				f.print("if calls != 1 {\nt.Fatalf(\"want the body called once, got %d\", calls)\n}\n")
			} else {
				f.printf("var want %s\nif !%s.As(err, &want) {\nt.Fatalf(\"want exception %s, got %%v\", err)\n}\n",
					f.goType(ir.Ref(r.exception)), f.im.Add("errors", "errors"), r.exception)
				switch seamRelation(d, r.exception) {
				case "prevents":
					f.print("if calls != 0 {\nt.Fatalf(\"want the body not called (prevents effects), got %d\", calls)\n}\n")
				case "keeps":
					f.print("if calls != 1 {\nt.Fatalf(\"want the body called once (keeps effects), got %d\", calls)\n}\n")
				default:
					f.print("_ = calls\n")
				}
			}
			f.print("})\n")
		}
		e.genSeamCatches(f, o, sm.Decorator, d, head, testing)
		f.print("})\n")
	}
	f.print("}\n")
	return f
}

// genSeamCatches は、継ぎ目一つの、受ける例外ごとのテスト（docs/plugin-go.md「デコレータのテストの口」の「受けるケース」）:
// t.Run("catches", …) の中に、例外ごとの t.Run。受ける例外がなければ、何も出さない。head は、継ぎ目を呼ぶ式の、にせものの中身の
// return の手前まで。
func (e *emitter) genSeamCatches(f *fileGen, o *ir.Op, fqn string, d *ir.Decorator, head, testing string) {
	catches, throws := e.seamCatches(o, fqn, d)
	if len(catches) == 0 {
		return
	}
	var clause ir.ThrowsClause
	for _, c := range catches {
		clause.Exceptions = append(clause.Exceptions, c.exception)
	}
	results := e.leafResults(&ir.Op{Throws: []ir.ThrowsClause{clause}})[1:] // 先頭は OK。名前の決まりは、結果と同じ
	f.printf("t.Run(\"catches\", func(t *%s.T) {\n", testing)
	for i, c := range catches {
		x := c.exception
		ret := "return " + f.goType(ir.Ref(x)) + "{}"
		if hasValue(o) {
			ret = "return " + f.zero(o.Returns) + ", " + f.goType(ir.Ref(x)) + "{}"
		}
		f.printf("t.Run(%s, func(t *%s.T) {\nk := cases.OK()\ncalls := 0\n%s%s\n})\n", strconv.Quote(results[i].name), testing, head, ret)
		thrown := false // 受けた例外が、デコレータの throws にもある（DEC-6 の warning が出る宣言）
		for _, y := range throws {
			thrown = thrown || y == x
		}
		noError := c.mode == ir.Ignores || (c.mode == ir.Recovers && len(throws) == 0)
		switch {
		case noError:
			f.printf("if err != nil {\nt.Fatalf(\"want no error (catches %s %s), got %%v\", err)\n}\n", x, c.mode)
		default:
			errorsPkg := f.im.Add("errors", "errors")
			if !thrown {
				f.printf("var caught %s\nif %s.As(err, &caught) {\nt.Fatalf(\"want exception %s caught (%s), got %%v\", err)\n}\n",
					f.goType(ir.Ref(x)), errorsPkg, x, c.mode)
			}
			if c.mode == ir.Recovers {
				cond := "err != nil"
				for j, y := range throws {
					f.printf("var want%d %s\n", j, f.goType(ir.Ref(y)))
					cond += " && !" + errorsPkg + ".As(err, &want" + strconv.Itoa(j) + ")"
				}
				f.printf("if %s {\nt.Fatalf(\"want no error or one of the decorator's exceptions (%s), got %%v\", err)\n}\n", cond, strings.Join(throws, ", "))
			}
		}
		f.print("if calls != 1 {\nt.Fatalf(\"want the body called once, got %d\", calls)\n}\n")
		f.print("})\n")
	}
	f.print("})\n")
}
