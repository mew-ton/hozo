package app

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"example.com/shop-server/gen/server"
	pkgtype "example.com/shop-server/gen/type"
)

// TestNoStorageImport は、このパッケージ（server の葉の op と mailer.send の実装）が保存先を知らないこと:
// テストでないファイルは、gen/store も、保存先との組み立ても import しない。
func TestNoStorageImport(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			switch path {
			case "example.com/shop-server/gen/server", "example.com/shop-server/gen/type":
			default:
				if strings.HasPrefix(path, "example.com/shop-server/") || path == "database/sql" {
					t.Errorf("%s が %s を import している", name, path)
				}
			}
		}
	}
}

// TestLeaves は、葉の op の本体。どれも引数だけで決まる（保存先に触れず、受け取った ctx も見ない）。
// どのメソッドも ctx を取り、error を返す（plugins/go/spec/go.hozo の Requirement.OpShape）。throws のない五つが error を返すのは、
// トークンと識別子を作れなかったとき（newSession、newOrder）だけ。
func TestLeaves(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 4, 5, 0, time.FixedZone("JST", 9*60*60))
	clock := now
	// 生成されたインターフェースを通して呼ぶ（*Leaves がそれを満たすことは、leaves.go がコンパイルのときに確かめている）
	var leaves server.Leaves = &Leaves{
		Now:        func() time.Time { return clock },
		NewToken:   func() (pkgtype.Token, error) { return "token-1", nil },
		NewOrderID: func() (pkgtype.OrderId, error) { return "order-1", nil },
		SessionTTL: time.Hour,
		PageSize:   2,
	}
	ctx := context.Background()
	alice := pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: HashPassword("password123")}

	// checkPassword: 合わなければ、仕様の例外を値で返す（go-http は値の型で見分ける）
	if err := leaves.CheckPassword(ctx, alice, "password123"); err != nil {
		t.Errorf("checkPassword = %v", err)
	}
	var invalid pkgtype.InvalidCredentials
	if err := leaves.CheckPassword(ctx, alice, "wrong-password"); !errors.As(err, &invalid) {
		t.Errorf("checkPassword = %v（期待は type.InvalidCredentials）", err)
	}

	// newSession と newOrder: 時刻は UTC にそろえる
	session, err := leaves.NewSession(ctx, alice)
	if err != nil || session.Token != "token-1" || session.UserId != "u1" ||
		!session.ExpiresAt.Equal(now.Add(time.Hour)) || session.ExpiresAt.Location() != time.UTC {
		t.Errorf("newSession = %+v, %v", session, err)
	}
	order, err := leaves.NewOrder(ctx, session, nil)
	if err != nil || order.Id != "order-1" || order.UserId != "u1" || order.Lines == nil || len(order.Lines) != 0 ||
		!order.PlacedAt.Equal(now) || order.PlacedAt.Location() != time.UTC {
		t.Errorf("newOrder = %+v, %v", order, err)
	}

	// requireFresh: 今が期限より前なら、同じセッション。期限ちょうどと、その後は type.Unauthorized
	var unauthorized pkgtype.Unauthorized
	for _, c := range []struct {
		elapsed time.Duration
		fresh   bool
	}{{0, true}, {time.Hour - time.Nanosecond, true}, {time.Hour, false}, {48 * time.Hour, false}} {
		clock = now.Add(c.elapsed)
		got, err := leaves.RequireFresh(ctx, session)
		if c.fresh && (err != nil || got != session) {
			t.Errorf("%v 後の requireFresh = %+v, %v（期待は、同じセッション）", c.elapsed, got, err)
		}
		if !c.fresh && !errors.As(err, &unauthorized) {
			t.Errorf("%v 後の requireFresh = %+v, %v（期待は type.Unauthorized）", c.elapsed, got, err)
		}
	}
	clock = now

	// toPublicUser: hash を落とす
	if public, err := leaves.ToPublicUser(ctx, alice); err != nil || public != (pkgtype.PublicUser{Id: "u1", Email: "alice@example.com", Name: "Alice"}) {
		t.Errorf("toPublicUser = %+v, %v", public, err)
	}

	// pageOf: 1 から数える。範囲の外は空で、1 より小さい page は 1
	orders := []pkgtype.Order{{Id: "o1"}, {Id: "o2"}, {Id: "o3"}}
	for page, want := range map[int64]string{1: "o1,o2", 2: "o3", 3: "", 0: "o1,o2", -5: "o1,o2"} {
		var ids []string
		paged, err := leaves.PageOf(ctx, orders, page)
		if err != nil {
			t.Errorf("pageOf(%d) = %v", page, err)
		}
		for _, o := range paged {
			ids = append(ids, string(o.Id))
		}
		if strings.Join(ids, ",") != want {
			t.Errorf("pageOf(%d) = %s（期待は %s）", page, strings.Join(ids, ","), want)
		}
	}

	// composeNotice
	if notice, err := leaves.ComposeNotice(ctx, order); err != nil || notice != (pkgtype.Notification{UserId: "u1", Message: "注文 order-1 を受け付けました"}) {
		t.Errorf("composeNotice = %+v, %v", notice, err)
	}
}
