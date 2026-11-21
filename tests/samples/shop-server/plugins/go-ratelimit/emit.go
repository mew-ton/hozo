package main

import (
	"errors"
	"fmt"
	"go/format"
	"os"
	"path"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは go-ratelimit の中身。規則の持ち主は ../../README.md「サンプルの中のプラグイン」の「出力の規則」。
// 基底の形（置き場、Seams のメソッドの名前と形、零値、型の置き場）は、公開の SDK plugin/golang で求める。基底の応答は見ない
// （docs/plugin-protocol.md「相乗りが基底の形を知る方法」）。

// TranspilerName は、このプラグインが提供するただ一つのトランスパイラの名前（マニフェストの [[transpiler]] の name）。
const TranspilerName = "go-ratelimit"

// FileName は、このトランスパイラが、入口のパッケージに置くファイルの名前（docs/plugin-go.md「相乗りのファイル」）。
const FileName = "ratelimit.go"

// 読む語彙（rate.hozo）: 継ぎ目を宣言したデコレータと、制限を超えたときの例外の FQN。
const (
	decoratorLimited = "rate.limited"
	exceptionLimit   = "rate.LimitExceeded"
)

// 診断のコード（../../README.md「サンプルの中のプラグイン」の「出力の規則」）。
const (
	codeRateOption    = "GORATE-1" // [emit.options.go-ratelimit] に知らないキーがある
	codeRateCollision = "GORATE-2" // 公開の名前が当たる
)

// Handler は go-ratelimit の plugin.Handler。emit だけを実装する。
type Handler struct {
	// Getenv は環境変数を引く関数。nil なら os.Getenv。HOZO_PROJECT_ROOT（docs/plugin-protocol.md「起動」）を、型のパッケージの
	// import パスを求めるルートに使う。空なら作業ディレクトリ（= プロジェクトのルート）。
	Getenv func(string) string
}

var _ plugin.Handler = Handler{}

// Emit は View.viewer の名前空間の ratelimit.go を作る。診断（GORATE-*）があれば Files は空で、Errors にそれを入れる。
// 要求の transpiler が [TranspilerName] でなければ plugin.ErrUnsupported（code "unsupported"）。
func (h Handler) Emit(req *plugin.EmitRequest) (*plugin.EmitResponse, error) {
	if req == nil || req.View == nil {
		return nil, fmt.Errorf("emit: request has no view")
	}
	if req.Transpiler != TranspilerName {
		return nil, fmt.Errorf("transpiler %q is not provided by go-ratelimit (it provides %q): %w", req.Transpiler, TranspilerName, plugin.ErrUnsupported)
	}
	getenv := h.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	root := getenv("HOZO_PROJECT_ROOT")
	if root == "" {
		root = "."
	}
	return Emit(req, root)
}

// Check は実装しない（語彙に固有の検査はない）。
func (Handler) Check(*plugin.CheckRequest) (*plugin.CheckResponse, error) {
	return nil, plugin.ErrUnsupported
}

// Import は実装しない。
func (Handler) Import(*plugin.ImportRequest) (*plugin.ImportResponse, error) {
	return nil, plugin.ErrUnsupported
}

// Coverage は実装しない（カバレッジを読むのは、基底）。
func (Handler) Coverage(*plugin.CoverageRequest) (*plugin.CoverageResponse, error) {
	return nil, plugin.ErrUnsupported
}

// Emit は、要求の View の出力 (U, N) の ratelimit.go を作る。root はプロジェクトのルート（go.mod を探す起点）。
// 対象（自分が宣言した継ぎ目）が一つもなければ、ファイルを出さない。診断があれば、ファイルは返さない。誤り（error）を返すのは、
// 利用者の仕様の誤りではないとき（View が欠けている、生成したコードを整形できない、型の置き場がない）。
func Emit(req *plugin.EmitRequest, root string) (*plugin.EmitResponse, error) {
	view := req.View
	if view == nil || view.Program == nil || view.Program.Namespace(view.Viewer) == nil {
		viewer := ""
		if view != nil {
			viewer = view.Viewer
		}
		return nil, fmt.Errorf("emit: View に、名前空間 %q の Program がない", viewer)
	}
	e := &emitter{view: view, root: root, shape: golang.ShapeOf(view)}

	// GORATE-1: オプションはない
	var none struct{}
	unknown, err := plugin.Options(req, &none)
	if err != nil {
		e.errorf(codeRateOption, "", "[emit.options.go-ratelimit] を読めない: %v", err)
	}
	for _, key := range unknown {
		e.errorf(codeRateOption, "", "%d 番目の [[emit]] の [emit.options.go-ratelimit] に、知らないキー %s がある（go-ratelimit にオプションはない）", view.Config.Index, key)
	}

	// 対象: 基底の Seams のメソッドのうち、自分が宣言した継ぎ目のもの（Seams の並びのまま）
	for _, s := range e.shape.Seams {
		if s.Transpiler == TranspilerName && s.Decorator == decoratorLimited {
			e.mine = append(e.mine, s)
		}
	}
	if len(e.mine) > 0 {
		e.checkNames()
	}
	if len(e.errs) > 0 {
		return &plugin.EmitResponse{Errors: e.errs}, nil
	}
	if len(e.mine) == 0 {
		return &plugin.EmitResponse{}, nil
	}

	layout, err := golang.LayoutOf(view, root)
	if err != nil {
		if errors.Is(err, golang.ErrNoGoMod) {
			// Dir(U, N) の上に go.mod がない。報告するのは基底（GO-1。docs/plugin-go.md「診断」）で、ここは何も出さない
			return &plugin.EmitResponse{}, nil
		}
		return nil, fmt.Errorf("emit: 名前空間 %s の出力先 %q の import パスを求められない: %w", view.Viewer, view.Config.Dir, err)
	}
	e.layout = layout
	content, err := e.generate()
	if err != nil {
		return nil, err
	}
	return &plugin.EmitResponse{Files: []plugin.File{{Path: path.Join(layout.Dir, FileName), Content: content}}}, nil
}

// emitter は一つの出力 (U, N) の ratelimit.go を作る状態。
type emitter struct {
	view   *ir.View
	root   string
	layout *golang.Layout
	shape  *golang.Shape
	mine   []golang.Seam // 対象の継ぎ目（基底の Seams の並びのまま）
	errs   []plugin.Diagnostic
}

func (e *emitter) errorf(code, at, format string, args ...any) {
	e.errs = append(e.errs, plugin.Diagnostic{Code: code, At: at, Message: fmt.Sprintf(format, args...)})
}

// generate は ratelimit.go の内容を作る。import の名前と、局所の名前が避ける名前を決めるために、二度作る（基底と同じ。
// docs/plugin-go.md「パッケージと import パス」の「import の名前」）。go/format で整形する。
func (e *emitter) generate() (string, error) {
	im := golang.NewImports(e.view, e.layout, e.root, false)
	e.build(im)
	im = im.Canonical()
	body := e.build(im)
	if err := im.Err(); err != nil {
		return "", fmt.Errorf("emit: %s: %w", path.Join(e.layout.Dir, FileName), err)
	}
	var b strings.Builder
	b.WriteString("// Code generated by hozoc go-ratelimit " + plugin.Version + "; DO NOT EDIT.\n\n")
	b.WriteString("package " + e.layout.EntryName + "\n\n")
	b.WriteString(im.Render())
	b.WriteString("\n" + body)
	src, err := format.Source([]byte(b.String()))
	if err != nil {
		return "", fmt.Errorf("emit: 生成した %s を整形できない（生成器の不具合）: %w\n---- 整形前 ----\n%s", path.Join(e.layout.Dir, FileName), err, b.String())
	}
	return string(src), nil
}

// build は、ratelimit.go の本体（import より後ろ）を一通り作る。
func (e *emitter) build(im *golang.Imports) string {
	ctx := im.Add("context", "context")
	// このファイルが使う名前: レシーバの s、ctx、import するパッケージの名前。引数の名前がこれと当たれば、末尾に _（基底と同じ規則）
	reserved := map[string]bool{"s": true, "ctx": true}
	for _, n := range im.Names() {
		reserved[n] = true
	}
	// New の引数 seams の名前（基底が、先に並んだ引数と当たらないように決めたもの。Shape.New の最後）
	seams := "seams"
	if n := len(e.shape.New); n > 0 {
		seams = e.shape.New[n-1].Name
	}

	var b strings.Builder
	printf := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	printf("// RateLimiter は、@rate.limited の付いた op を、今呼んでよいかを決める。人が実装し、NewRateSeams に渡す。\n")
	printf("type RateLimiter interface {\n")
	printf("\t// Allow は、op（FQN）を今呼んでよければ true。呼び出しを一回と数えるのも、この中。\n")
	printf("\tAllow(ctx %s.Context, op string) bool\n}\n\n", ctx)
	printf("// RateSeams は、@rate.limited の継ぎ目（Seams のメソッドのうち、このデコレータの分）の実装。New の引数 %s に渡す。\n", seams)
	printf("type RateSeams struct{ limiter RateLimiter }\n\n")
	printf("// NewRateSeams は、limiter に聞いて制限する RateSeams を返す。\n")
	printf("func NewRateSeams(limiter RateLimiter) RateSeams { return RateSeams{limiter: limiter} }\n\n")
	if len(e.mine) == len(e.shape.Seams) {
		// 基底の Seams のメソッドが、すべて自分の対象のときだけ（そうでなければ、RateSeams だけでは Seams を満たさない）
		printf("var _ Seams = RateSeams{}\n\n")
	}
	exception := golang.GoType(ir.Ref(exceptionLimit), im) + "{}"
	for _, s := range e.mine {
		op, ok := e.view.Program.Lookup(s.Op).(*ir.Op)
		if !ok {
			continue
		}
		params, results := golang.SeamSignature(op, im, reserved)
		args := []string{"ctx"}
		for _, p := range op.Params {
			args = append(args, golang.LocalName(p.Name, reserved))
		}
		denied := exception
		if zero := golang.Zero(op.Returns, im); zero != "" {
			denied = zero + ", " + exception
		}
		printf("// %s は op %s に付いた @%s の継ぎ目: limiter が許さなければ、中身を呼ばずに %s。許せば、中身をそのまま呼ぶ。\n",
			s.Method, s.Op, s.Decorator, exceptionLimit)
		printf("func (s RateSeams) %s(%s) %s {\n", s.Method, params, results)
		printf("\tif !s.limiter.Allow(ctx, %q) {\n\t\treturn %s\n\t}\n", s.Op, denied)
		printf("\treturn %s(%s)\n}\n\n", golang.SeamBodyParam(op, reserved), strings.Join(args, ", "))
	}
	return b.String()
}

// owned は、パッケージの名前一つと、それを生んだもの。
type owned struct {
	what string // メッセージに書く、生んだもの（「model shop.RateLimiter」）
	at   string // 診断の位置にする宣言の FQN（宣言が生んだ名前でなければ ""）
}

// baseNames は、基底 go が N の定義のパッケージに出す公開の名前（docs/plugin-go.md「名前」の「名前が当たれば GO-2」が並べるもの。
// 一覧は、基底と共有する golang.PublicNames）。入口のパッケージは、定義のすべての名前を再公開する。同じ名前が二つあれば、先のもの。
func (e *emitter) baseNames() map[string]owned {
	names := map[string]owned{}
	for _, n := range golang.PublicNames(e.view) {
		if _, dup := names[n.Name]; !dup {
			names[n.Name] = owned{what: n.What, at: n.Decl}
		}
	}
	return names
}

// checkNames は GORATE-2: このトランスパイラの公開の名前（RateLimiter、RateSeams、NewRateSeams）が、N の定義のパッケージの名前と
// 当たる。位置は、当たった宣言。
func (e *emitter) checkNames() {
	names := e.baseNames()
	for _, p := range []struct{ name, what string }{
		{"RateLimiter", "go-ratelimit のインターフェース RateLimiter"},
		{"RateSeams", "go-ratelimit の型 RateSeams"},
		{"NewRateSeams", "go-ratelimit の関数 NewRateSeams"},
	} {
		if prev, taken := names[p.name]; taken {
			e.errorf(codeRateCollision, prev.at, "入口のパッケージ（%s）の名前 %s が当たる: %s と %s（直すのは仕様の名前）", e.view.Config.Dir, p.name, prev.what, p.what)
		}
	}
}
