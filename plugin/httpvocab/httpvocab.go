// Package httpvocab は、標準の語彙 http（docs/vocab.md「`http` の語彙」）の、言語に依らない読み解き（docs/plugin-http.md）。
//
// 仕様（ir.Program）から、経路、入力の model のフィールドの読み方、通信に出る union を求め、書いたものの整合の検査（HTTP-*。
// 同「診断」）を返す。読むのは、仕様だけ: 単位にも、言語にも、依らない。
//
// 語彙のプラグイン http（plugins/http）は、ここの診断を、そのまま返す。http の語彙を、ある言語に出すトランスパイラ
// （plugins/go-http）は、同じ読み解きの結果から、コードを作る（決定 D428: 同じ読み解きを、語彙のプラグインと、
// 言語のプラグインで、分け合うのは、ライブラリで行う）。
package httpvocab

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/validvocab"
)

// Namespace は、この語彙の名前空間（マニフェストの [read] の namespaces。docs/plugin-http.md「マニフェスト」）。
const Namespace = "http"

// 読むデコレータの FQN。@http.retry は読まない（印）。
const (
	Get       = "http.get"
	Post      = "http.post"
	Header    = "http.header"
	Query     = "http.query"
	Body      = "http.body"
	HeaderObj = "http.headerObj"
	QueryObj  = "http.queryObj"
	BodyObj   = "http.bodyObj"
	Required  = "http.required"
	Default   = "http.default"
	Status    = "http.status"
)

// 診断の ID（docs/plugin-http.md「診断」）。
const (
	CodeExport     = "HTTP-1"  // @http.get / @http.post の付いた op に export がない
	CodeParams     = "HTTP-2"  // 経路の op の引数が、0 個でも、フィールドを持つ model 一つでもない
	CodePlace      = "HTTP-3"  // 入力の model のフィールドが、場所の六つのデコレータのどれも持たない、または二つ以上持つ
	CodeFieldType  = "HTTP-4"  // ヘッダかクエリから読むフィールド（全体の形なら、その model のフィールド）の型が受け付けられない
	CodeJSON       = "HTTP-5"  // 本文、結果、@http.status を持つ例外の引数の型に、通信に出せない union がある
	CodeGetBody    = "HTTP-6"  // @http.get の op の入力が、本文のデコレータ（@http.body、@http.bodyObj）のフィールドを持つ
	CodePath       = "HTTP-7"  // パスが / で始まらない、または { か } を含む
	CodeDuplicate  = "HTTP-8"  // 一つの名前空間の中に、同じメソッドとパスの経路が二つある
	CodeWholeType  = "HTTP-9"  // 全体の形を付けたフィールドの型が、フィールドを持つ model でない
	CodeWholeMix   = "HTTP-10" // 一つの場所に、全体の形が二つ以上ある、または、全体の形と項目の形の両方がある
	CodeNoItem     = "HTTP-11" // @http.required か @http.default の付いたフィールドが、項目の形を持たない
	CodeOptional   = "HTTP-12" // @http.required か @http.default の付いたフィールドの型が、省略可能
	CodeBoth       = "HTTP-13" // 一つのフィールドに、@http.required と @http.default の両方がある
	CodeDefault    = "HTTP-14" // @http.default のリテラルが、フィールドの型に渡せない
	CodeConstraint = "HTTP-15" // @http.default の値が、フィールドの制約（@valid.*）に合わない
	CodeLiteral    = "HTTP-16" // 本文の項目のフィールドの型が、リテラルの型そのもの（項目を、リテラルの型で読む形を、まだ決めていない）
)

// Place は、入力の model のフィールドを読む、通信の上の場所。
type Place int

const (
	InHeader Place = iota + 1
	InQuery
	InBody
)

// Word は、場所の、通信の上の名前（docs/plugin-http.md「通信の形」の「op を呼ぶ前の失敗」の、違反の path の先頭）。
func (p Place) Word() string {
	return [...]string{"", "header", "query", "body"}[p]
}

// label は、場所の、メッセージに書く名前。
func (p Place) label() string {
	return [...]string{"", "ヘッダ", "クエリ", "本文"}[p]
}

// readers は、要求のどこから読むかを言う六つのデコレータ: 三つの場所ごとの、項目の形と、全体の形（whole）。
var readers = map[string]struct {
	place Place
	whole bool
}{
	Header:    {InHeader, false},
	Query:     {InQuery, false},
	Body:      {InBody, false},
	HeaderObj: {InHeader, true},
	QueryObj:  {InQuery, true},
	BodyObj:   {InBody, true},
}

// Input は、入力の model のフィールド一つの読み方。中身は、フィールドに付いたデコレータが言ったことだけ。
type Input struct {
	Field *ir.Field
	// Decorator は、場所のデコレータの FQN（http.query）。Place は場所、Whole は全体の形か。
	Decorator string
	Place     Place
	Whole     bool
	// Name は、項目の形の、項目の名前: デコレータの引数。省いていれば、フィールドの名前。全体の形では ""。
	Name string
	// Required は @http.required が付いているか。Default は @http.default のリテラル（なければ nil）。
	Required bool
	Default  ir.Literal
}

// Location は、通信の上の場所: 項目の形は `<場所>.<名前>`、全体の形は `<場所>`。
func (in *Input) Location() string {
	if in.Whole {
		return in.Place.Word()
	}
	return in.Place.Word() + "." + in.Name
}

// Route は経路一つ: 経路の op と、メソッドとパス。
type Route struct {
	Op           *ir.Op
	Method, Path string
	// Model は入力の model。引数のない op なら nil。Inputs は、そのフィールドの読み方（フィールドの順）。
	Model  *ir.Model
	Inputs []Input
}

// HasBody は、入力が、本文のデコレータ（@http.body、@http.bodyObj）のフィールドを持つか。
func (r *Route) HasBody() bool {
	for i := range r.Inputs {
		if r.Inputs[i].Place == InBody {
			return true
		}
	}
	return false
}

// Union は、通信に出せる union（docs/plugin-http.md「通信の形」の「union」）一つ。
type Union struct {
	Decl *ir.Union
	Arms []Arm
	// Tag は、印のキー: model のアームの、すべてが持つ、同じ名前の、リテラルの型のフィールドで、値が、アームごとに違うもの。
	// 印を持たなければ ""（model のアームは、決め手で見分ける）。
	Tag string
}

// Arm は、通信に出せる union のアーム一つ。
type Arm struct {
	Name string
	// Model は、アームの型の model（フィールドを持つ）。リテラルの型のアームなら nil。
	Model *ir.Model
	// Literal は、リテラルの型のアームの、そのリテラル。model のアームなら nil。
	Literal ir.Literal
	// Keys は、決め手: そのアームの model の、省略可能でないフィールドのうち、ほかのどの model のアームの、どのフィールドとも、
	// 名前が違うもの。印を持つ union と、リテラルの型のアームでは、空。
	Keys []string
	// TagValue は、印を持つ union の、model のアームの、印の値。
	TagValue ir.Literal
}

// ExceptionStatus は、@http.status を持つ例外と、そのステータス。
type ExceptionStatus struct {
	Exception *ir.Exception
	Code      int64
}

// Reader は、仕様一つを読み解く。入力の model と union は、一度だけ調べ、診断も、一度だけ返す。
type Reader struct {
	p *ir.Program
	// read は、入力の model の FQN → そのフィールドの読み方。
	read map[string][]Input
	// unions は、通信に出せる union（最初に現れた順）。unionOf は、調べた union（出せないものも）。
	unions  []*Union
	unionOf map[string]*wireUnion
	// consts は、通信に出る型の中の、リテラルの型のフィールドを持つ model（最初に現れた順）。
	consts  []*ir.Model
	constOf map[string]bool

	errs []plugin.Diagnostic
	seen map[string]bool
}

// wireUnion は、調べた union: 出せるなら ok、出せないなら problem（理由）。
type wireUnion struct {
	decl    *ir.Union
	ok      *Union
	problem string
}

// NewReader は、仕様 p を読み解く Reader。
func NewReader(p *ir.Program) *Reader {
	return &Reader{p: p, read: map[string][]Input{}, unionOf: map[string]*wireUnion{}, constOf: map[string]bool{}, seen: map[string]bool{}}
}

// Check は、仕様 p の、すべての名前空間の経路を読み解いて、診断（HTTP-*）を返す（docs/plugin-http.md「対象」「診断」）。
// 並びは、名前空間の IR の順、その中は op の宣言順。
func Check(p *ir.Program) []plugin.Diagnostic {
	r := NewReader(p)
	if p != nil {
		for i := range p.Namespaces {
			r.Routes(p.Namespaces[i].Name, nil)
		}
	}
	return r.Errors()
}

// Errors は、ここまでに読み解いたものの診断（HTTP-*）。返した順。同じ診断は、一度だけ。
func (r *Reader) Errors() []plugin.Diagnostic { return r.errs }

// Unions は、ここまでに読み解いた経路の、本文、結果、@http.status を持つ例外の引数の型の中の、通信に出せる union。最初に現れた順。
func (r *Reader) Unions() []*Union { return r.unions }

// LiteralModels は、Routes が見た経路の、本文、結果、@http.status を持つ例外の引数の型の中の、リテラルの型のフィールドを持つ
// model（最初に現れた順。union と同じ辿り方）。通信の上では、そのフィールドも、キーと値を持つ（docs/plugin-http.md「通信の形」）。
func (r *Reader) LiteralModels() []*ir.Model { return r.consts }

// LiteralJSON は、リテラル lit の JSON（docs/plugin-http.md「通信の形」の表の、リテラルの型の行）。
func LiteralJSON(lit ir.Literal) string {
	var v any
	switch x := lit.(type) {
	case ir.StringLiteral:
		v = x.Value
	case ir.IntLiteral:
		v = x.Value
	case ir.FloatLiteral:
		v = x.Value
	case ir.BooleanLiteral:
		v = x.Value
	case ir.DurationLiteral:
		v = int64(x.Value)
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(data)
}

// sameJSON は、二つのリテラルの JSON が、同じ値か（数は、数として比べる: 整数の 1 と、小数の 1.0 は、同じ）。
func sameJSON(a, b ir.Literal) bool {
	num := func(l ir.Literal) (float64, bool) {
		switch x := l.(type) {
		case ir.IntLiteral:
			return float64(x.Value), true
		case ir.FloatLiteral:
			return x.Value, true
		case ir.DurationLiteral:
			return float64(x.Value), true
		}
		return 0, false
	}
	if x, ok := num(a); ok {
		y, isNum := num(b)
		return isNum && x == y
	}
	switch x := a.(type) {
	case ir.StringLiteral:
		y, ok := b.(ir.StringLiteral)
		return ok && x.Value == y.Value
	case ir.BooleanLiteral:
		y, ok := b.(ir.BooleanLiteral)
		return ok && x.Value == y.Value
	}
	return false
}

func (r *Reader) errorf(code, at, format string, args ...any) {
	d := plugin.Diagnostic{Code: code, At: at, Message: fmt.Sprintf(format, args...)}
	key := d.Code + "\x00" + d.At + "\x00" + d.Message
	if r.seen[key] {
		return
	}
	r.seen[key] = true
	r.errs = append(r.errs, d)
}

// display は、メッセージに書く、型（docs/grammar.md「型」の綴り）。
func display(t ir.Type) string {
	if t == nil {
		return ""
	}
	return t.String()
}

// Routes は、名前空間 namespace の op のうち、target が真のもの（nil なら、すべて）を宣言順に見て、経路を返し、診断を集める
// （docs/plugin-http.md「対象」）。HTTP-8（同じメソッドとパス）は、この呼び出しが見た経路の中で比べる。
func (r *Reader) Routes(namespace string, target func(op *ir.Op) bool) []Route {
	ns := r.p.Namespace(namespace)
	if ns == nil {
		return nil
	}
	var routes []Route
	taken := map[string]string{} // "<メソッド> <パス>" → 先に使った op
	for _, d := range ns.Decls {
		op, ok := d.(*ir.Op)
		if !ok || target != nil && !target(op) {
			continue
		}
		var apps []*ir.Application
		for i := range op.Decorators {
			if dec := op.Decorators[i].Decorator; dec == Get || dec == Post {
				apps = append(apps, &op.Decorators[i])
			}
		}
		if len(apps) == 0 {
			continue
		}
		// HTTP-1
		if !op.Exported {
			r.errorf(CodeExport, op.Name, "op %s に @%s が付いているが、export がない（経路の op は、名前空間の外からの呼び出しが届く入口。経路を黙って飛ばすことはしない）",
				op.Name, apps[0].Decorator)
		}
		// HTTP-2
		var model *ir.Model
		switch len(op.Params) {
		case 0:
		case 1:
			if ref, isRef := op.Params[0].Type.(ir.RefType); isRef {
				model, _ = r.p.Lookup(ref.Target).(*ir.Model)
			}
			if model != nil && model.Index != nil {
				model = nil // インデックスの model は、フィールドを持たないので、入力の model にできない
			}
			if model == nil {
				r.errorf(CodeParams, op.Name, "経路の op %s の引数 %s の型 %s が、フィールドを持つ model でない（引数は、0 個か、入力の model 一つ。インデックスの model は、入力の model にできない）",
					op.Name, op.Params[0].Name, display(op.Params[0].Type))
			}
		default:
			r.errorf(CodeParams, op.Name, "経路の op %s の引数が %d 個ある（引数は、0 個か、入力の model 一つ）", op.Name, len(op.Params))
		}
		route := Route{Op: op, Model: model}
		if model != nil {
			// HTTP-3、HTTP-4、HTTP-9〜HTTP-15（位置は model）
			route.Inputs = r.inputs(model)
		}
		// HTTP-5: 本文、結果、@http.status を持つ例外の引数の型
		for i := range route.Inputs {
			if in := &route.Inputs[i]; in.Place == InBody {
				// HTTP-16: 本文の項目のフィールドの型が、リテラルの型そのもの
				if lt, direct := in.Field.Type.(ir.LiteralType); direct {
					r.errorf(CodeLiteral, op.Name, "経路の op %s の本文の項目（model %s のフィールド %s）の型が、リテラルの型 %s そのもの。本文の項目を、リテラルの型で読む形を、まだ決めていない",
						op.Name, model.Name, in.Field.Name, display(lt))
					continue
				}
				r.checkJSON(op, in.Field.Type, fmt.Sprintf("本文（model %s のフィールド %s）の型", model.Name, in.Field.Name))
			}
		}
		r.checkJSON(op, op.Returns, "結果の型")
		for _, s := range Statuses(r.p, []Route{route}) {
			// 例外の value は、引数の名前をキーにしたオブジェクト（「通信の形」の「失敗の応答」）: 引数（Exception.params）の型を、一つずつ見る
			for _, p := range s.Exception.Params {
				r.checkJSON(op, p.Type, fmt.Sprintf("例外 %s の引数 %s の型", s.Exception.Name, p.Name))
			}
		}
		for _, app := range apps {
			route.Method = "GET"
			if app.Decorator == Post {
				route.Method = "POST"
			}
			route.Path, _ = stringArg(app, "path")
			// HTTP-6
			if route.Method == "GET" && route.HasBody() {
				r.errorf(CodeGetBody, op.Name, "@%s の op %s の入力 %s が、本文のデコレータ（@%s、@%s）のフィールドを持つ（GET の要求は、本文を持たない）",
					Get, op.Name, model.Name, Body, BodyObj)
			}
			// HTTP-7
			switch {
			case !strings.HasPrefix(route.Path, "/"):
				r.errorf(CodePath, op.Name, "op %s の @%s のパス %q が / で始まらない", op.Name, app.Decorator, route.Path)
			case strings.ContainsAny(route.Path, "{}"):
				r.errorf(CodePath, op.Name, "op %s の @%s のパス %q が { か } を含む（パスの中の値を入力のフィールドに結ぶ語彙が、まだない）", op.Name, app.Decorator, route.Path)
			}
			// HTTP-8
			key := route.Method + " " + route.Path
			if prev, dup := taken[key]; dup {
				r.errorf(CodeDuplicate, op.Name, "経路 %s が二つある: op %s と op %s", key, prev, op.Name)
			} else {
				taken[key] = op.Name
			}
			routes = append(routes, route)
		}
	}
	return routes
}

// Statuses は、経路 routes の op のどれかが投げうる例外（Op.signature.exceptions）のうち、@http.status を持つもの。
// 最初に現れた順（op の宣言順、その中は exceptions の順）。
func Statuses(p *ir.Program, routes []Route) []ExceptionStatus {
	var out []ExceptionStatus
	seen := map[string]bool{}
	for _, r := range routes {
		for _, fqn := range r.Op.Signature.Exceptions {
			if seen[fqn] {
				continue
			}
			seen[fqn] = true
			x, ok := p.Lookup(fqn).(*ir.Exception)
			if !ok {
				continue
			}
			for i := range x.Decorators {
				app := &x.Decorators[i]
				if app.Decorator != Status {
					continue
				}
				if l, ok := argLiteral(app, "code"); ok {
					if n, ok := l.(ir.IntLiteral); ok {
						out = append(out, ExceptionStatus{Exception: x, Code: n.Value})
					}
				}
			}
		}
	}
	return out
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

// stringArg は、適用 app の引数 param の文字列のリテラル。
func stringArg(app *ir.Application, param string) (string, bool) {
	l, _ := argLiteral(app, param)
	s, ok := l.(ir.StringLiteral)
	return s.Value, ok
}

// coreOf は、型 t の元の核の型（scalar は、その継承を辿る）。核の型を元に持たなければ false。
func (r *Reader) coreOf(t ir.Type) (ir.CoreName, bool) {
	for depth := 0; depth < 64; depth++ {
		switch x := t.(type) {
		case ir.CoreType:
			return x.Name, true
		case ir.RefType:
			s, ok := r.p.Lookup(x.Target).(*ir.Scalar)
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

// inputs は、入力の model m のフィールドの読み方（フィールドの順）。model ごとに一度だけ調べ、診断も一度だけ返す。
// 場所のデコレータをちょうど一つ持たないフィールド（HTTP-3）と、全体の形で model でないフィールド（HTTP-9）は、読み方に入れない。
func (r *Reader) inputs(m *ir.Model) []Input {
	if out, done := r.read[m.Name]; done {
		return out
	}
	out := []Input{}
	for i := range m.Fields {
		f := &m.Fields[i]
		var found []string
		var in Input
		item := false // 項目の形のデコレータを、一つでも持つか
		for j := range f.Decorators {
			app := &f.Decorators[j]
			rd, ok := readers[app.Decorator]
			if !ok {
				continue
			}
			found = append(found, "@"+app.Decorator)
			in = Input{Field: f, Decorator: app.Decorator, Place: rd.place, Whole: rd.whole}
			if !rd.whole {
				item = true
				if in.Name, ok = stringArg(app, "name"); !ok {
					in.Name = f.Name
				}
			}
		}
		// HTTP-11〜HTTP-15
		in.Required, in.Default = r.checkMissing(m, f, item, "")
		// HTTP-3
		switch len(found) {
		case 0:
			r.errorf(CodePlace, m.Name, "入力の model %s のフィールド %s が、要求のどこから読むかを言うデコレータ（@%s / @%s / @%s / @%s / @%s / @%s）のどれも持たない（ちょうど一つ要る）",
				m.Name, f.Name, Header, Query, Body, HeaderObj, QueryObj, BodyObj)
			continue
		case 1:
		default:
			r.errorf(CodePlace, m.Name, "入力の model %s のフィールド %s が、%s を持つ（要求のどこから読むかを言うデコレータは、ちょうど一つ要る）", m.Name, f.Name, strings.Join(found, " と "))
			continue
		}
		if in.Whole {
			// HTTP-9
			whole := r.wholeModel(f.Type)
			if whole == nil {
				r.errorf(CodeWholeType, m.Name, "入力の model %s の @%s のフィールド %s の型 %s が、フィールドを持つ model でない（全体の形は、型が model のフィールドにだけ付けられる。省略可能な model、配列、インデックスの model には、付けられない）",
					m.Name, in.Decorator, f.Name, display(f.Type))
				continue
			}
			// 全体の形では、その model のフィールドの一つずつを、同じ場所の項目として見る
			for k := range whole.Fields {
				inner := &whole.Fields[k]
				// HTTP-4（本文の全体は、op の位置の HTTP-5）
				r.checkPlaceType(whole, inner, in.Place, fmt.Sprintf("@%s で受け取る model %s のフィールド %s", in.Decorator, whole.Name, inner.Name))
				// HTTP-11 ほか: 全体の形で受け取る model の中のフィールドには、付けられない
				r.checkMissing(whole, inner, false, fmt.Sprintf("model %s が、この model を @%s（全体の形）で受け取る", m.Name, in.Decorator))
			}
		} else {
			// HTTP-4
			r.checkPlaceType(m, f, in.Place, fmt.Sprintf("入力の model %s の @%s のフィールド %s", m.Name, in.Decorator, f.Name))
		}
		out = append(out, in)
	}
	r.checkWholes(m, out)
	r.read[m.Name] = out
	return out
}

// wholeModel は、全体の形のフィールドの型 t の model。t が model への参照でなければ nil（省略可能な model、配列は、model ではない）。
func (r *Reader) wholeModel(t ir.Type) *ir.Model {
	ref, ok := t.(ir.RefType)
	if !ok {
		return nil
	}
	m, _ := r.p.Lookup(ref.Target).(*ir.Model)
	if m != nil && m.Index != nil {
		// インデックスの model は、フィールドを持たないので、全体の形にできない（docs/plugin-http.md「対象」）
		return nil
	}
	return m
}

// checkPlaceType は HTTP-4: model m のフィールド f を、ヘッダかクエリの項目として読むとき、型（scalar は元の核の型。省略可能は
// 外して見る）が受け付けられるか（docs/plugin-http.md「対象」の表）。本文（JSON にできる型）は、op の位置の HTTP-5 が見る。
// subject は、メッセージに書く、そのフィールド。
func (r *Reader) checkPlaceType(m *ir.Model, f *ir.Field, p Place, subject string) {
	t := f.Type
	if o, ok := t.(ir.OptionalType); ok {
		t = o.Of
	}
	core, isCore := r.coreOf(t)
	switch p {
	case InHeader:
		if !isCore || core != ir.CoreString {
			r.errorf(CodeFieldType, m.Name, "%s の型 %s は、ヘッダから読めない（string か、その省略可能）", subject, display(f.Type))
		}
	case InQuery:
		if !isCore || core != ir.CoreString && core != ir.CoreInt && core != ir.CoreFloat && core != ir.CoreBoolean {
			r.errorf(CodeFieldType, m.Name, "%s の型 %s は、クエリから読めない（string / int / float / boolean か、その省略可能）", subject, display(f.Type))
		}
	}
}

// checkWholes は HTTP-10: 入力の model m の一つの場所に、全体の形が二つ以上ある、または、全体の形と項目の形の両方がある。
func (r *Reader) checkWholes(m *ir.Model, inputs []Input) {
	for _, p := range []Place{InHeader, InQuery, InBody} {
		var wholes, items []string
		for i := range inputs {
			in := &inputs[i]
			if in.Place != p {
				continue
			}
			if in.Whole {
				wholes = append(wholes, in.Field.Name)
			} else {
				items = append(items, in.Field.Name)
			}
		}
		if len(wholes) > 1 {
			r.errorf(CodeWholeMix, m.Name, "入力の model %s が、%sの全体の形を二つ以上持つ（フィールド %s。一つの場所に、高々一つ）",
				m.Name, p.label(), strings.Join(wholes, "、"))
		}
		if len(wholes) > 0 && len(items) > 0 {
			r.errorf(CodeWholeMix, m.Name, "入力の model %s が、%sに、全体の形（フィールド %s）と項目の形（フィールド %s）の両方を持つ（同じ場所には、一緒に書けない）",
				m.Name, p.label(), strings.Join(wholes, "、"), strings.Join(items, "、"))
		}
	}
}

// checkMissing は、model m のフィールド f の、項目がないときの扱いを言うデコレータ（@http.required、@http.default）を読み、
// 四つの決まり（docs/plugin-http.md「対象」）と、補う値の制約（同「補う値と制約」）を確かめる。item は、f が項目の形のデコレータを
// 持つか。via は、f が、全体の形で受け取る model の中のフィールドのときの、メッセージに書く、その受け取り方（そのとき、item は false）。
// 戻り値は、デコレータが言ったこと: required（@http.required があるか）と、def（@http.default のリテラル。なければ nil）。
func (r *Reader) checkMissing(m *ir.Model, f *ir.Field, item bool, via string) (required bool, def ir.Literal) {
	var found []string
	for i := range f.Decorators {
		app := &f.Decorators[i]
		switch app.Decorator {
		case Required:
			required = true
		case Default:
			def, _ = argLiteral(app, "value")
		default:
			continue
		}
		found = append(found, "@"+app.Decorator)
	}
	if len(found) == 0 {
		return false, nil
	}
	subject := fmt.Sprintf("model %s のフィールド %s", m.Name, f.Name)
	// HTTP-11
	if !item {
		why := fmt.Sprintf("項目の形（@%s / @%s / @%s）を持たない", Header, Query, Body)
		if via != "" {
			why = via
		}
		r.errorf(CodeNoItem, m.Name, "%s に %s が付いているが、%s（全体の形の中の項目は、なければ、いつも零値のまま。扱いを言いたい項目は、項目の形で書く）",
			subject, strings.Join(found, " と "), why)
	}
	// HTTP-12
	if _, optional := f.Type.(ir.OptionalType); optional {
		r.errorf(CodeOptional, m.Name, "%s に %s が付いているが、型 %s が省略可能（型は「なくてよい」と言っている。なければ誤りにするか、値を入れるなら、省略可能にしない）",
			subject, strings.Join(found, " と "), display(f.Type))
	}
	// HTTP-13
	if required && def != nil {
		r.errorf(CodeBoth, m.Name, "%s に、@%s と @%s の両方が付いている（項目がなければ誤りにするか、値を入れるかの、どちらか）", subject, Required, Default)
	}
	if def != nil {
		r.checkDefault(m, f, def)
	}
	return required, def
}

// checkDefault は、@http.default のリテラル lit を、model m のフィールド f の宣言と突き合わせる。補う値についての検査は、ここだけ:
//
//   - HTTP-14: リテラルが、フィールドの型（scalar は元の核の型）に渡せない。
//   - HTTP-15: 値が、フィールドの制約（フィールドに付いた @valid.* と、型の scalar に付いた @valid.*）に合わない。
//     確かめるのは、go-cli の @cli.default と同じ validvocab.CheckLiteral。
func (r *Reader) checkDefault(m *ir.Model, f *ir.Field, lit ir.Literal) {
	// HTTP-14
	if !r.assignable(lit, f.Type) {
		r.errorf(CodeDefault, m.Name, "model %s のフィールド %s の @%s のリテラル %s（%s）は、型 %s に渡せない",
			m.Name, f.Name, Default, literalDisplay(lit), lit.Kind(), display(f.Type))
		return
	}
	// HTTP-15
	for _, x := range validvocab.CheckLiteral(r.p, f, lit) {
		owner := "フィールドの制約"
		if x.Scalar != "" {
			owner = "型の scalar " + x.Scalar + " の制約"
		}
		r.errorf(CodeConstraint, m.Name, "model %s のフィールド %s の @%s の値 %s が、%s %s に合わない（補う値は、制約に合う値にする）",
			m.Name, f.Name, Default, literalDisplay(lit), owner, x.Text)
	}
}

// assignable は、リテラル lit を、型 t の位置に渡せるか（docs/types.md「互換」の 10 と 2。scalar は、元の核の型で見る）:
// 文字列は string、整数は int か float、小数は float、真偽値は boolean、期間は Duration。
func (r *Reader) assignable(lit ir.Literal, t ir.Type) bool {
	if o, ok := t.(ir.OptionalType); ok {
		t = o.Of
	}
	core, ok := r.coreOf(t)
	if !ok {
		return false
	}
	switch lit.(type) {
	case ir.StringLiteral:
		return core == ir.CoreString
	case ir.IntLiteral:
		return core == ir.CoreInt || core == ir.CoreFloat
	case ir.FloatLiteral:
		return core == ir.CoreFloat
	case ir.BooleanLiteral:
		return core == ir.CoreBoolean
	case ir.DurationLiteral:
		return core == ir.CoreDuration
	}
	return false
}

// literalDisplay は、メッセージに書く、リテラル: 文字列は引用符付き、整数は 10 進、小数は 'g'（. も e も含まなければ .0 を足す）、
// 真偽値は true / false、期間は、Go の time.Duration の表示（1h30m0s）。
func literalDisplay(lit ir.Literal) string {
	switch x := lit.(type) {
	case ir.StringLiteral:
		return strconv.Quote(x.Value)
	case ir.IntLiteral:
		return strconv.FormatInt(x.Value, 10)
	case ir.FloatLiteral:
		s := strconv.FormatFloat(x.Value, 'g', -1, 64)
		if !strings.ContainsAny(s, ".e") {
			s += ".0"
		}
		return s
	case ir.BooleanLiteral:
		return strconv.FormatBool(x.Value)
	case ir.DurationLiteral:
		return x.Value.String()
	}
	return "nil"
}

// checkJSON は HTTP-5: 型 t（what: 本文、結果、例外の引数の、どの型か）の中の union が、どれも、通信に出せること。
// 出せる union は、最初に現れた順に、r.unions に集まる。
// 通信に出る、リテラルの型のフィールドを持つ model（r.consts）も、ここで集める。
func (r *Reader) checkJSON(op *ir.Op, t ir.Type, what string) {
	r.constsIn(t, map[string]bool{})
	for _, u := range r.unionsIn(t, map[string]bool{}) {
		if u.problem != "" {
			r.errorf(CodeJSON, op.Name, "経路の op %s の%s %s が、通信に出せない union %s を中に持つ: %s（通信に出せる union の条件は、docs/plugin-http.md「通信の形」の「union」）",
				op.Name, what, display(t), u.decl.Name, u.problem)
		}
	}
}

// unionsIn は、型 t の中の union（arms）を、辿った順に返す（アームの型の中の union も）。初めて見た union は、調べて、覚える。
func (r *Reader) unionsIn(t ir.Type, visited map[string]bool) []*wireUnion {
	switch x := t.(type) {
	case ir.OptionalType:
		return r.unionsIn(x.Of, visited)
	case ir.ArrayType:
		return r.unionsIn(x.Of, visited)
	case ir.CaseType:
		return r.unionsIn(x.Of, visited)
	case ir.RefType:
		if visited[x.Target] {
			return nil
		}
		visited[x.Target] = true
		switch d := r.p.Lookup(x.Target).(type) {
		case *ir.Union:
			if d.Variant != ir.UnionArms {
				return nil
			}
			out := []*wireUnion{r.wireUnion(d)}
			for _, a := range d.Arms {
				out = append(out, r.unionsIn(a.Type, visited)...)
			}
			return out
		case *ir.Model:
			var out []*wireUnion
			for _, f := range d.Fields {
				out = append(out, r.unionsIn(f.Type, visited)...)
			}
			if d.Index != nil {
				out = append(out, r.unionsIn(d.Index.Value, visited)...)
			}
			return out
		}
	}
	return nil
}

// constsIn は、型 t の中の、リテラルの型のフィールドを持つ model を、辿った順に、r.consts に足す（optional、array、model の
// フィールドとインデックスの値、union のアームの型を辿る）。
func (r *Reader) constsIn(t ir.Type, visited map[string]bool) {
	switch x := t.(type) {
	case ir.OptionalType:
		r.constsIn(x.Of, visited)
	case ir.ArrayType:
		r.constsIn(x.Of, visited)
	case ir.CaseType:
		r.constsIn(x.Of, visited)
	case ir.RefType:
		if visited[x.Target] {
			return
		}
		visited[x.Target] = true
		switch d := r.p.Lookup(x.Target).(type) {
		case *ir.Union:
			for _, a := range d.Arms {
				r.constsIn(a.Type, visited)
			}
		case *ir.Model:
			for _, f := range d.Fields {
				if _, direct := f.Type.(ir.LiteralType); direct && !r.constOf[d.Name] {
					r.constOf[d.Name] = true
					r.consts = append(r.consts, d)
				}
			}
			for _, f := range d.Fields {
				r.constsIn(f.Type, visited)
			}
			if d.Index != nil {
				r.constsIn(d.Index.Value, visited)
			}
		}
	}
}

// wireUnion は、union u を、通信に出せるか調べた結果（一度だけ調べる。docs/plugin-http.md「通信の形」の「union」の三つの条件）。
// 出せる union は、r.unions に足す。
func (r *Reader) wireUnion(u *ir.Union) *wireUnion {
	if w, ok := r.unionOf[u.Name]; ok {
		return w
	}
	w := &wireUnion{decl: u}
	r.unionOf[u.Name] = w
	// 1: アームの型は、フィールドを持つ model か、リテラルの型
	models := make([]*ir.Model, len(u.Arms))
	lits := make([]ir.Literal, len(u.Arms))
	for i, a := range u.Arms {
		switch x := a.Type.(type) {
		case ir.LiteralType:
			lits[i] = x.Literal
		case ir.RefType:
			if m, isModel := r.p.Lookup(x.Target).(*ir.Model); isModel && m.Index == nil {
				models[i] = m
			}
		}
		if models[i] == nil && lits[i] == nil {
			w.problem = fmt.Sprintf("アーム %s の型 %s が、フィールドを持つ model でも、リテラルの型でもない", a.Name, display(a.Type))
			return w
		}
	}
	// 2: リテラルの型のアームどうしは、JSON の値が違う
	for i := range u.Arms {
		for j := i + 1; j < len(u.Arms); j++ {
			if lits[i] != nil && lits[j] != nil && sameJSON(lits[i], lits[j]) {
				w.problem = fmt.Sprintf("アーム %s と、アーム %s の、リテラルの型の JSON の値が、同じ（%s）", u.Arms[i].Name, u.Arms[j].Name, LiteralJSON(lits[i]))
				return w
			}
		}
	}
	// 3: model のアームは、印か、決め手で見分ける
	ok := &Union{Decl: u, Tag: tagOf(models)}
	for i, a := range u.Arms {
		arm := Arm{Name: a.Name, Model: models[i], Literal: lits[i]}
		switch {
		case models[i] == nil:
		case ok.Tag != "":
			arm.TagValue = literalField(models[i], ok.Tag)
		default:
			others := map[string]bool{}
			for j := range models {
				if j != i && models[j] != nil {
					for _, f := range models[j].Fields {
						others[f.Name] = true
					}
				}
			}
			for _, f := range models[i].Fields {
				if _, optional := f.Type.(ir.OptionalType); !optional && !others[f.Name] {
					arm.Keys = append(arm.Keys, f.Name)
				}
			}
			if len(arm.Keys) == 0 {
				w.problem = fmt.Sprintf("アーム %s（%s）に、印も、そのアームだけが持つ、省略可能でないフィールドもない", a.Name, models[i].Name)
				return w
			}
		}
		ok.Arms = append(ok.Arms, arm)
	}
	w.ok = ok
	r.unions = append(r.unions, ok)
	return w
}

// literalField は、model m のフィールド name の型が、リテラルの型そのものなら、そのリテラル。そうでなければ nil。
func literalField(m *ir.Model, name string) ir.Literal {
	for _, f := range m.Fields {
		if f.Name == name {
			if lt, ok := f.Type.(ir.LiteralType); ok {
				return lt.Literal
			}
		}
	}
	return nil
}

// tagOf は、model のアーム（models の、nil でないもの）の印（docs/plugin-http.md「通信の形」の「union」）: すべてが持つ、同じ名前の、
// リテラルの型のフィールドで、JSON の値が、アームごとに違うもの。最初の model の、フィールドの順で、最初のもの。なければ ""。
func tagOf(models []*ir.Model) string {
	var arms []*ir.Model
	for _, m := range models {
		if m != nil {
			arms = append(arms, m)
		}
	}
	if len(arms) == 0 {
		return ""
	}
	for _, f := range arms[0].Fields {
		values := make([]ir.Literal, len(arms))
		distinct := true
		for i, m := range arms {
			if values[i] = literalField(m, f.Name); values[i] == nil {
				distinct = false
				break
			}
			for j := 0; j < i; j++ {
				if sameJSON(values[j], values[i]) {
					distinct = false
				}
			}
		}
		if distinct {
			return f.Name
		}
	}
	return ""
}
