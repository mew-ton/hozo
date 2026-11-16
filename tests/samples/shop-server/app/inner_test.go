package app_test

import (
	"context"
	"errors"
	"testing"

	"example.com/shop-server/app/handlertest"
	"example.com/shop-server/gen/server"
	"example.com/shop-server/gen/server/inner"
	pkgtype "example.com/shop-server/gen/type"
)

// このファイルは、server の内部の op（export のない op）と、継ぎ目の内側（op の中身）を、単独で試す。
// どちらも、入口のパッケージ（gen/server）からは呼べない。内側のパッケージ（gen/server/inner）の New が返す Ops は、
// すべての op を公開のメソッドに持つので、テストはそこから直に呼ぶ（別の名前空間から呼べないことを守るのは、Hozo の検査）。

// sessions は、server が呼ぶ store の op のうち、セッションを読み書きするものを、メモリの上に書いた server.StoreOps。
// ほかの op は、埋め込んだ nil のインターフェースのまま（呼べば panic）。
type sessions struct {
	server.StoreOps
	users map[pkgtype.Email]pkgtype.User
	byKey map[pkgtype.Token]pkgtype.Session
}

func (s *sessions) FindUser(_ context.Context, email pkgtype.Email) (*pkgtype.User, error) {
	if user, ok := s.users[email]; ok {
		return &user, nil
	}
	return nil, nil
}

func (s *sessions) FindSession(_ context.Context, token *pkgtype.Token) (*pkgtype.Session, error) {
	if token == nil {
		return nil, nil
	}
	if session, ok := s.byKey[*token]; ok {
		return &session, nil
	}
	return nil, nil
}

func (s *sessions) AddSession(_ context.Context, session pkgtype.Session) error {
	if s.byKey == nil {
		s.byKey = map[pkgtype.Token]pkgtype.Session{}
	}
	s.byKey[session.Token] = session
	return nil
}

// TestInternalWirings は、内部の配線を、内側の Ops から直に呼ぶ: authenticate と issueSession。
// 入口（login、getMe など）を通さないので、そこまでの道の op を用意しなくてよく、戻り値（セッション）もそのまま見られる。
func TestInternalWirings(t *testing.T) {
	t.Parallel()
	f := &handlertest.Fixture{}
	st := &sessions{}
	// 継ぎ目を持つ op（login）は呼ばないので、seams は渡さない
	ops := inner.New(f.Leaves(), f.Mailer(), st, nil)
	ctx := context.Background()
	alice := handlertest.Alice()

	// issueSession: パスワードが合えば、葉 newSession が作ったセッションを store.addSession に渡し、外に見せる形のユーザーとトークンを返す
	ok, err := ops.IssueSession(ctx, alice, handlertest.AlicePassword)
	want := server.LoginOk{User: pkgtype.PublicUser{Id: "u1", Email: "alice@example.com", Name: "Alice"}, Token: "token-1"}
	if err != nil || ok != want {
		t.Fatalf("IssueSession = %+v, %v（期待は %+v）", ok, err, want)
	}
	added, found := st.byKey["token-1"]
	if !found || added.UserId != "u1" || !added.ExpiresAt.Equal(handlertest.Now.Add(handlertest.SessionTTL)) {
		t.Errorf("store.addSession に渡ったセッション = %+v", added)
	}
	// パスワードが合わなければ type.InvalidCredentials で、セッションは足されない
	var invalid pkgtype.InvalidCredentials
	if _, err := ops.IssueSession(ctx, alice, "wrong-password"); !errors.As(err, &invalid) {
		t.Errorf("IssueSession（パスワード違い）= %v（期待は type.InvalidCredentials）", err)
	}
	if len(st.byKey) != 1 {
		t.Errorf("セッションが %d 件（期待は 1）", len(st.byKey))
	}

	// authenticate: ログインしていれば、そのセッション
	token := pkgtype.Token("token-1")
	if session, err := ops.Authenticate(ctx, &token); err != nil || session != added {
		t.Errorf("Authenticate = %+v, %v（期待は %+v）", session, err, added)
	}
	// トークンがない、知らないトークン、期限が過ぎたセッション: どれも type.Unauthorized
	var unauthorized pkgtype.Unauthorized
	unknown := pkgtype.Token("no-such-token")
	for name, tok := range map[string]*pkgtype.Token{"トークンなし": nil, "知らないトークン": &unknown} {
		if _, err := ops.Authenticate(ctx, tok); !errors.As(err, &unauthorized) {
			t.Errorf("Authenticate（%s）= %v（期待は type.Unauthorized）", name, err)
		}
	}
	f.Advance(handlertest.SessionTTL)
	if _, err := ops.Authenticate(ctx, &token); !errors.As(err, &unauthorized) {
		t.Errorf("Authenticate（期限切れ）= %v（期待は type.Unauthorized）", err)
	}
}

// through は、継ぎ目を素通しにする inner.Seams: 何も足さずに、op の中身を呼ぶ。通った op を覚える。
// 継ぎ目の実装は、プラグインの出力でなくてもよい（インターフェースを満たす値なら、何でも渡せる）。
type through struct{ passed []string }

func (s *through) LoginRateLimited(ctx context.Context, in inner.LoginInput, body func(ctx context.Context, in inner.LoginInput) (inner.LoginOk, error)) (inner.LoginOk, error) {
	s.passed = append(s.passed, "server.login")
	return body(ctx, in)
}

// TestBodyBehindSeam は、継ぎ目を持つ op（login）の中身（配線の本体）だけを試す形: 継ぎ目を素通しにして呼ぶ。
// 制限は掛からないので、何度呼んでも、中身の結果がそのまま返る。
func TestBodyBehindSeam(t *testing.T) {
	t.Parallel()
	f := &handlertest.Fixture{}
	alice := handlertest.Alice()
	st := &sessions{users: map[pkgtype.Email]pkgtype.User{alice.Email: alice}}
	seams := &through{}
	ops := inner.New(f.Leaves(), f.Mailer(), st, seams)
	ctx := context.Background()

	var invalid pkgtype.InvalidCredentials
	for i := range handlertest.LoginLimit * 2 {
		// いないユーザー: 中身の match の default のアームが type.InvalidCredentials を投げる
		in := inner.LoginInput{Body: inner.Credentials{Email: "bob@example.com", Password: handlertest.AlicePassword}}
		if _, err := ops.Login(ctx, in); !errors.As(err, &invalid) {
			t.Fatalf("%d 回目の Login（いないユーザー）= %v（期待は type.InvalidCredentials）", i+1, err)
		}
	}
	in := inner.LoginInput{Body: inner.Credentials{Email: alice.Email, Password: handlertest.AlicePassword}}
	ok, err := ops.Login(ctx, in)
	if err != nil || ok.Token != "token-1" || ok.User.Id != alice.Id {
		t.Errorf("Login = %+v, %v", ok, err)
	}
	// 外側の Login は、呼ばれるたびに継ぎ目を一度通る（中身を直に呼ぶ道は、パッケージの外にはない）
	if len(seams.passed) != handlertest.LoginLimit*2+1 {
		t.Errorf("継ぎ目を通った回数 = %d（期待は %d）", len(seams.passed), handlertest.LoginLimit*2+1)
	}
}

// replacing は、継ぎ目の中で、op の中身に渡す値と、返す値を差し替える inner.Seams。
// 書き方の指針（デコレータは、op の入力と出力の値を変えない）に反する実装で、基底は止めない、ということだけを確かめるためのもの。
type replacing struct{}

func (replacing) LoginRateLimited(ctx context.Context, in inner.LoginInput, body func(ctx context.Context, in inner.LoginInput) (inner.LoginOk, error)) (inner.LoginOk, error) {
	in.Body.Email = "alice@example.com"
	out, err := body(ctx, in)
	out.Token = "replaced"
	return out, err
}

// TestSeamOwnsBeforeAndAfter は、継ぎ目が、op の中身の前と後の両方を持つこと: 中身に何を渡し、何を返すかは、継ぎ目の実装が決める。
// 基底が出す外側は、継ぎ目に制御を渡すだけで、中身の前後に何も足さない。
func TestSeamOwnsBeforeAndAfter(t *testing.T) {
	t.Parallel()
	f := &handlertest.Fixture{}
	alice := handlertest.Alice()
	st := &sessions{users: map[pkgtype.Email]pkgtype.User{alice.Email: alice}}
	ops := inner.New(f.Leaves(), f.Mailer(), st, replacing{})
	in := inner.LoginInput{Body: inner.Credentials{Email: "someone@example.com", Password: handlertest.AlicePassword}}
	ok, err := ops.Login(context.Background(), in)
	if err != nil || ok.User.Id != alice.Id || ok.Token != "replaced" {
		t.Errorf("Login = %+v, %v（期待は、alice と、差し替えたトークン）", ok, err)
	}
	// 中身は動いている: 中身が作ったトークン（token-1）のセッションが、store に足されている
	if _, found := st.byKey["token-1"]; !found {
		t.Errorf("store.addSession に渡ったセッション = %v", st.byKey)
	}
}
