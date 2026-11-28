# valid の語彙のプラグイン

## この文書が決めること

語彙のプラグイン `valid`（`docs/plugin-protocol.md`「語彙のプラグイン」）が持つ、`valid` の語彙の、**言語に依らない決まり**: 制約が、何に付けられ、どの値を通すか（「制約」「形式」）、値一つを制約で確かめる方法（「値一つを確かめる」）、書いたものの整合の検査（「診断」の VALID-*）。プラグインは `plugins/valid/`。決まりを読み解くコードは、Go のライブラリ `plugin/validvocab`（`docs/packages.md`「`plugin/validvocab`（公開）」）で、このプラグインと、`valid` の制約を読むプラグイン（`go-valid`、`go-cli`、語彙のプラグイン `http`）が、同じものを使う。

語彙の宣言（五つのデコレータ）の持ち主は `docs/vocab.md`「同梱する語彙」。ある言語への出し方（確かめる関数の形）は、その言語のトランスパイラの文書が持つ（Go は `docs/plugin-go-valid.md`）。

## マニフェスト

```toml
name     = "valid"
wasm     = "plugin.wasm"
protocol = "0.27"

[read]
namespaces = ["valid"]
```

- 読み解くのは、標準の語彙 `valid`（`vocab/valid.hozo`。hozoc に同梱）。このプラグインは、語彙の宣言（`.hozo`）を持たない。
- トランスパイラは、提供しない: ファイルを出さず、単位の `transpilers` に書く名前を持たない。
- 頼まれるのは、仕様のどこかに `valid` のデコレータが付いているとき（`docs/plugin-protocol.md`「語彙のプラグイン」）。出力の単位は、要らない。

## 対象

仕様の、すべての名前空間の scalar と、model のフィールド（出力の単位がない名前空間のものも、含む。決定 D429）に付いた、**標準の制約**: `@valid.min`、`@valid.max`、`@valid.length`、`@valid.pattern`、`@valid.format`。

- **制約は、型に一度だけ書く、型の性質。** それ自体は、何も起こさない。いつ確かめるか（HTTP の要求を受けたとき、コマンドの引数を読んだとき）は、境界の語彙を出すトランスパイラが決める（Go は `docs/plugin-go-valid.md`「ほかの相乗りとの約束」）。
- **標準でない `valid` の制約**（名前空間 `valid` に、ほかのプラグインの語彙のファイルが宣言したデコレータ。仕様からは、足せない: `docs/vocab.md`「語彙の名前空間」）は、このプラグインは、読まない: 診断にも、しない。確かめる中身を、人が書くもので、受け取る形を持つかは、言語のトランスパイラが決める（`go-valid` は GOVALID-5）。
- 継承した model は、展開済みのフィールド（受け継いだデコレータを含む）で見る。受け継いだ制約に誤りがあれば、受け継いだ model にも、同じ診断が出る。

この節の細部は、決定 D711、D741 が決めた。

## 制約

**元の型**は、scalar なら、その継承を辿った核の型。フィールドなら、その型から省略可能（`T?`）を外したもの（scalar なら、同じに辿る）。

| 制約 | 付けられる先 | 通す値 | 違反の名前 |
|---|---|---|---|
| `min(value)` | 元の型が `int` か `float` | `value` 以上 | `min` |
| `max(value)` | 同上 | `value` 以下 | `max` |
| `length(min?, max?)` | 元の型が `string` | 文字の数（Unicode の符号位置の数）が、`min` 以上、`max` 以下。書かなかった側は、限らない | `length` |
| `length(min?, max?)`（配列のフィールド） | 型が配列（`T[]`、`T[]?`）のフィールド | **要素の数**が、`min` 以上、`max` 以下 | `count` |
| `pattern(regex)` | 元の型が `string` | 正規表現 `regex` が、値のどこかに当たる（値の全体に当てるなら、`^` と `$` を書く） | `pattern` |
| `format(name)` | 元の型が `string` | 名前 `name` の形式（「形式」）の文字列 | `format` |

- 表の「付けられる先」に合わない制約は VALID-1。配列のフィールドに付けられるのは `length` だけで、インデックスの model（`docs/types.md`「インデックスの model」）を型に持つフィールドには、どの制約も付けられない（制約を付けるのは、値の型）。どちらも VALID-1。
- 元の型が `int` の先に付けた `min` / `max` の `value` は、整数（整数のリテラルの範囲: 64 ビット符号付き。`docs/types.md`「診断」の LIT-1）。小数のリテラルでも、値が、その範囲の整数なら、よい（`1.0`）。整数でないか、範囲の外なら VALID-2。値は、丸めずに読む（2^53 を超える整数も、そのまま比べる。決定 D552）。`length` は、`min` か `max` の、少なくとも一方を書く。どちらもなければ VALID-2。
- **違反の名前**は、違反を伝えるときに、どの制約に合わなかったかを言う名前（HTTP では、`http.BadRequest` の `violations` の `constraint`: `docs/plugin-http.md`「通信の形」）。デコレータの名前と同じ。**例外は一つ**: 配列のフィールドに付けた `length` の違反は、`count`（文字列の長さの違反 `length` と、言い分ける。決定 D400）。
- **配列の要素に制約を掛ける道は、一つ**（同 401）: 要素の型を scalar にして、そちらに付ける（`@valid.length(min: 1, max: 20) scalar Tag extends string;` と `@valid.length(max: 10) tags: Tag[];`）。配列のフィールドに付けた `length` が、要素に掛かることは、ない。warning も、出さない。
- **正規表現の文法は、RE2**（<https://github.com/google/re2/wiki/Syntax>。後読み、後方参照は、ない）。RE2 で解釈できない `regex` は VALID-3。RE2 に決める理由: 当たるかどうかが、言語の正規表現の実装の違いで変わらないように、文法を一つに決める。RE2 は、当てるのに掛かる時間が、値の長さに比例する。
- 一つの先に、制約をいくつ付けてもよく、値は、すべてを通らなければならない。制約どうしの食い違い（`min` が `max` より大きい、など）は、確かめない。

## 形式

`format(name)` の `name` は、次の四つ。ほかの名前は VALID-4。

| `name` | 通す文字列 |
|---|---|
| `email` | `local@domain`。`local` は、RFC 5322 の dot-atom（atext を `.` でつないだもの。先頭、末尾、連続の `.` は不可）で、64 文字以内。`domain` は、`.` でつないだ、二つ以上のラベル（英数字とハイフン。先頭と末尾のハイフンは不可。各 63 文字以内、全体で 253 文字以内）。引用した `local`（`"a b"@x.y`）と、アドレスリテラル（`@[1.2.3.4]`）は、通さない |
| `uri` | RFC 3986 の絶対 URI。scheme を持ち、ASCII の可視文字だけ。相対参照（`/a/b`）は、通さない |
| `uuid` | RFC 4122 の文字列の形（8-4-4-4-12 の 16 進。大文字と小文字は、問わない）。版は、確かめない |
| `date-time` | RFC 3339 の date-time。`T` と `Z` は、小文字でもよい。閏秒（秒が 60）も通す。時刻帯は必須（`2024-01-02T03:04:05` は、通さない） |

Go の実装は `runtime.CheckFormat`（`docs/plugin-go-valid.md`「runtime」）。

## 値一つを確かめる

境界のデコレータが補う値（`@http.default`、`@cli.default`）は、そのフィールドの制約で、生成の前に確かめる（`docs/plugin-http.md`「補う値と制約」、`docs/plugin-go-cli.md`「補う値と制約」）。確かめ方は、一つ（`plugin/validvocab` の `CheckLiteral`）:

- 確かめる制約は、標準の制約のうち、フィールドの型の scalar（と、その元の scalar）に付いたものと、フィールドに付いたもの。順は、元の scalar から、自分の scalar へ、その後にフィールド。それぞれ、書かれた順。省略可能なフィールドは、中の型で見る。
- 値が制約を通るかは、「制約」の表の「通す値」で決める。
- 確かめられないものは、合わないとは言わない: 値の型に当たらない制約と、解釈できない正規表現（VALID-1、VALID-3 が、別に知らせる）、標準でない `valid` の制約。

## 診断

`read` の応答の `errors` で返す（`docs/plugin-protocol.md`「read」）。順は、名前空間の IR の順、その中は、宣言順、フィールドの順、制約の書かれた順。位置は、制約の付いた scalar か、フィールドを持つ model の FQN。warning は、返さない。

この見出しの診断の、ID と条件と重さの持ち主は、`spec/diagnostics/valid.hozo` の分類 `hozo.diagnostics.Valid` のケース。

- **どの診断も、書いたものの整合の検査**: 制約が言ったことと、付けた先の型が食い違っていれば、error にする。
- **VALID-* は、`go-valid` の GOVALID-1 を、四つに分けて移したもの。** 分け方の表は `docs/plugin-go-valid.md`「診断」の「欠番」。
- 仕様が、検査の前提を欠くとき（要求に `spec` がない）は、診断にせず、異常終了する（`Crashed`。`docs/plugin-protocol.md`「起動」）。

この節の細部は、決定 D711 が決めた。
