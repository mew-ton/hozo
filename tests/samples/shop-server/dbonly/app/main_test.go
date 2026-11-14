package main

import (
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"example.com/shop-server/dbonly/server"
	"github.com/mew-ton/hozo/runtime"
)

// setup はメモリの上の SQLite に server.SetupDB する。
func setup(t *testing.T) *sql.DB {
	t.Helper()
	// ":memory:" は接続ごとに別のデータベースになるので、接続を一つに限る
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if err := server.SetupDB(db); err != nil {
		t.Fatal(err)
	}
	return db
}

var (
	alice = server.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: "h1"}
	bob   = server.User{Id: "u2", Email: "bob@example.com", Name: "Bob", Hash: "h2"}
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// TestKeyedTable は、@db.key と @db.unique を持つ model の表（users）の往復。
func TestKeyedTable(t *testing.T) {
	setup(t)
	users := server.Tables.Users
	must(t, users.Insert(alice))
	must(t, users.Insert(bob))

	if n, err := users.Count(); err != nil || n != 2 {
		t.Errorf("Count() = %d, %v", n, err)
	}
	if got, err := users.List(); err != nil || !reflect.DeepEqual(got, []server.User{alice, bob}) {
		t.Errorf("List() = %+v, %v（期待は、足した順）", got, err)
	}
	if got, err := users.Get("u2"); err != nil || got == nil || *got != bob {
		t.Errorf("Get(u2) = %+v, %v", got, err)
	}
	if got, err := users.Get("u9"); err != nil || got != nil {
		t.Errorf("Get(u9) = %+v, %v（期待は nil）", got, err)
	}
	if got, err := users.GetByEmail("alice@example.com"); err != nil || got == nil || *got != alice {
		t.Errorf("GetByEmail(alice) = %+v, %v", got, err)
	}
	if got, err := users.GetByEmail("carol@example.com"); err != nil || got != nil {
		t.Errorf("GetByEmail(carol) = %+v, %v（期待は nil）", got, err)
	}

	renamed := alice
	renamed.Name = "Alice Liddell"
	must(t, users.Update(renamed))
	if got, err := users.Get("u1"); err != nil || got == nil || *got != renamed {
		t.Errorf("Update の後の Get(u1) = %+v, %v", got, err)
	}
	if err := users.Update(server.User{Id: "u9", Email: "x@example.com"}); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("ない行の Update = %v（期待は sql.ErrNoRows）", err)
	}

	must(t, users.Delete("u2"))
	must(t, users.Delete("u2")) // ない行を消しても何も起きない
	if got, err := users.List(); err != nil || !reflect.DeepEqual(got, []server.User{renamed}) {
		t.Errorf("Delete の後の List() = %+v, %v", got, err)
	}

	// @db.key は PRIMARY KEY、@db.unique は UNIQUE: 重なる行は足せない（エラーの型はドライバのもの）
	if err := users.Insert(server.User{Id: "u1", Email: "other@example.com"}); err == nil {
		t.Error("同じ id の行を足せた")
	}
	if err := users.Insert(server.User{Id: "u3", Email: renamed.Email}); err == nil {
		t.Error("同じ email の行を足せた")
	}
}

// TestColumnTypes は、Time の列（RFC 3339、UTC）と、配列の列（JSON）の往復。
func TestColumnTypes(t *testing.T) {
	db := setup(t)
	must(t, server.Tables.Users.Insert(alice))

	// Time: ナノ秒まで残り、読むと UTC になる
	jst := time.FixedZone("JST", 9*60*60)
	expires := time.Date(2026, 1, 2, 12, 4, 5, 123456789, jst)
	session := server.Session{Token: "t1", UserId: "u1", ExpiresAt: expires}
	must(t, server.Tables.Sessions.Insert(session))
	got, err := server.Tables.Sessions.Get("t1")
	if err != nil || got == nil {
		t.Fatalf("Get(t1) = %+v, %v", got, err)
	}
	if !got.ExpiresAt.Equal(expires) || got.ExpiresAt.Location() != time.UTC {
		t.Errorf("ExpiresAt = %v（期待は %v を UTC にしたもの）", got.ExpiresAt, expires)
	}
	var text string
	must(t, db.QueryRow(`SELECT "expiresAt" FROM "sessions"`).Scan(&text))
	if text != "2026-01-02T03:04:05.123456789Z" {
		t.Errorf("expiresAt の列 = %q", text)
	}
	// 小数部はいつも 9 桁なので、文字列の順が時刻の順になる
	must(t, server.Tables.Sessions.Insert(server.Session{Token: "t2", UserId: "u1", ExpiresAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}))
	must(t, db.QueryRow(`SELECT "token" FROM "sessions" ORDER BY "expiresAt" LIMIT 1`).Scan(&text))
	if text != "t2" {
		t.Errorf("expiresAt の順で最初の行 = %q（期待は t2）", text)
	}

	// 配列: JSON の文字列
	order := server.Order{
		Id:       "o1",
		UserId:   "u1",
		Lines:    []server.OrderLine{{Item: "apple", Quantity: 2}, {Item: "pear", Quantity: 1}},
		PlacedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	must(t, server.Tables.Orders.Insert(order))
	if got, err := server.Tables.Orders.Get("o1"); err != nil || got == nil || !reflect.DeepEqual(*got, order) {
		t.Errorf("Get(o1) = %+v, %v", got, err)
	}
	must(t, db.QueryRow(`SELECT "lines" FROM "orders"`).Scan(&text))
	if text != `[{"item":"apple","quantity":2},{"item":"pear","quantity":1}]` {
		t.Errorf("lines の列 = %q", text)
	}
	order.Lines = order.Lines[:1]
	must(t, server.Tables.Orders.Update(order))
	if got, err := server.Tables.Orders.Get("o1"); err != nil || got == nil || !reflect.DeepEqual(*got, order) {
		t.Errorf("Update の後の Get(o1) = %+v, %v", got, err)
	}
}

// TestKeylessTable は、@db.key を持たない model の表（notifications）: 足す、並べる、数えるだけ。
func TestKeylessTable(t *testing.T) {
	setup(t)
	must(t, server.Tables.Users.Insert(alice))
	n1 := server.Notification{UserId: "u1", Message: "一つ目"}
	n2 := server.Notification{UserId: "u1", Message: "二つ目"}
	must(t, server.Tables.Notifications.Insert(n1))
	must(t, server.Tables.Notifications.Insert(n2))
	if got, err := server.Tables.Notifications.List(); err != nil || !reflect.DeepEqual(got, []server.Notification{n1, n2}) {
		t.Errorf("List() = %+v, %v", got, err)
	}
	if n, err := server.Tables.Notifications.Count(); err != nil || n != 2 {
		t.Errorf("Count() = %d, %v", n, err)
	}
}

// TestRef は @db.ref(User) の REFERENCES: 接続で外部キーを有効にしていれば（DSN の _pragma）、いないユーザーを指す行は足せない。
func TestRef(t *testing.T) {
	setup(t)
	if err := server.Tables.Notifications.Insert(server.Notification{UserId: "u9", Message: "誰にも届かない"}); err == nil {
		t.Error("いないユーザーを指す通知を足せた")
	}
	must(t, server.Tables.Users.Insert(alice))
	must(t, server.Tables.Notifications.Insert(server.Notification{UserId: "u1", Message: "届く"}))
	if err := server.Tables.Users.Delete("u1"); err == nil {
		t.Error("通知が指しているユーザーを消せた")
	}
}

// TestStates は、SetupDB が入れる状態の単位の問い合わせ（IsEmpty）。
func TestStates(t *testing.T) {
	db := setup(t)
	for name, isEmpty := range map[string]func() (bool, error){
		"users":         server.States.Users.IsEmpty,
		"sessions":      server.States.Sessions.IsEmpty,
		"orders":        server.States.Orders.IsEmpty,
		"notifications": server.States.Notifications.IsEmpty,
	} {
		if empty, err := isEmpty(); err != nil || !empty {
			t.Errorf("States の %s の IsEmpty() = %v, %v（期待は true）", name, empty, err)
		}
	}
	must(t, server.Tables.Users.Insert(alice))
	if empty, err := server.States.Users.IsEmpty(); err != nil || empty {
		t.Errorf("States.Users.IsEmpty() = %v, %v（期待は false）", empty, err)
	}
	if empty, err := server.States.Sessions.IsEmpty(); err != nil || !empty {
		t.Errorf("States.Sessions.IsEmpty() = %v, %v（期待は true）", empty, err)
	}

	// SetupDB は何度呼んでもよい（CREATE TABLE IF NOT EXISTS）。行は残る
	must(t, server.SetupDB(db))
	if n, err := server.Tables.Users.Count(); err != nil || n != 1 {
		t.Errorf("二度目の SetupDB の後の Count() = %d, %v", n, err)
	}
}

// TestBeforeSetup は、SetupDB の前に表を使ったときのエラー: panic せず *runtime.MissingError（Kind は "state"）。
func TestBeforeSetup(t *testing.T) {
	server.Tables = server.TableSet{}
	_, err := server.Tables.Orders.Count()
	var missing *runtime.MissingError
	if !errors.As(err, &missing) || missing.Kind != "state" || missing.Name != "server.orders" {
		t.Errorf("SetupDB の前の Count() のエラー = %v", err)
	}
}

// TestReport は、この組の人の手のコード（main.go の report）。
func TestReport(t *testing.T) {
	setup(t)
	must(t, server.Tables.Users.Insert(alice))
	var out strings.Builder
	must(t, report(&out))
	want := "users         1\nsessions      0（空）\norders        0（空）\nnotifications 0（空）\n"
	if out.String() != want {
		t.Errorf("report:\n%s期待:\n%s", out.String(), want)
	}
}
