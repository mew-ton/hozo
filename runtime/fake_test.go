package runtime

import (
	"errors"
	"reflect"
	"testing"
)

// TestFake は、値を返すメソッドのにせもの（docs/plugin-go.md「runtime パッケージ」）: 呼ばれた引数を覚える。言われていなければ、本物を
// 呼ぶか（本物があるとき）、零値を返す。Returns と Throws は、後に言った方が効き、本物を呼ばせない。
func TestFake(t *testing.T) {
	type args struct{ ID int }
	boom := errors.New("boom")

	var f Fake[args, string]
	if got, err := f.Call(args{1}, nil); got != "" || err != nil {
		t.Fatalf("言われていない、本物のない呼び出し: %q, %v", got, err)
	}
	called := 0
	impl := func() (string, error) { called++; return "real", nil }
	if got, err := f.Call(args{2}, impl); got != "real" || err != nil || called != 1 {
		t.Fatalf("言われていない呼び出しは、本物を呼ぶ: %q, %v, %d 回", got, err, called)
	}
	f.Returns("said")
	if got, err := f.Call(args{3}, impl); got != "said" || err != nil || called != 1 {
		t.Fatalf("Returns の後は、本物を呼ばない: %q, %v, %d 回", got, err, called)
	}
	f.Throws(boom)
	if got, err := f.Call(args{4}, impl); got != "" || !errors.Is(err, boom) || called != 1 {
		t.Fatalf("Throws の後は、零値と、その失敗: %q, %v, %d 回", got, err, called)
	}
	f.Returns("again")
	if got, err := f.Call(args{5}, nil); got != "again" || err != nil {
		t.Fatalf("後に言った Returns が効く: %q, %v", got, err)
	}
	if want := []args{{1}, {2}, {3}, {4}, {5}}; !reflect.DeepEqual(f.Calls, want) {
		t.Fatalf("Calls = %v, want %v", f.Calls, want)
	}
}

// TestFakeVoid は、値を返さないメソッドのにせもの。
func TestFakeVoid(t *testing.T) {
	boom := errors.New("boom")
	var f FakeVoid[struct{}]
	if err := f.Call(struct{}{}, nil); err != nil {
		t.Fatalf("言われていない、本物のない呼び出し: %v", err)
	}
	called := 0
	impl := func() error { called++; return boom }
	if err := f.Call(struct{}{}, impl); !errors.Is(err, boom) || called != 1 {
		t.Fatalf("言われていない呼び出しは、本物を呼ぶ: %v, %d 回", err, called)
	}
	f.Returns()
	if err := f.Call(struct{}{}, impl); err != nil || called != 1 {
		t.Fatalf("Returns の後は、本物を呼ばずに、正常に終わる: %v, %d 回", err, called)
	}
	f.Throws(boom)
	if err := f.Call(struct{}{}, nil); !errors.Is(err, boom) {
		t.Fatalf("Throws の後は、その失敗: %v", err)
	}
	if len(f.Calls) != 4 {
		t.Fatalf("Calls = %d 件, want 4", len(f.Calls))
	}
}
