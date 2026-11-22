// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go`）: 例外を受ける文
// （docs/plugin-go.md「例外を受ける文」、docs/check.md「例外を受ける」）。受けるのは、仕様の例外だけで、仕様にない失敗は、
// そのまま返る。throw e は、受けた失敗を、包み直さずに返す。plugins/go の TestBehavior が、生成したコードのモジュールのルートに
// 置いて走らせる。
package behavior

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"example.com/app/gen/app"
)

// leaves は、葉の op。fail が、葉の名前ごとの、返す失敗。notes が、note に渡された値。
type leaves struct {
	fail  map[string]error
	notes []string
}

func (l *leaves) Send(context.Context, string) error { return l.fail["send"] }
func (l *leaves) Soft(context.Context, string) error { return l.fail["soft"] }
func (l *leaves) Load(_ context.Context, id string) (string, error) {
	if err := l.fail["load:"+id]; err != nil {
		return "", err
	}
	return "loaded " + id, l.fail["load"]
}
func (l *leaves) Fallback(_ context.Context, id string) (string, error) { return "fallback " + id, nil }
func (l *leaves) Note(_ context.Context, text string) error {
	l.notes = append(l.notes, text)
	return l.fail["note"]
}

func newOps(fail map[string]error) (app.Ops, *leaves) {
	l := &leaves{fail: fail}
	return app.New(l), l
}

// broken は、仕様にない失敗（runtime.Exception でない error）。
var broken = errors.New("接続が切れた")

// 受けて、記録して、投げ直す: 仕様の例外は、同じ値のまま返る。包まれた仕様の例外も、受けて、包まれたまま返す。
// 仕様にない失敗は、catch のブロックを通らずに、そのまま返る
func TestRethrow(t *testing.T) {
	ctx := context.Background()
	wrapped := fmt.Errorf("送信: %w", app.Lost{Code: 3})
	for _, tc := range []struct {
		name  string
		fail  error
		notes []string
	}{
		{"失敗しない", nil, nil},
		{"仕様の例外", app.Refused{}, []string{"failed"}},
		{"包まれた仕様の例外", wrapped, []string{"failed"}},
		{"仕様にない失敗", broken, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, l := newOps(map[string]error{"send": tc.fail})
			if err := o.Relay(ctx, "x"); err != tc.fail {
				t.Errorf("Relay = %v（期待は %v。受けた失敗は、そのまま返る）", err, tc.fail)
			}
			if !slices.Equal(l.notes, tc.notes) {
				t.Errorf("note = %v（期待は %v）", l.notes, tc.notes)
			}
		})
	}
	// catch のブロックの中の失敗は、その失敗が返る（受けた失敗では、ない）
	o, _ := newOps(map[string]error{"send": app.Refused{}, "note": broken})
	if err := o.Relay(ctx, "x"); err != broken {
		t.Errorf("Relay = %v（期待は、catch のブロックの中の失敗）", err)
	}
}

// 空の catch は、仕様の例外を、受けて、何もしない。仕様にない失敗は、受けない
func TestSwallow(t *testing.T) {
	ctx := context.Background()
	o, _ := newOps(map[string]error{"send": app.Broken{}})
	if err := o.Quiet(ctx, "x"); err != nil {
		t.Errorf("Quiet = %v（期待は、成功: 仕様の例外は、受ける）", err)
	}
	o, _ = newOps(map[string]error{"send": broken})
	if err := o.Quiet(ctx, "x"); err != broken {
		t.Errorf("Quiet = %v（期待は、仕様にない失敗が、そのまま）", err)
	}
}

// catch のブロックの throw 例外(値): 別の例外に替える
func TestReplace(t *testing.T) {
	o, _ := newOps(map[string]error{"send": app.Refused{}})
	if err := o.Replace(context.Background(), "x"); err != error(app.Lost{Code: 7}) {
		t.Errorf("Replace = %#v（期待は Lost{Code: 7}）", err)
	}
}

// catch (e by 分類): 例外の型で、アームが決まる。残りのケースのアームは、並べなかった例外のすべて
func TestClassified(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		fail  error
		want  error
		notes []string
	}{
		{"Soft（Refused）", app.Refused{}, nil, []string{"soft"}},
		{"Soft（Lost）", app.Lost{Code: 1}, nil, []string{"soft"}},
		{"Hard（残り: Broken）は、投げ直す", app.Broken{}, app.Broken{}, nil},
		{"仕様にない失敗", broken, broken, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, l := newOps(map[string]error{"send": tc.fail})
			if err := o.Sorted(ctx, "x"); err != tc.want {
				t.Errorf("Sorted = %v（期待は %v）", err, tc.want)
			}
			if !slices.Equal(l.notes, tc.notes) {
				t.Errorf("note = %v（期待は %v）", l.notes, tc.notes)
			}
		})
	}
}

// 残りのケースがない分類: none のアームは、何もしない。else のアームは、並べなかったケース。どのケースにも当たらない、
// 仕様の例外（葉が、仕様にない例外を返したとき）は、そのまま返る
func TestClosedClassification(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		fail  error
		want  error
		notes []string
	}{
		{"Soft は none", app.Refused{}, nil, nil},
		{"else（Lossy）", app.Lost{Code: 1}, nil, []string{"lost"}},
		{"どのケースにも当たらない", app.Broken{}, app.Broken{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, l := newOps(map[string]error{"soft": tc.fail})
			if err := o.Closed(ctx, "x"); err != tc.want {
				t.Errorf("Closed = %v（期待は %v）", err, tc.want)
			}
			if !slices.Equal(l.notes, tc.notes) {
				t.Errorf("note = %v（期待は %v）", l.notes, tc.notes)
			}
		})
	}
}

// try が入れ、catch も入れた val を、後ろで読む
func TestAssignedInBoth(t *testing.T) {
	ctx := context.Background()
	o, _ := newOps(nil)
	if got, err := o.Either(ctx, "a"); err != nil || got != "loaded a" {
		t.Errorf("Either = %q, %v（期待は、try が入れた値）", got, err)
	}
	o, _ = newOps(map[string]error{"load": app.Refused{}})
	if got, err := o.Either(ctx, "a"); err != nil || got != "fallback a" {
		t.Errorf("Either = %q, %v（期待は、catch が入れた値）", got, err)
	}
	o, _ = newOps(map[string]error{"load": broken})
	if got, err := o.Either(ctx, "a"); err != broken || got != "" {
		t.Errorf("Either = %q, %v（期待は、仕様にない失敗）", got, err)
	}
}

// for の中の try: 受けた回は、集めずに、次の回へ進む
func TestTryInLoop(t *testing.T) {
	o, l := newOps(map[string]error{"load:b": app.Refused{}})
	got, err := o.Each(context.Background(), []string{"a", "b", "c"})
	if err != nil || !slices.Equal(got, []string{"loaded a", "loaded c"}) {
		t.Errorf("Each = %v, %v（期待は、a と c）", got, err)
	}
	if !slices.Equal(l.notes, []string{"b"}) {
		t.Errorf("note = %v（期待は、受けた回の b）", l.notes)
	}
}

// 入れ子: 内側が投げ直した例外を、外側が受ける
func TestNested(t *testing.T) {
	o, l := newOps(map[string]error{"send": app.Refused{}})
	if err := o.Nested(context.Background(), "x"); err != nil {
		t.Errorf("Nested = %v（期待は、成功: 外側が受ける）", err)
	}
	if !slices.Equal(l.notes, []string{"inner", "outer"}) {
		t.Errorf("note = %v（期待は inner、outer）", l.notes)
	}
	o, l = newOps(map[string]error{"send": broken})
	if err := o.Nested(context.Background(), "x"); err != broken || len(l.notes) != 0 {
		t.Errorf("Nested = %v、note = %v（期待は、仕様にない失敗が、どちらの catch も通らずに返る）", err, l.notes)
	}
}

// 本体のあるデコレータの try { next(); … }: 値を返す op は、next() の結果を返す。例外は、受けて、記録して、投げ直す
func TestDecoratorTry(t *testing.T) {
	ctx := context.Background()
	o, l := newOps(nil)
	if got, err := o.Fetch(ctx, "a"); err != nil || got != "loaded a" || !slices.Equal(l.notes, []string{"done"}) {
		t.Errorf("Fetch = %q, %v、note = %v（期待は、結果と done）", got, err, l.notes)
	}
	o, l = newOps(map[string]error{"load": app.Refused{}})
	if got, err := o.Fetch(ctx, "a"); err != error(app.Refused{}) || got != "" || !slices.Equal(l.notes, []string{"failed"}) {
		t.Errorf("Fetch = %q, %v、note = %v（期待は Refused と failed）", got, err, l.notes)
	}
	// catch (e by 分類) のデコレータ: Soft は、受けて終わり、Hard は、投げ直す
	o, _ = newOps(map[string]error{"send": app.Lost{Code: 2}})
	if err := o.Push(ctx, "x"); err != nil {
		t.Errorf("Push = %v（期待は、成功: Soft は、受ける）", err)
	}
	o, _ = newOps(map[string]error{"send": app.Broken{}})
	if err := o.Push(ctx, "x"); err != error(app.Broken{}) {
		t.Errorf("Push = %v（期待は Broken: Hard は、投げ直す）", err)
	}
}
