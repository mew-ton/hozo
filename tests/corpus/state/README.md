# コーパス `state`

状態を分類で表す決定（`docs/decisions.md`「設計書を変える決定」S1〜S6、S8）の構文を、一つの名前空間で一か所ずつ使うプロジェクト（`docs/testing.md`「コーパス」）。`hozoc check` が error なしで通ること、IR（`docs/ir.md`）に載る形、Go の基底 `go` の出力（`docs/plugin-go.md`）の形を確かめる。

| ファイル | 中身 |
|---|---|
| `hozo.toml` | 単位が一つ: `namespace = "shop"`、`transpilers = ["go"]`（Go の基底。リポジトリの `plugins/go`。テストが `HOZO_PLUGIN_PATH` として渡す）、`out = "gen/shop"` |
| `shop.hozo` | `namespace shop`。三つの形の基底の状態、状態の分類、組み込みの効果と遷移の効果、状態での `match` |
| `testdata/expected/check.txt` | `hozoc check` の text 出力のゴールデン（`tests/corpus_emit_test.go` の `TestCorpusCheckGolden`）。更新は `go test ./tests -update` |
| `testdata/expected/emit/` | `hozoc emit` が書き出すファイルの木のゴールデン（同 `TestCorpusEmitGolden`。書き出した Go は一時のモジュールで `go vet` も通す） |

`.hozo` の各ブロックの上の `// docs/...「見出し」` のコメントが、その構文が確かめる文書の見出し。

## 見出し → 構文

`tests/view_test.go` の `TestStateCorpusIR` のサブテストは、この表の見出しの名前で期待値を明示する（`internal/project` の `Load` → `Sema.Program`）。

| 文書の見出し | 構文 | 確かめること |
|---|---|---|
| `docs/ir.md`「state / exception / effect」の `State` | `state carts: list<Line>;`、`state token: nullable<Token>;`、`state title: single<string>;` | `type` は状態の型（`array` / `optional` / そのまま）。`shape` は `list` / `nullable` / `single` で、`type` の最も外側から決まる。`element` は `optional` にならない |
| `docs/ir.md`「classify」の `subject` | `classify Checkout of carts, orders { ... }`、`condition LoggedIn of token;`、`classify LineKind of Line { ... }` | 状態の分類は `{ kind: "states", states }`（FQN、書かれた順）、値の分類は `{ kind: "type", type }`。`condition` の糖衣は状態の分類でも同じ |
| `docs/ir.md`「state / exception / effect」の `Effect.target`（`builtin`） | `effect addLine on carts.post;`、`effect rename on title.put;`、`effect removeToken on token.delete;` | `{ kind: "builtin", state, operation }`。`writes` はその状態 |
| 同（`transition`、宛先が状態の分類） | `effect placeOrder on Checkout: Shopping -> Ordered;`、`effect startOver on Checkout: any -> Shopping;`、`effect touchCheckout on Checkout: Shopping \| Ordered -> any;`、`effect signIn on LoggedIn: any -> LoggedIn;` | `{ kind: "transition", of, from, to }`。`CaseSet` は `"any"` かケース名の列（書かれた順）。`writes` は分類の対象のすべての基底の状態 |
| 同（`transition`、宛先が基底の状態） | `effect clearCart on carts: HasItems -> Empty;`、`effect saveToken on token: Empty -> HasItems;` | `of` は状態の FQN、ケースは `Empty` / `HasItems`。`writes` はその状態 |
| `docs/ir.md`「配線」の `Match.subject`（`value`。状態を名前で書いた対象） | `match token by nullable { ... }`、`match carts by list { ... }` | `{ kind: "value", atom }` で、`atom` は `{ kind: "state", state, path: [] }`（`docs/ir.md`「版」の 0.3 で、`subject` の `state` をやめた）。`classify` は標準の分類の名前 `"nullable"` / `"list"` |
| 同（`classify`） | `match by Checkout { ... }`、`match by LoggedIn { LoggedIn => ...; default => ...; }` | `{ kind: "classify" }`、`classify` は分類の FQN |
| 同（`value`。束縛か引数） | `match line by LineKind { ... }` | `{ kind: "value", atom }` で、`atom` は `{ kind: "name", path }` |
| `docs/types.md`「式の型」の `match` | `const placed = match by Checkout { Shopping => submitOrder(); Ordered => none; };` と `return placed;`（戻り値は `Order?`） | 状態の `match` の型も、値の `match` と同じ決め方（`none` のアームがあれば `optional`） |
| `docs/check.md`「計算」手順 0（配線） | `openCart`（`HasItems` のアームだけが `emptyCart()` を呼ぶ）、`checkout` | 状態の `match` もアームごとの呼び出しとして計算する。一部のアームにしかない効果は succ が `unknown` |
| `docs/check.md`「状態と効果」EFF-3、「計算」手順 0（配線） | 葉の `op storeToken(token: Token) effects saveToken;` と `op markLoggedIn() effects signIn;`、それを順に呼ぶ配線の `op login(token: Token) { storeToken(token); markLoggedIn(); }` | 葉の op が直に持つ効果は一つ。配線の `login` の `Signature.effects` は `saveToken`、`signIn`（行の順）で、`Op.effects` は `[]` のまま |

## 書き直した箇所

| 元の書き方 | 今の書き方 | 理由 |
|---|---|---|
| `op login(token: Token) effects saveToken \| signIn;`（葉の op に二つの効果） | 葉の `storeToken`（`effects saveToken`）と `markLoggedIn`（`effects signIn`）、その二つを順に呼ぶ配線の `login` | 葉の op が直に持つ効果は高々一つ（`docs/decisions.md`「設計書を変える決定」S8、`docs/check.md` EFF-3）。元のままだと EFF-3 の error |

## 想定する診断

`hozoc check` の error も warning もない。

## 出力

`testdata/expected/emit/gen/shop/` の、`docs/plugin-go.md` の見出しごとの対応。`shop` は `export` の op を持たない（すべて内部の op）ので、定義は内側のフォルダ `inner/` にあり、入口の `Ops` はメソッドを持たない（「内側のフォルダと入口」）。

| `docs/plugin-go.md` の見出し | 構文 | 出力（`inner/`） |
|---|---|---|
| 「状態の単位」の問い合わせ | `state carts: list<Line>;`、`state token: nullable<Token>;`、`state title: single<string>;` | `states.go` の `CartsState`（`IsEmpty`、`Get`）、`TokenState`（`IsEmpty`、`Get`）、`TitleState`（`Get`） |
| 「状態の単位」の命令 | `op addToCart(line: Line) effects addLine;`、`op submitOrder(): Order effects placeOrder ...;`、`op markLoggedIn() effects signIn;` | 効果を持つ葉の op は、その効果の宛先の状態の単位のメソッド（`CartsState.AddToCart`、`CheckoutState.SubmitOrder`、`LoggedInState.MarkLoggedIn`）。`Leaves` に載るのは、状態を読まず効果も持たない葉（`showLogin` など）だけ |
| 「状態の単位」「分類」の状態の分類 | `classify Checkout of carts, orders { ... }`、`condition LoggedIn of token;` | `classify.go` に列挙の型と定数、`ClassifyCheckout(ctx, s)` / `ClassifyLoggedIn(ctx, s)`。`states.go` に `CheckoutState` / `LoggedInState`（ケースごとの述語）。値の分類 `LineKind` は `LineKindCases` と `ClassifyLineKind(c, v)` |
| 「effect」とコメント | `effect placeOrder on Checkout: Shopping -> Ordered;` など | `types.go` の効果の定数（宣言したものすべて）。遷移は、命令のコメントに `（on shop.Checkout: Shopping -> Ordered）`（`any`、基底の状態が宛先の `on shop.carts: HasItems -> Empty` も） |
| 「match」の `match s by list` / `by nullable` | `openTop`、`openCart` | `wiring.go`: `o.<状態>.Get(ctx)` で読み、`if token != nil { ... } else { ... }` / `if len(carts) != 0 { ... } else { ... }` |
| 「match」の `match by C` | `checkout`（値の形。`none` のアームがあるので `*Order`）、`add`（値の分類の値の形と、状態の分類の文の形。`default` のアーム） | `wiring.go`: `k<n>, err := Classify<C>(...)` と `switch` |
| 「New」 | すべて | `New(leaves, carts, orders, token, title, checkout, loggedIn, lineKind)`: 状態の単位（基底の状態、状態の分類の順）と、述語で決まる分類の述語を、引数で受け取る |

出力の細かい形（`bind` で埋まる位置、アームの本体の組、継ぎ目、GO-* の診断）は、`plugins/go` の単体テスト（`internal/gobase` のゴールデンと、生成したコードを動かす振る舞いのテスト）が確かめる（`docs/testing.md`「単体テスト」）。
