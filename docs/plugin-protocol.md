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
unsupported = []                # 意味を保てない記法（デコレータの FQN）。使われていれば CFG-6

[check]
enabled = true                  # true なら hozoc check / emit で direction "check" を呼ぶ

[import]
formats = []                    # 取り込める形式の名前。空なら import を持たない
```

- `command` が文字列なら実行ファイルの名前かパスで、引数なし。配列なら先頭が実行ファイル。実行ファイルにパスの区切りが含まれていればマニフェストのディレクトリからの相対パス、含まれていなければ `PATH` から探す。
- `[emit]` がなければ `emit` を持たない。何を出力の対象に選ぶかはマニフェストには書かず、プラグインが `emit` の応答で返す（「選択」）。
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

`command` が `go run` のとき（開発中のマニフェスト）、`go run` はプログラムの終了コードを伝えず 1 で終わるので、終了コード 2 の応答も `proc.Crashed` として報告される。配布するマニフェストはビルドした実行ファイルを `command` に書く（`docs/plugin-go-cli.md`「マニフェスト」）。

## 信頼の前提

プラグインは利用者が `hozo.toml` に名前を書いて選んだコードで、hozoc と同じユーザーの権限で動く子プロセス。protoc のプラグインと同じ信頼モデルで、サンドボックスも、応答の `files` 以外のファイルの変化の検出も行わない。hozoc が確かめるのは応答の形だけ（PLG-1 / PLG-2、CFG-5）。IR の取得と出力の書き込みを標準入出力に限るのは、プラグインを単純に書けるようにするための約束で、強制ではない。

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
  "errors":   [ { "code": "CLI-1", "at": "hozoc.check", "message": "..." } ],  // 省略可。一つでもあれば書き出さない
  "select": {                                                                    // 「選択」
    "roots":  ["hozoc.check", "hozoc.emit", "hozoc.lsp"],
    "stops":  [],
    "states": [],
    "wraps":  []
  }
}
```

- `path` に `..` や絶対パスがあれば PLG-1。同じ応答の中で同じ `path` が二度あれば PLG-2。どちらも `proc.MalformedOutput` として終了する。
- `content` は UTF-8 の文字列。バイナリは出せない。
- `files` と `warnings`（`check` の `diagnostics` も）は空でも `[]` で出す。省けるのは `errors` と `select` だけ（`select` を省いたときの意味は「選択」。SDK は、何も選ばなくても四つの項目を `[]` にして書く）。
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

## 選択

何を出力の対象にするかは、プラグインが `View` を見て自由に決め、`emit` の応答の `select` で返す。どのデコレータを読むか、デコレータを読まずに構造から決めるか（たとえば「利用者の操作を表す op から呼ばれている op」を起点にする）は、プラグインの判断。起点にも、途中の op にも、そのプラグインの語彙のデコレータが付いている必要はない。プラグインが選ばなかった宣言は出力されず、どのプラグインも読まないデコレータは印で、何も生成しない。

```jsonc
"select": {
  "roots":  ["browser.onAddToCart"],      // 起点にする op（view.viewer の名前空間の op の FQN）
  "stops":  ["browser.askYamadaWarranty"],// 選ぶが、本体は辿らない op。実装を差し込む口だけを出す
  "states": ["server.users"],             // 状態を起点に選ぶ（保存先のプラグインなど）
  "wraps":  [ { "op": "browser.fetchMe", "decorator": "cache" } ]   // このプラグインが包む関数を実装するデコレータの適用
}
```

- hozoc は、名前空間に設定されたすべてのプラグインの `select` を合わせ、`roots` から配線の呼び出しを辿って出力の対象を決め、プラグインどうしが共有する形（`docs/emit-go.md`）で出力する。辿り方と、そこから決まる状態・分類・型は `docs/emit-go.md`「出力の対象」。
- 名前空間をまたぐ呼び出しは辿らない（相手は `Deps` の口になる）。API を呼ぶ直前までがその名前空間のコード、という境界は `stops` に書かなくても成り立つ。
- プラグイン自身のファイル（`files`）は、選んだ宣言が共有する形で出力されることを前提に書いてよい。
- `select` を省いたら、何も選ばない（そのプラグインのファイルだけが出力される）。
- `view.viewer` の名前空間にない FQN、存在しない FQN、`wraps` がその op に付いていないデコレータを指していたら PLG-3。

設計書 §4.6 のマニフェストの `decorate` / `entry` / `stop` は持たない。同じ選び方を静的な一覧と応答の二通りに書けないようにするため。デコレータで起点を選ぶ単純なプラグインのために、SDK が「このデコレータが付いた op」を集める関数を提供する（`docs/packages.md`「`plugin`（公開）」）。

## 診断

| ID | 重さ | 意味 |
|---|---|---|
| MAN-2 | error | `protocol` が合わない |
| MAN-3 | error | マニフェストの `name` がディレクトリ名と違う |
| MAN-4 | error | マニフェストの TOML の誤り、知らないキー、`command` が空 |
| MAN-5 | error | プラグインがその `direction` を実装していない（マニフェストに書いてあるのに） |
| PLG-1 | error | 応答の `path` がルートの外に出る |
| PLG-2 | error | 応答の中で同じ `path` が二度 |
| PLG-3 | error | 応答の `select` が、対象の名前空間にない宣言や、付いていないデコレータを指している |
| PLUGIN | warning | プラグインの `warnings` |

MAN-* は `ConfigInvalid`、PLG-* は `proc.MalformedOutput`（`docs/diagnostics.md`「終了コード」）。

## SDK

Go は `plugin` パッケージ、TS は `sdk/ts/`。形は `docs/packages.md`「`plugin`（公開）」。
