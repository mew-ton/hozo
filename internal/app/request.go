package app

import (
	"context"

	hozocli "github.com/mew-ton/hozo/internal/gen/hozo/cli"
)

// requestCases は、分類 hozo.cli.Request（build が、何を頼まれたか）の述語（docs/cli.md「build」の表）。コマンドの行の op build が、
// これで、入るユースケースを分ける。フラグを読むのは、ここ（と、表示の [Presenter]）だけで、ユースケースの側は、フラグを知らない。
//
//   - CheckOnly: --no-emit が付いている（--dry-run が一緒でも、こちら）。
//   - Preview: --no-emit がなく、--dry-run が付いている。
//   - Write: どちらも付いていない。
//
// どの入力も、ちょうど一つのケースに当たる。
type requestCases struct{}

var _ hozocli.RequestCases = requestCases{}

func (requestCases) CheckOnly(_ context.Context, in hozocli.BuildInput) (bool, error) {
	return isSet(in.NoEmit), nil
}

func (requestCases) Preview(_ context.Context, in hozocli.BuildInput) (bool, error) {
	return !isSet(in.NoEmit) && isSet(in.DryRun), nil
}

func (requestCases) Write(_ context.Context, in hozocli.BuildInput) (bool, error) {
	return !isSet(in.NoEmit) && !isSet(in.DryRun), nil
}

// isSet は、省略可能な真偽のフラグの値。与えられていなければ、偽。
func isSet(p *bool) bool { return p != nil && *p }
