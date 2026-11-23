package gobase

import (
	"strings"
	"testing"
)

// TestLiteralMatchBodies は docs/plugin-go.md「match」の、絞り込みが literal の分け方（`if <対象>`。真の側が if の枝）: testdata/matchliteral の
// op の本体が、文書から、手で書いた、下のコードと同じこと。ゴールデン（expected/）は、ファイルの全体を比べる。ここは、分け方だけを、
// 実装より先に書いた期待と比べる。
func TestLiteralMatchBodies(t *testing.T) {
	_, files := emitFixture(t, "matchliteral")
	wiring, ok := files["gen/app/wiring.go"]
	if !ok {
		t.Fatalf("gen/app/wiring.go がない: %v", fileNames(files))
	}
	for _, tt := range []struct{ method, want string }{
		// return match: アームを書いた順（Write、Dry）によらず、真の側が if の枝。else を書かず、偽の側を後ろに続ける
		{"Run", `	// match (opts.dryRun by app.Emit)
	if opts.DryRun {
		// Dry: opts.dryRun は true
		return o.Plan(ctx, opts)
	}
	// Write: opts.dryRun は false
	return o.Write(ctx, opts)`},
		// val match: 偽の側は、残りのケース（対象の型は、もとのまま）。else のアームは、ケースを名前で書いたアームと同じコード
		{"Label", `	// match (opts.verbose by app.Talk)
	var text string
	if opts.Verbose {
		// Loud: opts.verbose は true
		text = "loud"
	} else {
		// Quiet
		text = "quiet"
	}
	return text, nil`},
		// 文の match: 偽の側にケースがないので、else の枝が、分類の誤りの枝
		{"Check", `	// match (opts.dryRun by app.Must)
	if opts.DryRun {
		// Yes: opts.dryRun は true
		if err := o.Show(ctx, "dry"); err != nil {
			return err
		}
	} else {
		return &runtime.ClassificationError{Classify: "app.Must"}
	}
	return nil`},
		// 文の match: 真の側にケースがないので、if の枝が、分類の誤りの枝
		{"CheckNot", `	// match (opts.dryRun by app.MustNot)
	if opts.DryRun {
		return &runtime.ClassificationError{Classify: "app.MustNot"}
	} else {
		// No: opts.dryRun は false
		if err := o.Show(ctx, "wet"); err != nil {
			return err
		}
	}
	return nil`},
		// 一つのアームが両方のケースを受けていれば、分けない
		{"Both", `	// match (opts.dryRun by app.Emit)
	return o.Plan(ctx, opts)`},
	} {
		if got := methodBody(t, wiring, tt.method); got != tt.want {
			t.Errorf("%s の本体が、文書から書いた期待と違う\n--- got\n%s\n--- want\n%s", tt.method, got, tt.want)
		}
	}
}

// TestLiteralPredicateGuard は docs/plugin-go.md「分類」の「型で決まる絞り込みの as を持つケースは、値がその型に当たらなければ、
// 述語を呼ばずに偽とする」の literal: 判定関数の中で、as true のケースの述語は `if v`、as false のケースの述語は `if !v` で包む。
// 期待は、文書から、手で書いた。
func TestLiteralPredicateGuard(t *testing.T) {
	_, files := emitFixture(t, "matchliteral")
	src, ok := files["gen/app/types/classify.go"]
	if !ok {
		t.Fatalf("gen/app/types/classify.go がない: %v", fileNames(files))
	}
	guarded := func(cond, name string) string {
		return "\tif " + cond + " {\n\t\tif ok, err := c." + name + "(ctx, v); err != nil {\n\t\t\tif first == nil {\n\t\t\t\tfirst = err\n\t\t\t}\n" +
			"\t\t} else if ok {\n\t\t\thits = append(hits, \"" + name + "\")\n\t\t\tkind = Speed" + name + "\n\t\t}\n\t}\n"
	}
	want := "func ClassifySpeed(ctx context.Context, c SpeedCases, v bool) (Speed, error) {\n\tvar hits []string\n\tvar kind Speed\n\tvar first error\n" +
		guarded("v", "Fast") + guarded("v", "Careful") + guarded("!v", "Off")
	if !strings.Contains(src, want) {
		t.Errorf("ClassifySpeed の頭が、文書から書いた期待と違う\n--- want\n%s\n--- got\n%s", want, src)
	}
}
