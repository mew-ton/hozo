package sema

import (
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは、残りのケース（`対象 as else`）の IR（docs/ir.md「classify」の CaseTyping、docs/check.md「残りのケース」、
// docs/types.md「残りのケースでの対象の型」「アームの中の対象の型」）。診断（CLS-1、CLS-7）は testdata/rules が確かめる。

const elseSource = `
namespace a {
  model M { id: string; }
  model N { id: string; }
  model O { id: string; }
  union Two { Ready: M; Missing: N; }
  union Three { Ready: M; Missing: N; Broken: O; }
  state token: nullable<string>;
  op find(): M?;
  op takeN(n: N): void;
  op takeM(m: M): void;
  op f(): void;

  classify V of M? { Has: M? as M; /** ない */ Lacks: M? as else; }
  classify F of find { NotFound: find as else; Found: find as M; }
  classify S of token { LoggedIn: token as string; LoggedOut: token as else; }
  classify P2 of Two { IsReady: Two as M; Rest: Two as else; }
  classify P3 of Three { IsReady: Three as M; Rest: Three as else; }
  classify D of find { Found: find as M; }

  effect logout() on S: LoggedIn -> LoggedOut;

  op wire(two: Two, three: Three): void {
    val   m = find();
    match(m by F){ Found => takeM(m); NotFound => none; };
    match(two by P2){ IsReady => takeM(two); Rest => takeN(two); };
    match(three by P3){ IsReady => takeM(three); Rest => none; };
    match(two by P2){ IsReady, Rest => f(); };
    match(by S){ LoggedIn => f(); LoggedOut => none; };
  }
}
`

func analyzeElse(t *testing.T) *ir.Program {
	t.Helper()
	res, fs := analyzeProject(t, project{"a.hozo": elseSource}, false)
	noErrors(t, res, fs)
	for _, d := range res.Diags {
		t.Errorf("想定しない診断: %s %s", d.Code, d.Message)
	}
	return res.Program
}

// TestElseCaseTyping は、残りのケースの CaseTyping: by は else、type は、そのケースでの対象の型
// （presence は対象の型のまま、variant は、残りのアームがちょうど一つならそのアームの型、二つ以上なら対象の型のまま）。
func TestElseCaseTyping(t *testing.T) {
	p := analyzeElse(t)
	optM := ir.Optional(ir.Ref("a.M"))
	tests := []struct {
		classify, elseCase string
		state              string
		typ                ir.Type
	}{
		{"a.V", "Lacks", "", optM},
		{"a.F", "NotFound", "", optM},
		{"a.S", "LoggedOut", "a.token", ir.Optional(ir.String())},
		{"a.P2", "Rest", "", ir.Ref("a.N")},
		{"a.P3", "Rest", "", ir.Ref("a.Three")},
	}
	for _, tt := range tests {
		c := p.Lookup(tt.classify).(*ir.Classify)
		cs := c.Case(tt.elseCase)
		if cs == nil {
			t.Fatalf("%s: 残りのケース %s が cases にない", tt.classify, tt.elseCase)
		}
		if len(cs.As) != 1 {
			t.Fatalf("%s.%s の as = %+v", tt.classify, tt.elseCase, cs.As)
		}
		as := cs.As[0]
		if as.By != ir.NarrowByElse || as.State != tt.state || !ir.TypeEqual(as.Type, tt.typ) {
			t.Errorf("%s.%s の as = {state: %q, type: %s, by: %s}、期待は {state: %q, type: %s, by: else}",
				tt.classify, tt.elseCase, as.State, as.Type, as.By, tt.state, tt.typ)
		}
		if !cs.IsElse() || c.ElseCase() != cs {
			t.Errorf("%s: IsElse / ElseCase が残りのケース %s を指さない", tt.classify, tt.elseCase)
		}
		// 残りのケースは、ほかのケースと同じに cases に入る。分類は型で決まる
		if len(c.Cases) != 2 || c.DecidedBy != ir.DecidedByType {
			t.Errorf("%s: cases = %d、decidedBy = %s（期待は 2、type）", tt.classify, len(c.Cases), c.DecidedBy)
		}
	}
	// 残りのケースを書かない分類には、残りのケースがない。それでも、型で決まる（docs/check.md「ケースの判定」）
	d := p.Lookup("a.D").(*ir.Classify)
	if d.ElseCase() != nil || d.DecidedBy != ir.DecidedByType || len(d.Cases) != 1 {
		t.Errorf("a.D: ElseCase = %v、decidedBy = %s、cases = %d", d.ElseCase(), d.DecidedBy, len(d.Cases))
	}
	// 説明は、ほかのケースと同じに付く
	if cs := p.Lookup("a.V").(*ir.Classify).Case("Lacks"); cs.Description == nil || cs.Description.Summary != "ない" {
		t.Errorf("a.V.Lacks の説明 = %+v", cs.Description)
	}
	// 遷移のケースに、名前で書ける
	logout := p.Lookup("a.logout").(*ir.Effect)
	target, ok := logout.Target.(ir.TransitionTarget)
	if !ok || len(target.To.Cases) != 1 || target.To.Cases[0] != "LoggedOut" {
		t.Errorf("a.logout の宛先 = %+v", logout.Target)
	}
}

// TestElseArmNarrowing は、残りのケースのアームの中の対象の型（docs/types.md「アームの中の対象の型」の 3）。
func TestElseArmNarrowing(t *testing.T) {
	p := analyzeElse(t)
	stmts := p.Lookup("a.wire").(*ir.Op).Body.Statements
	arms := func(i int) []ir.MatchArm { return stmts[i].(ir.MatchStmt).Match.Arms }
	narrowed := func(a ir.MatchArm) []ir.Type {
		var out []ir.Type
		for _, n := range a.Narrowed {
			out = append(out, n.Type)
		}
		return out
	}
	one := func(name string, a ir.MatchArm, want ir.Type) {
		t.Helper()
		got := narrowed(a)
		if want == nil {
			if len(got) != 0 {
				t.Errorf("%s: 絞り込まれた（%v）。期待は、外のまま", name, got)
			}
			return
		}
		if len(got) != 1 || !ir.TypeEqual(got[0], want) {
			t.Errorf("%s: narrowed = %v、期待は %s", name, got, want)
		}
	}
	// presence: 型を書いたケースは T、残りのケースは外のまま
	one("F.Found", arms(1)[0], ir.Ref("a.M"))
	one("F.NotFound", arms(1)[1], nil)
	// variant で残りのアームが一つ: そのアームの型
	one("P2.IsReady", arms(2)[0], ir.Ref("a.M"))
	one("P2.Rest", arms(2)[1], ir.Ref("a.N"))
	// variant で残りのアームが二つ以上: 外のまま
	one("P3.Rest", arms(3)[1], nil)
	// 型を書いたケースと残りのケースを並べたアーム: 型が等しくないので、型を書かないケースと同じ絞り込み（規則 4）
	one("P2.IsReady, Rest", arms(4)[0], ir.Case("a.P2", []string{"IsReady", "Rest"}, ir.Ref("a.Two")))
	// 状態の分類: 残りのケースは外のまま
	one("S.LoggedOut", arms(5)[1], nil)
}
