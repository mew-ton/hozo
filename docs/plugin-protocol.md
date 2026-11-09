# プラグインのプロトコル

## この文書が決めること

プラグインのマニフェスト、探し方、起動、標準入出力の JSON の形。設計書 §4.6 を手続きにしたもの。hozoc 側は `internal/plugin`、プラグイン側の SDK は `plugin`（Go）と `sdk/ts/`（`docs/packages.md`）。§4.6 の形との違い（要求の `ir` を `view` にし、`protocol` を足したこと、マニフェストの追加項目）は `docs/decisions.md`。

## マニフェスト `hozo-plugin.toml`

```toml
name     = "react-router"       # 例。go-cli のマニフェストは docs/plugin-go-cli.md「マニフェスト」
command  = ["node", "dist/main.js"]   # 文字列か文字列の配列
language = "ts"
protocol = "0.1"                # 対応する docs/ir.md の version

vocabulary = ["vocab/page.hozo"]   # このプラグインが提供する語彙。マニフェストからの相対パス。省略時は []

[emit]
decorate    = []                # 包むデコレータの FQN
entry       = ["page.route", "page.interaction"]   # 起点にするデコレータの FQN
stop        = ["http.*"]        # 打ち切るデコレータの FQN。末尾 * で名前空間全体
unsupported = []                # 意味を保てない記法（デコレータの FQN）。使われていれば CFG-6

[check]
enabled = true                  # true なら hozoc check / emit で direction "check" を呼ぶ

[import]
formats = []                    # 取り込める形式の名前。空なら import を持たない
```

- `command` が文字列なら実行ファイルの名前かパスで、引数なし。配列なら先頭が実行ファイル。実行ファイルにパスの区切りが含まれていればマニフェストのディレクトリからの相対パス、含まれていなければ `PATH` から探す。
- `[emit]` がなければ `emit` を持たない。`[emit]` があれば `entry` か `decorate` の少なくとも一方が空でないこと（MAN-1）。
- `protocol` が hozoc の `docs/ir.md` の `version` と違えば MAN-2。
- `vocabulary` の `.hozo` の読み込みは `docs/vocab.md`「読み込み」。

## 探索

Conf(N).plugins（`docs/config.md`「名前空間の設定の解決」）の名前 p について、次の順で `hozo-plugin.toml` を探し、最初に見つかったものを使う。プロジェクトの中、利用者の環境、ホームの順に、近いものを優先する。

1. `hozo.toml` の `[hozo].plugin_dirs` の各ディレクトリ d について `<ルート>/<d>/<p>/hozo-plugin.toml`。
2. 環境変数 `HOZO_PLUGIN_PATH`（OS のパス区切りで複数）の各ディレクトリ d について `<d>/<p>/hozo-plugin.toml`。
3. `<ホーム>/.hozo/plugins/<p>/hozo-plugin.toml`。

見つからなければ CFG-9。マニフェストの `name` が p と違えば MAN-3。

## 起動

- 作業ディレクトリはプロジェクトのルート。環境変数は hozoc のものに `HOZO_PROJECT_ROOT`（ルートの絶対パス）と `HOZO_PLUGIN_DIR`（マニフェストのディレクトリの絶対パス）を足す。
- 標準入力に要求の JSON を一つ書いて閉じる。標準出力から応答の JSON を一つ読む。標準エラーは hozoc の標準エラーに、各行の先頭に `[<プラグイン名>] ` を付けて流す。
- 一つのプラグインは名前空間ごと、`direction` ごとに一回起動する。並列には起動しない（出力の順序を決定的にするため）。

終了コードと応答の読み方:

| 終了コード | 標準出力 | hozoc の扱い |
|---|---|---|
| 0 | 応答の JSON | 成功 |
| 0 | JSON として読めない、または形が違う | `proc.MalformedOutput` |
| 2 | `{ "error": { "code": "protocol", "message": "..." } }` | MAN-2 |
| 2 | `{ "error": { "code": "unsupported", "message": "..." } }` | MAN-5 |
| 2 | それ以外 | `proc.MalformedOutput` |
| それ以外 | — | `proc.Crashed` |

## 要求と応答

すべての要求は `protocol`（`docs/ir.md` の `version` と同じ値。`view.version` とも等しい）と `direction` を持つ。プラグインは `protocol` が合わなければ `code: "protocol"`、`direction` を実装していなければ `code: "unsupported"` の `error` を返して終了コード 2 で終わる。

### emit

```jsonc
// 要求
{ "protocol": "0.1", "direction": "emit", "view": { /* docs/ir.md の View。対象の名前空間は view.viewer */ } }

// 応答
{
  "files": [ { "path": "internal/gen/hozoc/cli/cli.go", "content": "..." } ],   // path はプロジェクトのルートからの相対パス（普通は view.config.dir の下）
  "warnings": [ { "at": "hozoc.emit", "message": "..." } ],  // at は FQN か、"file:line:col" の文字列
  "errors":   [ { "code": "CLI-1", "at": "hozoc.check", "message": "..." } ]   // 省略可。一つでもあれば書き出さない
}
```

- `path` に `..` や絶対パスがあれば PLG-1。同じ応答の中で同じ `path` が二度あれば PLG-2。どちらも `proc.MalformedOutput` として終了する。
- `content` は UTF-8 の文字列。バイナリは出せない。
- `files` と `warnings`（`check` の `diagnostics` も）は空でも `[]` で出す。省けるのは `errors` だけ。
- `warnings` は warning の診断（コード `PLUGIN`、メッセージの先頭にプラグイン名）。
- `errors` は、プラグインがその入力を変換できない理由（語彙の使い方の誤り。`docs/plugin-go-cli.md`「診断」の CLI-* など）。error の診断として報告し、`CheckFailed` になる（`docs/diagnostics.md`「終了コード」）。

### check

```jsonc
{ "protocol": "0.1", "direction": "check", "view": { /* View */ } }

{ "diagnostics": [ { "severity": "error", "code": "PAGE-1", "message": "...", "at": "browser.openOrders" } ] }
```

- `severity` は `"error"` か `"warning"`。`code` はプラグインが決める（語彙に固有の規則の ID。`docs/requirements.md`「検査の規則の一覧」の PAGE-1 / PAGE-2 など）。
- `at` は FQN（その宣言の位置に付ける）か `"file:line:col"`。どちらでもなければ位置なしで報告する。
- `check` の診断のメッセージにはプラグイン名を付けない（`code` がプラグインの規則を示す）。`warnings` には付ける。

### import

```jsonc
{ "protocol": "0.1", "direction": "import", "format": "openapi", "files": [ { "path": "openapi.yaml", "content": "..." } ] }

{ "ir": { /* docs/ir.md の Program */ }, "warnings": [ /* 同上 */ ] }
```

`hozoc import` の詳細は段階 6（`docs/requirements.md`「段階」）で決める。形だけをここに置く。

## entry / stop / decorate の意味

hozoc は `View` を渡し、何を変換するかはプラグインが決める（§4.3）。hozoc が `[emit]` の項目を使うのは、`unsupported` の warning（CFG-6）、マニフェストの整合（MAN-1）、既定の出力で包む関数を作るかどうかの判定（`docs/emit-go.md`「デコレータ」）。プラグインの側の約束:

- `entry` のデコレータが付いた宣言を起点に、配線の呼び出しを辿って出力する。
- `stop` のデコレータが付いた op に着いたら、そこから先は辿らず、既定の出力のインターフェース（`docs/emit-go.md`「op」）で呼ぶ。
- `decorate` のデコレータが付いた op について、既定の出力が生成する「包む関数」（`docs/emit-go.md`「デコレータ」の `Wrappers` のメソッド）の実装を出力する。

## 診断

| ID | 重さ | 意味 |
|---|---|---|
| MAN-1 | error | `[emit]` があるのに `entry` も `decorate` も空 |
| MAN-2 | error | `protocol` が合わない |
| MAN-3 | error | マニフェストの `name` がディレクトリ名と違う |
| MAN-4 | error | マニフェストの TOML の誤り、知らないキー、`command` が空 |
| MAN-5 | error | プラグインがその `direction` を実装していない（マニフェストに書いてあるのに） |
| PLG-1 | error | 応答の `path` がルートの外に出る |
| PLG-2 | error | 応答の中で同じ `path` が二度 |
| PLUGIN | warning | プラグインの `warnings` |

MAN-* は `ConfigInvalid`、PLG-* は `proc.MalformedOutput`（`docs/diagnostics.md`「終了コード」）。

## SDK

Go は `plugin` パッケージ、TS は `sdk/ts/`。形は `docs/packages.md`「`plugin`（公開）」。
