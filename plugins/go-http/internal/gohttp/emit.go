package gohttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
	"github.com/mew-ton/hozo/plugin/govalid"
)

// 診断のコード（docs/plugin-go-http.md「診断」）。
const (
	codeHTTPExport    = "GOHTTP-1"  // @http.get / @http.post の付いた op に export がない
	codeHTTPParams    = "GOHTTP-2"  // 経路の op の引数が、0 個でも model 一つでもない
	codeHTTPPlace     = "GOHTTP-3"  // 入力の model のフィールドが、@http.header / @http.query / @http.body のどれも持たない、または二つ以上持つ
	codeHTTPFieldType = "GOHTTP-4"  // @http.header か @http.query のフィールドの型が受け付けられない
	codeHTTPJSON      = "GOHTTP-5"  // 本文、結果、@http.status を持つ例外の値に、JSON にできない型がある
	codeHTTPGetBody   = "GOHTTP-6"  // @http.get の op の入力が @http.body のフィールドを持つ
	codeHTTPPath      = "GOHTTP-7"  // パスが / で始まらない、または { か } を含む
	codeHTTPDuplicate = "GOHTTP-8"  // 同じメソッドとパスの経路が二つある
	codeHTTPOption    = "GOHTTP-9"  // [emit.options.go-http] に、知らないキーか、型の違う値がある
	codeHTTPName      = "GOHTTP-10" // NewHandler が、N の定義のパッケージの名前と当たる
	codeHTTPValidate  = "GOHTTP-11" // validate = true で、Validate を持つ入力の model の置き場の単位が、go-valid を並べていない
)

// FileName は、このトランスパイラが出すただ一つのファイルの名前（docs/plugin-go.md「相乗りのファイル」）。入口のパッケージに置く。
const FileName = "http.go"

// handlerName は、このトランスパイラが入口のパッケージに足す、ただ一つの公開の名前。
const handlerName = "NewHandler"

// 読む語彙（標準の http。docs/plugin-go-http.md「マニフェスト」）。@http.retry は読まない（印）。
const (
	httpGet    = "http.get"
	httpPost   = "http.post"
	httpHeader = "http.header"
	httpQuery  = "http.query"
	httpBody   = "http.body"
	httpStatus = "http.status"
)

// options は [emit.options.go-http]（docs/plugin-go-http.md「オプション」）。
type options struct {
	// Validate が true なら、入力を読んだ後、op を呼ぶ前に、入力の制約（@valid.*）を確かめる。
	Validate bool `json:"validate"`
}

// Output は、一つの出力 (U, N) についての応答の中身。Errors が一つでもあれば、Files も Warnings も空。
type Output struct {
	Files    []plugin.File
	Warnings []plugin.Warning
	Errors   []plugin.Diagnostic
}

// Emit は、要求の View の出力 (U, N) の http.go を作る（docs/plugin-go-http.md「出力」）。root はプロジェクトのルート
// （go.mod を探す起点）。診断があれば、ファイルは返さない。経路の op が一つもなければ、ファイルを出さず warning を一つ返す。
// 戻り値の error は、利用者の仕様の誤りでないもの（View が欠けている、生成したコードを整形できない）。
func Emit(req *plugin.EmitRequest, root string) (*Output, error) {
	view := req.View
	if view == nil || view.Program == nil || view.Program.Namespace(view.Viewer) == nil {
		viewer := ""
		if view != nil {
			viewer = view.Viewer
		}
		return nil, fmt.Errorf("View に、名前空間 %s の Program がない", viewer)
	}
	g := &generator{view: view, root: root, p: view.Program, ns: view.Program.Namespace(view.Viewer)}
	g.prepare()

	// GOHTTP-9
	unknown, err := plugin.Options(req, &g.opts)
	if err != nil {
		// オプションは validate だけなので、型の違う値は、その値
		g.opts = options{}
		value, _ := json.Marshal(view.Config.Options[TranspilerName]["validate"])
		g.errorf(codeHTTPOption, "", "%d 番目の [[emit]] の [emit.options.%s] の validate の値 %s が、真偽値でない", view.Config.Index, TranspilerName, value)
	}
	for _, key := range unknown {
		g.errorf(codeHTTPOption, "", "%d 番目の [[emit]] の [emit.options.%s] に、知らないキー %s がある（あるのは validate）", view.Config.Index, TranspilerName, key)
	}

	routes := g.check()
	if len(g.errs) > 0 {
		return &Output{Errors: g.errs}, nil
	}
	if len(routes) == 0 {
		return &Output{Warnings: []plugin.Warning{{At: view.Viewer,
			Message: fmt.Sprintf("名前空間 %s の単位の対象に @%s か @%s の付いた op がないので、%s を出さない", view.Viewer, httpGet, httpPost, FileName)}}}, nil
	}
	// 置き場。go.mod がなければ、基底が GO-1 を返すので、ここでは何も返さない
	layout, err := golang.LayoutOf(view, root)
	if err != nil {
		if errors.Is(err, golang.ErrNoGoMod) {
			return &Output{}, nil
		}
		return nil, fmt.Errorf("名前空間 %s の出力先 %q の import パスを求められない: %w", view.Viewer, view.Config.Dir, err)
	}
	g.layout = layout
	content, err := g.render(routes)
	if err != nil {
		return nil, err
	}
	return &Output{Files: []plugin.File{{Path: path.Join(layout.Dir, FileName), Content: content}}}, nil
}

// generator は一つの出力 (U, N) の http.go を作る状態。
type generator struct {
	view   *ir.View
	root   string
	p      *ir.Program
	ns     *ir.Namespace // N
	opts   options
	layout *golang.Layout

	declNS map[string]string // 宣言の FQN → 宣言した名前空間
	files  map[string]bool   // 単位の対象のファイル（View.files）

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

// place は、入力の model のフィールドを読む、通信の上の場所の種類。
type place int

const (
	inHeader place = iota + 1
	inQuery
	inBody
)

// input は、入力の model のフィールド一つの読み方（docs/plugin-go-http.md「入力の読み方」）。
type input struct {
	field *ir.Field
	place place
	// header は @http.header の name。
	header string
	// optional は、フィールドの型が省略可能か。value は、省略可能を外した型。
	optional bool
	value    ir.Type
	// core は、ヘッダとクエリの値の、元の核の型（scalar は辿る）。
	core ir.CoreName
}

// route は経路一つ: 経路の op と、メソッドとパス。
type route struct {
	op           *ir.Op
	method, path string
	// model は入力の model。引数のない op なら nil。inputs は、そのフィールドの読み方（フィールドの順）。
	model  *ir.Model
	inputs []input
	// validate は、op を呼ぶ前に、入力の制約を確かめるか（単位のオプション validate = true で、入力の model が Validate を持つ）。
	validate bool
}

// bodies は、@http.body のフィールドの数。
func (r *route) bodies() int {
	n := 0
	for _, in := range r.inputs {
		if in.place == inBody {
			n++
		}
	}
	return n
}

// location は、フィールド in の、通信の上の場所（docs/plugin-go-http.md「入力の読み方」: httpPlace の二つ目と、違反の path の先頭）。
func (r *route) location(in *input) string {
	switch in.place {
	case inHeader:
		return "header." + in.header
	case inQuery:
		return "query." + in.field.Name
	}
	if r.bodies() == 1 {
		return "body"
	}
	return "body." + in.field.Name
}

// status は、@http.status を持つ例外一つ。
type status struct {
	exception *ir.Exception
	code      int64
}

// check は、N の単位の対象の op を宣言順に見て、診断（GOHTTP-1〜GOHTTP-8、GOHTTP-10、GOHTTP-11）を集め、経路を返す
// （docs/plugin-go-http.md「対象」）。
func (g *generator) check() []route {
	var routes []route
	taken := map[string]string{} // "<メソッド> <パス>" → 先に使った op
	for _, d := range g.ns.Decls {
		op, ok := d.(*ir.Op)
		if !ok || !g.files[op.Span.File] {
			continue
		}
		var apps []*ir.Application
		for i := range op.Decorators {
			if dec := op.Decorators[i].Decorator; dec == httpGet || dec == httpPost {
				apps = append(apps, &op.Decorators[i])
			}
		}
		if len(apps) == 0 {
			continue
		}
		// GOHTTP-1
		if !op.Exported {
			g.errorf(codeHTTPExport, op.Name, "op %s に @%s が付いているが、export がない（ハンドラは、入口の Ops のメソッドとして op を呼ぶ。経路を黙って飛ばすことはしない）",
				op.Name, apps[0].Decorator)
		}
		// GOHTTP-2
		var model *ir.Model
		switch len(op.Params) {
		case 0:
		case 1:
			if r, isRef := op.Params[0].Type.(ir.RefType); isRef {
				model, _ = g.p.Lookup(r.Target).(*ir.Model)
			}
			if model == nil {
				g.errorf(codeHTTPParams, op.Name, "経路の op %s の引数 %s の型 %s が model でない（引数は、0 個か、入力の model 一つ）",
					op.Name, op.Params[0].Name, golang.TypeDisplay(op.Params[0].Type))
			}
		default:
			g.errorf(codeHTTPParams, op.Name, "経路の op %s の引数が %d 個ある（引数は、0 個か、入力の model 一つ）", op.Name, len(op.Params))
		}
		var inputs []input
		if model != nil {
			inputs = g.inputs(model)
		}
		r := route{op: op, model: model, inputs: inputs}
		r.validate = g.opts.Validate && model != nil && govalid.HasValidate(g.p, model.Name)
		// GOHTTP-11
		if r.validate {
			g.checkValidateHome(op, model)
		}
		// GOHTTP-5: 本文、結果、@http.status を持つ例外の値
		for i := range inputs {
			if inputs[i].place == inBody {
				g.checkJSON(op, inputs[i].field.Type, fmt.Sprintf("本文（model %s のフィールド %s）", model.Name, inputs[i].field.Name))
			}
		}
		g.checkJSON(op, op.Returns, "結果")
		for _, s := range g.statuses([]route{r}) {
			g.checkJSON(op, s.exception.Value, fmt.Sprintf("例外 %s の値", s.exception.Name))
		}
		for _, app := range apps {
			r.method = "GET"
			if app.Decorator == httpPost {
				r.method = "POST"
			}
			r.path, _ = stringArg(app, "path")
			// GOHTTP-6
			if r.method == "GET" && r.bodies() > 0 {
				g.errorf(codeHTTPGetBody, op.Name, "@%s の op %s の入力 %s が、@%s のフィールドを持つ（GET の要求は、本文を持たない）", httpGet, op.Name, model.Name, httpBody)
			}
			// GOHTTP-7
			switch {
			case !strings.HasPrefix(r.path, "/"):
				g.errorf(codeHTTPPath, op.Name, "op %s の @%s のパス %q が / で始まらない", op.Name, app.Decorator, r.path)
			case strings.ContainsAny(r.path, "{}"):
				g.errorf(codeHTTPPath, op.Name, "op %s の @%s のパス %q が { か } を含む（パスの中の値を入力のフィールドに結ぶ語彙が、まだない）", op.Name, app.Decorator, r.path)
			}
			// GOHTTP-8
			key := r.method + " " + r.path
			if prev, dup := taken[key]; dup {
				g.errorf(codeHTTPDuplicate, op.Name, "経路 %s が二つある: op %s と op %s", key, prev, op.Name)
			} else {
				taken[key] = op.Name
			}
			routes = append(routes, r)
		}
	}
	if len(routes) > 0 {
		g.checkName()
	}
	return routes
}

// checkValidateHome は GOHTTP-11: 制約を確かめる経路の op の入力の model m（Validate を持つ）が、別の名前空間 M のものなら、M の型の
// 置き場（docs/config.md「型の置き場」）の単位が、transpilers に go-valid を並べていなければならない（Validate は、型の定義の
// パッケージにしか書けない。並べていなければ、ハンドラの in.Validate() がコンパイルできない）。N の model は、自分の単位が go-valid を
// 並べている（CFG-19）。置き場は、View だけから求まる（govalid.Unlisted）。
func (g *generator) checkValidateHome(op *ir.Op, m *ir.Model) {
	ns := g.declNS[m.Name]
	if ns == g.view.Viewer {
		return
	}
	if unit, unlisted := govalid.Unlisted(g.view, ns); unlisted {
		g.errorf(codeHTTPValidate, op.Name, "経路の op %s の入力 %s は Validate を持つが、名前空間 %s の型の置き場の単位（%d 番目の [[emit]]）が、transpilers に %s を並べていない（validate = true のハンドラは、入力の Validate を呼ぶ。メソッドは、型の定義のパッケージにしか書けない）",
			op.Name, m.Name, ns, unit, govalid.Transpiler)
	}
}

// inputs は、入力の model m のフィールドの読み方（フィールドの順）。GOHTTP-3 と GOHTTP-4 を報告する（位置は model）。
func (g *generator) inputs(m *ir.Model) []input {
	var out []input
	for i := range m.Fields {
		f := &m.Fields[i]
		in := input{field: f, value: f.Type}
		if o, ok := f.Type.(ir.OptionalType); ok {
			in.optional, in.value = true, o.Of
		}
		var found []string
		for j := range f.Decorators {
			app := &f.Decorators[j]
			switch app.Decorator {
			case httpHeader:
				in.place = inHeader
				in.header, _ = stringArg(app, "name")
			case httpQuery:
				in.place = inQuery
			case httpBody:
				in.place = inBody
			default:
				continue
			}
			found = append(found, "@"+app.Decorator)
		}
		// GOHTTP-3
		if len(found) != 1 {
			if len(found) == 0 {
				g.errorf(codeHTTPPlace, m.Name, "入力の model %s のフィールド %s が、@%s、@%s、@%s のどれも持たない（ちょうど一つ要る）", m.Name, f.Name, httpHeader, httpQuery, httpBody)
			} else {
				g.errorf(codeHTTPPlace, m.Name, "入力の model %s のフィールド %s が、%s を持つ（ちょうど一つ要る）", m.Name, f.Name, strings.Join(found, " と "))
			}
			continue
		}
		// GOHTTP-4
		core, isCore := g.coreOf(in.value)
		in.core = core
		switch in.place {
		case inHeader:
			if !isCore || core != ir.CoreString {
				g.errorf(codeHTTPFieldType, m.Name, "入力の model %s の @%s のフィールド %s の型 %s は受け付けられない（string か、その省略可能）",
					m.Name, httpHeader, f.Name, golang.TypeDisplay(f.Type))
				continue
			}
		case inQuery:
			if !isCore || core != ir.CoreString && core != ir.CoreInt && core != ir.CoreFloat && core != ir.CoreBoolean {
				g.errorf(codeHTTPFieldType, m.Name, "入力の model %s の @%s のフィールド %s の型 %s は受け付けられない（string / int / float / boolean か、その省略可能）",
					m.Name, httpQuery, f.Name, golang.TypeDisplay(f.Type))
				continue
			}
		}
		out = append(out, in)
	}
	return out
}

// coreOf は、型 t の元の核の型（scalar は、その継承を辿る）。核の型を元に持たなければ false。
func (g *generator) coreOf(t ir.Type) (ir.CoreName, bool) {
	for depth := 0; depth < 64; depth++ {
		switch x := t.(type) {
		case ir.CoreType:
			return x.Name, true
		case ir.RefType:
			s, ok := g.p.Lookup(x.Target).(*ir.Scalar)
			if !ok {
				return "", false
			}
			t = s.Base
		default:
			return "", false
		}
	}
	return "", false
}

// stringArg は、適用 app の引数 param の文字列のリテラル。
func stringArg(app *ir.Application, param string) (string, bool) {
	for _, a := range app.Args {
		if a.Param == param {
			if l, ok := a.Value.(ir.LiteralValue); ok {
				if s, ok := l.Literal.(ir.StringLiteral); ok {
					return s.Value, true
				}
			}
		}
	}
	return "", false
}

// statuses は、経路 routes の op のどれかが投げうる例外（Op.signature.exceptions）のうち、@http.status を持つもの。
// 最初に現れた順（op の宣言順、その中は exceptions の順。docs/plugin-go-http.md「補助」）。
func (g *generator) statuses(routes []route) []status {
	var out []status
	seen := map[string]bool{}
	for _, r := range routes {
		for _, fqn := range r.op.Signature.Exceptions {
			if seen[fqn] {
				continue
			}
			seen[fqn] = true
			x, ok := g.p.Lookup(fqn).(*ir.Exception)
			if !ok {
				continue
			}
			for i := range x.Decorators {
				app := &x.Decorators[i]
				if app.Decorator != httpStatus {
					continue
				}
				for _, a := range app.Args {
					if l, ok := a.Value.(ir.LiteralValue); ok && a.Param == "code" {
						if n, ok := l.Literal.(ir.IntLiteral); ok {
							out = append(out, status{exception: x, code: n.Value})
						}
					}
				}
			}
		}
	}
	return out
}

// checkJSON は GOHTTP-5: 型 t（what: 本文、結果、例外の値）が JSON にできる型でなければ、op の位置に報告する。
func (g *generator) checkJSON(op *ir.Op, t ir.Type, what string) {
	if union, bad := g.unionIn(t, map[string]bool{}); bad {
		g.errorf(codeHTTPJSON, op.Name, "経路の op %s の%sの型 %s が、union %s を中に持つ（union の通信の上の形を、まだ決めていない）",
			op.Name, what, golang.TypeDisplay(t), union)
	}
}

// unionIn は、型 t の中の union（arms）の FQN（docs/plugin-go-http.md「対象」の「JSON にできる型」: 核の型、scalar、model、
// optional、array、record と、それらの組み合わせのほかは、JSON にできない）。なければ false。
func (g *generator) unionIn(t ir.Type, visited map[string]bool) (string, bool) {
	switch x := t.(type) {
	case ir.OptionalType:
		return g.unionIn(x.Of, visited)
	case ir.ArrayType:
		return g.unionIn(x.Of, visited)
	case ir.CaseType:
		return g.unionIn(x.Of, visited)
	case ir.RecordType:
		if u, bad := g.unionIn(x.Key, visited); bad {
			return u, true
		}
		return g.unionIn(x.Value, visited)
	case ir.RefType:
		if visited[x.Target] {
			return "", false
		}
		visited[x.Target] = true
		switch d := g.p.Lookup(x.Target).(type) {
		case *ir.Union:
			return d.Name, true
		case *ir.Model:
			for _, f := range d.Fields {
				if u, bad := g.unionIn(f.Type, visited); bad {
					return u, true
				}
			}
		}
	}
	return "", false
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
		case golang.PublicNewModel:
			what = "既定値を持つ model " + n.Decl + " の関数"
		case golang.PublicArm:
			what = "union " + n.Decl + " のアーム " + n.Detail + " を包む型"
		case golang.PublicEffect:
			what = "effect " + n.Decl + " の定数"
		case golang.PublicClassify:
			what = "classify " + n.Decl + " の型"
		case golang.PublicCase:
			what = "classify " + n.Decl + " のケース " + n.Detail + " の定数"
			if n.Detail == "default" {
				what = "classify " + n.Decl + " の default の定数"
			}
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
