// Package runtime は、hozoc が生成した Go のコードが import する型と関数を持つ（決定 D814。持ち主は、このパッケージ）。
//
// Violation、ValidationError、CheckFormat の形は docs/plugin-go-valid.md「runtime」、DBTxKey は docs/plugin-go-sqlite.md「トランザクション」。
// 生成したコードのヘッダに書く版（plugins/go/spec/go.hozo の Requirement.Layout の「ファイル」）と合わせて互換を保つので、API を変えるときは
// そちらも見る。標準ライブラリにだけ依存する（docs/packages.md「依存の向き」）。
//
// # Go の基底の出力が使うもの
//
// 生成したコードが import する `github.com/mew-ton/hozo/runtime`。基底の出力が使うのは、次の四つと、核の型 `date`・`time` の値の型（下の表）。
//
//	type Effect string
//	type Exception interface { error; HozoException() string }
//	type ClassificationError struct { Classify string; Hits []string }   // 分類の誤り（判定関数と、型で決まる分類の match が返す）。Error() を持つ
//	type CaseError struct { Effect, Param, Classify string; Want []string; Got string }   // 効果の入口の確かめの失敗（plugins/go/spec/go.hozo の Requirement.Ports の「効果の入口の確かめ」）。Error() を持つ
//
// テストのにせもの（plugins/go/spec/go.hozo の Requirement.Fakes）が使うのは、次の二つ。`testing` は、import しない。
//
//	type Fake[A, R any] struct { Calls []A; … }    // 値を返すメソッド一つのにせもの
//	func (f *Fake[A, R]) Returns(v R)               // これからの呼び出しが、v を返す（本物は、呼ばない）
//	func (f *Fake[A, R]) Throws(err error)          // これからの呼び出しが、R の零値と、err を返す（本物は、呼ばない）
//	func (f *Fake[A, R]) ReturnsAt(n int, v R)      // n 回目の呼び出しだけが、v を返す
//	func (f *Fake[A, R]) ThrowsAt(n int, err error) // n 回目の呼び出しだけが、R の零値と、err を返す
//	func (f *Fake[A, R]) Call(args A, impl func() (R, error)) (R, error)
//
//	type FakeVoid[A any] struct { Calls []A; … }   // 値を返さないメソッド一つのにせもの
//	func (f *FakeVoid[A]) Returns()                 // これからの呼び出しが、何もせずに、正常に終わる（本物は、呼ばない）
//	func (f *FakeVoid[A]) Throws(err error)         // これからの呼び出しが、err を返す（本物は、呼ばない）
//	func (f *FakeVoid[A]) ReturnsAt(n int)          // n 回目の呼び出しだけが、何もせずに、正常に終わる
//	func (f *FakeVoid[A]) ThrowsAt(n int, err error) // n 回目の呼び出しだけが、err を返す
//	func (f *FakeVoid[A]) Call(args A, impl func() error) error
//
// - `Call` は、生成したコードが呼ぶ: `args` を `Calls` に足し（呼ばれた順）、`Returns` か `Throws` が言われていれば、そのとおりに返す。言われていなくて、`impl` が `nil` でなければ、`impl` を呼んで、その結果を返す。`nil` なら、零値を返す（失敗は、ない）。
// - `Returns` と `Throws` は、後に言った方が、効く。どちらも、すべての呼び出しに、同じ結果を返す。
// - **呼び出しごとに、違う結果を返させるには、`ReturnsAt` と `ThrowsAt`**（`Fake` には、`ReturnsAt(n int, v R)` と `ThrowsAt(n int, err error)`。決定 D424）。`n` は、何回目の呼び出しか（1 から数える）。その回だけ、言われた結果を返し、本物は、呼ばない。ほかの回は、`Returns` か `Throws`、どちらも言われていなければ、本物か、零値（今までどおり）。同じ回に、二度言えば、後に言った方が、効く。同じ流れの端を、二度呼ぶ流れ（`try` の外と中。繰り返しの中）で、「一度目は成功、二度目は失敗」を、言うためのもの。
// - 零値のまま、使える。同時に呼ばれることは、守らない（テストの中で、一つの流れが呼ぶためのもの）。
//
// - 核の型 `date` と `time` の値の型も、ここに置く（plugins/go/spec/go.hozo の Requirement.Types）。どちらも、時刻帯を持たず、`==` で比べられ、`encoding/json` では、文字列になる（`MarshalText` / `UnmarshalText`）。
//
//	| 型 | 核の型 | 零値 | 文字列の形 | 作る・読む | 取り出す |
//	|---|---|---|---|---|---|
//	| `Date` | `date` | `0001-01-01` | `2006-01-02` | `NewDate(年, 月, 日)`、`DateOf(time.Time)`、`ParseDate(文字列)` | `Year` / `Month` / `Day`、`In(時刻帯)`（その日の 0 時）、`Compare`、`String` |
//	| `TimeOfDay` | `time` | `00:00:00` | `15:04:05`。秒より細かい部分は、末尾の 0 を除いた小数部（読むときは、0〜9 桁） | `NewTimeOfDay(時, 分, 秒, ナノ秒)`、`TimeOfDayOf(time.Time)`、`ParseTimeOfDay(文字列)` | `Hour` / `Minute` / `Second` / `Nanosecond`、`SinceMidnight`、`Compare`、`String`、`Fixed`（小数部を、いつも 9 桁で書く） |
//
// - `Effect` と `Exception` は `types.go`、`ClassificationError` は `classify.go` と `wiring.go` が使う。`CaseError` は、`states.go` の、入口の確かめが使う: `Effect` は効果の FQN、`Param` は引数の名前、`Classify` は分類の FQN、`Want` は、引数の型が言うケース（型に書かれた順）、`Got` は、判定関数が返したケース。仕様の例外ではない（`Exception` を満たさない）。`ops.go` と `states.go` が `runtime` を import するのは、関数の引数か戻り値に、`date` か `time` があるときと、`states.go` が、入口の確かめを持つとき。
// - `MissingError` はなくした（plugins/go/spec/go.hozo の Requirement.Ports）。
// - `runtime` には、標準の相乗りが使う型も置く: `Violation`、`ValidationError`、`CheckFormat` は `go-valid` のもの（持ち主は `docs/plugin-go-valid.md`「runtime」）。`DBTxKey`（`ctx` が運ぶ DB のトランザクションの鍵。`struct { DB *sql.DB }`）は `go-sqlite` のもの（持ち主は `docs/plugin-go-sqlite.md`「トランザクション」）。Go の出力が import するライブラリを一つにするため。`DBTxKey` は、それに加えて、同じ DB を使う二つの名前空間の出力が、同じ鍵を使うため（出力のパッケージごとの型では、共有できない）。
// - API を変えるときは、生成したコードのヘッダに書く版（plugins/go/spec/go.hozo の Requirement.Layout の「ファイル」）と合わせて互換を保つ。
package runtime
