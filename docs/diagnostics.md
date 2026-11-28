# 診断

## この文書が決めること

検査・解析・設定・プラグインが報告する診断の構造、表示の形式、終了コード。Go の型は `internal/diag`。

## 構造

```go
type Severity int // Error, Warning

type Diagnostic struct {
    Severity Severity
    Code     string          // 規則の ID（"REL-4"、"RES-1"）か、規則の ID でないコード（"SYNTAX"、"PLUGIN"）
    Message  string          // 一文。何が、どこで、どう違うか。名前は FQN で
    Span     *source.Span    // 位置（バイトのオフセット。docs/packages.md「internal/source」）。位置がなければ nil
    File     string          // Span が nil のときのファイル（hozo.toml など）。空なら全体
    Related  []Related       // 関連する位置（重複の相手、例外の出どころの行など）
    Data     map[string]any  // 機械が読む補足。中身は spec/diagnostics.hozo の、Data の model。JSON に出し、コードアクション（docs/lsp.md「段階 4 の後」）が使う
}

type Related struct {
    Span    source.Span
    Message string
}
```

- `Code` は、仕様の名前空間 `hozo.diagnostics` の分類のケースの ID（`spec/diagnostics/`）。トランスパイラが `emit` の応答の `errors` で返したコード（`docs/plugin-protocol.md`「emit」）は、そのまま `Code` になる。規則の ID でないコードは下の「規則の ID でないコード」。コードが出しうる `Code` がどれかの表にあり、表の重さがコードと合っていることは `tests/codes_test.go` が確かめる。
- `Message` は、`docs/` の文書の言葉で書く。例: BIND-4 → `openOrders の呼び出しで必須の入力 token が明示でも bind でも埋まっていない`。
- `Message` は、設計書（`docs/README.md`「設計書という呼び名」）と、その節番号（「§n」）を引かない。設計書は仕様の持ち主ではなく、利用者が読んで確かめる先にならない。文書を引くなら、`docs/` の文書と見出しを引く（`docs/check.md`「残りのケース」）。
- `Data` の中身は `spec/diagnostics.hozo` の、Data の model。

### 規則の ID でないコード

規則の ID の形（`<語>-<番号>`）でないコードが、四つある: 構文エラーの `SYNTAX`、ファイルを読めない `IO`、トランスパイラの warning の `PLUGIN`、Go の基底の不具合の `GO-INTERNAL`。条件と重さの持ち主は、ほかの診断と同じく、仕様の名前空間 `hozo.diagnostics` のケース（`spec/diagnostics/hozoc.hozo` と `spec/diagnostics/go.hozo`）。

## 位置

`Span` はバイトのオフセットで持ち、表示するときに `source.FileSet.Position`（`docs/packages.md`「internal/source」）で 1 始まりの行と、Unicode のコードポイントで数えた 1 始まりの列にする（`docs/ir.md` の `Pos` と同じ）。`End` は含まない。テキスト表示は `Start` だけを使う。

## 表示

### text（既定）

```
examples/orders.hozo:42:5: error[DISJ-3]: disjoint で、二つのアームが、同じ状態 store.orders に、効果を持つ: …（同じ行に書くと、順で、結果が変わる。直列に書く）
  examples/orders.hozo:44:5: note: store.orders に書く
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
    { "severity": "error", "code": "MATCH-2", "message": "...",
      "file": "examples/orders.hozo", "start": { "line": 42, "col": 3 }, "end": { "line": 42, "col": 13 },
      "related": [ { "file": "...", "start": { "line": 44, "col": 16 }, "end": { "line": 44, "col": 23 }, "message": "..." } ],
      "data": { "missing": ["Registered"], "extra": [] } }
  ],
  "errors": 2,
  "warnings": 1
}
```

位置のないものは `start` / `end` を省く。`file` もなければ `file` も省く。

## 順序

ファイルの順（`docs/ir.md`「原則」。ファイルのない診断はすべての先頭）、同じファイルでは位置のないものが先、次に `Start` の順、`Code` の辞書順、`Message` の辞書順。同じ入力から同じ出力になる。

## 終了コード

`spec/cli.hozo`（`docs/cli.md`）の例外の `@cli.exitCode` と、`cli` の語彙の既定（`plugins/go-cli/spec/go-cli.hozo` の `Requirement.Run`）で決まる。64 と 70 は `sysexits` の慣習（`EX_USAGE`、`EX_SOFTWARE`）。

| コード | 例外 | 中身 |
|---|---|---|
| 0 | — | 成功。warning はあってよい |
| 1 | `CheckFailed` | error の診断が一つ以上で、設定とマニフェストの error（下の 2）がない。トランスパイラの `errors`（GO-*、GOVALID-*、GOSQL-*、GOHTTP-*、CLI-* など。それぞれのプラグインの文書の「診断」）を含む |
| 2 | `ConfigInvalid` / `LanguageMismatch` | 設定とマニフェストの error の診断が一つ以上: CFG-*（CFG-5 を除く。`docs/config.md`「診断」）と MAN-*（`docs/plugin-protocol.md`「診断」）。それがすべて CFG-16 なら `LanguageMismatch`、そうでなければ `ConfigInvalid`（`spec/cli.hozo` の `check` の手順の 5） |
| 3 | `OutputConflict` | CFG-5 |
| 4 | `TestsNotRun` | 求めるテストのうち、走ったと確かめられないものがある（TESTS-1。`spec/cli.hozo` の `confirmTests`） |
| 5 | `CoverageUnusable` | カバレッジを、求めるテストと突き合わせられない（TESTS-2、TESTS-3。同） |
| 64 | `UnknownNamespace`、使い方の誤り | 引数・フラグの解釈の失敗と、`spec/cli.hozo` で 64 を付けた例外 |
| 70 | `Crashed` / `MalformedOutput`、宣言にない例外 | プラグインの異常終了（`Crashed`）、プラグインの応答の誤り（`MalformedOutput`: JSON でない、形が違う、PLG-1、PLG-2。診断にはならず、例外のメッセージで示す。`docs/plugin-protocol.md`「起動」「応答の形」「診断」）、`cli.exitCode` のない例外 |

`hozoc build --no-emit` は、error が一つもなければ 0。warning を error に格上げするフラグは持たない（必要になったら足す）。
