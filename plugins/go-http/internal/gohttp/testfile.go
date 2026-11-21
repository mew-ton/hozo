package gohttp

import (
	"fmt"
	"go/format"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは、テストのパッケージの http.go（docs/plugin-go-http.md「インテグレーションテストの口」）: 経路の op ごとの、結果ごとの
// インテグレーションテストを受け取る口と、それを、HTTP の層を通して走らせる関数。検査（emit.go、inputs.go）を通った入力だけを描く。
// 期待する出力の実物は、サンプル tests/samples/shop-server/ の gen/server/servertest/http.go。
//
// 入口ごと、結果ごと、フィールドごとに出す行は、デコレータの引数（と、フィールドの名前）と、op が投げうる例外の集まりだけから作る
// （docs/requirements.md「出力を推論しない」）。型を書くのは、口の宣言（入力の model と、結果の型）と、例外の見分けの型引数だけ。

// TestFileName は、このトランスパイラが、N のテストのパッケージに出すファイルの名前（docs/plugin-go.md「相乗りのファイル」）。
const TestFileName = "http.go"

// テストのパッケージに足す、決まった公開の名前（docs/plugin-go-http.md「インテグレーションテストの口」の「置き場と名前」）。
const (
	casesName = "HTTPCases" // 入口のケースのまとめ。入口ごとの口は <Op>HTTPCases、ケース一つは <Op>HTTPCase
	runName   = "RunHTTP"   // 走らせる関数
)

// testBodyNames は、テストのパッケージの http.go の関数の本体が、決まった名前で使う識別子（同「置き場と名前」）。import の名前に使わない。
var testBodyNames = []string{"t", "c", "k", "f", "cases", "send", "force", "call", "res", "got", "method", "path"}

// group は、入口が投げうる例外一つと、その出どころ（同「必要なケース」）。name は、例外の名前で、テストの名前の <例外の名前>。
// status は、@http.status を持つか（持てば、code が、そのステータス）。
type group struct {
	name, exception string
	status          bool
	code            int64
	sources         []source
}

// source は、例外の出どころ一つ。made は、Hozo が、ケースを作るか。作らないなら、method が、口のメソッドの名前。
type source struct {
	golang.ThrowSource
	made   bool
	method string
}

// entry は、入口一つ: 経路の op と、その経路（書かれた順）と、テスト（正常、op を呼ぶ前の失敗、例外の出どころごと）。
type entry struct {
	op     *ir.Op
	routes []*route
	hasOK  bool // 正常のケースがある（never を返す op には、ない）
	hasBad bool // op を呼ぶ前の失敗のケースがある
	groups []group
}

// humanCase は、人か AI が書くケース一つ（<Op>HTTPCases のメソッド）: 名前と、コメントの文。
type humanCase struct{ name, comment string }

// human は、入口 e の、人か AI が書くケース（口のメソッドの順）: 正常、op を呼ぶ前の失敗、Hozo が作らない出どころ。
func (e *entry) human() []humanCase {
	var out []humanCase
	if e.hasOK {
		out = append(out, humanCase{"OK", "OK は、正常に終わるケース: 成功の応答。"})
	}
	if e.hasBad {
		out = append(out, humanCase{"BadRequest", "BadRequest は、op を呼ぶ前に、要求を断るケース: 400 と http.BadRequest。"})
	}
	for _, g := range e.groups {
		what := "500（@http.status を持たない例外は、中身を出さない）"
		if g.status {
			what = strconv.FormatInt(g.code, 10)
		}
		for _, s := range g.sources {
			if s.made {
				continue
			}
			if s.Kind == golang.SourceUnknown {
				out = append(out, humanCase{s.method, s.method + " は、例外 " + g.exception + " を投げるケース: " + what + "。"})
				continue
			}
			out = append(out, humanCase{s.method, s.method + " は、例外 " + g.exception + " を、" + s.Label + " が投げるケース: " + what + "。"})
		}
	}
	return out
}

// method は、op のメソッドの名前。口の名前 <Op>HTTPCases と <Op>HTTPCase の <Op>。
func (e *entry) method() string { return golang.MethodName(e.op.Name) }

// model と inputs は、入力の model と、そのフィールドの読み方（どの経路も同じ: 入力は、op のもの）。
func (e *entry) model() *ir.Model { return e.routes[0].model }
func (e *entry) inputs() []input  { return e.routes[0].inputs }

// hasValue は、op が、値を返すか（戻り値のない op と、never を返す op は、返さない）。
func (e *entry) hasValue() bool { return e.op.Returns != nil && !ir.IsNever(e.op.Returns) }

// entries は、経路 routes を、op ごとにまとめる（op の宣言順。routes は、op ごとに続いて並んでいる）。
func (g *generator) entries(routes []route) []entry {
	var out []entry
	for i := range routes {
		r := &routes[i]
		if n := len(out); n > 0 && out[n-1].op == r.op {
			out[n-1].routes = append(out[n-1].routes, r)
			continue
		}
		out = append(out, entry{op: r.op, routes: []*route{r}})
	}
	for i := range out {
		g.tests(&out[i])
	}
	return out
}

// tests は、入口 e のテストを決める（docs/plugin-go-http.md「インテグレーションテストの口」の「必要なケース」）: 正常（never を返す op には、
// ない）。op を呼ぶ前の失敗（入力が、本文のデコレータのフィールドか、@http.required の付いたフィールドを、一つでも持つ op だけ。
// 型は見ない）。op が投げうる例外（Op.signature.exceptions の順。デコレータが運ぶ例外を含む）の、出どころごと（golang.ThrowSources）。
// 例外の名前は、宣言の名前。その op の中で名前が当たるもの（別の名前空間の、同じ名前の例外。例外 OK、BadRequest）は、名前空間の
// セグメントを先頭大文字にして前に付ける（基底の、葉の op の口と同じ決まり）。
func (g *generator) tests(e *entry) {
	count := map[string]int{}
	if !ir.IsNever(e.op.Returns) {
		e.hasOK = true
		count["OK"]++
	}
	for _, in := range e.inputs() {
		if in.place == inBody || in.required {
			e.hasBad = true
			count["BadRequest"]++
			break
		}
	}
	sources := golang.ThrowSources(g.p, g.kit, e.op.Name)
	seen := map[string]bool{}
	for _, fqn := range e.op.Signature.Exceptions {
		x, ok := g.p.Lookup(fqn).(*ir.Exception)
		if !ok || seen[fqn] {
			continue
		}
		seen[fqn] = true
		gr := group{name: golang.TypeName(fqn, g.declNS[fqn]), exception: fqn}
		gr.code, gr.status = statusOf(x)
		for _, s := range sources {
			if s.Exception == fqn {
				// Hozo が作るのは、作れる出どころで、正常のケースがあり、にせものに言うことがあるなら、Fakes があるとき
				made := s.Made && e.hasOK && (len(s.Steps) == 0 || g.kit.HasFakes())
				gr.sources = append(gr.sources, source{ThrowSource: s, made: made})
			}
		}
		count[gr.name]++
		e.groups = append(e.groups, gr)
	}
	for i := range e.groups {
		gr := &e.groups[i]
		if count[gr.name] > 1 {
			var prefix string
			for _, seg := range strings.Split(g.declNS[gr.exception], ".") {
				prefix += golang.Capitalize(seg)
			}
			gr.name = prefix + gr.name
		}
		for j := range gr.sources {
			gr.sources[j].method = gr.name + gr.sources[j].Ident
		}
	}
}

// statusOf は、例外 x の @http.status のステータス。持たなければ false。
func statusOf(x *ir.Exception) (int64, bool) {
	for i := range x.Decorators {
		app := &x.Decorators[i]
		if app.Decorator != httpStatus {
			continue
		}
		if l, ok := argLiteral(app, "code"); ok {
			if n, ok := l.(ir.IntLiteral); ok {
				return n.Value, true
			}
		}
	}
	return 0, false
}

// inputField は、<Op>HTTPCase の、入力のフィールドの名前: op の引数の名前を、フィールド名と同じ決まりで。Given、Request、Then と
// 当たれば、末尾に _。
func inputField(e *entry) string {
	name := golang.FieldName(e.op.Params[0].Name)
	switch name {
	case "Given", "Request", "Then":
		return name + "_"
	}
	return name
}

// checkTestNames は GOHTTP-20: テストのパッケージに足す公開の名前が、そのパッケージの、基底の公開の名前（golang.PublicTestNames）か、
// 自分のほかの名前と当たる。<Op>HTTPCases のメソッドの名前（結果の名前）どうしが当たるときも。位置は、当たった自分の名前を生んだ
// 経路の op。決まった名前（HTTPCases、RunHTTP）が当たったなら、相手の名前を生んだ宣言。
func (g *generator) checkTestNames(entries []entry) {
	type owner struct{ what, decl string }
	taken := map[string]owner{}
	for _, n := range golang.PublicTestNames(g.view) {
		if _, dup := taken[n.Name]; !dup {
			taken[n.Name] = owner{"基底 go の、" + n.What, n.Decl}
		}
	}
	add := func(name, what, decl string) {
		prev, dup := taken[name]
		if !dup {
			taken[name] = owner{what, decl}
			return
		}
		at := decl
		if at == "" {
			at = prev.decl
		}
		g.errorf(codeHTTPTestName, at, "%s が、%s と当たる（どちらも、テストのパッケージの名前 %s。直すのは仕様の名前）", what, prev.what, name)
	}
	add(casesName, TranspilerName+" の、入口のケースのまとめ "+casesName, "")
	add(runName, TranspilerName+" の、入口のケースを走らせる "+runName, "")
	for i := range entries {
		e := &entries[i]
		m := e.method()
		add(m+casesName, TranspilerName+" の、op "+e.op.Name+" のケースの口 "+m+casesName, e.op.Name)
		add(m+"HTTPCase", TranspilerName+" の、op "+e.op.Name+" のケース "+m+"HTTPCase", e.op.Name)
		// 例外の名前（テストの名前の段）どうし
		groups := map[string]string{}
		for _, gr := range e.groups {
			if prev, dup := groups[gr.name]; dup {
				g.errorf(codeHTTPTestName, e.op.Name, "op %s の、例外 %s の名前 %s が、例外 %s の名前と当たる（名前空間を前に付けても、同じ名前。直すのは仕様の名前）",
					e.op.Name, gr.exception, gr.name, prev)
				continue
			}
			groups[gr.name] = gr.exception
		}
		methods := map[string]bool{}
		for _, h := range e.human() {
			if methods[h.name] {
				g.errorf(codeHTTPTestName, e.op.Name, "op %s のケースの口 %s%s の、メソッドの名前 %s が、二つのケースで当たる（直すのは仕様の名前）",
					e.op.Name, m, casesName, h.name)
				continue
			}
			methods[h.name] = true
		}
	}
}

// checkTestDir は GOHTTP-21: テストのパッケージのディレクトリが、同じ単位が出力する子の名前空間 N.<テストのパッケージの名前> の
// 出力先と重なる。基底が、同じことを GO-4 で返す出力（基底も、テストのパッケージを作る）では、返さない。位置は、最初の経路の op。
// 置き場が要るので、go.mod がない（layout が nil）ときは、見ない（基底が GO-1 を返し、何も書き出されない）。
func (g *generator) checkTestDir(entries []entry) {
	if g.layout == nil || len(golang.PublicTestNames(g.view)) > 0 {
		return
	}
	child := g.view.Viewer + "." + g.layout.TestName
	for _, c := range g.view.Namespaces[child] {
		if c.Index != g.view.Config.Index {
			continue
		}
		g.errorf(codeHTTPTestDir, entries[0].op.Name, "名前空間 %s は、インテグレーションテストの口を、テストのパッケージ %s に置くが、同じ単位（%d 番目の [[emit]]）が出力する子の名前空間 %s の出力先と重なる",
			g.view.Viewer, g.layout.TestDir, c.Index, child)
		return
	}
}

// testFile は、描いている途中の、テストのパッケージの http.go。
type testFile struct {
	g  *generator
	im *golang.Imports
	// renamed は、決まった名前で書くパッケージ（helperPackages）が、別の名前で import されたもの（あれば、生成器の不具合）。
	renamed []string
}

// use は、決まった名前で書くパッケージ path を import に足し、その名前を返す。
func (f *testFile) use(path string) string {
	want := helperPackages[path]
	if got := f.im.Add(path, want); got != want {
		f.renamed = append(f.renamed, fmt.Sprintf("%s が %s になった", path, got))
	}
	return want
}

// renderTest は、テストのパッケージの http.go の全体を作り、go/format で整形する。import の名前を文書の決まりの順で決めるために、
// 二度描く（render と同じ手順）。
func (g *generator) renderTest(entries []entry) (string, error) {
	where := g.layout.TestName + "/" + TestFileName
	im := golang.NewTestImports(g.view, g.layout, g.root)
	im.Reserve(testBodyNames...)
	first := &testFile{g: g, im: im}
	first.draw(entries)
	second := &testFile{g: g, im: first.im.Canonical()}
	text := second.draw(entries)
	if err := second.im.Err(); err != nil {
		return "", fmt.Errorf("%s: %w（CFG-22 が通したはずのもの）", where, err)
	}
	if len(second.renamed) > 0 {
		return "", fmt.Errorf("%s: 補助が使うパッケージの import の名前が変わった（生成器の不具合）: %s", where, strings.Join(second.renamed, "、"))
	}
	src, err := format.Source([]byte(text))
	if err != nil {
		return "", fmt.Errorf("生成した %s を整形できない（生成器の不具合）: %w\n%s", where, err, text)
	}
	return string(src), nil
}

// draw は、ファイルのテキスト（整形前）を作る: ヘッダ、package 節、import、入口ごとの口、まとめ、走らせる関数、補助。
func (f *testFile) draw(entries []entry) string {
	var body strings.Builder
	for i := range entries {
		body.WriteString(f.cases(&entries[i]))
	}
	body.WriteString(f.all(entries))
	body.WriteString(f.run(entries))
	body.WriteString(f.helpers(entries))
	var b strings.Builder
	b.WriteString("// Code generated by hozoc " + TranspilerName + " " + plugin.Version + "; DO NOT EDIT.\n\n")
	b.WriteString("package " + f.g.layout.TestName + "\n\n")
	b.WriteString(f.im.Render())
	b.WriteString("\n")
	b.WriteString(body.String())
	return b.String()
}

// cases は、入口一つの口（docs/plugin-go-http.md「インテグレーションテストの口」の「口の形」）: 人か AI が書くケースごとのメソッドを持つ
// <Op>HTTPCases と、ケース一つの <Op>HTTPCase。
func (f *testFile) cases(e *entry) string {
	m := e.method()
	fakes := f.g.kit.HasFakes()
	var paths []string
	for _, r := range e.routes {
		paths = append(paths, r.method+" "+r.path)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "// %s%s は、入口 %s（%s）の、人か AI が書くケース。メソッドごとに、一つ。\ntype %s%s interface {\n",
		m, casesName, e.op.Name, strings.Join(paths, "、"), m, casesName)
	for _, h := range e.human() {
		fmt.Fprintf(&b, "// %s\n%s() %sHTTPCase\n", h.comment, h.name, m)
	}
	b.WriteString("}\n\n")

	testing, httptest := f.use("testing"), f.use("net/http/httptest")
	what := "入力と、流れの端が返す値と、足りない確かめ"
	if !fakes {
		what = "入力と、足りない確かめ"
	}
	if model := e.model(); model != nil {
		ns := f.g.declNS[model.Name]
		fmt.Fprintf(&b, "// %sHTTPCase は、入口 %s の、ケース一つ: %s。\ntype %sHTTPCase struct {\n", m, e.op.Name, what, m)
		fmt.Fprintf(&b, "// %s は、入力。走らせる側が、要求に組み立てる。\n%s %s%s\n", inputField(e), inputField(e), f.im.Qualifier(ns), golang.TypeName(model.Name, ns))
	} else {
		fmt.Fprintf(&b, "// %sHTTPCase は、入口 %s の、ケース一つ: %s。\ntype %sHTTPCase struct {\n", m, e.op.Name, strings.TrimPrefix(what, "入力と、"), m)
	}
	tail, tailDoc := "", ""
	if fakes {
		b.WriteString("// Given は、要求を送る前に、にせものに、流れの端が返す値を言う（nil なら、呼ばない）。\nGiven func(f *Fakes)\n")
		tail, tailDoc = ", f *Fakes", "、f は、にせもの（覚えた呼び出しを、確かめる）"
	}
	fmt.Fprintf(&b, "// Request は、組み立てた要求を、送る前に、手直しする（nil なら、呼ばない）。入力の値では作れない要求に使う。\nRequest func(r *%s.Request)\n", f.use("net/http"))
	if e.hasValue() {
		fmt.Fprintf(&b, "// Then は、応答が、そのケースのものだと確かめた後に、呼ばれる（nil なら、呼ばない）。got は、成功の応答から読んだ値（正常のケースのほかでは、零値）、res は、応答%s。\n"+
			"Then func(t *%s.T, got %s, res *%s.ResponseRecorder%s)\n", tailDoc, testing, golang.GoType(e.op.Returns, f.im), httptest, tail)
	} else {
		fmt.Fprintf(&b, "// Then は、応答が、そのケースのものだと確かめた後に、呼ばれる（nil なら、呼ばない）。res は、応答%s。\n"+
			"Then func(t *%s.T, res *%s.ResponseRecorder%s)\n", tailDoc, testing, httptest, tail)
	}
	b.WriteString("}\n\n")
	return b.String()
}

// all は、入口のケースのまとめ HTTPCases: 入口ごとに、メソッド一つ（宣言順）。
func (f *testFile) all(entries []entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// %s は、@http.get / @http.post の付いた op の、インテグレーションテストのケース。入口ごとに、メソッド一つ（宣言順）。\n"+
		"// メソッドが一つでも足りない値は、%s に渡すところでコンパイルできない。\ntype %s interface {\n", casesName, runName, casesName)
	for i := range entries {
		m := entries[i].method()
		fmt.Fprintf(&b, "%s() %s%s\n", m, m, casesName)
	}
	b.WriteString("}\n\n")
	return b.String()
}

// reals は、RunHTTP が受け取る、人が書いたもの（NewOps が取るもの）の、引数（`<名前> <型>`）と、名前。名前は、このファイルの
// import の名前と、本体が決まった名前で使う識別子を避ける。
func (f *testFile) reals() (params, names []string) {
	taken := map[string]bool{}
	for _, n := range testBodyNames {
		taken[n] = true
	}
	for _, n := range f.im.Names() {
		taken[n] = true
	}
	for _, r := range f.g.kit.Reals {
		name := r.Name
		for taken[name] {
			name += "_"
		}
		taken[name] = true
		typ := f.im.Qualifier(r.Namespace) + "Leaves"
		if r.Kind == golang.KitClassify {
			ns := f.g.declNS[r.Classify]
			typ = f.im.Qualifier(ns) + golang.TypeName(r.Classify, ns) + "Cases"
		}
		params = append(params, name+" "+typ)
		names = append(names, name)
	}
	return params, names
}

// run は、走らせる関数 RunHTTP（同「口の形」）: 入口ごと、結果ごとに、t.Run を重ねる。例外は、その下に、出どころごと。経路を二つ以上
// 持つ op は、テスト一つの下に、経路ごとの段を足す。
func (f *testFile) run(entries []entry) string {
	testing, httptest := f.use("testing"), f.use("net/http/httptest")
	entryPkg := f.im.Qualifier(f.g.view.Viewer)
	fakes := f.g.kit.HasFakes()
	params, names := f.reals()
	handler := entryPkg + "NewHandler(" + entryPkg + "New())"
	if fakes {
		handler = entryPkg + "NewHandler(NewOps(" + strings.Join(append([]string{"f"}, names...), ", ") + "))"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "// %s は、入口ごと、結果ごとに、テストを走らせる。テストの名前は、<op の FQN>/<結果>。例外は、出どころごとに、<op の FQN>/<例外>/<出どころ>。\n", runName)
	if fakes {
		b.WriteString("// ケースごとに、新しいにせものの上に、流れを組み立て（NewOps）、入力を要求に組み立てて送り、応答が、そのケースのものであること\n" +
			"// （ステータスと、本文の形）を確かめてから、Then を呼ぶ。口にない、例外のケースは、正常のケースの用意の後に、にせものに、例外を起こさせる。\n")
	} else {
		b.WriteString("// ケースごとに、新しく、流れを組み立て、入力を要求に組み立てて送り、応答が、そのケースのものであること\n" +
			"// （ステータスと、本文の形）を確かめてから、Then を呼ぶ。\n")
	}
	fmt.Fprintf(&b, "func %s(%s) {\n", runName, strings.Join(append(append([]string{"t *" + testing + ".T"}, params...), "c "+casesName), ", "))
	for i := range entries {
		e := &entries[i]
		m := e.method()
		single := len(e.routes) == 1
		fmt.Fprintf(&b, "t.Run(%s, func(t *%s.T) {\ncases := c.%s()\n", strconv.Quote(e.op.Name), testing, m)
		// 組み立てと、要求の組み立て（同「要求の組み立て」）: フィールドごとに、デコレータと、その引数（か、フィールドの名前）だけで決まる一行
		args, call := "k "+m+"HTTPCase", "httpCall{method: "+strconv.Quote(e.routes[0].method)+", path: "+strconv.Quote(e.routes[0].path)+"}"
		if !single {
			args, call = "method, path string, "+args, "httpCall{method: method, path: path}"
		}
		if fakes {
			fmt.Fprintf(&b, "send := func(t *%s.T, %s, force func(f *Fakes)) (*%s.ResponseRecorder, *Fakes) {\n"+
				"f := new(Fakes)\nif k.Given != nil {\nk.Given(f)\n}\nif force != nil {\nforce(f)\n}\n", testing, args, httptest)
		} else {
			fmt.Fprintf(&b, "send := func(t *%s.T, %s) *%s.ResponseRecorder {\n", testing, args, httptest)
		}
		fmt.Fprintf(&b, "call := %s\n", call)
		for _, x := range e.inputs() {
			value := "k." + inputField(e) + "." + golang.FieldName(x.field.Name)
			name := strings.TrimPrefix(x.decorator, "http.")
			if x.whole {
				fmt.Fprintf(&b, "call.%s(%s)\n", name, value)
			} else {
				fmt.Fprintf(&b, "call.%s(%s, %s)\n", name, strconv.Quote(x.name), value)
			}
		}
		if fakes {
			fmt.Fprintf(&b, "return call.send(t, %s, k.Request), f\n}\n", handler)
		} else {
			fmt.Fprintf(&b, "return call.send(t, %s, k.Request)\n}\n", handler)
		}
		// test は、テスト一つ（か、経路ごとの段）を書く。body は、送る式の前半（"send(t, " か "send(t, <メソッド>, <パス>, "）から、中身を作る
		test := func(name string, body func(sendHead string) string) {
			fmt.Fprintf(&b, "t.Run(%s, func(t *%s.T) {\n", strconv.Quote(name), testing)
			if single {
				b.WriteString(body("send(t, "))
			} else {
				for _, route := range e.routes {
					fmt.Fprintf(&b, "t.Run(%s, func(t *%s.T) {\n%s})\n", strconv.Quote(route.method+" "+route.path), testing,
						body("send(t, "+strconv.Quote(route.method)+", "+strconv.Quote(route.path)+", "))
				}
			}
			b.WriteString("})\n")
		}
		if e.hasOK {
			test("OK", func(head string) string { return f.written(e, "OK", f.wantOK(e), head) })
		}
		if e.hasBad {
			test("BadRequest", func(head string) string { return f.written(e, "BadRequest", "httpWantBadRequest(t, res)\n", head) })
		}
		for gi := range e.groups {
			gr := &e.groups[gi]
			want := "httpWantInternal(t, res)\n"
			if gr.status {
				ns := f.g.declNS[gr.exception]
				want = fmt.Sprintf("httpWantThrown[%s%s](t, res, %d)\n", f.im.Qualifier(ns), golang.TypeName(gr.exception, ns), gr.code)
			}
			one := func(s *source) func(head string) string {
				return func(head string) string {
					if s.made {
						return f.made(s, want, head)
					}
					return f.written(e, s.method, want, head)
				}
			}
			if len(gr.sources) == 1 && gr.sources[0].Kind == golang.SourceUnknown {
				test(gr.name, one(&gr.sources[0]))
				continue
			}
			fmt.Fprintf(&b, "t.Run(%s, func(t *%s.T) {\n", strconv.Quote(gr.name), testing)
			for si := range gr.sources {
				test(gr.sources[si].Label, one(&gr.sources[si]))
			}
			b.WriteString("})\n")
		}
		b.WriteString("})\n")
	}
	b.WriteString("}\n\n")
	return b.String()
}

// wantOK は、正常のケースの確かめの行。
func (f *testFile) wantOK(e *entry) string {
	if e.hasValue() {
		return "httpWantResult(t, res, &got)\n"
	}
	return "httpWantResult(t, res, nil)\n"
}

// written は、人か AI が書いたケース method のテストの中: ケースを取り、送り、応答が、そのケースのものであること（want の行）を確かめ、
// Then を呼ぶ。head は、送る式の前半。
func (f *testFile) written(e *entry, method, want, head string) string {
	fakes := f.g.kit.HasFakes()
	var b strings.Builder
	fmt.Fprintf(&b, "k := cases.%s()\n", method)
	then := "k.Then(t, res"
	if fakes {
		fmt.Fprintf(&b, "res, f := %sk, nil)\n", head)
	} else {
		fmt.Fprintf(&b, "res := %sk)\n", head)
	}
	if e.hasValue() {
		fmt.Fprintf(&b, "var got %s\n", golang.GoType(e.op.Returns, f.im))
		then = "k.Then(t, got, res"
	}
	if fakes {
		then += ", f"
	}
	b.WriteString(want)
	fmt.Fprintf(&b, "if k.Then != nil {\n%s)\n}\n", then)
	return b.String()
}

// made は、Hozo が作るケースのテストの中: 正常のケースを取り、その用意の後に、にせものに言い（出どころの Steps を、一行ずつ）、
// 応答が、その例外のものであること（want の行）を確かめる。Then は、呼ばない。
func (f *testFile) made(s *source, want, head string) string {
	var b strings.Builder
	b.WriteString("k := cases.OK()\n")
	switch {
	case !f.g.kit.HasFakes():
		fmt.Fprintf(&b, "res := %sk)\n", head)
	case len(s.Steps) == 0:
		fmt.Fprintf(&b, "res, _ := %sk, nil)\n", head)
	default:
		fmt.Fprintf(&b, "res, _ := %sk, func(f *Fakes) {\n", head)
		for _, st := range s.Steps {
			at := "f." + strings.Join(st.Path, ".")
			switch st.Verb {
			case golang.StepThrows:
				ns := f.g.declNS[st.Exception]
				fmt.Fprintf(&b, "%s.Throws(%s%s{})\n", at, f.im.Qualifier(ns), golang.TypeName(st.Exception, ns))
			case golang.StepReturnsNone:
				fmt.Fprintf(&b, "%s.Returns(nil)\n", at)
			case golang.StepReturnsBool:
				fmt.Fprintf(&b, "%s.Returns(%t)\n", at, st.Bool)
			case golang.StepIs:
				ns := f.g.declNS[st.Classify]
				fmt.Fprintf(&b, "%s.Is(%s%s)\n", at, f.im.Qualifier(ns), golang.CaseConst(golang.TypeName(st.Classify, ns), st.Case))
			}
		}
		b.WriteString("})\n")
	}
	b.WriteString(want)
	return b.String()
}

// helpers は、RunHTTP の後ろの補助（同「テストの補助」）: 決まった順で、使うものだけを出す。どれを使うかは、入口の入力の
// デコレータと、結果の種類だけで決まる。
func (f *testFile) helpers(entries []entry) string {
	var header, query, body, headerObj, queryObj, bodyObj bool
	var badRequest, thrown, internal bool
	for i := range entries {
		for _, x := range entries[i].inputs() {
			switch x.decorator {
			case httpHeader:
				header = true
			case httpQuery:
				query = true
			case httpBody:
				body = true
			case httpHeaderObj:
				headerObj = true
			case httpQueryObj:
				queryObj = true
			case httpBodyObj:
				bodyObj = true
			}
		}
		badRequest = badRequest || entries[i].hasBad
		for _, gr := range entries[i].groups {
			if gr.status {
				thrown = true
			} else {
				internal = true
			}
		}
	}
	// 全体の形は、項目の形のメソッドで、項目を一つずつ足す
	header, query = header || headerObj, query || queryObj

	var b strings.Builder
	add := func(on bool, h helper) {
		if !on {
			return
		}
		for _, p := range h.imports {
			f.use(p)
		}
		b.WriteString(h.text)
	}
	add(true, testHelperCall)
	add(header, testHelperHeader)
	add(query, testHelperQuery)
	add(body, testHelperBody)
	add(headerObj, testHelperHeaderObj)
	add(queryObj, testHelperQueryObj)
	add(bodyObj, testHelperBodyObj)
	add(true, testHelperSend)
	add(header || query, testHelperText)
	add(headerObj || queryObj, testHelperFields)
	add(true, testHelperWantResult)
	add(badRequest, testHelperWantBadRequest)
	add(thrown, testHelperWantThrown)
	add(internal, testHelperWantInternal)
	add(badRequest || thrown || internal, testHelperWantFailure)
	add(true, testHelperWantJSON)
	return strings.TrimSuffix(b.String(), "\n")
}
