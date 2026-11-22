# サンプル `shop-server`

小さな店のサーバー（ログイン、自分の情報、注文）を、サンプルを書きながら決めたこと（`docs/decisions.md`「サンプルを書きながら決めたこと（P の番号との対応）」の P1〜P39）を**当てはめた後の形**で書いたもの。サンプルとは何か、置き方、テストの使い方は [`../README.md`](../README.md)。

P1〜P39 の言語の変更（`reads`、`as`、`throw`、`never`、`override`、`export`）と、ここに出てくる五つのトランスパイラ（`go`、`go-valid`、`go-sqlite`、`go-http` と、サンプルの中のプラグインの `go-ratelimit`。設計文書がある。`go-ratelimit` は、この README）は、実装が済んでいる。**その後の決定（`docs/decisions.md`「決めたこと」91〜122 と「設計書から離れた決定」S24: デコレータが出すコードは型に依らず、`store` の読み書きと、表と、列のしまい方は `db` の語彙のデコレータで、要求のどこから読むかと、項目がないときの扱いは `http` の語彙のデコレータで言う。フィールドの既定値は、言語にない）は、このサンプルと設計文書を先に書き、その後に実装した**（「まだないもの」に、書いたときになかったものと、今の状態）。**さらにその後の決定（同 124、132〜143 と S25〜S28: 節の区切りは `,` 一つ。状態に触れるのは、引数を持つ `read` と効果だけで、その中身は `db` の語彙のデコレータが宣言から作る。op は、それを呼ぶ配線か、状態に触れない葉。op のデコレータは、包むだけ）も、このサンプルと設計文書を先に書き、その後に実装を入れた**（書いたときになかったものと、今の状態は、同じ節の「R4 で、まだないもの」）。単位の `out`（`gen/`）の下のファイルはすべて、「`hozoc build` がこう出す」内容を人が書いたもので、トランスパイラを実装するときのブラックボックスのテストの仕様になる。コードはコンパイルでき、人が書いた部分（`app/`）と合わせて動く。 その後に足した、六つ目のトランスパイラ `go-design`（設計文書がある。テストを受け取る口だけを出す）は、「R6 のプラグインの部分で足したこと」。

**生成するコードの規則の持ち主は、トランスパイラごとの設計文書**（「各トランスパイラの規則」）。この文書が持つのは、ファイルの出どころ、この仕様に固有の事実、サンプルの中のプラグイン `go-ratelimit` の規則、書いて分かったこと。期待する出力（`gen/`）は、設計文書の形のとおりに書いてある（文書と違うところは、残していない）。

P1〜P33 を当てはめる前の形（一つの名前空間 `server` を三つの単位で出力する。以下「v1」）は、コミット `ac57788` のこのディレクトリにある。

## まだないもの

このサンプルは、実装より先に書いた（`docs/testing.md`「トランスパイラのテスト」）。書いたときになかったのは、`docs/requirements.md`「作り直しの段階」の R3 の実装（今の状態は、表の後ろ）。**この表と、その後ろの二つの段落は、R3 を書いたときの記録**で、`store` の書き方（`@db.table(type.<Model>)`、op に付ける `@db.find` / `@db.effects`、`SharedTable`、`@db.nullable`）と、引いている文書の見出しは、そのときのもの。今の形は、次の「R4 で、まだないもの」と、この節より下:

| ないもの | このサンプルで使っているところ | 文書 |
|---|---|---|
| 言語: 効果の宣言に付けるデコレータ、予約語の名前のデコレータ、デコレータが運ぶ `reads` / `effects`、残りの引数 | `store.hozo` の `@db.insert(users) effect createUser …`、`@db.list(orders, "userId")`、`@db.effects(createOrder, notifyUser)` | `docs/grammar.md`「decorator の宣言」、`docs/check.md`「デコレータが運ぶ reads と effects」 |
| 語彙 `db` の `find` / `list` / `insert` / `update` / `delete` / `effects` | `store.hozo` の八つの op と五つの効果 | `docs/vocab.md`「`db` の読み書きのデコレータ」 |
| 語彙 `db` の `table` と、しまい方（`text` / `time` / `json` など） | `store.hozo` の四つの状態の `@db.table(type.<Model>)`、`type.hozo` の四つの表の model のフィールド | `docs/vocab.md`「`db` の表と、しまい方」 |
| 語彙 `http` の、名前を省ける `header` / `query` / `body` と、`headerObj` / `queryObj` / `bodyObj` | `server.hozo` の `@http.bodyObj body`、`@http.query page`、`@http.body lines` | `docs/vocab.md`「`http` の語彙」 |
| 語彙 `http` の `required` と `default`（項目がないときの扱い）。言語から、フィールドの既定値がなくなる | `server.hozo` の `@http.required @http.body lines`、`@http.query @http.default(1) page: int`（前は `page: int = 1`） | `docs/vocab.md`「`http` の語彙」、`docs/grammar.md`「model」 |
| 語彙 `db` の `nullable`（値を持たなくてよい列） | 使っていない（表の model に、省略可能なフィールドがない）。`gen/store/db.go` の補助が、それを受け取る形（`dbOption`）になっているだけ | `docs/vocab.md`「`db` の表と、しまい方」 |
| `go-sqlite`: デコレータの引数だけから中身を出す。複数の効果は一つのトランザクション。`SharedTable`。列ごとのコードは、型に依らず、しまい方の補助の呼び出し | `gen/store/db.go` | `docs/plugin-go-sqlite.md`「op の中身」「トランザクション」「状態をまたぐ単位の型」「しまい方」「補助」 |
| `go-http`: フィールドごと・経路ごとのコードは、型に依らず、同じ補助の呼び出し。項目がないときの扱いは、デコレータが足す引数（`httpRequired`、`httpDefault(1)`）で決まる。成功の応答は、実行時に、返す値で決める | `gen/server/http.go` | `docs/plugin-go-http.md`「出力」「入力の読み方」「補助」 |
| 版 0.5 | `plugins/go-ratelimit/hozo-plugin.toml` の `protocol` は `"0.5"`。hozoc と、リポジトリのプラグインのマニフェストと、一緒に上げた | `docs/ir.md`「版」 |

**この表のどれにも、実装がある**（版 0.5 の hozoc。`docs/requirements.md`「作り直しの段階」の R3）。`TestSamples` のサブテストは、最後の形で書いてあり、足りないものがあれば、名指しして飛ばす（`../README.md`「飛ばす条件」「移行の間」）。R4 の書き方を入れる前のこのサンプルでは、`check`、`files`、`go`（このモジュールの `go vet` と `go test`）と、六つの `transpilers/*` が、飛ばしも、代役もなく、すべて通っていた。今の状態は、この節の「R4 で、まだないもの」。`sample.json` に、保留を書くところはない。

この段落の確かめは、R3 を書いたときのもの。`gen/type`、`gen/rate` と、`gen/server` の基底と `go-valid` と `go-ratelimit` のファイル（`http.go` のほかのすべて）は、そのときの hozoc でも確かめてある: 仕様の写しを、今の hozoc が読める書き方に直して（`store.hozo` は、同じ op を、`reads` と `effects` を op に書く形で。`placeOrder` の効果は一つだけ。しまい方のデコレータを外し、`@db.table(…)` を `@db` に、`@http.bodyObj` を `@http.body` に）`hozoc build` を掛けると、それらのファイルは、`placeOrder` の `Effects:` のコメント（効果が一つになる）のほかは、期待する出力とバイト単位で同じ。`gen/server/http.go`、`gen/store` と `gen/storemem` の `SharedState`、デコレータで決まる `db.go` は、文書のとおりに手で書いた。**この確かめは、決定 114〜122 を入れる前のもの。** その後、`gen/server/inner/types.go` と `gen/server/types.go` から、`NewGetOrdersInput`（既定値を入れる関数）を、手で外した。外した後の二つのファイルは、道具では確かめていない（「分かったこと」N110）。

### R4 で、まだないもの

`docs/requirements.md`「作り直しの段階」の R4（`docs/decisions.md`「決めたこと」124、132〜143）も、実装より先に、このサンプルに入れた。**次の表は、書いたときになかったもの。** 今の状態は、表の後ろの箇条。

| ないもの | このサンプルで使っているところ | 文書 |
|---|---|---|
| 言語: `throws` / `catches` / `effects` / `reads` の節の区切りの `,` | `server.hozo` の `placeOrder` の `throws type.Unauthorized, type.NotificationFailed`（この仕様で、節に二つ以上を並べているのは、ここだけ） | `docs/grammar.md`「op」「decorator の宣言」 |
| 言語: 引数を持つ `read` と `effect` の宣言、配線からの呼び出し。葉の op が `reads` / `effects` を書けないこと | `store.hozo` の五つの `read` と五つの効果と、それを呼ぶ九つの配線 | `docs/grammar.md`「exception / effect / read」「配線」、`docs/check.md`「状態に触れるもの」 |
| 語彙 `db`: 引数のない `table` / `find` / `list` / `insert` / `delete`、`latest`、`transaction` | `store.hozo` の四つの状態、五つの `read`、五つの効果、`placeOrder` | `docs/vocab.md`「`db` の語彙」 |
| 基底 `go`: `read` と効果が、状態の単位のメソッド。配線がそれを呼ぶ。`SharedState` がない | `gen/store` と `gen/storemem` の `states.go`、`ops.go`、`wiring.go` | `docs/plugin-go.md`「op の置き場」「状態の単位」「呼び出し」 |
| `go-sqlite`: `read` と効果の中身を、宣言から作る。`@db.transaction` の継ぎ目（マニフェストの `seams`）と、その実装 | `gen/store/db.go`、`gen/store/ops.go` の `Seams` | `docs/plugin-go-sqlite.md`「read の中身」「効果の中身」「トランザクション」 |
| `go-http`: 例外の値を、引数の名前をキーにしたオブジェクトで返す | `gen/server/http.go` の `httpThrown`（この仕様の例外は、どれも引数を持たないので、応答は変わらない） | `docs/plugin-go-http.md`「失敗の応答」 |
| 文書の版（`docs/ir.md`「版」） | `plugins/go-ratelimit/hozo-plugin.toml` の `protocol`。hozoc と、リポジトリのプラグインのマニフェストと、一緒に上げる | `docs/ir.md`「版」 |

- **表のどの行にも、このサンプルが使う分の実装が入った**（版 0.6 の hozoc と、基底 `go`、`go-valid`、`go-sqlite`、`go-http`、サンプルの中の `go-ratelimit`）。このサンプルの `TestSamples` は、`check`、`files`、`go` と、六つの `transpilers/*` が、飛ばしも、代役もなく通る。実装が入るまでの間に、どのサブテストが通らなかったかの記録と、段階の全体の持ち主は、`../README.md`「移行の間」と、`docs/requirements.md`「作り直しの段階」の R4 の行。
- **表のうち、基底 `go` の行は、実装が入った。** 基底のファイル（`sample.json` の `go` の 21 個）は、`hozoc build` の出力と、バイト単位で同じ（手で書いた期待する出力に、直すところはなかった）。確かめたのは、相乗りを代役（ファイルを返さないトランスパイラ。`../README.md`「飛ばす条件」）に置き換えた形で、`go-sqlite` の代役のマニフェストには、文書のとおり `seams = ["db.transaction"]` を足した。本物の `go-sqlite` のマニフェストが `seams` を宣言するまで、`gen/store` の `ops.go` と `wiring.go` は、継ぎ目のない形（`gen/storemem` と同じ形）で出る。
- **`plugins/go-ratelimit/testdata/` は、先に、入力の仕様だけを直してあった**（`seams/app.hozo` の、状態に触れる二つの op を、`read` と効果を呼ぶ配線に）。R4 の実装と一緒に、`views.json`（hozoc が置く `View`）を置き直し、`seams/behavior_test.go` の、状態の単位の見立てを、`read` と効果のメソッド（`ItemById`、`AddItem`）に直した（「サンプルの中のプラグイン」の「実装」）。`go-ratelimit` のコード（`emit.go`）と、その出力（ゴールデン `expected/` と、`gen/server/ratelimit.go`）は、変えていない: 継ぎ目のメソッドは、op の形が同じなので変わらない。
- この段階の期待する出力を書いたときは、どの hozoc でも確かめていなかった（「分かったこと」N121）。今は、`TestSamples` の `transpilers/*` が、`hozoc build` の出力とバイト単位で比べている。

### R5 で変えたこと

`docs/requirements.md`「作り直しの段階」の R5（`docs/decisions.md`「決めたこと」145〜148: 基底は op の中身に立ち入らず、どの関数も `ctx` を取り、`error` を返す）も、実装より先に、このサンプルに入れた。仕様（`.hozo`）と、`sample.json` は、変えていない。

| 変えたもの | 変えたこと | 文書 |
|---|---|---|
| `gen/server/inner/ops.go` | 七つの葉のメソッド（`Ops`、`Leaves`、構造体 `ops`）が、`ctx` を取り、`error` を返す。コメントの「純粋…」の行が、ない。`MailerOps` と `StoreOps` のコメントの二行目 | `docs/plugin-go.md`「op の形」「コメント」 |
| `gen/server/inner/wiring.go` | 葉を呼ぶ八つの行が、`ctx` を渡し、`error` を受ける（`session, err := o.NewSession(ctx, user)` と失敗の扱い。`return o.ToPublicUser(ctx, *user)`） | 同「呼び出し」「文」「match」 |
| `gen/server/ops.go` | `MailerOps` と `StoreOps` の別名のコメントの二行目 | 同「別の名前空間の op」 |
| `app/leaves.go` と、その葉を書くテスト | 七つのメソッドが、同じ形。`NewToken` と `NewOrderID`（`app.Leaves` のフィールド）は、`error` も返す | 「人が書いたもの」 |

- `gen/` のほかのファイル（`gen/type`、`gen/rate`、`gen/store`、`gen/storemem` と、`gen/server` の `http.go`、`ratelimit.go`、`validate.go`、`types.go`、`doc.go`）は、変わらない: `store` の op と、相乗りが呼ぶ op は、前から `ctx` を取り、`error` を返していた。
- 「分かったこと」の N8〜N13 と N60 は、前の形（純粋さで形を分ける）の記録。その後は、同じ節の「op の中身に立ち入らない（決定 145〜148）」。
- **実装も入った**（基底 `go` と `plugin/golang`）。`TestSamples` は、`check`、`files`、`go` と、六つの `transpilers/*` が、飛ばしも、代役もなく通る: 上の表の三つのファイルは、`hozoc build` の出力と、バイト単位で同じ（手で書いた期待する出力に、直すところはなかった）。

### 残りのケースで変えたこと

意味のある状態には名前を付ける指針（`docs/requirements.md`「書き方の指針」。`docs/decisions.md`「決めたこと」153）と、型で決まる分類の残りに名前を付ける書き方（残りのケース `対象 as else`。`docs/check.md`「残りのケース」。同 152）を、このサンプルに入れた。期待する出力は、生成器を直す前に、手で直した。

| 変えたもの | 変えたこと | 文書 |
|---|---|---|
| `store.hozo` | 三つの分類の `default;` を、残りのケースに: `Registration` は `Unregistered: findUser as else;`、`Account` は `Missing: findUserById as else;`、`Login` は `LoggedOut: findSession as else;` | `docs/check.md`「残りのケース」 |
| `server.hozo` | 分類 `Credential` の `default;` を `Absent: type.Token? as else;` に。四つの `match`（`login`、`authenticate`、`sessionOf`、`getMe`）の `default =>` のアームを、残りのケースの名前のアームに | 同、`docs/check.md`「classify / match」の MATCH-2 |
| `gen/server/inner/wiring.go` | 四つの `match` の、値がない側の枝の頭のコメントが、`// default` から、残りのケースの名前に（`// Unregistered`、`// Absent`、`// LoggedOut`、`// Missing`）。ほかの行は同じ | `docs/plugin-go.md`「match」 |
| `app/inner_test.go` | いないユーザーのログインの説明のコメント一行（「`default` のアーム」を「残りのケース `Unregistered` のアーム」に） | 「人が書いたもの」 |
| `plugins/go-ratelimit/hozo-plugin.toml`、`plugins/go-ratelimit/testdata/**/views.json`、`testdata/seams/plugins/tracer/hozo-plugin.toml` | `protocol` と `View` の版が `"0.7"`（`CaseTyping.by` に `else` が足された） | `docs/ir.md`「版」 |

- この仕様の `default;` は、なくなった: 四つの残りは、どれも意味のある状態（未登録、その識別子のユーザーがいない、ログインしていない、トークンがない）だった。
- `gen/` のほかのファイルは、変わらない: 型で決まる分類について、基底は何も出さず（残りのケースを持っていても同じ）、分類とケースの名前は、配線のコメントにだけ残る。動きも変わらないので、`app/` のテストは、コメントのほかは変えていない。
- **実装も入った**（言語、IR の版 0.7、基底 `go`）。`TestSamples` は、`check`、`files`、`go` と、六つの `transpilers/*` が、飛ばしも、代役もなく通る: `gen/server/inner/wiring.go` は、`hozoc build` の出力と、バイト単位で同じ（手で書いた期待する出力に、直すところはなかった）。

### 分類を閉じたときに変えたこと

分類を閉じる決定（`docs/decisions.md`「決めたこと」154〜162。名前のないケース `default`、`condition`、`default =>` のアーム、ケースの並びの `|` をなくし、`match` に `else` のアームを足す）で、このサンプルが変えたのは、版だけ。

| 変えたもの | 変えたこと | 文書 |
|---|---|---|
| `plugins/go-ratelimit/hozo-plugin.toml`、`plugins/go-ratelimit/testdata/**/views.json`、`testdata/seams/plugins/tracer/hozo-plugin.toml` | `protocol` と `View` の版が `"0.8"`（`Classify.hasDefault` などがなくなり、`MatchArm` に `else` が足された） | `docs/ir.md`「版」 |

- 仕様（`.hozo`）は、変えていない: 四つの分類は、どれも、残りのケースを持つ、型で決まる分類で、なくした書き方（`default;`、`condition`、`default =>`、アームの `|`）を使っていなかった。
- 期待する出力（`gen/**`）と、人が書いたもの（`app/**`）も、変えていない: `docs/plugin-go.md`「match」「分類」で変わったのは、述語で決まる分類の列挙と判定関数、型の `switch`、残りのケースを書かない分類の `match` で、このサンプルは、どれも持たない。その形の期待する出力は、基底 `go` の単体テストの入力（`plugins/go/internal/gobase/testdata/` の `matchswitch`、`matchclosed`）が持つ。
- `TestSamples` は、`check`、`files`、`go` と、六つの `transpilers/*` が、飛ばしも、代役もなく通る: `gen/**` は、`hozoc build` の出力と、バイト単位で同じ。

### 行の遷移で変えたこと

行の遷移（`docs/decisions.md`「決めたこと」163〜173。効果の宛先の後ろの `as` の節と、ケースの `as none`）で、このサンプルが変えたのは、版だけ。

| 変えたもの | 変えたこと | 文書 |
|---|---|---|
| `plugins/go-ratelimit/hozo-plugin.toml`、`plugins/go-ratelimit/testdata/**/views.json`、`testdata/seams/plugins/tracer/hozo-plugin.toml` | `protocol` と `View` の版が `"0.9"`（`Effect.row`、`Classify.absent`、`CaseTyping.by` の `none` が足された） | `docs/ir.md`「版」 |

- 仕様（`.hozo`）は、変えていない: このサンプルには、表の一行が進む段階（注文や支払いの状態）がなく、効果は、どれも `as` を書かない `post` / `put` / `delete`。
- 期待する出力（`gen/**`）と、人が書いたもの（`app/**`）も、変えていない: `docs/plugin-go.md` と `docs/plugin-go-sqlite.md` で変わったのは、行の遷移を持つ効果のコメントと、ない側のケースを持つ、述語で決まる分類の判定関数で、このサンプルは、どちらも持たない（四つの分類は、どれも型で決まり、基底は何も出さない。その値がない側の `as else` のケースは、IR では `absent` に入るが、出力は変わらない）。
- `TestSamples` は、`check`、`files`、`go` と、六つの `transpilers/*` が、飛ばしも、代役もなく通る: `gen/**` は、`hozoc build` の出力と、バイト単位で同じ。

### R6 で変えたこと

R6 の、言語の部分（`docs/requirements.md`「R6 の、言語の部分」。`docs/decisions.md`「決めたこと」294〜344）を、文書とサンプルに、実装より先に入れた。実装は、その後に入り、`TestSamples` は、飛ばしなく、通る: hozoc の出力は、手で書いた、期待する出力と、一致した。

| 変えたもの | 変えたこと | 文書 |
|---|---|---|
| `server.hozo`、`store.hozo` | `const` を `val` に。`match` の頭を、丸括弧で囲み、`}` の後ろの `;` を、省く | `docs/grammar.md`「配線」 |
| `server.hozo` | `getMyPage`（`GET /me/page`）: 名前を並べた、空の `val`。`disjoint`（ブロックのアームが一つ、文のアームが二つ。ブロックのアームの中で、`match` の値を、外の `val` に入れ、もう一方のアームは `throw`）。`disjoint for` と `collect`。葉 `summarize`、`myPage`。model `OrderSummary`、`MyPage` | `docs/check.md`「値の入れ物」「繰り返し」「disjoint」。`docs/decisions.md`「決めたこと」342 |
| `server.hozo` | `resendNotices`（`POST /orders/notices`）: 直列の `for`。中に、例外を出しうる呼び出し（`mailer.send`）と、効果を持つ呼び出し（`store.recordNotice`）と、`collect` | 同上 |
| `store.hozo` | `recordNotice`: 通知の記録を、一つ足す op（効果 `notifyUser` を呼ぶだけの配線） | — |
| `gen/server/inner/wiring.go` | `// match (<対象> by <分類>)` の一行に、丸括弧。`GetMyPage` と `ResendNotices` の本体: 空の `val` は `var` の一行ずつ。`disjoint` は、`// disjoint` の一行と、一つの Go のブロックで、直列。`disjoint for` は、`// disjoint` の一行と `for`。集める文は `append` | `docs/plugin-go.md`「文」「match」 |
| `gen/server/inner/ops.go`、`gen/server/ops.go`、`gen/server/inner/types.go`、`gen/server/types.go` | 足した op と model の分（`Ops`、`Leaves`、`StoreOps`、葉を呼ぶメソッド、型と、その別名） | `docs/plugin-go.md`「実装を受け取る口」「型」 |
| `gen/server/inner/validate.go` | `MyPage` の `Validate`（`user` と、省略可能な `latest`） | `docs/plugin-go-valid.md`「Validate を持つ型」「model」 |
| `gen/server/http.go` | 二つの経路（`GET /me/page`、`POST /orders/notices`） | `docs/plugin-go-http.md` |
| `gen/store/ops.go`、`gen/store/wiring.go`、`gen/storemem/ops.go`、`gen/storemem/wiring.go` | `RecordNotice` | `docs/plugin-go.md`「配線の本体」 |
| `app/leaves.go`、`app/server_test.go`、`app/both_test.go` | 葉 `Summarize`、`MyPage` の本体。`getMyPage` と `resendNotices` のテスト（集まる順、注文がないとき、アームの中の例外で止まること、繰り返しが、失敗した回で止まり、前の回の記録が残ること）。メソッドの一覧 | 「人が書いたもの」 |

- `plugins/go-ratelimit` の、マニフェストと、テストの入力（`testdata/**`）は、変えていない: 版と、`.hozo` の書き換えは、実装と一緒に入れる（`views.json` が、実装の出す IR と、組になっているため）。
- 期待する出力は、手で書いた。`go vet ./...` と `go test ./...` が、このモジュールで通る（サブテスト `go`）。
- **`getMyPage` は、`val` を、六つ持つ**（`session`、`user`、`latest`、`orders`、`found`、`summaries`）。`disjoint` のアームから、外に値を出すには、アームの数だけ、空の `val` が要るので、数が増える。既定の上限（5）を超えて、WIRE-6 の warning になるので、`hozo.toml` の `max_bindings` を 6 にした。

### 例外を投げる効果で変えたこと

効果が、仕様の例外を投げられるようにした（`docs/decisions.md`「決めたこと」352）のと、`db` の語彙の `@db.conflict`（同 353）を、このサンプルで確かめる。期待する出力は、手で書いた。

| 変えたもの | 変えたこと | 文書 |
|---|---|---|
| `type.hozo` | 例外 `EmailTaken`。`@http.status` は、付けない（投げる `store.addUser` は、HTTP の入口から呼ばれない） | `docs/grammar.md`「exception / effect / read」 |
| `store.hozo` | 効果 `createUser` に、`@db.conflict(type.EmailTaken)` と、`throws type.EmailTaken`。それを呼ぶ配線 `addUser` の `throws` に、同じ例外 | `docs/check.md`「read と効果」、`docs/vocab.md`「`db` の read と効果」 |
| `gen/type/types.go` | 例外の型 `EmailTaken` | `docs/plugin-go.md`「exception」 |
| `gen/store/states.go`、`gen/storemem/states.go` | `CreateUser` のコメントに、`throws type.EmailTaken`。メソッドの形は、同じ | `docs/plugin-go.md`「コメント」の「状態の単位のメソッド」 |
| `gen/store/ops.go`、`gen/store/wiring.go`、`gen/storemem/ops.go`、`gen/storemem/wiring.go` | `AddUser` のコメントに、`Throws: type.EmailTaken`。本体は、同じ（効果の失敗を、そのまま返す） | `docs/plugin-go.md`「コメント」 |
| `gen/store/types.go`、`gen/storemem/types.go` | 効果の定数のコメント（説明を変えた分） | — |
| `gen/store/db.go` | `CreateUser` が、`dbExec` を `dbConflict(…, pkgtype.EmailTaken{})` で包む。補助 `dbConflict` と、`strings` の import | `docs/plugin-go-sqlite.md`「効果の中身」「補助」「メソッドの形とコメント」 |
| `app/memstore/memstore.go`、`app/storetest/storetest.go` | 人が書いた `CreateUser` も、同じ鍵か、同じ email で、`type.EmailTaken` を返す。二つの実装に共通のテストが、それを確かめる（前は、「どのエラーかは決まっていない」） | 「人が書いたもの」 |

- 表 `users` の一意の制約は、前から、ある（`id` の `@db.key` と、`email` の `@db.unique`）。DDL は、変えていない。
- `@db.conflict` が当たるのは、その表の、どの一意の制約でも（キーを含む）。なので、同じ識別子の二人めも、`type.EmailTaken` になる。制約ごとに例外を分けるなら、その効果は、人が書く（`docs/plugin-go-sqlite.md`「効果の中身」）。

### 日時の型で変えたこと

日時を、素の型にした（`docs/decisions.md`「決めたこと」359）のを、このサンプルで確かめる。期待する出力は、手で書いた。

| 変えたもの | 変えたこと | 文書 |
|---|---|---|
| `type.hozo`、`server.hozo`、`store.hozo` ほか | 型の名前の書き換え: `Time` を `datetime` に。しまい方の `@db.time` を `@db.datetime` に | `docs/grammar.md`「型」、`docs/vocab.md`「`db` の表と、しまい方」 |
| `type.hozo` | `Order` に、省略可能なフィールドを二つ: `@db.date deliverOn: date?`、`@db.time deliverFrom: time?`。`date` と `time` の Go の形を、確かめるための、いちばん小さな追加（どの op も、まだ、値を入れない） | `docs/grammar.md`「型」 |
| `gen/type/types.go` | `Order` の `DeliverOn *runtime.Date` と `DeliverFrom *runtime.TimeOfDay` | `docs/plugin-go.md`「型」「runtime パッケージ」 |
| `gen/store/db.go` | 表 `orders` の二つの列（`TEXT`。`NOT NULL` なし）。`SELECT` と `INSERT` の列の並び。`dbArgsOrder` と `dbScanOrder` の `dbDate(…, dbNullable)` と `dbTime(…, dbNullable)`。補助の名前 `dbTime` を `dbDatetime` に（`dbDatetimeLayout`）。補助 `dbDate` と、新しい `dbTime` | `docs/plugin-go-sqlite.md`「しまい方」「補助」 |
| `app/onsqlite/store_test.go` | 二つの列が、値がなければ `NULL`、あれば、決まった形の文字列で、読み戻すと、同じ値になること | 「人が書いたもの」 |

### インデックスの model で変えたこと

`Record<K, V>` をなくして、キーが決まっていない構造を、model のインデックスで書くことにした（`docs/decisions.md`「決めたこと」360）。このサンプルの仕様は、`Record` を使っていないので、変わらない。変わったのは、期待する出力の、コメントの一行だけで、手で直した。

| 変えたもの | 変えたこと | 文書 |
|---|---|---|
| `gen/server/http.go` | 補助 `httpWriteResult` のコメントの一行（「値のない record」を、「値のない、インデックスの model（nil のマップ）」に） | `docs/plugin-go-http.md`「補助」 |

### 単体テストの口で変えたこと

R6 の、プラグインの部分の、段 2（`docs/requirements.md`「R6 の、プラグインの部分」）: 基底 `go` が、単体テストの口と、効果の入口の確かめを出す。期待する出力（`gen/server/servertest/`）は、手で書き、hozoc の出力と、バイト単位で同じ。

| 変えたもの | 変えたこと | 文書 |
|---|---|---|
| `gen/server/servertest/doc.go`、`leaves.go` | 足した: `server` の九つの葉の、`<Op>Cases` と `<Op>Case`、まとめの `LeafCases`、走らせる `RunLeaves` | `docs/plugin-go.md`「単体テストの口」 |
| `sample.json`、`gen/server/.hozoc-outputs.json` | 上の二つのファイルを、`go` のファイルと、`gen/server` の記録に足した | `../README.md`「sample.json」 |
| `app/leafcases_test.go` | 足した: 口を満たす型と、`TestLeafCases`、`TestIncompleteCasesAreRejected`（「人が書いたもの」） | — |

- **ほかの `gen/` のファイルは、変わらない。** この仕様には、述語で決まる値の分類も、絞り込んだ引数を持つ効果もないので、判定の口（`classify.go`）も、効果の入口の確かめ（`checked<状態>State`）も、出ない。その二つは、`plugins/go` の単体テストの入力 `unittest` と `rowstate` が確かめる（`docs/testing.md`「単体テスト」）。
- `app` のテストでないファイルが import するものは、変わらない（`TestNoStorageImport`）。テストのパッケージ `servertest` を import するのは、テストのファイルだけ（段 3 で、`app/handlertest` が加わった: 「インテグレーションテストの口で変えたこと」）。

### インテグレーションテストの口で変えたこと

R6 の、プラグインの部分の、段 3（`docs/requirements.md`「R6 の、プラグインの部分」）: `go-http` が、`@http.*` の入口に、結果ごとのインテグレーションテストの口を出す。期待する出力（`gen/server/servertest/http.go`）は、手で書き、hozoc の出力と、バイト単位で同じ。

| 変えたもの | 変えたこと | 文書 |
|---|---|---|
| `gen/server/servertest/http.go` | 足した: 八つの入口の、`<Op>HTTPCases` と `<Op>HTTPCase`、まとめの `HTTPCases`、走らせる `RunHTTP`、補助 | `docs/plugin-go-http.md`「インテグレーションテストの口」 |
| `sample.json`、`gen/server/.hozoc-outputs.json` | 上のファイルを、`go-http` のファイルと、`gen/server` の記録に足した | `../README.md`「sample.json」 |
| `app/handlertest/httpcases.go` | 足した: 口を満たす型 `HTTPCases` と、`RunCases`、`RunIncomplete`（「人が書いたもの」） | — |
| `app/onmem/compose_test.go`、`app/onsqlite/compose_test.go` | 足した: `TestHTTPCases`、`TestIncompleteHTTPCasesAreRejected` | — |

- **ほかの `gen/` のファイルは、変わらない。** 入口のパッケージの `gen/server/http.go` も、同じ。
- 入口ごとの結果（`docs/plugin-go-http.md`「インテグレーションテストの口」の「必要なケース」）:

| 入口 | 結果 |
|---|---|
| `login` | `OK`、`BadRequest`（本文の全体 `@http.bodyObj`）、`InvalidCredentials`（401）、`LimitExceeded`（429。`@rate.limited` が運ぶ例外） |
| `logout`、`getMe`、`getOrders`、`getLatestOrder`、`getMyPage` | `OK`、`Unauthorized`（401）。入力は、ヘッダと、`@http.required` のないクエリだけなので、`BadRequest` は、ない |
| `placeOrder` | `OK`、`BadRequest`（`@http.required` の、本文の項目 `lines`）、`Unauthorized`（401）、`NotificationFailed`（`@http.status` を持たないので、500 と `internal`） |
| `resendNotices` | `OK`、`Unauthorized`（401）、`NotificationFailed`（500 と `internal`） |

- `store` の `export` の op（`addUser` など）は、`@http.*` を持たないので、この口を持たない（`docs/open.md`「語彙とプラグイン」）。
- テストのパッケージ `servertest` を import するのは、テストのファイルと、テストを支えるパッケージ `app/handlertest`（`testing` を import する）だけ。

### R6 のプラグインの部分で足したこと

R6 の、プラグインの部分の、段 4（`docs/requirements.md`「R6 の、プラグインの部分」）: テストのプラグイン `go-design`（`docs/plugin-go-design.md`）が、フローの E2E テストの口と、要求ごとの、テストの入れ物の口を出す。仕様に、担い手と、要求と、操作の op の名前空間と、フローの名前空間を足した。期待する出力は、手で書き、hozoc の出力と、バイト単位で同じ。

| 足したもの | 中身 | 文書 |
|---|---|---|
| `design.hozo` | 名前空間 `design`（単位を置かない）: 担い手の分類 `Actor`（`@actor.human Customer`）と、要求の分類 `Requirement`（`PRD_001`、`PRD_002`）。どちらも、対象のない分類 | `docs/vocab.md`「設計の図の語彙」、`docs/check.md`「分類」の「対象のない分類」 |
| `client.hozo` | 名前空間 `client`: 操作の op（ユースケース）三つ（`signIn`、`order`、`viewOrders`。どれも `@usecase.of(design.Actor.Customer)` と `export`）。どれも、機能入口の op（`server.login`、`server.placeOrder`、`server.getOrders`）を、一つ呼ぶ配線 | `docs/ide-chain.md`「ユースケース図」 |
| `flows.hozo` | 名前空間 `flows`: フロー `signInAndOrder`（`@business.flow` と `export`）と、その結果の model `Ordered`。三つの操作の op を、順に呼ぶ配線 | `docs/ide-chain.md`「業務フロー図」 |
| `server.hozo` | `@prd.ref` を、三つの op に付けた: `authenticate`（`PRD_001`）、`getOrders`（`PRD_002`）、`placeOrder`（`PRD_001` と `PRD_002`）。op の宣言と配線は、変えていない | `docs/vocab.md`「設計の図の語彙」 |
| `hozo.toml` | 単位を三つ足した（6: `http`、7: `client`、8: `flows`）。単位 8 に、`[emit.options.go-design]` の `requirements = ["design.Requirement"]` | `docs/plugin-go-design.md`「オプション」 |
| `gen/http/doc.go`、`types.go` | 標準の語彙 `http` の型（例外三つと、model `Violation`） | `docs/plugin-go.md`「型」 |
| `gen/client/doc.go`、`ops.go`、`wiring.go` | 操作の op の `Ops`、`server` の口 `ServerOps`、`New`、三つの配線の本体 | `docs/plugin-go.md`「実装を受け取る口」「配線の本体」 |
| `gen/flows/doc.go`、`types.go`、`ops.go`、`wiring.go` | model `Ordered`、フローの `Ops`、`client` の口 `ClientOps`、`New`、フローの配線の本体 | 同上 |
| `gen/flows/flowstest/design.go` | `go-design` の出力: `SignInAndOrderFlowCase`、`FlowCases`、`RunFlows`、`Requirements`（`PRD_001`、`PRD_002`）、`RunRequirements` | `docs/plugin-go-design.md`「E2E テストの口」「要求の、テストの入れ物の口」 |
| `sample.json`、`gen/http/.hozoc-outputs.json`、`gen/client/.hozoc-outputs.json`、`gen/flows/.hozoc-outputs.json` | 上のファイルを、`go` と `go-design` のファイルと、出力先ごとの記録に足した | `../README.md`「sample.json」 |
| `app/flowtest/flowtest.go` | 足した: 二つの口を満たす型 `FlowCases`、`Requirements` と、`RunFlows`、`RunRequirements`、`RunIncomplete`（「人が書いたもの」） | — |
| `app/onmem/compose.go`、`app/onsqlite/compose.go` | 足した: `NewFlows`（同じ `store` と `server` の上に、`client` と `flows` を重ねる） | — |
| `app/onmem/compose_test.go`、`app/onsqlite/compose_test.go` | 足した: `TestFlows`、`TestRequirements`、`TestIncompleteFlowCasesAreRejected` | — |

- **今までの `gen/` のファイル（`gen/type`、`gen/rate`、`gen/store`、`gen/storemem`、`gen/server`）は、変わらない。** `@prd.ref` は、基底にも、ほかの相乗りにも、印（何も出ない）。
- **単位 6（`http`）が要るのは、型の置き場のため**（`docs/config.md`「型の置き場」）: 機能入口は `@http.post` / `@http.get` の op で、別の名前空間から呼ぶと、通信の失敗（`http.ServerError`、`http.NetworkError`）が、起こりうる例外に加わる（`docs/check.md`「計算」の、`external` なデコレータの `throws`）。操作の op と、フローは、それを `throws` に書くので、その例外の型の置き場が要る。単位がなければ CFG-22。
- **四つの口**（`docs/requirements.md`「R6 の、プラグインの部分」の条件）: 単体テスト（`gen/server/servertest/leaves.go`。基底）、インテグレーションテスト（`gen/server/servertest/http.go`。`go-http`）、E2E テストと、要求のテスト（`gen/flows/flowstest/design.go`。`go-design`）。`client` は、葉も、入口のデコレータも、フローも持たないので、口を持たない: 操作の op の中身は、基底が作る配線で、つながっていることは、フローの E2E テストが、通して確かめる。
- **フローは、`flows` の単位だけの対象**（テスト用の単位。`docs/ide-chain.md`「業務フロー図」）。サーバーの main（`app/cmd/`）は、`gen/client` も `gen/flows` も import しない。
- **人のテストは、操作の op と、機能入口を、Go の上で、直につなぐ**（`client.New(server.New(…))`）: `server.Ops` は、`client.ServerOps` を、そのまま満たす。HTTP の層は、通らないので、二つの通信の例外は、`throws` に出るが、起きない。HTTP の層は、インテグレーションテストが通す（`docs/open.md`「語彙とプラグイン」に、通信の層を通す E2E の点を置いた）。
- 操作の op は、状態を持たない: ログインのトークンは、`signIn` の戻り値で受け取り、`order` と `viewOrders` の引数で渡す。
- テストのパッケージ `flowstest` を import するのは、テストを支えるパッケージ `app/flowtest`（`testing` を import する）だけ。

### 走ったかの確かめで足したこと

求めるテストが、走ったかを、カバレッジで突き合わせる決定（`docs/decisions.md`「決めたこと」380）で、足したもの。仕様（`.hozo`、`hozo.toml`）と、人が書いたコード（`app/`）は、変えていない。

| 足した、変えたもの | 中身 | 文書 |
|---|---|---|
| `gen/server/servertest/hozo-tests.go.json` | 基底 `go` の、テストの対応表: `RunLeaves` の、11 のテスト（`server.checkPassword/OK` など）と、`leaves.go` の行 | `docs/plugin-go.md`「テストの対応表」 |
| `gen/server/servertest/hozo-tests.go-http.json` | `go-http` の、テストの対応表: `RunHTTP` の、21 のテスト（`server.login/OK` など）と、テストのパッケージの `http.go` の行 | `docs/plugin-go-http.md`「インテグレーションテストの口」の「置き場と名前」 |
| `gen/flows/flowstest/hozo-tests.go-design.json` | `go-design` の、テストの対応表: `RunFlows` の `flows.signInAndOrder` と、`RunRequirements` の `design.Requirement.PRD_001`、`PRD_002` と、`design.go` の行 | `docs/plugin-go-design.md`「出力」 |
| `gen/flows/flowstest/design.go` | `RunRequirements` が、要求の入れ物を、その場の関数で包んで呼ぶ形にした（`t.Run("design.Requirement.PRD_001", func(t *testing.T) { c.PRD_001(t) })`。行は、三つに分かれる）。`RunRequirements` の中に、要求ごとの行を持つため | `docs/plugin-go-design.md`「要求の、テストの入れ物の口」の「口の形」 |
| `gen/server/.hozoc-outputs.json`、`gen/flows/.hozoc-outputs.json`、`sample.json` | 三つの対応表を、出力先ごとの記録と、トランスパイラの `files` に足した。`sample.json` に、`tests`（`skip` と `missing`）を足した | `../README.md`「sample.json」 |

- **対応表は、手で書いた**（ほかの期待する出力と同じ）: 置いてある `leaves.go`、`http.go`、`design.go` の字面から、いちばん内側の `t.Run` の、`{` の次の行と、`})` の前の行を、数えた。`hozoc build` の出力と、バイト単位で同じことは、`transpilers/<名前>` が確かめる。
- **走ったと分かること**は、サブテスト `tests` が確かめる（`../README.md`「テスト」）。手で確かめるなら、このディレクトリで:

```bash
GOTOOLCHAIN=local go test -coverprofile=/tmp/cover.out -coverpkg=./... ./...
```

```bash
(cd ../../.. && go build -o /tmp/hozoc ./cmd/hozoc)
```

```bash
HOZO_PLUGIN_PATH="$PWD/../../../plugins" /tmp/hozoc tests --coverage /tmp/cover.out
```

- **口を渡す一行を、書かなかったとき**: `app/onmem/compose_test.go` と `app/onsqlite/compose_test.go` の `TestFlows`（`flowstest.RunFlows` を呼ぶ一行）を除いて走らせる（`go test -skip '^TestFlows$'`）と、`hozoc tests` は、`go-design: flows.signInAndOrder` が走っていない、と名指しして、終了コード 4 で終わる。`sample.json` の `tests` が、この組。
- `-coverpkg=./...` を付けなければ、テストのパッケージ（`gen/server/servertest`、`gen/flows/flowstest`）の行は、カバレッジに、0 回として載るだけで、35 のテストが、すべて、走っていない、と出る（`docs/plugin-go.md`「カバレッジを読む」）。
- `store`、`storemem`、`type`、`rate`、`http`、`client` の出力は、テストを受け取る口を持たないので、対応表もない。

### 配線の op の `throws` を消したこと

配線の op は、`throws` の節を書かなくてよい（`docs/decisions.md`「決めたこと」379。決まりの持ち主は `docs/check.md`「状態に触れるもの」の「節に書く要素」）。このサンプルの、配線の op から、`throws` の節を、すべて消した。`gen/` と `app/` は、一つも変えていない: op の例外（`Signature.exceptions`）は、どの op も、消す前と同じで、hozoc の出力は、期待する出力と、バイト単位で同じ。

| 変えたもの | `throws` の節を消した op |
|---|---|
| `server.hozo` | 11: `login`、`issueSession`、`authenticate`、`sessionOf`、`logout`、`getMe`、`getOrders`、`getLatestOrder`、`getMyPage`、`placeOrder`、`resendNotices` |
| `client.hozo` | 3: `signIn`、`order`、`viewOrders` |
| `flows.hozo` | 1: `signInAndOrder` |
| `store.hozo` | 1: `addUser` |

- **`throws` を書いているのは、定義する所だけ**: 葉の op 三つ（`server.checkPassword`、`server.requireFresh`、`mailer.send`）と、効果 `store.createUser`。語彙 `rate` のデコレータ `limited` の `throws` は、サンプルの中のプラグインの語彙が持つ（「サンプルの中のプラグイン `go-ratelimit`」）。
- **この README の、消す前の記録は、書き直していない。** 配線の op の `throws` を言う所（「R4 で、まだないもの」の表の `server.placeOrder`、「例外を投げる効果で変えたこと」の表の `addUser`、「下書きから変えたこと」の 18）は、書いたときの仕様の記録。
- **単位 6（`http`）は、今も要る。** 「R6 のプラグインの部分で足したこと」は、理由を「操作の op と、フローは、それを `throws` に書くので」と言うが、型の置き場が要るのは、op の例外（`Op.signature.exceptions`）が使う型（`docs/config.md`「型の置き場」の表の 3）で、節を書くかどうかには、よらない。
- 消した行の分、`server.hozo` の 60 行目から後ろの行番号は、前に詰まった。この README が引く `server.hozo:<行>` は、前から、ずれていて、直していない（`docs/open.md`「サンプルと、hozoc 自身の仕様」の、行番号の点）。

### メモリ版の `store` をなくしたこと

`docs/decisions.md`「決めたこと」388。`store` を、DB なしで出す単位（`gen/storemem`。トランスパイラは `go` だけ）と、その上に人が書いたものを、なくした。`store` の単位は、`gen/store`（`go`、`go-sqlite`）の一つ。

| なくしたもの | 中身だったもの |
|---|---|
| `hozo.toml` の単位 4、`gen/storemem/`（五つのファイルと、記録）、`sample.json` の、その五つ | 同じ `store` を、`go` だけで出した出力（継ぎ目がない） |
| `app/memstore/` | 四つの状態の単位の、メモリの上の実装 |
| `app/onmem/` | メモリの上の組み立てと、そのテスト |
| `app/cmd/shop-mem/` | メモリの上で待ち受ける main |

- **ほかの `gen/` のファイルは、変わらない。**
- **`app/both_test.go` は、`app/ops_test.go` に、名前を変えた。** 残るのは、`TestOpsHoldExportedOpsOnly` と、`gen/store` の `Ops` が満たすインターフェースの確かめ（「人が書いたもの」）。
- **単位の番号が、詰まった**: `server` は 5 から 4、`http` は 6 から 5、`client` は 7 から 6、`flows` は 8 から 7。
- **この節より上（「まだないもの」の下の、どの節も）と、「分かったこと」は、書き直していない。** そこは、書いたときの番号と形で言っている: `gen/storemem`、`app/memstore`、`app/onmem`、`app/cmd/shop-mem`、`app/both_test.go`、単位 4 から 8 の番号、「二つの組み立て」「二つの実装」は、今の形ではない。今の形を言うのは、「入力」から「人が書いたもの」まで。
- **消したテスト（対象が、なくなった）**: `app/onmem/compose_test.go` の九つ。そのうち八つ（`TestHandler`、`TestHTTPCases`、`TestIncompleteHTTPCasesAreRejected`、`TestTwoServers`、`TestStore`、`TestFlows`、`TestRequirements`、`TestIncompleteFlowCasesAreRejected`）は、同じものを、`app/onsqlite` の同じ名前のテストが、走らせている。`TestPlaceOrderIsNotOneTransaction` は、メモリ版だけの事実。`app/both_test.go` の `TestTwoStoresInOneProcess` と `TestStoreOutputsShareBaseFiles` は、二つの出力があることの事実。
- **移したテスト**: `TestStatesFromTwoImplementations`（一つの `store` の状態の単位を、状態ごとに、別の実装から取れる）は、`app/both_test.go` から `app/onsqlite/store_test.go` に移した。メモリの側は、`memstore` の代わりに、テストの中に書いた `memNotifications`（状態 `notifications` の単位の、三つのメソッド）。
- **`store` の配線まで通す、DB なしのテストは、今は、ない。** `app/handlertest`、`app/storetest`、`app/flowtest` は、メモリの上の SQLite で走る（`app/onsqlite/compose_test.go` の `openDB`）。メモリ版の役目（DB なしで、`server` をテストする）は、インテグレーションテストの作り直しで、Hozo が作る、流れの端のにせものが受け持つ（`docs/ide-chain.md`「テストの規約」の「インテグレーションテストの作り直し」。にせものと、その上のテストは、次の節「テストのにせもので足したこと」で、入った）。`server` だけのテスト（`app/server_test.go`、`app/inner_test.go`）は、前から、`store` のどの出力も使わず、`server.StoreOps` を満たす型を、テストが書く。
- **同じ名前空間を、複数の単位に書けること**（`docs/config.md`「単位の対象」）は、変わらない。このサンプルが、その例を持たなくなっただけ（出力の形は、`tests/corpus/units` などが確かめる）。

### テストのにせもので足したこと

`docs/decisions.md`「決めたこと」384、385、390、391。基底 `go` が、出力ごとに、実装を受け取る口のにせものと、にせものの上に、その名前空間の `Ops` を組み立てる `NewOps` を出す（`docs/plugin-go.md`「テストのにせもの」）。

| ファイル | 中身 |
|---|---|
| `gen/store/storetest/doc.go`、`fakes.go` | 足した: 四つの状態の単位のにせもの（`UsersStateFake` など）、継ぎ目のにせもの（`SeamsFake`: `placeOrder` の `@db.transaction`）、`Fakes`、`NewOps` |
| `gen/server/servertest/fakes.go` | 足した: 葉のにせもの（`LeavesFake`。本物を包む）、`mailer` の op のにせもの（`MailerOpsFake`。`mailer` には、単位がない）、継ぎ目のにせもの（`SeamsFake`: `login` の `@rate.limited`）、`Fakes`（`Store` は `storetest.Fakes`）、`NewOps` |
| `gen/client/clienttest/doc.go`、`fakes.go` | 足した: `Fakes` と `NewOps`。`New` の引数は、`server` の op だけ。その op は、どれも、相手の入口（`@http.*`）なので、流れの端で、`Server` は、にせもの `ServerOpsFake`（`docs/plugin-go.md`「テストのにせもの」の「別の名前空間」。作り直しの段 2 で、`servertest.Fakes` を通す形から、変えた） |
| `gen/flows/flowstest/doc.go`、`fakes.go` | 足した: `Fakes`（`Client` は `clienttest.Fakes`）と `NewOps` |
| `gen/server/servertest/doc.go` | パッケージのコメント（単体テストの口と、にせもの、の両方を言う） |
| `sample.json`、四つの `.hozoc-outputs.json` | 足したファイルの分 |
| `app/fakes_test.go` | 足した: にせものの上のテスト（「人が書いたもの」） |

- **`store` の配線まで通す、DB なしのテストが、戻った。** `servertest.NewOps(f, leaves)` が、`server` と `store` の配線（本物）を、流れの端（`store` の `read` と効果、`mailer.send`）と、二つの継ぎ目の、にせものの上に、組み立てる。
- **ほかの `gen/` のファイルは、変わらない。** `type`、`rate`、`http` は、op を持たないので、にせものも、ない。
- **テストの対応表は、変わらない。** にせものは、テストではない。

### インテグレーションテストの作り直しで変えたこと

`docs/decisions.md`「決めたこと」382〜391。`go-http` の口が、にせものの上に、流れを組み立て、例外のケースを、出どころごとに、作れる限り、作る（`docs/plugin-go-http.md`「インテグレーションテストの口」）。

| ファイル | 変えたこと |
|---|---|
| `gen/server/servertest/http.go` | 書き直した: `RunHTTP(t, leaves, c)`（出来上がったハンドラを、受け取らない）。ケースの `Given` は `func(f *Fakes)`、`Then` は、最後に `f *Fakes` を取る。口のメソッドは、人か AI が書くケースだけ。補助 `send` から、`given` をなくした |
| `gen/server/servertest/hozo-tests.go-http.json` | 書き直した: 38 のテスト |
| `app/httpcases_test.go` | 足した: 口を満たす型 `httpCases` と、`TestHTTPCases`、`TestIncompleteHTTPCasesAreRejected`（「人が書いたもの」）。DB は、要らない |
| `app/handlertest/httpcases.go` | なくした（中身は、`app/httpcases_test.go` に、新しい口で、書き直した） |
| `app/onsqlite/compose_test.go` | `TestHTTPCases` と `TestIncompleteHTTPCasesAreRejected` を、`app/httpcases_test.go` に移した |

八つの入口の、38 のテスト。人か AI が書くのは 18、Hozo が作るのは 20。

| 入口 | 人か AI が書くケース | Hozo が作るケース |
|---|---|---|
| `server.login` | `OK`、`BadRequest` | `InvalidCredentials/server.checkPassword`（葉）、`InvalidCredentials/server.login.Unregistered`（`userByEmail` が、「ない」を返す）、`LimitExceeded/server.login@rate.limited`（継ぎ目） |
| `server.logout`、`server.getOrders`、`server.getLatestOrder` | `OK`、`Unauthorized/server.authenticate.Absent` | `Unauthorized/server.requireFresh`（葉）、`Unauthorized/server.sessionOf.LoggedOut`（`sessionByToken` が、「ない」を返す） |
| `server.getMe`、`server.getMyPage` | 同じ | 同じ二つと、`Unauthorized/server.getMe.Missing`（`getMyPage` は `server.getMyPage.Missing`。`userById` が、「ない」を返す） |
| `server.placeOrder` | `OK`、`BadRequest`、`Unauthorized/server.authenticate.Absent` | 同じ二つと、`NotificationFailed/mailer.send`（外の仕組みの op） |
| `server.resendNotices` | `OK`、`Unauthorized/server.authenticate.Absent`、`NotificationFailed/mailer.send` | 同じ二つ |

- **`server.authenticate.Absent` を、人が書く**のは、アームの決め手（`token`）が、入口の入力（`in.token`）だから。口のメソッドは `UnauthorizedAuthenticateAbsent`。
- **`server.resendNotices` の `mailer.send` を、人が書く**のは、繰り返し（`for`）の中にあるから。口のメソッドは `NotificationFailedSend`。
- **ほかの `gen/` のファイルは、変わらない。**
- **この節より上の、インテグレーションテストの口の記述**（「インテグレーションテストの口で変えたこと」「走ったかの確かめで足したこと」の、`RunHTTP(t, handler, …)`、21 のテスト、`app/handlertest/httpcases.go`）は、書いたときの形で、今の形ではない。

### 本体のあるデコレータと、デコレータのテストで足したこと

`docs/decisions.md`「決めたこと」392〜395。仕様の書き方（`decorator fn`）の持ち主は `docs/grammar.md`「decorator の宣言」「デコレータの本体」、出力の形の持ち主は `docs/plugin-go.md`「本体のあるデコレータ」「デコレータのテストの口」。

| ファイル | 変えたこと | 持ち主 |
|---|---|---|
| `server.hozo`、`plugins/go-ratelimit/rate.hozo` | op に付けるデコレータの宣言を、`decorator fn` に（`decorator fn limited(): throws LimitExceeded prevents effects;`）。足した: 本体のあるデコレータ `server.audited`（`proceed(); recordAudit();`）と、それが呼ぶ葉 `server.recordAudit`。`logout` に `@audited` を付けた | `docs/grammar.md`「decorator の宣言」「デコレータの本体」 |
| `gen/server/inner/ops.go`、`wiring.go` | `Logout` が二層になった: 外側 `Logout` が `o.logoutServerAudited` を呼び、それ（デコレータの本体から作った外側）が、中身 `o.logoutBody` と、葉 `o.RecordAudit` を、順に呼ぶ。`Leaves` と、内側の `Ops` に、`RecordAudit` | `docs/plugin-go.md`「本体のあるデコレータ」 |
| `gen/server/servertest/leaves.go`、`fakes.go` | 葉 `recordAudit` の口（`RecordAuditCases`、`RecordAuditCase`）と、にせもの（`LeavesFake.RecordAudit`） | 同「単体テストの口」「テストのにせもの」 |
| `gen/server/servertest/seams.go`、`gen/store/storetest/seams.go` | 足した: デコレータのテストの口。`server` は `rate.limited`（結果は `OK` と `LimitExceeded`）、`store` は `db.transaction`（結果は `OK` だけ: 宣言が `throws` を持たない） | 同「デコレータのテストの口」 |
| `gen/server/servertest/hozo-tests.go.json`、`gen/store/storetest/hozo-tests.go.json` | テストの対応表に、`server.recordAudit/OK`、`server.login@rate.limited/OK`、`server.login@rate.limited/LimitExceeded`、`store.placeOrder@db.transaction/OK`。`store` の対応表は、新しいファイル | 同「テストの対応表」 |
| `app/leaves.go`、`app/leafcases_test.go` | 葉 `RecordAudit`（記録の先 `Audit` を、一度呼ぶ）と、そのケース。葉は、十になった | — |
| `app/seamcases_test.go`、`app/onsqlite/store_test.go` | 足した: デコレータのテストのケース。`@rate.limited` は、数え方 `Limiter` の、まだ呼べる状態と、もう呼べない状態。`@db.transaction` は、`go-sqlite` の `store.NewDbSeams(db)` | `docs/ide-chain.md`「テストの規約」の「デコレータのテスト」 |
| `app/httpcases_test.go` | `logout` の正常のケースが、葉 `recordAudit` が、一度、呼ばれたことも確かめる（本体のあるデコレータは、インテグレーションテストで、本物を通す） | 同「インテグレーションテストの作り直し」の「継ぎ目に置くもの」 |

- **`@rate.limited` は、本体のないまま**: プラグイン `go-ratelimit` が、継ぎ目として包む例。`@server.audited` は、本体のある例で、プラグインは、要らない。
- **インテグレーションテストの、Hozo が作るケースは、変わらない**: `audited` の本体は、例外を投げない（葉 `recordAudit` は、`throws` を持たない）。

### 型のパッケージで変えたこと

`docs/decisions.md`「決めたこと」402（`docs/plugin-go.md`「型のパッケージ」）。関数を持つ名前空間（`store`、`server`、`client`、`flows`）の出力は、型を、`types/` のパッケージに置く。型だけの名前空間（`type`、`rate`、`http`）の出力は、変わらない。

| ファイル | 変えたこと |
|---|---|
| `gen/store/types/types.go`、`gen/server/types/types.go`、`gen/flows/types/types.go` | 型の定義（前の `gen/store/types.go`、`gen/server/inner/types.go`、`gen/flows/types.go` を、そのまま移した。`package types`） |
| `gen/store/types/doc.go`、`gen/server/types/doc.go`、`gen/flows/types/doc.go` | 型のパッケージのコメント |
| `gen/store/types.go`、`gen/server/inner/types.go`、`gen/flows/types.go` | 型のパッケージの再公開（別名と、効果の定数） |
| `gen/server/types/validate.go` | 前の `gen/server/inner/validate.go`（`go-valid` の置き場が、型のパッケージ） |
| `gen/client/ops.go`、`gen/client/wiring.go` | `server` の型の import が、`server "example.com/shop-server/gen/server/types"` |

- **人が書いたコード（`app/`）は、変えていない。** 入口のパッケージの名前（`server.LoginInput`、`store.EffectCreateOrder`）は、同じ型を指す。
- `client` は、自分の型を持たないので、型のパッケージも、ない。
- 下の節と「分かったこと」が、`gen/server/inner/types.go`（型の定義）、`gen/server/inner/validate.go` と書いている所は、この節より前の形。

### 例外を受ける文で足したこと

`docs/decisions.md`「決めたこと」419、422（`docs/check.md`「例外を受ける」、`docs/plugin-go.md`「例外を受ける文」）。`resendNotices` の、配送（`mailer.send`）を、例外を受ける文で囲んだ: 受けて、葉 `recordAudit` で記録して、`throw e` で、そのまま投げ直す。

| ファイル | 足したこと |
|---|---|
| `server.hozo` | `resendNotices` の `for` の中の `try { mailer.send(notice); } catch (e) { recordAudit(); throw e; }`。op の説明 |
| `gen/server/inner/wiring.go` | `ResendNotices` の本体の、`// try` から `}` まで（手で書いた、期待する出力）。import に `errors` と runtime パッケージ |
| `gen/server/ops.go`、`gen/server/inner/ops.go` | `ResendNotices` のコメント（op の説明） |
| `app/server_test.go` | `TestResendNoticesCatch`: 仕様の例外は、受けて、記録して、そのまま投げ直す。仕様にない失敗は、受けない。失敗しなければ、`catch` は動かない |
| `app/httpcases_test.go` | `NotificationFailedSend` のケースに、`recordAudit` が、一度呼ばれることの確かめ |

- **op の関数の型と、テストの口は、変わらない。** `throw e` は、`try` の中の出どころ（`mailer.send`）を、そのまま出どころにする（`docs/ide-chain.md`「インテグレーションテストの作り直し」の「Hozo が作る、例外のケース」）ので、口のメソッドは `NotificationFailedSend` のまま。計算した例外も、効果も、同じ。
- 分類して受ける形（`catch (e by 分類)`）と、例外の分類の例は、`tests/samples/order-app` の `payment.prepay`、`notice.bestEffort`。

## 入力

仕様は名前空間ごとに一つのファイル。

| ファイル | 名前空間 | 中身 |
|---|---|---|
| `type.hozo` | `type` | 共有の型（P6）: scalar、model、例外。`model PublicUser extends User { override hash: never; }`（P24〜P26）。表の model（`User`、`Session`、`Order`、`Notification`）のフィールドは、しまい方のデコレータ（`@db.text`、`@db.datetime`、`@db.json`）を一つずつ持つ |
| `store.hozo` | `store` | DB の状態四つ（P5。どれも `@db.table`）と、それに触れる五つの `read`（`@db.find` が四つ、うち一つは `@db.latest("placedAt")` 付き。`@db.list` が一つ）と五つの効果（`@db.insert` が四つ、`@db.delete` が一つ）。十の op（どれも `export`）は、`read` か効果を呼ぶだけの配線で、`placeOrder` は、二つの効果を呼ぶ配線に `@db.transaction`。読んだ結果の分類（`classify C of <op>`、`Case: <op> as T;`） |
| `server.hozo` | `server` | HTTP の入口（`export` を付けた八つ）と、業務の流れ（配線）。人が書く op は九つ。`getMyPage` は、`disjoint` と `disjoint for`、`resendNotices` は、直列の `for` と `collect` を使う。入力の model のフィールドは、要求のどこから読むかを `@http.header("Authorization")` / `@http.query` / `@http.body` / `@http.bodyObj` で、項目がないときの扱いを `@http.required`（`lines`）/ `@http.default(1)`（`page`）で言う。`login` に `@rate.limited`（継ぎ目。P38）。トークンのない要求を断る分類 `Credential` |
| `mailer.hozo` | `mailer` | 通知を外へ届ける仕組み。`@smtp.sendMail` を付けた op `send`（`export`） |
| `smtp.hozo` | `smtp` | メールを SMTP で送る仕組みの語彙。デコレータ `sendMail`。読むトランスパイラがないので、印（何も出ない。P37） |
| `plugins/go-ratelimit/rate.hozo` | `rate` | サンプルの中のプラグイン `go-ratelimit` が提供する語彙: 例外 `LimitExceeded` と、デコレータ `limited`（「サンプルの中のプラグイン」） |
| `design.hozo` | `design` | 設計の図が持つ名前: 担い手の分類 `Actor` と、要求の分類 `Requirement`（どちらも、対象のない分類）。単位を置かない（「R6 のプラグインの部分で足したこと」） |
| `client.hozo` | `client` | 操作の op（ユースケース）三つ。機能入口の op を呼ぶ配線（同） |
| `flows.hozo` | `flows` | フロー一つと、その結果の model。操作の op を、順に呼ぶ配線（同） |

`hozo.toml` は、`[hozo]` の `plugin_dirs = ["plugins"]`（サンプルの中のプラグインの置き場）と、プロジェクト全体のオプション `[options.go]` の `module = "example.com/shop-server"`（Go のモジュールの名前。`go.mod` の `module` と同じもの。`docs/plugin-go.md`「マニフェスト」）と、七つの単位。`mailer`、`smtp`、`design` には単位を置かないので、コードは出ない。

| # | 単位の `out` | `namespace` | `transpilers` | 中身を埋める者 |
|---|---|---|---|---|
| 1 | `gen/type` | `type` | `go`、`go-valid` | —（型だけ） |
| 2 | `gen/rate` | `rate` | `go` | —（語彙 `rate` の例外の型だけ） |
| 3 | `gen/store` | `store` | `go`、`go-sqlite` | `go-sqlite`: 四つの状態の単位の実装（五つの `read`、五つの効果、状態の形で決まる問い合わせのすべて）と、`@db.transaction` の継ぎ目の実装。op の中身（配線）は、基底 |
| 4 | `gen/server` | `server` | `go`、`go-valid`、`go-http`、`go-ratelimit` | 人: `app/leaves.go`（葉の op 十）、`app/mailer.go`（`mailer.send`）、`app/limiter.go`（`@rate.limited` の数え方）。`go-ratelimit`: 継ぎ目の中身 |
| 5 | `gen/http` | `http` | `go` | —（標準の語彙 `http` の型だけ） |
| 6 | `gen/client` | `client` | `go` | —（三つの op は、どれも配線。中身は、基底） |
| 7 | `gen/flows` | `flows` | `go`、`go-design` | 人: `app/flowtest`（フローのケースと、要求ごとの入れ物の中身）。フローの配線は、基底。口と、走らせる関数は、`go-design` |

- 八つの単位は、どれも基底 `go` を一つ持ち、相乗りの依存（`go-http` は `go` と `go-valid`、`go-ratelimit` と `go-design` は `go`）をすべて並べている（`docs/config.md`「基底と相乗り」）。
- 使う型の名前空間は、どれも `go` の単位を持つ（同「型の置き場」）: `type` は単位 1、`rate` は単位 2、`http` は単位 5、`server`、`client`、`flows` は、自分の単位。**`rate` は語彙の名前空間**で、その例外 `rate.LimitExceeded` を `server` の出力が使う（`login` の `Op.signature.exceptions` にある。同「型の置き場」の表の 3）ので、単位が要る。単位 2 がなければ CFG-22。
- `gen/server` の単位だけ、`[emit.options.go-http]` で `validate = true` を渡す（P16）。
- **単位の継ぎ目**（`docs/config.md`「継ぎ目」）: 単位 3 は `db.transaction` → `go-sqlite`、単位 4 は `rate.limited` → `go-ratelimit`（それぞれのマニフェストの `seams`）。ほかの単位の継ぎ目は空。

### `export` を付けた op

別の名前空間から呼べる op には `export` を付ける（P33）。書く位置は、説明のコメントとデコレータの後ろ、`op` のすぐ前（`server.hozo:43-45` の `@http.post("/login")`、`@rate.limited`、`export fn login(…)`）。付けていない op は内部で、別の名前空間から呼べばエラー。対象は op だけで、型、例外、分類、効果、デコレータは、今までどおりどこからでも参照できる（`server` は `store.Registration` や `type.Unauthorized` を、そのまま書く）。

この仕様の三十一の op のうち、`export` を付けたのは十九。付けたのは、**その名前空間の外に、呼ぶ者がいる op だけ**。

| 名前空間 | `export` を付けた op | 外の呼び手 | 内部の op |
|---|---|---|---|
| `server` | `login`、`logout`、`getMe`、`getOrders`、`getLatestOrder`、`getMyPage`、`placeOrder`、`resendNotices`（八つ） | `go-http` のハンドラ（`@http.*` の入口）。仕様の中に、呼ぶ名前空間はない | 配線 `issueSession`、`authenticate`、`sessionOf` と、人が書く葉 `checkPassword`、`newSession`、`toPublicUser`、`requireFresh`、`pageOf`、`summarize`、`myPage`、`newOrder`、`composeNotice`（十二） |
| `store` | `findUser`、`findUserById`、`findSession`、`listOrders`、`latestOrder`、`addSession`、`removeSession`、`placeOrder`、`recordNotice`（九つ） | `server` の配線 | なし（`read` と効果は op ではなく、`export` を付けられない。使えるのは `store` の配線だけ） |
| `store` | `addUser` | 人が書くコード（二つの main と、テスト）。仕様の中に、呼ぶ名前空間はない | |
| `mailer` | `send` | `server` の配線 `placeOrder` | なし |

迷ったのは `store.addUser` だけ。仕様のどの名前空間も呼ばないので、「別の名前空間から呼べる」という P33 の言い方だけなら、内部でも検査は通る。`export` にしたのは、人が書くコード（起動のときにユーザーを入れる main、テスト）が、`store.New` の返す `Ops` を通して呼ぶため: `Ops` に並ぶのは `export` を付けた op だけなので、付けなければ `store.Ops` から消える。内部にしても、状態の単位の実装（`store.NewUsersTable(db).CreateUser`）を直に呼べば足せるが、それでは main とテストが、保存先の型に結びつく（v1 の「分かったこと」26 の「保存先を直に書き換える」に近い形に戻る）。`export` の意味が「仕様の別の名前空間」だけでないことは、「分かったこと」N66。

### 下書きから変えたこと

仕様の元は、`ac57788` までこのディレクトリにあった下書き `NEXT.md`（P1〜P19 を当てはめたもの）。そこから変えたのは次の二十五。9〜11 は、`docs/decisions.md`「決めたこと」91〜102 を、12〜15 は、同 103〜113 を、16 と 17 は、同 114〜122 を、18 は、同 124 を、20〜24 は、同 132〜143 を当てはめたもの。25 は、同 145〜148 を当てはめたもので、仕様は変えていない。**9、10、12、19 が書いた `store` の形（op に付ける `@db.find` / `@db.reads` / `@db.effects`、引数を持つ `@db.table`）は、20〜23 が置き換えた**: その四つの項目と、1〜18 が引く行番号は、その項目を書いたときのもの。

1. **継承**（P24〜P27）: `model PublicUser is User omit hash;` を `model PublicUser extends User { override hash: never; }` にした（`type.hozo:42-44`）。`type.User` を `type.PublicUser` に写す葉の op `toPublicUser` を `server` に足し（`server.hozo:74`）、下書きが `User` をそのまま渡していた二か所で呼ぶ（`server.hozo:62`、`:126`）。
2. **外部の入出力**（P30）: `server` の葉 `deliver` をやめ、名前空間 `mailer` の `op send` にし、送る仕組みの語彙のデコレータ `@smtp.sendMail` を付けた（`smtp.hozo:7`、`mailer.hozo:6-9`、`server.hozo:168`）。外部の入出力をする op は、その仕組みの語彙のデコレータを持つ形に書くのが望ましい（外部のサービスなら、そのサービスの語彙。規則ではなく書き方の指針で、検査はしない）。デコレータを持つ op は純粋でない。
3. **`store.addUser` と効果 `createUser` を足した**（`store.hozo:80-81`、`:102-103`）。下書きにない変更。ユーザーを足す op がなければ、起動のときとテストが、保存先を直に書き換えるしかない（v1 の「分かったこと」26）。
4. **`getMe` の `Found` のアームは、呼び出しをそのまま書いた**（`server.hozo:126` の `Found => toPublicUser(user);`）。写した値を先に束縛する形にはできない: `user` が `type.User` になるのはアームの中だけ。
5. **セッションの期限を確かめる**: 下書きは `expiresAt` を書くだけで、誰も読まなかった（期限の切れたトークンで入れた。「分かったこと」N3）。人が書く葉の op `requireFresh` を `server` に足し（`server.hozo:106-107`）、`LoggedIn` のアームを `LoggedIn => requireFresh(session);` にした（`server.hozo:100`。今は配線 `sessionOf` の中。下の 11）。期限が過ぎていれば `type.Unauthorized`。下書きのアームは値のアーム（P14 の `LoggedIn => session;`）だったので、この仕様に値のアームはなくなった。
6. 説明のコメントを足した。
7. **`export`**（P33）: 外に呼び手のいる op に `export` を付けた（上の「`export` を付けた op」）。下書きには、op を内部にする書き方がなかった。
8. **継ぎ目**（P36〜P38）: `login` に `@rate.limited` を付けた（`server.hozo:44`）。語彙 `rate` と、その継ぎ目を宣言するトランスパイラは、サンプルの中のプラグイン `go-ratelimit` が提供する（下の「サンプルの中のプラグイン」）。`login` の `throws` には `rate.LimitExceeded` を書いていない: デコレータの `throws` が足す例外は、op が書かなくても `Signature.exceptions` に入る（`docs/check.md`「計算」の手順 i）。`login` を呼ぶ仕様の名前空間があれば、そこは書くことになる（THROWS-1）。
9. **読み書きを、`db` の語彙のデコレータで言う**（決定 92〜95）: `store` の読む op の `reads <状態>` を `@db.find(<状態>, "<列>")` / `@db.list(<状態>, "<列>")` に、書く op の `effects <効果>` を `@db.effects(<効果>)` に置き換え、五つの効果に `@db.insert(<状態>)` / `@db.delete(<状態>, "<列>")` を付けた（`store.hozo`）。op は `reads` も `effects` も書かない（デコレータが運ぶ）。引数と列を、型が合うかどうかで結ぶ規則（N39）は、なくなった: 列は、仕様が名指しする。
10. **二つの効果を、一つのトランザクションで**（決定 94）: `store.placeOrder(order, notification)` を足した（`@db.effects(createOrder, notifyUser)`。`store.hozo:114-115`）。`server.placeOrder` は、注文と通知の文面を作ってから、`store.placeOrder` で二つを一緒に記録し、その後で `mailer.send` で配送する（`server.hozo:166-168`）。前は、`store.addOrder`、`mailer.send`、`store.addNotification` を順に呼んでいて、配送に失敗すると、注文だけが残り、通知の記録は残らなかった。今は、配送に失敗しても、注文と通知の記録の両方が残る（HTTP の応答は、前と同じ 500）。**なくした op**: `store.addOrder`、`store.addNotification`（呼ぶ者が、仕様にも、人が書くコードにも、いなくなった。テストは、注文と通知を `placeOrder` で足す。`export` を付けるのは外に呼ぶ者がいる op だけ、という上の「`export` を付けた op」の決まりのとおり）と、`server.notify`（中身が `mailer.send` の呼び出し一つになった）。効果 `createOrder` と `notifyUser` は残る。
11. **トークンのない要求は、`server` が断る**（決定 96）: `store.findSession` の引数を `type.Token?` から `type.Token` にした（列 `token` の型と等しい。列より広い引数は GOSQL-9）。「トークンがなければ、問い合わせずに『ない』」は、`go-sqlite` の規則ではなくなったので、仕様に書く: `server` に、値の分類 `classify Credential of type.Token? { Presented: type.Token? as type.Token; Absent: type.Token? as else; }` を足し（型で決まる分類。`server.hozo:79-82`。足したときは、`Absent` の行は `default;` だった。「残りのケースで変えたこと」）、配線 `authenticate(token: type.Token?)` は `match token by Credential` で分け、`Presented` のアームで、新しい内部の配線 `sessionOf(token: type.Token)` を呼び、`default` のアームで `throw type.Unauthorized`（`server.hozo:85-92`）。`sessionOf` が、前の `authenticate` の中身（`store.findSession` と、`store.Login` での分岐）を持つ（`server.hozo:95-103`）。HTTP から見た振る舞いは変わらない（トークンのない要求は 401 `type.Unauthorized`）。変わったのは、そのとき `store` が呼ばれないこと。

12. **表は `@db.table` で言う**（決定 104）: 四つの状態の `@db` を `@db.table(type.<要素の model>)` にした（`store.hozo:9-19`）。状態の型に書いてある model を、デコレータの引数でもう一度書く: 表の列を決めるのは、デコレータの引数だけ。
13. **列のしまい方を、フィールドに書く**（決定 105）: 四つの表の model の十五のフィールドに、`@db.text`（十）、`@db.datetime`（二: `expiresAt`、`placedAt`）、`@db.date`（一: `deliverOn`）、`@db.time`（一: `deliverFrom`）、`@db.json`（一: `lines`）を付けた（`type.hozo`）。`@db.key` / `@db.unique` / `@db.ref` は、そのまま並べて書く。表にならない `OrderLine` には付けない（`lines` の列の中の JSON）。
14. **要求のどこから読むか**（決定 107）: `LoginInput` の `@http.body body: Credentials` を `@http.bodyObj` にした（本文が、その model そのもの。通信の形は変わらない）。`PlaceOrderInput` の `@http.body lines: type.OrderLine[]` は、書き方はそのままで、意味が変わった: 項目の形なので、**本文は、配列そのものから、`{"lines":[…]}` に変わった**（違反の場所は `body[1].quantity` から `body.lines[1].quantity` に）。配列そのものを本文にする書き方は、なくなった（全体の形は、model だけ）。`@http.header(name: "Authorization")` は、名前を位置で渡す `@http.header("Authorization")` に書き直した（意味は同じ）。`@http.query page` は、名前を省いた項目の形（名前は、フィールドの名前 `page`）で、書き方も意味も変わらない（`server.hozo:15-38`）。
15. **省略可能な戻り値の経路を足した**（決定 108）: `@http.get("/orders/latest") export fn getLatestOrder(in: AuthInput): type.Order?` と、人が書く葉 `latestOf(orders: type.Order[]): type.Order?`（`server.hozo:146-157`）。注文がなければ 204 で本文なし、あれば 200 と、その注文。成功の応答を、op の型でなく、実行時の値で決めること（`docs/plugin-http.md`「通信の形」の「成功の応答」）を、期待する出力とテストで確かめるためだけに足した。
16. **なければ誤りの項目に、`@http.required` を付けた**（決定 114）: `PlaceOrderInput` の `@http.required @http.body lines: type.OrderLine[]`（`server.hozo:37`）。**振る舞いが一つ変わった**: `lines` のない `POST /orders`（本文が `{}`、または、ほかのキーだけ）は、前は、空の注文を作って 200 を返していた（本文の項目は、なくても誤りにしなかった）。今は、op を呼ばずに 400 `body lines: missing`。付けたのは、この一つ。ほかの項目の形のフィールド（三つの入力の `token` と、`page`）は、前も、なくても誤りにならなかった（`token` は省略可能で、`page` は既定値を持っていた）ので、付けていない。`LoginInput` の `body` は全体の形なので、付けられない（GOHTTP-14）。
17. **既定値を、型の宣言から、境界のデコレータに移した**（決定 S24、115）: `GetOrdersInput` の `@http.query page: int = 1` を `@http.query @http.default(1) page: int` にした（`server.hozo:31`）。`page` のない `GET /orders` が 1 ページ目であることは、変わらない。変わったのは、1 を入れる者: 前は、基底の `NewGetOrdersInput()`（型を作る誰もが入れる）、今は、要求を読むハンドラ（`req.query("page", &in.Page, httpDefault(1))`）。`server.GetOrdersInput{}` を自分で作るコード（op を直に呼ぶテストなど）の `Page` は、0 になる。この仕様のほかのフィールドに、既定値はなかった。

18. **節の区切りを `,` にした**（決定 124）: `server.placeOrder` の `throws type.Unauthorized | type.NotificationFailed` を `throws type.Unauthorized, type.NotificationFailed` にした（`server.hozo:162`）。この仕様で、一つの節に二つ以上を並べているのは、ここだけ（語彙 `rate` の `limited` の `throws` は一つ）。意味も、期待する出力も、変わらない。
19. **読む側を、書く側と同じ二段にした**（決定 125、128）: 四つの `read`（`userByEmail`、`userById`、`sessionByToken`、`ordersOfUser`）を宣言し、9 で読む op に付けた `@db.find` / `@db.list` を、そちらに移した（`store.hozo:23-37`）。四つの読む op には、行う `read` を名指しする `@db.reads(<read>)` を付けた（`store.hozo:41-55`）。op の引数と戻り値、読む状態（`Reads: store.users` など）、op の置き場、SQL は、変わらない。変わったのは、`gen/store/db.go` の四つのコメントの行（op のデコレータが `@db.reads` になり、`read` の名前が入る）。

20. **状態に触れるのは、`read` と効果だけにした**（決定 133、135、137）: `store` の四つの `read` に、引数と結果の型を書き（`read fn userByEmail(email: type.Email): type.User? reads users;`）、五つの効果に、引数を書いた（`effect fn createUser(user: type.User): effects users.post;`）。`db` の語彙のデコレータは、引数がなくなった（`@db.find`、`@db.list`、`@db.insert`、`@db.delete`）: 表は、`reads` の状態か、効果の宛先の状態、列は引数の名前。op に付けていた `@db.reads` と `@db.effects` は、なくなり、**八つの op は、`read` か効果を一つ呼ぶだけの配線になった**（`export fn findUser(email: type.Email): type.User? { return userByEmail(email); }`）。op の引数と戻り値、`Reads:` / `Effects:` は、変わらない。変わったのは、中身のある場所: 状態の単位のメソッドが、op（`FindUser`、`AddUser`）から、`read` と効果（`UserByEmail`、`CreateUser`）になり、op は、基底が出す配線（`gen/store/wiring.go`）になった。
21. **表は、状態の型から**（決定 134）: 四つの状態の `@db.table(type.<Model>)` を、引数のない `@db.table` にした。表の model は、状態の要素の型。
22. **二つの効果を、一つのトランザクションで**（決定 138）: `store.placeOrder` は、`@db.effects(createOrder, notifyUser)` を付けた葉から、二つの効果を順に呼ぶ配線に `@db.transaction` を付けたものになった。SQLite の上の振る舞い（どちらも足すか、どちらも足さない）は、変わらない。**メモリの上では、変わった**: 単位 4 には `@db.transaction` を読むトランスパイラがいないので、生成された配線は、二つの効果を順に呼ぶだけで、二つ目が失敗しても、一つ目は残る。前は、人が書いた `placeOrder` が、一度の lock の中で、両方を守っていた。このメモリ版（単位 4 と `app/memstore`）は、後で、なくした（「メモリ版の `store` をなくしたこと」）。
23. **いちばん新しい注文を、`read` が引く**（決定 136）: `read fn latestOrderOf(userId: type.UserId): type.Order? reads orders;` に `@db.find` と `@db.latest("placedAt")` を付け、それを呼ぶ op `store.latestOrder` を足した。`server.getLatestOrder` は、`store.listOrders` と、人が書く葉 `latestOf` の代わりに、`store.latestOrder` を呼ぶ。**なくした op**: `server.latestOf`。**振る舞いが一つ変わった**: 「一覧の最後（最後に足した注文）」から、「`placedAt` がいちばん新しい注文（同じ時刻なら、後から足した方）」に。時刻の順に足している限り、同じ注文を返す。
24. **`Leaves` の説明**（決定 139）: 葉の op は、状態に触れないものだけになったので、`server.Leaves` のコメントの一行目が変わった（`gen/server/inner/ops.go`、`gen/server/ops.go`）。`server` の葉は、八つから七つに。
25. **どの関数も `ctx` を取り、`error` を返す**（決定 145〜147）: 仕様（`.hozo`）は変えていない。`server` の七つの葉のメソッドに、`ctx` と `error` が付き、それを呼ぶ配線の行が変わった（「R5 で変えたこと」）。「分かったこと」N8〜N13、N60 の、純粋さで形が分かれる話は、この前の形の記録。

## サンプルの中のプラグイン `go-ratelimit`

継ぎ目（`docs/config.md`「継ぎ目」、Go での形は `docs/plugin-go.md`「継ぎ目」）の、いちばん小さな例。プロジェクトが、自分のデコレータに継ぎ目を持たせるには、それを宣言するプラグインをプロジェクトの中に置く。このサンプルでは `plugins/go-ratelimit/` で、`hozo.toml` の `plugin_dirs = ["plugins"]` が、そこを探索の場所にする（`docs/plugin-protocol.md`「探索」の 1）。**このプラグインの規則の持ち主は、この節**（リポジトリの `docs/` には、文書を持たない）。

| ファイル | 中身 | 今 |
|---|---|---|
| `plugins/go-ratelimit/hozo-plugin.toml` | マニフェスト: 語彙 `rate.hozo` と、トランスパイラ `go-ratelimit`（`language = "go"`、`requires = ["go"]`、`seams = ["rate.limited"]`） | ある |
| `plugins/go-ratelimit/rate.hozo` | 語彙 `rate`: 例外 `LimitExceeded`（`@http.status(429)`）と、デコレータ `decorator fn limited(): throws LimitExceeded prevents effects` | ある |
| `plugins/go-ratelimit/main.go`、`emit.go` | トランスパイラの実装（下の「実装」） | ある |
| `plugins/go-ratelimit/emit_test.go`、`testdata/` | 実装の単体テストと、その入力・ゴールデン（下の「実装」） | ある |
| `gen/server/ratelimit.go` | 期待する出力 | ある |

### 語彙と、型の置き場

- **デコレータ `rate.limited`**: 付いた op を呼べる頻度を制限する。制限を超えた呼び出しは、op の中身を呼ばずに `rate.LimitExceeded`。引数はない（何回まで、どの間隔で、を仕様には書かない。数え方は、実行のときに人が渡す）。`outer throws` ではない（前の `@external`）: 同じ名前空間の配線から呼んでも、例外は返る。`prevents effects` は、制限を超えたときに中身を呼ばないこと。
- **例外 `rate.LimitExceeded`** は、語彙の名前空間 `rate` に置いた（`http.BadRequest`、`proc.Crashed` と同じく、仕組みの語彙が、その失敗の例外を持つ）。`@http.status(429)` は、標準の語彙 `http` のデコレータで、`go-http` が読む（材料）。Go の型は、`rate` の単位（単位 2。`gen/rate/types.go`）の基底が出し、`gen/server` の `http.go` と `ratelimit.go` は、それを import する。プロジェクトの型の名前空間（`type`）に置いて、デコレータの引数で渡す形（`@rate.limited(type.TooManyRequests)`）も書けるが、語彙だけで閉じる方を取った。
- 語彙のファイルは `plugin_dirs` の下にあるので、仕様としては読まれず（`sources` から除かれる）、`go-ratelimit` が使うプラグインである（単位 4 が、そのトランスパイラを書いている）ので、語彙として読まれる（`docs/vocab.md`「読み込み」）。

### 出力の規則

`go-ratelimit` は、Go の基底 `go` に相乗りし、`docs/plugin-go.md`「相乗りのファイル」の決まりに従う。

- **対象**は、基底の `Seams` のメソッドのうち、自分が宣言した継ぎ目のもの: `plugin/golang` の `ShapeOf(view).Seams` のうち、`Transpiler` が `go-ratelimit` のもの（この仕様では、`server.login` の `@rate.limited` の一つ）。一つもなければ、ファイルを出さない。
- **ファイル**は、N の入口のパッケージに `ratelimit.go` を一つ。先頭は `// Code generated by hozoc go-ratelimit <版>; DO NOT EDIT.`。公開の名前は `RateLimiter`、`RateSeams`、`NewRateSeams` の三つ。非公開の名前を足すなら、前置きは `rate`。
- **`RateLimiter`** は、人が実装するインターフェース: `Allow(ctx context.Context, op string) bool`。`op` は、op の FQN。呼んでよければ `true` で、数えるのもこの中。
- **`RateSeams`** は、継ぎ目の実装の型（値の型。`struct{ limiter RateLimiter }`）。`NewRateSeams(limiter RateLimiter) RateSeams` が作る。対象の継ぎ目ごとに、基底と同じ名前と形のメソッド（`plugin/golang` の `SeamMethod` と `SeamSignature`）を、対象の並びのまま持つ:

```go
func (s RateSeams) <Op>RateLimited(<op の形の引数>, body func(<op の形の引数>) <op の形の戻り値>) <op の形の戻り値> {
	if !s.limiter.Allow(ctx, "<op の FQN>") {
		return <零値>, rate.LimitExceeded{}
	}
	return body(ctx, <引数>)
}
```

- 戻り値のない op では、`return rate.LimitExceeded{}` と `return body(ctx, <引数>)`。零値は、基底の規則（`docs/plugin-go.md`「呼び出し」）。例外の型は、`rate` の型の置き場の入口のパッケージから import し（`plugin/golang` の `HomeOf`）、値で返す。
- **`var _ Seams = RateSeams{}`** は、基底の `Seams` のメソッドが、すべて自分の対象のときだけ出す（単位に、継ぎ目を宣言したほかのトランスパイラがいれば、`RateSeams` だけでは `Seams` を満たさない。そのときは、人が、それぞれの型を一つの構造体に埋め込む。`docs/plugin-go.md`「継ぎ目の実装」）。
- op の入力と出力の値は変えない（`docs/requirements.md`「書き方の指針」）: 受け取った引数を、そのまま `body` に渡し、`body` の戻り値を、そのまま返す。
- コメントは、期待する出力のとおり（`RateLimiter` と、そのメソッド、`RateSeams`、`NewRateSeams` は決まった文。メソッドは `// <メソッド名> は op <FQN> に付いた @rate.limited の継ぎ目: limiter が許さなければ、中身を呼ばずに rate.LimitExceeded。許せば、中身をそのまま呼ぶ。`）。
- オプションはない。診断は二つで、どちらも error（`emit` の応答の `errors`）: **GORATE-1** は、`[emit.options.go-ratelimit]` に、知らないキーがある（位置なし）。**GORATE-2** は、公開の名前（`RateLimiter`、`RateSeams`、`NewRateSeams`）が、N の定義のパッケージの名前（型など。`docs/plugin-go.md`「名前」）と当たる（位置は当たった宣言。対象がなく、ファイルを出さない出力には出ない）。warning は返さない。Dir(U, N) の上に `go.mod` がないことは、基底が報告し（GO-1）、このトランスパイラは何も返さない。

### 数え方は、人が渡す

制限の数え方（何回まで、どの間隔で、何ごとに）は、仕様にも、プラグインにもない。`go-ratelimit` の出力は、インターフェース `server.RateLimiter` を出すだけで、実装は人が書く: `app/limiter.go` の `app.Limiter`（op ごとに、`Window` の間に `Max` 回まで）。組み立ては、`server.NewRateSeams(limiter)` を、`server.New` の最後の引数 `seams` に渡す一行（`app/onsqlite/compose.go:29`）。葉の op（`Leaves`）と同じく、渡し忘れはコンパイルエラーになる。

### 実装

`plugins/go-ratelimit/` の `main` パッケージ（`example.com/shop-server/plugins/go-ratelimit`。マニフェストの `wasm` は、これを WASM にビルドした `plugin.wasm`）。サンプルのモジュールの中のパッケージで、リポジトリの公開の SDK（`plugin`、`plugin/golang`）を、`go.mod` の `replace` を通して import する。`main.go` が入口、`emit.go` が中身。

- **ブラックボックスのテスト**は、リポジトリの `TestSamples` の `transpilers/go-ratelimit`: `gen/server/ratelimit.go` とバイト単位で比べる。
- **単体テスト**は `emit_test.go`（このモジュールの `go test ./...` が走らせる。`TestSamples` の `go`）。サンプルが使わない場合を、小さな入力 `testdata/<名前>/`（`hozo.toml` と `.hozo`）とゴールデン `testdata/<名前>/expected/` で確かめる: `seams`（戻り値のない op、零値の各行、当たる引数名、内部の op、継ぎ目を宣言したトランスパイラがもう一つある単位、対象のない名前空間）、`gorate1`、`gorate2`（診断）。生成したコードは、基底のプラグインをビルドして出させた基底のファイルと合わせて、一時のモジュールで `go vet` を通し、`testdata/seams/behavior_test.go` で動かす（制限を超えれば中身を呼ばず、零値と `rate.LimitExceeded` を返す。許せば、引数と戻り値をそのまま通す）。
- 入力の `View`（`testdata/<名前>/views.json`）は、リポジトリの `tests/pluginviews_test.go` が置く（`../README.md`「置き方」）。仕様を変えたら、リポジトリのルートで `go test ./tests -run TestPluginViewFixtures -update`、その後に、このディレクトリで `GOTOOLCHAIN=local go test ./plugins/go-ratelimit -update`。
- `testdata/seams/app.hozo` の、状態に触れる op（`add`、`lookup`）は、`read`（`itemById`）と効果（`addItem`）を呼ぶ配線（「R4 で、まだないもの」）。`testdata/seams/behavior_test.go` の、状態の単位の見立て `items` は、単位 `shop.ItemsState` のメソッド（形で決まる `IsEmpty` と `Get`、`ItemById`、`AddItem`）を、すべて自分で書く。葉の見立て `leaves` も同じで、どちらも、生成したインターフェースを埋め込まない: メソッドが足りないか、名前が違えば、`New` に渡すところでコンパイルできない（埋め込んでいた間は、名前の違うメソッドが、実行のときの nil の呼び出しになっていた）。ゴールデン（`expected/`）は、R4 で変わっていない: `add` と `lookup` の op の形は、同じ。

本体の側に要るもの（マニフェストの `seams` と `requires`、`View.config.seams`、基底 `go` の二層と `Seams`）は、できている（「まだないもの」）。継ぎ目を宣言する二つ目の例は、リポジトリの `go-sqlite` の `@db.transaction`（`docs/plugin-go-sqlite.md`「トランザクション」）。

## ファイルの出どころ

| ファイル | 出どころ |
|---|---|
| `type.hozo`、`store.hozo`、`server.hozo`、`mailer.hozo`、`smtp.hozo`、`design.hozo`、`client.hozo`、`flows.hozo`、`hozo.toml` | 人が書く（入力） |
| `plugins/go-ratelimit/hozo-plugin.toml`、`rate.hozo` | 人が書く（サンプルの中のプラグインのマニフェストと語彙。入力） |
| `sample.json` | 人が書く（各トランスパイラが出すファイル。`../README.md`「sample.json」） |
| `go.mod`、`go.sum` | 人が書く（「モジュール」） |
| `gen/type/doc.go` `types.go` | `go` |
| `gen/type/validate.go` | `go-valid` |
| `gen/rate/doc.go` `types.go` | `go` |
| `gen/store/doc.go` `types.go` `ops.go` `states.go` `wiring.go` | `go`（`types.go` は、型のパッケージの再公開） |
| `gen/store/types/doc.go` `types.go` | `go`（型のパッケージ: `store` は関数を持つので、型（ここでは、効果の定数）を、分けて置く。`docs/plugin-go.md`「型のパッケージ」） |
| `gen/store/db.go` | `go-sqlite` |
| `gen/store/storetest/doc.go` `fakes.go` | `go`（テストのパッケージ: `store` の、状態の単位と継ぎ目の、にせものと、`NewOps`。`docs/plugin-go.md`「テストのにせもの」） |
| `gen/server/servertest/fakes.go` | `go`（`server` の、葉と、`mailer` の op と、継ぎ目の、にせものと、`NewOps`） |
| `gen/server/inner/doc.go` `types.go` `ops.go` `wiring.go` | `go`（内側のパッケージ: すべての定義。`server` は内部の op を持つので、出力を二つに分ける） |
| `gen/server/types/doc.go` `types.go` | `go`（型のパッケージ: `server` の型の定義。内側の `types.go` は、その再公開） |
| `gen/server/types/validate.go` | `go-valid`（型にメソッドを足すので、型のパッケージ） |
| `gen/server/doc.go` `types.go` `ops.go` | `go`（入口のパッケージ: 内側の再公開と、`export` の op だけの `Ops`） |
| `gen/server/http.go` | `go-http`（入口のパッケージ） |
| `gen/server/servertest/http.go` `hozo-tests.go-http.json` | `go-http`（テストのパッケージ: 八つの入口の、インテグレーションテストの口と、その、テストの対応表。`docs/plugin-go-http.md`「インテグレーションテストの口」） |
| `gen/server/ratelimit.go` | `go-ratelimit`（入口のパッケージ） |
| `gen/server/servertest/doc.go` `leaves.go` `hozo-tests.go.json` | `go`（テストのパッケージ: `server` の葉の op の、単体テストの口と、その、テストの対応表。`docs/plugin-go.md`「単体テストの口」。`type`、`rate`、`http` は、葉の op も、述語で決まる値の分類も、にせものにするものも持たないので、テストのパッケージを持たない） |
| `gen/http/doc.go` `types.go` | `go` |
| `gen/client/doc.go` `ops.go` `wiring.go` | `go`（`client` は、内部の op を持たないので、出力を分けない。型も持たないので、`types.go` はない） |
| `gen/client/clienttest/doc.go` `fakes.go` | `go`（テストのパッケージ: `Fakes` と `NewOps`。`server` の op は、相手の入口なので、にせもの） |
| `gen/flows/doc.go` `types.go` `ops.go` `wiring.go` | `go` |
| `gen/flows/flowstest/doc.go` `fakes.go` | `go`（テストのパッケージ: `Fakes` と `NewOps`。`client` の出力を、本物で通す） |
| `gen/flows/flowstest/design.go` `hozo-tests.go-design.json` | `go-design`（テストのパッケージ: フローの E2E テストの口と、要求ごとの、テストの入れ物の口と、その、テストの対応表。`docs/plugin-go-design.md`。基底が、同じパッケージに出すのは、`doc.go` と `fakes.go`） |
| `gen/*/.hozoc-outputs.json` | hozoc の記録（`docs/cli.md`「書き出し」）。その出力のすべてのファイルのパス（`gen/server` の記録は、`inner/` の下のファイルも持つ） |
| `app/leaves.go`、`app/mailer.go`、`app/limiter.go` | 人が書く: `server` の葉の op 十の本体（`server.Leaves` を満たす型）、`mailer.send` の実装（`server.MailerOps` を満たす型）、`@rate.limited` の数え方（`server.RateLimiter` を満たす型）。保存先を知らない |
| `app/onsqlite/compose.go` | 人が書く: 組み立て（`server` を `gen/store` の上に）。`NewFlows` は、その上に、`client` と `flows` を重ねる |
| `app/cmd/shop-sqlite/main.go` | 人が書く: その組み立てで待ち受ける main |
| `app/handlertest/`、`app/storetest/`、`app/flowtest/` | 人が書く: 組み立て（`app/onsqlite`）が走らせるテスト（HTTP から見た振る舞い、`store` の op と状態の問い合わせの振る舞いのうち、保存先によらずに言えること、フローの E2E テストと、要求のテスト）。試す相手は、組み立てる関数で受け取る |
| `app/*_test.go`、`app/*/*_test.go` | 人が書く: テスト（「人が書いたもの」） |

## モジュール

- サンプルは自分のモジュール `example.com/shop-server`（`go.mod`）。生成したコードが import する `github.com/mew-ton/hozo/runtime` は `replace github.com/mew-ton/hozo => ../../..` で解決する。
- SQLite のドライバは cgo の要らない `modernc.org/sqlite`。import するのは人が書くコード（`app/cmd/shop-sqlite` と、テスト）だけ。期待する出力（`gen/`）は `database/sql` しか import しない。
- ドライバは `v1.46.1` に固定している。それより新しい版は、リポジトリが使う Go（1.24）より新しいツールチェーンを求める。`GOTOOLCHAIN=local` で動かす。

```
GOTOOLCHAIN=local go vet ./... && GOTOOLCHAIN=local go test -race ./...
GOTOOLCHAIN=local go run ./app/cmd/shop-sqlite   # http://localhost:8080。ユーザー alice@example.com / password123 を入れる
```

## 各トランスパイラの規則

規則の持ち主は設計文書。下の五つの節は、文書への入口と、この仕様が規則のどこを使っているかだけを書く（「分かったこと」は、この節の名前で規則を引いている）。`go-ratelimit` の規則は、上の「サンプルの中のプラグイン」。

## 基底 `go` が従う規則

持ち主は `docs/plugin-go.md`。この README が前に持っていた規則の、移した先:

| 前の項目 | 文書の見出し |
|---|---|
| 出すもの、`in` | `docs/plugin-go.md`「出力の対象」 |
| ヘッダ、ファイル、パッケージ | 同「配置」 |
| 継承した model | 同「model」 |
| 実装はインターフェースで受け取る、`Ops`、`Leaves`、状態の単位（`read` と効果のメソッド）、`<Ns>Ops` | 同「実装を受け取る口」 |
| 内部の op | 同「内側のフォルダと入口」（非公開のメソッドにする形は、やめた。「分かったこと」N68、N71） |
| メソッドの形、コメント | 同「op の形」「コメント」 |
| `New`、パッケージの変数はない、「実装がない」を返す道はない | 同「New」「実装を受け取る口」 |
| 配線の `match`、アーム | 同「match」 |
| 分類 | 同「分類」 |
| デコレータ（何も出さない） | 同「デコレータ」「継ぎ目」（読む者が継ぎ目を宣言したデコレータにだけ、二層と `Seams` を出す） |

この仕様が使っている出力の形: 分けない出力（`gen/type`、`gen/rate`、`gen/store`。内部の op がない）と、分けた出力（`gen/server`。内側 `inner` と入口）。継ぎ目は、配線の op 二つに一つずつ: `server.login` の `@rate.limited`（単位 4）と、`store.placeOrder` の `@db.transaction`（単位 3）。

この仕様が使っていない規則（文書にはある）: `nullable` と `single` の状態、状態を名前で読む配線、直に呼ぶ組み込みの効果（形で決まる命令 `Post` / `Put` / `Delete`）、遷移を宛先にする効果、述語で決まる分類、union、値のアーム、`const x = match`、`bind`、`never` を返す op、引数を持つ例外と `throw E(値)`。継ぎ目では: 葉と `never` を返す op の継ぎ目、一つの op の二つ以上の継ぎ目（入れ子）、内部の op の継ぎ目、継ぎ目を宣言したトランスパイラが二つ以上の単位。

この仕様での、op ごとの形（`docs/plugin-go.md`「op の形」「op の置き場」「継ぎ目」を当てはめたもの）:

| op（`server`） | 種類 | `export` | 純粋か（言語の事実。形には効かない） | `throws` | メソッドの形（どれも同じ） | Go で現れるところ |
|---|---|---|---|---|---|---|
| `login` | 配線 | ある | いいえ（デコレータ `@http.post` と `@rate.limited` があり、純粋でない op も呼ぶ） | ある（自分の `type.InvalidCredentials` と、`@rate.limited` が足す `rate.LimitExceeded`） | `ctx` を取り、`error` を返す | 入口と内側の `Ops` の `Login`。**継ぎ目を持つ**ので二層: 外側は `inner/ops.go` の `ops.Login`（`o.seams.LoginRateLimited(ctx, in, o.loginBody)`）、中身は `inner/wiring.go` の非公開のメソッド `loginBody`。継ぎ目の口は `Seams` の `LoginRateLimited`、その実装は `go-ratelimit` の `RateSeams` |
| `logout`、`getMe`、`getOrders`、`getLatestOrder`、`placeOrder` | 配線 | ある | いいえ（デコレータ `@http.*` があり、純粋でない op も呼ぶ） | ある | `ctx` を取り、`error` を返す | 入口と内側の `Ops` のメソッド（`Logout`）。本体は `inner/wiring.go` の、`ops` の公開のメソッド。`go-http` のハンドラが、入口の `Ops` を通して呼ぶ |
| `issueSession`、`authenticate`、`sessionOf` | 配線 | ない | いいえ（純粋でない op を呼ぶ: 順に `store.addSession`、`sessionOf`、`store.findSession`） | ある | `ctx` を取り、`error` を返す | 内側の `Ops` のメソッド（`Authenticate`）だけ。入口の `Ops` にはない。本体は `inner/wiring.go` の、`ops` の公開のメソッド |
| `checkPassword`、`requireFresh` | 葉 | ない | はい（デコレータも `reads` も効果もない） | ある | `ctx` を取り、`error` を返す | `Leaves` のメソッド（`CheckPassword`。人が実装する）と、内側の `Ops` のメソッド。`ops` のメソッドは、`o.leaves.CheckPassword(…)` を呼ぶだけ（`inner/ops.go`） |
| `newSession`、`toPublicUser`、`pageOf`、`newOrder`、`composeNotice` | 葉 | ない | はい（同じ） | ない | `ctx` を取り、`error` を返す（`(T, error)`） | 同じ（`Leaves` の `NewSession` と、内側の `Ops` の `NewSession`） |

`store` の十の op は、どれも `export` を付けた配線で、`read` か効果を呼ぶ（九つは一つだけ、`placeOrder` は効果を二つ）。本体は `gen/store/wiring.go` の、`ops` のメソッド（`return o.users.UserByEmail(ctx, email)`）。どれも、`ctx` を取り、`error` を返す（`throws` を書いている op は、ない。`addUser` だけが、呼ぶ効果 `createUser` から、例外 `type.EmailTaken` を、計算で持つ）。**五つの `read` と五つの効果は、op ではなく、読む状態・宛先の状態の単位（`<状態>State`）のメソッド**（`docs/plugin-go.md`「状態の単位」。`gen/store/states.go`）: `users` に `UserByEmail`、`UserById`、`CreateUser`、`sessions` に `SessionByToken`、`CreateSession`、`DeleteSession`、`orders` に `OrdersOfUser`、`LatestOrderOf`、`CreateOrder`、`notifications` に `NotifyUser`。`placeOrder` の二つの効果は、宛先が `orders` と `notifications` に分かれるが、それぞれの状態の単位のメソッドで、二つをまたぐ口は要らない。`placeOrder` は、継ぎ目を持つので二層（外側は `gen/store/ops.go` の `ops.PlaceOrder`、中身は `gen/store/wiring.go` の `placeOrderBody`）。`mailer.send` は、デコレータ `@smtp.sendMail` を持つ葉なので純粋でない（`mailer.hozo`）。`server.MailerOps` と `server.StoreOps` のメソッド（`mailer.send` と、`store` の九つ）は、相手がどれも純粋でないので、`ctx` を取り、`error` を返す。`mailer` に Go の単位を置けば、そこに出る `Ops` の `Send` も同じ判定で同じ形になり、その `Ops` は `server.MailerOps` をそのまま満たす。

**デコレータごとの、出力への効き方**（`docs/plugin-go.md`「デコレータ」の四つの形）:

| デコレータ | 読むトランスパイラ | 形 | 基底の出力への効き方 |
|---|---|---|---|
| `@http.post` / `@http.get`（op） | `go-http` | 入口を作る（ハンドラ） | なし |
| `@http.header` / `@http.query` / `@http.body` / `@http.bodyObj`、`@http.required` / `@http.default`（フィールド）、`@http.status`（例外） | `go-http` | 材料にする | なし |
| `@db.table`（状態） | `go-sqlite`（単位 3） | 実装を出す（`<状態>Table`） | なし |
| `@db.text` / `@db.datetime` / `@db.json`、`@db.key` / `@db.unique` / `@db.ref`（フィールド） | `go-sqlite`（単位 3） | 材料にする（`type` の単位 1 では、どのトランスパイラも読まない: 印） | なし |
| `@db.find` / `@db.list` / `@db.latest`（`read`）、`@db.insert` / `@db.delete`（効果） | `go-sqlite`（単位 3） | 実装を出す（`<状態>Table` の、その `read` と効果のメソッド） | なし（`read` と効果の口は、デコレータによらず、基底が出す。単位 4 でも同じ） |
| `@db.transaction`（op） | `go-sqlite`（単位 3） | 継ぎ目を要る | 二層・`Seams`・`New` の引数 `seams` |
| `@valid.*`（scalar、フィールド） | `go-valid` | 材料にする（`Validate`） | なし |
| `@rate.limited`（op） | `go-ratelimit`（単位 4） | 継ぎ目を要る | 二層・`Seams`・`New` の引数 `seams` |
| `@smtp.sendMail`（op） | なし（`mailer` には単位もない） | —（印） | なし |

## `go-valid` が従う規則

持ち主は `docs/plugin-go-valid.md`。出すファイルは `validate.go` 一つ（定義のパッケージ: `gen/type/` と、`gen/server/inner/`）。

- この仕様で `Validate` を持つ型（同「Validate を持つ型」）: `type` の `Email`、`Password`、`User`、`PublicUser`、`OrderLine`、`Order`。`server` の `Credentials`、`LoginInput`、`LoginOk`、`PlaceOrderInput`（`AuthInput` と `GetOrdersInput` にはない）。
- `server` の `Validate` は `type` の型のメソッドを呼ぶので、`type` の単位も `go-valid` を並べている（同「出力」の GOVALID-3）。
- `server` のメソッドは内側のパッケージの型に付く。入口の型は別名なので、入口の名前（`LoginInput`）の値も、同じメソッドを持つ。`go-http` は、型ごとに `in.Validate()` を書かず、補助 `httpValidate` が、実行時に、値が `Validate` を持つかを見る。

## `go-sqlite` が従う規則

持ち主は `docs/plugin-go-sqlite.md`。出すファイルは `db.go` 一つ（入口のパッケージ）。

- **五つの `read` と五つの効果は、どれも `db` の語彙のデコレータを持つ**（同「read の中身」「効果の中身」）: `read` は、`@db.find` が四つ（`userByEmail`、`userById`、`sessionByToken`、`latestOrderOf`）、`@db.list` が一つ（`ordersOfUser`）。効果は、`@db.insert` が四つ、`@db.delete` が一つ（`deleteSession`）。表は、宣言の `reads` の状態か、効果の宛先の状態で、列は、引数の名前（`email`、`id`、`token`、`userId`）。形で決まる問い合わせは、四つの状態に `IsEmpty` と `Get` で八つ。四つの型（`UsersTable`、`SessionsTable`、`OrdersTable`、`NotificationsTable`）は、合わせて十八のメソッドを持ち、どれも基底の口（四つの状態の単位）を満たす。op の中身は出さない（基底の配線が、これらのメソッドを呼ぶ）。
- **`@db.find` が一行に決まる理由は、三通り**（同「read の中身」）: `userById` と `sessionByToken` は、引数がキー（`@db.key`）。`userByEmail` は、引数が一意の列（`@db.unique`）。`latestOrderOf` は、引数（`userId`）では決まらず、`@db.latest("placedAt")` が、いちばん新しい一行に決める（`ORDER BY "placedAt" DESC, rowid DESC LIMIT 1`）。
- **`placeOrder` は、`@db.transaction` の継ぎ目で、一つのトランザクションになる**（同「トランザクション」）: `go-sqlite` の `DbSeams` が、始めて、基底の配線の本体（二つの効果を順に呼ぶ）を呼び、確定する。失敗すれば、取り消す。二つの効果のメソッド（`OrdersTable.CreateOrder`、`NotificationsTable.NotifyUser`）は、受け取った `ctx` が運ぶトランザクションで、文を実行する。組み立ては、四つの表の型と `store.NewDbSeams(db)` を `store.New` に渡す一行（`app/onsqlite/compose.go`）。
- 列と引数の型（同「列と引数の型」）は、どの `read` と効果でも等しい（`email: type.Email`、`id: type.UserId`、`token: type.Token`、`userId: type.UserId`）。条件は、どれも `"<列>" IS ?1` で、引数は、その列のしまい方の補助で包む（どれも `@db.text` の列なので `dbText(…)`）。
- **しまい方**（同「しまい方」）: 四つの表の十五の列は、`@db.text` が十、`@db.datetime` が二、`@db.date` が一、`@db.time` が一、`@db.json` が一。`dbArgs<Model>` と `dbScan<Model>` は、フィールドごとに、しまい方の補助の呼び出し一つ。scalar（`type.UserId` など）と核の型（`string`）を、書き分けていない。
- **省略可能な列は、二つ**（同「表」）: `Order` の `deliverOn` と `deliverFrom` だけが、省略可能で、`NOT NULL` が付かず、`dbArgsOrder` と `dbScanOrder` の行に `dbNullable` が付く。ほかの十三のフィールドは、どれも省略可能でなく、`NOT NULL`。三つの `@db.ref(User)` は、どれも先が決まる（要素の型が `type.User` の表の状態は `users` の一つで、`User` の `@db.key` は `id` の一つ）。
- デコレータを付けていない `read` か効果があるときの形（人が、プラグインの型を埋め込んで書き足す。文は `store.Conn` を通す）は、この仕様では起きないので、見立ての型で確かめている（`app/onsqlite/store_test.go` の `partialUsers` と `humanUsers`）。
- この仕様が使っていない規則: 省略可能なフィールドの列（と、`T?` の列と比べる引数）、`@db.int` / `@db.float` / `@db.bool` の列、`@db.update`、引数のない `@db.list`、二つ以上の引数の条件、`@db.where`、`@db.list` に足した `@db.latest`、複数の `@db.key`、形で決まる `Post`、戻り値のある op の `@db.transaction`、入れ子になる `@db.transaction` の op（継ぎ目の実装のメソッドを、テストが二重に呼んで確かめている）。

## `go-http` が従う規則

持ち主は `docs/plugin-go-http.md`。出すファイルは、入口のパッケージの `http.go`（この見出し）と、テストのパッケージの `http.go`（インテグレーションテストの口。「インテグレーションテストの口で変えたこと」）。**通信の上の形（要求と応答の JSON）の持ち主も、その文書の「通信の形」**で、`app/handlertest` が文字列のまま比べている JSON は、その実物。

- この仕様では、`@http.*` の付いた op と、`server` の `export` を付けた op が、同じ六つ。`NewHandler` が受け取る `Ops` は、ハンドラが呼ぶ op ちょうど。
- **六つのハンドラは、同じ形**（同「出力」）: `var in <Model>`、`req := httpRequest{r: r}`、フィールドごとの一行、`req.ok`、`httpValidate`、op の呼び出し、`httpWriteResult`（`gen/server/http.go:31-144`）。フィールドの読み方は四通りで、どれもデコレータとその引数だけで決まる: `req.bodyObj(&in.Body)`（`@http.bodyObj`）、`req.header("Authorization", &in.Token)`（`@http.header("Authorization")`）、`req.query("page", &in.Page, httpDefault(1))`（名前を省いた `@http.query` と、`@http.default(1)`）、`req.body("lines", &in.Lines, httpRequired)`（名前を省いた `@http.body` と、`@http.required`）。
- **項目がないときの扱いは三通りで、どれもデコレータだけで決まる**（同「入力の読み方」）: `lines`（`@http.required`）は 400、`page`（`@http.default(1)`）は 1、三つの入力の `token`（どちらもない。省略可能）は、値がないまま op に渡り、op の中の配線 `authenticate` が 401 にする。
- `gen/server` の単位だけが `validate = true` を渡す（同「オプション」）。`httpValidate` は、六つのハンドラのすべてにある。入力が `Validate` を持つ（`LoginInput`、`PlaceOrderInput`）か、持たない（`AuthInput`、`GetOrdersInput`）かは、実行時に見る。
- **成功の応答は、実行時に決まる**（同「通信の形」の「成功の応答」）: `logout`（戻り値がない）は、いつも 204。`getLatestOrder`（`type.Order?`）は、注文がなければ 204、あれば 200。`getOrders`（`type.Order[]`）は、注文がなくても 200 と `[]`。どのハンドラも、同じ `httpWriteResult` を呼ぶだけで、ステータスを書かない。
- **`@http.status` を持つ例外は三つ**: `type.InvalidCredentials`（401）、`rate.LimitExceeded`（429）、`type.Unauthorized`（401）。並びは、経路の op の宣言順と、その `exceptions` の順（`login` の二つが先）。`rate.LimitExceeded` は、語彙 `rate` の例外で、`login` の `throws` には書かれていないが、`@rate.limited` が足すので `login` の `Op.signature.exceptions` にあり、`go-http` は、ほかの例外と同じに扱う（`gen/rate` を import する）。`go-http` は、`go-ratelimit` も、継ぎ目も知らない。
- この仕様が使っていない規則: 名前を書いた `@http.query` と `@http.body`、名前を省いた `@http.header`、ヘッダとクエリの全体の形（`@http.headerObj`、`@http.queryObj`）、二つ以上の本文の項目、`@http.required` のヘッダとクエリ、`@http.default` のヘッダと本文の項目、どちらもない省略可能でない項目（零値のまま渡る）、`int` 以外のクエリ、引数を持つ例外（`value` のある失敗の応答）、record の戻り値。

v1 の規則から変えたのは三つ: `r.Context()` を渡す（P29）、制約の検査（P15、P16）、op を呼ぶ前の失敗の本文（v1 は `{"error":"http.BadRequest","message":"…"}`。「分かったこと」N44）。P33 で足したのは、`export` のない op についての規則一つ。決定 103〜110 で変えたのは、上の、同じ形のハンドラと、実行時に決まる成功の応答と、要求のどこから読むかの六つのデコレータ。決定 114〜116 で変えたのは、項目がないときの扱い（`@http.required`、`@http.default`）と、入力を `var` で置くこと。

## `go-design` が従う規則

持ち主は `docs/plugin-go-design.md`。出すファイルは、単位 7 の、テストのパッケージの `gen/flows/flowstest/design.go` と、その、テストの対応表 `hozo-tests.go-design.json`（「R6 のプラグインの部分で足したこと」「走ったかの確かめで足したこと」）。

- **フローは一つ**（同「E2E テストの口」）: `flows.signInAndOrder`。`SignInAndOrderFlowCase` のフィールドは、フローの三つの引数（`Email`、`Password`、`Lines`）と、`Given`、`Then`。フローは、値（`flows.Ordered`）を返すので、`Then` は `func(t *testing.T, got flows.Ordered)`。
- **フローの op の決まり**（同「フローの op の決まり」）は、どれも満たしている: `export` が付き、配線で、呼ぶ先の三つ（`client.signIn`、`client.order`、`client.viewOrders`）は、どれも `@usecase.of` の付いた op で、`flows` には、`@usecase.of` の付いた op がない。
- **要求は二つ**（同「要求の、テストの入れ物の口」）: オプション `requirements` に並べた分類 `design.Requirement` のケース `PRD_001`、`PRD_002`。分類 `design.Actor` は、並べていないので、要求ではない。
- **指している設計**（同「指している設計」）は、`server` の三つの op から: `PRD_001` は `server.authenticate`、`server.placeOrder`。`PRD_002` は `server.getOrders`、`server.placeOrder`（宣言順）。どちらも指されているので、warning は、出ない。
- `flows` は、葉の op も、述語で決まる値の分類も持たないので、基底が、テストのパッケージに出すのは、`doc.go` と `fakes.go`（「テストのにせもので足したこと」）だけ。`flowstest` の、テストの口は、`design.go` で、対応表は、`go-design` のものだけ。
- この仕様が使っていない規則: 値を返さないフロー、引数のないフロー、`Given`・`Then` と当たる引数の名前、二つ以上の要求の分類、どこからも指されていない要求、op のほかの宣言からの `@prd.ref`、要求だけの単位、基底が、テストのパッケージを作る出力、診断（GODESIGN-*）。`plugins/go-design` の単体テストが確かめる（`docs/testing.md`「単体テスト」）。

## 人が書いたもの

- **`app/leaves.go`**: `server` の葉の op 十の本体。型 `app.Leaves` のメソッドで、`*app.Leaves` が生成された `server.Leaves` を満たす（`app/leaves.go:38`。メソッドが足りなければ、ここでコンパイルできない）。`server.New` の引数 `leaves` に、そのまま渡す。時計、トークンと注文の識別子の作り方、セッションの有効期間、1 ページの件数は `app.Leaves` のフィールドで、テストが差し替える。**どのメソッドも、最初の引数に `ctx` を取り、最後の戻り値に `error` を返す**（`docs/plugin-go.md`「op の形」）: `server.Leaves` のメソッドの形（`gen/server/inner/ops.go` の `Leaves`）がそう決めていて、違う形のメソッドでは満たせない（コンパイルのときの事実。`app/server_test.go` の先頭の `var` が、メソッド式の代入で確かめる）。七つとも内部の op（`export` がない）で、入口の `server.Ops` にはない。葉の本体のテスト（`app/leaves_test.go` の `TestLeaves`）は、`server.Leaves` として持った `*app.Leaves` のメソッドを直に呼ぶ。`RequireFresh` は、今が期限より前ならセッションをそのまま返し、期限ちょうどから先は `type.Unauthorized`（`app/leaves.go:69-74`）。トークンと識別子を作る関数（`app.Leaves` の `NewToken`、`NewOrderID`）は `error` も返し、`NewSession` と `NewOrder` が、それをそのまま返す: main（`app/cmd/shop-sqlite`）は、乱数を読めないときに、panic にせず、その `error` を返す（ハンドラが 500 にする）。
- **`app/leafcases_test.go`**: 十の葉の、結果ごとの単体テスト。型 `leafCases` が、基底の出した口 `servertest.LeafCases` を満たし、`TestLeafCases` が、`servertest.RunLeaves(t, leaves, leafCases{})` に渡す。書いてあるのは、引数の値と、返り値の確かめ（`Then`）だけ: 葉を呼ぶ所と、結果の見分け（正常か、どの例外か）は、生成したコードが持つ。テストの名前は、`TestLeafCases/<op の FQN>/<結果>`（`TestLeafCases/server.checkPassword/InvalidCredentials`）。結果が二つあるのは、`checkPassword`（`type.InvalidCredentials`）と `requireFresh`（`type.Unauthorized`）で、ほかの七つは、正常のケースだけ。**ケースを欠いた型は、口を満たさない**（`TestIncompleteCasesAreRejected`: 欠いた型を渡す行は、コンパイルできないので、口が求めるメソッドを、リフレクションで確かめる。コンパイルできないことそのものは、リポジトリの `plugins/go` の単体テストが確かめる: `docs/testing.md`「単体テスト」）。
- **`app/fakes_test.go`**: 基底が出した、にせものの上のテスト。`TestFakesPlaceOrder`: 端が返す値（セッション）を言うと、`server.placeOrder` の流れが、配線と葉の本物で通り、効果（`createOrder`、`notifyUser`）と `mailer.send` が、言ったとおりの値で、一度ずつ呼ばれる。`TestFakesThrow`: にせもので、例外を起こす四つの道（流れの端が投げる、葉が投げる、端が「ない」を返す、継ぎ目が投げる）。`TestFakesFlow`: `flowstest.NewOps` が、`flows` と `client` の配線を重ね、`client` が呼ぶ `server` の op（相手の入口。流れの端）は、にせものが、言った値を返す。フローが、サーバーなしで通る。`TestFakesFlowThrow`: 相手の入口の境界で起きる例外（`@http.post` の `outer throws` の `http.NetworkError`）を、そのにせもので起こす。`server` の流れのテストには、葉は、人が書いた本物（`app.Leaves`）を渡す。
- **`app/mailer.go`**: `mailer.send` の実装。`*app.Mailer` が `server.MailerOps` を満たし、`server.New` の引数 `mailer` に、そのまま渡す。受け取った `ctx` は、届ける関数（`Deliver`）に渡す。届けられなければ `type.NotificationFailed`。届け方（`Deliver`）はテストが差し替える。`@smtp.sendMail` は、読むトランスパイラがなく、`mailer` には単位もないので、基底は何も出さない: デコレータの意味（SMTP で送る）を実装するのは、`server.MailerOps` を満たす型を書く人。
- **`app/limiter.go`**: `@rate.limited` の数え方。`*app.Limiter` が、`go-ratelimit` の出力の `server.RateLimiter` を満たす（`app/limiter.go:34`）。op ごとに、`Window` の間に `Max` 回まで。時計（`Now`）は、テストが差し替える。`server.NewRateSeams(limiter)` に渡し、その戻り値を `server.New` の最後の引数 `seams` に渡す。数え方のテストは `app/limiter_test.go`。
- `app` パッケージは保存先を知らない: import するのは `gen/server` と `gen/type` だけ（`app/leaves_test.go` の `TestNoStorageImport`）。
- **組み立て**: `app/onsqlite/compose.go`。表を作る `store.CreateTables(ctx, db)` を呼び、`go-sqlite` の四つの表の型と `store.NewDbSeams(db)` を `store.New` に渡す。`store.New(…)` の戻り値を `server.New(leaves, mailer, s, server.NewRateSeams(limiter))` の三つ目に渡し、その戻り値を `server.NewHandler` に渡す。組み立ては何度呼んでもよい（パッケージの変数がない）。メモリの上の組み立て（`app/onmem`）は、なくした（「メモリ版の `store` をなくしたこと」）。
- **`app/httpcases_test.go`**: 八つの入口の、インテグレーションテスト。型 `httpCases` が、`go-http` の出した口 `servertest.HTTPCases` を満たし、`TestHTTPCases` が、`servertest.RunHTTP(t, fakeLeaves(), httpCases{})` に渡す。書いてあるのは、入力の値と、流れの端が返す値（`Given`: にせものに、`read` が返す値を言う）と、足りない確かめ（`Then`: どの効果が、何の値で呼ばれたか）だけ: 流れの組み立て（にせものの上の、`server` と `store` の配線）、要求の組み立て、送ること、結果の見分けは、生成したコードが持つ。書くケースは、正常と、op を呼ぶ前の失敗と、Hozo が作れない例外のケース（「インテグレーションテストの作り直しで変えたこと」の表）の、18。テストの名前は、`TestHTTPCases/<op の FQN>/<結果>` と、`TestHTTPCases/<op の FQN>/<例外の名前>/<出どころ>`（`TestHTTPCases/server.placeOrder/Unauthorized/server.sessionOf.LoggedOut`）。`TestIncompleteHTTPCasesAreRejected` は、口のメソッドが、人か AI が書くケースだけであることを、リフレクションで確かめる（Hozo が作れる出どころは、口にない）。
- **`app/flowtest`**（`flowtest.go`）: フローの E2E テストと、要求のテスト。型 `FlowCases` が、`go-design` の出した口 `flowstest.FlowCases` を満たし、`RunFlows` が、`flowstest.RunFlows(t, s.Flows, FlowCases{s})` に渡す。書いてあるのは、フローの引数と、今の状態の用意（`Given`: alice を足す）と、後の状態と返り値の確かめ（`Then`: できた注文が、返ってきた一覧と、`store` の両方に、一つある）だけ: フローの配線は、基底が作り、`Given`、フロー、正常に終わったことの確かめ、`Then` の順は、生成したコードが持つ。テストの名前は、`TestFlows/flows.signInAndOrder`。型 `Requirements` が、口 `flowstest.Requirements` を満たし、要求ごとの入れ物（`PRD_001`、`PRD_002`）に、サブテストを二つずつ持つ（`TestRequirements/design.Requirement.PRD_001/…`）。どの設計を通すかは、入れ物が選ぶ: ここでは、どちらも、操作の op（`client`）を通す。`app/onsqlite/compose_test.go` の `TestFlows`、`TestRequirements` が、走らせる。**口を一つ欠いた型が、コンパイルできないこと**は、段 2・段 3 と同じ分け方: `RunIncomplete`（`app/onsqlite/compose_test.go` の `TestIncompleteFlowCasesAreRejected`）が、口のメソッド（`FlowCases` は、フローごとに一つ。`Requirements` は、要求ごとに一つ）と、欠いた型が、口を満たさないことを、リフレクションで確かめ、コンパイルできないことそのものは、リポジトリの `plugins/go-design` の `TestIncompleteDoNotCompile` が、生成したコードをコンパイルして確かめる。
- **`app/handlertest`**（`handlertest.go`）: HTTP から見た振る舞い。`app/onsqlite/compose_test.go` の `TestHandler` が走らせる。サブテストは、それぞれ新しいサーバーを作り、並列に走る（`t.Parallel()`）。応答の本文を文字列のまま比べるので、そこに書いた JSON が、クライアントが合わせる形になる。確かめること: パスワード違いとユーザーなしは 401 `type.InvalidCredentials`、制約に合わない入力は 400 `http.BadRequest` と違反の一覧、ログインでトークン、`GET /me`（トークンあり・なし・知らないトークン）、`POST /orders`（本文は `{"lines":[…]}`）で注文と通知が一つずつ（通知は、状態 `store.notifications` の `Get` で数える。`Server.count`）、`mailer.send` を失敗させると 500 で**注文も、通知の記録も残る**（二つは、配送の前に、一つの op で一緒に記録してある）、`GET /orders?page=…`（`page` がなければ、`@http.default(1)` の 1 ページ目）、読めない `page` は 400、ログアウトの後の `GET /me` は 401、中断された要求は 500 で状態が変わらない、**期限の切れたセッションは、どの入口でも 401 `type.Unauthorized`**（時計を進めて確かめる。サブテスト `expired`）。
  - **`login/LimitExceeded`**（`app/handlertest/handlertest.go:166-196`）: **`/login` を、制限（`LoginLimit` 回。パスワード違いの失敗も一回と数える）を超えて呼ぶと、429 `{"error":"rate.LimitExceeded"}`**。op の中身は呼ばれない（トークンが作られない）。制約に合わない入力は、制限を超えていても 400 が先（`go-http` の検査は op を呼ぶ前で、op を呼ばないので、数えない）。時間の幅が過ぎれば、また呼べる。`@rate.limited` の付いていない op（`GET /me`）は、何度呼んでも制限されない。
  - **`getLatestOrder`**: **省略可能な戻り値で、値がなければ 204 で本文なし**（`null` を返さない）。注文を足した後は、同じ経路が 200 と、その注文の JSON（二つの注文は、時計を止めてあるので `placedAt` が同じ。返るのは、後から足した方）。空の配列（`GET /orders`）は値なので 200 と `[]`、戻り値のない `POST /logout` は 204（サブテスト `getOrders`、`logout`）。
  - **`names`**: 入力のフィールドを、要求のどの名前から読むか。ヘッダは、書いた名前（`@http.header("Authorization") token`: `Authorization` で読み、フィールドの名前 `token` のヘッダは読まない）。クエリは、名前を省いたので、フィールドの名前（`@http.query page`: `page` で読み、`p` は読まない）。本文の項目も同じ（`@http.body lines`: 本文のオブジェクトのキー `lines`。ほかのキー `items` は読まないので、`lines` はないことになり、`@http.required` で 400）。本文の全体（`@http.bodyObj body`）は、本文そのものが `server.Credentials` で、フィールドの名前 `body` では包まない。
  - **`required`**（`app/handlertest/handlertest.go:325-346`）: **`@http.required` の項目（`lines`）がなければ、400 `{"error":"http.BadRequest","value":{"message":"body lines: missing","violations":[]}}`** で、op は呼ばれない（注文も通知も増えない）。トークンがなくても、401 より先に 400。キーがあれば、空の配列でも、ある（200）。`@http.required` のない項目（`Authorization`、`page`）は、なくても 400 にならない。
  - `handlertest.RunTwo`（`app/onsqlite/compose_test.go` の `TestTwoServers`）: **同じ出力の上の二つのサーバーが、一つのプロセスで同時に動き、混ざらない**。データベースが二つで、`store.New` と `server.New` を二度呼ぶ。二つに並行に要求を送り、注文と通知の数、ログアウトが、もう一方に及ばないことを確かめる（`app/handlertest/handlertest.go:404-430`）。数え方（`app.Limiter`）も、サーバーごとに別の値。
- **`app/storetest`**: `store` の op と、状態の形で決まる問い合わせの振る舞いのうち、保存先によらずに言えること。**`latestOrder` は、`placedAt` がいちばん新しい注文を返す**（最後に足した注文ではない。同じ時刻なら、後から足した方。サブテスト `orders`）。**`placeOrder` は、注文と通知を一緒に足し、一つ目の効果が失敗するとき（同じ id の注文）は、通知も足さない**（サブテスト `placeOrder`。二つ目だけが失敗するときは、継ぎ目で決まるので、ここでは確かめない）。`app/onsqlite/store_test.go` の `TestStore` が走らせる。`server` は組み立てない。サブテストは並列に走る。効果の結果は、`store.New` に渡したのと同じ、状態の単位の実装の `Get` と `IsEmpty` で確かめる（`read` のない `notifications` も読める。状態の単位の実装の型は、`storetest.List[T]` をそのまま満たす）。中断された `ctx` で呼ばれた op と問い合わせが `context.Canceled` を返し、四つの状態が変わらないことも確かめる。
- **`app/onsqlite/store_test.go`**: `go-sqlite` だけの事実。
  - **`placeOrder` は一つのトランザクション**（`TestPlaceOrderIsOneTransaction`）: 二つ目の効果（通知の行。`userId` が、いないユーザーで、外部キーの違反）が失敗すれば、一つ目の効果（注文の行）も取り消され、その後も続けて足せる。接続を一つに限ってあるので、二つの効果がトランザクションの外の接続を使えば、テストは戻ってこない。
  - **それを守っているのは、継ぎ目**（`TestTransactionIsTheSeam`）: 同じ表の型の上で、継ぎ目だけを素通しに替えると、二つ目の効果が失敗しても、注文の行が残る。
  - **入れ子は、外側に加わる**（`TestNestedTransactionJoins`）: 継ぎ目の中から、もう一度継ぎ目を通っても、新しいトランザクションは始まらず、外側が失敗すれば、内側のしたことも残らない。
  - `CreateTables` は中断された `ctx` では表を作らず、二度呼んでも足した行が残る。プラグインの型に足りないメソッドを、人が埋め込みで書き足せ、`store.Conn` を通せば、そのメソッドもトランザクションに入る（`TestHumanCompletesPluginType`。足りないままの型が `store.UsersState` を満たさないことは、リフレクションで確かめる）。`datetime` と配列の列の形、`REFERENCES`。
  - **状態の単位は、状態ごとに、別の実装から取れる**（`TestStatesFromTwoImplementations`）: `users`、`sessions`、`orders` は `go-sqlite` の型、`notifications` は、テストの中に人が書いた、メモリの上の型（`memNotifications`）。`store.New` は、状態ごとのインターフェースを満たす値を受け取るだけで、どの実装かを知らない。`@db.transaction` の継ぎ目が取り消せるのは、同じ `db` の上の `go-sqlite` の型がしたことだけで、メモリの上の単位がしたことは、入らない。
- **`app/server_test.go`**: `server` だけ（`store` のどの出力も使わず、`server.StoreOps` を満たす型をテストが書く）。
  - メソッドの形（どれも、最初に `ctx`、最後に `error`）を、メソッド式の代入で確かめる: 入口の `server.Ops` の六つ、`server.Leaves`、`server.MailerOps`、`server.StoreOps`、**内側の `inner.Ops` の内部の配線**（`IssueSession`、`Authenticate`、`SessionOf`）、**継ぎ目 `server.Seams.LoginRateLimited`**（op の引数の後ろに、op と同じ形の `body`）。入口の型が内側の型の別名であること、`go-ratelimit` の `server.RateSeams` が `inner.Seams` も満たすことも、代入で確かめる。
  - 要求の `context` が `server.StoreOps` のメソッドにそのまま届く、`go-http` が読めない要求と制約に合わない入力では op を呼ばない、`New` は `nil` を確かめない（`TestNilIsNotChecked`: `nil` の `leaves` と、`nil` の `seams`）。
  - **`TestMissingItems`**（`app/server_test.go:340-413`）: 要求に項目がないときに、**op に渡る入力**を、入力を覚えるだけの `server.Ops` で確かめる。`page` のない `GET /orders` は `Page` が 1（`@http.default(1)`）、`page=3` は 3、空の `page=` は「ある」ので、既定値にならず 400。`lines` のない `POST /orders` は、op が呼ばれない（`@http.required`）。トークンのない要求は、`Token` が `nil` のまま op に渡る（どちらのデコレータもない）。
  - **`TestReplaceOneLeaf`**: 葉の op を一つだけ差し替え（`newSession`）、内部の配線 `issueSession` を、**内側の `Ops` から直に呼ぶ**（`inner.New(…).IssueSession(ctx, user, password)`）。`issueSession` が通る別の名前空間の op（`store.addSession`）だけを書けばよい。
  - **`TestLeafErrorIsReturned`**: `throws` を持たない葉（`newSession`）が返した `error` は、配線 `issueSession` を通って、包まれずに呼び出し元に届き、その後ろの行（`store.addSession`）は呼ばれない。
  - **`TestPlaceOrderRecordsThenSends`**: 配線 `server.placeOrder` は、注文と通知を、一つの op（`store.placeOrder`）に一緒に渡してから、配送する（`mailer.send`）。配送に失敗しても、`store.placeOrder` は済んでいる。
  - **`TestLoginIsLimitedThroughOps`**: **制限は、HTTP ではなく op に掛かる**。`server.New` が返す `Ops` の `Login` を直に呼んでも、`LoginLimit` 回を超えれば `rate.LimitExceeded`（値）が返り、中身は呼ばれない（`store` のどの op も呼ばれない）。
- **`app/inner_test.go`**: 内側のパッケージ（`gen/server/inner`）を通すテスト。
  - `TestInternalWirings`: 内部の配線 `issueSession`、`authenticate`、`sessionOf` を、`inner.New(…)` の戻り値から直に呼ぶ（パスワード違い、トークンなし、知らないトークン、期限切れ）。**トークンのない `authenticate` は、`store.findSession` を呼ばずに `type.Unauthorized`**（分類 `server.Credential` の、残りのケース `Absent` のアーム）。継ぎ目を持つ op を呼ばないので、`seams` は `nil` のまま。
  - `TestBodyBehindSeam`: 継ぎ目を持つ op の**中身だけ**を試す。継ぎ目を素通しにする `inner.Seams`（テストが書いた型 `through`）を渡すと、制限なしに、配線の本体の結果がそのまま返る。外側は、呼ばれるたびに継ぎ目を一度通る。
  - `TestSeamOwnsBeforeAndAfter`: 継ぎ目は、中身の前と後の両方を持つ（中身に渡す値と、返す値を、継ぎ目の実装が決める。基底の外側は、何も足さない）。
- **`app/ops_test.go`**: **入口の `Ops` に並ぶのは `export` を付けた op だけ**（`TestOpsHoldExportedOpsOnly`。インターフェースの型のリフレクションで確かめる）: `server.Ops` のメソッドはちょうど八つ、内側の `inner.Ops` は二十のすべて、`server.Seams` は `LoginRateLimited` の一つ、`store.Seams` は `PlaceOrderDbTransaction` の一つ。`store.Ops` は十とも持ち、`server.StoreOps`（そのうちの九つ）を満たす。`read` と効果は、どの `Ops` にもない。内部の op を持たない出力（`store`、`type`、`rate`）に、内側のフォルダはない。内部の op を、入口の `New` が返す値から取り出せないことは、確かめない（約束していない。`docs/plugin-go.md`「内側のフォルダと入口」）。前の名前は `app/both_test.go`（「メモリ版の `store` をなくしたこと」）。

## 分かったこと

サンプルを書いたときの記録。**N1〜N86 が引く行番号と、op の数え方（`store` の九つの op、`notify`、`addOrder`、`server` の十五の op など）、`gen/server/http.go` と `gen/store/db.go` の形（`httpReadBody`、`in.Validate()`、`dbOpt` など）は、その項目を書いたときのもの**で、決定 91〜113 を当てはめた書き直し（「下書きから変えたこと」の 9〜15）で、`type.hozo`、`store.hozo`、`server.hozo`、`gen/store`、`gen/server`、`app/` は変わっている。今の形を引いているのは、この節より上と、N118 から後ろ（N87〜N117 のうち、`store` の op と `go-sqlite` の形 — op に付ける `@db.find` / `@db.reads` / `@db.effects`、`SharedState` と `SharedTable`、`@db.nullable`、`latestOf` — を引くものは、その項目を書いたときのもの）。規則を引いている `docs/emit-go.md` は、そのときの文書（本体に組み込んでいた Go の出力）で、今は `docs/plugin-go.md` が置き換えた。「基底 `go` が従う規則」などの節の名前で引いている規則は、上の「各トランスパイラの規則」が指す文書にある。設計文書を書くときに決まったもの（N23 の、人が書く述語の置き場。N29 の `runtime` の持ち主。N30 の、単位をまたぐ依存。N32 の、オプションと依存。N44 の `http.BadRequest`。N55 の名前の衝突）は、項目を書き直さず、ここに挙げるだけにする: 決まった中身は、文書の方にある。期待する出力を文書の形にそろえたとき（P35 の内側のフォルダ、P36〜P39 の継ぎ目）に形が変わった項目（N26、N51、N52、N54、N56、N65、N68、N70、N71、N72）は、番号と元の記録を残して、「その後」を書き足した。ファイルと行の参照は、今の `gen/` と `app/` のもの。ただし、メモリ版の `store`（`gen/storemem`、`app/memstore`、`app/onmem`、`app/cmd/shop-mem`、`app/both_test.go`、「単位 4」、二つの組み立て・二つの実装）を引く項目は、その項目を書いたときのもの: メモリ版は、`docs/decisions.md`「決めたこと」388 で、なくした（「メモリ版の `store` をなくしたこと」）。

### v1 の「分かったこと」の、その後

v1 の 63 項目（`git show ac57788:tests/samples/shop-server/README.md` の「分かったこと」。番号はそのまま）が、P1〜P33 を当てはめてどうなったか。「解決」は、その項目が言っていた不足がこの形ではなくなったもの。「残る」は、形が変わっても同じ不足があるもの。「当たらない」は、v1 の組み方にだけあった事情。N で始まる番号は「v2 を書いて分かったこと」の項目。

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
| 11 | 記録を代役の emit から作れない | 当たらない | 代役（`pick`）と `shared` は、なくなった。記録は、`files` が、その下のファイルの一覧と比べる |
| 12 | キャンセルと期限が伝わらない | 解決 | P29 |
| 13 | 認証の要る葉が、それぞれ呼んだ人を引く | 解決 | P1〜P3、P13: 配線 `authenticate` が一度引き、セッションを引数で渡す |
| 14 | トランザクションの口がない | 残る | N37 |
| 15 | 葉をまたぐ原子性は書けない | 残る | N37 |
| 16 | パッケージの変数（`States`）を守るものがない | 解決 | P32: パッケージの変数がなくなった。状態の実装は `New` の引数 |
| 17 | テストを並列にできない | 解決 | P32: サーバーを作り直すサブテストを `t.Parallel()` にした（`app/handlertest/handlertest.go:140-141`、`app/storetest/storetest.go:76-77`）。N53 |
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
| 28 | 空の配列（入れ子の `nil` は `null`） | 残る | `app/leaves.go:94` で人が空の配列を作る |
| 29 | 例外の応答の形 | 残る | このサンプルで決めたまま。N48 |
| 30 | `http.BadRequest` が語彙にない | 残る | N44 |
| 31 | 500 の `internal` は FQN ではない | 残る | |
| 32 | 404 と 405 が JSON でない | 残る | |
| 33 | `datetime` のゾーンを人がそろえる | 残る | `app/leaves.go:58,95` |
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
| 45 | ステータスを持たない例外が、不具合と区別できない | 残る | `app/handlertest/handlertest.go:230-241` |
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

解決 19、残る 42、当たらない 2（P31、P32 で解決に変わったのは、16〜20 の五つ。P33 で変わったものはない）。

### v2 を書いて分かったこと

決定が決めていなかったので書きながら決めたこと、二つの決定が引っ張り合ったところ、理想のコードに決定が与えてくれなかったもの、v1 より悪くなったところ。それぞれ「何が起きたか → サンプルではどうしたか」。パスはこのディレクトリから。

N1〜N53 は、P1〜P30 を当てはめた形を書いたときのもの。その後の決定（直した P30、P31、P32）と仕様の直しで片づいたものは、番号を残して、見出しに「解決」と、何で解決したかを書く。N54〜N65 は、P31 と P32 を当てはめた形（実装をインターフェースで受け取る）を書いて分かったこと。N66〜N75 は、P33 を当てはめた形（`export` を付けた op だけが `Ops` に並ぶ）を書いて分かったこと。N76〜N86 は、期待する出力を文書の形にそろえ（P35 の、内側のフォルダと入口）、継ぎ目（P36〜P39）を入れて分かったこと。N87〜N96 は、決定 91〜102（プラグインは出力を推論しない。読み書きを `db` の語彙のデコレータで言う）を当てはめて分かったこと。N97〜N106 は、決定 103〜113（デコレータが出すコードは型に依らない。表、しまい方、要求のどこから読むか、実行時に決まる応答）を当てはめて分かったこと。

N1〜N96 のうち、解決は 16: N3（仕様の直し）、N11（直した P30）、N22（先にあった決定に合わせた）、N27 と N28（P31）、N10、N14、N17、N18、N53（P32）、N26（P33）、N71 と N72（P35）、N5 と N38（決定 96）、N39（決定 92）。ほかに、N36 は規則そのものがなくなり、N37 は「複数の op を一つのトランザクションにする」だけが、形を変えて解決した（どちらも見出しに書く。数には入れていない）。ほかに、N61 は前半だけが P33 で解決した（見出しに書く。数には入れていない）。N68 と N70 は、P35 で形が変わった（見出しに書く）。

#### 仕様

- **N1 `Found => toPublicUser(user)` は、先に束縛できない**。`user` が `type.User` になるのはアームの中だけ（P3）なので、写す呼び出しはアームに書くしかない（`server.hozo:108`）。レコードのフィールドに渡す方（`issueSession`）は、先に束縛した（`server.hozo:60-61`）。アームに書けるのが呼び出し一つまでなら、「絞り込んだ値を写してから、もう一つ呼ぶ」は、そのための配線の op を別に作ることになる。
- **N2 写す op を人が書く**（P27）と、**v1 より悪くなる**。v1 は基底が `UserToPublicUser` を出していた。v2 は三つのフィールドを人が写す（`app/leaves.go:56-57`）。`type.User` にフィールドを足すと、`type.PublicUser` は継承で同じフィールドを得るが、人が書いた `toPublicUser` は写さないままコンパイルが通り、零値が外へ出る。気づく仕組みはない。
- **N3（解決: 仕様に、人が書く葉 `requireFresh` を足した）セッションの期限を、誰も確かめなかった**。下書きの配線 `authenticate` は「セッションがあるか」だけで分けていた（`store.hozo:48-51`）。`newSession` は `expiresAt` を入れる（`app/leaves.go:51`）が、読む者がいなかったので、**期限切れのトークンで入れた**。v1 は葉が時計と比べていた（期限切れは 401）。仕様に書く方法として考えたのは、時計に依る述語を持つ分類（「`as` で絞り込む場合に、人が書く述語を足せるか」が決まっていない。P3）と、比較を持つ読む op（`go-sqlite` の規則で表せない。N37）。→ どちらでもなく、`server` に葉 `fn requireFresh(session: type.Session): type.Session throws type.Unauthorized;` を足し、`authenticate` の `LoggedIn` のアームから呼ぶ（`server.hozo:77-89`）。本体は人が書き、時計と比べる（`app/leaves.go:62-66`）。期限ちょうどから先は、どの入口でも 401（`app/handlertest` の `expired`）。残ること:
  - **書き方は「受け取った値を、そのまま返すか、投げる葉」**。アームに書けるのは呼び出し一つ（N1）なので、確かめるだけの op も、値を返す形にした。「期限が過ぎていない」は、説明のコメントと人の本体にしかない。
  - **`requireFresh` は時計を使うが、仕様の上では純粋**（N9）で、メソッドは `ctx` を取らない。
  - **切れたセッションは消えない**（v1 の 23）。`logout` も `authenticate` を通るので、切れたトークンでは 401 になり、その行を消せない。
  - **値のアーム（P14）を使うところが、この仕様からなくなった**。`LoggedIn => session;` が、ただ一つの値のアームだった。基底の規則には形を残した（「アーム」）が、期待する出力には現れない。
- **N4 三つの分類が同じ意味**。`Registration`、`Account`、`Login` は、どれも「`T?` が `T` である場合と、それ以外」（`store.hozo:36-51`）。P20 の `by nullable` と変わらず、違うのは場合の名前だけ（P21 の指針どおり）。その後、「それ以外」の側にも名前を付けた（残りのケース `Unregistered`、`Missing`、`LoggedOut`。「残りのケースで変えたこと」）。`match user by store.Registration` の `user` が `findUser` の結果でなければならないのか（P2 の「その op が返した値の分類」）は決まっていない。基底の出力は、どちらでも同じ（N23）。
- **N5（解決: 決定 96。`findSession` は `type.Token` を取り、トークンがない場合は `server` の分類 `Credential` が断る）`findSession(token: type.Token?)` は下書きのまま**（`store.hozo:28`）。受ける側を `T?` にしたので、`go-sqlite` に「省略可能な引数」の規則が要った（N38）。P20 で `in.token` を先に絞り込めば、`findSession(token: type.Token)` にでき、その規則は要らない。
- **N6 継承した model は、語彙のデコレータも受け継ぐ**。`type.PublicUser` のフィールドは `@db.key`、`@db.unique`、`@valid.length` を持ったまま（`type.hozo:41-43`）。外すことはできない（P25 は足すことだけ）。`go-valid` は `PublicUser` にも同じ検査を出す（`gen/type/validate.go:50-61`）。
- **N7 `logout` は、トークンがなければ 401**。下書きの配線が `authenticate` を通るため（`server.hozo:93-97`）。v1 は 204 だった。決定の帰結ではなく、下書きの仕様の変更（`app/handlertest/handlertest.go:272-274`）。

#### 純粋さと `ctx`（P29、P30）

- **N8 この仕様では「純粋な op」と「`server` で人が書く op」が同じ集合になった**。七つの葉がすべて純粋で、配線はどれも、純粋でない op（`store` の op か `mailer.send`）を呼ぶので純粋でない（「基底 `go` が従う規則」の表）。メソッドの形で「保存先に触れない」が分かる、という狙いどおり。P33 の後は、もう一つ重なる: `server` で `export` を付けた op は、`@http.*` を持つ五つの配線とちょうど同じで、残りの十（三つの配線と七つの葉）が内部。
- **N9 純粋と判定された op が、時計と乱数を使う**。`newSession`、`requireFresh`、`newOrder` は、仕様の上ではデコレータも `reads` も効果もないので純粋で、メソッドは `ctx` を取らない（`gen/server/inner/ops.go:114,125,135`）。本体は、今の時刻と新しい識別子を外から得る（`app/leaves.go:49,51,63,80,83`）。P30 の「関数の形を見れば、その op が状態や外部に触れるかが分かる」は、ここでは成り立たない。仕様に書く方法は、その op にデコレータを付けること（どのトランスパイラも読まない印でよい。`mailer.send` の `@remote` と同じ）で、付けなくても検査は通る。
- **N10（解決: P32）純粋な op も `error` を返していた**。`toPublicUser` の理想の形は `func(pkgtype.User) pkgtype.PublicUser` だが、P32 の前は、口が埋まっていないことを `*runtime.MissingError` で返すために `error` が要り、配線は、決して失敗しない写しの後でも `err` を確かめていた。P29、P30 は `ctx` だけを決めていて、`error` には触れていなかった。→ P32: 純粋で `throws` のない op は `error` を返さない。五つの葉（`newSession`、`toPublicUser`、`pageOf`、`newOrder`、`composeNotice`）は値だけを返し（`gen/server/inner/ops.go:104-140`）、配線は `publicUser := o.toPublicUser(user)` と書く（`gen/server/inner/wiring.go:38`）。「口が埋まっていない」は、実行のときには起きなくなった。この形で分かったことは N60。
- **N11（解決: 直した P30）外部の入出力を「純粋でない」と書く方法が、置き場しかなかった**。直す前のサンプルは「別の名前空間の op の口は、相手が純粋かどうかによらず `ctx` を取る」と決めていた。`mailer.send` は、宣言だけを見ればデコレータも `reads` も効果もない純粋な葉で、`ctx` を取るのは `server` から見た口だけだった（`mailer` に Go の単位を置けば、`ctx` を取らない `Send(notice)` が出て、口に入らなくなる形）。直した P30 は、純粋かどうかを op そのものの性質とし（どの名前空間から見ても同じ。口も相手の op の判定に従う）、別のプロセスや外部にある op には受け渡しのデコレータが付く、とした。→ `send` に、送る仕組みの語彙のデコレータ `@smtp.sendMail` を付けた（`mailer.hozo:7`。語彙は `smtp.hozo`）。`mailer.send` は純粋でなくなり、`server.MailerOps` の `Send` は `ctx` を取る（`gen/server/inner/ops.go:149`）。理由が「別の名前空間だから」から「相手が純粋でないから」に変わった。`mailer` に Go の単位を置いても、同じ判定で `ctx` を取る `Send(ctx, notice)` が出る。残ること: `smtp` の語彙を提供するプラグインがまだないので、サンプルの中で宣言している（今はどのトランスパイラも読まない印。プラグインができれば、`send` の中身を出せる）。付け忘れても検査は通り、op は純粋と判定されて `ctx` が消える（N9 と同じ）。`throws` があるので `error` は残る（P32）。生成したメソッドのコメントに、純粋でない理由（どのデコレータか）は出ない（`gen/server/inner/ops.go:146-149`）。
- **N12 宣言を一つ変えると、メソッドの形が変わる**。純粋な葉に、デコレータ（どのトランスパイラも読まない印でも）、`reads`、効果のどれかを足すと、その op のメソッド（`Leaves` か状態の単位と、`export` を付けた op なら `Ops`）に `ctx` が増える（`throws` がなかった op なら、`error` も増える）。配線なら、呼び出し先の一つが純粋でなくなるだけで同じことが起きる（伝わる）。人が書いた本体と、`Ops` を呼ぶ人のコードは、コンパイルが通らなくなる（気づける）。内部の配線の形が変わっても、人のコードには何も起きない（呼ぶのは、生成し直される配線だけ。P33）。別の名前空間のインターフェース（`<Ns>Ops`）のメソッドも、相手の op の判定に従うので同じように変わるが、相手の出力の `Ops` をそのまま渡している組み立ては、両方が同時に変わるので、そのまま通る（P31）。通らなくなるのは、人が書いた実装（`app.Mailer`、`app/memstore`）。`throws` を足したときのことは N60。
- **N13 純粋さの判定を使うのは、基底だけだった**。`go-http` が呼ぶ op は `@http.*` を持ち、`go-sqlite` が中身を出す op は `reads` か効果を持ち、形で決まる問い合わせは状態を読むので、どれも必ず `ctx` を取り、`error` を返す。相乗りのプラグインは、判定の結果を知らなくても正しい呼び出しとメソッドを書ける（`gen/server/http.go:40,55,69,92,118`、`gen/store/db.go:55-189`）。
- **N14（解決: P32 の後の `go-sqlite` の規則）`ctx` を受けない入口があった**。P32 の前の `SetupDB(db *sql.DB)` は `ctx` を取らず、表を作る文は中断できなかった。P29 が決めているのは op の関数だけ。→ `SetupDB` は、表を作ることと、実装を `States` に入れることの二つをしていた。後ろの方がなくなった（実装は、人が `New` に渡す）ので、残った方を `CreateTables(ctx context.Context, db *sql.DB) error` にした（`gen/store/db.go:40`。`app/onsqlite/store_test.go` の `TestCreateTables`）。
- **N15 基底が使う名前が増えた**。引数名 `ctx`、パッケージ名 `context` と `pkgtype`。仕様の op の引数が `ctx` という名前なら、末尾に `_` を付ける規則（`docs/emit-go.md`「名前」）に足すことになる。

#### 基底の形

- **N16 型を共有できるのは、共有の名前空間に置いた型だけ**。`gen/store` と `gen/storemem` の `Ops` が、どちらも同じ `server.StoreOps` を満たすのは、引数と戻り値の型がすべて `gen/type` のものだから（`app/both_test.go:30-36`）。`store` が自分の model を持てば、二つの出力は別々の Go の型を持ち、`server` が import するのは一方だけなので、もう一方の `Ops` は満たさない。「同じ名前空間を二つの単位で出すなら、境を越える型は共有の名前空間に置く」は、P5、P6 では指針だが、この組み方では守らなければ組めない。相手の `Ops` をそのまま渡せること（P31）も、同じ前提の上にある。
- **N17（解決: P32）何がパッケージの変数になるかは、名前空間の境で決まっていた**。P32 の前、`server` は状態を持たないので、すべてが `New(leaves, deps)` の値で、`store` は九つの op がすべて状態の単位にあり、`New()` は引数を取らず、実装はパッケージの変数 `States` にあった。→ P32: パッケージの変数はない。`server` も `store` も、実装はすべて `New` の引数（`gen/server/inner/ops.go:205`、`gen/store/ops.go:61`）。
- **N18（解決: P32）一つのプロセスに二つの `store`**。P32 の前は、二つの出力（`gen/store`、`gen/storemem`）が別のパッケージで、それぞれの `States` を持つので、同じ `server` を両方の上に同時に組み立てられた（今も: `app/both_test.go:112-139`）が、同じ出力の上に二つ（SQLite の DB を二つ、メモリの `Store` を二つ）は持てなかった: 準備が `States` を上書きした。→ P32: 同じ出力の `New` を何度呼んでもよい。二つの組み立ての `TestTwoServers` が、SQLite のデータベース二つ、`memstore.Store` 二つの上で、二つのサーバーに並行に要求を送る（`app/onsqlite/compose_test.go:53`、`app/onmem/compose_test.go:33`）。
- **N19 二つの出力は、どちらも `package store`**。パッケージ名は名前空間から決まり、`out` によらない。両方を import するファイルは、一方に別の名前を付ける（`app/both_test.go:23`）。片方だけでも、import パスの最後（`storemem`）と名前（`store`）が違う（`app/onmem/compose.go:11`）。
- **N20 パッケージ名 `pkgtype`**。共有の型の名前空間を、下書きのとおり `type` と名付けると、Go では `pkgtype.User`、ディレクトリは `gen/type`、import には名前を書くことになる（`gen/server/inner/types.go:5`）。利用者がパッケージ名を選ぶ方法はない。通信の上の例外の名前も `type.Unauthorized` になる（N48）。名前空間を `types` や `model` と名付ければ起きない。
- **N21 状態の単位に置く op**。「`reads` が一つの葉は、その状態の単位の問い合わせ」は、命令の規則（`docs/emit-go.md`「state」）と対に決まった（`gen/store/states.go:13-62`）。単位の中の並びは、形で決まる問い合わせ（N22）、op の問い合わせ、命令。決定が言っていない二つは、ここで決めた: 効果と `reads` の両方を持つ葉は効果の宛先の単位、複数の状態を読む葉はインターフェース `SharedState`（どの状態のものでもなく、実装するのは状態の持ち主なので、`Leaves` ではない。`New` の引数は、状態の単位の後ろの `shared`）。`SharedState` と `shared` という名前は、`shared` という名前の状態と当たる。
- **N22（解決: 形で決まる問い合わせは、いつも出す）状態を読む口（`IsEmpty`、`Get`）は、配線が読むときだけ出す、と決めていた**。生成したコードが呼ぶ口だけを出す、という規則で、帰結は「効果の結果を、読む op のない状態では確かめられない」だった: `notifications` には `addNotification` しかないので、テストは `go-sqlite` の表を SQL で直に数え、メモリの方は人がテストのための読む口を足していた。これは、先にあった決定（`docs/emit-go.md`「state」: 状態の単位は、配線が読まなくても、形で決まる問い合わせをすべて持つ。テストが効果の結果の判定に使う。`docs/decisions.md` の S6 も同じ向き）に反していたので、その決定に合わせて直した。→ 基底は、どの状態の単位にも `IsEmpty` と `Get` を出す（`gen/store/states.go:15-17`、`:58-60`）。`go-sqlite` と `memstore` がそれを実装し（`gen/store/db.go:178-185`、`app/memstore/memstore.go:90-108`）、テストは、その状態の単位の実装の `Get` で読む（`app/onsqlite/compose_test.go:40`、`app/onmem/compose_test.go:20`）。SQL を直に書くテストと、`memstore` の読む口はなくなった。直して分かったこと:
  - **`Ops` に載らない**。形で決まる問い合わせは op ではないので、テストは、`New` に渡した状態の単位の実装のメソッドを直に呼ぶ（`go-sqlite` の型は、同じ `db` から作り直した値でも同じ表を読む）。P32 の後は、実装していなければその型が状態の単位を満たさないので、「埋めていない口」はない。
  - **共通のテストへの渡し方**（解決: P32）。P32 の前は、二つの出力の `StateSet` が別の型で、共通のテスト（`app/storetest`）へは、口を一つずつ渡していた。今は、どちらの実装の型も、共通のテストのインターフェース（`storetest.List[T]`: `IsEmpty` と `Get`）をそのまま満たすので、`store.New` に渡したのと同じ値を渡す（`app/onsqlite/store_test.go:24-29`、`app/onmem/compose_test.go:44-48`）。
  - **`list<T>` の `Get`** は、`docs/emit-go.md`「state」の表にない（そこでは `list<T>` は `IsEmpty` だけ）。状態を名前で読むと `T[]` の値になる（P4、P22）ことから足した。
  - **`Get` は、すべての要素を返す**。件数や条件で絞る書き方はない（N37）。
- **N23 分類について、基底は何も出さない**、と決めた（規則は「基底 `go` が従う規則」の「分類」）。`store.Registration` などの名前は、`gen/server/inner/wiring.go:17,51,89` のコメントにだけ残る。残りのケースの名前も同じ（値がない側の枝の頭のコメント。「残りのケースで変えたこと」）。決まっていないこと: 絞り込む場合と、人が述語を書く場合が混ざった分類の形、人が書く述語をどのインターフェースに置き、`New` のどの引数で受け取るか（P32 は葉の op と状態の単位について決めた。P6 は分類の置き場を自由にした）。
- **N24 `match` は `nil` の検査になった**。`return match` はアームごとの `return`（`gen/server/inner/wiring.go:17-23`）、絞り込んだ値は使う位置で `*user`、`*session`（`:24`、`:58`、`:96`）。値を束縛する `match`（`const x = match …`）はこの仕様になく、形を決めていない。
- **N25 `throw` は例外を値で返す**（`gen/server/inner/wiring.go:23,57,95`）。v1 の「分かったこと」43（値かポインタか）は、生成したコードについては決まった。値を運ぶ例外の `throw` は、この仕様に出てこない（P13 の未定のまま）。
- **N26（解決: P33）`Ops` に、業務の途中の op がすべて載っていた**（P11）。P33 の前は、`Authenticate`、`IssueSession`、`Notify` と、七つの葉が、`login` などの入口と同じ `Ops` のメソッドだった。`go-http` の `NewHandler(ops Ops)` も十五のメソッドを受け取り、五つだけ呼んでいた。入口とそうでない op を分ける書き方が、仕様になかった。→ P33: 外から呼べる op に `export` を付ける。`server.Ops` は、`export` を付けた五つの入口だけ（`gen/server/ops.go:12-46`）。途中の十は、入口の `Ops` にはない（`app/both_test.go` の `TestOpsHoldExportedOpsOnly`）。その後（P35）: 途中の十を、`ops` の非公開のメソッドにする形はやめ、内側のパッケージ `gen/server/inner` の `Ops`（十五のすべて）に置いた（N68、N71）。P31、P32 の後の形での続きは N61、この形で分かったことは N66〜N75。
- **N27（解決: P31）`Deps` に入るのは、`Ops` のメソッドだけだった**。P31 の前、`server.StoreFindUserFunc` と `store.FindUserFunc` は、中身が同じでも名前の違う関数の型なので、`States` のフィールドは型を変換しなければ `Deps` に入らなかった（メソッドの値は名前のない関数の型なので、そのまま入った）。→ P31、P32: 関数の型も、フィールドに入れる行もない。相手の `Ops` の値を、そのまま渡す。
- **N28（解決: P31）組み立ては、口の数だけの行を人が書いていた**。P31 の前、`server.StoreOps` の八つのフィールドを一つずつ入れる行が、二つの組み立てに同じ形で二度あった。→ P31: `server.StoreOps` はインターフェースで、`store.New(…)` の戻り値をそのまま渡す（`app/onsqlite/compose.go:25`、`app/onmem/compose.go:21`）。`server.StoreOps` にない op（`addUser`）を `store.Ops` が余分に持っていても渡せ、足りなければコンパイルできない。「実装の単位は関数の型」（`docs/emit-go.md`「実装の単位」）は、人が実装するもの（葉の op、状態の単位）についても、P32 でインターフェースのメソッドになった。
- **N29 `runtime` の持ち主**。`go-valid` の出力と `go-http` の出力が使う `runtime.ValidationError` は、このリポジトリの `runtime` パッケージにある。本体が特定の言語を知らない（P9）なら、`runtime` は基底 `go` のもので、`ValidationError` は `go-valid` のものになるはずだが、置き場は決まっていない。P32 の後、**`runtime.MissingError` を使う出力は、この仕様にはない**（P32 の前は、`Kind` が `"leaf"`、`"state"`、`"dep"` のものを基底が返していた）。`runtime` には、ほかの `Kind`（人が書く分類の述語の `"classifier"`、デコレータの `"wrapper"`、制約の `"constraint"`）のためにも残っている。それらを P32 と同じにインターフェースで受け取るかは、この仕様に出てこないので書いていない（N23）。

#### `go-valid`

- **N30 単位をまたぐ依存**。`server` の `Validate` は `gen/type` のメソッドを呼ぶ（`gen/server/inner/validate.go:15-16,36,47`）ので、`type` の単位にも `go-valid` を並べなければコンパイルできない。P18 が言うのは「一つの単位の `transpilers` に依存をすべて書く」で、別の単位に何を並べるかは範囲の外。型の名前空間を出す Go の単位がない（別の言語、単位なし）ときは、メソッドでは書けず、使う側のパッケージに関数を出す形が要る。
- **N31 `go-http` は、`go-valid` の規則を知っている**。入力の model が `Validate` を持つか（`LoginInput` と `PlaceOrderInput` は持ち、`AuthInput` と `GetOrdersInput` は持たない）を、`go-http` が `go-valid` と同じ規則で決める（`gen/server/http.go:36,114` で呼び、`:54,68,91` では呼ばない）。「依存を宣言する」（P17）だけでは足りず、依存先の出力の形を再現している。v1 の「分かったこと」2 と同じ構図。
- **N32 依存がオプションで変わる**。`go-http` が `go-valid` に依存するのは `validate = true` のときだけ。P17 の依存は、オプション（P16）によらず宣言するのか、決まっていない。
- **N33 メソッド名 `Validate` が、フィールドと当たる**。model に `validate` というフィールドがあれば、Go では同じ型にフィールドとメソッド `Validate` ができ、コンパイルできない。
- **N34 誰も呼ばない `Validate`**。結果の型 `LoginOk` にも出る（`gen/server/inner/validate.go:34-41`）。補助の関数 `validAppend` は、パッケージごとに同じものが出る（`gen/type/validate.go:88`、`gen/server/inner/validate.go:56`）。
- **N35 `store` は制約を確かめない**。`gen/store` の単位は `go-valid` を並べず、`go-sqlite` は足す前に確かめない。`store.addUser` に、名前が空のユーザーを渡せば入る。確かめるのは HTTP の境だけ（P15 の「いつ確かめるかは、境界を持つプラグインが決める」のとおり）。

#### `go-sqlite`

- **N36（決定 92〜94 で、当たるかどうかを見る規則そのものがなくなった。今は、八つの op のすべてに、仕様がデコレータを付けている）規則は、この仕様の九つの op に足りた**（「`go-sqlite` が従う規則」の末尾）。人が `store` について書いたのは、組み立ての二つの文（表を作る `CreateTables` と、四つの型を `store.New` に渡す行。`app/onsqlite/compose.go:18-22`）だけ。
- **N37（「複数の op を一つのトランザクションにする」は、決定 94 で形を変えて解決: 一緒に行うものは、一つの op の `@db.effects` に並べる。ほかは残る）規則で表せないもの**。
  - **順序**: `listOrders` の「足した順」は説明のコメントにしかない（`store.hozo:30`）。規則が `ORDER BY rowid` と決めているだけで（`gen/store/db.go:155`）、「新しい順」は書けない。
  - **件数と位置**: ページで切るのは、すべて読んだ後の `pageOf`（`gen/server/inner/wiring.go:107-111`）。
  - **比較**: 「期限が今より後」。読む op では書けないので、読んだ後の葉 `requireFresh` で確かめる（N3）。
  - **一意の違反**: `addUser` で `email` が重なると、ドライバのエラーがそのまま返り、HTTP では 500 になる。仕様の例外に写す書き方がなく、`db.go` はドライバを import しないのでエラーを見分けられない（`app/storetest/storetest.go:96-101` は「エラーである」ことだけ、`app/onsqlite/store_test.go:179-181` は文言を見ている）。人が書き足せる場所はできた: `UsersTable` を埋め込んだ型で `AddUser` を置き換えれば、ドライバのエラーを見分けて写せる（N63）。写す先の例外は、仕様の `addUser` に `throws` がないので、ない。
  - **書き換え**（`put`）、**複数の状態にまたがる読み取り**: この仕様にないので、規則を決めていない。
  - **複数の op を一つのトランザクションにする**: `placeOrder` の `addOrder` と `addNotification` は別々の文（`gen/server/inner/wiring.go:125`、`:147`）。配線が op の間に渡すのは仕様の引数と `ctx` だけで、`ctx` に値を載せない（P29）ので、トランザクションを運ぶ道がない。v1 では一つの葉の中に書けた処理が、配線になった分、原子的に書ける範囲は狭くなった。状態ごとの型は、それぞれが `*sql.DB` を持つ（`*sql.Tx` を受け取る作り方はない）。
- **N38（解決: 決定 96。規則をなくした）省略可能な引数の規則は、下書きの書き方のために足した**（N5）。「値がなければ、問い合わせずに『ない』」（`gen/store/db.go:109-111`）。SQL の `= NULL` がどの行にも合わないのと同じ結果にした。
- **N39（解決: 決定 92。列は、仕様が `@db.find(users, "email")` のように名指しする）引数と列を、型で結ぶ**。`findUser(email: type.Email)` が列 `email` に、`listOrders(userId: type.UserId)` が列 `userId` に結ばれるのは、フィールドごとに別の scalar を宣言しているから。`name: string` のような核の型の引数は、`string` のフィールドが二つあれば（`type.User` の `name` と `hash`）結べない。引数の名前は見ていない（`findUserById` の引数は `id`、`listOrders` の引数は `userId` で、どちらも型は `type.UserId`）。
- **N40 `delete` で行が減らなくても、エラーにしない**（`removeSession`。`gen/store/db.go:127-130`）。組み込みの効果 `delete` は「列数が減る」変化だが、消す行がないときを区別する書き方はない。
- **N41 `go-sqlite` の公開の名前**。P32 の前は `Schema` と `SetupDB` だけだった（v1 の表の型 `Tables`、`Insert`、`Get` などは、人が呼ぶ理由がなくなったので出さなくなっていた）。P32 の後は `Schema`、`CreateTables` と、状態ごとの `<状態>Table`、`New<状態>Table`（この仕様で、合わせて十）。状態ごとの型が戻ったが、v1 の表の型とは違い、メソッドは基底の状態の単位のもの（形で決まる問い合わせと、仕様の op）だけで、`go-sqlite` が自分で決めた読み書きはない。仕様に op のない読み書きのうち、状態の値をすべて読むことは、形で決まる問い合わせでできる（N22）。それで足りないもの（条件を付けた読み取り、書き換え）は、人が SQL を書く。
- **N42 `@db.key`、`@db.unique`、`@db.ref` の意味は、`go-sqlite` の出力にしかない**。`gen/storemem` の単位では、どのトランスパイラも `@db` を読まない（印）。仕様は「`addUser` は、`email` が重なれば失敗する」と言っていないので、メモリの上の実装がそうするかは人が決める。→ キーと一意は合わせ（`app/memstore/memstore.go:18`、`:149-150`）、参照先の検査は合わせていない（`app/onsqlite/store_test.go:172-181` の `TestRef` は SQLite だけ）。「同じ `store` の二つの実装」が同じ振る舞いをすることは、仕様からは言えない。状態の単位のインターフェースが保証するのは、メソッドの形までで、振る舞いではない。
- **N43 プラグインごとの局所の名前**。状態ごとの型のメソッドの引数は、基底と同じ規則で仕様の引数名から作る（`email`、`token`）。`go-sqlite` が自分で使う名前（レシーバの `t`、`args`、`err`）と当たれば、基底と同じく末尾に `_` を付けることになるが、当たる名前の一覧はプラグインごとに違う。形で決まる問い合わせのメソッド名は、P32 の前は `go-sqlite` が決めていて（`UsersGet`）、同じ名前になる op（`usersGet`）と当たった。今は基底のメソッド名のまま（`Get`）なので、衝突は基底の側のものになった（N58）。

#### `go-http` と JSON の形

- **N44 op を呼ぶ前の失敗を、例外の形にそろえた**。P15 の「失敗は、そのプラグインの語彙の例外で伝える」に合わせ、`{"error":"http.BadRequest","value":{"message":…,"violations":[…]}}` とした（`gen/server/http.go:129-152`）。v1 は `{"error":"http.BadRequest","message":…}` で、例外の形（`error` と `value`）ではなかった。クライアントのトランスパイラは、ほかの例外と同じ規則で読める。**`http` の語彙には、`BadRequest` も、それが運ぶ値の model もまだない**（`vocab/http.hozo` にあるのは `ServerError` と `NetworkError`）。
- **N45 `http.BadRequest` を投げるかどうかが、設定で変わる**。制約の違反で投げるのは `validate = true` のときだけ。呼ぶ側の名前空間が計算する `throws`（`@http.post` の宣言が持つ例外）は仕様だけから決まるので、単位のオプション（P16）で増える例外を表せない。
- **N46 違反の位置を、通信の上の場所にした**。`Validate` の `Path`（`lines[1].quantity`）の先頭を、そのフィールドを読む場所に置き換える（`body[1].quantity`。`gen/server/http.go:37,115,193-202`）。v1 の「分かったこと」39 はこれで解決した。
- **N47 400 が 401 より先になる**。制約を確かめるのは op を呼ぶ前なので、トークンのない `POST /orders` でも、本文が制約に合わなければ 400（`app/handlertest/handlertest.go:210-213`）。v1 は葉が先に認証していた。順序を選ぶ書き方はない。
- **N48 例外の FQN が、型の置き場を通信に出す**。`{"error":"type.Unauthorized"}`（`gen/server/http.go:220,224`）。v1 は `server.Unauthorized`。例外を別の名前空間に移すと、クライアントが見る名前が変わる。

#### 名前の衝突

- **N49 プラグインが足す公開の名前**。基底: `New`、`Ops`、`Leaves`、`SharedState`、`<Ns>Ops`、`<状態>State`、`Effect<効果>`、`New<Model>`（P31、P32 でなくなったもの: `Deps`、`States`、`StateSet`、`<Op>Func`、`<Ns><Op>Func`）。`go-valid`: メソッド `Validate`（N33）。`go-sqlite`: `Schema`、`CreateTables`、`<状態>Table`、`New<状態>Table`。`go-http`: `NewHandler`。利用者の宣言と当たる例: model `Schema`、model `UsersTable`（状態 `users` の型と当たる）、既定値を持つ model `Handler`（基底の `NewHandler()` と `go-http` の `NewHandler` が当たる）、既定値を持つ model `UsersTable`（基底の `NewUsersTable()` と `go-sqlite` の `NewUsersTable` が当たる）。プラグインをまたぐ衝突を見つける者がいないのは、v1 の「分かったこと」48 のまま。その後: 基底に `Seams` が加わった（継ぎ目を持つ op があるときだけ）。サンプルの中のプラグイン `go-ratelimit` は `RateLimiter`、`RateSeams`、`NewRateSeams` を足す。当たる例: model `Seams`、既定値を持つ model `RateSeams`（基底の `NewRateSeams()` と当たる）。
- **N50 非公開の名前とファイル名**。前置きは `valid`、`db`、`http`、ファイルは `validate.go`、`db.go`、`http.go`。どれも、プラグインどうしの約束がないまま、当たらないように選んだだけ（v1 の「分かったこと」47、49）。その後: 標準の相乗りの分は、`docs/plugin-go.md`「相乗りのファイル」の表が約束になった。表にないトランスパイラ（`go-ratelimit` の `ratelimit.go` と前置き `rate`）は、表を避けて自分で選ぶ。プロジェクトの中のプラグインどうしが当たらないことを確かめる者は、今もいない（ファイルが当たれば CFG-5、名前が当たればコンパイルエラー）。

#### 設定とテスト

- **N51 今の hozoc は、この `hozo.toml` を読めない**。`[emit.options.go-http]` は、単位の知らないキー `options` として CFG-7 になり、その単位（`gen/server`）は読まれない。はじめは、`tests/samples_test.go` が、保留のあるサンプルに限り、このテーブルを除いた写しから単位を読み、`check` は、仕様（`.hozo`）だけを写して言語の検査だけを行っていた。その後（P36）: テストを最後の形にした。`hozo.toml` を書き換えて読む道はなくし、今の hozoc が読めない間は、単位が要るサブテスト（`files`、`transpilers/*`）を、読めない理由（CFG-7 の文言）を名指しして飛ばす。`check` は、足りないものがある間だけ、言語の検査に落とす（`../README.md`「テスト」）。読めないものは増えた: サンプルの中のプラグインのマニフェストの `requires` と `seams`（MAN-4）。
- **N52 「共有する形」と「トランスパイラのファイル」の区別がなくなった**（P9）。`gen/` のどのファイルも、どれかのトランスパイラのもの。`sample.json` の `select`（代役の規則）は書いていない。その後: `select` と `pending` は、`sample.json` から、書けるキーとしてもなくなった（`../README.md`「sample.json」）。
- **N53（解決: P32）テストは並列にできなかった**。`store.States` がパッケージの変数だったので、サーバーを作り直すサブテストは順に走らせていた。→ P32: サーバーも `store` も、作るたびに別の値で、何も共有しない。`app/handlertest` と `app/storetest` のサブテストと、その上のテストを `t.Parallel()` にした（`app/handlertest/handlertest.go:126-135`、`app/storetest/storetest.go:60-77`）。`go test -race` で通る。

#### 実装をインターフェースで受け取る形（P31、P32）

- **N54 `New` の位置引数は、状態と名前空間の数だけ並ぶ**。`store.New` は四つ（`gen/store/ops.go:61`）、`server.New` は三つだった（その後、継ぎ目の `seams` が最後に加わって四つ。`gen/server/inner/ops.go:205`。N79）。状態が二十あれば、引数は二十。Go では、呼ぶ側に引数の名前を書けないので、何番目がどの状態かは、型だけが頼りになる。
  - **取り違えは、型が違えばコンパイルエラー**。ただし、**メソッドの並びが同じ二つの状態**（同じ形、同じ要素の型で、どちらにも、それを読み書きする葉の op がない。例: `state drafts: list<type.Order>` と `state archive: list<type.Order>`）は、インターフェースの中身が同じになり、入れ替えて渡してもコンパイルが通る。フィールドに名前で入れる形（P32 の前）では、起きなかった取り違え。
  - **並びは宣言順**なので、仕様で状態の宣言を入れ替えると、`New` を呼ぶすべての行が変わる。状態か、呼ぶ名前空間を一つ足しても同じ（こちらは、渡し忘れをコンパイルエラーにする、という狙いどおり）。別の名前空間を辞書順にしたのは、配線の本体を書き換えただけで並びが変わらないようにするため。
  - → サンプルでは、四つを一行に並べた（`app/onsqlite/compose.go:22`、`app/onmem/compose.go:18`）。P32 の「フィールドは省けるが、引数は省けない」と引き換えに、名前を失った。
- **N55 `New` の引数の名前**。`leaves`、状態の名前、別の名前空間の名前を、そのまま引数の名前にした。当たる例: `leaves` や `shared` という名前の状態、自分の状態と同じ名前の別の名前空間（`mailer` という状態を持つ名前空間が、名前空間 `mailer` を呼ぶ）、Go の予約語の名前空間（`type` の op を呼ぶ名前空間の `New` は、引数 `type` を持てない）。基底の、末尾に `_` を付ける規則（`docs/emit-go.md`「名前」）に足すことになる。非公開の構造体 `ops` のフィールドも同じ名前を使う。P33 の後は、そのフィールドが、内部の op の非公開のメソッドとも当たる（N68）。この仕様では、どれも起きない。その後: 引数 `seams` が加わった。`seams` という名前の状態か、呼ぶ名前空間があれば当たり、後ろの `seams` の方に `_` が付く（引数の規則のまま）。内部の op の非公開のメソッドはなくなった（N68）が、継ぎ目を持つ op の中身のメソッド（`loginBody`）が、フィールドと当たりうる（GO-2）。
- **N56 `nil` は、書けば渡せる**。`server.New(nil, nil, nil, nil)` はコンパイルが通る（四つ目は、後で加わった継ぎ目の `seams`）。型のある `nil`（`(*app.Leaves)(nil)`）、プラグインの型の零値（`store.UsersTable{}`: `db` が `nil`）、フィールドを入れていない人の型（`&app.Leaves{}`: `Now` が `nil`）も同じ。`New` は確かめないので、その実装を使う op が呼ばれたところで panic になる（`app/server_test.go` の `TestNilIsNotChecked`: `nil` の `leaves` と、`nil` の `seams`）。使わない実装は `nil` のままでよく、テストは、それを使っている（内部の配線だけを呼ぶテストは、`seams` を渡さない。`app/inner_test.go` の `TestInternalWirings`）。`*runtime.MissingError` のような、見分けられるエラーにはならない。`net/http` のサーバーは、ハンドラの panic を受け止めて、その接続を切る（500 の `internal` は返らない）。P32 が防ぐのは「書き忘れ」で、「`nil` と書くこと」ではない。`New` が `nil` を確かめるなら、panic するか、`New` が `(Ops, error)` を返す形になる。→ 確かめない、とした（生成したコードから、実行のときの「ない」をなくす方を取った）。人の型のフィールド（`app.Leaves` の `Now` など）の入れ忘れには、P32 は届かない。
- **N57 葉の op は、すべて一つの値のメソッドになる**。`server.Leaves` は七つのメソッドを持つ一つのインターフェースで、`New` の引数は一つ。→ 一つの型 `app.Leaves` に七つを書いた（`app/leaves.go:22-88`）。分かったこと:
  - **外から何も受け取らない葉も、メソッドになる**。`checkPassword`、`toPublicUser`、`composeNotice` はレシーバを使わない（P32 の前は、ただの関数を口に入れていた）。
  - **人の型の名前と、葉のメソッド名が、同じ場所に入る**。`app.Leaves` のフィールド `NewOrderID` と、葉のメソッド `NewOrder` は一語違い（`app/leaves.go:28`、`:78`）。フィールドを `NewOrder` と名付ければ、コンパイルできない。仕様に葉を足すと、人の型にある同じ名前のフィールドや補助のメソッドと当たる。
  - **分けて書くには、埋め込む**。関心ごとの型（認証の葉、注文の葉）を一つの構造体に埋め込めば、メソッドが合わさって `server.Leaves` を満たす。二つの型が同じ名前のメソッドを持てば、渡す行が「あいまい」でコンパイルできない。葉の一部を、その語彙を読むトランスパイラが実装するとき（この仕様にはない）も、`go-sqlite` の状態の型と同じに、プラグインの型を人の型に埋め込むことになる。仕様の側に、葉をまとまりに分ける書き方（`Leaves` を二つのインターフェースにする単位）はない。
- **N58 標準の読み取り（`IsEmpty`、`Get`）と、op の名前が、同じインターフェースに入る**。状態の単位のメソッドは、形で決まる問い合わせと、その状態の葉の op。その状態を読む op が `get` か `isEmpty` という名前なら、同じ名前のメソッドが二つになり、生成したコードがコンパイルできない（P32 の前の、構造体のフィールドでも同じだった。避ける規則は、ないまま）。`Ops` には形で決まる問い合わせを載せないので、そちらは当たらない。もう一つ: どの状態の単位も `Get` を持ち、戻り値の型が状態ごとに違うので、**一つの型は、要素の型が違う二つの状態の単位を兼ねられない**。→ `go-sqlite` の型は状態ごとに一つ（四つ）、`memstore` も、一つの `Store` の上に、状態ごとの型を四つ書いた（`app/memstore/memstore.go:42-63`）。要素の型が同じ状態どうしなら兼ねられるが、そのときは N54 の取り違えが起きる。
- **N59 テストが op を一つだけ差し替えるには、型を一つ書く**。P32 の前は、`server.StoreOps{FindSession: func(…) {…}}` と、差し替える関数だけをフィールドに入れればよかった（入れなかった口は、呼ばれたら `*runtime.MissingError`）。今は、インターフェースを満たす型が要る。→ 二つの書き方を使った。
  - **インターフェースを埋め込み、書きたいメソッドだけを書く**。`seenStore` は `nil` の `server.StoreOps` を埋め込み、二つのメソッドを書く（`app/server_test.go:71-84`）。書いていない op が呼ばれれば、`nil` のインターフェースの呼び出しで panic になる。生成したコードからなくした「実行のときの、ない」が、テストの中にだけ、人の選びで戻る。`server.Ops` のスパイ（`spyOps`）は、前からこの形だった。
  - **本物の実装を埋め込み、一つだけ置き換える**。`fixedNotice` は、`server.Leaves` を満たす値を埋め込んで `ComposeNotice` を置き換える（`app/server_test.go:128-131`）。`humanUsers` も同じ形（N63）。
  - Go では、メソッドを関数の中に書けないので、型はテストの関数の外に置き、テストごとの値はフィールドで渡す（`seenStore.seen`、`humanUsers.added`）。クロージャ一つで済んでいたものが、型とフィールドとメソッドになる。
- **N60 `throws` を足すと、メソッドの形が変わる**。純粋で `throws` のない op に `throws` を足すと、戻り値に `error` が増える（`NewSession(user) Session` が `NewSession(user) (Session, error)` になる）。`Leaves` のメソッドと、人が書いた本体が、コンパイルできなくなる（気づける）。`export` を付けた op なら、`Ops` のメソッドと、それを呼ぶ人のコード（`x := ops.<Op>(…)`）も同じ。配線と、`ops` の非公開のメソッドは、生成し直される。最後の `throws` を外せば、逆のことが起きる。N12 の `ctx` と合わせると、op の形は三通り（純粋でない / 純粋で `throws` がある / 純粋で `throws` がない）で、仕様の一語で行き来する。分かったこと:
  - **`error` のない葉には、失敗を返す道がない**。`newSession` と `newOrder` は、トークンと識別子を作る。main では乱数（`crypto/rand`）で、読めなければ panic にしている（`app/cmd/shop-sqlite/main.go:70-73`）。「純粋で `throws` がない」は仕様の上の性質で、本体が本当に失敗しないことは、誰も確かめない（N9 と同じ構図）。失敗を返したければ、仕様に `throws` を書く（業務の例外ではない失敗のための、ステータスを持たない例外を宣言することになる）。
  - **純粋な配線**は、この仕様にない（配線は八つとも、純粋でない op を呼ぶ）。`error` を返さない配線の本体（`return o.<Op>(…)` が `error` なしで続く形、`match` の `throw` のない形）は書いていない。
  - **戻り値がなく、純粋で、`throws` もない op** は、引数を受け取って何も返さないメソッドになる。意味のある本体は書けないが、検査は通る。
- **N61（前半は解決: P33）`Ops` は、業務の途中の op をすべて載せていた**（N26 の続き）。P33 の前、`server.Ops` の十五のメソッドのうち七つは、`New` に渡された `leaves` の同じ名前のメソッドを呼ぶだけで、組み立てた人は、その値をすでに持っていた。P31 で、別の名前空間が使う op は、呼ぶ側が自分のインターフェースで言うようになった（`server.StoreOps` は `store` の九つのうち八つ）ので、`Ops` が「すべて」である理由は「op はすべて出る」（P11）だけだった。重さが出たのは `go-http`: `NewHandler(ops Ops)` は十五のメソッドを求めて、五つだけ呼んでいた。→ P33: `Ops` は `export` を付けた op だけ。この仕様では、それが `@http.*` の五つと同じなので、`NewHandler(ops Ops)` は、呼ぶ op ちょうどを受け取る。入口とそうでない op を分ける書き方も、仕様にできた（`export`）。
  - **残る後半: `go-http` は、自分が呼ぶ op だけのインターフェースを持たない**。`NewHandler` が受け取るのは、今も基底の `Ops`。`export` を付けたが `@http.*` のない op（別の名前空間や人のコードだけが呼ぶ op。`store.addUser` のようなもの）が `server` にあれば、`NewHandler` は、呼ばないメソッドも求める。`go-http` が、呼ぶ op だけのインターフェースを出して受け取る形（P31 と同じ考え）にすれば、`New` の戻り値はそのまま渡せて、スパイの書き忘れはコンパイルエラーになる。→ `NewHandler(ops Ops)` のままにした。スパイは、今も `server.Ops` を埋め込んで、数える二つだけを書く（`app/server_test.go:177-191`。N59 の一つ目の書き方。五つなら、すべて書くこともできる）。
- **N62 同じ名前空間を二つの単位で出す理由が、薄くなった**。P32 の前は、実装がパッケージの変数に入ったので、SQLite とメモリを一つのプロセスに持つには、出力を二つ（`gen/store`、`gen/storemem`）に分けるしかなかった。今は、実装は `New` の引数。二つの出力の状態の単位は、中身が同じインターフェースなので、`gen/store` の `New` に `memstore` の型を渡せる（`app/both_test.go:38-39`）。一つの `store` の中で、状態ごとに別の実装を混ぜることもできる（`users` は SQLite、ほかはメモリ。`app/both_test.go:144-152`）。`gen/storemem` に残る意味は、`go-sqlite` のファイル（`db.go`）と `database/sql` の import を持たないパッケージであること。→ 二つの単位は残した（基底のファイルが、単位に並べたほかのトランスパイラによらないことを、`app/both_test.go` の `TestStoreOutputsShareBaseFiles` が確かめる）。混ぜたとき、`@db.ref` の `REFERENCES` は SQLite の表の間でしか効かない（N42）。
- **N63 プラグインが出す型の形は、ここで決めた**。P32 が言うのは「プラグインは、機械的に決まる op を実装した型を出し、足りない op は人が書き足す」まで。決めたもの: 名前（`<状態>Table`、`New<状態>Table`）、値の型でありポインタではないこと、作るのが関数であること（`db` を渡し忘れられない。零値は書ける: N56）、すべて実装できた型に出す `var _ <状態>State = …`。足りない op があるときに分かったこと（見立ての型で確かめた。`app/onsqlite/store_test.go:94-117`）:
  - **足りないことを人が知るのは、`New` に渡した行のコンパイルエラーから**（`missing method AddUser`）。プラグインがどの op を実装しなかったかの一覧は、どこにも出ない。足りない型には `var _` の行がない、という違いだけがある。
  - **書き足す人の型は、同じ `*sql.DB` を自分でも持つ**。プラグインの型の `db` は非公開で、読み書きの補助（`dbScanUser`、`dbArgsUser`）も非公開なので、人の SQL は列の並びを書き直す（`app/onsqlite/store_test.go:104-108`）。
  - **置き換えもできる**。すべて実装した型でも、埋め込んで一つのメソッドを書けば、そのメソッドだけが人のものになる（一意の違反を見分ける `AddUser` など。N37）。プラグインの型は、埋め込まれることを前提にした形になっている。
- **N64 表を作り忘れても、コンパイルは通る**。`go-sqlite` の出力を使う手順は二つ（`CreateTables` を呼ぶ、型を `New` に渡す）で、P32 がコンパイルエラーにするのは後ろだけ。`CreateTables` を呼ばなければ、はじめの問い合わせが「表がない」というドライバのエラーになる。型を作る関数が `ctx` を取って表も作る形（`NewUsersTable(ctx, db) (UsersTable, error)`）なら忘れられないが、作るたびに文を実行し、`error` が増え、`store.New(…)` の引数に直に書けなくなる。→ 分けたままにした（表を作るのは、起動のときに一度）。移行の形がないこと（v1 の 59）とも、つながる。
- **N65 op の説明が、インターフェースごとに重なる**。P32 の前は、op の説明と `Throws:` などは、op ごとの関数の型（`<Op>Func`）のコメントに一度と、`Ops` のメソッドにあった。関数の型がなくなったので、その op のメソッドを宣言するところごとに書く: `store.findUser` は `store.Ops`、`store.UsersState`、`server.StoreOps` の三か所。P33 の前は、`checkPassword` も `Ops` と `Leaves` に同じコメントがあった。→ `Ops`、`Leaves`、`<Ns>Ops` には op の説明を、状態の単位には、問い合わせか命令かの一行を書いた。`Ops` が `Leaves` を埋め込めば重なりは減るが、`Ops` のメソッドの並びが宣言順でなくなる。**P33 の後、`server` の重なりは減った**: 七つの葉は内部になり、説明は `Leaves` の一か所だけになった。**その後（P35）、また重なった**: 内側の `Ops` は内部の op も持つので、七つの葉の説明は、内側の `Ops` と `Leaves` の二か所（`gen/server/inner/ops.go:26`、`:105`）、`export` を付けた配線の説明は、入口の `Ops`、内側の `Ops`、配線の本体の三か所にある。重なるのは、`export` を付けた op（`Ops` のメソッドと、配線の本体。`export` を付けた葉なら、`Ops` と、`Leaves` か状態の単位）。内部の配線は、逆に、説明が見えにくくなった（N72）。

#### `export` と内部の op（P33）

- **N66 `export` を付ける理由になる「外の呼び手」は、三種類あった**。P33 の言い方は「別の名前空間から呼べる op」だが、この仕様で `export` を付けた十五のうち、仕様の別の名前空間が呼ぶのは九つ（`store` の八つと `mailer.send`）だけ。`server` の五つを呼ぶのは `go-http` のハンドラ（プラグインの出力）、`store.addUser` を呼ぶのは人が書くコード（main とテスト）で、どちらも仕様には現れない。基底の形では、`export` は「`Ops` に載る」と同じ意味になり、`Ops` を受け取る者は、みな外の呼び手。→ `addUser` は `export` にした（「`export` を付けた op」）。帰結:
  - **「`export` を付けたのに、どの名前空間も呼ばない」を、検査は言えない**。呼び手が仕様の外にいるかどうかは、仕様から分からない。六つ（`server` の五つと `addUser`）がそうなっている。
  - **人のコードのためだけに `export` を付けると、仕様の別の名前空間からも呼べるようになる**。「人のコードには見せるが、仕様の配線からは呼ばせない」は書けない。`addUser` は、`server` の配線からも呼べる。
- **N67 HTTP の入口には、`@http.*` と `export` の二つを書く**。五つの入口は、どれも `@http.post` / `@http.get` と `export` を並べて持つ（`server.hozo:41-43` など）。デコレータは「どう呼ばれるか」、`export` は「外から呼べるか」で、言っていることは別だが、`@http.*` を付けて `export` を付けない op に意味はない。→ `go-http` の規則にした: `export` のない op に `@http.get` / `@http.post` が付いていれば、プラグインが error を報告する（「`go-http` が従う規則」）。考えた別の形は二つ。(a) 黙って飛ばす: 仕様に書いた経路が、エラーなしにサーバーから消えるので取らなかった。(b) `@http.*` が付いていれば `export` と見なす: 語彙のデコレータが、本体の検査（別の名前空間から呼べるか）の結果を変えることになるので取らなかった。残ること: 付け忘れが分かるのは、`go-http` を並べた単位を emit したとき（または、そのプラグインの `check`）。`hozoc build --no-emit` の本体の規則ではないので、`go-http` を使わない設定では、何も言われない。
- **N68（P35 で変わった）内部の op の名前は、ここで決めた**。P33 が言うのは「`Ops` には `export` の付いた op だけが並ぶ」までで、内部の op を Go でどう持つかは言っていない。→ 構造体 `ops` の非公開のメソッドにし、名前は op の名前の先頭を小文字にしたもの（規則は「基底 `go` が従う規則」の「内部の op」）。分かったこと:
  - **非公開のメソッドにしたので、パッケージの外からは、どうやっても呼べない**。考えた別の形は、「公開のメソッドのまま、`Ops` から外すだけ」（`o.Authenticate`）。配線の書き方は P33 の前と変わらないが、`New` の戻り値を、外で宣言したインターフェースに型アサーションすれば呼べる（`ops.(interface{ Authenticate(…) … })`）。非公開のメソッドなら、それも通らない（そのときの `app/both_test.go` が確かめていた）。
  - **`export` を付け外しすると、メソッドの名前が変わる**（`Notify` と `notify`）。`Ops` を呼ぶ人のコードは、外した op を呼んでいればコンパイルできなくなる（気づける）。その op を呼ぶ別の名前空間は、検査でエラーになる。生成したコードの中では、その op を呼ぶすべての配線の行が変わる。
  - **Go の予約語と当たるのは、P33 から**。op の名前は小文字で始めるので、`go`、`select`、`range`、`import`、`default` のような名前の内部の op は、そのままではメソッドにできない（先頭を大文字にする `export` の op では起きない）。→ 末尾に `_`（`docs/emit-go.md`「名前」の規則のまま）。同じ op が、`export` を付ければ `Select`、外せば `select_` になる。
  - **`ops` のフィールドと当たる**（N55 の続き）。フィールドは `New` の引数の名前（`leaves`、`shared`、状態の名前、呼ぶ名前空間の名前）なので、名前空間 `mailer` を呼ぶ `server` に、内部の op `mailer` や `leaves` があれば当たる。→ メソッドの方に `_` を付ける、とした。パッケージの名前（プラグインの `httpWriteJSON` など。N50）とは、メソッドなので当たらない。
  - **先頭の大文字・小文字だけが違う二つの op**（`foo` と `Foo`）は、P33 の前から GOEMIT-2（先頭を大文字にした名前で比べる）。一方が内部なら、`ops` のメソッドは `Foo` と `foo` で当たらなくなるが、`Leaves` と状態の単位では、内部の op も公開の名前になるので、この規則は `export` によらず、そのままにする。
  - **その後（P35）: 非公開のメソッドにする形は、やめた。** 内部の op も、`ops` の公開のメソッド（`Authenticate`）で、内側のパッケージ `gen/server/inner` の `Ops` に並ぶ。入口のパッケージの `Ops` には並ばない（`docs/plugin-go.md`「内側のフォルダと入口」）。この項目の下の四つ（付け外しで名前が変わる、予約語、フィールドとの衝突、`foo` と `Foo`）は、起きなくなった: メソッドの名前は、`export` によらず、op 名の先頭を大文字にしたもの。「パッケージの外からは、どうやっても呼べない」は、手放した: `inner` を import すれば呼べる。別の名前空間から呼べないことを守るのは、Hozo の検査（NS-5）。`export` を付け外しして変わるのは、入口の `Ops` のメソッドの有無と、内部の op が一つもなくなれば、内側のフォルダの有無（N76）。
- **N69 「内部」が隠すのは配線だけで、人が書く葉は、Go の上では公開のまま**。P33 は「内部の op も、人が書くものは実装を受け取るインターフェースに並ぶ」と決めている。`server.Leaves` の七つは内部の op だが、人が別のパッケージで実装するので、メソッドは公開の名前（`CheckPassword`）で、`app.Leaves` の公開のメソッドになる。実装の値を持つ者は、誰でも呼べる: 葉のテストは `leaves.CheckPassword(…)` と直に呼ぶ（`app/leaves_test.go:66`）。→ 矛盾ではない、と読んだ: 仕様の「内部」は「別の名前空間の配線から呼べず、`Ops` に載らない」で、「その本体を、パッケージの外の Go のコードが実行できない」ではない。本体は人のもので、基底は、それを `Ops` から配らないだけ。ただし:
  - **`Leaves` を見ても、どの op が内部かは分からない**。`export` を付けた葉（この仕様の `server` にはない）も、同じ `Leaves` に、同じ形で並ぶ。メソッドのコメントに「内部」とは書いていない。
  - **状態の単位でも同じことが起きる**（この仕様では、`store` の九つがすべて `export` なので、起きていない）。`store` の内部の葉は、`store.Ops` からは消えるが、`<状態>State` のメソッドと、`go-sqlite` が出す公開の型のメソッドには残る: `store.NewUsersTable(db).FindUser(ctx, …)` は、どのパッケージからも呼べる。`store` の op を内部にしても、保存先を読み書きする口は閉じない。
  - **テストと、人のコードには、それが抜け道になる**。`addUser` を内部にして、main が `store.NewUsersTable(db).AddUser` を呼ぶ形でも動く（そうしなかった理由は「`export` を付けた op」）。
- **N70（P35 で変わった）葉を呼ぶだけの非公開のメソッドが、七つ残った**。P33 の前、`ops` の `CheckPassword` などは、`Ops` を満たすために要った。今は、`o.checkPassword(…)` を呼ぶのは配線だけで、中身は `o.leaves.CheckPassword(…)` の一行（`gen/server/inner/ops.go:222-247`）。配線が `o.leaves.CheckPassword(user, password)` と直に呼べば、七つのメソッドは要らない。→ 残した。配線の本体の規則が一つで済む（自分の名前空間の op は、いつも `ops` のその op のメソッド。呼び出し先が葉か配線か、`export` があるか、実装が `Leaves` か状態の単位か `SharedState` かで、本体の書き方が変わらない）方を取った。直に呼ぶ形にするなら、`export` を付けた葉（`Ops` を満たすために、`ops` のメソッドが要る）だけが、二通りの呼び方を持つことになる。
  - **その後（P35）**: 七つは公開のメソッド（`o.CheckPassword(…)`）になり、内側の `Ops` を満たすために要るものに戻った。テストは、内側の `Ops` から、葉も直に呼べる。配線の本体の規則が一つで済むことは、変わらない。
- **N71（解決: P35）内部の配線は、単独では試せない**。P33 の前、テストは `ops.Notify(ctx, order)` と呼び、`notify` が通る二つの op（`mailer.send`、`store.addNotification`）だけを書けばよかった。今は、`notify` に届く入口 `placeOrder` を呼ぶしかなく、そこまでの道が通るもの（`store.findSession`、葉 `requireFresh` と `newOrder`、`store.addOrder`）も用意する（そのときの `app/server_test.go` の `TestReplaceOneLeaf`。テストの型のメソッドは二つから四つに増えた）。`authenticate` の振る舞い（トークンなし、知らないトークン、期限切れ）は、P33 の前から、四つの入口を通して確かめている（`app/handlertest` の `expired` など）。分かったこと:
  - **同じパッケージにテストを置く、という Go の普通の手が使えない**。`gen/server` は単位の `out` で、そこにあるファイルは hozoc のもの（記録 `.hozoc-outputs.json`）。人の `_test.go` を置く場所ではない。
  - **内部の配線の、メソッドの形**（`ctx` と `error` の有無）は、外から書けない。P33 の前は、`server.Ops.Authenticate` のメソッド式で確かめていた。今、確かめているのは `export` を付けた五つと、葉と、別の名前空間の op だけ（そのときの `app/server_test.go` の先頭の `var`）。内部の配線の形は、人のコードの誰にも見えないので、確かめる理由もなくなった、とも言える。
  - **`nil` を渡したときの panic**（N56）も、入口から届かせる: `TestNilIsNotChecked` は、ユーザーがいつもいる `store` を書いて、`login` を葉 `checkPassword` まで進める（そのときの `TestNilIsNotChecked`）。
  - 隠したのだから、入口を通して試すのは狙いどおり。重さは、入口から遠い配線ほど大きくなる。
  - **→ P35: 内部の op を持つ出力を、内側のフォルダ `inner` と入口に分け、テストは内側を参照する。** 内側の `Ops` は、すべての op を公開のメソッドに持つ。`TestReplaceOneLeaf` は、`inner.New(…).Notify(ctx, order)` と呼ぶ形に戻った（テストの型のメソッドは二つ。`app/server_test.go:156-173`）。内部の配線の形も、メソッド式で書ける（`inner.Ops.Authenticate`。`app/server_test.go:47-50`）。`authenticate` と `issueSession` を単独で試すテストを足した（`app/inner_test.go` の `TestInternalWirings`）。`nil` の `leaves` の panic は、葉を直に呼んで確かめる。同じパッケージにテストを置けないことは変わらないが、要らなくなった。
- **N72（解決: P35）内部の配線の説明は、非公開のメソッドにしかない**。P33 の前、`authenticate` の説明と `Throws:`、`Reads:` は、`Ops` のメソッドのコメント（公開。`go doc` に出る）と、配線の本体の二か所にあった。今は、`wiring.go` の非公開の構造体の、非公開のメソッドのコメントだけ（そのときの `gen/server/wiring.go`）で、`go doc` には出ない。読むには、生成したファイルを開く。内部の葉の説明は、`Leaves`（公開）にある。→ そのままにした（外から呼べないものの説明を、外に出す理由がない）。残ること: `export` を付けた op のコメントの `Throws:`、`Reads:`、`Effects:` は、内部の op から伝わってきたものを含む（`PlaceOrder` の `Effects: store.createOrder, store.notifyUser` の後ろは、内部の `notify` のもの）が、どの内部の op を通ったかは、公開のコメントからは分からない。仕様を読めば分かる。
  - **→ P35**: 内部の配線の説明と `Throws:`、`Reads:` は、内側の `Ops` のメソッドのコメント（公開。内側のパッケージの `go doc` に出る）と、配線の本体にある（`gen/server/inner/ops.go:42-46`、`gen/server/inner/wiring.go:42-46`）。入口のパッケージの `go doc` には、今も出ない（外から呼べないものの説明を、入口に出す理由がない）。
- **N73 内部の op の分類を、外から使えるか**。P33 は、分類を、今までどおりどこからでも参照できるものにした。分類は op について宣言する（`classify Registration of findUser`。`store.hozo:36-39`）ので、**その op が内部でも、分類の名前は外から書ける**。この仕様の三つの分類は、どれも `export` を付けた op のもので、`server` は、自分で呼んだ `store.findUser` の結果を `by store.Registration` で分けている（`server.hozo:46-47`）。`findUser` が内部だったら:
  - `server` は `findUser` を呼べないので、その結果を持てない。`match x by store.Registration` の `x` が `findUser` の結果でなければならないなら（N4 の、決まっていない点）、内部の op の分類は、外からは**参照できるが、使えない**。
  - `store` の `export` を付けた配線が、内部の `findUser` の結果をそのまま返すとき、その値を外で `by store.Registration` で分けられるか（値の出どころが、配線を通って伝わるか）も、N4 と同じく決まっていない。
  - 分類の宣言 `Registered: findUser as type.User;` は、内部の op の名前と戻り値の型を、外から読めるところに書く。
  - 基底は、この形の分類について何も出さない（N23）ので、Go のコードは、どちらに決まっても変わらない。人が述語を書く分類（基底が判定関数を出す形）なら、内部の op の分類の関数を、公開の名前で出すかどうかを決めることになる。
- **N74 すべての op が `export` の名前空間では、`export` は九回書くだけの語**。`store` の op は、ほかの名前空間に読み書きさせるためにあるので、九つとも `export`（`store.hozo:22-83`）。`mailer` も、一つの op が `export`。内部の op があるのは、配線を持つ `server` だけ。既定を逆（付けなければ外から呼べる。隠したいものに印）にすれば、`store` と `mailer` には何も書かずに済むが、`server` の十に印が要る。P33 は「付けたものだけが外から見える」を取っていて、付け忘れは、呼ぶ側の検査のエラーか、人のコードのコンパイルエラーで分かる（外に見せすぎる方の誤りは、気づけない）。関連して:
  - **単位のない名前空間（`mailer`）の `export`** が効くのは、検査だけ。`server.MailerOps` は、P33 の前と同じ。
  - **`export` を付けた op が一つもない名前空間**は、`New` が空の `Ops` を返す、と決めた（「基底 `go` が従う規則」）。外から何も呼べないパッケージに意味があるのかは、この仕様に出てこないので考えていない（相乗りのプラグインが、`Ops` を通さずに入口を作る形があれば、意味がある）。
- **N75 呼ばれない内部の op を、検査が言えるようになった**。P33 の前は、どの op も `Ops` にあり、仕様のどこからも呼ばれていない op も、人のコードが呼んでいるかもしれなかった。今は、内部の op を呼べるのは、同じ名前空間の配線だけなので、**どの配線からも呼ばれない内部の op は、誰にも呼べない**。配線なら、使われないメソッドが出る（P35 の後は、内側の `Ops` の公開のメソッド。テストだけが呼べる）。葉なら、人は、誰も呼ばない op の本体を `Leaves` に書かされる（書かなければコンパイルできない）。この仕様の十の内部の op は、どれも呼ばれている。検査の規則（warning）にできるが、P33 は決めていない。

#### 内側のフォルダと入口（P35）、継ぎ目（P36〜P39）

- **N76 `export` の付け外しで、定義のパッケージが動く**。内部の op が一つでもあれば、出力は内側 `inner` と入口に分かれ、一つもなくなれば、分かれなくなる。この仕様では `server` だけが分かれる。入口の名前（`server.Ops`、`server.LoginInput`、`server.New`）は変わらないので、`app/` の、テストでないコードは、どれも入口だけを import している。動くもの: 型の本当の名前（`%T`、コンパイルエラーの文言は `inner.LoginInput`）、`go-valid` の `validate.go` の置き場（`gen/server/inner/validate.go`。型にメソッドを足すので、定義のパッケージ）、`inner` を import するテスト（内部の op がなくなれば、パッケージごとなくなって、コンパイルできない）。最後のものは、狙いどおり: 内部の op を試すテストは、内部の op がなくなれば要らない。
- **N77 入口の型は別名なので、同じ型に名前が二つある**。`server.LoginInput` と `inner.LoginInput` は同じ型で、どちらで書いても通る。`go-ratelimit` の出力は入口のパッケージにあるので入口の名前で書き、テストが書く継ぎ目（`app/inner_test.go` の `through`）は内側の名前で書いた。どちらも `inner.Seams` を満たす（`app/server_test.go:57-63`）。人が読むときに、二つが同じものだと分かるのは、`gen/server/types.go` の `type LoginInput = inner.LoginInput` を見たとき。
- **N78 入口の `New` が返す値は、内部の op も持っている**。入口の `New` は、内側の `New` の戻り値を、入口の `Ops`（五つ）として返す。値そのものは十五のメソッドを持つので、`ops.(inner.Ops)` と型アサーションすれば、内部の op を呼べる。→ そのままにした（文書の決定: 外から呼べなくすることは目的にしない）。`TestOpsHoldExportedOpsOnly` は、インターフェースのメソッドだけを見る形に直した。P33 の後の形（N68）で確かめていた「型アサーションでも取り出せない」は、確かめなくなった。
- **N79 継ぎ目は、`New` の引数を一つ増やす**。継ぎ目を宣言するトランスパイラを単位に足すと、`New` を呼ぶ行（二つの組み立てと、`server.New` を直に呼ぶテスト）が、どれもコンパイルできなくなり、`seams` を渡すように直す。`Ops` を使うコード（`go-http` の出力、main）と、葉の実装（`app/leaves.go`）は、変わらない。逆向きも気づける: 単位から `go-ratelimit` を外すと、`Seams` と四つ目の引数がなくなり（語彙 `rate` も読まれなくなるので、その前に `@rate.limited` が解決できない）、仕様から `@rate.limited` を消すと、`ratelimit.go` が出なくなって `server.NewRateSeams` がなくなる。どちらも、組み立ての行がコンパイルできない。「外したのに、掛かっていると思い込む」は、この単位では起きない（起きる場合は N81）。
- **N80 継ぎ目の内側だけを試すには、素通しの継ぎ目を書く**。中身のメソッド（`loginBody`）は非公開で、どのインターフェースにもない。テストは、`body` をそのまま呼ぶ `inner.Seams`（`app/inner_test.go:102-107`）を書いて `inner.New` に渡し、外側の `Login` を呼ぶ。継ぎ目が一つなら、型は一つ、メソッドは一つ。継ぎ目を持つ op が二十あれば、素通しの型のメソッドも二十（op ごとに型の付いたメソッドなので、まとめて一つには書けない）。→ そのままにした。基底が素通しの実装を出す（`PassSeams` のような型）ことは、決めていない。
- **N81 同じ名前空間の別の単位では、同じデコレータが、黙って印になる**。継ぎ目は単位ごとのもの（`docs/config.md`「継ぎ目」）。この仕様の `store` のように、同じ名前空間を二つの単位で出力していて、`go-ratelimit` を一方にしか並べなければ、もう一方の出力の `login` は、制限なしで動く。warning はなく（P37: 読む者がなければ、何も出ない）、`Ops` のコメントには、どちらの出力でも `Throws: … rate.LimitExceeded` と出る（`Signature.exceptions` は仕様だけで決まる）。この仕様では、`server` の単位は一つなので起きない。どのトランスパイラも読まないデコレータが `throws` / `catches` を持つときに、何か言うかどうか（単位ごとの warning など）は、P37 が「言わない」と決めている。
- **N82 デコレータの節が言うことを守るのは、継ぎ目の中身だけ**。`decorator limited … throws LimitExceeded prevents effects` の「制限を超えたら、効果は起きない（中身を呼ばない）」は、検査の計算に効く（`login` の `rate.LimitExceeded` と `store.createSession` の関係は `prevented`）が、`go-ratelimit` の出力がそう書いてあること（`gen/server/ratelimit.go:27-29`）を、hozoc も基底も確かめない。確かめているのは、人のテスト（`app/server_test.go` の `TestLoginIsLimitedThroughOps`: 制限を超えた呼び出しで、`store` のどの op も呼ばれない）。「デコレータは、op の入力と出力の値を変えない」（P39）も指針で、継ぎ目は、中身に渡す値も、返す値も差し替えられる（`app/inner_test.go` の `TestSeamOwnsBeforeAndAfter` が、差し替える継ぎ目を書いて、基底が止めないことを確かめている）。
- **N83 語彙の例外を使うと、設定に単位が一つ増える**。`rate.LimitExceeded` の Go の型の置き場として、`rate` の単位（`gen/rate`。型だけのパッケージ）を足した。プロジェクトの中のプラグインを使うために `hozo.toml` に書くのは三つ: `plugin_dirs`、継ぎ目を持つ単位の `transpilers` に `go-ratelimit`、語彙の名前空間の単位。三つ目は、忘れると CFG-22 が、足す単位の形を示す。例外を `type` に置いて、デコレータの引数で渡す形なら、単位は増えない（「サンプルの中のプラグイン」の「語彙と、型の置き場」）。`go-http` は、`go-ratelimit` を知らないまま 429 を返す: 語彙 `rate` が、自分の例外に、標準の語彙のデコレータ `@http.status(429)` を書いているから（プラグインの語彙が、別の語彙に依ってよいか、依る先が標準でない語彙だったらどうするかは、決めていない）。
- **N84 数え方の受け取り方は、プラグインが決めた。呼んだ人ごとの制限は、書けていない**。基底の `Seams` は、継ぎ目の実装が実行のときに何を要るかを知らない。`go-ratelimit` は、`RateLimiter` を出して、`NewRateSeams(limiter)` で受け取る形にした（葉の op の `Leaves` と同じ作法）。`Allow(ctx, op)` が受け取るのは、op の FQN だけ。**呼んだ人ごとに数えるには、呼んだ人が要る**が、op の入力（`LoginInput`）は op ごとに型が違い、送り元のアドレスは op の入力になく（HTTP の層にある）、基底は `ctx` に値を載せない。→ このサンプルでは、op ごとの回数だけを数えた。道は二つ考えられる: デコレータの引数で、入力のどの値ごとに数えるかを書く（`@rate.limited(by: …)`。継ぎ目のメソッドは op ごとに型が付いているので、プラグインは、その値を `Allow` に渡すコードを出せる）か、人が `ctx` に載せて、人の `Allow` が読む。どちらも決めていない。
- **N85 継ぎ目は、`go-http` の検査より内側にある**。`POST /login` は、読めない要求と制約の違反（400）が先で、制限（429）は op の中。制限を超えていても、制約に合わない入力には 400 が返り、その要求は数えない（`app/handlertest/handlertest.go:174-177`。N47 と同じ並び）。本文を読む前に断る（負荷を避ける）ための制限は、op の継ぎ目ではなく、`http.Handler` を包む人のコード（ミドルウェア）の仕事になる。`@rate.limited` が言うのは「この op を呼べる頻度」で、HTTP の要求の頻度ではない: `server.New` の `Ops` を直に呼んでも、同じに数える。
- **N86 テストは、まだないものを名指しして飛ばす**。`TestSamples` を最後の形にした（P36）。サンプルの中のプラグインの実装がまだないことは、`sample.json` に書かず、テストが見分ける（マニフェストの `command` が `go run` で、そのディレクトリに Go のファイルがない）。今の hozoc は、その前に、このマニフェストを読めない（`requires` と `seams` が知らないキーで MAN-4）。`hozo.toml` も読めない（N51）ので、単位の `out` が要る `files`（`sample.json` と記録の照合）も、今は飛ぶ: 文書の形にそろえた今回の `gen/` と `sample.json` と記録の一致は、同じ規則を手で当てて確かめた。hozoc が `[emit.options]` を読むようになれば、`files` が、何も変えずに走る。

#### デコレータで言う読み書き（決定 91〜102）

- **N87 列を渡す引数の名前を `by` にできなかった**。決定の書き方は `find(table: any state, by: string)` だったが、`by` は予約語（`match … by`）で、デコレータの引数の名前（`Ident`）に書けない。→ `column` にした（`docs/vocab.md`「`db` の読み書きのデコレータ」）。位置で渡すので、仕様の書き方（`@db.find(users, "email")`）は変わらない。
- **N88 `@db.find` の列は、一行に決まる列でなければならない**（GOSQL-13）。決定が並べた整合の検査にはなかった。足さなければ、一意でない列を名指しした `find` が、合う行のうちのどれか一つを黙って返す。前の規則の「対応した列のどれかが `@db.key` か `@db.unique`」を、検査として残した形。
- **N89 `T?` の列と `T?` の引数で、値がないときの意味**。決定 96 は「型が等しければよい」までで、値のない引数が何に合うかは言っていない。→ 等しい型どうしの比較として、値のない行に合う、と決めた（`"c" IS ?n`。`docs/plugin-go-sqlite.md`「列と引数の型」）。この仕様には、省略可能な列がないので、期待する出力には現れない。
- **N90 配送に失敗したときに、通知の記録が残るようになった**。前は、`mailer.send` の後で `store.addNotification` を呼んでいたので、配送に失敗すれば記録は残らなかった。二つの書き込みを一つの op にまとめるには、記録を配送の前に置くしかない（配送は別の名前空間の op で、トランザクションに入らない）。記録が「送った通知」ではなく「送るべき通知」になり、送れたかどうかは、どこにも残らない（`app/handlertest/handlertest.go:227-238`）。配送の前に注文だけを書き、配送の後で通知を書く形に戻すなら、二つの効果を一つの op にする理由がなくなる。
- **N91 メモリの上の実装では、「一体」を人が守る**（`app/memstore/memstore.go` の `Shared.PlaceOrder`）。`gen/storemem` の単位には `@db.effects` を読むトランスパイラがいないので、「一つのトランザクション」は、デコレータの説明のコメントにしかない。二つ目の効果だけが失敗する場合（SQLite では外部キーの違反で起こせる。`TestPlaceOrderIsOneTransaction`）は、メモリの上では起こせないので、二つの実装が共に走らせるテスト（`storetest` の `placeOrder`）は、一つ目が失敗する場合だけを確かめる。N42 と同じ: 状態の単位のインターフェースが保証するのは、メソッドの形まで。
- **N92 トークンがない場合を仕様に書くと、配線が一つ増える**。アームに書けるのは呼び出し一つ（N1）なので、「トークンがあれば、セッションを引いて、分ける」は、`authenticate` のアームから呼ぶ別の配線 `sessionOf` になった（`server.hozo:95-103`）。分類 `Credential` は、`by nullable` と同じ判定で、場合に名前を付けただけ（N4 と同じ）。
- **N93 `SharedState` の実装は、またがる状態と同じ保存先のものでなければ意味がないが、型はそれを言わない**。`store.New(users, sessions, orders, notifications, shared)` に、`orders` はメモリ、`shared` は SQLite、と渡しても、コンパイルは通る（`placeOrder` は SQLite の表に書き、`listOrders` はメモリを読む）。`app/both_test.go` の `TestStatesFromTwoImplementations` は、そろえて渡している。N42 の、状態ごとの実装を混ぜられることの、裏側。
- **N94 サンプルの中のプラグインのマニフェストの `protocol` を、先に上げられない**。文書の版は 0.5 だが、`plugins/go-ratelimit/hozo-plugin.toml` を 0.5 にすると、今の hozoc が MAN-2 で断り、プラグインの単体テストの入力（`testdata/*/views.json`。今の hozoc が作る）とも食い違う。マニフェストの版は、仕様ではなく、読む hozoc と対のものなので、実装と一緒に上げる（「まだないもの」）。
- **N95 文書にあって、この仕様が使っていない形**: 述語で決まる状態の分類の単位の型（`<C>Table`）、遷移を宛先にする効果のデコレータ、`@db.update`、列のない `@db.list`、`T?` の列。どれも、期待する出力による確かめがない（`go-sqlite` の単体テストが、小さな入力で確かめることになる。`docs/testing.md`「単体テスト」）。
- **N96 期待する出力を、実装のない間に、どこまで道具で確かめられたか**。`gen/server` は、今の hozoc で確かめられた（「まだないもの」の末尾。その後、`gen/server/http.go` は、手で書いたものに変わった: N105）。`gen/store` の基底のファイル（`SharedState`、`New` の引数 `shared`）と `db.go` は、文書から手で書いたもので、確かめたのは、コンパイルできることと、テストが通ること（一つのトランザクションであることを含む）まで。空白やコメントの文言が文書の型どおりかは、実装ができて `transpilers/go` と `transpilers/go-sqlite` が走るまで、機械では確かめられていない。

#### 型に依らない出力（決定 103〜113）

- **N97 表の model を言う引数の名前を `model` にできなかった**。`model` は予約語で、デコレータの引数の名前（`Ident`）に書けない（N87 と同じ）。→ `element` にした（`docs/vocab.md`「`db` の表と、しまい方」）。位置で渡すので、仕様の書き方（`@db.table(type.Order)`）は変わらない。
- **N98 しまい方の名前のうち、`int` と `float` は予約語**。デコレータの名前には書ける（決定 98 の、予約語のデコレータ名）が、今の hozoc は、まだ読めない（「まだないもの」）。この仕様は `@db.text`、`@db.datetime`、`@db.json` しか使わないので、予約語の名前のしまい方は、期待する出力に現れない。
- **N99 配列そのものを、本文にできなくなった**。全体の形（`@http.bodyObj`）は model だけなので、`@http.body lines: type.OrderLine[]` は、キー `lines` のオブジェクトになる（「下書きから変えたこと」の 14）。配列や文字列一つを本文そのものにする API は、今の語彙では書けない。
- **N100 省略可能なものと、そうでないものを、一つの型引数の関数では受け取れない**。`*pkgtype.Token`（省略可能）と `pkgtype.Token` を同じ補助に渡す形を、Go の型引数と制約では書けなかった（`P interface{ *T | **T }` という制約は書けるが、呼び出しから T を推論できず、行ごとに型を書くことになる。その型は、フィールドの型）。→ 読む先を `any` で受け取り、実行時に `reflect` で見る形にした（`gen/server/http.go` の `httpRequest.text`、`gen/store/db.go` の `dbCol`）。受け付けない型を渡すことが、コンパイルエラーにならない。防ぐのは、整合の検査（HTTP-4、GOSQL-17）。例外（`httpThrown[E]`）は、型引数で書けた。
- **N101（解決: 決定 114、115、S24）実行時には、「既定値が零値」と「既定値がない」を見分けられなかった**。要るヘッダやクエリがないことを、補助は、読む先がポインタでなく零値のままかどうかで見ていた。`page: int = 1` は足りないことにならないが、`= 0` と書けば、既定値なしと同じになった。→ 補助は、読む先の値も型も見ない。なければ誤りか、何を入れるかは、デコレータが、行の引数で言う（`httpRequired`、`httpDefault(0)`）。
- **N102（解決: 決定 117）省略可能な配列は、実行時には、空の配列と見分けられない**。Go で、どちらも `nil` のスライス。`httpWriteResult` は、`nil` の配列を、いつも `[]` で 200 にする。この仕様の `getOrders` は `type.Order[]` なので、それで正しい。→ 省略可能な配列と record を返す op は、経路にできない（GOHTTP-16）。
- **N103（解決: 決定 116、S24）入力の初めの値だけは、ハンドラの形が二つあった**（`in := NewGetOrdersInput()` と `var in AuthInput`）。既定値を持つ model だけが `New<Model>` を持つという、基底の形に合わせていた。→ 既定値が言語からなくなり、基底は `New<Model>` を出さない。六つのハンドラは、どれも `var in <Model>`。
- **N104 条件を `IS` にそろえても、この仕様の振る舞いは変わらない**。五つの条件の列は、どれも省略可能でなく、引数も同じ型なので、`=` と同じ行に合う。変わったのは SQL の文字だけ（`gen/store/db.go` の `FindUser` など）。
- **N105 `gen/server/http.go` は、今の hozoc では確かめられない**。今の `go-http` は、型で形を変える前の出力を出す。新しい `http.go` は、文書から手で書き、確かめたのは、コンパイルできることと、`app/handlertest` が通ること（204 と、項目の名前を含む）まで。`gen/server` のほかのファイルは、今の hozoc の出力と比べてある（「まだないもの」の末尾）。
- **N106 `httpValidate` が、確かめるもののない入力にも出る**。`AuthInput` と `GetOrdersInput` は `Validate` を持たないが、四つのハンドラが `httpValidate` を呼ぶ（実行時に、何もしないで `true`）。型を見て行を省かないことの、目に見える帰結。

#### 値がないときの扱い（決定 114〜122、S24）

- **N107 `lines` のない注文が、400 になった**。前の `http.go` は、本文の項目がなくても誤りにしなかったので、`{}` の `POST /orders` は、空の注文を作って 200 を返していた（サブテスト `names` が、それを確かめていた）。`lines` は省略可能でも既定値持ちでもなかったが、「なければ誤り」は、ヘッダとクエリにしか掛かっていなかった。`@http.required` を付けたので、400 `body lines: missing`（「下書きから変えたこと」の 16）。キーがあって空の配列（`{"lines":[]}`）は、今も、空の注文を作る: 「一行は要る」は、`@valid` の制約で言うことで、この仕様は言っていない。
- **N108 既定値を入れる者が、型から、ハンドラに移った**。`page` の 1 を入れるのは、`req.query("page", &in.Page, httpDefault(1))` で、`server.GetOrdersInput{}` を自分で作る者（op を直に呼ぶテスト、別の入口）には、入らない（`Page` は 0）。`app/server_test.go` の `TestMissingItems` は、ハンドラを通した入力で 1 を確かめる。葉 `pageOf` に 0 が渡ったときの振る舞いは、仕様が言っていない。
- **N109 `httpDefault` の値は、`any` で渡る**。`httpDefault(1)` の 1 は Go の `int` で、補助が、実行時に、読む先の型（`int64`。scalar なら、その名前付きの型）に `reflect` で変える。フィールドの型への変換を行に書かないのは、型で形を変えないため（N100 と同じ事情）。型に合わないリテラル（`int` のフィールドに `"x"`）は、コンパイルエラーにならず、防ぐのは GOHTTP-18。
- **N110（解決: 基底 `go` の R3 の実装）基底のファイルの一部は、手で直した後、道具で確かめていなかった**。`gen/server/inner/types.go` と `gen/server/types.go` から `NewGetOrdersInput` を外したのは、手。この直しのときは、リポジトリが言語の実装の途中でビルドできず、`hozoc build` を掛けていなかった。→ `New<Model>` を出さない基底 `go`（`docs/plugin-go.md`「model」）で、この仕様のまま `hozoc build` を掛けると、二つのファイルを含む基底の十九のファイルは、どれも、期待する出力とバイト単位で同じ。手の直しに、食い違いはなかった。
- **N111 この仕様には、`@db.nullable` の列がない**。表の model のフィールドは、どれも省略可能でない。`gen/store/db.go` に現れるのは、補助が `dbNullable` を受け取れる形（`dbOption`、しまい方の補助の `opts`、`dbCol` の `Value` と `Scan`）だけで、`dbNullable` を渡す行も、`NOT NULL` のない列も、期待する出力にない（`go-sqlite` の単体テストが、小さな入力で確かめることになる。`docs/testing.md`「単体テスト」）。先の決まらない `@db.ref`（GOSQL-20）も、この仕様では起きない。
- **N112 `default` の値を、制約で確かめる者がいない**。`@http.default(1)` の 1 が、フィールドの制約（`@valid.*`）に合うかは、生成するときには確かめられない（前は、`go-valid` が、既定値を GOVALID-2 で確かめていた）。`validate = true` のこの単位では、入れた値も、実行時に `httpValidate` を通る。`page` に制約はないので、この仕様では起きない。その後、生成するときに確かめると決めた（`docs/plugin-http.md`「補う値と制約」。GOHTTP-19）。
- **N113 `@cli.required` と `@cli.default` の、期待する出力は、ここにない**。この仕様に CLI はない。`go-cli` の形（`line.required(&in.<F>)`、`line.orDefault(&in.<F>, <リテラル>)`）を固定するのは、`go-cli` の単体テストのゴールデンと、hozoc 自身の生成物（`docs/plugin-go-cli.md`「コマンドごとの関数」）で、hozoc 自身の `cli.hozo` も、この二つを使わない。

#### 例外、効果、`read` を同じ書き方に（決定 124〜131、S25。125〜131 は、次の節の決定が置き換えた）

- **N114 期待する出力は、コメントの四行しか変わらなかった**。`read` に名前を付けても、基底の出力は変わらない: op が読む状態（`Signature.reads`。`Reads: store.users`）も、op の置き場も、状態の単位のメソッドのコメント（`// FindUser は問い合わせ: store.users を読む（reads）葉の op store.findUser。`）も、`read` の名前を持たない。`read` の名前が出るのは、`go-sqlite` の四つのメソッドのコメントだけ。基底のコメントに `read` の名前を書くかどうかは、決めていない（`docs/plugin-go.md`「コメント」の未決）。
- **N115 表を、二度書く**。`@db.find(users, "email") read userByEmail on users;` は、表 `users` を、デコレータの引数と、`on` の後ろの両方に書く。効果（`@db.insert(users) effect createUser on users.post;`）と同じ形で、食い違いは GOSQL-21。出力を決めるのがデコレータの引数だけ、という原則の帰結。
- **N116 読む op の説明と、`read` の説明が、ほとんど同じ文になる**。`read userByEmail`（「email が同じユーザー」）と `op findUser`（「email からユーザーを引く」）。`read` を一つだけ行い、引いたものをそのまま返す op では、二つの宣言が同じことを言う。書く側の `effect createUser` と `op addUser` も同じ。`read` を二つ以上並べる op が書けるようになれば、分かれる（`docs/plugin-go-sqlite.md`「未決: 複数の `read` と、戻り値の型」）。
- **N117 この段階の期待する出力は、道具で確かめていない**。書いたときの hozoc は、この仕様を読めない（節の `,` と、`read` の宣言）。確かめたのは、このモジュールの `go vet` と `go test` が通ることまで（変えたのは、コメントだけ）。

#### 状態に触れるのは `read` と効果だけ（決定 132〜143、S26〜S28）

- **N118（N115 の解決）表を、一度しか書かない**。`@db.find read fn userByEmail(email: type.Email): type.User? reads users;` は、表を `reads` に、列を引数の名前に、引くものを結果の型に、一度ずつ書く。デコレータに残ったのは、宣言が言っていないこと（一行か、すべてか）だけ。
- **N119（N116 の続き）九つの op のうち八つは、`read` か効果を、そのまま呼ぶだけの配線になった**。`read` と効果を呼べるのは `store` の配線だけで、`export` は op にしか付かないので、`server` が使う `read` と効果には、同じ引数と同じ結果の op が、一つずつ要る（`findUser` と `userByEmail`、`addSession` と `createSession`）。説明のコメントも、二つずつ書く。op が、`read` と効果を組み合わせる（`placeOrder`）ようになって、初めて、二つの宣言が別のことを言う。
- **N120 メモリの上の `placeOrder` は、「どちらも足すか、どちらも足さない」を守らなくなった**。前は、一つの葉 `placeOrder` を人が書き、一度の lock の中で両方を守っていた。今は、op が基底の配線で、人が書くのは二つの効果だけ: 二つをまたいで守る場所が、人の側にない。`@db.transaction` は、単位 4 では読む者がなく、印。**同じ仕様が、単位によって、守ることが違う**（「人が書いたもの」の `app/memstore`）。仕様の op の説明は、どちらの単位でも同じ文（「どちらも足すか、どちらも足さない」）のまま出る。
- **N121 この段階の期待する出力は、道具で確かめていない**。書いたときの hozoc は、この仕様を読めない。確かめたのは、このモジュールの `go vet` と `go test` が通ることまでで、それも、リポジトリの `ir` と `plugin/golang` が R4 の実装の途中だったので、R3 のコミット（`3370523`）の木に、このディレクトリだけを重ねたもので走らせた（`go-ratelimit` の単体テストは、R3 の基底と、前の版の `views.json` で通っている）。
- **N122 人が書く単位は、デコレータの意味を、文書から写す**。`latestOrderOf` を、メモリの上に書く人が頼れるのは、`read` の説明（「いちばん新しい注文」）と、`go-sqlite` の文書（`placedAt` が同じなら、後から足した方）。`@db.find` と `@db.latest("placedAt")` は、単位 4 では、誰も読まない。二つの実装が同じ振る舞いをすることは、共通のテスト（`app/storetest`）でしか守れない。
- **N123 同じ名前空間の二つの単位の、基底のファイルが、同じでなくなった**。前は、`gen/store` と `gen/storemem` の基底の四つのファイルが、バイト単位で同じだった。今は、`go-sqlite` が継ぎ目を宣言するので、`ops.go`（`Seams`、`New` の引数、外側）と `wiring.go`（中身の名前）が違う。`store.New` の引数の数も、単位で違う（五つと四つ）。
- **N124 `Conn` を、公開の名前にした**。トランザクションは `ctx` が運び、表のメソッドは、補助を通して、それに入る。人が書き足すメソッドも入れるように、その補助を公開にした（`store.Conn(ctx, db)`）。公開にしなければ、人のメソッドは、トランザクションの外の接続で文を実行する（取り消されず、SQLite では、トランザクションが終わるまで待つ）。
- **N125 引数を持つ例外を、この仕様は使っていない**。三つの例外と `rate.LimitExceeded` は、どれも引数がない。`go-http` の `value` の形（引数の名前をキーにしたオブジェクト）と、基底の、引数ごとのフィールドは、`gen/` に現れない（`gen/server/http.go` の `httpThrown` の本体が変わっただけ）。`rate.LimitExceeded` に `retryAfter: duration` を足せば現れるが、429 の応答と、`go-ratelimit` の `RateLimiter` の形が変わるので、決めていない。

#### op の中身に立ち入らない（決定 145〜148）

- **N126 メソッドの形が、一通りになった**。基底は、どの関数に `ctx` と `error` が要るかを判定しない（`docs/requirements.md`「op の中身に立ち入らない」）。`server` の七つの葉は、仕様の上では今も純粋だが、メソッドは `ctx` を取り、`error` を返す（`gen/server/inner/ops.go` の `Leaves`）。N8 の「メソッドの形で、保存先に触れないことが分かる」は、もう言えない。N9 の「純粋と判定された op が、時計と乱数を使う」は、形の上の食い違いではなくなった（判定が、形に出ない）。N12 と N60 の「宣言を一つ変えると、メソッドの形が変わる」は、起きない: デコレータ、`reads`、効果、`throws` を足し引きしても、人が書いた本体と、`Ops` を呼ぶコードは、そのまま通る。N13 の「純粋さの判定を使うのは、基底だけ」は、「誰も使わない」になった。
- **N127 `error` のない葉が、なくなった**（N60 の一つ目の続き）。`newSession` と `newOrder` は、トークンと識別子を作れなければ、その `error` を返す（`app/leaves.go` の `NewSession`、`NewOrder`）。仕様に `throws` を書かなくてよい: 仕様の例外でない `error` は、配線が、そのまま呼び出し元へ返し（`app/server_test.go` の `TestLeafErrorIsReturned`）、`go-http` は 500 `{"error":"internal"}` にする。main の `randomHex` の panic は、なくした。
- **N128 決して失敗しない写しの後でも、配線は `err` を確かめる**（N10 が解いたことの、逆戻り）。`publicUser, err := o.ToPublicUser(ctx, user)` と、失敗の扱いの三行（`gen/server/inner/wiring.go` の `IssueSession`）。`toPublicUser` の本体は、いつも `nil` を返す。引き換えに得たのは、N126 と N127。
- **N129 `ctx` を受け取って、使わない本体が七つある**（`app/leaves.go` の、引数 `_ context.Context`）。どれも待たないので、中断を見ない。見るかどうかは、本体を書く人が決める。
- **N130 相乗りのファイルは、変わらなかった**。`go-http` のハンドラが呼ぶ op、`go-ratelimit` と `go-sqlite` の継ぎ目の op は、前から `ctx` を取り、`error` を返していた（N13）。`gen/server/http.go`、`gen/server/ratelimit.go`、`gen/store/db.go` は、一バイトも変わらない。`gen/store` と `gen/storemem` の基底のファイルも同じ（九つの op は、どれも前から同じ形）。
