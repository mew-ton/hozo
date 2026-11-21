package app_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"example.com/shop-server/app/storetest"
	"example.com/shop-server/gen/server"
	"example.com/shop-server/gen/server/inner"
	"example.com/shop-server/gen/store"
)

// store の出力（gen/store）の Ops が満たすもの。型は、共有の gen/type のものなので、別のパッケージが、同じメソッドの並びの
// インターフェースを書けば、Ops は、そのまま満たす。
var (
	// storetest.Ops は、store のテストが呼ぶ op を、人が書き並べたインターフェース
	_ storetest.Ops = store.Ops(nil)
	// server が呼ぶ store の op のインターフェース（server.StoreOps）も、そのまま満たす。
	// server.StoreOps は store の十の op のうち、配線が呼ぶ九つ（addUser がない）。別の名前空間から呼べるのは export を
	// 付けた op だけで、Ops にはそれがすべて並ぶので、<Ns>Ops のメソッドは、いつも相手の Ops にある
	_ server.StoreOps = store.Ops(nil)
	// 継ぎ目の口（Seams）の実装は、@db.transaction の継ぎ目を宣言した go-sqlite が出した型
	_ store.Seams = store.DbSeams{}
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
//   - server: 二十の op のうち、HTTP の入口の八つ。内部の op（配線 issueSession、authenticate、sessionOf と、九つの葉）は、
//     入口の Ops にはなく、内側のパッケージ（gen/server/inner）の Ops にある（二十のすべて）。
//   - store: 十とも export。内部の op がないので、出力を分けない（内側のパッケージがない）。server が呼ぶ九つ
//     （server.StoreOps）は、その中にある。read と効果は op ではないので、Ops にはない（状態の単位のメソッド）。
//
// 確かめるのは、インターフェースのメソッド。内部の op を、入口の New が返す値から取り出せないことは、約束していない
// （守るのは Hozo の検査 NS-5。生成したコードは、置き方で分けるだけ）。
func TestOpsHoldExportedOpsOnly(t *testing.T) {
	t.Parallel()
	entries := []string{"GetLatestOrder", "GetMe", "GetMyPage", "GetOrders", "Login", "Logout", "PlaceOrder", "ResendNotices"}
	if got := methodNames(reflect.TypeFor[server.Ops]()); !slices.Equal(got, entries) {
		t.Errorf("server.Ops のメソッド = %v（期待は %v）", got, entries)
	}
	everything := []string{
		"Authenticate", "CheckPassword", "ComposeNotice", "GetLatestOrder", "GetMe", "GetMyPage", "GetOrders", "IssueSession",
		"Login", "Logout", "MyPage", "NewOrder", "NewSession", "PageOf", "PlaceOrder", "RecordAudit", "RequireFresh", "ResendNotices",
		"SessionOf", "Summarize", "ToPublicUser",
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

	// store の継ぎ目も、op と、継ぎ目を宣言されたデコレータの組ごとに一つ: placeOrder の @db.transaction だけ
	if got := methodNames(reflect.TypeFor[store.Seams]()); !slices.Equal(got, []string{"PlaceOrderDbTransaction"}) {
		t.Errorf("store.Seams のメソッド = %v（期待は PlaceOrderDbTransaction だけ）", got)
	}

	all := []string{
		"AddSession", "AddUser", "FindSession", "FindUser", "FindUserById", "LatestOrder", "ListOrders", "PlaceOrder", "RecordNotice", "RemoveSession",
	}
	storeOps := reflect.TypeFor[server.StoreOps]()
	if got := methodNames(reflect.TypeFor[store.Ops]()); !slices.Equal(got, all) {
		t.Errorf("gen/store の Ops のメソッド = %v（期待は %v）", got, all)
	}
	if !reflect.TypeFor[store.Ops]().Implements(storeOps) {
		t.Error("gen/store の Ops が server.StoreOps を満たさない")
	}
	// server.StoreOps は、store.Ops から addUser を除いたもの
	called := slices.DeleteFunc(slices.Clone(all), func(name string) bool { return name == "AddUser" })
	if got := methodNames(storeOps); !slices.Equal(got, called) {
		t.Errorf("server.StoreOps のメソッド = %v（期待は %v）", got, called)
	}
	// 内部の op を持たない出力（store、型だけの type と rate）には、内側のフォルダがない
	for _, dir := range []string{"store", "type", "rate"} {
		if _, err := os.Stat(filepath.Join("..", "gen", dir, "inner")); !os.IsNotExist(err) {
			t.Errorf("gen/%s/inner がある（内部の op を持たない出力は、分けない）: %v", dir, err)
		}
	}
}
