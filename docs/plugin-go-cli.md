# go-cli プラグイン

## この文書が決めること

`cli` の語彙（`docs/vocab.md` の `vocab/cli.hozo`）を Go の CLI の骨組みにするプラグイン `plugins/go-cli/`。hozoc 自身の骨組みもこれで生成する（`docs/requirements.md`「自己展開」）。

## マニフェスト

```toml
name     = "go-cli"
command  = ["go", "run", "github.com/mew-ton/hozo/plugins/go-cli"]
language = "go"
protocol = "0.1"

[emit]
entry = ["cli.command"]
```

`vocabulary` は書かない（`cli` は標準の語彙）。

## 出力

名前空間 N の `View.config.out` の下に `cli/cli.go`（パッケージ `cli`）を一つ。N のパッケージを import する。

```go
// Main は引数を解釈して ops の op を呼び、結果を p で表示し、終了コードを返す。
func Main(ops hozoc.Ops, p Presenter, args []string, env func(string) string, stdout, stderr io.Writer) int

// Presenter は結果と例外の表示。既定は JSON（JSONPresenter）。
type Presenter interface {
    Present(w io.Writer, command string, input any, result any) error
    PresentError(w io.Writer, command string, input any, err error) error
}

type JSONPresenter struct{}
```

- `Main` は `package main` ではない。人が書く `main.go` は、`Leaves` などの実装を組み立てて `os.Exit(cli.Main(ops, p, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))` を呼ぶだけ。
- `ops` の型は N の `Ops`（`docs/emit-go.md`「op」）。配線の op とデコレータの包み（`Wrappers`）を通すため、`Leaves` ではない。

## 対象

`@cli.command(name)` の付いた op がサブコマンド。

- 引数は 0 個か、model 一つ。それ以外は CLI-1。
- 引数の model の各フィールドは、`@cli.arg`、`@cli.flag(name)`、`@cli.env(name)` のどれかを持つ。`@cli.flag` と `@cli.env` は両方付けられる。どれもなければ CLI-2。
- フィールドの型は `string`、`boolean`、`int`、`float`、元がこれらの `scalar`、またはそれらの `optional`、`@cli.arg` に限り `string`（と `string` の scalar）の `array`。それ以外（model、`Record`、`Time`）は CLI-3。
- `@cli.arg` の `array` のフィールドは、`@cli.arg` のフィールドの中で最後の一つだけ。違えば CLI-4。
- `@cli.arg` に `boolean` は付けられない（CLI-3）。
- 同じ `@cli.flag` の名前、同じ `@cli.command` の名前が二つあれば CLI-5。

CLI-* は emit の応答の `errors`（`docs/plugin-protocol.md`）。

## 引数の解釈

```
hozoc <command> [flags] [args]
```

- `<command>` がなければ使い方を標準エラーに出して 64。`help` か `--help` / `-h` なら使い方を標準出力に出して 0。`<command> --help` はそのコマンドの使い方。
- フラグは `--name value`、`--name=value`。`boolean` は `--name` で true（値は取らない。`--name=false` は取れる）。`--` 以降は位置引数。
- 知らないフラグ、フラグの値がない、値を型に変換できない（`int` に `"x"`）、必須のフラグがない、位置引数が足りない・多い → 使い方を標準エラーに出して 64。
- 必須: 型が `optional` でなく、`default` もないフィールド。
- `@cli.env` は、フラグが与えられなかったときに環境変数の値を使う（フラグが優先）。環境変数が空文字列なら「ない」とみなす。
- `@cli.arg` は `@cli.arg` のフィールドの宣言順に位置引数を当てる。`array` は残りすべて（0 個でもよい）。
- 値の変換: `int` は `strconv.ParseInt(v, 10, 64)`、`float` は `ParseFloat`、`boolean` は `ParseBool`。scalar は元の型で変換してから scalar の型にし、`Validate()` を呼ぶ。違反は 64（メッセージに `Violation`）。
- 解釈した入力の model で `Validate()` を呼び、違反は 64。

## 実行と表示

1. `ops.<Op>(context.Background(), input)` を呼ぶ。
2. 成功なら `p.Present(stdout, command, input, result)` を呼び（void なら `result` は `nil`）、0。
3. 例外なら、終了コードを決めてから `p.PresentError(stderr, command, input, err)` を呼ぶ。終了コードは、`err` が `@cli.exitCode(code)` の付いた例外の型（`errors.As`）ならその `code`。`Signature.exceptions` の順に試す。どれでもなければ 70。
4. `Present` / `PresentError` 自体が失敗したら、そのエラーを標準エラーに出して 70。

`JSONPresenter`:

- `Present`: `result` を `json.MarshalIndent(result, "", "  ")` で標準出力に。`nil` なら何も出さない。
- `PresentError`: `{"error": "<HozoException()>", "value": <Value があれば>}` を標準エラーに。`runtime.Exception` でなければ `{"error": "<err.Error()>"}`。

## 使い方の表示

```
usage: hozoc <command> [flags]

commands:
  check   仕様を検査する。ファイルは書き出さない
  emit    仕様を検査し、名前空間ごとに設定されたプラグインを呼んで、生成したファイルを書き出す
```

- コマンドの一行は `Op.description.summary`。コマンドごとの使い方は、フィールドの `description.summary` を `--flag` / `ARG` / `ENV` と並べる。
- 使い方に出すプログラム名は `filepath.Base(os.Args[0])`。`Main` の `args` は `os.Args[1:]` の約束で、プログラム名を含まない。

## Go の名前

N のパッケージの型・メソッド名は `docs/emit-go.md`「名前」の規則で決める。プラグインが同じ規則を使えるよう、規則の実装は公開パッケージ `plugin/golang`（`docs/packages.md`）に置き、`internal/emit/golang` もそれを使う。

## 診断

| ID | 意味 |
|---|---|
| CLI-1 | `@cli.command` の op の引数が、0 個でも model 一つでもない |
| CLI-2 | 入力の model のフィールドに `cli` のデコレータがない |
| CLI-3 | フィールドの型が受け付けられない |
| CLI-4 | `array` の `@cli.arg` が最後でない、または二つ以上 |
| CLI-5 | フラグ名かコマンド名の重複 |
