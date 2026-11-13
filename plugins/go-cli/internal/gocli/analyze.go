package gocli

import (
	"fmt"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
)

// cli の語彙のデコレータの FQN（vocab/cli.hozo）。
const (
	decCommand  = "cli.command"
	decArg      = "cli.arg"
	decFlag     = "cli.flag"
	decEnv      = "cli.env"
	decExitCode = "cli.exitCode"
)

// decStdio は標準の語彙 proc のデコレータ（vocab/proc.hozo）。プロセスの境界を越える op の印で、選択の stops にする
// （docs/plugin-go-cli.md「マニフェスト」）。
const decStdio = "proc.stdio"

// defaultExitCode は @cli.exitCode のない例外と、宣言にない例外の終了コード（docs/plugin-go-cli.md「実行と表示」）。
const defaultExitCode = 70

// kind は CLI が受け付けるフィールドの元の型（docs/plugin-go-cli.md「対象」）。
type kind int

const (
	kindString kind = iota
	kindBoolean
	kindInt
	kindFloat
)

// label は使い方に出す型の名前（<string> など）。
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

// program は、一つの名前空間から生成する CLI の全体。analyze が作り、generate が Go にする。
type program struct {
	Namespace string // 見る側の名前空間の FQN
	Package   string // 名前空間のパッケージの名前（import に使う名前。Imports.Add の戻り値）
	Imports   *golang.Imports
	Commands  []*command // 宣言順
}

// command は @cli.command の付いた op 一つ。
type command struct {
	Name       string // @cli.command の name
	Op         *ir.Op
	Method     string // Ops のメソッド名
	Summary    string // description.summary（一行に潰したもの）
	Input      *model // 引数の model。引数が 0 個なら nil
	Void       bool   // returns が null
	Exceptions []exception
}

// model は入力の model。
type model struct {
	Decl       *ir.Model
	GoType     string // "hozoc.CheckInput"
	HasDefault bool   // 既定値を持つフィールドがあれば New<Model>() で作る（docs/emit-go.md「model」）
	Fields     []*field
	Args       []*field // @cli.arg のフィールド（宣言順）
}

// field は入力の model のフィールド一つ。
type field struct {
	Name      string // 元の名前
	GoName    string // Go のフィールド名
	Summary   string
	Flag      string // @cli.flag の名前。なければ ""
	Env       string // @cli.env の名前。なければ ""
	Arg       bool   // @cli.arg
	Kind      kind
	Array     bool   // string[]（@cli.arg だけ）
	Optional  bool   // T?
	Required  bool   // optional でなく default もない
	Default   string // 既定値の表示。なければ ""
	ConvType  string // 値をこの Go の型に変換する（"string"、"int64"、"hozoc.Path" など）。配列なら要素の型
	TypeLabel string // 使い方に出す型（"Path"、"int"）
}

// exception は op が投げうる例外一つ（Signature.exceptions の順）。
type exception struct {
	FQN      string
	GoType   string // "hozoc.CheckFailed"
	HasValue bool
	ExitCode int
}

// analyzeError は analyze が返す誤り。CLI-* の診断（Errors）か、IR の形の誤り（Err）。
type analyzeError struct {
	Errors []plugin.Diagnostic
	Err    error
}

func (e *analyzeError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("%d CLI-* error(s)", len(e.Errors))
}

// analyze は View から CLI の全体を作る。docs/plugin-go-cli.md「対象」の CLI-1〜CLI-5 を確かめ、
// 一つでもあれば *analyzeError（Errors）を返す。IR の形が約束と違えば *analyzeError（Err）。
// nsImportPath は見る側の名前空間のパッケージの import パス。
func analyze(view *ir.View, nsImportPath string) (*program, error) {
	if view == nil || view.Program == nil {
		return nil, &analyzeError{Err: fmt.Errorf("view has no program")}
	}
	ns := view.Program.Namespace(view.Viewer)
	if ns == nil {
		return nil, &analyzeError{Err: fmt.Errorf("namespace %q is not in the program", view.Viewer)}
	}
	a := &analyzer{view: view, prog: view.Program}
	a.imports = &golang.Imports{Viewer: view.Viewer, Resolve: a.resolve}
	for _, p := range stdImports {
		a.imports.Add(p, p[strings.LastIndexByte(p, '/')+1:])
	}
	a.nsImportPath = nsImportPath
	a.pkg = a.imports.Add(nsImportPath, golang.PackageName(view.Viewer))

	out := &program{Namespace: view.Viewer, Package: a.pkg, Imports: a.imports}
	seenCommands := map[string]string{}
	for _, d := range ns.Decls {
		op, ok := d.(*ir.Op)
		if !ok || !view.InUnit(op.Name) {
			continue // サブコマンドにするのは単位の対象の op だけ（docs/plugin-go-cli.md「対象」）
		}
		app := findDecorator(op.Decorators, decCommand)
		if app == nil {
			continue
		}
		name, ok := stringArg(app, "name")
		if !ok {
			return nil, &analyzeError{Err: fmt.Errorf("%s: @cli.command has no string name", op.Name)}
		}
		if prev, dup := seenCommands[name]; dup {
			a.errorf("CLI-5", op.Name, "コマンド名 %q が %s と重複している", name, prev)
		} else {
			seenCommands[name] = op.Name
		}
		cmd, err := a.command(name, op)
		if err != nil {
			return nil, err
		}
		if cmd != nil {
			out.Commands = append(out.Commands, cmd)
		}
	}
	if len(a.errors) > 0 {
		return nil, &analyzeError{Errors: a.errors}
	}
	return out, nil
}

// stdImports は生成するファイルが常に使う標準ライブラリ。名前空間のパッケージより先に登録し、
// 名前が衝突すれば名前空間の側に別名が付くようにする。
var stdImports = []string{
	"encoding/json", "errors", "fmt", "io", "os", "path/filepath", "strconv", "strings",
	"github.com/mew-ton/hozo/runtime",
}

type analyzer struct {
	view         *ir.View
	prog         *ir.Program
	imports      *golang.Imports
	nsImportPath string
	pkg          string
	errors       []plugin.Diagnostic
	models       map[string]*model // 確かめた model（同じ model を使う op が複数でも一度だけ）
}

func (a *analyzer) errorf(code, at, format string, args ...any) {
	a.errors = append(a.errors, plugin.Diagnostic{Code: code, At: at, Message: fmt.Sprintf(format, args...)})
}

// resolve は参照の FQN をすべて見る側の名前空間のパッケージの型にする（golang.Resolver）。
// 別の名前空間の型は、既定の出力が見る側のパッケージに複製する（docs/emit-go.md「配置」）ものとして、
// その宣言の名前空間でセグメントを切った名前にする。
func (a *analyzer) resolve(fqn string) (namespace, pkgPath, pkgName string, local bool) {
	return a.namespaceOf(fqn), a.nsImportPath, golang.PackageName(a.view.Viewer), false
}

// namespaceOf は宣言を持つ名前空間の FQN。宣言が見つからなければ FQN の形から推測する。
func (a *analyzer) namespaceOf(fqn string) string {
	for i := range a.prog.Namespaces {
		for _, d := range a.prog.Namespaces[i].Decls {
			if d != nil && d.Common().Name == fqn {
				return a.prog.Namespaces[i].Name
			}
		}
	}
	if i := strings.LastIndexByte(fqn, '.'); i >= 0 {
		return fqn[:i]
	}
	return ""
}

// typeName は宣言の FQN を、見る側のパッケージで修飾した Go の型にする。
func (a *analyzer) typeName(fqn string) string {
	return golang.GoType(ir.Ref(fqn), a.imports)
}

// command は op 一つを command にする。CLI-1 なら nil（誤りは a.errors に足す）。
func (a *analyzer) command(name string, op *ir.Op) (*command, error) {
	cmd := &command{
		Name:   name,
		Op:     op,
		Method: golang.MethodName(lastSegment(op.Name)),
		Void:   op.Returns == nil,
	}
	if op.Description != nil {
		cmd.Summary = oneLine(op.Description.Summary)
	}
	switch len(op.Params) {
	case 0:
	case 1:
		ref, ok := op.Params[0].Type.(ir.RefType)
		var m *ir.Model
		if ok {
			m, _ = a.prog.Lookup(ref.Target).(*ir.Model)
		}
		if m == nil {
			a.errorf("CLI-1", op.Name, "@cli.command の op %s の引数 %s は model でなく %s", op.Name, op.Params[0].Name, op.Params[0].Type)
			return nil, nil
		}
		cmd.Input = a.model(m)
	default:
		a.errorf("CLI-1", op.Name, "@cli.command の op %s の引数は 0 個か model 一つでなければならないが %d 個ある", op.Name, len(op.Params))
		return nil, nil
	}

	exceptions := op.Signature.Exceptions
	if sig := a.view.Signature(op.Name); sig != nil {
		exceptions = sig.Exceptions
	}
	for _, fqn := range exceptions {
		ex, ok := a.prog.Lookup(fqn).(*ir.Exception)
		if !ok {
			return nil, &analyzeError{Err: fmt.Errorf("%s: exception %s is not declared", op.Name, fqn)}
		}
		e := exception{FQN: fqn, GoType: a.typeName(fqn), HasValue: ex.Value != nil, ExitCode: defaultExitCode}
		if app := findDecorator(ex.Decorators, decExitCode); app != nil {
			code, ok := intArg(app, "code")
			if !ok {
				return nil, &analyzeError{Err: fmt.Errorf("%s: @cli.exitCode has no int code", fqn)}
			}
			e.ExitCode = int(code)
		}
		cmd.Exceptions = append(cmd.Exceptions, e)
	}
	return cmd, nil
}

// model は入力の model を確かめて model にする（CLI-2〜CLI-5）。同じ model は一度だけ確かめる。
func (a *analyzer) model(decl *ir.Model) *model {
	if a.models == nil {
		a.models = map[string]*model{}
	}
	if m, ok := a.models[decl.Name]; ok {
		return m
	}
	m := &model{Decl: decl, GoType: a.typeName(decl.Name)}
	a.models[decl.Name] = m
	flags := map[string]string{}
	for i := range decl.Fields {
		f := &decl.Fields[i]
		at := decl.Name + "." + f.Name
		fd := &field{Name: f.Name, GoName: golang.FieldName(f.Name)}
		if f.Description != nil {
			fd.Summary = oneLine(f.Description.Summary)
		}
		if f.Default != nil {
			m.HasDefault = true
			fd.Default = literalString(f.Default)
		}
		fd.Arg = findDecorator(f.Decorators, decArg) != nil
		if app := findDecorator(f.Decorators, decFlag); app != nil {
			fd.Flag, _ = stringArg(app, "name")
		}
		if app := findDecorator(f.Decorators, decEnv); app != nil {
			fd.Env, _ = stringArg(app, "name")
		}
		switch {
		case !fd.Arg && fd.Flag == "" && fd.Env == "":
			a.errorf("CLI-2", at, "フィールド %s に @cli.arg、@cli.flag、@cli.env のどれもない", at)
			continue
		case fd.Arg && fd.Flag != "":
			a.errorf("CLI-2", at, "フィールド %s に @cli.arg と @cli.flag の両方は付けられない", at)
			continue
		}
		if fd.Flag != "" {
			if prev, dup := flags[fd.Flag]; dup {
				a.errorf("CLI-5", at, "フラグ名 %q が %s と重複している", fd.Flag, prev)
			} else {
				flags[fd.Flag] = at
			}
		}
		if !a.fieldType(fd, f.Type, at) {
			continue
		}
		fd.Required = !fd.Optional && !fd.Array && f.Default == nil
		m.Fields = append(m.Fields, fd)
		if fd.Arg {
			m.Args = append(m.Args, fd)
		}
	}
	for i, f := range m.Args {
		if f.Array && i != len(m.Args)-1 {
			a.errorf("CLI-4", decl.Name+"."+f.Name, "@cli.arg の array のフィールド %s は @cli.arg のフィールドの最後でなければならない", f.Name)
		}
	}
	return m
}

// fieldType はフィールドの型を確かめて fd に写す（CLI-3）。受け付けなければ false。
func (a *analyzer) fieldType(fd *field, t ir.Type, at string) bool {
	if opt, ok := t.(ir.OptionalType); ok {
		fd.Optional = true
		t = opt.Of
		if _, isArray := t.(ir.ArrayType); isArray {
			a.errorf("CLI-3", at, "フィールド %s の型 %s: array に optional は付けられない（0 個を表せるため）", at, ir.Optional(t))
			return false
		}
	}
	if arr, ok := t.(ir.ArrayType); ok {
		if !fd.Arg {
			a.errorf("CLI-3", at, "フィールド %s の型 %s: array は @cli.arg にだけ付けられる", at, t)
			return false
		}
		fd.Array = true
		t = arr.Of
		k, conv, label, ok := a.scalarKind(t)
		if !ok || k != kindString {
			a.errorf("CLI-3", at, "フィールド %s の型 %s: array の要素は string か string の scalar でなければならない", at, arr)
			return false
		}
		fd.Kind, fd.ConvType, fd.TypeLabel = k, conv, label
		return true
	}
	k, conv, label, ok := a.scalarKind(t)
	if !ok {
		a.errorf("CLI-3", at, "フィールド %s の型 %s は受け付けられない（string、boolean、int、float、その scalar、その optional、@cli.arg なら string の array）", at, t)
		return false
	}
	if k == kindBoolean && fd.Arg {
		a.errorf("CLI-3", at, "フィールド %s: @cli.arg に boolean は付けられない", at)
		return false
	}
	fd.Kind, fd.ConvType, fd.TypeLabel = k, conv, label
	return true
}

// scalarKind は core か scalar の型について、元の型の種類、変換先の Go の型、使い方に出す名前を返す。
func (a *analyzer) scalarKind(t ir.Type) (k kind, conv, label string, ok bool) {
	switch x := t.(type) {
	case ir.CoreType:
		switch x.Name {
		case ir.CoreString:
			return kindString, "string", "string", true
		case ir.CoreBoolean:
			return kindBoolean, "bool", "boolean", true
		case ir.CoreInt:
			return kindInt, "int64", "int", true
		case ir.CoreFloat:
			return kindFloat, "float64", "float", true
		}
	case ir.RefType:
		sc, isScalar := a.prog.Lookup(x.Target).(*ir.Scalar)
		if !isScalar {
			return 0, "", "", false
		}
		k, _, _, ok := a.scalarKind(sc.Base)
		if !ok {
			return 0, "", "", false
		}
		return k, a.typeName(x.Target), lastSegment(x.Target), true
	}
	return 0, "", "", false
}

// findDecorator は fqn のデコレータの適用。なければ nil。
func findDecorator(apps []ir.Application, fqn string) *ir.Application {
	for i := range apps {
		if apps[i].Decorator == fqn {
			return &apps[i]
		}
	}
	return nil
}

// stringArg は引数 param の文字列リテラル。
func stringArg(app *ir.Application, param string) (string, bool) {
	for _, arg := range app.Args {
		if arg.Param != param {
			continue
		}
		if lit, ok := arg.Value.(ir.LiteralValue); ok {
			if s, ok := lit.Literal.(ir.StringLiteral); ok {
				return s.Value, true
			}
		}
	}
	return "", false
}

// intArg は引数 param の整数リテラル。
func intArg(app *ir.Application, param string) (int64, bool) {
	for _, arg := range app.Args {
		if arg.Param != param {
			continue
		}
		if lit, ok := arg.Value.(ir.LiteralValue); ok {
			if n, ok := lit.Literal.(ir.IntLiteral); ok {
				return n.Value, true
			}
		}
	}
	return 0, false
}

// literalString は既定値の表示。
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

func lastSegment(fqn string) string {
	return fqn[strings.LastIndexByte(fqn, '.')+1:]
}

// hasFlags は @cli.flag のフィールドがあるか。
func (m *model) hasFlags() bool {
	for _, f := range m.Fields {
		if f.Flag != "" {
			return true
		}
	}
	return false
}

// oneLine は summary を使い方の一行にする: 改行と連続する空白を一つの空白にし、末尾の句点を落とす
// （docs/plugin-go-cli.md「使い方の表示」の例は「仕様を検査する。ファイルは書き出さない」と句点なし）。
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSuffix(s, "。")
	return strings.TrimSuffix(s, ".")
}
