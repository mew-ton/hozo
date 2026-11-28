# go-cli プラグイン

## この文書が決めること

トランスパイラ `go-cli`（プラグインは `plugins/go-cli/`）が受け付けない書き方。`go-cli` は、語彙 `cli` を Go の CLI の骨組みにし、Go の基底 `go`（`plugins/go/spec/go.hozo`）と、検証の `go-valid`（`docs/plugin-go-valid.md`）に相乗りする。hozoc 自身の骨組みも、これで生成する（`docs/requirements.md`「自己展開」）。

ほかのことの持ち主は、プラグインのファイル（決定 D811）:

- 生成する CLI の骨組みの形と振る舞い: `plugins/go-cli/spec/go-cli.hozo` の要求 `hozo.gocli.Requirement`。
- 語彙 `cli` の宣言と意味: `plugins/go-cli/cli.hozo`。
- マニフェスト: `plugins/go-cli/hozo-plugin.toml`。
- 診断（CLI-*）の ID と条件と重さ: `spec/diagnostics/go-cli.hozo`（「診断」）。

## 対象

`View.viewer` の名前空間の op のうち、単位の対象（`docs/config.md`「in」。`View.files`）で、`@cli.command(name)` の付いたものがサブコマンド（`plugin.OpsWith`）。

- **`@cli.command` の付いた op は `export` が付いていなければならない。** `Main` は、名前空間の外から、入口の `Ops`（`export` の op だけ）を通して op を呼ぶ。付いていなければ CLI-6（`http` の HTTP-1 と同じ理由。`docs/plugin-http.md`「対象」）。
- 引数は 0 個か、model 一つ。それ以外は CLI-1。
- 引数の model の各フィールドは、`@cli.arg(position)`、`@cli.flag`、`@cli.env` のどれかを持つ。`@cli.env` は `@cli.arg` か `@cli.flag` と一緒に付けられる（与えられなかったときの代わり）。`@cli.arg` と `@cli.flag` の両方、またはどれもなければ CLI-2。
- フィールドの型は `string`、`boolean`、`int`、`float`、元がこれらの `scalar`、またはそれらの `optional`、`@cli.arg` に限り `string`（と `string` の scalar）の `array`（`optional` は付けない。0 個を表せるため）。それ以外（model（インデックスの model を含む）、`datetime`、`optional` の `array`）は CLI-3。
- `@cli.required` と `@cli.default` は、省略可能なフィールド（`T?`）に付けられない。付いていれば CLI-11（型は「なくてよい」と言っている: なければ誤りにするなら、また、なければ値を入れるなら、省略可能にしない）。一つのフィールドに、両方は付けられない（CLI-12）。`@cli.default` のリテラルは、フィールドの型（scalar は元の核の型）に渡せなければならない（`docs/types.md`「互換」の 10。渡せなければ CLI-13。`array` のフィールドに渡せるリテラルはない）。値の出どころ（`@cli.arg`、`@cli.flag`、`@cli.env`）を一つも持たないフィールドは、この二つが付いていても CLI-2。
- `@cli.arg` の位置は、1 から、重なりも抜けもなく続く（一つなら 1、三つなら 1、2、3）。1 より小さい位置、同じ位置が二つ、抜けた位置があれば CLI-10。
- `@cli.arg` の `array` のフィールド（残りの引数をすべて受け取る）は、いちばん後ろの位置の一つだけ。違えば CLI-4。
- `@cli.arg` に `boolean` は付けられない（CLI-3）。
- リテラルの型（`docs/types.md`「リテラルの型」）と、リテラルの型のアームを持つ union は、まだ、受け付けない（CLI-3）: 値の集まりを、フラグや引数の値として読む形（合わない値の扱いを含む）を、決めていない（`docs/open.md`「語彙とプラグイン」）。
- 同じフラグの名前（`@cli.flag` の引数。省いていれば、フィールドの名前）、同じ `@cli.command` の名前が二つあれば CLI-5。
- 型についての決まり（CLI-3、CLI-4、CLI-11、CLI-13）は、書いたものの整合の検査で、出力を変えない（どの型でも、同じ補助の呼び出しになる。`plugins/go-cli/spec/go-cli.hozo` の `Requirement.CommandFunctions`）。

- 入力の model が**別の名前空間 M のもの**で、`Validate` を持つ型（`govalid.HasValidate`。`docs/plugin-go-valid.md`「Validate を持つ型」）なら、M の型の置き場（`docs/config.md`「型の置き場」）の単位も `transpilers` に `go-valid` を並べていなければならない。並べていなければ CLI-9（その入力が、確かめられないまま op に渡るのを、先に報告する。`go-http` の GOHTTP-11 と同じ。`docs/plugin-go-http.md`「制約の検査」。確かめ方は `govalid.Unlisted`）。N の model は、自分の単位が `go-valid` を並べている（CFG-19）。

CLI-* は emit の応答の `errors` で返す。検査だけの実行（`hozoc build --no-emit`）も、同じ応答の診断を使う（`docs/plugin-protocol.md`「起動」「emit」）ので、どれも error の診断になる。

この節の細部は、決定 D781 が決めた。

## 補う値と制約

`@cli.default` の値は、生成するときに、そのフィールドの制約（`@valid.*`）で確かめる。合わなければ CLI-14。確かめ方の持ち主は `docs/plugin-valid.md`「値一つを確かめる」。生成するときに確かめる理由は、語彙のプラグイン `http` と同じ（`docs/plugin-http.md`「補う値と制約」）。

## 診断

この見出しの診断の、ID と条件と重さの持ち主は、`spec/diagnostics/go-cli.hozo` の分類 `hozo.diagnostics.GoCli` のケース。

warning（コードは `PLUGIN`）は一つ: `@cli.command` の op が一つもない。

- 入力の model の診断（CLI-2〜CLI-5 のフラグ名、CLI-10〜CLI-14）は、同じ model を二つのサブコマンドが使っていても、一度だけ返す。
- 基底のオプション `module` がないとき（同じ単位の基底が GO-8 を返し、何も書き出されない。`docs/plugin-go.md`「診断」）は、ファイルを返さず、GO-8 も重ねて返さない。上の診断は、そのときも返す。
- 生成器の不具合（`View` が欠けている、生成したコードを `go/format` で整形できない）は、診断にせず、異常終了する（`Crashed`。`docs/plugin-protocol.md`「起動」）。
