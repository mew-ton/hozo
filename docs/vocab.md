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
| `vocab/http.hozo` | `namespace http { ServerError / NetworkError / BadRequest / Violation / get / post / status / header / query / body / headerObj / queryObj / bodyObj / required / retry }` | §4.2。`BadRequest` と `Violation` は足したもの（下）。`header` / `query` / `body` の引数と、`headerObj` / `queryObj` / `bodyObj`、`required` は、出典から変えたか、足したもの（「`http` の語彙」） |
| `vocab/db.hozo` | `namespace db { table / text / int / float / bool / time / json / nullable / key / unique / ref / find / list / insert / update / delete / effects }` | §4.2。`key` / `unique` / `ref` のほかは、出典から変えたか、足したもの（「`db` の読み書きのデコレータ」「`db` の表と、しまい方」） |
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

- `db` は、出典のグローバルのデコレータ `db`（`@db state …`）をなくして `db.table` に置き換え、出典にないデコレータを足す。中身は「`db` の読み書きのデコレータ」「`db` の表と、しまい方」。
- `http` は、要求のどこから読むかを言うフィールドのデコレータを、出典から変える。中身は「`http` の語彙」。
- `proc.stdio` だけは出典から変える: 引数 `command` を持たず、`@external` も付けない（`decorator stdio throws Crashed | MalformedOutput;`）。呼び先は呼び出す op の入力で渡し（`docs/cli.md` の `runEmit`）、例外は同じ名前空間の呼び出し元にも見えなければならないため。`docs/decisions.md`。

### `db` の読み書きのデコレータ

状態が表であること、フィールドを列にどうしまうか、列が値を持たなくてよいか（「`db` の表と、しまい方」）、op が表から何を引き、効果が表に何をするか（この節）を、仕様が言うためのデコレータ（`docs/decisions.md`「決めたこと」92〜95、104、105、117）。`vocab/db.hozo` の全文は次のとおり。出典にあるのは `key` / `unique` / `ref` だけ。

```ts
namespace db {
  /** この状態を、element の行の表として、DB が持つ。列は、element のフィールド */
  decorator table(element: any model) for state;

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

  /** この列は、値がなくてよい。付けていない列は、いつも値を持つ */
  decorator nullable for field;

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
- 表は状態の名前で、列は文字列で渡す。列の名前は、フィールドの名前。列は一つだけ（複数の列の条件は、要るサンプルが出てから。そのときは `column` の型を `string | string[]` に広げる。今の書き方は、そのまま通る）。
- 列を渡す引数の名前を `by` にしないのは、`by` が予約語で、引数の名前（`Ident`）に書けないため（`docs/grammar.md`「予約語」）。
- `int`、`float`、`list`、`nullable`、`effects` は予約語、`delete` は文脈キーワードだが、デコレータの名前には書ける（`docs/grammar.md`「識別子」の `DecoratorName`）。
- 何を生成するかと、書いたものの整合の検査（表は `@db.table` の状態か、列はあるか、引数と戻り値の型は合うか）は、語彙を読むトランスパイラが決める（`docs/plugin-go-sqlite.md`「op の中身」「診断」）。hozoc は、デコレータの引数の種類（`docs/types.md`「デコレータの引数」）だけを確かめる。
- **`vocab/db.hozo` のファイルは、言語の実装（デコレータの `reads` / `effects` の節、残りの引数、対象 `effect`、予約語のデコレータ名）と一緒に、この全文に置き換える。** それまでは、今のパーサーが読める今の中身（グローバルの `db` と、`key`、`unique`、`ref`）のまま（`docs/requirements.md`「作り直しの段階」の R3）。

### `db` の表と、しまい方

- **表**は、状態に `@db.table(<要素の model>)` を付けて言う（`@db.table(type.Order) state orders: list<type.Order>;`）。出典のグローバルのデコレータ `db`（`@db state …`）は、なくした: 表の列を決める model を、状態の型から読ませずに、引数で言わせる（`docs/requirements.md`「出力を推論しない」）。引数の名前が `element` なのは、`model` が予約語で、引数の名前（`Ident`）に書けないため。
- **しまい方**（`text` / `int` / `float` / `bool` / `time` / `json`）は、フィールドを列にどうしまうかの種類で、保存先の製品に依らない名前。表の model のフィールドは、どれか一つを、ちょうど一つ持つ。保存先の列の型と、値の読み書きは、語彙を読むトランスパイラが、この種類だけから決める（`docs/plugin-go-sqlite.md`「しまい方」。フィールドの型は、種類に合うかの検査にだけ使う）。同じ種類の中で、型によって変え方が分かれるようになったら、種類を足す。
- **値がなくてよい列**は、表の model のフィールドに `@db.nullable` を付けて言う（`@db.nullable @db.text nickname: string?`）。付けていない列は、いつも値を持つ。フィールドの型（省略可能かどうか）からは決めない: 型は、食い違いを確かめるのにだけ使い、省略可能でないフィールドの `@db.nullable` も、`@db.nullable` のない省略可能なフィールドも、語彙を読むトランスパイラの error。
- **`@db.ref(M)`** の指す先（M の表と、その列）が一つに決まらなければ、語彙を読むトランスパイラの error（黙って何も出さない、ということはない）。
- 書いたものの整合の検査（状態の要素は表の model か、フィールドはしまい方をちょうど一つ持つか、しまい方は型に合うか、`@db.nullable` は省略可能なフィールドと合っているか、`@db.ref` の先は決まるか）は、語彙を読むトランスパイラが決める（`docs/plugin-go-sqlite.md`「表」「しまい方」「診断」）。

### `http` の語彙

入力の model のフィールドを、要求のどこから読むかと、その項目がなければ誤りかを言うデコレータ（`docs/decisions.md`「決めたこと」107、114）。場所は三つ（ヘッダ、クエリ、本文）で、どれも同じ二つの形を持つ。`vocab/http.hozo` の、この部分は次のとおり（ほかの宣言は、出典のまま）。

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

  /** このフィールドの項目が要求になければ、誤り（op は呼ばれない）。付けていない項目は、なくてもよい */
  decorator required for field;
```

| 場所 | 項目一つ | 場所の全体を、一つの model で |
|---|---|---|
| ヘッダ | `@http.header` / `@http.header("Authorization")` | `@http.headerObj` |
| クエリ | `@http.query` / `@http.query("q")` | `@http.queryObj` |
| 本文 | `@http.body` / `@http.body("from")` | `@http.bodyObj` |

- **項目の形**は、名前を引数に取り、省けば、フィールドの名前（`@http.query page` は `page`、`@http.query("q") keyword` は `q`）。
- **全体の形**（`…Obj`）は、型が model のフィールドにだけ付けられ、項目の名前は、その model のフィールドの名前。一つの場所に高々一つで、同じ場所の項目の形とは一緒に書けない。
- **`@http.required`** は、項目の形（`@http.header` / `@http.query` / `@http.body`）を持つフィールドに足す（`@http.required @http.query id: int`）。その項目が要求になければ、op を呼ぶ前の失敗（`http.BadRequest`）。付けていない項目は、なくても誤りにしない。項目がないことを誤りにするのは、このデコレータだけで、フィールドの型（省略可能かどうか）からは決めない。型は、食い違いを確かめるのにだけ使う: 項目の形を持たないフィールド（全体の形のフィールドと、その model の中のフィールドを含む）に付けられず、省略可能なフィールド（`T?`）にも付けられない。出典にないデコレータ。
- 出典（§4.2）から変えたところ: `header` の `name` を省けるようにし、`query` と `body` に、同じ `name` を足した。「`@http.body` が一つなら、本文がその値。二つ以上なら、フィールドの名前をキーにしたオブジェクト」という、数で形が変わる読み方は、やめた: 本文そのものを一つの model で受け取るなら `@http.bodyObj`、キー一つなら `@http.body`。
- 通信の上の形と、書いたものの整合の検査は、語彙を読むトランスパイラが決める（`docs/plugin-go-http.md`「対象」「通信の形」「診断」）。
- **`vocab/http.hozo` のファイルは、`go-http` の実装と一緒に、この形に置き換える。** それまでは、今の中身（`header(name: string)`、引数のない `body` と `query`。`required` はない）のまま（`docs/requirements.md`「作り直しの段階」の R3）。

## 同梱しない語彙

- `cli`（`examples/hozoc.hozo` の `namespace cli { command / arg / flag / env / exitCode }`）: go-cli プラグインが `vocabulary` で提供する（`plugins/go-cli/cli.hozo`。宣言の全文と、出典から変えたところ（`arg` の位置、`flag` と `env` の省ける名前、足した `required`）の持ち主は `docs/plugin-go-cli.md`「語彙」）。`http` や `page` と同じく、意味をコードにするプラグインが語彙の持ち主になる。hozoc 自身は `hozo.toml` の単位の `transpilers = ["go", "go-valid", "go-cli"]` で読む。
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
