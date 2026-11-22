package gocli

import (
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
	"github.com/mew-ton/hozo/plugin/govalid"
)

// 診断のコード（docs/plugin-go-cli.md「診断」）。
const (
	codeCLIParams      = "CLI-1"  // @cli.command の op の引数が、0 個でも model 一つでもない
	codeCLISource      = "CLI-2"  // 入力の model のフィールドに値の出どころ（@cli.arg、@cli.flag、@cli.env）がない、または @cli.arg と @cli.flag の両方がある
	codeCLIFieldType   = "CLI-3"  // フィールドの型が受け付けられない
	codeCLIArrayArg    = "CLI-4"  // array の @cli.arg が、いちばん後ろの位置でない
	codeCLIDuplicate   = "CLI-5"  // フラグ名かコマンド名の重複
	codeCLIExport      = "CLI-6"  // @cli.command の付いた op に export がない
	codeCLIDir         = "CLI-7"  // ディレクトリ cli が、同じ単位が出力する子の名前空間 N.cli の出力先と重なる
	codeCLIOption      = "CLI-8"  // [emit.options.go-cli] に、知らないキーがある
	codeCLIValidate    = "CLI-9"  // Validate を持つ入力の model の置き場の単位が、go-valid を並べていない
	codeCLIPosition    = "CLI-10" // @cli.arg の位置が、1 から、重なりも抜けもなく続いていない
	codeCLIOptional    = "CLI-11" // @cli.required か @cli.default の付いたフィールドの型が、省略可能
	codeCLIBoth        = "CLI-12" // 一つのフィールドに、@cli.required と @cli.default の両方がある
	codeCLIDefaultType = "CLI-13" // @cli.default のリテラルが、フィールドの型に渡せない
	codeCLIDefault     = "CLI-14" // @cli.default の値が、フィールドの制約（@valid.*）に合わない
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
	cliRequired = "cli.required"
	cliDefault  = "cli.default"
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

// Emit は、要求の View の出力 (U, N) の cli/cli.go を作る（docs/plugin-go-cli.md「出力」）。読むのは、要求だけ
// （ファイルも、環境変数も、読まない）。診断があれば、ファイルは返さない。@cli.command の op が一つもなければ、ファイルを出さず warning を一つ返す。
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
	g := &generator{view: view, p: view.Program}
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
	// 置き場。基底のオプション module がなければ、基底が GO-8 を返すので、ここでは何も返さない
	layout, err := golang.LayoutOf(view)
	if err != nil {
		if errors.Is(err, golang.ErrNoModule) {
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

// command は、サブコマンド一つ: @cli.command の付いた op。
type command struct {
	name    string // @cli.command の name
	op      *ir.Op
	method  string // Ops のメソッドの名前
	summary string // Op.description.summary を一行にしたもの
	input   *model // 入力の model。引数のない op なら nil
	// void は、呼ぶ相手の、基底のメソッドの形: 結果の値がないか（returns が null か never。docs/plugin-go.md「op の形」）。
	void       bool
	exceptions []exception
}

// model は、入力の model。
type model struct {
	decl   *ir.Model
	fields []*field // フィールドの順
}

// field は、入力の model のフィールド一つについて、cli.go に書くもの: 名前と、説明と、cli のデコレータごとの一行。
// フィールドの型は、ここに持たない: 出すコードは、デコレータの引数だけから作る（docs/requirements.md「出力を推論しない」）。
// 型を見るのは、検査（checkField）だけ。
type field struct {
	name    string // 仕様の名前
	summary string
	lines   []line // デコレータを書いた順
}

// line は、cli のデコレータ一つが出す、run<Op> の一行（docs/plugin-go-cli.md「コマンドごとの関数」）: `line.<method>(…)`。
type line struct {
	method string // cliLine のメソッド: flag / env / arg / required / orDefault
	arg    string // デコレータの引数の、Go での書き方: 名前の文字列、位置、補う値のリテラル。required は ""
}

// 行のメソッド（生成する cliLine のメソッドの名前）。
const (
	lineFlag     = "flag"
	lineEnv      = "env"
	lineArg      = "arg"
	lineRequired = "required"
	lineDefault  = "orDefault"
)

// exception は、op が投げうる例外一つ（Signature.exceptions の順）。
type exception struct {
	decl     *ir.Exception
	exitCode int64
}

// check は、対象の op（@cli.command の付いた、単位の対象の op。宣言順）を見て、診断（CLI-1〜CLI-7、CLI-9〜CLI-14）を集め、
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
			g.checkValidateHome(op, decl)
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

// checkValidateHome は CLI-9: 入力の model m が Validate を持ち（govalid.HasValidate）、別の名前空間 M のものなら、M の型の置き場
// （docs/config.md「型の置き場」）の単位が、transpilers に go-valid を並べていなければならない。並べていなければ、その入力は、
// 確かめられないまま op に渡る（cli.go は、どの入力にも同じ補助を呼び、補助は、入力が Validate を持つかを実行時に見るので、
// コンパイルはできてしまう）。書いたものの整合の検査で、出力は変えない（docs/plugin-go-cli.md「引数の解釈」）。N の model は、
// 自分の単位が go-valid を並べている（CFG-19）。置き場は、View だけから求まる（govalid.Unlisted）。
func (g *generator) checkValidateHome(op *ir.Op, m *ir.Model) {
	ns := g.declNS[m.Name]
	if ns == g.view.Viewer || !govalid.HasValidate(g.p, m.Name) {
		return
	}
	if unit, unlisted := govalid.Unlisted(g.view, ns); unlisted {
		g.errorf(codeCLIValidate, op.Name, "@%s の op %s の入力 %s は制約を持つ（Validate を持つ型）が、名前空間 %s の型の置き場の単位（%d 番目の [[emit]]）が、transpilers に %s を並べていない（Validate のメソッドが出ないので、入力が、確かめられないまま op に渡る）",
			cliCommand, op.Name, m.Name, ns, unit, govalid.Transpiler)
	}
}

// model は、入力の model decl のフィールドごとの行。CLI-2〜CLI-5 のフラグ名、CLI-10〜CLI-14 を報告する（位置は model）。
// 同じ model は、一度だけ確かめる。
func (g *generator) model(decl *ir.Model) *model {
	if m, ok := g.models[decl.Name]; ok {
		return m
	}
	m := &model{decl: decl}
	g.models[decl.Name] = m
	flags := map[string]string{} // フラグ名 → 先に使ったフィールド
	var args []placed            // @cli.arg のフィールド（フィールドの順）
	for i := range decl.Fields {
		fd := &decl.Fields[i]
		f := &field{name: fd.Name}
		if fd.Description != nil {
			f.summary = oneLine(fd.Description.Summary)
		}
		// 行: フィールドに付いた cli のデコレータごとに一つ、書いた順
		var arg, flag, env, required, def *ir.Application
		for j := range fd.Decorators {
			app := &fd.Decorators[j]
			switch app.Decorator {
			case cliFlag:
				flag = app
				f.lines = append(f.lines, line{lineFlag, strconv.Quote(nameArg(app, fd.Name))})
			case cliEnv:
				env = app
				f.lines = append(f.lines, line{lineEnv, strconv.Quote(nameArg(app, fd.Name))})
			case cliArg:
				arg = app
				position, _ := intArg(app, "position")
				f.lines = append(f.lines, line{lineArg, strconv.FormatInt(position, 10)})
			case cliRequired:
				required = app
				f.lines = append(f.lines, line{lineRequired, ""})
			case cliDefault:
				def = app
				f.lines = append(f.lines, line{lineDefault, goLiteral(literalArg(app, "value"))})
			}
		}
		m.fields = append(m.fields, f)
		// CLI-2
		if arg == nil && flag == nil && env == nil {
			also := ""
			if required != nil || def != nil {
				also = fmt.Sprintf("（@%s と @%s は、値の出どころではない）", cliRequired, cliDefault)
			}
			g.errorf(codeCLISource, decl.Name, "入力の model %s のフィールド %s が、値の出どころ（@%s、@%s、@%s）を一つも持たない%s", decl.Name, fd.Name, cliArg, cliFlag, cliEnv, also)
			continue
		}
		if arg != nil && flag != nil {
			g.errorf(codeCLISource, decl.Name, "入力の model %s のフィールド %s が、@%s と @%s の両方を持つ（位置引数か、フラグか、どちらか一つ）", decl.Name, fd.Name, cliArg, cliFlag)
		}
		// CLI-5: フラグ名
		if flag != nil {
			name := nameArg(flag, fd.Name)
			if prev, dup := flags[name]; dup {
				g.errorf(codeCLIDuplicate, decl.Name, "入力の model %s のフラグ名 --%s が重複している: フィールド %s と %s", decl.Name, name, prev, fd.Name)
			} else {
				flags[name] = fd.Name
			}
		}
		// CLI-12
		if required != nil && def != nil {
			g.errorf(codeCLIBoth, decl.Name, "入力の model %s のフィールド %s が、@%s と @%s の両方を持つ（値が与えられなかったときの扱いは、どちらか一つ）", decl.Name, fd.Name, cliRequired, cliDefault)
		}
		// CLI-3、CLI-11、CLI-13、CLI-14: 型についての決まり。書いたものの整合の検査で、出力（f.lines）は変えない
		shape, accepted := g.checkType(decl, fd, arg != nil)
		if arg != nil {
			position, _ := intArg(arg, "position")
			args = append(args, placed{field: fd.Name, position: position, array: accepted && shape.array})
		}
		if !accepted {
			continue
		}
		if shape.optional {
			for _, app := range []*ir.Application{required, def} {
				if app != nil {
					g.errorf(codeCLIOptional, decl.Name, "入力の model %s のフィールド %s の型 %s は省略可能なのに、@%s が付いている（型は「なくてよい」と言っている。なければ誤りにするか、値を入れるなら、省略可能にしない）",
						decl.Name, fd.Name, golang.TypeDisplay(fd.Type), app.Decorator)
				}
			}
		}
		if def != nil {
			g.checkDefault(decl, fd, def, shape)
		}
	}
	g.checkPositions(decl, args)
	return m
}

// placed は、@cli.arg の付いたフィールドと、その位置。array は、受け付けた型が array か（残りの位置引数をすべて受け取る）。
type placed struct {
	field    string
	position int64
	array    bool
}

// checkPositions は CLI-10 と CLI-4: @cli.arg の位置は、1 から、重なりも抜けもなく続く。array の @cli.arg は、いちばん後ろの
// 位置の一つだけ（docs/plugin-go-cli.md「対象」）。
func (g *generator) checkPositions(decl *ir.Model, args []placed) {
	taken := map[int64]string{} // 位置 → 先に使ったフィールド
	var last int64
	for _, a := range args {
		switch prev, dup := taken[a.position]; {
		case a.position < 1:
			g.errorf(codeCLIPosition, decl.Name, "入力の model %s のフィールド %s の @%s の位置 %d が、1 より小さい（位置は、1 から数える）", decl.Name, a.field, cliArg, a.position)
			continue
		case dup:
			g.errorf(codeCLIPosition, decl.Name, "入力の model %s の @%s の位置 %d が重なっている: フィールド %s と %s", decl.Name, cliArg, a.position, prev, a.field)
			continue
		}
		taken[a.position] = a.field
		if a.position > last {
			last = a.position
		}
	}
	var missing []string
	for n := int64(1); n < last && len(missing) < 8; n++ { // 抜けは、初めの 8 個まで示す（大きな位置を一つ書いただけで、長いメッセージにしない）
		if _, ok := taken[n]; !ok {
			missing = append(missing, strconv.FormatInt(n, 10))
		}
	}
	if len(missing) > 0 {
		g.errorf(codeCLIPosition, decl.Name, "入力の model %s の @%s の位置が、1 から %d まで続いていない: 位置 %s がない", decl.Name, cliArg, last, strings.Join(missing, "、"))
	}
	// CLI-4
	for _, a := range args {
		if a.array && a.position >= 1 && a.position != last {
			g.errorf(codeCLIArrayArg, decl.Name, "入力の model %s の array の @%s(%d) のフィールド %s が、いちばん後ろの位置（%d）でない（array は、残りの位置引数をすべて受け取るので、いちばん後ろの一つだけ）",
				decl.Name, cliArg, a.position, a.field, last)
		}
	}
}

// kind は、フィールドの値の、元の核の型（docs/plugin-go-cli.md「対象」: string / boolean / int / float）。検査にだけ使う。
type kind int

const (
	kindString kind = iota
	kindBoolean
	kindInt
	kindFloat
)

// shape は、受け付けたフィールドの型の形。検査（CLI-4、CLI-11、CLI-13）にだけ使い、出力には渡さない。
type shape struct {
	kind     kind // 値（array なら要素）の、元の核の型
	array    bool // string（か string の scalar）の array。@cli.arg だけ
	optional bool // 型が省略可能
}

// acceptedTypes は、受け付ける型の一覧（CLI-3 のメッセージ用）。
const acceptedTypes = "string / boolean / int / float、元がこれらの scalar、その省略可能、@cli.arg に限り string の array"

// checkType は、フィールド fd の型を確かめる（docs/plugin-go-cli.md「対象」）。arg は、@cli.arg が付いているか。受け付けなければ
// CLI-3 を報告して false。
func (g *generator) checkType(decl *ir.Model, fd *ir.Field, arg bool) (shape, bool) {
	reject := func(reason string) (shape, bool) {
		g.errorf(codeCLIFieldType, decl.Name, "入力の model %s のフィールド %s の型 %s は受け付けられない: %s（受け付けるのは、%s）",
			decl.Name, fd.Name, golang.TypeDisplay(fd.Type), reason, acceptedTypes)
		return shape{}, false
	}
	var s shape
	t := fd.Type
	if o, ok := t.(ir.OptionalType); ok {
		s.optional, t = true, o.Of
		if _, isArray := t.(ir.ArrayType); isArray {
			return reject("array に省略可能は付けられない（0 個を表せる）")
		}
	}
	if a, ok := t.(ir.ArrayType); ok {
		if !arg {
			return reject("array は @" + cliArg + " にだけ付けられる")
		}
		k, isCore := g.kindOf(a.Of)
		if !isCore || k != kindString {
			return reject("array の要素は、string か、string の scalar")
		}
		s.array, s.kind = true, k
		return s, true
	}
	k, isCore := g.kindOf(t)
	if !isCore {
		return reject("値を、コマンドラインの一つの文字列から読めない")
	}
	if k == kindBoolean && arg {
		return reject("@" + cliArg + " に boolean は付けられない")
	}
	s.kind = k
	return s, true
}

// checkDefault は、@cli.default（適用 def）の値を確かめる: リテラルが、フィールドの型（scalar は元の核の型）に渡せること
// （CLI-13。docs/types.md「互換」の 10。array のフィールドに渡せるリテラルはない）と、フィールドの制約（@valid.*）に合うこと
// （CLI-14。docs/plugin-go-cli.md「補う値と制約」）。補う値の検査は、ここだけ。
func (g *generator) checkDefault(decl *ir.Model, fd *ir.Field, def *ir.Application, s shape) {
	lit := literalArg(def, "value")
	if lit == nil {
		return
	}
	fits := false
	if !s.array {
		switch lit.(type) {
		case ir.StringLiteral:
			fits = s.kind == kindString
		case ir.IntLiteral:
			fits = s.kind == kindInt || s.kind == kindFloat
		case ir.FloatLiteral:
			fits = s.kind == kindFloat
		case ir.BooleanLiteral:
			fits = s.kind == kindBoolean
		}
	}
	if !fits {
		g.errorf(codeCLIDefaultType, decl.Name, "入力の model %s のフィールド %s の @%s のリテラル %s（%s）は、フィールドの型 %s に渡せない",
			decl.Name, fd.Name, cliDefault, goLiteral(lit), lit.Kind(), golang.TypeDisplay(fd.Type))
		return
	}
	for _, miss := range govalid.CheckLiteral(g.p, fd, lit) {
		where := "フィールドの制約"
		if miss.Scalar != "" {
			where = "型 " + miss.Scalar + " の制約"
		}
		g.errorf(codeCLIDefault, decl.Name, "入力の model %s のフィールド %s の @%s の値 %s が、%s %s に合わない（このフィールドを与えない呼び出しが、いつも誤りになる）",
			decl.Name, fd.Name, cliDefault, goLiteral(lit), where, miss.Text)
	}
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

// literalArg は、適用 app の引数 param のリテラル。なければ nil。
func literalArg(app *ir.Application, param string) ir.Literal {
	for _, a := range app.Args {
		if a.Param == param {
			if l, ok := a.Value.(ir.LiteralValue); ok {
				return l.Literal
			}
		}
	}
	return nil
}

// stringArg は、適用 app の引数 param の文字列のリテラル。
func stringArg(app *ir.Application, param string) (string, bool) {
	s, ok := literalArg(app, param).(ir.StringLiteral)
	return s.Value, ok
}

// intArg は、適用 app の引数 param の整数のリテラル。
func intArg(app *ir.Application, param string) (int64, bool) {
	n, ok := literalArg(app, param).(ir.IntLiteral)
	return n.Value, ok
}

// nameArg は、名前を取るデコレータ（@cli.flag、@cli.env）の適用 app の名前: 引数 name。省いていれば、フィールドの名前
// （docs/plugin-go-cli.md「語彙」）。
func nameArg(app *ir.Application, fieldName string) string {
	if name, ok := stringArg(app, "name"); ok {
		return name
	}
	return fieldName
}

// goLiteral は、リテラル l の Go での書き方。基底と同じ（docs/plugin-go.md「値」の表の literal の行）: 文字列は strconv.Quote、
// 整数は 10 進、小数は strconv.FormatFloat(v, 'g', -1, 64)（`.` も `e` も含まなければ `.0` を足す）、真偽値は true / false。
// フィールドの型への変換は書かない（補助が、実行時に、読む先の型に変える）。
func goLiteral(l ir.Literal) string {
	switch x := l.(type) {
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
	}
	return "nil"
}

// oneLine は、summary を使い方の一行にする: 改行と連続する空白を一つの空白にし、末尾の句点を落とす
// （docs/plugin-go-cli.md「使い方の表示」の例は「仕様を検査する。ファイルは書き出さない」と、末尾に句点がない）。
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSuffix(s, "。")
	return strings.TrimSuffix(s, ".")
}
