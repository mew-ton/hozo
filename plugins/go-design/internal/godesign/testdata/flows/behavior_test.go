// 入力 flows の出力を動かす（plugins/go-design の TestBehavior が、基底 go と go-design の出力を書いたモジュールのルートに置いて
// 走らせる）。確かめること（docs/plugin-go-design.md）:
//
//   - 「E2E テストの口」: すべてのフローのケースを持つ型を RunFlows に渡せば、通る。テストの名前は、フローの FQN。走らせる側は、
//     Given、フロー、Then の順に呼ぶ。フローには、ケースの引数が、そのまま渡り、Then には、フローが返した値が渡る。Given と Then が
//     nil のケースは、どちらも呼ばずに、通る。ops は、すべてのフローで、同じもの（状態は、引き継がれる）。
//   - 「要求の、テストの入れ物の口」: すべての要求の入れ物を持つ型を RunRequirements に渡せば、通る。テストの名前は、要求の名前で、
//     入れ物の中のサブテストは、その下に出る。
//   - 「出力」: 基底が、テストのパッケージを作る出力（mixed）でも、作らない出力（flows、k、reqonly）でも、同じに使える。
package behavior

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"example.com/app/gen/act"
	"example.com/app/gen/flows"
	"example.com/app/gen/flows/flowstest"
	"example.com/app/gen/k"
	"example.com/app/gen/k/ktest"
	"example.com/app/gen/mixed"
	"example.com/app/gen/mixed/mixedtest"
	"example.com/app/gen/reqonly/reqonlytest"
)

// recorder は、呼ばれたもの（Given、操作の op、Then）を、呼ばれた順に覚える。failTouch があれば、操作の op touch が、それを返す
// （mismatch_test.go が使う）。
type recorder struct {
	log       []string
	failTouch error
}

func (r *recorder) add(name string) { r.log = append(r.log, name) }

// actLeaves は act の葉の op（操作の op の中身）。
type actLeaves struct{ r *recorder }

func (l actLeaves) Buy(_ context.Context, item string, count int64) (act.Receipt, error) {
	l.r.add("buy")
	return act.Receipt{Item: item, Count: count}, nil
}

func (l actLeaves) Touch(context.Context) error { l.r.add("touch"); return l.r.failTouch }

func (l actLeaves) Echo(_ context.Context, a, b string) (string, error) {
	l.r.add("echo")
	return a + b, nil
}

func (l actLeaves) Find(_ context.Context, name string) (*string, error) {
	l.r.add("find")
	if name == "" {
		return nil, nil
	}
	return &name, nil
}

func (l actLeaves) Count(_ context.Context, items []string) (int64, error) {
	l.r.add("count")
	return int64(len(items)), nil
}

func (l actLeaves) Note(_ context.Context, text string) error { l.r.add("note " + text); return nil }

// flowCases は flowstest.FlowCases を満たす。
type flowCases struct{ r *recorder }

func (c flowCases) Checkout() flowstest.CheckoutFlowCase {
	return flowstest.CheckoutFlowCase{
		Item:  "pen",
		Count: 2,
		Given: func(context.Context) error { c.r.add("given checkout"); return nil },
		Then: func(t *testing.T, got act.Receipt) {
			c.r.add("then checkout")
			if want := (act.Receipt{Item: "pen", Count: 2}); got != want {
				t.Errorf("got = %+v, want %+v（フローには、ケースの引数が、そのまま渡る）", got, want)
			}
		},
	}
}

// Given も Then も nil: どちらも呼ばずに、通る
func (c flowCases) Ping() flowstest.PingFlowCase { return flowstest.PingFlowCase{} }

func (c flowCases) Rename() flowstest.RenameFlowCase {
	return flowstest.RenameFlowCase{
		Given_: "a",
		Then_:  "b",
		Then: func(t *testing.T, got string) {
			if got != "ab" {
				t.Errorf("got = %q, want %q", got, "ab")
			}
		},
	}
}

func (c flowCases) Branch() flowstest.BranchFlowCase {
	return flowstest.BranchFlowCase{
		Name:  "n",
		Items: []string{"x", "y"},
		Given: func(context.Context) error { c.r.add("given branch"); return nil },
		Then: func(t *testing.T, got string) {
			c.r.add("then branch")
			if got != "nn" {
				t.Errorf("got = %q, want %q", got, "nn")
			}
		},
	}
}

func (c flowCases) Lookup() flowstest.LookupFlowCase {
	return flowstest.LookupFlowCase{
		Then: func(t *testing.T, got *string) {
			if got == nil || *got != "x" {
				t.Errorf("got = %v, want x", got)
			}
		},
	}
}

func TestFlows(t *testing.T) {
	r := &recorder{}
	flowstest.RunFlows(t, flows.New(act.New(actLeaves{r})), flowCases{r})
	// Given、フロー（操作の op を、配線の順に）、Then の順。フローの順は、宣言順
	want := []string{
		"given checkout", "touch", "buy", "then checkout",
		"touch",
		"echo",
		"given branch", "find", "note x", "note y", "touch", "note n", "echo", "then branch",
		"find",
	}
	if !reflect.DeepEqual(r.log, want) {
		t.Errorf("呼ばれた順 =\n  %q\nwant\n  %q", r.log, want)
	}
}

// requirements は flowstest.Requirements を満たす。入れ物の中に、サブテストを、いくつでも書ける。
type requirements struct{ r *recorder }

func (c requirements) R1(t *testing.T) {
	t.Run("first", func(*testing.T) { c.r.add("R1/first") })
	t.Run("second", func(*testing.T) { c.r.add("R1/second") })
}
func (c requirements) R2(*testing.T) { c.r.add("R2") }
func (c requirements) R3(*testing.T) { c.r.add("R3") }
func (c requirements) X1(*testing.T) { c.r.add("X1") }

func TestRequirements(t *testing.T) {
	r := &recorder{}
	flowstest.RunRequirements(t, requirements{r})
	// requirements に書いた分類の順、その中は、ケースの宣言順
	if want := []string{"R1/first", "R1/second", "R2", "R3", "X1"}; !reflect.DeepEqual(r.log, want) {
		t.Errorf("呼ばれた順 = %q, want %q", r.log, want)
	}
}

// 基底が、テストのパッケージを作る出力（葉がある）: 同じパッケージに、基底の口（LeafCases、RunLeaves）と、go-design の口が並ぶ
type mixedLeaves struct{}

func (mixedLeaves) Helper(_ context.Context, x int64) (int64, error) { return x, nil }

type mixedCases struct{}

func (mixedCases) Run() mixedtest.RunFlowCase {
	return mixedtest.RunFlowCase{
		Items: []string{"a", "b", "c"},
		Then: func(t *testing.T, got int64) {
			if got != 3 {
				t.Errorf("got = %d, want 3", got)
			}
		},
	}
}

type mixedRequirements struct{ called *bool }

func (c mixedRequirements) X1(*testing.T) { *c.called = true }

func TestMixed(t *testing.T) {
	var _ mixedtest.LeafCases // 基底の口も、同じパッケージにある
	r := &recorder{}
	mixedtest.RunFlows(t, mixed.New(mixedLeaves{}, act.New(actLeaves{r})), mixedCases{})
	var called bool
	mixedtest.RunRequirements(t, mixedRequirements{&called})
	if !called {
		t.Error("要求 design.Extra.X1 の入れ物が、呼ばれていない")
	}
}

// パッケージの名前が、本体の局所の名前 k と当たる名前空間
type kCases struct{}

func (kCases) Go() ktest.GoFlowCase {
	return ktest.GoFlowCase{
		Text: "t",
		Then: func(t *testing.T, got k.Out) {
			if got.Text != "tt" {
				t.Errorf("got = %+v, want tt", got)
			}
		},
	}
}

func TestK(t *testing.T) {
	ktest.RunFlows(t, k.New(act.New(actLeaves{&recorder{}})), kCases{})
}

// 要求だけの単位
type onlyRequirements struct{ r *recorder }

func (c onlyRequirements) R1(*testing.T) { c.r.add("R1") }
func (c onlyRequirements) R2(*testing.T) { c.r.add("R2") }
func (c onlyRequirements) R3(*testing.T) { c.r.add("R3") }

func TestReqOnly(t *testing.T) {
	r := &recorder{}
	reqonlytest.RunRequirements(t, onlyRequirements{r})
	if want := []string{"R1", "R2", "R3"}; !reflect.DeepEqual(r.log, want) {
		t.Errorf("呼ばれた順 = %q, want %q", r.log, want)
	}
}

// errBoom は、mismatch_test.go が、Given と、操作の op に返させる失敗。
var errBoom = errors.New("boom")
