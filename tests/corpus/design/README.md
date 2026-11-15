# コーパス `design`

設計書（`mew-ton/drafts` の `hozo/README.md`。以下「§n」はその節）§3.4、§5〜§11 の例を、名前空間ごとに一つのファイルにまとめたプロジェクト（`docs/testing.md`「コーパス」）。

| ファイル | 中身 |
|---|---|
| `hozo.toml` | 単位（`[[emit]]`）が二つ: `namespace = "server"` と `namespace = "browser"`。どちらも `transpilers = ["pick"]`（Go のトランスパイラ。下の表の 1） |
| `plugins/pick/` | テスト用のプラグイン（`tests/plugins/pick`。トランスパイラ `pick` を提供する）のマニフェストと `pick.json`。設計書のプラグインの代わりに、既定の出力の対象を語彙のデコレータで選ぶ（下の「既定の出力」） |
| `server.hozo` | `namespace server` の例すべてと、§6.5 の `namespace valid { decorator checkDigit ... }` |
| `browser.hozo` | `namespace browser` の例すべて |

設計書の言葉とドキュメンテーションコメントはそのまま写した。各ブロックの上の `// §n.m` が出典、`// 追加:` が設計書にない宣言。標準の語彙（`http` / `db` / `page` / `valid` と、グローバルの `db` / `localStorage`。§4.2）は hozoc が同梱する（`docs/vocab.md`）ので、ここには写していない。

`docs/testing.md`「コーパス」は一覧の置き場を `tests/corpus/README.md` としているが、コーパスごとに分けるためこのディレクトリに置いた。

ゴールデン（`docs/testing.md`「コーパス」、`tests/corpus_emit_test.go`）は `testdata/expected/` の下（Go のツールが `.go` を拾わないように `testdata`）: `check.txt` は `hozoc check` の text 出力、`emit/` は `hozoc emit` が書き出すファイルの木。更新は `go test ./tests -update`。

## 設計書から変えた箇所

| # | 設計書の箇所と原文 | このコーパスでの書き方 | 理由 |
|---|---|---|---|
| 1 | §4.5 `[browser] language = "ts"`、`plugins = [...]`（名前空間ごとのテーブル） | 単位の一覧（`[[emit]]`）で、`server` も `browser` も `transpilers = ["pick"]`（テスト用の Go のトランスパイラ） | `hozo.toml` は名前空間ごとのテーブルではなく単位の一覧（`docs/config.md`「形」、`docs/decisions.md`「設計書を変える決定」S14）。言語はトランスパイラが持つ。TS の既定の出力は段階 5 まで存在せず、TS のトランスパイラを書けば CFG-11 のエラー（`docs/config.md`「診断」、`docs/decisions.md` 6）。設計書のプラグイン（http のサーバー、ルーター、db）はまだないので、そのトランスパイラを書けば CFG-14。出力の対象を選ぶために、テスト用のプラグインを置く（下の「既定の出力」） |
| 2 | §7.1 `model LoginInput { @http.header(name: "Authorization") token?: Token; ... }`、§9 `model LogoutInput { ... token?: Token; }`、§11.2 `model OrdersQuery { page: PosInt = 1; status?: server.OrderStatus; }` | `token: Token?`、`status: server.OrderStatus?` | 省略可能は型の `?` だけで書く（`docs/grammar.md`「決めたこと」、`docs/decisions.md` 8、「設計書へ返すこと」の「省略可能の書き方」） |
| 3 | §5.2 `@page.path state location: single<Route>;` | `single<page.Route>` | `Route` は語彙 `page` の宣言なので修飾する（`docs/resolve.md`「手続き」、`docs/decisions.md`「設計書へ返すこと」の「`location` の型」） |
| 4 | §5.2 `namespace browser { @localStorage state token: single<Token?>; }` | `nullable<server.Token>` | 二つの理由。(a) `Token` は `server` の scalar（§6.5）。別の名前空間の宣言は常に修飾する（§3.2、NS-1）。写したままだと RES-1（`docs/resolve.md`「診断」、`docs/decisions.md`「設計書へ返すこと」の「`Token` の修飾」）。(b) 空になりうる状態は `nullable<T>` と書く（`docs/decisions.md`「設計書を変える決定」S2）。`single<T?>` も同じ型（`nullable<T>`）として読める（S21）が、コーパスは形の語で書く |
| 5 | §6.5 `model User { @valid.length(min: 1, max: 100) name: string; }`（§5.2 の `model User` と別に書かれている） | §5.2 の `User` の `name` に `@valid.length(min: 1, max: 100)` を付けて一つにした | 同じ名前空間に同じ名前の宣言が二つあれば RES-6（`docs/resolve.md`「名前空間の併合と重複」） |
| 6 | §11.2 `Ready: { userName: string; orders: OrderRow[] };`、`Empty: { userName: string };` | `Ready: { userName: string; orders: OrderRow[]; };`、`Empty: { userName: string; };` | その場で書く model のフィールドは `;` で終わる（`docs/grammar.md`「model」`Field = ... ";"`）。`ProfilePage.Ready` も同じ |
| 7 | §8.1 `decorator redirectOn(error: ApiError, to: RedirectByError) catches error halts to;`、`decorator redirectToLogin = redirectOn(server.Unauthorized, transitionToLogin);` | 書かない | スコープ外（`docs/decisions.md`「スコープ外にした設計書の例」1 行目）。引数名 `to` は予約語（`docs/grammar.md`「予約語」） |
| 8 | §11.1 `@redirectToLogin` `@cache(key: "me", staleTime: 5m)` `op fetchMe()`、`@redirectToLogin` `@cache(key: ["orders", query], staleTime: 30s)` `op fetchOrders(query)` | デコレータを外し、配線と `throws http.NetworkError \| http.ServerError` だけを残した | 7 と同じ行、および「スコープ外」2 行目（`cache`）。§11.2 の `openOrders` / `openProfile` が呼ぶので op 自体は残す |
| 9 | §8.1 `decorator requiresAuth throws Unauthorized prevents effects;` は、§11.1 の `fetchMe` に付く `@redirectToLogin` が `server.Unauthorized` を吸収することから、`server.getMe` に付いていると読める | 追加した `getMe` / `getOrders` には `@requiresAuth` を付けない。`requiresAuth` は宣言だけ残す | 8 で `@redirectToLogin` を外したため、付ければ `fetchMe` / `fetchOrders` が `server.Unauthorized` を宣言も吸収もせず THROWS-1（`docs/check.md`「throws」）。`fetchMe` の `throws` を増やせば `openOrders` / `openProfile`（「fallback が吸収するので throws は書かない」）にも波及する |
| 10 | §6.5 `namespace valid { decorator checkDigit for scalar \| field; }` | `server.hozo` の先頭に置いた | `server.EmployeeCode` が使う。`valid` は語彙の名前空間なので専用のファイルにせず、使う側のファイルに置いた（§3.2「語彙プラグインの名前空間にも、ユーザーが宣言を足せる」、`docs/vocab.md`「読み込み」） |
| 11 | §3.4 `op requestPasswordReset(email: Email) effects sendResetMail throws RateLimited prevents effects;`（名前空間の記述なし） | `server` に置いた | `@see login` が `server.login` を指すため |
| 12 | §10.2 の文の断片 `const plan = match item by WarrantyAvailable { WarrantyAvailable => askWarrantyPlan(item); default => none; };` | 追加した `op chooseWarrantyPlan` の本体に置いた | 文は op の本体にしか書けない（`docs/grammar.md`「配線」）。`condition` と `default` のアームの例を残すため |
| 13 | §9 `effect removeToken on token.put;   // none を入れる` | `effect removeToken on token.delete;` | 空にするのは `delete`（`docs/decisions.md`「設計書を変える決定」S2、`docs/check.md`「状態の形」の表: `nullable` の `delete` は `HasItems` → `Empty`）。`put` は列数を変えない組み込みの効果なので、空にする意味にならない |
| 14 | §7.3 `op placeOrder(in: OrderInput): Order effects createOrder \| notifyUser throws NotificationFailed keeps createOrder prevents notifyUser;`（葉の op） | 葉の op `saveOrder`（`effects createOrder`）と `sendOrderNotice`（`effects notifyUser`、`throws NotificationFailed prevents effects`）を足し、`placeOrder` はその二つを順に呼ぶ配線（`throws NotificationFailed`。`effects` も `prevents` / `keeps` も書かない） | 葉の op が直に持つ効果は高々一つ（`docs/decisions.md`「設計書を変える決定」S8、`docs/check.md` EFF-3）。`prevents` / `keeps` の目的語は `effects` だけで、効果を名前で書き分ける形はない（同 S9、`docs/grammar.md`「決めたこと」）。原文のままだと SYNTAX。配線にすると、`Signature.relations` は `NotificationFailed` × `createOrder` = `kept`、`NotificationFailed` × `notifyUser` = `prevented` と計算され（`docs/check.md`「計算」手順 0 の配線）、原文が手で書いていた関係と同じになる（`tests/view_test.go` の `TestDesignPlaceOrderRelations`） |
| 15 | §5.2 `model PublicUser is User omit hash;` | `model PublicUser extends User { override hash: never; }` | `model A is B omit …` はなくなり、継承した model は `extends` と `override` で書く（`docs/decisions.md`「設計書を変える決定」S22、`docs/grammar.md`「model」）。写したままだと SYNTAX |
| 16 | §5.3、§9〜§11 の、`browser` が呼ぶ `server` の op（`op logout(...)`、追加した `getMe` / `getOrders` / `getItem` / `addToCart` / `attachWarranty`） | `export op ...` | 別の名前空間から呼べる op には `export` を付ける（S23、`docs/check.md`「export」）。付けなければ NS-5。`server` の中からしか呼ばない op（`login`、`placeOrder` など）には付けていない |

## 追加した宣言

設計書の例が参照しているのに宣言がないものと、それを成り立たせるための最小限の宣言。型と名前は設計書の文章（§10.3 の `item.hasMakerWarranty` / `item.yamadaEligible` など）に合わせた。

`server`（`server.hozo`）:

- scalar: `UserStatus`（§5.2 `User.status`）、`OrderId`、`OrderStatus`（§11.2 `status?: server.OrderStatus`）、`ItemId`、`PlanId`（§10.2 `server.PlanId`）
- model: `Item`（§10.1 `classify ... of server.Item`）、`CartLine`、`WarrantyLine`（§10.2）、`Order`（§7.3）、`OrderSummary`（§11.1）、`Notification`、`PasswordReset`、`CancelInput`、`OrderInput`（§7.3）、`MeInput`、`GetOrdersInput`、`GetItemInput`、`AddToCartInput`、`AttachWarrantyInput`
- state: `items`、`carts`、`orders`、`notifications`、`passwordResets`（追加した効果の `on` の先）
- effect: `refund`、`createOrder`、`notifyUser`（§7.3）、`sendResetMail`（§3.4）、`addCartLine`、`attachWarrantyToLine`
- exception: `RefundPending`、`NotificationFailed`（§7.3）、`RateLimited`（§3.4。`@http.status(429)` を付けた）
- op（葉。§7.3 の `placeOrder` を分けたもの。表の 14）: `saveOrder`、`sendOrderNotice`
- op（葉。すべて `@http.get` / `@http.post`）: `getMe`（§11.1 `server.getMe({})`）、`getOrders`（§11.1）、`getItem`、`addToCart`（§10.2 `server.addToCart({ item })`）、`attachWarranty`（§10.2 `server.attachWarranty({ line, plan })`）。`Token` 型の位置はすべて `@http.header(name: "Authorization") token: Token?` の一か所にし、§5.3 の `bind token to server.*` が BIND-1 / BIND-2 に触れないようにした。`getItem` だけは `Token` の位置を持たない（「該当する位置がない op には何もしない」の例）

`browser`（`browser.hozo`）:

- model: `OrderRow`（§11.2 `orders: OrderRow[]`）、`ProductQuery`
- union: `ProductPage`（§10.2 `when: ProductPage.Ready`）
- op: `chooseStandardWarranty`（§10.2 の `match` のアーム。葉）、`chooseWarrantyPlan`（表の 12）、`askWarrantyPlan`（`@page.dialog`）、`productPageVm`、`fetchItem`、`openProduct`（§10.2 `page: openProduct`。`@unavailableOnFailure` と `@page.route("/product", from: query)` を付けた配線）

## 既定の出力

既定の出力は、プラグインが選んだ宣言だけを出す（`docs/emit-go.md`「出力の対象」）。設計書のプラグインがまだないので、テスト用の `pick`（`tests/plugins/pick`）が、`plugins/pick/pick.json` に書いた語彙のデコレータで選ぶ。設計書の例（`docs/plugin-protocol.md`「選択」の `roots` / `stops` / `states`）に合わせた選び方:

| `select` | デコレータ | 選ばれるもの |
|---|---|---|
| `roots` | `http.get`、`http.post`（http のサーバー）、`page.route`、`page.interaction`（ルーター） | `server` は `login` などの API の op。`browser` は `openOrders` / `openProfile` / `openProduct` と `onAddToCart`、そこから配線で辿られる op |
| `stops` | `page.dialog` | `browser` の `askYamadaWarranty` / `askWarrantyPlan`（口だけを出す） |
| `states` | `db` | `server` の `@db` の状態 |

選ばれない宣言は出力されない: `server` の `placeOrder` / `saveOrder` / `sendOrderNotice` / `cancelOrder`（`@http.*` が付いていない）、`browser` の `logout` / `requestLogout` / `clearToken`（どの起点からも辿られない）。`@page.route` や `@page.fallback` などのデコレータは、どのプラグインも `wraps` で引き受けていないので、包む関数にならない。`browser` は、`server` の出力が持つ型（API の op から辿れるもの）を import する。

## 設計書の例が規則に触れるところ

設計書の原文のまま写すと `docs/` の規則に当たる箇所と、規則の側が決めていない箇所。上の表の番号を引く。

| 設計書 | 規則 | 扱い |
|---|---|---|
| §5.2 `state token: single<Token?>;`（`browser`） | §3.2「別の名前空間の宣言は、常に名前空間で修飾して参照する」。`docs/resolve.md` RES-1「名前が見つからない」。`docs/check.md` STATE-3「状態の要素の型が `optional`」 | 表の 4。`docs/decisions.md`「設計書へ返すこと」の「`Token` の修飾」と「状態を分類で表す」（S2） |
| §9 `effect removeToken on token.put;   // none を入れる` | `docs/check.md`「状態の形」: `put` は列数を変えない。空にするのは `delete`（S2） | 表の 13。`docs/decisions.md`「設計書へ返すこと」の「状態を分類で表す」 |
| §11.2 `Ready: { userName: string; orders: OrderRow[] };` | `docs/grammar.md`「model」`Field = [ DocComment ] { Decorator } Ident ":" Type [ "=" Literal ] ";"` | 表の 6。同じく返す候補 |
| §6.5 `model User { @valid.length(min: 1, max: 100) name: string; }` | `docs/resolve.md`「同じ名前空間に同じ名前の宣言が二つあれば RES-6」 | 表の 5。設計書では「フィールドに付けた制約」の説明のための抜粋と読める |
| §7.3 `op placeOrder(in: OrderInput): Order effects createOrder \| notifyUser throws NotificationFailed keeps createOrder prevents notifyUser;` | `docs/grammar.md`「decorator の宣言」`Relation = ( "prevents" \| "keeps" ) "effects"`（効果の名前は SYNTAX）。`docs/check.md` EFF-3「葉の op の `Op.effects` が二つ以上」 | 表の 14。`docs/decisions.md`「設計書へ返すこと」の「状態を分類で表す」（S8、S9） |
| §11.2 `model OrdersQuery { page: PosInt = 1; ... }`（`PosInt` は `@valid.min(1) scalar PosInt extends int;`） | `docs/types.md` TYPE-1「核の型（リテラルを含む）の値を `scalar` の位置に渡した」。同「互換」10 はリテラルを核の型にしか渡さない。フィールドの既定値（`docs/grammar.md`「model」`[ "=" Literal ]`）をどう照合するかは `docs/types.md` に書かれていない | そのまま写した。既定値のリテラルは `scalar` の元の型（`base`）に対して照合する、と `docs/types.md` で決める必要がある。決めなければ設計書の例が TYPE-1 に落ちる |
| §8.1 `decorator redirectOn(error: ApiError, to: RedirectByError)` | `docs/grammar.md`「予約語」の `to` | 表の 7（`docs/decisions.md` に既にある） |
| §10.2 `@page.interaction(page: openProduct, when: ProductPage.Ready)`、§11.2 `orders: OrderRow[]`、§7.3 の `CancelInput` / `Order` / `refund` など | 宣言がないので RES-1 | 「追加した宣言」で補った |
| §11.1 `fetchMe` の `@redirectToLogin`（`catches server.Unauthorized halts transitionToLogin`） | `server.getMe` が `server.Unauthorized` を投げること（`@requiresAuth`）を前提にしているが、`getMe` の宣言は設計書にない | 表の 9 |

このコーパスで `hozoc check` に error も warning も出ない想定（`docs/check.md` の各規則を手で追った結果。THROWS-1 は配線の op ごとに呼び先の `Sig(callee, ns)` を集めて確かめ、EFF-2 は `effects` を書いた配線の op がないので対象なし、EFF-3 は葉の op の `effects` がどれも一つ以下、MATCH-2 は `WarrantyKind` の三つのケースと `WarrantyAvailable` の `default`、BIND-4 は `server.*` への呼び出し六つ（`logout` `getMe` `getOrders` `getItem` `addToCart` `attachWarranty`）の必須の入力を確かめた）。
