# hozoc

## この文書が決めること

`hozoc` の振る舞いと実装が、どこに書いてあるか。この文書が自分で持つのは、`hozoc tests` の診断のメッセージの形（「診断」）だけ。

コマンドとユースケースの振る舞いの持ち主は、hozoc 自身の仕様（「cli.hozo」）。

葉の op と、状態の単位の実装（手で書くもの）の持ち主は、`internal/app` のコードとコメント（パッケージ `app` の説明）と、`cmd/hozoc/main.go`（決定 D809）。

## cli.hozo

hozoc 自身の仕様の持ち主は、`spec/cli.hozo`（決定 D807、D808）。宣言の意味と、決定に書いていない理由は、その宣言の説明のコメントが持つ。形を決めた決定は、その宣言の `@adr.decided` が名指しする。この文書は、写しを置かない。

`spec/cli.hozo` から生成する単位（名前空間と、トランスパイラと、置き場）は、リポジトリ直下の `hozo.toml` が持つ。

この文書と `docs/` のほかの文書が、修飾なしに `cli.hozo` と書けば、`spec/cli.hozo` を指す。go-cli プラグインの語彙のファイルは、`plugins/go-cli/cli.hozo` と書く（`plugins/go-cli/hozo-plugin.toml`）。

`spec/` のほかのファイルが何を書き、持ち主がどこで、コードになるかは、それぞれのファイルの頭のコメントが言う。

## 診断

`hozoc tests`（`spec/cli.hozo` の `confirmTests`）が出す診断（TESTS-*）の、ID と条件と重さの持ち主は、`spec/diagnostics/hozoc.hozo` の分類 `hozo.diagnostics.Hozoc` のケース。

- メッセージは、テストを `<トランスパイラの名前>: <テストの名前>` で名指す（`go-http: server.login/OK`）。同じ名前のテストが、二つのトランスパイラの対応表にあってよい（葉の op に経路を付ければ、単体テストと、インテグレーションテストが、同じ `<op の FQN>/<結果>` になる）。
