# go プラグイン（Go の基底）

## この文書が決めること

Go の**基底**のトランスパイラ `go`（`docs/config.md`「基底と相乗り」）が、どんな仕様に対しても何を出すか。プラグインは `plugins/go/`、形の規則を写した公開の SDK は `plugin/golang`（`docs/packages.md`）、生成したコードが import するのは `runtime`。入力は `docs/ir.md` の `View`（出力ごとに一つ。見る側は出力する名前空間）、出力は `View.config.dir` の下のファイル（パスはルートからの相対）。

基底が出すのは、**語彙を知らなくても、仕様と設定だけから一通りに決まるもの**: 型、op の関数の形、実装を受け取る口（インターフェース。`read` と効果は、状態の単位のメソッド）、配線の本体、単位のトランスパイラが宣言した継ぎ目（「継ぎ目」）、それらを組み立てる `New`。「設定」は、`hozo.toml` と、単位のトランスパイラのマニフェスト（`View.config`）。語彙に固有のもの（制約を確かめる関数、HTTP のハンドラ、表と SQL、CLI）は出さない。それは、この形の上に相乗りするトランスパイラが足す（`docs/plugin-go-valid.md`、`docs/plugin-go-sqlite.md`、`docs/plugin-go-http.md`、`docs/plugin-go-cli.md`）。型を写す op の中身も、機械的に決まる場合を含めて出さない（写すのは人が書く葉の op。写し忘れは、このツールでは確かめない）。

この文書の「N」は出力する名前空間（`View.viewer`）、「U」はその単位。「単位」は設定の単位（`docs/config.md`「単位」）で、「状態の単位」はこの文書の中の別の言葉（「状態の単位」）。期待する出力の実物は、サンプル `tests/samples/shop-server/` の `gen/`（`docs/testing.md`「トランスパイラのテスト」）。

## マニフェスト

```toml
name     = "go"
command  = ["go", "run", "github.com/mew-ton/hozo/plugins/go"]
protocol = "0.18"

[[transpiler]]
name     = "go"
language = "go"

[coverage]
enabled = true

[check]
enabled = true
```

`[check]` は、`emit` が `errors` で返す誤りを、検査の段（`hozoc check`）でも返すためのもの（`docs/plugin-protocol.md`「check」）。

- 提供するトランスパイラは `go` の一つ。`requires` を持たない（基底）。言語 `go` を定める。
- `[coverage] enabled`: `hozoc tests` が渡すカバレッジのファイルを読む（「カバレッジを読む」）。
- オプションはない。`[emit.options.go]` にキーが一つでもあれば GO-5。
- 語彙は提供せず、どの語彙も読まない。`seams` も持たない。デコレータの適用が基底の出力に効く道は「デコレータ」の二つで、どちらも hozoc が計算して IR と `View` に置いたもの（`Signature`、`View.config.seams`）を通る。
- `command` が `go run` なのは、このリポジトリの中で `plugin_dirs = ["plugins"]` から使うため。配布するときはビルドした実行ファイル `hozoc-plugin-go` と、それを `command` に書いたマニフェストを `~/.hozo/plugins/go/` に置く（`docs/plugin-protocol.md`「探索」）。このリポジトリのほかの Go のプラグインも同じ。

## 出力の対象

出力 (U, N) に出す宣言。選ぶ者はいない。設定だけで決まる。

| 宣言 | 出すもの |
|---|---|
| N の model / scalar / union（`arms`）/ exception | **すべて**。`in` によらない |
| N の effect | **すべて**: 定数（「effect」）と、宛先の単位の命令（「状態の単位」）。`in` によらない |
| N の read | **すべて**: 読む状態の単位の問い合わせ（「状態の単位」）。`in` によらない |
| N の基底の状態 | すべて（状態の単位）。`in` によらない |
| N の分類 | 述語で決まる分類（`Classify.decidedBy` が `predicate`）はすべて。型で決まる分類は何も出さない（「分類」） |
| N の op | **単位の対象の op のすべて**（`View.InUnit`。`docs/config.md`「in」）。起点から辿れるものだけに絞らない。`export` の有無は、出すかどうかを変えない |
| N の op のうち、単位の対象でないもの | 単位の対象の配線が呼んでいるものだけを、**本体のない op として**出す（配線でも本体を出さず、人が実装する葉として `Leaves` に置く。「op の置き場」） |
| 別の名前空間の op | 単位の対象の配線が呼んでいるものを、実装を受け取る口のメソッドとして出す（「別の名前空間の op」） |
| decorator（宣言）、`set` の union、`bind` | 何も出さない |
| デコレータの適用 | 単位の継ぎ目（`docs/config.md`「継ぎ目」）にあるデコレータが、単位の対象の op に付いているものだけ、継ぎ目を出す（「継ぎ目」）。それ以外の適用には、何も出さない（「デコレータ」） |

- 型と、状態と、`read` と効果を `in` によらず出すのは、ほかの出力がこの出力の型を import でき（`docs/config.md`「型の置き場」）、状態の単位の形が、`in` で変わらないようにするため（状態の単位を実装する者は、単位の対象を知らない）。`in` が絞るのは op だけ。
- 以降、この文書が「N の op」と言えば、上の表で出す op（単位の対象の op と、本体のない op として出すもの）。
- `View` が見る側のために合成した model（`<viewer>.<op 名>$<引数名>`。`synthesizedFrom` が op の FQN。`docs/ir.md`「その名前空間から見た IR」）は出さない。別の名前空間の op の引数は、相手の宣言のまま使う（「bind で埋まる位置」）。
- コードにしたくない op は、単位を置かない名前空間に書くか、`in` で外す。

## 配置

### パッケージと import パス

- 出力 (U, N) のファイルは `<ルート>/<Dir(U, N)>/` の下に置く（`View.config.dir`）。
- **パッケージ名**は、N の最後のセグメントを小文字にし、ASCII の英数字以外を落としたもの。それが空、数字で始まる、または Go の予約語なら `pkg` を前に付ける（名前空間 `type` は `pkgtype`）。パッケージ名は名前空間から決まり、`out` によらない（同じ名前空間の二つの単位は、同じ名前のパッケージを別のディレクトリに持つ）。
- **import パス**は、`<Dir(U, N)>` から親へ向かって最初に見つかる `go.mod` の `module` に、そこからの相対パスを足したもの。ルートは環境変数 `HOZO_PROJECT_ROOT`（`docs/plugin-protocol.md`「起動」）。`go.mod` が見つからなければ GO-1。
- **別の名前空間 M の型**（と、述語で決まる、値の分類・op の分類）は、M の型の置き場（`docs/config.md`「型の置き場」: U 自身が M を出力するならその出力、そうでなければ基底が `go` で M を出力する最初の単位の出力）の**型のパッケージ**（「型のパッケージ」）を import する。複製はしない。置き場があることは hozoc が確かめている（CFG-22）。テストのパッケージのファイルだけは、M の入口のパッケージを import する（入口は、型のすべての別名を持つ。テストのパッケージは、どの出力からも import されない）。
- 同じ名前空間 N を出力する別の単位のパッケージは import しない。
- **import の名前**: 名前空間のパッケージは、パッケージ名で import する。パッケージ名が import パスの最後の要素と違えば、名前を書く（`pkgtype "example.com/shop-server/gen/type"`）。一つのファイルの中で名前が当たるとき（標準ライブラリか `runtime` の import、内側のパッケージ `inner`、先に決めた別の名前空間のパッケージ）は、名前空間の FQN の辞書順に名前を決めていき、当たった方に `2`、`3`、… を付ける（`store2`）。標準ライブラリ、`runtime`、`inner` の名前は変えない。**基底が決まった名前で使う識別子**も、import の名前に使わない: パッケージ名がそれと同じ名前空間は、初めから `2`、`3`、… を付けて import する（名前空間 `ops` は `ops2`、名前空間 `ctx` は `ctx2`）。二種類ある。（1）基底が定義のパッケージに宣言する非公開の名前のうち、パッケージ名になりうるもの: `New` が返す構造体 `ops` と、補助 `ptr`。Go では、パッケージの宣言と、そのパッケージのどのファイルの import の名前も、同じ名前ならコンパイルできないので、**どのファイルでも**（入口のパッケージのファイルと、相乗りのファイルでも）使わない。（2）基底が、関数の本体の中で決まった名前で使う識別子のうち、その本体が名前空間のパッケージの型を書きうるもの: メソッドのレシーバ `o` と、`ctx`、`err`。その名前の局所の変数が、同じ名前のパッケージを隠す（`return ctx.Info{}, err` が書けない）ので、**基底のファイルでは**使わない。相乗りのファイルは、（2）に従わず、自分の局所の名前の方を変える（それぞれの文書）。import のブロックは、標準ライブラリ、空行、それ以外の順で、それぞれパスの辞書順。一つなら一行で書く。
- **型のパッケージから、型のパッケージへの import は、一方向でなければならない。** 型のパッケージが、import を辿って自分に戻るなら GO-3 で、何も書き出さない（Go は import の循環をコンパイルできない）。辿るのは、型のパッケージに置く宣言（「型のパッケージ」）が使う、別の名前空間の宣言だけ: `docs/config.md`「型の置き場」の表の 1 と、2 のうち、値の分類・op の分類の分。関数の側（op、`read`、効果、状態）が使う型は、輪にならない: 関数を持つパッケージは、型のパッケージを import するだけで、どの型のパッケージからも import されない（`docs/decisions.md`「決めたこと」402）。直すのは仕様の側: 互いに使う型を、一方か、第三の名前空間に寄せる。言語は、有限の値を持てる型の輪を、止めない（`docs/types.md`「値を、有限に作れない型」）: 名前空間をまたぐ輪を、Go に出せないことを、止めるのは、この診断。

### 内側のフォルダと入口

内部の op（`export` のない op。`docs/check.md`「export」）も、単独でテストできなければならない。「別の名前空間から呼べない」は Hozo の検査（NS-5）が守るので、生成したコードの上では、置き方で解く。

N の op に内部の op が**一つでもある**出力は、二つのパッケージに分ける。

| パッケージ | ディレクトリ | パッケージ名 | 中身 |
|---|---|---|---|
| **内側** | `<Dir(U, N)>/inner/` | `inner` | すべての定義: 型、実装を受け取る口、**すべての op** を並べた `Ops`、配線の本体、継ぎ目の二層と `Seams`、`New` |
| **入口** | `<Dir(U, N)>/` | N のパッケージ名 | 内側の再公開: 型と口の別名、**`export` の op だけ**を並べた `Ops`、内側の `New` を呼ぶ `New` |

N の op に内部の op が一つもない出力（op を持たない、型だけの名前空間を含む）は、分けない。`<Dir(U, N)>/` の一つのパッケージがすべての定義を持ち、それが入口でもある。

- **定義のパッケージ**は、型と口と配線を定義するパッケージ: 分けた出力では内側、分けない出力では `<Dir(U, N)>/` のパッケージ。**入口のパッケージ**は、どちらの出力でも `<Dir(U, N)>/` のパッケージ。
- 人が書くコード、別の名前空間の出力、相乗りのファイルは、入口のパッケージを使う。内側を import するのは、入口と、テストだけ。内側の `Ops` は内部の op も公開のメソッドとして持つので、テストは `inner.New(…)` の戻り値から、内部の op を直に呼べる。
- 内側のフォルダの名前が `internal` でないのは、Go のコンパイラが、`internal` という名前のディレクトリの下のパッケージを、その親の下からしか import させないため。単位の `out` の下は hozoc のもの（記録 `.hozoc-outputs.json` の通りに消す）で、人のテストは置けない。テストは `out` の外にあるので、`internal` では import できない。
- 分けるかどうかを内部の op の有無で決めるのは、内側のフォルダが、内部の op をテストするためだけにあるから。型だけの名前空間や、すべての op が `export` の名前空間に、別名だけの入口を重ねない。`export` を付け外しして内部の op の有無が変わると、定義のパッケージが動くが、入口のパッケージの名前（型、口、`Ops`、`New`）は変わらないので、入口を使うコードはそのまま通る。
- 内側のパッケージの型は、入口の別名と同じ型（`type User = inner.User`）。エラーメッセージや `%T` には `inner.User` と出る。
- 同じ単位が、N の子の名前空間 `N.inner` を出力するなら GO-4（Dir(U, N.inner) が内側のフォルダと重なる）。テストのパッケージのディレクトリ（「単体テストの口」）と重なる子の名前空間も、同じ GO-4。
- 内部の op を、パッケージの外から呼べなくすること（非公開のメソッドにする、型アサーションでも取り出せなくする）は、目的にしない。内側のフォルダを import すれば呼べる。守るのは検査。

入口の再公開のしかた（分けた出力だけ）:

| 定義（内側） | 入口 |
|---|---|
| model、scalar、union、exception の型（union のアームを包む型を含む） | `type <名前> = inner.<名前>`（型の別名。メソッドもそのまま付いてくる） |
| effect の定数、分類の列挙の定数 | `const <名前> = inner.<名前>` |
| `Leaves`、状態の単位（`<状態>State`）、`<Ns>Ops`、`Seams`、分類の型と述語（`<C>`、`<C>Cases`） | `type <名前> = inner.<名前>` |
| 分類の判定関数 `Classify<C>` | 同じ引数と戻り値で、`inner.Classify<C>(…)` を呼んで返す関数 |
| `Ops` | **別名にしない。** `export` の op だけを、宣言順に並べたインターフェースを、入口で宣言する |
| `New` | 同じ引数で、`return inner.New(…)`。戻り値は入口の `Ops`（内側の `Ops` の値は、メソッドが多いだけなので、そのまま返せる） |

- 別名と定数と関数には、定義と同じコメントを書く。
- `export` の op が一つもない名前空間でも、入口は空の `Ops` と、それを返す `New` を出す。

### 型のパッケージ

`docs/decisions.md`「決めたこと」402。二つの名前空間が、互いの型を使っても（片方の型が、相手の型を使い、相手の関数が、こちらの型を使う）、Go の import が輪にならないように、型だけのパッケージを、関数を持つパッケージから、分ける。

**関数を持つ名前空間**（op、`read`、効果、状態の宣言を、一つでも持つ名前空間。単位の `in` は、見ない）の出力は、型を、`<Dir(U, N)>/types/` のパッケージ（パッケージ名は `types`）に置く。関数を持たない名前空間（型だけの名前空間）の出力は、分けない: `<Dir(U, N)>/` のパッケージが、型のパッケージでもある。

| 型のパッケージに置くもの | ファイル |
|---|---|
| model、scalar、union（アームを包む型と、マーカーメソッドを含む）、exception、effect の定数 | `types.go` |
| 述語で決まる、値の分類・op の分類の、列挙の型と定数、述語のインターフェース、判定関数 | `classify.go` |
| 型にメソッドを足す相乗りのファイル（`go-valid` の `validate.go`） | 「相乗りのファイル」 |

- **定義のパッケージは、型のパッケージを、再公開する**（分けた出力の入口が、内側を再公開するのと、同じ形。「内側のフォルダと入口」の表）: `types.go` は、型の別名と、effect の定数。`classify.go` は、値の分類・op の分類の再公開（別名、定数、判定関数を呼んで返す関数）と、状態の分類の定義（状態の単位を引数に取るので、定義のパッケージに残る）。定義のパッケージが、型のパッケージを import する名前は、`types`（`inner` と同じに、ほかの import に使わせない）。
- **人が書くコードから見た名前は、変わらない。** 入口のパッケージは、今までどおり、型と分類のすべてを、同じ名前で持つ（別名なので、同じ型）。エラーメッセージや `%T` には、`types.User` と出る。
- **別の名前空間の出力と、相乗りのファイルは、型のパッケージを import する**（「パッケージと import パス」）。名前空間のパッケージ名で import し、名前を書く（`store "example.com/shop-server/gen/store/types"`）。
- `types.go` も `classify.go`（型のパッケージの分）もなければ、型のパッケージを作らず、定義のパッケージに、再公開のファイルも作らない。作るときは、`doc.go`（`// Package types は名前空間 <N> の型: …`）を置く。
- 分けるかどうかを、関数の有無で決めるのは、型だけの名前空間に、別名だけのパッケージを重ねないため。名前空間に、初めての関数を足すと、型の置き場が `types/` に動くが、入口の名前は変わらないので、入口を使うコードは、そのまま通る（内側のフォルダと、同じ考え方）。
- 同じ単位が、N の子の名前空間 `N.types` を出力するなら GO-4（Dir(U, N.types) が、型のパッケージのディレクトリと重なる）。

### ファイル

ファイルの先頭は `// Code generated by hozoc go <版>; DO NOT EDIT.`、空行、（`doc.go` だけはパッケージのコメント、）`package` 節。版は、このプラグインの版（`plugin.Version`。このリポジトリのプラグインはどれも同じ値を持つ）。`go/format` で整形する。整形に失敗したら GO-INTERNAL（生成器の不具合）で、整形前の内容を標準エラーに出す。

定義のパッケージのファイル（中身がなければ作らない。`doc.go` はいつも作る）。関数を持つ名前空間では、`types.go` と、`classify.go` のうち、値の分類・op の分類の分は、型のパッケージにあり、定義のパッケージのものは、その再公開（「型のパッケージ」）:

| ファイル | 中身（この順） |
|---|---|
| `doc.go` | パッケージのコメント（「コメント」） |
| `types.go` | model、scalar、union、exception、effect を、宣言の順に一つずつ（「型」） |
| `ops.go` | `Ops`、`Leaves`、`<Ns>Ops`（名前空間の FQN の辞書順）、`Seams`、`New`、構造体 `ops`、`ops` のメソッドのうち配線の本体でないもの（op の宣言順。継ぎ目を持つ op の外側のメソッドと、`never` を返す op の中身のメソッドを含む。「継ぎ目」）。N の op が一つもなければ作らない |
| `states.go` | 状態の単位（基底の状態と、述語で決まる状態の分類を、宣言順に）。入口の確かめを持つ単位は、インターフェースのすぐ後ろに、包む型 `checked<状態>State` と、そのメソッド（「効果の入口の確かめ」） |
| `classify.go` | 述語で決まる分類の、列挙の型と定数、述語のインターフェース、判定関数（宣言順） |
| `wiring.go` | 配線の op の本体（宣言順。継ぎ目を持つ配線は、中身の名前のメソッド）。値をポインタにする補助 `ptr` を使っていれば、末尾に一つ |

分けた出力の入口のファイルは、内側の同じ名前のファイルの再公開: `doc.go`（N のパッケージのコメント）、`types.go`、`ops.go`（入口の `Ops`、`Leaves` と `<Ns>Ops` と `Seams` の別名、`New`）、`states.go`、`classify.go`。`wiring.go` はない。内側のファイルに再公開するものがなければ、入口のファイルも作らない。

テストのパッケージ（「単体テストの口」の「テストのパッケージ」）のファイルは、`doc.go`（パッケージのコメント）、`leaves.go`（葉の op の口）、`classify.go`（分類の判定の口）、`fakes.go`（にせものと、テストの中の組み立て。「テストのにせもの」）、`hozo-tests.go.json`（テストの対応表。Go のファイルではないので、先頭の行も、整形もない。「テストの対応表」）。中身がなければ作らず、`leaves.go` も `classify.go` も `fakes.go` もなければ、パッケージそのものを作らない。

### 相乗りのファイル

`go` に相乗りするトランスパイラの決まり。どれも、プラグインどうしで当たらないようにするためのもの。

- ファイルは、**入口のパッケージ**（`<Dir(U, N)>/`）に、プラグインごとに決めた名前のファイル一つで置く。型にメソッドを足すもの（`go-valid` の `Validate`）だけは、Go がメソッドを、型を定義したパッケージにしか書かせないので、**型のパッケージ**（「型のパッケージ」。関数を持たない名前空間では、入口のパッケージ）に置く。自分のパッケージを持つもの（`go-cli`）は、`<Dir(U, N)>/` の下の、プラグインごとに決めたディレクトリに置く。
- 基底のファイル名（`doc.go`、`types.go`、`ops.go`、`states.go`、`classify.go`、`wiring.go`）と、ディレクトリ名 `inner`、`types` は使わない。テストのパッケージ（「単体テストの口」の「テストのパッケージ」）の中では、基底のファイル名 `doc.go`、`leaves.go`、`classify.go`、`fakes.go`、`hozo-tests.go.json` を使わない。
- パッケージに足す非公開の名前には、プラグインごとの前置きを付ける。公開の名前は、プラグインの文書に並べる。
- **テストのコード**（`testing` を import するコード。自分の層を通すテストを受け取る口と、それを走らせる関数）は、入口にも、定義のパッケージにも置かず、N の**テストのパッケージ**（「単体テストの口」の「テストのパッケージ」）に、プラグインごとに決めた名前のファイル一つで置く。ファイルの決まり（N の入口のパッケージを、いつも名前を書いて import する。import の名前に使わない識別子を、`Imports.Reserve` に渡す。失敗の文面は英語）は、基底のファイルと同じで、非公開の名前の前置きも、同じものを使う。基底が、そのパッケージを作らない出力でも、置いてよい（`doc.go` は、基底が、口か、にせものを出すときにだけ、ある）。テストのパッケージの、基底の公開の名前の一覧は、`golang.PublicTestNames`（`docs/packages.md`「`plugin/golang`（公開）」）で求め、当たれば、自分の診断で報告する。

| トランスパイラ | ファイル | 置くパッケージ | 非公開の名前の前置き | 文書 |
|---|---|---|---|---|
| `go-valid` | `validate.go` | 型 | `valid` | `docs/plugin-go-valid.md` |
| `go-sqlite` | `db.go` | 入口 | `db` | `docs/plugin-go-sqlite.md` |
| `go-http` | `http.go` | 入口 | `http` | `docs/plugin-go-http.md` |
| `go-http` | `http.go` | テスト | `http` | `docs/plugin-go-http.md`「インテグレーションテストの口」 |
| `go-cli` | `cli/cli.go`（パッケージ `cli`） | 自分のパッケージ | — | `docs/plugin-go-cli.md` |
| `go-design` | `design.go` | テスト | `design` | `docs/plugin-go-design.md`「出力」 |

- テストのコードを置く相乗り（表の、置くパッケージが「テスト」の行）は、同じディレクトリに、自分の、テストの対応表 `hozo-tests.<トランスパイラの名前>.json` も置く（`hozo-tests.go-http.json`、`hozo-tests.go-design.json`。「テストの対応表」）。
- 相乗りは、置き場と名前を `plugin/golang`（`docs/packages.md`「`plugin/golang`（公開）」）で求める。この文書の規則を自分で書き写さない（`docs/plugin-protocol.md`「相乗りが基底の形を知る方法」）。
- 表にないトランスパイラ（プロジェクトの中のプラグインのものなど）も、同じ決まりに従う: 表のファイル名、前置き、公開の名前を避け、自分のものを自分の文書に書く。
- 関数の引数の並び（最初の `ctx` を含む。「op の形」）を決めるのは基底だけで、相乗りは変えられない。相乗りが基底の出力に足せるのは、自分のファイルと、宣言した継ぎ目（「継ぎ目」）だけ。
- 基底の名前と、相乗りの公開の名前、利用者の宣言の名前が当たること（model `Schema` と `go-sqlite` の `Schema` など）は、当たった名前を足すトランスパイラが、自分の診断で報告する（基底は GO-2）。当たる相手（定義のパッケージの公開の名前）の一覧は、`golang.PublicNames`（`docs/packages.md`「`plugin/golang`（公開）」）で求める。

## 名前

| IR | Go |
|---|---|
| 宣言名（型の位置） | 先頭を大文字に（NAME-1 は warning なので小文字で始まる名前も来る） |
| 合成した model `OrderPage.Ready`、`ClientError.Value`（`docs/ir.md`「正規化」） | 宣言した名前空間より後ろのセグメントをそれぞれ先頭大文字にして連結: `OrderPageReady`、`ClientErrorValue` |
| フィールド名 | 先頭を大文字に（`id` → `Id`、`userId` → `UserId`）。`json:"<元の名前>"` タグ。型が省略可能なら `,omitempty` |
| op 名 | 先頭を大文字に（メソッド名: `login` → `Login`）。`export` の有無によらない |
| read 名 | 先頭を大文字に（状態の単位のメソッド名: `userByEmail` → `UserByEmail`） |
| effect 名 | 定数は、先頭を大文字にし、`Effect` を前に付ける（`EffectCreateSession`）。状態の単位のメソッド名は、先頭を大文字に（`createSession` → `CreateSession`） |
| 例外の引数名 | フィールド名と同じ（先頭を大文字に。`json:"<元の名前>"` タグ。型が省略可能なら `,omitempty`） |
| 直に呼ぶ組み込みの効果の操作 | 先頭を大文字に（状態の単位のメソッド名: `post` → `Post`、`put` → `Put`、`delete` → `Delete`） |
| 状態の単位 | 状態（か状態の分類）の名前を先頭大文字にして `State` を付ける（`UsersState`、`OutputsSyncState`） |
| 別の名前空間の口 | 名前空間の FQN のセグメントをそれぞれ先頭大文字にして連結し、`Ops` を付ける（`store` → `StoreOps`、`shop.admin` → `ShopAdminOps`） |
| 分類 C の列挙と述語 | 型 `<C>`、定数 `<C><ケース名>`、述語のインターフェース `<C>Cases`、判定関数 `Classify<C>` |
| 継ぎ目のメソッド（`Seams`） | op 名を先頭大文字にしたものに、デコレータの FQN のセグメントをそれぞれ先頭大文字にして連結したものを付ける（`server.login` の `@rate.limited` → `LoginRateLimited`） |
| 継ぎ目を持つ op の、中身のメソッド | op 名の先頭を小文字にして `Body` を付ける（`login` → `loginBody`）。非公開の名前 |
| `New` の引数と、構造体 `ops` のフィールド | 「New」 |
| op・read・effect の引数名、配線の `val` と `for` の変数の名前 | そのまま。下の「局所の名前」で当たれば末尾に `_` |

- 先頭だけを大文字にする（`Id`、`Url`）のは、Go の慣習（`ID`）より決定性と予測しやすさを取るため。
- **局所の名前**: メソッドの中で、次の名前は生成したコードが使う。op・read・effect の引数名と、`val`・`for` の変数の名前がこれと同じなら、末尾に `_` を付ける: Go の予約語、事前宣言の識別子（`error`、`string`、`nil`、`len` など）、`o`、`ctx`、`err`、`ptr`、定義のパッケージの `ops.go` と `wiring.go` が import するパッケージの名前（`context`、`time`、`runtime`、名前空間のパッケージ）。
- 生成したコードが作る局所の変数（状態を読んだ値、途中の値）の名前は「配線の本体」。引数名、`val`・`for` の変数の名前、先に作った変数と当たれば、当たらなくなるまで末尾に `_` を足す。
- **名前が当たれば GO-2**: 定義のパッケージの公開の名前どうし（型、`Effect…`、`Ops`、`Leaves`、`<状態>State`、`<Ns>Ops`、`Seams`、分類の名前、`New`、`Classify<C>`。大文字化で同じになる `id` と `Id`、合成した名前と宣言の名前を含む）、一つの構造体のフィールドどうし（例外の引数を含む）、一つのインターフェースのメソッドどうし（状態の単位の、形で決まるメソッド `IsEmpty` / `Get` / `Post` / `Put` / `Delete` と、ケースの述語と、その単位の read と効果の名前を先頭大文字にしたもの: 状態 `users` の read `get`、同じ状態の read `findUser` と効果 `FindUser`。形で決まるメソッドは、その単位に出すものだけを数える: 配線が `users.post(…)` を直に呼んでいなければ、`Post` は単位になく、効果 `post` は当たらない。`Ops` の中の `foo` と `Foo`。`Seams` の中の、op `loginRate` の `@limited` と op `login` の `@rate.limited`、`@repeatable` のデコレータを一つの op に二度付けたもの）、構造体 `ops` のフィールドと中身のメソッド（状態 `loginBody` と、継ぎ目を持つ op `login`）。直すのは仕様の名前。

## 型

| IR | Go |
|---|---|
| `string` / `boolean` / `int` / `float` | `string` / `bool` / `int64` / `float64` |
| `datetime` / `duration` | `time.Time` / `time.Duration` |
| `date` / `time` | `runtime.Date` / `runtime.TimeOfDay`（Go に、日付だけ、時刻だけの型がないので、「runtime パッケージ」が持つ） |
| `optional(T)` | T が `array`、インデックスの model への `ref`、union（`arms`）への `ref` ならそのまま（`nil` が「なし」）。それ以外は `*T` |
| `array(T)` | `[]T` |
| model への `ref` | 構造体の型。インデックスの model なら、名前付きのマップの型（「model」） |
| scalar への `ref` | 名前付きの型 |
| union（`arms`）への `ref` | インターフェースの型 |
| `case(C, cases, of)` | `of` の型（絞り込みは Go の型にしない） |

`never` は型にならない（「op の形」）。別の名前空間の型は、置き場の入口のパッケージの名前で修飾する（「パッケージと import パス」）。

### model

```go
// User はユーザー
type User struct {
	Id    UserId `json:"id"`
	Email Email  `json:"email"`
	Name  string `json:"name"`
	// パスワードのハッシュ
	Hash string `json:"hash"`
}
```

- フィールドは `Model.fields` の順。フィールドの説明は、フィールドの前の行のコメント。
- **継承した model**（`Model.extends`）は、元の model とは別の、普通の構造体。フィールドは展開済みの `fields`（なくしたフィールドは、もうない）。元の model から写す関数は出さない。コメントに継承の一行を足す（「コメント」）。
- union のアームの合成 model には、union のマーカーメソッドを足す（「union」）。
- **インデックスの model**（`Model.index`。`docs/types.md`「インデックスの model」）は、構造体ではなく、名前付きのマップの型: `type Attrs map[string][]string`（キーと値の Go の型は、この表のとおり）。コメントは、ほかの model と同じ。値のないマップ（`nil`）は、空の構造と同じに扱う。
- model から出すのは、構造体だけ。値を作る関数（初めの値を入れる関数）は出さない: フィールドの既定値は、言語にない（`docs/decisions.md`「設計書から離れた決定」S24）。使う側は、零値（`var in <Model>`、`<Model>{}`）から始め、決まった値を入れるのは、それを言うデコレータを読む相乗り（`docs/plugin-go-http.md`「入力の読み方」、`docs/plugin-go-cli.md`「引数の解釈」）。

### scalar

```go
// Email はメールアドレス
type Email string
```

元の型（`Scalar.base`）をそのまま書いた、名前付きの型（元が scalar なら `type A B`）。メソッドは出さない。

### union

```go
// OrderPage は注文の画面の状態
type OrderPage interface{ isOrderPage() }

func (OrderPageReady) isOrderPage() {}
func (OrderPageEmpty) isOrderPage() {}
```

- `variant: "set"` の union は Go の型にしない（例外と効果の集合は `throws` と `effects` に展開済み）。
- マーカーメソッドは、アームの順に、インターフェースの直後に並べる。
- アームの型が、**N の model への `ref`**（その場で書いた合成 model を含む）なら、その構造体にマーカーを生やす。
- それ以外のアームの型（別の名前空間の model、scalar、核の型、`optional`、`array`、union）は、マーカーをその型に生やせないので、定義のパッケージに包む型 `type <Union><アーム名> struct{ Value <型> }` を作り、それにマーカーを生やす。包む型は、マーカーメソッドの並びの後ろに、アームの順で置く。

### exception

```go
// InvalidCredentials: メールアドレスかパスワードが一致しない
type InvalidCredentials struct{}

func (InvalidCredentials) Error() string         { return "type.InvalidCredentials" }
func (InvalidCredentials) HozoException() string { return "type.InvalidCredentials" }

var _ runtime.Exception = InvalidCredentials{}
```

- 引数を持つ例外（`exception LimitExceeded(retryAfter: duration);`。`docs/grammar.md`「exception / effect / read」）は、**引数ごとにフィールド一つ**の構造体。並びは `Exception.params` の順で、名前とタグは model のフィールドと同じ（「名前」）。引数のない例外は `struct{}`。値を一つの `Value` に包む形は、なくした。

```go
// LimitExceeded: 呼び出しの頻度の制限を超えた
type LimitExceeded struct {
	RetryAfter time.Duration `json:"retryAfter"`
}
```

- 二つのメソッドが返すのは例外の FQN（引数の値は入れない）。`var _ runtime.Exception = <型>{}` は、引数があっても、零値のリテラルで書く。
- 例外は**値で**返し、値の型で見分ける（`errors.As(err, &e)` の `e` は `InvalidCredentials`）。生成したコードはポインタで返さない。人が書く葉も、値で返す。
- 配線の `throw E(<値>, …)` は、この構造体のリテラルを作る（「match」の「アームの本体」）。

### effect

```go
// EffectCreateUser はユーザーを登録する
const EffectCreateUser runtime.Effect = "store.createUser"
```

効果ごとに、この定数一つ。効果を行うメソッドは、定数とは別に、宛先の単位に出す（「状態の単位」）。宛先が分類の遷移（`Effect.target` が `transition`）の効果は、コメントの末尾に一行足す: `// effects <宛先の FQN> as <前> -> <後>`（ケースの集合は `any` か `A, B`）。行の遷移を持つ効果（`Effect.row`。`docs/check.md`「行の遷移」）は、コメントの末尾に一行足す: `// [<絞り込んだ引数の表示>、]effects <状態の FQN>.<post / put / delete> <行の遷移の表示>`（表示の形は「コメント」の「行の遷移の表示」「絞り込んだ引数の表示」）。

```go
// EffectStartProduction は作り始める
// order on shop.Stage.Accepted、effects shop.orders.put as shop.Stage.InProduction
const EffectStartProduction runtime.Effect = "shop.startProduction"
```

## op の形

op のメソッドの形は、その op の宣言だけで決まり、どのインターフェースに現れても、どの名前空間から見ても同じ。`export` の有無にも、純粋かどうか（`Op.signature.pure`）にも、`throws` の有無にもよらない。

| 部分 | 規則 |
|---|---|
| 名前 | op 名を先頭大文字にしたもの |
| `ctx` | 最初の引数に、**いつも** `ctx context.Context` を取る |
| 引数 | `Op.params` の並びのまま、`<局所の名前> <型>` |
| 戻り値 | `returns` が型なら、その型。`null`（void）と `never` なら、値はない |
| `error` | 最後の戻り値に、**いつも** `error` を返す。値があれば `(T, error)`、なければ `error` だけ |

- **基底は、どの関数に `ctx` と `error` が要るかを判定しない**（原則の持ち主は `docs/requirements.md`「op の中身に立ち入らない」）。基底が出す関数のうち、仕様の宣言を写したものは、どれも同じ形: 最初の引数が `ctx context.Context`、最後の戻り値が `error`。op（配線でも葉でも）のほかに、`read` と効果のメソッド（「状態の単位」）、分類の述語と判定関数（「分類」）、継ぎ目のメソッドとその `body`（「Seams」）。
- 仕様の宣言を写したものでない関数は、Go がその形を決めているので、この形にしない: `New`（「New」）、補助 `ptr`（「値」）、union のマーカーメソッド（「union」）、例外の `Error()` と `HozoException()`（「exception」）、テストのパッケージの関数とインターフェース（`testing` が形を決める。「単体テストの口」）。
- `ctx` は、中断と期限を伝えるための、標準の `context.Context`（Go の作法）。自前のコンテキスト型は作らず、基底は、値を載せて引数の代わりにすることには使わない。生成したコードは、受け取った `ctx` を、呼ぶ先にそのまま渡す（継ぎ目の実装が、`body` に渡す `ctx` に、呼び出しの間だけのものを載せることはある: `docs/plugin-go-sqlite.md`「トランザクション」）。
- 純粋さ（`docs/check.md`「純粋」）は、言語が仕様から割り出す事実として IR に残る。基底は、それを読まない: 関数の形にも、コメントにも使わない。
- `throws` を持たない op の実装も、`error` を返せる。仕様の例外でない `error`（中断、入出力の失敗）は、配線がそのまま呼び出し元へ返す（「呼び出し」の「失敗の扱い」）。
- **`never` を返す op** は、値を返さず `error` だけを返す（`Unauthorized(ctx context.Context) error`）。中身は基底が出す（「op の置き場」）。

## 実装を受け取る口

人やプラグインが実装するものは、どれも**インターフェース**で、実装は `New` の**位置引数**で渡す。実装の漏れ（メソッドの書き忘れ、渡し忘れ）は、コンパイルエラーになる。関数のフィールドに一つずつ代入する形、パッケージの変数、op ごとの関数の型（`<Op>Func`）は持たない。

- 生成したメソッドは、実装が `nil` かどうかを確かめない。「実装がない」を実行のときに返す道（`*runtime.MissingError`）はない。`nil` と書いて渡せば、使われたところで panic になる。
- 同じ出力の `New` は何度呼んでもよく、返る値は互いに何も共有しない（テストを並列にでき、同じ出力を一つのプロセスで二つ使える）。

### op の置き場

**op は、関数の枠だけを作る**（`docs/requirements.md`「出力を推論しない」）。状態に触れるのは `read` と効果で、op ではない（`docs/check.md`「状態に触れるもの」）。だから、op の実装のある場所は、三つだけ。上から順に、最初に当たる行。

| # | op | 実装のある場所 |
|---|---|---|
| 1 | 単位の対象の配線（本体を持つ op） | 生成した本体（`wiring.go`。「配線の本体」） |
| 2 | `never` を返す op | 生成した本体（`ops.go`）: `return <例外の型>{}` |
| 3 | 葉（本体を持たない op）と、単位の対象でない配線（「出力の対象」） | `Leaves`。人が書く |

- 葉は、状態に触れない（`reads` と `effects` の節を書けない。`docs/check.md`「葉の op」）ので、状態の単位には置かない。葉に付いたデコレータが `reads` / `effects` を運んでいても（`Op.carried`。同「デコレータが運ぶ reads と effects」）、置き場は `Leaves` のまま: 状態に触れるのは、デコレータの処理で、葉の中身ではない。
- op のデコレータは、置き場を変えない。op の中身の前後に処理を足すデコレータは、継ぎ目になる（「継ぎ目」）。
- **`read` と効果は op ではなく、置き場は、いつも状態の単位**（「状態の単位」）。配線は、それを op と同じ形で呼ぶ（「呼び出し」）。

### Ops

```go
// Ops は、この名前空間の外から呼べる op: 仕様で export を付けた op。宣言順。New が返す。
type Ops interface {
	// Login はログインする
	//
	// Throws: type.InvalidCredentials, rate.LimitExceeded
	// Reads: store.users
	// Effects: store.createSession
	Login(ctx context.Context, in LoginInput) (LoginOk, error)
	…
}
```

- **入口の `Ops`** は、N の op のうち `export` を付けたものだけ（葉でも配線でも）。宣言順。名前空間の外（別の名前空間の配線、相乗りの出力、人が書くコード）が使う。
- **内側の `Ops`**（分けた出力だけ）は、N の op のすべて。宣言順。コメントは `// Ops は、この名前空間のすべての op（export の有無によらない）。宣言順。New が返す。テストは、内部の op をここから呼ぶ。`
- 分けない出力では、N の op はすべて `export` なので、一つの `Ops` がどちらでもある。
- メソッドの間は一行空ける。
- op が継ぎ目を持つかどうかは、`Ops` を変えない。`Ops` のメソッドを呼べば、継ぎ目を通る（「継ぎ目」）。

### Leaves

```go
// Leaves は、人が実装する葉の op: 本体のない op。状態には触れない。export の有無によらない。
// 実装は New の引数 leaves に渡す。メソッドが一つでも足りない値は、渡すところでコンパイルできない。
type Leaves interface {
	// CheckPassword はパスワードが合わなければ InvalidCredentials（人が書く）
	//
	// Throws: type.InvalidCredentials
	CheckPassword(ctx context.Context, user pkgtype.User, password pkgtype.Password) error
	…
}
```

「op の置き場」の 3 の op を、宣言順に。一つもなければ出さない。その op の語彙を読む相乗りが、一部のメソッドを実装した型を出してもよい（人は、それを自分の型に埋め込んで、残りを書く）。

### 状態の単位

基底の状態ごとに一つ、インターフェース `<状態>State` を出す。その状態に触れるもののすべて: 形で決まるメソッドと、その状態の `read` と、その状態を宛先にする効果。状態そのもの（保存場所）は出さない。保存場所は、実装する者（保存先の相乗りか、人）が決める。

```go
// UsersState は状態 store.users（list<type.User>）の単位。実装は New の引数 users に渡す。
// メソッドが一つでも足りない値は、渡すところでコンパイルできない。
type UsersState interface {
	// IsEmpty は問い合わせ（形 list で決まる）: store.users に要素が一つもなければ true。
	IsEmpty(ctx context.Context) (bool, error)
	// Get は問い合わせ（形 list で決まる）: store.users の値。すべての要素を、足した順に。
	Get(ctx context.Context) ([]pkgtype.User, error)
	// UserByEmail は問い合わせ（read store.userByEmail）: email が同じユーザー
	UserByEmail(ctx context.Context, email pkgtype.Email) (*pkgtype.User, error)
	// CreateUser は命令（効果 store.createUser、effects store.users.post）: ユーザーを登録する
	CreateUser(ctx context.Context, user pkgtype.User) error
}
```

メソッドは、この順: 形で決まる問い合わせ、形で決まる命令（配線が直に呼ぶものだけ）、`read`（宣言順）、効果（宣言順）。どれも、op と同じに、`ctx` を取り、`error` を返す（「op の形」）。

| メソッド | 出すとき | 形 |
|---|---|---|
| 形で決まる問い合わせ（下の表） | いつも | 下の表 |
| 形で決まる命令 `Post` / `Put` / `Delete` | 単位の対象の配線が、その状態の組み込みの効果を直に呼んでいる（`users.post(user);`）操作だけ。`Post`、`Put`、`Delete` の順 | `<操作>(ctx context.Context[, value <T>]) error`。引数は、`docs/check.md`「直に呼ぶ組み込みの効果」の表のとおり: 値を取る操作は、状態の要素の型 T の `value` 一つ、`nullable` の `delete` は、なし |
| `read`（`Read.state` が、その状態） | N の `read` のすべて。配線が呼んでいるかによらない | `<Read>(ctx context.Context, <引数>) (<結果の型>, error)`。引数は `Read.params` の並びのまま |
| 効果（宛先の単位が、その状態の単位） | N の効果のすべて。同上 | `<Effect>(ctx context.Context, <引数>) error`。引数は `Effect.params` の並びのまま |

**効果の宛先の単位**: 効果の宛先（`Effect.target`）が、組み込みの効果か、基底の状態の遷移なら、その基底の状態の単位。状態の分類の遷移なら、その分類が述語で決まるならその分類の単位、型で決まるなら、その分類のただ一つの対象の状態の単位。

**形で決まる問い合わせ**は、どの状態の単位にも、いつも出す。その名前空間の配線が状態を読むかどうかによらない（テストが、効果の結果の判定に使う）。

| 形（`State.shape`） | 問い合わせ |
|---|---|
| `list` | `IsEmpty(ctx context.Context) (bool, error)`、`Get(ctx context.Context) ([]T, error)`（すべての要素を、足した順に） |
| `nullable` | `IsEmpty(ctx context.Context) (bool, error)`、`Get(ctx context.Context) (<optional(T) の型>, error)`（値がなければ `nil`） |
| `single` | `Get(ctx context.Context) (T, error)` |

- `Get` は、状態を名前で読んだときの値（名前を付けない `read`。`docs/check.md`「状態の参照」）。戻り値の型は `State.type` の Go の型。
- 生成した配線が呼ぶ形で決まる問い合わせは `Get` だけ（「状態の参照」）。`IsEmpty` は、テストと人のコードのためのもの。
- **形で決まる命令を、呼ばれているものだけにする**のは、実装する者に、誰も呼ばない操作を書かせないため（別の名前空間の口 `<M>Ops` と同じ決め方）。問い合わせと違って、テストが効果の結果を見るのには要らない。配線の本体を書き換えて、直に呼ぶ操作が増えれば、単位のメソッドが増え、実装は、渡すところでコンパイルできなくなる。
- **`read` と効果を、呼ばれていなくても出す**のは、宣言が、その状態にできることの約束だから（`docs/check.md`「read と効果」）。`read` の引数と結果の型、効果の引数は、宣言に書かれたままで、基底は、宛先の状態の型と照らさない（照らすのは、デコレータを読む相乗り）。
- **例外を投げる効果**（`Effect.throws`。`docs/check.md`「read と効果」）も、宛先の単位のメソッドで、形は、ほかの効果と同じ。実装は、例外を、`error` として、値で返す（「exception」）。配線は、ほかの呼び出しの失敗と同じに、そのまま返す（「呼び出し」）。基底が `throws` から出すのは、単位のメソッドのコメントだけ（「コメント」の「状態の単位のメソッド」）。
- **行の遷移を持つ効果**（`Effect.row`）も、宛先の単位のメソッドで、形は、ほかの効果と同じ（引数は `Effect.params` の並びのまま。行の引数も、ただの引数）。基底が行の遷移から出すのは、コメントだけ: 効果の定数の一行（「effect」）と、単位のメソッドのコメントの宛先（「コメント」の「状態の単位のメソッド」）。**出さないもの**: 効果の後で、行が、言ったケースにあるかを確かめるコード、遷移の表。メソッドの中身が、宣言のとおりに行を変えることは、実装する者（相乗りか、人）が担う。
- **絞り込んだ引数を持つ効果**は、入口で、引数のケースを確かめる（「効果の入口の確かめ」）。単位のインターフェースの形は、変わらない。
- **述語で決まる状態の分類** C も、同じ形の単位 `<C>State` を持つ。メソッドは、ケースごとの述語（宣言順。どのケースも持つ: 「分類」。`<ケース名>(ctx context.Context) (bool, error)`）、命令（その分類の遷移を宛先にする効果。宣言順）。型で決まる状態の分類は、単位を持たない。
- 二つの基底の状態にまたがるものは、ない: `read` の宛先は基底の状態一つ（`docs/check.md`「read の宛先」）、効果の宛先は一つの状態の一つの変化（同「効果の宛先」）。二つの状態を一緒に変える処理は、二つの効果を呼ぶ配線で書く。
- 単位のメソッドの実装は、その `read` か効果のデコレータを読む相乗りが出すか（`docs/plugin-go-sqlite.md`「状態ごとの型」）、人が書く。デコレータのない `read` と効果、形で決まる命令のうち、相乗りが出さなかったものは、人が、相乗りの型を埋め込んで書き足す。

### 効果の入口の確かめ

絞り込んだ引数を持つ効果は、入口で、引数が、言ったとおりのケースにあるかを確かめる（`docs/decisions.md`「決めたこと」371、372）。葉の op が、絞り込んだ型で返した値や、読んでから時間が経った行を、状態を書く手前で、最後に確かめる所。

```ts
classify Standing of Payment { Pending; Held; Settled; }
effect fn capture(payment: Payment on Standing.Held, fee: Yen): effects payments.put as Standing.Settled;
```

```go
// checkedPaymentsState は、状態 pay.payments の単位を包む: 絞り込んだ引数を持つ効果の入口で、引数が、言ったとおりのケースにあるかを確かめる。
// New が、引数 payments を、これで包む。ほかのメソッドは、包んだ単位を、そのまま呼ぶ。
type checkedPaymentsState struct {
	PaymentsState
	standing StandingCases
}

// Capture は、効果 pay.capture の入口の確かめ: payment が、pay.Standing の Held にあること。違えば、*runtime.CaseError を返し、効果は、起きない。
func (o checkedPaymentsState) Capture(ctx context.Context, payment Payment, fee Yen) error {
	if kind, err := ClassifyStanding(ctx, o.standing, payment); err != nil {
		return err
	} else if kind != StandingHeld {
		return &runtime.CaseError{Effect: "pay.capture", Param: "payment", Classify: "pay.Standing", Want: []string{"Held"}, Got: [...]string{"Pending", "Held", "Settled"}[kind]}
	}
	return o.PaymentsState.Capture(ctx, payment, fee)
}
```

- **確かめる引数**は、N の効果の引数のうち、型が、述語で決まる値の分類か op の分類で絞り込んだ型（`T on C.K`。`Param.type` が、そのまま `case`）のもの。行の引数でなくてもよい。型で決まる分類で絞り込んだ引数は、確かめない（値の型から決まる）。`optional` や `array` の中の絞り込み（`list<Order on Stage.Draft>`）も、確かめない。op と `read` の引数も、確かめない（372）。
- **包む型** `checked<状態>State` は、確かめる引数を持つ効果が、一つでもある状態の単位ごとに一つ。`states.go` の、その単位のインターフェースのすぐ後ろに出す。非公開の型で、入口には、再公開しない。フィールドは、埋め込んだ単位（`<状態>State`）と、使う分類ごとの述語（名前は、`New` の、その分類の引数と同じ）。メソッドは、確かめる引数を持つ効果だけ（宣言順）。ほかのメソッドは、埋め込んだ単位のものが、そのまま呼ばれる。
- **コメント**: 包む型は、例の二行（`<状態の FQN>` と、`New` の引数の名前が入る）。メソッドは、`// <メソッド> は、効果 <効果の FQN> の入口の確かめ: <句>。違えば、*runtime.CaseError を返し、効果は、起きない。` で、句は、確かめる引数ごとの `<引数> が、<分類の FQN> の <ケース> にあること` を、宣言の順に「、」でつないだもの。ケースが二つ以上なら「か」でつなぐ（`Sent か Done`）。
- **メソッドの中身**: レシーバは `o`。確かめる引数を、宣言の順に、一つずつ: 判定関数 `Classify<C>` を呼ぶ。判定関数が `error` を返したら（述語の `error` か、分類の誤り）、そのまま返す。返ったケースが、引数の型が言うケース（`Ks`）のどれでもなければ、`*runtime.CaseError`（「runtime パッケージ」）を返す。すべて通れば、埋め込んだ単位の、同じ名前のメソッドを、同じ引数で呼んで返す。
- 判定関数に渡す値は、引数そのもの。分類の対象の型が `optional(T)` で、引数の Go の型が `T` なら、`&<引数>`。
- **包むのは `New`**（「New」）: 状態の単位の引数を、`checked<状態>State{<引数>, <述語>…}` で包んで、構造体 `ops` に入れる。配線は、いつも、包んだ単位を呼ぶので、単位の実装が、相乗りのものでも、人が書いたものでも、確かめは掛かる。`New` に渡す前の単位を、人のコードが直に呼べば、確かめは掛からない。
- 述語は、`New` が、引数で受け取る（「New」の 3。配線が `match` に使っていない分類でも、確かめに使うなら、受け取る）。
- 確かめの失敗は、仕様の例外ではない（`throws` に出ない。`docs/check.md`「行の遷移」）。配線は、ほかの呼び出しの失敗と同じに、そのまま返す。

### 別の名前空間の op

```go
// StoreOps は名前空間 store の op のうち、この名前空間の配線が呼ぶもの。実装は New の引数 store に渡す。
// メソッドの形は相手の op の宣言だけで決まる（どの名前空間から見ても同じ）ので、store の出力の Ops は、そのまま渡せる。
type StoreOps interface {
	// FindUser は op store.findUser: email からユーザーを引く
	//
	// Reads: store.users
	FindUser(ctx context.Context, email pkgtype.Email) (*pkgtype.User, error)
	…
}
```

- 単位の対象の配線が呼ぶ別の名前空間 M ごとに一つ、インターフェース `<M>Ops`。メソッドは、M の op のうち、**単位の対象の配線が呼ぶものだけ**。並びは M の宣言順。
- メソッドの形は、相手の op の宣言から「op の形」で決める（引数は相手の `Op.params` のまま）。相手の出力の `Ops`（`export` の op をすべて持つ。呼べるのは `export` の op だけ。NS-5）が、型を共有していれば、そのまま満たす: `store.New(…)` の戻り値を、`server.New` の引数 `store` に渡せる。足りない op はコンパイルエラーになる。
- 相手が出力を持たない名前空間（単位を置かない、人の操作や外部の仕組み）でも同じで、人かテストが、このインターフェースを満たす型を書く。
- 相手の op そのものは import しない。import するのは、引数と戻り値の型だけ。

### New

```go
// New はこの名前空間の Ops を組み立てる。引数の並びは、leaves、別の名前空間（名前の辞書順）、seams。
// 引数が nil かどうかは確かめない。
func New(leaves Leaves, mailer MailerOps, store StoreOps, seams Seams) Ops {
	return &ops{leaves: leaves, mailer: mailer, store: store, seams: seams}
}

// ops は New が返す非公開の構造体。フィールドは New の引数そのまま。メソッドは、この名前空間の op のすべて。
type ops struct {
	leaves Leaves
	mailer MailerOps
	store  StoreOps
	seams  Seams
}
```

引数の並びは次の順。当たるものがなければ、その引数はない。

| # | 引数 | 名前 | 型 |
|---|---|---|---|
| 1 | 人が実装する葉（`Leaves` があるとき） | `leaves` | `Leaves` |
| 2 | 状態の単位。基底の状態と、述語で決まる状態の分類を、宣言順に | 状態（分類）の名前 | `<状態>State` |
| 3 | 単位の対象の配線が `match` に使う、述語で決まる値の分類と op の分類と、N の効果の入口の確かめ（「効果の入口の確かめ」）が使う分類（どちらも、別の名前空間のものを含む）。分類の FQN の辞書順 | 分類の名前 | `<C>Cases`（別の名前空間のものは修飾する） |
| 4 | 単位の対象の配線が呼ぶ別の名前空間。FQN の辞書順 | 名前空間の FQN のセグメントをつないだもの（`store`、`shopAdmin`） | `<M>Ops` |
| 5 | 継ぎ目（`Seams` があるとき。「継ぎ目」） | `seams` | `Seams` |

- 引数の名前は、先頭を小文字にする（名前空間のセグメントは、二つ目から先頭大文字にしてつなぐ）。Go の予約語か事前宣言の識別子なら末尾に `_`（名前空間 `type` は `type_`）。`ops` も末尾に `_`（`New` の本体が、構造体 `ops` のリテラルを書く。状態 `ops` や、別の名前空間 `ops` の引数は `ops_`）。分けた出力（「内側のフォルダと入口」）では、`inner` も末尾に `_`（入口の `New` が、内側のパッケージを `inner` の名前で呼ぶ）。先に並んだ引数の名前と当たれば、当たらなくなるまで末尾に `_` を足す。構造体 `ops` のフィールドは、引数と同じ名前、同じ順。
- 別の名前空間を辞書順にするのは、配線の本体を書き換えただけで並びが変わらないようにするため。状態は宣言順なので、仕様で宣言を入れ替えると、`New` を呼ぶ行が変わる。
- 継ぎ目を最後に置くのは、継ぎ目を宣言するトランスパイラを単位に足し引きしても、ほかの引数の位置が動かないようにするため。
- 引数の型が同じ二つの状態（同じ形、同じ要素の型で、どちらにも `read` も効果もない）は、入れ替えて渡してもコンパイルが通る。位置引数と引き換えに受け入れた弱さ。
- `New` のコメントの一行目は `// New はこの名前空間の Ops を組み立てる。引数の並びは、<句>。` で、句は、ある引数の種類を、この順に「、」でつないだもの: `leaves`、`状態の単位（宣言順）`、`分類の述語（FQN の辞書順）`、`別の名前空間（名前の辞書順）`、`seams`。二行目は `// 引数が nil かどうかは確かめない。`。引数が一つもなければ、`// New はこの名前空間の Ops を組み立てる。` の一行だけ。

- **入口の確かめを持つ単位**（「効果の入口の確かめ」）は、`New` が包む: 構造体のリテラルの、その単位の所が、`<引数>: checked<状態>State{<引数>, <述語の引数>…}` になる（述語は、包む型のフィールドの順 = 分類の FQN の辞書順）。構造体 `ops` のフィールドの型は、`<状態>State` のまま。包む単位が一つでもあれば、構造体 `ops` のコメントは、`// ops は New が返す非公開の構造体。フィールドは New の引数（入口の確かめを持つ状態の単位は、New が包んだもの）。メソッドは、この名前空間の op のすべて。`

**構造体 `ops`** は、N の op のすべてをメソッドに持つ（公開の名前）。配線のメソッドは `wiring.go`、それ以外は `ops.go` に、宣言順で置く。配線でないメソッドは、実装を呼ぶだけ。継ぎ目を持つ op は、公開の名前のメソッドが外側になり、下の表の中身は、その内側に入る（「継ぎ目」）。

| 置き場 | メソッドの中身 |
|---|---|
| `Leaves` | `return o.leaves.<Op>(ctx, <引数>)` |
| `never` を返す op | `return <例外の型>{}` |

`ops` の、配線でないメソッドには、コメントを書かない（継ぎ目を持つ op の外側と中身は、一行のコメントを持つ。「コメント」）。

## 配線の本体

単位の対象の配線ごとに、`wiring.go` に `ops` のメソッドを一つ出す。コメントは `Ops` のメソッドと同じ（「コメント」）。実行の順序は、文を辿る順（`docs/check.md`「Sig(O, v)」）で、並列にはしない（`disjoint` も。「文」）。継ぎ目を持つ配線は、このメソッドの名前が中身の名前（`loginBody`）になり、コメントが一行になる。本体の規則は変わらない（「継ぎ目」）。

```go
func (o *ops) Authenticate(ctx context.Context, token *pkgtype.Token) (pkgtype.Session, error) {
	session, err := o.store.FindSession(ctx, token)
	if err != nil {
		return pkgtype.Session{}, err
	}
	// match (session by store.Login)
	if session != nil {
		// LoggedIn: session は type.Session
		return o.RequireFresh(ctx, *session)
	}
	// LoggedOut
	return pkgtype.Session{}, pkgtype.Unauthorized{}
}
```

### 呼び出し

呼び出し（`Call`）の先は、op か、`read` か、効果（`Call.callee`。`docs/ir.md`「配線」）。どれも、同じ形のメソッドの呼び出しになる。

| 呼ぶもの（`Call.callee`） | Go |
|---|---|
| 同じ名前空間の op（葉でも配線でも、`export` の有無によらず） | `o.<Op>(ctx, <引数>)`。実装がどの口にあるか、継ぎ目を持つかは、書き方を変えない（継ぎ目を持つ op は、同じ名前空間の配線から呼んでも継ぎ目を通る） |
| 別の名前空間の op | `o.<名前空間の引数名>.<Op>(ctx, <引数>)` |
| `read`（`userByEmail(email)`） | `o.<読む状態の引数名>.<Read>(ctx, <引数>)`（`o.users.UserByEmail(ctx, email)`） |
| 効果（`createOrder(order);`） | `o.<宛先の単位の引数名>.<Effect>(ctx, <引数>)`（`o.orders.CreateOrder(ctx, order)`） |
| 直に呼ぶ組み込みの効果（`users.post(user);`） | `o.<状態の引数名>.<操作>(ctx[, <値>])`（`o.users.Post(ctx, user)`、`o.token.Delete(ctx)`） |

- 最初の引数は、いつも、受け取った `ctx`（「op の形」）。続く引数は、呼ぶ先の引数（`Op.params`、`Read.params`、`Effect.params`）の順に、`Call.args` の値（「値」）。渡されていない省略可能な引数は `nil`。`bind` で埋まる位置は「bind で埋まる位置」（op の呼び出しだけ）。
- `read` と効果を呼べるのは、宣言した名前空間の配線だけ（NS-6。`docs/check.md`「配線からの呼び出し」）なので、呼ぶ先は、いつも N の状態の単位。単位は「状態の単位」の表のとおり。
- 状態を名前で読むこと（名前を付けない `read`）は、呼び出しではなく、値（「状態の参照」）。
- **失敗の扱い**: 呼び出しは、どれも `error` を返す（「op の形」）。その結果を`val` か文で受ける呼び出しは、すぐ後に `if err != nil { return <零値>, err }` を置く（`<零値>` は、この配線の戻り値の型の零値。戻り値がなければ `return err`）。配線も、いつも `error` を返すので、返す先は、いつもある。受け取った `error` は、包まず、そのまま返す。
- **零値**: ポインタ・インターフェース・スライス・マップ（インデックスの model を含む）は `nil`、構造体は `<型>{}`、`time.Time`、`runtime.Date`、`runtime.TimeOfDay` は `<型>{}`、文字列が元の型は `""`、数が元の型は `0`、真偽値は `false`。

### 値

`Atom` を Go の式にする規則。「当てはめる型」は、その値を渡す位置の型（引数の型、フィールドの型、戻り値の型、`match` の型）。

| `Atom` | Go |
|---|---|
| `name`（`val`・`for` の変数・引数。パスなし） | その局所の名前 |
| `name` のパス（`in.body.email`） | フィールドを先頭大文字にしてつなぐ（`in.Body.Email`）。途中に省略可能な model があるときは「省略可能を通るパス」 |
| `state` | その文の前で読んだ値（「状態の参照」）と、パス |
| `literal` | 文字列は Go の文字列リテラル（`strconv.Quote`）、整数は 10 進、小数は `strconv.FormatFloat(v, 'g', -1, 64)`（`.` も `e` も含まなければ `.0` を足す）、真偽値は `true` / `false`、`duration` は `time.Duration(<ナノ秒>)` |
| `none` | `nil` |
| `record` | 当てはめる model の構造体リテラル `<型>{<フィールド>: <値>, …}`。並びは model のフィールドの順。書かれていないフィールド（省略可能なフィールドだけ。`docs/types.md`「当てはまる」）は書かず、零値のまま。当てはめる型が省略可能なら `&<型>{…}`。インデックスの model に当てはまるのは、空のリテラルだけで、`<型>{}`（空のマップ。省略可能でも、`&` を付けない） |

- **絞り込まれた対象**（`match` のアームの中の、対象と同じ `Atom`。`docs/types.md`「アームの中の対象の型」）は、「match」の「絞り込んだ値」の式に置き換えてから、上の表を当てる。
- **型をそろえる**: 値の型が、当てはめる型と Go で違うとき（`docs/types.md`「互換」の規則で渡せる組）は、次のように包む。

| 値の型 → 当てはめる型 | Go |
|---|---|
| T → `optional(T)` で、Go の型が `*T` | `ptr(<式>)`。リテラルは `ptr(<T の Go の型>(<リテラル>))` |
| T → `optional(T)` で、Go の型がそのまま（`array`、インデックスの model、union） | そのまま |
| scalar → その元の型（規則 11） | `<元の型>(<式>)` |
| `array` で、要素の型をそろえる必要があるもの | 同じ長さのスライスを作り、要素ごとにそろえて写すループを、その文の前に置く。値が `nil` なら `nil` のまま |
| `case(…)` → `of`、`case` どうし | そのまま（Go では同じ型） |

- `ptr` は `func ptr[T any](v T) *T { return &v }`。`wiring.go` が使っていれば、その末尾に一つだけ出す（「ファイル」）。
- **省略可能を通るパス**: パスの途中の値の型が `optional(model)`（Go ではポインタ）で、その先のフィールドを読むとき、式の型は `optional(<フィールドの型>)` になる（`docs/types.md`「式の型」）。その文の前に、途中の値を作る。`<x>` はそこまでの式、`<F>` は残りのパス。フィールドの型が既に `nil` を持てる型なら `v<n> = <x>.<F>`。残りのパスが、さらに省略可能な model を通るなら、同じ形を入れ子にする。

```go
var v0 <optional(フィールドの型) の Go の型>
if <x> != nil {
	v0 = ptr(<x>.<F>)
}
```

- 途中の値の名前は `v0`、`v1`、…（メソッドの中の通し番号）。引数名、`val`・`for` の変数の名前、import するパッケージの名前に当たる番号は、飛ばす（分類の判定の結果 `k0`、`k1`、… も同じ）。

### 文

| 文 | Go |
|---|---|
| `val x = 呼び出し`（op か `read`） | `x, err := <呼び出し>` と失敗の扱い。型を書いた `val` で、型をそろえる必要があれば、`v<n>, err := <呼び出し>` と失敗の扱い、`x := <そろえた値>` |
| `val x = 値` | `x := <値>`（当てはめる型は、値の型）。型を書いた `val` で、値の型と Go の型が違うとき（と、値がレコードリテラルのとき）は、`var x <val の型の Go の型> = <値>`（当てはめる型は、`val` の型） |
| `val x;`（空の宣言。名前を並べた宣言は、名前ごとに一行） | `var x <val の型の Go の型>`（型は、`Statement` の `type`） |
| `x = 呼び出し`（op か `read`） | `x, err = <呼び出し>` と失敗の扱い。型をそろえる必要があれば、`v<n>, err := <呼び出し>` と失敗の扱い、`x = <そろえた値>` |
| `x = 値` | `x = <値>` |
| `xs collect 呼び出し`（op か `read`） | `v<n>, err := <呼び出し>` と失敗の扱い、`xs = append(xs, <そろえた値>)` |
| `xs collect 値` | `xs = append(xs, <値>)` |
| `呼び出し;`（op か効果） | `if err := <呼び出し>; err != nil { return <零値>, err }`。値を返す op なら、値を捨てる: `if _, err := <呼び出し>; …` |
| `return 呼び出し`（op か `read`） | 型をそろえる必要がなければ `return <呼び出し>`（`return o.users.UserByEmail(ctx, email)`。戻り値の並びは、どちらも `(T, error)`）。型をそろえる必要があれば、`v<n>` に受けて失敗の扱いを置いてから `return <そろえた値>, nil` |
| `return 値` | `return <値>, nil` |
| `val x = match …` / `x = match …` / `xs collect match …` / `return match …` / `match …` | 「match」 |
| `for (val v of 並び) { … }` | `for _, v := range <並び> { … }`。ブロックが `v` を読んでいなければ `for range <並び> { … }`。並びは「値」の規則で書く（状態なら、先に読む: 「状態の参照」）。ブロックの中の文は、この表を、そのまま当てる |
| `disjoint { アーム … }` | `// disjoint` の一行と、Go のブロック `{ … }`。中に、アームを、書いた順に並べる。ブロックのアーム（`Arm.block` が `true`。中の文が一つでも）は、Go のブロック `{ … }`。文の一つのアームは、その文を、そのまま（この表を当てる） |
| `disjoint for (…) { … }` | `// disjoint` の一行と、上の `for` |
| 「返らない」呼び出し（`never` を返す op）の文と `return` | `return <零値>, <呼び出し>`（戻り値がなければ `return <呼び出し>`） |
| 本体の終わり（戻り値のない配線） | 最後に `return nil`。最後の文が「返らない」なら足さない |

- `val` を二度目以降に作るときも `:=` を使う（`err` は同じ変数が使い回される）。
- **`disjoint` は、直列に出す。** 仕様が言うのは、アームどうしが、互いに触れないことで、動かし方ではない（`docs/check.md`「disjoint」）。Go で、いちばん素直に読める形にする。並列に動かす形と、それを選ぶオプションは、持たない（`docs/decisions.md`「決めたこと」339、344）。足すときに決める点: 既定を直列にし、並列は、オプションで選ぶ形にするか。並列を選んだとき、検査が確かめない二点（`docs/check.md`「disjoint」の「検査が、確かめないこと」）が、人の責任になることを、オプションの説明に書くか。
- **失敗の扱いは、どのブロックの中でも、同じ。** `for` のブロックと、`disjoint` のブロックの中の失敗も、メソッドから `return <零値>, err` で抜ける。繰り返しは、そこで止まる。
- **`x, err = <呼び出し>` の `err`**: その位置で、`err` が、まだ、見えていなければ（そのメソッドの、囲んでいる Go のブロックの、上の行に、`err` を作る文 `…, err := …` が、ない）、その文の直前に `var err error` を置く。
- **読まれていない `val`**（WIRE-8）: Go は、使っていない変数を、コンパイルエラーにするので、`val x = …` の文と、空の宣言 `var x …` の直後に、`_ = x` を足す。
- 集める先の `val` は、`var xs []T` のまま、`append` で足す。一つも集めなければ、`nil` のスライス（長さ 0 の並び）。

```go
	var latest *pkgtype.Order
	var orders []pkgtype.Order
	// disjoint
	{
		latest, err = o.store.LatestOrder(ctx, session.UserId)
		if err != nil {
			return MyPage{}, err
		}
		orders, err = o.store.ListOrders(ctx, session.UserId)
		if err != nil {
			return MyPage{}, err
		}
	}
	var summaries []OrderSummary
	// disjoint
	for _, order := range orders {
		v0, err := o.Summarize(ctx, order)
		if err != nil {
			return MyPage{}, err
		}
		summaries = append(summaries, v0)
	}
```

### 状態の参照

配線が、自分の名前空間の状態を名前で読む位置（`Atom` の `state`、`match` の対象）は、状態の単位の `Get` を呼んで読む。

```go
token, err := o.token.Get(ctx)
if err != nil {
	return <零値>, err
}
```

- 読むのは、その状態を使う文の直前。一つの文の中の同じ状態は、一度だけ読む。別の文なら、もう一度読む（文の間に効果が挟まりうる）。`match` の対象が状態なら、`match` の前に一度読み、アームの中の同じ状態の参照は、その値を使う。アームの本体だけが使う状態は、そのアームの枝の中で読む。
- 読んだ値の変数の名前は、状態の名前（先頭を小文字に。当たれば末尾に `_`。「名前」）。同じ状態をもう一度読むときは、同じ変数に `=` で代入する。

### bind で埋まる位置

呼び出しの `Call.bound`（`docs/check.md`「bind」）の位置は、**呼ぶ先が同じ名前空間でも別の名前空間でも、配線が埋める**。状態は、`bind` を書いた名前空間（呼ぶ側）のもので、読めるのは呼ぶ側だけだから。別の名前空間の口（`<M>Ops`）のメソッドは、相手の op の引数をそのまま持つ（`bind` の位置を消した型は作らない）。

1. 呼び出しの前に、`bound` の状態を「状態の参照」と同じに読む。
2. 位置が引数そのもの（パスが `[p]`）なら、読んだ値を、その引数に渡す（型は「値」の「型をそろえる」）。
3. 位置が、レコードリテラルで渡した引数の中のフィールドなら、構造体リテラルのそのフィールドに、読んだ値を書く（入れ子のレコードリテラルも同じ）。
4. 位置が、渡されていない引数の中のフィールドなら、その引数の型の構造体リテラルを作り、そのフィールドに読んだ値を書いて渡す。

形が `nullable` の状態を、省略可能でない位置に埋めることは、ない（BIND-2。`docs/check.md`「bind」）。

名前で渡した引数の中の位置は、明示した位置で、`bound` に入らない（`docs/check.md`「bind」）。

### match

`match` は、分類の種類ごとの**分け方**と、使われ方（`return` / `val` / 文）ごとの**アームの本体**の組み合わせで書く。どの形でも、分ける前に `// match (<対象> by <分類>)` の一行を置く（`<対象>` は仕様に書いたままのパス、`<分類>` は FQN、標準の分類なら `list` / `nullable`。状態の分類での `match` は `// match (by <分類>)`）。

**分け方**:

| 分類 | 条件 |
|---|---|
| 標準の分類 `nullable`、型で決まる分類のうち絞り込みが `presence` のもの | `if <対象> != nil`。この枝が、値のある場合（`Some`、型を書いたケース）。もう一方が `None` / ない側のケース（`Classify.absent`: `対象 as else` か `対象 as none`。`docs/check.md`「ない側のケース」）。ない側のケースを持たない分類では、もう一方は、分類の誤りの枝（下） |
| 標準の分類 `list` | `if len(<対象>) != 0`。この枝が `HasItems`、もう一方が `Empty` |
| 型で決まる分類のうち絞り込みが `variant` のもの | `switch <対象>.(type)`。型を書いたケースごとに `case <アームの Go の型>:`（アームを包む型なら、その包む型）。残りのケースは、残りのアーム（`docs/check.md`「残りのケース」）の Go の型を、union の宣言の順に並べた `case <型>, <型>:`（同じ Go の型は一度だけ）。最後の節は、いつも `default:` で、分類の誤りの枝（下） |
| 述語で決まる値の分類・op の分類（ない側のケースを持つものを含む。「分類」） | `k<n>, err := <パッケージ>.Classify<C>(ctx, o.<分類の引数名>, <対象>)` と失敗の扱いの後、`switch k<n>`。ケースごとに `case <C><ケース名>:` |
| 述語で決まる状態の分類 | `k<n>, err := Classify<C>(ctx, o.<分類の引数名>)` と失敗の扱いの後、`switch k<n>`。上と同じ |
| 型で決まる状態の分類 | 対象の状態を「状態の参照」で読んでから、その値を対象にして、上の `presence` か `variant` の分け方 |

- 対象が状態なら、先に「状態の参照」で読む。対象が名前でない `Atom`（リテラル、レコードリテラル、`none`）なら、`v<n>` に受けてから分ける。
- **`else` のアームは、受けるケース（`MatchArm.cases`）を名前で並べたアームと、同じコードになる**: `if` の枝も、`case <C>A, <C>B:` も、`case <型>, <型>:` も、枝の頭のコメントも。基底は `MatchArm.else` を読まない。Go の `default:` の節は、`else` のアームには使わない: `else` が受けるのは宣言したケースだけで（`docs/check.md`「アームが受けるケース」）、型の `switch` の `default:` は、宣言したケースの外の値が来る節だから。
- **分類の誤りの枝**: 型で決まる分類は判定関数を持たないので、どのケースにも当たらない値（`docs/check.md`「ケースの判定」の「当たらない値」）は、`match` のコードが受ける。枝の中身は、`match` の使われ方（`return` / `val` / 文）によらず、`return <零値>, &runtime.ClassificationError{Classify: "<分類の FQN>"}` の一行。枝の頭のコメントは書かない。置くのは、次の二つの形だけ。標準の分類の `match`（どの値も、どちらかのケースに当たる）と、述語で決まる分類の `match`（判定関数が、先に同じ誤りを返す。「分類」）には、ない:
  - `if` の形で、分類が、ない側のケース（`対象 as else` か `対象 as none`）を持たないとき、値のない側の枝。
  - `switch <対象>.(type)` の、最後の `default:` の節。来るのは、どのケースも `as` に書かなかった型のアームの値と、union の値がない（`nil`）とき。書いた型が union のすべてのアームを覆っていても、`nil` のために、いつも置く。
- **二つに分ける形**（`if`）は、アームを書いた順によらず、値のある場合を `if` の枝にする。`return match` では、`else` を書かず、もう一方をそのまま後ろに続ける（どの枝も `return` で終わる）。それ以外では `if … { … } else { … }`。枝の中身がないとき（文の `match` の `none` のアーム）は、その枝を書かない（`if` の枝がなければ、条件を逆にして `if <対象> == nil` / `if len(<対象>) == 0` にする）。分類の誤りの枝も、もう一方の枝として、同じに置く（`return match` なら後ろに続け、それ以外なら `else` の枝。中身がなくなることはない）。一つのアームが両方のケースを受けていれば（`Empty, HasItems => f();`）、分けずに、そのアームの本体だけを書く。
- **`switch` の形**は、アームごとに `case` を一つ。並びは、アームが受ける最初のケース（`MatchArm.cases` の先頭）の、分類の宣言での順。複数のケースを受けるアームは `case A, B:`。述語で決まる分類の `switch k<n>` は `default:` の節を持たず、どの節も関数を抜けるとき（`return match` と、すべてのアームが `throw` か「返らない」呼び出しの `match`）は、後ろに `return <零値>, &runtime.ClassificationError{Classify: "<分類の FQN>"}` を置く（判定関数が先に同じ誤りを返すので、ここには来ない。Go が、関数の終わりの `return` を求めるため）。型の `switch` は、`default:` の節（分類の誤りの枝）が関数を抜けるので、後ろには何も置かない。
- それぞれの枝（`if` の枝、後ろに続く部分、`else` の枝）の頭に、ケースの名前をコメントで書く: `// <ケース名>`（複数なら `, ` でつなぐ。残りのケースは、ほかのケースと同じく、その名前。`else` のアームは、受けるケースの名前）。対象が絞り込まれるアームは `// <ケース名>: <対象> は <型>`（型の書き方は「コメント」の「型の表示」）。絞り込まれるアームとは、`narrowed` の型の表示が、外での型の表示と違うアーム: ケースで絞っただけで、もとの型が同じアーム（`case` の `of` が、外と同じ）は、Go の型が変わらないので、数えない。`switch` の節には書かない（`case` が名前を持つ）。

**絞り込んだ値**（アームの中で、対象と同じ `Atom` を値として使う位置の式。フィールドを読むだけなら、Go がポインタを辿るので、元の式のまま）:

| 絞り込みの種類（`CaseTyping.by`、標準の分類は `presence`） | 式 |
|---|---|
| `presence` | `*<対象>`（対象の Go の型がポインタのとき。`array`、インデックスの model、union はそのまま） |
| `variant` | `<対象>.(<アームの Go の型>)`。アームを包む型なら `<対象>.(<包む型>).Value` |
| `predicate` | 対象の型から絞り込んだ型へ: 省略可能を外すなら `*<対象>`、Go の型が違えば `<絞り込んだ型>(<…>)`（`string` を `Email` に: `Email(<対象>)`） |
| `else`（残りのケース） | 残りのアームがちょうど一つで、対象がそのアームの型になるとき（`docs/types.md`「残りのケースでの対象の型」）だけ、`variant` と同じ式。それ以外は、対象の型が変わらないので、何も置き換えない |
| `none`（`as none` のケース） | 対象の型が変わらないので、何も置き換えない |

- アームの中の対象の型（`docs/types.md`「アームの中の対象の型」）が、外と、Go で同じ型のアームは、何も置き換えない: 標準の分類と状態の分類で、型が変わらないアーム。値の分類と op の分類で、ケースで絞り込んだだけで、もとの型が変わらないアーム（`case(C, Ks, B)` の Go の型は、B の Go の型）。
- `variant` の絞り込みで、`as` の型と等しい型のアームを union が二つ以上持つなら GO-6（どのアームかを、型で分けられない）。

**アームの本体**:

| 本体 | `return match` | `val x = match` | 文の `match` |
|---|---|---|---|
| 呼び出し | `return <呼び出し>`。型をそろえる必要があれば、`v<n>` に受けてから `return <そろえた値>, nil` | `v<n>, err := <呼び出し>` と失敗の扱い、`x = <そろえた値>` | `if err := <呼び出し>; err != nil { return <零値>, err }` |
| 「返らない」呼び出し | `return <零値>, <呼び出し>` | 同じ | 同じ |
| 値 | `return <値>, nil` | `x = <値>` | —（書けない。WIRE-2） |
| `none` | `return nil, nil` | 何も書かない（`x` は零値の `nil` のまま） | 何も書かない |
| `throw E` | `return <零値>, <E の型>{}` | 同じ | 同じ |
| `throw E(<値>, …)` | `return <零値>, <E の型>{<F>: <値>, …}`。`<F>` は例外の引数のフィールド名（「exception」）で、並びは `Exception.params` の順。値は、引数の型を当てはめる型にして「値」の規則で書く | 同じ | 同じ |

- `val x = match` は、分ける前に `var x <match の型>`（`docs/types.md`「match の型」。型を書いた `val` なら、その型）を置く。アームの値は、その型にそろえる。`val` が、どこからも読まれていなければ、`match` の後ろに `_ = x`。
- `x = match` は、`val x = match` と同じで、`var x` の行を置かない（空の宣言の所に、もう、ある）。`xs collect match` は、`v<n>` に、`val v<n> = match` と同じに受けてから、`xs = append(xs, v<n>)`。
- アームの呼び出しの `err` は、その枝の中の変数（`:=`）で、失敗の扱いも枝の中に置く。
- 例外は値で作る（「exception」）。`throw` の引数（`MatchArm.body` の `args`）は、数も型も、検査が例外の宣言と照らしてある（`docs/check.md`「throw」）ので、フィールドは、いつもすべて書く。

### 分類

- **対象のない分類**（`Classify.decidedBy` が `none`。`classify C of never`。`docs/check.md`「分類」の「対象のない分類」）について、基底は何も出さない: 列挙の型も、定数も、ない。ケースに付いたデコレータ（`Classify.cases[].decorators`）も、読まない。何を出すかは、その分類を読むプラグインが決める。
- **型で決まる分類**（`Classify.decidedBy` が `type`。標準の分類を含む）について、基底は何も出さない: 列挙の型も、判定関数も、`New` の引数もない。どのケースに当たるかは、値から決まる（「match」の `presence` と `variant`）。分類の名前とケースの名前は、それを使う配線のコメントにだけ残る。残りのケース（`対象 as else`）や、ない側のケース（`対象 as none`）を持つ分類も同じで、そのケースのための定数も出さない。どのケースにも当たらない値は、`match` のコードが分類の誤りにする（「match」の「分類の誤りの枝」）。
- **述語で決まる値の分類・op の分類** C は、宣言した名前空間の定義のパッケージの `classify.go` に、次を出す。述語は、人が実装する（排他になるように、一人がまとめて書く）。

```go
// WarrantyKind は保証の種類
type WarrantyKind int

const (
	// 標準の保証
	WarrantyKindStandard WarrantyKind = iota
	WarrantyKindYamada
)

// WarrantyKindCases は分類 shop.WarrantyKind のケースごとの述語。人が実装し、この分類を match に使う名前空間の New に渡す。
type WarrantyKindCases interface {
	// Standard はケース Standard の述語。
	Standard(ctx context.Context, v Item) (bool, error)
	// Yamada はケース Yamada の述語。
	Yamada(ctx context.Context, v Item) (bool, error)
}

// ClassifyWarrantyKind は v を shop.WarrantyKind で分類する。すべての述語を呼び、真になった数を数える。
func ClassifyWarrantyKind(ctx context.Context, c WarrantyKindCases, v Item) (WarrantyKind, error)
```

- **述語は、ない側のケースを除く、ケースの数だけある。** 人が実装するのは、ない側のケース（`Classify.absent`。`docs/check.md`「ない側のケース」）を除く、宣言したケースのすべての述語で、基底は、「ほかのどれでもない」ケースを導かない（`docs/check.md`「ケースの判定」）。二つに分かれる分類（`classify Role of Member { Admin; Regular; }`）は、二つの述語を持ち、一方をもう一方の否定として書くのは、人。
- 述語は、`ctx` と、分類の対象の値（値の分類は `subject` の型、op の分類はその op の `returns`）を取り、`(bool, error)` を返す。判定関数は、`ctx`、述語、対象の値を、この順に取る。どちらも、op と同じ形（「op の形」）。
- 判定関数は、すべての述語を宣言順に呼び、真の数を数える。述語が `error` を返したら、残りも呼んだうえで、最初の `error` を返す。どの述語も `error` を返さなければ、真の数を確かめる: 真がちょうど一つなら、そのケース。0 か、二つ以上なら、`*runtime.ClassificationError{Classify: "<FQN>", Hits: <真になったケース名>}` を返す。
- 型で決まる絞り込み（`presence` / `variant`）の `as` を持つケースは、値がその型に当たらなければ、述語を呼ばずに偽とする（`docs/check.md`「ケースの判定」）。
- **ない側のケース**（`Classify.absent` のケース。`Absent: Order? as none;` か、残りが「値がない」の `as else`）は、列挙の定数を持つが、述語を持たない: `<C>Cases` に、そのメソッドはない。判定関数は、そのケースの番で、述語を呼ぶ代わりに、値がなければ（`v == nil`）真とする。ほかのケースは、どれも `T? as T`（`presence`）なので、値がなければ述語を呼ばない（上の行）。値がなければ、真はない側のケースだけで、述語は一つも呼ばれない。値があれば、ない側のケースは偽で、ほかのケースの述語で決まる。

```go
// LifeCases は分類 shop.Life のケースごとの述語。人が実装し、この分類を match に使う名前空間の New に渡す。
type LifeCases interface {
	// Draft はケース Draft の述語。
	Draft(ctx context.Context, v *Order) (bool, error)
	// Placed はケース Placed の述語。
	Placed(ctx context.Context, v *Order) (bool, error)
}
```
- **判定の誤りは、配線が返す。** 判定関数が返した `error`（述語の `error` か、`*runtime.ClassificationError`）は、ほかの呼び出しの失敗と同じに、配線がそのまま返す（「呼び出し」の「失敗の扱い」）。どの配線も `error` を返す（「op の形」）ので、返す先は、いつもある。基底は、`panic` を呼ぶコードを出さない。どの述語も真でないのは、値が、宣言したケースの外にあるということ（`docs/check.md`「閉じた分類」）。二つ以上が真なのは、述語の実装の誤り（排他であることは、述語を書く人が担保する）。どちらも、仕様の例外（`throws`）ではない `error` として、呼び出し元に届く。
- 述語の実装は、その分類を `match` に使う名前空間と、効果の入口の確かめ（「効果の入口の確かめ」）に使う名前空間の `New` が、引数で受け取る（「New」の 3）。分類を宣言した名前空間の `New` が受け取るのは、その名前空間が、どちらかに使うときだけ。
- **述語で決まる状態の分類** C は、`classify.go` に列挙の型と定数と、判定関数 `func Classify<C>(ctx context.Context, s <C>State) (<C>, error)` を出す。述語は状態の単位 `<C>State` のメソッド（「状態の単位」）。判定関数は、値の分類と同じ規則で、すべての述語を呼び、真の数を確かめる。配線が `match by C` に使っていなくても出す（テストが、効果の結果の判定に使う）。

## デコレータ

**デコレータの適用が何を出力するかは、そのデコレータを読むトランスパイラが決める。** 基底は語彙を知らないので、デコレータの名前からも、宣言の節（`throws`、`catches … recovers` / `halts` / `ignores`）からも、何も生成しない。デコレータが基底の出力に効くのは、検査が計算した `Throws:` などのコメントと、単位のトランスパイラが宣言した継ぎ目（「継ぎ目」）の二つだけ。op のメソッドの形は、デコレータの有無で変わらない（「op の形」）。

**付ける先で、デコレータのできることが分かれる**（原則の持ち主は `docs/requirements.md`「出力を推論しない」）。

| 付ける先 | デコレータのすること | 基底が出すもの |
|---|---|---|
| op | op の中身の前後に処理を足す（包む）。**op の中身は決めない** | 関数の枠（op の形）と、継ぎ目の口。中身は、配線の本体か、人が書く葉 |
| op 以外（`read`、効果、例外、状態、model のフィールド） | その宣言から作るものを、読むトランスパイラが、宣言の全体（型と引数を含む）から決める材料になる | 口だけ（状態の単位のメソッド、型）。中身は、読むトランスパイラが出すか、人が書く |

読むトランスパイラが、デコレータを形にする仕方は四つ。上の三つは、相乗りが自分のファイルの中だけで行い、基底は関わらない。

| 形 | すること | 例 |
|---|---|---|
| 入口を作る | 外からの呼び出しを、入口の `Ops` のメソッドに届ける | `go-http` のハンドラ（`@http.get`）、`go-cli` の `Main`（`@cli.command`） |
| 実装を出す | 口（状態の単位）を満たす型を出す | `go-sqlite` の `<状態>Table`（`@db.table`）と、そのメソッド（`read` の `@db.find` / `@db.list`、効果の `@db.insert` / `@db.update` / `@db.delete`） |
| 材料にする | 自分が出すコードの中身を決めるために読む | `@http.header`、`@http.status`、`@db.text`、`@db.key`、`@db.where`、`@valid.*` |
| 継ぎ目を要る | op の中身の前後に処理を置く。口は基底が出し、中身はそのトランスパイラが出す（「継ぎ目」） | `go-sqlite` の `@db.transaction`、サンプルの `go-ratelimit`（`@rate.limited`） |

- **読む者がなければ、何も出ない**（規則の持ち主は `docs/config.md`「継ぎ目」）。単位のどのトランスパイラも読まないデコレータの適用には、基底は何も出さない: 継ぎ目も、呼び出しも、warning も。何も生成しないデコレータは、この言語のために用意されたものではないデコレータ（要求やドメインの印、ほかの道具や別の言語の出力のための注記）と区別がつかないので、同じに扱う。
- プロジェクトが自分で宣言したデコレータも同じで、読むトランスパイラがなければ印。処理をさせたいなら、配線の中の op の呼び出しとして書くか、そのデコレータを読むプラグインをプロジェクトの中に置く（`docs/config.md`「継ぎ目」）。
- デコレータが宣言した `throws` / `catches` と、`reads` / `effects`（`docs/check.md`「デコレータが運ぶ reads と effects」）は、読む者がいてもいなくても、検査の計算（`docs/check.md`「計算」）に効き、`Signature` と、コメントの `Throws:` / `Reads:` / `Effects:` に現れる。op の置き場は変えない（「op の置き場」）。それを実際に起こすのは、読むトランスパイラが出したコード（継ぎ目の中身）で、基底ではない。
- **帰結**: `catches` を持つデコレータが付いた op で、それを読むトランスパイラが単位にいなければ、生成した本体は例外を吸収しないので、その op のメソッドは、コメントの `Throws:` にない例外を返しうる。基底は、これを warning にもしない（節を見ない）。

これまでの二つの形は、やめた: 単位のトランスパイラが `wraps` で引き受けたデコレータの適用に、包む関数の型と、それを束ねる構造体を生成する形（引き受ける者を集める仕組み `select` がなくなった）と、基底は何も生成せず、相乗りが入口の `Ops` を外から包む形（同じ名前空間の配線からの呼び出しに掛からない）。

## 継ぎ目

継ぎ目とは何か、どのデコレータが継ぎ目を持つか（単位の継ぎ目）、宣言のしかたの持ち主は `docs/config.md`「継ぎ目」。この節が決めるのは、Go での形。期待する出力の実物は、サンプルの `gen/server/inner/ops.go`、`gen/server/inner/wiring.go`（基底）と、`gen/server/ratelimit.go`（継ぎ目を宣言したトランスパイラ）。

**継ぎ目を持つ op** は、N の単位の対象の op のうち、単位の継ぎ目（`View.config.seams` のキー）にあるデコレータが付いたもの。付いているかは `Op.decorators` の `Application.decorator` で見る（名前付きデコレータは展開済み）。葉か配線か、`export` の有無にはよらない。単位の対象でない op（本体のない op として出すもの。「出力の対象」）と、op 以外の宣言への適用は、継ぎ目を持たない。

継ぎ目を持つ op は、**二層**で出す: op の**中身**と、継ぎ目に制御を渡す**外側**。継ぎ目は、インターフェース `Seams` のメソッドで、実装は `New` の位置引数で受け取る（「実装を受け取る口」と同じ作法）。

```go
// ops.go

// Seams は、継ぎ目: 単位のトランスパイラが継ぎ目を宣言したデコレータの、op への適用ごとに、メソッド一つ。実装は New の引数 seams に渡す。
// メソッドが一つでも足りない値は、渡すところでコンパイルできない。
type Seams interface {
	// LoginRateLimited は op server.login に付いた @rate.limited の継ぎ目（go-ratelimit）。body が op の中身で、その前後に置くものを、実装が決める。
	LoginRateLimited(ctx context.Context, in LoginInput, body func(ctx context.Context, in LoginInput) (LoginOk, error)) (LoginOk, error)
}

// Login は、継ぎ目（@rate.limited）を通して、中身（o.loginBody）を呼ぶ。
func (o *ops) Login(ctx context.Context, in LoginInput) (LoginOk, error) {
	return o.seams.LoginRateLimited(ctx, in, o.loginBody)
}
```

```go
// wiring.go

// loginBody は op server.login の中身（配線の本体）。外側の Login が、継ぎ目を通して呼ぶ。
func (o *ops) loginBody(ctx context.Context, in LoginInput) (LoginOk, error) {
	user, err := o.store.FindUser(ctx, in.Body.Email)
	…
}
```

### 中身

中身は、「op の置き場」が決めた実装そのもの。置き場は、継ぎ目の有無で変わらない。

| op の置き場 | 中身 | 外側が渡す式 |
|---|---|---|
| 1（配線） | `wiring.go` の、`ops` のメソッド `<op>Body`。本体は「配線の本体」の規則のまま | `o.<op>Body` |
| 2（`never` を返す op） | `ops.go` の、外側のすぐ後ろの、`ops` のメソッド `<op>Body`: `return <例外の型>{}` | `o.<op>Body` |
| 3（葉） | `Leaves` のメソッド。**名前は op 名のまま** | `o.leaves.<Op>`（メソッドの値） |

- **中身の名前**（`<op>Body`: op 名の先頭を小文字にして `Body` を付けた、非公開の名前）を持つのは、基底が中身を出す op（配線と、`never` を返す op）だけ。形（引数と戻り値）は op の形のまま。どのインターフェースにも現れない。当たる名前は「名前」の GO-2。
- **葉の実装のメソッドに、中身の名前を付けない**のは、口（`Leaves`）の形を、継ぎ目の有無で変えないため。継ぎ目は、単位にどのトランスパイラを並べたかで決まり、同じ名前空間でも単位ごとに違いうる。口の名前がそれで変われば、人の実装が、関係のないトランスパイラの宣言で壊れる。口のメソッドを直に呼べば継ぎ目を通らないが、それは実装を呼ぶことで、op を呼ぶことではない。op を呼ぶのは、`Ops` のメソッド（外側）。
- 継ぎ目が掛かるのは op だけで、`read` と効果には掛からない（「継ぎ目を持つ op」は op）。配線の中身が呼ぶ `read` と効果は、状態の単位のメソッドを、そのまま呼ぶ。
- 継ぎ目を持つ配線の中から、別の op を呼ぶ書き方は変わらない（「呼び出し」）。継ぎ目を持つ op を、同じ名前空間の配線から呼ぶと（`o.<Op>(…)`）、外側を通る。

### Seams

- 定義のパッケージの `ops.go` に、インターフェース `Seams` を一つ出す。継ぎ目を持つ op が一つもなければ出さず、`New` の引数 `seams` もない（単位の継ぎ目にあるデコレータが、単位の対象のどの op にも付いていないときを含む）。
- メソッドは、継ぎ目を持つ op と、それに付いた、単位の継ぎ目にあるデコレータの適用ごとに一つ。並びは op の宣言順、その中は `Op.decorators` の順（書かれた順）。`@repeatable` でないデコレータは一つの op に一度しか付かない（DEC-13）ので、組は重ならない。`@repeatable` のデコレータを継ぎ目にして、一つの op に二度付けていれば、メソッドの名前が当たる（GO-2）。メソッドの間は一行空ける。
- メソッドの形:

| 部分 | 規則 |
|---|---|
| 名前 | `<Op><Decorator>`（「名前」: `LoginRateLimited`） |
| 引数 | op の形の引数（`ctx context.Context` と、`Op.params`）の後ろに、`body func(<op の形の引数>) <op の形の戻り値>` |
| 戻り値 | op の形の戻り値 |

- 継ぎ目のメソッドも `body` も、op と同じに、いつも `ctx` を取り、`error` を返す（「op の形」）。
- `body` の関数の型には、引数の名前も書く（op の引数の局所の名前）。`body` という名前が op の引数の局所の名前と当たれば、当たらなくなるまで、`body` の方の末尾に `_` を足す（「名前」の、生成したコードが作る名前の規則）。
- 基底は、デコレータの引数（`Application.args`）を継ぎ目に渡さない。中身を出すトランスパイラが `View` から読んで、自分のコードに書く。

### 外側

継ぎ目を持つ op の、`ops` の公開のメソッド `<Op>`（`Ops` を満たすメソッド）。`ops.go` の、配線でないメソッドの並びの、その op の位置に置く。形は op の形のまま。

- 継ぎ目が一つなら `return o.seams.<Op><Decorator>(ctx, <引数>, <中身の式>)`。
- **継ぎ目が二つ以上なら、入れ子にする。順序は、言語のデコレータの適用順（DEC-9: op に近いものから）**: `Op.decorators` の最後（op に最も近い）のものが最も内側で、中身を直に受け取る。先に書かれたものほど外側。内側は、同じ形の関数リテラルにして、外側の継ぎ目の `body` に渡す。継ぎ目を持たないデコレータは、間にあっても層にならない。

```go
// @trace.span
// @rate.limited
// export fn foo(in: FooInput): FooOk { … }

// Foo は、継ぎ目（@trace.span、@rate.limited）を通して、中身（o.fooBody）を呼ぶ。
func (o *ops) Foo(ctx context.Context, in FooInput) (FooOk, error) {
	return o.seams.FooTraceSpan(ctx, in, func(ctx context.Context, in FooInput) (FooOk, error) {
		return o.seams.FooRateLimited(ctx, in, o.fooBody)
	})
}
```

### 内側のフォルダと、別の名前空間

- **分けた出力**（「内側のフォルダと入口」）では、二層と `Seams` は内側のパッケージにある。入口は `type Seams = inner.Seams` と、同じ引数の `New` を出す。内側の `Ops` のメソッドも外側なので、テストが内側の `New` の戻り値から内部の op を呼んでも、継ぎ目を通る。中身だけを試すテストは、`body` をそのまま呼ぶ `Seams` を自分で書いて、`inner.New` に渡す。
- **継ぎ目が掛かるのは、op の持ち主の名前空間の出力。** 別の名前空間 M の op を `<M>Ops` で呼ぶ側（N）は、M の op の継ぎ目について何も出さない（N の `Seams` に、M の op のメソッドはない。N の単位の継ぎ目に、そのデコレータがあっても）。M の出力の `Ops` のメソッドは外側なので、それを N の `New` に渡せば、N からの呼び出しは M の継ぎ目を通る。
- M が**単位を持たない名前空間**なら、基底の出力がないので、継ぎ目もない。`<M>Ops` を満たす型を書く人が、そのデコレータの意味をどう実装するかを決める。
- 同じ名前空間を二つの単位が出力していて、継ぎ目を宣言したトランスパイラが一方にしか並んでいなければ、継ぎ目を持つのは、その出力だけ（単位は互いに独立。`docs/config.md`「単位の対象」）。

### 継ぎ目の実装

- `Seams` のメソッドの実装は、**その継ぎ目を宣言したトランスパイラが出す**: 自分が宣言したデコレータの分のメソッドを持つ具体的な型を、自分のファイル（「相乗りのファイル」。入口のパッケージ）に、op ごとに、型の付いたコードで出す。型の名前、作り方、実行のときに要るもの（数え方、保存先）をどう受け取るかは、そのトランスパイラが決め、自分の文書に書く。メソッドの名前と形は、`plugin/golang` で求める（`docs/plugin-protocol.md`「相乗りが基底の形を知る方法」）。
- 単位で継ぎ目を宣言したトランスパイラが一つなら、その型が `Seams` を満たし、そのまま `New` に渡せる。二つ以上なら、`Seams` のメソッドがそれぞれの型に分かれるので、人が、それらを埋め込んだ構造体を一つ書いて渡す（`struct{ server.RateSeams; server.TraceSeams }`）。どれかが足りなければ、渡すところでコンパイルできない。
- 宣言したトランスパイラがメソッドを出さなくても、hozoc は確かめない（起動は互いに独立）。`New` に渡すところでコンパイルできなくなる。
- 人が書いた型を渡してもよい（テストで継ぎ目を素通しにする、一つのメソッドだけを埋め込みで差し替える）。
- 継ぎ目の中身は、原則として、op の入力と出力の値を変えない（`docs/requirements.md`「書き方の指針」）。基底は確かめない: `body` に何を渡し、何を返すかは、継ぎ目の実装が決める。

### 継ぎ目で変わるもの、変わらないもの

- 変わらない: op のメソッドの形（「op の形」。`ctx`、引数、戻り値、`error`）、`Ops`、`Leaves`、状態の単位（`read` と効果のメソッドを含む）、`<Ns>Ops`、コメントの `Throws:` など（どれも仕様だけで決まる）。継ぎ目を宣言するトランスパイラを単位に足しても、`Ops` を使うコードと、口を実装するコードは、そのまま通る。
- 変わる: `Seams` と `New` の最後の引数、継ぎ目を持つ op の `ops.go` と `wiring.go` の中（外側と、中身の名前）。`New` を呼ぶ行は、引数が一つ増える。

## 本体のあるデコレータ

`docs/decisions.md`「決めたこと」392〜395。本体のあるデコレータ（`docs/grammar.md`「デコレータの本体」）を付けた op は、継ぎ目を持つ op（「継ぎ目」）と同じに、**二層**で出す。違いは、外側を、基底が、デコレータの本体から作ること: 人も、プラグインも、実装しない。`Seams` には、載らない。

仕様:

```
decorator fn audited() {
  proceed();
  recordAudit();
}

@audited
export fn getMe(in: AuthInput): type.PublicUser { … }
```

出力:

```go
// ops.go

// GetMe は、デコレータ（@server.audited）を通して、中身（o.getMeBody）を呼ぶ。
func (o *ops) GetMe(ctx context.Context, in AuthInput) (pkgtype.PublicUser, error) {
	return o.getMeServerAudited(ctx, in)
}
```

```go
// wiring.go

// getMeServerAudited は op server.getMe に付いた @server.audited の外側（デコレータの本体）。proceed() が、内側（o.getMeBody）を呼ぶ。
func (o *ops) getMeServerAudited(ctx context.Context, in AuthInput) (pkgtype.PublicUser, error) {
	result, err := o.getMeBody(ctx, in)
	if err != nil {
		return pkgtype.PublicUser{}, err
	}
	if err := o.RecordAudit(ctx); err != nil {
		return pkgtype.PublicUser{}, err
	}
	return result, nil
}
```

- **外側のメソッド**は、適用ごとに一つ。名前は、継ぎ目のメソッドの名前（「Seams」）の、先頭を小文字にしたもの（op `getMe` と `server.audited` → `getMeServerAudited`）。非公開で、形（引数と戻り値）は、op の形のまま。置き場は `wiring.go`: 配線の op なら、その op の中身のメソッドの、すぐ後ろ。そのほかの op（葉、`never` を返す op）なら、op の宣言順の、その位置。当たる名前は「名前」の GO-2。
- **本体は、「配線の本体」の規則で書く。** 本体の中の名前は、op の配線が書いたのと、同じ式になる: 同じ名前空間の op は `o.<Op>(…)`、別の名前空間の op は `o.<名前空間の引数名>.<Op>(…)`（「別の名前空間の op」の口に、その op が並ぶ）、`read` と効果は、状態の単位のメソッド。
- **`proceed()` は、内側を、op の引数のまま呼ぶ式**になる: 内側が、op の中身なら `o.<op>Body(ctx, …)`（葉なら `o.leaves.<Op>(ctx, …)`）、本体のある別のデコレータなら、その外側のメソッド、継ぎ目なら `o.seams.<継ぎ目>(ctx, …, <その内側>)`。
- **op が値を返すとき**、`proceed()` の結果を、そのまま返す: `proceed()` が、本体の最後の文（か、最後の文の `match` のアーム）なら `return <内側の呼び出し>`。後ろに文が続くなら、結果を `result` に受けて、最後に `return result, nil`。値を返さない op では、`proceed()` は、ふつうの、値のない呼び出しと同じ。
- **`proceed()` のアームを持つ `match` の後ろに、文が続くとき**も、同じ: 値を返す op では、`var result <型>` を置き、`proceed()` のアームが `result` に入れ（内側が失敗すれば、その場で、失敗を返す）、`throw` のアームは、その場で、失敗を返し、最後に `return result, nil`。
- **`never` を返す op** では、`proceed()` は、「返らない」呼び出しと同じ（「文」）: `return <内側の呼び出し>`（op の形は、`error` だけを返す）。後ろに文を持つデコレータは、付けられない（`docs/check.md`「デコレータ」の DEC-25）。
- **デコレータの、値の引数**は、適用に書いたリテラルを、「値」の規則で、その場に書く。渡されていない省略可能な引数は `nil`。
- **中身の名前**（`<op>Body`）は、継ぎ目を持つ op と同じに、基底が中身を出す op（配線と、`never` を返す op）が、包むデコレータを一つでも持てば、付く（「継ぎ目」の「中身」）。
- **コメントの言い方**: 包むものに、本体のあるデコレータが一つでもあれば、外側と中身のコメントは、「継ぎ目」の代わりに「デコレータ」と言う（`Logout は、デコレータ（@server.audited）を通して、中身（o.logoutBody）を呼ぶ。`、`logoutBody は op server.logout の中身（配線の本体）。外側の Logout が、デコレータを通して呼ぶ。`）。
- **継ぎ目と重ねたとき**は、書かれた順に、外から内へ、入れ子にする（「外側」）。op の公開のメソッドは、いちばん外の層を呼ぶ: 継ぎ目なら `o.seams.<継ぎ目>(ctx, …, <内側>)`、本体のあるデコレータなら、その外側のメソッド。
- **単位のトランスパイラが、本体のあるデコレータに、継ぎ目を宣言していれば GO-7。** 包み方が、仕様にあるのに、プラグインも包もうとしている。基底は、本体から外側を作り、継ぎ目の口は、出さない。
- **テストのにせもの**（「テストのにせもの」）と、インテグレーションテスト（`docs/ide-chain.md`「インテグレーションテストの作り直し」の「継ぎ目に置くもの」）では、本体のあるデコレータは、本物を通す: 外側は、op の一部で、にせものにする口が、ない。

## 単体テストの口

人か AI が中身を書く関数に、基底は、結果ごとの単体テストを受け取る口と、それを走らせる関数を出す（`docs/decisions.md`「決めたこと」373〜375）。対象は、葉の op（`Leaves` のメソッド）と、述語で決まる値の分類・op の分類の判定（`<C>Cases` の述語）。Hozo が担保することと、その確かめ方（ケースの漏れは、コンパイルエラー。走ったかは、「テストの対応表」と、カバレッジの突き合わせ。通ったかは、テストの実行の結果）の持ち主は、`docs/ide-chain.md`「テストの規約」の「担保すること」。中身が作られるもの（配線の op）と、型で決まる分類には、出さない。

人が書くのは、口を満たす型と、それを走らせる関数に渡す、一行のテスト:

```go
func TestLeaves(t *testing.T) { servertest.RunLeaves(t, app.Leaves{}, leafCases{}) }
```

### テストのパッケージ

- 口は、**テストのパッケージ**に置く: ディレクトリは `<Dir(U, N)>/<N のパッケージ名>test/`、パッケージ名は `<N のパッケージ名>test`（`gen/server/servertest`、パッケージ `servertest`）。`testing` を import するので、アプリのコード（入口と、定義のパッケージ）とは、分ける。人のテスト（`out` の外に置く）が、import する。
- テストのパッケージは、N の**入口のパッケージ**を import する（N の型、`Leaves`、`<C>Cases`、`Classify<C>` は、どれも、入口にある）。内側のパッケージは、import しない。
- ファイルは、`doc.go`、`leaves.go`（葉の op が、一つでもあるとき）、`classify.go`（述語で決まる値の分類か op の分類が、N に、一つでもあるとき）、`seams.go`（「デコレータのテストの口」の対象が、一つでもあるとき）、`fakes.go`（「テストのにせもの」の「出すとき」）。四つとも、なければ、パッケージを作らない。`doc.go` のコメントは、`// Package <名前> は、名前空間 <N> の、テストのためのもの: 単体テストの口と、にせもの（docs/plugin-go.md「単体テストの口」「テストのにせもの」）。`
- import の名前の決まり（「パッケージと import パス」の「import の名前」）は、同じ。違うのは二つ。（1）N の入口のパッケージは、名前が、import パスの最後の要素と同じでも、**いつも、名前を書いて** import する（`server "example.com/shop-server/gen/server"`）: 出力先のディレクトリの名前で、ファイルの中身が変わらないようにする（「決定性」の、同じ名前空間の二つの単位）。（2）import の名前に使わないのは、テストのパッケージの関数の本体が、決まった名前で使う識別子: `t`、`c`、`x`、`k`、`cases`、`leaves`、`got`、`err`、`want`、`kind`、`none`（と、どのファイルでも使わない `ops`、`ptr`）。`fakes.go` では、`f`、`o`、`ctx`、`k`（「テストのにせもの」）。
- テストのパッケージの、公開の名前どうしが当たれば、GO-2（op `leaf` の `LeafCases` と、まとめの `LeafCases` など）。同じ単位が、子の名前空間 `N.<N のパッケージ名>test` を出力するなら、GO-4。基底の公開の名前の一覧は、`golang.PublicTestNames`（`docs/packages.md`「`plugin/golang`（公開）」）。相乗りが、このパッケージに足すファイルの決まりは、「相乗りのファイル」。
- 失敗の文面（`t.Fatalf`）は、`runtime` の誤りと同じに、英語で書く。

### 葉の op

葉（`Shape.Leaves`。「Leaves」）ごとに、結果ごとのケースを受け取るインターフェース `<Op>Cases` と、ケース一つの構造体 `<Op>Case` を出す。その後ろに、葉をまとめる `LeafCases` と、走らせる `RunLeaves` を出す。

```ts
/** パスワードが合わなければ InvalidCredentials（人が書く） */
fn checkPassword(user: type.User, password: type.Password): throws type.InvalidCredentials;
/** トークンと期限を決める（人が書く） */
fn newSession(user: type.User): type.Session;
```

```go
// CheckPasswordCases は、葉 server.checkPassword の、結果ごとのケース。結果の数だけ、メソッドがある。
type CheckPasswordCases interface {
	// OK は、正常に終わるケース。
	OK() CheckPasswordCase
	// InvalidCredentials は、例外 type.InvalidCredentials を投げるケース。
	InvalidCredentials() CheckPasswordCase
}

// CheckPasswordCase は、葉 server.checkPassword の、ケース一つ: 渡す引数と、足りない確かめ。
type CheckPasswordCase struct {
	User     pkgtype.User
	Password pkgtype.Password
	// Then は、結果が、そのケースのものだと確かめた後に、呼ばれる（nil なら、呼ばない）。err は、葉が返した失敗（正常のケースでは nil）。
	Then func(t *testing.T, err error)
}

// NewSessionCases は、葉 server.newSession の、結果ごとのケース。結果の数だけ、メソッドがある。
type NewSessionCases interface {
	// OK は、正常に終わるケース。
	OK() NewSessionCase
}

// NewSessionCase は、葉 server.newSession の、ケース一つ: 渡す引数と、足りない確かめ。
type NewSessionCase struct {
	User pkgtype.User
	// Then は、結果が、そのケースのものだと確かめた後に、呼ばれる（nil なら、呼ばない）。got は、葉が返した値、err は、葉が返した失敗（正常のケースでは nil）。
	Then func(t *testing.T, got pkgtype.Session, err error)
}

// LeafCases は、人が実装する葉の op の、単体テストのケース。葉ごとに、メソッド一つ（Leaves と同じ並び）。
// メソッドが一つでも足りない値は、RunLeaves に渡すところでコンパイルできない。
type LeafCases interface {
	CheckPassword() CheckPasswordCases
	NewSession() NewSessionCases
}

// RunLeaves は、葉ごと、結果ごとに、テストを一つ走らせる。テストの名前は、<op の FQN>/<結果>。
// ケースごとに、leaves の葉を呼び、結果が、そのケースのものであることを確かめてから、Then を呼ぶ。
func RunLeaves(t *testing.T, leaves server.Leaves, c LeafCases) {
	t.Run("server.checkPassword", func(t *testing.T) {
		cases := c.CheckPassword()
		t.Run("OK", func(t *testing.T) {
			k := cases.OK()
			err := leaves.CheckPassword(context.Background(), k.User, k.Password)
			if err != nil {
				t.Fatalf("want no error, got %v", err)
			}
			if k.Then != nil {
				k.Then(t, err)
			}
		})
		t.Run("InvalidCredentials", func(t *testing.T) {
			k := cases.InvalidCredentials()
			err := leaves.CheckPassword(context.Background(), k.User, k.Password)
			var want pkgtype.InvalidCredentials
			if !errors.As(err, &want) {
				t.Fatalf("want exception type.InvalidCredentials, got %v", err)
			}
			if k.Then != nil {
				k.Then(t, err)
			}
		})
	})
	t.Run("server.newSession", func(t *testing.T) {
		cases := c.NewSession()
		t.Run("OK", func(t *testing.T) {
			k := cases.OK()
			got, err := leaves.NewSession(context.Background(), k.User)
			if err != nil {
				t.Fatalf("want no error, got %v", err)
			}
			if k.Then != nil {
				k.Then(t, got, err)
			}
		})
	})
}
```

- **必要なケース**は、葉の結果ごとに、一つ: 正常（`OK`）と、その op が `throws` に書いた例外ごと（`Op.throws` の、書かれた順。同じ例外は、一度だけ）。デコレータが運ぶ例外（`Op.carried`）は、数えない（投げるのは、デコレータの処理で、葉の中身ではない）。例外を一つ足せば、`<Op>Cases` のメソッドが一つ増え、口を満たしていた型は、`RunLeaves` に渡すところでコンパイルできなくなる。
- **結果の名前**: 正常は `OK`。例外は、宣言の名前（`type.InvalidCredentials` → `InvalidCredentials`）。その op の中で、名前が当たるとき（別の名前空間の、同じ名前の例外。例外 `OK`）は、当たったものを、名前空間のセグメントを先頭大文字にして前に付けた名前にする（`rate.Busy` → `RateBusy`）。メソッドの名前と、テストの名前の `<結果>` は、同じ。
- **テストの名前**（ラベル）は、`<op の FQN>/<結果>`（`server.checkPassword/InvalidCredentials`）。`t.Run` を、op と結果の、二段に重ねる。
- **`<Op>Case` のフィールド**は、op の引数（`Op.params` の順）。名前は、フィールド名と同じ決まり（先頭を大文字に。「名前」）。`Then` と当たる引数（`then`）は、`Then_`。型は、引数の Go の型。その後ろに `Then`。
- **`Then`** は、戻り値を持つ op なら `func(t *testing.T, got <戻り値の型>, err error)`、持たない op なら `func(t *testing.T, err error)`。走らせる側が、結果の見分けを済ませた後に、呼ぶ。例外のケースでは、`err` から、例外が運ぶ値を確かめられる（`errors.As`）。`got` は、例外のケースでは、葉が返した値のまま（ふつうは、零値）。
- **走らせる側が確かめること**: 正常のケースは、失敗がないこと（`err == nil`）。例外のケースは、返ってきた失敗が、その例外であること（`errors.As`。「exception」）。戻り値の型が、述語で決まる値の分類か op の分類で絞り込んだ型（`T on C.K`。`Op.returns` が、そのまま `case`）なら、正常のケースで、判定関数 `Classify<C>` を呼び、返ったケースが、型が言うケースのどれかであることも、確かめる。
- **`RunLeaves` の引数**は、`t`、`leaves`（`<入口>.Leaves`）、戻り値の確かめに使う分類の述語（`<C>Cases`。分類の FQN の辞書順。名前は、分類の名前の先頭を小文字にしたもの。Go の予約語か事前宣言の識別子、本体が決まった名前で使う識別子、このファイルの import の名前、先に並んだ述語の名前と当たれば、当たらなくなるまで末尾に `_`）、`c`（`LeafCases`）。述語を使う葉がなければ、`t`、`leaves`、`c` の三つ。
- 葉は、`leaves` のメソッドを、直に呼ぶ（`Ops` を通さない: 継ぎ目も、ほかの op も、通らない）。`ctx` は `context.Background()`。葉は、状態に触れない（`docs/check.md`「葉の op」）ので、状態の用意は、要らない。
- 単位の対象でない配線（「op の置き場」の 3 の、もう一方）も、`Leaves` のメソッドなので、同じに、口を持つ。

### 分類の判定

N が宣言した、述語で決まる値の分類と op の分類 C ごとに、ケースごとの例を受け取るインターフェース `<C>Examples` と、走らせる `Run<C>` を出す（宣言順）。

```go
// WarrantyKindExamples は、分類 shop.WarrantyKind の判定の、ケースごとの例: そのケースに当たる値。
// メソッドが一つでも足りない値は、RunWarrantyKind に渡すところでコンパイルできない。
type WarrantyKindExamples interface {
	// Standard は、ケース Standard に当たる値。
	Standard() shop.Item
	// Yamada は、ケース Yamada に当たる値。
	Yamada() shop.Item
}

// RunWarrantyKind は、ケースごとに、テストを一つ走らせる。テストの名前は、<分類の FQN>/<ケース>。
// ケースごとに、例の値を、判定関数 ClassifyWarrantyKind に渡し、そのケースになることを確かめる。
func RunWarrantyKind(t *testing.T, cases shop.WarrantyKindCases, x WarrantyKindExamples) {
	t.Run("shop.WarrantyKind", func(t *testing.T) {
		t.Run("Standard", func(t *testing.T) {
			kind, err := shop.ClassifyWarrantyKind(context.Background(), cases, x.Standard())
			if err != nil {
				t.Fatalf("want case Standard, got error %v", err)
			}
			if kind != shop.WarrantyKindStandard {
				t.Fatalf("want case Standard, got %s", [...]string{"Standard", "Yamada"}[kind])
			}
		})
		t.Run("Yamada", func(t *testing.T) {
			kind, err := shop.ClassifyWarrantyKind(context.Background(), cases, x.Yamada())
			if err != nil {
				t.Fatalf("want case Yamada, got error %v", err)
			}
			if kind != shop.WarrantyKindYamada {
				t.Fatalf("want case Yamada, got %s", [...]string{"Standard", "Yamada"}[kind])
			}
		})
	})
}
```

- **必要なケース**は、分類のケースごとに、一つ（宣言順）。人が書くのは、そのケースに当たる値の例（型は、分類の対象の型: 「分類」）。確かめ（判定関数が、そのケースを返すこと）は、基底が作る。判定関数は、すべての述語を呼ぶので、一つの例が、ほかのケースの述語が偽を返すことも、確かめる。
- **ない側のケース**（`Classify.absent`）は、例を求めない: `<C>Examples` に、メソッドがない。走らせる側が、値がない（零値の `nil`）ことを渡して、そのケースになることを確かめる（作れる分は、テストごと作る。373）。
- テストの名前は、`<分類の FQN>/<ケース>`。
- 述語で決まる**状態の分類**（述語は、状態の単位のメソッド）には、出さない: 状態を用意する道が、要る（`docs/open.md`「Hozo の側の改善する点」）。

### デコレータのテストの口

`docs/decisions.md`「決めたこと」382、395、398。規則の持ち主は `docs/ide-chain.md`「テストの規約」の「デコレータのテスト」。この節が決めるのは、Go での形。期待する出力の実物は、サンプルの `gen/server/servertest/seams.go`。例外を受けるデコレータの分（「受けるケース」）の実物は、単体テストの入力 `wraps` の `gen/desk/desktest/seams.go`（`plugins/go/internal/gobase/testdata/wraps/expected/`）。

**対象**は、単位の継ぎ目（`docs/config.md`「継ぎ目」）にあるデコレータのうち、単位の対象の op に、一つでも付いているもの（`Seams` にメソッドを持つもの。「継ぎ目」）。本体のあるデコレータ（「本体のあるデコレータ」）と、語彙として読むだけのデコレータには、出さない。対象が一つもなければ、ファイルを出さない。

```go
// seams.go

// RateLimitedCases は、デコレータ rate.limited の、結果ごとのケース。結果の数だけ、メソッドがある。
type RateLimitedCases interface {
	// OK は、op の中身を、ちょうど一度呼んで、その結果を、そのまま返すケース。
	OK() RateLimitedCase
	// LimitExceeded は、例外 rate.LimitExceeded を投げるケース（prevents effects: op の中身を、呼ばない）。
	LimitExceeded() RateLimitedCase
}

// RateLimitedCase は、デコレータ rate.limited の、ケース一つ: その結果になる状態の、継ぎ目の実装。
type RateLimitedCase struct {
	// Seams は、継ぎ目の実装を返す。走らせる側が、付けた op ごとに、一度呼ぶ。
	Seams func() server.Seams
}

// SeamCases は、継ぎ目を持つデコレータの、テストのケース。デコレータごとに、メソッド一つ。
// メソッドが一つでも足りない値は、RunSeams に渡すところでコンパイルできない。
type SeamCases interface {
	RateLimited() RateLimitedCases
}

// RunSeams は、継ぎ目ごと（付けた op と、デコレータの組）、結果ごとに、テストを一つ走らせる。テストの名前は、
// <op の FQN>@<デコレータの FQN>/<結果>。ケースごとに、継ぎ目を、にせものの中身（呼ばれた回数を数え、零値を返す）と一緒に呼び、
// 結果が、そのケースのものであることを確かめる。
func RunSeams(t *testing.T, c SeamCases) {
	t.Run("server.login@rate.limited", func(t *testing.T) {
		cases := c.RateLimited()
		t.Run("OK", func(t *testing.T) {
			k := cases.OK()
			calls := 0
			_, err := k.Seams().LoginRateLimited(context.Background(), server.LoginInput{}, func(ctx context.Context, in server.LoginInput) (server.LoginOk, error) {
				calls++
				return server.LoginOk{}, nil
			})
			if err != nil {
				t.Fatalf("want no error, got %v", err)
			}
			if calls != 1 {
				t.Fatalf("want the body called once, got %d", calls)
			}
		})
		t.Run("LimitExceeded", func(t *testing.T) {
			k := cases.LimitExceeded()
			calls := 0
			_, err := k.Seams().LoginRateLimited(context.Background(), server.LoginInput{}, func(ctx context.Context, in server.LoginInput) (server.LoginOk, error) {
				calls++
				return server.LoginOk{}, nil
			})
			var want rate.LimitExceeded
			if !errors.As(err, &want) {
				t.Fatalf("want exception rate.LimitExceeded, got %v", err)
			}
			if calls != 0 {
				t.Fatalf("want the body not called (prevents effects), got %d", calls)
			}
		})
	})
}
```

- **`<D>Cases` と `<D>Case`** は、デコレータの宣言ごとに、一組。`<D>` は、デコレータの FQN のセグメントを、それぞれ先頭大文字にして連結したもの（`rate.limited` → `RateLimited`）。並びは、`Seams` のメソッドの並びで、そのデコレータが、最初に出てくる順。
- **結果**は、正常（`OK`）と、デコレータの宣言の `throws` の例外ごと（書かれた順。同じ例外は、一度だけ）。名前の決まりは、葉の op の結果と同じ（「葉の op」: 例外の宣言の名前。当たれば、名前空間のセグメントを前に付ける）。`throws` に、デコレータの引数の名前で書いた例外（`throws error`）は、結果にしない（`docs/open.md`「Hozo の側の改善する点」）。`catches` の節も、結果にしない: 受ける例外ごとのテストは、口のメソッドなしに、`RunSeams` が作る（「受けるケース」）。`throws` を持たず、`catches` だけのデコレータの結果は、`OK` だけ。
- **`<D>Case` のフィールドは、`Seams` 一つ**: その結果になる状態の、継ぎ目の実装を返す関数。型は、N の `Seams`（単位の継ぎ目の、すべてのメソッドを持つ）。
- **`SeamCases`** は、対象のデコレータごとに、メソッド一つ（名前は `<D>`）。**`RunSeams`** の引数は、`t` と `c SeamCases` だけ。
- **走らせる単位は、`Seams` のメソッドごと**（付けた op と、デコレータの組）。テストの名前は、`<op の FQN>@<デコレータの FQN>/<結果>`（インテグレーションテストの、出どころの書き方と、同じ。`docs/plugin-go-http.md`「インテグレーションテストの口」）。対応表（「テストの対応表」）に、この名前で載る。
- **継ぎ目に渡すもの**: `context.Background()`、op の引数ごとの零値（「値」）、にせものの中身。にせものの中身は、呼ばれた回数（`calls`）を数え、零値と `nil` を返す関数リテラルで、形は、`Seams` のメソッドの `body` の型。
- **確かめること**は、結果と、その `throws` の節の関係で決まる。

| 結果 | 確かめること |
|---|---|
| `OK` | 失敗が返らない。`calls` が 1 |
| 例外（その節の関係が `prevents effects` だけ） | 返った失敗が、その例外。`calls` が 0 |
| 例外（その節の関係が `keeps effects` だけ） | 返った失敗が、その例外。`calls` が 1 |
| 例外（関係がない。両方ある） | 返った失敗が、その例外。`calls` は、見ない（`_ = calls` を置く: Go は、読まれない変数を、許さない） |

- 当たる名前は、GO-2（「テストのパッケージ」）: `SeamCases`、`RunSeams`、`<D>Cases`、`<D>Case` は、`golang.PublicTestNames` に並ぶ。

#### 受けるケース

`docs/decisions.md`「決めたこと」398。`catches` の節を持つデコレータの継ぎ目には、`RunSeams` が、受ける例外ごとに、テストを一つ足す。人か AI が書くケースは、増えない。

```go
	t.Run("desk.load@trace.renamed", func(t *testing.T) {
		cases := c.TraceRenamed()
		t.Run("OK", func(t *testing.T) { … })
		t.Run("catches", func(t *testing.T) {
			t.Run("Down", func(t *testing.T) {
				k := cases.OK()
				calls := 0
				_, err := k.Seams().LoadTraceRenamed(context.Background(), "", func(ctx context.Context, id string) (string, error) {
					calls++
					return "", trace.Down{}
				})
				var caught trace.Down
				if errors.As(err, &caught) {
					t.Fatalf("want exception trace.Down caught (recovers), got %v", err)
				}
				var want0 trace.Late
				if err != nil && !errors.As(err, &want0) {
					t.Fatalf("want no error or one of the decorator's exceptions (trace.Late), got %v", err)
				}
				if calls != 1 {
					t.Fatalf("want the body called once, got %d", calls)
				}
			})
		})
	})
```

- **受ける例外**は、`Seams` のメソッドごと（付けた op と、デコレータの組）に決める: その op への、そのデコレータの適用のすべて（`@repeatable` のデコレータを、二度付けたものを含む。`Op.decorators` の順）の、宣言の `catches` の節の例外（節の順、節の中の順）。デコレータの引数の名前（`catches error`）は、その適用が渡した例外で置き換える（`Application.args`。`A | B` と、残りの引数で渡したものは、渡した例外ごと。渡していなければ、ない）。例外のユニオンは、メンバーごと（IR で、展開済み）。同じ例外は、一度だけで、扱い（`recovers` / `halts` / `ignores`）は、最初に出てきた節のもの。
- **その op の、計算した例外にない例外**（DEC-16 の warning が出る適用。`docs/check.md`「デコレータ」）にも、作る。確かめるのは、継ぎ目の実装が、デコレータの宣言のとおりに動くかで、op の中身が、その例外を投げうるかでは、ない。
- **置き場**は、その継ぎ目の `t.Run` の中の、結果ごとのテストの後ろの、`t.Run("catches", …)` の中。受ける例外が一つもなければ、`t.Run("catches", …)` を出さない。**テストの名前**は、`<op の FQN>@<デコレータの FQN>/catches/<例外の名前>`。例外の名前の決まりは、結果の名前と同じ（例外の宣言の名前。受ける例外どうしで当たれば、名前空間のセグメントを前に付ける）。`catches` は、予約語（`docs/grammar.md`「字句」）なので、結果の名前（`OK` と、例外の宣言の名前）と、当たらない。対応表（「テストの対応表」）に、この名前で載る。
- **継ぎ目の実装**は、正常のケースのもの（`cases.OK()` の `Seams()`）。
- **`RunSeams` のコメント**には、受けるケースを持つ継ぎ目が、一つでもあれば、二行を足す（「例外を受けるデコレータの継ぎ目には、受ける例外ごとのテスト（<op の FQN>@<デコレータの FQN>/catches/<例外の名前>）を足す: 正常のケースの継ぎ目を、その例外を返す、にせものの中身と一緒に呼び、デコレータの宣言のとおりに終わることを確かめる。」）。一つもなければ、コメントは、変わらない。
- **にせものの中身**は、呼ばれた回数（`calls`）を数え、零値と、その例外（`<例外の型>{}`）を返す。値を返さない op では、その例外だけ。
- **確かめること**は、その例外を受けた節の扱いで決まる。どの扱いでも、`calls` が 1。

| 扱い | 確かめること |
|---|---|
| `ignores` | 失敗が返らない |
| `recovers`（その op への適用の引数を当てた後の、デコレータの `throws` の例外が、一つもない） | 失敗が返らない |
| `recovers`（その `throws` の例外が、一つでもある） | 返った失敗が、受けた例外でない（変数 `caught`）。失敗が返るなら、その `throws` の例外の、どれか（変数 `want0`、`want1`、…。適用の順、節の順。同じ例外は、一度）。受けた例外が、`throws` にもあれば（DEC-6 の warning が出る宣言）、一つ目は、確かめない |
| `halts` | 返った失敗が、受けた例外でない（変数 `caught`）。`halts` の効果が、呼ばれたかは、見ない（継ぎ目の口は、効果を受け取らない。`docs/open.md`「Hozo の側の改善する点」） |

### 口を持たないもの

- 配線の op（中身は、基底が作る。375）。`never` を返す op。
- 状態の単位の、人が書くメソッド（デコレータのない `read` と効果、形で決まるメソッド）。対象に入れるかは、決めていない（`docs/open.md`「Hozo の側の改善する点」）。継ぎ目の実装は、「デコレータのテストの口」。
- 呼ぶ先の op を、にせものに差し替えて、配線の op 一つだけを試す、単体テストの口（375 の、目標）。にせものそのものは、ある（「テストのにせもの」）。

### テストの対応表

走らせる関数（`RunLeaves`、`Run<C>`。相乗りの `RunHTTP`、`RunFlows`、`RunRequirements`）を出すトランスパイラは、テストのパッケージのディレクトリに、テストの対応表 `hozo-tests.<トランスパイラの名前>.json` を出す（形と、hozoc の読み方の持ち主は `docs/plugin-protocol.md`「テストの対応表」）。走らせる関数を呼ぶ一行を、人が書き忘れても、`hozoc tests` が、走っていないテストを名指せるようにする（`docs/cli.md`「tests」）。

```json
{
  "tests": [
    {"name": "server.checkPassword/OK", "file": "gen/server/servertest/leaves.go", "lines": [160, 167]},
    {"name": "server.checkPassword/InvalidCredentials", "file": "gen/server/servertest/leaves.go", "lines": [170, 178]}
  ]
}
```

- **テスト一つ**は、走らせる関数の中の、いちばん内側の `t.Run(<名前>, func(t *testing.T) { … })`。その場の関数の中に、さらに `t.Run` を持つ段（op ごとの段、分類ごとの段）は、テストに数えない。
- **`name`** は、外の段から、その `t.Run` までの、名前の文字列を、`/` でつないだもの（`server.checkPassword/OK`）。口の文書が「テストの名前」と呼ぶものと、同じ。
- **`lines`** は、その場の関数の本体の、`{` の次の行から、`}` の前の行まで。`{` の行と `}` の行を入れないのは、Go のカバレッジが、外の段の文（`t.Run(` の行。その場の関数の `{` まで）を、別の区切りとして数え、その区切りは、絞って走らせて、中の関数が呼ばれなかったときにも、実行済みになるため。本体の中の区切りは、どれも、`{` の次の行より後ろに届く（本体は、空でなく、`{` の後ろで改行する）。
- 並びは、ファイルの順（基底は `leaves.go`、`classify.go`）、その中は、行の順。テスト一つを、一行に書く。項目の順は `name`、`file`、`lines`。字下げは空白二つで、末尾に改行を一つ。
- 対応表は、出したファイルのテキストから作る: 整形した後の Go のソースを読み、上の形の `t.Run` を探す（`golang.TestMap`。`docs/packages.md`「`plugin/golang`（公開）」）。仕様からは、作らない（行は、整形した結果で決まる）。走らせる関数の `t.Run` の、名前が文字列のリテラルでない、中身がその場の関数でない、本体が空、は、生成器の不具合（GO-INTERNAL。相乗りは、`emit` の誤り）。
- 走らせる関数を一つも出さなければ（テストのパッケージを作らないとき）、対応表も出さない。

### カバレッジを読む

基底 `go` は、direction `coverage`（`docs/plugin-protocol.md`「coverage」）を実装する。読むのは、`go test -coverprofile=<パス>` が書く形。

```bash
go test -coverprofile=cover.out -coverpkg=./... ./...
```

```bash
hozoc tests --coverage cover.out
```

- **`-coverpkg` が要る。** 走らせる関数は、テストのパッケージ（`gen/server/servertest`）にあり、それを呼ぶ人のテストは、別のパッケージにある。`go test` は、既定では、テストしているパッケージだけを測るので、`-coverpkg` に、テストのパッケージが入るパターン（`./...` など）を渡さなければ、走らせる関数の行は、実行済みにならない: テストのパッケージが、カバレッジに載らない（`go test ./app/...` のように、パッケージを絞ったとき。`hozoc tests` は、そのファイルを測っていない、と報告する）か、載っても、回数が、すべて 0（`go test ./...` は、テストのないパッケージも、0 回として載せる。`hozoc tests` は、走っていない、と報告する）。どちらも TESTS-1。
- **読める形**（`recognized`）: 最初の行が `mode: set`、`mode: count`、`mode: atomic` のどれか。そうでなければ `recognized: false`。
- 二行目からは、空行を除いて、`<import パス>/<ファイル名>:<始まりの行>.<列>,<終わりの行>.<列> <文の数> <回数>`。この形でない行があれば、誤り（終了コード 1。標準エラーに、行の番号）。
- 要求の `files` のファイルごとに、`<そのディレクトリの import パス>/<ファイル名>`（import パスは「パッケージと import パス」）を名前に持つ行を探す。一行もなければ、そのファイルは、測られていない（応答の `files` に出さない）。`go.mod` が見つからないファイルも、同じ。
- 応答の `lines` は、回数が 1 以上の行の、`[<始まりの行>, <終わりの行>]`。重複を除き、始まりの行、終わりの行の順に並べる。同じ区切りが、二度出てくる（パッケージごとのテストの実行を、一つのファイルにまとめたもの）ときは、どれかの回数が 1 以上なら、実行済み。
- 列は、使わない（対応表の `lines` が、行だけで分けられるように取ってある: 「テストの対応表」）。

## テストのにせもの

基底は、出力ごとに、実装を受け取る口（「実装を受け取る口」）のにせものと、にせものの上に、その名前空間の `Ops` を組み立てる関数を出す（`docs/decisions.md`「決めたこと」384、385、390、391）。使うのは、流れを通すテスト（入口のデコレータのプラグインが出す、インテグレーションテスト: `docs/plugin-go-http.md`「インテグレーションテストの口」）と、人が書くテスト。何を、にせものにし、何を、本物で通すかの決まりの持ち主は、`docs/ide-chain.md`「テストの規約」の「インテグレーションテストの作り直し」。この節が決めるのは、Go での形。期待する出力の実物は、サンプルの `gen/store/storetest/fakes.go` と `gen/server/servertest/fakes.go`。

```go
f := new(servertest.Fakes)                           // 零値のまま、使える
f.Store.Sessions.SessionByToken.Returns(&session)    // 端（read fn）が返す値を言う
f.Mailer.Send.Throws(pkgtype.NotificationFailed{})   // 外の仕組みの op が、例外を投げる
ops := servertest.NewOps(f, leaves)                  // 配線と、葉（leaves）は、本物
_, err := ops.PlaceOrder(ctx, in)
n := len(f.Store.Orders.CreateOrder.Calls)           // どの効果が、何の値で、呼ばれたか
```

### 出すとき

- ファイルは、テストのパッケージ（「単体テストの口」の「テストのパッケージ」）の `fakes.go`。テストの対応表（「テストの対応表」）には、載らない（テストではない）。import の名前に使わない識別子は、`f`、`o`、`ctx`、`k`（本体が、決まった名前で使う）。口を満たす型のメソッドの、引数の局所の名前は、`o`、`ctx` と、このファイルの import の名前を避ける（「名前」の「局所の名前」）。
- **出すのは、`New` が、引数を一つでも取り、テストの中の組み立てを、作れる出力**。`New` が引数を取らない出力には、にせものにするものがない（使う側は、`New()` を、そのまま呼ぶ）。op が一つもない出力（`New` がない: 「ファイル」）にも、出さない。
- **組み立てを作れない出力**（下の「別の名前空間」の表の、最後の行に当たる別の名前空間を、一つでも持つ出力）には、`fakes.go` を出さない。診断も、出さない: 設定は、正しい。その出力の組み立てを要るプラグインが、自分の診断で、報告する（`golang.TestKitOf` の `Blocked`。`docs/packages.md`「`plugin/golang`（公開）」）。

### にせものの型

`New` の引数（「New」の表）ごとに、にせものを、一つ持つ。並びは、`New` の引数の順。

| `New` の引数 | にせものの型 | 言われていないとき |
|---|---|---|
| `leaves` | `LeavesFake`。フィールドは、葉ごとに一つ（`Leaves` の並び） | 本物の葉（`NewOps` に渡したもの）を呼ぶ |
| 状態の単位 | `<状態>StateFake`。フィールドは、単位のメソッドごとに一つ（「状態の単位」の並び: 形で決まる問い合わせ、形で決まる命令、`read`、効果。状態の分類は、述語、効果） | 零値を返す。失敗は、ない |
| 分類の述語 | `<C>CasesFake`。メソッド `Is(k <C>)` | 本物の述語（`NewOps` に渡したもの）を呼ぶ |
| 別の名前空間 | 下の「別の名前空間」 | — |
| `seams` | `SeamsFake`。フィールドは、継ぎ目ごとに一つ（`Seams` の並び） | op の中身（`body`）を、そのまま呼ぶ |

```go
// SessionsStateFake は、状態 store.sessions の単位（store.SessionsState）のにせもの。メソッドごとに、フィールド一つ。
// 言われていないメソッドは、零値を返す。
type SessionsStateFake struct {
	IsEmpty        runtime.Fake[struct{}, bool]
	Get            runtime.Fake[struct{}, []pkgtype.Session]
	SessionByToken runtime.Fake[struct{ Token pkgtype.Token }, *pkgtype.Session]
	CreateSession  runtime.FakeVoid[struct{ Session pkgtype.Session }]
	DeleteSession  runtime.FakeVoid[struct{ Token pkgtype.Token }]
}

// sessionsStateFake は、SessionsStateFake を、store.SessionsState にする。
type sessionsStateFake struct{ f *SessionsStateFake }

var _ store.SessionsState = sessionsStateFake{}

func (o sessionsStateFake) SessionByToken(ctx context.Context, token pkgtype.Token) (*pkgtype.Session, error) {
	return o.f.SessionByToken.Call(struct{ Token pkgtype.Token }{token}, nil)
}
```

- **フィールドの型**は、メソッドが値を返すなら `runtime.Fake[A, R]`、返さないなら `runtime.FakeVoid[A]`（「runtime パッケージ」）。`R` は、メソッドの戻り値の型（`error` の前のもの）。フィールドの名前は、メソッドの名前。コメントは、書かない（何のメソッドかは、口のインターフェースのコメントが言う）。
- **`A` は、呼ばれた引数を覚える、名前のない構造体**: フィールドは、メソッドの引数（`ctx` と、継ぎ目の `body` を除く。宣言の順）。名前は、フィールド名と同じ決まり（「名前」）、型は、引数の Go の型。引数がなければ `struct{}`。形で決まる命令の引数は、`Value`。
- **口を満たすのは、非公開の型**（にせものの型の名前の、先頭を小文字にしたもの: `sessionsStateFake`）。フィールドは、にせもの `f` と、本物を包むもの（`leaves` と、分類の述語）は、本物 `impl`。メソッドは、口のメソッドのすべてで、どれも、`o.f.<メソッド>.Call(<覚える引数>, <本物>)` を返す。`<本物>` は、葉は、本物の葉を呼ぶ関数、継ぎ目は、`body` を呼ぶ関数、状態の単位と、別の名前空間は `nil`。型のすぐ後ろに、口を満たすことの確かめ（`var _ <口> = <型>{}`）を置く。レシーバは `o`。
- **にせものの型と、口を満たす型を、分ける**のは、Go では、フィールドとメソッドが、同じ名前を持てないため。口を満たす型は、公開しない: にせものを、口に渡すのは、`NewOps` だけ。
- **`<C>CasesFake`** は、フィールドを公開せず、メソッド `Is` だけを持つ。`Is(k)` の後は、値によらず、ケース `k` の述語が真、ほかの述語が偽を返す（本物の述語は、呼ばない）。ない側のケース（述語を持たない。「分類」）は、`Is` では、入らせられない（値がないことで、決まる）。

```go
// StandingCasesFake は、分類 pay.Standing の述語（pay.StandingCases）のにせもの。言われていなければ、本物の述語を呼ぶ。
type StandingCasesFake struct {
	set  bool
	kind pay.Standing
}

// Is は、これからの判定が、値によらず、ケース k になるようにする。
func (f *StandingCasesFake) Is(k pay.Standing) { f.set, f.kind = true, k }
```

- コメント: にせものの型は、`// <型> は、<もの>（<口>）のにせもの。<フィールドの言い方>。` と、二行目に、言われていないときの振る舞い。`<もの>` は、`人が実装する葉の op`、`状態 <FQN> の単位`、`名前空間 <M> の op`、`継ぎ目`。口を満たす型は、`// <型> は、<にせものの型> を、<口> にする。`

### 別の名前空間

別の名前空間 M の口（`<M>Ops`。「別の名前空間の op」）に、何を渡すかは、M を出力する、同じ基底の単位の数で決まる（`View.namespaces`。`docs/decisions.md`「決めたこと」383、387）。

| M を出力する単位 | 渡すもの | `Fakes` のフィールド |
|---|---|---|
| この出力と同じ単位が、M も出力する。か、同じ基底の単位が、ちょうど一つ | その出力の配線（本物）: その出力のテストのパッケージの `NewOps(&f.<M>, …)`。その出力が `fakes.go` を持たない（`New` が、引数を取らない）なら、その出力の `New()` | その出力の `Fakes`（`fakes.go` を持たなければ、ない） |
| 同じ基底の単位が、ない（外の仕組み。単位を置かない名前空間） | にせもの `<M>OpsFake`。フィールドは、`<M>Ops` のメソッドごとに一つ。言われていないメソッドは、零値を返す | `<M>OpsFake` |
| 同じ基底の単位が、二つ以上ある（同じ単位のものは、ない）。その出力の組み立てを、作れない。名前空間どうしが、輪になって呼び合う | — | 組み立てを作れない（「出すとき」） |

- **本物を通す出力は、選ばない。** 同じ名前空間を、二つの単位に出したとき、どちらも、テストの対象で（387）、二つを、どう組み合わせて通すかは、決めていない（`docs/open.md`「Hozo の側の改善する点」）。型の置き場（`docs/config.md`「型の置き場」）が、設定の順で最初の単位を選ぶのとは、違う。
- **別の単位の `in` は、見えない**（`View` にあるのは、この単位の `in` に合ったファイルだけ）。別の単位の出力の形（葉を持つか、など）は、`in` を省いた単位として、求める（`golang.HomeOf` と、同じ）。`in` で絞った単位の出力を、別の単位から通すと、`NewOps` の引数が合わず、コンパイルできないことがある。
- テストのパッケージは、通す出力の、入口のパッケージと、テストのパッケージを import する（テストのコードに限った、単位をまたぐ import。テストでない出力は、今までどおり、別の名前空間の op を import しない）。

### Fakes と NewOps

```go
// Fakes は、名前空間 server の、テストの中の組み立て（NewOps）に使う、にせもの。フィールドは、New の引数の順。零値のまま、使える。
type Fakes struct {
	// Leaves は、葉のにせもの。言われていない葉は、本物を呼ぶ。
	Leaves LeavesFake
	// Mailer は、名前空間 mailer（単位がない）の op のにせもの。
	Mailer MailerOpsFake
	// Store は、名前空間 store の出力（gen/store）の、にせもの。その配線は、本物を通す。
	Store storetest.Fakes
	// Seams は、継ぎ目のにせもの。言われていない継ぎ目は、op の中身を、そのまま呼ぶ。
	Seams SeamsFake
}

// NewOps は、にせもの f の上に、名前空間 server の Ops を組み立てる: 配線は、本物。流れの端と、継ぎ目は、にせもの。
// 人が書いたもの（leaves）は、引数で受け取り、にせものが包む。
func NewOps(f *Fakes, leaves server.Leaves) server.Ops {
	return server.New(leavesFake{&f.Leaves, leaves}, mailerOpsFake{&f.Mailer}, storetest.NewOps(&f.Store), seamsFake{&f.Seams})
}
```

- **`Fakes`** のフィールドは、`New` の引数ごとに一つ（引数を取らない出力を通す、別の名前空間には、ない）。名前は、`New` の引数の名前を、フィールド名にしたもの（先頭を大文字に）。コメントは、種類ごとに決まった一行: 葉は、例のとおり。状態の単位は `// <名前> は、状態 <FQN> の単位のにせもの。`、分類の述語は `// <名前> は、分類 <FQN> の述語のにせもの。言われていなければ、本物の述語を呼ぶ。`、別の名前空間と、継ぎ目は、例のとおり（出力の場所は、その出力の `Dir`）。
- **`NewOps` の引数**は、`f *Fakes` の後ろに、人が書いたもの: この出力の `leaves`（`<入口>.Leaves`）、分類の述語（`New` と同じ名前、同じ順。型は `<C>Cases`）、その後ろに、本物を通す別の名前空間の出力の `NewOps` が取る、人が書いたものを、別の名前空間の順に（名前は、`<別の名前空間の引数の名前><その引数の名前の先頭を大文字にしたもの>`: `storeLeaves`）。先に並んだ名前、`f`、このファイルの import の名前と当たれば、当たらなくなるまで、末尾に `_`。人が書いたものを、名前のある構造体ではなく、位置引数で受け取るのは、渡し忘れを、コンパイルエラーにするため（`CLAUDE.md`「実装の漏れは、コンパイルエラーにする」）。
- **`NewOps` の本体**は、入口の `New` を、一度、呼ぶ。引数は、`New` の引数の順に、口を満たす型の値（`<型>{&f.<フィールド>[, <本物>]}`）か、本物を通す出力の `NewOps(…)` / `New()`。効果の入口の確かめ（「効果の入口の確かめ」）は、`New` が包むので、にせものの単位にも、掛かる。
- **`NewOps` のコメント**は、例の二行。二行目の括弧の中は、人が書いたものの引数の名前を、「、」でつないだもの。人が書いたものを取らなければ、一行目だけ。
- **戻り値は、入口の `Ops`**（`export` の op だけ）。内部の op を、にせものの上で呼ぶ道は、出さない。
- `NewOps` は、何度呼んでもよく、`f` を書き換えない。同じ `f` に、二度、組み立てれば、二つの `Ops` が、同じにせものを使う。
- テストのパッケージの、公開の名前（`Fakes`、`NewOps`、にせものの型）が、ほかの公開の名前と当たれば、GO-2（「単体テストの口」の「テストのパッケージ」）。

## コメント

説明（`description.body`）は、行ごとに `// ` を付けて写す（空の行は `//`）。説明がなければ、下の「説明がないとき」を使う。`<名前> は<説明>` の形で、説明が ASCII の文字で始まるときは、「は」の後ろに空白を一つ置く（`// FindUser は email からユーザーを引く`）。

| 宣言 | コメント |
|---|---|
| パッケージ（`doc.go`） | `// Package <パッケージ名> は<名前空間の説明>`。説明がなければ `// Package <パッケージ名> は名前空間 <FQN>。`。内側のパッケージは `// Package inner は名前空間 <FQN> の定義: 型と、内部の op を含むすべての op。使う側は入口のパッケージ <入口の import パス> を import する。テストは、内部の op をここから呼ぶ。` |
| model、scalar、union | `// <型名> は<説明>`。説明がなければ `// <型名> は <種類> <FQN>。`（種類は `model` / `scalar` / `union`） |
| 継承した model | 上の後ろに、`// <元の FQN> を継承し（extends）、<句>もの。<元の FQN> とは別の型で、暗黙には渡せない。`。句は、当たるものをこの順に「、」でつなぐ: なくしたフィールドがあれば `<名前の並び> をなくした`、変えたフィールドがあれば `<名前の並び> を変えた`、足したフィールドがあれば `<名前の並び> を足した`（名前の並びは「、」でつなぐ）。どれもなければ `何も変えていない` |
| exception | `// <型名>: <説明>`。説明がなければ `// <型名> は exception <FQN>。` |
| effect | `// <定数名> は<説明>`。説明がなければ `// <定数名> は effect <FQN>。` |
| op（`Ops`、`Leaves` のメソッド、配線の本体） | 下の「op のコメント」 |
| `Seams` | `// Seams は、継ぎ目: 単位のトランスパイラが継ぎ目を宣言したデコレータの、op への適用ごとに、メソッド一つ。実装は New の引数 seams に渡す。` と `// メソッドが一つでも足りない値は、渡すところでコンパイルできない。` の二行 |
| `Seams` のメソッド | `// <メソッド名> は op <FQN> に付いた @<デコレータの FQN> の継ぎ目（<宣言したトランスパイラの名前>）。body が op の中身で、その前後に置くものを、実装が決める。` |
| 継ぎ目を持つ op の外側（`ops.go`） | `// <Op> は、継ぎ目（<@デコレータの FQN を、書かれた順に「、」で>）を通して、中身（<外側が渡す式>）を呼ぶ。` |
| 継ぎ目を持つ op の中身（`<op>Body`） | 配線なら `// <op>Body は op <FQN> の中身（配線の本体）。外側の <Op> が、継ぎ目を通して呼ぶ。`、`never` を返す op なら `// <op>Body は op <FQN> の中身（<例外の FQN> を投げる）。外側の <Op> が、継ぎ目を通して呼ぶ。`。op のコメント（説明、`Throws:` など）は、`Ops` のメソッドにだけ書く（`Throws:` は継ぎ目を通した後のもので、中身だけのものではない） |
| `<M>Ops` のメソッド | 「op のコメント」の一行目を `// <メソッド名> は op <FQN>: <説明>` に替えたもの。`Throws:` などは、見る側から見た `View.ops[<FQN>]` の値 |
| 状態の単位のメソッド | 下の「状態の単位のメソッド」 |

**op のコメント**:

```go
// <メソッド名> は<説明>
//
// 引数:
//   - <引数名>: <@param の説明>
//
// Throws: <例外の FQN を「, 」で>
// Reads: <状態の FQN を「, 」で>
// Effects: <効果を「, 」で>
```

- 説明がなければ、一行目は `// <メソッド名> は op <FQN>。`。
- `引数:` の段は、`@param` があるときだけ。`Throws:` / `Reads:` / `Effects:` の行は、それぞれ `Op.signature` の `exceptions` / `reads` / `effects` が空でないときだけで、並びは、その列の順。区切りは、仕様の節と同じ `,`（と空白一つ）。
- **`Reads:` に並ぶのは、読む基底の状態**（`Signature.reads`）: 名前を付けた `read` を呼んでも、状態を名前で読んでも、その状態の FQN（`Reads: store.users`。どの `read` かは書かない。読む状態は、書き方によらない: `docs/check.md`「節に書く要素」）。
- **`Effects:` に並ぶのは、効果**（`Signature.effects` の `EffectRef`）: 宣言した効果は、その FQN（`store.createOrder`）。直に呼んだ組み込みの効果は、`<状態の FQN>.<操作>`（`store.users.post`）。
- 説明の段と次の段の間は `//` の一行で空ける。`Throws:` から最後の行までは、空けずに続ける。説明のほかに何もなければ、説明だけ。
- op が純粋かどうかは、コメントに書かない（「op の形」）。
- op が内部かどうかは、コメントに書かない。

**状態の単位のメソッド**:

| メソッド | コメント |
|---|---|
| `IsEmpty`（`list`） | `// IsEmpty は問い合わせ（形 list で決まる）: <状態の FQN> に要素が一つもなければ true。` |
| `IsEmpty`（`nullable`） | `// IsEmpty は問い合わせ（形 nullable で決まる）: <状態の FQN> に値がなければ true。` |
| `Get`（`list`） | `// Get は問い合わせ（形 list で決まる）: <状態の FQN> の値。すべての要素を、足した順に。` |
| `Get`（`nullable`） | `// Get は問い合わせ（形 nullable で決まる）: <状態の FQN> の値。なければ nil。` |
| `Get`（`single`） | `// Get は問い合わせ（形 single で決まる）: <状態の FQN> の値。` |
| `Post` / `Put` / `Delete`（形で決まる命令） | `// <操作> は命令（形 <形> で決まる）: <状態の FQN> の組み込みの効果 <post / put / delete>。` |
| `read` | `// <メソッド名> は問い合わせ（read <read の FQN>）: <説明>`。説明がなければ `// <メソッド名> は問い合わせ（read <read の FQN>）。` |
| 効果 | `// <メソッド名> は命令（効果 <効果の FQN>、effects <宛先>）: <説明>`。説明がなければ `// <メソッド名> は命令（効果 <効果の FQN>、effects <宛先>）。`。効果が `throws` を持てば（`Effect.throws`）、`effects` の前に、`throws <例外の FQN を「, 」で>、` を置く（`（効果 store.createUser、throws type.EmailTaken、effects store.users.post）`。仕様の節と同じ順）。宛先は、組み込みの効果なら `<状態の FQN>.<post / put / delete>`、行の遷移を持つなら（`Effect.row`）その後ろに空白一つと「行の遷移の表示」（`shop.orders.put as shop.Stage.InProduction`）、遷移なら `<宛先の FQN> as <前> -> <後>`。効果が、絞り込んだ型の引数を持てば、`throws` と `effects` の前に、「絞り込んだ引数の表示」と `、` を置く（`（効果 shop.startProduction、order on shop.Stage.Accepted、effects shop.orders.put as shop.Stage.InProduction）`）。語は、仕様の宣言の綴り（`effects 宛先`。`docs/grammar.md`「exception / effect / read」）と同じ |
| ケースの述語 | `// <ケース名> は問い合わせ: ケース <ケース名> の述語。` |

- `read` と効果の説明（`description.body`）を、単位のメソッドのコメントに写すのは、デコレータのない `read` と効果の中身を人が書くとき、何をするものかを言うのが、説明だけだから。説明が二行以上なら、二行目から先は、行ごとに `// ` を付けて続ける。「は」の後ろの空白の規則（この節の頭）は、ここでは使わない（説明の前は `: `）。

状態の単位の型のコメントは、基底の状態なら `// <型名> は状態 <FQN>（<形の語><<要素の型の表示>>）の単位。実装は New の引数 <引数名> に渡す。` と `// メソッドが一つでも足りない値は、渡すところでコンパイルできない。` の二行。状態の分類なら、一行目が `// <型名> は状態の分類 <FQN>（<対象の状態の FQN を「、」で> を対象にした classify）の単位。実装は New の引数 <引数名> に渡す。`。

**行の遷移の表示**（コメントの中の、`Effect.row`）: 仕様の書き方で、IR から組み立てる。`to` があれば `as <分類の FQN>.<ケース名>`、なければ `as else`。

**絞り込んだ引数の表示**（コメントの中の、効果の引数）: 効果の引数（`Effect.params`）のうち、型が `case` のものを、仕様の書き方で、宣言の順に、`、` でつなぐ: `<引数名> on <分類の FQN>.<ケース名>`（いくつかのケースは ` | ` でつなぐ: `order on shop.Stage.Draft | shop.Stage.Accepted`）。行の引数なら、これが、効果の前のケース。

**型の表示**（コメントの中の型）: 核の型は名前、`ref` は FQN、`optional(T)` は `T?`、`array(T)` は `T[]`、`case` はその `of`。

## runtime パッケージ

生成したコードが import する `github.com/mew-ton/hozo/runtime`。基底の出力が使うのは、次の四つと、核の型 `date`・`time` の値の型（下の表）。

```go
type Effect string
type Exception interface { error; HozoException() string }
type ClassificationError struct { Classify string; Hits []string }   // 分類の誤り（判定関数と、型で決まる分類の match が返す）。Error() を持つ
type CaseError struct { Effect, Param, Classify string; Want []string; Got string }   // 効果の入口の確かめの失敗（「効果の入口の確かめ」）。Error() を持つ
```

テストのにせもの（「テストのにせもの」）が使うのは、次の二つ。`testing` は、import しない。

```go
type Fake[A, R any] struct { Calls []A; … }    // 値を返すメソッド一つのにせもの
func (f *Fake[A, R]) Returns(v R)               // これからの呼び出しが、v を返す（本物は、呼ばない）
func (f *Fake[A, R]) Throws(err error)          // これからの呼び出しが、R の零値と、err を返す（本物は、呼ばない）
func (f *Fake[A, R]) Call(args A, impl func() (R, error)) (R, error)

type FakeVoid[A any] struct { Calls []A; … }   // 値を返さないメソッド一つのにせもの
func (f *FakeVoid[A]) Returns()                 // これからの呼び出しが、何もせずに、正常に終わる（本物は、呼ばない）
func (f *FakeVoid[A]) Throws(err error)         // これからの呼び出しが、err を返す（本物は、呼ばない）
func (f *FakeVoid[A]) Call(args A, impl func() error) error
```

- `Call` は、生成したコードが呼ぶ: `args` を `Calls` に足し（呼ばれた順）、`Returns` か `Throws` が言われていれば、そのとおりに返す。言われていなくて、`impl` が `nil` でなければ、`impl` を呼んで、その結果を返す。`nil` なら、零値を返す（失敗は、ない）。
- `Returns` と `Throws` は、後に言った方が、効く。どちらも、すべての呼び出しに、同じ結果を返す（呼び出しごとに、違う値を返す道は、ない）。
- 零値のまま、使える。同時に呼ばれることは、守らない（テストの中で、一つの流れが呼ぶためのもの）。

- 核の型 `date` と `time` の値の型も、ここに置く（「型」）。どちらも、時刻帯を持たず、`==` で比べられ、`encoding/json` では、文字列になる（`MarshalText` / `UnmarshalText`）。

  | 型 | 核の型 | 零値 | 文字列の形 | 作る・読む | 取り出す |
  |---|---|---|---|---|---|
  | `Date` | `date` | `0001-01-01` | `2006-01-02` | `NewDate(年, 月, 日)`、`DateOf(time.Time)`、`ParseDate(文字列)` | `Year` / `Month` / `Day`、`In(時刻帯)`（その日の 0 時）、`Compare`、`String` |
  | `TimeOfDay` | `time` | `00:00:00` | `15:04:05`。秒より細かい部分は、末尾の 0 を除いた小数部（読むときは、0〜9 桁） | `NewTimeOfDay(時, 分, 秒, ナノ秒)`、`TimeOfDayOf(time.Time)`、`ParseTimeOfDay(文字列)` | `Hour` / `Minute` / `Second` / `Nanosecond`、`SinceMidnight`、`Compare`、`String`、`Fixed`（小数部を、いつも 9 桁で書く） |

- `Effect` と `Exception` は `types.go`、`ClassificationError` は `classify.go` と `wiring.go` が使う。`CaseError` は、`states.go` の、入口の確かめが使う: `Effect` は効果の FQN、`Param` は引数の名前、`Classify` は分類の FQN、`Want` は、引数の型が言うケース（型に書かれた順）、`Got` は、判定関数が返したケース。仕様の例外ではない（`Exception` を満たさない）。`ops.go` と `states.go` が `runtime` を import するのは、関数の引数か戻り値に、`date` か `time` があるときと、`states.go` が、入口の確かめを持つとき。
- `MissingError` はなくした（「実装を受け取る口」）。
- `runtime` には、標準の相乗りが使う型も置く: `Violation`、`ValidationError`、`CheckFormat` は `go-valid` のもの（持ち主は `docs/plugin-go-valid.md`「runtime」）。`DBTxKey`（`ctx` が運ぶ DB のトランザクションの鍵。`struct { DB *sql.DB }`）は `go-sqlite` のもの（持ち主は `docs/plugin-go-sqlite.md`「トランザクション」）。Go の出力が import するライブラリを一つにするため。`DBTxKey` は、それに加えて、同じ DB を使う二つの名前空間の出力が、同じ鍵を使うため（出力のパッケージごとの型では、共有できない）。
- API を変えるときは、生成したコードのヘッダに書く版（「ファイル」）と合わせて互換を保つ。

## 決定性

- 同じ `View` から、同じバイト列を返す（`docs/plugin-protocol.md`「emit」）。読むのは `View` と、import パスを求めるための `go.mod` だけ。時刻、乱数、環境（`HOZO_PROJECT_ROOT` を除く）、マップの走査の順に依らない。
- 並びは、この文書に書いた順（宣言順、FQN の辞書順）だけで決まる。
- 応答の `files` は、パスの辞書順。
- 同じ名前空間の二つの単位の基底のファイルは、`in` と単位の継ぎ目が同じなら、import パスに現れる自分のディレクトリ（テストの対応表では、`file` のパスの頭の、出力先のディレクトリ）を除いて、バイト単位で同じ。単位に並べたほかのトランスパイラが基底の出力を変えるのは、マニフェストで宣言した継ぎ目（`View.config.seams`）を通してだけで、そのオプションは変えない。

## 診断

`emit` の応答の `errors` で返し、同じものを、検査の段（direction `check`）でも返す（どれも error の診断になり、`CheckFailed`。`docs/plugin-protocol.md`「emit」「check」）。位置は、表に書いた宣言の FQN。

| ID | 重さ | 意味 |
|---|---|---|
| GO-1 | error | Dir(U, N) の上に `go.mod` がない（「パッケージと import パス」）。位置なし |
| GO-2 | error | 生成する名前が当たる（「名前」）。当たった二つの宣言をメッセージに示す。位置は後ろの宣言 |
| GO-3 | error | 型のパッケージどうしの import が循環する（「パッケージと import パス」。型と、値の分類・op の分類が、互いの名前空間の型を使う）。輪に入る出力を、単位（何番目の `[[emit]]` か）と名前空間でメッセージに示す。位置なし。輪に入る出力ごとに出る |
| GO-4 | error | 基底が作るディレクトリが、同じ単位が出力する子の名前空間の出力先と重なる: 内側のフォルダ `inner` と `N.inner`（「内側のフォルダと入口」。位置は、名前空間の最初の内部の op）、テストのパッケージ `<パッケージ名>test` と `N.<パッケージ名>test`（「単体テストの口」。位置は、口を持つ最初の宣言）、型のパッケージ `types` と `N.types`（「型のパッケージ」。位置なし） |
| GO-5 | error | `[emit.options.go]` に、知らないキーがある（「マニフェスト」）。位置なし |
| GO-6 | error | `variant` の絞り込みの `as` の型と等しい型のアームが、union に二つ以上ある（「match」）。位置は分類 |
| GO-7 | error | 単位のトランスパイラが、本体のあるデコレータに、継ぎ目を宣言している（「本体のあるデコレータ」）。位置は、デコレータの宣言。メッセージに、宣言したトランスパイラの名前 |
| GO-INTERNAL | error | 生成器の不具合（利用者の仕様の誤りではない）: 生成したコードを `go/format` で整形できない、検査を通った IR には来ないはずの形、`View` が欠けている、型の置き場がない（CFG-22 が通したはずのもの） |

- warning は返さない。`catches` を持つデコレータが配線の op に付いているときの warning は、やめた（基底はデコレータの節を見ない。「デコレータ」）。
- GOEMIT-1〜GOEMIT-6 と GOEMIT-INTERNAL は欠番。本体に組み込んだ Go の出力（「既定の出力」）の規則で、出力を基底のプラグインに移したときになくなった。番号は別の意味に使い回さない（欠番の一覧は `docs/requirements.md`「検査の規則の一覧」）。行き先:

| なくなった ID | 規則 | 行き先 |
|---|---|---|
| GOEMIT-1 | `out` の上に `go.mod` がない | GO-1 |
| GOEMIT-2 | 大文字化で名前が衝突する | GO-2（当たる名前の範囲を広げた） |
| GOEMIT-3 | `valid` の制約が元の型に合わない、など | GOVALID-1（`docs/plugin-go-valid.md`） |
| GOEMIT-4 | 既定値が `valid` の制約に合わない | GOVALID-2（同上。フィールドの既定値をなくしたときに、欠番になった） |
| GOEMIT-5 | 出力どうしの import が循環する | GO-3 |
| GOEMIT-6 | 組み込みの出力が支えない書き方（移行の間だけの規則） | なし（基底は、検査を通ったすべての書き方を出す。支えないのは GO-6 だけ） |
| GOEMIT-INTERNAL | 生成器の不具合 | GO-INTERNAL |

## これまでの形から変えたこと

本体に組み込んでいた Go の出力（これまでの文書「Go の既定の出力」。この文書が置き換えたので、今は git の履歴にだけある）と、その後のこの基底の形から変えたこと。決定の番号は `docs/decisions.md`。

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
| 行の引数のケースは、どこでも確かめない。テストを受け取る口は、ない | 絞り込んだ引数を持つ効果は、`New` が包んだ単位が、入口で、引数のケースを確かめる（「効果の入口の確かめ」）。人か AI が中身を書く関数（葉の op、分類の判定）に、結果ごとの単体テストを受け取る口を、テストのパッケージに出す（「単体テストの口」） | 371〜375 |
