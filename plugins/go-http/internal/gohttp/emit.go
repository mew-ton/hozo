package gohttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
	"github.com/mew-ton/hozo/plugin/govalid"
	"github.com/mew-ton/hozo/plugin/httpvocab"
)

// 診断のコード（docs/plugin-go-http.md「診断」）。
const (
	codeHTTPOption   = "GOHTTP-9"  // [emit.options.go-http] に、知らないキーか、型の違う値がある
	codeHTTPName     = "GOHTTP-10" // NewHandler が、N の定義のパッケージの名前と当たる
	codeHTTPValidate = "GOHTTP-11" // validate = true で、Validate を持つ入力の model の置き場の単位が、go-valid を並べていない
	codeHTTPResult   = "GOHTTP-16" // 経路の op の結果の型が、省略可能な配列か、省略可能な、インデックスの model
	codeHTTPTestName = "GOHTTP-20" // テストのパッケージに足す公開の名前が、基底の公開の名前か、自分のほかの名前と当たる
	codeHTTPTestDir  = "GOHTTP-21" // テストのパッケージのディレクトリが、同じ単位が出力する子の名前空間の出力先と重なる
)

// FileName は、このトランスパイラが、入口のパッケージに出すファイルの名前（docs/plugin-go.md「相乗りのファイル」）。
// テストのパッケージに出すファイルの名前は TestFileName（testfile.go）。
const FileName = "http.go"

// handlerName は、このトランスパイラが入口のパッケージに足す、ただ一つの公開の名前。
const handlerName = "NewHandler"

// 読む語彙（標準の http。docs/plugin-go-http.md「マニフェスト」）のデコレータの FQN。持ち主は plugin/httpvocab。
const (
	httpGet       = httpvocab.Get
	httpPost      = httpvocab.Post
	httpHeader    = httpvocab.Header
	httpQuery     = httpvocab.Query
	httpBody      = httpvocab.Body
	httpHeaderObj = httpvocab.HeaderObj
	httpQueryObj  = httpvocab.QueryObj
	httpBodyObj   = httpvocab.BodyObj
	httpStatus    = httpvocab.Status
)

// options は [emit.options.go-http]（docs/plugin-go-http.md「オプション」）。
type options struct {
	// Validate が true なら、入力を読んだ後、op を呼ぶ前に、入力の制約（@valid.*）を確かめる。
	Validate bool `json:"validate"`
	// MaxBodyBytes は request body の上限（バイト）。nil なら既定（defaultMaxBodyBytes）。
	MaxBodyBytes *int64 `json:"maxBodyBytes"`
}

// defaultMaxBodyBytes は、オプション maxBodyBytes の既定（1 MiB）。
const defaultMaxBodyBytes = 1 << 20

// maxBodyBytes は、NewHandler が http.MaxBytesHandler に渡す、request body の上限。
func (o options) maxBodyBytes() int64 {
	if o.MaxBodyBytes == nil {
		return defaultMaxBodyBytes
	}
	return *o.MaxBodyBytes
}

// Output は、一つの出力 (U, N) についての応答の中身。Errors が一つでもあれば、Files も Warnings も空。
type Output struct {
	Files    []plugin.File
	Warnings []plugin.Warning
	Errors   []plugin.Diagnostic
}

// Emit は、要求の View の出力 (U, N) の、入口のパッケージの http.go（docs/plugin-go-http.md「出力」）と、テストのパッケージの http.go
// （同「インテグレーションテストの口」）を作る。読むのは、要求だけ
// （ファイルも、環境変数も、読まない）。診断があれば、ファイルは返さない。経路の op が一つもなければ、ファイルを出さず warning を一つ返す。
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

	// GOHTTP-9
	g.opts = g.options(req)

	// 置き場。基底のオプション module がなければ、基底が GO-8 を返すので、ここでは何も返さない（診断は、そのときも返す）
	layout, err := golang.LayoutOf(view)
	if err != nil && !errors.Is(err, golang.ErrNoModule) {
		return nil, fmt.Errorf("名前空間 %s の出力先 %q の import パスを求められない: %w", view.Viewer, view.Config.Dir, err)
	}
	g.layout = layout // module がなければ nil
	// テストの中の組み立て。module のない出力の置き場は、nil のまま（そのときは、ファイルを返さない）
	kit, err := golang.TestKitOf(view)
	if err != nil && !errors.Is(err, golang.ErrNoModule) {
		return nil, fmt.Errorf("名前空間 %s の、テストの中の組み立てに使う出力の import パスを求められない: %w", view.Viewer, err)
	}
	g.kit = kit

	routes, err := g.check()
	if err != nil {
		return nil, err
	}
	entries := g.entries(routes)
	if len(routes) > 0 {
		// GOHTTP-20、GOHTTP-21
		g.checkTestNames(entries)
		g.checkTestDir(entries)
	}
	if len(g.errs) > 0 {
		return &Output{Errors: g.errs}, nil
	}
	if len(routes) == 0 {
		return &Output{Warnings: []plugin.Warning{{At: view.Viewer,
			Message: fmt.Sprintf("名前空間 %s の単位の対象に @%s か @%s の付いた op がないので、%s を出さない", view.Viewer, httpGet, httpPost, FileName)}}}, nil
	}
	if g.layout == nil {
		return &Output{}, nil
	}
	content, err := g.render(routes)
	if err != nil {
		return nil, err
	}
	if g.kit.Blocked != "" {
		// テストの中の組み立てを、作れない: 口を出さない（docs/plugin-go-http.md「インテグレーションテストの口」の「置き場と名前」）
		return &Output{Files: []plugin.File{{Path: path.Join(layout.Dir, FileName), Content: content}},
			Warnings: []plugin.Warning{{At: view.Viewer,
				Message: fmt.Sprintf("名前空間 %s の、テストの中の組み立てを作れないので、インテグレーションテストの口（%s/%s）を出さない: %s", view.Viewer, layout.TestName, TestFileName, g.kit.Blocked)}}}, nil
	}
	test, err := g.renderTest(entries)
	if err != nil {
		return nil, err
	}
	testFile := plugin.File{Path: path.Join(layout.TestDir, TestFileName), Content: test}
	// テストの対応表（docs/plugin-go-http.md「インテグレーションテストの口」の「置き場と名前」）: 出したテキストから作る
	tests, err := golang.TestMap(testFile.Path, testFile.Content)
	if err != nil {
		return nil, fmt.Errorf("テストの対応表を作れない（生成器の不具合）: %w", err)
	}
	return &Output{Files: []plugin.File{
		{Path: path.Join(layout.Dir, FileName), Content: content},
		testFile,
		plugin.TestMapFile(layout.TestDir, TranspilerName, tests),
	}}, nil
}

// generator は一つの出力 (U, N) の、二つの http.go を作る状態。
type generator struct {
	view   *ir.View
	p      *ir.Program
	ns     *ir.Namespace // N
	opts   options
	layout *golang.Layout
	// kit は、N の出力の、テストのにせものと、テストの中の組み立て（docs/plugin-go.md「テストのにせもの」）。
	kit *golang.TestKit

	declNS map[string]string // 宣言の FQN → 宣言した名前空間
	files  map[string]bool   // 単位の対象のファイル（View.files）

	// unions は、通信に出る union（最初に現れた順。unions.go）。
	unions []*wireUnion
	// consts は、通信に出る、リテラルの型のフィールドを持つ model（最初に現れた順。unions.go）。
	consts []*wireConst

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
	g.declNS, g.files = map[string]string{}, map[string]bool{}
	for i := range g.p.Namespaces {
		for _, d := range g.p.Namespaces[i].Decls {
			if d == nil {
				continue
			}
			if _, dup := g.declNS[d.Common().Name]; !dup {
				g.declNS[d.Common().Name] = g.p.Namespaces[i].Name
			}
		}
	}
	for _, f := range g.view.Files {
		g.files[f] = true
	}
}

// route は経路一つ: 経路の op と、メソッドとパス。
type route struct {
	op           *ir.Op
	method, path string
	// model は入力の model。引数のない op なら nil。inputs は、そのフィールドの読み方（フィールドの順）。
	model  *ir.Model
	inputs []input
}

// hasBody は、入力が、本文のデコレータ（@http.body、@http.bodyObj）のフィールドを持つか。
func (r *route) hasBody() bool {
	for i := range r.inputs {
		if r.inputs[i].place == inBody {
			return true
		}
	}
	return false
}

// status は、@http.status を持つ例外一つ。
type status struct {
	exception *ir.Exception
	code      int64
}

// check は、N の単位の対象の op の経路を返し、Go に固有の診断（GOHTTP-10、GOHTTP-11、GOHTTP-16）を集める（docs/plugin-go-http.md「対象」）。
// 経路と、入力の読み方と、通信に出る union を読み解くのは、語彙のプラグインと同じライブラリ plugin/httpvocab。http の語彙の使い方の
// 誤り（HTTP-*。docs/plugin-http.md「診断」）は、語彙のプラグインが、先に断っている: ここで見つかれば、検査を通っていない仕様を
// 受け取ったことになるので、診断にせず、誤りを返す（異常終了）。テストのパッケージの診断（GOHTTP-20、GOHTTP-21）は、testfile.go。
func (g *generator) check() ([]route, error) {
	reader := httpvocab.NewReader(g.p)
	read := reader.Routes(g.view.Viewer, func(op *ir.Op) bool { return g.files[op.Span.File] })
	if errs := reader.Errors(); len(errs) > 0 {
		return nil, fmt.Errorf("名前空間 %s の仕様が、http の語彙の検査を通っていない（語彙のプラグイン http が、先に断るもの。docs/plugin-http.md「診断」）: %s %s: %s",
			g.view.Viewer, errs[0].Code, errs[0].At, errs[0].Message)
	}
	routes := make([]route, len(read))
	for i := range read {
		r := &read[i]
		routes[i] = route{op: r.Op, method: r.Method, path: r.Path, model: r.Model, inputs: inputsOf(r.Inputs)}
		// GOHTTP-11
		if r.Model != nil && g.opts.Validate {
			g.checkValidateHome(r.Op, r.Model)
		}
		// GOHTTP-16
		g.checkResult(r.Op)
	}
	g.wireUnions(reader.Unions())
	g.wireConsts(reader.LiteralModels())
	if len(routes) > 0 {
		g.checkName()
	}
	return routes, nil
}

// checkValidateHome は GOHTTP-11: validate = true の単位で、経路の op の入力の model m が、別の名前空間 M のもので、Validate を持つ
// なら、M の型の置き場（docs/config.md「型の置き場」）の単位が、transpilers に go-valid を並べていなければならない（Validate は、
// 型の定義のパッケージにしか書けない）。並べていなくても、生成したコードはコンパイルできる（補助 httpValidate は、入力が Validate を
// 持つかを実行時に見る）が、確かめると書いた入力が、確かめられないまま op に渡る。書いたものの食い違いなので、error にする
// （docs/plugin-go-http.md「制約の検査」）。型（Validate を持つか）を使うのは、この整合の検査だけで、出力は変えない。
// N の model は、自分の単位が go-valid を並べている（CFG-19）。置き場は、View だけから求まる（govalid.Unlisted）。
func (g *generator) checkValidateHome(op *ir.Op, m *ir.Model) {
	ns := g.declNS[m.Name]
	if ns == g.view.Viewer || !govalid.HasValidate(g.p, m.Name) {
		return
	}
	if unit, unlisted := govalid.Unlisted(g.view, ns); unlisted {
		g.errorf(codeHTTPValidate, op.Name, "経路の op %s の入力 %s は Validate を持つが、名前空間 %s の型の置き場の単位（%d 番目の [[emit]]）が、transpilers に %s を並べていない（validate = true と書いたのに、この入力は、確かめられないまま op に渡る。メソッドは、型の定義のパッケージにしか書けない）",
			op.Name, m.Name, ns, unit, govalid.Transpiler)
	}
}

// checkResult は GOHTTP-16: 経路の op の結果の型が、省略可能な配列か、省略可能な、インデックスの model。Go では、どちらも nil が「値がない」を
// 表すので、補助 httpWriteResult は、実行時に、値がない（204）ことと、空である（200）ことを見分けられない。
func (g *generator) checkResult(op *ir.Op) {
	t := op.Returns
	if c, ok := t.(ir.CaseType); ok {
		t = c.Of
	}
	o, ok := t.(ir.OptionalType)
	if !ok {
		return
	}
	of := o.Of
	if c, ok := of.(ir.CaseType); ok {
		of = c.Of
	}
	var what string
	switch {
	case isArray(of):
		what = "省略可能な配列"
	case g.p.IndexOf(of) != nil:
		what = "省略可能な、インデックスの model"
	default:
		return
	}
	g.errorf(codeHTTPResult, op.Name, "経路の op %s の結果の型 %s が、%s（Go では、値がないことと、空であることを見分けられないので、204 か 200 かを決められない。省略可能にしないか、model で包む）",
		op.Name, golang.TypeDisplay(op.Returns), what)
}

// argLiteral は、適用 app の引数 param のリテラル。書かれていない（省いた）か、リテラルでなければ false。
func argLiteral(app *ir.Application, param string) (ir.Literal, bool) {
	for _, a := range app.Args {
		if a.Param == param {
			l, ok := a.Value.(ir.LiteralValue)
			return l.Literal, ok && l.Literal != nil
		}
	}
	return nil, false
}

// statuses は、経路 routes の op のどれかが投げうる例外（Op.signature.exceptions）のうち、@http.status を持つもの。
// 最初に現れた順（op の宣言順、その中は exceptions の順。docs/plugin-go-http.md「補助」）。求めるのは httpvocab.Statuses。
func (g *generator) statuses(routes []route) []status {
	read := make([]httpvocab.Route, len(routes))
	for i := range routes {
		read[i] = httpvocab.Route{Op: routes[i].op}
	}
	var out []status
	for _, s := range httpvocab.Statuses(g.p, read) {
		out = append(out, status{exception: s.Exception, code: s.Code})
	}
	return out
}

// checkName は GOHTTP-10: NewHandler が、N の定義のパッケージの公開の名前（docs/plugin-go.md「名前」。入口のパッケージは、その
// すべてを再公開する）と当たる。位置は、当たった宣言。名前の一覧は、基底と共有する golang.PublicNames。
func (g *generator) checkName() {
	for _, n := range golang.PublicNames(g.view) {
		if n.Name != handlerName {
			continue
		}
		var what string
		switch n.Kind {
		case golang.PublicType:
			what = g.p.Lookup(n.Decl).Kind() + " " + n.Decl + " の型"
		case golang.PublicArm:
			what = "union " + n.Decl + " のアーム " + n.Detail + " を包む型"
		case golang.PublicEffect:
			what = "effect " + n.Decl + " の定数"
		case golang.PublicClassify:
			what = "classify " + n.Decl + " の型"
		case golang.PublicCase:
			what = "classify " + n.Decl + " のケース " + n.Detail + " の定数"
		case golang.PublicClassifyFunc:
			what = "classify " + n.Decl + " の判定関数"
		case golang.PublicCases:
			what = "classify " + n.Decl + " の述語のインターフェース"
		default:
			what = n.What // 状態の単位、Ops、New など（NewHandler とは当たらない形の名前）
		}
		g.errorf(codeHTTPName, n.Decl, "%s %s が、%s が入口のパッケージに足す関数 %s と当たる（直すのは仕様の名前）", what, n.Name, TranspilerName, handlerName)
	}
}

// isArray は、型 t が array か。
func isArray(t ir.Type) bool {
	_, ok := t.(ir.ArrayType)
	return ok
}

// options は [emit.options.go-http]（docs/plugin-go-http.md「オプション」）を読む。GOHTTP-9 を集める: 知らないキーと、型の違う値
// （maxBodyBytes では、正の整数でない値も）。型の違うキーは、ないものとして（既定で）読む。
func (g *generator) options(req *plugin.EmitRequest) options {
	var opts options
	keys, _ := plugin.Options(req, &struct{}{}) // どのキーも知らないキーとして、辞書順に返る
	table := g.view.Config.Options[TranspilerName]
	at := func(key, want string) {
		value, _ := json.Marshal(table[key])
		g.errorf(codeHTTPOption, "", "%d 番目の [[emit]] の [emit.options.%s] の %s の値 %s が、%sでない", g.view.Config.Index, TranspilerName, key, value, want)
	}
	// 型の違う値が先、知らないキーが後（どちらも、キーの辞書順）
	var unknown []string
	for _, key := range keys {
		data, _ := json.Marshal(table[key])
		switch key {
		case "validate":
			if err := json.Unmarshal(data, &opts.Validate); err != nil {
				opts.Validate = false
				at(key, "真偽値")
			}
		case "maxBodyBytes":
			var n int64
			if err := json.Unmarshal(data, &n); err != nil || n <= 0 {
				at(key, "正の整数")
				continue
			}
			opts.MaxBodyBytes = &n
		default:
			unknown = append(unknown, key)
		}
	}
	for _, key := range unknown {
		g.errorf(codeHTTPOption, "", "%d 番目の [[emit]] の [emit.options.%s] に、知らないキー %s がある（あるのは validate と maxBodyBytes）", g.view.Config.Index, TranspilerName, key)
	}
	return opts
}
