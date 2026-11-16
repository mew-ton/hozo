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
		Ops: store.New(users, sessions, orders, notifications, store.NewDbSeams(db)),
		// 形で決まる問い合わせは、store.New に渡したのと同じ値で読む
		States: storetest.States{Users: users, Sessions: sessions, Orders: orders, Notifications: notifications},
	}
}

// TestStore は、go-sqlite が出した read と効果の上の、store の op の振る舞い。onmem の TestStore と同じ storetest.Run を走らせる。
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
	if err := users.CreateUser(ctx, pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: "h"}); err != nil {
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

// 以下の三つの型は、「ある効果に db の語彙のデコレータを付けなかった（go-sqlite は、その効果のメソッドを出さない）」ときに
// 人がすること（../../README.md「go-sqlite が従う規則」）を、この仕様で形にしたもの。この仕様では、五つの read と五つの効果の
// すべてにデコレータが付いていて、go-sqlite はすべてのメソッドを出すので、メソッドの足りない型は見立てで作る。

// usersReads は、store.UsersTable のメソッドのうち、CreateUser を除いたもの。
type usersReads interface {
	IsEmpty(ctx context.Context) (bool, error)
	Get(ctx context.Context) ([]pkgtype.User, error)
	UserByEmail(ctx context.Context, email pkgtype.Email) (*pkgtype.User, error)
	UserById(ctx context.Context, id pkgtype.UserId) (*pkgtype.User, error)
}

// partialUsers は、効果 createUser に @db.insert を付けなかったときの go-sqlite の型の見立て: メソッド CreateUser を持たない。
// store.UsersState を満たさないので、store.New の引数 users に渡すと、コンパイルできない。
type partialUsers struct{ usersReads }

// humanUsers は、人が書き足した型: プラグインの型を埋め込み、足りないメソッド CreateUser を足す。
type humanUsers struct {
	partialUsers
	db    *sql.DB
	added *int
}

// CreateUser は、人が書いた効果 store.createUser の本体。文は、db を直に使わずに、store.Conn を通して実行する:
// ctx が @db.transaction のトランザクションを運んでいれば、それに入る。
func (u humanUsers) CreateUser(ctx context.Context, user pkgtype.User) error {
	*u.added++
	_, err := store.Conn(ctx, u.db).ExecContext(ctx, `INSERT INTO "users" ("id", "email", "name", "hash") VALUES (?1, ?2, ?3, ?4)`,
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
		t.Error("CreateUser のない型が store.UsersState を満たす")
	}
	if !reflect.TypeOf(humanUsers{}).Implements(state) {
		t.Error("CreateUser を足した型が store.UsersState を満たさない")
	}

	db, ctx := openDB(t), context.Background()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	added := 0
	users := humanUsers{partialUsers{store.NewUsersTable(db)}, db, &added}
	s := store.New(users, store.NewSessionsTable(db), store.NewOrdersTable(db), store.NewNotificationsTable(db), store.NewDbSeams(db))
	// addUser（効果 createUser を呼ぶ配線）は人が書いたメソッドへ、findUser（read userByEmail）はプラグインのメソッドへ届く
	if err := s.AddUser(ctx, pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: "h"}); err != nil {
		t.Fatal(err)
	}
	if added != 1 {
		t.Errorf("人が書いた CreateUser が呼ばれた回数が %d（期待は 1）", added)
	}
	if user, err := s.FindUser(ctx, "alice@example.com"); err != nil || user == nil || user.Id != "u1" {
		t.Errorf("FindUser = %v, %v", user, err)
	}

	// 人が書いたメソッドも、store.Conn を通せば、トランザクションに入る: 継ぎ目の中で足した行は、中身が失敗すれば残らない。
	// 接続は一つだけ（openDB）なので、db を直に使えば、トランザクションの外の接続を待って、戻ってこない
	failed := errors.New("中身の失敗")
	seams := store.NewDbSeams(db)
	err := seams.PlaceOrderDbTransaction(ctx, pkgtype.Order{}, pkgtype.Notification{}, func(ctx context.Context, _ pkgtype.Order, _ pkgtype.Notification) error {
		if err := users.CreateUser(ctx, pkgtype.User{Id: "u2", Email: "bob@example.com", Name: "Bob", Hash: "h"}); err != nil {
			return err
		}
		return failed
	})
	if !errors.Is(err, failed) {
		t.Fatalf("継ぎ目のエラーが %v（期待は、中身の失敗）", err)
	}
	if got, err := users.Get(ctx); err != nil || len(got) != 1 {
		t.Errorf("中身が失敗した後の users = %+v, %v（期待は、u1 だけ）", got, err)
	}
}

// TestColumns は、go-sqlite が決めた列の形: Time は RFC 3339・UTC・小数部 9 桁の TEXT、配列は JSON の TEXT。
func TestColumns(t *testing.T) {
	t.Parallel()
	db, ctx := openDB(t), context.Background()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := store.NewUsersTable(db).CreateUser(ctx, pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: "h"}); err != nil {
		t.Fatal(err)
	}
	placedAt := time.Date(2026, 1, 2, 12, 4, 5, 120000000, time.FixedZone("JST", 9*60*60))
	order := pkgtype.Order{Id: "o1", UserId: "u1", Lines: []pkgtype.OrderLine{{Item: "apple", Quantity: 2}}, PlacedAt: placedAt}
	if err := store.NewOrdersTable(db).CreateOrder(ctx, order); err != nil {
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

// TestPlaceOrderIsOneTransaction は、@db.transaction を付けた op（placeOrder: 二つの効果を呼ぶ配線）が、SQLite の上で
// 一つのトランザクションであること: 二つ目の効果（通知の行）が失敗すれば、一つ目の効果（注文の行）も残らない。
// 一つ目の効果が失敗する場合（同じ id の注文）は、二つの実装が共に走らせる storetest の placeOrder が確かめる。
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

// through は、継ぎ目を素通しにする store.Seams: 中身を、そのまま呼ぶ（トランザクションを始めない）。
type through struct{}

func (through) PlaceOrderDbTransaction(ctx context.Context, order pkgtype.Order, notification pkgtype.Notification, body func(ctx context.Context, order pkgtype.Order, notification pkgtype.Notification) error) error {
	return body(ctx, order, notification)
}

// TestTransactionIsTheSeam は、「どちらも足すか、どちらも足さない」を守っているのが、@db.transaction の継ぎ目
// （store.DbSeams）であること: 同じ表の型の上で、継ぎ目だけを素通しに替えると、二つ目の効果が失敗しても、一つ目の効果の
// 行が残る。表の型のメソッドは、ctx がトランザクションを運んでいなければ、一つずつ、すぐに確定する。
func TestTransactionIsTheSeam(t *testing.T) {
	t.Parallel()
	db, ctx := openDB(t), context.Background()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	orders := store.NewOrdersTable(db)
	s := store.New(store.NewUsersTable(db), store.NewSessionsTable(db), orders, store.NewNotificationsTable(db), through{})
	if err := s.AddUser(ctx, pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: "h"}); err != nil {
		t.Fatal(err)
	}
	order := pkgtype.Order{Id: "o1", UserId: "u1", PlacedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	if err := s.PlaceOrder(ctx, order, pkgtype.Notification{UserId: "nobody", Message: "残らない"}); err == nil {
		t.Fatal("いないユーザーへの通知を持つ placeOrder が、失敗しなかった")
	}
	if got, err := orders.Get(ctx); err != nil || len(got) != 1 {
		t.Errorf("継ぎ目を素通しにした placeOrder の後の orders = %+v, %v（期待は、注文の行が残る）", got, err)
	}
}

// TestNestedTransactionJoins は、トランザクションの中から、もう一度 @db.transaction の継ぎ目を通ると（入れ子）、
// 新しいトランザクションを始めずに、外側のものに加わること: 内側が成功した後で外側が失敗すれば、内側のしたことも残らない。
// この仕様に、入れ子になる op はないので、継ぎ目の実装（store.DbSeams）のメソッドを、テストが二重に呼ぶ。
func TestNestedTransactionJoins(t *testing.T) {
	t.Parallel()
	db, ctx := openDB(t), context.Background()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := store.NewUsersTable(db).CreateUser(ctx, pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: "h"}); err != nil {
		t.Fatal(err)
	}
	orders, notifications, seams := store.NewOrdersTable(db), store.NewNotificationsTable(db), store.NewDbSeams(db)
	order := pkgtype.Order{Id: "o1", UserId: "u1", PlacedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	notice := pkgtype.Notification{UserId: "u1", Message: "x"}
	failed := errors.New("外側の失敗")
	// inner は、内側の中身: 注文と通知を足して、成功する
	inner := func(ctx context.Context, order pkgtype.Order, notification pkgtype.Notification) error {
		if err := orders.CreateOrder(ctx, order); err != nil {
			return err
		}
		return notifications.NotifyUser(ctx, notification)
	}
	// 外側の中身: 内側を、継ぎ目を通して呼んだ後で、fail なら失敗する。
	// 接続は一つだけ（openDB）なので、内側が新しいトランザクションを始めようとすれば、戻ってこない
	outer := func(fail bool) error {
		return seams.PlaceOrderDbTransaction(ctx, order, notice, func(ctx context.Context, order pkgtype.Order, notification pkgtype.Notification) error {
			if err := seams.PlaceOrderDbTransaction(ctx, order, notification, inner); err != nil {
				return err
			}
			// 外側のトランザクションの中からは、内側の足した行が見える
			if empty, err := orders.IsEmpty(ctx); err != nil || empty {
				t.Errorf("外側の中から見た orders の IsEmpty = %v, %v（期待は false）", empty, err)
			}
			if fail {
				return failed
			}
			return nil
		})
	}
	if err := outer(true); !errors.Is(err, failed) {
		t.Fatalf("外側が失敗した入れ子のエラーが %v（期待は、外側の失敗）", err)
	}
	if empty, err := orders.IsEmpty(ctx); err != nil || !empty {
		t.Errorf("外側が失敗した後の orders の IsEmpty = %v, %v（期待は true: 内側のしたことも残らない）", empty, err)
	}
	if err := outer(false); err != nil {
		t.Fatalf("外側が成功した入れ子: %v", err)
	}
	if got, err := notifications.Get(ctx); err != nil || len(got) != 1 {
		t.Errorf("外側が成功した後の notifications = %+v, %v（期待は一つ）", got, err)
	}
}
