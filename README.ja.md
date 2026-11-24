# Hozo

[English](README.md) | 日本語

**Hozo は、アプリの設計の、ただ一つの正（SSOT）になる、仕様の言語。**

- 要件や設計を、AI が書いても、人が、レビューして、手直しできる形で、残る。
- レビューした仕様が、そのまま、コードの枠と、テストのハーネスになる。
- 人が責任を持つのは、仕様。中身は、AI に任せられ、中身が仕様から外れないことは、ハーネスが確かめる。

Hozo は、未公開で、版は 0.x。言語も、IR も、プラグインとのやり取りも、互換を保たずに、変わる（[`docs/ir.md`](docs/ir.md)「版」）。

## 仕様の見た目

サンプル [`tests/samples/shop-server/server.hozo`](tests/samples/shop-server/server.hozo) からの、抜き書き:

```
/** 注文する。注文と、その通知の記録は、一緒に残す。通知の配送に失敗しても、どちらも残る */
@http.post("/orders")
@prd.ref(design.Requirement.PRD_001, design.Requirement.PRD_002)
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

| 仕様に書くもの | `hozoc` が、それをどうするか |
|---|---|
| 中身のある関数（`placeOrder`） | 中身は、配線: どの関数を、どの順に呼ぶか。コードは、そこから作る。 |
| 中身のない関数（`newOrder`） | 人か AI が書く。実装が欠けていれば、生成したコードの、コンパイルエラーになる。 |
| 語彙のデコレータ（`@http.post`） | それを解釈するプラグインが、コードを作る。ここでは、HTTP のハンドラ。 |
| 要求への結び付け（`@prd.ref`） | 注記: その関数が、どの要求に応えるかを言う。コードは、変えない。要求ごとに、テストの入れ物ができ（[`docs/plugin-go-design.md`](docs/plugin-go-design.md)）、要るテストが走ったことは、`hozoc tests` が確かめる。 |

文法は [`docs/grammar.md`](docs/grammar.md)。仕様の書き方の勧めは [`docs/requirements.md`](docs/requirements.md)「書き方の指針」。

## このリポジトリにあるもの

| 場所 | 中身 |
|---|---|
| `cmd/hozoc`、`internal/` | コンパイラ `hozoc`: `build`（検査して、コードを書き出す）、`tests`（要るテストが走ったかを確かめる）、`lsp`（言語サーバー） |
| `plugins/` | プラグイン。語彙: `http`、`valid`、`db`。Go のトランスパイラ: `go`、`go-valid`、`go-http`、`go-sqlite`、`go-cli`、`go-design` |
| `plugin/`、`ir/`、`schema/` | プラグインを Go で書くための、見本の SDK。IR。IR の JSON Schema |
| `runtime/` | 生成したコードが import する、Go のパッケージ |
| `vocab/` | 同梱の語彙の宣言 |
| `editors/vscode` | VS Code 拡張 |
| `*.hozo`、`hozo.toml` | Hozo で書いた、Hozo 自身の仕様。`hozoc` のコマンドラインは、`cli.hozo` から生成している |
| `tests/` | コーパス、規則ごとのテスト、手で書いた期待する出力を持つサンプル |
| `docs/` | 仕様と、設計の文書 |

`hozoc` の本体は、どの言語のコードも持たない。生成するコードは、すべて、プラグインが出す。プラグインは、WebAssembly のモジュールで、`hozoc` の中の、サンドボックスで動く。ネットワークにも、ファイルにも、触れない（[`docs/plugin-protocol.md`](docs/plugin-protocol.md)）。

まだ、ないもの: Go のほかの言語への出力。仕様をレビューして、手直しする UI。

## 試す

Go 1.24.7 以上が要る。

```bash
make build
```

```bash
bin/hozoc build --no-emit
```

二つ目は、このリポジトリ自身の仕様を、検査する。ファイルは、書かない。`bin/hozoc build --dry-run` は、書き出す予定のファイルを、並べる。コマンドの説明は [`docs/cli.md`](docs/cli.md)。

## 文書

文書は、今は、日本語。処理系の第一言語は、英語で、コンパイラの出力と、文書は、英語に移す（[`docs/decisions.md`](docs/decisions.md) 442、488）。

| 知りたいこと | 読む場所 |
|---|---|
| どの文書が、何を決めているか。読む順 | [`docs/README.md`](docs/README.md) |
| 何のための言語か | [`docs/requirements.md`](docs/requirements.md)「価値と位置づけ」 |
| 決めたこと | [`docs/decisions.md`](docs/decisions.md) |
| まだ決めていないこと。次に何をするか | [`docs/open.md`](docs/open.md) |

## 開発

```bash
make test
```

```bash
make generate
```

`make generate` は、`cli.hozo` から、`internal/gen/hozo/` を、生成し直す。push の前に走らせる確かめと、CI が確かめることは、[`CLAUDE.md`](CLAUDE.md)「確かめ方」と、[`docs/testing.md`](docs/testing.md)。
