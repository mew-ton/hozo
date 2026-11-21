package check

import (
	"sort"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/ir"
)

// テストの IR は手で組む（docs/testing.md「単体テスト」）。宣言ごとに違う位置を振り、Related の照合に使う。

var lineCounter int

// sp は宣言ごとに違う位置。file は名前空間の名前から決める（語彙は hozo: の形）。
func sp(fqn string) ir.Span {
	lineCounter++
	return ir.Span{File: fileOf(fqn), Start: ir.Pos{Line: lineCounter, Col: 1}, End: ir.Pos{Line: lineCounter, Col: 10}}
}

func fileOf(fqn string) string {
	for _, prefix := range []string{"@", "call.", "p."} {
		fqn = strings.TrimPrefix(fqn, prefix)
	}
	ns := parentOf(fqn)
	switch {
	case ns == "":
		return "global.hozo"
	case strings.HasPrefix(ns, "http") || strings.HasPrefix(ns, "page") || strings.HasPrefix(ns, "valid"):
		return "hozo:vocab/" + strings.SplitN(ns, ".", 2)[0] + ".hozo"
	}
	return strings.SplitN(ns, ".", 2)[0] + ".hozo"
}

func exc(fqn string) *ir.Exception {
	return &ir.Exception{DeclBase: ir.DeclBase{Name: fqn, Span: sp(fqn)}}
}

// eff は基底の状態の組み込みの効果（`effect fn e(): effects s.op`）。書き込むのはその状態。
func eff(fqn, state string, op ir.Operation) *ir.Effect {
	return &ir.Effect{DeclBase: ir.DeclBase{Name: fqn, Span: sp(fqn)},
		Target: ir.BuiltinTarget{State: state, Operation: op}, Writes: []string{state}}
}

// transition は分類の遷移の効果（`effect fn e(): effects X as from -> to`）。writes は書き込む基底の状態。
func transition(fqn, of string, from, to ir.CaseSet, writes ...string) *ir.Effect {
	return &ir.Effect{DeclBase: ir.DeclBase{Name: fqn, Span: sp(fqn)},
		Target: ir.TransitionTarget{Of: of, From: from, To: to}, Writes: writes}
}

// state は基底の状態。型は形と要素から決める（list は elem[]、nullable は elem?、single は elem）。
func state(fqn string, shape ir.StateShape, elem ir.Type) *ir.State {
	t := elem
	switch shape {
	case ir.StateList:
		t = ir.Array(elem)
	case ir.StateNullable:
		t = ir.Optional(elem)
	}
	return &ir.State{DeclBase: ir.DeclBase{Name: fqn, Span: sp(fqn)}, Type: t, Shape: shape, Element: elem}
}

func scalar(fqn string, base ir.Type) *ir.Scalar {
	return &ir.Scalar{DeclBase: ir.DeclBase{Name: fqn, Span: sp(fqn)}, Base: base}
}

func model(fqn string, fields ...ir.Field) *ir.Model {
	m := &ir.Model{DeclBase: ir.DeclBase{Name: fqn, Span: sp(fqn)}}
	for _, f := range fields {
		f.Span = sp(fqn + "." + f.Name)
		m.Fields = append(m.Fields, f)
	}
	return m
}

func field(name string, t ir.Type) ir.Field { return ir.Field{Name: name, Type: t} }

func param(name string, t ir.Type) ir.OpParam {
	return ir.OpParam{Name: name, Type: t, Span: sp("p." + name)}
}

// leaf は葉の op。effects / throws / decorators は返した値に足す。別の名前空間から呼ぶテストが多いので、
// export を付けた op として作る。内部の op は internal で作る。
func leaf(fqn string, returns ir.Type, params ...ir.OpParam) *ir.Op {
	return &ir.Op{DeclBase: ir.DeclBase{Name: fqn, Span: sp(fqn)}, Exported: true, Params: params, Returns: returns}
}

// internal は o を、export のない op（内部の op。docs/check.md「export」）にする。
func internal(o *ir.Op) *ir.Op {
	o.Exported = false
	return o
}

// reads は o に reads を足す。
func reads(o *ir.Op, states ...string) *ir.Op {
	o.Reads = append(o.Reads, ir.StateReads(states...)...)
	return o
}

// wire は配線の op。
func wire(fqn string, returns ir.Type, params []ir.OpParam, stmts ...ir.Statement) *ir.Op {
	o := leaf(fqn, returns, params...)
	o.Body = &ir.Wiring{Statements: stmts}
	return o
}

func throws(exceptions []string, rels ...ir.Relation) ir.ThrowsClause {
	return ir.ThrowsClause{Exceptions: exceptions, Relations: rels}
}

// prevents は `prevents effects`。目的語は常にその op のすべての効果（docs/ir.md「decorator」の Relation）。
func prevents() ir.Relation { return ir.Relation{Kind: ir.Prevents} }

// keeps は `keeps effects`。
func keeps() ir.Relation { return ir.Relation{Kind: ir.Keeps} }

func decorator(fqn string, params ...ir.Param) *ir.Decorator {
	return &ir.Decorator{DeclBase: ir.DeclBase{Name: fqn, Span: sp(fqn)}, Params: params, Targets: []ir.DecoratorTarget{ir.TargetOp}}
}

func catches(mode ir.CatchMode, halts string, exceptions ...string) ir.CatchesClause {
	return ir.CatchesClause{Exceptions: exceptions, Mode: mode, Halts: halts}
}

func app(fqn string, args ...ir.Arg) ir.Application {
	return ir.Application{Decorator: fqn, Args: args, Span: sp("@" + fqn)}
}

func refArg(param, target string) ir.Arg {
	return ir.Arg{Param: param, Value: ir.RefValue{Target: target}}
}

func refsArg(param string, targets ...string) ir.Arg {
	return ir.Arg{Param: param, Value: ir.RefsValue{Targets: targets}}
}

func call(op string, args ...ir.CallArg) ir.Call {
	return ir.Call{Callee: ir.OpCallee{Op: op}, Args: args, Span: sp("call." + op)}
}

func carg(param string, v ir.Atom) ir.CallArg { return ir.CallArg{Param: param, Value: v} }

func nameAtom(path ...string) ir.Atom { return ir.NameAtom{Path: path} }

// stateAtom は状態の参照（`token`、`session.userId`）。
func stateAtom(state string, path ...string) ir.Atom { return ir.StateAtom{State: state, Path: path} }

func rec(fields ...ir.RecordField) ir.Atom { return ir.RecordAtom{Fields: fields} }

func rf(name string, v ir.Atom) ir.RecordField { return ir.RecordField{Name: name, Value: v} }

func cst(name string, c ir.Call) ir.Statement {
	return ir.ValStmt{Name: name, Expr: ir.CallExpr{Call: c}, Span: c.Span}
}

func cs(c ir.Call) ir.Statement { return ir.CallStmt{Call: c, Span: c.Span} }

func ret(c ir.Call) ir.Statement { return ir.ReturnStmt{Expr: ir.CallExpr{Call: c}, Span: c.Span} }

func retAtom(a ir.Atom) ir.Statement { return ir.ReturnStmt{Expr: ir.AtomExpr{Atom: a}} }

// arm は match のアーム。body が nil なら none。cases がなければ else のアーム（受けるケースは、ここの計算が読まないので、置かない）。
func arm(body *ir.Call, cases ...string) ir.MatchArm {
	a := ir.MatchArm{Cases: cases, Else: len(cases) == 0}
	if body == nil {
		a.Body = ir.NoneBody{}
	} else {
		a.Body = ir.CallExpr{Call: *body}
	}
	return a
}

// matchStmt は値の分類での match の文（`match(x by C){ ... };`）。
func matchStmt(classify string, arms ...ir.MatchArm) ir.Statement {
	return ir.MatchStmt{Match: ir.Match{Subject: ir.ValueSubject{Atom: nameAtom("x")}, Classify: classify, Arms: arms}}
}

func matchConst(name, classify string, arms ...ir.MatchArm) ir.Statement {
	return ir.ValStmt{Name: name, Expr: ir.MatchExpr{Match: ir.Match{Subject: ir.ValueSubject{Atom: nameAtom("x")}, Classify: classify, Arms: arms}}}
}

// matchState は状態を名前で書いた、標準の分類での match の文（`match(s by list){ ... };`）。
func matchState(state string, shape ir.StateShape, arms ...ir.MatchArm) ir.Statement {
	return ir.MatchStmt{Match: ir.Match{Subject: ir.ValueSubject{Atom: stateAtom(state)}, Classify: string(shape), Arms: arms}}
}

// throwArm はアームの本体が `throw E` のアーム。
func throwArm(exception string, cases ...string) ir.MatchArm {
	return ir.MatchArm{Cases: cases, Else: len(cases) == 0, Body: ir.ThrowBody{Exception: exception, Span: sp("call." + parentOf(exception) + ".throw")}}
}

// atomArm はアームの本体が値のアーム。
func atomArm(v ir.Atom, cases ...string) ir.MatchArm {
	return ir.MatchArm{Cases: cases, Else: len(cases) == 0, Body: ir.AtomExpr{Atom: v}}
}

// matchByClassify は状態の分類での match の文（`match(by C){ ... };`）。
func matchByClassify(classify string, arms ...ir.MatchArm) ir.Statement {
	return ir.MatchStmt{Match: ir.Match{Subject: ir.ClassifyOwnedSubject{}, Classify: classify, Arms: arms}}
}

func namespace(name string, decls ...ir.Decl) ir.Namespace {
	return ir.Namespace{Name: name, Decls: decls}
}

func program(binds []ir.Bind, nss ...ir.Namespace) *ir.Program {
	return &ir.Program{Version: ir.Version, Namespaces: nss, Binds: binds}
}

func bind(ns, st, target string, deep bool) ir.Bind {
	return ir.Bind{Namespace: ns, State: st, Target: ir.BindTarget{Namespace: target, Deep: deep}, Span: sp(ns + ".bind." + st)}
}

// cfgFor は名前空間ごとに一つの単位（トランスパイラ t、out は gen/<名前空間>）を持つ設定。
func cfgFor(nss ...string) *config.Config {
	c := config.Default("/root")
	for _, n := range nss {
		c.Units = append(c.Units, config.Unit{Index: len(c.Units) + 1, Namespace: n, Transpilers: []string{"t"}, Out: "gen/" + n, Options: map[string]map[string]any{}})
	}
	return c
}

// allFiles は Program の宣言のファイル（Span.File）のすべてを、ユーザーのファイルを辞書順に、その後に語彙のファイルを辞書順に
// 並べて返す（in を省いた単位の Files。docs/ir.md「その名前空間から見た IR」の View.files）。
func allFiles(p *ir.Program) []string {
	seen := map[string]bool{}
	var user, vocab []string
	for i := range p.Namespaces {
		for _, d := range p.Namespaces[i].Decls {
			f := d.Common().Span.File
			if seen[f] {
				continue
			}
			seen[f] = true
			if strings.HasPrefix(f, "hozo:") {
				vocab = append(vocab, f)
			} else {
				user = append(user, f)
			}
		}
	}
	sort.Strings(user)
	sort.Strings(vocab)
	return append(user, vocab...)
}

// goUnits は cfg の単位のすべてを、言語が go、基底が go で、in を省いたもの（Files は Program のすべてのファイル）にする。
func goUnits(p *ir.Program, cfg *config.Config) []Unit {
	units := make([]Unit, len(cfg.Units))
	for i := range units {
		units[i] = Unit{Language: "go", Base: "go", Files: allFiles(p)}
	}
	return units
}

// viewsOf は、単位がどれも in を省いた Go の単位のときの View を、名前空間 → View で返す。同じ名前空間を複数の単位が
// 出力していれば、設定の順で最初の単位のもの。
func viewsOf(p *ir.Program, cfg *config.Config) map[string]*ir.View {
	out := map[string]*ir.View{}
	for _, m := range Views(p, cfg, goUnits(p, cfg)) {
		for ns, v := range m {
			if out[ns] == nil {
				out[ns] = v
			}
		}
	}
	return out
}

// httpVocab は http の語彙の最小の写し（docs/vocab.md）。post は external で ServerError | NetworkError を投げる。
func httpVocab() ir.Namespace {
	post := decorator("http.post", ir.Param{Name: "path", Type: ir.TypeParam{Types: []ir.Type{ir.String()}}})
	post.External = true
	post.Throws = []ir.ThrowsClause{{Exceptions: []string{"http.ServerError", "http.NetworkError"}}}
	get := decorator("http.get", ir.Param{Name: "path", Type: ir.TypeParam{Types: []ir.Type{ir.String()}}})
	get.External = true
	get.Throws = []ir.ThrowsClause{{Exceptions: []string{"http.ServerError", "http.NetworkError"}}}
	return namespace("http", exc("http.ServerError"), exc("http.NetworkError"), post, get)
}

// locatorFor は テストの Span を直す Locator。テストで使うファイル名ごとに十分な行数の中身を持つ FileSet を作る。
func locatorFor() Locator {
	fs := source.NewFileSet()
	content := []byte(strings.Repeat("xxxxxxxxxxxxxxxxxxxx\n", 20000))
	for _, f := range []string{"global.hozo", "server.hozo", "browser.hozo", "hozo:vocab/http.hozo", "hozo:vocab/page.hozo"} {
		fs.Add(f, content)
	}
	return FileSetLocator(fs)
}

func sigOf(t *testing.T, p *ir.Program, fqn string) ir.Signature {
	t.Helper()
	o, ok := p.Lookup(fqn).(*ir.Op)
	if !ok {
		t.Fatalf("%s is not an op", fqn)
	}
	return o.Signature
}

// wantNoErrors は、diags に error が一つもないことを確かめる（warning は、あってよい）。
func wantNoErrors(t *testing.T, diags []diag.Diagnostic) {
	t.Helper()
	for _, d := range diags {
		if d.Severity == diag.Error {
			t.Errorf("想定しない error: %s %s", d.Code, d.Message)
		}
	}
}

// effectOf は、テストが効果を書く二つの形を EffectRef にする: 文字列は宣言の FQN（名前で書いた効果）、EffectRef はそのまま。
func effectOf(e any) ir.EffectRef {
	if s, ok := e.(string); ok {
		return ir.NamedEffect{Effect: s}
	}
	return e.(ir.EffectRef)
}

func relOf(t *testing.T, sig ir.Signature, x string, e any) ir.EffectState {
	t.Helper()
	for _, r := range sig.Relations {
		if r.Exception == x && r.Effect == effectOf(e) {
			return r.State
		}
	}
	t.Fatalf("no relation (%s, %s) in %v", x, e, sig.Relations)
	return ""
}

func succOf(t *testing.T, sig ir.Signature, e any) ir.EffectState {
	t.Helper()
	for _, s := range sig.SuccessEffects {
		if s.Effect == effectOf(e) {
			return s.State
		}
	}
	t.Fatalf("no success effect %s in %v", e, sig.SuccessEffects)
	return ""
}

func wantStrings(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func codes(diags []diag.Diagnostic) []string {
	var out []string
	for _, d := range diags {
		out = append(out, d.Code)
	}
	return out
}

func withCode(diags []diag.Diagnostic, code string) []diag.Diagnostic {
	var out []diag.Diagnostic
	for _, d := range diags {
		if d.Code == code {
			out = append(out, d)
		}
	}
	return out
}

func wantCodes(t *testing.T, diags []diag.Diagnostic, want ...string) {
	t.Helper()
	got := codes(diags)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("codes = %v, want %v\n%s", got, want, messages(diags))
	}
}

func wantNoCode(t *testing.T, diags []diag.Diagnostic, code string) {
	t.Helper()
	if ds := withCode(diags, code); len(ds) > 0 {
		t.Errorf("unexpected %s: %s", code, messages(ds))
	}
}

func wantOneCode(t *testing.T, diags []diag.Diagnostic, code string) diag.Diagnostic {
	t.Helper()
	ds := withCode(diags, code)
	if len(ds) != 1 {
		t.Fatalf("want exactly one %s, got %d\n%s", code, len(ds), messages(diags))
	}
	return ds[0]
}

func messages(diags []diag.Diagnostic) string {
	var b strings.Builder
	for _, d := range diags {
		b.WriteString("  " + d.Severity.String() + "[" + d.Code + "] " + d.Message + "\n")
	}
	return b.String()
}

// effectNames は効果の列を、表示の文字列（宣言は FQN、直に書いた効果は "<状態の FQN>.<操作>"）の列にする。
func effectNames(effects []ir.EffectRef) []string {
	out := make([]string, len(effects))
	for i, e := range effects {
		out[i] = e.String()
	}
	return out
}
