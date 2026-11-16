# コーパス `model`

「モデルが先」の決定（`docs/decisions.md`「設計書を変える決定」S10〜S12、`docs/requirements.md`「モデルが先」）の構文を、小さな一つのプロジェクトで一か所ずつ使う（`docs/testing.md`「コーパス」）。コードを生成したい名前空間は一つ（`shop`）だけで、残りは設計のためだけにある: 印の語彙（`marks`）、人の操作（`customer`）、外部のシステム（`carrier`）。`hozoc check` が error も warning もなしで通ること、IR（`docs/ir.md`）に載る形、検査と計算（`docs/check.md`）が出力しない名前空間にも同じに掛かることを確かめる。

| ファイル | 中身 |
|---|---|
| `hozo.toml` | 単位が三つ: 出力したい `namespace = "shop"` と、`shop` が使う型（例外 `carrier.Closed`、`marks.NotApproved`）の置き場の `"carrier"`、`"marks"`（`docs/config.md`「型の置き場」。なければ CFG-22）。どれも `transpilers = ["none"]`。`customer` はどの単位の対象でもない |
| `plugins/none/` | テスト用のプラグインのマニフェスト。トランスパイラ `none`（`requires` を持たない、テストのための基底。sh で動く）は、ファイルを返さない |
| `marks.hozo` | `namespace marks`。どのプラグインも読まない印のデコレータ: `context`（`for model \| state \| op`）、`manual`（`for op`）、`requirement`（op。`throws NotApproved prevents effects` を宣言する） |
| `shop.hozo` | `namespace shop`。出力する名前空間。`@marks.context("販売")` の付いた model と状態、効果を呼ぶ配線（`addToCart`、`@marks.requirement` の付いた `placeOrder`）と、その葉（`checkStock`、`newOrder`）、`carrier` の op を呼ぶ配線 |
| `carrier.hozo` | `namespace carrier`。外部のシステム。状態・効果・例外と、効果を呼ぶ配線（`pickup`）、その葉（`checkOpen`）を持つ。単位は、`shop` が使う例外の置き場としてだけ持つ。状態と op に `@marks.context("配送")` |
| `customer.hozo` | `namespace customer` のブロックが二つ。人の操作。`@marks.manual` の付いた op（効果を呼ぶ配線 `pick`、葉の `choose` と `leave`）と、`shop` の op を呼ぶ配線を持つが、出力しない |
| `testdata/expected/check.txt` | `hozoc check` の text 出力のゴールデン（`tests/corpus_emit_test.go` の `TestCorpusCheckGolden`）。更新は `go test ./tests -update` |

`.hozo` の各ブロックの上の `// docs/...「見出し」` のコメントが、その構文が確かめる文書の見出し。

`emit` のゴールデンは持たない（`tests/corpus_emit_test.go` の `corpora` の `noOutput`）: 単位のトランスパイラ `none` はファイルを返さず、hozoc 本体はコードを出力しない（`docs/plugin-protocol.md`「この文書が決めること」）ので、`hozoc emit` は終了コード 0 で何も書かない。印のデコレータ（`@marks.*`）も何も生成しない。前回の出力が残っていれば消えること（`docs/cli.md`「emit」の 5 と 7）は、`TestModelCorpus` の「cli.md emit」のサブテストが確かめる。印のデコレータと、別の名前空間の op を呼ぶ配線の、Go の出力の形は、コーパス `design` と `units` が通る。

## 見出し → 構文

`tests/model_test.go` の `TestModelCorpus` のサブテストは、この表の見出しの名前で期待値を明示する（`internal/project` の `Load` → `Sema.Program` と `Views()`）。

| 文書の見出し | 構文（ファイル） | 確かめること |
|---|---|---|
| `docs/grammar.md`「ファイルと宣言」（デコレータは名前空間に付けられない）、`docs/ir.md`「デコレータの適用」 | `@marks.context("販売") model Order { }`、`@marks.context("販売") state orders: ...;`（`shop.hozo`）、`@marks.context("配送") state parcels: ...;`、`@marks.context("配送") op pickup(...)`（`carrier.hozo`）、`customer.hozo` の二つの `namespace customer` のブロック | 印は名前空間ではなく宣言（model / state / op）に付ける。`DeclBase.decorators` は `Application` の列（引数は `param` に対応付け済み、位置は適用そのもの）。同じ名前の `namespace` のブロックは一つの `Namespace` にまとまり、`Namespace` は `decorators` を持たない。`namespace` の前に書けば構文エラー（`tests/rules/SYN-1` の `ng_1` / `ng_4`） |
| `docs/grammar.md`「decorator の宣言」の `Target`、`docs/ir.md`「decorator」の `targets` | `decorator context(name: string) for model \| state \| op;`、`decorator manual for op;`、`decorator requirement(id: string) throws ...;`（`marks.hozo`） | `targets` は `["model", "state", "op"]`（書いた順）、`["op"]`、省略なら `["op"]`。`for` にない対象に付ければ DEC-1（`tests/rules/DEC-1`） |
| `docs/config.md`「単位の対象」「型の置き場」 | `hozo.toml` の単位は `shop` と、型の置き場の `carrier`、`marks`。`customer` は `state` と `op` を持つが、どの単位の対象でもない | どの単位の対象でもない名前空間に診断は出ない。`Views()` は `shop`、`carrier`、`marks` で、`View.namespaces` にも `customer` は入らない。`View.program` は全体そのまま、`View.ops` は出力しない名前空間の op も `Sig(O, shop)` で持つ |
| `docs/requirements.md`「モデルが先」の「決まったこと」（人の操作の書き方） | 担い手ごとの名前空間 `namespace customer` と、op に印を付ける形 `@marks.manual op pick()`、`@marks.manual op leave()`（`customer.hozo`） | どちらの形でも書ける（言語は決めない）。名前空間そのものには印を付けられないので、担い手が人であることは名前空間の説明と、op ごとの印で表す |
| `docs/check.md`「計算」手順 i の `throws`（印のデコレータ） | `@marks.requirement("PRD-12") op placeOrder(): Order effects createOrder { … }`（`shop.hozo`） | どのプラグインも読まない印でも、`throws` を宣言していれば付けた op の `Signature` に効く: `exceptions` に `marks.NotApproved`、`relations` は `prevented`。op 自身の `throws` は `[]` のまま。`throws` を持たない印（`@marks.manual`）は `Signature` を変えない |
| `docs/check.md`「計算」手順 0（配線）: 名前空間をまたぐ伝播 | `shop.fulfill`（`shop` → 別の名前空間の `carrier.pickup`）、`customer.buy`（対象でない名前空間 → 対象の `shop.addToCart` / `shop.fulfill`） | 効果と例外は名前空間をまたいで伝播し、`rel` と `succ` も同じ規則で計算される。呼び先の例外を `throws` に書かなければ、出力しない名前空間の配線でも THROWS-1 |
| `docs/check.md`「状態に触れるもの」の「配線からの呼び出し」「葉の op」 | `carrier.pickup`（`accept(order);`）、`customer.pick`（`forget();`） | 状態に触れるのは、効果を呼ぶ配線だけで、葉（`checkOpen`、`choose`）は節を書かない。出力しない名前空間（`customer`）にも同じ規則が掛かる: 呼べる効果は、自分の名前空間のものだけ（NS-6） |
| `docs/cli.md`「emit」（どのトランスパイラもファイルを返さない） | `hozo.toml` の単位（トランスパイラ `none` はファイルを返さない）と、前回の出力に見立てた `gen/shop/ops.go` と `.hozoc-outputs.json` | `hozoc emit` は終了コード 0 で何も書かない。前回の出力は古いファイルとして消え（`removed: gen/shop/ops.go`）、`.hozoc-outputs.json` も残らない。記録にないファイルには触らない |
| `docs/config.md`「置き場と探し方」の「設定なし」 | 同じ `.hozo` を `hozo.toml` なしのディレクトリに置く（`TestModelCorpus` の最後のサブテスト） | `hozoc check` は終了コード 0 で `0 errors, 0 warnings`。`Views()` は空。`hozoc emit` は CFG-7（終了コード 2） |

## 想定する診断

`hozoc check` の error も warning もない。
