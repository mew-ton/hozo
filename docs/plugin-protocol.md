# プラグインのプロトコル

## この文書が決めること

プラグインのマニフェスト、探し方、起動、標準入出力の JSON の形。設計書 §4.6 を手続きにしたもの。hozoc 側は `internal/plugin`、プラグイン側の SDK は `plugin`（Go）と `sdk/ts/`（`docs/packages.md`）。§4.6 の形との違い（要求の `ir` を `view` にし、`protocol` を足したこと、マニフェストの追加項目、プラグインとトランスパイラを分けたこと）は `docs/decisions.md`。

**プラグイン**は入れるもの。マニフェストと実行ファイルと、あれば語彙を一つのディレクトリに持つ。**トランスパイラ**は、プラグインが提供する名前の付いた変換で、`hozo.toml` の単位が `transpilers` に名前を書いて選ぶ（`docs/config.md`「単位」）。二つの言葉の定義の持ち主はこの段落で、ほかの文書も同じ意味で使う: 起動される実行ファイルと、マニフェスト、語彙、`check` の話は「プラグイン」、設定が選ぶもの、出力の対象を選ぶもの、ファイルを出すものの話は「トランスパイラ」。

## マニフェスト `hozo-plugin.toml`

```toml
name     = "react-router"       # 例。プラグインの名前。go-cli のマニフェストは docs/plugin-go-cli.md「マニフェスト」
command  = ["node", "dist/main.js"]   # 文字列か文字列の配列
protocol = "0.2"                # 対応する docs/ir.md の version

vocabulary = ["vocab/page.hozo"]   # このプラグインが提供する語彙。マニフェストからの相対パス。省略時は []

[[transpiler]]                  # このプラグインが提供するトランスパイラ。いくつでも書ける
name        = "react-router"    # hozo.toml の transpilers に書く名前。必須
language    = "ts"              # 出力する言語。必須。"go" | "ts"
unsupported = []                # 意味を保てない記法（デコレータの FQN）。使われていれば CFG-6。省略時は []

[check]
enabled = true                  # true なら hozoc check / emit で direction "check" を呼ぶ。テーブルごと省けば false

[import]
formats = []                    # 取り込める形式の名前。空なら import を持たない。テーブルごと省けば []
```

- `command` が文字列なら実行ファイルの名前かパスで、引数なし。配列なら先頭が実行ファイル。実行ファイルにパスの区切りが含まれていればマニフェストのディレクトリからの相対パス、含まれていなければ `PATH` から探す。
- `[[transpiler]]` が一つもなければ `emit` を持たない。トランスパイラの名前はプラグインの名前と同じでも違ってもよい。一つのマニフェストの中で同じ名前を二度書けば MAN-4。`language` が `"go"` でも `"ts"` でもなければ MAN-4。
- 言語を持つのはトランスパイラで、プラグインは持たない（一つのプラグインが言語の違うトランスパイラを提供してよい）。単位の言語の決め方は `docs/config.md`「単位」。
- 何を出力の対象に選ぶかはマニフェストには書かず、トランスパイラが `emit` の応答で返す（「選択」）。
- `protocol` が hozoc の `docs/ir.md` の `version` と違えば MAN-2（省いたときも、違うものとして MAN-2）。
- 省いたときの値: `vocabulary` は `[]`、`[[transpiler]]` はなし、`unsupported` は `[]`、`[check]` は `enabled = false`（`check` を呼ばない）、`[import]` は `formats = []`（`import` を持たない）。`name` と `command` は省けない（MAN-4）。
- `vocabulary` の `.hozo` の読み込みは `docs/vocab.md`「読み込み」。

## 探索

**入っているプラグイン**は、次のディレクトリ d の直下で `<d>/<p>/hozo-plugin.toml` を持つディレクトリ p のすべて。p がプラグインの名前。同じ名前 p が複数の場所にあれば、次の順で最初に見つかったものだけを使う。プロジェクトの中、利用者の環境、ホームの順に、近いものを優先する。

1. `hozo.toml` の `[hozo].plugin_dirs` の各ディレクトリ（ルートからの相対パス）。
2. 環境変数 `HOZO_PLUGIN_PATH`（OS のパス区切りで複数）の各ディレクトリ。
3. `<ホーム>/.hozo/plugins`。

- 2 と 3 はプロジェクトの外の場所で、探索（`internal/plugin`）と解析（`internal/project`）は自分ではプロセスの環境を読まない。`hozoc` が、環境変数を `cli.hozo` の入力 `pluginPath` として（`docs/cli.md`「cli.hozo」）、ホームディレクトリを `cmd/hozoc` から受け取って渡す。渡されなければ 1 だけを探す。同じ入力から同じ結果にするためで、テストは実行する人の環境に入っているプラグインに左右されない（`docs/testing.md`「環境からの独立」）。
- 単位の `transpilers` の名前 t（`docs/config.md`「単位」）からトランスパイラを探す手続きの持ち主はここ: 入っているすべてのプラグインのマニフェストの `[[transpiler]]` の `name` から探す。どのプラグインにもなければ CFG-14。二つ以上のプラグインにあれば CFG-15（以降の処理は、上の順で最初のプラグインのものを使う）。
- **使うプラグイン**は、どれかの単位の `transpilers` に書かれたトランスパイラを提供するプラグイン。語彙を読むのも（`docs/vocab.md`「読み込み」）、`check` を呼ぶのも（「起動」）、使うプラグインだけ。入っているだけのプラグインは、名前を探す相手になる以外は何もしない。
- マニフェストは入っているプラグインのすべてについて読む（トランスパイラの名前を知るため）。読めないマニフェスト（MAN-4）と、`name` がディレクトリの名前 p と違うマニフェスト（MAN-3）は、使っていなくても報告する。`protocol` の不一致（MAN-2）は、使うプラグインにだけ報告する（ホームに入れた古いプラグインが、それを使わないプロジェクトを止めないようにするため）。
- 設定なし（`docs/config.md`「置き場と探し方」）のときは探索しない（選ぶ単位がない）。

## 起動

- 作業ディレクトリはプロジェクトのルート。環境変数は hozoc のものに `HOZO_PROJECT_ROOT`（ルートの絶対パス）と `HOZO_PLUGIN_DIR`（マニフェストのディレクトリの絶対パス）を足す。
- 標準入力に要求の JSON を一つ書いて閉じる。標準出力から応答の JSON を一つ読む。標準エラーは hozoc の標準エラーに、各行の先頭に `[<プラグイン名>] ` を付けて流す。
- `emit` は、単位と、その単位が出力する名前空間（`docs/config.md`「単位の対象」「in」）と、その単位のトランスパイラの組ごとに一回起動する（順序は、単位の設定の順、名前空間の IR の順、`transpilers` に書いた順）。一つのプラグインの二つのトランスパイラを同じ単位に書けば、そのプラグインを二回起動する。
- `check` は、使うプラグイン（「探索」）のうち `[check] enabled` のものを、プラグインと名前空間の組ごとに一回起動する。相手の名前空間は、そのプラグインのトランスパイラを書いた単位が出力するもの。渡す `view` は、その単位のうち設定の順で最初のもの。
- 並列には起動しない（出力の順序を決定的にするため）。

終了コードと応答の読み方:

| 終了コード | 標準出力 | hozoc の扱い |
|---|---|---|
| 0 | 応答の JSON | 成功 |
| 0 | JSON として読めない、または形が違う（「応答の形」） | `proc.MalformedOutput` |
| 2 | `{ "error": { "code": "protocol", "message": "..." } }` | MAN-2 |
| 2 | `{ "error": { "code": "unsupported", "message": "..." } }` | MAN-5 |
| 2 | それ以外 | `proc.MalformedOutput` |
| それ以外 | — | `proc.Crashed` |

`command` が `go run` のとき（開発中のマニフェスト）、`go run` はプログラムの終了コードを伝えず 1 で終わるので、終了コード 2 の応答も `proc.Crashed` として報告される。配布するマニフェストはビルドした実行ファイルを `command` に書く（`docs/plugin-go-cli.md`「マニフェスト」）。

## 信頼の前提

プラグインは利用者が入れ、`hozo.toml` にそのトランスパイラの名前を書いて選んだコードで、hozoc と同じユーザーの権限で動く子プロセス。protoc のプラグインと同じ信頼モデルで、サンドボックスも、応答の `files` 以外のファイルの変化の検出も行わない。hozoc が確かめるのは応答だけ（「応答の形」、PLG-1〜PLG-3、CFG-5）。IR の取得と出力の書き込みを標準入出力に限るのは、プラグインを単純に書けるようにするための約束で、強制ではない。

## 要求と応答

すべての要求は `protocol`（`docs/ir.md` の `version` と同じ値。`view.version` とも等しい）と `direction` を持つ。プラグインは `protocol` が合わなければ `code: "protocol"`、`direction` を実装していなければ（`emit` なら、要求の `transpiler` を実装していなければ）`code: "unsupported"` の `error` を返して終了コード 2 で終わる。`protocol` を先に確かめる。「合わない」は、プラグインが話す版と同じ文字列でないことで、`protocol` がない、`null`、文字列でない場合を含む。文字列でない `direction` は、実装していない `direction` と同じ。要求が JSON のオブジェクトとして読めなければ、応答を書かずに終了コード 1 で終わる（hozoc からは `proc.Crashed`）。二つの SDK はこの読み方を同じ表（`plugin/testdata/request_head.json`）でテストする。

### 応答の形

終了コード 0 の標準出力は、JSON の値一つで、下の各 direction の応答の形をしていなければならない。hozoc は次を確かめ、違えば `proc.MalformedOutput`（「起動」）。

- 応答は JSON のオブジェクト。
- **必須の項目**がある。`null` は「ない」と同じに扱う。`emit` の応答の `files` と `warnings`、`check` の応答の `diagnostics`（どれも、空でも `[]` で出す）。要素では、`files` の要素の `path` と `content`、`warnings` の要素の `message`、`errors` と `diagnostics` の要素の `code` と `message`、`diagnostics` の要素の `severity`。
- **省ける項目**は、なくても `null` でもよい。`emit` の応答の `errors` と `select`、`select` の `roots` / `stops` / `states` / `wraps`（省けば空。SDK は、何も選ばなくても四つを `[]` にして書く）、`warnings` / `errors` / `diagnostics` の要素の `at`（省けば位置なし）。`errors` の要素の `severity` は見ない（常に error）。
- 書かれた項目の**型**が合う: 上の配列は配列、その要素はオブジェクト、`path` / `content` / `message` / `code` / `at` と `select` の `roots` / `stops` / `states` の要素、`wraps` の要素の `op` と `decorator`（どちらも必須）は文字列。`diagnostics` の `severity` は `"error"` か `"warning"`。
- **知らない項目は無視する**。応答のどの階層でも、ここにない名前の項目があってよい（プラグインが新しい版の項目を書いていても、古い hozoc はそれを読まないだけ）。項目の名前は大文字と小文字を区別する。

形が合った後に、`path`（PLG-1 / PLG-2）と `select` の中身（PLG-3）を確かめる。`import` の応答の形は、段階 6 で `hozoc import` と一緒に決める。

### emit

```jsonc
// 要求
{ "protocol": "0.2", "direction": "emit",
  "transpiler": "go-cli",                 // 起動するトランスパイラの名前（マニフェストの [[transpiler]] の name）
  "view": { /* docs/ir.md の View。対象の名前空間は view.viewer、単位の設定は view.config */ } }

// 応答
{
  "files": [ { "path": "internal/gen/hozoc/cli/cli.go", "content": "..." } ],   // path はプロジェクトのルートからの相対パスで、view.config.dir（Dir(U, N)）の下
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

- `transpiler` は、単位の `transpilers` に書かれた名前のうち、このプラグインが提供する一つ。複数のトランスパイラを提供するプラグインは、この名前で処理を分ける。
- `path` は、その起動の出力 (U, N) の出力先 Dir(U, N)（要求の `view.config.dir`。`docs/config.md`「単位の対象」）の下のファイルでなければならない。そうでなければ PLG-1: 絶対パス、`..` でルートや Dir(U, N) の外へ出るパス、ルートの中の別のディレクトリ、別の出力の Dir、Dir(U, N) そのもの。比べるのは `./` や `..` を畳んだパス。出力先ごとの記録（`docs/cli.md`「emit」の `.hozoc-outputs.json`）と、出力先の持ち主を一つに決める規則（CFG-17）が、どの出力のファイルもその Dir の下にあることを前提にしているため。同じ応答の中で同じ `path` が二度あれば PLG-2。どちらも `proc.MalformedOutput` として終了する。
- `content` は UTF-8 の文字列。バイナリは出せない。
- `files` と `warnings` は空でも `[]` で出す。省けるのは `errors` と `select`（「応答の形」。`select` を省いたときの意味は「選択」）。
- `warnings` は warning の診断（コード `PLUGIN`、メッセージの先頭にトランスパイラの名前）。
- `errors` は、トランスパイラがその入力を変換できない理由（語彙の使い方の誤り。`docs/plugin-go-cli.md`「診断」の CLI-* など）。error の診断として報告し、`CheckFailed` になる（`docs/diagnostics.md`「終了コード」）。

### check

```jsonc
{ "protocol": "0.2", "direction": "check", "view": { /* View */ } }

{ "diagnostics": [ { "severity": "error", "code": "PAGE-1", "message": "...", "at": "browser.openOrders" } ] }
```

- `severity` は `"error"` か `"warning"`。`code` はプラグインが決める（語彙に固有の規則の ID。`docs/requirements.md`「検査の規則の一覧」の PAGE-1 / PAGE-2 など）。
- `at` は FQN（その宣言の位置に付ける）か `"file:line:col"`。どちらでもなければ（省いたときも）位置なしで報告する。
- `check` の診断のメッセージには名前を付けない（`code` がプラグインの規則を示す）。`emit` の `warnings` にはトランスパイラの名前を付ける。
- `check` はトランスパイラごとではなくプラグインごとの処理で、要求は `transpiler` を持たない。

### import

```jsonc
{ "protocol": "0.2", "direction": "import", "format": "openapi", "files": [ { "path": "openapi.yaml", "content": "..." } ] }

{ "ir": { /* docs/ir.md の Program */ }, "warnings": [ /* 同上 */ ] }
```

`hozoc import` の詳細は段階 6（`docs/requirements.md`「段階」）で決める。形だけをここに置く。

## 選択

何を出力の対象にするかは、トランスパイラが `View` を見て自由に決め、`emit` の応答の `select` で返す。どのデコレータを読むか、デコレータを読まずに構造から決めるか（たとえば「利用者の操作を表す op から呼ばれている op」を起点にする）は、トランスパイラの判断。起点にも、途中の op にも、そのプラグインの語彙のデコレータが付いている必要はない。単位のどのトランスパイラも選ばなかった宣言はその単位の出力に現れず、どのトランスパイラも読まないデコレータは印で、何も生成しない。

```jsonc
"select": {                               // view.viewer が browser のときの例
  "roots":  ["browser.onAddToCart"],      // 起点にする op
  "stops":  ["browser.askYamadaWarranty"],// 辿られたときに、本体を辿らず口だけを出す op。起点ではない
  "states": ["browser.token"],            // 起点にする状態（保存先のトランスパイラなど）
  "wraps":  [ { "op": "browser.fetchMe", "decorator": "cache" } ]   // このトランスパイラが包む関数を実装するデコレータの適用
}
```

- `roots` と `stops` は op、`states` は状態、`wraps` の `op` は op の FQN。`states` に書けるのは、基底の状態と、状態の分類（状態を対象にした `classify`。`docs/check.md`「状態の形」）。値の分類は書けない。
- `stops` に書いただけの op は選ばれない。どの起点からも辿られなければ出力に現れず、辿られたときに限って、本体を辿らない op として選ばれる（`docs/emit-go.md`「出力の対象」の 2 が、辿り方の持ち主）。
- `wraps` の `decorator` は、その op に付いたデコレータの適用を指す FQN。汎用のデコレータの FQN（`Application.decorator`）でも、名前付きのデコレータを通して付けたならその名前の FQN（`Application.via`。`docs/ir.md`「デコレータの適用」）でもよく、どちらかがその op の適用と一致すればよい。
- hozoc は、単位のすべてのトランスパイラの `select` を名前空間ごとに合わせ、`roots` から配線の呼び出しを辿って出力の対象を決め、トランスパイラどうしが共有する形（`docs/emit-go.md`）で、単位と名前空間の組ごとに一度だけ出力する。辿り方と、そこから決まる状態・分類・型は `docs/emit-go.md`「出力の対象」。
- 同じ単位のトランスパイラは互いを知らない。つながるのは共有する形を通してで、呼び出しはモデルの向きに流れる（あるトランスパイラが出したハンドラが配線を呼び、配線が、別のトランスパイラが実装を入れた状態の命令を呼ぶ）。別の単位のトランスパイラの `select` は合わせない。
- 選べるのは、単位の対象の宣言（`docs/config.md`「in」）のうち `view.viewer` の名前空間のものだけ。`View` の中でそれを見分けるのは `view.files`（`docs/ir.md`「その名前空間から見た IR」）。`roots` / `stops` / `states` / `wraps` のどれも同じ。
- 名前空間をまたぐ呼び出しは辿らない（相手は `Deps` の口になる）。API を呼ぶ直前までがその名前空間のコード、という境界は `stops` に書かなくても成り立つ。`in` の外で宣言された op も同じで、辿られたら口だけが出る（`docs/emit-go.md`「出力の対象」）。
- トランスパイラ自身のファイル（`files`）は、選んだ宣言が共有する形で出力されることを前提に書いてよい。
- `select` を省いたら、何も選ばない（そのトランスパイラのファイルだけが出力される）。
- `view.viewer` の名前空間にない FQN、存在しない FQN、種類の違う宣言（`roots` / `stops` / `wraps` の `op` に op でないもの、`states` に状態でないもの）、単位の対象でない宣言（`in` の外で宣言されたもの）、`wraps` がその op に付いていないデコレータを指していたら PLG-3。

設計書 §4.6 のマニフェストの `decorate` / `entry` / `stop` は持たない。同じ選び方を静的な一覧と応答の二通りに書けないようにするため。デコレータで起点を選ぶ単純なトランスパイラのために、SDK が「このデコレータが付いた、単位の対象の op」を集める関数を提供する（`docs/packages.md`「`plugin`（公開）」）。

## 診断

| ID | 重さ | 意味 |
|---|---|---|
| MAN-2 | error | `protocol` が合わない（マニフェストの `protocol` が違う。または起動したプラグインが `code: "protocol"` で断った） |
| MAN-3 | error | マニフェストの `name` がディレクトリ名と違う |
| MAN-4 | error | マニフェストを読めない: ファイルを開けない、TOML の構文の誤り、値の型違い、知らないキー、`name` がない、`command` がない・空・文字列でも文字列の配列でもない、`[[transpiler]]` に `name` か `language` がない、`language` が `"go"` でも `"ts"` でもない、同じ名前の `[[transpiler]]` が二つある。使うプラグインの `vocabulary` のファイルを読めないときも MAN-4 |
| MAN-5 | error | プラグインがその `direction` か、要求の `transpiler` を実装していない（マニフェストに書いてあるのに。起動したプラグインが `code: "unsupported"` で断った） |
| PLG-1 | — | 応答の `files` の `path` が、その起動の Dir(U, N) の下にない（「emit」） |
| PLG-2 | — | 応答の中で同じ `path` が二度 |
| PLG-3 | — | 応答の `select` が、対象の名前空間にない宣言、種類の違う宣言、単位の対象でない宣言、付いていないデコレータを指している（「選択」） |
| PLUGIN | warning | トランスパイラの `warnings` |

- MAN-1 は欠番。マニフェストの `[emit]` の `entry` も `decorate` も空であることを報告していた規則で、`[emit]` を `[[transpiler]]` に替え、出力の対象を応答の `select` で返す形にしたときになくなった（番号を使い回さないのは `docs/config.md`「診断」と同じ）。
- MAN-* は診断で、例外は `ConfigInvalid`。
- PLG-* は診断にならない（重さが「—」）。応答そのものが信用できないので、診断の一覧に足して続けることはせず、例外 `proc.MalformedOutput` で終わる。ID は、その例外のメッセージの頭に書く（`プラグイン fake の出力が約束した形でない: PLG-1: …`）。`--format json` でも `diagnostics` には現れず、`{"error": "proc.MalformedOutput"}` になる（`docs/plugin-go-cli.md`「実行と表示」の `JSONPresenter`）。「応答の形」の誤りも同じ例外で、ID は持たない。
- 終了コードは `docs/diagnostics.md`「終了コード」。

## SDK

Go は `plugin` パッケージ、TS は `sdk/ts/`。形は `docs/packages.md`「`plugin`（公開）」。
