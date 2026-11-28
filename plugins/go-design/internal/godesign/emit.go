package godesign

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
)

// 診断のコード（docs/plugin-go-design.md「診断」）。
const (
	codeFlowExport = "GODESIGN-1" // @prd.business.flow の付いた op に export がない
	codeFlowShape  = "GODESIGN-2" // @prd.business.flow の付いた op が、本体を持たない（葉）、または never を返す
	codeFlowCalls  = "GODESIGN-3" // フローの op の配線が、操作の op（@prd.usecase.of の付いた op）でないものを呼ぶ
	codeFlowHome   = "GODESIGN-4" // フローを持つ名前空間に、@prd.usecase.of の付いた op がある
	codeOption     = "GODESIGN-5" // [emit.options.go-design] に、知らないキーか、型の違う値か、要求の分類でない FQN がある
	codeTestName   = "GODESIGN-6" // テストのパッケージに足す名前が、当たる、または Go の公開の名前にならない
	codeTestDir    = "GODESIGN-7" // テストのパッケージのディレクトリが、同じ単位が出力する子の名前空間の出力先と重なる
)

// FileName は、このトランスパイラが、N のテストのパッケージに出すファイルの名前（plugins/go/spec/go.hozo の Requirement.Layout の「相乗りのファイル」）。
const FileName = "design.go"

// 読む語彙（標準の business、usecase、prd。plugins/go-design/hozo-plugin.toml）。actor の語彙は読まない（印）。
const (
	businessFlow = "prd.business.flow"
	usecaseOf    = "prd.usecase.of"
	prdRef       = "prd.ref"
)

// options は [emit.options.go-design]（plugins/go-design/spec/go-design.hozo の Requirement.RequirementPort）。
type options struct {
	// Requirements は、要求の分類の FQN（書かれた順）。
	Requirements []string `json:"requirements"`
}

// Output は、一つの出力 (U, N) についての応答の中身。Errors が一つでもあれば、Files も Warnings も空。
type Output struct {
	Files    []plugin.File
	Warnings []plugin.Warning
	Errors   []plugin.Diagnostic
}

// Emit は、要求の View の出力 (U, N) の、テストのパッケージの design.go（plugins/go-design/spec/go-design.hozo の Requirement.Output）を作る。読むのは、要求だけ
// （ファイルも、環境変数も、読まない）。診断があれば、ファイルも warning も返さない。フローも、要求もなければ、ファイルを
// 出さず warning を一つ返す。どこからも指されていない要求ごとに、warning を一つ返す。
// 戻り値の error は、利用者の仕様の誤りでないもの（View が欠けている、生成したコードを整形できない）。
func Emit(req *plugin.EmitRequest) (*Output, error) {
	view := req.View
	if view == nil || view.Program == nil || view.Program.Namespace(view.Viewer) == nil {
		viewer := ""
		if view != nil {
			viewer = view.Viewer
		}
		return nil, fmt.Errorf("View に、名前空間 %s の Program がない", viewer)
	}
	g := &generator{view: view, p: view.Program, ns: view.Program.Namespace(view.Viewer)}
	g.prepare()

	// GODESIGN-5
	classes := g.requirementClasses(req)

	// 置き場。基底のオプション module がなければ、基底が GO-8 を返すので、ここでは何も返さない（診断と warning は、そのときも返す）
	layout, err := golang.LayoutOf(view)
	if err != nil && !errors.Is(err, golang.ErrNoModule) {
		return nil, fmt.Errorf("名前空間 %s の出力先 %q の import パスを求められない: %w", view.Viewer, view.Config.Dir, err)
	}
	g.layout = layout // module がなければ nil

	// GODESIGN-1〜GODESIGN-4
	flows := g.flows()
	if len(flows) > 0 || len(classes) > 0 {
		// GODESIGN-6、GODESIGN-7
		g.checkTestNames(flows, classes)
		g.checkTestDir(flows, classes)
	}
	if len(g.errs) > 0 {
		return &Output{Errors: g.errs}, nil
	}
	if len(flows) == 0 && len(classes) == 0 {
		return &Output{Warnings: []plugin.Warning{{At: view.Viewer,
			Message: fmt.Sprintf("名前空間 %s の単位の対象に @%s の付いた op がなく、[emit.options.%s] の requirements も空なので、%s を出さない",
				view.Viewer, businessFlow, TranspilerName, FileName)}}}, nil
	}
	var warnings []plugin.Warning
	for _, c := range classes {
		for i := range c.cases {
			if k := &c.cases[i]; len(k.designs) == 0 {
				warnings = append(warnings, plugin.Warning{At: c.decl.Name,
					Message: fmt.Sprintf("要求 %s は、どこからも指されていない（@%s で指す設計がない）", k.label, prdRef)})
			}
		}
	}
	if g.layout == nil {
		return &Output{Warnings: warnings}, nil
	}
	content, err := g.render(flows, classes)
	if err != nil {
		return nil, err
	}
	file := plugin.File{Path: path.Join(layout.TestDir, FileName), Content: content}
	// テストの対応表（plugins/go-design/spec/go-design.hozo の Requirement.Output）: 出したテキストから作る
	tests, err := golang.TestMap(file.Path, file.Content)
	if err != nil {
		return nil, fmt.Errorf("テストの対応表を作れない（生成器の不具合）: %w", err)
	}
	return &Output{Files: []plugin.File{file, plugin.TestMapFile(layout.TestDir, TranspilerName, tests)}, Warnings: warnings}, nil
}

// generator は一つの出力 (U, N) の design.go を作る状態。
type generator struct {
	view   *ir.View
	p      *ir.Program
	ns     *ir.Namespace // N
	layout *golang.Layout

	files map[string]bool // 単位の対象のファイル（View.files）

	errs []plugin.Diagnostic
	seen map[string]bool
}

func (g *generator) errorf(code, at, format string, args ...any) {
	d := plugin.Diagnostic{Code: code, At: at, Message: fmt.Sprintf(format, args...)}
	key := d.Code + "\x00" + d.At + "\x00" + d.Message
	if g.seen == nil {
		g.seen = map[string]bool{}
	}
	if g.seen[key] {
		return
	}
	g.seen[key] = true
	g.errs = append(g.errs, d)
}

// prepare は、View から、検査と生成の間ずっと使う表を作る。
func (g *generator) prepare() {
	g.files = map[string]bool{}
	for _, f := range g.view.Files {
		g.files[f] = true
	}
}

// has は、宣言 d に、デコレータ decorator が付いているか。
func has(d *ir.DeclBase, decorator string) bool {
	for i := range d.Decorators {
		if d.Decorators[i].Decorator == decorator {
			return true
		}
	}
	return false
}

// flow は、フロー一つ: @prd.business.flow の付いた、単位の対象の op。
type flow struct {
	op *ir.Op
}

// method は、op のメソッドの名前。口の名前 <Op>FlowCase の <Op>。
func (f *flow) method() string { return golang.MethodName(f.op.Name) }

// hasValue は、フローが、値を返すか。
func (f *flow) hasValue() bool { return f.op.Returns != nil && !ir.IsNever(f.op.Returns) }

// fields は、<Op>FlowCase の、フローの引数のフィールドの名前（Op.params の順）: 引数の名前を、フィールド名と同じ決まりで。
// Given、Then と当たれば、末尾に _（plugins/go-design/spec/go-design.hozo の Requirement.FlowPort）。
func (f *flow) fields() []string {
	names := make([]string, len(f.op.Params))
	for i, p := range f.op.Params {
		names[i] = golang.FieldName(p.Name)
		switch names[i] {
		case "Given", "Then":
			names[i] += "_"
		}
	}
	return names
}

// flows は、N の単位の対象の op を宣言順に見て、フローを集め、診断（GODESIGN-1〜GODESIGN-4）を集める
// （plugins/go-design/spec/go-design.hozo の Requirement.Targets、docs/plugin-go-design.md「フローの op の決まり」）。
func (g *generator) flows() []flow {
	var flows []flow
	for _, d := range g.ns.Decls {
		op, ok := d.(*ir.Op)
		if !ok || !g.files[op.Span.File] || !has(op.Common(), businessFlow) {
			continue
		}
		flows = append(flows, flow{op: op})
		// GODESIGN-1
		if !op.Exported {
			g.errorf(codeFlowExport, op.Name, "op %s に @%s が付いているが、export がない（走らせる側は、入口の Ops のメソッドとして、フローを呼ぶ）", op.Name, businessFlow)
		}
		// GODESIGN-2
		switch {
		case ir.IsNever(op.Returns):
			g.errorf(codeFlowShape, op.Name, "フローの op %s が、never を返す（正常に終わる道がないので、E2E テストのケースを、書けない）", op.Name)
		case op.Body == nil:
			g.errorf(codeFlowShape, op.Name, "フローの op %s が、本体を持たない（フローは、操作の op を、順に呼ぶ配線）", op.Name)
		}
		// GODESIGN-3
		if op.Body != nil {
			g.checkCalls(op)
		}
	}
	if len(flows) == 0 {
		return nil
	}
	// GODESIGN-4: N の op のすべて（単位の対象でないものも）
	for _, d := range g.ns.Decls {
		if op, ok := d.(*ir.Op); ok && has(op.Common(), usecaseOf) {
			g.errorf(codeFlowHome, op.Name, "op %s に @%s が付いているが、名前空間 %s は、フロー（%s）を持つ（操作の op は、フローとは、別の名前空間に置く）",
				op.Name, usecaseOf, g.view.Viewer, flows[0].op.Name)
		}
	}
	return flows
}

// checkCalls は GODESIGN-3: フロー op の配線の呼び出し（文と、match のアームの本体。for と disjoint の中も）の先が、どれも、
// @prd.usecase.of の付いた op であること。呼んだ先ごとに、診断を一つ（同じ先を二度呼んでも、一つ）。
func (g *generator) checkCalls(op *ir.Op) {
	check := func(call ir.Call) {
		switch c := call.Callee.(type) {
		case ir.OpCallee:
			if callee, ok := g.p.Lookup(c.Op).(*ir.Op); ok && has(callee.Common(), usecaseOf) {
				return
			}
			g.errorf(codeFlowCalls, op.Name, "フローの op %s が、op %s を呼ぶが、@%s が付いていない（フローが呼べるのは、操作の op だけ）", op.Name, c.Op, usecaseOf)
		case ir.ReadCallee:
			g.errorf(codeFlowCalls, op.Name, "フローの op %s が、read %s を呼ぶ（フローが呼べるのは、操作の op だけ）", op.Name, c.Read)
		case ir.EffectCallee:
			g.errorf(codeFlowCalls, op.Name, "フローの op %s が、効果 %s を呼ぶ（フローが呼べるのは、操作の op だけ）", op.Name, c.String())
		}
	}
	ir.WalkStatements(op.Body.Statements, func(st ir.Statement) {
		expr, ok := ir.StatementExpr(st)
		if !ok {
			return
		}
		switch e := expr.(type) {
		case ir.CallExpr:
			check(e.Call)
		case ir.MatchExpr:
			for _, arm := range e.Match.Arms {
				if body, ok := arm.Body.(ir.CallExpr); ok {
					check(body.Call)
				}
			}
		}
	})
}

// class は、要求の分類一つ（オプション requirements の項目）と、そのケース（宣言順）。
type class struct {
	decl  *ir.Classify
	cases []requirement
}

// requirement は、要求一つ: 要求の分類のケース。
type requirement struct {
	// method は、Requirements のメソッドの名前: ケースの名前の、先頭を大文字にしたもの。
	method string
	// label は、要求の名前（<分類の FQN>.<ケース>）。テストの名前でもある。
	label string
	doc   *ir.Doc
	// designs は、この要求を指している設計（@prd.ref の引数に、この要求を持つ宣言の FQN）。名前空間の FQN の辞書順、その中は宣言順。
	designs []string
}

// requirementClasses は、オプション requirements を読み、要求の分類と、そのケースと、指している設計を返す
// （plugins/go-design/spec/go-design.hozo の Requirement.RequirementPort）。GODESIGN-5 を集める。
func (g *generator) requirementClasses(req *plugin.EmitRequest) []class {
	var opts options
	index := g.view.Config.Index
	unknown, err := plugin.Options(req, &opts)
	if err != nil {
		// オプションは requirements だけなので、型の違う値は、その値
		opts = options{}
		value, _ := json.Marshal(g.view.Config.Options[TranspilerName]["requirements"])
		g.errorf(codeOption, "", "%d 番目の [[emit]] の [emit.options.%s] の requirements の値 %s が、文字列の配列でない", index, TranspilerName, value)
	}
	for _, key := range unknown {
		g.errorf(codeOption, "", "%d 番目の [[emit]] の [emit.options.%s] に、知らないキー %s がある（あるのは requirements）", index, TranspilerName, key)
	}
	designs := g.designs()
	var classes []class
	listed := map[string]bool{}
	for _, fqn := range opts.Requirements {
		if listed[fqn] {
			g.errorf(codeOption, "", "%d 番目の [[emit]] の [emit.options.%s] の requirements に、%s が二度ある", index, TranspilerName, fqn)
			continue
		}
		listed[fqn] = true
		c, ok := g.p.Lookup(fqn).(*ir.Classify)
		switch {
		case g.p.Lookup(fqn) == nil:
			g.errorf(codeOption, "", "%d 番目の [[emit]] の [emit.options.%s] の requirements の %s という宣言がない（書くのは、要求の分類の FQN）", index, TranspilerName, fqn)
			continue
		case !ok:
			g.errorf(codeOption, "", "%d 番目の [[emit]] の [emit.options.%s] の requirements の %s は、分類でない（書くのは、要求の分類の FQN）", index, TranspilerName, fqn)
			continue
		case !c.IsSubjectless():
			g.errorf(codeOption, "", "%d 番目の [[emit]] の [emit.options.%s] の requirements の分類 %s は、対象を持つ（要求の分類は、対象のない分類: classify … of never）", index, TranspilerName, fqn)
			continue
		}
		cl := class{decl: c}
		for i := range c.Cases {
			k := &c.Cases[i]
			label := c.Name + "." + k.Name
			cl.cases = append(cl.cases, requirement{method: golang.Capitalize(k.Name), label: label, doc: k.Description, designs: designs[label]})
		}
		classes = append(classes, cl)
	}
	return classes
}

// designs は、要求の名前（<分類の FQN>.<ケース>）→ それを指している設計（@prd.ref の引数に、その要求を持つ宣言の FQN）。
// 仕様の全体から集める。並びは、名前空間の FQN の辞書順、その中は宣言順。同じ宣言は、一度だけ
// （plugins/go-design/spec/go-design.hozo の Requirement.RequirementPort）。
func (g *generator) designs() map[string][]string {
	namespaces := make([]*ir.Namespace, 0, len(g.p.Namespaces))
	for i := range g.p.Namespaces {
		namespaces = append(namespaces, &g.p.Namespaces[i])
	}
	sort.SliceStable(namespaces, func(i, j int) bool { return namespaces[i].Name < namespaces[j].Name })
	out := map[string][]string{}
	for _, ns := range namespaces {
		for _, d := range ns.Decls {
			if d == nil {
				continue
			}
			base := d.Common()
			pointed := map[string]bool{}
			for i := range base.Decorators {
				app := &base.Decorators[i]
				if app.Decorator != prdRef {
					continue
				}
				for _, arg := range app.Args {
					for _, c := range caseValues(arg.Value) {
						label := c.Classify + "." + c.Case
						if !pointed[label] {
							pointed[label] = true
							out[label] = append(out[label], base.Name)
						}
					}
				}
			}
		}
	}
	return out
}

// caseValues は、デコレータの引数の値 v が指す、分類のケース（残りの引数は、列の中のもの）。
func caseValues(v ir.ArgValue) []ir.CaseValue {
	switch x := v.(type) {
	case ir.CaseValue:
		return []ir.CaseValue{x}
	case ir.ListValue:
		var out []ir.CaseValue
		for _, item := range x.Items {
			out = append(out, caseValues(item)...)
		}
		return out
	}
	return nil
}

// テストのパッケージに足す、決まった公開の名前（plugins/go-design/spec/go-design.hozo の Requirement.Output）。フローごとのケースは <Op>FlowCase。
const (
	flowCaseSuffix   = "FlowCase"
	flowCasesName    = "FlowCases"       // フローのケースのまとめ
	runFlowsName     = "RunFlows"        // フローを走らせる関数
	requirementsName = "Requirements"    // 要求ごとの、テストの入れ物
	runReqsName      = "RunRequirements" // 要求の入れ物を走らせる関数
)

// checkTestNames は GODESIGN-6: テストのパッケージに足す公開の名前が、そのパッケージの、基底の公開の名前（golang.PublicTestNames）か、
// 自分のほかの名前と当たる。<Op>FlowCase のフィールドの名前どうし、Requirements のメソッドの名前どうしが当たるときも。フローの op の
// 名前か、要求のケースの名前が _ で始まるときも（Go の公開の名前にならない）。位置は、当たった自分の名前を生んだフローの op か、
// 要求の分類。決まった名前が当たったなら、相手の名前を生んだ宣言。
func (g *generator) checkTestNames(flows []flow, classes []class) {
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
		g.errorf(codeTestName, at, "%s が、%s と当たる（どちらも、テストのパッケージの名前 %s。直すのは仕様の名前）", what, prev.what, name)
	}
	if len(flows) > 0 {
		add(flowCasesName, TranspilerName+" の、フローのケースのまとめ "+flowCasesName, "")
		add(runFlowsName, TranspilerName+" の、フローを走らせる "+runFlowsName, "")
	}
	for i := range flows {
		f := &flows[i]
		if strings.HasPrefix(short(f.op.Name), "_") {
			g.errorf(codeTestName, f.op.Name, "フローの op %s の名前が、_ で始まる（Go の公開の名前にならず、口を、別のパッケージから満たせない）", f.op.Name)
			continue
		}
		name := f.method() + flowCaseSuffix
		add(name, TranspilerName+" の、フロー "+f.op.Name+" のケース "+name, f.op.Name)
		fields := map[string]string{"Given": "今の状態の用意 Given", "Then": "足りない確かめ Then"}
		for j, field := range f.fields() {
			what := "引数 " + f.op.Params[j].Name
			if prev, dup := fields[field]; dup {
				g.errorf(codeTestName, f.op.Name, "フロー %s のケース %s の、%s のフィールド %s が、%s と当たる（直すのは仕様の名前）", f.op.Name, name, what, field, prev)
				continue
			}
			fields[field] = what
		}
	}
	if len(classes) > 0 {
		add(requirementsName, TranspilerName+" の、要求ごとの、テストの入れ物 "+requirementsName, "")
		add(runReqsName, TranspilerName+" の、要求の入れ物を走らせる "+runReqsName, "")
	}
	methods := map[string]string{}
	for _, c := range classes {
		for i := range c.cases {
			k := &c.cases[i]
			if strings.HasPrefix(k.method, "_") {
				g.errorf(codeTestName, c.decl.Name, "要求 %s のケースの名前が、_ で始まる（Go の公開の名前にならず、口を、別のパッケージから満たせない）", k.label)
				continue
			}
			if prev, dup := methods[k.method]; dup {
				g.errorf(codeTestName, c.decl.Name, "要求 %s の入れ物 %s が、要求 %s の入れ物と当たる（どちらも、%s のメソッド %s。直すのは仕様の名前）",
					k.label, k.method, prev, requirementsName, k.method)
				continue
			}
			methods[k.method] = k.label
		}
	}
}

// short は、FQN の最後のセグメント。
func short(fqn string) string {
	if i := strings.LastIndexByte(fqn, '.'); i >= 0 {
		return fqn[i+1:]
	}
	return fqn
}

// checkTestDir は GODESIGN-7: テストのパッケージのディレクトリが、同じ単位が出力する子の名前空間 N.<テストのパッケージの名前> の
// 出力先と重なる。基底が、同じことを GO-4 で返す出力（基底も、テストのパッケージを作る）では、返さない。位置は、最初のフローの op。
// フローがなければ、最初の要求の分類。置き場が要るので、module がない（layout が nil）ときは、見ない（基底が GO-8 を返し、何も
// 書き出されない）。
func (g *generator) checkTestDir(flows []flow, classes []class) {
	if g.layout == nil || len(golang.PublicTestNames(g.view)) > 0 {
		return
	}
	child := g.view.Viewer + "." + g.layout.TestName
	for _, c := range g.view.Namespaces[child] {
		if c.Index != g.view.Config.Index {
			continue
		}
		var at string
		if len(flows) > 0 {
			at = flows[0].op.Name
		} else {
			at = classes[0].decl.Name
		}
		g.errorf(codeTestDir, at, "名前空間 %s は、E2E テストの口と、要求の入れ物の口を、テストのパッケージ %s に置くが、同じ単位（%d 番目の [[emit]]）が出力する子の名前空間 %s の出力先と重なる",
			g.view.Viewer, g.layout.TestDir, c.Index, child)
		return
	}
}
