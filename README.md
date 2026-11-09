# hozo

Hozo は、プログラムを「状態」と「入出力」だけで記述する仕様言語。このリポジトリはその実装のモノレポで、仕様を検査して各言語のコードに展開する CLI `hozoc`、IDE 向けの言語サーバー、エディタの対応（シンタックスハイライト）、プラグイン、標準の語彙を持つ。

- 設計書: `mew-ton/drafts` の `hozo/README.md`（https://github.com/mew-ton/drafts/blob/main/hozo/README.md ）
- 要件・リポジトリの構成・段階: [`docs/requirements.md`](docs/requirements.md)

## 状態

実装前。最初の段階（`hozoc check` とシンタックスハイライト）に入る前に決めることは `docs/requirements.md` の「最初に決めること」にある。
