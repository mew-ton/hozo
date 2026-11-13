# 決定の記録

## この文書が決めること

設計書（`mew-ton/drafts` の `hozo/README.md`）が決めていないことについて、実装のためにこのリポジトリで決めたことの一覧と、設計書の内容を変える決定、設計書へ返すこと、今のスコープから外した設計書の例。決定の中身と理由の持ち主は「持ち主」の列の文書と見出し。設計書が決めれば、その項目は消して設計書を引く。

## 決めたこと

| # | 決めたこと | 持ち主 |
|---|---|---|
| 1 | 本体は Go。IDE の対応は、その IDE の拡張が要求する言語で書く。初期は VS Code 拡張だけ | `docs/requirements.md`「実装言語」 |
| 2 | 当面の展開先は Go と TS。プラグインは展開先の言語で書き、SDK も Go と TS | `docs/requirements.md`「プラグインの書き方」 |
| 3 | IR の構造と JSON の形の持ち主は `docs/ir.md`。`ir` パッケージはそれを写し、スキーマと TS の型は生成 | `docs/requirements.md`「プラグインの書き方」 |
| 4 | hozoc 自身を `cli.hozo` で書き、骨組みを生成する | `docs/requirements.md`「自己展開」 |
| 5 | 言語サーバーは `hozoc lsp`。別のバイナリにしない | `docs/requirements.md`「言語サーバー」 |
| 6 | 既定の出力の担い手は hozoc 本体で、最初の言語は Go。TS は段階 5 | `docs/requirements.md`「設計書にない、実装に要るもの」の行「既定の出力の担い手と言語ごとの形」 |
| 7 | 予約語と文脈キーワードの分け方 | `docs/grammar.md`「予約語」「決めたこと」 |
| 8 | 省略可能は型の `?` だけで表し、名前に付ける `x?: T` は書けない | `docs/grammar.md`「決めたこと」 |
| 9 | 識別子は ASCII。`Duration` の単位は `ms` `s` `m` `h` `d`。`Time` のリテラルはない | `docs/grammar.md`「決めたこと」 |
| 10 | 付く先のないドキュメンテーションコメント、`omit` のない `is`、名前付き引数の後の位置引数はエラー | `docs/grammar.md`「決めたこと」 |
| 11 | `@external` は宣言ではなく核の印 | `docs/grammar.md`「決めたこと」 |
| 12 | その場で書く model はユニオンのアームと例外の値にだけ置ける | `docs/grammar.md`「決めたこと」 |
| 13 | `return` は本体の最後の文にだけ書ける | `docs/check.md`「配線」WIRE-3 |
| 14 | IR の参照はすべて FQN。その場の model は `<親>.<名前>` の model に合成 | `docs/ir.md`「原則」「正規化」 |
| 15 | 例外・効果・`bind` の計算結果を IR に置く | `docs/ir.md`「原則」 |
| 16 | デコレータの適用は入れ子にせず順序付きの列 | `docs/ir.md`「デコレータの適用」 |
| 17 | 「その名前空間から見た IR」は `Program` 全体に見る側の `Signature` を足したもの | `docs/ir.md`「その名前空間から見た IR」 |
| 18 | 名前の先頭のセグメントの探し方の順序 | `docs/resolve.md`「手続き」 |
| 19 | 標準の語彙（`valid` `http` `db` `localStorage` `page` `fs` `proc`）は hozoc に同梱し、常に読む。`cli` は go-cli が `vocabulary` で提供する | `docs/vocab.md`「読み込み」「同梱しない語彙」 |
| 20 | `proc.stdio` は引数を持たず `@external` でもない | `docs/vocab.md`「同梱する語彙」 |
| 21 | プラグインは `[hozo].plugin_dirs` → `HOZO_PLUGIN_PATH` → `~/.hozo/plugins` の順で探す | `docs/plugin-protocol.md`「探索」 |
| 22 | マニフェストに `protocol` `vocabulary` `[check]` を足す。`command` は配列も可。要求は `ir` でなく `view` を持ち `protocol` を足す。応答に `errors` を持つ | `docs/plugin-protocol.md`「マニフェスト」「要求と応答」 |
| 23 | 語彙に固有の検査（PAGE-1 / PAGE-2）はプラグインの `check` が行う | `docs/plugin-protocol.md`「check」 |
| 24 | 設定されたプラグインは名前空間ごとに常に一回呼ぶ。並列にしない | `docs/plugin-protocol.md`「起動」 |
| 25 | `[hozo]` に `sources` `plugin_dirs` `max_bindings`。`out` の継承は同じ値で、子のディレクトリは出力の側が切る | `docs/config.md`「形」「名前空間の設定の解決」 |
| 26 | TOML は `github.com/BurntSushi/toml`。ほかに外部のモジュールは使わない | `docs/packages.md`「依存の向き」 |
| 27 | Go の既定の出力で人やプラグインが実装するものは、宣言一つにつき型一つ。束ねるのは構造体で、未設定は `*runtime.MissingError`。生成するコードは `context` を引数に取らない | `docs/emit-go.md`「実装の単位」 |
| 47 | 状態はパッケージの変数 `States` に置き、状態の単位は問い合わせ（述語、`Get`）と命令（その状態を宛先にする効果を持つ葉の op）を持つ | `docs/emit-go.md`「実装の単位」「state」 |
| 48 | 予約語はフィールド名の位置に書ける。ほかの位置では予約語のまま | `docs/grammar.md`「識別子」「決めたこと」 |
| 28 | 出力の対象はプラグインが `emit` の応答の `select` で返す。マニフェストの `decorate` / `entry` / `stop` は持たない。包む関数を生成するのは `wraps` で引き受けたデコレータの適用だけ | `docs/plugin-protocol.md`「選択」、`docs/emit-go.md`「出力の対象」「デコレータ」 |
| 29 | 絞り込まれた型（`C.Case<T>`）は Go では `T` に消す | `docs/emit-go.md`「型」 |
| 30 | 生成したコードは `github.com/mew-ton/hozo/runtime` を import する | `docs/emit-go.md`「runtime パッケージ」 |
| 31 | 同じプロジェクトの Go の名前空間どうしは、相手の出力が持つ型を import する。別の言語の型と、相手の出力にない型は複製する | `docs/emit-go.md`「配置」 |
| 32 | Go の識別子は先頭だけ大文字にする | `docs/emit-go.md`「名前」 |
| 33 | 別の名前空間への呼び出しの `bind` の位置は `Deps` の実装者が埋める | `docs/emit-go.md`「op」 |
| 34 | `go-cli` の `Main` は `package main` ではなく、`Presenter` で表示を差し替えられる。`proc.stdio` は `stop` | `docs/plugin-go-cli.md`「マニフェスト」「出力」 |
| 35 | 終了コード 64（使い方）、70（内部・プラグインの異常） | `docs/diagnostics.md`「終了コード」 |
| 36 | `cli.hozo` では `emit` を配線にし、`outputs` に `@fs.files` を付けず、`lsp` と `format` と `UnknownNamespace` を足す | `docs/cli.md`「cli.hozo」 |
| 37 | `emit` は前回の出力を `<out>/.hozoc-outputs.json` に記録して古いものを消す | `docs/cli.md`「emit」 |
| 38 | LSP は JSON-RPC を自前で実装し、`hozo.toml` がなければ設定なしで動いて `LSP-1` を出す | `docs/lsp.md`「通信」「プロジェクト」 |
| 39 | WIRE-6 の閾値は既定 5。NAME-1 は warning | `docs/check.md`「配線」「名前空間」 |
| 40 | 規則のテストは `// ERROR: ID` のコメントで期待を書く | `docs/testing.md`「規則ごとのテスト」 |
| 41 | 設計書の例のコーパスで写せなかった箇所は、そのコーパスの `README.md` に一覧する | `docs/testing.md`「コーパス」 |
| 42 | 既定値のリテラルは `scalar` の元の型に対して検査する | `docs/types.md`「既定値」 |
| 43 | 名前空間の出力先 Dir(N) は、`out` を書いた最も近い祖先からのセグメントをパスにして足す。`View` に全名前空間の設定と Dir を持たせる | `docs/config.md`「名前空間の設定の解決」、`docs/ir.md`「その名前空間から見た IR」 |
| 44 | THROWS-1 と Sig は `external` の例外を同じく数えず、`@throws` は `external` の例外も書ける | `docs/check.md`「throws」「名前空間」 |
| 45 | プラグインは protoc と同じ信頼モデル。隔離も、応答以外のファイルの変化の検出も行わない | `docs/plugin-protocol.md`「信頼の前提」 |
| 46 | `cli` の語彙は go-cli が提供し、hozoc には同梱しない | `docs/vocab.md`「同梱しない語彙」 |

## 設計書を変える決定

設計書の節の内容そのものを変える決定。上の表の決定（設計書にないことを決めたもの）と違い、設計書の側を書き換える必要がある。

| # | 決めたこと | 変わる節 | 持ち主 |
|---|---|---|---|
| S1 | 基底の状態の形は `list` / `nullable` / `single` の三つで、列数の範囲で決まる。`post` / `put` / `delete` は列数の相対的な変化を表す組み込みの効果。`list` と `nullable` は組み込みの分類 `Empty` / `HasItems` を持つ | §5.2 | `docs/check.md`「状態の形」 |
| S2 | 空になりうる状態は `nullable<T>` とだけ書き、`single<T?>` は書けない。空にするのは `delete` | §5.2、§6.2（`removeToken` の例）、§16（「single の `clear`」「`maybe<T>`」の廃案を覆す） | `docs/grammar.md`「決めたこと」、`docs/check.md`「状態の形」 |
| S3 | `classify` は一つ以上の基底の状態を対象にできる（状態の分類） | §6.3、§10.1 | `docs/check.md`「状態の形」、`docs/grammar.md`「classify / condition」 |
| S4 | 一つの効果が変えるのは一つの状態の一つの変化。宛先は組み込みの効果か、分類の遷移（`effect e on C: A -> B`） | §6.2 | `docs/check.md`「状態の形」、`docs/grammar.md`「exception / effect」 |
| S5 | 配線は状態の場合で分岐できる（`match s by list`、`match by C`） | §9、§10.2 | `docs/grammar.md`「配線」 |
| S6 | 状態の述語は判定関数に訳され、配線の `match` と、テストでの効果の結果の判定が呼ぶ。効果の観測のうち分類の場合で言える範囲は Hozo の中に入る | §10.3、§12.1、§12.2 | `docs/emit-go.md`「state」「classify」。テストのアサーションの形は段階 6 で決める |
| S7 | 検査器は遷移を行をまたいで追わない。例外と状態の場合を結びつける書き方は持たない | — | `docs/check.md`「状態の形」 |
| S8 | 葉の op が直に持つ効果は高々一つ。複数の効果を持つ処理は配線で書く（効果は伝播する） | §5.2（削除の連鎖）、§7.3（`placeOrder` の例） | `docs/check.md`「状態と効果」EFF-3 |
| S9 | `prevents` / `keeps` の目的語は `effects` だけ。効果を名前で書き分ける形はない | §7.3 | `docs/grammar.md`「decorator の宣言」「決めたこと」 |
| S10 | デコレータは名前空間に付けられない（「名前空間にも付けられる」とした決定を取り消した）。名前空間は複数のファイルの複数のブロックに分けて宣言できる（TypeScript の `interface` と同じ）ので、付けたデコレータをファイルをまたいで一つにまとめて管理することになり、直感的でない。印は中の宣言に付ける | —（§3.1、§8.2 は変えない） | `docs/grammar.md`「ファイルと宣言」「決めたこと」 |
| S11 | 出力するのはプラグインが選んだ宣言だけ。プラグインが参照しなかった op は出力せず、どのプラグインも読まないデコレータは印で何も生成しない。「プラグインがなくても、これだけで生成できる」「対応するプラグインがなければ人が実装する包む関数を出す」「どのプラグインの対象にもならない宣言は既定の出力のまま」をやめる | §4.3 | `docs/emit-go.md`「出力の対象」「デコレータ」 |
| S12 | `hozo.toml` にない名前空間は出力しない。仕様はコードを生成しない部分（人の操作、外部のシステム）を含んでよく、設定なしでも検査できる | §1、§4.5 | `docs/config.md`「出力する名前空間」、`docs/requirements.md`「モデルが先」 |
| S13 | Hozo の価値はエンジニアでなくても設計できること。UML に当たる記述を検査できる仕様として持ち、将来は UI で編集する | §1、§14 | `docs/requirements.md`「価値と位置づけ」 |

## スコープ外にした設計書の例

今の段階では扱わず、コーパス（`docs/testing.md`「コーパス」）からも外す例。段階 5（TS 向けの一次プラグイン）で、react-router / vue-router / Next / Nuxt などのプラグインの設計と一緒に決める。

| 例 | 理由 |
|---|---|
| §8.1 の `redirectOn` / `redirectToLogin` と、それを使う §11.1 の `fetchMe` / `fetchOrders` | ルーターのプラグインが提供するデコレータの話で、引数名 `to` の予約語との衝突もそのときに決める |
| §4.2 の `cache` と、§11.1 の `@cache` | キャッシュのプラグインの話 |

## 設計書へ返すこと

設計書の §15「未決事項」に足す、または例を直すことを提案するもの。中身の持ち主は「決定」の列の行が指す文書。

| 項目 | 内容 | 決定 |
|---|---|---|
| 状態を分類で表す | 「設計書を変える決定」の S1〜S9。§5.2・§6.2・§6.3・§7.3・§9・§10・§12・§16 を書き換える | S1〜S9 |
| モデルが先 | 「設計書を変える決定」の S11〜S13。§1・§4.3・§4.5・§14 を書き換える（S10 は取り消したので、設計書へ返すものはない） | S11〜S13 |
| 省略可能の書き方 | `x?: T` の例（§7.1 `token?: Token`、§11.2 `status?: server.OrderStatus`、§4.2 `min?: int` / `from?: any state`、`examples/hozoc.hozo` の `config?: Path`）を `x: T?` に直す | 8 |
| `Token` の修飾 | §5.2 の `browser` の `state token: single<Token?>;` は `server.Token` でなければ §3.2 に反する | 18 |
| ユニオンのアームの中の `;` | §11.2 の `Ready: { userName: string; orders: OrderRow[] }` は、フィールドの末尾に `;` が要る（`docs/grammar.md`「model」） | 7 |
| §6.5 の `model User` | §5.2 の `model User` と二重の宣言になる。`@valid.length` のフィールドは §5.2 の宣言に足す | 18 |
| scalar のフィールドの既定値 | §11.2 の `page: PosInt = 1` のような既定値の型の規則がない。`docs/types.md`「既定値」の規則を提案する | — |
| 予約語のフィールド名 | 予約語をフィールド名の位置（model のフィールド、レコードリテラルのキー、値のパスの二つ目以降）に書けると決めた。`{ "op": "foo" }` のようなオブジェクトを返す API を書けるようにするため。§3 に足す。`examples/hozoc.hozo` の `EmitRequest` の `namespace: string` はそのまま通る | 48 |
| `location` の型 | §5.2 の `state location: single<Route>;` の `Route` は標準の語彙の型なので修飾して書く（`page.Route`。`http` の語彙に置くかは設計書で決める）。ルーターのプラグインに固有の型は使わない | 18、19 |
| `examples/hozoc.hozo` の「書けなかったもの」 | 三つの解き方。§15 が「なし」のままになっている | 20、36 |
| 語彙の宣言の読み込み元 | §4.2 の語彙を hozoc が同梱すること、プラグインの `vocabulary` | 19、22 |
| プラグインの探索 | 探す順序 | 21 |
| プラグインとの JSON の形 | §4.6 の要求の `ir` を `view` にし、`protocol` を足す。マニフェストの `decorate` / `entry` / `stop` をやめ、応答の `select` にする | 22、28 |
| 語彙に固有の検査 | §11.2 の検査の担い手をプラグインの `check` にする | 23 |
| `View` が `Program` 全体を含むこと | §4.6「その名前空間から見た IR だけ」の読み方 | 17 |
| 命名規則の強さと束縛の数の閾値 | 設計書に値と強さがない | 39 |
