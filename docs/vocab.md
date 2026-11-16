# 標準の語彙

## この文書が決めること

hozoc が同梱して常に読み込む語彙（`vocab/*.hozo`）と、その読み込み方。語彙の宣言の意味の持ち主は設計書 §4.2 と `examples/hozoc.hozo`。ここに置く `.hozo` はそれを写したもの。

## 読み込み

- `vocab/*.hozo` は hozoc のバイナリに埋め込む（Go の `embed`）。`hozoc check` / `emit` / `lsp` は、これらをプロジェクトのファイルの一部として読む（順序は `docs/ir.md`「原則」）。同梱して常に読むのは、仕様の検査に展開のプラグインを要求しないため（宣言は実行環境を持たない。§4.5）。`Span.file` の形は `docs/ir.md`「Program」。
- プラグインのマニフェストの `vocabulary`（`docs/plugin-protocol.md`「マニフェスト」）の `.hozo` も同じ扱い。読むのは使うプラグイン（そのプラグインのトランスパイラが、どれかの単位の `transpilers` に書かれている。`docs/plugin-protocol.md`「探索」）のものだけ。入っているだけのプラグインの語彙は読まない。入れただけで、使っていないプラグインの宣言がプロジェクトの名前に加わらないようにするため（名前の衝突は NS-3 / RES-6）。
- ユーザーは同じ名前空間に宣言を足せる（§3.2、§6.5）。標準の語彙の宣言と同じ名前を足せば RES-6。
- 標準の語彙は `hozoc lsp` の定義へジャンプの飛び先になる。埋め込んだ内容を読み取り専用の仮想文書として返す（`docs/lsp.md`）。

## 語彙の名前空間

**語彙の名前空間**は、宣言のすべてが語彙のファイル（`Span.file` が `hozo:` で始まる。`docs/ir.md`「Program」）にある名前空間。標準の語彙（`valid`、`http`、`page` など）も、使うプラグインの語彙（`cli`）も同じに数える。宣言を一つも持たない名前空間は数えない。ユーザーが同じ名前空間に宣言を一つでも足せば（§6.5 の `valid` への制約の追加など）、その名前空間はもう語彙の名前空間ではない。

語彙の名前空間も、単位の `namespace` に書ける（`docs/config.md`「単位の対象」）。語彙の型（`http` や `proc` の例外、`page.Route` など）を使う出力は、その語彙の名前空間に、同じ基底の単位を要る（`docs/config.md`「型の置き場」）。語彙のファイルの宣言は、単位の `in` によらず、いつも単位の対象の宣言（`docs/config.md`「in」）。デコレータだけの語彙（`valid`、`db`、`cli` など）は、型を持たないので、単位は要らない。

## 同梱する語彙

| ファイル | 中身 | 出典 |
|---|---|---|
| `vocab/valid.hozo` | `namespace valid { min / max / length / pattern / format }` | §4.2、§6.5 |
| `vocab/http.hozo` | `namespace http { ServerError / NetworkError / BadRequest / Violation / get / post / status / header / body / query / retry }` | §4.2。`BadRequest` と `Violation` は足したもの（下） |
| `vocab/db.hozo` | グローバルの `decorator db for state;` と `namespace db { key / unique / ref / find / list / insert / update / delete / effects }` | §4.2。`find` から後ろの六つは足したもの（下の「`db` の読み書きのデコレータ」） |
| `vocab/localstorage.hozo` | グローバルの `decorator localStorage for state;` | §4.2 |
| `vocab/page.hozo` | `namespace page { Aborted / Route / path / query / route / dialog / fallback / interaction }` | §4.2 |
| `vocab/fs.hozo` | `namespace fs { files }` | `examples/hozoc.hozo` |
| `vocab/proc.hozo` | `namespace proc { Crashed / MalformedOutput / stdio }` | `examples/hozoc.hozo` |

- 各ファイルの中身は出典の宣言をそのまま写す。ドキュメンテーションコメントも写す（ホバーに出るため）。ただし省略可能な引数の `x?: T` は `x: T?` に直す（`length(min: int?, max: int?)`、`route(path: string, from: any state?, onInvalid: any effect?)`。`docs/grammar.md`「決めたこと」）。
- `fs` / `proc` は `examples/hozoc.hozo` では同じファイルの中に語彙として書かれているが、hozoc 自身の境界（ファイルとプラグインの起動）なので標準に入れる。`cli.hozo` からはこれらの宣言を外す（二重に宣言すると RES-6）。
- `@http.get` / `@http.post` の `@external` はそのまま写す。
- `http` には、出典にない例外を一つ足す: op を呼ぶ前の失敗（要求を入力に読めない、入力が制約に合わない）を伝える `BadRequest` と、それが運ぶ値。HTTP のサーバーのトランスパイラが返し（`docs/plugin-go-http.md`「op を呼ぶ前の失敗」）、通信の上の名前 `http.BadRequest` と値の形の持ち主になる。`@http.get` / `@http.post` の `throws`（外から見たときに加わる例外）には、まだ入れない: 入れると、HTTP の op を呼ぶすべての配線が `http.BadRequest` を書くことになる。呼ぶ側のトランスパイラを作るときに決める。

```ts
  /** 制約の違反一つ。path は通信の上の場所、constraint は valid の語彙のデコレータの名前 */
  model Violation { path: string; constraint: string; }
  /** 要求を op の入力に読めなかった、または入力が制約に合わなかった（op は呼ばれていない） */
  exception BadRequest: { message: string; violations: Violation[]; };
```

- `db` には、出典にないデコレータを六つ足す。中身は「`db` の読み書きのデコレータ」。
- `proc.stdio` だけは出典から変える: 引数 `command` を持たず、`@external` も付けない（`decorator stdio throws Crashed | MalformedOutput;`）。呼び先は呼び出す op の入力で渡し（`docs/cli.md` の `runEmit`）、例外は同じ名前空間の呼び出し元にも見えなければならないため。`docs/decisions.md`。

### `db` の読み書きのデコレータ

op が表から何を引き、効果が表に何をするかを、仕様が言うためのデコレータ（`docs/decisions.md`「決めたこと」92〜95）。`vocab/db.hozo` の全文は次のとおり。`find` から後ろが足したもの。

```ts
/** この状態を DB が持つ */
decorator db for state;

namespace db {
  decorator key for field;
  decorator unique for field;
  /** 別の model のレコードを指す。DB の構造（外部キーなど）は db の語彙とプラグインが決める */
  decorator ref(target: any model) for field;

  /** この op は、表 table から、列 column が引数の値と同じ一行を引く。なければ、ない */
  decorator find(table: any state, column: string) for op reads table;
  /** この op は、表 table の行をすべて引く。column を書けば、その列が引数の値と同じ行だけ */
  decorator list(table: any state, column: string?) for op reads table;

  /** この効果は、表 table に行を一つ足す */
  decorator insert(table: any state) for effect;
  /** この効果は、表 table の、キー（@db.key）が同じ行を書き換える */
  decorator update(table: any state) for effect;
  /** この効果は、表 table から、列 column が引数の値と同じ行を消す */
  decorator delete(table: any state, column: string) for effect;

  /** この op は、並べた効果を、書いた順に、一つのトランザクションで行う。n 番目の効果が、op の n 番目の引数を受け取る */
  decorator effects(...targets: any effect) for op effects targets;
}
```

- `find` と `list` は、表の状態を `reads` で、`effects` は、並べた効果を `effects` で op に運ぶ（`docs/check.md`「デコレータが運ぶ reads と effects」）。これらを付けた op は、`reads` も `effects` も自分では書かない。
- 表は状態の名前で、列は文字列で渡す。列は一つだけ（複数の列の条件は、要るサンプルが出てから。そのときは `column` の型を `string | string[]` に広げる。今の書き方は、そのまま通る）。
- 列を渡す引数の名前を `by` にしないのは、`by` が予約語で、引数の名前（`Ident`）に書けないため（`docs/grammar.md`「予約語」）。
- `list` と `effects` は予約語、`delete` は文脈キーワードだが、デコレータの名前には書ける（`docs/grammar.md`「識別子」の `DecoratorName`）。
- 何を生成するかと、書いたものの整合の検査（表は `@db` の状態か、列はあるか、引数と戻り値の型は合うか）は、語彙を読むトランスパイラが決める（`docs/plugin-go-sqlite.md`「op の中身」「診断」）。hozoc は、デコレータの引数の種類（`docs/types.md`「デコレータの引数」）だけを確かめる。
- **`vocab/db.hozo` のファイルは、言語の実装（デコレータの `reads` / `effects` の節、残りの引数、対象 `effect`、予約語のデコレータ名）と一緒に、この全文に置き換える。** それまでは、今のパーサーが読める今の中身（`db`、`key`、`unique`、`ref`）のまま（`docs/requirements.md`「作り直しの段階」の R3）。

## 同梱しない語彙

- `cli`（`examples/hozoc.hozo` の `namespace cli { command / arg / flag / env / exitCode }`）: go-cli プラグインが `vocabulary` で提供する（`plugins/go-cli/cli.hozo`。`docs/plugin-go-cli.md`「マニフェスト」）。`http` や `page` と同じく、意味をコードにするプラグインが語彙の持ち主になる。hozoc 自身は `hozo.toml` の単位の `transpilers = ["go", "go-valid", "go-cli"]` で読む。
- `cache`（§4.2）: 今のスコープでは考えない（`docs/decisions.md`「スコープ外にした設計書の例」）。

## 語彙の意味の実装

語彙の宣言は名前と説明と関係だけ（§2）。意味をコードにするのは:

| 語彙 | 実装 |
|---|---|
| `valid` の標準の制約 | `go-valid`（`docs/plugin-go-valid.md`） |
| `cli`（go-cli の語彙） | `plugins/go-cli/`（`docs/plugin-go-cli.md`） |
| `fs` / `proc` | 語彙を実装するプラグインはない。`@proc.stdio` の付いた op は、基底が、人が実装する葉として出す（`docs/plugin-go.md`「op の置き場」。hozoc 自身では `runEmit`。`docs/cli.md`「手で書くもの」）。`@fs.files` は読むトランスパイラがなく、印のまま（`docs/plugin-go.md`「デコレータ」）。`proc` の例外の型は、`proc` の単位の基底が出す |
| `http`（サーバー） | `go-http`（`docs/plugin-go-http.md`） |
| `db` | `go-sqlite`（`docs/plugin-go-sqlite.md`） |
| `http`（クライアント）/ `localStorage` / `page` | 段階 5 のプラグイン |
