package app_test

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	_ "modernc.org/sqlite"

	"example.com/shop-server/app/handlertest"
	"example.com/shop-server/app/memstore"
	"example.com/shop-server/app/onmem"
	"example.com/shop-server/app/onsqlite"
	"example.com/shop-server/app/storetest"
	"example.com/shop-server/gen/server"
	"example.com/shop-server/gen/server/inner"
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
	// server.StoreOps は store の八つの op のうち、配線が呼ぶ七つ（addUser がない）。別の名前空間から呼べるのは export を
	// 付けた op だけで、Ops にはそれがすべて並ぶので、<Ns>Ops のメソッドは、いつも相手の Ops にある
	_ server.StoreOps = store.Ops(nil)
	_ server.StoreOps = storemem.Ops(nil)
	// 状態の単位のインターフェースも、二つの出力で中身が同じ。一方のための実装は、もう一方の New にも渡せる
	_ storemem.UsersState = store.UsersTable{}
	_ store.UsersState    = memstore.Users{}
	// 複数の状態にまたがる op の口（SharedState）も同じ
	_ storemem.SharedState = store.SharedTable{}
	_ store.SharedState    = memstore.Shared{}
)

// methodNames は、型 T の公開のメソッドの名前（名前順）。T がインターフェースなら、そのメソッドのすべて。
func methodNames(typ reflect.Type) []string {
	var names []string
	for i := range typ.NumMethod() {
		names = append(names, typ.Method(i).Name)
	}
	return names
}

// TestOpsHoldExportedOpsOnly は、入口の Ops に並ぶのが、仕様で export を付けた op だけであること。
//   - server: 十五の op のうち、HTTP の入口の五つ。内部の op（配線 issueSession、authenticate、sessionOf と、七つの葉）は、
//     入口の Ops にはなく、内側のパッケージ（gen/server/inner）の Ops にある（十五のすべて）。
//   - store: 八つとも export。内部の op がないので、出力を分けない（内側のパッケージがない）。server が呼ぶ七つ
//     （server.StoreOps）は、その中にある。
//
// 確かめるのは、インターフェースのメソッド。内部の op を、入口の New が返す値から取り出せないことは、約束していない
// （守るのは Hozo の検査 NS-5。生成したコードは、置き方で分けるだけ）。
func TestOpsHoldExportedOpsOnly(t *testing.T) {
	t.Parallel()
	entries := []string{"GetMe", "GetOrders", "Login", "Logout", "PlaceOrder"}
	if got := methodNames(reflect.TypeFor[server.Ops]()); !slices.Equal(got, entries) {
		t.Errorf("server.Ops のメソッド = %v（期待は %v）", got, entries)
	}
	everything := []string{
		"Authenticate", "CheckPassword", "ComposeNotice", "GetMe", "GetOrders", "IssueSession", "Login", "Logout",
		"NewOrder", "NewSession", "PageOf", "PlaceOrder", "RequireFresh", "SessionOf", "ToPublicUser",
	}
	if got := methodNames(reflect.TypeFor[inner.Ops]()); !slices.Equal(got, everything) {
		t.Errorf("inner.Ops のメソッド = %v（期待は %v）", got, everything)
	}
	// 内側の Ops は、入口の Ops を満たす（入口の New は、内側の New の戻り値を、そのまま返す）
	if !reflect.TypeFor[inner.Ops]().Implements(reflect.TypeFor[server.Ops]()) {
		t.Error("inner.Ops が server.Ops を満たさない")
	}
	// 継ぎ目のメソッドは、継ぎ目を持つ op と、継ぎ目を宣言されたデコレータの組ごとに一つ: login の @rate.limited だけ。
	// @http.post などの、継ぎ目を宣言されていないデコレータの分はない
	if got := methodNames(reflect.TypeFor[server.Seams]()); !slices.Equal(got, []string{"LoginRateLimited"}) {
		t.Errorf("server.Seams のメソッド = %v（期待は LoginRateLimited だけ）", got)
	}

	all := []string{
		"AddSession", "AddUser", "FindSession", "FindUser", "FindUserById", "ListOrders", "PlaceOrder", "RemoveSession",
	}
	storeOps := reflect.TypeFor[server.StoreOps]()
	for name, typ := range map[string]reflect.Type{
		"gen/store": reflect.TypeFor[store.Ops](), "gen/storemem": reflect.TypeFor[storemem.Ops](),
	} {
		if got := methodNames(typ); !slices.Equal(got, all) {
			t.Errorf("%s の Ops のメソッド = %v（期待は %v）", name, got, all)
		}
		if !typ.Implements(storeOps) {
			t.Errorf("%s の Ops が server.StoreOps を満たさない", name)
		}
	}
	// server.StoreOps は、store.Ops から addUser を除いたもの
	called := slices.DeleteFunc(slices.Clone(all), func(name string) bool { return name == "AddUser" })
	if got := methodNames(storeOps); !slices.Equal(got, called) {
		t.Errorf("server.StoreOps のメソッド = %v（期待は %v）", got, called)
	}
	// 内部の op を持たない出力（store、型だけの type と rate）には、内側のフォルダがない
	for _, dir := range []string{"store", "storemem", "type", "rate"} {
		if _, err := os.Stat(filepath.Join("..", "gen", dir, "inner")); !os.IsNotExist(err) {
			t.Errorf("gen/%s/inner がある（内部の op を持たない出力は、分けない）: %v", dir, err)
		}
	}
}

// TestTwoStoresInOneProcess は、同じ server（gen/server と人が書いた葉。どちらも一つしかない）を、一つのプロセスで
// 二つの保存先の上に組み立てられること。違うのは組み立て（onsqlite.New と onmem.New）だけ。
// 同じ出力の上の二つは、onsqlite と onmem の TestTwoServers。
func TestTwoStoresInOneProcess(t *testing.T) {
	t.Parallel()
	db := openDB(t)
	ctx := context.Background()
	onDB, onMem := &handlertest.Fixture{}, &handlertest.Fixture{}
	_, sqliteStore, err := onsqlite.New(ctx, db, onDB.Leaves(), onDB.Mailer(), onDB.Limiter())
	if err != nil {
		t.Fatal(err)
	}
	_, memStore := onmem.New(memstore.New(), onMem.Leaves(), onMem.Mailer(), onMem.Limiter())

	// SQLite の方にだけユーザーと注文を足す
	if err := sqliteStore.AddUser(ctx, handlertest.Alice()); err != nil {
		t.Fatal(err)
	}
	order := pkgtype.Order{Id: "o1", UserId: handlertest.AliceID, PlacedAt: handlertest.Now}
	if err := sqliteStore.PlaceOrder(ctx, order, pkgtype.Notification{UserId: handlertest.AliceID, Message: "x"}); err != nil {
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
// 満たす値を受け取るだけで、どの実装かを知らない。二つの状態（orders と notifications）にまたがる op（placeOrder）の
// 実装は、その二つの状態と同じ保存先のもの（memstore の Shared）を渡す。
func TestStatesFromTwoImplementations(t *testing.T) {
	t.Parallel()
	db, m := openDB(t), memstore.New()
	ctx := context.Background()
	if err := store.CreateTables(ctx, db); err != nil {
		t.Fatal(err)
	}
	users := store.NewUsersTable(db)
	s := store.New(users, m.Sessions(), m.Orders(), m.Notifications(), m.Shared())
	if err := s.AddUser(ctx, handlertest.Alice()); err != nil {
		t.Fatal(err)
	}
	order := pkgtype.Order{Id: "o1", UserId: handlertest.AliceID, PlacedAt: handlertest.Now}
	if err := s.PlaceOrder(ctx, order, pkgtype.Notification{UserId: handlertest.AliceID, Message: "x"}); err != nil {
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
