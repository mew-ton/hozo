package gocli

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
	"github.com/mew-ton/hozo/plugin/govalid"
)

// 診断のコード（docs/plugin-go-cli.md「診断」）。
const (
	codeCLIParams    = "CLI-1" // @cli.command の op の引数が、0 個でも model 一つでもない
	codeCLIDecorator = "CLI-2" // 入力の model のフィールドに cli のデコレータがない、または @cli.arg と @cli.flag の両方がある
	codeCLIFieldType = "CLI-3" // フィールドの型が受け付けられない
	codeCLIArrayArg  = "CLI-4" // array の @cli.arg が最後でない、または二つ以上
	codeCLIDuplicate = "CLI-5" // フラグ名かコマンド名の重複
	codeCLIExport    = "CLI-6" // @cli.command の付いた op に export がない
	codeCLIDir       = "CLI-7" // ディレクトリ cli が、同じ単位が出力する子の名前空間 N.cli の出力先と重なる
	codeCLIOption    = "CLI-8" // [emit.options.go-cli] に、知らないキーがある
	codeCLIValidate  = "CLI-9" // Validate を持つ入力の model の置き場の単位が、go-valid を並べていない
)

// このトランスパイラが出すただ一つのファイルの置き場（docs/plugin-go.md「相乗りのファイル」）: Dir(U, N) の下のディレクトリ DirName
// （パッケージの名前でもある）の FileName。OutputPath は、Dir(U, N) からの相対パス。
const (
	DirName    = "cli"
	FileName   = "cli.go"
	OutputPath = DirName + "/" + FileName
)

// 読む語彙（このプラグインの cli.hozo。docs/plugin-go-cli.md「マニフェスト」）。
const (
	cliCommand  = "cli.command"
	cliArg      = "cli.arg"
	cliFlag     = "cli.flag"
	cliEnv      = "cli.env"
	cliExitCode = "cli.exitCode"
)

// defaultExitCode は、@cli.exitCode のない例外と、見分けられなかった誤りの終了コード（docs/plugin-go-cli.md「実行と表示」）。
const defaultExitCode = 70

// Output は、一つの出力 (U, N) についての応答の中身。Errors が一つでもあれば、Files も Warnings も空。
type Output struct {
	Files    []plugin.File
	Warnings []plugin.Warning
	Errors   []plugin.Diagnostic
}

// Emit は、要求の View の出力 (U, N) の cli/cli.go を作る（docs/plugin-go-cli.md「出力」）。root はプロジェクトのルート
// （go.mod を探す起点）。診断があれば、ファイルは返さない。@cli.command の op が一つもなければ、ファイルを出さず warning を一つ返す。
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
	g := &generator{view: view, root: root, p: view.Program}
	g.prepare()

	// CLI-8: オプションはない
	var none struct{}
	unknown, err := plugin.Options(req, &none)
	if err != nil {
		return nil, fmt.Errorf("[emit.options.%s] を読めない: %w", TranspilerName, err)
	}
	for _, key := range unknown {
		g.errorf(codeCLIOption, "", "%d 番目の [[emit]] の [emit.options.%s] に、知らないキー %s がある（%s にオプションはない）", view.Config.Index, TranspilerName, key, TranspilerName)
	}

	targets := plugin.OpsWith(view, cliCommand)
	commands := g.check(targets)
	if len(g.errs) > 0 {
		return &Output{Errors: g.errs}, nil
	}
	if len(targets) == 0 {
		return &Output{Warnings: []plugin.Warning{{At: view.Viewer,
			Message: fmt.Sprintf("名前空間 %s の単位の対象に @%s の付いた op がないので、%s を出さない", view.Viewer, cliCommand, OutputPath)}}}, nil
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
	content, err := g.render(commands)
	if err != nil {
		return nil, err
	}
	return &Output{Files: []plugin.File{{Path: path.Join(layout.Dir, OutputPath), Content: content}}}, nil
}

// generator は一つの出力 (U, N) の cli.go を作る状態。
type generator struct {
	view   *ir.View
	root   string
	p      *ir.Program
	layout *golang.Layout

	declNS map[string]string // 宣言の FQN → 宣言した名前空間
	models map[string]*model // 確かめた入力の model（同じ model を使う op が複数でも、一度だけ確かめる）

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
	g.declNS, g.models = map[string]string{}, map[string]*model{}
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
}

// kind は、フィールドの値の、元の核の型（docs/plugin-go-cli.md「対象」: string / boolean / int / float）。
type kind int

const (
	kindString kind = iota
	kindBoolean
	kindInt
	kindFloat
)

// label は、使い方と誤りのメッセージに出す型の名前。
func (k kind) label() string {
	switch k {
	case kindBoolean:
		return "boolean"
	case kindInt:
		return "int"
	case kindFloat:
		return "float"
	}
	return "string"
}

// command は、サブコマンド一つ: @cli.command の付いた op。
type command struct {
	name    string // @cli.command の name
	op      *ir.Op
	method  string // Ops のメソッドの名前
	summary string // Op.description.summary を一行にしたもの
	input   *model // 入力の model。引数のない op なら nil
	// void は、結果の値がないか（returns が null か never）。
	void       bool
	exceptions []exception
}

// model は、入力の model。
type model struct {
	decl *ir.Model
	// hasDefault は、既定値を持つフィールドがあるか（初めの値は、基底の New<Model>()）。
	hasDefault bool
	// validate は、解釈した入力の Validate を呼ぶか（model が Validate を持つ。govalid.HasValidate）。
	validate bool
	fields   []*field // フィールドの順（診断を返したフィールドを除く）
	args     []*field // @cli.arg のフィールド（宣言順）
}

// field は、入力の model のフィールド一つの読み方（docs/plugin-go-cli.md「引数の解釈」）。
type field struct {
	name     string // 仕様の名前
	summary  string
	flag     string // @cli.flag の名前。なければ ""
	env      string // @cli.env の名前。なければ ""
	arg      bool   // @cli.arg
	kind     kind
	array    bool    // string（か string の scalar）の array。@cli.arg だけ
	optional bool    // 型が省略可能
	required bool    // 省略可能でなく、既定値もなく、array でもない
	def      string  // 既定値の表示。なければ ""
	value    ir.Type // 値の型（省略可能と array を外したもの。核の型か scalar）
}

// exception は、op が投げうる例外一つ（Signature.exceptions の順）。
type exception struct {
	decl     *ir.Exception
	exitCode int64
}

// check は、対象の op（@cli.command の付いた、単位の対象の op。宣言順）を見て、診断（CLI-1〜CLI-7、CLI-9）を集め、
// サブコマンドを返す（docs/plugin-go-cli.md「対象」）。
func (g *generator) check(targets []string) []command {
	var commands []command
	taken := map[string]string{} // コマンド名 → 先に使った op
	for _, fqn := range targets {
		op, _ := g.p.Lookup(fqn).(*ir.Op)
		if op == nil {
			continue
		}
		app := application(op.Decorators, cliCommand)
		if app == nil {
			continue
		}
		c := command{op: op, method: golang.MethodName(op.Name), void: op.Returns == nil || ir.IsNever(op.Returns)}
		c.name, _ = stringArg(app, "name")
		if op.Description != nil {
			c.summary = oneLine(op.Description.Summary)
		}
		// CLI-6
		if !op.Exported {
			g.errorf(codeCLIExport, op.Name, "op %s に @%s が付いているが、export がない（Main は、入口の Ops のメソッドとして op を呼ぶ。サブコマンドを黙って飛ばすことはしない）",
				op.Name, cliCommand)
		}
		// CLI-5: コマンド名
		if prev, dup := taken[c.name]; dup {
			g.errorf(codeCLIDuplicate, op.Name, "コマンド名 %q が重複している: op %s と op %s", c.name, prev, op.Name)
		} else {
			taken[c.name] = op.Name
		}
		// CLI-1
		var decl *ir.Model
		switch len(op.Params) {
		case 0:
		case 1:
			if r, isRef := op.Params[0].Type.(ir.RefType); isRef {
				decl, _ = g.p.Lookup(r.Target).(*ir.Model)
			}
			if decl == nil {
				g.errorf(codeCLIParams, op.Name, "@%s の op %s の引数 %s の型 %s が model でない（引数は、0 個か、入力の model 一つ）",
					cliCommand, op.Name, op.Params[0].Name, golang.TypeDisplay(op.Params[0].Type))
			}
		default:
			g.errorf(codeCLIParams, op.Name, "@%s の op %s の引数が %d 個ある（引数は、0 個か、入力の model 一つ）", cliCommand, op.Name, len(op.Params))
		}
		if decl != nil {
			c.input = g.model(decl)
			// CLI-9
			if c.input.validate {
				g.checkValidateHome(op, decl)
			}
		}
		for _, x := range op.Signature.Exceptions {
			ex, ok := g.p.Lookup(x).(*ir.Exception)
			if !ok {
				continue
			}
			e := exception{decl: ex, exitCode: defaultExitCode}
			if app := application(ex.Decorators, cliExitCode); app != nil {
				if code, ok := intArg(app, "code"); ok {
					e.exitCode = code
				}
			}
			c.exceptions = append(c.exceptions, e)
		}
		commands = append(commands, c)
	}
	if len(targets) > 0 {
		g.checkDir()
	}
	return commands
}

// checkDir は CLI-7: このトランスパイラのディレクトリ cli が、同じ単位が出力する子の名前空間 N.cli の出力先と重なる
// （Dir(U, N.cli) は `<Dir(U, N)>/cli`）。位置なし。cli.go を出すとき（@cli.command の op があるとき）だけ確かめる。
func (g *generator) checkDir() {
	child := g.view.Viewer + "." + DirName
	for _, c := range g.view.Namespaces[child] {
		if c.Index != g.view.Config.Index {
			continue
		}
		g.errorf(codeCLIDir, "", "名前空間 %s の %s を置くディレクトリ %s が、同じ単位（%d 番目の [[emit]]）が出力する子の名前空間 %s の出力先と重なる",
			g.view.Viewer, FileName, path.Join(g.view.Config.Dir, DirName), c.Index, child)
		return
	}
}

// checkValidateHome は CLI-9: 入力の model m（Validate を持つ）が、別の名前空間 M のものなら、M の型の置き場（docs/config.md
// 「型の置き場」）の単位が、transpilers に go-valid を並べていなければならない（Validate は、型の定義のパッケージにしか書けない。
// 並べていなければ、cli.go の in.Validate() がコンパイルできない）。N の model は、自分の単位が go-valid を並べている（CFG-19）。
// 置き場は、View だけから求まる（govalid.Unlisted）。
func (g *generator) checkValidateHome(op *ir.Op, m *ir.Model) {
	ns := g.declNS[m.Name]
	if ns == g.view.Viewer {
		return
	}
	if unit, unlisted := govalid.Unlisted(g.view, ns); unlisted {
		g.errorf(codeCLIValidate, op.Name, "@%s の op %s の入力 %s は Validate を持つが、名前空間 %s の型の置き場の単位（%d 番目の [[emit]]）が、transpilers に %s を並べていない（Main は、解釈した入力の Validate を呼ぶ。メソッドは、型の定義のパッケージにしか書けない）",
			cliCommand, op.Name, m.Name, ns, unit, govalid.Transpiler)
	}
}

// model は、入力の model decl のフィールドの読み方。CLI-2〜CLI-5 を報告する（位置は model）。同じ model は、一度だけ確かめる。
func (g *generator) model(decl *ir.Model) *model {
	if m, ok := g.models[decl.Name]; ok {
		return m
	}
	m := &model{decl: decl, validate: govalid.HasValidate(g.p, decl.Name)}
	g.models[decl.Name] = m
	flags := map[string]string{} // フラグ名 → 先に使ったフィールド
	for i := range decl.Fields {
		fd := &decl.Fields[i]
		f := &field{name: fd.Name, arg: application(fd.Decorators, cliArg) != nil}
		if fd.Description != nil {
			f.summary = oneLine(fd.Description.Summary)
		}
		// IR にフィールドの既定値はなくなった（版 0.5。docs/ir.md「版」）ので、既定値の表示（f.def、m.hasDefault）は付かない
		hasFlag, hasEnv := false, false
		if app := application(fd.Decorators, cliFlag); app != nil {
			hasFlag = true
			f.flag, _ = stringArg(app, "name")
		}
		if app := application(fd.Decorators, cliEnv); app != nil {
			hasEnv = true
			f.env, _ = stringArg(app, "name")
		}
		// CLI-2
		switch {
		case !f.arg && !hasFlag && !hasEnv:
			g.errorf(codeCLIDecorator, decl.Name, "入力の model %s のフィールド %s が、@%s、@%s、@%s のどれも持たない", decl.Name, fd.Name, cliArg, cliFlag, cliEnv)
			continue
		case f.arg && hasFlag:
			g.errorf(codeCLIDecorator, decl.Name, "入力の model %s のフィールド %s が、@%s と @%s の両方を持つ（位置引数か、フラグか、どちらか一つ）", decl.Name, fd.Name, cliArg, cliFlag)
			continue
		}
		// CLI-5: フラグ名
		if hasFlag {
			if prev, dup := flags[f.flag]; dup {
				g.errorf(codeCLIDuplicate, decl.Name, "入力の model %s のフラグ名 --%s が重複している: フィールド %s と %s", decl.Name, f.flag, prev, fd.Name)
			} else {
				flags[f.flag] = fd.Name
			}
		}
		// CLI-3
		if !g.fieldType(decl, fd, f) {
			continue
		}
		f.required = !f.optional && !f.array
		m.fields = append(m.fields, f)
		if f.arg {
			m.args = append(m.args, f)
		}
	}
	// CLI-4
	for i, f := range m.args {
		if f.array && i != len(m.args)-1 {
			g.errorf(codeCLIArrayArg, decl.Name, "入力の model %s の array の @%s のフィールド %s が、@%s のフィールドの最後でない（array は、残りの位置引数をすべて受け取るので、最後の一つだけ）",
				decl.Name, cliArg, f.name, cliArg)
		}
	}
	return m
}

// accepted は、受け付ける型の一覧（CLI-3 のメッセージ用）。
const accepted = "string / boolean / int / float、元がこれらの scalar、その省略可能、@cli.arg に限り string の array"

// fieldType は、フィールド fd の型を確かめて f に写す（docs/plugin-go-cli.md「対象」）。受け付けなければ CLI-3 を報告して false。
func (g *generator) fieldType(decl *ir.Model, fd *ir.Field, f *field) bool {
	reject := func(reason string) bool {
		g.errorf(codeCLIFieldType, decl.Name, "入力の model %s のフィールド %s の型 %s は受け付けられない: %s（受け付けるのは、%s）",
			decl.Name, fd.Name, golang.TypeDisplay(fd.Type), reason, accepted)
		return false
	}
	t := fd.Type
	if o, ok := t.(ir.OptionalType); ok {
		f.optional, t = true, o.Of
		if _, isArray := t.(ir.ArrayType); isArray {
			return reject("array に省略可能は付けられない（0 個を表せる）")
		}
	}
	if a, ok := t.(ir.ArrayType); ok {
		if !f.arg {
			return reject("array は @" + cliArg + " にだけ付けられる")
		}
		k, isCore := g.kindOf(a.Of)
		if !isCore || k != kindString {
			return reject("array の要素は、string か、string の scalar")
		}
		f.array, f.kind, f.value = true, k, a.Of
		return true
	}
	k, isCore := g.kindOf(t)
	if !isCore {
		return reject("値を、コマンドラインの一つの文字列から読めない")
	}
	if k == kindBoolean && f.arg {
		return reject("@" + cliArg + " に boolean は付けられない")
	}
	f.kind, f.value = k, t
	return true
}

// kindOf は、型 t の元の核の型の種類（scalar は、その継承を辿る）。string / boolean / int / float を元に持たなければ false。
func (g *generator) kindOf(t ir.Type) (kind, bool) {
	for depth := 0; depth < 64; depth++ {
		switch x := t.(type) {
		case ir.CoreType:
			switch x.Name {
			case ir.CoreString:
				return kindString, true
			case ir.CoreBoolean:
				return kindBoolean, true
			case ir.CoreInt:
				return kindInt, true
			case ir.CoreFloat:
				return kindFloat, true
			}
			return 0, false
		case ir.RefType:
			s, ok := g.p.Lookup(x.Target).(*ir.Scalar)
			if !ok {
				return 0, false
			}
			t = s.Base
		default:
			return 0, false
		}
	}
	return 0, false
}

// application は、apps のうち、デコレータ decorator（FQN）の適用。なければ nil。名前付きデコレータは展開済み。
func application(apps []ir.Application, decorator string) *ir.Application {
	for i := range apps {
		if apps[i].Decorator == decorator {
			return &apps[i]
		}
	}
	return nil
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

// intArg は、適用 app の引数 param の整数のリテラル。
func intArg(app *ir.Application, param string) (int64, bool) {
	for _, a := range app.Args {
		if a.Param == param {
			if l, ok := a.Value.(ir.LiteralValue); ok {
				if n, ok := l.Literal.(ir.IntLiteral); ok {
					return n.Value, true
				}
			}
		}
	}
	return 0, false
}

// literalString は、既定値の、使い方での表示。
func literalString(l ir.Literal) string {
	switch x := l.(type) {
	case ir.StringLiteral:
		return x.Value
	case ir.IntLiteral:
		return fmt.Sprint(x.Value)
	case ir.FloatLiteral:
		return fmt.Sprint(x.Value)
	case ir.BooleanLiteral:
		return fmt.Sprint(x.Value)
	case ir.DurationLiteral:
		return x.Value.String()
	}
	return ""
}

// oneLine は、summary を使い方の一行にする: 改行と連続する空白を一つの空白にし、末尾の句点を落とす
// （docs/plugin-go-cli.md「使い方の表示」の例は「仕様を検査する。ファイルは書き出さない」と、末尾に句点がない）。
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSuffix(s, "。")
	return strings.TrimSuffix(s, ".")
}
