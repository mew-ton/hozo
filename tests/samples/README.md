# サンプル

## この文書が決めること

サンプルとは何か、置き方、テストがそれをどう使うか。これらの事実の持ち主はこの文書（`docs/testing.md`「トランスパイラのテスト」がここを指す）。

## サンプルとは

**（Hozo の仕様、完成したコード）の組**を、手で書いたもの。トランスパイラのプロトコルと、生成するコードの形についての問いは、具体的な組がなければ決められない。サンプルはその具体例で、二つの役目を持つ。

- **トランスパイラの仕様**。サンプルの入力に `hozoc emit` を掛けた出力が、サンプルの期待する出力とバイト単位で同じにならなければならない（ブラックボックスのテスト。`docs/testing.md`「トランスパイラのテスト」）。トランスパイラより先に書く。
- **今の設計の点検**。理想のコードを書く途中で、今の設計（基底の形、プロトコル、語彙）が与えてくれなかったものを、サンプルの `README.md`「分かったこと」に残す。

期待するコードは、本当にコンパイルできて動くものにする。動かないコードは、仕様として信用できない。

生成するコードの規則の持ち主は、トランスパイラごとの設計文書（`docs/plugin-go.md` など）。サンプルは、その規則を当てはめた実物で、規則を自分では持たない。文書とサンプルが食い違ったら、どちらが正しいかを決めてから、正しくない方を直す。

## 置き方

```
tests/samples/<名前>/
  README.md      ファイルごとの出どころ、「分かったこと」
  hozo.toml      本物の設定。単位の transpilers には、まだないトランスパイラの名前を書いてもよい
  *.hozo         仕様
  sample.json    トランスパイラごとの、出すファイル（下）
  go.mod, go.sum サンプル自身の Go のモジュール（Go のサンプルのとき）
  <out>/         単位の out。期待する出力（各トランスパイラのファイルと、記録 .hozoc-outputs.json）
  それ以外       人が書くコード（葉の op の本体、準備、main、テスト）
```

- 期待する出力は、`hozo.toml` の単位の `out` のディレクトリそのものに置く。`hozoc emit` が書き出す場所と同じで、置いたままコンパイルできる。
- 期待する出力のファイルは、どれも、どれかのトランスパイラのもの（hozoc 本体はコードを出力しない。`docs/plugin-protocol.md`「この文書が決めること」）。どのトランスパイラのものかは `sample.json` が決める。どれも人が書き、`hozoc emit` の出力で書き直さない。
- サンプルは自分の `go.mod` を持つ、リポジトリのモジュールとは別のモジュールにする。リポジトリの `go build ./...` / `go vet ./...` / `go test ./...` はサンプルの Go を見ない（サンプルだけが使う依存を、リポジトリのモジュールに入れないため）。生成したコードが import する `runtime` は、`replace github.com/mew-ton/hozo => ../../..` で解決する。`gofmt -l .` はモジュールを見ないので、サンプルの Go も整形しておく。

## sample.json

```json
{
  "transpilers": {
    "go":      { "files": ["gen/server/doc.go", "gen/server/ops.go"] },
    "go-http": { "files": ["gen/server/http.go"] }
  }
}
```

- `transpilers` のキーは、`hozo.toml` の単位に書いたトランスパイラの名前。単位に書いた名前はすべてここに要り、ここに書いた名前はどれかの単位が使っていなければならない。
- `files` は、期待する出力のうち、そのトランスパイラが出すファイル（サンプルのルートからの相対パス）。単位の `out` の下のファイルは、出力先ごとの記録を除いて、どれかのトランスパイラの `files` になければならない。
- ほかのキーはない。選ぶ宣言を代役に伝える `select` と、保留にするサブテストを書く `pending` は、なくした（`docs/testing.md`「サンプル」）。

## テスト

`tests/samples_test.go` の `TestSamples` が、`tests/samples/*/sample.json` のあるディレクトリをサンプルとして見つけ（足せば自動で入る）、サンプルごとに次を確かめる。

| サブテスト | 確かめること |
|---|---|
| `check` | サンプルの仕様と設定が、本物の `hozo.toml` と、リポジトリの `plugins/` のプラグインで、`hozoc check` を error なしで通る |
| `files` | hozoc を動かさずに確かめられること: 単位の `out` の下のファイルは、記録を除いてどれも `sample.json` のどれかのトランスパイラの `files` にあり、`files` のファイルはどれもあり、記録 `.hozoc-outputs.json` は、その出力先の下のファイルのパスを辞書順に並べたものと同じ |
| `go` | **サンプルが動く**: サンプルのモジュールで `go vet ./...` と `go test ./...` が通る（期待する出力がコンパイルでき、人が書いたコードと合わせて仕様の通りに動く） |
| `transpilers/<名前>` | **トランスパイラごとのブラックボックスのテスト**: `hozoc emit` の出力のうち、`sample.json` のそのトランスパイラの `files` が、期待する出力とバイト単位で同じ（足りないファイル、内容の違いを、ファイルごとに示す） |
| `transpilers/all` | `hozoc emit` の出力の全体が、期待する出力と同じ: どのトランスパイラの `files` にもないファイルが書かれていない。記録が同じ |

- `check` と `transpilers/*` は、サンプルの入力（`hozo.toml`、`.hozo`、`go.mod`）を一時ディレクトリに写して走らせる。プラグインは、リポジトリの `plugins/` を `HOZO_PLUGIN_PATH` として hozoc に渡す（`docs/plugin-protocol.md`「探索」の 2）。`hozoc emit` は一度だけ走らせ、出力を `transpilers/*` が分けて比べる。
- サンプルのトランスパイラのうち、`plugins/` のどのプラグインも提供していないものがあれば、`check` と `transpilers/*` を、足りない名前を示して飛ばす。提供するプラグインを足せば、何も変えずに走る。`files` と `go` は飛ばさない。
- 期待する出力は仕様なので、`-update` では書き直さない。
- `go` は、サンプルの `go.sum` の通りの依存をモジュールキャッシュから使う（`GOFLAGS=-mod=readonly`、`GOTOOLCHAIN=local`、`GOWORK=off`）。キャッシュにあればネットワークは要らない。なければ `go` が `GOPROXY` から取りに行き、取れなければ（オフラインなど）失敗する。飛ばさないのは、動くと確かめていないサンプルを仕様として通さないため。先に取っておくには、サンプルのディレクトリで `GOTOOLCHAIN=local go mod download`。
- サンプルだけを手で確かめるには、サンプルのディレクトリで `GOTOOLCHAIN=local go vet ./... && GOTOOLCHAIN=local go test ./...`。

## 移行の間

この文書は、トランスパイラを作る段階（`docs/requirements.md`「作り直しの段階」の R2）の後の形を書いている。その段階の実装が済むまで、次は前の形のまま。

- `tests/samples_test.go` は、前のサブテスト（`check` / `shared` / `go` / `transpilers`）と、`sample.json` の `pending` を読む。
- サンプル `shop-server` の `sample.json` は `pending` を持ち、`gen/` は、文書の形と少し違う（`shop-server/README.md`「文書の形との違い」）。

## サンプルの一覧

| サンプル | 中身 |
|---|---|
| [`shop-server`](shop-server/README.md) | 共有の型 `type`、保存 `store`、サーバー `server`、外部の配送 `mailer` の四つの名前空間を、基底 `go`（`docs/plugin-go.md`）と、それに相乗りする `go-valid`（`docs/plugin-go-valid.md`）、`go-sqlite`（`docs/plugin-go-sqlite.md`。`database/sql` の上の表と、`store` の op の中身）、`go-http`（`docs/plugin-go-http.md`。`net/http` のハンドラ）が Go に出力する。同じ `server` を、SQLite の上とメモリの上に組み立てる |
