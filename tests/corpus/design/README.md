# コーパス `design`

設計書（出発点のアイデア集で、仕様の持ち主ではない。`docs/README.md`「設計書という呼び名」。以下「§n」はその節）§3.4、§5〜§11 の例を、名前空間ごとに一つのファイルにまとめたプロジェクト（`docs/testing.md`「コーパス」）。

| ファイル | 中身 |
|---|---|
| `hozo.toml` | 単位（`[[emit]]`）が四つ: `namespace = "server"`、`"browser"` と、使う型の置き場の `"http"`、`"page"`。どれも `transpilers = ["go"]`（Go の基底。リポジトリの `plugins/go`。テストが `HOZO_PLUGIN_PATH` として渡す。下の表の 1） |
| `server.hozo` | `namespace server` の例すべてと、§6.5 の `namespace valid { decorator checkDigit ... }` |
| `browser.hozo` | `namespace browser` の例すべて |

設計書の言葉とドキュメンテーションコメントはそのまま写した。各ブロックの上の `// §n.m` が出典、`// 追加:` が設計書にない宣言。標準の語彙（`http` / `db` / `page` / `valid` と、グローバルの `localStorage`。§4.2）は hozoc が同梱する（`docs/vocab.md`）ので、ここには写していない。

`docs/testing.md`「コーパス」は一覧の置き場を `tests/corpus/README.md` としているが、コーパスごとに分けるためこのディレクトリに置いた。

ゴールデン（`docs/testing.md`「コーパス」、`tests/corpus_emit_test.go`）は `testdata/expected/` の下（Go のツールが `.go` を拾わないように `testdata`）: `check.txt` は `hozoc check` の text 出力、`emit/` は `hozoc emit` が書き出すファイルの木。更新は `go test ./tests -update`。

## 設計書から変えた箇所

| # | 設計書の箇所と原文 | このコーパスでの書き方 | 理由 |
|---|---|---|---|
| 1 | §4.5 `[browser] language = "ts"`、`plugins = [...]`（名前空間ごとのテーブル） | 単位の一覧（`[[emit]]`）で、`server` も `browser` も `transpilers = ["go"]`（Go の基底）。語彙の `http` と `page` にも `go` の単位を足す | `hozo.toml` は名前空間ごとのテーブルではなく単位の一覧（`docs/config.md`「形」、`docs/decisions.md`「設計書から離れた決定」S14）。言語はトランスパイラが持つ。TS の基底はまだないので、`browser` も Go で出力する。設計書のプラグイン（http のサーバー、ルーター、db）は並べない（相乗りの `go-http` などは、このコーパスの語彙の使い方を対象にしていない）。`http` と `page` の単位は、`server` と `browser` が使う語彙の型（例外、`page.Route`）の置き場（`docs/config.md`「型の置き場」。なければ CFG-22） |
| 2 | §7.1 `model LoginInput { @http.header(name: "Authorization") token?: Token; ... }`、§9 `model LogoutInput { ... token?: Token; }`、§11.2 `model OrdersQuery { page: PosInt = 1; status?: server.OrderStatus; }` | `token: Token?`、`status: server.OrderStatus?` | 省略可能は型の `?` だけで書く（`docs/grammar.md`「決めたこと」、`docs/decisions.md` 8） |
| 3 | §5.2 `@page.path state location: single<Route>;` | `single<page.Route>` | `Route` は語彙 `page` の宣言なので修飾する（`docs/resolve.md`「手続き」） |
| 4 | §5.2 `namespace browser { @localStorage state token: single<Token?>; }` | `nullable<server.Token>` | 二つの理由。(a) `Token` は `server` の scalar（§6.5）。別の名前空間の宣言は常に修飾する（§3.2、NS-1）。写したままだと RES-1（`docs/resolve.md`「診断」）。(b) 空になりうる状態は `nullable<T>` と書く（`docs/decisions.md`「設計書から離れた決定」S2）。`single<T?>` も同じ型（`nullable<T>`）として読める（S21）が、コーパスは形の語で書く |
| 5 | §6.5 `model User { @valid.length(min: 1, max: 100) name: string; }`（§5.2 の `model User` と別に書かれている） | §5.2 の `User` の `name` に `@valid.length(min: 1, max: 100)` を付けて一つにした | 同じ名前空間に同じ名前の宣言が二つあれば RES-6（`docs/resolve.md`「名前空間の併合と重複」） |
| 6 | §11.2 `Ready: { userName: string; orders: OrderRow[] };`、`Empty: { userName: string };` | `Ready: { userName: string; orders: OrderRow[]; };`、`Empty: { userName: string; };` | その場で書く model のフィールドは `;` で終わる（`docs/grammar.md`「model」`Field = ... ";"`）。`ProfilePage.Ready` も同じ |
| 7 | §8.1 `decorator redirectOn(error: ApiError, to: RedirectByError) catches error halts to;`、`decorator redirectToLogin = redirectOn(server.Unauthorized, transitionToLogin);` | 書かない | スコープ外（`docs/decisions.md`「スコープ外にした設計書の例」1 行目）。引数名 `to` は予約語（`docs/grammar.md`「予約語」） |
| 8 | §11.1 `@redirectToLogin` `@cache(key: "me", staleTime: 5m)` `fn fetchMe()`、`@redirectToLogin` `@cache(key: ["orders", query], staleTime: 30s)` `fn fetchOrders(query)` | デコレータを外し、配線と `throws http.NetworkError, http.ServerError` だけを残した | 7 と同じ行、および「スコープ外」2 行目（`cache`）。§11.2 の `openOrders` / `openProfile` が呼ぶので op 自体は残す |
| 9 | §8.1 `decorator requiresAuth throws Unauthorized prevents effects;` は、§11.1 の `fetchMe` に付く `@redirectToLogin` が `server.Unauthorized` を吸収することから、`server.getMe` に付いていると読める | 追加した `getMe` / `getOrders` には `@requiresAuth` を付けない。`requiresAuth` は宣言だけ残す | 8 で `@redirectToLogin` を外したため、付ければ `fetchMe` / `fetchOrders` が `server.Unauthorized` を宣言も吸収もせず THROWS-1（`docs/check.md`「throws」）。`fetchMe` の `throws` を増やせば `openOrders` / `openProfile`（「fallback が吸収するので throws は書かない」）にも波及する |
| 10 | §6.5 `namespace valid { decorator checkDigit for scalar, field; }` | `server.hozo` の先頭に置いた | `server.EmployeeCode` が使う。`valid` は語彙の名前空間なので専用のファイルにせず、使う側のファイルに置いた（§3.2「語彙プラグインの名前空間にも、ユーザーが宣言を足せる」、`docs/vocab.md`「読み込み」） |
| 11 | §3.4 `fn requestPasswordReset(email: Email): effects sendResetMail throws RateLimited prevents effects;`（名前空間の記述なし） | `server` に置いた | `@see login` が `server.login` を指すため |
| 12 | §10.2 の文の断片 `const plan = match item by WarrantyAvailable { WarrantyAvailable => askWarrantyPlan(item); default => none; };` | 追加した `op chooseWarrantyPlan` の本体に、`Available => askWarrantyPlan(item); Unavailable => none;` と書いて置いた | 文は op の本体にしか書けない（`docs/grammar.md`「配線」）。`default` のアームは書けず、アームは、分類のケースを名前で受ける（表の 23） |
| 13 | §9 `effect removeToken on token.put;   // none を入れる` | `effect fn removeToken(): effects token.delete;` | 空にするのは `delete`（`docs/decisions.md`「設計書から離れた決定」S2、`docs/check.md`「状態の形」の表: `nullable` の `delete` は `Some` → `None`）。`put` は列数を変えない組み込みの効果なので、空にする意味にならない |
| 14 | §7.3 `fn placeOrder(in: OrderInput): Order effects createOrder \| notifyUser throws NotificationFailed keeps createOrder prevents notifyUser;`（葉の op） | 効果を一つずつ呼ぶ配線 `saveOrder`（`effects createOrder`）と `sendOrderNotice`（`effects notifyUser`、`throws NotificationFailed prevents effects`）を足し、`placeOrder` はその二つを順に呼ぶ配線（`throws NotificationFailed`。`effects` も `prevents` / `keeps` も書かない） | `prevents` / `keeps` の目的語は `effects` だけで、効果を名前で書き分ける形はない（`docs/decisions.md`「設計書から離れた決定」S9）。例外ごとの効果の状態は、配線の行の順から計算される（`docs/check.md`「計算」手順 0）。葉の op が `effects` を書けないことは、表の 20 |
| 15 | §5.2 `model PublicUser is User omit hash;` | `model PublicUser extends User { override hash: never; }` | `model A is B omit …` はなくなり、継承した model は `extends` と `override` で書く（`docs/decisions.md`「設計書から離れた決定」S22、`docs/grammar.md`「model」）。写したままだと SYNTAX |
| 16 | §5.3、§9〜§11 の、`browser` が呼ぶ `server` の op（`fn logout(...)`、追加した `getMe` / `getOrders` / `getItem` / `addToCart` / `attachWarranty`） | `export fn ...` | 別の名前空間から呼べる op には `export` を付ける（S23、`docs/check.md`「export」）。付けなければ NS-5。`server` の中からしか呼ばない op（`login`、`placeOrder` など）には付けていない |
| 17 | §5.2 `@db state users: list<User>;`、`@db state sessions: list<Session>;`（追加した状態も同じ） | `@db.table state users: list<User>;` | グローバルのデコレータ `db` はなくなり、表は `@db.table` で言う（`docs/vocab.md`「`db` の表と、しまい方」）。写したままだと RES-1。しまい方（`@db.text` など）と `@db.find` などは書いていない: このコーパスは `go-sqlite` を並べず、`db` の語彙のデコレータは印のまま（下の「出力」） |
| 18 | §7.1 `model LoginInput { ... @http.body body: LoginInputBody; }`（一つの `@http.body` を、本文そのものと読ませる例）。追加した `AddToCartInput` の `@http.body item: Item` も同じ | `@http.bodyObj body: LoginInputBody;`、`@http.bodyObj item: Item;` | 本文の全体を一つの model で受け取るのは `@http.bodyObj`。`@http.body` は本文のキー一つ（`docs/vocab.md`「`http` の語彙」、`docs/decisions.md`「決めたこと」107）。二つの `@http.body` を持つ `AttachWarrantyInput` は、項目の形のまま |
| 19 | §11.2 `model OrdersQuery { page: PosInt = 1; status?: server.OrderStatus; }`（フィールドの既定値） | `page: PosInt;`（既定値を書かない）。追加した `server.GetOrdersInput` の `page` は、境界のデコレータで `@http.default(1) @http.query page: int;` | フィールドの既定値は書けない（`docs/grammar.md`「model」「決めたこと」）。値がないときに何を入れるかは、境界のデコレータが言う。`browser.OrdersQuery` は `@page.query` の状態の型で、`page` の語彙には値を埋めるデコレータがないので、既定値を落とした（書けなかったこと） |
| 20 | §3.4、§7.1、§7.3、§9、§10.2 の、`effects` を書いた葉の op（`fn login(in: LoginInput): LoginOk effects createSession throws … prevents effects;`、`cancelOrder`、`requestPasswordReset`、`logout`、`browser` の `clearToken` / `goPayment` など） | 効果を呼ぶ配線にした: `{ const ok = authenticate(in); createSession(ok); return ok; }`。状態に触れない部分は、追加した葉（`authenticate`、`confirmRefund`、`newOrder`、`composeNotice`、`newPasswordReset`、`newCartLine`、`withWarranty`）に分けた。`effects` と `prevents effects` / `keeps effects` の節は、原文のまま残した | 状態に触れるのは `read` と効果だけで、葉の op は `reads` / `effects` の節を書けない（`docs/check.md`「葉の op」。写したままだと STATE-5）。書いた節は、本体から計算したものと照合される（EFF-2、REL-4） |
| 21 | §5〜§11 の `effect createSession on sessions.post;`（引数なし）、§6.1 `exception ClientError: Violations;` | `effect fn createSession(login: LoginOk): effects sessions.post;`、引数のない効果は `effect fn transitionToLogin(): effects location.put;`、`exception ClientError(value: Violations);` | 効果と例外は、op と同じ形の引数を持つ（`docs/grammar.md`「exception / effect / read」）。効果の括弧は省けず、例外の値は引数で書く。写したままだと SYNTAX |
| 22 | §7.1、§8、§10.2、§11.1 の `throws A \| B`、`catches A \| B` | `throws A, B`、`catches A, B` | op とデコレータの宣言の節の並びは `,` で区切る（`docs/grammar.md`「op」「decorator の宣言」）。`\|` は型のユニオンと `union`、デコレータの対象に残る。写したままだと SYNTAX |
| 23 | §10.1 `classify CampaignKind of server.Item { Spring; Summer; default; }`、`condition WarrantyAvailable of server.Item;` | `default;` は、名前のあるケース `NoCampaign;` に。`condition` は `classify WarrantyAvailable of server.Item { Available; Unavailable; }` に | 分類は閉じている: ケースは、どれも名前を持ち、名前のないケース `default` と `condition` の宣言は書けない（`docs/decisions.md`「設計書から離れた決定」S29、`docs/check.md`「閉じた分類」、`docs/grammar.md`「classify」）。写したままだと SYNTAX |

## 追加した宣言

設計書の例が参照しているのに宣言がないものと、それを成り立たせるための最小限の宣言。型と名前は設計書の文章（§10.3 の `item.hasMakerWarranty` / `item.yamadaEligible` など）に合わせた。

`server`（`server.hozo`）:

- scalar: `UserStatus`（§5.2 `User.status`）、`OrderId`、`OrderStatus`（§11.2 `status?: server.OrderStatus`）、`ItemId`、`PlanId`（§10.2 `server.PlanId`）
- model: `Item`（§10.1 `classify ... of server.Item`）、`CartLine`、`WarrantyLine`（§10.2）、`Order`（§7.3）、`OrderSummary`（§11.1）、`Notification`、`PasswordReset`、`CancelInput`、`OrderInput`（§7.3）、`MeInput`、`GetOrdersInput`、`GetItemInput`、`AddToCartInput`、`AttachWarrantyInput`
- state: `items`、`carts`、`orders`、`notifications`、`passwordResets`（追加した効果の宛先）
- effect: `refund`、`createOrder`、`notifyUser`（§7.3）、`sendResetMail`（§3.4）、`addCartLine`、`attachWarrantyToLine`
- exception: `RefundPending`、`NotificationFailed`（§7.3）、`RateLimited`（§3.4。`@http.status(429)` を付けた）
- op（配線。§7.3 の `placeOrder` を分けたもの。表の 14）: `saveOrder`、`sendOrderNotice`
- op（葉。効果を呼ぶ配線の、状態に触れない部分。表の 20）: `authenticate`、`confirmRefund`、`newOrder`、`composeNotice`、`newPasswordReset`、`newCartLine`、`withWarranty`
- op（すべて `@http.get` / `@http.post`。`getMe`、`getOrders`、`getItem` は葉、効果を持つ `addToCart`、`attachWarranty` は配線）: `getMe`（§11.1 `server.getMe({})`）、`getOrders`（§11.1）、`getItem`、`addToCart`（§10.2 `server.addToCart({ item })`）、`attachWarranty`（§10.2 `server.attachWarranty({ line, plan })`）。`Token` 型の位置はすべて `@http.header(name: "Authorization") token: Token?` の一か所にし、§5.3 の `bind token to server.*` が BIND-1 / BIND-2 に触れないようにした。`getItem` だけは `Token` の位置を持たない（「該当する位置がない op には何もしない」の例）

`browser`（`browser.hozo`）:

- model: `OrderRow`（§11.2 `orders: OrderRow[]`）、`ProductQuery`
- union: `ProductPage`（§10.2 `when: ProductPage.Ready`）
- op: `chooseStandardWarranty`（§10.2 の `match` のアーム。葉）、`chooseWarrantyPlan`（表の 12）、`askWarrantyPlan`（`@page.dialog`）、`productPageVm`、`fetchItem`、`openProduct`（§10.2 `page: openProduct`。`@unavailableOnFailure` と `@page.route("/product", from: query)` を付けた配線）

## 出力

`testdata/expected/emit/` は、基底 `go` が出したもの（`docs/plugin-go.md`）。実装が書いたものを `-update` で置いた、後戻りの検出のためのゴールデン（`docs/testing.md`「コーパス」）。

| 出力 | 中身 |
|---|---|
| `gen/server/` | `server` は内部の op を持つので、定義は内側のフォルダ `inner/`、入口は `export` の op だけの `Ops` と、型・状態の単位の再公開（`docs/plugin-go.md`「内側のフォルダと入口」）。すべての op が出る: 配線（効果を呼ぶ op を含む）は `inner/wiring.go`、本体のない op は、どれも `Leaves`（同「op の置き場」）。効果は、宛先の状態の単位のメソッド（`inner/states.go` の `SessionsState.CreateSession`、`CartsState.AddCartLine` など。同「状態の単位」）で、配線が `o.sessions.CreateSession(ctx, …)` の形で呼ぶ。引数を持つ例外は、引数ごとにフィールド一つの構造体（`ClientError{Value Violations}`。同「exception」） |
| `gen/browser/` | 同じく `inner/` と入口。述語で決まる分類（`WarrantyKind`、`WarrantyAvailable`）の `classify.go`、`server` の op の口 `ServerOps`、`server` の入口のパッケージの型の import |
| `gen/http/`、`gen/page/` | 語彙の型（`http` の例外、`page.Route` など）だけ。op を持たないので、`inner/` はない。`http.BadRequest` は、二つの引数（`message`、`violations`）が、そのまま二つのフィールド |

相乗りを並べていないので、語彙のデコレータ（`@http.get`、`@page.route`、`@db.table` など）と、利用者が `valid` に足した制約（§6.5）は印のままで、出力に現れない（`docs/plugin-go.md`「デコレータ」）。

## 設計書の例が規則に触れるところ

設計書の原文のまま写すと `docs/` の規則に当たる箇所と、規則の側が決めていない箇所。上の表の番号を引く。

| 設計書 | 規則 | 扱い |
|---|---|---|
| §5.2 `state token: single<Token?>;`（`browser`） | §3.2「別の名前空間の宣言は、常に名前空間で修飾して参照する」。`docs/resolve.md` RES-1「名前が見つからない」。`docs/check.md` STATE-3「状態の要素の型が `optional`」 | 表の 4（`docs/decisions.md`「設計書から離れた決定」S2） |
| §9 `effect removeToken on token.put;   // none を入れる` | `docs/check.md`「状態の形」: `put` は列数を変えない。空にするのは `delete`（S2） | 表の 13 |
| §11.2 `Ready: { userName: string; orders: OrderRow[] };` | `docs/grammar.md`「model」`Field = [ DocComment ] { Decorator } FieldName ":" Type ";"` | 表の 6 |
| §6.5 `model User { @valid.length(min: 1, max: 100) name: string; }` | `docs/resolve.md`「同じ名前空間に同じ名前の宣言が二つあれば RES-6」 | 表の 5。設計書では「フィールドに付けた制約」の説明のための抜粋と読める |
| §7.3 `fn placeOrder(in: OrderInput): Order effects createOrder \| notifyUser throws NotificationFailed keeps createOrder prevents notifyUser;` | `docs/grammar.md`「decorator の宣言」`Relation = ( "prevents" \| "keeps" ) "effects"`（効果の名前は SYNTAX）。`docs/check.md` STATE-5「葉の op が `reads` / `effects` の節を書いた」 | 表の 14、20（`docs/decisions.md`「設計書から離れた決定」S8、S9） |
| §11.2 `model OrdersQuery { page: PosInt = 1; ... }` | `docs/grammar.md`「model」: フィールドに既定値は書けない（SYNTAX） | 表の 19 |
| §10.1 `default;`、`condition WarrantyAvailable of server.Item;`、§10.2 `default => none;` | `docs/grammar.md`「classify」「配線」: 名前のないケース、`condition` の宣言、`default` のアームは書けない（SYNTAX） | 表の 12、23 |
| §5.2 `@db state users: list<User>;` | `docs/resolve.md` RES-1（グローバルのデコレータ `db` はない） | 表の 17 |
| §8.1 `decorator redirectOn(error: ApiError, to: RedirectByError)` | `docs/grammar.md`「予約語」の `to` | 表の 7（`docs/decisions.md` に既にある） |
| §10.2 `@page.interaction(page: openProduct, when: ProductPage.Ready)`、§11.2 `orders: OrderRow[]`、§7.3 の `CancelInput` / `Order` / `refund` など | 宣言がないので RES-1 | 「追加した宣言」で補った |
| §11.1 `fetchMe` の `@redirectToLogin`（`catches server.Unauthorized halts transitionToLogin`） | `server.getMe` が `server.Unauthorized` を投げること（`@requiresAuth`）を前提にしているが、`getMe` の宣言は設計書にない | 表の 9 |

このコーパスで `hozoc check` に error も warning も出ない想定（`docs/check.md` の各規則を手で追った結果。REL-4 は、関係の節を書いた配線（表の 20）の本体と照合し（節と、計算した分の、過不足を見る THROWS-1 と EFF-2 は、欠番になった: `docs/decisions.md`「決めたこと」381）、STATE-5 は葉の op が `reads` / `effects` を書いていないこと、MATCH-2 は `WarrantyKind` の三つのケースと `WarrantyAvailable` の `default`、BIND-4 は `server.*` への呼び出し六つ（`logout` `getMe` `getOrders` `getItem` `addToCart` `attachWarranty`）の必須の入力を確かめた）。
