# コーパス `model`

「モデルが先」の決定（`docs/decisions.md`「設計書を変える決定」S10〜S12、`docs/requirements.md`「モデルが先」）の構文を、小さな一つのプロジェクトで一か所ずつ使う（`docs/testing.md`「コーパス」）。コードを生成する名前空間は一つ（`shop`）だけで、残りは設計のためだけにある: 印の語彙（`marks`）、人の操作（`customer`）、外部のシステム（`carrier`）。`hozoc check` が error も warning もなしで通ること、IR（`docs/ir.md`）に載る形、検査と計算（`docs/check.md`）が出力しない名前空間にも同じに掛かることを確かめる。

| ファイル | 中身 |
|---|---|
| `hozo.toml` | `[shop]`（`language = "go"`）だけ。`marks`、`customer`、`carrier` のテーブルはない |
| `marks.hozo` | `namespace marks`。どのプラグインも読まない印のデコレータ: `context`（`for namespace`）、`manual`（`for namespace \| op`）、`requirement`（op。`throws NotApproved prevents effects` を宣言する） |
| `shop.hozo` | `@marks.context("販売") namespace shop`。出力する名前空間。`@marks.requirement` の付いた葉と、`carrier` の op を呼ぶ配線 |
| `carrier.hozo` | `@marks.context("配送") namespace carrier`。外部のシステム。状態・効果・例外・葉の op を持つが、出力しない |
| `customer.hozo` | `@marks.context("利用者") namespace customer` と、二つ目のブロック `@marks.manual namespace customer`。人の操作。`shop` の op を呼ぶ配線を持つが、出力しない |
| `testdata/expected/check.txt` | `hozoc check` の text 出力のゴールデン（`tests/corpus_emit_test.go` の `TestCorpusCheckGolden`）。更新は `go test ./tests -update` |

`.hozo` の各ブロックの上の `// docs/...「見出し」` のコメントが、その構文が確かめる文書の見出し。

`emit` のゴールデンは持たない（`tests/corpus_emit_test.go` の `corpora` の `noOutput`）: 出力する名前空間 `shop` にプラグインがなく、出力するのはプラグインが選んだ宣言だけ（`docs/emit-go.md`「出力の対象」、`docs/decisions.md` S11）なので、`hozoc emit` は終了コード 0 で何も書かない。印のデコレータ（`@marks.*`）も何も生成しない（`docs/emit-go.md`「デコレータ」）。前回の出力が残っていれば消えること（`docs/cli.md`「emit」の 6 と 8）は、`TestModelCorpus` の「emit-go.md 出力の対象」のサブテストが確かめる。プラグインが選んだときの出力の形（印のデコレータ、出力しない名前空間の op を呼ぶ配線）は、コーパス `select` と `view` が確かめる。

## 見出し → 構文

`tests/model_test.go` の `TestModelCorpus` のサブテストは、この表の見出しの名前で期待値を明示する（`internal/project` の `Load` → `Sema.Program` と `Views()`）。

| 文書の見出し | 構文（ファイル） | 確かめること |
|---|---|---|
| `docs/grammar.md`「ファイルと宣言」の `Decl = [ DocComment ] { Decorator } NamespaceDecl` | `/** ... */ @marks.context("販売") namespace shop { }`（`shop.hozo`）、`@marks.context("配送") namespace carrier { }`（`carrier.hozo`） | DocComment → デコレータ → `namespace` の順で書ける。適用の名前は、ブロックが書かれた場所（ここではグローバル）で解決する（`docs/resolve.md`「スコープ」） |
| `docs/ir.md`「Program」の `Namespace.decorators` | 同上と、`customer.hozo` の二つのブロック（`@marks.context("利用者")`、`@marks.manual`） | `Application` の列（引数は `param` に対応付け済み、位置は適用そのもの）。同じ名前空間の複数のブロックの分は出現順に連結して一つの `Namespace` に。付けていない名前空間（`marks`）は `[]`（省かない） |
| `docs/grammar.md`「decorator の宣言」の `Target`、`docs/ir.md`「decorator」の `targets` | `decorator context(name: string) for namespace;`、`decorator manual for namespace \| op;`、`decorator requirement(id: string) throws ...;`（`marks.hozo`） | `targets` は `["namespace"]`、`["namespace", "op"]`、省略なら `["op"]`。`for` にない対象に付ければ DEC-1（`tests/rules/DEC-1` の `ng_2` / `ok_2`） |
| `docs/config.md`「出力する名前空間」 | `hozo.toml` の `[shop]` だけ。`carrier` と `customer` は `state` と `op` を持つ | テーブルのない名前空間に診断は出ない（CFG-1 は「テーブルのある名前空間」の規則）。`Views()` は `shop` だけで、`View.namespaces` にも `shop` だけ。`View.program` は全体そのまま、`View.ops` は出力しない名前空間の op も `Sig(O, shop)` で持つ |
| `docs/requirements.md`「モデルが先」の「決まったこと」（人の操作の書き方） | 担い手ごとの名前空間 `namespace customer`（`@marks.manual` を名前空間に付ける）と、op に印を付ける形 `@marks.manual op pick()`（`customer.hozo`） | どちらの形でも書ける（言語は決めない）。同じ印を名前空間にも op にも付けられる |
| `docs/check.md`「計算」手順 i の `throws`（印のデコレータ） | `@marks.requirement("PRD-12") op placeOrder(): Order effects createOrder;`（`shop.hozo`） | どのプラグインも読まない印でも、`throws` を宣言していれば付けた op の `Signature` に効く: `exceptions` に `marks.NotApproved`、`relations` は `prevented`。op 自身の `throws` は `[]` のまま。`throws` を持たない印（`@marks.manual`）は `Signature` を変えない |
| `docs/check.md`「計算」手順 0（配線）: 名前空間をまたぐ伝播 | `shop.fulfill`（設定のある名前空間 → 設定のない `carrier.pickup`）、`customer.buy`（設定のない名前空間 → 設定のある `shop.addToCart` / `shop.fulfill`） | 効果と例外は名前空間をまたいで伝播し、`rel` と `succ` も同じ規則で計算される。呼び先の例外を `throws` に書かなければ、出力しない名前空間の配線でも THROWS-1 |
| `docs/check.md`「状態と効果」STATE-2、EFF-3 | `carrier.pickup`（`effects accept`）、`customer.pick`（`effects forget`） | 出力しない名前空間にも状態の持ち主の規則が掛かる（効果は自分の名前空間の状態への効果）。葉の op の効果は一つ |
| `docs/emit-go.md`「出力の対象」（プラグインのない名前空間） | `hozo.toml` の `[shop]`（`plugins` なし）と、前回の出力に見立てた `gen/shop/ops.go` と `.hozoc-outputs.json` | `hozoc emit` は終了コード 0 で何も書かない。前回の出力は古いファイルとして消え（`removed: gen/shop/ops.go`）、`.hozoc-outputs.json` も残らない。記録にないファイルには触らない |
| `docs/config.md`「置き場と探し方」の「設定なし」 | 同じ `.hozo` を `hozo.toml` なしのディレクトリに置く（`TestModelCorpus` の最後のサブテスト） | `hozoc check` は終了コード 0 で `0 errors, 0 warnings`。`Views()` は空。`hozoc emit` は CFG-7（終了コード 2） |

## 想定する診断

`hozoc check` の error も warning もない。
