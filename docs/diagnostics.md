# 診断

## この文書が決めること

検査・解析・設定・プラグインが報告する診断の構造、表示の形式、終了コード。Go の型は `internal/diag`。

## 構造

```go
type Severity int // Error, Warning

type Diagnostic struct {
    Severity Severity
    Code     string          // 規則の ID（"THROWS-1"、"RES-1"、"SYNTAX"、"PLUGIN"）
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

- `Code` は `docs/requirements.md`「検査の規則の一覧」と、各設計文書の診断の表のもの。構文エラーは `SYNTAX`、プラグインの warning は `PLUGIN`。
- `Message` は設計書の言葉で書く。例: THROWS-1 → `openOrders は http.NetworkError を投げうるが throws に書かれていない（出どころ: fetchMe の呼び出し）`。
- `Data` の中身は `docs/check.md`「Data」。

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
| 1 | `CheckFailed` | error の診断が一つ以上（CFG-* と MAN-* 以外のすべて。プラグインの `errors` と GOEMIT-* を含む） |
| 2 | `ConfigInvalid` / `LanguageMismatch` | `docs/config.md`「診断」と `docs/plugin-protocol.md`「診断」の error |
| 3 | `OutputConflict` | CFG-5 |
| 64 | `UnknownNamespace`、使い方の誤り | 引数・フラグの解釈の失敗と、`cli.hozo` で 64 を付けた例外 |
| 70 | `proc.Crashed` / `proc.MalformedOutput`、宣言にない例外 | プラグインの異常終了（`docs/plugin-protocol.md`「起動」）と、`cli.exitCode` のない例外 |

`hozoc check` は、error が一つもなければ 0。warning を error に格上げするフラグは持たない（必要になったら足す）。
