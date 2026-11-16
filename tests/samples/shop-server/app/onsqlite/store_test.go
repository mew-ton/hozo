package onsqlite

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"example.com/shop-server/app/storetest"
	"example.com/shop-server/gen/store"
	pkgtype "example.com/shop-server/gen/type"
)

// newStore は、新しいメモリの上の SQLite に表を作り、その上の store を返す（server は組み立てない。gen/store だけで動く）。
func newStore(t *testing.T) storetest.Store {
	t.Helper()
	db := openDB(t)
	if err := store.CreateTables(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	users, sessions, orders, notifications :=
		store.NewUsersTable(db), store.NewSessionsTable(db), store.NewOrdersTable(db), store.NewNotificationsTable(db)
	return storetest.Store{
		Ops: store.New(users, sessions, orders, notifications, store.NewSharedTable(db)),
		// 形で決まる問い合わせは、store.New に渡したのと同じ値で読む
		States: storetest.States{Users: users, Sessions: sessions, Orders: orders, Notifications: notifications},
	}
}

// TestStore は、go-sqlite が出した store の op の振る舞い。onmem の TestStore と同じ storetest.Run を走らせる。
func TestStore(t *testing.T) {
	t.Parallel()
	storetest.Run(t, newStore)
}

// TestCreateTables は、go-sqlite の CreateTables: 受け取った ctx で表を作る（中断されていれば作らない）。
// すでにある表には触れないので、何度呼んでもよい。
func TestCreateTables(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	tables := func() (n int) {
		t.Helper()
		if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.CreateTables(cancelled, db); !errors.Is(err, context.Canceled) {
		t.Errorf("中断された ctx での CreateTables のエラーが %v（期待は context.Canceled）", err)
	}
	if n := tables(); n != 0 {
		t.Errorf("中断された ctx での CreateTables の後に、表が %d", n)
	}

	ctx := context.Background()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	if n := tables(); n != 4 {
		t.Errorf("表が %d（期待は、状態ごとに一つで 4）", n)
	}
	users := store.NewUsersTable(db)
	if err := users.AddUser(ctx, pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: "h"}); err != nil {
		t.Fatal(err)
	}
	// 二度目は何もしない。足した行は残る
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	if empty, err := users.IsEmpty(ctx); err != nil || empty {
		t.Errorf("二度目の CreateTables の後の users の IsEmpty = %v, %v（期待は false, nil）", empty, err)
	}
}

// 以下の三つの型は、「ある op に db の語彙のデコレータを付けなかった（go-sqlite は、その op のメソッドを出さない）」ときに
// 人がすること（../../README.md「go-sqlite が従う規則」）を、この仕様で形にしたもの。この仕様では八つの op のすべてに
// デコレータが付いていて、go-sqlite はすべてのメソッドを出すので、メソッドの足りない型は見立てで作る。

// usersReads は、store.UsersTable のメソッドのうち、AddUser を除いたもの。
type usersReads interface {
	IsEmpty(ctx context.Context) (bool, error)
	Get(ctx context.Context) ([]pkgtype.User, error)
	FindUser(ctx context.Context, email pkgtype.Email) (*pkgtype.User, error)
	FindUserById(ctx context.Context, id pkgtype.UserId) (*pkgtype.User, error)
}

// partialUsers は、addUser に @db.effects を付けなかったときの go-sqlite の型の見立て: メソッド AddUser を持たない。
// store.UsersState を満たさないので、store.New の引数 users に渡すと、コンパイルできない。
type partialUsers struct{ usersReads }

// humanUsers は、人が書き足した型: プラグインの型を埋め込み、足りないメソッド AddUser を足す。
type humanUsers struct {
	partialUsers
	db    *sql.DB
	added *int
}

// AddUser は、人が書いた store.addUser の本体。
func (u humanUsers) AddUser(ctx context.Context, user pkgtype.User) error {
	*u.added++
	_, err := u.db.ExecContext(ctx, `INSERT INTO "users" ("id", "email", "name", "hash") VALUES (?1, ?2, ?3, ?4)`,
		string(user.Id), string(user.Email), user.Name, user.Hash)
	return err
}

// 書き足した型は、状態の単位を満たす（満たさなければ、ここでコンパイルできない）。
var _ store.UsersState = humanUsers{}

// TestHumanCompletesPluginType は、プラグインの型に足りないメソッドを、人が埋め込みで書き足せること。
// 足りないままの型を渡す行（store.New(partialUsers{…}, …)）は、コンパイルできないのでここには書けない。
// 代わりに、その型が store.UsersState を満たさないことを、リフレクションで確かめる。
func TestHumanCompletesPluginType(t *testing.T) {
	t.Parallel()
	state := reflect.TypeOf((*store.UsersState)(nil)).Elem()
	if reflect.TypeOf(partialUsers{}).Implements(state) {
		t.Error("AddUser のない型が store.UsersState を満たす")
	}
	if !reflect.TypeOf(humanUsers{}).Implements(state) {
		t.Error("AddUser を足した型が store.UsersState を満たさない")
	}

	db, ctx := openDB(t), context.Background()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	added := 0
	users := humanUsers{partialUsers{store.NewUsersTable(db)}, db, &added}
	s := store.New(users, store.NewSessionsTable(db), store.NewOrdersTable(db), store.NewNotificationsTable(db), store.NewSharedTable(db))
	// addUser は人が書いたメソッドへ、findUser はプラグインのメソッドへ届く
	if err := s.AddUser(ctx, pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: "h"}); err != nil {
		t.Fatal(err)
	}
	if added != 1 {
		t.Errorf("人が書いた AddUser が呼ばれた回数が %d（期待は 1）", added)
	}
	if user, err := s.FindUser(ctx, "alice@example.com"); err != nil || user == nil || user.Id != "u1" {
		t.Errorf("FindUser = %v, %v", user, err)
	}
}

// TestColumns は、go-sqlite が決めた列の形: Time は RFC 3339・UTC・小数部 9 桁の TEXT、配列は JSON の TEXT。
func TestColumns(t *testing.T) {
	t.Parallel()
	db, ctx := openDB(t), context.Background()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := store.NewUsersTable(db).AddUser(ctx, pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: "h"}); err != nil {
		t.Fatal(err)
	}
	placedAt := time.Date(2026, 1, 2, 12, 4, 5, 120000000, time.FixedZone("JST", 9*60*60))
	order := pkgtype.Order{Id: "o1", UserId: "u1", Lines: []pkgtype.OrderLine{{Item: "apple", Quantity: 2}}, PlacedAt: placedAt}
	if err := store.NewSharedTable(db).PlaceOrder(ctx, order, pkgtype.Notification{UserId: "u1", Message: "x"}); err != nil {
		t.Fatal(err)
	}
	var lines, at string
	if err := db.QueryRow(`SELECT "lines", "placedAt" FROM "orders" WHERE "id" = 'o1'`).Scan(&lines, &at); err != nil {
		t.Fatal(err)
	}
	if lines != `[{"item":"apple","quantity":2}]` || at != "2026-01-02T03:04:05.120000000Z" {
		t.Errorf("列の値が lines = %s、placedAt = %s", lines, at)
	}
}

// TestRef は、@db.ref の REFERENCES: 接続で外部キーを有効にしていれば、いないユーザーの行は足せない。
// これは SQLite の制約で、仕様（store.hozo）が言っていることではない。メモリの上の実装（memstore）は確かめない。
func TestRef(t *testing.T) {
	t.Parallel()
	s, ctx := newStore(t), context.Background()
	err := s.AddSession(ctx, pkgtype.Session{Token: "t1", UserId: "nobody", ExpiresAt: time.Now()})
	if err == nil {
		t.Fatal("いないユーザーのセッションを足せた")
	}
	// どの制約の違反かは、ドライバのエラーの文言でしか分からない（gen/store はドライバを import しない）
	if !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Errorf("エラーが %v", err)
	}
}

// TestPlaceOrderIsOneTransaction は、二つの効果を持つ op（@db.effects(createOrder, notifyUser)）が、SQLite の上で
// 一つのトランザクションであること: 二つ目の文（通知の行）が失敗すれば、一つ目の文（注文の行）も残らない。
// 一つ目の文が失敗する場合（同じ id の注文）は、二つの実装が共に走らせる storetest の placeOrder が確かめる。
// ここでは、二つ目だけを失敗させる: 外部キーを有効にした接続で、通知の userId（@db.ref）を、いないユーザーにする。
func TestPlaceOrderIsOneTransaction(t *testing.T) {
	t.Parallel()
	s, ctx := newStore(t), context.Background()
	if err := s.AddUser(ctx, pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: "h"}); err != nil {
		t.Fatal(err)
	}
	order := pkgtype.Order{Id: "o1", UserId: "u1", PlacedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	err := s.PlaceOrder(ctx, order, pkgtype.Notification{UserId: "nobody", Message: "残らない"})
	if err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Fatalf("いないユーザーへの通知を持つ placeOrder のエラーが %v（期待は、外部キーの違反）", err)
	}
	// 注文の行は、取り消されている（状態 orders も notifications も空のまま）
	if empty, err := s.States.Orders.IsEmpty(ctx); err != nil || !empty {
		t.Errorf("失敗した placeOrder の後の orders の IsEmpty = %v, %v（期待は true: 注文も残らない）", empty, err)
	}
	if empty, err := s.States.Notifications.IsEmpty(ctx); err != nil || !empty {
		t.Errorf("失敗した placeOrder の後の notifications の IsEmpty = %v, %v（期待は true）", empty, err)
	}
	// トランザクションは閉じている: 接続は一つだけ（openDB）なので、開いたままなら、次の op は戻ってこない。
	// 同じ注文を、いるユーザーへの通知と一緒に足せる（失敗したときの注文の行が残っていれば、キーの違反になる）
	if err := s.PlaceOrder(ctx, order, pkgtype.Notification{UserId: "u1", Message: "残る"}); err != nil {
		t.Fatalf("失敗の後の placeOrder: %v", err)
	}
	orders, err := s.ListOrders(ctx, "u1")
	if err != nil || len(orders) != 1 || orders[0].Id != "o1" {
		t.Errorf("listOrders(u1) = %+v, %v（期待は o1 が一つ）", orders, err)
	}
	notices, err := s.States.Notifications.Get(ctx)
	if err != nil || len(notices) != 1 || notices[0].Message != "残る" {
		t.Errorf("notifications の Get = %+v, %v（期待は「残る」が一つ）", notices, err)
	}
}
