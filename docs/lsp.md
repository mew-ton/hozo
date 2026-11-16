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
- 見つからなければ、ワークスペースのディレクトリをルートにし、設定なしで動く: `sources` は既定、設定に関わる検査（CFG-*）は行わない。各ファイルの先頭（1 行 1 列）に warning LSP-1 を出す（「診断」）。
- 解析の単位はプロジェクト全体。`internal/project.LoadWith`（`docs/packages.md`「internal/project」）を呼び直す。`Overlay` は開いている文書の最新の内容（保存していなくても）で、ディスクより優先する。プラグインの探索が見るプロジェクトの外の場所（`HOZO_PLUGIN_PATH` とホーム。`docs/plugin-protocol.md`「探索」）は、`hozoc lsp` の入力から渡されたもの（`docs/cli.md`「cli.hozo」の `LspInput`、「手で書くもの」）。
- `didOpen` / `didChange` / `didClose` / `didSave`、および `hozo.toml` の変更（`workspace/didChangeWatchedFiles` を `**/hozo.toml` と `**/*.hozo` で登録）で解析し直す。`didChange` は最後の変更から 150 ms 待ってまとめる。
- 解析し直すたびに、診断のあるファイル（語彙のファイルと `hozo.toml` を含む）について `publishDiagnostics` を送る。前に診断を送っていて、今回は診断がなくなったファイルには、空の配列を送る。前回と同じ内容なら送らない。一度も診断を送っていないファイルには、空の配列を送らない（診断のないファイルと語彙のファイルの数だけ、最初に空の通知が並ぶのを避ける。クライアントは、通知の来ていないファイルを診断なしとして扱う）。
- 語彙のファイルの URI は `Span.file`（`docs/ir.md`「Program」の `hozo:` の形）をそのまま使う。クライアントが開けるよう、要求 `hozo/vocabulary`（params `{ uri }`、result `{ content }`）を提供する。`docs/editors.md`。

## 診断

`docs/diagnostics.md` の `Diagnostic` を LSP の `Diagnostic` に写す。`source` は `hozoc`。

| hozoc | LSP |
|---|---|
| `Severity` | `Error` → 1、`Warning` → 2 |
| `Code` | `code` |
| `Message` | `message` |
| `Span` | `range`（`End` は含まないので、そのまま） |
| `Related` | `relatedInformation` |
| `Data` | `data`（コードアクションが使う） |
| 位置のない診断（`hozo.toml` の誤り） | `hozo.toml` の 1 行 1 列。ファイルもないものは `hozo.toml` の URI に付ける |

`hozoc check` と同じ `internal/check` の呼び出しなので、同じ入力に同じ診断が出る（`docs/testing.md`「言語サーバーと check の一致」）。言語サーバーだけが出す診断は一つ:

| ID | 重さ | 意味 |
|---|---|---|
| LSP-1 | warning | `hozo.toml` が見つからない。設定の検査は行っていない（「プロジェクト」）。ユーザーの各 `.hozo` の 1 行 1 列に出す |

## 定義へジャンプ

カーソル位置のトークンが名前（`Ident`、または `QualifiedName` か値の位置の `Path` の一部。`docs/grammar.md`「識別子」）なら、`internal/sema` の `Ref`（`docs/packages.md`「internal/sema」）の `Target` の `Span` を `Location` で返す。

| カーソルの位置 | 飛び先 |
|---|---|
| 宣言への参照（型、`throws`、デコレータ名、呼び出しの op など） | その宣言の名前のトークン |
| `model D extends B` の `B` | model `B` の名前 |
| `override f` の `f` | 元の model の `fields` の、そのフィールドの位置（`Field.span`。`docs/ir.md`「model / scalar / union」。元の model が受け継いだままのフィールドなら、さらに元の宣言の位置になる） |
| `classify C of op` の `op`、ケースの `op as T` の `op` | その op の名前 |
| ケースの `X as T` の `X`（状態、型）と `T` | 状態なら `state` の名前、型ならその宣言 |
| `reads` の状態の名前 | `state` の名前 |
| デコレータの宣言の `reads` / `effects` の節の名前 | 状態か効果の宣言。デコレータの引数の名前（残りの引数を含む）なら、その `Param` |
| 予約語のデコレータ名（`@db.list`、`@effects`、別名の `=` の後ろ） | ほかのデコレータ名と同じく、その `decorator` の名前 |
| 配線の値の位置の、状態の名前（`f(token)`、`match token by nullable`） | `state` の名前。同じ名前の束縛か引数があれば、そちら（`docs/resolve.md`「手続き」） |
| `match x by ns.C` の `C` | `classify` の名前 |
| アームの `throw E` の `E` | `exception` の名前 |
| `a.b.c` の途中のセグメント | そのセグメントまでで決まるもの（名前空間なら、その名前空間の最初のブロックの名前） |
| フィールドの参照 `x.f` | フィールドの宣言 |
| 束縛名 | `const` の名前 |
| 引数名 | 引数 |
| `match` のケース名 | `classify` のケース。標準の分類（`by list` / `by nullable`）の `Empty` / `HasItems` と、`by` の後ろの `list` / `nullable` は宣言を持たないので `null` |
| デコレータの引数名（`name:`） | デコレータの宣言の `Param` |
| ドキュメンテーションコメントの `@param` / `@throws` / `@see` の名前 | 同上（`internal/sema` の `Refs` にはないので、言語サーバーが構文木から解決する） |
| 語彙の宣言 | `hozo:` の URI の位置 |

解決できなければ `null`。

## ホバー

Markdown の `MarkupContent`。順に:

1. Hozo の構文で書いた宣言の見出し（コードブロック、言語 `hozo`）。IR から組み立てる: `op login(in: LoginInput): LoginOk`、`export op login(in: LoginInput): LoginOk`（`Op.exported`）、`op unauthorized(): never`、`model User`、`model PublicUser extends User`（`Model.extends.base`）、`scalar Email extends string`、`exception InvalidCredentials: Violations`、`effect createSession on sessions.post`、`effect placeOrder on Checkout: Shopping -> Ordered`、`decorator validated throws ClientError prevents effects`、`decorator find(table: any state, column: string) reads table`、`decorator effects(...targets: any effect) effects targets`（残りの引数は `...名前: 型`。節は `throws`、`catches`、`reads`、`effects` の順にまとめて書く）、`decorator insert(table: any state) for effect`、`state users: list<User>`、`classify WarrantyKind of server.Item`、`classify Registration of findUser`（op の分類）、`classify Checkout of carts, orders`、フィールドなら `token: Token?`、`as` を持つケースなら `Registered: findUser as type.User`。名前は見ている名前空間から見た修飾（同じなら修飾なし）。
   - 型は `T?` と `T[]` で書く。状態の見出しだけは、形の語で書く（`State.shape` と `State.element` から `list<T>` / `nullable<T>` / `single<T>`）。
   - 配線の値の位置の状態の名前は、ほかの宣言への参照と同じく、その状態の見出し。アームの中で絞り込まれていても、見出しは宣言のまま。
   - 継承した model のフィールドは、受け継いだものも `fields` にあるので、同じ形で出る。
2. `description.body`。
3. op なら `throws:` と `effects:` の一覧（`Signature`）。例外ごとの効果の状態（`relations`）が `unknown` でないものを添える。続けて、`Signature.reads` が空でなければ `reads:` の一覧、`Signature.pure` が真なら `純粋` の一行。
4. デコレータの適用なら、適用先ではなくデコレータの宣言の 1〜2 と、固定した引数。

## ドキュメントのシンボル

`DocumentSymbol` の木。名前空間は `Namespace`、その中に宣言。

| 宣言 | `SymbolKind` |
|---|---|
| `model` | `Struct`。フィールドは子の `Field`。構文木から引くので、継承した model は本体に書いたメンバー（`Field` と `Override`）だけが子になる |
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
- **コードアクション**: 診断の `Data` から作る。THROWS-1 → `throws` に `missing` を足す。READS-2 → `reads` を計算した集合に直す。MATCH-2 → 足りないアームを足す。RES-1 で候補があれば置き換える。NS-5 → 同じプロジェクトの op なら `export` を足す。
- **semantic tokens**: 型の位置の名前と値の位置の名前を区別して色を付ける（TextMate では命名規則に頼るしかない部分）。
