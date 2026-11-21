package sema

import (
	"reflect"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは、閉じた分類（docs/check.md「閉じた分類」「ケースの判定」「アームが受けるケース」）の IR: else のアームの
// cases と else（docs/ir.md「配線」の MatchArm）、else のアームの中の対象の型（docs/types.md「アームの中の対象の型」の 1）、
// 残りのケースを書かない分類の decidedBy。診断（MATCH-2、MATCH-9、CLS-1、CLS-2）は testdata/rules が確かめる。

const closedSource = `
namespace a {
  model M { id: string; }
  model N { id: string; }
  model O { id: string; }
  union Three { Ready: M; Missing: N; Broken: O; }
  scalar Email extends string;
  state token: nullable<string>;
  state carts: list<M>;
  fn find(): M?;
  fn takeM(m: M): void;
  fn takeO(o: O): void;
  fn takeA(m: K.A<M>): void;
  fn takeString(s: string): void;
  fn f(): void;

  classify K of M { A; B; C; D; E; }
  classify Arrival of find { Arrived: find as M; }
  classify Part of Three { IsReady: Three as M; IsMissing: Three as N; }
  classify Whole of Three { IsReady: Three as M; IsMissing: Three as N; IsBroken: Three as O; }
  classify F of find { Found: find as M; NotFound: find as else; }
  classify Held of token { Holding: token as string; }
  classify Valid of string { Ok: string as Email; }
  classify Session of token { LoggedIn: token as string; LoggedOut: token as else; }

  fn wire(m: M, three: Three): void {
    match(m by K){ A => takeA(m); else => none; };
    match(m by K){ D, A => f(); C => f(); else => none; };
    match(m by K){ B, C, D, E => f(); else => takeA(m); };
    val   x = find();
    match(x by F){ NotFound => none; else => takeM(x); };
    match(x by Arrival){ Arrived => takeM(x); };
    match(three by Whole){ IsReady => takeM(three); else => f(); };
    match(three by Whole){ IsReady, IsMissing => f(); else => takeO(three); };
    match(token by nullable){ Some => f(); else => none; };
    match(x by nullable){ None => none; else => takeM(x); };
    match(carts by list){ HasItems => f(); else => none; };
    match(by Session){ LoggedOut => none; else => takeString(token); };
  }
}
`

func analyzeClosed(t *testing.T) *ir.Program {
	t.Helper()
	res, fs := analyzeProject(t, project{"a.hozo": closedSource}, false)
	noErrors(t, res, fs)
	for _, d := range res.Diags {
		t.Errorf("想定しない診断: %s %s", d.Code, d.Message)
	}
	return res.Program
}

// TestClosedDecidedBy は、残りを受けるケースのない分類も、条件 1〜3 を満たせば、型で決まること（docs/check.md「ケースの判定」。
// 条件 4 は欠番）。述語の要る絞り込みのケースがあれば、述語で決まる。
func TestClosedDecidedBy(t *testing.T) {
	p := analyzeClosed(t)
	for name, want := range map[string]ir.DecidedBy{
		"a.Arrival": ir.DecidedByType,      // presence で、残りのケースがない
		"a.Part":    ir.DecidedByType,      // variant で、すべてのアームを覆わず、残りのケースもない
		"a.Whole":   ir.DecidedByType,      // variant で、すべてのアームを覆う
		"a.F":       ir.DecidedByType,      // presence と、残りのケース
		"a.Held":    ir.DecidedByType,      // 対象の状態が一つの状態の分類
		"a.K":       ir.DecidedByPredicate, // as を持たないケース
		"a.Valid":   ir.DecidedByPredicate, // 述語の要る絞り込み
	} {
		c := p.Lookup(name).(*ir.Classify)
		if c.DecidedBy != want {
			t.Errorf("%s: decidedBy = %s、期待は %s", name, c.DecidedBy, want)
		}
	}
	if c := p.Lookup("a.Arrival").(*ir.Classify); len(c.Cases) != 1 || c.ElseCase() != nil {
		t.Errorf("a.Arrival: cases = %d、ElseCase = %v", len(c.Cases), c.ElseCase())
	}
}

// TestElseArm は、else のアームの IR と、その中の対象の型。
func TestElseArm(t *testing.T) {
	p := analyzeClosed(t)
	stmts := p.Lookup("a.wire").(*ir.Op).Body.Statements
	type arm struct {
		cases    []string
		isElse   bool
		narrowed []string
	}
	armsOf := func(i int) []arm {
		var out []arm
		for _, a := range stmts[i].(ir.MatchStmt).Match.Arms {
			x := arm{cases: a.Cases, isElse: a.Else}
			for _, n := range a.Narrowed {
				x.narrowed = append(x.narrowed, n.State+":"+n.Type.String())
			}
			out = append(out, x)
		}
		return out
	}
	tests := []struct {
		name string
		stmt int
		want []arm
	}{
		// else のアームの cases は、ほかのアームが並べなかったケース（分類の宣言の順）。二つ以上なら、対象は絞り込まれない
		{"1 : 4", 0, []arm{{[]string{"A"}, false, []string{":a.K.A<a.M>"}}, {[]string{"B", "C", "D", "E"}, true, nil}}},
		// 名前で並べたアームの cases は書かれた順。else は、並べなかった B と E
		{"名前のアームが二つ", 1, []arm{{[]string{"D", "A"}, false, []string{":a.K.(D, A)<a.M>"}}, {[]string{"C"}, false, []string{":a.K.C<a.M>"}}, {[]string{"B", "E"}, true, nil}}},
		// 受けるケースがちょうど一つの else のアームは、そのケースを名前で書いたアームと同じ（型を書かないケースは case(C, [K], X)）
		{"else が一つのケース", 2, []arm{{[]string{"B", "C", "D", "E"}, false, []string{":a.K.(B, C, D, E)<a.M>"}}, {[]string{"A"}, true, []string{":a.K.A<a.M>"}}}},
		// 型を書いたケース一つを受ける else のアーム: 書いた型
		{"else が as のケース", 4, []arm{{[]string{"NotFound"}, false, nil}, {[]string{"Found"}, true, []string{":a.M"}}}},
		// ケースが一つの分類: アームは一つ
		{"ケースが一つの分類", 5, []arm{{[]string{"Arrived"}, false, []string{":a.M"}}}},
		// variant: 二つのケースを受ける else のアームは外のまま、一つなら、そのアームの型
		{"variant、else が二つ", 6, []arm{{[]string{"IsReady"}, false, []string{":a.M"}}, {[]string{"IsMissing", "IsBroken"}, true, nil}}},
		{"variant、else が一つ", 7, []arm{{[]string{"IsReady", "IsMissing"}, false, []string{":a.Whole.(IsReady, IsMissing)<a.Three>"}}, {[]string{"IsBroken"}, true, []string{":a.O"}}}},
		// 標準の分類: ケースは、ない側、ある側の順（nullable は None、Some。list は Empty、HasItems）。Some だけを受ける
		// by nullable の else のアームは T
		{"by nullable、else が None", 8, []arm{{[]string{"Some"}, false, []string{":string"}}, {[]string{"None"}, true, nil}}},
		{"by nullable、else が Some", 9, []arm{{[]string{"None"}, false, nil}, {[]string{"Some"}, true, []string{":a.M"}}}},
		{"by list", 10, []arm{{[]string{"HasItems"}, false, nil}, {[]string{"Empty"}, true, nil}}},
		// 状態の分類: narrowed は state を持つ
		{"状態の分類", 11, []arm{{[]string{"LoggedOut"}, false, nil}, {[]string{"LoggedIn"}, true, []string{"a.token:string"}}}},
	}
	for _, tt := range tests {
		if got := armsOf(tt.stmt); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: アーム = %+v、期待は %+v", tt.name, got, tt.want)
		}
	}
}
