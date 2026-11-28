package gobase

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// catchFaults は、宣言に反する、継ぎ目の実装（入力 wraps の名前空間 desk）。testdata/wraps/behavior_test.go と同じパッケージに置く。
const catchFaults = `package behavior

import (
	"context"
	"errors"
	"testing"

	"example.com/app/gen/desk/desktest"
	"example.com/app/gen/trace"
)

// leaky は、受けるはずの例外を、そのまま返す（中身を呼んで、結果を、そのまま返すだけ）。
type leaky struct{}

func (leaky) PingTraceQuiet(ctx context.Context, body func(ctx context.Context) error) error {
	return body(ctx)
}
func (leaky) FetchTraceSteady(ctx context.Context, id string, body func(ctx context.Context, id string) (string, error)) (string, error) {
	return body(ctx, id)
}
func (leaky) LoadTraceRenamed(ctx context.Context, id string, body func(ctx context.Context, id string) (string, error)) (string, error) {
	return body(ctx, id)
}
func (leaky) DialTraceStopped(ctx context.Context, body func(ctx context.Context) error) error {
	return body(ctx)
}
func (leaky) IdleTraceQuiet(ctx context.Context, body func(ctx context.Context) error) error {
	return body(ctx)
}
func (leaky) RelayTraceSwapped(ctx context.Context, body func(ctx context.Context) (string, error)) (string, error) {
	return body(ctx)
}
func (leaky) EchoTraceEchoed(ctx context.Context, body func(ctx context.Context) error) error {
	return body(ctx)
}
func (leaky) HushTraceMuted(ctx context.Context, body func(ctx context.Context) error) error {
	return body(ctx)
}

// noisy は、例外を受けるが、宣言にない失敗を返す: ignores と、throws のない recovers なのに、別の失敗。
// throws を持つ recovers（trace.renamed）なのに、その throws にない例外（trace.Odd）。二度付けた trace.swapped なのに、どちらの適用が
// 渡した例外でもない失敗。halts（trace.stopped）と、trace.echoed は、宣言どおり。
type noisy struct{ deskSeams }

var errOther = errors.New("other")

func (noisy) PingTraceQuiet(ctx context.Context, body func(ctx context.Context) error) error {
	if err := body(ctx); err != nil {
		return errOther
	}
	return nil
}
func (noisy) FetchTraceSteady(ctx context.Context, id string, body func(ctx context.Context, id string) (string, error)) (string, error) {
	v, err := body(ctx, id)
	if err != nil {
		return "", errOther
	}
	return v, nil
}
func (noisy) LoadTraceRenamed(ctx context.Context, id string, body func(ctx context.Context, id string) (string, error)) (string, error) {
	v, err := body(ctx, id)
	if err != nil {
		return "", trace.Odd{}
	}
	return v, nil
}
func (x noisy) IdleTraceQuiet(ctx context.Context, body func(ctx context.Context) error) error {
	return x.PingTraceQuiet(ctx, body)
}
func (noisy) RelayTraceSwapped(ctx context.Context, body func(ctx context.Context) (string, error)) (string, error) {
	v, err := body(ctx)
	if err != nil {
		return "", errOther
	}
	return v, nil
}

// skipping は、中身を、呼ばない。
type skipping struct{}

func (skipping) PingTraceQuiet(context.Context, func(ctx context.Context) error) error { return nil }
func (skipping) FetchTraceSteady(context.Context, string, func(ctx context.Context, id string) (string, error)) (string, error) {
	return "", nil
}
func (skipping) LoadTraceRenamed(context.Context, string, func(ctx context.Context, id string) (string, error)) (string, error) {
	return "", nil
}
func (skipping) DialTraceStopped(context.Context, func(ctx context.Context) error) error { return nil }
func (skipping) IdleTraceQuiet(context.Context, func(ctx context.Context) error) error   { return nil }
func (skipping) RelayTraceSwapped(context.Context, func(ctx context.Context) (string, error)) (string, error) {
	return "", nil
}
func (skipping) EchoTraceEchoed(context.Context, func(ctx context.Context) error) error { return nil }
func (skipping) HushTraceMuted(context.Context, func(ctx context.Context) error) error  { return nil }

func TestLeaky(t *testing.T)    { desktest.RunSeams(t, deskCases{leaky{}}) }
func TestNoisy(t *testing.T)    { desktest.RunSeams(t, deskCases{noisy{}}) }
func TestSkipping(t *testing.T) { desktest.RunSeams(t, deskCases{skipping{}}) }
`

// catchCasesFail は、継ぎ目の実装が、デコレータの宣言に反すれば、RunSeams が、受ける例外ごとのテストを落とすこと
// （plugins/go/spec/go.hozo の Requirement.UnitTestPort の「デコレータのテストの口」の「受けるケース」の、扱いごとの確かめ）: 受けるはずの例外を、そのまま返す。
// ignores と、throws のない recovers なのに、別の失敗を返す。throws を持つ recovers なのに、その throws にない例外を返す。
// 中身を、呼ばない。宣言どおりのところは、同じ走りの中でも、通る。
func catchCasesFail(t *testing.T) {
	root, files := emitFixture(t, "wraps")
	writeFiles(t, root, files)
	test, err := os.ReadFile(filepath.Join("testdata", "wraps", "behavior_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "behavior_test.go"), string(test))
	writeFile(t, filepath.Join(root, "faults_test.go"), catchFaults)
	out, err := runGo(t, root, "test", "-count=1", "-v", "-run", "^(TestLeaky|TestNoisy|TestSkipping)$", ".")
	if err == nil {
		t.Fatalf("落ちるはずのテストが、通った:\n%s", out)
	}
	fails := []string{
		// 受けるはずの例外を、そのまま返す: 受けるケースが、どれも落ちる
		"TestLeaky/desk.ping@trace.quiet/catches/Down",
		"TestLeaky/desk.fetch@trace.steady/catches/Down", "TestLeaky/desk.fetch@trace.steady/catches/Cut",
		"TestLeaky/desk.load@trace.renamed/catches/Down",
		"TestLeaky/desk.dial@trace.stopped/catches/Cut",
		"TestLeaky/desk.idle@trace.quiet/catches/Down",
		"TestLeaky/desk.relay@trace.swapped/catches/TraceDown", "TestLeaky/desk.relay@trace.swapped/catches/DeskDown",
		"TestLeaky/desk.hush@trace.muted/catches/Down", "TestLeaky/desk.hush@trace.muted/catches/Cut",
		// 宣言にない失敗を返す
		"TestNoisy/desk.ping@trace.quiet/catches/Down",
		"TestNoisy/desk.fetch@trace.steady/catches/Down", "TestNoisy/desk.fetch@trace.steady/catches/Cut",
		"TestNoisy/desk.load@trace.renamed/catches/Down",
		"TestNoisy/desk.idle@trace.quiet/catches/Down",
		"TestNoisy/desk.relay@trace.swapped/catches/TraceDown", "TestNoisy/desk.relay@trace.swapped/catches/DeskDown",
		// 中身を、呼ばない: 正常のケースも、受けるケースも、落ちる
		"TestSkipping/desk.ping@trace.quiet/OK", "TestSkipping/desk.ping@trace.quiet/catches/Down",
		"TestSkipping/desk.fetch@trace.steady/OK", "TestSkipping/desk.fetch@trace.steady/catches/Down", "TestSkipping/desk.fetch@trace.steady/catches/Cut",
		"TestSkipping/desk.load@trace.renamed/OK", "TestSkipping/desk.load@trace.renamed/catches/Down",
		"TestSkipping/desk.dial@trace.stopped/OK", "TestSkipping/desk.dial@trace.stopped/catches/Cut",
		"TestSkipping/desk.idle@trace.quiet/OK", "TestSkipping/desk.idle@trace.quiet/catches/Down",
		"TestSkipping/desk.relay@trace.swapped/OK", "TestSkipping/desk.relay@trace.swapped/catches/TraceDown", "TestSkipping/desk.relay@trace.swapped/catches/DeskDown",
		"TestSkipping/desk.echo@trace.echoed/OK", "TestSkipping/desk.echo@trace.echoed/catches/Down",
		"TestSkipping/desk.hush@trace.muted/OK", "TestSkipping/desk.hush@trace.muted/catches/Down", "TestSkipping/desk.hush@trace.muted/catches/Cut",
	}
	for _, name := range fails {
		if !strings.Contains(out, "--- FAIL: "+name+" ") {
			t.Errorf("テスト %s が、落ちていない:\n%s", name, out)
		}
	}
	passes := []string{
		// 中身が、失敗しなければ、どの実装も、そのまま返すので、正常のケースは、通る
		"TestLeaky/desk.ping@trace.quiet/OK", "TestLeaky/desk.fetch@trace.steady/OK", "TestLeaky/desk.load@trace.renamed/OK",
		"TestLeaky/desk.dial@trace.stopped/OK", "TestLeaky/desk.idle@trace.quiet/OK",
		"TestNoisy/desk.ping@trace.quiet/OK", "TestNoisy/desk.load@trace.renamed/OK",
		// halts は、宣言どおり（noisy は、deskSeams のものを使う）
		"TestNoisy/desk.dial@trace.stopped/catches/Cut",
		// 残りの引数で渡した例外（trace.muted）も、noisy は、deskSeams のものを使う
		"TestNoisy/desk.hush@trace.muted/catches/Down", "TestNoisy/desk.hush@trace.muted/catches/Cut",
		// 受けた例外が、デコレータの throws にもある（trace.echoed）: その例外が、そのまま返っても、宣言のうち
		"TestLeaky/desk.echo@trace.echoed/catches/Down", "TestNoisy/desk.echo@trace.echoed/catches/Down",
		// 結果 Down のケース（デコレータが、自分で投げる状態の継ぎ目）は、どの走りでも、通る
		"TestLeaky/desk.echo@trace.echoed/Down", "TestSkipping/desk.echo@trace.echoed/Down",
	}
	for _, name := range passes {
		if !strings.Contains(out, "--- PASS: "+name+" ") {
			t.Errorf("テスト %s が、通っていない:\n%s", name, out)
		}
	}
	// 落ちたものは、挙げたものだけ（いちばん内側の段で数える）
	want := map[string]bool{}
	for _, name := range fails {
		want[name] = true
	}
	for _, line := range strings.Split(out, "\n") {
		name, ok := strings.CutPrefix(strings.TrimSpace(line), "--- FAIL: ")
		if !ok {
			continue
		}
		name, _, _ = strings.Cut(name, " ")
		inner := false // 外の段（子が落ちたので、落ちた段）は、数えない
		for f := range want {
			if strings.HasPrefix(f, name+"/") {
				inner = true
			}
		}
		if !inner && !want[name] {
			t.Errorf("落ちるはずのないテスト %s が、落ちた:\n%s", name, out)
		}
	}
	for _, msg := range []string{
		"want no error (catches trace.Down ignores), got ",
		"want no error (catches trace.Down recovers), got ",
		"want no error (catches trace.Cut recovers), got ",
		"want exception trace.Down caught (recovers), got ",
		"want exception trace.Cut caught (halts), got ",
		"want no error or one of the decorator's exceptions (trace.Late), got ",
		"want exception desk.Down caught (recovers), got ",
		"want no error (catches trace.Cut ignores), got ",
		"want no error or one of the decorator's exceptions (trace.Late, trace.Odd), got other",
		"want the body called once, got 0",
		"want no error (catches trace.Down ignores), got other",
	} {
		if !strings.Contains(out, msg) {
			t.Errorf("出力に %q がない:\n%s", msg, out)
		}
	}
}
