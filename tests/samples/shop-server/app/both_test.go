package app_test

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"example.com/shop-server/app/handlertest"
	"example.com/shop-server/app/memstore"
	"example.com/shop-server/app/onmem"
	"example.com/shop-server/app/onsqlite"
	"example.com/shop-server/app/storetest"
	"example.com/shop-server/gen/server"
	"example.com/shop-server/gen/store"
	storemem "example.com/shop-server/gen/storemem" // このパッケージの名前も store なので、別の名前で import する
	pkgtype "example.com/shop-server/gen/type"
)

// store の二つの出力（gen/store と gen/storemem）は、どちらも package store で、別の Ops を持つ。型は同じ gen/type の
// ものなので、二つの Ops は同じメソッドの並びになり、同じインターフェースを満たす。
var (
	_ storetest.Ops = store.Ops(nil)
	_ storetest.Ops = storemem.Ops(nil)
	// server が呼ぶ store の op のインターフェース（server.StoreOps）も、どちらの Ops もそのまま満たす。
	// server.StoreOps は store の九つの op のうち、配線が呼ぶ八つ（addUser がない）
	_ server.StoreOps = store.Ops(nil)
	_ server.StoreOps = storemem.Ops(nil)
	// 状態の単位のインターフェースも、二つの出力で中身が同じ。一方のための実装は、もう一方の New にも渡せる
	_ storemem.UsersState = store.UsersTable{}
	_ store.UsersState    = memstore.Users{}
)

// TestTwoStoresInOneProcess は、同じ server（gen/server と人が書いた葉。どちらも一つしかない）を、一つのプロセスで
// 二つの保存先の上に組み立てられること。違うのは組み立て（onsqlite.New と onmem.New）だけ。
// 同じ出力の上の二つは、onsqlite と onmem の TestTwoServers。
func TestTwoStoresInOneProcess(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	ctx := context.Background()
	onDB, onMem := &handlertest.Fixture{}, &handlertest.Fixture{}
	_, sqliteStore, err := onsqlite.New(ctx, db, onDB.Leaves(), onDB.Mailer())
	if err != nil {
		t.Fatal(err)
	}
	_, memStore := onmem.New(memstore.New(), onMem.Leaves(), onMem.Mailer())

	// SQLite の方にだけユーザーと注文を足す
	if err := sqliteStore.AddUser(ctx, handlertest.Alice()); err != nil {
		t.Fatal(err)
	}
	if err := sqliteStore.AddOrder(ctx, pkgtype.Order{Id: "o1", UserId: handlertest.AliceID, PlacedAt: handlertest.Now}); err != nil {
		t.Fatal(err)
	}
	if user, err := sqliteStore.FindUser(ctx, handlertest.AliceEmail); err != nil || user == nil {
		t.Errorf("SQLite の store の FindUser = %v, %v", user, err)
	}
	if user, err := memStore.FindUser(ctx, handlertest.AliceEmail); err != nil || user != nil {
		t.Errorf("メモリの store の FindUser = %v, %v（期待は、ない）", user, err)
	}
	if orders, err := memStore.ListOrders(ctx, handlertest.AliceID); err != nil || len(orders) != 0 {
		t.Errorf("メモリの store の ListOrders = %v, %v（期待は空）", orders, err)
	}
}

// TestStatesFromTwoImplementations は、一つの store の状態の単位を、状態ごとに別の実装から取れること:
// users は SQLite（go-sqlite の型）、ほかの三つはメモリ（memstore の型）。store.New は、状態ごとのインターフェースを
// 満たす値を受け取るだけで、どの実装かを知らない。
func TestStatesFromTwoImplementations(t *testing.T) {
	t.Parallel()
	db, m := openDB(t), memstore.New()
	ctx := context.Background()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	users := store.NewUsersTable(db)
	s := store.New(users, m.Sessions(), m.Orders(), m.Notifications())
	if err := s.AddUser(ctx, handlertest.Alice()); err != nil {
		t.Fatal(err)
	}
	if err := s.AddOrder(ctx, pkgtype.Order{Id: "o1", UserId: handlertest.AliceID, PlacedAt: handlertest.Now}); err != nil {
		t.Fatal(err)
	}
	// ユーザーは SQLite の表に、注文はメモリにある
	if got, err := users.Get(ctx); err != nil || len(got) != 1 {
		t.Errorf("SQLite の users = %v, %v（期待は一人）", got, err)
	}
	if got, err := m.Users().Get(ctx); err != nil || len(got) != 0 {
		t.Errorf("メモリの users = %v, %v（期待は空）", got, err)
	}
	if got, err := m.Orders().Get(ctx); err != nil || len(got) != 1 {
		t.Errorf("メモリの orders = %v, %v（期待は一つ）", got, err)
	}
	if empty, err := store.NewOrdersTable(db).IsEmpty(ctx); err != nil || !empty {
		t.Errorf("SQLite の orders の IsEmpty = %v, %v（期待は true）", empty, err)
	}
}

// openDB は、メモリの上の SQLite を開く。呼ぶたびに別のデータベースになる。
func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

// TestStoreOutputsShareBaseFiles は、同じ名前空間 store の二つの出力で、基底 go のファイルがバイト単位で同じであること
// （基底の出力は、仕様と名前空間だけで決まり、単位に並べたほかのトランスパイラによらない）。
func TestStoreOutputsShareBaseFiles(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"doc.go", "types.go", "ops.go", "states.go"} {
		a, err := os.ReadFile(filepath.Join("..", "gen", "store", name))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join("..", "gen", "storemem", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("gen/store/%s と gen/storemem/%s が違う", name, name)
		}
	}
}
