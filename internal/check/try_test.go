package check

import (
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/sema"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/ir"
)

// compileText は、仕様の文字列を、解析して、検査する。構文と意味の診断があれば、失敗。戻り値は、検査の診断。
func compileText(t *testing.T, src string, nss ...string) (*ir.Program, []diag.Diagnostic) {
	t.Helper()
	fs := source.NewFileSet()
	f, ds := syntax.Parse(fs, fs.Add("try.hozo", []byte(src)))
	res := sema.Analyze(fs, []*ast.File{f})
	ds = append(ds, res.Diags...)
	for _, d := range ds {
		if d.Severity == diag.Error {
			t.Fatalf("解析の診断: %s[%s]: %s", d.Severity, d.Code, d.Message)
		}
	}
	return res.Program, Check(res.Program, cfgFor(nss...))
}

// trySpec は、例外を受ける文の計算（docs/check.md「Sig(O, v)」の「例外を受ける文」）を確かめる仕様。
const trySpec = `
namespace a {
  model M { id: string; }
  state rows: list<M>;
  state notes: list<M>;
  exception X;
  exception Y;
  exception Z;
  classify Failure of any exception {
    Soft: any exception as X;
    Hard: any exception as else;
  }
  effect fn save(m: M): effects rows.post;
  effect fn note(m: M): effects notes.post;
  fn send(m: M):
    throws X, Y;
  fn load(m: M): M
    throws X;
  fn fallback(m: M): M;
  fn again(m: M):
    throws X;

  // 受けて、何もしない: 例外は、なくなる。try の中の効果は、成功のアームで unknown
  fn swallow(m: M) {
    try {
      save(m);
      send(m);
    } catch (e) {
    }
  }
  // 受けて、効果を行い、投げ直す: 例外は、そのまま。try の中の効果は、例外のとき unknown。catch の中の効果は、例外のとき kept。
  // 成功のアームでは、try の中の効果は kept（catch が「返らない」ので、try は、終わりまで動いている）、catch の中の効果は prevented
  fn rethrow(m: M) {
    try {
      save(m);
      send(m);
    } catch (e) {
      note(m);
      throw e;
    }
  }
  // 別の例外に替える
  fn replace(m: M) {
    try {
      send(m);
    } catch (e) {
      throw Z;
    }
  }
  // 分類して受ける: Soft（X）は、効果を行って終わり、Hard（残り: Y）は、投げ直す
  fn classified(m: M) {
    try {
      send(m);
    } catch (e by Failure) {
      Soft => note(m);
      Hard => throw e;
    }
  }
  // try の後ろの文の例外: catch が「返らない」でないので、try の中の効果は unknown。catch が「返らない」なら kept
  fn after(m: M) {
    try {
      save(m);
    } catch (e) {
    }
    send(m);
  }
  fn afterNever(m: M) {
    try {
      save(m);
      send(m);
    } catch (e) {
      note(m);
      throw e;
    }
    again(m);
  }
  // try が入れ、catch も入れた val
  fn either(m: M): M {
    val x;
    try {
      x = load(m);
    } catch (e) {
      x = fallback(m);
    }
    return x;
  }
}
`

func TestSigTry(t *testing.T) {
	p, diags := compileText(t, trySpec, "a")
	wantCodes(t, diags)
	save, note := ir.NamedEffect{Effect: "a.save"}, ir.NamedEffect{Effect: "a.note"}

	sw := sigOf(t, p, "a.swallow")
	wantStrings(t, "swallow.E", sw.Exceptions, nil)
	if got := succOf(t, sw, save); got != ir.Unknown {
		t.Errorf("swallow.succ(save) = %s（期待は unknown: 受けて、成功で終わる道では、起きたか、決まらない）", got)
	}

	re := sigOf(t, p, "a.rethrow")
	wantStrings(t, "rethrow.E", re.Exceptions, []string{"a.X", "a.Y"})
	for _, x := range []string{"a.X", "a.Y"} {
		if got := relOf(t, re, x, save); got != ir.Unknown {
			t.Errorf("rethrow.rel(%s, save) = %s（期待は unknown: try の中の効果）", x, got)
		}
		if got := relOf(t, re, x, note); got != ir.Kept {
			t.Errorf("rethrow.rel(%s, note) = %s（期待は kept: throw e の前に、必ず動く）", x, got)
		}
	}
	if got := succOf(t, re, save); got != ir.Kept {
		t.Errorf("rethrow.succ(save) = %s（期待は kept: catch が「返らない」）", got)
	}
	if got := succOf(t, re, note); got != ir.Prevented {
		t.Errorf("rethrow.succ(note) = %s（期待は prevented: 成功したなら、catch は、動いていない）", got)
	}

	wantStrings(t, "replace.E", sigOf(t, p, "a.replace").Exceptions, []string{"a.Z"})

	cl := sigOf(t, p, "a.classified")
	wantStrings(t, "classified.E", cl.Exceptions, []string{"a.Y"})
	if got := succOf(t, cl, note); got != ir.Unknown {
		t.Errorf("classified.succ(note) = %s（期待は unknown）", got)
	}

	af := sigOf(t, p, "a.after")
	wantStrings(t, "after.E", af.Exceptions, []string{"a.X", "a.Y"})
	if got := relOf(t, af, "a.X", save); got != ir.Unknown {
		t.Errorf("after.rel(X, save) = %s（期待は unknown: catch が、受けて、続けたかもしれない）", got)
	}
	an := sigOf(t, p, "a.afterNever")
	wantStrings(t, "afterNever.E", an.Exceptions, []string{"a.X", "a.Y"})
	if got := relOf(t, an, "a.Y", save); got != ir.Unknown {
		t.Errorf("afterNever.rel(Y, save) = %s（期待は unknown: Y の出どころは、throw e だけ）", got)
	}
	if got := relOf(t, an, "a.X", note); got != ir.Unknown {
		t.Errorf("afterNever.rel(X, note) = %s（期待は unknown: throw e では kept、後ろの again では、catch は動いていない）", got)
	}

	wantStrings(t, "either.E", sigOf(t, p, "a.either").Exceptions, nil)
}

// TestTryIR は、例外を受ける文の IR（docs/ir.md「配線」の Catch、Throw、subject kind "caught"）。
func TestTryIR(t *testing.T) {
	p, _ := compileText(t, trySpec, "a")
	body := func(fqn string) ir.TryStmt {
		o, _ := p.Lookup(fqn).(*ir.Op)
		if o == nil || o.Body == nil {
			t.Fatalf("%s がない", fqn)
		}
		for _, st := range o.Body.Statements {
			if try, ok := st.(ir.TryStmt); ok {
				return try
			}
		}
		t.Fatalf("%s に try がない", fqn)
		return ir.TryStmt{}
	}
	if try := body("a.swallow"); try.Catch.Name != "e" || try.Catch.Never || len(try.Catch.Body) != 0 || len(try.Body) != 2 {
		t.Errorf("swallow の try = %+v", try)
	}
	re := body("a.rethrow")
	if !re.Catch.Never || len(re.Catch.Body) != 2 {
		t.Fatalf("rethrow の catch = %+v", re.Catch)
	}
	if th, ok := re.Catch.Body[1].(ir.ThrowStmt); !ok || th.Throw.Caught != "e" || th.Throw.Exception != "" {
		t.Errorf("rethrow の最後の文 = %+v（期待は throw e）", re.Catch.Body[1])
	}
	if th, ok := body("a.replace").Catch.Body[0].(ir.ThrowStmt); !ok || th.Throw.Exception != "a.Z" || th.Throw.Caught != "" {
		t.Errorf("replace の catch の文が、throw a.Z でない")
	}
	// catch (e by C) { … } は、match (e by C) { … } の文一つ
	cl := body("a.classified")
	if cl.Catch.Never || len(cl.Catch.Body) != 1 {
		t.Fatalf("classified の catch = %+v", cl.Catch)
	}
	ms, ok := cl.Catch.Body[0].(ir.MatchStmt)
	if !ok {
		t.Fatalf("classified の catch の文が match でない: %+v", cl.Catch.Body[0])
	}
	if s, isCaught := ms.Match.Subject.(ir.CaughtSubject); !isCaught || s.Name != "e" || ms.Match.Classify != "a.Failure" || len(ms.Match.Arms) != 2 {
		t.Errorf("classified の match = %+v", ms.Match)
	}
	if th, isThrow := ms.Match.Arms[1].Body.(ir.ThrowBody); !isThrow || th.Caught != "e" {
		t.Errorf("Hard のアームが throw e でない: %+v", ms.Match.Arms[1].Body)
	}
	// 例外の分類
	c, _ := p.Lookup("a.Failure").(*ir.Classify)
	if c == nil || !c.IsExceptionClass() || c.DecidedBy != ir.DecidedByType || len(c.Cases) != 2 {
		t.Fatalf("a.Failure = %+v", c)
	}
	wantStrings(t, "Soft.exceptions", c.Cases[0].Exceptions, []string{"a.X"})
	if !c.Cases[1].Rest || len(c.Cases[1].Exceptions) != 0 {
		t.Errorf("Hard = %+v（期待は、残りのケース）", c.Cases[1])
	}
	// JSON を、読み直せる
	data, err := p.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var back ir.Program
	if err := back.UnmarshalJSON(data); err != nil {
		t.Fatalf("読み直せない: %v", err)
	}
	again, err := back.MarshalJSON()
	if err != nil || string(again) != string(data) {
		t.Errorf("読み直した IR が、同じ JSON にならない（%v）", err)
	}
}
