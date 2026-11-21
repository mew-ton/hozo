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
var testBodyNames = []string{"t", "c", "k", "cases", "send", "call", "res", "got", "handler", "method", "path"}

// resultKind は、入口の結果の種類（同「必要なケース」の表の行）。
type resultKind int

const (
	resultOK         resultKind = iota // 正常
	resultBadRequest                   // op を呼ぶ前の失敗（400 と http.BadRequest）
	resultStatus                       // @http.status を持つ例外
	resultInternal                     // @http.status を持たない例外（500 と internal）
)

// result は、入口の結果一つ。name は、<Op>HTTPCases のメソッドの名前で、テストの名前の <結果> でもある。
// exception は、例外の FQN（例外でなければ ""）。code は、@http.status のステータス（resultStatus のときだけ）。
type result struct {
	name      string
	kind      resultKind
	exception string
	code      int64
}

// entry は、入口一つ: 経路の op と、その経路（書かれた順）と、結果。
type entry struct {
	op      *ir.Op
	routes  []*route
	results []result
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
		out[i].results = g.results(&out[i])
	}
	return out
}

// results は、入口 e の結果（docs/plugin-go-http.md「インテグレーションテストの口」の「必要なケース」）。この順:
// 正常（never を返す op には、ない）。op を呼ぶ前の失敗（入力が、本文のデコレータのフィールドか、@http.required の付いたフィールドを、
// 一つでも持つ op だけ。型は見ない）。op が投げうる例外ごと（Op.signature.exceptions の順。デコレータが運ぶ例外を含む）。
// 例外の名前は、宣言の名前。その op の中で名前が当たるもの（別の名前空間の、同じ名前の例外。例外 OK、BadRequest）は、名前空間の
// セグメントを先頭大文字にして前に付ける（基底の、葉の op の口と同じ決まり）。
func (g *generator) results(e *entry) []result {
	var out []result
	count := map[string]int{}
	add := func(r result) {
		count[r.name]++
		out = append(out, r)
	}
	if !ir.IsNever(e.op.Returns) {
		add(result{name: "OK", kind: resultOK})
	}
	for _, in := range e.inputs() {
		if in.place == inBody || in.required {
			add(result{name: "BadRequest", kind: resultBadRequest})
			break
		}
	}
	seen := map[string]bool{}
	for _, fqn := range e.op.Signature.Exceptions {
		x, ok := g.p.Lookup(fqn).(*ir.Exception)
		if !ok || seen[fqn] {
			continue
		}
		seen[fqn] = true
		r := result{name: golang.TypeName(fqn, g.declNS[fqn]), kind: resultInternal, exception: fqn}
		if code, has := statusOf(x); has {
			r.kind, r.code = resultStatus, code
		}
		add(r)
	}
	for i := range out {
		if out[i].exception == "" || count[out[i].name] == 1 {
			continue
		}
		var prefix string
		for _, seg := range strings.Split(g.declNS[out[i].exception], ".") {
			prefix += golang.Capitalize(seg)
		}
		out[i].name = prefix + out[i].name
	}
	return out
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
		methods := map[string]string{}
		for _, r := range e.results {
			what := "正常の結果"
			switch r.kind {
			case resultBadRequest:
				what = "op を呼ぶ前の失敗の結果"
			case resultStatus, resultInternal:
				what = "例外 " + r.exception + " の結果"
			}
			if prev, dup := methods[r.name]; dup {
				g.errorf(codeHTTPTestName, e.op.Name, "op %s のケースの口 %s%s の、%sの名前 %s が、%sの名前と当たる（直すのは仕様の名前）",
					e.op.Name, m, casesName, what, r.name, prev)
				continue
			}
			methods[r.name] = what
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

// cases は、入口一つの口（docs/plugin-go-http.md「インテグレーションテストの口」の「口の形」）: 結果ごとのメソッドを持つ <Op>HTTPCases と、
// ケース一つの <Op>HTTPCase。
func (f *testFile) cases(e *entry) string {
	m := e.method()
	var paths []string
	for _, r := range e.routes {
		paths = append(paths, r.method+" "+r.path)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "// %s%s は、入口 %s（%s）の、結果ごとのケース。結果の数だけ、メソッドがある。\ntype %s%s interface {\n",
		m, casesName, e.op.Name, strings.Join(paths, "、"), m, casesName)
	for _, r := range e.results {
		switch r.kind {
		case resultOK:
			fmt.Fprintf(&b, "// %s は、正常に終わるケース: 成功の応答。\n", r.name)
		case resultBadRequest:
			fmt.Fprintf(&b, "// %s は、op を呼ぶ前に、要求を断るケース: 400 と http.BadRequest。\n", r.name)
		case resultStatus:
			fmt.Fprintf(&b, "// %s は、例外 %s を投げるケース: %d。\n", r.name, r.exception, r.code)
		case resultInternal:
			fmt.Fprintf(&b, "// %s は、例外 %s を投げるケース: 500（@http.status を持たない例外は、中身を出さない）。\n", r.name, r.exception)
		}
		fmt.Fprintf(&b, "%s() %sHTTPCase\n", r.name, m)
	}
	b.WriteString("}\n\n")

	testing, httptest := f.use("testing"), f.use("net/http/httptest")
	if model := e.model(); model != nil {
		ns := f.g.declNS[model.Name]
		fmt.Fprintf(&b, "// %sHTTPCase は、入口 %s の、ケース一つ: 入力と、今の状態の用意と、足りない確かめ。\ntype %sHTTPCase struct {\n", m, e.op.Name, m)
		fmt.Fprintf(&b, "// %s は、入力。走らせる側が、要求に組み立てる。\n%s %s%s\n", inputField(e), inputField(e), f.im.Qualifier(ns), golang.TypeName(model.Name, ns))
	} else {
		fmt.Fprintf(&b, "// %sHTTPCase は、入口 %s の、ケース一つ: 今の状態の用意と、足りない確かめ。\ntype %sHTTPCase struct {\n", m, e.op.Name, m)
	}
	fmt.Fprintf(&b, "// Given は、要求を送る前に、今の状態を用意する（nil なら、呼ばない）。\nGiven func(ctx %s.Context) error\n", f.use("context"))
	fmt.Fprintf(&b, "// Request は、組み立てた要求を、送る前に、手直しする（nil なら、呼ばない）。入力の値では作れない要求に使う。\nRequest func(r *%s.Request)\n", f.use("net/http"))
	if e.hasValue() {
		fmt.Fprintf(&b, "// Then は、応答が、そのケースのものだと確かめた後に、呼ばれる（nil なら、呼ばない）。got は、成功の応答から読んだ値（正常のケースのほかでは、零値）、res は、応答。\n"+
			"Then func(t *%s.T, got %s, res *%s.ResponseRecorder)\n", testing, golang.GoType(e.op.Returns, f.im), httptest)
	} else {
		fmt.Fprintf(&b, "// Then は、応答が、そのケースのものだと確かめた後に、呼ばれる（nil なら、呼ばない）。res は、応答。\n"+
			"Then func(t *%s.T, res *%s.ResponseRecorder)\n", testing, httptest)
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

// run は、走らせる関数 RunHTTP（同「口の形」）: 入口ごと、結果ごとに、t.Run を重ねる。経路を二つ以上持つ op は、結果の下に、
// 経路ごとの段を足す。
func (f *testFile) run(entries []entry) string {
	testing, http, httptest := f.use("testing"), f.use("net/http"), f.use("net/http/httptest")
	var b strings.Builder
	fmt.Fprintf(&b, "// %s は、入口ごと、結果ごとに、テストを一つ走らせる。テストの名前は、<op の FQN>/<結果>。\n"+
		"// ケースごとに、入力を要求に組み立てて handler に送り、応答が、そのケースのものであること（ステータスと、本文の形）を確かめてから、Then を呼ぶ。\n"+
		"func %s(t *%s.T, handler %s.Handler, c %s) {\n", runName, runName, testing, http, casesName)
	for i := range entries {
		e := &entries[i]
		m := e.method()
		single := len(e.routes) == 1
		fmt.Fprintf(&b, "t.Run(%s, func(t *%s.T) {\ncases := c.%s()\n", strconv.Quote(e.op.Name), testing, m)
		// 要求の組み立て（同「要求の組み立て」）: フィールドごとに、デコレータと、その引数（か、フィールドの名前）だけで決まる一行
		if single {
			fmt.Fprintf(&b, "send := func(t *%s.T, k %sHTTPCase) *%s.ResponseRecorder {\ncall := httpCall{method: %s, path: %s}\n",
				testing, m, httptest, strconv.Quote(e.routes[0].method), strconv.Quote(e.routes[0].path))
		} else {
			fmt.Fprintf(&b, "send := func(t *%s.T, method, path string, k %sHTTPCase) *%s.ResponseRecorder {\ncall := httpCall{method: method, path: path}\n",
				testing, m, httptest)
		}
		for _, x := range e.inputs() {
			value := "k." + inputField(e) + "." + golang.FieldName(x.field.Name)
			name := strings.TrimPrefix(x.decorator, "http.")
			if x.whole {
				fmt.Fprintf(&b, "call.%s(%s)\n", name, value)
			} else {
				fmt.Fprintf(&b, "call.%s(%s, %s)\n", name, strconv.Quote(x.name), value)
			}
		}
		b.WriteString("return call.send(t, handler, k.Given, k.Request)\n}\n")
		for _, r := range e.results {
			fmt.Fprintf(&b, "t.Run(%s, func(t *%s.T) {\n", strconv.Quote(r.name), testing)
			if single {
				b.WriteString(f.body(e, &r, "send(t, k)"))
			} else {
				for _, route := range e.routes {
					fmt.Fprintf(&b, "t.Run(%s, func(t *%s.T) {\n%s})\n", strconv.Quote(route.method+" "+route.path), testing,
						f.body(e, &r, "send(t, "+strconv.Quote(route.method)+", "+strconv.Quote(route.path)+", k)"))
				}
			}
			b.WriteString("})\n")
		}
		b.WriteString("})\n")
	}
	b.WriteString("}\n\n")
	return b.String()
}

// body は、テスト一つの中: ケースを取り、送り（send は、送る式）、応答が、結果 r のものであることを確かめ、Then を呼ぶ。
// 行の形は、結果の種類と、op が値を返すかだけで決まる。
func (f *testFile) body(e *entry, r *result, send string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "k := cases.%s()\nres := %s\n", r.name, send)
	then, dst := "k.Then(t, res)", "nil"
	if e.hasValue() {
		fmt.Fprintf(&b, "var got %s\n", golang.GoType(e.op.Returns, f.im))
		then, dst = "k.Then(t, got, res)", "&got"
	}
	switch r.kind {
	case resultOK:
		fmt.Fprintf(&b, "httpWantResult(t, res, %s)\n", dst)
	case resultBadRequest:
		b.WriteString("httpWantBadRequest(t, res)\n")
	case resultStatus:
		ns := f.g.declNS[r.exception]
		fmt.Fprintf(&b, "httpWantThrown[%s%s](t, res, %d)\n", f.im.Qualifier(ns), golang.TypeName(r.exception, ns), r.code)
	case resultInternal:
		b.WriteString("httpWantInternal(t, res)\n")
	}
	fmt.Fprintf(&b, "if k.Then != nil {\n%s\n}\n", then)
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
		for _, r := range entries[i].results {
			switch r.kind {
			case resultBadRequest:
				badRequest = true
			case resultStatus:
				thrown = true
			case resultInternal:
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
