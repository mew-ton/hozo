package gobase

import (
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは、デコレータのテストの口（docs/plugin-go.md「デコレータのテストの口」）: 単位の継ぎ目にあるデコレータの、結果ごとの
// ケース（<D>Cases、<D>Case）、まとめの SeamCases、走らせる RunSeams。

const fileSeams = "seams.go"

// seamBodyNames は、seams.go の関数の本体が、決まった名前で使う識別子。import の名前に使わない。
var seamBodyNames = []string{"t", "c", "k", "cases", "calls", "err", "want", "ctx"}

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

// genTestSeams はテストのパッケージの seams.go。対象のデコレータが一つもなければ nil。
func (e *emitter) genTestSeams() *fileGen {
	decorators := golang.SeamDecorators(e.shape)
	if len(decorators) == 0 {
		return nil
	}
	f := e.newTestFile(fileSeams, seamBodyNames)
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

	f.print(comment("RunSeams は、継ぎ目ごと（付けた op と、デコレータの組）、結果ごとに、テストを一つ走らせる。テストの名前は、\n" +
		"<op の FQN>@<デコレータの FQN>/<結果>。ケースごとに、継ぎ目を、にせものの中身（呼ばれた回数を数え、零値を返す）と一緒に呼び、\n" +
		"結果が、そのケースのものであることを確かめる。"))
	f.printf("func RunSeams(t *%s.T, c SeamCases) {\n", testing)
	ctxPkg := f.im.Add("context", "context")
	reserved := map[string]bool{}
	for k, v := range e.reserved {
		reserved[k] = v
	}
	for _, n := range seamBodyNames {
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
		call := lhs + " := k.Seams()." + sm.Method + "(" + strings.Join(args, ", ") + ", func(" + params + ") " + results + " {\ncalls++\n" + ret + "\n})"
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
		f.print("})\n")
	}
	f.print("}\n")
	return f
}
