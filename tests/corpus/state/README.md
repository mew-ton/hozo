# コーパス `state`

状態を分類で表す決定（`docs/decisions.md`「設計書を変える決定」S1〜S6、S8）の構文を、一つの名前空間で一か所ずつ使うプロジェクト（`docs/testing.md`「コーパス」）。`hozoc check` が error なしで通ること、IR（`docs/ir.md`）に載る形、Go の既定の出力（`docs/emit-go.md`）の形を確かめる。

| ファイル | 中身 |
|---|---|
| `hozo.toml` | `[shop]`（`language = "go"`、`plugins = ["pick"]`） |
| `plugins/pick/` | テスト用のプラグイン（`tests/plugins/pick`）のマニフェストと `pick.json`（`"all": true`）。すべての op を起点にし、すべての状態（基底の状態と状態の分類）を選ぶ。既定の出力はプラグインが選んだ宣言だけを出す（`docs/emit-go.md`「出力の対象」）ので、これがなければ何も出力されない |
| `shop.hozo` | `namespace shop`。三つの形の基底の状態、状態の分類、組み込みの効果と遷移の効果、状態での `match` |
| `testdata/expected/check.txt` | `hozoc check` の text 出力のゴールデン（`tests/corpus_emit_test.go` の `TestCorpusCheckGolden`）。更新は `go test ./tests -update` |
| `testdata/expected/emit/` | `hozoc emit` が書き出すファイルの木のゴールデン（同 `TestCorpusEmitGolden`。書き出した Go は一時のモジュールで `go vet` も通す） |

`.hozo` の各ブロックの上の `// docs/...「見出し」` のコメントが、その構文が確かめる文書の見出し。

## 見出し → 構文

`tests/view_test.go` の `TestStateCorpusIR` のサブテストは、この表の見出しの名前で期待値を明示する（`internal/project` の `Load` → `Sema.Program`）。

| 文書の見出し | 構文 | 確かめること |
|---|---|---|
| `docs/ir.md`「state / exception / effect」の `State` | `state carts: list<Line>;`、`state token: nullable<Token>;`、`state title: single<string>;` | `shape` は `list` / `nullable` / `single`。`element` は `optional` にならない |
| `docs/ir.md`「classify」の `subject` | `classify Checkout of carts, orders { ... }`、`condition LoggedIn of token;`、`classify LineKind of Line { ... }` | 状態の分類は `{ kind: "states", states }`（FQN、書かれた順）、値の分類は `{ kind: "type", type }`。`condition` の糖衣は状態の分類でも同じ |
| `docs/ir.md`「state / exception / effect」の `Effect.target`（`builtin`） | `effect addLine on carts.post;`、`effect rename on title.put;`、`effect removeToken on token.delete;` | `{ kind: "builtin", state, operation }`。`writes` はその状態 |
| 同（`transition`、宛先が状態の分類） | `effect placeOrder on Checkout: Shopping -> Ordered;`、`effect startOver on Checkout: any -> Shopping;`、`effect touchCheckout on Checkout: Shopping \| Ordered -> any;`、`effect signIn on LoggedIn: any -> LoggedIn;` | `{ kind: "transition", of, from, to }`。`CaseSet` は `"any"` かケース名の列（書かれた順）。`writes` は分類の対象のすべての基底の状態 |
| 同（`transition`、宛先が基底の状態） | `effect clearCart on carts: HasItems -> Empty;`、`effect saveToken on token: Empty -> HasItems;` | `of` は状態の FQN、ケースは `Empty` / `HasItems`。`writes` はその状態 |
| `docs/ir.md`「配線」の `Match.subject`（`state`） | `match token by nullable { ... }`、`match carts by list { ... }` | `{ kind: "state", state }`、`classify` は `"nullable"` / `"list"` |
| 同（`classify`） | `match by Checkout { ... }`、`match by LoggedIn { LoggedIn => ...; default => ...; }` | `{ kind: "classify" }`、`classify` は分類の FQN |
| 同（`value`） | `match line by LineKind { ... }` | `{ kind: "value", atom }` |
| `docs/types.md`「式の型」の `match` | `const placed = match by Checkout { Shopping => submitOrder(); Ordered => none; };` と `return placed;`（戻り値は `Order?`） | 状態の `match` の型も、値の `match` と同じ決め方（`none` のアームがあれば `optional`） |
| `docs/check.md`「計算」手順 0（配線） | `openCart`（`HasItems` のアームだけが `emptyCart()` を呼ぶ）、`checkout` | 状態の `match` もアームごとの呼び出しとして計算する。一部のアームにしかない効果は succ が `unknown` |
| `docs/check.md`「状態と効果」EFF-3、「計算」手順 0（配線） | 葉の `op storeToken(token: Token) effects saveToken;` と `op markLoggedIn() effects signIn;`、それを順に呼ぶ配線の `op login(token: Token) { storeToken(token); markLoggedIn(); }` | 葉の op が直に持つ効果は一つ。配線の `login` の `Signature.effects` は `saveToken`、`signIn`（行の順）で、`Op.effects` は `[]` のまま |

## 書き直した箇所

| 元の書き方 | 今の書き方 | 理由 |
|---|---|---|
| `op login(token: Token) effects saveToken \| signIn;`（葉の op に二つの効果） | 葉の `storeToken`（`effects saveToken`）と `markLoggedIn`（`effects signIn`）、その二つを順に呼ぶ配線の `login` | 葉の op が直に持つ効果は高々一つ（`docs/decisions.md`「設計書を変える決定」S8、`docs/check.md` EFF-3）。元のままだと EFF-3 の error |

## 想定する診断

`hozoc check` の error も warning もない。

## 既定の出力

`testdata/expected/emit/gen/shop/` の、`docs/emit-go.md` の見出しごとの対応。

| `docs/emit-go.md` の見出し | 構文 | 出力 |
|---|---|---|
| 「state」の状態の単位 | `state carts: list<Line>;`、`state token: nullable<Token>;`、`state title: single<string>;` | `states.go` の `CartsState`（`IsEmpty`）、`TokenState`（`IsEmpty`、`Get`）、`TitleState`（`Get`）。束ねる `StateSet` と、パッケージの変数 `States`（宣言の順） |
| 「state」の命令 | `op addToCart(line: Line) effects addLine;`、`op submitOrder(): Order effects placeOrder ...;`、`op markLoggedIn() effects signIn;` | 効果を持つ葉の op は、その効果の宛先の状態の単位のフィールド（`CartsState.AddToCart`、`CheckoutState.SubmitOrder`、`LoggedInState.MarkLoggedIn`）。`Leaves` に載るのは効果を持たない葉（`showLogin` など）だけ |
| 「state」「classify」の状態の分類 | `classify Checkout of carts, orders { ... }`、`condition LoggedIn of token;` | `classify.go` に列挙の型と定数、`states.go` に `CheckoutState` / `LoggedInState`（ケースごとの述語）と `ClassifyCheckout()` / `ClassifyLoggedIn()`。値の分類 `LineKind` は `LineKindCases` と `ClassifyLineKind(c, v)`、`Classifiers` |
| 「effect」の遷移のコメント | `effect placeOrder on Checkout: Shopping -> Ordered;` など | `types.go` の定数のコメントに `// on shop.Checkout: Shopping -> Ordered`（`any`、基底の状態が宛先の `on shop.carts: HasItems -> Empty` も）。出るのは選ばれた op の効果の定数だけ（`rename` と `touchCheckout` はどの op も持たないので出ない） |
| 「match」の `match s by list` / `by nullable` | `openTop`、`openCart` | `wiring.go`: `empty, err := States.<S>.IsEmpty()` と `if empty { ... } else { ... }` |
| 「match」の `match by C` | `checkout`（値の形。`none` のアームがあるので `*Order`）、`add`（文の形。`default` のアーム） | `wiring.go`: `kind, err := Classify<C>()` と `switch` |
| 「実装の単位」 | すべて | 生成したコードは `context` を引数に取らない。`New(leaves, classifiers)` は状態を引数に取らず、配線は `States` を読む |

出力の細かい形（`none` のアームの枝を書かない、一つのアームに両方のケース、アームの中での `bind`）、選ばれなかった状態が出ないこと、入れていないフィールドが `*runtime.MissingError` になることは、`internal/emit/golang` の golden `testdata/state`（手で組んだ `View`）と、生成したコードを動かす `testdata/state/behavior_test.go`、単体テストが確かめる。
