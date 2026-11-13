# コーパス `select`

既定の出力が、プラグインの選んだ宣言だけを出すこと（`docs/emit-go.md`「出力の対象」「デコレータ」、`docs/plugin-protocol.md`「選択」、`docs/decisions.md` 28 と S11）を、小さな一つのプロジェクトで確かめる（`docs/testing.md`「コーパス」）。起点から辿られる op と辿られない op、`stops` の op、`wraps` で引き受けたデコレータと印のままのデコレータ、`states` で選ぶ状態、別の名前空間の op、プラグインのない名前空間を一か所ずつ使う。

| ファイル | 中身 |
|---|---|
| `hozo.toml` | `[shop]`（`language = "go"`、`plugins = ["pick"]`）と `[stock]`（`language = "go"`。プラグインなし） |
| `t.hozo` | テストの語彙 `namespace t`。`pick` が読むデコレータ `entry` / `stop` / `cache` / `store` と、どのプラグインも読まない印 `note` |
| `shop.hozo` | `namespace shop`。出力する名前空間 |
| `stock.hozo` | `namespace stock`。テーブルはあるがプラグインがないので、何も出力しない。`shop` の配線から呼ばれる |
| `plugins/pick/` | テスト用のプラグイン（`tests/plugins/pick`）のマニフェストと `pick.json`。`emit` の応答の `select` で、`roots` に `@t.entry` の op、`stops` に `@t.stop` の op、`states` に `@t.store` の状態、`wraps` に `@t.cache` の適用を返す。ファイルは出さない |
| `testdata/expected/check.txt` | `hozoc check` の text 出力のゴールデン（`tests/corpus_emit_test.go` の `TestCorpusCheckGolden`） |
| `testdata/expected/emit/` | `hozoc emit` が書き出すファイルの木のゴールデン（同 `TestCorpusEmitGolden`。書き出した Go は一時のモジュールで `go vet` も通す）。`gen/shop` だけで、`gen/stock` はない |

ゴールデンの更新は `go test ./tests -update`。`.hozo` の各ブロックの上の `// docs/...「見出し」` のコメントが、その構文が確かめる文書の見出し。

## 見出し → 構文

`tests/select_test.go` の `TestSelectCorpus` のサブテストは、実物の `hozoc emit` の出力を、この表の見出しと手順の番号の名前で確かめる。

| `docs/emit-go.md` の見出し | 構文 | 出力（`gen/shop`） |
|---|---|---|
| 「出力の対象」1〜2（起点と、op を辿る） | `@t.entry op checkout() { placeOrder(); stock.reserve(order); notify(order); issue(order); }`、`@t.entry op openCart() { match carts by list { ... } }` | `Ops` に載るのは `checkout`、`openCart` と、辿られた `placeOrder`、`notify`、`issue`、`showEmpty`、`showCart`。どこからも辿られない `saveForLater` と `addToCart`（`@t.note` が付いていても、印では選ばれない）は出力しない。エラーでも warning でもない |
| 同 2（`stops`） | `@t.stop op notify(order: Order) { sendMail(order); logSent(); }` | `notify` は配線だが、本体を出さず `Leaves.Notify` の口だけを出す。本体が呼ぶ `sendMail` と `logSent` は選ばれない |
| 「デコレータ」（`wraps` と印） | `@t.cache @t.note("領収書") op issue(order: Order): Receipt;` | `@t.cache` は `pick` が `wraps` で引き受けたので、`IssueTCacheWrapper`、`TCacheWrappers`、`Wrappers`、`New(leaves, deps, wrappers)`。`@t.note` はどのプラグインも読まない印で、何も生成しない |
| 「出力の対象」3（状態）と「state」 | `state carts`（`openCart` の `match`）、`state orders`（葉の `placeOrder` の効果 `createOrder` の宛先）、`@t.store state audits`（`states`）、`state drafts`（選ばれない） | `states.go` の `CartsState`、`OrdersState`（命令 `PlaceOrder`）、`AuditsState`。`DraftsState` はない。`placeOrder` は `Leaves` ではなく `States.Orders.PlaceOrder` |
| 同 5（別の名前空間の op） | `stock.reserve(order);`（`stock.reserve` は `count` と `keep` を呼ぶ配線） | `Deps.Stock.Reserve`（`StockReserveFunc`）。`reserve` の本体は辿らない |
| 同 6（型と効果の定数） | `model Line` / `Order` / `Receipt` / `Audit`、選ばれない `model Draft`。`effect createOrder`、選ばれない op だけが持つ `addLine` / `saveDraft` | `types.go` に `Line`、`Order`、`Receipt`、`Audit` と `EffectCreateOrder`。`Draft`、`EffectAddLine`、`EffectSaveDraft` はない |
| 「配置」（import と複製） | `model Line { item: stock.ItemId; ... }`、`throws stock.Short` | `stock` は Go の名前空間だが、その出力が型を持たない（何も出力しない）ので、`ItemId` と `Short` を `shop` のパッケージに複製する |
| 「出力の対象」（プラグインのない名前空間） | `hozo.toml` の `[stock]`（`plugins` なし） | `gen/stock` はできない。`.hozoc-outputs.json` も置かない |
| 「実装の単位」 | すべて | 生成したコードは `context` を引数に取らない |

選択を変えれば出力も変わる（`pick.json` から `stops` を外すと、`notify` の本体を出し、`sendMail` と `logSent` まで辿る）ことも `TestSelectCorpus` が確かめる。生成したコードを実際に動かす振る舞い（入れていないフィールドが `*runtime.MissingError` になる、`States` を通る配線）は、`internal/emit/golang` の `TestBehavior` が確かめる。

## 想定する診断

`hozoc check` の error も warning もない。
