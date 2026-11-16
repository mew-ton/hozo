package runtime

import "database/sql"

// DBTxKey は、ctx が運ぶ DB のトランザクション（*sql.Tx）の鍵（docs/plugin-go-sqlite.md「トランザクション」）。DB ごとに一つ。
// @db.transaction の継ぎ目の実装が `context.WithValue(ctx, runtime.DBTxKey{DB: db}, tx)` で置き、文を実行するところが
// `ctx.Value(runtime.DBTxKey{DB: db}).(*sql.Tx)` で引く。
//
// 型をここに置くのは、鍵を、生成したパッケージをまたいで同じにするため: 出力のパッケージごとの型にすると、同じ db の上でも、
// 別の名前空間の出力は、そのトランザクションを見つけられない（文が、トランザクションの外で実行される）。
// 値を比べるのは Go の context（鍵の == ）なので、同じ *sql.DB から作った鍵は、どのパッケージで作っても等しい。
type DBTxKey struct {
	DB *sql.DB
}
