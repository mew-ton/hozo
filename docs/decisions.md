# 決定の記録

## この文書が決めること

設計書（`mew-ton/drafts` の `hozo/README.md`）が決めていないことについて、実装のためにこのリポジトリで決めたことと、その理由。決定の中身の持ち主は各設計文書で、ここは一覧と理由と、設計書へ返すかどうか。設計書が決めれば、その項目は消して設計書を引く。

## 決めたこと

| # | 決めたこと | 理由 | 持ち主 |
|---|---|---|---|
| 1 | 本体と言語サーバーは Go。VS Code 拡張だけ TS | Go 向けプラグインが IR の型を直接使え、ツールチェーンが二つで済む | `docs/requirements.md`「実装言語」 |
| 2 | プラグインは展開先の言語で書く。SDK は Go と TS | 実行環境の要求を利用者の手元に合わせる | `docs/requirements.md`「プラグインの書き方」 |
| 3 | IR の形の持ち主は `ir` パッケージ。スキーマと TS の型は生成 | 手で書く場所を一つにする | 同上 |
| 4 | hozoc 自身を `cli.hozo` で書き、骨組みを生成する | 最初の利用者を hozoc 自身にして既定の出力とプロトコルを本物で確かめる | `docs/requirements.md`「自己展開」 |
| 5 | 言語サーバーは `hozoc lsp`。別のバイナリにしない | 配布物を一つにし、CLI と診断がずれる余地をなくす | `docs/requirements.md`「言語サーバー」 |
| 6 | 既定の出力の最初の言語は Go。TS は段階 5 | 自己展開のため | `docs/requirements.md`「既定の出力の担い手と言語」 |
| 7 | 予約語は 47 語。`field` `variant` `post` `put` `delete` は文脈キーワード | §6.1 の `field: string` を通す | `docs/grammar.md`「予約語」 |
| 8 | `x?: T` と `x: T?` は同じ意味 | §5.3 が両方を「省略可能」として扱う | `docs/grammar.md`「決めたこと」 |
| 9 | 識別子は ASCII。`Duration` の単位は `ms` `s` `m` `h` `d`。`Time` のリテラルはない | 同上の表 | 同上 |
| 10 | 付く先のないドキュメンテーションコメント、`omit` のない `is`、名前付き引数の後の位置引数はエラー | 同上の表 | 同上 |
| 11 | `return` は本体の最後の文にだけ書ける | 途中の `return` は下の行が評価されないので意味がない | `docs/check.md` WIRE-3 |
| 12 | IR の参照はすべて FQN。その場の model は `<親>.<名前>` の model に合成 | プラグインが名前を解決しなくて済む | `docs/ir.md`「原則」「正規化」 |
| 13 | 例外・効果・`bind` の計算結果を IR に置く | プラグインが再計算しなくて済む | `docs/ir.md`「op」 |
| 14 | 「その名前空間から見た IR」は `Program` 全体に見る側の `Signature` を足したもの | 参照先を切り出す規則を持つより単純で、何を変換するかはプラグインが `entry` / `stop` で決める（§4.3） | `docs/ir.md`「その名前空間から見た IR」 |
| 15 | 名前の先頭のセグメントは、束縛・引数 → デコレータの引数 → ルートの名前空間 → 現在の名前空間 → グローバルの順 | §3.2 の「外側を探さない」を一段の探索にする | `docs/resolve.md`「手続き」 |
| 16 | デコレータの引数に限り、`scalar` の位置に元の型のリテラルを渡せる | `@cache(key: "me")` のような定数。制約の検査は語彙のプラグインの仕事 | `docs/types.md`「デコレータの引数」 |
| 17 | 標準の語彙（`valid` `http` `db` `localStorage` `page` `cli` `fs` `proc`）は hozoc に同梱し、常に読む。プラグインは `vocabulary` で足せる | 検査に展開のプラグインを要求しない。宣言に実行環境はない | `docs/vocab.md` |
| 18 | `cache` は同梱しない | `key` の型が一つに決まらない | `docs/vocab.md`「同梱しない語彙」 |
| 19 | `proc.stdio` は引数を持たず `@external` でもない | 呼び先は実行時に決まる。例外は同じ名前空間の呼び出し元にも見える | `docs/vocab.md`、`docs/cli.md` |
| 20 | プラグインは `[hozo].plugin_dirs` → `HOZO_PLUGIN_PATH` → `~/.hozo/plugins` の順で探す | `hozo.toml` の名前からマニフェストに至る規則がなかった | `docs/plugin-protocol.md`「探索」 |
| 21 | マニフェストに `protocol` `vocabulary` `[check]` を足す。`command` は配列も可。応答に `errors` を持つ | 版の照合、語彙の提供、語彙に固有の検査、`go run` での起動、語彙の使い方の誤りの報告 | `docs/plugin-protocol.md` |
| 22 | 語彙に固有の検査（PAGE-1 / PAGE-2）はプラグインの `check` が行う | §4.1 で核は語彙を組み込まない | `docs/plugin-protocol.md`「check」 |
| 23 | 設定されたプラグインは名前空間ごとに常に一回呼ぶ。並列にしない | 出力の順序を決定的にする | `docs/plugin-protocol.md`「起動」 |
| 24 | `[hozo]` に `sources` `plugin_dirs` `max_bindings`。`out` を引き継ぐ子は親の `out` の下にディレクトリを足す | 全体の設定の置き場と、子の出力先の規則がなかった | `docs/config.md` |
| 25 | TOML は `github.com/BurntSushi/toml`。ほかに外部のモジュールは使わない | 依存を最小にする | `docs/packages.md`「依存の向き」 |
| 26 | Go の既定の出力は `Leaves` / `Ops` / `Deps` / `States` / `Wrappers` / `Classifiers` と `New` | 人が実装する単位（§10.3「人が書くのは…小さな単位だけ」）をインターフェースにする | `docs/emit-go.md`「op」 |
| 27 | 包む関数を生成するのは、`external` でなく、設定されたプラグインの `decorate` / `entry` / `stop` にないデコレータ | §4.3「対応するプラグインがなければ…人が実装する」 | `docs/emit-go.md`「デコレータ」 |
| 28 | 絞り込まれた型（`C.Case<T>`）は Go では `T` に消す | Go に部分型がない。前提はコメントに | `docs/emit-go.md`「型」 |
| 29 | 生成したコードは `github.com/mew-ton/hozo/runtime` を import する | 例外・違反・分類の誤りの型を一か所に | `docs/emit-go.md`「runtime パッケージ」 |
| 30 | 同じプロジェクトの Go の名前空間どうしは import し、別の言語の型は複製する | §4.5「使う側の名前空間の言語でも型として出力する」を Go↔Go では import で満たす | `docs/emit-go.md`「配置」 |
| 31 | フィールド名は先頭だけ大文字にする（`Id`、`Url`） | 決定性と単純さ。慣習（`ID`）より予測できることを取る | `docs/emit-go.md`「名前」 |
| 32 | `go-cli` の `Main` は `package main` ではなく、`Presenter` で表示を差し替えられる | hozoc 自身が診断を text で出すため | `docs/plugin-go-cli.md`「出力」 |
| 33 | 終了コード 64（使い方）、70（内部・プラグインの異常） | `sysexits` の慣習 | `docs/diagnostics.md`「終了コード」 |
| 34 | `cli.hozo` では `emit` を配線にし、`outputs` に `@fs.files` を付けず、`lsp` と `format` を足す | `examples/hozoc.hozo` の「書けなかったもの」を解く | `docs/cli.md`「cli.hozo」 |
| 35 | `emit` は前回の出力を `<out>/.hozoc-outputs.json` に記録して古いものを消す | `removeStaleOutputs` の判定に前回の一覧が要る | `docs/cli.md`「emit」 |
| 36 | LSP は JSON-RPC を自前で実装し、`hozo.toml` がなければ設定なしで動いて `LSP-1` を出す | 依存を増やさない。開いただけのファイルでも診断を出す | `docs/lsp.md` |
| 37 | 標準の語彙は `hozo:` スキームの仮想文書 | 定義へジャンプの飛び先にする | `docs/lsp.md`「プロジェクト」 |
| 38 | WIRE-6 の閾値は既定 5。NAME-1 は warning | 設計書に値と強さがない | `docs/check.md` |
| 39 | 規則のテストは `// ERROR: ID` のコメントで期待を書く | 期待が例のそばにある | `docs/testing.md` |

## 設計書へ返すこと

設計書の §15「未決事項」に足す、または例を直すことを提案するもの。

| 項目 | 内容 | 関連する決定 |
|---|---|---|
| `redirectOn` の引数名 `to` | §8.1 の `decorator redirectOn(error: ApiError, to: RedirectByError) ... halts to;` の `to` が `bind x to` の予約語とぶつかる。`then` か `target` に | 7 |
| `Route` の修飾 | §5.2 の `state location: single<Route>;` は `page.Route` でなければ §3.2 に反する | 15 |
| `@cache` の `key` の型 | §4.2 の `cache(key: CacheKey, staleTime: Duration)` の `CacheKey` が未宣言で、§11.1 は文字列と配列の両方を渡している | 18 |
| `proc.stdio` の引数と `@external` | `examples/hozoc.hozo` の「書けなかったもの」の三つ目。引数をなくし、`@external` を外す | 19 |
| 語彙の宣言の読み込み元 | §4.2 の語彙を hozoc が同梱すること、プラグインの `vocabulary` | 17、21 |
| プラグインの探索 | 20 の順序 | 20 |
| 語彙に固有の検査 | §11.2 の検査の担い手をプラグインの `check` にする | 22 |
| `examples/hozoc.hozo` の「書けなかったもの」 | 三つの解き方（34、19）。§15 が「なし」のままになっている | 34、19 |
| 命名規則の強さと束縛の数の閾値 | 38 | 38 |
| `View` が `Program` 全体を含むこと | §4.6「その名前空間から見た IR だけ」の読み方 | 14 |
