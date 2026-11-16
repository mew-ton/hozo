package golang

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは、テストの入力の View を手で組み立てる道具。

func span(file string) ir.Span { return ir.Span{File: file} }

func base(fqn, file string, decorators ...string) ir.DeclBase {
	b := ir.DeclBase{Name: fqn, Span: span(file)}
	for _, d := range decorators {
		b.Decorators = append(b.Decorators, ir.Application{Decorator: d})
	}
	return b
}

func model(fqn string, fields ...ir.Field) *ir.Model {
	return &ir.Model{DeclBase: base(fqn, fileOf(fqn)), Fields: fields}
}

func field(name string, t ir.Type) ir.Field { return ir.Field{Name: name, Type: t} }

func scalar(fqn string, b ir.Type) *ir.Scalar {
	return &ir.Scalar{DeclBase: base(fqn, fileOf(fqn)), Base: b}
}

func union(fqn string, arms ...ir.UnionArm) *ir.Union {
	return &ir.Union{DeclBase: base(fqn, fileOf(fqn)), Variant: ir.UnionArms, Arms: arms}
}

func exception(fqn string) *ir.Exception { return &ir.Exception{DeclBase: base(fqn, fileOf(fqn))} }

func state(fqn string, t ir.Type) *ir.State {
	shape, elem := ir.ShapeOf(t)
	return &ir.State{DeclBase: base(fqn, fileOf(fqn)), Type: t, Shape: shape, Element: elem}
}

func effect(fqn string, target ir.EffectTarget, params ...ir.OpParam) *ir.Effect {
	return &ir.Effect{DeclBase: base(fqn, fileOf(fqn)), Params: params, Target: target}
}

// read は read の宣言: 基底の状態 stateFQN を読んで、returns を得る。
func read(fqn, stateFQN string, returns ir.Type, params ...ir.OpParam) *ir.Read {
	return &ir.Read{DeclBase: base(fqn, fileOf(fqn)), Params: params, Returns: returns, State: stateFQN}
}

// classify は分類。subject は ir.TypeSubject / ir.OpSubject / ir.StatesSubject。
func classify(fqn string, subject ir.ClassifySubject, by ir.DecidedBy, cases ...string) *ir.Classify {
	c := &ir.Classify{DeclBase: base(fqn, fileOf(fqn)), Subject: subject, DecidedBy: by, HasDefault: true}
	for _, name := range cases {
		c.Cases = append(c.Cases, ir.ClassifyCase{Name: name})
	}
	return c
}

func param(name string, t ir.Type) ir.OpParam { return ir.OpParam{Name: name, Type: t} }

// op は葉の op。純粋さと例外は、宣言から簡単に決める（デコレータ・reads・effects があれば純粋でない）。
func op(fqn string, returns ir.Type, params ...ir.OpParam) *ir.Op {
	return &ir.Op{DeclBase: base(fqn, fileOf(fqn)), Exported: true, Params: params, Returns: returns, Signature: ir.Signature{Pure: true}}
}

type opOption func(*ir.Op)

func with(o *ir.Op, opts ...opOption) *ir.Op {
	for _, f := range opts {
		f(o)
	}
	return o
}

func internal() opOption { return func(o *ir.Op) { o.Exported = false } }
func impure() opOption   { return func(o *ir.Op) { o.Signature.Pure = false } }
func throwing(exceptions ...string) opOption {
	return func(o *ir.Op) { o.Signature.Exceptions = exceptions }
}

// carryingReads と carryingEffects は、デコレータが運ぶ reads / effects（Op.carried。docs/check.md「デコレータが運ぶ reads と effects」）。
// 葉の op が状態に触れるのは、この分だけ（葉は reads / effects の節を書けない）。Signature にも入る。
func carryingReads(states ...string) opOption {
	return func(o *ir.Op) {
		o.Carried.Reads = append(o.Carried.Reads, ir.StateReads(states...)...)
		o.Signature.Reads = append(o.Signature.Reads, states...)
		o.Signature.Pure = false
	}
}
func carryingEffects(effects ...string) opOption {
	return func(o *ir.Op) {
		o.Carried.Effects = append(o.Carried.Effects, ir.NamedEffects(effects...)...)
		o.Signature.Effects = append(o.Signature.Effects, ir.NamedEffects(effects...)...)
		o.Signature.Pure = false
	}
}
func decorated(decorators ...string) opOption {
	return func(o *ir.Op) {
		for _, d := range decorators {
			o.Decorators = append(o.Decorators, ir.Application{Decorator: d})
		}
		o.Signature.Pure = false
	}
}
func inFile(file string) opOption { return func(o *ir.Op) { o.Span.File = file } }
func wired(stmts ...ir.Statement) opOption {
	return func(o *ir.Op) { o.Body = &ir.Wiring{Statements: stmts}; o.Signature.Pure = false }
}

func call(opFQN string) ir.Statement {
	return ir.CallStmt{Call: ir.Call{Callee: ir.OpCallee{Op: opFQN}}}
}

// callRead、callEffect、callDirect は、read の宣言、effect の宣言、直に呼ぶ組み込みの効果の呼び出し（docs/ir.md「配線」の Callee）。
func callRead(readFQN string) ir.Statement {
	return ir.ConstStmt{Name: "x", Expr: ir.CallExpr{Call: ir.Call{Callee: ir.ReadCallee{Read: readFQN}}}}
}

func callEffect(effectFQN string) ir.Statement {
	return ir.CallStmt{Call: ir.Call{Callee: ir.EffectCallee{Effect: ir.NamedEffect{Effect: effectFQN}}}}
}

func callDirect(stateFQN string, operation ir.Operation) ir.Statement {
	return ir.CallStmt{Call: ir.Call{Callee: ir.EffectCallee{Effect: ir.DirectEffect{State: stateFQN, Operation: operation}}}}
}

func matchBy(classifyFQN string, arms ...ir.MatchArm) ir.Statement {
	return ir.MatchStmt{Match: ir.Match{Subject: ir.ValueSubject{Atom: ir.NameAtom{Path: []string{"v"}}}, Classify: classifyFQN, Arms: arms}}
}

// armDirect は、直に呼ぶ組み込みの効果を本体にしたアーム。
func armDirect(stateFQN string, operation ir.Operation, cases ...string) ir.MatchArm {
	return ir.MatchArm{Cases: ir.ArmCases{Cases: cases},
		Body: ir.CallExpr{Call: ir.Call{Callee: ir.EffectCallee{Effect: ir.DirectEffect{State: stateFQN, Operation: operation}}}}}
}

func armCall(opFQN string, cases ...string) ir.MatchArm {
	return ir.MatchArm{Cases: ir.ArmCases{Cases: cases}, Body: ir.CallExpr{Call: ir.Call{Callee: ir.OpCallee{Op: opFQN}}}}
}

func namespace(name string, decls ...ir.Decl) ir.Namespace {
	return ir.Namespace{Name: name, Decls: decls}
}

// fileOf は宣言のファイル: 名前空間の先頭のセグメント + ".hozo"。
func fileOf(fqn string) string {
	for i := 0; i < len(fqn); i++ {
		if fqn[i] == '.' {
			return fqn[:i] + ".hozo"
		}
	}
	return "global.hozo"
}

// conf は単位 index での名前空間の設定（基底は go、out と dir は dir）。
func conf(index int, dir string) ir.NamespaceConfig {
	return ir.NamespaceConfig{Index: index, Language: "go", Base: "go", Out: dir, Dir: dir, Transpilers: []string{"go"},
		Options: map[string]map[string]any{}, Seams: map[string]string{}}
}

// viewOf は、viewer を gen/<viewer> に出力する単位 1 の View。files は、Program のすべてのファイル（in を省いた単位）。
// namespaces は、viewer だけ（ほかの名前空間の置き場は、テストが足す）。
func viewOf(viewer string, nss ...ir.Namespace) *ir.View {
	p := &ir.Program{Namespaces: nss}
	seen := map[string]bool{}
	var files []string
	for _, ns := range nss {
		for _, d := range ns.Decls {
			if f := d.Common().Span.File; !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}
	c := conf(1, "gen/"+viewer)
	return &ir.View{Version: ir.Version, Viewer: viewer, Config: c, Files: files, Program: p,
		Namespaces: map[string][]ir.NamespaceConfig{viewer: {c}}}
}

// projectRoot は go.mod（module example.com/app）を持つ一時のプロジェクトのルート。
func projectRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/app\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
