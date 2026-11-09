# 言語サーバー

## この文書が決めること

`hozoc lsp` が話す Language Server Protocol（3.17）の範囲と、各機能の中身。機能の一覧と本体への前提条件の持ち主は `docs/requirements.md`「言語サーバー」で、ここはその実装の形。`internal/lsp` が写す。

## 通信

- 標準入出力。`Content-Length` ヘッダ付きの JSON-RPC 2.0。実装は自前（`internal/lsp/jsonrpc`。`docs/packages.md`「依存の向き」）。
- 要求の型は使うものだけを `internal/lsp/protocol` に手で定義する。
- 位置のエンコーディングは `initialize` の `general.positionEncodings` に `utf-8` があれば `utf-8`、なければ `utf-16`。`docs/ir.md` の `Pos`（コードポイント）から変換する。
- `$/cancelRequest` は受けるが無視する（どの要求も短い）。
- ログは `--log` のファイルに。標準エラーには書かない（クライアントによっては表示が乱れる）。

## ライフサイクル

| 要求 | 振る舞い |
|---|---|
| `initialize` | `rootUri`（なければ `workspaceFolders` の先頭）をワークスペースにし、下の「プロジェクト」を作る。capabilities を返す |
| `initialized` | 開いているファイルがなくても、プロジェクト全体の診断を一度発行する |
| `shutdown` / `exit` | 終了。`lsp` の op は値を返さず例外も持たないので、終了コードは常に 0 |

capabilities: `textDocumentSync: { openClose: true, change: Full, save: { includeText: false } }`、`definitionProvider`、`hoverProvider`、`documentSymbolProvider`、`positionEncoding`。段階 4 の後に `completionProvider`、`referencesProvider`、`codeActionProvider`、`semanticTokensProvider` を足す。

## プロジェクト

- ワークスペースのディレクトリから親へ向かって `hozo.toml` を探す（`--config` があればそれ）。見つかったディレクトリがルート。
- 見つからなければ、ワークスペースのディレクトリをルートにし、設定なしで動く: `sources` は既定、設定に関わる検査（CFG-*）は行わない。各ファイルの先頭（1 行 1 列）に warning `LSP-1`「hozo.toml が見つからない。設定の検査は行っていない」を出す。
- 解析の単位はプロジェクト全体。`internal/project.Load`（`docs/packages.md`「internal/project」）を呼び直す。`overlay` は開いている文書の最新の内容（保存していなくても）で、ディスクより優先する。
- `didOpen` / `didChange` / `didClose` / `didSave`、および `hozo.toml` の変更（`workspace/didChangeWatchedFiles` を `**/hozo.toml` と `**/*.hozo` で登録）で解析し直す。`didChange` は最後の変更から 150 ms 待ってまとめる。
- 解析し直すたびに、プロジェクトのすべてのファイルについて `publishDiagnostics` を送る。診断がなくなったファイルには空の配列を送る。前回と同じ内容なら送らない。
- 語彙のファイルの URI は `Span.file`（`docs/ir.md`「Program」の `hozo:` の形）をそのまま使う。クライアントが開けるよう、要求 `hozo/vocabulary`（params `{ uri }`、result `{ content }`）を提供する。`docs/editors.md`。

## 診断

`docs/diagnostics.md` の `Diagnostic` を LSP の `Diagnostic` に写す。

| hozoc | LSP |
|---|---|
| `Severity` | `Error` → 1、`Warning` → 2 |
| `Code` | `code` |
| `Message` | `message` |
| `Span` | `range`（`End` は含まないので、そのまま） |
| `Related` | `relatedInformation` |
| `Data` | `data`（コードアクションが使う） |
| 位置のない診断（`hozo.toml` の誤り） | `hozo.toml` の 1 行 1 列。ファイルもないものは `hozo.toml` の URI に付ける |

`hozoc check` と同じ `internal/check` の呼び出しなので、同じ入力に同じ診断が出る（`docs/testing.md`「言語サーバーと check の一致」）。

## 定義へジャンプ

カーソル位置のトークンが名前（`Ident` か `QualifiedName` の一部）なら、`internal/sema` の `Ref`（`docs/packages.md`「internal/sema」）の `Target` の `Span` を `Location` で返す。

| カーソルの位置 | 飛び先 |
|---|---|
| 宣言への参照（型、`throws`、デコレータ名、呼び出しの op など） | その宣言の名前のトークン |
| `a.b.c` の途中のセグメント | そのセグメントまでで決まるもの（名前空間なら、その名前空間の最初のブロックの名前） |
| フィールドの参照 `x.f` | フィールドの宣言 |
| 束縛名 | `const` の名前 |
| 引数名 | 引数 |
| `match` のケース名 | `classify` のケース |
| デコレータの引数名（`name:`） | デコレータの宣言の `Param` |
| ドキュメンテーションコメントの `@param` / `@throws` / `@see` の名前 | 同上 |
| 語彙の宣言 | `hozo:` の URI の位置 |

解決できなければ `null`。

## ホバー

Markdown の `MarkupContent`。順に:

1. Hozo の構文で書いた宣言の見出し（コードブロック、言語 `hozo`）。IR から組み立てる: `op login(in: LoginInput): LoginOk`、`model User`、`scalar Email extends string`、`exception InvalidCredentials: Violations`、`effect createSession on sessions.post`、`decorator validated throws ClientError prevents effects`、`state users: list<User>`、`classify WarrantyKind of server.Item`、フィールドなら `token?: Token`。名前は見ている名前空間から見た修飾（同じなら修飾なし）。
2. `description.body`。
3. op なら `throws:` と `effects:` の一覧（`Signature`）。例外ごとの効果の状態（`relations`）が `unknown` でないものを添える。
4. デコレータの適用なら、適用先ではなくデコレータの宣言の 1〜2 と、固定した引数。

## ドキュメントのシンボル

`DocumentSymbol` の木。名前空間は `Namespace`、その中に宣言。

| 宣言 | `SymbolKind` |
|---|---|
| `model` | `Struct`。フィールドは子の `Field` |
| `scalar` | `TypeParameter` |
| `union` | `Enum`。アームは子の `EnumMember` |
| `state` | `Variable` |
| `op` | `Function` |
| `exception` | `Class` |
| `effect` | `Event` |
| `classify` / `condition` | `Enum`。ケースは子の `EnumMember` |
| `decorator` | `Interface` |
| `bind` | `Property` |

## 段階 4 の後

- **補完**: 名前の位置で、`docs/resolve.md`「位置ごとに受け付ける種類」に合う名前。別の名前空間のものは修飾した形。
- **参照の検索**: 宣言から、それを参照しているトークンすべて。
- **コードアクション**: 診断の `Data` から作る。THROWS-1 → `throws` に `missing` を足す。MATCH-2 → 足りないアームを足す。RES-1 で候補があれば置き換える。
- **semantic tokens**: 型の位置の名前と値の位置の名前を区別して色を付ける（TextMate では命名規則に頼るしかない部分）。
