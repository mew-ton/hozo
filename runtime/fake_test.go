package runtime

import (
	"errors"
	"reflect"
	"testing"
)

// TestFake は、値を返すメソッドのにせもの（パッケージ runtime の説明（runtime/doc.go））: 呼ばれた引数を覚える。言われていなければ、本物を
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

// TestFakeAt は、呼び出しごとの結果（パッケージ runtime の説明（runtime/doc.go）の ReturnsAt と ThrowsAt）: 言われた回だけが、その結果を
// 返し、本物は、呼ばない。ほかの回は、Returns か Throws、どちらも言われていなければ、本物か、零値。同じ回に二度言えば、後が効く。
func TestFakeAt(t *testing.T) {
	type args struct{ ID int }
	boom := errors.New("boom")
	called := 0
	impl := func() (string, error) { called++; return "real", nil }

	// 一度目は本物、二度目は失敗、三度目は言われた値、四度目は本物
	var f Fake[args, string]
	f.ThrowsAt(2, boom)
	f.ReturnsAt(3, "third")
	if got, err := f.Call(args{1}, impl); got != "real" || err != nil || called != 1 {
		t.Fatalf("一度目は、本物: %q, %v, %d 回", got, err, called)
	}
	if got, err := f.Call(args{2}, impl); got != "" || !errors.Is(err, boom) || called != 1 {
		t.Fatalf("二度目は、零値と、その失敗（本物は、呼ばない）: %q, %v, %d 回", got, err, called)
	}
	if got, err := f.Call(args{3}, impl); got != "third" || err != nil || called != 1 {
		t.Fatalf("三度目は、言われた値（本物は、呼ばない）: %q, %v, %d 回", got, err, called)
	}
	if got, err := f.Call(args{4}, impl); got != "real" || err != nil || called != 2 {
		t.Fatalf("四度目は、本物: %q, %v, %d 回", got, err, called)
	}
	if want := []args{{1}, {2}, {3}, {4}}; !reflect.DeepEqual(f.Calls, want) {
		t.Fatalf("Calls = %v（期待は %v）", f.Calls, want)
	}

	// 言われていない回は、Returns のとおり。同じ回に二度言えば、後が効く
	var g Fake[args, string]
	g.Returns("always")
	g.ThrowsAt(1, boom)
	g.ReturnsAt(1, "first")
	if got, err := g.Call(args{1}, impl); got != "first" || err != nil {
		t.Fatalf("同じ回に二度言えば、後が効く: %q, %v", got, err)
	}
	if got, err := g.Call(args{2}, impl); got != "always" || err != nil {
		t.Fatalf("言われていない回は、Returns のとおり: %q, %v", got, err)
	}

	// 本物のない呼び出しでも、言われた回だけが、その結果
	var h Fake[args, string]
	h.ThrowsAt(2, boom)
	if got, err := h.Call(args{1}, nil); got != "" || err != nil {
		t.Fatalf("本物がなく、言われていない回は、零値: %q, %v", got, err)
	}
	if _, err := h.Call(args{2}, nil); !errors.Is(err, boom) {
		t.Fatalf("二度目は、その失敗: %v", err)
	}
}

// TestFakeVoidAt は、値を返さないメソッドの、呼び出しごとの結果。
func TestFakeVoidAt(t *testing.T) {
	type args struct{ ID int }
	boom := errors.New("boom")
	called := 0
	impl := func() error { called++; return nil }

	var f FakeVoid[args]
	f.ThrowsAt(2, boom)
	if err := f.Call(args{1}, impl); err != nil || called != 1 {
		t.Fatalf("一度目は、本物: %v, %d 回", err, called)
	}
	if err := f.Call(args{2}, impl); !errors.Is(err, boom) || called != 1 {
		t.Fatalf("二度目は、その失敗（本物は、呼ばない）: %v, %d 回", err, called)
	}
	if err := f.Call(args{3}, impl); err != nil || called != 2 {
		t.Fatalf("三度目は、本物: %v, %d 回", err, called)
	}

	// いつも失敗する中で、一度だけ、正常に終わる
	var g FakeVoid[args]
	g.Throws(boom)
	g.ReturnsAt(2)
	if err := g.Call(args{1}, impl); !errors.Is(err, boom) {
		t.Fatalf("一度目は、Throws のとおり: %v", err)
	}
	if err := g.Call(args{2}, impl); err != nil || called != 2 {
		t.Fatalf("二度目は、正常に終わる（本物は、呼ばない）: %v, %d 回", err, called)
	}
}
