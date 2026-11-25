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
| `initialize` | `rootUri`（なければ `workspaceFolders` の先頭）をワークスペースにし、下の「プロジェクト」を作る。`initializationOptions` から、プラグインを起動してよいかを読む（「プラグインの診断」の「許し」）。capabilities を返す |
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

`hozoc build --no-emit` と同じ `internal/check` の呼び出しなので、同じ入力に同じ診断が出る（`docs/testing.md`「言語サーバーと check の一致」）。設定とマニフェストだけで決まる診断（CFG-6、CFG-26 など。`docs/config.md`「診断」）も、プラグインを起動せずに、解析のたびに出る。違いは、二つ。

- プラグインの診断（`docs/cli.md`「検査」の 4。HTTP-5 など、語彙のプラグインが返す誤りと、GOHTTP-16 など、トランスパイラが返す誤りと、warning）は、解析のたびには出さず、後から足す（「プラグインの診断」）。
- 言語サーバーだけが出す診断が、二つある:

| ID | 重さ | 意味 |
|---|---|---|
| LSP-1 | warning | `hozo.toml` が見つからない。設定の検査は行っていない（「プロジェクト」）。ユーザーの各 `.hozo` の 1 行 1 列に出す |
| LSP-2 | warning | プラグインの check が、最後まで走らなかった（プラグインの異常終了、形の違う応答。`docs/plugin-protocol.md`「応答の形」）。プラグインの診断は、出ていない。`hozo.toml` の 1 行 1 列に出す。メッセージに、その誤りを入れる |

この節の細部は、決定 D680 が決めた。

### プラグインの診断

決定 D405、D412。プラグインの診断（`docs/cli.md`「検査」の 4）は、プラグインを起動する（頼むのは、`hozoc build --no-emit` と同じ: 語彙のプラグインへの `read` と、トランスパイラへの `emit`。使うのは、応答の診断だけで、ファイルは、書かない: `docs/plugin-protocol.md`「語彙のプラグイン」「起動」）ので、解析より、桁が違って遅い: サンプル `shop-server` で、解析が 5 ミリ秒、プラグインを足すと、13.7 秒（実測。同「起動」の「速さ」。言語サーバーは、途中で止められる形で、動かすので、`hozoc build --no-emit` の 3.7 秒より、遅い）。なので、解析と、分ける。この見出しは、プラグインを起動して、診断を求めることを、「プラグインの check」と呼ぶ。

- **プラグインを起動するのは、クライアントが、許したときだけ**（同 412。下の「許し」）。既定は、許していない: プラグインは、どのきっかけでも、起動せず、出るのは、hozoc 自身の診断だけ。この見出しの、ほかの決まりは、許されているときのもの。
- **解析（「プロジェクト」の、解析し直し）は、プラグインを起動しない。** hozoc 自身の診断を、すぐ出す。
- **プラグインの check は、裏で走らせる。** 走っている間も、要求（ホバー、定義へジャンプ、次の解析）は、待たない。終わったら、hozoc 自身の診断に、プラグインの診断を足したもの（`hozoc build --no-emit` と同じ診断）を、出し直す。
- **走らせるきっかけ**は、解析の中身（下の「中身」）が、ディスクに書かれたと分かるとき:

| きっかけ | 走らせる条件 |
|---|---|
| `didSave` | いつも（同じ中身の check が、もう走っていれば、それを待つ） |
| `didOpen`、`workspace/didChangeWatchedFiles` | 今の中身の、プラグインの診断を、まだ持っていないとき（同じ中身の check が、走っていれば、それを待つ） |
| `didChange`（打鍵）、`didClose`、`initialized` | 走らせない |

- **走らせないとき**: クライアントが、許していない（「許し」）。`hozo.toml` がない。hozoc 自身の診断に、error がある（`docs/plugin-protocol.md`「起動」: 検査を通っていない仕様では、起動しない）。起動する相手（同「語彙のプラグイン」: 仕様が使っている語彙のプラグイン。同「起動」: 受け持つ出力のある、使うプラグイン）が、一つもない。
- **中身**は、解析が読んだ、すべてのファイル（ユーザーのファイル、語彙のファイル）と、`hozo.toml` の内容。開いている文書は、エディタの中の内容（「プロジェクト」の `Overlay`）で、保存していなくても、それを読む。プラグインの check も、同じものを読む。
- **プラグインの診断は、それを求めた中身と、今の中身が同じ間だけ、出す。** 中身が変わったら（保存の後に、編集した。別の保存が来た）、プラグインの診断は、消える（hozoc 自身の診断だけに戻る）: 位置が、ずれた診断を、残さない。クライアントが、診断の位置を、編集に合わせて動かすことは、当てにしない。中身が、もとに戻れば（編集を、取り消した）、また出す。
- **裏の check は、一度に一つ。** 走っている間に、中身が変わったら、走っている check を、やめさせる（動いているプラグインも、その場で止める: `docs/plugin-protocol.md`「起動」）。やめさせた check の答えは、使わない。次の check は、前の check が終わってから、始める。待たせるのは、最後に求めた一つだけ。
- **古い答えは、出さない。** 終わった check の答えが、今の中身のものでなければ、出さない（覚えておき、中身が、そこに戻ったときに、出す）。
- **hozoc 自身の診断に、error があるとき**は、プラグインを起動しない。そのとき、前のプラグインの診断は、出ない（error を足した時点で、中身が変わっている）。
- **プラグインの check が、最後まで走らなかったとき**（異常終了、形の違う応答）は、そこまでに返った診断と、LSP-2 を出す。サーバーは、止まらない。誤りは、ログにも残す。
- **終わるとき**（`exit`、入力の終わり）は、走っている check を、やめさせて、それが止まるのを待つ（動いているプラグインは、その場で止まる。5 秒で、待つのをやめる）。

- **`didSave` のために、クライアントに、足すものは、ない。** capabilities の `save`（「ライフサイクル」）で、送られる。

決めていないこと（`docs/open.md`「文書と道具」）: 打つのを止めてから、少し後に、走らせる形。プラグインの check の速さ。既定を、許す、に変えるか。

この節の細部は、決定 D677、D678、D679、D681、D699 が決めた。

#### 許し

決定 D412、D426。プラグインを起動してよいかは、クライアントが、言う。この形は、プラグインが、利用者と同じ権限で動いていた間に、決めた: エディタで、ファイルを開いただけで、プロジェクトの中のマニフェストが決めたコードが、動かないようにするため。今は、プラグインは、サンドボックスの中で動く（`docs/plugin-protocol.md`「信頼の前提」）が、この形は、変えていない（同 426）。既定を変えるかは、決めていない（`docs/open.md`「文書と道具」）。

| 口 | 形 | いつ |
|---|---|---|
| `initialize` の `initializationOptions` | `{ "pluginChecks": true }` | 起動のとき |
| 通知 `workspace/didChangeConfiguration` の `settings` | `{ "hozo": { "pluginChecks": true } }` | 動いている間に、変わったとき |

- **既定は、許していない。** `initializationOptions` がない、`pluginChecks` がない、真偽値でない、は、どれも、許していない、と読む（誤りには、しない。真偽値でなければ、ログに残す）。
- **`workspace/didChangeConfiguration`** は、`settings.hozo.pluginChecks` が、真偽値のときだけ、許しを、その値に変える。なければ、変えない。ほかの項目は、読まない。
- **許しが、与えられたとき**: 今の中身について、プラグインの check を、求める（`didOpen` と同じ条件: 今の中身の診断を、まだ持っていないとき）。
- **許しが、なくなったとき**: 走っている check を、やめさせ、待たせている check を、捨てる。覚えている答えも、捨てて、診断を、hozoc 自身のものだけで、出し直す（プラグインの診断と、LSP-2 が、消える）。その後に届いた、裏の check の答えは、使わない。
- **VS Code 拡張**が、いつ、許すかは、`docs/editors.md`「VS Code 拡張」。
- **ほかのクライアント**から `hozoc lsp` を使うときは、プロジェクトを信頼しているときに、起動の設定で、`initializationOptions` に `{ "pluginChecks": true }` を渡す。渡さなければ、プラグインの診断は、出ない（`hozoc build --no-emit` では、出る）。

この節の細部は、決定 D681 が決めた。

## 定義へジャンプ

カーソル位置のトークンが名前（`Ident`、または `QualifiedName` か値の位置の `Path` の一部。`docs/grammar.md`「識別子」）なら、`internal/sema` の `Ref`（`docs/packages.md`「internal/sema」）の `Target` の `Span` を `Location` で返す。

| カーソルの位置 | 飛び先 |
|---|---|
| 宣言への参照（型、`throws`、デコレータ名、呼び出しの op・`read`・効果など） | その宣言の名前のトークン |
| 配線で直に呼ぶ `状態.操作(…)` | 状態の名前のセグメントは `state` の名前。操作の語（`post` / `put` / `delete`）は宣言を持たないので `null` |
| `exception`、`effect`、`read` の引数の型、`read` の結果の型 | その型の宣言 |
| 効果の `throws` の例外（`effect fn e(…): throws E effects s.post` の `E`） | `exception` の名前（op の `throws` と同じ） |
| `model D extends B` の `B` | model `B` の名前 |
| `override f` の `f` | 元の model の `fields` の、そのフィールドの位置（`Field.span`。`docs/ir.md`「model / scalar / union」。元の model が受け継いだままのフィールドなら、さらに元の宣言の位置になる） |
| `classify C of op` の `op`、ケースの `op as T` の `op` | その op の名前 |
| ケースの `X as T` の `X`（状態、型）と `T` | 状態なら `state` の名前、型ならその宣言。残りのケースの `X as else` の `else` と、ない側のケースの `X as none` の `none` は宣言を持たないので `null` |
| 行の遷移のケース `C.K`（`effect fn e(…): effects s.put as C.K`）と、絞り込んだ型のケース（`order: Order on C.K`） | `C` のセグメントは `classify` の名前、`K` のセグメントは、その `classify` のケース。`as else` の `else` は宣言を持たないので `null` |
| `reads` の要素 | `read` の名前（名前で書いた要素）か、`state` の名前（直に書いた要素） |
| `effects` の要素 | `effect` か効果の `union` の名前（名前で書いた要素）。直に書いた `状態.操作` は、状態の名前のセグメントが `state` の名前、操作の語（`post` / `put` / `delete`）は宣言を持たないので `null` |
| `read fn r(…): T reads s` の `s` | `state` の名前 |
| デコレータの宣言の `reads` / `effects` の節の名前 | op の節と同じ。デコレータの引数の名前（残りの引数を含む）なら、その `Param` |
| 予約語のデコレータ名（`@http.default`、`@effects`、別名の `=` の後ろ） | ほかのデコレータ名と同じく、その `decorator` の名前 |
| 配線の値の位置の、状態の名前（`f(token)`、`match (token by nullable)`） | `state` の名前。同じ名前の `val`、`for` の変数、引数があれば、そちら（`docs/resolve.md`「手続き」） |
| `match (x by ns.C)` の `C` | `classify` の名前 |
| アームの `throw E`、`throw E(…)` の `E` | `exception` の名前。引数の値は、配線のほかの値と同じ |
| `a.b.c` の途中のセグメント | そのセグメントまでで決まるもの（名前空間なら、その名前空間の最初のブロックの名前） |
| フィールドの参照 `x.f` | フィールドの宣言 |
| `val` の名前、`for` の変数（値の位置。値を入れる文の左 `x = …`、集める文の左 `xs collect …` も） | `val` の宣言の名前、`for` の頭の名前 |
| `catch` が受けた例外の名前（`match (e by …)` の対象、`throw e`） | `catch (e)` の頭の名前。ホバーは、op の見出しと `catch (e)` の一行 |
| `catch (e by C)` と `match (e by C)` の分類の名前、アームのケースの名前 | ほかの `match` と同じ（分類の宣言、そのケース） |
| デコレータの引数の、分類のケース（`@prd.usecase.of(Actor.Customer)`） | `Actor` のセグメントは `classify` の名前、`Customer` のセグメントは、その `classify` のケース |
| デコレータの宣言の、分類のケース（`case of Actor`、`case of Order`）の、`of` の後ろの名前 | `classify` の名前か、型の宣言。`case`、`of`、`any`、`never` の語は、宣言を持たないので `null` |
| 引数名 | 引数 |
| `match` のケース名 | `classify` のケース。標準の分類（`by list` / `by nullable`）のケース（`Empty` / `HasItems`、`None` / `Some`）と、`by` の後ろの `list` / `nullable` は宣言を持たないので `null`。アームの頭の `else` も、一つのケースを指さないので `null` |
| デコレータの引数名（`name:`） | デコレータの宣言の `Param` |
| ドキュメンテーションコメントの `@param` / `@throws` / `@see` の名前 | 同上（`internal/sema` の `Refs` にはないので、言語サーバーが構文木から解決する） |
| ドキュメンテーションコメントの中の、デコレータのタグの名前（`@adr.context`） | デコレータの宣言（宣言の前に書いた適用の名前と、同じ扱い。ホバーも、同じ） |
| 語彙の宣言 | `hozo:` の URI の位置 |

解決できなければ `null`。

この節の細部は、決定 D559、D762 が決めた。

## ホバー

Markdown の `MarkupContent`。順に:

1. Hozo の構文で書いた宣言の見出し（コードブロック、言語 `hozo`）。IR から組み立てる: `fn login(in: LoginInput): LoginOk`、`export fn login(in: LoginInput): LoginOk`（`Op.exported`）、`fn unauthorized(): never`、`model User`、`model PublicUser extends User`（`Model.extends.base`）、インデックスの model なら `model Attrs { [name: string]: string[] }`（`Model.index`）、`scalar Email extends string`、`exception InvalidCredentials`、`exception LimitExceeded(wait: duration)`、`effect fn createSession(session: Session on Stage.Accepted): effects sessions.post`、`effect fn placeOrder(): effects Checkout as Shopping -> Ordered`、例外を投げる効果なら `effect fn createUser(user: User): throws EmailTaken effects users.post`（`Effect.throws`。例外は、見ている名前空間から見た名前）、行の遷移を持つ効果なら `effect fn startProduction(order: Order): effects orders.put as Stage.InProduction`（`Effect.row` から、`docs/plugin-go.md`「コメント」の「行の遷移の表示」と同じ形で組み立てる。分類の名前は、見ている名前空間から見た修飾）、`read fn userByEmail(email: Email): User? reads users`、`decorator validated throws ClientError prevents effects`、`decorator limit(counter: any state) reads counter`、`decorator effects(...targets: any effect) effects targets`（残りの引数は `...名前: 型`。節は `throws`、`catches`、`reads`、`effects` の順にまとめて書き、並びの区切りは `,`）、`decorator insert for effect`、`decorator find for read`、`decorator human for case of Actor`、`decorator of(who: any case of never)`、`decorator note for case of any`（分類のケースは、`Decorator.caseTargets` と、`ParamType` の `case` の絞りごとに、もとの形 `case of …` で書く。絞りがなければ `case of any`）、`@repeatable decorator where(column: string, value: string | int | float | boolean) for read`（核の印 `@repeatable` / `@entry` / `@hint` は、`decorator` の前に、この順で。`outer throws` の節は、`throws` の節の後ろに）、`state users: list<User>`、`classify WarrantyKind of server.Item`、`classify Registration of findUser`（op の分類）、`classify Checkout of carts, orders`、フィールドなら `token: Token?`、`as` を持つケースなら `Registered: findUser as type.User`、残りのケース（`docs/check.md`「残りのケース」）なら `Unregistered: findUser as else`、`as none` のケース（同「ない側のケース」）なら `Absent: Order? as none`。名前は見ている名前空間から見た修飾（同じなら修飾なし）。
   - 型は `T?` と `T[]` で書く。状態の見出しだけは、形の語で書く（`State.shape` と `State.element` から `list<T>` / `nullable<T>` / `single<T>`）。絞り込んだ型（`case`）の形は、`docs/types.md`「型の種類」の「絞り込んだ型」。リテラルの型（`literal`）は、仕様の綴りで書く（`severity: "error"`、`times: 3`、`boolean as true`。`docs/grammar.md`「型」の `LiteralType`）。
   - デコレータの見出しは、宣言の形のまま（`docs/grammar.md`「decorator の宣言」）: 関数に付けるデコレータは `decorator fn limited(): throws LimitExceeded prevents effects`（節は、`:` の後ろに、`throws`、`catches`、`reads`、`effects` の順。本体は、出さない）。そのほかは `decorator table for state`。
   - デコレータの本体の中の名前（呼び出し、`val`、値の引数）も、op の配線の中と同じに、ホバーと、定義への移動が働く。値の引数の定義は、そのデコレータの宣言の引数。包んだ関数の引数（`docs/grammar.md`「デコレータの本体」）の呼び出しも、定義は、宣言のその引数で、ホバーは、見出し（デコレータの宣言。付ける側が書く引数だけ）と、`<名前>: fn` の一行。
   - 配線の呼び出しの名前は、呼び出し先の宣言（op、`read`、`effect`）の見出し。直に呼ぶ `状態.操作(…)` の状態のセグメントは、その状態の見出し。
   - 配線の値の位置の状態の名前は、ほかの宣言への参照と同じく、その状態の見出し。アームの中で絞り込まれていても、見出しは宣言のまま。
   - 継承した model のフィールドは、受け継いだものも `fields` にあるので、同じ形で出る。
2. `description.body`。
3. op なら `throws:` と `effects:` の一覧（`Signature`）。例外ごとの効果の状態（`relations`）が `unknown` でないものを添える。効果は、宣言なら見ている名前空間から見た名前、直に書いた効果なら `状態.操作`。続けて、`Signature.reads` が空でなければ `reads:` の一覧（基底の状態）、`Signature.pure` が真なら `純粋` の一行。どれも、計算した結果（`docs/check.md`「計算」）で、配線の op が、`throws`、`reads`、`effects` の節を書いていなくても、出る: 節を書かなくてよい op（同「状態に触れるもの」の「節に書く要素」）の関数の型を、見る所。
4. デコレータの適用なら、適用先ではなくデコレータの宣言の 1〜2 と、固定した引数。

この節の細部は、決定 D587、D594、D727 が決めた。

## ドキュメントのシンボル

`DocumentSymbol` の木。名前空間は `Namespace`、その中に宣言。

| 宣言 | `SymbolKind` |
|---|---|
| `model` | `Struct`。フィールドは子の `Field`。構文木から引くので、継承した model は本体に書いたメンバー（`Field` と `Override`）だけが子になる。インデックスの model は、子を持たない |
| `scalar` | `TypeParameter` |
| `union` | `Enum`。アームは子の `EnumMember` |
| `state` | `Variable` |
| `op` | `Function` |
| `exception` | `Class` |
| `effect` | `Event` |
| `read` | `Key` |
| `classify` | `Enum`。ケースは子の `EnumMember` |
| `decorator` | `Interface` |
| `bind` | `Property` |

## 段階 4 の後

- **補完**: 名前の位置で、`docs/resolve.md`「位置ごとに受け付ける種類」に合う名前。別の名前空間のものは修飾した形。
- **参照の検索**: 宣言から、それを参照しているトークンすべて。
- **コードアクション**: 診断の `Data` から作る。MATCH-2 → 足りないアームを足す。RES-1 で候補があれば置き換える。NS-5 → 同じプロジェクトの op なら `export` を足す。 配線の op の節（`throws`、`reads`、`effects`）を直すコードアクションは、ない: 節と、計算した分を、照合する診断が、なくなった（決定 D381。THROWS-1、READS-2 は欠番）。
- **semantic tokens**: 型の位置の名前と値の位置の名前を区別して色を付ける（TextMate では命名規則に頼るしかない部分）。

この節の細部は、決定 D652 が決めた。
