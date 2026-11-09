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
    Span     *source.Span    // 位置。設定ファイルの誤りなど、位置のないものは nil で File だけを Related に持たない
    File     string          // Span が nil のときのファイル（hozo.toml など）。空なら全体
    Related  []Related       // 関連する位置（重複の相手、例外の出どころの行など）
    Data     map[string]any  // 機械が読む補足。コードアクションが使う（docs/lsp.md）。JSON に出す
}

type Related struct {
    Span    source.Span
    Message string
}
```

- `Code` は `docs/requirements.md`「検査の規則の一覧」と、各設計文書の診断の表のもの。構文エラーは `SYNTAX`、プラグインの warning は `PLUGIN`。
- `Message` は設計書の言葉で書く。例: THROWS-1 → `openOrders は http.NetworkError を投げうるが throws に書かれていない（出どころ: fetchMe の呼び出し）`。
- `Data` の中身は規則ごとに `docs/check.md` または `docs/lsp.md`「コードアクション」が決める。THROWS-1 なら `{"missing": ["http.NetworkError"]}`。

## 位置

`source.Span{File, Start, End}`。`Start` / `End` は 1 始まりの行と、Unicode のコードポイントで数えた 1 始まりの列（`docs/ir.md` の `Pos` と同じ）。`End` は含まない。テキスト表示は `Start` だけを使う。

## 表示

### text（既定）

```
examples/orders.hozo:42:3: error[THROWS-1]: openOrders は http.NetworkError を投げうるが throws に書かれていない
  examples/orders.hozo:44:16: note: 出どころ: fetchMe の呼び出し
hozo.toml: error[CFG-1]: 名前空間 server に language がない
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

位置のないものは `file` / `start` / `end` を省く。

## 順序

ファイルのパスの辞書順（標準の語彙 `hozo:` はユーザーのファイルの後）、`Start` の順、`Code` の辞書順、`Message` の辞書順。同じ入力から同じ出力になる。

## 終了コード

`examples/hozoc.hozo` の `cli.exitCode` と、`cli` の語彙の既定（`docs/plugin-go-cli.md`）に合わせる。

| コード | 意味 | 出どころ |
|---|---|---|
| 0 | 成功。warning はあってよい | |
| 1 | `CheckFailed`: error が一つ以上 | 解析、解決、型、検査 |
| 2 | `ConfigInvalid` / `LanguageMismatch`: 設定、マニフェスト、探索の誤り | CFG-1〜4、7〜11、MAN-* |
| 3 | `OutputConflict` | CFG-5 |
| 64 | 使い方の誤り（引数、フラグ） | `go-cli` の既定 |
| 70 | 内部エラー、プラグインの異常終了（`proc.Crashed` / `proc.MalformedOutput`）、PLG-* | `go-cli` の既定 |

`hozoc check` は、error が一つもなければ 0。warning を error に格上げするフラグは持たない（必要になったら足す）。
