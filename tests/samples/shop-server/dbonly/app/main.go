// Command app は、サンプル shop-server の「go-sqlite だけ」の組で人が書く部分（../../README.md）: 表を直に使う小さな道具。
// SQLite のデータベースを開き、@db の状態ごとの行の数を表示する。
//
// この組の server パッケージに op はない（go-sqlite は状態だけを選ぶ）。あるのは表（server.Tables）と、
// 状態の単位の問い合わせ（server.States.<状態>.IsEmpty）。
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	_ "modernc.org/sqlite" // database/sql のドライバ "sqlite"。生成された server パッケージは import しない

	"example.com/shop-server/dbonly/server"
)

func main() {
	dsn := flag.String("db", "file:shop.db?_pragma=foreign_keys(1)", "SQLite の DSN")
	flag.Parse()

	db, err := sql.Open("sqlite", *dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := server.SetupDB(db); err != nil {
		log.Fatal(err)
	}
	if err := report(os.Stdout); err != nil {
		log.Fatal(err)
	}
}

// report は @db の状態ごとに、行の数と、空かどうか（状態の単位の問い合わせ）を w に書く。
func report(w io.Writer) error {
	states := []struct {
		name    string
		count   func() (int64, error)
		isEmpty func() (bool, error)
	}{
		{"users", server.Tables.Users.Count, server.States.Users.IsEmpty},
		{"sessions", server.Tables.Sessions.Count, server.States.Sessions.IsEmpty},
		{"orders", server.Tables.Orders.Count, server.States.Orders.IsEmpty},
		{"notifications", server.Tables.Notifications.Count, server.States.Notifications.IsEmpty},
	}
	for _, s := range states {
		n, err := s.count()
		if err != nil {
			return err
		}
		empty, err := s.isEmpty()
		if err != nil {
			return err
		}
		mark := ""
		if empty {
			mark = "（空）"
		}
		if _, err := fmt.Fprintf(w, "%-13s %d%s\n", s.name, n, mark); err != nil {
			return err
		}
	}
	return nil
}
