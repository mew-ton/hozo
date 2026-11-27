# エディタの対応

## この文書が決めること

この文書の細部は、決定 D727、D762 が決めた。

VS Code 拡張（`editors/vscode/`）の中身と、TextMate の文法。要件の持ち主は `docs/requirements.md`「シンタックスハイライト」「言語サーバー」。

## VS Code 拡張

```
editors/vscode/
  package.json                 contributes: languages / grammars / configuration
  language-configuration.json  コメント（// と /* */）、括弧、自動閉じ
  syntaxes/hozo.tmLanguage.json
  syntaxes/generate.go         go generate の置き場（internal/tools/tmgrammar を呼ぶ）
  src/extension.ts             クライアント
  src/trust.ts                 プラグインを起動してよいかを決める関数（vscode に依らない）
  test/                        TextMate の文法のテスト（vscode-textmate で実物の .hozo を字句に分ける）と、package.json の宣言と src/trust.ts のテスト
```

- 言語 ID は `hozo`、拡張子 `.hozo`。
- クライアントは `vscode-languageclient` で `hozoc lsp` を起動する。`hozoc` の場所は設定 `hozo.hozocPath`（既定は `PATH` の `hozoc`）。見つからなければ、エラーの通知と、設定を開くボタン。バイナリの同梱と取得は段階 4 の後。
- `hozo:` スキーム（`docs/ir.md`「Program」の `Span.file`）の `TextDocumentContentProvider` を登録し、要求 `hozo/vocabulary` で内容を取る（`docs/lsp.md`「プロジェクト」）。
- 設定 `hozo.trace.server`（LSP の通信のログ）。
- **プラグインを起動してよいと、言語サーバーに言うのは、ワークスペースが信頼されていて、設定 `hozo.pluginChecks` が入のときだけ**（決定 D412。口は `docs/lsp.md`「プラグインの診断」の「許し」）。
  - 設定 `hozo.pluginChecks`（既定は、入）: 信頼したワークスペースでも、エディタからの、プラグインの check を、切れる。
  - 起動のとき、`vscode.workspace.isTrusted` と、設定から決めて、`initializationOptions` に渡す。
  - 動いている間に、ワークスペースが信頼されたとき（`onDidGrantWorkspaceTrust`）と、設定 `hozo.pluginChecks` が変わったときは、`workspace/didChangeConfiguration` で、渡し直す（サーバーは、起動し直さない）。
  - `package.json` の `capabilities.untrustedWorkspaces` は `limited`: 信頼していないワークスペースでは、シンタックスハイライトと、hozoc 自身の診断・定義へジャンプ・ホバーは働き、プラグインの診断は、出ない。設定 `hozo.hozocPath` は、`restrictedConfigurations` に入れる: 信頼していないワークスペースの設定（`.vscode/settings.json`）が、起動する実行ファイルを、決められない。
- 言語の知識（キーワード、検査）は持たない。

この節の細部は、決定 D682 が決めた。

## TextMate の文法

`scopeName` は `source.hozo`。パターンと scope:

| 対象 | scope | 決め方 |
|---|---|---|
| 予約語（宣言の頭） | `keyword.declaration.hozo` | `token.Keywords` から生成 |
| 予約語（修飾: `export`、`override`） | `storage.modifier.hozo` | 同上 |
| 予約語（節の頭、文の頭。`reads`、`as`、`throw`、`as` の後ろと `match` のアームの頭の `else`、状態の遷移の `any`、行の遷移（`docs/grammar.md`「exception / effect / read」の `RowClause`）の `else`、絞り込んだ型の `on`（同「型」の `OnType`）を含む） | `keyword.control.hozo` | 同上。どの生成規則も使わない予約語 `default`（`docs/grammar.md`「予約語」）は除く: 予約語として塗らず、フィールド名とデコレータ名の位置の規則（下の行）だけが拾う |
| 核の型と形の名前（`docs/grammar.md`「予約語」の 13 語。予約語ではない） | `support.type.primitive.hozo` | `token.TypeNames` から生成した、専用のパターン。**型の位置の規則（下の「型の位置の名前」の行が言う位置）の中でだけ**塗る: 名前の位置（引数、`val`、関数の名前、配線の値）の同じ語は、ふつうの名前で、scope を付けない。`:` の後ろは、値の位置（レコードリテラル `{ at: time }`）でも、型の位置として読むので、そこでは、値の名前でも塗られる（文法は、二つを見分けない） |
| `true` / `false` / `none` | `constant.language.hozo` | 同上 |
| フィールド名の位置の予約語（`op: string;`、`{ op: x }`、`res.op`） | `variable.other.property.hozo` | `:` の前か `.` の後ろの予約語。予約語のパターンより先に拾い、予約語として塗らない（`docs/grammar.md`「識別子」の `FieldName`）。`override: string;` の `override` もここに当たる（`:` の前） |
| `override` の後ろの予約語（`override op;`、`override to: never;`） | `variable.other.property.hozo` | `\boverride\s+` の直後の一語。フィールド名の位置（`docs/grammar.md`「model」の `Override`）なので、予約語として塗らない。`override` そのものは修飾の予約語（`storage.modifier.hozo`）。予約語でないフィールド名には scope を付けない |
| 文脈キーワード `outer` | `keyword.other.hozo` | 直後が `throws` のときに限る（節 `outer throws`。`docs/grammar.md`「decorator の宣言」）。ほかの位置では、ふつうの名前 |
| 文脈キーワード `field` / `variant` | `keyword.other.hozo` | `\bfor\s+` の後と `\bany\s+` の後に限る（`docs/grammar.md`「予約語」）。`for` の対象の `effect` と `read` は予約語（関数の種類の語。`docs/grammar.md`「関数の宣言」）で、ほかの対象（`op`、`state` など）と同じ scope。`for` の対象の並び（区切りは `,`）は、節の頭（`throws` / `catches` / `reads` / `effects`）か、`,` のほかの記号で終わる |
| 分類のケースの絞り（`case of …`。`docs/grammar.md`「decorator の宣言」の `CaseKind`） | `case` と `of` は `keyword.control.hozo`。`of` の後ろの `any` は `keyword.control.hozo`、`never` は `support.type.primitive.hozo`、大文字で始まる名前（型か、分類の名前）は `entity.name.type.hozo` | `\bcase\s+of\s+` の直後の一語。`for` の対象の並びの中と、`any` の後ろの、どちらでも。小文字で始まる型（`string` など）は、予約語のパターンが塗る |
| 文脈キーワード `post` / `put` / `delete` | `keyword.other.hozo` | `effects` の節の要素の最後の `.` の後（`effect fn` の宛先 `effects 状態.操作` と、op の節に直に書いた `状態.操作`。`docs/grammar.md`「exception / effect / read」の `BuiltinEffect`、「op」の `EffectElem`）と、`\bon\s+[\w.]+\.` の後（デコレータの引数の絞り `any effect on 状態.操作`。同「decorator の宣言」の `AnyType`）と、`.` の後で直後が `(` のとき（配線で直に呼ぶ `状態.操作(…)`。同「配線」の `Call`）に限る。`effects` の節は、`;` か `{` か次の節の頭で終わる。節の中の `as`（行の遷移と、状態の遷移）は、「型の位置の名前」の `as` の規則で読む。名前の解決をしないので、名前空間で修飾した `post` という名前の効果や op（`ns.post`、`ns.post(…)`）にも同じ scope が付く |
| `->`（状態の遷移）と `=>`（`match` のアーム） | `keyword.operator.arrow.hozo` | 記号そのもの。`->` は、状態の遷移の `as` の後ろに来るので、`as` の後ろの型の位置でも拾う。遷移のケースと `match` のアームのケース（標準の分類の `Empty` / `HasItems`、`None` / `Some` を含む）は予約語ではないので、専用の scope は付けない（`as` の後ろでは「型の位置の名前」の規則が当たる） |
| デコレータ | `entity.name.function.decorator.hozo` | `@` から `DecoratorRef` まで（`docs/grammar.md`「識別子」）。最後の要素が予約語でも（`@http.default`、`@effects`）、一つの字句のまま、予約語としては塗らない。名前付きのデコレータの `=` の後ろの `DecoratorRef`（`decorator all = db.list(orders);`）も同じ scope |
| `...`（残りの引数） | `keyword.operator.rest.hozo` | 記号そのもの（`docs/grammar.md`「decorator の宣言」の `RestParam`） |
| `@repeatable`、`@entry`、`@hint` | `storage.modifier.hozo` | 核の印でデコレータではない（`docs/grammar.md`「予約語」）ので、デコレータより先に拾う |
| 宣言の名前 | `entity.name.type.hozo`（model / scalar / union / exception / classify）、`entity.name.function.hozo`（`fn` / decorator）、`variable.other.hozo`（state） | 宣言の頭の予約語の直後の `Ident`（`exception` は、その後ろに引数の括弧が続いてもよい）。関数（op、`read`、効果）の名前は、`fn` の直後の `Ident`: `read fn userByEmail(…)` の `read` と `fn` は、どちらも、宣言の頭の予約語。`decorator` の直後は `DecoratorName` で、予約語でも（`decorator list(…)`、`decorator effects(…)`）宣言の名前として塗る。関数に付けるデコレータ（`decorator fn limited(…)`）は、`decorator` と `fn` が、どちらも、宣言の頭の予約語で、名前は、`fn` の直後。包んだ関数の引数（`decorator fn audited(next: fn)`。`docs/grammar.md`「デコレータの本体」）は、型の位置の `fn` を、予約語として塗り、名前（`next`）と、その呼び出し（`next()`）は、ほかの引数、ほかの呼び出しと同じ（特別な scope を付けない） |
| 型の位置の名前 | `entity.name.type.hozo` | `:` の後、`extends` / `of` / `as` の後、`<` の中、`throws` / `catches` / `throw` の後、`by` の後（`match x by store.Registration`）、型の後ろの `on` の後（`Order on Stage.Draft | Stage.Accepted` の二つ。`|` は記号で、scope を付けない）の `QualifiedName` のうち、最後の要素が大文字で始まるもの（`as` の後ろの型の位置は、`;` か `{` まで続く: 行の遷移のケース（`as Stage.Cancelled`）と、状態の遷移のケース（`as HasItems -> Empty` の二つ）は、どれも、この規則で、型の位置の名前として塗る。状態の分類のケースの、`,` で続く次の `対象 as T` も、同じ位置の中で読む。§3.3 の命名規則。`:` は値の位置（名前で渡す引数、レコードリテラル）にも現れるので、小文字で始まる名前は型にしない。関数の `)` の後ろの `:`（`docs/grammar.md`「関数の宣言」）の、型の位置は、節の頭（`throws` / `reads` / `effects`）の手前で終わる。`of` の後ろの op や状態の名前、`as` の前の op や状態の名前、`reads` の後ろの `read` や状態の名前は小文字で始まるので、scope を付けない。`throws` / `catches` の並びの `,` は記号で、scope を付けない。`throw` の後ろの型の位置は、引数の `(` の手前で終わり、引数（`throw Invalid("weak")`）は値の位置として塗る） |
| 文字列 | `string.quoted.double.hozo`（複数行の文字列 `"""…"""` は `string.quoted.triple.hozo`） | エスケープは `constant.character.escape.hozo`。複数行の文字列は、`"` より先に拾う |
| 数値 | `constant.numeric.hozo` | `Int` / `Float` / `Duration`（単位も含む） |
| リテラルの型（`docs/grammar.md`「型」の `LiteralType`: `severity: "error";`、`boolean as true`） | 上の、文字列、数値、`true` / `false` の scope のまま | リテラルのパターンが、型の位置の名前のパターンより先に拾う。型の名前（`entity.name.type.hozo`）としては、塗らない |
| コメント | `comment.line.double-slash.hozo`、`comment.block.hozo` | |
| ドキュメンテーションコメント | `comment.block.documentation.hozo` | 中の `@param` / `@throws` / `@see` は `storage.type.class.jsdoc`、続く名前は `variable.other.jsdoc`。行の頭の、デコレータのタグ（`@adr.context`。`docs/grammar.md`「ドキュメンテーションコメント」）は、`@` と名前を、宣言の前のデコレータと同じ scope（`entity.name.function.decorator.hozo`） |

- 予約語の部分は `internal/tools/tmgrammar` が `token.Keywords` から生成する（`docs/packages.md`「生成するもの」）。それ以外のパターンも同じ道具の中（`grammar.go`）に手で書き、ファイル全体を `go generate ./editors/...`（`syntaxes/generate.go`）で決定的に生成する。生成物とのずれは `internal/tools/tmgrammar` のテストと CI が見る（`docs/testing.md`「生成物の一致」）。
- `is`、`omit`、`condition` は予約語ではない（`docs/grammar.md`「予約語」）ので、どのパターンにも現れない。
- 文法のテストは `editors/vscode/test/`。`vscode-textmate` + `vscode-oniguruma` で、コーパス・標準の語彙・`spec/cli.hozo` を字句に分け、選んだ字句の scope がこの表の通りであることを `npm test` で確かめる。
- 型の位置と値の位置の区別は TextMate では位置の規則でしか付けられない。残りは命名規則（§3.3）に頼り、正確な区別は段階 4 の後の semantic tokens に任せる（`docs/lsp.md`）。

この節の細部は、決定 D594、D730 が決めた。

## ほかのエディタ

Neovim / Zed などは各エディタの設定で `hozoc lsp` を起動する（`docs/requirements.md`「言語サーバー」）。linguist への登録は段階 4 の後に検討する。
