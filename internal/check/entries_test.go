package check

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは、入口ごとの、求めるケース（docs/check.md「入口ごとの、求めるケース」）: 入口から辿れる、例外の出どころと、
// そこへ、差し替えで入らせる方法と、受ける道。仕様の文字列から、手で書いた期待と比べる。出どころの並びは、入口の、計算した例外の順
// （Op.signature.exceptions）で、その中は、辿った順。

// entryLines は、仕様 src の、名前空間 viewer の入口 entry の、求めるケースを、一行ずつの文字列にする:
// `<例外> <種類> <出どころ> made|human <差し替えに言うこと>`。受ける道は、頭に `caught `。units は、単位を置く名前空間
// （同じ名前を二度書けば、単位が二つ）。
func entryLines(t *testing.T, src, entry string, units ...string) []string {
	t.Helper()
	p, _ := compileText(t, src, units...)
	viewer := entry[:strings.LastIndex(entry, ".")]
	v := viewsOf(p, cfgFor(units...))[viewer]
	if v == nil {
		t.Fatalf("名前空間 %s の View がない", viewer)
	}
	e := v.Entry(entry)
	if e == nil {
		t.Fatalf("入口 %s が、View.entries にない（entries = %+v）", entry, v.Entries)
	}
	if e.Blocked != "" {
		return []string{"blocked " + e.Blocked}
	}
	line := func(s ir.EntrySource) string {
		out := s.Exception + " " + string(s.Kind) + " " + s.Label
		if s.Why != "" {
			return out + " human"
		}
		out += " made"
		for _, st := range s.Steps {
			at := string(st.Kind) + " " + st.Name
			if st.Decorator != "" {
				at += "@" + st.Decorator
			}
			if len(st.Via) > 0 {
				at = strings.Join(st.Via, ">") + ">" + at
			}
			switch st.Verb {
			case ir.StepThrows:
				out += " [" + at + " throws " + st.Exception + "]"
			case ir.StepNone:
				out += " [" + at + " none]"
			case ir.StepIs:
				out += " [" + at + " is " + st.Case + "]"
			}
		}
		return out
	}
	var out []string
	for _, s := range e.Sources {
		out = append(out, line(s))
	}
	for _, s := range e.Caught {
		out = append(out, "caught "+line(s))
	}
	return out
}

func wantLines(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("%s =\n  %s\nwant\n  %s", what, strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

const entrySpec = `
namespace app {
  exception Rejected;
  exception Taken;
  exception Shut;
  exception Bad;
  exception Missing;
  exception Boom;
  exception Caught;
  exception Failed;
  exception Broken;
  exception LimitExceeded;

  state items: list<string>;
  state door: nullable<string>;

  effect fn addItem(v: string): throws Taken effects items.post;
  read fn itemById(): string? reads items;

  /** 述語で決まる、値の分類 */
  classify Size of string { Big: string as string; Small: string as string; }
  /** 述語で決まる、状態の分類 */
  classify Door of door { Opened; Closed; }

  @entry decorator fn route();
  /** 本体のないデコレータ: 包むことで、投げる */
  decorator fn limited(): throws LimitExceeded prevents effects;
  /** 本体のないデコレータ: 受ける */
  decorator fn swallow(): catches Caught ignores;
  /** 本体のあるデコレータ */
  decorator fn guarded() {
    match (by Door) {
      Opened => proceed();
      Closed => throw Shut;
    }
  }
  decorator fn noted() {
    proceed();
    note();
  }

  fn leaf();
  fn check(v: string): throws Bad;
  fn fail(): never throws Boom;
  fn risky(): throws Bad, Caught;
  fn note(): throws Failed;
  fn work(): throws Broken;

  @swallow
  fn inner() { risky(); }
  fn other() { risky(); }

  // 述語で決まる分類の、throw のアーム。throw でないアームが、一つだけなら、その中の出どころに、何もしなくても、届く
  @route
  export fn add(v: string) {
    match (v by Size) {
      Small => addItem(v);
      Big   => throw Rejected;
    }
  }

  // 状態の分類の、throw のアーム
  @route
  export fn byDoor() {
    match (by Door) {
      Opened => leaf();
      Closed => throw Shut;
    }
  }

  // 値がないことで入るアーム: 値の出どころ（read）が、「ない」を返す
  @route
  export fn byValue() {
    val v = itemById();
    match (v by nullable) {
      Some => check(v);
      None => throw Missing;
    }
  }

  // 値が要るアームで、throw でないアームが、ほかにもある: その中の出どころには、入らせられない
  @route
  export fn twoArms() {
    val v = itemById();
    match (v by nullable) {
      Some => check(v);
      None => leaf();
    }
  }

  // 入口の入力で決まるアーム
  @route
  export fn byInput(v: string?) {
    match (v by nullable) {
      Some => check(v);
      None => throw Missing;
    }
  }

  // 繰り返しの中
  @route
  export fn looped(vs: string[]) {
    for (val v of vs) {
      check(v);
    }
  }

  // never を返す op。途中で受けられる例外。同じ葉を、二か所から呼ぶ
  @route
  export fn run() {
    inner();
    inner();
    fail();
  }
  @route
  export fn both() {
    inner();
    other();
  }

  // 本体のないデコレータの throws と、本体のあるデコレータ
  @route
  @limited
  export fn limitedEntry() { leaf(); }
  @route
  @guarded
  @noted
  export fn wrapped() { work(); }
}
`

func TestEntrySources(t *testing.T) {
	lines := func(entry string) []string { return entryLines(t, entrySpec, "app."+entry, "app") }
	wantLines(t, "add", lines("add"),
		"app.Taken effect app.addItem made [effect app.addItem throws app.Taken]",
		"app.Rejected arm app.add.Big made [classify app.Size is Big]")
	wantLines(t, "byDoor", lines("byDoor"),
		"app.Shut arm app.byDoor.Closed made [classify app.Door is Closed]")
	wantLines(t, "byValue", lines("byValue"),
		"app.Bad leaf app.check made [leaf app.check throws app.Bad]",
		"app.Missing arm app.byValue.None made [read app.itemById none]")
	wantLines(t, "twoArms", lines("twoArms"),
		"app.Bad leaf app.check human")
	wantLines(t, "byInput", lines("byInput"),
		"app.Bad leaf app.check made [leaf app.check throws app.Bad]",
		"app.Missing arm app.byInput.None human")
	wantLines(t, "looped", lines("looped"),
		"app.Bad leaf app.check human")
	// run: 葉 risky は、inner を通して、二度呼ばれるが、出どころは、一つ。Caught は、inner のデコレータが受けるので、数えない
	wantLines(t, "run", lines("run"),
		"app.Bad leaf app.risky made [leaf app.risky throws app.Bad]",
		"app.Boom never app.fail made")
	// both: Caught は、other を通る道でだけ、入口まで伝わる
	wantLines(t, "both", lines("both"),
		"app.Bad leaf app.risky made [leaf app.risky throws app.Bad]",
		"app.Caught leaf app.risky made [leaf app.risky throws app.Caught]")
	wantLines(t, "limitedEntry", lines("limitedEntry"),
		"app.LimitExceeded decorator app.limitedEntry@app.limited made [decorator app.limitedEntry@app.limited throws app.LimitExceeded]")
	// 本体の throw のアーム、proceed() の先の葉、proceed() の後ろで、本体が呼ぶ葉
	wantLines(t, "wrapped", lines("wrapped"),
		"app.Broken leaf app.work made [leaf app.work throws app.Broken]",
		"app.Failed leaf app.note made [leaf app.note throws app.Failed]",
		"app.Shut arm app.wrapped@app.guarded.Closed made [classify app.Door is Closed]")
}

const entryTrySpec = `
namespace app {
  exception Refused;
  exception Broken;
  exception Other;
  classify Failure of any exception {
    Soft: any exception as Refused;
    Hard: any exception as else;
  }
  @entry decorator fn route();
  /** 本体のあるデコレータが、受ける */
  decorator fn quiet() {
    try { proceed(); } catch (e) { }
  }
  fn send(): throws Refused, Broken;
  fn note(): throws Other;

  @route export fn swallow() { try { send(); } catch (e) { } }
  @route export fn noteAndRethrow() { try { send(); } catch (e) { note(); throw e; } }
  @route export fn classified() {
    try { send(); } catch (e by Failure) {
      Soft => note();
      Hard => throw e;
    }
  }
  @route export fn replace() { try { send(); } catch (e) { throw Other; } }
  @route export fn replaceArm() {
    try { send(); } catch (e by Failure) {
      Soft => throw Other;
      Hard => throw e;
    }
  }
  @route export fn outside() {
    try { note(); } catch (e) { }
    note();
  }
  @route @quiet export fn hushed() { send(); }
}
`

// 例外を受ける文: try のブロックの中の出どころは、catch が受けるので、そのままでは、数えない。throw e は、try の中の出どころを、
// そのまま、出どころにする。投げずに終わる道は、受ける道。catch の中の出どころには、try の中の、差し替えで起こせる、最初の出どころに、
// 投げさせて、入る。
func TestEntryTry(t *testing.T) {
	lines := func(entry string) []string { return entryLines(t, entryTrySpec, "app."+entry, "app") }
	wantLines(t, "swallow", lines("swallow"),
		"caught app.Refused leaf app.send made [leaf app.send throws app.Refused]",
		"caught app.Broken leaf app.send made [leaf app.send throws app.Broken]")
	wantLines(t, "noteAndRethrow", lines("noteAndRethrow"),
		"app.Other leaf app.note made [leaf app.send throws app.Refused] [leaf app.note throws app.Other]",
		"app.Refused leaf app.send made [leaf app.send throws app.Refused]",
		"app.Broken leaf app.send made [leaf app.send throws app.Broken]")
	// Soft（Refused）は、受けて終わり、Hard（残り: Broken）だけを、投げ直す
	wantLines(t, "classified", lines("classified"),
		"app.Other leaf app.note made [leaf app.send throws app.Refused] [leaf app.note throws app.Other]",
		"app.Broken leaf app.send made [leaf app.send throws app.Broken]",
		"caught app.Refused leaf app.send made [leaf app.send throws app.Refused]")
	wantLines(t, "replace", lines("replace"),
		"app.Other arm app.replace.catch made [leaf app.send throws app.Refused]")
	wantLines(t, "replaceArm", lines("replaceArm"),
		"app.Other arm app.replaceArm.catch.Soft made [leaf app.send throws app.Refused]",
		"app.Broken leaf app.send made [leaf app.send throws app.Broken]")
	// 同じ出どころが、受ける文の外でも呼ばれる: 受ける道は、差し替えでは、作れない（差し替えは、どの呼び出しにも、効く）
	wantLines(t, "outside", lines("outside"),
		"app.Other leaf app.note made [leaf app.note throws app.Other]",
		"caught app.Other leaf app.note human")
	wantLines(t, "hushed", lines("hushed"),
		"caught app.Refused leaf app.send made [leaf app.send throws app.Refused]",
		"caught app.Broken leaf app.send made [leaf app.send throws app.Broken]")
}

const entryEdgeSpec = `
namespace gate {
  exception Net;
  @entry decorator fn remote(): outer throws Net;
}
namespace ext {
  exception Down;
  @gate.remote
  export fn charge(): throws Down;
  export fn peek(): string?;
}
namespace store {
  exception Full;
  export fn save() { put(); }
  fn put(): throws Full;
}
namespace api {
  exception Late;
  @gate.remote
  export fn ping(): throws Late;
}
namespace app {
  exception Empty;
  @gate.remote
  export fn run() {
    ext.charge();
    store.save();
    api.ping();
  }
  @gate.remote
  export fn look() {
    val v = ext.peek();
    match (v by nullable) {
      Some => store.save();
      None => throw Empty;
    }
  }
}
`

// 流れの端と、境界: 通す出力のない名前空間の op と、入口のデコレータを持つ op（相手の入口）は、流れの端。その op の境界
// （デコレータの outer throws）も、出どころ。通す出力の中の差し替えは、通った名前空間（via）を持つ。
func TestEntryEdges(t *testing.T) {
	wantLines(t, "run", entryLines(t, entryEdgeSpec, "app.run", "app", "store", "api"),
		"ext.Down external ext.charge made [op ext.charge throws ext.Down]",
		"gate.Net boundary ext.charge@gate.remote made [op ext.charge throws gate.Net]",
		"gate.Net boundary api.ping@gate.remote made [op api.ping throws gate.Net]",
		"store.Full leaf store.put made [store>leaf store.put throws store.Full]",
		"api.Late external api.ping made [op api.ping throws api.Late]")
	wantLines(t, "look", entryLines(t, entryEdgeSpec, "app.look", "app", "store", "api"),
		"store.Full leaf store.put made [store>leaf store.put throws store.Full]",
		"app.Empty arm app.look.None made [op ext.peek none]")
	// 通す先の名前空間を出力する、同じ基底の単位が、二つ以上ある: 辿れない
	got := entryLines(t, entryEdgeSpec, "app.run", "app", "store", "store", "api")
	if len(got) != 1 || !strings.HasPrefix(got[0], "blocked ") || !strings.Contains(got[0], "store") {
		t.Errorf("単位が二つあるとき = %q（期待は、blocked と、名前空間 store）", got)
	}
}

// 入口は、単位の対象の op のうち、入口のデコレータ（@entry）を持つもの（op の宣言順）。正常の結果（normal）は、never を返す op には、ない。
func TestEntryList(t *testing.T) {
	p, _ := compileText(t, `
namespace app {
  exception Boom;
  @entry decorator fn route();
  decorator fn plain();

  @route
  export fn first(): string;
  @plain
  export fn notEntry(): string;
  export fn bare(): string;
  @route
  export fn halt(): never throws Boom;
}
`, "app")
	v := viewsOf(p, cfgFor("app"))["app"]
	if v == nil {
		t.Fatal("名前空間 app の View がない")
	}
	var got []string
	for _, e := range v.Entries {
		got = append(got, fmt.Sprintf("%s normal=%t sources=%d", e.Op, e.Normal, len(e.Sources)))
	}
	wantLines(t, "View.entries", got,
		"app.first normal=true sources=0",
		"app.halt normal=false sources=1")
}
