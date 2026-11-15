package app

import (
	"errors"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	pkgtype "example.com/shop-server/gen/type"
)

// TestNoStorageImport は、このパッケージ（server の葉の op と mailer.send の実装）が保存先を知らないこと:
// テストでないファイルは、gen/store も gen/storemem も、保存先との組み立ても import しない。
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

// TestLeaves は、葉の op の本体。どれも引数だけで決まる（ctx を取らず、保存先に触れない）。
func TestLeaves(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 4, 5, 0, time.FixedZone("JST", 9*60*60))
	l := &Leaves{
		Now:        func() time.Time { return now },
		NewToken:   func() pkgtype.Token { return "token-1" },
		NewOrderID: func() pkgtype.OrderId { return "order-1" },
		SessionTTL: time.Hour,
		PageSize:   2,
	}
	leaves := l.Server()
	alice := pkgtype.User{Id: "u1", Email: "alice@example.com", Name: "Alice", Hash: HashPassword("password123")}

	// checkPassword: 合わなければ、仕様の例外を値で返す（go-http は値の型で見分ける）
	if err := leaves.CheckPassword(alice, "password123"); err != nil {
		t.Errorf("checkPassword = %v", err)
	}
	var invalid pkgtype.InvalidCredentials
	if err := leaves.CheckPassword(alice, "wrong-password"); !errors.As(err, &invalid) {
		t.Errorf("checkPassword = %v（期待は type.InvalidCredentials）", err)
	}

	// newSession と newOrder: 時刻は UTC にそろえる
	session, err := leaves.NewSession(alice)
	if err != nil || session.Token != "token-1" || session.UserId != "u1" ||
		!session.ExpiresAt.Equal(now.Add(time.Hour)) || session.ExpiresAt.Location() != time.UTC {
		t.Errorf("newSession = %+v, %v", session, err)
	}
	order, err := leaves.NewOrder(session, nil)
	if err != nil || order.Id != "order-1" || order.UserId != "u1" || order.Lines == nil || len(order.Lines) != 0 ||
		!order.PlacedAt.Equal(now) || order.PlacedAt.Location() != time.UTC {
		t.Errorf("newOrder = %+v, %v", order, err)
	}

	// toPublicUser: hash を落とす
	public, err := leaves.ToPublicUser(alice)
	if err != nil || public != (pkgtype.PublicUser{Id: "u1", Email: "alice@example.com", Name: "Alice"}) {
		t.Errorf("toPublicUser = %+v, %v", public, err)
	}

	// pageOf: 1 から数える。範囲の外は空で、1 より小さい page は 1
	orders := []pkgtype.Order{{Id: "o1"}, {Id: "o2"}, {Id: "o3"}}
	for page, want := range map[int64]string{1: "o1,o2", 2: "o3", 3: "", 0: "o1,o2", -5: "o1,o2"} {
		got, err := leaves.PageOf(orders, page)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, o := range got {
			ids = append(ids, string(o.Id))
		}
		if strings.Join(ids, ",") != want {
			t.Errorf("pageOf(%d) = %s（期待は %s）", page, strings.Join(ids, ","), want)
		}
	}

	// composeNotice
	notice, err := leaves.ComposeNotice(order)
	if err != nil || notice != (pkgtype.Notification{UserId: "u1", Message: "注文 order-1 を受け付けました"}) {
		t.Errorf("composeNotice = %+v, %v", notice, err)
	}
}
