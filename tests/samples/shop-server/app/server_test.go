package app_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"example.com/shop-server/app/handlertest"
	"example.com/shop-server/gen/rate"
	"example.com/shop-server/gen/server"
	"example.com/shop-server/gen/server/inner"
	pkgtype "example.com/shop-server/gen/type"
)

// 生成されたメソッドの形。インターフェースのメソッド式（最初の引数がそのインターフェース）を、関数の型に代入できることで確かめる。
//
//   - ctx: どの op も、最初の引数に取る。
//   - error: どの op も、最後の戻り値に返す。
//
// 形は一通り（docs/plugin-go.md「op の形」）: 純粋かどうか（デコレータ、reads、effects の有無）でも、throws の有無でも、
// 葉か配線かでも、自分の名前空間の op か別の名前空間の op（MailerOps、StoreOps）かでも、変わらない。
//
// 入口のパッケージ（gen/server）から形を書けるのは、export を付けた op（server.Ops）、人が実装する葉（server.Leaves）、
// 別の名前空間の op（server.MailerOps、server.StoreOps）、継ぎ目（server.Seams）。内部の配線（issueSession、authenticate、
// sessionOf）は、内側のパッケージ（gen/server/inner）の Ops の公開のメソッドで、テストはそこから名指しできる。
var (
	// 仕様の上で純粋で、throws がない葉: それでも、ctx も error もある
	_ func(server.Leaves, context.Context, pkgtype.User) (pkgtype.Session, error)                       = server.Leaves.NewSession
	_ func(server.Leaves, context.Context, pkgtype.User) (pkgtype.PublicUser, error)                    = server.Leaves.ToPublicUser
	_ func(server.Leaves, context.Context, []pkgtype.Order, int64) ([]pkgtype.Order, error)             = server.Leaves.PageOf
	_ func(server.Leaves, context.Context, pkgtype.Session, []pkgtype.OrderLine) (pkgtype.Order, error) = server.Leaves.NewOrder
	_ func(server.Leaves, context.Context, pkgtype.Order) (pkgtype.Notification, error)                 = server.Leaves.ComposeNotice
	// 仕様の上で純粋で、throws がある葉: 同じ
	_ func(server.Leaves, context.Context, pkgtype.User, pkgtype.Password) error     = server.Leaves.CheckPassword
	_ func(server.Leaves, context.Context, pkgtype.Session) (pkgtype.Session, error) = server.Leaves.RequireFresh
	// 仕様の上で純粋でない op（デコレータを持つ、状態に触れる op を呼ぶ）: 同じ
	_ func(server.Ops, context.Context, server.LoginInput) (server.LoginOk, error)        = server.Ops.Login
	_ func(server.Ops, context.Context, server.AuthInput) error                           = server.Ops.Logout
	_ func(server.Ops, context.Context, server.AuthInput) (pkgtype.PublicUser, error)     = server.Ops.GetMe
	_ func(server.Ops, context.Context, server.GetOrdersInput) ([]pkgtype.Order, error)   = server.Ops.GetOrders
	_ func(server.Ops, context.Context, server.AuthInput) (*pkgtype.Order, error)         = server.Ops.GetLatestOrder
	_ func(server.Ops, context.Context, server.PlaceOrderInput) (pkgtype.Order, error)    = server.Ops.PlaceOrder
	_ func(server.Ops, context.Context, server.AuthInput) (server.MyPage, error)          = server.Ops.GetMyPage
	_ func(server.Ops, context.Context, server.AuthInput) ([]pkgtype.Notification, error) = server.Ops.ResendNotices
	_ func(server.MailerOps, context.Context, pkgtype.Notification) error                 = server.MailerOps.Send
	_ func(server.StoreOps, context.Context, pkgtype.Token) (*pkgtype.Session, error)     = server.StoreOps.FindSession
	// 二つの効果を呼ぶ配線（store.placeOrder）も、引数の並びは op の宣言のまま
	_ func(server.StoreOps, context.Context, pkgtype.Order, pkgtype.Notification) error = server.StoreOps.PlaceOrder
	// 内部の配線も同じ形
	_ func(inner.Ops, context.Context, pkgtype.User, pkgtype.Password) (server.LoginOk, error) = inner.Ops.IssueSession
	_ func(inner.Ops, context.Context, *pkgtype.Token) (pkgtype.Session, error)                = inner.Ops.Authenticate
	_ func(inner.Ops, context.Context, pkgtype.Token) (pkgtype.Session, error)                 = inner.Ops.SessionOf
	// 継ぎ目: op の引数（ctx と in）の後ろに、op と同じ形の body を取り、op と同じものを返す。
	// 継ぎ目も body も、op と同じに、いつも ctx を取り、error を返す
	_ func(server.Seams, context.Context, server.LoginInput, func(context.Context, server.LoginInput) (server.LoginOk, error)) (server.LoginOk, error) = server.Seams.LoginRateLimited
)

// 入口の型は、内側の型の別名（同じ型）。入口の New が受け取る口は、内側の New にも、そのまま渡せる。
var (
	_ inner.Leaves    = server.Leaves(nil)
	_ inner.MailerOps = server.MailerOps(nil)
	_ inner.StoreOps  = server.StoreOps(nil)
	_ inner.Seams     = server.Seams(nil)
	_ inner.Seams     = server.RateSeams{} // go-ratelimit が入口のパッケージに出した型は、内側の Seams も満たす
)

// ctxKey は、要求の context を見分けるために、テストが値を載せる鍵（生成されたコードは context に値を載せない）。
type ctxKey struct{}

// seenStore は、server が呼ぶ store の op（server.StoreOps）のうち、二つだけを書いたもの。届いた context を覚える。
// 書いていない op は、埋め込んだ nil のインターフェースのまま（呼べば panic）。構造体のフィールドに関数を一つずつ入れる形は
// もうないので、op を一つだけ差し替えるにも、型を一つ書く。
type seenStore struct {
	server.StoreOps
	seen []context.Context
}

func (s *seenStore) FindSession(ctx context.Context, token pkgtype.Token) (*pkgtype.Session, error) {
	s.seen = append(s.seen, ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &pkgtype.Session{Token: token, UserId: "u1", ExpiresAt: handlertest.Now.Add(time.Hour)}, nil
}

func (s *seenStore) FindUserById(ctx context.Context, id pkgtype.UserId) (*pkgtype.User, error) {
	s.seen = append(s.seen, ctx)
	return &pkgtype.User{Id: id, Email: "alice@example.com", Name: "Alice", Hash: "h"}, nil
}

// TestRequestContextReachesStore は、ハンドラが受けた要求の context が、配線を通って、別の名前空間の op
// （server.StoreOps）にそのまま届くこと。server は store のどの出力もなしに、インターフェースを満たす値を渡すだけで動く。
func TestRequestContextReachesStore(t *testing.T) {
	t.Parallel()
	f := &handlertest.Fixture{}
	st := &seenStore{}
	handler := server.NewHandler(server.New(f.Leaves(), f.Mailer(), st, server.NewRateSeams(f.Limiter())))

	get := func(ctx context.Context) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/me", nil).WithContext(ctx)
		req.Header.Set("Authorization", "token-1")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	// 生きている要求: 二つの op（findSession、findUserById）に、要求の context が届く
	rec := get(context.WithValue(context.Background(), ctxKey{}, "request-1"))
	if rec.Code != 200 || rec.Body.String() != `{"id":"u1","email":"alice@example.com","name":"Alice"}` {
		t.Fatalf("GET /me: %d %s", rec.Code, rec.Body)
	}
	if len(st.seen) != 2 || st.seen[0].Value(ctxKey{}) != "request-1" || st.seen[1].Value(ctxKey{}) != "request-1" {
		t.Errorf("store の op に届いた context が、要求のものではない: %v", st.seen)
	}

	// 中断された要求: op は中断を見て、そのエラーを返す。後ろの op は呼ばれない
	st.seen = nil
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "request-2"))
	cancel()
	rec = get(ctx)
	if rec.Code != 500 || rec.Body.String() != `{"error":"internal"}` {
		t.Errorf("中断された GET /me: %d %s", rec.Code, rec.Body)
	}
	if len(st.seen) != 1 || st.seen[0].Value(ctxKey{}) != "request-2" || !errors.Is(st.seen[0].Err(), context.Canceled) {
		t.Errorf("中断された要求で store の op に届いた context: %v", st.seen)
	}
}

// fixedSession は、葉の op を一つだけ差し替えた server.Leaves: 埋め込んだ実装のうち、NewSession を置き換える。
type fixedSession struct{ server.Leaves }

func (fixedSession) NewSession(_ context.Context, user pkgtype.User) (pkgtype.Session, error) {
	return pkgtype.Session{Token: "差し替えたトークン", UserId: user.Id, ExpiresAt: handlertest.Now.Add(time.Minute)}, nil
}

// sessionLog は、内部の配線 issueSession が呼ぶ別の名前空間の op を書いたもの: store.addSession。
// ほかの op は、埋め込んだ nil のインターフェースのまま。
type sessionLog struct {
	server.StoreOps
	added []pkgtype.Session
}

func (l *sessionLog) AddSession(_ context.Context, session pkgtype.Session) error {
	l.added = append(l.added, session)
	return nil
}

// TestReplaceOneLeaf は、テストが葉の op を一つだけ差し替える形: 実装を埋め込んだ型に、そのメソッドだけを書く。
// 内部の配線 issueSession は、差し替えた newSession の結果を、store.addSession に渡し、そのトークンを返す。
//
// issueSession は export のない op なので、入口の server.Ops にはない。内側のパッケージの New が返す Ops（すべての op）から、
// 直に呼ぶ。issueSession が通らない op（store.findSession、mailer.send、葉 newOrder など）は、用意しなくてよい。
func TestReplaceOneLeaf(t *testing.T) {
	t.Parallel()
	f := &handlertest.Fixture{}
	log := &sessionLog{}
	// mailer を通らず、継ぎ目を持つ op（login）も通らないので、mailer と seams は渡さない（nil。使われないものは、確かめられない）
	ops := inner.New(fixedSession{f.Leaves()}, nil, log, nil)
	ok, err := ops.IssueSession(context.Background(), handlertest.Alice(), handlertest.AlicePassword)
	if err != nil {
		t.Fatal(err)
	}
	want := pkgtype.Session{Token: "差し替えたトークン", UserId: handlertest.AliceID, ExpiresAt: handlertest.Now.Add(time.Minute)}
	if ok.Token != want.Token || len(log.added) != 1 || log.added[0] != want {
		t.Errorf("IssueSession のトークンが %q、store.addSession に %v（期待は、%v が一つ）", ok.Token, log.added, want)
	}
	// 差し替えていない葉は、埋め込んだ実装のまま。葉も、内側の Ops から直に呼べる
	if order, err := ops.NewOrder(context.Background(), pkgtype.Session{UserId: "u1"}, nil); err != nil || order.Id != "order-1" || order.UserId != "u1" {
		t.Errorf("NewOrder = %+v, %v", order, err)
	}
}

// noToken は、葉 newSession が、仕様の例外でない error を返す server.Leaves（乱数を読めなかった、など）。
type noToken struct {
	server.Leaves
	err error
}

func (l noToken) NewSession(context.Context, pkgtype.User) (pkgtype.Session, error) {
	return pkgtype.Session{}, l.err
}

// TestLeafErrorIsReturned は、throws を持たない葉（newSession）が返した error が、配線 issueSession を通って、包まれずに
// 呼び出し元に届くこと。その後ろの行（store.addSession）は呼ばれない。葉は、仕様の上で純粋でも、error を返す形を持つ
// （docs/plugin-go.md「op の形」）ので、失敗を panic にしなくてよい。
func TestLeafErrorIsReturned(t *testing.T) {
	t.Parallel()
	f := &handlertest.Fixture{}
	log := &sessionLog{}
	failed := errors.New("トークンを作れない")
	ops := inner.New(noToken{f.Leaves(), failed}, nil, log, nil)
	if _, err := ops.IssueSession(context.Background(), handlertest.Alice(), handlertest.AlicePassword); err != failed {
		t.Errorf("IssueSession = %v（期待は、葉が返した error そのもの）", err)
	}
	if len(log.added) != 0 {
		t.Errorf("葉が失敗したのに、store.addSession が呼ばれた: %v", log.added)
	}
}

// orderLog は、配線 placeOrder が呼ぶ別の名前空間の op を、呼ばれた順に覚える: store.findSession、store.placeOrder、mailer.send。
// server.MailerOps と、server.StoreOps（ほかの op は、埋め込んだ nil のインターフェースのまま）の両方として渡す。
type orderLog struct {
	server.StoreOps
	calls    []string
	order    pkgtype.Order
	notice   pkgtype.Notification
	sent     pkgtype.Notification
	failSend bool
}

func (l *orderLog) FindSession(_ context.Context, token pkgtype.Token) (*pkgtype.Session, error) {
	l.calls = append(l.calls, "store.findSession")
	return &pkgtype.Session{Token: token, UserId: "u1", ExpiresAt: handlertest.Now.Add(time.Hour)}, nil
}

func (l *orderLog) PlaceOrder(_ context.Context, order pkgtype.Order, notification pkgtype.Notification) error {
	l.calls = append(l.calls, "store.placeOrder")
	l.order, l.notice = order, notification
	return nil
}

func (l *orderLog) Send(_ context.Context, notice pkgtype.Notification) error {
	l.calls = append(l.calls, "mailer.send")
	l.sent = notice
	if l.failSend {
		return pkgtype.NotificationFailed{}
	}
	return nil
}

// TestPlaceOrderRecordsThenSends は、配線 placeOrder の順序: 注文と通知の記録を、一つの op（store.placeOrder。二つの効果）に
// 一緒に渡し、その後で配送する（mailer.send）。配送に失敗しても、store.placeOrder は済んでいる（仕様の「どちらも残る」）。
// 二つの記録が一緒に残ることそのもの（一つのトランザクション）は、store の実装の仕事で、ここでは確かめない
// （onsqlite の TestPlaceOrderIsOneTransaction と、storetest の placeOrder）。
func TestPlaceOrderRecordsThenSends(t *testing.T) {
	t.Parallel()
	for _, failSend := range []bool{false, true} {
		f := &handlertest.Fixture{}
		log := &orderLog{failSend: failSend}
		ops := server.New(f.Leaves(), log, log, nil)
		token := pkgtype.Token("token-1")
		order, err := ops.PlaceOrder(context.Background(), server.PlaceOrderInput{Token: &token, Lines: []pkgtype.OrderLine{{Item: "apple", Quantity: 1}}})
		var failed pkgtype.NotificationFailed
		if failSend != errors.As(err, &failed) || (!failSend && err != nil) {
			t.Fatalf("PlaceOrder（配送の失敗 %v）= %+v, %v", failSend, order, err)
		}
		if want := []string{"store.findSession", "store.placeOrder", "mailer.send"}; !slices.Equal(log.calls, want) {
			t.Errorf("呼ばれた順が %v（期待は %v）", log.calls, want)
		}
		if log.order.Id != "order-1" || log.notice.UserId != "u1" || log.sent != log.notice {
			t.Errorf("store.placeOrder に注文 %+v と通知 %+v、mailer.send に %+v（期待は、同じ通知）", log.order, log.notice, log.sent)
		}
	}
}

// pageStore は、配線 getMyPage と resendNotices が呼ぶ別の名前空間の op。呼ばれた順と、記録した通知を覚える。
// server.MailerOps と、server.StoreOps（ほかの op は、埋め込んだ nil のインターフェースのまま）の両方として渡す。
type pageStore struct {
	server.StoreOps
	orders   []pkgtype.Order
	noUser   bool
	failSend pkgtype.OrderId // この注文の通知は、送れない
	sendErr  error           // 送れないときに返す失敗。nil なら、仕様の例外 NotificationFailed
	calls    []string
	recorded []pkgtype.Notification
}

func (p *pageStore) FindSession(_ context.Context, token pkgtype.Token) (*pkgtype.Session, error) {
	p.calls = append(p.calls, "store.findSession")
	return &pkgtype.Session{Token: token, UserId: "u1", ExpiresAt: handlertest.Now.Add(time.Hour)}, nil
}

func (p *pageStore) FindUserById(_ context.Context, id pkgtype.UserId) (*pkgtype.User, error) {
	p.calls = append(p.calls, "store.findUserById")
	if p.noUser {
		return nil, nil
	}
	return &pkgtype.User{Id: id, Email: "alice@example.com", Name: "Alice", Hash: "h"}, nil
}

func (p *pageStore) LatestOrder(context.Context, pkgtype.UserId) (*pkgtype.Order, error) {
	p.calls = append(p.calls, "store.latestOrder")
	if len(p.orders) == 0 {
		return nil, nil
	}
	return &p.orders[len(p.orders)-1], nil
}

func (p *pageStore) ListOrders(context.Context, pkgtype.UserId) ([]pkgtype.Order, error) {
	p.calls = append(p.calls, "store.listOrders")
	return p.orders, nil
}

func (p *pageStore) RecordNotice(_ context.Context, notification pkgtype.Notification) error {
	p.calls = append(p.calls, "store.recordNotice")
	p.recorded = append(p.recorded, notification)
	return nil
}

func (p *pageStore) Send(_ context.Context, notice pkgtype.Notification) error {
	p.calls = append(p.calls, "mailer.send")
	if p.failSend != "" && strings.Contains(notice.Message, string(p.failSend)) {
		if p.sendErr != nil {
			return p.sendErr
		}
		return pkgtype.NotificationFailed{}
	}
	return nil
}

// threeOrders は、行の数が 1、2、3 の、三つの注文。
func threeOrders() []pkgtype.Order {
	line := pkgtype.OrderLine{Item: "apple", Quantity: 1}
	return []pkgtype.Order{
		{Id: "order-a", UserId: "u1", Lines: []pkgtype.OrderLine{line}},
		{Id: "order-b", UserId: "u1", Lines: []pkgtype.OrderLine{line, line}},
		{Id: "order-c", UserId: "u1", Lines: []pkgtype.OrderLine{line, line, line}},
	}
}

// TestGetMyPageGathers は、配線 getMyPage（disjoint と、disjoint for）: 三つの読みの結果が、葉 myPage に渡り、
// 注文ごとの要約は、注文の並びの順に、集まる。基底 go は、disjoint を、書いた順に、直列に出す（docs/plugin-go.md「文」）ので、
// 呼ばれる順も、仕様に書いた順。
func TestGetMyPageGathers(t *testing.T) {
	t.Parallel()
	f := &handlertest.Fixture{}
	store := &pageStore{orders: threeOrders()}
	ops := server.New(f.Leaves(), store, store, nil)
	token := pkgtype.Token("token-1")
	page, err := ops.GetMyPage(context.Background(), server.AuthInput{Token: &token})
	if err != nil {
		t.Fatalf("GetMyPage = %v", err)
	}
	if page.User.Id != "u1" || page.Latest == nil || page.Latest.Id != "order-c" {
		t.Errorf("GetMyPage の user と latest が %+v、%+v", page.User, page.Latest)
	}
	want := []server.OrderSummary{{Id: "order-a", Lines: 1}, {Id: "order-b", Lines: 2}, {Id: "order-c", Lines: 3}}
	if !slices.Equal(page.Summaries, want) {
		t.Errorf("要約が %+v（期待は、注文の順の %+v）", page.Summaries, want)
	}
	if want := []string{"store.findSession", "store.findUserById", "store.latestOrder", "store.listOrders"}; !slices.Equal(store.calls, want) {
		t.Errorf("呼ばれた順が %v（期待は %v）", store.calls, want)
	}
}

// TestGetMyPageWithoutOrders は、注文がなければ、いちばん新しい注文は、値がなく、要約は、空の並び（繰り返しは、一回も回らない）。
func TestGetMyPageWithoutOrders(t *testing.T) {
	t.Parallel()
	f := &handlertest.Fixture{}
	store := &pageStore{}
	ops := server.New(f.Leaves(), store, store, nil)
	token := pkgtype.Token("token-1")
	page, err := ops.GetMyPage(context.Background(), server.AuthInput{Token: &token})
	if err != nil || page.Latest != nil || len(page.Summaries) != 0 {
		t.Errorf("GetMyPage = %+v, %v（期待は、latest がなく、要約が空）", page, err)
	}
}

// TestGetMyPageStopsAtException は、disjoint のアームの中の例外（ユーザーがいない: Unauthorized）で、op が終わること。
// 後ろのアーム（二つの読み）は、動かない。
func TestGetMyPageStopsAtException(t *testing.T) {
	t.Parallel()
	f := &handlertest.Fixture{}
	store := &pageStore{orders: threeOrders(), noUser: true}
	ops := server.New(f.Leaves(), store, store, nil)
	token := pkgtype.Token("token-1")
	_, err := ops.GetMyPage(context.Background(), server.AuthInput{Token: &token})
	var unauthorized pkgtype.Unauthorized
	if !errors.As(err, &unauthorized) {
		t.Fatalf("GetMyPage = %v（期待は Unauthorized）", err)
	}
	if want := []string{"store.findSession", "store.findUserById"}; !slices.Equal(store.calls, want) {
		t.Errorf("呼ばれた順が %v（期待は %v）", store.calls, want)
	}
}

// TestResendNoticesInOrder は、配線 resendNotices（直列の for）: 注文の順に、送って、記録して、集める。
func TestResendNoticesInOrder(t *testing.T) {
	t.Parallel()
	f := &handlertest.Fixture{}
	store := &pageStore{orders: threeOrders()}
	ops := server.New(f.Leaves(), store, store, nil)
	token := pkgtype.Token("token-1")
	sent, err := ops.ResendNotices(context.Background(), server.AuthInput{Token: &token})
	if err != nil || len(sent) != 3 || !slices.Equal(sent, store.recorded) {
		t.Fatalf("ResendNotices = %+v, %v（記録は %+v）", sent, err, store.recorded)
	}
	for i, id := range []string{"order-a", "order-b", "order-c"} {
		if !strings.Contains(sent[i].Message, id) {
			t.Errorf("%d 番目の通知が %q（期待は、注文 %s のもの）", i, sent[i].Message, id)
		}
	}
	want := []string{"store.findSession", "store.listOrders", "mailer.send", "store.recordNotice", "mailer.send", "store.recordNotice", "mailer.send", "store.recordNotice"}
	if !slices.Equal(store.calls, want) {
		t.Errorf("呼ばれた順が %v（期待は %v）", store.calls, want)
	}
}

// TestResendNoticesStopsAtFailure は、一つの回が失敗したら、繰り返しが、そこで止まること（docs/check.md「繰り返し」）:
// 二つ目の注文の通知を送れないと、三つ目は、送らない。一つ目の記録は、残る（言語は、起きた効果を、取り消さない）。
func TestResendNoticesStopsAtFailure(t *testing.T) {
	t.Parallel()
	f := &handlertest.Fixture{}
	store := &pageStore{orders: threeOrders(), failSend: "order-b"}
	ops := server.New(f.Leaves(), store, store, nil)
	token := pkgtype.Token("token-1")
	sent, err := ops.ResendNotices(context.Background(), server.AuthInput{Token: &token})
	var failed pkgtype.NotificationFailed
	if !errors.As(err, &failed) || sent != nil {
		t.Fatalf("ResendNotices = %+v, %v（期待は NotificationFailed）", sent, err)
	}
	if len(store.recorded) != 1 || !strings.Contains(store.recorded[0].Message, "order-a") {
		t.Errorf("記録が %+v（期待は、一つ目の注文の通知だけ）", store.recorded)
	}
	want := []string{"store.findSession", "store.listOrders", "mailer.send", "store.recordNotice", "mailer.send"}
	if !slices.Equal(store.calls, want) {
		t.Errorf("呼ばれた順が %v（期待は %v）", store.calls, want)
	}
}

// auditLeaves は、葉 recordAudit の呼び出しを数える server.Leaves。ほかの葉は、本物のまま。
type auditLeaves struct {
	server.Leaves
	audits int
}

func (l *auditLeaves) RecordAudit(context.Context) error {
	l.audits++
	return nil
}

// TestResendNoticesCatch は、配線 resendNotices の、例外を受ける文（docs/check.md「例外を受ける」。
// try { mailer.send(notice); } catch (e) { recordAudit(); throw e; }）:
//
//   - 配送が、仕様の例外（NotificationFailed）で失敗したら、catch のブロックが、葉 recordAudit を、一度呼んでから、
//     受けた例外を、そのまま投げ直す（包み直さない）。
//   - 仕様にない失敗は、受けない: recordAudit を呼ばずに、その失敗が、そのまま返る。
//   - 配送が、失敗しなければ、catch のブロックは、動かない。
func TestResendNoticesCatch(t *testing.T) {
	t.Parallel()
	broken := errors.New("SMTP の接続が切れた")
	for _, tc := range []struct {
		name     string
		failSend pkgtype.OrderId
		sendErr  error
		want     error
		audits   int
	}{
		{"仕様の例外は、受けて、記録して、投げ直す", "order-b", nil, pkgtype.NotificationFailed{}, 1},
		{"仕様にない失敗は、受けない", "order-b", broken, broken, 0},
		{"失敗しなければ、catch は動かない", "", nil, nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &handlertest.Fixture{}
			leaves := &auditLeaves{Leaves: f.Leaves()}
			store := &pageStore{orders: threeOrders(), failSend: tc.failSend, sendErr: tc.sendErr}
			ops := server.New(leaves, store, store, nil)
			token := pkgtype.Token("token-1")
			_, err := ops.ResendNotices(context.Background(), server.AuthInput{Token: &token})
			if err != tc.want {
				t.Errorf("ResendNotices の失敗 = %v（期待は %v。受けた例外は、そのまま返る）", err, tc.want)
			}
			if leaves.audits != tc.audits {
				t.Errorf("recordAudit の呼び出し = %d 回（期待は %d 回）", leaves.audits, tc.audits)
			}
		})
	}
}

// spyOps は、呼ばれた回数を数えるだけの server.Ops（go-http が受け取る、export を付けた八つの op）。
// 数えない六つは、埋め込んだ nil のインターフェースのまま（呼べば panic）。
type spyOps struct {
	server.Ops
	logins, placeOrders int
}

func (s *spyOps) Login(context.Context, server.LoginInput) (server.LoginOk, error) {
	s.logins++
	return server.LoginOk{}, nil
}

func (s *spyOps) PlaceOrder(context.Context, server.PlaceOrderInput) (pkgtype.Order, error) {
	s.placeOrders++
	return pkgtype.Order{}, nil
}

// TestValidationPrecedesOp は、go-http（validate = true）が、読めない要求と制約に合わない入力では op を呼ばないこと。
func TestValidationPrecedesOp(t *testing.T) {
	t.Parallel()
	spy := &spyOps{}
	handler := server.NewHandler(spy)
	post := func(path, body string) int {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return rec.Code
	}
	for _, c := range []struct {
		path, body string
		status     int
	}{
		{"/login", `{"email":"not-an-email","password":"password123"}`, 400},
		{"/login", `{"email":"alice@example.com","password":"short"}`, 400},
		{"/login", `{"email":`, 400},
		{"/orders", `{"lines":[{"item":"apple","quantity":0}]}`, 400},
		{"/orders", `{"lines":{}}`, 400},
		{"/orders", `[{"item":"apple","quantity":1}]`, 400},
	} {
		if got := post(c.path, c.body); got != c.status {
			t.Errorf("POST %s %s: %d（期待は %d）", c.path, c.body, got, c.status)
		}
	}
	if spy.logins != 0 || spy.placeOrders != 0 {
		t.Fatalf("op が呼ばれた: login %d 回、placeOrder %d 回", spy.logins, spy.placeOrders)
	}
	// 制約に合う入力なら、op を呼ぶ
	if got := post("/login", `{"email":"alice@example.com","password":"password123"}`); got != 200 {
		t.Errorf("POST /login: %d", got)
	}
	if got := post("/orders", `{"lines":[{"item":"apple","quantity":1}]}`); got != 200 {
		t.Errorf("POST /orders: %d", got)
	}
	if spy.logins != 1 || spy.placeOrders != 1 {
		t.Errorf("op が呼ばれた回数: login %d 回、placeOrder %d 回（期待は 1 回ずつ）", spy.logins, spy.placeOrders)
	}
}

// TestBodyTooLarge は、request body が上限（オプション maxBodyBytes の既定、1 MiB）を超えた要求を、op を呼ばずに 413 で断り、
// 本文が http.BadRequest の "body: too large" であること（go-http）。上限ちょうどの本文は、読む（JSON として読めないので 400）。
func TestBodyTooLarge(t *testing.T) {
	t.Parallel()
	spy := &spyOps{}
	handler := server.NewHandler(spy)
	post := func(size int) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(strings.Repeat(" ", size))))
		return rec
	}
	rec := post(1<<20 + 1)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("上限を超えた本文: %d（期待は 413）", rec.Code)
	}
	if want := `{"error":"http.BadRequest","value":{"message":"body: too large","violations":[]}}`; strings.TrimSpace(rec.Body.String()) != want {
		t.Errorf("上限を超えた本文の応答 = %s, want %s", rec.Body.String(), want)
	}
	if rec := post(1 << 20); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "body: invalid JSON") {
		t.Errorf("上限ちょうどの本文: %d %s（期待は 400 body: invalid JSON）", rec.Code, rec.Body.String())
	}
	if spy.placeOrders != 0 {
		t.Fatalf("op が呼ばれた: placeOrder %d 回", spy.placeOrders)
	}
}

// seenInputs は、渡された入力を覚えるだけの server.Ops。書いていない op は、埋め込んだ nil のインターフェースのまま。
type seenInputs struct {
	server.Ops
	logouts     []server.AuthInput
	getOrders   []server.GetOrdersInput
	placeOrders []server.PlaceOrderInput
}

func (s *seenInputs) GetOrders(_ context.Context, in server.GetOrdersInput) ([]pkgtype.Order, error) {
	s.getOrders = append(s.getOrders, in)
	return nil, nil
}

func (s *seenInputs) Logout(_ context.Context, in server.AuthInput) error {
	s.logouts = append(s.logouts, in)
	return nil
}

func (s *seenInputs) PlaceOrder(_ context.Context, in server.PlaceOrderInput) (pkgtype.Order, error) {
	s.placeOrders = append(s.placeOrders, in)
	return pkgtype.Order{}, nil
}

// TestMissingItems は、要求に項目がないときの扱いが、フィールドのデコレータだけで決まること（go-http）:
// @http.required の項目（PlaceOrderInput の lines）がなければ、op を呼ばずに 400。
// @http.default(1) の項目（GetOrdersInput の page）がなければ、op に 1 が渡る。
// どちらもない項目（どの入力でも token）がなければ、op に零値（省略可能なので nil）が渡る。
func TestMissingItems(t *testing.T) {
	t.Parallel()
	seen := &seenInputs{}
	handler := server.NewHandler(seen)

	// @http.default の項目: なければ、デコレータの値。あれば、読んだ値。空の値は「ある」ので、既定値にはならず、読めない要求
	for _, c := range []struct {
		target string
		status int
	}{{"/orders", 200}, {"/orders?p=2", 200}, {"/orders?page=3", 200}, {"/orders?page=", 400}} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.target, nil))
		if rec.Code != c.status {
			t.Errorf("GET %s: %d %s（期待は %d）", c.target, rec.Code, rec.Body, c.status)
		}
	}
	if len(seen.getOrders) != 3 {
		t.Fatalf("GetOrders に渡った入力: %+v（期待は三つ）", seen.getOrders)
	}
	for i, page := range []int64{1, 1, 3} {
		if in := seen.getOrders[i]; in.Page != page || in.Token != nil {
			t.Errorf("GetOrders に渡った %d 番目の入力: %+v（期待は、Page が %d で、Token が nil）", i+1, in, page)
		}
	}
	post := func(path, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", token)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	// @http.required の項目がない: 400 で、op は呼ばれない。トークンの有無によらない
	for _, token := range []string{"", "token-1"} {
		rec := post("/orders", token, `{}`)
		want := `{"error":"http.BadRequest","value":{"message":"body lines: missing","violations":[]}}`
		if rec.Code != 400 || rec.Body.String() != want {
			t.Errorf("lines のない POST /orders（トークン %q）: %d %s", token, rec.Code, rec.Body)
		}
	}
	if len(seen.placeOrders) != 0 {
		t.Fatalf("lines のない要求で op が呼ばれた: %+v", seen.placeOrders)
	}

	// @http.required も @http.default もない項目（token）がない: op は呼ばれ、フィールドは零値（nil）のまま。あれば、その値
	if rec := post("/logout", "", ""); rec.Code != 204 {
		t.Errorf("トークンのない POST /logout: %d %s", rec.Code, rec.Body)
	}
	if rec := post("/orders", "", `{"lines":[]}`); rec.Code != 200 {
		t.Errorf("トークンのない POST /orders: %d %s", rec.Code, rec.Body)
	}
	if rec := post("/orders", "token-1", `{"lines":[{"item":"apple","quantity":1}]}`); rec.Code != 200 {
		t.Errorf("POST /orders: %d %s", rec.Code, rec.Body)
	}
	if len(seen.logouts) != 1 || seen.logouts[0].Token != nil {
		t.Errorf("Logout に渡った入力: %+v（期待は、Token が nil のものが一つ）", seen.logouts)
	}
	if len(seen.placeOrders) != 2 {
		t.Fatalf("PlaceOrder に渡った入力: %+v（期待は二つ）", seen.placeOrders)
	}
	// キーがあれば、空の配列でも、ある（nil ではなく、空のスライスが渡る）
	if in := seen.placeOrders[0]; in.Token != nil || in.Lines == nil || len(in.Lines) != 0 {
		t.Errorf("トークンがなく、lines が空の入力: %+v", in)
	}
	if in := seen.placeOrders[1]; in.Token == nil || *in.Token != "token-1" || len(in.Lines) != 1 || in.Lines[0].Item != "apple" {
		t.Errorf("トークンと lines のある入力: %+v", in)
	}
}

// TestNilIsNotChecked は、New が引数を確かめないこと。渡し忘れ（引数が足りない、メソッドが足りない）はコンパイルできないが、
// nil を書いて渡すことはできる。そのときは、その実装を使う op を呼んだところで panic になる。
// 使わない op は呼べる（上の TestReplaceOneLeaf は、mailer と seams が nil のまま issueSession を呼んでいる）。
func TestNilIsNotChecked(t *testing.T) {
	t.Parallel()
	panics := func(name string, call func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s: nil の実装に届く op を呼んでも panic にならない", name)
			}
		}()
		call()
	}
	f := &handlertest.Fixture{}
	// 葉: nil の leaves（内側の Ops から、葉を直に呼ぶ）
	panics("leaves", func() { _, _ = inner.New(nil, nil, nil, nil).ToPublicUser(context.Background(), pkgtype.User{}) })
	// 継ぎ目: nil の seams。継ぎ目を持つ op（login）は、中身に進む前に、継ぎ目を呼ぶ
	panics("seams", func() {
		_, _ = server.New(f.Leaves(), f.Mailer(), registered{}, nil).Login(context.Background(), server.LoginInput{})
	})
}

// registered は、どの email にも、パスワードが handlertest.AlicePassword のユーザーがいる server.StoreOps。
// login が通る store の op（findUser と addSession）だけを書き、呼ばれた回数を数える。
type registered struct {
	server.StoreOps
	found, added *int
}

func (r registered) FindUser(_ context.Context, email pkgtype.Email) (*pkgtype.User, error) {
	if r.found != nil {
		*r.found++
	}
	user := handlertest.Alice()
	user.Email = email
	return &user, nil
}

func (r registered) AddSession(context.Context, pkgtype.Session) error {
	if r.added != nil {
		*r.added++
	}
	return nil
}

// TestLoginIsLimitedThroughOps は、@rate.limited が、HTTP のハンドラではなく op そのものに掛かること:
// server.New が返す Ops の Login を直に呼んでも、制限を超えれば rate.LimitExceeded が返り、中身（配線の本体）は呼ばれない。
// 継ぎ目は、基底が op の外側に出した口（server.Seams）で、go-http の出力とは関係がない。
func TestLoginIsLimitedThroughOps(t *testing.T) {
	t.Parallel()
	f := &handlertest.Fixture{}
	var found, added int
	ops := server.New(f.Leaves(), f.Mailer(), registered{found: &found, added: &added}, server.NewRateSeams(f.Limiter()))
	ctx := context.Background()
	in := server.LoginInput{Body: server.Credentials{Email: handlertest.AliceEmail, Password: handlertest.AlicePassword}}

	// 制限まで: 中身が動き、セッションができる
	for i := range handlertest.LoginLimit {
		if _, err := ops.Login(ctx, in); err != nil {
			t.Fatalf("%d 回目の Login: %v", i+1, err)
		}
	}
	if found != handlertest.LoginLimit || added != handlertest.LoginLimit {
		t.Fatalf("store.findUser が %d 回、store.addSession が %d 回（期待は、どちらも %d 回）", found, added, handlertest.LoginLimit)
	}
	// 制限を超えた呼び出し: 仕様の例外 rate.LimitExceeded が値で返る。中身は呼ばれない（store のどの op も呼ばれない。
	// 仕様の `throws LimitExceeded prevents effects` を、継ぎ目の中身が守っている）
	out, err := ops.Login(ctx, in)
	var exceeded rate.LimitExceeded
	if !errors.As(err, &exceeded) {
		t.Fatalf("制限を超えた Login = %+v, %v（期待は rate.LimitExceeded）", out, err)
	}
	if out != (server.LoginOk{}) || found != handlertest.LoginLimit || added != handlertest.LoginLimit {
		t.Errorf("制限を超えた Login が中身を呼んだ: 戻り値 %+v、store.findUser %d 回、store.addSession %d 回", out, found, added)
	}
	// 時間の幅が過ぎれば、また呼べる
	f.Advance(handlertest.LoginWindow)
	if _, err := ops.Login(ctx, in); err != nil {
		t.Errorf("幅が過ぎた後の Login: %v", err)
	}
}
