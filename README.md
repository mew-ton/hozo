# hozo

Hozo は、プログラムを「状態」と「入出力」だけで記述する仕様言語。このリポジトリはその実装のモノレポで、仕様を検査して各言語のコードに展開する CLI `hozoc`、IDE 向けの言語サーバー、エディタの対応（シンタックスハイライト）、プラグイン、標準の語彙を持つ。本体と言語サーバーは Go。

- 設計書: `mew-ton/drafts` の `hozo/README.md`（https://github.com/mew-ton/drafts/blob/main/hozo/README.md ）
- 要件・リポジトリの構成・段階: [`docs/requirements.md`](docs/requirements.md)

## 開発

```
make build   # bin/ に hozoc と hozo-ls を作る
make test
```

## 状態

雛形のみ。`hozoc` のサブコマンドは未実装。次に書くのは `docs/grammar.md`（文法）と `docs/ir.md`（IR）。
