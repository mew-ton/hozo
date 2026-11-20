# 標準の語彙

## この文書が決めること

hozoc が同梱して常に読み込む語彙（`vocab/*.hozo`）と、その読み込み方。語彙の宣言の持ち主は、この文書と `vocab/*.hozo`。出どころは、設計書 §4.2 と `examples/hozoc.hozo`（`docs/README.md`「設計書という呼び名」）で、そこから変えたものがある。

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
| `vocab/http.hozo` | `namespace http { ServerError / NetworkError / BadRequest / Violation / get / post / status / header / query / body / headerObj / queryObj / bodyObj / required / default / retry }` | §4.2。`BadRequest` と `Violation` は足したもの（下）。`header` / `query` / `body` の引数と、`headerObj` / `queryObj` / `bodyObj`、`required`、`default` は、出典から変えたか、足したもの（「`http` の語彙」） |
| `vocab/db.hozo` | `namespace db { table / text / int / float / bool / time / json / key / unique / ref / find / list / where / latest / insert / update / delete / transaction }` | §4.2。`key` / `unique` / `ref` のほかは、出典から変えたか、足したもの（「`db` の語彙」「`db` の表と、しまい方」「`db` の read と効果」） |
| `vocab/localstorage.hozo` | グローバルの `decorator localStorage for state;` | §4.2 |
| `vocab/page.hozo` | `namespace page { Aborted / Route / path / query / route / dialog / fallback / interaction }` | §4.2 |
| `vocab/fs.hozo` | `namespace fs { files }` | `examples/hozoc.hozo` |
| `vocab/proc.hozo` | `namespace proc { Crashed / MalformedOutput / stdio }` | `examples/hozoc.hozo` |

- 各ファイルの中身は出典の宣言をそのまま写す。ドキュメンテーションコメントも写す（ホバーに出るため）。ただし、出典の書き方のうち、言語からなくしたものは直す: 省略可能な引数の `x?: T` は `x: T?` に（`length(min: int?, max: int?)`、`route(path: string, from: any state?, onInvalid: any effect?)`）、節に並べるものの区切りの `|` は `,` に（`decorator get(path: string) throws ServerError, NetworkError;`。`docs/decisions.md`「設計書から離れた決定」S25）。どちらも `docs/grammar.md`「決めたこと」。
- `fs` / `proc` は `examples/hozoc.hozo` では同じファイルの中に語彙として書かれているが、hozoc 自身の境界（ファイルとプラグインの起動）なので標準に入れる。`cli.hozo` からはこれらの宣言を外す（二重に宣言すると RES-6）。
- `@http.get` / `@http.post` の `@external` はそのまま写す。
- `http` には、出典にない例外を一つ足す: op を呼ぶ前の失敗（要求を入力に読めない、入力が制約に合わない）を伝える `BadRequest`。運ぶ値は、例外の引数で書く（`docs/grammar.md`「exception / effect / read」）。HTTP のサーバーのトランスパイラが返し（`docs/plugin-go-http.md`「op を呼ぶ前の失敗」）、通信の上の名前 `http.BadRequest` と値の形の持ち主になる。`@http.get` / `@http.post` の `throws`（外から見たときに加わる例外）には、まだ入れない: 入れると、HTTP の op を呼ぶすべての配線が `http.BadRequest` を書くことになる。呼ぶ側のトランスパイラを作るときに決める。

```ts
  /** 制約の違反一つ。path は通信の上の場所、constraint は valid の語彙のデコレータの名前 */
  model Violation { path: string; constraint: string; }
  /** 要求を op の入力に読めなかった、または入力が制約に合わなかった（op は呼ばれていない） */
  exception BadRequest(message: string, violations: Violation[]);
```

- `db` は、出典のグローバルのデコレータ `db`（`@db state …`）をなくして `db.table` に置き換え、出典にないデコレータを足す。中身は「`db` の語彙」。
- `http` は、要求のどこから読むかを言うフィールドのデコレータを、出典から変える。中身は「`http` の語彙」。
- `proc.stdio` だけは出典から変える: 引数 `command` を持たず、`@external` も付けない（`decorator stdio throws Crashed, MalformedOutput;`。節の区切りは `docs/grammar.md`「decorator の宣言」）。呼び先は呼び出す op の入力で渡し（`docs/cli.md` の `runEmit`）、例外は同じ名前空間の呼び出し元にも見えなければならないため。`docs/decisions.md`。

### `db` の語彙

状態が表であること、フィールドを列にどうしまうか（「`db` の表と、しまい方」）、`read` が表から何を引き、効果が表に何をするか、op のすることを一つのトランザクションにするか（「`db` の read と効果」）を、仕様が言うためのデコレータ（`docs/decisions.md`「決めたこと」134〜138）。`vocab/db.hozo` の全文は次のとおり。出典にあるのは `key` / `unique` / `ref` だけ。

```ts
namespace db {
  /** この状態を、表として、DB が持つ。行は、状態の要素の model で、列は、そのフィールド */
  decorator table for state;

  /** このフィールドを、文字列の列にしまう */
  decorator text for field;
  /** このフィールドを、整数の列にしまう */
  decorator int for field;
  /** このフィールドを、小数の列にしまう */
  decorator float for field;
  /** このフィールドを、真偽値の列にしまう */
  decorator bool for field;
  /** このフィールドを、時刻の列にしまう */
  decorator time for field;
  /** このフィールドを、JSON にして一つの列にしまう */
  decorator json for field;

  decorator key for field;
  decorator unique for field;
  /** 別の model のレコードを指す。DB の構造（外部キーなど）は db の語彙とプラグインが決める */
  decorator ref(target: any model) for field;

  /** この read は、on の状態の表から、一行を引く。引数の名前が列で、どの列も引数の値と同じ行。なければ、ない */
  decorator find for read;
  /** この read は、on の状態の表から、行を引く。引数の名前が列で、どの列も引数の値と同じ行のすべて。引数がなければ、すべての行 */
  decorator list for read;
  /** この read が引くのは、列 column が value と同じ行だけ。何度でも書ける */
  @repeatable
  decorator where(column: string, value: string | int | float | boolean) for read;
  /** この read は、列 column の新しい順に引く */
  decorator latest(column: string) for read;

  /** この効果は、on の状態の表に、引数の行を足す */
  decorator insert for effect;
  /** この効果は、on の状態の表の、キー（@db.key）が引数の行と同じ行を、引数の行に書き換える */
  decorator update for effect;
  /** この効果は、on の状態の表から、行を消す。引数の名前が列で、どの列も引数の値と同じ行 */
  decorator delete for effect;

  /** この op が DB にすることは、すべて、一つのトランザクションで起きる。op が失敗すれば、どれも残らない */
  decorator transaction for op;
}
```

- **`vocab/db.hozo` のファイルは、この全文と、バイト単位で同じ**（`tests/parse_test.go` の `TestDocsEmbedVocabulary` が確かめる。`docs/testing.md`「コーパス」）。言語（`read` と効果の宣言と引数、デコレータの対象 `read` と `effect`、核の印 `@repeatable`、予約語のデコレータ名）は hozoc が読む（`docs/grammar.md`「exception / effect / read」「decorator の宣言」）。語彙を読んで出力にするのは `go-sqlite`（`docs/plugin-go-sqlite.md`）。
- `int`、`float`、`list` は予約語、`delete` は文脈キーワードだが、デコレータの名前には書ける（`docs/grammar.md`「識別子」の `DecoratorName`）。
- hozoc が確かめるのは、付ける対象（`for`）と、引数の種類と型（`docs/types.md`「デコレータの引数」）と、`@repeatable` のないデコレータを一つの宣言に二度付けていないこと（DEC-13）だけ。書いたものどうしの整合（表は `@db.table` の状態か、列はあるか、型は合うか）は、語彙を読むトランスパイラが確かめる（`docs/plugin-go-sqlite.md`「診断」）。

### `db` の表と、しまい方

表の model のフィールドに書くデコレータ（`docs/decisions.md`「決めたこと」105、134）。何を出すかと、整合の検査の持ち主は `docs/plugin-go-sqlite.md`「表」「しまい方」。

- **表**は、状態に `@db.table` を付けて言う（`@db.table state orders: list<type.Order>;`）。引数はない: 表の行の model（**表の model**）は、状態の要素の型（`docs/ir.md`「state / exception / effect」の `State.element`）。出典のグローバルのデコレータ `db`（`@db state …`）は、名前空間 `db` の中の `table` にした。
- **しまい方**（`text` / `int` / `float` / `bool` / `time` / `json`）は、フィールドを列にどうしまうかの種類で、保存先の製品に依らない名前。表の model のフィールドは、どれか一つを、ちょうど一つ持つ。**しまい方は、フィールドの型からは当てない**: `string` のフィールドでも `@db.text` を書く（同じ型を、文字列でも JSON でもしまえる。どれにするかは、書いた人が決める）。型は、種類に合うかの検査にだけ使う。同じ種類の中で、型によって変え方が分かれるようになったら、種類を足す。
- **値がなくてよい列**かどうかは、フィールドの型の `?` が言う（`@db.text nickname: string?` の列は、値がなくてよい。`?` のないフィールドの列は、いつも値を持つ）。型に書いてあることで、当てるのではないので、別のデコレータは要らない。
- **`@db.key`** は、行を一つに決める列（の組）。**`@db.unique`** は、その列だけで行が一つに決まる列。**`@db.ref(M)`** は、M の表の行を指す列で、指す先（M の表と、その列）が一つに決まらなければ、語彙を読むトランスパイラの error。

### `db` の read と効果

`read` と効果の宣言（`docs/check.md`「read と効果」）に付けて、その中身を言うデコレータと、op に付ける `transaction`（`docs/decisions.md`「決めたこと」135〜138）。付けた宣言の中身は、語彙を読むトランスパイラが出し、付けていない宣言の中身は、人が書く。何を出すかと、整合の検査の持ち主は `docs/plugin-go-sqlite.md`「read の中身」「効果の中身」「トランザクション」。

```ts
/** email が同じユーザー */
@db.find
read fn userByEmail(email: type.Email): type.User? on users;

/** そのユーザーの、支払いの済んだ請求。新しい順 */
@db.list
@db.where("paid", true)
@db.latest("issuedAt")
read fn paidInvoicesOf(userId: type.UserId): Invoice[] on invoices;

/** そのトークンのセッションを消す */
@db.delete
effect fn deleteSession(token: type.Token) on sessions.delete;
```

- **デコレータは、宣言がすでに言っていることを、もう一度言わない**（`docs/requirements.md`「出力を推論しない」）: 表は、宣言の `on` の状態。列は、引数の名前（表の model の、同じ名前のフィールド）。引くものの型は、`read` の結果の型。表を引数でもう一度名指しする書き方（`@db.find(users, "email")`）は、なくした。
- **引数が二つ以上なら、どの列も同じ行**（かつ）。`find` と `list` と `delete` に共通。
- **`where`** は、引数にならない、決まった値の条件（列 `column` が、リテラル `value` と同じ）。`find` か `list` を付けた `read` に、いくつでも足せる（`@repeatable`）。引数の条件と合わせて、どれも満たす行。
- **`latest`** は、並べ方。`list` に足せば、列 `column` の新しい順（付けていない `list` は、足した順）。`find` に足せば、条件に合う行のうち、いちばん新しい一行。
- **`find`** は、一行に決まるものでなければならない（決まるかどうかの確かめ方は `docs/plugin-go-sqlite.md`「read の中身」）。
- **`insert`** は宛先が `<状態>.post`、**`update`** は `<状態>.put`、**`delete`** は `<状態>.delete` の効果に付ける。`insert` と `update` の効果の引数は、表の model の行一つ。
- **`transaction`** は、op に付ける。その op が DB にすることが、一つのトランザクションで起きる。op のデコレータなので、op の中身は決めず、中身の前後に処理を足すだけ（継ぎ目。`docs/config.md`「継ぎ目」）。どこまでがトランザクションに入るかと、入れ子の扱いは、語彙を読むトランスパイラが決める（`docs/plugin-go-sqlite.md`「トランザクション」）。
- 等しいかどうかでない比較、件数と位置、二つ以上の列での並べ方は、この語彙では言えない。その `read` か効果には `db` のデコレータを付けず、人が書く。

### `http` の語彙

入力の model のフィールドを、要求のどこから読むかと、その項目が要求にないときの扱いを言うデコレータ（`docs/decisions.md`「決めたこと」107、114、115）。場所は三つ（ヘッダ、クエリ、本文）で、どれも同じ二つの形を持つ。`vocab/http.hozo` の、この部分は次のとおり（ほかの宣言は「同梱する語彙」）。

```ts
  /** このフィールドを、要求のヘッダ一つから読む。name を省けば、フィールドの名前 */
  decorator header(name: string?) for field;
  /** このフィールドを、クエリの項目一つから読む。name を省けば、フィールドの名前 */
  decorator query(name: string?) for field;
  /** このフィールドを、本文（JSON のオブジェクト）のキー一つから読む。name を省けば、フィールドの名前 */
  decorator body(name: string?) for field;

  /** このフィールド（model）で、ヘッダの全体を受け取る。項目の名前は、その model のフィールドの名前 */
  decorator headerObj for field;
  /** このフィールド（model）で、クエリの全体を受け取る。項目の名前は、その model のフィールドの名前 */
  decorator queryObj for field;
  /** このフィールド（model）で、本文の全体を受け取る。本文が、その model の JSON */
  decorator bodyObj for field;

  /** このフィールドの項目が要求になければ、誤り（op は呼ばれない） */
  decorator required for field;
  /** このフィールドの項目が要求になければ、value にする */
  decorator default(value: string | int | float | boolean | Duration) for field;
```

| 場所 | 項目一つ | 場所の全体を、一つの model で |
|---|---|---|
| ヘッダ | `@http.header` / `@http.header("Authorization")` | `@http.headerObj` |
| クエリ | `@http.query` / `@http.query("q")` | `@http.queryObj` |
| 本文 | `@http.body` / `@http.body("from")` | `@http.bodyObj` |

- **項目の形**は、名前を引数に取り、省けば、フィールドの名前（`@http.query page` は `page`、`@http.query("q") keyword` は `q`）。
- **全体の形**（`…Obj`）は、型が model のフィールドにだけ付けられ、項目の名前は、その model のフィールドの名前。一つの場所に高々一つで、同じ場所の項目の形とは一緒に書けない。
- **項目が要求にないときの扱い**は、項目の形（`@http.header` / `@http.query` / `@http.body`）を持つフィールドに足す、二つのデコレータが言う。どちらも出典にない。
  - `@http.required`（`@http.required @http.body lines: OrderLine[]`）: op を呼ぶ前の失敗（`http.BadRequest`）。
  - `@http.default(<リテラル>)`（`@http.query @http.default(1) page: int`）: フィールドを、その値にする。`value` の型は、リテラルの型のすべて（`docs/grammar.md`「リテラル」の `Literal`）。
  - どちらもないフィールドは、零値のまま（省略可能なフィールドは、値がない）。
- **型の宣言に書く既定値（`page: int = 1`）は、言語にない**（`docs/decisions.md`「設計書から離れた決定」S24）。共有の型の既定値は、その型を使うすべての名前空間（ブラウザも、サーバーも）が入れることになる。誰が入れるかは振る舞いなので、境界のデコレータと、それを読むプラグインが言う: HTTP の要求なら、読むサーバーが入れる（`docs/plugin-go-http.md`「通信の形」の「要求」）。
- 項目がないときの扱いは、フィールドの型（省略可能かどうか）からは決めない。型は、食い違いを確かめるのにだけ使う: 二つのデコレータは、項目の形を持たないフィールド（全体の形のフィールドと、その model の中のフィールドを含む）にも、省略可能なフィールド（`T?`）にも付けられず、一つのフィールドに両方は付けられず、`default` のリテラルは、フィールドの型に渡せなければならない（確かめるのは、語彙を読むトランスパイラ）。
- `default` は予約語だが、デコレータの名前には書ける（`docs/grammar.md`「識別子」の `DecoratorName`）。
- 出典（§4.2）から変えたところ: `header` の `name` を省けるようにし、`query` と `body` に、同じ `name` を足した。「`@http.body` が一つなら、本文がその値。二つ以上なら、フィールドの名前をキーにしたオブジェクト」という、数で形が変わる読み方は、やめた: 本文そのものを一つの model で受け取るなら `@http.bodyObj`、キー一つなら `@http.body`。
- 通信の上の形と、書いたものの整合の検査は、語彙を読むトランスパイラが決める（`docs/plugin-go-http.md`「対象」「通信の形」「診断」）。
- **`vocab/http.hozo` のファイルは、この部分を、そのまま含む**（同じテストが確かめる）。読むのは `go-http`（`docs/plugin-go-http.md`「対象」）。

## 同梱しない語彙

- `cli`（`examples/hozoc.hozo` の `namespace cli { command / arg / flag / env / exitCode }`）: go-cli プラグインが `vocabulary` で提供する（`plugins/go-cli/cli.hozo`。宣言の全文と、出典から変えたところ（`arg` の位置、`flag` と `env` の省ける名前、足した `required` と `default`）の持ち主は `docs/plugin-go-cli.md`「語彙」）。`http` や `page` と同じく、意味をコードにするプラグインが語彙の持ち主になる。hozoc 自身は `hozo.toml` の単位の `transpilers = ["go", "go-valid", "go-cli"]` で読む。
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
