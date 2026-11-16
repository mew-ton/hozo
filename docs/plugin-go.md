# go プラグイン（Go の基底）

## この文書が決めること

Go の**基底**のトランスパイラ `go`（`docs/config.md`「基底と相乗り」）が、どんな仕様に対しても何を出すか。プラグインは `plugins/go/`、形の規則を写した公開の SDK は `plugin/golang`（`docs/packages.md`）、生成したコードが import するのは `runtime`。入力は `docs/ir.md` の `View`（出力ごとに一つ。見る側は出力する名前空間）、出力は `View.config.dir` の下のファイル（パスはルートからの相対）。

基底が出すのは、**語彙を知らなくても、仕様と設定だけから一通りに決まるもの**: 型、op の関数の形、実装を受け取る口（インターフェース。`read` と効果は、状態の単位のメソッド）、配線の本体、単位のトランスパイラが宣言した継ぎ目（「継ぎ目」）、それらを組み立てる `New`。「設定」は、`hozo.toml` と、単位のトランスパイラのマニフェスト（`View.config`）。語彙に固有のもの（制約を確かめる関数、HTTP のハンドラ、表と SQL、CLI）は出さない。それは、この形の上に相乗りするトランスパイラが足す（`docs/plugin-go-valid.md`、`docs/plugin-go-sqlite.md`、`docs/plugin-go-http.md`、`docs/plugin-go-cli.md`）。型を写す op の中身も、機械的に決まる場合を含めて出さない（写すのは人が書く葉の op。写し忘れは、このツールでは確かめない）。

この文書の「N」は出力する名前空間（`View.viewer`）、「U」はその単位。「単位」は設定の単位（`docs/config.md`「単位」）で、「状態の単位」はこの文書の中の別の言葉（「状態の単位」）。期待する出力の実物は、サンプル `tests/samples/shop-server/` の `gen/`（`docs/testing.md`「トランスパイラのテスト」）。

## マニフェスト

```toml
name     = "go"
command  = ["go", "run", "github.com/mew-ton/hozo/plugins/go"]
protocol = "0.6"

[[transpiler]]
name     = "go"
language = "go"
```

- 提供するトランスパイラは `go` の一つ。`requires` を持たない（基底）。言語 `go` を定める。
- オプションはない。`[emit.options.go]` にキーが一つでもあれば GO-5。
- 語彙は提供せず、どの語彙も読まない。`seams` も持たない。デコレータの適用が基底の出力に効く道は「デコレータ」の三つで、どれも hozoc が計算して IR と `View` に置いたもの（`Signature`、`View.config.seams`）を通る。
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
- **別の名前空間 M の型**（と、述語で決まる分類）は、M の型の置き場（`docs/config.md`「型の置き場」: U 自身が M を出力するならその出力、そうでなければ基底が `go` で M を出力する最初の単位の出力）の**入口のパッケージ**（下）を import する。複製はしない。置き場があることは hozoc が確かめている（CFG-22）。
- 同じ名前空間 N を出力する別の単位のパッケージは import しない。
- **import の名前**: 名前空間のパッケージは、パッケージ名で import する。パッケージ名が import パスの最後の要素と違えば、名前を書く（`pkgtype "example.com/shop-server/gen/type"`）。一つのファイルの中で名前が当たるとき（標準ライブラリか `runtime` の import、内側のパッケージ `inner`、先に決めた別の名前空間のパッケージ）は、名前空間の FQN の辞書順に名前を決めていき、当たった方に `2`、`3`、… を付ける（`store2`）。標準ライブラリ、`runtime`、`inner` の名前は変えない。**基底が決まった名前で使う識別子**も、import の名前に使わない: パッケージ名がそれと同じ名前空間は、初めから `2`、`3`、… を付けて import する（名前空間 `ops` は `ops2`、名前空間 `ctx` は `ctx2`）。二種類ある。（1）基底が定義のパッケージに宣言する非公開の名前のうち、パッケージ名になりうるもの: `New` が返す構造体 `ops` と、補助 `ptr`。Go では、パッケージの宣言と、そのパッケージのどのファイルの import の名前も、同じ名前ならコンパイルできないので、**どのファイルでも**（入口のパッケージのファイルと、相乗りのファイルでも）使わない。（2）基底が、関数の本体の中で決まった名前で使う識別子のうち、その本体が名前空間のパッケージの型を書きうるもの: メソッドのレシーバ `o` と、`ctx`、`err`。その名前の局所の変数が、同じ名前のパッケージを隠す（`return ctx.Info{}, err` が書けない）ので、**基底のファイルでは**使わない。相乗りのファイルは、（2）に従わず、自分の局所の名前の方を変える（それぞれの文書）。import のブロックは、標準ライブラリ、空行、それ以外の順で、それぞれパスの辞書順。一つなら一行で書く。
- **出力から出力への import は一方向でなければならない。** 出力（単位と名前空間の組）が、import を辿って自分に戻るなら GO-3 で、何も書き出さない（Go は import の循環をコンパイルできない）。import する相手は、`docs/config.md`「型の置き場」の表が数える宣言の置き場で、どの出力についても `View` だけから求まる。直すのは仕様の側: 互いに使う型を、一方か、第三の名前空間に寄せる。

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
- 同じ単位が、N の子の名前空間 `N.inner` を出力するなら GO-4（Dir(U, N.inner) が内側のフォルダと重なる）。
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

### ファイル

ファイルの先頭は `// Code generated by hozoc go <版>; DO NOT EDIT.`、空行、（`doc.go` だけはパッケージのコメント、）`package` 節。版は、このプラグインの版（`plugin.Version`。このリポジトリのプラグインはどれも同じ値を持つ）。`go/format` で整形する。整形に失敗したら GO-INTERNAL（生成器の不具合）で、整形前の内容を標準エラーに出す。

定義のパッケージのファイル（中身がなければ作らない。`doc.go` はいつも作る）:

| ファイル | 中身（この順） |
|---|---|
| `doc.go` | パッケージのコメント（「コメント」） |
| `types.go` | model、scalar、union、exception、effect を、宣言の順に一つずつ（「型」） |
| `ops.go` | `Ops`、`Leaves`、`<Ns>Ops`（名前空間の FQN の辞書順）、`Seams`、`New`、構造体 `ops`、`ops` のメソッドのうち配線の本体でないもの（op の宣言順。継ぎ目を持つ op の外側のメソッドと、`never` を返す op の中身のメソッドを含む。「継ぎ目」）。N の op が一つもなければ作らない |
| `states.go` | 状態の単位（基底の状態と、述語で決まる状態の分類を、宣言順に） |
| `classify.go` | 述語で決まる分類の、列挙の型と定数、述語のインターフェース、判定関数（宣言順） |
| `wiring.go` | 配線の op の本体（宣言順。継ぎ目を持つ配線は、中身の名前のメソッド）。値をポインタにする補助 `ptr` を使っていれば、末尾に一つ |

分けた出力の入口のファイルは、内側の同じ名前のファイルの再公開: `doc.go`（N のパッケージのコメント）、`types.go`、`ops.go`（入口の `Ops`、`Leaves` と `<Ns>Ops` と `Seams` の別名、`New`）、`states.go`、`classify.go`。`wiring.go` はない。内側のファイルに再公開するものがなければ、入口のファイルも作らない。

### 相乗りのファイル

`go` に相乗りするトランスパイラの決まり。どれも、プラグインどうしで当たらないようにするためのもの。

- ファイルは、**入口のパッケージ**（`<Dir(U, N)>/`）に、プラグインごとに決めた名前のファイル一つで置く。型にメソッドを足すもの（`go-valid` の `Validate`）だけは、Go がメソッドを型の定義のパッケージにしか書かせないので、**定義のパッケージ**に置く。自分のパッケージを持つもの（`go-cli`）は、`<Dir(U, N)>/` の下の、プラグインごとに決めたディレクトリに置く。
- 基底のファイル名（`doc.go`、`types.go`、`ops.go`、`states.go`、`classify.go`、`wiring.go`）と、ディレクトリ名 `inner` は使わない。
- パッケージに足す非公開の名前には、プラグインごとの前置きを付ける。公開の名前は、プラグインの文書に並べる。

| トランスパイラ | ファイル | 置くパッケージ | 非公開の名前の前置き | 文書 |
|---|---|---|---|---|
| `go-valid` | `validate.go` | 定義 | `valid` | `docs/plugin-go-valid.md` |
| `go-sqlite` | `db.go` | 入口 | `db` | `docs/plugin-go-sqlite.md` |
| `go-http` | `http.go` | 入口 | `http` | `docs/plugin-go-http.md` |
| `go-cli` | `cli/cli.go`（パッケージ `cli`） | 自分のパッケージ | — | `docs/plugin-go-cli.md` |

- 相乗りは、置き場と名前を `plugin/golang`（`docs/packages.md`「`plugin/golang`（公開）」）で求める。この文書の規則を自分で書き写さない（`docs/plugin-protocol.md`「相乗りが基底の形を知る方法」）。
- 表にないトランスパイラ（プロジェクトの中のプラグインのものなど）も、同じ決まりに従う: 表のファイル名、前置き、公開の名前を避け、自分のものを自分の文書に書く。
- 関数の引数の並び（`ctx` の有無を含む）を決めるのは基底だけで、相乗りは変えられない。相乗りが基底の出力に足せるのは、自分のファイルと、宣言した継ぎ目（「継ぎ目」）だけ。
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
| 分類 C の列挙と述語 | 型 `<C>`、定数 `<C><ケース名>`（`default` は `<C>Default`）、述語のインターフェース `<C>Cases`、判定関数 `Classify<C>` |
| 継ぎ目のメソッド（`Seams`） | op 名を先頭大文字にしたものに、デコレータの FQN のセグメントをそれぞれ先頭大文字にして連結したものを付ける（`server.login` の `@rate.limited` → `LoginRateLimited`） |
| 継ぎ目を持つ op の、中身のメソッド | op 名の先頭を小文字にして `Body` を付ける（`login` → `loginBody`）。非公開の名前 |
| `New` の引数と、構造体 `ops` のフィールド | 「New」 |
| op・read・effect の引数名、配線の束縛名 | そのまま。下の「局所の名前」で当たれば末尾に `_` |

- 先頭だけを大文字にする（`Id`、`Url`）のは、Go の慣習（`ID`）より決定性と予測しやすさを取るため。
- **局所の名前**: メソッドの中で、次の名前は生成したコードが使う。op・read・effect の引数名と束縛名がこれと同じなら、末尾に `_` を付ける: Go の予約語、事前宣言の識別子（`error`、`string`、`nil`、`len` など）、`o`、`ctx`、`err`、`ptr`、定義のパッケージの `ops.go` と `wiring.go` が import するパッケージの名前（`context`、`time`、`runtime`、名前空間のパッケージ）。
- 生成したコードが作る局所の変数（状態を読んだ値、途中の値）の名前は「配線の本体」。引数名、束縛名、先に作った変数と当たれば、当たらなくなるまで末尾に `_` を足す。
- **名前が当たれば GO-2**: 定義のパッケージの公開の名前どうし（型、`Effect…`、`Ops`、`Leaves`、`<状態>State`、`<Ns>Ops`、`Seams`、分類の名前、`New`、`Classify<C>`。大文字化で同じになる `id` と `Id`、合成した名前と宣言の名前を含む）、一つの構造体のフィールドどうし（例外の引数を含む）、一つのインターフェースのメソッドどうし（状態の単位の、形で決まるメソッド `IsEmpty` / `Get` / `Post` / `Put` / `Delete` と、ケースの述語と、その単位の read と効果の名前を先頭大文字にしたもの: 状態 `users` の read `get`、同じ状態の read `findUser` と効果 `FindUser`。形で決まるメソッドは、その単位に出すものだけを数える: 配線が `users.post(…)` を直に呼んでいなければ、`Post` は単位になく、効果 `post` は当たらない。`Ops` の中の `foo` と `Foo`。`Seams` の中の、op `loginRate` の `@limited` と op `login` の `@rate.limited`、`@repeatable` のデコレータを一つの op に二度付けたもの）、構造体 `ops` のフィールドと中身のメソッド（状態 `loginBody` と、継ぎ目を持つ op `login`）。直すのは仕様の名前。

## 型

| IR | Go |
|---|---|
| `string` / `boolean` / `int` / `float` | `string` / `bool` / `int64` / `float64` |
| `Time` / `Duration` | `time.Time` / `time.Duration` |
| `optional(T)` | T が `array`、`record`、union（`arms`）への `ref` ならそのまま（`nil` が「なし」）。それ以外は `*T` |
| `array(T)` | `[]T` |
| `record(K, V)` | `map[K]V` |
| model への `ref` | 構造体の型 |
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
- model から出すのは、構造体だけ。値を作る関数（初めの値を入れる関数）は出さない: フィールドの既定値は、言語にない（`docs/decisions.md`「設計書を変える決定」S24）。使う側は、零値（`var in <Model>`、`<Model>{}`）から始め、決まった値を入れるのは、それを言うデコレータを読む相乗り（`docs/plugin-go-http.md`「入力の読み方」、`docs/plugin-go-cli.md`「引数の解釈」）。

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
- それ以外のアームの型（別の名前空間の model、scalar、核の型、`optional`、`array`、`record`、union）は、マーカーをその型に生やせないので、定義のパッケージに包む型 `type <Union><アーム名> struct{ Value <型> }` を作り、それにマーカーを生やす。包む型は、マーカーメソッドの並びの後ろに、アームの順で置く。

### exception

```go
// InvalidCredentials: メールアドレスかパスワードが一致しない
type InvalidCredentials struct{}

func (InvalidCredentials) Error() string         { return "type.InvalidCredentials" }
func (InvalidCredentials) HozoException() string { return "type.InvalidCredentials" }

var _ runtime.Exception = InvalidCredentials{}
```

- 引数を持つ例外（`exception LimitExceeded(retryAfter: Duration);`。`docs/grammar.md`「exception / effect / read」）は、**引数ごとにフィールド一つ**の構造体。並びは `Exception.params` の順で、名前とタグは model のフィールドと同じ（「名前」）。引数のない例外は `struct{}`。値を一つの `Value` に包む形は、なくした。

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

効果ごとに、この定数一つ。効果を行うメソッドは、定数とは別に、宛先の単位に出す（「状態の単位」）。宛先が分類の遷移（`Effect.target` が `transition`）の効果は、コメントの末尾に一行足す: `// on <宛先の FQN>: <前> -> <後>`（ケースの集合は `any` か `A | B`）。

## op の形

op のメソッドの形は、その op の宣言だけで決まり、どのインターフェースに現れても、どの名前空間から見ても同じ。`export` の有無にもよらない。

| 部分 | 規則 |
|---|---|
| 名前 | op 名を先頭大文字にしたもの |
| `ctx` | 最初の引数に `ctx context.Context` を取る。**純粋な op（`Op.signature.pure` が真。`docs/check.md`「純粋」）は取らない** |
| 引数 | `Op.params` の並びのまま、`<局所の名前> <型>` |
| 戻り値 | `returns` が型なら、その型。`null`（void）と `never` なら、値はない |
| `error` | 最後の戻り値に `error` を返す。**純粋で、かつ `Op.signature.exceptions` が空の op は返さない**（失敗しえない）。戻り値が二つなら `(T, error)` |

- `ctx` は、中断と期限を伝えるための、標準の `context.Context`（Go の作法）。自前のコンテキスト型は作らず、基底は、値を載せて引数の代わりにすることには使わない。生成したコードは、受け取った `ctx` を、呼ぶ先にそのまま渡す（継ぎ目の実装が、`body` に渡す `ctx` に、呼び出しの間だけのものを載せることはある: `docs/plugin-go-sqlite.md`「トランザクション」）。
- 純粋さと `Signature.exceptions` は、op 自身の名前空間から見たもの（`View.program` の `Op.signature`）を使う。純粋な op はデコレータを持たないので、どの名前空間から見ても同じ値になる。
- 形は三通り: 純粋でない（`ctx` を取り、`error` を返す）、純粋で `throws` がある（`ctx` を取らず、`error` を返す）、純粋で `throws` がない（どちらもない）。関数の形を見れば、その op が状態や外部に触れるかが分かる。仕様の一語（デコレータ、`throws`）と、配線が呼ぶもので形が変わり、人が書いた実装は、コンパイルエラーで気づく。
- **`read` と効果のメソッドの形**は、op とは別に決まる（「状態の単位」）: 状態に触れるので、いつも `ctx` を取り、`error` を返す。
- 戻り値がなく、純粋で、`throws` もない op は、引数を受け取って何も返さないメソッドになる。
- **`never` を返す op** は、値を返さず `error` だけを返す（`Unauthorized() error`。デコレータが付いていれば `ctx` も取る）。中身は基底が出す（「op の置き場」）。

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
	// 純粋（ctx を取らない）
	CheckPassword(user pkgtype.User, password pkgtype.Password) error
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
	// CreateUser は命令（効果 store.createUser、on store.users.post）: ユーザーを登録する
	CreateUser(ctx context.Context, user pkgtype.User) error
}
```

メソッドは、この順: 形で決まる問い合わせ、形で決まる命令（配線が直に呼ぶものだけ）、`read`（宣言順）、効果（宣言順）。どれも状態に触れるので、`ctx` を取り、`error` を返す。

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
- **述語で決まる状態の分類** C も、同じ形の単位 `<C>State` を持つ。メソッドは、ケースごとの述語（宣言順。`default` は持たない。`<ケース名>(ctx context.Context) (bool, error)`）、命令（その分類の遷移を宛先にする効果。宣言順）。型で決まる状態の分類は、単位を持たない。
- 二つの基底の状態にまたがるものは、ない: `read` の宛先は基底の状態一つ（`docs/check.md`「read の宛先」）、効果の宛先は一つの状態の一つの変化（同「効果の宛先」）。二つの状態を一緒に変える処理は、二つの効果を呼ぶ配線で書く。
- 単位のメソッドの実装は、その `read` か効果のデコレータを読む相乗りが出すか（`docs/plugin-go-sqlite.md`「状態ごとの型」）、人が書く。デコレータのない `read` と効果、形で決まる命令のうち、相乗りが出さなかったものは、人が、相乗りの型を埋め込んで書き足す。

### 別の名前空間の op

```go
// StoreOps は名前空間 store の op のうち、この名前空間の配線が呼ぶもの。実装は New の引数 store に渡す。
// メソッドの形は相手の op の判定に従う（どの名前空間から見ても同じ）ので、store の出力の Ops は、そのまま渡せる。
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
| 3 | 単位の対象の配線が `match` に使う、述語で決まる値の分類と op の分類（別の名前空間のものを含む）。分類の FQN の辞書順 | 分類の名前 | `<C>Cases`（別の名前空間のものは修飾する） |
| 4 | 単位の対象の配線が呼ぶ別の名前空間。FQN の辞書順 | 名前空間の FQN のセグメントをつないだもの（`store`、`shopAdmin`） | `<M>Ops` |
| 5 | 継ぎ目（`Seams` があるとき。「継ぎ目」） | `seams` | `Seams` |

- 引数の名前は、先頭を小文字にする（名前空間のセグメントは、二つ目から先頭大文字にしてつなぐ）。Go の予約語か事前宣言の識別子なら末尾に `_`（名前空間 `type` は `type_`）。`ops` も末尾に `_`（`New` の本体が、構造体 `ops` のリテラルを書く。状態 `ops` や、別の名前空間 `ops` の引数は `ops_`）。分けた出力（「内側のフォルダと入口」）では、`inner` も末尾に `_`（入口の `New` が、内側のパッケージを `inner` の名前で呼ぶ）。先に並んだ引数の名前と当たれば、当たらなくなるまで末尾に `_` を足す。構造体 `ops` のフィールドは、引数と同じ名前、同じ順。
- 別の名前空間を辞書順にするのは、配線の本体を書き換えただけで並びが変わらないようにするため。状態は宣言順なので、仕様で宣言を入れ替えると、`New` を呼ぶ行が変わる。
- 継ぎ目を最後に置くのは、継ぎ目を宣言するトランスパイラを単位に足し引きしても、ほかの引数の位置が動かないようにするため。
- 引数の型が同じ二つの状態（同じ形、同じ要素の型で、どちらにも `read` も効果もない）は、入れ替えて渡してもコンパイルが通る。位置引数と引き換えに受け入れた弱さ。
- `New` のコメントの一行目は `// New はこの名前空間の Ops を組み立てる。引数の並びは、<句>。` で、句は、ある引数の種類を、この順に「、」でつないだもの: `leaves`、`状態の単位（宣言順）`、`分類の述語（FQN の辞書順）`、`別の名前空間（名前の辞書順）`、`seams`。二行目は `// 引数が nil かどうかは確かめない。`。引数が一つもなければ、`// New はこの名前空間の Ops を組み立てる。` の一行だけ。

**構造体 `ops`** は、N の op のすべてをメソッドに持つ（公開の名前）。配線のメソッドは `wiring.go`、それ以外は `ops.go` に、宣言順で置く。配線でないメソッドは、実装を呼ぶだけ。継ぎ目を持つ op は、公開の名前のメソッドが外側になり、下の表の中身は、その内側に入る（「継ぎ目」）。

| 置き場 | メソッドの中身 |
|---|---|
| `Leaves` | `return o.leaves.<Op>(<引数>)`（op が `ctx` を取るなら、最初に `ctx`） |
| `never` を返す op | `return <例外の型>{}` |

戻り値のないメソッド（値も `error` もない）は、`return` を書かずに呼ぶだけ。`ops` の、配線でないメソッドには、コメントを書かない（継ぎ目を持つ op の外側と中身は、一行のコメントを持つ。「コメント」）。

## 配線の本体

単位の対象の配線ごとに、`wiring.go` に `ops` のメソッドを一つ出す。コメントは `Ops` のメソッドと同じ（「コメント」）。実行の順序は行の順で、並列にはしない。継ぎ目を持つ配線は、このメソッドの名前が中身の名前（`loginBody`）になり、コメントが一行になる。本体の規則は変わらない（「継ぎ目」）。

```go
func (o *ops) Authenticate(ctx context.Context, token *pkgtype.Token) (pkgtype.Session, error) {
	session, err := o.store.FindSession(ctx, token)
	if err != nil {
		return pkgtype.Session{}, err
	}
	// match session by store.Login
	if session != nil {
		// LoggedIn: session は type.Session
		return o.RequireFresh(*session)
	}
	// default
	return pkgtype.Session{}, pkgtype.Unauthorized{}
}
```

### 呼び出し

呼び出し（`Call`）の先は、op か、`read` か、効果（`Call.callee`。`docs/ir.md`「配線」）。どれも、同じ形のメソッドの呼び出しになる。

| 呼ぶもの（`Call.callee`） | Go |
|---|---|
| 同じ名前空間の op（葉でも配線でも、`export` の有無によらず） | `o.<Op>(<引数>)`。実装がどの口にあるか、継ぎ目を持つかは、書き方を変えない（継ぎ目を持つ op は、同じ名前空間の配線から呼んでも継ぎ目を通る） |
| 別の名前空間の op | `o.<名前空間の引数名>.<Op>(<引数>)` |
| `read`（`userByEmail(email)`） | `o.<読む状態の引数名>.<Read>(ctx, <引数>)`（`o.users.UserByEmail(ctx, email)`） |
| 効果（`createOrder(order);`） | `o.<宛先の単位の引数名>.<Effect>(ctx, <引数>)`（`o.orders.CreateOrder(ctx, order)`） |
| 直に呼ぶ組み込みの効果（`users.post(user);`） | `o.<状態の引数名>.<操作>(ctx[, <値>])`（`o.users.Post(ctx, user)`、`o.token.Delete(ctx)`） |

- 呼ぶ op が `ctx` を取るなら、最初の引数に `ctx` を渡す。`read` と効果は、いつも `ctx` を取る。引数は、呼ぶ先の引数（`Op.params`、`Read.params`、`Effect.params`）の順に、`Call.args` の値（「値」）。渡されていない省略可能な引数は `nil`。`bind` で埋まる位置は「bind で埋まる位置」（op の呼び出しだけ）。
- `read` と効果を呼べるのは、宣言した名前空間の配線だけ（NS-6。`docs/check.md`「配線からの呼び出し」）なので、呼ぶ先は、いつも N の状態の単位。単位は「状態の単位」の表のとおり。
- 状態を名前で読むこと（名前を付けない `read`）は、呼び出しではなく、値（「状態の参照」）。
- **失敗の扱い**: `error` を返すものの呼び出し（`read` と効果は、いつも）は、すぐ後に `if err != nil { return <零値>, err }` を置く（`<零値>` は、この配線の戻り値の型の零値。戻り値がなければ `return err`）。`error` を返さない op の呼び出しには置かない。
- **零値**: ポインタ・インターフェース・スライス・マップは `nil`、構造体は `<型>{}`、`time.Time` は `time.Time{}`、文字列が元の型は `""`、数が元の型は `0`、真偽値は `false`。

### 値

`Atom` を Go の式にする規則。「当てはめる型」は、その値を渡す位置の型（引数の型、フィールドの型、戻り値の型、`match` の型）。

| `Atom` | Go |
|---|---|
| `name`（束縛か引数。パスなし） | その局所の名前 |
| `name` のパス（`in.body.email`） | フィールドを先頭大文字にしてつなぐ（`in.Body.Email`）。途中に省略可能な model があるときは「省略可能を通るパス」 |
| `state` | その文の前で読んだ値（「状態の参照」）と、パス |
| `literal` | 文字列は Go の文字列リテラル（`strconv.Quote`）、整数は 10 進、小数は `strconv.FormatFloat(v, 'g', -1, 64)`（`.` も `e` も含まなければ `.0` を足す）、真偽値は `true` / `false`、`Duration` は `time.Duration(<ナノ秒>)` |
| `none` | `nil` |
| `record` | 当てはめる model の構造体リテラル `<型>{<フィールド>: <値>, …}`。並びは model のフィールドの順。書かれていないフィールド（省略可能なフィールドだけ。`docs/types.md`「当てはまる」）は書かず、零値のまま。当てはめる型が省略可能なら `&<型>{…}` |

- **絞り込まれた対象**（`match` のアームの中の、対象と同じ `Atom`。`docs/types.md`「アームの中の対象の型」）は、「match」の「絞り込んだ値」の式に置き換えてから、上の表を当てる。
- **型をそろえる**: 値の型が、当てはめる型と Go で違うとき（`docs/types.md`「互換」の規則で渡せる組）は、次のように包む。

| 値の型 → 当てはめる型 | Go |
|---|---|
| T → `optional(T)` で、Go の型が `*T` | `ptr(<式>)`。リテラルは `ptr(<T の Go の型>(<リテラル>))` |
| T → `optional(T)` で、Go の型がそのまま（`array`、`record`、union） | そのまま |
| scalar → その元の型（規則 11） | `<元の型>(<式>)` |
| `array` / `record` で、要素の型をそろえる必要があるもの | 同じ長さのスライス（同じキーのマップ）を作り、要素ごとにそろえて写すループを、その文の前に置く。値が `nil` なら `nil` のまま |
| `case(…)` → `of`、`case` どうし | そのまま（Go では同じ型） |

- `ptr` は `func ptr[T any](v T) *T { return &v }`。`wiring.go` が使っていれば、その末尾に一つだけ出す（「ファイル」）。
- **省略可能を通るパス**: パスの途中の値の型が `optional(model)`（Go ではポインタ）で、その先のフィールドを読むとき、式の型は `optional(<フィールドの型>)` になる（`docs/types.md`「式の型」）。その文の前に、途中の値を作る。`<x>` はそこまでの式、`<F>` は残りのパス。フィールドの型が既に `nil` を持てる型なら `v<n> = <x>.<F>`。残りのパスが、さらに省略可能な model を通るなら、同じ形を入れ子にする。

```go
var v0 <optional(フィールドの型) の Go の型>
if <x> != nil {
	v0 = ptr(<x>.<F>)
}
```

- 途中の値の名前は `v0`、`v1`、…（メソッドの中の通し番号）。引数名、束縛名、import するパッケージの名前に当たる番号は、飛ばす（分類の判定の結果 `k0`、`k1`、… も同じ）。

### 文

| 文 | Go |
|---|---|
| `const x = 呼び出し`（op か `read`） | `x, err := <呼び出し>` と失敗の扱い。`error` を返さない op なら `x := <呼び出し>`。下の行で使っていない束縛（WIRE-8）は、直後に `_ = x` を足す |
| `const x = 値` | `x := <値>`（当てはめる型は、値の型） |
| `呼び出し;`（op か効果） | `if err := <呼び出し>; err != nil { return <零値>, err }`。`error` を返さない op なら `<呼び出し>` だけ |
| `return 呼び出し`（op か `read`） | 呼ぶ先の戻り値の並びが、この配線と同じで、型をそろえる必要がなければ `return <呼び出し>`（`return o.users.UserByEmail(ctx, email)`）。呼ぶ op だけが `error` を返さないなら `return <呼び出し>, nil`。型をそろえる必要があれば、`v<n>` に束縛してから `return <そろえた値>, nil` |
| `return 値` | `return <値>, nil`（`error` を返さない配線なら `return <値>`） |
| `const x = match …` / `return match …` / `match …;` | 「match」 |
| 「返らない」呼び出し（`never` を返す op）の文と `return` | `return <零値>, <呼び出し>`（戻り値がなければ `return <呼び出し>`） |
| 本体の終わり（戻り値のない配線） | `error` を返す配線なら、最後に `return nil`。最後の文が「返らない」なら足さない |

束縛を二度目以降に作るときも `:=` を使う（`err` は同じ変数が使い回される）。

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

`match` は、分類の種類ごとの**分け方**と、使われ方（`return` / `const` / 文）ごとの**アームの本体**の組み合わせで書く。どの形でも、分ける前に `// match <対象> by <分類>` の一行を置く（`<対象>` は仕様に書いたままのパス、`<分類>` は FQN、標準の分類なら `list` / `nullable`。状態の分類での `match` は `// match by <分類>`）。

**分け方**:

| 分類 | 条件 |
|---|---|
| 標準の分類 `nullable`、型で決まる分類のうち絞り込みが `presence` のもの | `if <対象> != nil`。この枝が、値のある場合（`HasItems`、名前の付いたケース）。もう一方が `Empty` / `default` |
| 標準の分類 `list` | `if len(<対象>) != 0`。この枝が `HasItems`、もう一方が `Empty` |
| 型で決まる分類のうち絞り込みが `variant` のもの | `switch <対象>.(type)`。ケースごとに `case <アームの Go の型>:`（アームを包む型なら、その包む型）。`default` のアームは `default:` |
| 述語で決まる値の分類・op の分類 | `k<n>, err := <パッケージ>.Classify<C>(o.<分類の引数名>, <対象>)` と失敗の扱いの後、`switch k<n>`。ケースごとに `case <C><ケース名>:`、`default` のアームは `case <C>Default:` |
| 述語で決まる状態の分類 | `k<n>, err := Classify<C>(ctx, o.<分類の引数名>)` と失敗の扱いの後、`switch k<n>`。上と同じ |
| 型で決まる状態の分類 | 対象の状態を「状態の参照」で読んでから、その値を対象にして、上の `presence` か `variant` の分け方 |

- 対象が状態なら、先に「状態の参照」で読む。対象が名前でない `Atom`（リテラル、レコードリテラル、`none`）なら、`v<n>` に束縛してから分ける。
- **二つに分ける形**（`if`）は、アームを書いた順によらず、値のある場合を `if` の枝にする。`return match` では、`else` を書かず、もう一方をそのまま後ろに続ける（どの枝も `return` で終わる）。それ以外では `if … { … } else { … }`。枝の中身がないとき（文の `match` の `none` のアーム）は、その枝を書かない（`if` の枝がなければ、条件を逆にして `if <対象> == nil` / `if len(<対象>) == 0` にする）。一つのアームが両方のケースを並べていれば（`Empty | HasItems => f();`）、分けずに、そのアームの本体だけを書く。
- **`switch` の形**は、アームごとに `case` を一つ。並びは、アームの最初のケースの、分類の宣言での順。`default` のアームは最後。複数のケースを並べたアームは `case A, B:`。`return match` で、`default` の節を持たない `switch` の後ろには、`return <零値>, &runtime.ClassificationError{Classify: "<分類の FQN>"}` を置く（判定関数が先に同じ誤りを返すので、ここには来ない。Go が、関数の終わりの `return` を求めるため）。
- それぞれの枝（`if` の枝、後ろに続く部分、`else` の枝）の頭に、ケースの名前をコメントで書く: `// <ケース名>`（複数なら ` | ` でつなぐ。`default` は `default`）。対象が絞り込まれるアームは `// <ケース名>: <対象> は <型>`（型の書き方は「コメント」の「型の表示」）。`switch` の節には書かない（`case` が名前を持つ）。

**絞り込んだ値**（アームの中で、対象と同じ `Atom` を値として使う位置の式。フィールドを読むだけなら、Go がポインタを辿るので、元の式のまま）:

| 絞り込みの種類（`CaseTyping.by`、標準の分類は `presence`） | 式 |
|---|---|
| `presence` | `*<対象>`（対象の Go の型がポインタのとき。`array`、`record`、union はそのまま） |
| `variant` | `<対象>.(<アームの Go の型>)`。アームを包む型なら `<対象>.(<包む型>).Value` |
| `predicate` | 対象の型から絞り込んだ型へ: 省略可能を外すなら `*<対象>`、Go の型が違えば `<絞り込んだ型>(<…>)`（`string` を `Email` に: `Email(<対象>)`） |

- 複数のケースを並べたアームで、対象の型が変わらないもの（`docs/types.md`「アームの中の対象の型」の 1、2、4、5）は、何も置き換えない。
- `variant` の絞り込みで、`as` の型と等しい型のアームを union が二つ以上持つなら GO-6（どのアームかを、型で分けられない）。

**アームの本体**:

| 本体 | `return match` | `const x = match` | 文の `match` |
|---|---|---|---|
| 呼び出し（`error` を返す） | `return <呼び出し>`。型をそろえる必要があれば、束縛してから `return <そろえた値>, nil` | `v<n>, err := <呼び出し>` と失敗の扱い、`x = <そろえた値>` | `if err := <呼び出し>; err != nil { return <零値>, err }` |
| 呼び出し（`error` を返さない） | `return <呼び出し>, nil`（配線も返さないなら `return <呼び出し>`） | `x = <呼び出し>` | `<呼び出し>` |
| 「返らない」呼び出し | `return <零値>, <呼び出し>` | 同じ | 同じ |
| 値 | `return <値>, nil` | `x = <値>` | —（書けない。WIRE-2） |
| `none` | `return nil, nil` | 何も書かない（`x` は零値の `nil` のまま） | 何も書かない |
| `throw E` | `return <零値>, <E の型>{}` | 同じ | 同じ |
| `throw E(<値>, …)` | `return <零値>, <E の型>{<F>: <値>, …}`。`<F>` は例外の引数のフィールド名（「exception」）で、並びは `Exception.params` の順。値は、引数の型を当てはめる型にして「値」の規則で書く | 同じ | 同じ |

- `const x = match` は、分ける前に `var x <match の型>`（`docs/types.md`「match の型」）を置く。アームの値は、その型にそろえる。束縛が下の行で使われていなければ、`match` の後ろに `_ = x`。
- アームの呼び出しの `err` は、その枝の中の変数（`:=`）で、失敗の扱いも枝の中に置く。
- 例外は値で作る（「exception」）。`throw` の引数（`MatchArm.body` の `args`）は、数も型も、検査が例外の宣言と照らしてある（`docs/check.md`「throw」）ので、フィールドは、いつもすべて書く。

### 分類

- **型で決まる分類**（`Classify.decidedBy` が `type`。標準の分類を含む）について、基底は何も出さない: 列挙の型も、判定関数も、`New` の引数もない。どのケースに当たるかは、値から決まる（「match」の `presence` と `variant`）。分類の名前は、それを使う配線のコメントにだけ残る。
- **述語で決まる値の分類・op の分類** C は、宣言した名前空間の定義のパッケージの `classify.go` に、次を出す。述語は、人が実装する（排他になるように、一人がまとめて書く）。

```go
// WarrantyKind は保証の種類
type WarrantyKind int

const (
	// 標準の保証
	WarrantyKindStandard WarrantyKind = iota
	WarrantyKindYamada
	// hasDefault なら、最後に WarrantyKindDefault
)

// WarrantyKindCases は分類 shop.WarrantyKind のケースごとの述語。人が実装し、この分類を match に使う名前空間の New に渡す。
type WarrantyKindCases interface {
	// Standard はケース Standard の述語。
	Standard(v Item) bool
	Yamada(v Item) bool
}

// ClassifyWarrantyKind は v を shop.WarrantyKind で分類する。すべての述語を呼び、真になった数を数える。
func ClassifyWarrantyKind(c WarrantyKindCases, v Item) (WarrantyKind, error)
```

- 述語の引数は、分類の対象の型（値の分類は `subject` の型、op の分類はその op の `returns`）。値だけを見るので、`ctx` を取らず、`error` を返さない。
- 判定関数は、すべての述語を宣言順に呼び、真の数を数える。`hasDefault` が false なら、真がちょうど一つでなければ、true なら、真が二つ以上なら、`*runtime.ClassificationError{Classify: "<FQN>", Hits: <真になったケース名>}` を返す。真が一つならそのケース、0 で `hasDefault` なら `<C>Default`。
- 型で決まる絞り込み（`presence` / `variant`）の `as` を持つケースは、値がその型に当たらなければ、述語を呼ばずに偽とする（`docs/check.md`「ケースの判定」）。
- **判定の誤りを返す先がない配線**（純粋で、例外を持たない配線は `error` を返さない。「op の形」）が、述語で決まる分類で `match` するときは、判定関数の誤りで `panic` する: `if err != nil { panic(err) }`。`return match` で `default` の節を持たない `switch` の後ろ（「match」）も、`panic(&runtime.ClassificationError{Classify: "<分類の FQN>"})`。述語が排他でない・全集合でないのは実装の誤りで、仕様はこの配線に失敗を許していない。
- 述語の実装は、その分類を `match` に使う名前空間の `New` が、引数で受け取る（「New」の 3）。分類を宣言した名前空間の `New` が受け取るのは、その名前空間の配線が使うときだけ。
- **述語で決まる状態の分類** C は、`classify.go` に列挙の型と定数と、判定関数 `func Classify<C>(ctx context.Context, s <C>State) (<C>, error)` を出す。述語は状態の単位 `<C>State` のメソッド（「状態の単位」）。判定関数は、すべての述語を呼び（述語がエラーを返したら、残りも呼んだうえで最初のエラーを返す）、真の数を上と同じ規則で確かめる。配線が `match by C` に使っていなくても出す（テストが、効果の結果の判定に使う）。

## デコレータ

**デコレータの適用が何を出力するかは、そのデコレータを読むトランスパイラが決める。** 基底は語彙を知らないので、デコレータの名前からも、宣言の節（`throws`、`catches … recovers` / `halts` / `ignores`）からも、何も生成しない。デコレータが基底の出力に効くのは、op の純粋さ（付いていれば純粋でなく、`ctx` を取り、`error` を返す）、検査が計算した `Throws:` などのコメント、単位のトランスパイラが宣言した継ぎ目（「継ぎ目」）の三つだけ。

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

- デコレータの付いた op は純粋でない（`docs/check.md`「純粋」）ので、継ぎ目のメソッドも `body` も、いつも `ctx` を取り、`error` を返す。
- `body` の関数の型には、引数の名前も書く（op の引数の局所の名前）。`body` という名前が op の引数の局所の名前と当たれば、当たらなくなるまで、`body` の方の末尾に `_` を足す（「名前」の、生成したコードが作る名前の規則）。
- 基底は、デコレータの引数（`Application.args`）を継ぎ目に渡さない。中身を出すトランスパイラが `View` から読んで、自分のコードに書く。

### 外側

継ぎ目を持つ op の、`ops` の公開のメソッド `<Op>`（`Ops` を満たすメソッド）。`ops.go` の、配線でないメソッドの並びの、その op の位置に置く。形は op の形のまま。

- 継ぎ目が一つなら `return o.seams.<Op><Decorator>(ctx, <引数>, <中身の式>)`。
- **継ぎ目が二つ以上なら、入れ子にする。順序は、言語のデコレータの適用順（DEC-9: op に近いものから）**: `Op.decorators` の最後（op に最も近い）のものが最も内側で、中身を直に受け取る。先に書かれたものほど外側。内側は、同じ形の関数リテラルにして、外側の継ぎ目の `body` に渡す。継ぎ目を持たないデコレータは、間にあっても層にならない。

```go
// @trace.span
// @rate.limited
// export op foo(in: FooInput): FooOk { … }

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
// 純粋（ctx を取らない）。失敗しない（error を返さない）
```

- 説明がなければ、一行目は `// <メソッド名> は op <FQN>。`。
- `引数:` の段は、`@param` があるときだけ。`Throws:` / `Reads:` / `Effects:` の行は、それぞれ `Op.signature` の `exceptions` / `reads` / `effects` が空でないときだけで、並びは、その列の順。区切りは、仕様の節と同じ `,`（と空白一つ）。
- **`Reads:` に並ぶのは、読む基底の状態**（`Signature.reads`）: 名前を付けた `read` を呼んでも、状態を名前で読んでも、その状態の FQN（`Reads: store.users`。どの `read` かは書かない。読む状態は、書き方によらない: `docs/check.md`「節に書く要素」）。
- **`Effects:` に並ぶのは、効果**（`Signature.effects` の `EffectRef`）: 宣言した効果は、その FQN（`store.createOrder`）。直に呼んだ組み込みの効果は、`<状態の FQN>.<操作>`（`store.users.post`）。最後の行は、純粋な op だけで、`error` を返すなら `// 純粋（ctx を取らない）`。
- 説明の段と次の段の間は `//` の一行で空ける。`Throws:` から最後の行までは、空けずに続ける。説明のほかに何もなければ、説明だけ。
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
| 効果 | `// <メソッド名> は命令（効果 <効果の FQN>、on <宛先>）: <説明>`。説明がなければ `// <メソッド名> は命令（効果 <効果の FQN>、on <宛先>）。`。宛先は、組み込みの効果なら `<状態の FQN>.<post / put / delete>`、遷移なら `<宛先の FQN>: <前> -> <後>` |
| ケースの述語 | `// <ケース名> は問い合わせ: ケース <ケース名> の述語。` |

- `read` と効果の説明（`description.body`）を、単位のメソッドのコメントに写すのは、デコレータのない `read` と効果の中身を人が書くとき、何をするものかを言うのが、説明だけだから。説明が二行以上なら、二行目から先は、行ごとに `// ` を付けて続ける。「は」の後ろの空白の規則（この節の頭）は、ここでは使わない（説明の前は `: `）。

状態の単位の型のコメントは、基底の状態なら `// <型名> は状態 <FQN>（<形の語><<要素の型の表示>>）の単位。実装は New の引数 <引数名> に渡す。` と `// メソッドが一つでも足りない値は、渡すところでコンパイルできない。` の二行。状態の分類なら、一行目が `// <型名> は状態の分類 <FQN>（<対象の状態の FQN を「、」で> を対象にした classify）の単位。実装は New の引数 <引数名> に渡す。`。

**型の表示**（コメントの中の型）: 核の型は名前、`ref` は FQN、`optional(T)` は `T?`、`array(T)` は `T[]`、`record(K, V)` は `Record<K, V>`、`case` はその `of`。

## runtime パッケージ

生成したコードが import する `github.com/mew-ton/hozo/runtime`。基底の出力が使うのは次の三つ。

```go
type Effect string
type Exception interface { error; HozoException() string }
type ClassificationError struct { Classify string; Hits []string }   // 述語で決まる分類の判定の誤り。Error() を持つ
```

- `Effect` と `Exception` は `types.go`、`ClassificationError` は `classify.go` と `wiring.go` が使う。`ops.go` と `states.go` は `runtime` を import しない。
- `MissingError` はなくした（「実装を受け取る口」）。
- `runtime` には、標準の相乗りが使う型も置く: `Violation`、`ValidationError`、`CheckFormat` は `go-valid` のもの（持ち主は `docs/plugin-go-valid.md`「runtime」）。`DBTxKey`（`ctx` が運ぶ DB のトランザクションの鍵。`struct { DB *sql.DB }`）は `go-sqlite` のもの（持ち主は `docs/plugin-go-sqlite.md`「トランザクション」）。Go の出力が import するライブラリを一つにするため。`DBTxKey` は、それに加えて、同じ DB を使う二つの名前空間の出力が、同じ鍵を使うため（出力のパッケージごとの型では、共有できない）。
- API を変えるときは、生成したコードのヘッダに書く版（「ファイル」）と合わせて互換を保つ。

## 決定性

- 同じ `View` から、同じバイト列を返す（`docs/plugin-protocol.md`「emit」）。読むのは `View` と、import パスを求めるための `go.mod` だけ。時刻、乱数、環境（`HOZO_PROJECT_ROOT` を除く）、マップの走査の順に依らない。
- 並びは、この文書に書いた順（宣言順、FQN の辞書順）だけで決まる。
- 応答の `files` は、パスの辞書順。
- 同じ名前空間の二つの単位の基底のファイルは、`in` と単位の継ぎ目が同じなら、import パスに現れる自分のディレクトリを除いて、バイト単位で同じ。単位に並べたほかのトランスパイラが基底の出力を変えるのは、マニフェストで宣言した継ぎ目（`View.config.seams`）を通してだけで、そのオプションは変えない。

## 診断

`emit` の応答の `errors` で返す（どれも error の診断になり、`CheckFailed`。`docs/plugin-protocol.md`「emit」）。位置は、表に書いた宣言の FQN。

| ID | 重さ | 意味 |
|---|---|---|
| GO-1 | error | Dir(U, N) の上に `go.mod` がない（「パッケージと import パス」）。位置なし |
| GO-2 | error | 生成する名前が当たる（「名前」）。当たった二つの宣言をメッセージに示す。位置は後ろの宣言 |
| GO-3 | error | 出力どうしの import が循環する（「パッケージと import パス」）。輪に入る出力を、単位（何番目の `[[emit]]` か）と名前空間でメッセージに示す。位置なし。輪に入る出力ごとに出る |
| GO-4 | error | 内側のフォルダ `inner` が、同じ単位が出力する子の名前空間 `N.inner` の出力先と重なる（「内側のフォルダと入口」）。位置は名前空間の最初の内部の op |
| GO-5 | error | `[emit.options.go]` に、知らないキーがある（「マニフェスト」）。位置なし |
| GO-6 | error | `variant` の絞り込みの `as` の型と等しい型のアームが、union に二つ以上ある（「match」）。位置は分類 |
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
| 生成するコードは `context` を取らない | 純粋でない op は `ctx context.Context` を取る。純粋で例外のない op は `error` を返さない | 72 |
| 実装は、関数のフィールドを持つ構造体（`Leaves`、`Deps`、`Wrappers`、`Classifiers`）と、パッケージの変数 `States` | インターフェースと、`New` の位置引数。未設定の `*runtime.MissingError` はない | 73 |
| `Ops` はすべての op | 入口の `Ops` は `export` の op だけ。内部の op は内側のフォルダから | 74 |
| 別の名前空間への呼び出しの `bind` の位置は、`Deps` を埋める者が埋める | 配線が埋める | 79 |
| `wraps` で引き受けたデコレータに、包む関数を生成する | デコレータの出力は、読むトランスパイラが決める。基底が出すのは、単位のトランスパイラがマニフェストで宣言した継ぎ目（二層と `Seams`）だけ。読む者がなければ何も出さない | 84、85、86（78 を置き換えた） |
| `match s by list` / `by nullable` は `IsEmpty` を呼ぶ | 状態は `Get` で読み、値として分ける | 72（状態は名前で書いた値。S21） |
| union を `optional` にすると `*<Union>` | インターフェースのまま（`nil` が「なし」） | 69 |
| 状態に触れる葉の op を、読む状態と効果で、状態の単位か、複数の状態にまたがる口 `SharedState` に置く | 状態に触れるのは `read` と効果で、どちらも状態の単位のメソッド。葉の op は `Leaves` だけ。`SharedState` はない | 139（100 を置き換えた） |
| 値を運ぶ例外は `struct{ Value <型> }` | 引数ごとにフィールド一つ | 139 |
