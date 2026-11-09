# エディタの対応

## この文書が決めること

VS Code 拡張（`editors/vscode/`）の中身と、TextMate の文法。要件の持ち主は `docs/requirements.md`「シンタックスハイライト」「言語サーバー」。

## VS Code 拡張

```
editors/vscode/
  package.json                 contributes: languages / grammars / configuration
  language-configuration.json  コメント（// と /* */）、括弧、自動閉じ
  syntaxes/hozo.tmLanguage.json
  src/extension.ts             クライアント
```

- 言語 ID は `hozo`、拡張子 `.hozo`。
- クライアントは `vscode-languageclient` で `hozoc lsp` を起動する。`hozoc` の場所は設定 `hozo.hozocPath`（既定は `PATH` の `hozoc`）。見つからなければ、エラーの通知と、設定を開くボタン。バイナリの同梱と取得は段階 4 の後。
- `hozo:` スキーム（`docs/ir.md`「Program」の `Span.file`）の `TextDocumentContentProvider` を登録し、要求 `hozo/vocabulary` で内容を取る（`docs/lsp.md`「プロジェクト」）。
- 設定 `hozo.trace.server`（LSP の通信のログ）。
- 言語の知識（キーワード、検査）は持たない。

## TextMate の文法

`scopeName` は `source.hozo`。パターンと scope:

| 対象 | scope | 決め方 |
|---|---|---|
| 予約語（宣言の頭） | `keyword.declaration.hozo` | `token.Keywords` から生成 |
| 予約語（節の頭、文の頭） | `keyword.control.hozo` | 同上 |
| 核の型 | `support.type.primitive.hozo` | 同上 |
| `true` / `false` / `none` | `constant.language.hozo` | 同上 |
| 文脈キーワード `field` / `variant` | `keyword.other.hozo` | `\bfor\s+` の後と `\bany\s+` の後に限る（`docs/grammar.md`「予約語」） |
| 文脈キーワード `post` / `put` / `delete` | `keyword.other.hozo` | `\bon\s+[\w.]+\.` の後に限る |
| デコレータ | `entity.name.function.decorator.hozo` | `@` から `QualifiedName` まで |
| 宣言の名前 | `entity.name.type.hozo`（model / scalar / union / exception / classify）、`entity.name.function.hozo`（op / effect / decorator）、`variable.other.hozo`（state） | 宣言の頭の予約語の直後の `Ident` |
| 型の位置の名前 | `entity.name.type.hozo` | `:` の後、`extends` / `of` の後、`<` の中、`throws` / `catches` の後の `QualifiedName` |
| 文字列 | `string.quoted.double.hozo` | エスケープは `constant.character.escape.hozo` |
| 数値 | `constant.numeric.hozo` | `Int` / `Float` / `Duration`（単位も含む） |
| コメント | `comment.line.double-slash.hozo`、`comment.block.hozo` | |
| ドキュメンテーションコメント | `comment.block.documentation.hozo` | 中の `@param` / `@throws` / `@see` は `storage.type.class.jsdoc`、続く名前は `variable.other.jsdoc` |

- 予約語の部分は `internal/tools/tmgrammar` が `token.Keywords` から生成する（`docs/packages.md`「生成するもの」）。手で書くのはそれ以外のパターン。
- 型の位置と値の位置の区別は TextMate では位置の規則でしか付けられない。残りは命名規則（§3.3）に頼り、正確な区別は段階 4 の後の semantic tokens に任せる（`docs/lsp.md`）。

## ほかのエディタ

Neovim / Zed などは各エディタの設定で `hozoc lsp` を起動する（`docs/requirements.md`「言語サーバー」）。linguist への登録は段階 4 の後に検討する。
