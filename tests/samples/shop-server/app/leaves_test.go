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

	"example.com/shop-server/gen/server"
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
// throws のない六つは、値だけを返す（error を確かめる行がない）。
func TestLeaves(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 4, 5, 0, time.FixedZone("JST", 9*60*60))
	clock := now
	// 生成されたインターフェースを通して呼ぶ（*Leaves がそれを満たすことは、leaves.go がコンパイルのときに確かめている）
	var leaves server.Leaves = &Leaves{
		Now:        func() time.Time { return clock },
		NewToken:   func() pkgtype.Token { return "token-1" },
		NewOrderID: func() pkgtype.OrderId { return "order-1" },
		SessionTTL: time.Hour,
		PageSize:   2,
	}
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
	session := leaves.NewSession(alice)
	if session.Token != "token-1" || session.UserId != "u1" ||
		!session.ExpiresAt.Equal(now.Add(time.Hour)) || session.ExpiresAt.Location() != time.UTC {
		t.Errorf("newSession = %+v", session)
	}
	order := leaves.NewOrder(session, nil)
	if order.Id != "order-1" || order.UserId != "u1" || order.Lines == nil || len(order.Lines) != 0 ||
		!order.PlacedAt.Equal(now) || order.PlacedAt.Location() != time.UTC {
		t.Errorf("newOrder = %+v", order)
	}

	// requireFresh: 今が期限より前なら、同じセッション。期限ちょうどと、その後は type.Unauthorized
	var unauthorized pkgtype.Unauthorized
	for _, c := range []struct {
		elapsed time.Duration
		fresh   bool
	}{{0, true}, {time.Hour - time.Nanosecond, true}, {time.Hour, false}, {48 * time.Hour, false}} {
		clock = now.Add(c.elapsed)
		got, err := leaves.RequireFresh(session)
		if c.fresh && (err != nil || got != session) {
			t.Errorf("%v 後の requireFresh = %+v, %v（期待は、同じセッション）", c.elapsed, got, err)
		}
		if !c.fresh && !errors.As(err, &unauthorized) {
			t.Errorf("%v 後の requireFresh = %+v, %v（期待は type.Unauthorized）", c.elapsed, got, err)
		}
	}
	clock = now

	// toPublicUser: hash を落とす
	if public := leaves.ToPublicUser(alice); public != (pkgtype.PublicUser{Id: "u1", Email: "alice@example.com", Name: "Alice"}) {
		t.Errorf("toPublicUser = %+v", public)
	}

	// pageOf: 1 から数える。範囲の外は空で、1 より小さい page は 1
	orders := []pkgtype.Order{{Id: "o1"}, {Id: "o2"}, {Id: "o3"}}
	for page, want := range map[int64]string{1: "o1,o2", 2: "o3", 3: "", 0: "o1,o2", -5: "o1,o2"} {
		var ids []string
		for _, o := range leaves.PageOf(orders, page) {
			ids = append(ids, string(o.Id))
		}
		if strings.Join(ids, ",") != want {
			t.Errorf("pageOf(%d) = %s（期待は %s）", page, strings.Join(ids, ","), want)
		}
	}

	// latestOf: 最後の一つ。空なら、ない
	if latest := leaves.LatestOf(orders); latest == nil || latest.Id != "o3" {
		t.Errorf("latestOf = %+v（期待は o3）", latest)
	}
	if latest := leaves.LatestOf(nil); latest != nil {
		t.Errorf("latestOf(空) = %+v（期待は、ない）", latest)
	}

	// composeNotice
	if notice := leaves.ComposeNotice(order); notice != (pkgtype.Notification{UserId: "u1", Message: "注文 order-1 を受け付けました"}) {
		t.Errorf("composeNotice = %+v", notice)
	}
}
