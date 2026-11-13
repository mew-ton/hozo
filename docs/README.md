# 設計文書

Hozo の実装（このリポジトリ）の設計。言語の意味の持ち主は設計書（`mew-ton/drafts` の `hozo/README.md`。各文書の「§n」はその節）で、ここにあるのは実装のための決定。設計書にない決定の一覧は `decisions.md`。

## 読む順

| 文書 | 決めること | 写す場所 |
|---|---|---|
| [`requirements.md`](requirements.md) | 作るもの、段階、自己展開、品質の要件、検査の規則の ID | — |
| [`grammar.md`](grammar.md) | 字句と構文（EBNF） | `internal/syntax` |
| [`ir.md`](ir.md) | IR の構造、正規化、JSON、その名前空間から見た IR | `ir`、`internal/sema` |
| [`resolve.md`](resolve.md) | 名前の解決 | `internal/sema` |
| [`types.md`](types.md) | 型の互換と式の型 | `internal/sema` |
| [`check.md`](check.md) | 例外・効果・`bind` の計算と、規則ごとの検査 | `internal/check` |
| [`config.md`](config.md) | `hozo.toml` | `internal/config` |
| [`vocab.md`](vocab.md) | 標準の語彙 | `vocab/`、`internal/vocab` |
| [`plugin-protocol.md`](plugin-protocol.md) | マニフェスト、探索、JSON のやり取り | `internal/plugin`、`plugin`、`sdk/ts` |
| [`emit-go.md`](emit-go.md) | Go の既定の出力 | `internal/emit/golang`、`runtime`、`plugin/golang` |
| [`plugin-go-cli.md`](plugin-go-cli.md) | `cli` の語彙を Go の CLI にするプラグイン | `plugins/go-cli` |
| [`diagnostics.md`](diagnostics.md) | 診断の形、表示、終了コード | `internal/diag` |
| [`cli.md`](cli.md) | `hozoc` の振る舞いと `cli.hozo` | `cli.hozo`、`internal/app`、`cmd/hozoc` |
| [`lsp.md`](lsp.md) | 言語サーバー | `internal/lsp` |
| [`packages.md`](packages.md) | ディレクトリ、Go のパッケージと API、依存の向き、生成するもの | 全体 |
| [`testing.md`](testing.md) | テストの置き場と書き方、CI | `tests/`、`.github/workflows` |
| [`editors.md`](editors.md) | VS Code 拡張と TextMate の文法 | `editors/vscode` |
| [`decisions.md`](decisions.md) | 設計書にない決定と、設計書へ返すこと | — |

## 言葉

二つ以上の文書で使う言葉は、一つの文書が定義し、ほかの文書は同じ意味で使う。似た言葉の持ち主:

| 言葉 | 持ち主 |
|---|---|
| プラグイン（入れるもの）、トランスパイラ（設定が選ぶ変換） | `plugin-protocol.md`「この文書が決めること」 |
| 入っているプラグイン、使うプラグイン | `plugin-protocol.md`「探索」 |
| 単位、単位の言語 | `config.md`「単位」 |
| 単位の対象（単位が対象にする名前空間）、Dir(U, N) | `config.md`「単位の対象」 |
| 単位の対象の宣言、出力（単位と、その単位が出力する名前空間の組） | `config.md`「in」 |
| 出力の対象（一つの出力に出す宣言） | `emit-go.md`「出力の対象」 |
| emit の対象（一度の `hozoc emit` が書き出す出力） | `cli.md`「emit」 |
| 基底の状態、状態の分類、値の分類 | `check.md`「状態の形」 |
| 状態の単位、実装の単位（生成した Go の中のまとまり。設定の単位とは別） | `emit-go.md`「state」「実装の単位」 |
| 語彙の名前空間 | `vocab.md`「語彙の名前空間」 |
| `cli.hozo`（リポジトリ直下の、hozoc 自身の仕様）と `plugins/go-cli/cli.hozo`（go-cli の語彙） | `cli.md`「cli.hozo」 |

## 書き方

一段落に一つの主張を、見出しの下に置く。別の文書が決めたことは、その文書と見出しを名指しして引き、書き写さない。決めた理由は決めた文書に置く。`decisions.md` は決定と持ち主の一覧で、理由は持たない。
