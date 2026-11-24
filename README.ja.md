# Hozo

[English](README.md) | 日本語

**Hozo は、アプリの設計の、ただ一つの正（SSOT）になる、仕様の言語。**

コードの大半を、AI が書くようになっても、人が持ち続けるのは、設計: アプリが、何を、どの順にして、何を、決してしないか。Hozo は、その設計を、人が、レビューして、手直しできる形で持ち、レビューした設計を、そのまま、コードの枠と、テストのハーネスにする。人が責任を持つのは、仕様。中身は、AI に任せられる。

## 書くものと、得られるもの

書くのは、仕様。サンプル [`tests/samples/shop-server/server.hozo`](tests/samples/shop-server/server.hozo) からの、省いた抜き書き:

```
/** 注文する。注文と、その通知の記録は、一緒に残す。通知の配送に失敗しても、どちらも残る */
@http.post("/orders")
export fn placeOrder(in: PlaceOrderInput): type.Order {
  val session = authenticate(in.token);
  val order = newOrder(session, in.lines);
  val notice = composeNotice(order);
  store.placeOrder(order, notice);
  mailer.send(notice);
  return order;
}

/** 識別子と時刻を決めて、注文を作る（人が書く） */
fn newOrder(session: type.Session, lines: type.OrderLine[]): type.Order;
```

`hozoc build` が、コードを書き出す。そのサンプルが期待する Go（[`tests/samples/shop-server/gen/server`](tests/samples/shop-server/gen/server)）からの、省いた抜き書き:

```go
// inner/wiring.go: 中身のある関数は、配線。コードは、生成される。
func (o *ops) PlaceOrder(ctx context.Context, in PlaceOrderInput) (pkgtype.Order, error) {
	session, err := o.Authenticate(ctx, in.Token)
	// …
	order, err := o.NewOrder(ctx, session, in.Lines)
	// …
}

// inner/ops.go: 中身のない関数は、人か AI が書く。
// メソッドが一つでも足りない値は、渡すところで、コンパイルできない。
type Leaves interface {
	NewOrder(ctx context.Context, session pkgtype.Session, lines []pkgtype.OrderLine) (pkgtype.Order, error)
	// …
}

// servertest/leaves.go: その関数の、一つ一つに要る、テストのケース。
// ケースが一つでも足りない値も、コンパイルできない。
type LeafCases interface {
	NewOrder() NewOrderCases
	// …
}

// http.go: デコレータ @http.post は、プラグインが解釈する。
func NewHandler(ops Ops) http.Handler
```

Go の抜き書きの、コメントは、ここで、書き足したもの。

## 誰が、何を担保するか

| 何を | 誰が確かめるか |
|---|---|
| 呼ぶ順と、何を、何に渡すか | Hozo: コードを、仕様から生成する |
| 中身のない関数の、どれにも、実装があること | 出力先の言語の、コンパイラ（抜き書きの `Leaves`） |
| 仕様が求めるテストのケースが、すべて、書かれていること | 出力先の言語の、コンパイラ（抜き書きの `LeafCases`） |
| 求めるテストが、走ったこと | `hozoc tests --coverage` |
| テストが、通ったこと | 利用者の、テストの実行 |
| 関数の中身と、テストのケースの中身 | それを書く、人か AI |

Hozo が、わざと、持たないものが、二つある。

- **仕様に、条件を書かない。** どの場合に、何をするかは、言う。どうやって、その場合だと分かるかは、言わない。それは、中身。
- **プラグインは、出力を推論しない。** 書き方や型から、出力を決めない。何を生成するかは、宣言と、デコレータで決まる。

理由は [`docs/requirements.md`](docs/requirements.md)「価値と位置づけ」「出力を推論しない」。

## 状態

Hozo は、未公開で、版は 0.x。言語も、IR も、プラグインとのやり取りも、互換を保たずに、変わる。

| | |
|---|---|
| 今、動くもの | `hozoc build`（検査して、コードを書き出す）、`hozoc tests`（求めるテストが走ったかを確かめる）、`hozoc lsp`（言語サーバー）、VS Code 拡張 |
| 出力先の言語 | Go だけ: 型と関数、値の制約の確かめ、`net/http` のハンドラ、SQLite、コマンドライン |
| プラグイン | 生成するコードは、すべて、プラグインが出す。プラグインは、WebAssembly のモジュールで、`hozoc` の中の、サンドボックスで動く。ネットワークにも、ファイルにも、触れない |
| まだ、ないもの | Go のほかの言語への出力。仕様をレビューして、手直しする UI。ライセンス |

## 試す

Go 1.24.7 以上が要る。

```bash
make build
```

```bash
bin/hozoc build --no-emit
```

二つ目は、このリポジトリ自身の仕様を、検査して、何も書かない（`hozoc` は、Hozo で仕様を書いている: ルートの `cli.hozo` と、ほかの `*.hozo`）。`bin/hozoc build --dry-run` は、書き出す予定のファイルを、並べる。

仕様と、そのコードの、そろった一組を読むなら、[`tests/samples/shop-server`](tests/samples/shop-server): `*.hozo` が仕様、`gen/` が生成されたコード、`app/` が手で書いたコード。

## 次に読むもの

文書は、今は、日本語。処理系の第一言語は、英語で、コンパイラの出力と、文書は、英語に移す。

| したいこと | 読む場所 |
|---|---|
| 何のための言語かを知る | [`docs/requirements.md`](docs/requirements.md)「価値と位置づけ」 |
| 仕様を書く | [`docs/grammar.md`](docs/grammar.md)、[`docs/requirements.md`](docs/requirements.md)「書き方の指針」、[`docs/vocab.md`](docs/vocab.md)、[`docs/config.md`](docs/config.md) |
| コマンドを使う。エディタを整える | [`docs/cli.md`](docs/cli.md)、[`docs/editors.md`](docs/editors.md) |
| プラグインを書く | [`docs/plugin-protocol.md`](docs/plugin-protocol.md)、[`docs/ir.md`](docs/ir.md)、[`schema/ir.schema.json`](schema/ir.schema.json) |
| Hozo 自身を直す | [`CLAUDE.md`](CLAUDE.md)。続けて、どの文書が何を決めているかは [`docs/README.md`](docs/README.md) |
| 決めたことと、決めていないことを知る | [`docs/decisions.md`](docs/decisions.md)、[`docs/open.md`](docs/open.md) |
