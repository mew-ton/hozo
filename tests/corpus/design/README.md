# コーパス `design`

設計書（`mew-ton/drafts` の `hozo/README.md`。以下「§n」はその節）§3.4、§5〜§11 の例を、名前空間ごとに一つのファイルにまとめたプロジェクト（`docs/testing.md`「コーパス」）。

| ファイル | 中身 |
|---|---|
| `hozo.toml` | `[server]` と `[browser]`。どちらも `language = "go"`（下の表の 1） |
| `server.hozo` | `namespace server` の例すべてと、§6.5 の `namespace valid { decorator checkDigit ... }` |
| `browser.hozo` | `namespace browser` の例すべて |

設計書の言葉とドキュメンテーションコメントはそのまま写した。各ブロックの上の `// §n.m` が出典、`// 追加:` が設計書にない宣言。標準の語彙（`http` / `db` / `page` / `valid` と、グローバルの `db` / `localStorage`。§4.2）は hozoc が同梱する（`docs/vocab.md`）ので、ここには写していない。

`docs/testing.md`「コーパス」は一覧の置き場を `tests/corpus/README.md` としているが、コーパスごとに分けるためこのディレクトリに置いた。

ゴールデン（`docs/testing.md`「コーパス」、`tests/corpus_emit_test.go`）は `testdata/expected/` の下（Go のツールが `.go` を拾わないように `testdata`）: `check.txt` は `hozoc check` の text 出力、`emit/` は `hozoc emit` が書き出すファイルの木。更新は `go test ./tests -update`。

## 設計書から変えた箇所

| # | 設計書の箇所と原文 | このコーパスでの書き方 | 理由 |
|---|---|---|---|
| 1 | §4.5 `[browser] language = "ts"`、`plugins = [...]` | `language = "go"`、`plugins` なし | TS の既定の出力は段階 5 まで存在せず、`"ts"` は CFG-11 のエラー（`docs/config.md`「診断」、`docs/decisions.md` 6）。プラグインもまだないので書けば CFG-9 |
| 2 | §7.1 `model LoginInput { @http.header(name: "Authorization") token?: Token; ... }`、§9 `model LogoutInput { ... token?: Token; }`、§11.2 `model OrdersQuery { page: PosInt = 1; status?: server.OrderStatus; }` | `token: Token?`、`status: server.OrderStatus?` | 省略可能は型の `?` だけで書く（`docs/grammar.md`「決めたこと」、`docs/decisions.md` 8、「設計書へ返すこと」の「省略可能の書き方」） |
| 3 | §5.2 `@page.path state location: single<Route>;` | `single<page.Route>` | `Route` は語彙 `page` の宣言なので修飾する（`docs/resolve.md`「手続き」、`docs/decisions.md`「設計書へ返すこと」の「`location` の型」） |
| 4 | §5.2 `namespace browser { @localStorage state token: single<Token?>; }` | `single<server.Token?>` | `Token` は `server` の scalar（§6.5）。別の名前空間の宣言は常に修飾する（§3.2、NS-1）。写したままだと RES-1（`docs/resolve.md`「診断」）。`docs/decisions.md`「設計書へ返すこと」にはまだない |
| 5 | §6.5 `model User { @valid.length(min: 1, max: 100) name: string; }`（§5.2 の `model User` と別に書かれている） | §5.2 の `User` の `name` に `@valid.length(min: 1, max: 100)` を付けて一つにした | 同じ名前空間に同じ名前の宣言が二つあれば RES-6（`docs/resolve.md`「名前空間の併合と重複」） |
| 6 | §11.2 `Ready: { userName: string; orders: OrderRow[] };`、`Empty: { userName: string };` | `Ready: { userName: string; orders: OrderRow[]; };`、`Empty: { userName: string; };` | その場で書く model のフィールドは `;` で終わる（`docs/grammar.md`「model」`Field = ... ";"`）。`ProfilePage.Ready` も同じ |
| 7 | §8.1 `decorator redirectOn(error: ApiError, to: RedirectByError) catches error halts to;`、`decorator redirectToLogin = redirectOn(server.Unauthorized, transitionToLogin);` | 書かない | スコープ外（`docs/decisions.md`「スコープ外にした設計書の例」1 行目）。引数名 `to` は予約語（`docs/grammar.md`「予約語」） |
| 8 | §11.1 `@redirectToLogin` `@cache(key: "me", staleTime: 5m)` `op fetchMe()`、`@redirectToLogin` `@cache(key: ["orders", query], staleTime: 30s)` `op fetchOrders(query)` | デコレータを外し、配線と `throws http.NetworkError \| http.ServerError` だけを残した | 7 と同じ行、および「スコープ外」2 行目（`cache`）。§11.2 の `openOrders` / `openProfile` が呼ぶので op 自体は残す |
| 9 | §8.1 `decorator requiresAuth throws Unauthorized prevents effects;` は、§11.1 の `fetchMe` に付く `@redirectToLogin` が `server.Unauthorized` を吸収することから、`server.getMe` に付いていると読める | 追加した `getMe` / `getOrders` には `@requiresAuth` を付けない。`requiresAuth` は宣言だけ残す | 8 で `@redirectToLogin` を外したため、付ければ `fetchMe` / `fetchOrders` が `server.Unauthorized` を宣言も吸収もせず THROWS-1（`docs/check.md`「throws」）。`fetchMe` の `throws` を増やせば `openOrders` / `openProfile`（「fallback が吸収するので throws は書かない」）にも波及する |
| 10 | §6.5 `namespace valid { decorator checkDigit for scalar \| field; }` | `server.hozo` の先頭に置いた | `server.EmployeeCode` が使う。`valid` は語彙の名前空間なので専用のファイルにせず、使う側のファイルに置いた（§3.2「語彙プラグインの名前空間にも、ユーザーが宣言を足せる」、`docs/vocab.md`「読み込み」） |
| 11 | §3.4 `op requestPasswordReset(email: Email) effects sendResetMail throws RateLimited prevents effects;`（名前空間の記述なし） | `server` に置いた | `@see login` が `server.login` を指すため |
| 12 | §10.2 の文の断片 `const plan = match item by WarrantyAvailable { WarrantyAvailable => askWarrantyPlan(item); default => none; };` | 追加した `op chooseWarrantyPlan` の本体に置いた | 文は op の本体にしか書けない（`docs/grammar.md`「配線」）。`condition` と `default` のアームの例を残すため |

## 追加した宣言

設計書の例が参照しているのに宣言がないものと、それを成り立たせるための最小限の宣言。型と名前は設計書の文章（§10.3 の `item.hasMakerWarranty` / `item.yamadaEligible` など）に合わせた。

`server`（`server.hozo`）:

- scalar: `UserStatus`（§5.2 `User.status`）、`OrderId`、`OrderStatus`（§11.2 `status?: server.OrderStatus`）、`ItemId`、`PlanId`（§10.2 `server.PlanId`）
- model: `Item`（§10.1 `classify ... of server.Item`）、`CartLine`、`WarrantyLine`（§10.2）、`Order`（§7.3）、`OrderSummary`（§11.1）、`Notification`、`PasswordReset`、`CancelInput`、`OrderInput`（§7.3）、`MeInput`、`GetOrdersInput`、`GetItemInput`、`AddToCartInput`、`AttachWarrantyInput`
- state: `items`、`carts`、`orders`、`notifications`、`passwordResets`（追加した効果の `on` の先）
- effect: `refund`、`createOrder`、`notifyUser`（§7.3）、`sendResetMail`（§3.4）、`addCartLine`、`attachWarrantyToLine`
- exception: `RefundPending`、`NotificationFailed`（§7.3）、`RateLimited`（§3.4。`@http.status(429)` を付けた）
- op（葉。すべて `@http.get` / `@http.post`）: `getMe`（§11.1 `server.getMe({})`）、`getOrders`（§11.1）、`getItem`、`addToCart`（§10.2 `server.addToCart({ item })`）、`attachWarranty`（§10.2 `server.attachWarranty({ line, plan })`）。`Token` 型の位置はすべて `@http.header(name: "Authorization") token: Token?` の一か所にし、§5.3 の `bind token to server.*` が BIND-1 / BIND-2 に触れないようにした。`getItem` だけは `Token` の位置を持たない（「該当する位置がない op には何もしない」の例）

`browser`（`browser.hozo`）:

- model: `OrderRow`（§11.2 `orders: OrderRow[]`）、`ProductQuery`
- union: `ProductPage`（§10.2 `when: ProductPage.Ready`）
- op: `chooseStandardWarranty`（§10.2 の `match` のアーム。葉）、`chooseWarrantyPlan`（表の 12）、`askWarrantyPlan`（`@page.dialog`）、`productPageVm`、`fetchItem`、`openProduct`（§10.2 `page: openProduct`。`@unavailableOnFailure` と `@page.route("/product", from: query)` を付けた配線）

## 設計書の例が規則に触れるところ

設計書の原文のまま写すと `docs/` の規則に当たる箇所と、規則の側が決めていない箇所。上の表の番号を引く。

| 設計書 | 規則 | 扱い |
|---|---|---|
| §5.2 `state token: single<Token?>;`（`browser`） | §3.2「別の名前空間の宣言は、常に名前空間で修飾して参照する」。`docs/resolve.md` RES-1「名前が見つからない」 | 表の 4。`docs/decisions.md`「設計書へ返すこと」に足す候補 |
| §11.2 `Ready: { userName: string; orders: OrderRow[] };` | `docs/grammar.md`「model」`Field = [ DocComment ] { Decorator } Ident ":" Type [ "=" Literal ] ";"` | 表の 6。同じく返す候補 |
| §6.5 `model User { @valid.length(min: 1, max: 100) name: string; }` | `docs/resolve.md`「同じ名前空間に同じ名前の宣言が二つあれば RES-6」 | 表の 5。設計書では「フィールドに付けた制約」の説明のための抜粋と読める |
| §7.3 `op placeOrder(in: OrderInput): Order effects createOrder \| notifyUser throws NotificationFailed keeps createOrder prevents notifyUser;` | `docs/check.md` REL-3（warning）「`relations.effects` が `all` でなく名前の列」。設計書 §7.3 自身が「言語はこの書き方をサポートするが、検査器は警告する」としている | そのまま写した。`hozoc check` のゴールデン（`expected.check.txt`）には `placeOrder` の `throws` に REL-3 の warning が出る |
| §11.2 `model OrdersQuery { page: PosInt = 1; ... }`（`PosInt` は `@valid.min(1) scalar PosInt extends int;`） | `docs/types.md` TYPE-1「核の型（リテラルを含む）の値を `scalar` の位置に渡した」。同「互換」10 はリテラルを核の型にしか渡さない。フィールドの既定値（`docs/grammar.md`「model」`[ "=" Literal ]`）をどう照合するかは `docs/types.md` に書かれていない | そのまま写した。既定値のリテラルは `scalar` の元の型（`base`）に対して照合する、と `docs/types.md` で決める必要がある。決めなければ設計書の例が TYPE-1 に落ちる |
| §8.1 `decorator redirectOn(error: ApiError, to: RedirectByError)` | `docs/grammar.md`「予約語」の `to` | 表の 7（`docs/decisions.md` に既にある） |
| §10.2 `@page.interaction(page: openProduct, when: ProductPage.Ready)`、§11.2 `orders: OrderRow[]`、§7.3 の `CancelInput` / `Order` / `refund` など | 宣言がないので RES-1 | 「追加した宣言」で補った |
| §11.1 `fetchMe` の `@redirectToLogin`（`catches server.Unauthorized halts transitionToLogin`） | `server.getMe` が `server.Unauthorized` を投げること（`@requiresAuth`）を前提にしているが、`getMe` の宣言は設計書にない | 表の 9 |

このコーパスで `hozoc check` に error は出ない想定。warning は上の REL-3 だけの想定（`docs/check.md` の各規則を手で追った結果。THROWS-1 は配線の op ごとに呼び先の `Sig(callee, browser)` を集めて確かめ、EFF-2 は `effects` を書いた配線の op がないので対象なし、MATCH-2 は `WarrantyKind` の三つのケースと `WarrantyAvailable` の `default`、BIND-4 は `server.*` への呼び出し六つ（`logout` `getMe` `getOrders` `getItem` `addToCart` `attachWarranty`）の必須の入力を確かめた）。
