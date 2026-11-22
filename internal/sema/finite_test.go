package sema

import (
	"strings"
	"testing"
)

// TestFiniteTypeMessage は TYPE-19（docs/types.md「値を、有限に作れない型」）の、位置と、文面と、Related:
// 互いに辿り合える宣言のまとまりごとに一つ。位置は、その中で最初に現れる宣言。文面は、自分に戻る、いちばん短い道。
func TestFiniteTypeMessage(t *testing.T) {
	res, fs := analyzeProject(t, project{"a.hozo": `namespace a {
  model Pair { either: Either; }
  union Either {
    Left: Pair;
    Right: { pair: Pair; note: string?; };
  }
}
`}, false)
	var got []string
	for _, d := range res.Diags {
		if d.Code != "TYPE-19" {
			t.Errorf("想定しない診断: %s %s", d.Code, d.Message)
			continue
		}
		pos := fs.Position(d.Span.File, d.Span.Start)
		if pos.Line != 2 {
			t.Errorf("位置 = %d 行目（期待は、最初に現れる宣言 Pair の 2 行目）", pos.Line)
		}
		got = append(got, d.Message)
		if !strings.Contains(d.Message, "a.Pair.either → a.Either.Left → a.Pair と辿って、自分に戻る") {
			t.Errorf("文面に、いちばん短い道がない: %s", d.Message)
		}
		var rel []string
		for _, r := range d.Related {
			rel = append(rel, r.Message)
		}
		if want := "輪の上の宣言: a.Pair|輪の上の宣言: a.Either"; strings.Join(rel, "|") != want {
			t.Errorf("Related = %q（期待は %q）", strings.Join(rel, "|"), want)
		}
	}
	if len(got) != 1 {
		t.Fatalf("TYPE-19 が %d 件（期待は、まとまりごとに一つ）: %v", len(got), got)
	}
}
