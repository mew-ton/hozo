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
)

// TestTwoStoresInOneProcess は、同じ server（gen/server と人が書いた葉。どちらも一つしかない）を、一つのプロセスで
// 二つの保存先の上に組み立てられること。違うのは組み立て（onsqlite.New と onmem.New）だけ。
//
// 二つが混ざらないのは、状態の変数 States が出力ごとの別のパッケージ（gen/store と gen/storemem）にあるため。
// 同じ出力の上に二つ（SQLite の DB を二つ）は持てない: 最後に組み立てた方だけが効く。
func TestTwoStoresInOneProcess(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	ctx := context.Background()
	onDB, onMem := &handlertest.Fixture{}, &handlertest.Fixture{}
	_, sqliteStore, err := onsqlite.New(db, onDB.Leaves(), onDB.Send())
	if err != nil {
		t.Fatal(err)
	}
	_, memStore, _ := onmem.New(onMem.Leaves(), onMem.Send())

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

// TestStoreOpsFillDeps は、store の op が、server が呼ぶ口（server.StoreOps）にそのまま入ること。
// 入るのは Ops のメソッド（名前のない関数の型）で、States のフィールド（store.FindUserFunc）は、名前の違う関数の型
// （server.StoreFindUserFunc）なので、型を変換しなければ入らない。
func TestStoreOpsFillDeps(t *testing.T) {
	var deps server.StoreOps
	deps.FindUser = store.New().FindUser
	deps.FindUser = storemem.New().FindUser
	deps.FindUser = server.StoreFindUserFunc(store.States.Users.FindUser)
	deps.FindUser = server.StoreFindUserFunc(storemem.States.Users.FindUser)
	_ = deps
}

// TestStoreOutputsShareBaseFiles は、同じ名前空間 store の二つの出力で、基底 go のファイルがバイト単位で同じであること
// （基底の出力は、仕様と名前空間だけで決まり、単位に並べたほかのトランスパイラによらない）。
func TestStoreOutputsShareBaseFiles(t *testing.T) {
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
