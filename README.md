# hozo

Hozo は、プログラムを「状態」と「入出力」だけで記述する仕様言語。このリポジトリはその実装のモノレポで、仕様を検査して各言語のコードに展開する CLI `hozoc`、IDE 向けの言語サーバー（`hozoc lsp`）、エディタの対応（シンタックスハイライト）、プラグイン、標準の語彙を持つ。実装言語は `docs/requirements.md`「実装言語」。

- 仕様と設計の文書: [`docs/README.md`](docs/README.md)（要件、文法、IR、検査、プラグインと Go の基底、CLI、言語サーバー、テスト）。言語の仕様の持ち主は、この `docs/`
- 文書が「設計書」と呼ぶもの: `mew-ton/drafts` の `hozo/README.md`。この言語の出発点になった、粗いアイデアの集まりで、保守されておらず、仕様の持ち主ではない（`docs/README.md`「設計書という呼び名」）

## 開発

```
make build      # bin/ に hozoc を作る
make test
make generate   # cli.hozo から internal/gen/（hozoc と proc）を生成し直す（docs/packages.md「自己生成の手順」）
```

## 状態

段階 4 までと、その後の作り直しの段階 R1〜R4 まで（`docs/requirements.md`「段階」「作り直しの段階」）。`hozoc check` / `hozoc emit` / `hozoc lsp` が動く。hozoc 本体は特定の言語のコードを持たず、Go のコードは、プラグイン（基底 `go` と、それに相乗りする `go-valid`、`go-sqlite`、`go-http`、`go-cli`。`plugins/`）が出す。`cmd/hozoc` の骨組みは、`cli.hozo` から `go` + `go-valid` + `go-cli` で生成したもの（`internal/gen/hozoc` と `internal/gen/proc`。`make generate` で生成し直す）。VS Code 拡張は `editors/vscode`。次は段階 5（TS の基底のプラグインと、TS 向けの一次プラグイン）。
