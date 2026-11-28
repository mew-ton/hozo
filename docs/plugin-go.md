# go プラグイン（Go の基底）

## この文書が決めること

Go の基底のトランスパイラ `go`（`docs/config.md`「基底と相乗り」。プラグインは `plugins/go/`）の、診断の補足（「診断」）と、形を変えてきた経緯（「これまでの形から変えたこと」）。

ほかのことの持ち主（決定 D814）:

- どんな仕様に対しても、何を出すか（置き場、名前、型、op の形、実装を受け取る口、配線の本体、継ぎ目、テストの口、コメント）: `plugins/go/spec/go.hozo` の要求 `hozo.gobase.Requirement`。
- マニフェストと、オプション `module`: `plugins/go/hozo-plugin.toml`。
- 生成したコードが import するパッケージ `runtime`: そのパッケージの説明（`runtime/doc.go`）。
- 診断（GO-*）の ID と条件と重さ: `spec/diagnostics/go.hozo`（「診断」）。

## 診断

`emit` の応答の `errors` で返す。検査だけの実行（`hozoc build --no-emit`）も、同じ応答の診断を使う（どれも error の診断になり、`CheckFailed`。`docs/plugin-protocol.md`「起動」「emit」）。位置は、ケースの説明に書いた宣言の FQN。

この見出しの診断の、ID と条件と重さの持ち主は、`spec/diagnostics/go.hozo` の分類 `hozo.diagnostics.Go` のケース。

- warning は返さない。`catches` を持つデコレータが配線の op に付いているときの warning は、やめた（基底はデコレータの節を見ない。`plugins/go/spec/go.hozo` の `Requirement.Decorators`）。
- GOEMIT-1〜GOEMIT-6 と GOEMIT-INTERNAL は欠番。本体に組み込んだ Go の出力（「既定の出力」）の規則で、出力を基底のプラグインに移したときになくなった。番号は別の意味に使い回さない（欠番は、`spec/diagnostics/` のケースの `@retired`）。行き先:

| なくなった ID | 規則 | 行き先 |
|---|---|---|
| GOEMIT-1 | `out` の上に `go.mod` がない | GO-1（その後、欠番: `spec/diagnostics/go.hozo`） |
| GOEMIT-2 | 大文字化で名前が衝突する | GO-2（当たる名前の範囲を広げた） |
| GOEMIT-3 | `valid` の制約が元の型に合わない、など | GOVALID-1。その後、VALID-1〜VALID-4 に移した（`docs/plugin-go-valid.md`「診断」の「欠番」） |
| GOEMIT-4 | 既定値が `valid` の制約に合わない | GOVALID-2（同上。フィールドの既定値をなくしたときに、欠番になった） |
| GOEMIT-5 | 出力どうしの import が循環する | GO-3 |
| GOEMIT-6 | 組み込みの出力が支えない書き方（移行の間だけの規則） | なし（基底は、検査を通ったすべての書き方を出す。支えないのは GO-6 だけ） |
| GOEMIT-INTERNAL | 生成器の不具合 | GO-INTERNAL |

この節の細部は、決定 D631、D667、D780 が決めた。

## これまでの形から変えたこと

本体に組み込んでいた Go の出力（これまでの文書「Go の既定の出力」。この文書が置き換えたので、今は git の履歴にだけある）と、その後のこの基底の形から変えたこと。決定の番号は `spec/decisions/` の決定。

| これまで | 今 | 決定 |
|---|---|---|
| hozoc 本体が出す。トランスパイラが選んだ宣言だけ | 基底のプラグインが出す。型と状態はすべて、op は単位の対象のすべて | 68、69 |
| 別の言語の型と、どの出力にもない型は、使う側に複製する | 複製しない。置き場がなければ CFG-22 | 67 |
| `Validate` を基底が出す | `go-valid` が出す | 71 |
| 生成するコードは `context` を取らない。その後の形では、純粋でない op だけが `ctx context.Context` を取り、純粋で例外のない op は `error` を返さない | 仕様の宣言を写した関数は、どれも、最初の引数に `ctx context.Context` を取り、最後の戻り値に `error` を返す。純粋かどうかは見ない | 145、146（72 の、純粋さで形を分ける部分を置き換えた） |
| 分類の列挙は、`default` のための定数 `<C>Default` を持ち、判定関数は、真が一つもなければ `<C>Default` を返す。`default` のアームは `case <C>Default:` か、型の `switch` の `default:` | `<C>Default` はない。どのケースも述語を持ち、真がちょうど一つでなければ `*runtime.ClassificationError`。型で決まる分類の `match` は、どのケースにも当たらない値を、分類の誤りの枝で返す。`else` のアームは、受けるケースを並べたアームと同じコード | 154、156、160 |
| 分類の述語は、値だけを取って `bool` を返す。判定の誤りを返す先がない配線は、`panic` する | 述語も判定関数も、`ctx` を取り、`error` を返す。判定の誤りは、配線が返す。基底は `panic` を出さない | 146、147（90 の帰結を置き換えた） |
| 実装は、関数のフィールドを持つ構造体（`Leaves`、`Deps`、`Wrappers`、`Classifiers`）と、パッケージの変数 `States` | インターフェースと、`New` の位置引数。未設定の `*runtime.MissingError` はない | 73 |
| `Ops` はすべての op | 入口の `Ops` は `export` の op だけ。内部の op は内側のフォルダから | 74 |
| 別の名前空間への呼び出しの `bind` の位置は、`Deps` を埋める者が埋める | 配線が埋める | 79 |
| `wraps` で引き受けたデコレータに、包む関数を生成する | デコレータの出力は、読むトランスパイラが決める。基底が出すのは、単位のトランスパイラがマニフェストで宣言した継ぎ目（二層と `Seams`）だけ。読む者がなければ何も出さない | 84、85、86（78 を置き換えた） |
| `match s by list` / `by nullable` は `IsEmpty` を呼ぶ | 状態は `Get` で読み、値として分ける | 72（状態は名前で書いた値。S21） |
| union を `optional` にすると `*<Union>` | インターフェースのまま（`nil` が「なし」） | 69 |
| 状態に触れる葉の op を、読む状態と効果で、状態の単位か、複数の状態にまたがる口 `SharedState` に置く | 状態に触れるのは `read` と効果で、どちらも状態の単位のメソッド。葉の op は `Leaves` だけ。`SharedState` はない | 139（100 を置き換えた） |
| 値を運ぶ例外は `struct{ Value <型> }` | 引数ごとにフィールド一つ | 139 |
| 述語で決まる分類は、どのケースも述語を持つ。効果のコメントの宛先は、`<状態>.<操作>` か、状態の分類の遷移 | ない側のケース（`Classify.absent`）は、述語を持たず、判定関数が、値がないことで決める。行の遷移を持つ効果は、コメントに、行の遷移の表示を持つ。効果のメソッドの形は変わらない | 169 |
| 行の引数のケースは、どこでも確かめない。テストを受け取る口は、ない | 絞り込んだ引数を持つ効果は、`New` が包んだ単位が、入口で、引数のケースを確かめる（`plugins/go/spec/go.hozo` の `Requirement.Ports` の「効果の入口の確かめ」）。人か AI が中身を書く関数（葉の op、分類の判定）に、結果ごとの単体テストを受け取る口を、テストのパッケージに出す（`plugins/go/spec/go.hozo` の `Requirement.UnitTestPort`） | 371〜375 |
