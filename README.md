# hozo

Hozo は、プログラムを「状態」と「入出力」だけで記述する仕様言語。このリポジトリはその実装のモノレポで、仕様を検査して各言語のコードに展開する CLI `hozoc`、IDE 向けの言語サーバー（`hozoc lsp`）、エディタの対応（シンタックスハイライト）、プラグイン、標準の語彙を持つ。本体と言語サーバーは Go。

- 設計書: `mew-ton/drafts` の `hozo/README.md`（https://github.com/mew-ton/drafts/blob/main/hozo/README.md ）
- 設計文書: [`docs/README.md`](docs/README.md)（要件、文法、IR、検査、既定の出力、プラグイン、CLI、言語サーバー、テスト）

## 開発

```
make build   # bin/ に hozoc を作る
make test
```

## 状態

詳細設計の段階（`docs/requirements.md`「段階」の 0）。`hozoc` のサブコマンドは未実装。
