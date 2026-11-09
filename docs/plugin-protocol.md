# プラグインのプロトコル

## この文書が決めること

プラグインのマニフェスト、探し方、起動、標準入出力の JSON の形。設計書 §4.6 を手続きにしたもの。hozoc 側は `internal/plugin`、プラグイン側の SDK は `plugin`（Go）と `sdk/ts/`。

## マニフェスト `hozo-plugin.toml`

```toml
name     = "go-cli"
command  = ["go", "run", "github.com/mew-ton/hozo/plugins/go-cli"]   # 文字列か文字列の配列
language = "go"
protocol = "0.1"                # 対応する docs/ir.md の version

vocabulary = ["cli.hozo"]       # このプラグインが提供する語彙。マニフェストからの相対パス。省略時は []

[emit]
decorate    = []                # 包むデコレータの FQN
entry       = ["cli.command"]   # 起点にするデコレータの FQN
stop        = []                # 打ち切るデコレータの FQN。"http.*" のように末尾 * で名前空間全体
unsupported = []                # 意味を保てない記法（デコレータの FQN）。使われていれば CFG-6

[check]
enabled = false                 # true なら hozoc check / emit で direction "check" を呼ぶ

[import]
formats = []                    # 取り込める形式の名前。空なら import を持たない
```

- `command` が文字列なら実行ファイルの名前かパスで、引数なし。配列なら先頭が実行ファイル。実行ファイルにパスの区切りが含まれていればマニフェストのディレクトリからの相対パス、含まれていなければ `PATH` から探す。
- `[emit]` がなければ `emit` を持たない。`[emit]` があれば `entry` か `decorate` の少なくとも一方が空でないこと（MAN-1）。
- `protocol` が hozoc の `docs/ir.md` の `version` と違えば MAN-2（エラー）。
- `vocabulary` の `.hozo` は、そのプラグインがどこかの名前空間の `plugins` に書かれていれば読む（`docs/vocab.md`）。

## 探索

Conf(N).plugins の名前 `p` について、次の順で `hozo-plugin.toml` を探し、最初に見つかったものを使う。

1. `hozo.toml` の `[hozo].plugin_dirs` の各ディレクトリ d について `<ルート>/<d>/<p>/hozo-plugin.toml`。
2. 環境変数 `HOZO_PLUGIN_PATH`（OS のパス区切りで複数）の各ディレクトリ d について `<d>/<p>/hozo-plugin.toml`。
3. `<ホーム>/.hozo/plugins/<p>/hozo-plugin.toml`。

見つからなければ CFG-9。マニフェストの `name` が `p` と違えば MAN-3。

## 起動

- 作業ディレクトリはプロジェクトのルート。環境変数は hozoc のものに `HOZO_PROJECT_ROOT`（ルートの絶対パス）と `HOZO_PLUGIN_DIR`（マニフェストのディレクトリの絶対パス）を足す。
- 標準入力に要求の JSON を一つ書いて閉じる。標準出力から応答の JSON を一つ読む。標準エラーは hozoc の標準エラーに、各行の先頭に `[p] ` を付けて流す。
- 終了コード 0 で応答が読めれば成功。0 以外なら `proc.Crashed`（終了コード 70）。0 だが応答が JSON として読めない、または形が違えば `proc.MalformedOutput`（70）。
- 一つのプラグインは名前空間ごとに一回起動する（`direction` ごとに一回）。並列には起動しない（出力の順序を決定的にするため）。

## 要求と応答

すべての要求は `protocol` を持つ。プラグインは `protocol` が合わなければ `{ "error": { "message": "..." } }` を返して終了コード 2 で終わる。hozoc はそれを `MAN-2` として報告する。

### emit

```jsonc
// 要求
{ "protocol": "0.1", "direction": "emit", "namespace": "hozoc", "view": { /* docs/ir.md の View */ } }

// 応答
{
  "files": [ { "path": "cli/cli.go", "content": "..." } ],   // path は View.config.out からの相対パス
  "warnings": [ { "at": "hozoc.emit", "message": "..." } ],  // at は FQN か、"file:line:col" の文字列
  "errors":   [ { "code": "CLI-1", "at": "hozoc.check", "message": "..." } ]   // 省略可。一つでもあれば書き出さない
}
```

- `errors` は、プラグインがその入力を変換できない理由（語彙の使い方の誤り。`docs/plugin-go-cli.md` の CLI-*）。error の診断として報告し、終了コードは 1。

- `path` に `..` や絶対パスがあれば PLG-1（エラー）。同じ応答の中で同じ `path` が二度あれば PLG-2。
- `content` は UTF-8 の文字列。バイナリは出せない。
- `warnings` は CFG-6 と同じ warning として報告する（コード `PLUGIN`、メッセージの先頭にプラグイン名）。

### check

```jsonc
{ "protocol": "0.1", "direction": "check", "namespace": "browser", "view": { /* View */ } }

{ "diagnostics": [ { "severity": "error", "code": "PAGE-1", "message": "...", "at": "browser.openOrders" } ] }
```

- `severity` は `"error"` か `"warning"`。`code` はプラグインが決める（語彙に固有の規則の ID。`docs/requirements.md` の PAGE-1 / PAGE-2 など）。
- `at` は FQN（その宣言の位置に付ける）か `"file:line:col"`。

### import

```jsonc
{ "protocol": "0.1", "direction": "import", "format": "openapi", "files": [ { "path": "openapi.yaml", "content": "..." } ] }

{ "ir": { /* docs/ir.md の Program */ }, "warnings": [ /* 同上 */ ] }
```

`hozoc import` の詳細は段階 6（`docs/requirements.md`「段階」）で決める。形だけをここに置く。

## entry / stop / decorate の意味

hozoc は `View` に `Program` 全体を渡し、何を変換するかはプラグインが決める（§4.3）。hozoc が `[emit]` の項目を使うのは、`unsupported` の warning（CFG-6）と、マニフェストの整合（MAN-1）だけ。プラグインの側の約束:

- `entry` のデコレータが付いた宣言を起点に、配線の呼び出しを辿って出力する。
- `stop` のデコレータが付いた op に着いたら、そこから先は辿らず、既定の出力のインターフェース（`docs/emit-go.md`「op」）で呼ぶ。
- `decorate` のデコレータが付いた op は、既定の出力が呼ぶ「包む関数」（`docs/emit-go.md`「デコレータ」）の実装を出力する。

## 診断

| ID | 重さ | 意味 |
|---|---|---|
| MAN-1 | error | `[emit]` があるのに `entry` も `decorate` も空 |
| MAN-2 | error | `protocol` が合わない |
| MAN-3 | error | マニフェストの `name` がディレクトリ名と違う |
| MAN-4 | error | マニフェストの TOML の誤り、知らないキー、`command` が空 |
| PLG-1 | error | 応答の `path` がルートの外に出る |
| PLG-2 | error | 応答の中で同じ `path` が二度 |
| PLUGIN | warning | プラグインの `warnings` |

## SDK

Go: `plugin` パッケージ。`plugin.Main(h Handler)` が標準入出力の読み書きと `protocol` の照合を行い、`Handler` の `Emit(view *ir.View) (*EmitResponse, error)` / `Check(...)` / `Import(...)` を呼ぶ。`docs/packages.md`。

TS: `sdk/ts/`。同じ形。IR の型は `schema/ir.schema.json` から生成する。
