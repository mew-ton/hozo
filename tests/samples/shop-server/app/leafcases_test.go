package app

import (
	"reflect"
	"testing"
	"time"

	"example.com/shop-server/gen/server"
	"example.com/shop-server/gen/server/servertest"
	pkgtype "example.com/shop-server/gen/type"
)

// このファイルは、葉の op の、結果ごとの単体テスト（plugins/go/spec/go.hozo の Requirement.UnitTestPort）。基底が出した口（servertest.LeafCases）を
// 満たす型を書き、servertest.RunLeaves に渡す。葉を呼ぶ所と、結果の見分け（正常か、どの例外か）は、生成したコードが持つ。
// ここに書くのは、引数の値と、返り値の確かめだけ。葉に、例外を一つ足せば、足りないメソッドで、この型はコンパイルできなくなる。

// leafNow は、このテストの時計。
var leafNow = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

// leafCases は、servertest.LeafCases を満たす: 葉ごとに、結果ごとのケースを返す。
type leafCases struct{}

var _ servertest.LeafCases = leafCases{}

// leafAlice は、パスワードが "password123" のユーザー。
var leafAlice = pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: HashPassword("password123")}

// leafSession は、期限が、時計の一時間後のセッション。
var leafSession = pkgtype.Session{Token: "token-1", UserId: "u1", ExpiresAt: leafNow.Add(time.Hour)}

type checkPasswordCases struct{}

func (checkPasswordCases) OK() servertest.CheckPasswordCase {
	return servertest.CheckPasswordCase{User: leafAlice, Password: "password123"}
}
func (checkPasswordCases) InvalidCredentials() servertest.CheckPasswordCase {
	return servertest.CheckPasswordCase{User: leafAlice, Password: "wrong-password"}
}
func (leafCases) CheckPassword() servertest.CheckPasswordCases { return checkPasswordCases{} }

type newSessionCases struct{}

func (newSessionCases) OK() servertest.NewSessionCase {
	return servertest.NewSessionCase{User: leafAlice, Then: func(t *testing.T, got pkgtype.Session, _ error) {
		if got != leafSession {
			t.Errorf("newSession = %+v（期待は %+v）", got, leafSession)
		}
	}}
}
func (leafCases) NewSession() servertest.NewSessionCases { return newSessionCases{} }

type toPublicUserCases struct{}

func (toPublicUserCases) OK() servertest.ToPublicUserCase {
	return servertest.ToPublicUserCase{User: leafAlice, Then: func(t *testing.T, got pkgtype.PublicUser, _ error) {
		if want := (pkgtype.PublicUser{Id: "u1", Email: "alice@example.com", Name: "Alice"}); got != want {
			t.Errorf("toPublicUser = %+v（期待は %+v）", got, want)
		}
	}}
}
func (leafCases) ToPublicUser() servertest.ToPublicUserCases { return toPublicUserCases{} }

type requireFreshCases struct{}

func (requireFreshCases) OK() servertest.RequireFreshCase {
	return servertest.RequireFreshCase{Session: leafSession, Then: func(t *testing.T, got pkgtype.Session, _ error) {
		if got != leafSession {
			t.Errorf("requireFresh = %+v（期待は、同じセッション）", got)
		}
	}}
}
func (requireFreshCases) Unauthorized() servertest.RequireFreshCase {
	// 期限が、時計と同じ: もう、新しくない
	return servertest.RequireFreshCase{Session: pkgtype.Session{Token: "token-0", UserId: "u1", ExpiresAt: leafNow}}
}
func (leafCases) RequireFresh() servertest.RequireFreshCases { return requireFreshCases{} }

type pageOfCases struct{}

func (pageOfCases) OK() servertest.PageOfCase {
	return servertest.PageOfCase{Orders: []pkgtype.Order{{Id: "o1"}, {Id: "o2"}, {Id: "o3"}}, Page: 2,
		Then: func(t *testing.T, got []pkgtype.Order, _ error) {
			if len(got) != 1 || got[0].Id != "o3" {
				t.Errorf("pageOf(2) = %+v（期待は o3 だけ）", got)
			}
		}}
}
func (leafCases) PageOf() servertest.PageOfCases { return pageOfCases{} }

type summarizeCases struct{}

func (summarizeCases) OK() servertest.SummarizeCase {
	return servertest.SummarizeCase{Order: pkgtype.Order{Id: "o1", Lines: []pkgtype.OrderLine{{}, {}}},
		Then: func(t *testing.T, got server.OrderSummary, _ error) {
			if want := (server.OrderSummary{Id: "o1", Lines: 2}); got != want {
				t.Errorf("summarize = %+v（期待は %+v）", got, want)
			}
		}}
}
func (leafCases) Summarize() servertest.SummarizeCases { return summarizeCases{} }

type myPageCases struct{}

func (myPageCases) OK() servertest.MyPageCase {
	user := pkgtype.PublicUser{Id: "u1", Email: "alice@example.com", Name: "Alice"}
	summaries := []server.OrderSummary{{Id: "o1", Lines: 2}}
	return servertest.MyPageCase{User: user, Latest: nil, Summaries: summaries,
		Then: func(t *testing.T, got server.MyPage, _ error) {
			if got.User != user || got.Latest != nil || len(got.Summaries) != 1 || got.Summaries[0] != summaries[0] {
				t.Errorf("myPage = %+v", got)
			}
		}}
}
func (leafCases) MyPage() servertest.MyPageCases { return myPageCases{} }

type newOrderCases struct{}

func (newOrderCases) OK() servertest.NewOrderCase {
	return servertest.NewOrderCase{Session: leafSession, Lines: nil,
		Then: func(t *testing.T, got pkgtype.Order, _ error) {
			if got.Id != "order-1" || got.UserId != "u1" || got.Lines == nil || !got.PlacedAt.Equal(leafNow) {
				t.Errorf("newOrder = %+v", got)
			}
		}}
}
func (leafCases) NewOrder() servertest.NewOrderCases { return newOrderCases{} }

type composeNoticeCases struct{}

func (composeNoticeCases) OK() servertest.ComposeNoticeCase {
	return servertest.ComposeNoticeCase{Order: pkgtype.Order{Id: "order-1", UserId: "u1"},
		Then: func(t *testing.T, got pkgtype.Notification, _ error) {
			if want := (pkgtype.Notification{UserId: "u1", Message: "注文 order-1 を受け付けました"}); got != want {
				t.Errorf("composeNotice = %+v（期待は %+v）", got, want)
			}
		}}
}
func (leafCases) ComposeNotice() servertest.ComposeNoticeCases { return composeNoticeCases{} }

type recordAuditCases struct{}

func (recordAuditCases) OK() servertest.RecordAuditCase {
	return servertest.RecordAuditCase{Then: func(t *testing.T, _ error) {
		if audited != 1 {
			t.Errorf("記録の先が呼ばれた回数 = %d（期待は、一度）", audited)
		}
	}}
}
func (leafCases) RecordAudit() servertest.RecordAuditCases { return recordAuditCases{} }

// audited は、TestLeafCases の Leaves の、記録の先（Audit）が呼ばれた回数。
var audited int

// TestLeafCases は、葉ごと、結果ごとのケースを、すべて走らせる。テストの名前は、<op の FQN>/<結果>
// （TestLeafCases/server.checkPassword/InvalidCredentials など）。
func TestLeafCases(t *testing.T) {
	leaves := &Leaves{
		Now:        func() time.Time { return leafNow },
		NewToken:   func() (pkgtype.Token, error) { return "token-1", nil },
		NewOrderID: func() (pkgtype.OrderId, error) { return "order-1", nil },
		SessionTTL: time.Hour,
		PageSize:   2,
		Audit:      func() { audited++ },
	}
	servertest.RunLeaves(t, leaves, leafCases{})
}

// withoutComposeNotice は、LeafCases から、葉 composeNotice のケースを欠いた口。
// missingResult は、checkPasswordCases から、例外 InvalidCredentials のケースを欠いた型。
type (
	withoutComposeNotice interface {
		CheckPassword() servertest.CheckPasswordCases
		NewSession() servertest.NewSessionCases
		ToPublicUser() servertest.ToPublicUserCases
		RequireFresh() servertest.RequireFreshCases
		PageOf() servertest.PageOfCases
		Summarize() servertest.SummarizeCases
		MyPage() servertest.MyPageCases
		NewOrder() servertest.NewOrderCases
		RecordAudit() servertest.RecordAuditCases
	}
	missingResult struct{}
)

func (missingResult) OK() servertest.CheckPasswordCase { return checkPasswordCases{}.OK() }

// TestIncompleteCasesAreRejected は、ケースを欠いた型が、口を満たさないこと。欠いた型を渡す行
// （servertest.RunLeaves(t, leaves, <葉を一つ欠いた型>)）は、コンパイルできないので、ここには書けない。代わりに、口が求める
// メソッドを、リフレクションで確かめる: LeafCases は、葉ごとに一つ（server.Leaves と同じ名前、同じ数）。<Op>Cases は、
// 結果ごとに一つ（正常と、throws の例外ごと）。コンパイルできないことそのものは、リポジトリの plugins/go の
// TestIncompleteCasesDoNotCompile が、生成したコードをコンパイルして確かめる。
func TestIncompleteCasesAreRejected(t *testing.T) {
	cases := reflect.TypeOf((*servertest.LeafCases)(nil)).Elem()
	leaves := reflect.TypeOf((*server.Leaves)(nil)).Elem()
	if cases.NumMethod() != leaves.NumMethod() {
		t.Fatalf("LeafCases のメソッドは %d、Leaves のメソッドは %d", cases.NumMethod(), leaves.NumMethod())
	}
	for i := 0; i < leaves.NumMethod(); i++ {
		if _, ok := cases.MethodByName(leaves.Method(i).Name); !ok {
			t.Errorf("葉 %s のケースを、LeafCases が求めない", leaves.Method(i).Name)
		}
	}
	if !reflect.TypeOf(leafCases{}).Implements(cases) {
		t.Error("すべての葉のケースを持つ型が、LeafCases を満たさない")
	}
	if reflect.TypeOf((*withoutComposeNotice)(nil)).Elem().Implements(cases) {
		t.Error("composeNotice のケースを欠いた口の型が、LeafCases を満たす")
	}

	// 結果ごと: checkPassword は、正常と、例外 type.InvalidCredentials
	results := reflect.TypeOf((*servertest.CheckPasswordCases)(nil)).Elem()
	var names []string
	for i := 0; i < results.NumMethod(); i++ {
		names = append(names, results.Method(i).Name)
	}
	if want := []string{"InvalidCredentials", "OK"}; !reflect.DeepEqual(names, want) {
		t.Errorf("CheckPasswordCases のメソッド = %v, want %v", names, want)
	}
	if reflect.TypeOf(missingResult{}).Implements(results) {
		t.Error("InvalidCredentials のケースを欠いた型が、CheckPasswordCases を満たす")
	}
	if !reflect.TypeOf(checkPasswordCases{}).Implements(results) {
		t.Error("二つの結果のケースを持つ型が、CheckPasswordCases を満たさない")
	}
}
