// 生成したコードの振る舞いのテスト（docs/testing.md「単体テスト」の `plugins/go`）: 引数を持つ throw が作る例外の値
// （docs/plugin-go.md「exception」と、「match」の「アームの本体」の throw E(<値>, …)）。例外は値で返り、errors.As で、値の型で
// 見分けて、引数ごとのフィールドを読める。plugins/go の TestBehavior が、生成したコードのモジュールのルートに置いて走らせる。
package behavior

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"example.com/app/gen/app"
	"example.com/app/gen/limits"
	"github.com/mew-ton/hozo/runtime"
)

type leaves struct{}

func (leaves) Show(context.Context, string) error { return nil }

// attempts は、状態 app.attempts の単位。
type attempts struct {
	n   int64
	err error
}

func (a attempts) Get(context.Context) (int64, error) { return a.n, a.err }

func newOps(a attempts) app.Ops { return app.New(leaves{}, a) }

func ptr[T any](v T) *T { return &v }

// 引数が一つの例外と、引数のない例外: errors.As の先は、例外の値の型（ポインタではない）
func TestThrowOneArgument(t *testing.T) {
	ctx := context.Background()
	o := newOps(attempts{})
	if got, err := o.Need(ctx, ptr("u1"), ptr(true)); err != nil || got != "u1" {
		t.Fatalf("Need = %q, %v", got, err)
	}
	_, err := o.Need(ctx, nil, ptr(true))
	var missing app.Missing
	if !errors.As(err, &missing) || missing.Id != "unknown" {
		t.Fatalf("Need の err = %#v, want Missing{Id: unknown}", err)
	}
	if missing.Error() != "app.Missing" || missing.HozoException() != "app.Missing" {
		t.Errorf("Missing の Error = %q, HozoException = %q", missing.Error(), missing.HozoException())
	}
	// 値で返るので、同じ値と == で比べられる
	if err != error(app.Missing{Id: "unknown"}) {
		t.Errorf("Need の err = %#v", err)
	}
	var asPointer *app.Missing
	if errors.As(err, &asPointer) {
		t.Error("例外が、ポインタで返っている")
	}
	_, err = o.Need(ctx, ptr("u1"), nil)
	var plain app.Plain
	if !errors.As(err, &plain) || errors.As(err, &missing) {
		t.Fatalf("Need の err = %#v, want Plain", err)
	}
	// どの例外も runtime.Exception
	var ex runtime.Exception
	if !errors.As(err, &ex) || ex.HozoException() != "app.Plain" {
		t.Errorf("runtime.Exception として取り出せない: %#v", err)
	}
}

// 引数が二つ以上の例外: フィールドは引数ごとに一つ。レコードリテラル、array、省略可能な引数（値、省略可能を通るパス）
func TestThrowSeveralArguments(t *testing.T) {
	ctx := context.Background()
	o := newOps(attempts{})
	in := app.Input{Code: "c1", Sub: &app.Sub{At: 9}}
	tags := []string{"a", "b"}

	if got, err := o.Check(ctx, in, tags, ptr("ok")); err != nil || got != "ok" {
		t.Fatalf("Check = %q, %v", got, err)
	}

	_, err := o.Check(ctx, in, tags, nil)
	var invalid app.Invalid
	if !errors.As(err, &invalid) {
		t.Fatalf("Check の err = %#v, want Invalid", err)
	}
	want := app.Invalid{Code: "c1", Detail: app.Detail{Field: "text"}, Tags: tags, At: ptr(int64(7))}
	if !reflect.DeepEqual(invalid, want) {
		t.Errorf("Invalid = %+v, want %+v", invalid, want)
	}

	_, err = o.Check(ctx, in, nil, ptr("first"))
	if !errors.As(err, &invalid) {
		t.Fatalf("Check の err = %#v, want Invalid", err)
	}
	want = app.Invalid{Code: "c1", Detail: app.Detail{Field: "tags", Hint: ptr("first")}, At: ptr(int64(9))}
	if !reflect.DeepEqual(invalid, want) {
		t.Errorf("Invalid = %+v, want %+v", invalid, want)
	}
	// 省略可能を通るパスの途中に値がなければ、その引数は nil
	_, err = o.Check(ctx, app.Input{Code: "c2"}, nil, ptr("first"))
	if !errors.As(err, &invalid) || invalid.At != nil || invalid.Code != "c2" {
		t.Errorf("Invalid = %+v, want At が nil", invalid)
	}

	// フィールドのタグは、引数の名前（省略可能なら omitempty）
	data, err := json.Marshal(app.Invalid{Code: "c", Detail: app.Detail{Field: "f"}, Tags: []string{"t"}})
	if err != nil || string(data) != `{"code":"c","detail":{"field":"f"},"tags":["t"]}` {
		t.Errorf("Invalid の JSON = %s, %v", data, err)
	}
	data, err = json.Marshal(limits.LimitExceeded{RetryAfter: time.Second, Scope: ptr("s")})
	if err != nil || string(data) != `{"retryAfter":1000000000,"scope":"s"}` {
		t.Errorf("LimitExceeded の JSON = %s, %v", data, err)
	}
}

// 文の match の throw: 戻り値のない配線は、例外だけを返す。別の名前空間の例外、渡されていない省略可能な引数
func TestThrowInStatement(t *testing.T) {
	ctx := context.Background()
	o := newOps(attempts{})
	err := o.Guard(ctx, ptr(app.Code("c9")), nil)
	var missing app.Missing
	if !errors.As(err, &missing) || missing.Id != "c9" {
		t.Fatalf("Guard の err = %#v, want Missing{Id: c9}", err)
	}
	err = o.Guard(ctx, nil, ptr(true))
	var limit limits.LimitExceeded
	if !errors.As(err, &limit) || limit.RetryAfter != 5*time.Minute || limit.Scope == nil || *limit.Scope != "guard" {
		t.Fatalf("Guard の err = %#v, want LimitExceeded{5m, guard}", err)
	}
	if limit.HozoException() != "limits.LimitExceeded" {
		t.Errorf("HozoException = %q", limit.HozoException())
	}
	err = o.Guard(ctx, nil, nil)
	if !errors.As(err, &limit) || limit.RetryAfter != time.Second || limit.Scope != nil {
		t.Fatalf("Guard の err = %#v, want LimitExceeded{1s, nil}", err)
	}
}

// throw の引数に書いた状態は、その枝の中で読む（もう一方の枝では読まない）。読めなければ、そのエラーを返す
func TestThrowReadsState(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("boom")
	if got, err := newOps(attempts{err: boom}).Retry(ctx, ptr("ok")); err != nil || got != "ok" {
		t.Fatalf("Retry = %q, %v（値のある枝は、状態を読まない）", got, err)
	}
	_, err := newOps(attempts{n: 4}).Retry(ctx, nil)
	var tooMany app.TooMany
	if !errors.As(err, &tooMany) || tooMany.Attempts != 4 || tooMany.Last != nil {
		t.Fatalf("Retry の err = %#v, want TooMany{Attempts: 4}", err)
	}
	if _, err := newOps(attempts{err: boom}).Retry(ctx, nil); err != boom {
		t.Fatalf("Retry の err = %v, want boom", err)
	}
	// 束縛を、throw の引数に使う
	_, err = newOps(attempts{}).Count(ctx, nil)
	if !errors.As(err, &tooMany) || tooMany.Attempts != 3 {
		t.Fatalf("Count の err = %#v, want TooMany{Attempts: 3}", err)
	}
}
