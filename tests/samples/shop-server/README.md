# サンプル `shop-server`

小さな店のサーバー（ログイン、自分の情報、注文）を、`docs/decisions.md`「決めたが、まだ文書と実装に入れていないこと」の **P1〜P32 を当てはめた後の形**で書いたもの。サンプルとは何か、置き方、テストの使い方は [`../README.md`](../README.md)。

P1〜P32 の言語の変更も、ここに出てくる四つのトランスパイラ（`go`、`go-valid`、`go-sqlite`、`go-http`）も、まだない。単位の `out`（`gen/`）の下のファイルはすべて、「`hozoc emit` がいつかこう出す」内容を人が書いたもので、トランスパイラを実装したときのブラックボックスのテストの仕様になる。コードはコンパイルでき、人が書いた部分（`app/`）と合わせて動く。

P1〜P32 を当てはめる前の形（一つの名前空間 `server` を三つの単位で出力する。以下「v1」）は、コミット `ac57788` のこのディレクトリにある。

## 保留

`sample.json` の `pending` が、`TestSamples` の `check` と `shared` を、このサンプルについて保留にしている（保留の仕組みは `../README.md`「保留」）。

- `check`: 仕様と設定が、今の hozoc が読めない書き方を使う。使っている書き方と、それを決めた決定は「入力」の表。
- `shared`: 期待する出力のどのファイルも、今の hozoc の既定の出力（`docs/emit-go.md`）が出す形ではない。基底の `go` も、まだないトランスパイラの一つ（P9）。

## 入力

仕様は名前空間ごとに一つのファイル。「まだない書き方」の語（`reads`、`as`、`throw`、`never`、`override`、`[emit.options.<トランスパイラ>]`）は、どれも `docs/decisions.md` が仮に置いた語のまま。

| ファイル | 名前空間 | 中身 | まだない書き方 |
|---|---|---|---|
| `type.hozo` | `type` | 共有の型（P6）: scalar、model、例外 | `model PublicUser extends User { override hash: never; }`（P24〜P26） |
| `store.hozo` | `store` | DB の状態四つ（P5）、それを読む op と書く op、読んだ結果の分類 | `reads`（P19）、`classify C of <op>`（P2）、`Case: <op> as T;`（P3） |
| `server.hozo` | `server` | HTTP の入口と、業務の流れ（配線）。人が書く op は七つ | `match x by C` の絞り込み（P3）、`throw E`（P13） |
| `mailer.hozo` | `mailer` | 通知を外へ届ける仕組み。印のデコレータ `remote` と、それを付けた op `send` | — |

`hozo.toml` の単位は四つ。`mailer` には単位を置かないので、コードは出ない。

| 単位の `out` | `namespace` | `transpilers` | 中身を埋める者 |
|---|---|---|---|
| `gen/type` | `type` | `go`、`go-valid` | —（型だけ） |
| `gen/store` | `store` | `go`、`go-sqlite` | `go-sqlite`: 四つの状態の単位の実装（九つの op と、状態の形で決まる問い合わせのすべて） |
| `gen/storemem` | `store` | `go` | 人: `app/memstore`（四つの状態の単位の実装。メモリの上） |
| `gen/server` | `server` | `go`、`go-valid`、`go-http` | 人: `app/leaves.go`（葉の op 七つ）、`app/mailer.go`（`mailer.send`） |

`gen/server` の単位だけ、`[emit.options.go-http]` で `validate = true` を渡す（P16）。

### 下書きから変えたこと

仕様の元は、`ac57788` までこのディレクトリにあった下書き `NEXT.md`（P1〜P19 を当てはめたもの）。そこから変えたのは次の六つ。

1. **継承**（P24〜P27）: `model PublicUser is User omit hash;` を `model PublicUser extends User { override hash: never; }` にした（`type.hozo:41-43`）。`type.User` を `type.PublicUser` に写す葉の op `toPublicUser` を `server` に足し（`server.hozo:71`）、下書きが `User` をそのまま渡していた二か所で呼ぶ（`server.hozo:59`、`:107`）。
2. **外部の入出力**（P30）: `server` の葉 `deliver` をやめ、名前空間 `mailer` の `op send` にし、プロセスの外にあることを示す印のデコレータ `@remote` を付けた（`mailer.hozo:7`、`:10-12`、`server.hozo:147`）。デコレータを持つ op は純粋でない。
3. **`store.addUser` と効果 `createUser` を足した**（`store.hozo:56`、`:71`）。下書きにない変更。ユーザーを足す op がなければ、起動のときとテストが、保存先を直に書き換えるしかない（v1 の「分かったこと」26）。
4. **`getMe` の `Found` のアームは、呼び出しをそのまま書いた**（`server.hozo:107` の `Found => toPublicUser(user);`）。写した値を先に束縛する形にはできない: `user` が `type.User` になるのはアームの中だけ。
5. **セッションの期限を確かめる**: 下書きは `expiresAt` を書くだけで、誰も読まなかった（期限の切れたトークンで入れた。「分かったこと」N3）。人が書く葉の op `requireFresh` を `server` に足し（`server.hozo:86-88`）、配線 `authenticate` の `LoggedIn` のアームを `LoggedIn => requireFresh(session);` にした（`server.hozo:81`）。期限が過ぎていれば `type.Unauthorized`。下書きのアームは値のアーム（P14 の `LoggedIn => session;`）だったので、この仕様に値のアームはなくなった。
6. 説明のコメントを足した。

## ファイルの出どころ

| ファイル | 出どころ |
|---|---|
| `type.hozo`、`store.hozo`、`server.hozo`、`mailer.hozo`、`hozo.toml` | 人が書く（入力） |
| `sample.json` | 人が書く（保留の理由と、各トランスパイラが出すファイル。`../README.md`「sample.json」） |
| `go.mod`、`go.sum` | 人が書く（「モジュール」） |
| `gen/type/doc.go` `types.go` | `go` |
| `gen/type/validate.go` | `go-valid` |
| `gen/store/doc.go` `types.go` `ops.go` `states.go` | `go` |
| `gen/store/db.go` | `go-sqlite` |
| `gen/storemem/doc.go` `types.go` `ops.go` `states.go` | `go`（`gen/store` の同じ名前のファイルとバイト単位で同じ） |
| `gen/server/doc.go` `types.go` `ops.go` `wiring.go` | `go` |
| `gen/server/validate.go` | `go-valid` |
| `gen/server/http.go` | `go-http` |
| `gen/*/.hozoc-outputs.json` | hozoc の記録（`docs/cli.md`「emit」）。その出力のすべてのファイルのパス |
| `app/leaves.go`、`app/mailer.go` | 人が書く: `server` の葉の op 七つの本体（`server.Leaves` を満たす型）と、`mailer.send` の実装（`server.MailerOps` を満たす型）。保存先を知らない |
| `app/memstore/memstore.go` | 人が書く: `store` の四つの状態の単位の、メモリの上の実装（`gen/storemem` の `New` に渡す型） |
| `app/onsqlite/compose.go`、`app/onmem/compose.go` | 人が書く: 二つの組み立て（`server` を `gen/store` の上に、`gen/storemem` の上に） |
| `app/cmd/shop-sqlite/main.go`、`app/cmd/shop-mem/main.go` | 人が書く: それぞれの組み立てで待ち受ける main |
| `app/handlertest/`、`app/storetest/` | 人が書く: 二つの組み立てが共に走らせるテスト（HTTP から見た振る舞い、`store` の op と状態の問い合わせの振る舞い） |
| `app/*_test.go`、`app/*/*_test.go` | 人が書く: テスト（「人が書いたもの」） |

## モジュール

- サンプルは自分のモジュール `example.com/shop-server`（`go.mod`）。生成したコードが import する `github.com/mew-ton/hozo/runtime` は `replace github.com/mew-ton/hozo => ../../..` で解決する。
- SQLite のドライバは cgo の要らない `modernc.org/sqlite`。import するのは人が書くコード（`app/cmd/shop-sqlite` と、テスト）だけ。期待する出力（`gen/`）は `database/sql` しか import しない。
- ドライバは `v1.46.1` に固定している。それより新しい版は、リポジトリが使う Go（1.24）より新しいツールチェーンを求める。`GOTOOLCHAIN=local` で動かす。

```
GOTOOLCHAIN=local go vet ./... && GOTOOLCHAIN=local go test -race ./...
GOTOOLCHAIN=local go run ./app/cmd/shop-sqlite   # http://localhost:8080。ユーザー alice@example.com / password123 を入れる
GOTOOLCHAIN=local go run ./app/cmd/shop-mem      # 同じサーバーを、メモリの上で
```

## 基底 `go` が従う規則

出発点は今の Go の既定の出力（`docs/emit-go.md`）。ここに書くのは、P1〜P32 を当てはめて変えたところと、決定が決めていなかったのでこのサンプルで決めたところ（後者は「分かったこと」に番号で挙げる）。書いていないことは `docs/emit-go.md` のまま。

- **出すもの**: 単位の対象の名前空間の、型（model、scalar、union、例外）と効果の定数のすべて（P8）、op のすべて（P11）。選択はない。制約を確かめる関数は出さない（P10）。
- **ヘッダ**: 先頭は `// Code generated by hozoc go <版>; DO NOT EDIT.`。
- **ファイル**: `doc.go`、`types.go`、`ops.go`、`states.go`、`wiring.go`。`validate.go` は出さない。`classify.go` は、人が述語を書く分類があるときだけ（この仕様にはない）。
- **パッケージ**: 名前は `docs/emit-go.md`「配置」の規則のまま。名前空間 `type` は Go の予約語なので `pkgtype`（ディレクトリは `gen/type`）。別の名前空間の型は、その名前空間の出力を import する（P7。複製しない）。パッケージ名が import パスの最後の要素と違うときは、import に名前を書く（`pkgtype "example.com/shop-server/gen/type"`）。
- **継承した model**（P24〜P27）: 元の model とは別の構造体。フィールドは、元の宣言順から `never` にしたものを除いた並び。元の model から写す関数は出さない（`gen/type/types.go:38-44`）。
- **実装はインターフェースで受け取る**（P31、P32）: 人やプラグインが実装するものは、どれもインターフェースで、実装は `New` の引数で渡す。実装の漏れ（メソッドの書き忘れ、渡し忘れ）は、コンパイルエラーになる。基底が出すインターフェースは次の四種類（`ops.go` と `states.go`）。
  - **`Ops`**: その名前空間の op のすべて（葉と配線。P11）。宣言順。呼ぶ側が使うもので、`New` が返す（`gen/server/ops.go:12`、`gen/store/ops.go:12`）。
  - **`Leaves`**: **人が実装する葉の op**: 本体がなく、状態を読まず（`reads` がない）、効果も持たない op。宣言順（`gen/server/ops.go:104`）。そういう op がなければ出さない（`store`）。
  - **`<状態>State`**（状態の単位）: 状態ごとに一つ。メソッドは、**形で決まる問い合わせ**、その状態を読む葉の op（**問い合わせ**: `reads` がその状態一つ）、その状態を宛先にする効果を持つ葉の op（**命令**）。この順で、op はそれぞれ宣言順。効果を持つ葉は、`reads` があっても効果の宛先の単位に置く。
    - **形で決まる問い合わせは、どの状態の単位にもいつも出す**。その名前空間の配線が状態を読むかどうかによらない（`docs/emit-go.md`「state」: 配線が読まなくても、形で決まる問い合わせをすべて持つ。テストが効果の結果の判定に使う）。状態を読むので、どれも `ctx` を取り、`error` を返す。並びは `IsEmpty`、`Get`。
      - `list<T>`: `IsEmpty(ctx context.Context) (bool, error)`（要素が一つもなければ真）と、`Get(ctx context.Context) ([]T, error)`（状態の値。すべての要素を、足した順に）。
      - `nullable<T>`: `IsEmpty`（同じ形）と、`Get(ctx context.Context) (*T, error)`（値がなければ `nil`）。
      - `single<T>`: `Get(ctx context.Context) (T, error)`。
      - `Get` は、状態を名前で読んだときの値（P4、P22）。`docs/emit-go.md`「state」の表は `list<T>` に `Get` を持たないので、そこに足した形になる。この仕様の四つの状態はどれも `list`（`gen/store/states.go:13-62`）で、`nullable` と `single` の形は書いていない。
    - 複数の状態を読む葉は、どの状態の単位にも置かず、インターフェース `SharedState` に置く（`New` の引数は `shared`）。この仕様にはない。
  - **`<Ns>Ops`**（P31）: 配線が呼ぶ別の名前空間ごとに一つ（`MailerOps`、`StoreOps`。`gen/server/ops.go:145`、`:154`）。メソッドは、その名前空間の op のうち、**この名前空間の配線が呼ぶものだけ**（`server.StoreOps` は、`store` の九つの op のうち八つ。`addUser` がない）。並びは相手の宣言順。メソッドの形は相手の op の判定に従う（下）ので、相手の出力の `Ops` が、そのまま満たす: `store.New(…)` の戻り値を、`server.New` の引数 `store` に渡せる（型を共有しているとき。N16）。
- **op ごとの関数の型は出さない**: P32 の前は、関数のフィールドを持つ構造体（`Leaves`、`<状態>State`、`Deps` の `<Ns>Ops`）と、そのフィールドの型（`<Op>Func`、`<Ns><Op>Func`）を出していた。構造体がインターフェースになり、関数の型を使う宣言がなくなったので、どちらも出さない。op の説明のコメントは、インターフェースのメソッドに書く。
- **メソッドの形**（P29、P30、P32）: op のメソッドの形は、その op の宣言だけで決まり、どのインターフェースに現れても、どの名前空間から見ても同じ（P30）。この仕様での判定は下の表。
  - **`ctx`**: 最初の引数に `ctx context.Context` を取る。**純粋な op** は取らない。純粋なのは、デコレータがなく、`reads` も `effects` もなく、状態に依らない op（配線なら、呼び出し先もすべて純粋で、状態を参照しない）。
  - **`error`**: 最後の戻り値に `error` を返す。**純粋で、かつ `throws` を宣言していない op は返さない**（失敗しえない。P32）: 戻り値があればその値だけを返し、なければ何も返さない。純粋でも、`throws` があれば返す。純粋でなければ、`throws` がなくても返す（保存先や外部の失敗と、`ctx` のエラーがある）。配線も同じに判定する（呼び出し先が投げる例外は、配線の `throws` にすべて書いてある。`docs/check.md` の THROWS-1）。
  - **コメント**: `Ops`、`Leaves`、`<Ns>Ops` のメソッドには、op の説明と、`Throws:`、`Reads:`（P28 の、伝わってきた `reads`）、`Effects:` を書く。純粋なら `純粋（ctx を取らない）`、`error` も返さなければ、続けて `失敗しない（error を返さない）`。状態の単位のメソッドには、問い合わせか命令かの一行を書く。
- **`New`**（P32）: 実装を**位置引数**で受け取り、`Ops` を返す（構造体のフィールドは書かずに済ませられるが、引数は済ませられない）。並びは次の順。当たるものがなければ、その引数はない。
  1. `leaves Leaves`（人が実装する葉の op があるとき）。
  2. 自分の状態の単位。状態の宣言順で、引数の名前は状態の名前（`users UsersState`）。複数の状態を読む葉があれば、その後ろに `shared SharedState`。
  3. 別の名前空間。**名前空間の名前の辞書順**で、引数の名前は名前空間の名前（`mailer MailerOps`、`store StoreOps`）。配線が呼ぶ順にはよらない。

  この仕様では `server.New(leaves Leaves, mailer MailerOps, store StoreOps) Ops`（`gen/server/ops.go:198`）と、`store.New(users UsersState, sessions SessionsState, orders OrdersState, notifications NotificationsState) Ops`（`gen/store/ops.go:62`）。`New` は、引数が `nil` かどうかを確かめない（N56）。
- **パッケージの変数はない**（P32）: `States` と `StateSet` は出さない。実装があるのは、`New` が返す値（非公開の構造体 `ops`。フィールドは `New` の引数そのまま）の中だけ。同じ出力の `New` は何度呼んでもよく、返る `Ops` は互いに何も共有しない。
- **「実装がない」を実行のときに返す道はない**（P32）: 生成したメソッドは、実装が `nil` かどうかを確かめず、`*runtime.MissingError` を返さない。`ops.go` と `wiring.go` は `runtime` を import しない。基底の出力に残る `runtime` は、`types.go` の `runtime.Effect`（効果の定数の型）と `runtime.Exception`（例外の型がそれを満たすことの確認）の二つ。`runtime.ValidationError` と `runtime.Violation` を使うのは、`go-valid` と `go-http` の出力。
- **葉のメソッドと、配線の本体**: `ops` の葉のメソッドは、`New` に渡された実装の、同じ名前のメソッドを呼ぶだけ（`o.leaves.<Op>(…)`、`o.<状態>.<Op>(ctx, …)`。`gen/server/ops.go:209-234`、`gen/store/ops.go:74-107`）。配線の本体は、自分の名前空間の op を `o.<Op>(…)` で、別の名前空間の op を `o.<名前空間>.<Op>(ctx, …)` で呼ぶ。`error` を返さない op の呼び出しには、`err` を確かめる行がない（`session := o.NewSession(user)`。`gen/server/wiring.go:38`）。`error` を返す配線が、返さない op の結果をそのまま返すときは `return o.<Op>(…), nil`（`:96`、`:115`）。
- **配線の `match`**（P2、P3、P20）: 分類のどの場合も型から決まる（下の「分類」）なら、判定関数を呼ばず、`nil` かどうかで分ける。絞り込む場合のアームを `if x != nil { … }` の枝にし、もう一方のアームをその後ろに続ける（アームを書いた順によらない）。枝の中で絞り込んだ名前を値として使う位置は `*x`。`return match` は、アームごとの `return`。それぞれの枝の頭に、場合の名前をコメントで書く。
- **アーム**: `throw E`（P13）は、例外を**値で**返す（`return <零値>, pkgtype.Unauthorized{}`）。呼び出しのアームは、その呼び出しを返す（`return o.RequireFresh(*session)`。`gen/server/wiring.go:58`）。値のアーム（P14）は、その値を返す（`return *session, nil`）が、この仕様にはもうない（「下書きから変えたこと」5）。
- **分類**（P2、P3）: 場合が「対象の `T?` を `T` に絞り込む場合一つ」と `default` だけの分類は、どの場合に当たるかが値から決まる。基底は、そういう分類について何も出さない（列挙の型も、判定関数も、`New` の引数も）。分類の名前は、それを使う配線のコメントにだけ残る。この仕様の三つの分類はどれもこの形。

| op（`server`） | 種類 | 純粋か | `throws` | メソッドの形 |
|---|---|---|---|---|
| `login`、`logout`、`getMe`、`getOrders`、`placeOrder` | 配線 | いいえ（デコレータ `@http.*` があり、純粋でない op も呼ぶ） | ある | `ctx` を取り、`error` を返す |
| `issueSession`、`authenticate`、`notify` | 配線 | いいえ（純粋でない op を呼ぶ: 順に `store.addSession`、`store.findSession`、`mailer.send` と `store.addNotification`） | ある | `ctx` を取り、`error` を返す |
| `checkPassword`、`requireFresh` | 葉 | はい（デコレータも `reads` も効果もない） | ある | `ctx` を取らず、`error` を返す |
| `newSession`、`toPublicUser`、`pageOf`、`newOrder`、`composeNotice` | 葉 | はい（同じ） | ない | `ctx` を取らず、`error` も返さない（戻り値だけ） |

`store` の九つの op は、どれも `reads` か効果を持つ葉なので純粋でなく、`ctx` を取り、`error` を返す（`throws` は、どれにもない）。`mailer.send` は、デコレータ `@remote` を持つ葉なので純粋でない（`mailer.hozo:10-12`）。`server.MailerOps` と `server.StoreOps` のメソッド（`mailer.send` と、`store` の八つ）は、相手がどれも純粋でないので、`ctx` を取り、`error` を返す。`mailer` に Go の単位を置けば、そこに出る `Ops` の `Send` も同じ判定で同じ形になり、その `Ops` は `server.MailerOps` をそのまま満たす。

## `go-valid` が従う規則

出すファイルは `validate.go` 一つ（基底と同じパッケージ）。

- **ヘッダ**: 先頭は `// Code generated by hozoc go-valid <版>; DO NOT EDIT.`。
- **`Validate` を持つ型**: その単位の名前空間の scalar と model のうち、`@valid.*` が付いているか、付いたフィールドを持つか、`Validate` を持つ型を（配列、省略可能を通して）フィールドに持つもの。それ以外の型には出さない。継承した model は、受け継いだフィールドの制約も確かめる。
  - `type`: `Email`、`Password`、`User`、`PublicUser`、`OrderLine`、`Order`。
  - `server`: `Credentials`、`LoginInput`、`LoginOk`、`PlaceOrderInput`（`AuthInput` と `GetOrdersInput` にはない）。
- **形**: `func (v T) Validate() error`。違反は `*runtime.ValidationError`（`Violation{Path, Constraint}`）で、`Path` と制約の実装は `docs/emit-go.md`「scalar」「model の Validate」のまま。
- **別の名前空間の型のフィールド**: その型のメソッド `Validate` を呼ぶ（`gen/server/validate.go:15` の `m.Email.Validate()`）。メソッドは型を持つパッケージにしか書けないので、**その型の名前空間を出す Go の単位も `go-valid` を並べていなければならない**（この設定では `gen/type`）。並べていなければエラーにする。
- 非公開の名前には `valid` を前に付ける（`validAppend`）。パッケージごとに一つずつ出す。

## `go-sqlite` が従う規則

出すファイルは `db.go` 一つ（基底と同じパッケージ）。import するのは `database/sql` と標準ライブラリ、型のパッケージだけで、ドライバは import しない。

- **ヘッダ**: 先頭は `// Code generated by hozoc go-sqlite <版>; DO NOT EDIT.`。並びはどれも宣言順。
- **表**: `@db` の状態ごとに表一つ。名前は状態の名前。要素の model のフィールドごとに列一つ。名前はフィールドの名前。名前はいつも `"…"` で引用する。
- **列の型**: string を元にした型は `TEXT`、`int` は `INTEGER`、`boolean` は `INTEGER`、`float` は `REAL`、`Time` は `TEXT`（RFC 3339、UTC、小数部はいつも 9 桁）、配列と model は JSON を入れた `TEXT`。省略可能でない列は `NOT NULL`。
- **制約**: `@db.key` は `PRIMARY KEY`、`@db.unique` は `UNIQUE`。`@db.ref(M)` は、要素が `M` であるただ一つの `@db` の状態の表の、`@db.key` の列への `REFERENCES`（そういう状態がちょうど一つでなければ、または `M` が `@db.key` を持たなければ、制約なし）。
- **`const Schema`**: 表ごとの `CREATE TABLE IF NOT EXISTS` を宣言順に並べたもの。
- **`func CreateTables(ctx context.Context, db *sql.DB) error`**: `Schema` を、受け取った `ctx` で実行する（`gen/store/db.go:40-42`）。するのは表を作ることだけ。
- **状態ごとの型**（P32）: `@db` の状態ごとに、公開の型 `<状態>Table` と、それを作る `func New<状態>Table(db *sql.DB) <状態>Table` を出す（`UsersTable`、`NewUsersTable`。`gen/store/db.go:47-50`）。型は `*sql.DB` を一つ持つだけの値で、同じ `db` から作った値は、同じ表を読み書きする。メソッドは、基底のその状態の単位（`<状態>State`）のメソッドのうち、**中身を機械的に決められるもの**: 下の「形で決まる問い合わせの中身」（いつも）と、「op の中身」の規則に当たる op。名前と形は、基底のメソッドのまま。受け取った `ctx` で `QueryContext` / `QueryRowContext` / `ExecContext` を呼ぶ。
- **規則に当たらない op があるとき**: その状態の型は、その op のメソッドを持たない。型は `<状態>State` を満たさず、基底の `New` に渡す行がコンパイルできない。人は、**プラグインの型を自分の型に埋め込み、足りないメソッドを書き足して**、その型を `New` に渡す（この仕様では起きないので、見立ての型で形にした: `app/onsqlite/store_test.go` の `partialUsers` と `humanUsers`。`app/onsqlite/store_test.go:85-112`）。基底のメソッドをすべて持つ型には、それを確かめる `var _ <状態>State = <状態>Table{}` を出す（`gen/store/db.go:52`）。
- **形で決まる問い合わせの中身**: 基底がその状態の単位にいつも出す問い合わせ（「基底 `go` が従う規則」の「状態の単位」）を、型のメソッド `IsEmpty`、`Get` として出す。`list` の状態は、`IsEmpty` が「`SELECT COUNT(*) FROM <表>` の結果が 0」、`Get` が `SELECT <すべての列> FROM <表> ORDER BY rowid`（足した順）。`nullable` と `single` の状態は、この仕様にないので決めていない。
- **op の中身**: `@db` の状態 S（要素の model は M）を読み書きする葉の op のうち、次のどれかに当たるものの中身を、S の型のメソッドとして出す。
  - **引数と列の対応**（下の三つが使う）: 引数の型（省略可能なら、その中の型）と同じ型のフィールドが、M に**ちょうど一つ**あれば、その列。なければ、または二つ以上あれば、対応しない。
  - **一行を引く**: `reads S` だけを持ち、戻り値が `M?`。引数は一つ以上で、どれも列に対応し、対応した列のどれかが `@db.key` か `@db.unique` → `SELECT … WHERE <列> = ? [AND …]`。行がなければ `nil`。
  - **並びを引く**: `reads S` だけを持ち、戻り値が `M[]`。引数（なくてもよい）はどれも列に対応する → `SELECT … [WHERE …] ORDER BY rowid`（足した順）。
  - **足す**: 効果が一つで、その宛先が `S.post`。引数は M の一つだけで、戻り値がない → `INSERT`。
  - **消す**: 効果が一つで、その宛先が `S.delete`。引数は一つ以上で、どれも列に対応し、戻り値がない → `DELETE … WHERE <列> = ? [AND …]`。行がなければ何もしない。
  - **省略可能な引数**: 値がなければ、問い合わせない。引く op は「ない」（`nil`、空の並び）を返し、消す op は何もしない。
- 公開の名前は、`Schema`、`CreateTables`、状態ごとの `<状態>Table` と `New<状態>Table`。非公開の名前には `db` を前に付ける（`dbGet`、`dbScanUser` など）。

この仕様の九つの op は、どれも規則に当たる: `findUser`、`findUserById`、`findSession` は「一行を引く」、`listOrders` は「並びを引く」、`addUser`、`addSession`、`addOrder`、`addNotification` は「足す」、`removeSession` は「消す」。形で決まる問い合わせは、四つの状態に `IsEmpty` と `Get` で八つ。四つの型（`UsersTable`、`SessionsTable`、`OrdersTable`、`NotificationsTable`）は、合わせて十七のメソッドを持ち、どれも基底の状態の単位を満たす（`gen/store/db.go:47-197`）。組み立ては、四つの型を `store.New` に渡す一行（`app/onsqlite/compose.go:22`）。

## `go-http` が従う規則

出すファイルは `http.go` 一つ（基底と同じパッケージ）。標準ライブラリだけを使う（Go 1.22 以降の `ServeMux` のパターン）。

- **ヘッダ**: 先頭は `// Code generated by hozoc go-http <版>; DO NOT EDIT.`。並びはどれも宣言順。
- **入口**: `func NewHandler(ops Ops) http.Handler`（基底の `New` の戻り値を、そのまま渡す）。`@http.get` / `@http.post` の付いた op ごとに経路を一つ、`"<メソッド> <パス>"` のパターンで `http.NewServeMux` に足す。op には `r.Context()` を渡す。
- **入力**: op の引数の model を、フィールドのデコレータで読む。初めの値は、既定値を持つ model なら基底の `New<Model>()`、そうでなければ零値。
  - `@http.header(name)`: 省略可能な、string を元にした scalar。ヘッダがあって空でなければ、その値をそのまま入れる（なければ `nil`）。
  - `@http.query`: クエリにその名前があれば、フィールドの型で読む（`int` は 10 進の 64 ビット）。なければ初めの値（既定値）のまま。
  - `@http.body`: そのフィールドが一つだけなら、本文の JSON がそのフィールドの値。二つ以上なら、本文はフィールド名をキーにした JSON のオブジェクト（このサンプルでは使っていない）。
- **制約の検査**（単位のオプション `validate = true` のとき。P15、P16）: 入力を読んだ後、op を呼ぶ前に、入力の model の `Validate`（`go-valid` の出力）を呼ぶ。入力の model が `Validate` を持たなければ（`go-valid` の規則で決まる）、呼ばない。
- **op を呼ぶ前の失敗**は 400 で、`http` の語彙の例外 `http.BadRequest` として返す。本文は例外の形（下の「エラー」）で、運ぶ値は `{"message": string, "violations": [{"path": string, "constraint": string}]}`。op は呼ばない。
  - **読めない要求**（本文が JSON として読めない・型が違う、クエリが数として読めない）: `message` は `<場所>: <理由>` の決まった文言（`body: invalid JSON`、`query page: invalid int`）、`violations` は空の配列。
  - **制約に合わない入力**: `message` は `invalid input`、`violations` は違反の一覧。`path` は**通信の上の場所**: `Validate` の `Path` の先頭（入力の model のフィールド名）を、そのフィールドを読む場所（`body`、`query.<名前>`、`header.<名前>`）に置き換えたもの（`body.email`、`body[1].quantity`）。`constraint` は `valid` の語彙のデコレータの名前。
- **成功**: 200 で、結果の JSON。結果が配列で `nil` なら `[]`。戻り値のない op は 204 で本文なし。
- **エラー**: `@http.status(n)` を持つ例外は、ステータス n で `{"error":"<例外の FQN>"}`、値を運ぶ例外は `"value"` にその JSON を足す。ステータスを持たない例外と、それ以外のエラー（保存先のエラー、`ctx` のエラーを含む）は 500 で `{"error":"internal"}`（中身を出さない）。見分ける例外は、`@http.get` / `@http.post` の付いた op が投げうる例外（`Signature.exceptions`）のうち `@http.status` を持つもので、値の型で見分ける（基底の `throw` も、人が書く葉も、`pkgtype.Unauthorized{}` のように値で返す）。
- **本文のある応答**は `Content-Type: application/json`。
- 非公開の名前には `http` を前に付ける（`httpWriteJSON` など）。

v1 の規則から変えたのは三つ: `r.Context()` を渡す（P29）、制約の検査（P15、P16）、op を呼ぶ前の失敗の本文（v1 は `{"error":"http.BadRequest","message":"…"}`。「分かったこと」N44）。

## 人が書いたもの

- **`app/leaves.go`**: `server` の葉の op 七つの本体。型 `app.Leaves` のメソッドで、`*app.Leaves` が生成された `server.Leaves` を満たす（`app/leaves.go:36`。メソッドが足りなければ、ここでコンパイルできない）。`server.New` の引数 `leaves` に、そのまま渡す。時計、トークンと注文の識別子の作り方、セッションの有効期間、1 ページの件数は `app.Leaves` のフィールドで、テストが差し替える。**どのメソッドも `ctx` を取らず、`throws` のない五つは `error` も返さない**: `server.Leaves` のメソッドの形（`gen/server/ops.go:104-140`）がそう決めていて、違う形のメソッドでは満たせない（コンパイルのときの事実。`app/server_test.go:24-40` が、メソッド式の代入で確かめる）。`RequireFresh` は、今が期限より前ならセッションをそのまま返し、期限ちょうどから先は `type.Unauthorized`（`app/leaves.go:62-66`）。
- **`app/mailer.go`**: `mailer.send` の実装。`*app.Mailer` が `server.MailerOps` を満たし、`server.New` の引数 `mailer` に、そのまま渡す。`ctx` を取る（`mailer.send` は `@remote` を持ち、純粋でない）。届けられなければ `type.NotificationFailed`。届け方（`Deliver`）はテストが差し替える。
- `app` パッケージは保存先を知らない: import するのは `gen/server` と `gen/type` だけ（`app/leaves_test.go` の `TestNoStorageImport`）。
- **`app/memstore/memstore.go`**: `store` の四つの状態の単位の、メモリの上の実装。`memstore.New()` が空の `Store` を返し、`Users()`、`Sessions()`、`Orders()`、`Notifications()` が、その上の状態ごとの型を返す（`app/memstore/memstore.go:42-64`）。四つの型は、`gen/storemem` の `UsersState` などを満たす（`app/memstore/memstore.go:35-38`）。メソッドは合わせて十七（形で決まる問い合わせ `IsEmpty`、`Get` が八つ、op が九つ）で、どれも、はじめに `ctx.Err()` を見る。`Store` はいくつ作ってもよく、互いに混ざらない。テストのためだけの読む口は持たない。
- **二つの組み立て**: `app/onsqlite/compose.go`（表を作る `store.CreateTables(ctx, db)` と、`go-sqlite` の四つの型を渡す `store.New`。`app/onsqlite/compose.go:17-24`）と `app/onmem/compose.go`（`memstore` の四つの型を渡す `store.New`。`app/onmem/compose.go:16-20`）。どちらも、`store.New(…)` の戻り値を、そのまま `server.New(leaves, mailer, s)` の三つ目に渡し、その戻り値を `server.NewHandler` に渡す。**二つが使う `gen/server` と `app` の葉は同じ一つのもの**で、違うのは `store` の import と、状態の単位の実装だけ。組み立ては何度呼んでもよい（パッケージの変数がない）。
- **`app/handlertest`**: HTTP から見た振る舞い。二つの組み立て（`app/onsqlite/compose_test.go`、`app/onmem/compose_test.go` の `TestHandler`）が同じものを走らせる。サブテストは、それぞれ新しいサーバーを作り、並列に走る（`t.Parallel()`）。応答の本文を文字列のまま比べるので、そこに書いた JSON が、クライアントが合わせる形になる。確かめること: パスワード違いとユーザーなしは 401 `type.InvalidCredentials`、制約に合わない入力は 400 `http.BadRequest` と違反の一覧、ログインでトークン、`GET /me`（トークンあり・なし・知らないトークン）、`POST /orders` で注文と通知が一つずつ（通知は、状態 `store.notifications` の `Get` で数える。`app/handlertest/handlertest.go:359-374`）、`mailer.send` を失敗させると 500 で**注文は残る**、`GET /orders?page=…`、読めない `page` は 400、ログアウトの後の `GET /me` は 401、中断された要求は 500 で状態が変わらない、**期限の切れたセッションは、どの入口でも 401 `type.Unauthorized`**（時計を進めて確かめる。`app/handlertest/handlertest.go:251-270`）。
  - `handlertest.RunTwo`（二つの組み立ての `TestTwoServers`）: **同じ出力の上の二つのサーバーが、一つのプロセスで同時に動き、混ざらない**。SQLite ならデータベースが二つ、メモリなら `memstore.Store` が二つで、`store.New` と `server.New` を二度呼ぶ。二つに並行に要求を送り、注文と通知の数、ログアウトが、もう一方に及ばないことを確かめる（`app/handlertest/handlertest.go:277-302`）。
- **`app/storetest`**: `store` の op と、状態の形で決まる問い合わせの振る舞い。二つの実装（`app/onsqlite/store_test.go`、`app/onmem/compose_test.go` の `TestStore`）が同じものを走らせる。`server` は組み立てない。サブテストは並列に走る。効果の結果は、`store.New` に渡したのと同じ、状態の単位の実装の `Get` と `IsEmpty` で確かめる（読む op のない `notifications` も読める。どちらの実装の型も、`storetest.List[T]` をそのまま満たす）。中断された `ctx` で呼ばれた op と問い合わせが `context.Canceled` を返し、四つの状態が変わらないことも確かめる。
- **`app/onsqlite/store_test.go`**: `go-sqlite` だけの事実。`CreateTables` は中断された `ctx` では表を作らず、二度呼んでも足した行が残る。プラグインの型に足りないメソッドを、人が埋め込みで書き足せる（`TestHumanCompletesPluginType`。足りないままの型が `store.UsersState` を満たさないことは、リフレクションで確かめる）。`Time` と配列の列の形、`REFERENCES`。
- **`app/server_test.go`**: `server` だけ（`store` のどの出力も使わず、`server.StoreOps` を満たす型をテストが書く）。メソッドの形（`ctx` と `error` の有無）、要求の `context` が `server.StoreOps` のメソッドにそのまま届く、`go-http` が読めない要求と制約に合わない入力では op を呼ばない、葉の op を一つだけ差し替える（`TestReplaceOneLeaf`）、`New` は `nil` を確かめない（`TestNilIsNotChecked`）。
- **`app/both_test.go`**: 一つのプロセスで、同じ `server` を二つの保存先の上に組み立てられ、二つが混ざらない。`store` の二つの出力の `Ops` が、どちらも `server.StoreOps` をそのまま満たす（`app/both_test.go:31-32`）。一つの `store` の状態の単位を、状態ごとに別の実装（SQLite とメモリ）から取れる（`TestStatesFromTwoImplementations`）。`gen/store` と `gen/storemem` の基底のファイルが、バイト単位で同じ。

## 分かったこと

### v1 の「分かったこと」の、その後

v1 の 63 項目（`git show ac57788:tests/samples/shop-server/README.md` の「分かったこと」。番号はそのまま）が、P1〜P32 を当てはめてどうなったか。「解決」は、その項目が言っていた不足がこの形ではなくなったもの。「残る」は、形が変わっても同じ不足があるもの。「当たらない」は、v1 の組み方にだけあった事情。N で始まる番号は「v2 を書いて分かったこと」の項目。

| # | v1 の項目 | その後 | 理由 |
|---|---|---|---|
| 1 | トランスパイラのファイルが参照するのは共有する形だけ | 解決 | P9、P17: 参照してよいのは、基底の形と、依存を宣言したプラグインの出力。ただし N31 |
| 2 | Go の名前は `View` にない | 残る | 相乗りのプラグインは、基底の名前の規則を同じに当てはめる前提のまま |
| 3 | 既定値は基底の `New<Model>()` を呼ぶ | 解決 | P9: 基底の形を前提にしてよい、と決まった |
| 4 | `New` の引数の並びが、選択を合わせた後に決まる | 解決 | P9、P11: 仕様と設定だけで決まる |
| 5 | 命令の口があるかが、ほかのトランスパイラの選択で決まる | 解決 | P11、P19: op はすべて出る。`go-sqlite` が、その実装を出せる |
| 6 | 別の名前空間の型を import するか複製するかが、`View` から決まらない | 解決 | P7、P8 |
| 7 | ヘッダの版が、hozoc のものかトランスパイラのものか | 残る | |
| 8 | `httponly` に `users` の単位がない | 解決 | P11、P19: 同じ名前空間の二つの単位（`gen/store`、`gen/storemem`）の基底のファイルは、バイト単位で同じ |
| 9 | 保存先だけの単位とハンドラだけの単位を、別の `out` に出して組めない | 解決 | P5〜P7。ただし N16 |
| 10 | 誰も呼ばない `IsEmpty` が出る | 残る | 形で決まる問い合わせは、配線が読まなくてもいつも出す（N22）。生成したコードは呼ばない。呼ぶのはテスト |
| 11 | 記録を代役の emit から作れない | 当たらない | 代役を使う `shared` が保留 |
| 12 | キャンセルと期限が伝わらない | 解決 | P29 |
| 13 | 認証の要る葉が、それぞれ呼んだ人を引く | 解決 | P1〜P3、P13: 配線 `authenticate` が一度引き、セッションを引数で渡す |
| 14 | トランザクションの口がない | 残る | N37 |
| 15 | 葉をまたぐ原子性は書けない | 残る | N37 |
| 16 | パッケージの変数（`States`）を守るものがない | 解決 | P32: パッケージの変数がなくなった。状態の実装は `New` の引数 |
| 17 | テストを並列にできない | 解決 | P32: サーバーを作り直すサブテストを `t.Parallel()` にした（`app/handlertest/handlertest.go:123-124`、`app/storetest/storetest.go:76-77`）。N53 |
| 18 | 一つのプロセスに二つ持てない | 解決 | P32: 同じ出力の上にも、いくつでも持てる（二つの組み立ての `TestTwoServers`）。N18 |
| 19 | 実体の持ち方が半分ずつ（`New` の引数と `States`） | 解決 | P32: すべて `New` の引数（N17） |
| 20 | 入れ忘れが起動のときに分からない | 解決 | P31、P32: 入れ忘れは、起動より前の、コンパイルのときに分かる。`nil` と書いて渡すことは防げない（N56） |
| 21 | 500 の原因がどこにも出ない | 残る | |
| 22 | ユーザーで絞ってページで切る問い合わせを引けない | 残る | 絞る方は解決（P19 と `go-sqlite` の規則）。ページと順序は N37 |
| 23 | 期限切れのセッションを消せない | 残る | 条件を付けた削除がない。期限を確かめる方は、葉 `requireFresh` を足して直した（N3）が、切れたセッションの行は残り続ける |
| 24 | 一意の違反を見分けられない | 残る | N37 |
| 25 | 人が書く葉が、保存先のトランスパイラの API に結びつく | 解決 | P1、P5、P19 |
| 26 | ユーザーを足す op が仕様にない | 解決 | 仕様に `store.addUser` を足した（「下書きから変えたこと」3）。P の決定ではない |
| 27 | 成功の応答の形 | 残る | このサンプルで決めたまま |
| 28 | 空の配列（入れ子の `nil` は `null`） | 残る | `app/leaves.go:82` で人が空の配列を作る |
| 29 | 例外の応答の形 | 残る | このサンプルで決めたまま。N48 |
| 30 | `http.BadRequest` が語彙にない | 残る | N44 |
| 31 | 500 の `internal` は FQN ではない | 残る | |
| 32 | 404 と 405 が JSON でない | 残る | |
| 33 | `Time` のゾーンを人がそろえる | 残る | `app/leaves.go:51,83` |
| 34 | `int` が JS の `number` に収まらない | 残る | |
| 35 | 省略可能の JSON の上の形 | 残る | |
| 36 | ユニオンの判別の書き方 | 残る | |
| 37 | 本文の束ね方 | 残る | |
| 38 | ヘッダの値をそのまま scalar にする | 残る | |
| 39 | 違反の位置に、通信に現れない名前が入る | 解決 | P15 と、このサンプルで決めた規則（N46） |
| 40 | 入力の model の `json` タグは通信の形ではない | 残る | |
| 41 | 読み方の甘さ（知らないキー、ないキー、本文の大きさ） | 残る | |
| 42 | `Validate` を呼ぶのは誰か | 解決 | P15、P16 |
| 43 | 例外を値の型で見分ける（ポインタで返せば 500） | 残る | 基底の `throw` は値で返す（N25）。人が書く葉は、どちらでも書ける |
| 44 | 例外をステータスに写す表が、名前空間に一つ | 残る | |
| 45 | ステータスを持たない例外が、不具合と区別できない | 残る | `app/handlertest/handlertest.go:181-192` |
| 46 | 公開の名前の衝突 | 残る | プラグインが増えた分、増えた（N49） |
| 47 | 非公開の名前の前置きが、このサンプルだけの約束 | 残る | `valid` が増えた |
| 48 | 識別子の衝突を見つける者がいない | 残る | |
| 49 | ファイル名の約束がない | 残る | `validate.go` は基底のものから `go-valid` のものになった |
| 50 | 利用者の名前から局所の名前を作らない | 残る | N43 |
| 51 | SQL の表の名前が状態の名前だけ | 残る | |
| 52 | v1 の仕様で変えたこと | 当たらない | v1 の仕様についての記録。v2 の分は「下書きから変えたこと」 |
| 53 | 配列の要素数を書けない | 解決 | P22: 配列の値を `by list` で分けられる。この仕様では使っていない |
| 54 | 成功のステータスを書けない | 残る | |
| 55 | 応答のヘッダを書けない | 残る | |
| 56 | `@db.ref` が指す列 | 残る | このサンプルで決めたまま |
| 57 | 外部キーは既定では効かない | 残る | `app/onsqlite/compose_test.go:17-18` |
| 58 | メモリの上の SQLite は接続ごとに別 | 残る | `app/onsqlite/compose_test.go:22-23` |
| 59 | 表の作り直し（移行）の形がない | 残る | |
| 60 | `db.Exec(Schema)` が複数の文を一度に実行する | 残る | 今は `db.ExecContext(ctx, Schema)`（`gen/store/db.go:41`） |
| 61 | 表は `valid` の制約を確かめない | 残る | N35 |
| 62 | `@db` は、どの保存先かを言わない | 残る | |
| 63 | `go-sqlite` の細部で決めたもの | 残る | 表の型（`Update`、`Delete`）はなくなった。残りはそのまま |

解決 19、残る 42、当たらない 2（P31、P32 で解決に変わったのは、16〜20 の五つ）。

### v2 を書いて分かったこと

決定が決めていなかったので書きながら決めたこと、二つの決定が引っ張り合ったところ、理想のコードに決定が与えてくれなかったもの、v1 より悪くなったところ。それぞれ「何が起きたか → サンプルではどうしたか」。パスはこのディレクトリから。

N1〜N53 は、P1〜P30 を当てはめた形を書いたときのもの。その後の決定（直した P30、P31、P32）と仕様の直しで片づいたものは、番号を残して、見出しに「解決」と、何で解決したかを書く。N54〜N65 は、P31 と P32 を当てはめた形（実装をインターフェースで受け取る）を書いて分かったこと。

N1〜N65 のうち、解決は 10: N3（仕様の直し）、N11（直した P30）、N22（先にあった決定に合わせた）、N27 と N28（P31）、N10、N14、N17、N18、N53（P32）。

#### 仕様

- **N1 `Found => toPublicUser(user)` は、先に束縛できない**。`user` が `type.User` になるのはアームの中だけ（P3）なので、写す呼び出しはアームに書くしかない（`server.hozo:107`）。レコードのフィールドに渡す方（`issueSession`）は、先に束縛した（`server.hozo:59-60`）。アームに書けるのが呼び出し一つまでなら、「絞り込んだ値を写してから、もう一つ呼ぶ」は、そのための配線の op を別に作ることになる。
- **N2 写す op を人が書く**（P27）と、**v1 より悪くなる**。v1 は基底が `UserToPublicUser` を出していた。v2 は三つのフィールドを人が写す（`app/leaves.go:56-57`）。`type.User` にフィールドを足すと、`type.PublicUser` は継承で同じフィールドを得るが、人が書いた `toPublicUser` は写さないままコンパイルが通り、零値が外へ出る。気づく仕組みはない。
- **N3（解決: 仕様に、人が書く葉 `requireFresh` を足した）セッションの期限を、誰も確かめなかった**。下書きの配線 `authenticate` は「セッションがあるか」だけで分けていた（`store.hozo:48-51`）。`newSession` は `expiresAt` を入れる（`app/leaves.go:51`）が、読む者がいなかったので、**期限切れのトークンで入れた**。v1 は葉が時計と比べていた（期限切れは 401）。仕様に書く方法として考えたのは、時計に依る述語を持つ分類（「`as` で絞り込む場合に、人が書く述語を足せるか」が決まっていない。P3）と、比較を持つ読む op（`go-sqlite` の規則で表せない。N37）。→ どちらでもなく、`server` に葉 `op requireFresh(session: type.Session): type.Session throws type.Unauthorized;` を足し、`authenticate` の `LoggedIn` のアームから呼ぶ（`server.hozo:76-88`）。本体は人が書き、時計と比べる（`app/leaves.go:62-66`）。期限ちょうどから先は、どの入口でも 401（`app/handlertest` の `expired`）。残ること:
  - **書き方は「受け取った値を、そのまま返すか、投げる葉」**。アームに書けるのは呼び出し一つ（N1）なので、確かめるだけの op も、値を返す形にした。「期限が過ぎていない」は、説明のコメントと人の本体にしかない。
  - **`requireFresh` は時計を使うが、仕様の上では純粋**（N9）で、メソッドは `ctx` を取らない。
  - **切れたセッションは消えない**（v1 の 23）。`logout` も `authenticate` を通るので、切れたトークンでは 401 になり、その行を消せない。
  - **値のアーム（P14）を使うところが、この仕様からなくなった**。`LoggedIn => session;` が、ただ一つの値のアームだった。基底の規則には形を残した（「アーム」）が、期待する出力には現れない。
- **N4 三つの分類が同じ意味**。`Registration`、`Account`、`Login` は、どれも「`T?` が `T` である場合と、それ以外」（`store.hozo:36-51`）。P20 の `by nullable` と変わらず、違うのは場合の名前だけ（P21 の指針どおり）。`match user by store.Registration` の `user` が `findUser` の結果でなければならないのか（P2 の「その op が返した値の分類」）は決まっていない。基底の出力は、どちらでも同じ（N23）。
- **N5 `findSession(token: type.Token?)` は下書きのまま**（`store.hozo:28`）。受ける側を `T?` にしたので、`go-sqlite` に「省略可能な引数」の規則が要った（N38）。P20 で `in.token` を先に絞り込めば、`findSession(token: type.Token)` にでき、その規則は要らない。
- **N6 継承した model は、語彙のデコレータも受け継ぐ**。`type.PublicUser` のフィールドは `@db.key`、`@db.unique`、`@valid.length` を持ったまま（`type.hozo:41-43`）。外すことはできない（P25 は足すことだけ）。`go-valid` は `PublicUser` にも同じ検査を出す（`gen/type/validate.go:50-61`）。
- **N7 `logout` は、トークンがなければ 401**。下書きの配線が `authenticate` を通るため（`server.hozo:92-96`）。v1 は 204 だった。決定の帰結ではなく、下書きの仕様の変更（`app/handlertest/handlertest.go:229-231`）。

#### 純粋さと `ctx`（P29、P30）

- **N8 この仕様では「純粋な op」と「`server` で人が書く op」が同じ集合になった**。七つの葉がすべて純粋で、配線はどれも、純粋でない op（`store` の op か `mailer.send`）を呼ぶので純粋でない（「基底 `go` が従う規則」の表）。メソッドの形で「保存先に触れない」が分かる、という狙いどおり。
- **N9 純粋と判定された op が、時計と乱数を使う**。`newSession`、`requireFresh`、`newOrder` は、仕様の上ではデコレータも `reads` も効果もないので純粋で、メソッドは `ctx` を取らない（`gen/server/ops.go:114,125,135`）。本体は、今の時刻と新しい識別子を外から得る（`app/leaves.go:49,51,63,80,83`）。P30 の「関数の形を見れば、その op が状態や外部に触れるかが分かる」は、ここでは成り立たない。仕様に書く方法は、その op にデコレータを付けること（どのトランスパイラも読まない印でよい。`mailer.send` の `@remote` と同じ）で、付けなくても検査は通る。
- **N10（解決: P32）純粋な op も `error` を返していた**。`toPublicUser` の理想の形は `func(pkgtype.User) pkgtype.PublicUser` だが、P32 の前は、口が埋まっていないことを `*runtime.MissingError` で返すために `error` が要り、配線は、決して失敗しない写しの後でも `err` を確かめていた。P29、P30 は `ctx` だけを決めていて、`error` には触れていなかった。→ P32: 純粋で `throws` のない op は `error` を返さない。五つの葉（`newSession`、`toPublicUser`、`pageOf`、`newOrder`、`composeNotice`）は値だけを返し（`gen/server/ops.go:104-140`）、配線は `publicUser := o.ToPublicUser(user)` と書く（`gen/server/wiring.go:42`）。「口が埋まっていない」は、実行のときには起きなくなった。この形で分かったことは N60。
- **N11（解決: 直した P30）外部の入出力を「純粋でない」と書く方法が、置き場しかなかった**。直す前のサンプルは「別の名前空間の op の口は、相手が純粋かどうかによらず `ctx` を取る」と決めていた。`mailer.send` は、宣言だけを見ればデコレータも `reads` も効果もない純粋な葉で、`ctx` を取るのは `server` から見た口だけだった（`mailer` に Go の単位を置けば、`ctx` を取らない `Send(notice)` が出て、口に入らなくなる形）。直した P30 は、純粋かどうかを op そのものの性質とし（どの名前空間から見ても同じ。口も相手の op の判定に従う）、別のプロセスや外部にある op には受け渡しのデコレータが付く、とした。→ `mailer` に印のデコレータ `remote` を宣言し、`send` に付けた（`mailer.hozo:7`、`:10`）。`mailer.send` は純粋でなくなり、`server.MailerOps` の `Send` は `ctx` を取る（`gen/server/ops.go:149`）。理由が「別の名前空間だから」から「相手が純粋でないから」に変わった。`mailer` に Go の単位を置いても、同じ判定で `ctx` を取る `Send(ctx, notice)` が出る。残ること: この仕様にはメールを届ける語彙がないので、`remote` は、どのトランスパイラも読まない印を自分で宣言したもの。付け忘れても検査は通り、op は純粋と判定されて `ctx` が消える（N9 と同じ）。`throws` があるので `error` は残る（P32）。生成したメソッドのコメントに、純粋でない理由（どのデコレータか）は出ない（`gen/server/ops.go:146-149`）。
- **N12 宣言を一つ変えると、メソッドの形が変わる**。純粋な葉に、デコレータ（どのトランスパイラも読まない印でも）、`reads`、効果のどれかを足すと、その op のメソッド（`Ops` と、`Leaves` か状態の単位）に `ctx` が増える（`throws` がなかった op なら、`error` も増える）。配線なら、呼び出し先の一つが純粋でなくなるだけで同じことが起きる（伝わる）。人が書いた本体と、`Ops` を呼ぶ人のコードは、コンパイルが通らなくなる（気づける）。別の名前空間のインターフェース（`<Ns>Ops`）のメソッドも、相手の op の判定に従うので同じように変わるが、相手の出力の `Ops` をそのまま渡している組み立ては、両方が同時に変わるので、そのまま通る（P31）。通らなくなるのは、人が書いた実装（`app.Mailer`、`app/memstore`）。`throws` を足したときのことは N60。
- **N13 純粋さの判定を使うのは、基底だけだった**。`go-http` が呼ぶ op は `@http.*` を持ち、`go-sqlite` が中身を出す op は `reads` か効果を持ち、形で決まる問い合わせは状態を読むので、どれも必ず `ctx` を取り、`error` を返す。相乗りのプラグインは、判定の結果を知らなくても正しい呼び出しとメソッドを書ける（`gen/server/http.go:39,54,68,91,117`、`gen/store/db.go:55-191`）。
- **N14（解決: P32 の後の `go-sqlite` の規則）`ctx` を受けない入口があった**。P32 の前の `SetupDB(db *sql.DB)` は `ctx` を取らず、表を作る文は中断できなかった。P29 が決めているのは op の関数だけ。→ `SetupDB` は、表を作ることと、実装を `States` に入れることの二つをしていた。後ろの方がなくなった（実装は、人が `New` に渡す）ので、残った方を `CreateTables(ctx context.Context, db *sql.DB) error` にした（`gen/store/db.go:40`。`app/onsqlite/store_test.go` の `TestCreateTables`）。
- **N15 基底が使う名前が増えた**。引数名 `ctx`、パッケージ名 `context` と `pkgtype`。仕様の op の引数が `ctx` という名前なら、末尾に `_` を付ける規則（`docs/emit-go.md`「名前」）に足すことになる。

#### 基底の形

- **N16 型を共有できるのは、共有の名前空間に置いた型だけ**。`gen/store` と `gen/storemem` の `Ops` が、どちらも同じ `server.StoreOps` を満たすのは、引数と戻り値の型がすべて `gen/type` のものだから（`app/both_test.go:27-32`）。`store` が自分の model を持てば、二つの出力は別々の Go の型を持ち、`server` が import するのは一方だけなので、もう一方の `Ops` は満たさない。「同じ名前空間を二つの単位で出すなら、境を越える型は共有の名前空間に置く」は、P5、P6 では指針だが、この組み方では守らなければ組めない。相手の `Ops` をそのまま渡せること（P31）も、同じ前提の上にある。
- **N17（解決: P32）何がパッケージの変数になるかは、名前空間の境で決まっていた**。P32 の前、`server` は状態を持たないので、すべてが `New(leaves, deps)` の値で、`store` は九つの op がすべて状態の単位にあり、`New()` は引数を取らず、実装はパッケージの変数 `States` にあった。→ P32: パッケージの変数はない。`server` も `store` も、実装はすべて `New` の引数（`gen/server/ops.go:198`、`gen/store/ops.go:62`）。
- **N18（解決: P32）一つのプロセスに二つの `store`**。P32 の前は、二つの出力（`gen/store`、`gen/storemem`）が別のパッケージで、それぞれの `States` を持つので、同じ `server` を両方の上に同時に組み立てられた（今も: `app/both_test.go:41-66`）が、同じ出力の上に二つ（SQLite の DB を二つ、メモリの `Store` を二つ）は持てなかった: 準備が `States` を上書きした。→ P32: 同じ出力の `New` を何度呼んでもよい。二つの組み立ての `TestTwoServers` が、SQLite のデータベース二つ、`memstore.Store` 二つの上で、二つのサーバーに並行に要求を送る（`app/onsqlite/compose_test.go:53`、`app/onmem/compose_test.go:33`）。
- **N19 二つの出力は、どちらも `package store`**。パッケージ名は名前空間から決まり、`out` によらない。両方を import するファイルは、一方に別の名前を付ける（`app/both_test.go:20`）。片方だけでも、import パスの最後（`storemem`）と名前（`store`）が違う（`app/onmem/compose.go:11`）。
- **N20 パッケージ名 `pkgtype`**。共有の型の名前空間を、下書きのとおり `type` と名付けると、Go では `pkgtype.User`、ディレクトリは `gen/type`、import には名前を書くことになる（`gen/server/types.go:5`）。利用者がパッケージ名を選ぶ方法はない。通信の上の例外の名前も `type.Unauthorized` になる（N48）。名前空間を `types` や `model` と名付ければ起きない。
- **N21 状態の単位に置く op**。「`reads` が一つの葉は、その状態の単位の問い合わせ」は、命令の規則（`docs/emit-go.md`「state」）と対に決まった（`gen/store/states.go:13-62`）。単位の中の並びは、形で決まる問い合わせ（N22）、op の問い合わせ、命令。決定が言っていない二つは、ここで決めた: 効果と `reads` の両方を持つ葉は効果の宛先の単位、複数の状態を読む葉はインターフェース `SharedState`（どの状態のものでもなく、実装するのは状態の持ち主なので、`Leaves` ではない。`New` の引数は、状態の単位の後ろの `shared`）。`SharedState` と `shared` という名前は、`shared` という名前の状態と当たる。
- **N22（解決: 形で決まる問い合わせは、いつも出す）状態を読む口（`IsEmpty`、`Get`）は、配線が読むときだけ出す、と決めていた**。生成したコードが呼ぶ口だけを出す、という規則で、帰結は「効果の結果を、読む op のない状態では確かめられない」だった: `notifications` には `addNotification` しかないので、テストは `go-sqlite` の表を SQL で直に数え、メモリの方は人がテストのための読む口を足していた。これは、先にあった決定（`docs/emit-go.md`「state」: 状態の単位は、配線が読まなくても、形で決まる問い合わせをすべて持つ。テストが効果の結果の判定に使う。`docs/decisions.md` の S6 も同じ向き）に反していたので、その決定に合わせて直した。→ 基底は、どの状態の単位にも `IsEmpty` と `Get` を出す（`gen/store/states.go:15-17`、`:58-60`）。`go-sqlite` と `memstore` がそれを実装し（`gen/store/db.go:180-187`、`app/memstore/memstore.go:90-108`）、テストは、その状態の単位の実装の `Get` で読む（`app/onsqlite/compose_test.go:40`、`app/onmem/compose_test.go:20`）。SQL を直に書くテストと、`memstore` の読む口はなくなった。直して分かったこと:
  - **`Ops` に載らない**。形で決まる問い合わせは op ではないので、テストは、`New` に渡した状態の単位の実装のメソッドを直に呼ぶ（`go-sqlite` の型は、同じ `db` から作り直した値でも同じ表を読む）。P32 の後は、実装していなければその型が状態の単位を満たさないので、「埋めていない口」はない。
  - **共通のテストへの渡し方**（解決: P32）。P32 の前は、二つの出力の `StateSet` が別の型で、共通のテスト（`app/storetest`）へは、口を一つずつ渡していた。今は、どちらの実装の型も、共通のテストのインターフェース（`storetest.List[T]`: `IsEmpty` と `Get`）をそのまま満たすので、`store.New` に渡したのと同じ値を渡す（`app/onsqlite/store_test.go:24-29`、`app/onmem/compose_test.go:44-48`）。
  - **`list<T>` の `Get`** は、`docs/emit-go.md`「state」の表にない（そこでは `list<T>` は `IsEmpty` だけ）。状態を名前で読むと `T[]` の値になる（P4、P22）ことから足した。
  - **`Get` は、すべての要素を返す**。件数や条件で絞る書き方はない（N37）。
- **N23 分類について、基底は何も出さない**、と決めた（規則は「基底 `go` が従う規則」の「分類」）。`store.Registration` などの名前は、`gen/server/wiring.go:21,55,93` のコメントにだけ残る。決まっていないこと: 絞り込む場合と、人が述語を書く場合が混ざった分類の形、人が書く述語をどのインターフェースに置き、`New` のどの引数で受け取るか（P32 は葉の op と状態の単位について決めた。P6 は分類の置き場を自由にした）。
- **N24 `match` は `nil` の検査になった**。`return match` はアームごとの `return`（`gen/server/wiring.go:21-27`）、絞り込んだ値は使う位置で `*user`、`*session`（`:24`、`:58`、`:96`）。値を束縛する `match`（`const x = match …`）はこの仕様になく、形を決めていない。
- **N25 `throw` は例外を値で返す**（`gen/server/wiring.go:27,61,99`）。v1 の「分かったこと」43（値かポインタか）は、生成したコードについては決まった。値を運ぶ例外の `throw` は、この仕様に出てこない（P13 の未定のまま）。
- **N26 `Ops` に、業務の途中の op がすべて載る**（P11）。`Authenticate`、`IssueSession`、`Notify` と、七つの葉が、`login` などの入口と同じ `Ops` のメソッド（`gen/server/ops.go:12-99`）。`go-http` の `NewHandler(ops Ops)` も十五のメソッドを受け取り、五つだけ呼ぶ。入口とそうでない op を分ける書き方は仕様にない。P31、P32 の後の形での続きは N61。
- **N27（解決: P31）`Deps` に入るのは、`Ops` のメソッドだけだった**。P31 の前、`server.StoreFindUserFunc` と `store.FindUserFunc` は、中身が同じでも名前の違う関数の型なので、`States` のフィールドは型を変換しなければ `Deps` に入らなかった（メソッドの値は名前のない関数の型なので、そのまま入った）。→ P31、P32: 関数の型も、フィールドに入れる行もない。相手の `Ops` の値を、そのまま渡す。
- **N28（解決: P31）組み立ては、口の数だけの行を人が書いていた**。P31 の前、`server.StoreOps` の八つのフィールドを一つずつ入れる行が、二つの組み立てに同じ形で二度あった。→ P31: `server.StoreOps` はインターフェースで、`store.New(…)` の戻り値をそのまま渡す（`app/onsqlite/compose.go:24`、`app/onmem/compose.go:20`）。`server.StoreOps` にない op（`addUser`）を `store.Ops` が余分に持っていても渡せ、足りなければコンパイルできない。「実装の単位は関数の型」（`docs/emit-go.md`「実装の単位」）は、人が実装するもの（葉の op、状態の単位）についても、P32 でインターフェースのメソッドになった。
- **N29 `runtime` の持ち主**。`go-valid` の出力と `go-http` の出力が使う `runtime.ValidationError` は、このリポジトリの `runtime` パッケージにある。本体が特定の言語を知らない（P9）なら、`runtime` は基底 `go` のもので、`ValidationError` は `go-valid` のものになるはずだが、置き場は決まっていない。P32 の後、**`runtime.MissingError` を使う出力は、この仕様にはない**（P32 の前は、`Kind` が `"leaf"`、`"state"`、`"dep"` のものを基底が返していた）。`runtime` には、ほかの `Kind`（人が書く分類の述語の `"classifier"`、デコレータの `"wrapper"`、制約の `"constraint"`）のためにも残っている。それらを P32 と同じにインターフェースで受け取るかは、この仕様に出てこないので書いていない（N23）。

#### `go-valid`

- **N30 単位をまたぐ依存**。`server` の `Validate` は `gen/type` のメソッドを呼ぶ（`gen/server/validate.go:15-16,36,47`）ので、`type` の単位にも `go-valid` を並べなければコンパイルできない。P18 が言うのは「一つの単位の `transpilers` に依存をすべて書く」で、別の単位に何を並べるかは範囲の外。型の名前空間を出す Go の単位がない（別の言語、単位なし）ときは、メソッドでは書けず、使う側のパッケージに関数を出す形が要る。
- **N31 `go-http` は、`go-valid` の規則を知っている**。入力の model が `Validate` を持つか（`LoginInput` と `PlaceOrderInput` は持ち、`AuthInput` と `GetOrdersInput` は持たない）を、`go-http` が `go-valid` と同じ規則で決める（`gen/server/http.go:35,113` で呼び、`:54,68,91` では呼ばない）。「依存を宣言する」（P17）だけでは足りず、依存先の出力の形を再現している。v1 の「分かったこと」2 と同じ構図。
- **N32 依存がオプションで変わる**。`go-http` が `go-valid` に依存するのは `validate = true` のときだけ。P17 の依存は、オプション（P16）によらず宣言するのか、決まっていない。
- **N33 メソッド名 `Validate` が、フィールドと当たる**。model に `validate` というフィールドがあれば、Go では同じ型にフィールドとメソッド `Validate` ができ、コンパイルできない。
- **N34 誰も呼ばない `Validate`**。結果の型 `LoginOk` にも出る（`gen/server/validate.go:34-41`）。補助の関数 `validAppend` は、パッケージごとに同じものが出る（`gen/type/validate.go:88`、`gen/server/validate.go:56`）。
- **N35 `store` は制約を確かめない**。`gen/store` の単位は `go-valid` を並べず、`go-sqlite` は足す前に確かめない。`store.addUser` に、名前が空のユーザーを渡せば入る。確かめるのは HTTP の境だけ（P15 の「いつ確かめるかは、境界を持つプラグインが決める」のとおり）。

#### `go-sqlite`

- **N36 規則は、この仕様の九つの op に足りた**（「`go-sqlite` が従う規則」の末尾）。人が `store` について書いたのは、組み立ての二つの文（表を作る `CreateTables` と、四つの型を `store.New` に渡す行。`app/onsqlite/compose.go:18-22`）だけ。
- **N37 規則で表せないもの**。
  - **順序**: `listOrders` の「足した順」は説明のコメントにしかない（`store.hozo:30`）。規則が `ORDER BY rowid` と決めているだけで（`gen/store/db.go:156`）、「新しい順」は書けない。
  - **件数と位置**: ページで切るのは、すべて読んだ後の `pageOf`（`gen/server/wiring.go:111-115`）。
  - **比較**: 「期限が今より後」。読む op では書けないので、読んだ後の葉 `requireFresh` で確かめる（N3）。
  - **一意の違反**: `addUser` で `email` が重なると、ドライバのエラーがそのまま返り、HTTP では 500 になる。仕様の例外に写す書き方がなく、`db.go` はドライバを import しないのでエラーを見分けられない（`app/storetest/storetest.go:96-101` は「エラーである」ことだけ、`app/onsqlite/store_test.go:179-181` は文言を見ている）。人が書き足せる場所はできた: `UsersTable` を埋め込んだ型で `AddUser` を置き換えれば、ドライバのエラーを見分けて写せる（N63）。写す先の例外は、仕様の `addUser` に `throws` がないので、ない。
  - **書き換え**（`put`）、**複数の状態にまたがる読み取り**: この仕様にないので、規則を決めていない。
  - **複数の op を一つのトランザクションにする**: `placeOrder` の `addOrder` と `addNotification` は別々の文（`gen/server/wiring.go:129`、`:147`）。配線が op の間に渡すのは仕様の引数と `ctx` だけで、`ctx` に値を載せない（P29）ので、トランザクションを運ぶ道がない。v1 では一つの葉の中に書けた処理が、配線になった分、原子的に書ける範囲は狭くなった。状態ごとの型は、それぞれが `*sql.DB` を持つ（`*sql.Tx` を受け取る作り方はない）。
- **N38 省略可能な引数の規則は、下書きの書き方のために足した**（N5）。「値がなければ、問い合わせずに『ない』」（`gen/store/db.go:109-111`）。SQL の `= NULL` がどの行にも合わないのと同じ結果にした。
- **N39 引数と列を、型で結ぶ**。`findUser(email: type.Email)` が列 `email` に、`listOrders(userId: type.UserId)` が列 `userId` に結ばれるのは、フィールドごとに別の scalar を宣言しているから。`name: string` のような核の型の引数は、`string` のフィールドが二つあれば（`type.User` の `name` と `hash`）結べない。引数の名前は見ていない（`findUserById` の引数は `id`、`listOrders` の引数は `userId` で、どちらも型は `type.UserId`）。
- **N40 `delete` で行が減らなくても、エラーにしない**（`removeSession`。`gen/store/db.go:127-130`）。組み込みの効果 `delete` は「列数が減る」変化だが、消す行がないときを区別する書き方はない。
- **N41 `go-sqlite` の公開の名前**。P32 の前は `Schema` と `SetupDB` だけだった（v1 の表の型 `Tables`、`Insert`、`Get` などは、人が呼ぶ理由がなくなったので出さなくなっていた）。P32 の後は `Schema`、`CreateTables` と、状態ごとの `<状態>Table`、`New<状態>Table`（この仕様で、合わせて十）。状態ごとの型が戻ったが、v1 の表の型とは違い、メソッドは基底の状態の単位のもの（形で決まる問い合わせと、仕様の op）だけで、`go-sqlite` が自分で決めた読み書きはない。仕様に op のない読み書きのうち、状態の値をすべて読むことは、形で決まる問い合わせでできる（N22）。それで足りないもの（条件を付けた読み取り、書き換え）は、人が SQL を書く。
- **N42 `@db.key`、`@db.unique`、`@db.ref` の意味は、`go-sqlite` の出力にしかない**。`gen/storemem` の単位では、どのトランスパイラも `@db` を読まない（印）。仕様は「`addUser` は、`email` が重なれば失敗する」と言っていないので、メモリの上の実装がそうするかは人が決める。→ キーと一意は合わせ（`app/memstore/memstore.go:18`、`:149-150`）、参照先の検査は合わせていない（`app/onsqlite/store_test.go:172-181` の `TestRef` は SQLite だけ）。「同じ `store` の二つの実装」が同じ振る舞いをすることは、仕様からは言えない。状態の単位のインターフェースが保証するのは、メソッドの形までで、振る舞いではない。
- **N43 プラグインごとの局所の名前**。状態ごとの型のメソッドの引数は、基底と同じ規則で仕様の引数名から作る（`email`、`token`）。`go-sqlite` が自分で使う名前（レシーバの `t`、`args`、`err`）と当たれば、基底と同じく末尾に `_` を付けることになるが、当たる名前の一覧はプラグインごとに違う。形で決まる問い合わせのメソッド名は、P32 の前は `go-sqlite` が決めていて（`UsersGet`）、同じ名前になる op（`usersGet`）と当たった。今は基底のメソッド名のまま（`Get`）なので、衝突は基底の側のものになった（N58）。

#### `go-http` と JSON の形

- **N44 op を呼ぶ前の失敗を、例外の形にそろえた**。P15 の「失敗は、そのプラグインの語彙の例外で伝える」に合わせ、`{"error":"http.BadRequest","value":{"message":…,"violations":[…]}}` とした（`gen/server/http.go:128-151`）。v1 は `{"error":"http.BadRequest","message":…}` で、例外の形（`error` と `value`）ではなかった。クライアントのトランスパイラは、ほかの例外と同じ規則で読める。**`http` の語彙には、`BadRequest` も、それが運ぶ値の model もまだない**（`vocab/http.hozo` にあるのは `ServerError` と `NetworkError`）。
- **N45 `http.BadRequest` を投げるかどうかが、設定で変わる**。制約の違反で投げるのは `validate = true` のときだけ。呼ぶ側の名前空間が計算する `throws`（`@http.post` の宣言が持つ例外）は仕様だけから決まるので、単位のオプション（P16）で増える例外を表せない。
- **N46 違反の位置を、通信の上の場所にした**。`Validate` の `Path`（`lines[1].quantity`）の先頭を、そのフィールドを読む場所に置き換える（`body[1].quantity`。`gen/server/http.go:36,114,192-201`）。v1 の「分かったこと」39 はこれで解決した。
- **N47 400 が 401 より先になる**。制約を確かめるのは op を呼ぶ前なので、トークンのない `POST /orders` でも、本文が制約に合わなければ 400（`app/handlertest/handlertest.go:167-170`）。v1 は葉が先に認証していた。順序を選ぶ書き方はない。
- **N48 例外の FQN が、型の置き場を通信に出す**。`{"error":"type.Unauthorized"}`（`gen/server/http.go:218,220`）。v1 は `server.Unauthorized`。例外を別の名前空間に移すと、クライアントが見る名前が変わる。

#### 名前の衝突

- **N49 プラグインが足す公開の名前**。基底: `New`、`Ops`、`Leaves`、`SharedState`、`<Ns>Ops`、`<状態>State`、`Effect<効果>`、`New<Model>`（P31、P32 でなくなったもの: `Deps`、`States`、`StateSet`、`<Op>Func`、`<Ns><Op>Func`）。`go-valid`: メソッド `Validate`（N33）。`go-sqlite`: `Schema`、`CreateTables`、`<状態>Table`、`New<状態>Table`。`go-http`: `NewHandler`。利用者の宣言と当たる例: model `Schema`、model `UsersTable`（状態 `users` の型と当たる）、既定値を持つ model `Handler`（基底の `NewHandler()` と `go-http` の `NewHandler` が当たる）、既定値を持つ model `UsersTable`（基底の `NewUsersTable()` と `go-sqlite` の `NewUsersTable` が当たる）。プラグインをまたぐ衝突を見つける者がいないのは、v1 の「分かったこと」48 のまま。
- **N50 非公開の名前とファイル名**。前置きは `valid`、`db`、`http`、ファイルは `validate.go`、`db.go`、`http.go`。どれも、プラグインどうしの約束がないまま、当たらないように選んだだけ（v1 の「分かったこと」47、49）。

#### 設定とテスト

- **N51 今の hozoc は、この `hozo.toml` を読めない**。`[emit.options.go-http]` は、単位の知らないキー `options` として CFG-7 になり、その単位（`gen/server`）は読まれない。`tests/samples_test.go` は、`check` が保留のサンプルに限り、このテーブルを除いた写しから単位を読む（`../README.md`「保留」）。
- **N52 「共有する形」と「トランスパイラのファイル」の区別がなくなった**（P9）。`gen/` のどのファイルも、どれかのトランスパイラのもの。`sample.json` の `select`（代役の規則）は書いていない。
- **N53（解決: P32）テストは並列にできなかった**。`store.States` がパッケージの変数だったので、サーバーを作り直すサブテストは順に走らせていた。→ P32: サーバーも `store` も、作るたびに別の値で、何も共有しない。`app/handlertest` と `app/storetest` のサブテストと、その上のテストを `t.Parallel()` にした（`app/handlertest/handlertest.go:115-124`、`app/storetest/storetest.go:60-77`）。`go test -race` で通る。

#### 実装をインターフェースで受け取る形（P31、P32）

- **N54 `New` の位置引数は、状態と名前空間の数だけ並ぶ**。`store.New` は四つ（`gen/store/ops.go:62`）、`server.New` は三つ（`gen/server/ops.go:198`）。状態が二十あれば、引数は二十。Go では、呼ぶ側に引数の名前を書けないので、何番目がどの状態かは、型だけが頼りになる。
  - **取り違えは、型が違えばコンパイルエラー**。ただし、**メソッドの並びが同じ二つの状態**（同じ形、同じ要素の型で、どちらにも、それを読み書きする葉の op がない。例: `state drafts: list<type.Order>` と `state archive: list<type.Order>`）は、インターフェースの中身が同じになり、入れ替えて渡してもコンパイルが通る。フィールドに名前で入れる形（P32 の前）では、起きなかった取り違え。
  - **並びは宣言順**なので、仕様で状態の宣言を入れ替えると、`New` を呼ぶすべての行が変わる。状態か、呼ぶ名前空間を一つ足しても同じ（こちらは、渡し忘れをコンパイルエラーにする、という狙いどおり）。別の名前空間を辞書順にしたのは、配線の本体を書き換えただけで並びが変わらないようにするため。
  - → サンプルでは、四つを一行に並べた（`app/onsqlite/compose.go:22`、`app/onmem/compose.go:18`）。P32 の「フィールドは省けるが、引数は省けない」と引き換えに、名前を失った。
- **N55 `New` の引数の名前**。`leaves`、状態の名前、別の名前空間の名前を、そのまま引数の名前にした。当たる例: `leaves` や `shared` という名前の状態、自分の状態と同じ名前の別の名前空間（`mailer` という状態を持つ名前空間が、名前空間 `mailer` を呼ぶ）、Go の予約語の名前空間（`type` の op を呼ぶ名前空間の `New` は、引数 `type` を持てない）。基底の、末尾に `_` を付ける規則（`docs/emit-go.md`「名前」）に足すことになる。非公開の構造体 `ops` のフィールドも同じ名前を使う。この仕様では、どれも起きない。
- **N56 `nil` は、書けば渡せる**。`server.New(nil, nil, nil)` はコンパイルが通る。型のある `nil`（`(*app.Leaves)(nil)`）、プラグインの型の零値（`store.UsersTable{}`: `db` が `nil`）、フィールドを入れていない人の型（`&app.Leaves{}`: `Now` が `nil`）も同じ。`New` は確かめないので、その実装を使う op が呼ばれたところで panic になる（`app/server_test.go:206-214`）。`*runtime.MissingError` のような、見分けられるエラーにはならない。`net/http` のサーバーは、ハンドラの panic を受け止めて、その接続を切る（500 の `internal` は返らない）。P32 が防ぐのは「書き忘れ」で、「`nil` と書くこと」ではない。`New` が `nil` を確かめるなら、panic するか、`New` が `(Ops, error)` を返す形になる。→ 確かめない、とした（生成したコードから、実行のときの「ない」をなくす方を取った）。人の型のフィールド（`app.Leaves` の `Now` など）の入れ忘れには、P32 は届かない。
- **N57 葉の op は、すべて一つの値のメソッドになる**。`server.Leaves` は七つのメソッドを持つ一つのインターフェースで、`New` の引数は一つ。→ 一つの型 `app.Leaves` に七つを書いた（`app/leaves.go:22-88`）。分かったこと:
  - **外から何も受け取らない葉も、メソッドになる**。`checkPassword`、`toPublicUser`、`composeNotice` はレシーバを使わない（P32 の前は、ただの関数を口に入れていた）。
  - **人の型の名前と、葉のメソッド名が、同じ場所に入る**。`app.Leaves` のフィールド `NewOrderID` と、葉のメソッド `NewOrder` は一語違い（`app/leaves.go:28`、`:78`）。フィールドを `NewOrder` と名付ければ、コンパイルできない。仕様に葉を足すと、人の型にある同じ名前のフィールドや補助のメソッドと当たる。
  - **分けて書くには、埋め込む**。関心ごとの型（認証の葉、注文の葉）を一つの構造体に埋め込めば、メソッドが合わさって `server.Leaves` を満たす。二つの型が同じ名前のメソッドを持てば、渡す行が「あいまい」でコンパイルできない。葉の一部を、その語彙を読むトランスパイラが実装するとき（この仕様にはない）も、`go-sqlite` の状態の型と同じに、プラグインの型を人の型に埋め込むことになる。仕様の側に、葉をまとまりに分ける書き方（`Leaves` を二つのインターフェースにする単位）はない。
- **N58 標準の読み取り（`IsEmpty`、`Get`）と、op の名前が、同じインターフェースに入る**。状態の単位のメソッドは、形で決まる問い合わせと、その状態の葉の op。その状態を読む op が `get` か `isEmpty` という名前なら、同じ名前のメソッドが二つになり、生成したコードがコンパイルできない（P32 の前の、構造体のフィールドでも同じだった。避ける規則は、ないまま）。`Ops` には形で決まる問い合わせを載せないので、そちらは当たらない。もう一つ: どの状態の単位も `Get` を持ち、戻り値の型が状態ごとに違うので、**一つの型は、要素の型が違う二つの状態の単位を兼ねられない**。→ `go-sqlite` の型は状態ごとに一つ（四つ）、`memstore` も、一つの `Store` の上に、状態ごとの型を四つ書いた（`app/memstore/memstore.go:42-63`）。要素の型が同じ状態どうしなら兼ねられるが、そのときは N54 の取り違えが起きる。
- **N59 テストが op を一つだけ差し替えるには、型を一つ書く**。P32 の前は、`server.StoreOps{FindSession: func(…) {…}}` と、差し替える関数だけをフィールドに入れればよかった（入れなかった口は、呼ばれたら `*runtime.MissingError`）。今は、インターフェースを満たす型が要る。→ 二つの書き方を使った。
  - **インターフェースを埋め込み、書きたいメソッドだけを書く**。`seenStore` は `nil` の `server.StoreOps` を埋め込み、二つのメソッドを書く（`app/server_test.go:49-62`）。書いていない op が呼ばれれば、`nil` のインターフェースの呼び出しで panic になる。生成したコードからなくした「実行のときの、ない」が、テストの中にだけ、人の選びで戻る。`server.Ops` のスパイ（`spyOps`）は、前からこの形だった。
  - **本物の実装を埋め込み、一つだけ置き換える**。`fixedNotice` は、`server.Leaves` を満たす値を埋め込んで `ComposeNotice` を置き換える（`app/server_test.go:106-109`）。`humanUsers` も同じ形（N63）。
  - Go では、メソッドを関数の中に書けないので、型はテストの関数の外に置き、テストごとの値はフィールドで渡す（`seenStore.seen`、`humanUsers.added`）。クロージャ一つで済んでいたものが、型とフィールドとメソッドになる。
- **N60 `throws` を足すと、メソッドの形が変わる**。純粋で `throws` のない op に `throws` を足すと、戻り値に `error` が増える（`NewSession(user) Session` が `NewSession(user) (Session, error)` になる）。`Leaves` と `Ops` のメソッド、人が書いた本体、`Ops` を呼ぶ人のコード（`x := ops.NewSession(u)`）が、コンパイルできなくなる（気づける）。配線は生成し直される。最後の `throws` を外せば、逆のことが起きる。N12 の `ctx` と合わせると、op の形は三通り（純粋でない / 純粋で `throws` がある / 純粋で `throws` がない）で、仕様の一語で行き来する。分かったこと:
  - **`error` のない葉には、失敗を返す道がない**。`newSession` と `newOrder` は、トークンと識別子を作る。main では乱数（`crypto/rand`）で、読めなければ panic にしている（`app/cmd/shop-sqlite/main.go:68-71`）。「純粋で `throws` がない」は仕様の上の性質で、本体が本当に失敗しないことは、誰も確かめない（N9 と同じ構図）。失敗を返したければ、仕様に `throws` を書く（業務の例外ではない失敗のための、ステータスを持たない例外を宣言することになる）。
  - **純粋な配線**は、この仕様にない（配線は八つとも、純粋でない op を呼ぶ）。`error` を返さない配線の本体（`return o.<Op>(…)` が `error` なしで続く形、`match` の `throw` のない形）は書いていない。
  - **戻り値がなく、純粋で、`throws` もない op** は、引数を受け取って何も返さないメソッドになる。意味のある本体は書けないが、検査は通る。
- **N61 `Ops` は、今も業務の途中の op をすべて載せる**（N26 の続き）。`server.Ops` の十五のメソッドのうち七つは、`New` に渡された `leaves` の同じ名前のメソッドを呼ぶだけ（`gen/server/ops.go:209-234`）で、組み立てた人は、その値をすでに持っている。P31 で、別の名前空間が使う op は、呼ぶ側が自分のインターフェースで言うようになった（`server.StoreOps` は `store` の九つのうち八つ）ので、`Ops` が「すべて」である理由は「op はすべて出る」（P11）だけになった。重さが出るのは `go-http`: `NewHandler(ops Ops)` は十五のメソッドを求めて、五つだけ呼ぶ。スパイは `server.Ops` を埋め込んで、二つだけ書く（`app/server_test.go:150-160`。N59 の一つ目の書き方）。`go-http` が、自分が呼ぶ op だけのインターフェースを出して受け取れば（P31 と同じ考え）、`New` の戻り値はそのまま渡せて、スパイは五つを書けば済み、書き忘れはコンパイルエラーになる。→ `NewHandler(ops Ops)` のままにした。入口とそうでない op を分ける書き方が仕様にないことは、変わらない。
- **N62 同じ名前空間を二つの単位で出す理由が、薄くなった**。P32 の前は、実装がパッケージの変数に入ったので、SQLite とメモリを一つのプロセスに持つには、出力を二つ（`gen/store`、`gen/storemem`）に分けるしかなかった。今は、実装は `New` の引数。二つの出力の状態の単位は、中身が同じインターフェースなので、`gen/store` の `New` に `memstore` の型を渡せる（`app/both_test.go:34-35`）。一つの `store` の中で、状態ごとに別の実装を混ぜることもできる（`users` は SQLite、ほかはメモリ。`app/both_test.go:73-81`）。`gen/storemem` に残る意味は、`go-sqlite` のファイル（`db.go`）と `database/sql` の import を持たないパッケージであること。→ 二つの単位は残した（基底のファイルが、単位に並べたほかのトランスパイラによらないことを、`app/both_test.go` の `TestStoreOutputsShareBaseFiles` が確かめる）。混ぜたとき、`@db.ref` の `REFERENCES` は SQLite の表の間でしか効かない（N42）。
- **N63 プラグインが出す型の形は、ここで決めた**。P32 が言うのは「プラグインは、機械的に決まる op を実装した型を出し、足りない op は人が書き足す」まで。決めたもの: 名前（`<状態>Table`、`New<状態>Table`）、値の型でありポインタではないこと、作るのが関数であること（`db` を渡し忘れられない。零値は書ける: N56）、すべて実装できた型に出す `var _ <状態>State = …`。足りない op があるときに分かったこと（見立ての型で確かめた。`app/onsqlite/store_test.go:94-117`）:
  - **足りないことを人が知るのは、`New` に渡した行のコンパイルエラーから**（`missing method AddUser`）。プラグインがどの op を実装しなかったかの一覧は、どこにも出ない。足りない型には `var _` の行がない、という違いだけがある。
  - **書き足す人の型は、同じ `*sql.DB` を自分でも持つ**。プラグインの型の `db` は非公開で、読み書きの補助（`dbScanUser`、`dbArgsUser`）も非公開なので、人の SQL は列の並びを書き直す（`app/onsqlite/store_test.go:104-108`）。
  - **置き換えもできる**。すべて実装した型でも、埋め込んで一つのメソッドを書けば、そのメソッドだけが人のものになる（一意の違反を見分ける `AddUser` など。N37）。プラグインの型は、埋め込まれることを前提にした形になっている。
- **N64 表を作り忘れても、コンパイルは通る**。`go-sqlite` の出力を使う手順は二つ（`CreateTables` を呼ぶ、型を `New` に渡す）で、P32 がコンパイルエラーにするのは後ろだけ。`CreateTables` を呼ばなければ、はじめの問い合わせが「表がない」というドライバのエラーになる。型を作る関数が `ctx` を取って表も作る形（`NewUsersTable(ctx, db) (UsersTable, error)`）なら忘れられないが、作るたびに文を実行し、`error` が増え、`store.New(…)` の引数に直に書けなくなる。→ 分けたままにした（表を作るのは、起動のときに一度）。移行の形がないこと（v1 の 59）とも、つながる。
- **N65 op の説明が、インターフェースごとに重なる**。P32 の前は、op の説明と `Throws:` などは、op ごとの関数の型（`<Op>Func`）のコメントに一度と、`Ops` のメソッドにあった。関数の型がなくなったので、その op のメソッドを宣言するところごとに書く: `checkPassword` は `Ops` と `Leaves` に同じコメント（`gen/server/ops.go:26`、`:105`）、`store.findUser` は `store.Ops`、`store.UsersState`、`server.StoreOps` の三か所。→ `Ops`、`Leaves`、`<Ns>Ops` には op の説明を、状態の単位には、問い合わせか命令かの一行を書いた。`Ops` が `Leaves` を埋め込めば重なりは減るが、`Ops` のメソッドの並びが宣言順でなくなる。
