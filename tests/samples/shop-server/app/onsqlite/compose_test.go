package onsqlite

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite" // database/sql のドライバ "sqlite"。生成された gen/store は import しない

	"example.com/shop-server/app/flowtest"
	"example.com/shop-server/app/handlertest"
	"example.com/shop-server/gen/store"
)

// openDB は、メモリの上の SQLite を開く。呼ぶたびに別のデータベースになる。
func openDB(t *testing.T) *sql.DB {
	t.Helper()
	// 外部キー（@db.ref の REFERENCES）は、接続ごとに有効にしないと SQLite は確かめない。ドライバの DSN で指定する
	db, err := sql.Open("sqlite", "file::memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	// ":memory:" は接続ごとに別のデータベースになるので、接続を一つに限る
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

// newServer は、新しいデータベースの上に組み立てたサーバー。
func newServer(t *testing.T, f *handlertest.Fixture) handlertest.Server {
	db := openDB(t)
	handler, s, err := New(context.Background(), db, f.Leaves(), f.Mailer(), f.Limiter())
	if err != nil {
		t.Fatal(err)
	}
	return handlertest.Server{
		Handler: handler,
		Store:   s,
		// 通知の read は仕様にないので、状態の単位の、形で決まる問い合わせで読む（go-sqlite の型のメソッド。
		// 同じ db の上の値なら、組み立てに渡したものと同じ表を読む）
		Notifications: store.NewNotificationsTable(db).Get,
	}
}

// TestHandler は、go-http のハンドラ → 配線 → go-sqlite が出した read と効果、の全体の振る舞い。
// handlertest.Run を走らせる。
func TestHandler(t *testing.T) {
	t.Parallel()
	handlertest.Run(t, newServer)
}

// TestTwoServers は、同じ出力（gen/store と gen/server）の上の二つのサーバーが、一つのプロセスで同時に動き、
// 互いに混ざらないこと: SQLite のデータベースが二つ、store.New と server.New が二度。
func TestTwoServers(t *testing.T) {
	t.Parallel()
	handlertest.RunTwo(t, newServer)
}

// newSystem は、新しいデータベースの上に、server、client、flows を重ねたもの。
func newSystem(t *testing.T, f *handlertest.Fixture) flowtest.System {
	flowOps, clientOps, s, err := NewFlows(context.Background(), openDB(t), f.Leaves(), f.Mailer(), f.Limiter())
	if err != nil {
		t.Fatal(err)
	}
	return flowtest.System{Flows: flowOps, Client: clientOps, Store: s}
}

// TestFlows は、フローの E2E テスト（plugins/go-design/spec/go-design.hozo の Requirement.FlowPort）: フロー → 操作の op → 機能入口 → 配線 → go-sqlite が
// 出した read と効果。go-design が出した flowstest.RunFlows に、flowtest.FlowCases を渡す。
func TestFlows(t *testing.T) {
	t.Parallel()
	flowtest.RunFlows(t, newSystem)
}

// TestRequirements は、要求のテスト（同「要求の、テストの入れ物の口」）: 要求ごとの入れ物を、flowstest.RunRequirements が走らせる。
func TestRequirements(t *testing.T) {
	t.Parallel()
	flowtest.RunRequirements(t, newSystem)
}

// TestIncompleteFlowCasesAreRejected は、ケースか、入れ物を欠いた型が、口 flowstest.FlowCases、flowstest.Requirements を満たさないこと。
func TestIncompleteFlowCasesAreRejected(t *testing.T) { flowtest.RunIncomplete(t) }
