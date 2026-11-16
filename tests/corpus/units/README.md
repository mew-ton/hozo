# コーパス `units`

`hozo.toml` の出力の単位（`docs/config.md`「単位」「単位の対象」「in」、`docs/decisions.md`「設計書を変える決定」S14）を、小さな一つのプロジェクトで一か所ずつ使う（`docs/testing.md`「コーパス」）。同じ名前空間の二つの単位、`in` で絞った単位、子孫を名指しする単位、単位をまたぐ一方向の import を持つ。

| ファイル | 中身 |
|---|---|
| `hozo.toml` | 単位が四つ（下の表） |
| `t.hozo` | テストの語彙 `namespace t`。印のデコレータ `stock` / `api` / `batch` / `admin`（どのトランスパイラも読まないので、何も出ない。付いた op は純粋でなくなる）。どの単位の対象でもない |
| `stock.hozo` | `namespace stock`。`shop` の出力がその型を import する相手 |
| `shop/api.hozo` | `namespace shop` の前半。単位 2 の `in` に合うただ一つのファイル |
| `shop/batch.hozo` | `namespace shop` の後半と、子の名前空間 `shop.admin` |
| `testdata/expected/check.txt` | `hozoc check` の text 出力のゴールデン |
| `testdata/expected/emit/` | `hozoc emit` が書き出すファイルの木のゴールデン（`gen/stock`、`gen/api`、`gen/batch`、`gen/admin`）。`TestCorpusEmitGolden` が、書き出した Go を `go vet` にも通す |

ゴールデンの更新は `go test ./tests -update`。

## 単位

四つの単位とも、トランスパイラは Go の基底 `go`（リポジトリの `plugins/go`。テストが `HOZO_PLUGIN_PATH` として渡す）。

| # | `in` | `namespace` | `transpilers` | `out` | 出るもの |
|---|---|---|---|---|---|
| 1 | — | `stock` | `go` | `gen/stock` | `stock` のすべて。op は `export` の `reserve` だけなので、内側のフォルダはない |
| 2 | `shop/api.hozo` | `shop` | `go` | `gen/api` | `shop` の型と状態のすべてと、`shop/api.hozo` の op。`in` の外の op は、呼ばれている `nightly` の口だけ |
| 3 | — | `shop` | `go` | `gen/batch` | `shop` のすべて |
| 4 | — | `shop.admin` | `go` | `gen/admin` | `shop.admin` のすべて |

## 見出し → 構文

`tests/units_test.go` の `TestUnitsCorpus` のサブテストは、この表の見出しの名前で期待値を明示する。

| 文書の見出し | 構文（ファイル） | 確かめること |
|---|---|---|
| `docs/config.md`「単位」 | 単位 2 と 3（どちらも `namespace = "shop"`） | 同じ名前空間を、別の `out` に、互いに独立に出力する。どちらも自分の型（`Item`、`Receipt`、`Report`）をすべて持ち、相手のパッケージは import しない。記録（`.hozoc-outputs.json`）は出力ごと |
| `docs/config.md`「in」、`docs/plugin-go.md`「出力の対象」 | 単位 2 の `in = ["shop/api.hozo"]` と、`open` が呼ぶ `nightly`（`shop/batch.hozo` の配線） | `in` が絞るのは op だけ。`gen/api` では、型（`Report` は `shop/batch.hozo` の宣言）と状態はすべて出て、`nightly` は `Leaves` の口だけで、本体も、本体が呼ぶ `sweep` も、呼ばれていない `report` も出ない。`gen/batch` では同じ `nightly` の本体と、`sweep`、`report` が出る |
| `docs/config.md`「単位の対象」 | 単位 4（`namespace = "shop.admin"`） | `shop.admin` は単位 4 だけが出力する（`gen/admin`）。`gen/api/admin` も `gen/batch/admin` もできない |
| `docs/config.md`「型の置き場」 | `model Item { id: stock.ItemId; ... }`、`op ban(item: shop.Item)` | `gen/api` と `gen/batch` は `gen/stock` の `ItemId` を import する（単位をまたぐ一方向の import。複製しない）。`gen/admin` は、`shop` を出力する単位のうち設定の順で最初の単位 2 の、入口のパッケージ `gen/api` を import する（内側は import しない）。`gen/stock` は誰も import しない |
| `docs/plugin-go.md`「内側のフォルダと入口」 | `stock`（op は `export` の `reserve` だけ）と `shop`（op はどれも内部） | `gen/stock` は一つのパッケージで、`inner/` を持たない。`gen/api` は定義が `inner/` にあり、入口の `Ops` はメソッドを持たない |
| `docs/cli.md`「emit」の 1 | `hozoc emit shop`、`hozoc emit t` | 引数は名前空間。`shop` とその子孫を出力する単位 2・3・4 の分を書き出し、内容はすべてを対象にしたときと同じ。どの単位の対象でもない `t` は `UnknownNamespace` |

単位の規則のうち設定だけで決まるもの（CFG-12、CFG-14〜CFG-23）は `tests/rules/` が、出力どうしの import の循環（GO-3）は `plugins/go` の単体テストが確かめる。

## 想定する診断

`hozoc check` の error も warning もない。
