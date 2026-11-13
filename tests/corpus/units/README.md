# コーパス `units`

`hozo.toml` の出力の単位（`docs/config.md`「単位」「単位の対象」「in」、`docs/decisions.md`「設計書を変える決定」S14）を、小さな一つのプロジェクトで一か所ずつ使う（`docs/testing.md`「コーパス」）。同じ名前空間の二つの単位、`in` で絞った単位、子孫を名指しする単位、単位をまたぐ一方向の import を持つ。

| ファイル | 中身 |
|---|---|
| `hozo.toml` | 単位が四つ（下の表） |
| `t.hozo` | テストの語彙 `namespace t`。トランスパイラごとの起点の印 `stock` / `api` / `batch` / `admin`。どの単位の対象でもない |
| `stock.hozo` | `namespace stock`。`shop` の出力がその型を import する相手 |
| `shop/api.hozo` | `namespace shop` の前半。単位 2 の `in` に合うただ一つのファイル |
| `shop/batch.hozo` | `namespace shop` の後半と、子の名前空間 `shop.admin` |
| `plugins/pick/` | テスト用のプラグイン（`tests/plugins/pick`）のマニフェストと `pick.json`。一つのプラグインが四つのトランスパイラを提供し、`pick.json` の `transpilers` がトランスパイラごとに読むデコレータを決める。ファイルは出さない |
| `testdata/expected/check.txt` | `hozoc check` の text 出力のゴールデン |
| `testdata/expected/emit/` | `hozoc emit` が書き出すファイルの木のゴールデン（`gen/stock`、`gen/api`、`gen/batch`、`gen/admin`）。`TestCorpusEmitGolden` が、書き出した Go を `go vet` にも通す |

ゴールデンの更新は `go test ./tests -update`。

## 単位

| # | `in` | `namespace` | `transpilers` | `out` | 起点（`pick.json`） |
|---|---|---|---|---|---|
| 1 | — | `stock` | `pick-stock` | `gen/stock` | `@t.stock` の `reserve` |
| 2 | `shop/api.hozo` | `shop` | `pick-api` | `gen/api` | `@t.api` の `open`（`@t.batch` も読むが、`in` の外の op は選べない） |
| 3 | — | `shop` | `pick-batch` | `gen/batch` | `@t.batch` の `nightly` と `report` |
| 4 | — | `shop.admin` | `pick-admin` | `gen/admin` | `@t.admin` の `ban` |

## 見出し → 構文

`tests/units_test.go` の `TestUnitsCorpus` のサブテストは、この表の見出しの名前で期待値を明示する。

| 文書の見出し | 構文（ファイル） | 確かめること |
|---|---|---|
| `docs/config.md`「単位」 | 単位 2 と 3（どちらも `namespace = "shop"`） | 同じ名前空間を、別のトランスパイラで別の `out` に、互いに独立に出力する。`gen/api` は `open` から、`gen/batch` は `nightly` と `report` から辿った宣言を持ち、どちらも自分の `Item` を持つ。記録（`.hozoc-outputs.json`）は出力ごと |
| `docs/config.md`「in」、`docs/emit-go.md`「出力の対象」2 | 単位 2 の `in = ["shop/api.hozo"]` と、`open` が呼ぶ `nightly`（`shop/batch.hozo` の配線） | `gen/api` では `nightly` は `Leaves` の口だけで、本体も、本体が呼ぶ `sweep` も出ない。`gen/batch` では同じ `nightly` の本体が出る。`pick-api` は `@t.batch` も読むが、`plugin.OpsWith` は単位の対象の op だけを返すので、`nightly` と `report` は起点にならない |
| `docs/emit-go.md`「出力の対象」（`in` の外の型） | `nightly(item: Item)`（`Item` は `shop/api.hozo` の宣言） | 選ばれた宣言が参照する型は、宣言されたファイルによらず出力する |
| `docs/config.md`「単位の対象」 | 単位 4（`namespace = "shop.admin"`） | `shop.admin` は単位 4 だけが出力する（`gen/admin`）。`gen/api/admin` も `gen/batch/admin` もできない |
| `docs/emit-go.md`「配置」 | `model Item { id: stock.ItemId; ... }`、`op ban(item: shop.Item)` | `gen/api` と `gen/batch` は `gen/stock` の `ItemId` を import する（単位をまたぐ一方向の import）。`gen/admin` は、`shop` を出力する単位のうち設定の順で最初に `Item` を持つ単位 2 の `gen/api` を import する。`gen/stock` は誰も import しない |
| `docs/cli.md`「emit」の 1 | `hozoc emit shop`、`hozoc emit t` | 引数は名前空間。`shop` とその子孫を出力する単位 2・3・4 の分を書き出し、内容はすべてを対象にしたときと同じ。どの単位の対象でもない `t` は `UnknownNamespace` |
| `docs/plugin-protocol.md`「選択」（PLG-3） | `pick.json` の `fqnRoots` で、単位 2 に `shop.report`（`in` の外）を選ばせる | `proc.MalformedOutput`（70）。`in` を省いた単位 3 なら同じ FQN を選べる |

単位の規則のうち設定だけで決まるもの（CFG-12、CFG-14〜CFG-18）は `tests/rules/` が、出力どうしの import の循環（GOEMIT-5）は `internal/app` の `TestEmitImportCycle` が確かめる。

## 想定する診断

`hozoc check` の error も warning もない。
