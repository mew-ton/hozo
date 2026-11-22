# Hozo for VS Code

Hozo の仕様（`.hozo`）のシンタックスハイライトと、言語サーバー `hozoc lsp` への接続。設計は `docs/editors.md`（「VS Code 拡張」「TextMate の文法」）。

## 使い方

- `hozoc` を PATH に置くか、設定 `hozo.hozocPath` に場所を書く。見つからなければエラーの通知と「設定を開く」ボタンが出る。
- `hozo.trace.server` で VS Code と `hozoc lsp` の通信のログを出せる。
- プラグインの診断は、ワークスペースを信頼していて、設定 `hozo.pluginChecks`（既定は、入）が入のときだけ、出る（ファイルを開いたときと、保存したときに、プラグインを起動する）。信頼していないワークスペースでは、プラグインは、起動しない（`docs/editors.md`「VS Code 拡張」）。
- 標準の語彙（`hozo:vocab/*.hozo`）は定義へジャンプの飛び先になる。内容は要求 `hozo/vocabulary` で言語サーバーから取る（`docs/lsp.md`「プロジェクト」）。
- バイナリの同梱と取得は段階 4 の後。

## 開発

```
npm ci
npm run build    # src/ → out/
npm test         # TextMate の文法で実物の .hozo を字句に分け、scope を確かめる
```

`syntaxes/hozo.tmLanguage.json` は手で編集しない。予約語は `internal/syntax/token` の `Keywords` から、それ以外は `internal/tools/tmgrammar/grammar.go` から、リポジトリのルートで `go generate ./editors/...` で生成し直す（`docs/packages.md`「生成するもの」）。
