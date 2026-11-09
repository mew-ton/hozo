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
stop  = ["proc.stdio"]
```

- `vocabulary` は書かない（`cli` は標準の語彙。`docs/vocab.md`）。
- `stop` に `proc.stdio` を置くのは、プロセスの境界を越える op を辿らず既定のインターフェースで呼ぶため。これにより `docs/emit-go.md`「デコレータ」の規則で `@proc.stdio` の包む関数は生成されない。
- `command` が `go run` なのは、このリポジトリの中で `plugin_dirs = ["plugins"]` から使うため（作業ディレクトリがリポジトリのルートで、`go.mod` がある）。配布するときはビルドした実行ファイル `hozoc-plugin-go-cli` と、それを `command` に書いたマニフェストを `~/.hozo/plugins/go-cli/` に置く（`docs/plugin-protocol.md`「探索」）。

## 出力

`View.viewer` の名前空間 N について、`View.config.dir` の下に `cli/cli.go`（パッケージ `cli`）を一つ。`@cli.command` の op が一つもなければファイルを出さず warning を一つ返す。N のパッケージを import する。import パスは `plugin/golang.ImportPath`（`docs/packages.md`「`plugin/golang`（公開）」）で、ルートは環境変数 `HOZO_PROJECT_ROOT`（`docs/plugin-protocol.md`「起動」）、ディレクトリは `View.config.dir`。

```go
// Main は引数を解釈して ops の op を呼び、結果を p で表示し、終了コードを返す。
func Main(ops hozoc.Ops, p Presenter, args []string, env func(string) string, stdout, stderr io.Writer) int

// Presenter は結果と例外の表示。既定は JSON（JSONPresenter）。
type Presenter interface {
    Present(w io.Writer, command string, input any, result any) error
    PresentError(w io.Writer, command string, input any, f Failure) error
}

// Failure は例外の内容。Name は例外の FQN、Value は運ぶ値、Err は op が返した error そのもの（Presenter がメッセージに使える）。
// 生成した cli.go が N の例外の型を見分けて入れる。宣言にない例外なら Name は空で Err だけ
type Failure struct { Name string; Value any; Err error }

type JSONPresenter struct{}
```

- `Main` は `package main` ではない。人が書く `main.go` は、`Leaves` などの実装を組み立てて `os.Exit(cli.Main(ops, p, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))` を呼ぶだけ。
- `ops` の型は N の `Ops`（`docs/emit-go.md`「op」）。配線の op とデコレータの包み（`Wrappers`）を通すため、`Leaves` ではない。

## 対象

`View.viewer` の名前空間の op のうち、`@cli.command(name)` の付いたものがサブコマンド。

- 引数は 0 個か、model 一つ。それ以外は CLI-1。
- 引数の model の各フィールドは、`@cli.arg`、`@cli.flag(name)`、`@cli.env(name)` のどれかを持つ。`@cli.env` は `@cli.arg` か `@cli.flag` と一緒に付けられる（与えられなかったときの代わり）。`@cli.arg` と `@cli.flag` の両方、またはどれもなければ CLI-2。
- フィールドの型は `string`、`boolean`、`int`、`float`、元がこれらの `scalar`、またはそれらの `optional`、`@cli.arg` に限り `string`（と `string` の scalar）の `array`（`optional` は付けない。0 個を表せるため）。それ以外（model、`Record`、`Time`、`optional` の `array`）は CLI-3。
- `@cli.arg` の `array` のフィールドは、`@cli.arg` のフィールドの中で最後の一つだけ。違えば CLI-4。
- `@cli.arg` に `boolean` は付けられない（CLI-3）。
- 同じ `@cli.flag` の名前、同じ `@cli.command` の名前が二つあれば CLI-5。

CLI-* は emit の応答の `errors`（`docs/plugin-protocol.md`）。

## 引数の解釈

```
hozoc <command> [flags] [args]
```

- `<command>` がなければ使い方を標準エラーに出して 64。`help` か `--help` / `-h` なら使い方を標準出力に出して 0。`<command> --help` はそのコマンドの使い方。
- フラグは `--name value`、`--name=value`。`boolean` は `--name` で true（値は取らない。`--name=false` は取れる）。同じフラグが二度あれば後のものが勝つ。`-x` の形はフラグではなく位置引数（`-h` を除く。負の数を渡せるようにするため）。`--` 以降は位置引数。
- 知らないフラグ、フラグの値がない、値を型に変換できない（`int` に `"x"`）、必須のフラグがない、位置引数が足りない・多い → 使い方を標準エラーに出して 64。
- 必須: 型が `optional` でなく、`default` もないフィールド。
- `@cli.env` は、フラグが与えられなかったときに環境変数の値を使う（フラグが優先）。環境変数が空文字列なら「ない」とみなす。
- `@cli.arg` は `@cli.arg` のフィールドの宣言順に位置引数を当てる。`array` は残りすべて（0 個でもよい）。
- 値の変換: `int` は `strconv.ParseInt(v, 10, 64)`、`float` は `ParseFloat`、`boolean` は `ParseBool`。scalar は元の型で変換してから scalar の型にする。
- 解釈した入力の model で `Validate()`（`docs/emit-go.md`「model の Validate」。scalar のフィールドの制約も確かめる）を呼び、違反は 64（メッセージに `Violation` の `Path` と `Constraint`）。

## 実行と表示

1. `ops.<Op>(context.Background(), input)` を呼ぶ。
2. 成功なら `p.Present(stdout, command, input, result)` を呼び（void なら `result` は `nil`）、0。
3. 例外なら、`Signature.exceptions` の順に `errors.As` で例外の型を見分けて `Failure` を作り、終了コードを決めてから `p.PresentError(stderr, command, input, failure)` を呼ぶ。終了コードは、見分けた例外に `@cli.exitCode(code)` があればその `code`、なければ 70。見分けられなければ `Failure{Err: err}` で 70。
4. `Present` / `PresentError` 自体が失敗したら、そのエラーを標準エラーに出して 70。

`JSONPresenter`:

- `Present`: `result` を `json.MarshalIndent(result, "", "  ")` で標準出力に。`nil` なら何も出さない。
- `PresentError`: `Name` があれば `{"error": "<Name>", "value": <Value>}`（`Value` が `nil` なら `value` を省く）を、なければ `{"error": "<Err.Error()>"}` を標準エラーに。

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

N のパッケージの型・メソッド名は `plugin/golang`（`docs/packages.md`「`plugin/golang`（公開）」）で求める。

## 診断

| ID | 意味 |
|---|---|
| CLI-1 | `@cli.command` の op の引数が、0 個でも model 一つでもない |
| CLI-2 | 入力の model のフィールドに `cli` のデコレータがない |
| CLI-3 | フィールドの型が受け付けられない |
| CLI-4 | `array` の `@cli.arg` が最後でない、または二つ以上 |
| CLI-5 | フラグ名かコマンド名の重複 |
