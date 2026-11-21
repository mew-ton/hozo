# 診断

## この文書が決めること

検査・解析・設定・プラグインが報告する診断の構造、表示の形式、終了コード。Go の型は `internal/diag`。

## 構造

```go
type Severity int // Error, Warning

type Diagnostic struct {
    Severity Severity
    Code     string          // 規則の ID（"THROWS-1"、"RES-1"）か、規則の ID でないコード（"SYNTAX"、"PLUGIN"）
    Message  string          // 一文。何が、どこで、どう違うか。名前は FQN で
    Span     *source.Span    // 位置（バイトのオフセット。docs/packages.md「internal/source」）。位置がなければ nil
    File     string          // Span が nil のときのファイル（hozo.toml など）。空なら全体
    Related  []Related       // 関連する位置（重複の相手、例外の出どころの行など）
    Data     map[string]any  // 機械が読む補足。中身は docs/check.md「Data」。JSON に出し、コードアクション（docs/lsp.md「段階 4 の後」）が使う
}

type Related struct {
    Span    source.Span
    Message string
}
```

- `Code` は `docs/requirements.md`「検査の規則の一覧」と、各設計文書の「診断」の表のもの。トランスパイラが `emit` の応答の `errors` で返したコード（`docs/plugin-protocol.md`「emit」）は、そのまま `Code` になる。規則の ID でないコードは下の「規則の ID でないコード」。コードが出しうる `Code` がどれかの表にあり、表の重さがコードと合っていることは `tests/codes_test.go` が確かめる。
- `Message` は、`docs/` の文書の言葉で書く。例: THROWS-1 → `openOrders は http.NetworkError を投げうるが throws に書かれていない（出どころ: fetchMe の呼び出し）`。
- `Message` は、設計書（`docs/README.md`「設計書という呼び名」）と、その節番号（「§n」）を引かない。設計書は仕様の持ち主ではなく、利用者が読んで確かめる先にならない。文書を引くなら、`docs/` の文書と見出しを引く（`docs/check.md`「残りのケース」）。
- `Data` の中身は `docs/check.md`「Data」。

### 規則の ID でないコード

| ID | 重さ | 意味 |
|---|---|---|
| SYNTAX | error | 構文エラー（`docs/grammar.md` の文法に合わない）。誤りのある箇所を読み飛ばして続ける（回復のしかたは `docs/packages.md`「パッケージ」の `internal/syntax` の項） |
| IO | error | `sources` に合うファイルを探せない、または読めない（`docs/cli.md`「check」の 2）。読めたファイルの検査は続ける。位置は持たず、読めなかったファイルが分かればそのファイル |

ほかに、トランスパイラの warning の `PLUGIN`（`docs/plugin-protocol.md`「診断」）、Go の基底の不具合の `GO-INTERNAL`（`docs/plugin-go.md`「診断」）、言語サーバーだけが出す `LSP-1`（`docs/lsp.md`「診断」）がある。持ち主はそれぞれの文書。

## 位置

`Span` はバイトのオフセットで持ち、表示するときに `source.FileSet.Position`（`docs/packages.md`「internal/source」）で 1 始まりの行と、Unicode のコードポイントで数えた 1 始まりの列にする（`docs/ir.md` の `Pos` と同じ）。`End` は含まない。テキスト表示は `Start` だけを使う。

## 表示

### text（既定）

```
examples/orders.hozo:42:3: error[THROWS-1]: openOrders は http.NetworkError を投げうるが throws に書かれていない
  examples/orders.hozo:44:16: note: 出どころ: fetchMe の呼び出し
hozo.toml: error[CFG-12]: 2 番目の [[emit]]（namespace = "server"）に out がない
2 errors, 1 warning
```

- 一行目は `ファイル:行:列: 重さ[コード]: メッセージ`。位置がなければ `ファイル: ` か、ファイルもなければ `hozoc: `。
- `Related` は二つのスペースで字下げし、`note:`。
- 最後に件数。`0 errors` でも出す。
- 色は付けない（段階 4 の後に、標準出力が端末のときだけ付けることを検討する）。

### json

```json
{
  "diagnostics": [
    { "severity": "error", "code": "THROWS-1", "message": "...",
      "file": "examples/orders.hozo", "start": { "line": 42, "col": 3 }, "end": { "line": 42, "col": 13 },
      "related": [ { "file": "...", "start": { "line": 44, "col": 16 }, "end": { "line": 44, "col": 23 }, "message": "..." } ],
      "data": { "missing": ["http.NetworkError"] } }
  ],
  "errors": 2,
  "warnings": 1
}
```

位置のないものは `start` / `end` を省く。`file` もなければ `file` も省く。

## 順序

ファイルの順（`docs/ir.md`「原則」。ファイルのない診断はすべての先頭）、同じファイルでは位置のないものが先、次に `Start` の順、`Code` の辞書順、`Message` の辞書順。同じ入力から同じ出力になる。

## 終了コード

`cli.hozo`（`docs/cli.md`）の例外の `@cli.exitCode` と、`cli` の語彙の既定（`docs/plugin-go-cli.md`「実行と表示」）で決まる。64 と 70 は `sysexits` の慣習（`EX_USAGE`、`EX_SOFTWARE`）。

| コード | 例外 | 中身 |
|---|---|---|
| 0 | — | 成功。warning はあってよい |
| 1 | `CheckFailed` | error の診断が一つ以上で、設定とマニフェストの error（下の 2）がない。トランスパイラの `errors`（GO-*、GOVALID-*、GOSQL-*、GOHTTP-*、CLI-* など。それぞれのプラグインの文書の「診断」）を含む |
| 2 | `ConfigInvalid` / `LanguageMismatch` | 設定とマニフェストの error の診断が一つ以上: CFG-*（CFG-5 を除く。`docs/config.md`「診断」）と MAN-*（`docs/plugin-protocol.md`「診断」）。それがすべて CFG-16 なら `LanguageMismatch`、そうでなければ `ConfigInvalid`（`docs/cli.md`「check」の 5） |
| 3 | `OutputConflict` | CFG-5 |
| 4 | `TestsNotRun` | 求めるテストのうち、走ったと確かめられないものがある（TESTS-1。`docs/cli.md`「tests」） |
| 5 | `CoverageUnusable` | カバレッジを、求めるテストと突き合わせられない（TESTS-2、TESTS-3。同） |
| 64 | `UnknownNamespace`、使い方の誤り | 引数・フラグの解釈の失敗と、`cli.hozo` で 64 を付けた例外 |
| 70 | `proc.Crashed` / `proc.MalformedOutput`、宣言にない例外 | プラグインの異常終了（`proc.Crashed`）、プラグインの応答の誤り（`proc.MalformedOutput`: JSON でない、形が違う、PLG-1、PLG-2。診断にはならず、例外のメッセージで示す。`docs/plugin-protocol.md`「起動」「応答の形」「診断」）、`cli.exitCode` のない例外 |

`hozoc check` は、error が一つもなければ 0。warning を error に格上げするフラグは持たない（必要になったら足す）。
