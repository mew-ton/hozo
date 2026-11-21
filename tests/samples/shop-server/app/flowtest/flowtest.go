// Package flowtest は、サンプル shop-server の、フローの E2E テストと、要求のテスト
// （docs/plugin-go-design.md「E2E テストの口」「要求の、テストの入れ物の口」）。
//
// go-design が出した二つの口（flowstest.FlowCases、flowstest.Requirements）を満たす型を書き、flowstest.RunFlows と
// flowstest.RunRequirements に渡す。フローの配線（操作の op を、順に呼ぶ）は、基底が作り、走らせる側（Given、フロー、正常に終わった
// ことの確かめ、Then の順）は、go-design が作る。ここに書くのは、フローの引数と、今の状態の用意（Given）と、後の状態と返り値の
// 確かめ（Then）と、要求ごとの入れ物の中身だけ。フローか、要求を、一つ足せば、足りないメソッドで、ここの型はコンパイルできなくなる。
//
// 二つの組み立て（../onmem、../onsqlite）が、同じものを走らせる。操作の op（client）は、機能入口（server の入口の Ops）を、Go の上で
// 直に呼ぶ: HTTP の層は、通らない（その層は、インテグレーションテストが通す: ../handlertest/httpcases.go）。
package flowtest

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"example.com/shop-server/app/handlertest"
	"example.com/shop-server/gen/client"
	"example.com/shop-server/gen/flows"
	"example.com/shop-server/gen/flows/flowstest"
	pkgtype "example.com/shop-server/gen/type"
)

// Store は、テストが、状態を用意し、確かめるのに使う store の op。gen/store と gen/storemem の Ops のどちらも満たす。
type Store interface {
	AddUser(ctx context.Context, user pkgtype.User) error
	ListOrders(ctx context.Context, userId pkgtype.UserId) ([]pkgtype.Order, error)
}

// System は試す相手: 一つの store の上に、server、client、flows を重ねたもの。
type System struct {
	// Flows は、フローの op（flows.New の戻り値）。
	Flows flows.Ops
	// Client は、その下の、操作の op（client.New の戻り値）。要求のテストが、操作を、一つずつ呼ぶのに使う。
	Client client.Ops
	// Store は、その下の store。
	Store Store
}

// NewSystem は、新しい System を作る関数: 組み立てごとに（../onmem、../onsqlite）、一つ書く。f は、葉と、配送と、数え方の材料。
type NewSystem func(t *testing.T, f *handlertest.Fixture) System

// lines は、テストが注文する行。
var lines = []pkgtype.OrderLine{{Item: "apple", Quantity: 2}}

// ---- フローの E2E テスト ----

// FlowCases は、flowstest.FlowCases を満たす: フローごとに、ケースを一つ返す。
type FlowCases struct{ s System }

var _ flowstest.FlowCases = FlowCases{}

// RunFlows は、newSystem が作る System 一つに、すべてのフローのケースを走らせる。テストの名前は、フローの FQN（flows.signInAndOrder）。
func RunFlows(t *testing.T, newSystem NewSystem) {
	s := newSystem(t, &handlertest.Fixture{})
	flowstest.RunFlows(t, s.Flows, FlowCases{s})
}

// SignInAndOrder: alice がいる状態から、ログインして、注文し、一覧を見る。
func (c FlowCases) SignInAndOrder() flowstest.SignInAndOrderFlowCase {
	return flowstest.SignInAndOrderFlowCase{
		Email:    handlertest.AliceEmail,
		Password: handlertest.AlicePassword,
		Lines:    lines,
		// 今の状態: alice が、登録されている
		Given: func(ctx context.Context) error { return c.s.Store.AddUser(ctx, handlertest.Alice()) },
		Then: func(t *testing.T, got flows.Ordered) {
			// 返り値: できた注文は、alice のもので、その後の一覧に、その注文が出る
			if got.Placed.UserId != handlertest.AliceID || !reflect.DeepEqual(got.Placed.Lines, lines) {
				t.Errorf("placed = %+v（期待は、%s の、行 %+v の注文）", got.Placed, handlertest.AliceID, lines)
			}
			if len(got.Listed) != 1 || got.Listed[0].Id != got.Placed.Id {
				t.Errorf("listed = %+v（期待は、できた注文 %s が一つ）", got.Listed, got.Placed.Id)
			}
			// 後の状態: store に、その注文が、一つ残っている
			orders, err := c.s.Store.ListOrders(context.Background(), handlertest.AliceID)
			if err != nil {
				t.Fatal(err)
			}
			if len(orders) != 1 || orders[0].Id != got.Placed.Id {
				t.Errorf("フローの後の、store の注文 = %+v（期待は、できた注文 %s が一つ）", orders, got.Placed.Id)
			}
		},
	}
}

// ---- 要求のテスト ----

// Requirements は、flowstest.Requirements を満たす: 要求ごとに、テストの入れ物を一つ持つ。どの設計を通して確かめるかは、
// 入れ物ごとに選ぶ（ここでは、どちらも、操作の op を通す）。入れ物の中のサブテストは、それぞれ、新しい System を作る。
type Requirements struct{ newSystem NewSystem }

var _ flowstest.Requirements = Requirements{}

// RunRequirements は、すべての要求の入れ物を走らせる。テストの名前は、要求の名前（design.Requirement.PRD_001）。
func RunRequirements(t *testing.T, newSystem NewSystem) {
	flowstest.RunRequirements(t, Requirements{newSystem})
}

// start は、新しい System を作り、alice を足す。
func (r Requirements) start(t *testing.T) System {
	t.Helper()
	s := r.newSystem(t, &handlertest.Fixture{})
	if err := s.Store.AddUser(context.Background(), handlertest.Alice()); err != nil {
		t.Fatal(err)
	}
	return s
}

// orders は、ユーザー id の、store の注文の数。
func orders(t *testing.T, s System, id pkgtype.UserId) int {
	t.Helper()
	got, err := s.Store.ListOrders(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return len(got)
}

// PRD_001: ログインしていない人は、注文できない。
func (r Requirements) PRD_001(t *testing.T) {
	t.Run("どのセッションのものでもないトークンでは、注文できない", func(t *testing.T) {
		s := r.start(t)
		_, err := s.Client.Order(context.Background(), "no-such-token", lines)
		var unauthorized pkgtype.Unauthorized
		if !errors.As(err, &unauthorized) {
			t.Fatalf("order のエラー = %v（期待は type.Unauthorized）", err)
		}
		if n := orders(t, s, handlertest.AliceID); n != 0 {
			t.Errorf("断られた注文の後の、store の注文は %d 件（期待は 0）", n)
		}
	})
	t.Run("ログインすれば、注文できる", func(t *testing.T) {
		s, ctx := r.start(t), context.Background()
		token, err := s.Client.SignIn(ctx, handlertest.AliceEmail, handlertest.AlicePassword)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Client.Order(ctx, token, lines); err != nil {
			t.Fatalf("ログインした後の order のエラー = %v", err)
		}
		if n := orders(t, s, handlertest.AliceID); n != 1 {
			t.Errorf("注文の後の、store の注文は %d 件（期待は 1）", n)
		}
	})
}

// PRD_002: 注文したら、その注文が、自分の注文の一覧に出る。
func (r Requirements) PRD_002(t *testing.T) {
	t.Run("注文が、自分の一覧に出る", func(t *testing.T) {
		s, ctx := r.start(t), context.Background()
		token, err := s.Client.SignIn(ctx, handlertest.AliceEmail, handlertest.AlicePassword)
		if err != nil {
			t.Fatal(err)
		}
		placed, err := s.Client.Order(ctx, token, lines)
		if err != nil {
			t.Fatal(err)
		}
		listed, err := s.Client.ViewOrders(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) != 1 || listed[0].Id != placed.Id {
			t.Errorf("注文の後の一覧 = %+v（期待は、できた注文 %s が一つ）", listed, placed.Id)
		}
	})
	t.Run("ほかの人の一覧には、出ない", func(t *testing.T) {
		s, ctx := r.start(t), context.Background()
		bob := handlertest.Alice()
		bob.Id, bob.Email, bob.Name = "u2", "bob@example.com", "Bob"
		if err := s.Store.AddUser(ctx, bob); err != nil {
			t.Fatal(err)
		}
		alice, err := s.Client.SignIn(ctx, handlertest.AliceEmail, handlertest.AlicePassword)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Client.Order(ctx, alice, lines); err != nil {
			t.Fatal(err)
		}
		other, err := s.Client.SignIn(ctx, bob.Email, handlertest.AlicePassword)
		if err != nil {
			t.Fatal(err)
		}
		listed, err := s.Client.ViewOrders(ctx, other)
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) != 0 {
			t.Errorf("注文していない人の一覧 = %+v（期待は、空）", listed)
		}
	})
}

// ---- 口を欠いた型 ----

// withoutFlow は、フローのケースを、一つも持たない型。withoutPRD002 は、要求 PRD_002 の入れ物を欠いた型。
type (
	withoutFlow   struct{}
	withoutPRD002 struct{}
)

func (withoutPRD002) PRD_001(*testing.T) {}

// RunIncomplete は、ケースか、入れ物を欠いた型が、口を満たさないこと。欠いた型を渡す行（flowstest.RunFlows(t, ops, withoutFlow{})）は、
// コンパイルできないので、ここには書けない。代わりに、口が求めるメソッドを、リフレクションで確かめる: FlowCases は、@business.flow の
// 付いた op ごとに一つ。Requirements は、要求ごとに一つ。コンパイルできないことそのものは、リポジトリの plugins/go-design の
// TestIncompleteDoNotCompile が、生成したコードをコンパイルして確かめる。
func RunIncomplete(t *testing.T) {
	methods := func(typ reflect.Type) []string {
		var names []string
		for i := 0; i < typ.NumMethod(); i++ {
			names = append(names, typ.Method(i).Name)
		}
		return names
	}
	// フローごと
	cases := reflect.TypeOf((*flowstest.FlowCases)(nil)).Elem()
	if got, want := methods(cases), []string{"SignInAndOrder"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FlowCases のメソッド = %v, want %v", got, want)
	}
	if !reflect.TypeOf(FlowCases{}).Implements(cases) {
		t.Error("すべてのフローのケースを持つ型が、FlowCases を満たさない")
	}
	if reflect.TypeOf(withoutFlow{}).Implements(cases) {
		t.Error("フローのケースを欠いた型が、FlowCases を満たす")
	}
	// 要求ごと（reflect は、名前の順に並べる）
	containers := reflect.TypeOf((*flowstest.Requirements)(nil)).Elem()
	if got, want := methods(containers), []string{"PRD_001", "PRD_002"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Requirements のメソッド = %v, want %v", got, want)
	}
	if !reflect.TypeOf(Requirements{}).Implements(containers) {
		t.Error("すべての要求の入れ物を持つ型が、Requirements を満たさない")
	}
	if reflect.TypeOf(withoutPRD002{}).Implements(containers) {
		t.Error("PRD_002 の入れ物を欠いた型が、Requirements を満たす")
	}
}
