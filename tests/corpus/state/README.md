# コーパス `state`

状態を分類で表す決定（`docs/decisions.md`「設計書を変える決定」S1〜S6）の構文を、一つの名前空間で一か所ずつ使うプロジェクト（`docs/testing.md`「コーパス」）。`hozoc check` が error なしで通ること、IR（`docs/ir.md`）に載る形、Go の既定の出力（`docs/emit-go.md`）の形を確かめる。

| ファイル | 中身 |
|---|---|
| `hozo.toml` | `[shop]`（`language = "go"`） |
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

## 想定する診断

`hozoc check` の error も warning もない。

## 既定の出力

`testdata/expected/emit/gen/shop/` の、`docs/emit-go.md` の見出しごとの対応。

| `docs/emit-go.md` の見出し | 構文 | 出力 |
|---|---|---|
| 「state」の `StateCases` | `state carts: list<Line>;`、`state orders: list<Order>;`、`state token: nullable<Token>;`（`title` は `single` なので述語なし） | `ops.go` の `StateCases`（`CartsIsEmpty` / `OrdersIsEmpty` / `TokenIsEmpty`。宣言の順）。配線に `match ... by list` / `by nullable` があるので `New(leaves, stateCases, classifiers)` |
| 「classify」の状態の分類 | `classify Checkout of carts, orders { ... }`、`condition LoggedIn of token;` | `classify.go` の `CheckoutCases` / `LoggedInCases`（述語は `(ctx) (bool, error)`）と `ClassifyCheckout(ctx, c)` / `ClassifyLoggedIn(ctx, c)`。値の分類 `LineKind` は今まで通り `ClassifyLineKind(c, v)` |
| 「effect」の遷移のコメント | `effect placeOrder on Checkout: Shopping -> Ordered;` など | `types.go` の定数のコメントに `// on shop.Checkout: Shopping -> Ordered`（`any`、`Shopping \| Ordered`、基底の状態が宛先の `on shop.carts: HasItems -> Empty` も） |
| 「match」の `match s by list` / `by nullable` | `openTop`、`openCart` | `wiring.go`: `empty, err := o.stateCases.<S>IsEmpty(ctx)` と `if empty { ... } else { ... }` |
| 「match」の `match by C` | `checkout`（値の形。`none` のアームがあるので `*Order`）、`add`（文の形。`default` のアーム） | `wiring.go`: `kind, err := Classify<C>(ctx, o.classifiers.<C>)` と `switch` |

出力の細かい形（`none` のアームの枝を書かない、一つのアームに両方のケース、アームの中での `bind`）と、`StateCases` を出すが `New` が取らない場合は、`internal/emit/golang` の golden `testdata/state`（手で組んだ `View`）と単体テストが確かめる。
