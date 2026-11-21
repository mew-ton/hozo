package gobase

import (
	"sort"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは、テストのパッケージ（docs/plugin-go.md「単体テストの口」）: 人か AI が中身を書く関数に出す、結果ごとの単体テストを
// 受け取る口と、それを走らせる関数。葉の op（leaves.go）と、述語で決まる値の分類・op の分類の判定（classify.go）。
// テストのパッケージは、N の入口のパッケージを import する。

// テストのパッケージのファイル名（docs/plugin-go.md「ファイル」）。doc.go と classify.go は、定義のパッケージと同じ名前。
const fileLeaves = "leaves.go"

// testBodyNames は、テストのパッケージの関数の本体が、決まった名前で使う識別子（docs/plugin-go.md「単体テストの口」の
// 「テストのパッケージ」）。import の名前に使わない。
var testBodyNames = []string{"t", "c", "x", "k", "cases", "leaves", "got", "err", "want", "kind", "none"}

// newTestFile は、テストのパッケージのファイル name を作り始める。reserved は、そのファイルの本体が、決まった名前で使う識別子
// （import の名前に使わない）。
func (e *emitter) newTestFile(name string, reserved []string) *fileGen {
	f := &fileGen{e: e, name: name, inTest: true, pkg: e.layout.TestName, key: "test/" + name}
	f.im = e.imports[f.key]
	if f.im == nil {
		f.im = golang.NewTestImports(e.view, e.layout, e.root)
		f.im.Reserve(reserved...)
		e.imports[f.key] = f.im
	}
	return f
}

// testClassifies は、判定の口を出す分類: N が宣言した、述語で決まる値の分類と op の分類（宣言順）。状態の分類には、出さない。
func (e *emitter) testClassifies() []*ir.Classify {
	var out []*ir.Classify
	for _, c := range e.predicateClassifies() {
		if _, isState := c.SubjectStates(); !isState {
			out = append(out, c)
		}
	}
	return out
}

// hasTests は、テストのパッケージを作るか: 葉の op か、判定の口を出す分類か、デコレータのテストの口の対象が、一つでもあるか、
// fakes.go を出す。
func (e *emitter) hasTests() bool {
	return len(e.shape.Leaves) > 0 || len(e.testClassifies()) > 0 || len(e.shape.Seams) > 0 || e.hasFakes()
}

// leafResult は、葉の結果一つ（docs/plugin-go.md「単体テストの口」の「葉の op」）: 正常か、例外一つ。name は、<Op>Cases の
// メソッドの名前で、テストの名前の <結果> でもある。exception は、例外の FQN（正常なら ""）。
type leafResult struct {
	name, exception string
}

// leafResults は、葉 o の結果: 正常（OK）と、o が throws に書いた例外ごと（書かれた順。同じ例外は、一度だけ）。名前は、例外の
// 宣言の名前。その op の中で名前が当たるもの（別の名前空間の、同じ名前の例外。例外 OK）は、名前空間のセグメントを先頭大文字に
// して前に付ける。
func (e *emitter) leafResults(o *ir.Op) []leafResult {
	out := []leafResult{{name: "OK"}}
	seen := map[string]bool{}
	count := map[string]int{"OK": 1}
	for _, c := range o.Throws {
		for _, x := range c.Exceptions {
			if seen[x] || e.lookup(x) == nil {
				continue
			}
			seen[x] = true
			name := e.typeName(x)
			count[name]++
			out = append(out, leafResult{name: name, exception: x})
		}
	}
	for i := range out {
		if out[i].exception == "" || count[out[i].name] == 1 {
			continue
		}
		var prefix string
		for _, seg := range strings.Split(e.declNS[out[i].exception], ".") {
			prefix += golang.Capitalize(seg)
		}
		out[i].name = prefix + out[i].name
	}
	return out
}

// caseField は、<Op>Case の、引数 name のフィールドの名前: フィールド名と同じ決まり。Then と当たれば Then_。
func caseField(name string) string {
	if f := golang.FieldName(name); f != "Then" {
		return f
	}
	return "Then_"
}

// hasValue は、op o が、値を返すか（戻り値のない op と、never を返す op は、返さない）。
func hasValue(o *ir.Op) bool { return o.Returns != nil && !ir.IsNever(o.Returns) }

// leafClassifies は、葉の戻り値の確かめに使う分類の FQN（辞書順）: 戻り値の型が、そのまま、述語で決まる分類で絞り込んだ型の葉のもの。
func (e *emitter) leafClassifies() []string {
	seen := map[string]bool{}
	var out []string
	for _, fqn := range e.shape.Leaves {
		if c, _, ok := golang.PredicateNarrowing(e.p, e.op(fqn).Returns); ok && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

// genTestDoc はテストのパッケージの doc.go。口が一つもなければ nil。
func (e *emitter) genTestDoc() *fileGen {
	if !e.hasTests() {
		return nil
	}
	f := e.newTestFile(fileDoc, testBodyNames)
	f.doc = comment("Package " + e.layout.TestName + " は、名前空間 " + e.view.Viewer +
		" の、テストのためのもの: 単体テストの口と、にせもの（docs/plugin-go.md「単体テストの口」「テストのにせもの」）。")
	return f
}

// genTestLeaves はテストのパッケージの leaves.go: 葉ごとの <Op>Cases と <Op>Case、まとめの LeafCases、走らせる RunLeaves。
// 葉が一つもなければ nil。
func (e *emitter) genTestLeaves() *fileGen {
	s := e.shape
	if len(s.Leaves) == 0 {
		return nil
	}
	f := e.newTestFile(fileLeaves, testBodyNames)
	testing := f.im.Add("testing", "testing")
	entry := f.im.Qualifier(e.view.Viewer)
	for _, fqn := range s.Leaves {
		o := e.op(fqn)
		m := golang.MethodName(fqn)
		f.print(comment(m + "Cases は、葉 " + fqn + " の、結果ごとのケース。結果の数だけ、メソッドがある。"))
		f.printf("type %sCases interface {\n", m)
		for _, r := range e.leafResults(o) {
			if r.exception == "" {
				f.printf("\t// OK は、正常に終わるケース。\n\tOK() %sCase\n", m)
				continue
			}
			f.printf("\t// %s は、例外 %s を投げるケース。\n\t%s() %sCase\n", r.name, r.exception, r.name, m)
		}
		f.print("}\n\n")
		f.print(comment(m + "Case は、葉 " + fqn + " の、ケース一つ: 渡す引数と、足りない確かめ。"))
		f.printf("type %sCase struct {\n", m)
		for _, p := range o.Params {
			f.printf("\t%s %s\n", caseField(p.Name), f.goType(p.Type))
		}
		if hasValue(o) {
			f.printf("\t// Then は、結果が、そのケースのものだと確かめた後に、呼ばれる（nil なら、呼ばない）。got は、葉が返した値、err は、葉が返した失敗（正常のケースでは nil）。\n"+
				"\tThen func(t *%s.T, got %s, err error)\n", testing, f.goType(o.Returns))
		} else {
			f.printf("\t// Then は、結果が、そのケースのものだと確かめた後に、呼ばれる（nil なら、呼ばない）。err は、葉が返した失敗（正常のケースでは nil）。\n"+
				"\tThen func(t *%s.T, err error)\n", testing)
		}
		f.print("}\n\n")
	}

	// LeafCases
	f.print(comment("LeafCases は、人が実装する葉の op の、単体テストのケース。葉ごとに、メソッド一つ（Leaves と同じ並び）。\n" +
		"メソッドが一つでも足りない値は、RunLeaves に渡すところでコンパイルできない。"))
	f.print("type LeafCases interface {\n")
	for _, fqn := range s.Leaves {
		m := golang.MethodName(fqn)
		f.printf("\t%s() %sCases\n", m, m)
	}
	f.print("}\n\n")

	// RunLeaves。述語の引数の名前は、本体が使う名前と、このファイルが import するパッケージの名前を避ける
	taken := map[string]bool{}
	for _, n := range testBodyNames {
		taken[n] = true
	}
	for _, n := range f.im.Names() {
		taken[n] = true
	}
	preds := map[string]string{} // 分類の FQN → 引数の名前
	params := []string{"t *" + testing + ".T", "leaves " + entry + "Leaves"}
	for _, fqn := range e.leafClassifies() {
		name := golang.LocalName(strings.ToLower(short(fqn)[:1])+short(fqn)[1:], nil)
		for taken[name] {
			name += "_"
		}
		taken[name] = true
		preds[fqn] = name
		params = append(params, name+" "+f.im.Qualifier(e.declNS[fqn])+e.typeName(fqn)+"Cases")
	}
	params = append(params, "c LeafCases")
	f.print(comment("RunLeaves は、葉ごと、結果ごとに、テストを一つ走らせる。テストの名前は、<op の FQN>/<結果>。\n" +
		"ケースごとに、leaves の葉を呼び、結果が、そのケースのものであることを確かめてから、Then を呼ぶ。"))
	f.printf("func RunLeaves(%s) {\n", strings.Join(params, ", "))
	ctx := f.im.Add("context", "context") + ".Background()"
	for _, fqn := range s.Leaves {
		o := e.op(fqn)
		m := golang.MethodName(fqn)
		args := []string{ctx}
		for _, p := range o.Params {
			args = append(args, "k."+caseField(p.Name))
		}
		call, then := "err := leaves."+m+"("+strings.Join(args, ", ")+")", "k.Then(t, err)"
		if hasValue(o) {
			call, then = "got, "+call, "k.Then(t, got, err)"
		}
		f.printf("t.Run(%s, func(t *%s.T) {\ncases := c.%s()\n", strconv.Quote(fqn), testing, m)
		for _, r := range e.leafResults(o) {
			f.printf("t.Run(%s, func(t *%s.T) {\nk := cases.%s()\n%s\n", strconv.Quote(r.name), testing, r.name, call)
			if r.exception == "" {
				f.print("if err != nil {\nt.Fatalf(\"want no error, got %v\", err)\n}\n")
				if classify, ks, ok := golang.PredicateNarrowing(e.p, o.Returns); ok {
					e.genLeafNarrowing(f, o, classify, ks, preds[classify], ctx)
				}
			} else {
				f.printf("var want %s\nif !%s.As(err, &want) {\nt.Fatalf(\"want exception %s, got %%v\", err)\n}\n",
					f.goType(ir.Ref(r.exception)), f.im.Add("errors", "errors"), r.exception)
			}
			f.printf("if k.Then != nil {\n%s\n}\n})\n", then)
		}
		f.print("})\n")
	}
	f.print("}\n")
	return f
}

// genLeafNarrowing は、葉 o の、正常のケースの、戻り値の確かめ: 戻り値の型が、述語で決まる分類 classify のケース ks で絞り込んだ
// 型なら、判定関数を呼び、返ったケースが、ks のどれかであることを確かめる。pred は、述語の引数の名前。
func (e *emitter) genLeafNarrowing(f *fileGen, o *ir.Op, classify string, ks []string, pred, ctx string) {
	cl, _ := e.lookup(classify).(*ir.Classify)
	q := f.im.Qualifier(e.declNS[classify])
	tn := e.typeName(classify)
	var conds []string
	for _, k := range ks {
		conds = append(conds, "kind != "+q+golang.CaseConst(tn, k))
	}
	v := e.classifyArg(f, cl, "got", f.goType(o.Returns), o.Name)
	f.printf("if kind, err := %sClassify%s(%s, %s, %s); err != nil {\nt.Fatalf(\"want case %s of %s, got error %%v\", err)\n} else if %s {\n"+
		"t.Fatalf(\"want case %s of %s, got %%s\", [...]string{%s}[kind])\n}\n",
		q, tn, ctx, pred, v, strings.Join(ks, " or "), classify, strings.Join(conds, " && "), strings.Join(ks, " or "), classify, caseNames(cl))
}

// genTestClassify はテストのパッケージの classify.go: 分類ごとの <C>Examples と Run<C>。判定の口を出す分類がなければ nil。
func (e *emitter) genTestClassify() *fileGen {
	cs := e.testClassifies()
	if len(cs) == 0 {
		return nil
	}
	f := e.newTestFile(fileClassify, testBodyNames)
	testing := f.im.Add("testing", "testing")
	entry := f.im.Qualifier(e.view.Viewer)
	ctx := f.im.Add("context", "context") + ".Background()"
	for _, c := range cs {
		name := e.typeName(c.Name)
		subject := f.goType(c.SubjectType())
		f.print(comment(name + "Examples は、分類 " + c.Name + " の判定の、ケースごとの例: そのケースに当たる値。\n" +
			"メソッドが一つでも足りない値は、Run" + name + " に渡すところでコンパイルできない。"))
		f.printf("type %sExamples interface {\n", name)
		for _, cs := range c.Cases {
			if cs.Name == c.Absent {
				continue // ない側のケースは、例を求めない
			}
			m := golang.Capitalize(cs.Name)
			f.printf("\t// %s は、ケース %s に当たる値。\n\t%s() %s\n", m, cs.Name, m, subject)
		}
		f.print("}\n\n")
		f.print(comment("Run" + name + " は、ケースごとに、テストを一つ走らせる。テストの名前は、<分類の FQN>/<ケース>。\n" +
			"ケースごとに、例の値を、判定関数 Classify" + name + " に渡し、そのケースになることを確かめる。"))
		f.printf("func Run%s(t *%s.T, cases %s%sCases, x %sExamples) {\nt.Run(%s, func(t *%s.T) {\n", name, testing, entry, name, name, strconv.Quote(c.Name), testing)
		for _, cs := range c.Cases {
			f.printf("t.Run(%s, func(t *%s.T) {\n", strconv.Quote(cs.Name), testing)
			value := "x." + golang.Capitalize(cs.Name) + "()"
			if cs.Name == c.Absent {
				// ない側のケース: 値がないことを渡す
				f.printf("var none %s\n", subject)
				value = "none"
			}
			f.printf("kind, err := %sClassify%s(%s, cases, %s)\nif err != nil {\nt.Fatalf(\"want case %s, got error %%v\", err)\n}\n", entry, name, ctx, value, cs.Name)
			f.printf("if kind != %s%s {\nt.Fatalf(\"want case %s, got %%s\", [...]string{%s}[kind])\n}\n})\n", entry, golang.CaseConst(name, cs.Name), cs.Name, caseNames(c))
		}
		f.print("})\n}\n\n")
	}
	return f
}

// checkTestNames は GO-2 のうち、テストのパッケージの分: 公開の名前どうしと、<Op>Case のフィールドどうし。公開の名前の一覧は、
// このパッケージにファイルを足す相乗りと共有する golang.PublicTestNames。
func (e *emitter) checkTestNames() {
	if !e.hasTests() {
		return
	}
	pkg := e.names("テストのパッケージ（" + e.layout.TestDir + "）")
	for _, n := range golang.PublicTestNames(e.view) {
		pkg.add(n.Name, n.What, n.Decl)
	}
	for _, fqn := range e.shape.Leaves {
		fields := e.names("op " + fqn + " のケースの構造体")
		fields.add("Then", "足りない確かめ Then", "")
		for _, p := range e.op(fqn).Params {
			fields.add(caseField(p.Name), "引数 "+p.Name, fqn)
		}
	}
}

// checkTestDir は GO-4 のうち、テストのパッケージの分: テストのパッケージのディレクトリが、同じ単位が出力する子の名前空間
// N.<テストのパッケージの名前> の出力先と重なる。位置は、口を持つ最初の宣言。
func (e *emitter) checkTestDir() {
	if !e.hasTests() {
		return
	}
	child := e.view.Viewer + "." + e.layout.TestName
	for _, c := range e.view.Namespaces[child] {
		if c.Index != e.view.Config.Index {
			continue
		}
		at := ""
		if len(e.shape.Leaves) > 0 {
			at = e.shape.Leaves[0]
		} else if cs := e.testClassifies(); len(cs) > 0 {
			at = cs[0].Name
		}
		e.errorf(codeInnerDir, at, "名前空間 %s は、単体テストの口を、テストのパッケージ %s に置くが、同じ単位（%d 番目の [[emit]]）が出力する子の名前空間 %s の出力先と重なる",
			e.view.Viewer, e.layout.TestDir, c.Index, child)
		return
	}
}
