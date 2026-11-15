package onsqlite

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"example.com/shop-server/app/storetest"
	"example.com/shop-server/gen/store"
	pkgtype "example.com/shop-server/gen/type"
	"github.com/mew-ton/hozo/runtime"
)

// newStore は、メモリの上の SQLite に表を作り、その上の store を返す（server は組み立てない。gen/store だけで動く）。
func newStore(t *testing.T) storetest.Store {
	t.Helper()
	if err := store.SetupDB(openDB(t)); err != nil {
		t.Fatal(err)
	}
	return storetest.Store{Ops: store.New(), States: states()}
}

// states は、store.States の形で決まる問い合わせを、storetest に渡す形に並べる（SetupDB の後で呼ぶ）。
// gen/store と gen/storemem の StateSet は別の型なので、共通のテストへは口を一つずつ渡す。
func states() storetest.States {
	u, s, o, n := store.States.Users, store.States.Sessions, store.States.Orders, store.States.Notifications
	return storetest.States{
		Users:         storetest.List[pkgtype.User]{IsEmpty: u.IsEmpty, Get: u.Get},
		Sessions:      storetest.List[pkgtype.Session]{IsEmpty: s.IsEmpty, Get: s.Get},
		Orders:        storetest.List[pkgtype.Order]{IsEmpty: o.IsEmpty, Get: o.Get},
		Notifications: storetest.List[pkgtype.Notification]{IsEmpty: n.IsEmpty, Get: n.Get},
	}
}

// TestStore は、go-sqlite が出した store の op の振る舞い。onmem の TestStore と同じ storetest.Run を走らせる。
func TestStore(t *testing.T) {
	storetest.Run(t, newStore)
}

// TestSetupDBFillsStates は、SetupDB が store.States のすべての口を埋めること: 四つの状態の、形で決まる問い合わせ
// （IsEmpty、Get）が八つと、op が九つ（この仕様の store の op は九つとも、go-sqlite の規則で中身が決まる）。
// 埋まっていない口を調べる関数は生成されないので、リフレクションで数える。
func TestSetupDBFillsStates(t *testing.T) {
	store.States = store.StateSet{}
	// 埋める前は、どの op も *runtime.MissingError（Kind は "state"）
	_, err := store.New().FindUser(context.Background(), "alice@example.com")
	var missing *runtime.MissingError
	if !errors.As(err, &missing) || missing.Kind != "state" || missing.Name != "store.findUser" {
		t.Errorf("SetupDB の前の FindUser のエラーが %v", err)
	}
	// 形で決まる問い合わせは Ops を通らない（States のフィールドを直に呼ぶ）ので、埋める前は nil の関数のまま
	if store.States.Notifications.Get != nil {
		t.Error("SetupDB の前の States.Notifications.Get が nil でない")
	}

	if err := store.SetupDB(openDB(t)); err != nil {
		t.Fatal(err)
	}
	filled := 0
	states := reflect.ValueOf(store.States)
	for i := 0; i < states.NumField(); i++ {
		unit := states.Field(i)
		for j := 0; j < unit.NumField(); j++ {
			if unit.Field(j).IsNil() {
				t.Errorf("States.%s.%s が埋まっていない", states.Type().Field(i).Name, unit.Type().Field(j).Name)
			}
			filled++
		}
	}
	if filled != 17 {
		t.Errorf("States の口が %d（期待は 17）", filled)
	}
}

// TestColumns は、go-sqlite が決めた列の形: Time は RFC 3339・UTC・小数部 9 桁の TEXT、配列は JSON の TEXT。
func TestColumns(t *testing.T) {
	db := openDB(t)
	if err := store.SetupDB(db); err != nil {
		t.Fatal(err)
	}
	s, ctx := store.New(), context.Background()
	if err := s.AddUser(ctx, pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: "h"}); err != nil {
		t.Fatal(err)
	}
	placedAt := time.Date(2026, 1, 2, 12, 4, 5, 120000000, time.FixedZone("JST", 9*60*60))
	order := pkgtype.Order{Id: "o1", UserId: "u1", Lines: []pkgtype.OrderLine{{Item: "apple", Quantity: 2}}, PlacedAt: placedAt}
	if err := s.AddOrder(ctx, order); err != nil {
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
