# 設計文書

Hozo の言語と、その実装（このリポジトリ）の設計。言語の意味（何が正しい Hozo か）の持ち主は、ここに並ぶ文書。決定の一覧は `decisions.md`、まだ決めていないことの一覧は `open.md`。作業の進め方の決まりは、リポジトリ直下の `CLAUDE.md`。

## 設計書という呼び名

これらの文書と、コードやテストのコメントが「設計書」と書くのは、`mew-ton/drafts` の `hozo/README.md`（https://github.com/mew-ton/drafts/blob/main/hozo/README.md ）と、それに付いた例のファイル `examples/hozoc.hozo`。「§n」は、その `README.md` の節番号で、読んだのはコミット `19e0820` の内容。

**名前は設計書だが、設計書ではない。** この言語の出発点になった、粗いアイデアの集まりで、実装を始めた時点で古くなっていて、保守されていない。仕様の持ち主ではない: Hozo の仕様の持ち主は、このリポジトリの `docs/` で、食い違えば `docs/` が正しい。

「設計書 §n」「設計書の例」は、その考えや例の出どころを指すだけ。設計書へ返すことはなく、設計書との違いを追う一覧も持たない（`decisions.md`「決めたこと」150）。

## 読む順

| 文書 | 決めること | 写す場所 |
|---|---|---|
| [`requirements.md`](requirements.md) | 価値と、そこから導く要件、作るもの、段階、自己展開、品質の要件、検査の規則の ID | — |
| [`grammar.md`](grammar.md) | 字句と構文（EBNF） | `internal/syntax` |
| [`ir.md`](ir.md) | IR の構造、正規化、JSON、その名前空間から見た IR | `ir`、`internal/sema` |
| [`resolve.md`](resolve.md) | 名前の解決 | `internal/sema` |
| [`types.md`](types.md) | 型の互換、形の書き方、継承した model、インデックスの model、絞り込み、式の型 | `internal/sema` |
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
| [`decisions.md`](decisions.md) | 決定と、その持ち主の一覧 | — |
| [`tools.md`](tools.md) | 誰が、何のために、どの道具（IDE、MCP サーバー、言語サーバー、VS Code 拡張、CLI）で、何をするか。道具の境目と、共有する操作、作る順（前提のほかは、案） | — |
| [`ide-scenarios.md`](ide-scenarios.md) | IDE の設計の段 1: 設計する人のユースケースの、具体例の上の筋書き（案） | — |
| [`ide-methods.md`](ide-methods.md) | IDE の設計の段 1: サンプル `order-app` を、設計の手法（UML など）に写した結果。調べの記録 | — |
| [`ide-needs.md`](ide-needs.md) | IDE の設計の段 2: 筋書きが要る情報と操作。今、どこから得られるか（案） | — |
| [`ide-screens.md`](ide-screens.md) | IDE の設計の段 3: 設計の場であるノート（自由なキャンバス）、ノートに置ける図形と線と塊、ノートのほかの場、移り方、割り当ての確かめ（前提のほかは、案） | — |
| [`ide-shapes.md`](ide-shapes.md) | IDE の設計の段 3: ノートに置く図形と線を、プラグインが定義する仕組み（前提のほかは、案） | — |
| [`ide-chain.md`](ide-chain.md) | IDE の設計の段 3: 設計の手法の部品ごとに、描くと、仕様の何に反映され、どんなハーネスが生まれるか（対応の表は、案） | — |
| [`ide-harness-trial.md`](ide-harness-trial.md) | IDE の設計の段 3 の確かめ: ハーネスのプレビュー（節ごとの二つの強度）を、サンプル `order-app` に当てて、数えた記録 | — |
| [`wiring-trial.md`](wiring-trial.md) | R6 の、配線の綴りの確かめ: 文法の全体の案と、サンプル `order-app` の op の書き直しと、分かったこと（記録） | — |
| [`ide-mock.md`](ide-mock.md) | IDE の設計の段 4: ノートの見本（`ide-mock/notes.html`）と、筋書きをなぞって分かったこと（案） | — |
| [`ide-stack.md`](ide-stack.md) | IDE の設計の段 5: 動きの要件、枠組みを選ぶ規準、候補の比べ、動く試作で確かめること（案） | — |
| [`open.md`](open.md) | まだ決めていないことと、その持ち主の一覧。持ち主のない点の中身 | — |

## 言葉

二つ以上の文書で使う言葉は、一つの文書が定義し、ほかの文書は同じ意味で使う。似た言葉の持ち主:

| 言葉 | 持ち主 |
|---|---|
| 設計書、§n（出発点のアイデア集の呼び名。仕様の持ち主ではない） | この文書の「設計書という呼び名」 |
| 関数（op、`read`、効果の総称。宣言の頭は `fn`） | `grammar.md`「関数の宣言」 |
| 関数の型（関数の宣言の、`)` の後ろの `:` から始まる部分: 戻り値と、`throws`、`reads`、`effects` の節） | `grammar.md`「関数の宣言」 |
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
| IDE（エンジニアでない人が、プロジェクトを開いて設計する道具。VS Code などは「エディタ」と呼び、IDE とは呼ばない。`decisions.md`「決めたこと」1 の「IDE」は、エディタのこと）、MCP サーバー、担い手（設計する人、AI、エンジニア）、ユースケースの名前、入口（仕様の中の、誰からも呼ばれない op） | `tools.md`「道具」「担い手」「ユースケース」「試作で分かったこと」 |
| ノート（IDE の、自由なキャンバス。設計は持たず、配置と自由な図形だけを持つ）、結ばれた図形と自由な図形、宣言の線と導いた線、塊、アウトライン、コンテキスト（詳細に設計する単位。名前空間） | `ide-screens.md`「決まっている前提」「図形の二つの種類」「ノート」 |
| ただ一つの正（SSOT。Hozo の価値: 設計を言うものは、仕様だけ） | `requirements.md`「価値と位置づけ」「ただ一つの正」 |
| ハーネス（中身が仕様から外れないことを確かめる仕組みの全体） | `requirements.md`「ハーネス」 |
| 仕様の図（IDE の場の一つ。仕様の全体を、op と、その依存の図として、漏れなく出し、直す） | `ide-screens.md`「仕様の図」 |
| 領域（ノートの上の、設計図の一つ分の場。設計図の種類を一つ持ち、その図形しか置けない） | `ide-screens.md`「領域」 |
| 強度（op、効果、read の一つずつに出す、中身の強度と、設計を守る強度）、穴 | `ide-screens.md`「ハーネスのプレビュー」 |
| テストの規約、書かなければならないテスト（設計から決まるテスト。必要なテストケースが、用意されていて、通っていることを、Hozo が担保する） | `ide-chain.md`「テストの規約」 |
| 要求（システムが、守らなければならないこと。約束と、同じもの。278 より前の文書は、「約束」と書いている） | `ide-chain.md`「要求の文書」「要求トレーサビリティマトリクス」 |
| 出力を推論しない（プラグインの原則: op のデコレータは包むだけで、出すコードは型に依らない。op 以外は、宣言の全体から作る） | `requirements.md`「出力を推論しない」 |
| op の中身に立ち入らない（基底の原則: op の中身に何が要るかを判定せず、言語の作法で関数に付くものを、どの関数にも同じ形で与える） | `requirements.md`「op の中身に立ち入らない」 |
| 表の状態、表の model、しまい方 | `plugin-go-sqlite.md`「出力」「表」「しまい方」（語彙は `vocab.md`「`db` の表と、しまい方」） |
| 項目の形、全体の形（要求のどこから読むか） | `plugin-go-http.md`「対象」（語彙は `vocab.md`「`http` の語彙」） |
| 形の語、状態の型 | `types.md`「形の書き方」 |
| 継承した model、元の model、継承している | `types.md`「継承した model」 |
| インデックスの model（本体にインデックス `[名前: K]: V;` を書いた model。キーが決まっていない構造） | `types.md`「インデックスの model」 |
| 絞り込み、型で決まる絞り込み、述語の要る絞り込み | `types.md`「絞り込み」 |
| 「値がない」「返らない」（式の型） | `types.md`「型の種類」 |
| 状態の単位（生成した Go の中の、状態ごとのインターフェース。設定の単位とは別） | `plugin-go.md`「状態の単位」 |
| 語彙の名前空間 | `vocab.md`「語彙の名前空間」 |
| `cli.hozo`（リポジトリ直下の、hozoc 自身の仕様）と `plugins/go-cli/cli.hozo`（go-cli の語彙） | `cli.md`「cli.hozo」 |

## 書き方

一段落に一つの主張を、見出しの下に置く。別の文書が決めたことは、その文書と見出しを名指しして引き、書き写さない。決めた理由は決めた文書に置く。`decisions.md` は決定と持ち主の一覧で、理由は持たない。`open.md` は未決の点と持ち主の一覧で、問いの中身は、持ち主の文書に置く（持ち主のない点だけ、`open.md` が持つ）。
