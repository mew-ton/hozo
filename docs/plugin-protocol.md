# プラグインのプロトコル

## この文書が決めること

プラグインのマニフェスト、探し方、起動、標準入出力の JSON の形（出どころは設計書 §4.6）。hozoc 側は `internal/plugin`、プラグイン側の SDK は `plugin`（Go）と `sdk/ts/`（`docs/packages.md`）。この形にした決定（要求の `ir` を `view` にし、`protocol` を足したこと、マニフェストの追加項目、プラグインとトランスパイラを分けたこと、基底と相乗りに分けたこと）の一覧は `docs/decisions.md`「決めたこと」。

**プラグイン**は入れるもの。マニフェストと実行ファイルと、あれば語彙を一つのディレクトリに持つ。**トランスパイラ**は、プラグインが提供する名前の付いた変換で、`hozo.toml` の単位が `transpilers` に名前を書いて選ぶ（`docs/config.md`「単位」）。二つの言葉の定義の持ち主はこの段落で、ほかの文書も同じ意味で使う: 起動される実行ファイルと、マニフェスト、語彙、`check` の話は「プラグイン」、設定が選ぶもの、ファイルを出すもの、実装を出すものの話は「トランスパイラ」。

**hozoc 本体は、コードを出力しない。** 書き出すファイルは、どれも、どれかのトランスパイラが `emit` の応答で返したもの。本体は特定の言語を知らず、型や op の形を決めるのは、言語ごとの基底のトランスパイラ（`docs/config.md`「基底と相乗り」）。本体がトランスパイラの選択を集めて形を決める仕組み（`select`）は持たない（「選択をなくしたこと」）。

## マニフェスト `hozo-plugin.toml`

```toml
name     = "go-http"            # プラグインの名前
command  = ["hozoc-plugin-go-http"]   # 文字列か文字列の配列
protocol = "0.9"                # 対応する docs/ir.md の version

vocabulary = []                 # このプラグインが提供する語彙。マニフェストからの相対パス。省略時は []

[[transpiler]]                  # このプラグインが提供するトランスパイラ。いくつでも書ける
name        = "go-http"         # hozo.toml の transpilers に書く名前。必須
language    = "go"              # 出力する言語。必須。空でない文字列
requires    = ["go", "go-valid"]   # このトランスパイラが、出力の形を前提にするトランスパイラの名前。省略時は []（基底）
unsupported = []                # 意味を保てない記法（デコレータの FQN）。使われていれば CFG-6。省略時は []
seams       = []                # 継ぎ目を要るデコレータの FQN（docs/config.md「継ぎ目」）。例: ["rate.limited"]。省略時は []

[check]
enabled = false                 # true なら hozoc check / emit で direction "check" を呼ぶ。テーブルごと省けば false

[import]
formats = []                    # 取り込める形式の名前。空なら import を持たない。テーブルごと省けば []
```

基底のマニフェストは、`requires` を書かない（`docs/plugin-go.md`「マニフェスト」）。

- `command` が文字列なら実行ファイルの名前かパスで、引数なし。配列なら先頭が実行ファイル。実行ファイルにパスの区切りが含まれていればマニフェストのディレクトリからの相対パス、含まれていなければ `PATH` から探す。
- `[[transpiler]]` が一つもなければ `emit` を持たない。トランスパイラの名前はプラグインの名前と同じでも違ってもよい。一つのマニフェストの中で同じ名前を二度書けば MAN-4。
- **`requires`** は、このトランスパイラが依存するトランスパイラの名前の一覧。空なら**基底**、空でなければ**相乗り**（二つの言葉の定義と、単位に掛かる規則の持ち主は `docs/config.md`「基底と相乗り」: 単位は基底をちょうど一つ持ち、`requires` の名前はすべて単位の `transpilers` に書く）。
  - 書くのは、出力の形を前提にする相手のすべて。基底だけでなく、別の相乗り（検証の `go-valid` など）も書く。相手が依存するものまでは書かなくてよいが、単位の `transpilers` には、辿って届くすべてが要る（相手の `requires` が、相手について確かめられる）。
  - 相手は名前で指す。同じプラグインのトランスパイラでも、別のプラグインのものでもよい。その名前のトランスパイラが入っているかは、マニフェストでは確かめない（単位に書いたときに CFG-19 / CFG-14 で分かる）。
  - 自分の名前を書く、同じ名前を二度書く、文字列の配列でない、は MAN-4。
  - 依存は一方向で、オプションによらない（`docs/config.md`「基底と相乗り」）。
- **`seams`** は、このトランスパイラが継ぎ目を要るデコレータの FQN の一覧（継ぎ目の定義と、単位に掛かる規則の持ち主は `docs/config.md`「継ぎ目」）。このトランスパイラを書いた単位の基底は、単位の対象の op のうち、ここに書いたデコレータが付いたものを二層で出し、このトランスパイラは、その継ぎ目の中身を出す。
  - マニフェストに書くのは、宣言を静的にするため: 単位の継ぎ目は、設定とマニフェストだけから、どのトランスパイラを起動するより前に決まる。`emit` の応答で宣言する道はない（「選択をなくしたこと」）。
  - 書くのは、汎用のデコレータの FQN（名前付きデコレータの FQN は、どの適用にも当たらない）。その FQN のデコレータが宣言されているか、付けられる対象に op があるかは、確かめない。
  - 文字列の配列でない、空の文字列がある、同じ FQN を二度書く、は MAN-4。同じ単位の別のトランスパイラと重なるのは CFG-23（単位に書いたときに分かる）。
  - 基底（`requires` が空）も書ける。オプションによらない。
- `language` は、出力する言語の名前。中身は基底が決める（`"go"`、`"ts"`）。hozoc は、単位の中でそろっていること（CFG-16）だけを確かめ、名前の一覧を持たない。空の文字列は MAN-4。
- 言語を持つのはトランスパイラで、プラグインは持たない（一つのプラグインが言語の違うトランスパイラを提供してよい）。単位の言語の決め方は `docs/config.md`「基底と相乗り」。
- トランスパイラが受け取るオプション（`docs/config.md`「オプション」）は、マニフェストには書かない。キーと値の型を確かめるのはトランスパイラ自身で、一覧はそのプラグインの文書が持つ。
- `protocol` が hozoc の `docs/ir.md` の `version` と違えば MAN-2（省いたときも、違うものとして MAN-2）。
- 省いたときの値: `vocabulary` は `[]`、`[[transpiler]]` はなし、`requires` と `unsupported` と `seams` は `[]`、`[check]` は `enabled = false`（`check` を呼ばない）、`[import]` は `formats = []`（`import` を持たない）。`name` と `command` は省けない（MAN-4）。
- `vocabulary` の `.hozo` の読み込みは `docs/vocab.md`「読み込み」。

## 探索

**入っているプラグイン**は、次のディレクトリ d の直下で `<d>/<p>/hozo-plugin.toml` を持つディレクトリ p のすべて。p がプラグインの名前。同じ名前 p が複数の場所にあれば、次の順で最初に見つかったものだけを使う。プロジェクトの中、利用者の環境、ホームの順に、近いものを優先する。

1. `hozo.toml` の `[hozo].plugin_dirs` の各ディレクトリ（ルートからの相対パス）。
2. 環境変数 `HOZO_PLUGIN_PATH`（OS のパス区切りで複数）の各ディレクトリ。
3. `<ホーム>/.hozo/plugins`。

- 2 と 3 はプロジェクトの外の場所で、探索（`internal/plugin`）と解析（`internal/project`）は自分ではプロセスの環境を読まない。`hozoc` が、環境変数を `cli.hozo` の入力 `pluginPath` として（`docs/cli.md`「cli.hozo」）、ホームディレクトリを `cmd/hozoc` から受け取って渡す。渡されなければ 1 だけを探す。同じ入力から同じ結果にするためで、テストは実行する人の環境に入っているプラグインに左右されない（`docs/testing.md`「環境からの独立」）。
- 単位の `transpilers` の名前 t（`docs/config.md`「単位」）からトランスパイラを探す手続きの持ち主はここ: 入っているすべてのプラグインのマニフェストの `[[transpiler]]` の `name` から探す。どのプラグインにもなければ CFG-14。二つ以上のプラグインにあれば CFG-15（以降の処理は、上の順で最初のプラグインのものを使う）。
- **使うプラグイン**は、どれかの単位の `transpilers` に書かれたトランスパイラを提供するプラグイン（依存していても、書かれていなければ使うプラグインではない。足りない依存は CFG-19）。語彙を読むのも（`docs/vocab.md`「読み込み」）、`check` を呼ぶのも（「起動」）、使うプラグインだけ。入っているだけのプラグインは、名前を探す相手になる以外は何もしない。
- マニフェストは入っているプラグインのすべてについて読む（トランスパイラの名前を知るため）。読めないマニフェスト（MAN-4）と、`name` がディレクトリの名前 p と違うマニフェスト（MAN-3）は、使っていなくても報告する。`protocol` の不一致（MAN-2）は、使うプラグインにだけ報告する（ホームに入れた古いプラグインが、それを使わないプロジェクトを止めないようにするため）。
- 設定なし（`docs/config.md`「置き場と探し方」）のときは探索しない（選ぶ単位がない）。

## 起動

- 作業ディレクトリはプロジェクトのルート。環境変数は hozoc のものに `HOZO_PROJECT_ROOT`（ルートの絶対パス）と `HOZO_PLUGIN_DIR`（マニフェストのディレクトリの絶対パス）を足す。
- 標準入力に要求の JSON を一つ書いて閉じる。標準出力から応答の JSON を一つ読む。標準エラーは hozoc の標準エラーに、各行の先頭に `[<プラグイン名>] ` を付けて流す。
- `emit` は、出力（単位と、その単位が出力する名前空間の組。`docs/config.md`「in」）と、その単位のトランスパイラの組ごとに一回起動する。順序は、単位の設定の順、名前空間の IR の順、`transpilers` に書いた順。一つのプラグインの二つのトランスパイラを同じ単位に書けば、そのプラグインを二回起動する。
- **起動は互いに独立。** どのトランスパイラも、受け取るのは同じ出力の `View` だけで、ほかのトランスパイラの応答（ファイル、warning）は受け取らない。相乗りは、基底の応答を見ずに、基底と同じファイルの置き場と名前を自分で求める（「相乗りが基底の形を知る方法」）。`transpilers` に書いた順は、起動の順と診断の並びにだけ効き、書き出すファイルを変えない。
- `check` は、使うプラグイン（「探索」）のうち `[check] enabled` のものを、プラグインと名前空間の組ごとに一回起動する。相手の名前空間は、そのプラグインのトランスパイラを書いた単位が出力するもの。渡す `view` は、その単位のうち設定の順で最初のもの。
- 並列には起動しない（診断の順序を決定的にするため）。

終了コードと応答の読み方:

| 終了コード | 標準出力 | hozoc の扱い |
|---|---|---|
| 0 | 応答の JSON | 成功 |
| 0 | JSON として読めない、または形が違う（「応答の形」） | `proc.MalformedOutput` |
| 2 | `{ "error": { "code": "protocol", "message": "..." } }` | MAN-2 |
| 2 | `{ "error": { "code": "unsupported", "message": "..." } }` | MAN-5 |
| 2 | それ以外 | `proc.MalformedOutput` |
| それ以外 | — | `proc.Crashed` |

`command` が `go run` のとき（開発中のマニフェスト）、`go run` はプログラムの終了コードを伝えず 1 で終わるので、終了コード 2 の応答も `proc.Crashed` として報告される。配布するマニフェストはビルドした実行ファイルを `command` に書く（`docs/plugin-go.md`「マニフェスト」）。

## 信頼の前提

プラグインは利用者が入れ、`hozo.toml` にそのトランスパイラの名前を書いて選んだコードで、hozoc と同じユーザーの権限で動く子プロセス。protoc のプラグインと同じ信頼モデルで、サンドボックスも、応答の `files` 以外のファイルの変化の検出も行わない。hozoc が確かめるのは応答だけ（「応答の形」、PLG-1、PLG-2、CFG-5）。IR の取得と出力の書き込みを標準入出力に限るのは、プラグインを単純に書けるようにするための約束で、強制ではない（Go のトランスパイラは、import パスを求めるために `go.mod` を読む。`docs/plugin-go.md`「配置」）。

## 要求と応答

すべての要求は `protocol`（`docs/ir.md` の `version` と同じ値。`view.version` とも等しい）と `direction` を持つ。プラグインは `protocol` が合わなければ `code: "protocol"`、`direction` を実装していなければ（`emit` なら、要求の `transpiler` を実装していなければ）`code: "unsupported"` の `error` を返して終了コード 2 で終わる。`protocol` を先に確かめる。「合わない」は、プラグインが話す版と同じ文字列でないことで、`protocol` がない、`null`、文字列でない場合を含む。文字列でない `direction` は、実装していない `direction` と同じ。要求が JSON のオブジェクトとして読めなければ、応答を書かずに終了コード 1 で終わる（hozoc からは `proc.Crashed`）。二つの SDK はこの読み方を同じ表（`plugin/testdata/request_head.json`）でテストする。

### 応答の形

終了コード 0 の標準出力は、JSON の値一つで、下の各 direction の応答の形をしていなければならない。hozoc は次を確かめ、違えば `proc.MalformedOutput`（「起動」）。

- 応答は JSON のオブジェクト。
- **必須の項目**がある。`null` は「ない」と同じに扱う。`emit` の応答の `files` と `warnings`、`check` の応答の `diagnostics`（どれも、空でも `[]` で出す）。要素では、`files` の要素の `path` と `content`、`warnings` の要素の `message`、`errors` と `diagnostics` の要素の `code` と `message`、`diagnostics` の要素の `severity`。
- **省ける項目**は、なくても `null` でもよい。`emit` の応答の `errors`、`warnings` / `errors` / `diagnostics` の要素の `at`（省けば位置なし）。`errors` の要素の `severity` は見ない（常に error）。
- 書かれた項目の**型**が合う: 上の配列は配列、その要素はオブジェクト、`path` / `content` / `message` / `code` / `at` は文字列。`diagnostics` の `severity` は `"error"` か `"warning"`。
- **知らない項目は無視する**。応答のどの階層でも、ここにない名前の項目があってよい（プラグインが新しい版の項目を書いていても、古い hozoc はそれを読まないだけ。版 0.3 までの `select` を書いたままの応答も、読まれないだけで通る）。項目の名前は大文字と小文字を区別する。

形が合った後に、`path`（PLG-1 / PLG-2）を確かめる。`import` の応答の形は、段階 6 で `hozoc import` と一緒に決める。

### emit

```jsonc
// 要求
{ "protocol": "0.9", "direction": "emit",
  "transpiler": "go-http",                // 起動するトランスパイラの名前（マニフェストの [[transpiler]] の name）
  "view": { /* docs/ir.md の View。対象の名前空間は view.viewer、単位の設定は view.config */ } }

// 応答
{
  "files": [ { "path": "gen/server/http.go", "content": "..." } ],   // path はプロジェクトのルートからの相対パスで、view.config.dir（Dir(U, N)）の下
  "warnings": [ { "at": "server.login", "message": "..." } ],        // at は FQN か、"file:line:col" の文字列
  "errors":   [ { "code": "GOHTTP-1", "at": "server.login", "message": "..." } ]   // 省略可。一つでもあれば書き出さない
}
```

- `transpiler` は、単位の `transpilers` に書かれた名前のうち、このプラグインが提供する一つ。複数のトランスパイラを提供するプラグインは、この名前で処理を分ける。
- **オプション**は、要求の別の項目ではなく `view.config.options` にある（`docs/ir.md`「その名前空間から見た IR」）。起動されたトランスパイラへのオプションは `view.config.options[<transpiler>]` で、書かれていなければキーがない（空のテーブルと同じに読む）。SDK がこの読み方を持つ（`docs/packages.md`「`plugin`（公開）」の `Options`）。知らないキーと、型の違う値は、トランスパイラが `errors` で返す。
- `path` は、その起動の出力 (U, N) の出力先 Dir(U, N)（要求の `view.config.dir`。`docs/config.md`「単位の対象」）の下のファイルでなければならない。そうでなければ PLG-1: 絶対パス、`..` でルートや Dir(U, N) の外へ出るパス、ルートの中の別のディレクトリ、別の出力の Dir、Dir(U, N) そのもの。比べるのは `./` や `..` を畳んだパス。出力先ごとの記録（`docs/cli.md`「emit」の `.hozoc-outputs.json`）と、出力先の持ち主を一つに決める規則（CFG-17）が、どの出力のファイルもその Dir の下にあることを前提にしているため。同じ応答の中で同じ `path` が二度あれば PLG-2。どちらも `proc.MalformedOutput` として終了する。同じ出力の二つのトランスパイラが同じ `path` を返せば CFG-5（`docs/config.md`「診断」）。
- `content` は UTF-8 の文字列。バイナリは出せない。
- `files` と `warnings` は空でも `[]` で出す。省けるのは `errors`（「応答の形」）。
- `warnings` は warning の診断（コード `PLUGIN`、メッセージの先頭にトランスパイラの名前）。
- `errors` は、トランスパイラがその入力を変換できない理由（語彙の使い方の誤り、オプションの誤り、その言語で形にできない宣言。`docs/plugin-go.md`「診断」の GO-*、`docs/plugin-go-cli.md`「診断」の CLI-* など）。error の診断として報告し、`CheckFailed` になる（`docs/diagnostics.md`「終了コード」）。一つの出力のどれかのトランスパイラが `errors` を返せば、emit の全体が何も書き出さない（`docs/cli.md`「emit」）。
- 応答は、同じ要求から同じバイト列でなければならない（`docs/requirements.md`「品質の要件」の決定性）。時刻、乱数、環境変数、実行する人の環境に入っているものに依らない。

### check

```jsonc
{ "protocol": "0.9", "direction": "check", "view": { /* View */ } }

{ "diagnostics": [ { "severity": "error", "code": "PAGE-1", "message": "...", "at": "browser.openOrders" } ] }
```

- `severity` は `"error"` か `"warning"`。`code` はプラグインが決める（語彙に固有の規則の ID。`docs/requirements.md`「検査の規則の一覧」の PAGE-1 / PAGE-2 など）。
- `at` は FQN（その宣言の位置に付ける）か `"file:line:col"`。どちらでもなければ（省いたときも）位置なしで報告する。
- `check` の診断のメッセージには名前を付けない（`code` がプラグインの規則を示す）。`emit` の `warnings` にはトランスパイラの名前を付ける。
- `check` はトランスパイラごとではなくプラグインごとの処理で、要求は `transpiler` を持たない。

### import

```jsonc
{ "protocol": "0.9", "direction": "import", "format": "openapi", "files": [ { "path": "openapi.yaml", "content": "..." } ] }

{ "ir": { /* docs/ir.md の Program */ }, "warnings": [ /* 同上 */ ] }
```

`hozoc import` の詳細は段階 6（`docs/requirements.md`「段階」）で決める。形だけをここに置く。

## 相乗りが基底の形を知る方法

相乗りのトランスパイラは、基底が出すファイルの置き場と名前（パッケージ、型、メソッド、引数の並び）を前提に、自分のファイルを書く。前提にしてよいのは、`requires` に書いたトランスパイラの出力の形だけ。

- **知る手段は `View` と SDK で、基底の応答ではない。** 基底の形は、`View`（仕様と設定）だけから決まる決定的な規則で、その規則の持ち主は基底の文書（Go なら `docs/plugin-go.md`）。規則を写した関数を、基底と同じリポジトリの公開の SDK が持ち（Go なら `plugin/golang`。`docs/packages.md`「`plugin/golang`（公開）」）、基底も相乗りも同じ関数を呼ぶ。相乗りが規則を自分で書き写すことはしない。
- 相乗りが、別の相乗りの出力の形（`go-valid` が `Validate` を出す型）を前提にするときも同じで、その規則を写した公開の SDK（`plugin/govalid`）を呼ぶ。
- `View` が形のために持つもの（`docs/ir.md`「その名前空間から見た IR」）:

| `View` の項目 | 相乗りが知ること |
|---|---|
| `config.dir`、`config.out` | 自分のファイルを置く Dir(U, N) |
| `config.transpilers`、`config.base` | 同じ単位に並ぶトランスパイラと、その基底（依存がそろっていることは、hozoc が CFG-19 / CFG-20 で確かめてから起動する） |
| `config.options` | 単位のすべてのトランスパイラへのオプション（基底のオプションで形が変わるなら、それを再現できる） |
| `config.seams` | 単位の継ぎ目（`docs/config.md`「継ぎ目」）: どのデコレータの継ぎ目を、単位のどのトランスパイラが宣言したか。基底はこれで二層にする op を決め、宣言したトランスパイラは、同じ値から、自分が中身を出す口を基底と同じに求める。ほかの相乗りも、基底の `New` の引数が増えることを、ここから知る |
| `files` | 単位の対象の宣言（`in`） |
| `namespaces` | ほかの名前空間の型の置き場（`docs/config.md`「型の置き場」）: どの単位が、どの基底で、どこへ出力するか |
| `program`、`ops` | 仕様のすべてと、計算した結果（例外、効果、読む状態、純粋さ、`bind` の位置） |

- 相乗りが足すファイルと公開の名前は、基底と、同じ基底のほかの相乗りのものと当たらないように、プラグインの文書で決める（`docs/plugin-go.md`「相乗りのファイル」が、Go の相乗りの決まりの持ち主）。当たったファイルは CFG-5 になる。

## 選択をなくしたこと

版 0.3 までの `emit` の応答は `select`（`roots` / `stops` / `states` / `wraps`）を持ち、hozoc が、単位のすべてのトランスパイラの `select` を合わせて、本体に組み込んだ出力（「既定の出力」）の対象を決めていた。版 0.4 でなくした（`docs/decisions.md`「決めたこと」77）。

- 何を出すかは、基底が、仕様と設定だけから決める（Go なら `docs/plugin-go.md`「出力の対象」: 型と状態はすべて、op は単位の対象のすべて）。トランスパイラが選ぶ余地はない。
- コードにしたくない op は、単位を置かない名前空間に書くか、`in` で外す（`docs/config.md`「in」）。
- デコレータに応じて包む関数を本体が生成する仕組み（`wraps`）もない。デコレータの意味は、それを読むトランスパイラが形にする（`docs/plugin-go.md`「デコレータ」）。op の中身の前後に処理を置くデコレータは、マニフェストの `seams` で継ぎ目を宣言する（`docs/config.md`「継ぎ目」）。`wraps` と違うのは、宣言が応答ではなくマニフェストにあり（起動の前に決まる）、口を出すのが本体ではなく基底で、中身を出すのが宣言したトランスパイラであること。
- SDK の `Select`、`Wrap` は持たない。「このデコレータが付いた、単位の対象の op」を集める関数（`OpsWith`）は、相乗りが自分の対象を探すための道具として残す。

設計書 §4.6 のマニフェストの `decorate` / `entry` / `stop` も持たない。

## 診断

| ID | 重さ | 意味 |
|---|---|---|
| MAN-2 | error | `protocol` が合わない（マニフェストの `protocol` が違う。または起動したプラグインが `code: "protocol"` で断った） |
| MAN-3 | error | マニフェストの `name` がディレクトリ名と違う |
| MAN-4 | error | マニフェストを読めない: ファイルを開けない、TOML の構文の誤り、値の型違い、知らないキー、`name` がない、`command` がない・空・文字列でも文字列の配列でもない、`[[transpiler]]` に `name` か `language` がない、`language` が空、`requires` に自分の名前か同じ名前が二度ある、`seams` に空の文字列か同じ FQN が二度ある、同じ名前の `[[transpiler]]` が二つある。使うプラグインの `vocabulary` のファイルを読めないときも MAN-4 |
| MAN-5 | error | プラグインがその `direction` か、要求の `transpiler` を実装していない（マニフェストに書いてあるのに。起動したプラグインが `code: "unsupported"` で断った） |
| PLG-1 | — | 応答の `files` の `path` が、その起動の Dir(U, N) の下にない（「emit」） |
| PLG-2 | — | 応答の中で同じ `path` が二度 |
| PLUGIN | warning | トランスパイラの `warnings` |

- MAN-1 は欠番。マニフェストの `[emit]` の `entry` も `decorate` も空であることを報告していた規則で、`[emit]` を `[[transpiler]]` に替えたときになくなった。
- PLG-3 は欠番。応答の `select` が、対象の名前空間にない宣言、種類の違う宣言、単位の対象でない宣言、付いていないデコレータを指していることを報告していた規則で、`select` をなくした（「選択をなくしたこと」）ときになくなった。
- 番号は別の意味に使い回さない（`docs/config.md`「診断」と同じ。欠番の一覧は `docs/requirements.md`「検査の規則の一覧」）。
- MAN-4 の「`language` が `"go"` でも `"ts"` でもない」という条件はなくした（言語の名前は基底が決める。「マニフェスト」）。
- MAN-* は診断で、例外は `ConfigInvalid`。
- PLG-* は診断にならない（重さが「—」）。応答そのものが信用できないので、診断の一覧に足して続けることはせず、例外 `proc.MalformedOutput` で終わる。ID は、その例外のメッセージの頭に書く（`プラグイン fake の出力が約束した形でない: PLG-1: …`）。`--format json` でも `diagnostics` には現れず、`{"error": "proc.MalformedOutput"}` になる（`docs/plugin-go-cli.md`「実行と表示」の `JSONPresenter`）。「応答の形」の誤りも同じ例外で、ID は持たない。
- 終了コードは `docs/diagnostics.md`「終了コード」。

## SDK

Go は `plugin` パッケージ、TS は `sdk/ts/`。形は `docs/packages.md`「`plugin`（公開）」。
