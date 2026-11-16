# 設計文書

Hozo の実装（このリポジトリ）の設計。言語の意味の持ち主は設計書（`mew-ton/drafts` の `hozo/README.md`。各文書の「§n」はその節）で、ここにあるのは実装のための決定。設計書にない決定の一覧は `decisions.md`。

## 読む順

| 文書 | 決めること | 写す場所 |
|---|---|---|
| [`requirements.md`](requirements.md) | 作るもの、段階、自己展開、品質の要件、検査の規則の ID | — |
| [`grammar.md`](grammar.md) | 字句と構文（EBNF） | `internal/syntax` |
| [`ir.md`](ir.md) | IR の構造、正規化、JSON、その名前空間から見た IR | `ir`、`internal/sema` |
| [`resolve.md`](resolve.md) | 名前の解決 | `internal/sema` |
| [`types.md`](types.md) | 型の互換、形の書き方、継承した model、絞り込み、式の型 | `internal/sema` |
| [`check.md`](check.md) | 状態の形、分類、`export` などの op の規則、例外・効果・読む状態・純粋さ・`bind` の計算と、規則ごとの検査 | `internal/check` |
| [`config.md`](config.md) | `hozo.toml`: 単位、基底と相乗り、オプション、継ぎ目、型の置き場 | `internal/config`、`internal/project`、`internal/check` |
| [`vocab.md`](vocab.md) | 標準の語彙 | `vocab/`、`internal/vocab` |
| [`plugin-protocol.md`](plugin-protocol.md) | マニフェスト（基底と相乗りの `requires`、継ぎ目の `seams`）、探索、JSON のやり取り、相乗りが基底の形を知る方法 | `internal/plugin`、`plugin`、`sdk/ts` |
| [`plugin-go.md`](plugin-go.md) | Go の基底のトランスパイラ `go`: 語彙によらずに決まる Go の出力のすべて（置き場、名前、型、op の形、実装を受け取る口、配線の本体、継ぎ目） | `plugins/go`、`plugin/golang`、`runtime` |
| [`plugin-go-valid.md`](plugin-go-valid.md) | `valid` の制約を確かめる関数を出す相乗り `go-valid` | `plugins/go-valid`、`plugin/govalid`、`runtime` |
| [`plugin-go-sqlite.md`](plugin-go-sqlite.md) | `db` の状態を SQLite の表と読み書きにする相乗り `go-sqlite` | `plugins/go-sqlite` |
| [`plugin-go-http.md`](plugin-go-http.md) | `http` の op を `net/http` のハンドラにする相乗り `go-http` と、HTTP の通信の上の形 | `plugins/go-http` |
| [`plugin-go-cli.md`](plugin-go-cli.md) | `cli` の語彙を Go の CLI にする相乗り `go-cli` | `plugins/go-cli` |
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
| 単位 | `config.md`「単位」 |
| 基底、相乗り、単位の基底、単位の言語 | `config.md`「基底と相乗り」 |
| 継ぎ目、単位の継ぎ目（op の「中身」と「外側」） | `config.md`「継ぎ目」。Go での形（`Seams`、中身の名前）は `plugin-go.md`「継ぎ目」 |
| 型の置き場、「名前空間が基底の出力を持つ」 | `config.md`「型の置き場」 |
| 単位の対象（単位が対象にする名前空間）、Dir(U, N) | `config.md`「単位の対象」 |
| 単位の対象の宣言、出力（単位と、その単位が出力する名前空間の組） | `config.md`「in」 |
| 出力の対象（基底が一つの出力に出す宣言） | `plugin-go.md`「出力の対象」 |
| 定義のパッケージ、入口のパッケージ、内側のフォルダ | `plugin-go.md`「内側のフォルダと入口」 |
| op の置き場、形で決まる問い合わせ、問い合わせ、命令 | `plugin-go.md`「op の置き場」「状態の単位」 |
| 通信の形（HTTP の要求と応答の JSON） | `plugin-go-http.md`「通信の形」 |
| emit の対象（一度の `hozoc emit` が書き出す出力） | `cli.md`「emit」 |
| 基底の状態、状態の分類、値の分類 | `check.md`「状態の形」 |
| op の分類、標準の分類、型で決まる分類、述語で決まる分類 | `check.md`「分類」 |
| 読む op、内部の op、純粋 | `check.md`「reads」「export」「純粋」 |
| 直に読む状態、直に持つ効果、デコレータが運ぶもの（`Op.carried`） | `check.md`「デコレータが運ぶ reads と effects」 |
| 出力を推論しない（プラグインの原則: デコレータが出すコードは型に依らない） | `requirements.md`「出力を推論しない」。当てはめて決められなかったところは、`plugin-go-sqlite.md`、`plugin-go-http.md`、`plugin-go-cli.md` の「未決」 |
| 表の状態、表の model、しまい方 | `plugin-go-sqlite.md`「出力」「表」「しまい方」（語彙は `vocab.md`「`db` の表と、しまい方」） |
| 項目の形、全体の形（要求のどこから読むか） | `plugin-go-http.md`「対象」（語彙は `vocab.md`「`http` の語彙」） |
| 形の語、状態の型 | `types.md`「形の書き方」 |
| 継承した model、元の model、継承している | `types.md`「継承した model」 |
| 絞り込み、型で決まる絞り込み、述語の要る絞り込み | `types.md`「絞り込み」 |
| 「値がない」「返らない」（式の型） | `types.md`「型の種類」 |
| 状態の単位（生成した Go の中の、状態ごとのインターフェース。設定の単位とは別） | `plugin-go.md`「状態の単位」 |
| 語彙の名前空間 | `vocab.md`「語彙の名前空間」 |
| `cli.hozo`（リポジトリ直下の、hozoc 自身の仕様）と `plugins/go-cli/cli.hozo`（go-cli の語彙） | `cli.md`「cli.hozo」 |

## 書き方

一段落に一つの主張を、見出しの下に置く。別の文書が決めたことは、その文書と見出しを名指しして引き、書き写さない。決めた理由は決めた文書に置く。`decisions.md` は決定と持ち主の一覧で、理由は持たない。
