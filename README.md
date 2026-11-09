# hozo

Hozo は、プログラムを「状態」と「入出力」だけで記述する仕様言語。このリポジトリはその実装のモノレポで、仕様を検査して各言語のコードに展開する CLI `hozoc`、IDE 向けの言語サーバー（`hozoc lsp`）、エディタの対応（シンタックスハイライト）、プラグイン、標準の語彙を持つ。実装言語は `docs/requirements.md`「実装言語」。

- 設計書: `mew-ton/drafts` の `hozo/README.md`（https://github.com/mew-ton/drafts/blob/main/hozo/README.md ）
- 設計文書: [`docs/README.md`](docs/README.md)（要件、文法、IR、検査、既定の出力、プラグイン、CLI、言語サーバー、テスト）

## 開発

```
make build   # bin/ に hozoc を作る
make test
```

## 状態

段階 4 まで（`docs/requirements.md`「段階」）。`hozoc check` / `hozoc emit` / `hozoc lsp` が動き、`cmd/hozoc` の骨組みは `cli.hozo` から生成したもの（`internal/gen/hozoc`。`make generate` で生成し直す）。VS Code 拡張は `editors/vscode`。次は段階 5（TS の既定の出力と TS 向けの一次プラグイン）。
