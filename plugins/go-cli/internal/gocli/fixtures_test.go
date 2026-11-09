package gocli

import (
	"github.com/mew-ton/hozo/ir"
)

// このファイルはテストの入力の View を手で組み立てる。hozocView は docs/cli.md「cli.hozo」を写したもの、
// demoView は cli.hozo にない型（int / float / boolean の必須・既定値・位置引数）を確かめるための小さな名前空間。

func sp(line int) ir.Span {
	return ir.Span{File: "cli.hozo", Start: ir.Pos{Line: line, Col: 3}, End: ir.Pos{Line: line, Col: 40}}
}

func doc(summary string) *ir.Doc { return &ir.Doc{Summary: summary, Body: summary} }

func strLit(s string) ir.ArgValue { return ir.LiteralValue{Literal: ir.StringLiteral{Value: s}} }
func intLit(n int64) ir.ArgValue  { return ir.LiteralValue{Literal: ir.IntLiteral{Value: n}} }

func app(decorator string, args ...ir.Arg) ir.Application {
	return ir.Application{Decorator: decorator, Args: args, Span: sp(0)}
}

func cliCommand(name string) ir.Application {
	return app("cli.command", ir.Arg{Param: "name", Value: strLit(name)})
}
func cliFlag(name string) ir.Application {
	return app("cli.flag", ir.Arg{Param: "name", Value: strLit(name)})
}
func cliEnv(name string) ir.Application {
	return app("cli.env", ir.Arg{Param: "name", Value: strLit(name)})
}
func cliArg() ir.Application { return app("cli.arg") }
func cliExitCode(code int64) ir.Application {
	return app("cli.exitCode", ir.Arg{Param: "code", Value: intLit(code)})
}

func fld(name, summary string, t ir.Type, decs ...ir.Application) ir.Field {
	f := ir.Field{Name: name, Type: t, Decorators: decs, Span: sp(0)}
	if summary != "" {
		f.Description = doc(summary)
	}
	return f
}

func mdl(fqn, summary string, fields ...ir.Field) *ir.Model {
	m := &ir.Model{DeclBase: ir.DeclBase{Name: fqn, Span: sp(0)}, Fields: fields}
	if summary != "" {
		m.Description = doc(summary)
	}
	return m
}

func scl(fqn string, base ir.Type, decs ...ir.Application) *ir.Scalar {
	return &ir.Scalar{DeclBase: ir.DeclBase{Name: fqn, Span: sp(0), Decorators: decs}, Base: base}
}

func exc(fqn string, value ir.Type, decs ...ir.Application) *ir.Exception {
	return &ir.Exception{DeclBase: ir.DeclBase{Name: fqn, Span: sp(0), Decorators: decs}, Value: value}
}

func opDecl(fqn, summary string, params []ir.OpParam, returns ir.Type, exceptions []string, decs ...ir.Application) *ir.Op {
	o := &ir.Op{
		DeclBase:  ir.DeclBase{Name: fqn, Span: sp(0), Decorators: decs},
		Params:    params,
		Returns:   returns,
		Signature: ir.Signature{Exceptions: exceptions},
	}
	if summary != "" {
		o.Description = doc(summary)
	}
	return o
}

func param(name, model string) []ir.OpParam {
	return []ir.OpParam{{Name: name, Type: ir.Ref(model), Span: sp(0)}}
}

// vocabNamespaces は cli と proc の語彙（vocab/cli.hozo、vocab/proc.hozo）のうちテストに要る宣言。
func vocabNamespaces() []ir.Namespace {
	dec := func(fqn string, targets []ir.DecoratorTarget, params ...ir.Param) ir.Decl {
		return &ir.Decorator{DeclBase: ir.DeclBase{Name: fqn, Span: sp(0)}, Params: params, Targets: targets}
	}
	str := ir.Param{Name: "name", Type: ir.TypeParam{Types: []ir.Type{ir.String()}}}
	return []ir.Namespace{
		{Name: "cli", Decls: []ir.Decl{
			dec("cli.command", []ir.DecoratorTarget{ir.TargetOp}, str),
			dec("cli.arg", []ir.DecoratorTarget{ir.TargetField}),
			dec("cli.flag", []ir.DecoratorTarget{ir.TargetField}, str),
			dec("cli.env", []ir.DecoratorTarget{ir.TargetField}, str),
			dec("cli.exitCode", []ir.DecoratorTarget{ir.TargetException}, ir.Param{Name: "code", Type: ir.TypeParam{Types: []ir.Type{ir.Int()}}}),
		}},
		{Name: "proc", Decls: []ir.Decl{
			exc("proc.Crashed", nil),
			exc("proc.MalformedOutput", nil),
			dec("proc.stdio", []ir.DecoratorTarget{ir.TargetOp}),
		}},
	}
}

// view は名前空間 ns を見る側にした View。out を自分で書いた名前空間なので Dir = out。ops は各 op の Signature をそのまま写す。
func view(ns ir.Namespace, out string) *ir.View {
	conf := ir.NamespaceConfig{Language: "go", Out: out, Dir: out, Plugins: []string{"go-cli"}}
	v := &ir.View{
		Version:    ir.Version,
		Viewer:     ns.Name,
		Config:     conf,
		Namespaces: map[string]ir.NamespaceConfig{ns.Name: conf},
		Program:    &ir.Program{Version: ir.Version, Namespaces: append([]ir.Namespace{{Name: ""}}, append(vocabNamespaces(), ns)...)},
		Ops:        map[string]*ir.ViewedSignature{},
	}
	for _, d := range ns.Decls {
		if o, ok := d.(*ir.Op); ok {
			v.Ops[o.Name] = &ir.ViewedSignature{Signature: o.Signature, Params: o.Params}
		}
	}
	return v
}

// hozocNamespace は docs/cli.md「cli.hozo」の namespace hozoc（CLI に関わる宣言）。
func hozocNamespace() ir.Namespace {
	const path = "hozoc.Path"
	diags := ir.Ref("hozoc.Diagnostics")
	return ir.Namespace{
		Name:        "hozoc",
		Description: doc("hozo の仕様を検査し、各言語のコードに展開する CLI"),
		Decls: []ir.Decl{
			scl(path, ir.String()),
			scl("hozoc.OutputFormat", ir.String(), app("valid.pattern", ir.Arg{Param: "regex", Value: strLit("^(text|json)$")})),
			mdl("hozoc.Position", "位置（1 始まりの行と列）", fld("line", "", ir.Int()), fld("col", "", ir.Int())),
			mdl("hozoc.Diagnostic", "検査やプラグインが報告する一件の指摘",
				fld("severity", "error か warning", ir.String()),
				fld("code", "規則の ID", ir.String()),
				fld("message", "", ir.String()),
				fld("file", "", ir.Optional(ir.Ref(path))),
				fld("start", "", ir.Optional(ir.Ref("hozoc.Position"))),
				fld("end", "", ir.Optional(ir.Ref("hozoc.Position"))),
			),
			mdl("hozoc.Diagnostics", "指摘の一覧", fld("items", "", ir.Array(ir.Ref("hozoc.Diagnostic")))),
			exc("hozoc.CheckFailed", diags, cliExitCode(1)),
			exc("hozoc.ConfigInvalid", diags, cliExitCode(2)),
			exc("hozoc.LanguageMismatch", diags, cliExitCode(2)),
			exc("hozoc.OutputConflict", diags, cliExitCode(3)),
			exc("hozoc.UnknownNamespace", nil, cliExitCode(64)),
			mdl("hozoc.CheckResult", "検査の結果（誤りではない指摘）", fld("warnings", "", ir.Array(ir.Ref("hozoc.Diagnostic")))),
			mdl("hozoc.EmitResult", "書き出しの結果",
				fld("warnings", "", ir.Array(ir.Ref("hozoc.Diagnostic"))),
				fld("written", "書いたファイル（新規と更新）", ir.Array(ir.Ref(path))),
				fld("removed", "消したファイル", ir.Array(ir.Ref(path))),
			),
			mdl("hozoc.CheckInput", "check の入力",
				fld("config", "設定ファイルの場所", ir.Optional(ir.Ref(path)), cliFlag("config"), cliEnv("HOZO_CONFIG")),
				fld("format", "表示の形式", ir.Optional(ir.Ref("hozoc.OutputFormat")), cliFlag("format")),
			),
			mdl("hozoc.EmitInput", "emit の入力",
				fld("config", "設定ファイルの場所", ir.Optional(ir.Ref(path)), cliFlag("config"), cliEnv("HOZO_CONFIG")),
				fld("format", "表示の形式", ir.Optional(ir.Ref("hozoc.OutputFormat")), cliFlag("format")),
				fld("namespaces", "生成する名前空間（一つもなければすべて）", ir.Array(ir.String()), cliArg()),
				fld("dryRun", "書き出さずに、書き出す予定のファイルだけを表示する", ir.Optional(ir.Boolean()), cliFlag("dry-run")),
			),
			mdl("hozoc.LspInput", "lsp の入力",
				fld("config", "設定ファイルの場所", ir.Optional(ir.Ref(path)), cliFlag("config"), cliEnv("HOZO_CONFIG")),
				fld("log", "ログの書き出し先", ir.Optional(ir.Ref(path)), cliFlag("log"), cliEnv("HOZO_LSP_LOG")),
			),
			opDecl("hozoc.check", "仕様を検査する。ファイルは書き出さない。", param("in", "hozoc.CheckInput"), ir.Ref("hozoc.CheckResult"),
				[]string{"hozoc.CheckFailed", "hozoc.ConfigInvalid", "hozoc.LanguageMismatch", "proc.Crashed", "proc.MalformedOutput"},
				cliCommand("check")),
			opDecl("hozoc.writeOutputs", "", []ir.OpParam{
				{Name: "in", Type: ir.Ref("hozoc.EmitInput"), Span: sp(0)},
				{Name: "checked", Type: ir.Ref("hozoc.CheckResult"), Span: sp(0)},
			}, ir.Ref("hozoc.EmitResult"),
				[]string{"hozoc.CheckFailed", "hozoc.OutputConflict", "hozoc.UnknownNamespace", "proc.Crashed", "proc.MalformedOutput"}),
			opDecl("hozoc.emit", "仕様を検査し、名前空間ごとに設定されたプラグインを呼んで、生成したファイルを書き出す。", param("in", "hozoc.EmitInput"), ir.Ref("hozoc.EmitResult"),
				[]string{"hozoc.CheckFailed", "hozoc.ConfigInvalid", "hozoc.LanguageMismatch", "hozoc.OutputConflict", "hozoc.UnknownNamespace", "proc.Crashed", "proc.MalformedOutput"},
				cliCommand("emit")),
			opDecl("hozoc.lsp", "言語サーバーとして標準入出力で動く（docs/lsp.md）。クライアントが exit を送るまで戻らない。", param("in", "hozoc.LspInput"), nil, nil,
				cliCommand("lsp")),
		},
	}
}

func hozocView() *ir.View { return view(hozocNamespace(), "internal/gen/hozoc") }

// demoNamespace は変換と必須・既定値・位置引数を確かめる名前空間。
//
//	@cli.command("add") op add(in: AddInput): Sum throws Overflow;
//	model AddInput {
//	  @cli.arg a: int;                       // 必須の位置引数
//	  @cli.arg b: float?;                    // 省略可能な位置引数
//	  @cli.flag("count") count: Count = 1;   // 既定値を持つ scalar（int）
//	  @cli.flag("verbose") verbose: boolean; // 必須でない（既定値 false）... ではなく必須の boolean フラグ
//	  @cli.flag("label") @cli.env("DEMO_LABEL") label: string?;
//	  @cli.env("DEMO_SCALE") scale: float?;  // 環境変数だけ
//	}
//	@cli.command("ping") op ping();          // 引数なし、void
func demoNamespace() ir.Namespace {
	return ir.Namespace{
		Name: "demo",
		Decls: []ir.Decl{
			scl("demo.Count", ir.Int(), app("valid.min", ir.Arg{Param: "value", Value: intLit(0)})),
			mdl("demo.Sum", "", fld("total", "", ir.Float())),
			exc("demo.Overflow", nil, cliExitCode(5)),
			exc("demo.Tired", nil),
			mdl("demo.AddInput", "add の入力",
				fld("a", "左", ir.Int(), cliArg()),
				fld("b", "右", ir.Optional(ir.Float()), cliArg()),
				func() ir.Field {
					f := fld("count", "回数", ir.Ref("demo.Count"), cliFlag("count"))
					f.Default = ir.IntLiteral{Value: 1}
					return f
				}(),
				fld("verbose", "詳しく", ir.Boolean(), cliFlag("verbose")),
				fld("label", "名札", ir.Optional(ir.String()), cliFlag("label"), cliEnv("DEMO_LABEL")),
				fld("scale", "倍率", ir.Optional(ir.Float()), cliEnv("DEMO_SCALE")),
			),
			opDecl("demo.add", "足す", param("in", "demo.AddInput"), ir.Ref("demo.Sum"), []string{"demo.Overflow", "demo.Tired"}, cliCommand("add")),
			opDecl("demo.ping", "何もしない", nil, nil, nil, cliCommand("ping")),
		},
	}
}

func demoView() *ir.View { return view(demoNamespace(), "gen/demo") }
