package app

import (
	"context"
	"testing"

	hozocli "github.com/mew-ton/hozo/internal/gen/hozo/cli"
	compiler "github.com/mew-ton/hozo/internal/gen/hozo/compiler"
)

// usecaseSpy は、ユースケース（名前空間 hozo.compiler）の代わり。呼ばれたユースケースの名前を、順に覚える。
type usecaseSpy struct{ called []string }

func (s *usecaseSpy) Verify(context.Context, compiler.EmitInput) (compiler.BuildResult, error) {
	s.called = append(s.called, "verify")
	return compiler.BuildResult{}, nil
}

func (s *usecaseSpy) Preview(context.Context, compiler.EmitInput) (compiler.BuildResult, error) {
	s.called = append(s.called, "preview")
	return compiler.BuildResult{}, nil
}

func (s *usecaseSpy) Write(context.Context, compiler.EmitInput) (compiler.BuildResult, error) {
	s.called = append(s.called, "write")
	return compiler.BuildResult{}, nil
}

func (s *usecaseSpy) ConfirmTests(context.Context, compiler.CoverageInput) (compiler.TestsResult, error) {
	s.called = append(s.called, "confirmTests")
	return compiler.TestsResult{}, nil
}

func (s *usecaseSpy) Serve(context.Context, compiler.ServeInput) error {
	s.called = append(s.called, "serve")
	return nil
}

// TestBuildFlags は、コマンドの行の build が、フラグから、入るユースケースを分けること（docs/cli.md「build」の表）: フラグの組の
// すべてで、ちょうど一つのユースケースが呼ばれ、--no-emit と --dry-run の両方なら、検査だけ（verify）。分け方は、spec/cli.hozo の
// 配線（分類 hozo.cli.Flag の match。build と、内部の op writeOrPreview）が言い、生成した本体が、そのまま行う: 人が書く述語は、ない
// （前は、述語で決まる分類 Request と、その述語 requestCases を、ここで確かめていた）。
func TestBuildFlags(t *testing.T) {
	cases := []struct {
		noEmit, dryRun bool
		want           string
	}{
		{false, false, "write"},
		{false, true, "preview"},
		{true, false, "verify"},
		{true, true, "verify"},
	}
	for _, c := range cases {
		spy := &usecaseSpy{}
		if _, err := hozocli.New(spy).Build(context.Background(), hozocli.BuildInput{NoEmit: c.noEmit, DryRun: c.dryRun}); err != nil {
			t.Fatalf("noEmit=%v dryRun=%v: %v", c.noEmit, c.dryRun, err)
		}
		if len(spy.called) != 1 || spy.called[0] != c.want {
			t.Errorf("noEmit=%v dryRun=%v: 呼ばれたユースケース = %v, want [%s]", c.noEmit, c.dryRun, spy.called, c.want)
		}
	}
}
