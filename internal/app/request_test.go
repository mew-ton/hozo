package app

import (
	"context"
	"testing"

	hozocli "github.com/mew-ton/hozo/internal/gen/hozo/cli"
	"github.com/mew-ton/hozo/internal/gen/hozo/cli/clitest"
)

// requestExamples は、分類 hozo.cli.Request の、ケースごとの例（生成した口 clitest.RequestExamples）。
type requestExamples struct{}

func (requestExamples) CheckOnly() hozocli.BuildInput {
	yes := true
	return hozocli.BuildInput{NoEmit: &yes}
}

func (requestExamples) Preview() hozocli.BuildInput {
	yes := true
	return hozocli.BuildInput{DryRun: &yes}
}

func (requestExamples) Write() hozocli.BuildInput { return hozocli.BuildInput{} }

// TestRequest は、build が、フラグから、何を頼まれたかを分ける述語（[requestCases]。docs/cli.md「build」の表）: 生成した口の
// ケースごとの例と、フラグの組のすべて（与えられていない、偽、真）。どの組も、ちょうど一つのケースに当たり、--no-emit と
// --dry-run の両方なら、検査だけ（CheckOnly）。
func TestRequest(t *testing.T) {
	clitest.RunRequest(t, requestCases{}, requestExamples{})

	yes, no := true, false
	flags := []*bool{nil, &no, &yes}
	for _, noEmit := range flags {
		for _, dryRun := range flags {
			want := hozocli.RequestWrite
			switch {
			case noEmit != nil && *noEmit:
				want = hozocli.RequestCheckOnly
			case dryRun != nil && *dryRun:
				want = hozocli.RequestPreview
			}
			got, err := hozocli.ClassifyRequest(context.Background(), requestCases{}, hozocli.BuildInput{NoEmit: noEmit, DryRun: dryRun})
			if err != nil || got != want {
				t.Errorf("noEmit=%v dryRun=%v: Request = %v, %v, want %v", deref(noEmit), deref(dryRun), got, err, want)
			}
		}
	}
}

// deref は、省略可能な真偽の値の、表示のための形。
func deref(p *bool) string {
	switch {
	case p == nil:
		return "なし"
	case *p:
		return "真"
	}
	return "偽"
}
