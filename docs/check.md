# 検査

## この文書が決めること

`hozoc check` と `hozoc lsp` が IR（`docs/ir.md`）の上で行う計算と、規則ごとの検査の手続き・診断を決める。規則の ID は `docs/requirements.md`「検査の規則の一覧」のもので、規則の意味の持ち主は設計書の節。ここでは手続きだけを書く。設計書にない規則はこの文書の「診断」の各表が持ち主。名前の解決（`RES-*`）と型（`TYPE-*`、`LIT-1`）はそれぞれ `docs/resolve.md`「診断」、`docs/types.md`「診断」。

検査は IR が作れた後に行う。IR を作る途中（解析、解決、正規化）の誤りがあるファイルでも、作れた部分には検査を行う（`docs/requirements.md`「言語サーバー」）。一つの検査で見つかる誤りはすべて報告する。

## 状態の形

設計書 §5.2 の状態の形を、状態を分類で表す決定と、形と型を揃える決定（`docs/decisions.md`「設計書を変える決定」S1〜S9、S21）に合わせて定める。

### 基底の状態

基底の状態は、名前空間が持つ値。型は宣言に書いた型（`State.type`。形の語と `?` / `[]` のどちらで書いても同じ。`docs/types.md`「形の書き方」）で、**形**はその型の最も外側から決まる。形は列数の範囲を表し、形ごとに組み込みの分類と組み込みの効果が決まる。

| 状態の型 | 形（`State.shape`） | 要素（`State.element`） | 列数 | 組み込みの分類 | 組み込みの効果 |
|---|---|---|---|---|---|
| `array(T)`（`list<T>`、`T[]`） | `list` | T | 0 以上 | `Empty` / `HasItems` | `post` / `put` / `delete` |
| `optional(T)`（`nullable<T>`、`T?`） | `nullable` | T | 0 か 1 | `Empty` / `HasItems` | `post` / `put` / `delete` |
| それ以外（`single<T>`、`T`） | `single` | その型 | ちょうど 1 | なし（常に一列） | `put` |

- 形を決めるのは最も外側だけ: `T[]?` は `nullable`（要素は `T[]`）、`T?[]` は `list`。`list` の要素は `optional` にできない（STATE-3）。
- 組み込みの分類は、形の型についての標準の分類（「分類」の「標準の分類」）を、その状態に当てたもの。
- 状態は、持ち主の名前空間の配線の中で、名前で値として読める（「op の宣言と本体」の「状態の参照」）。

組み込みの効果は列数の相対的な変化。`post` は増える、`put` は変わらない、`delete` は減る。組み込みの分類をどう変えるかは形から決まる。

| 形 | `post` | `put` | `delete` |
|---|---|---|---|
| `list` | どの場合でも → `HasItems` | `HasItems` → `HasItems` | `HasItems` → `Empty` か `HasItems` |
| `nullable` | `Empty` → `HasItems` | `HasItems` → `HasItems` | `HasItems` → `Empty` |

### 状態の分類

`classify` は、一つ以上の基底の状態を対象にして、その場合に名前を付ける。これを**状態の分類**と呼ぶ（型を対象にした `classify` は値の分類、op を対象にしたものは op の分類。「分類」）。状態の分類は、基底の状態と並ぶもう一つの「状態」でもある: 効果の宛先になり（「効果の宛先」）、配線が `match by C` で読み、Go の基底の出力では、述語で決まるものが、基底の状態と同じく状態の単位になる（`docs/plugin-go.md`「状態の単位」）。基底の状態と並べて言うときも「状態の分類」と呼び、別の言葉（「分類の状態」）は使わない。値の分類（§10.1）と同じく、名前と説明を宣言し、判定は述語として人が実装する（型で決まる分類を除く。「分類」の「ケースの判定」）。排他・全集合・`default` も同じ。対象にできるのは宣言した名前空間の基底の状態だけで（CLS-3）、状態の分類を対象にはできない。効果が書き込む状態を宣言から直接読めるようにするため。

### 効果の宛先

一つの効果が変えるのは、一つの状態の一つの変化だけ。宛先は、基底の状態の組み込みの効果か、分類の遷移（`docs/grammar.md`「exception / effect」）。実装が二つの基底の状態を同時に変える効果は、その二つを対象にした状態の分類を宣言し、その遷移として書く。

**効果が書き込む状態**（`Effect.writes`）は、組み込みの効果ならその基底の状態、遷移なら、宛先が状態の分類ならその対象のすべての基底の状態、宛先が基底の状態ならその状態。

### 読むことの宛先

`read` の宣言（`docs/grammar.md`「read」）は、基底の状態一つを読むことに名前を付ける。宛先（`Read.state`）は、宣言した名前空間の基底の状態一つ。別の名前空間の状態なら RES-5、状態でないもの（状態の分類を含む）なら RES-4（`docs/resolve.md`「位置ごとに受け付ける種類」）。状態の分類を宛先にできないのは、分類を読むのが配線の `match by C` で、葉の op が読むのは基底の状態だから。

`read` の宣言そのものは、何も読まない。op の `reads` の要素（「直に書く要素」）か、デコレータの `any read` の引数（`docs/types.md`「デコレータの引数」）になって、op が読むものを指す。どの op も指していない `read` は診断しない（どの op も持たない効果を診断しないのと同じ）。

### 直に書く要素

op の `reads` と `effects` の要素は、宣言の名前でも、直にでも書ける（`docs/grammar.md`「op」）。デコレータの宣言の `reads` / `effects` の節の要素も同じ（「計算」の「デコレータが運ぶ reads と effects」）。

| 節 | 名前で書く（`named`） | 直に書く（`direct`） | 直に書いたものが指すもの |
|---|---|---|---|
| `reads` | `read` の宣言の名前（`reads userByEmail`） | 基底の状態の名前（`reads users`） | その状態を読むこと |
| `effects` | `effect` の宣言の名前か、効果のユニオンの名前（`effects addUser`） | `状態.操作`（`effects users.post`） | その状態の組み込みの効果（「基底の状態」の表） |

- 直に書いた要素は、同じ宛先を持つ宣言と同じ規則に従う: 操作がその状態の形になければ STATE-1、葉の op が別の名前空間の状態に書けば STATE-2、読めば READS-1。直に書いた効果が書き込む状態は、その状態。
- **効果は、書き方ごとに別のもの**として数える。直に書いた `users.post` は、状態 `users` の組み込みの効果 `post` そのもので、同じ宛先を持つ宣言（`effect addUser on users.post;`）とは別の効果。宣言した効果は、宛先が同じでも、宣言ごとに別。効果を数えるところ（Sig の F と rel、EFF-2、EFF-3）は、どれもこの数え方に従う。
- **読む状態は、書き方によらない。** 読む状態（「計算」の「reads」の R）は基底の状態の集合で、`read` の宣言は、その宛先の状態として数える。`reads userByEmail`（`read userByEmail on users;`）と `reads users` は、R に同じ `users` を足す。
- 分類の遷移は、直には書けない（`effects Checkout` は RES-4）。宣言した `effect` の宛先にだけ書ける（「効果の宛先」）。
- 一つの節に、名前で書いた要素と直に書いた要素を混ぜてよい。IR は、要素ごとに、どちらで書いたかを持つ（`docs/ir.md`「op」の `ReadRef`、`EffectRef`）。

### 検査器が遷移を使う範囲

検査器は、遷移のケースの所属（STATE-4）を確かめ、IR に載せるだけ。配線の行をまたいで状態の場合を追うこと（効果の後の `match` で到達しないアームの検出など）はしない。§7.3 の「不明」と絡むので、要るようになってから別に決める。例外と状態の場合を結びつける書き方（`AlreadyLoggedIn` は前の場合が合わなかったことを表す、など）も持たない（§16 の「状態 × 入力の表で分岐を網羅させる」に近づくため）。

リストの一部（「このユーザーのカート」）の場合は状態の分類では言えない。読む op（「計算」の「reads」）で値を取ってから、値の分類か op の分類で書く。

## 分類

`classify`（と糖衣の `condition`）の対象、置き場、ケースの型、ケースの判定を定める。設計書 §10.1〜§10.3 に、`docs/decisions.md`「設計書を変える決定」の S3、S16、S18、S21 を当てたもの。構文は `docs/grammar.md`「classify / condition」、型の規則は `docs/types.md`「絞り込み」。

### 対象

| 分類 | 対象（`Classify.subject`） | 対象の型 | `match` |
|---|---|---|---|
| 値の分類 | 型一つ | その型 | `match x by C` |
| **op の分類** | op 一つ | その op の `returns` | `match x by C` |
| 状態の分類（「状態の形」） | 分類を宣言した名前空間の基底の状態一つ以上 | 状態ごとに、その状態の型 | `match by C` |

- op の分類は、その op が返した値の分類（`classify Registration of findUser { ... }`）。対象の型をその op の `returns` とした値の分類として扱う。違いは、何を分類しているかが宣言の名前で読めることと、`as` の前の書き方だけ。
- `match x by C` の `x` は、型が対象の型に渡せる値ならよい（MATCH-6）。その op の呼び出しの結果である必要はない。値の出どころは追わない（「検査器が遷移を使う範囲」と同じく、検査器は配線の行をまたいで値を追わない）。
- 対象にできる op は、値を返す op（`returns` が `null` でも `never` でもない）。そうでなければ CLS-4。どの名前空間の op でもよく、`export` の有無によらない（対象にすることは呼び出しではない。「op の宣言と本体」の「export」）。
- `C.Case<T>`（絞り込まれた型）を書けるのは、値の分類と op の分類。

### 置き場

分類は、どの名前空間にも宣言でき、値の分類と op の分類は、どの名前空間からも使える。別の名前空間のものは修飾して書く（`match user by store.Registration { ... }`、`store.Registration.Registered<type.User>`）。複数の担い手が共有する型の分類を、型と同じ名前空間にも、使う側の名前空間にも置ける。

状態の分類を `match by C` で読めるのは、その分類を宣言した名前空間の配線だけ。読むのは状態で、状態を読めるのは持ち主の名前空間だけだから（別の名前空間の状態の分類なら RES-5。`docs/resolve.md`「診断」）。

「`classify` は宣言した名前空間の中でだけ使える」（NS-4）はなくした（「診断」の「名前空間」）。

### 標準の分類

`nullable` と `list` は、形の型についての標準の分類。宣言を持たず、名前は `match` の `by` の後ろにだけ書ける。`default` はない。

| 名前 | 対象の型 | ケース | 判定 | アームでの絞り込み |
|---|---|---|---|---|
| `nullable` | `nullable<T>`（`T?`） | `Empty` / `HasItems` | 値がなければ `Empty`、あれば `HasItems` | `HasItems` だけのアームで T |
| `list` | `list<T>`（`T[]`） | `Empty` / `HasItems` | 要素が一つもなければ `Empty`、あれば `HasItems` | なし |

- 対象は値。状態に限らない（`match in.token by nullable { ... }`、`match order.lines by list { ... }`）。状態は、名前で書けば値になる（`match token by nullable { ... }`）。
- 基底の状態の「組み込みの分類」（「状態の形」）は、この分類をその状態に当てたもの。遷移の宛先（`effect clearCart on carts: HasItems -> Empty;`）になるのは、基底の状態に当てたものだけ（「効果の宛先」）。
- 自分で宣言する分類も、`nullable<T>` や `list<T>` を対象にできる（`classify Presence of nullable<User> { ... }`）。

### ケースの型

ケースは、そのケースのときに対象が何の型になるかを `対象 as T` で書ける（`Registered: findUser as type.User;`）。書かなければ今までどおり（`docs/types.md`「アームの中の対象の型」の 4）。

- `as` の前に書けるのは、その分類の対象: 値の分類なら `subject` の型と等しい型、op の分類ならその op、状態の分類なら対象の状態のどれか。それ以外を書けば CLS-5。一つのケースに同じ対象の `as` を二度書いても CLS-5。
- 状態の分類は、一つのケースに、対象の状態ごとの `as` を並べられる。値の分類と op の分類は対象が一つなので、`as` は一つ。
- `as` の後ろの T は、対象の型から絞り込める型でなければならない（`docs/types.md`「絞り込める型」。CLS-6）。
- 意味: そのケースが成り立つとき、対象は T の値である。`match` のそのケースのアームの中で、対象の型は T になる（`docs/types.md`「アームの中の対象の型」）。

### ケースの判定

分類のケースが何で決まるかは、宣言から二通りに分かれる（`Classify.decidedBy`）。

**型で決まる分類**（`type`）は、次のすべてを満たす分類。述語を持たず、どのケースに当たるかは値の型から決まる。

1. 対象が一つ（値の分類、op の分類、対象の状態が一つの状態の分類）。
2. 名前の付いたケースのすべてが、その対象の `as` を持ち、どれも型で決まる絞り込み（`docs/types.md`「型で決まる絞り込み」）。
3. `as` に書いた型が、ケースの間ですべて違う。
4. 絞り込みが `presence` なら `default` がある。`variant` なら、`default` があるか、書いた型がその union のすべてのアームの型を覆う。

当たるケースは、値が `as` の型に当たるケース。どれにも当たらなければ `default`。条件 3 と 4 から、どの値もちょうど一つのケースに当たる。`presence` の分類は、条件 2 と 3 から、名前の付いたケースが一つだけになる。

```
classify Registration of findUser {     // findUser(email: Email): User?
  Registered: findUser as User;         // 値があるとき
  default;                              // 値がないとき
}
```

標準の分類も型で決まる分類。

**述語で決まる分類**（`predicate`）は、それ以外のすべて。今までどおり、名前の付いたケースごとに述語を人が実装し、排他・全集合・`default` の規則（§10.1、§10.3）が掛かる。`as` を持つケースは、次の意味を足す。

- 述語が真なら、対象は T の値である。述語を実装する人がそれを保証する（`string` の対象に `Valid: string as Email;` と書いたケースの述語は、値が `Email` の制約を満たすときだけ真を返す）。
- `as` が型で決まる絞り込みなら、値がその型に当たらないとき、述語が真でもそのケースは成り立たない。

同じ型に絞り込むケースが二つあれば（`Fresh: findSession as Session; Expired: findSession as Session; default;`）、条件 3 を満たさないので述語で決まる分類になる。

## op の宣言と本体

`export`、`never` を返す op、アームの `throw`、状態の参照の規則。`docs/decisions.md`「設計書を変える決定」の S17、S19、S20、S23。構文は `docs/grammar.md`「op」「配線」。

### export

`export` を付けた op は、別の名前空間から呼べる。付けていない op は**内部の op**で、呼べるのは同じ名前空間の配線だけ。親と子の名前空間も、互いに別の名前空間（§3.2）。

別の名前空間の内部の op を呼べば NS-5。「呼ぶ」に当たるのは次の二つ。

| 呼び方 | 呼ぶ側の名前空間 |
|---|---|
| 配線の呼び出し（文、束縛、`return`、アームの本体） | その配線の op の名前空間 |
| デコレータの引数に渡した op（`any op` の引数。名前付きのデコレータが固定した引数を含む） | デコレータを付けた宣言の名前空間（名前付きのデコレータの固定した引数なら、その別名の宣言の名前空間） |

当たらないもの: 分類の対象にする（`classify C of ns.op`）、ドキュメンテーションコメントの `@see`、`bind` の対象の名前空間に含まれること。

- 対象は op だけ。型、例外、分類、効果、`read`、デコレータは、今までどおり修飾すればどこからでも参照できる。状態の規則（持ち主の名前空間だけが読み書きする）も変わらない。
- `bind` は、別の名前空間の内部の op には位置を作らない（「計算」の「bind」の 1）。呼べない op のために BIND-1 / BIND-2 を出さないため。
- 計算（Sig、`View.ops`）は `export` によらず、すべての op について行う。
- `export` は IR に載る（`Op.exported`）。検査が見るのは仕様の中の呼び出しだけで、`export` を付けた op をどの名前空間も呼んでいないことは診断しない（呼び手が、プラグインの出力や人が書くコードのように、仕様の外にいることがある）。
- 標準の語彙は op を宣言していないので、変えるものはない。プラグインの語彙が、利用者に呼ばせる op を宣言するなら、同じく `export` を付ける。

### never を返す op

`ReturnType` が `never` の op は、決して値を返さない。次のすべてを満たさなければ THROWS-3。

1. 葉である（本体を持たない）。
2. `throws` の例外（ユニオンを展開した後）が、ちょうど一つ。
3. その例外が値を運ばない（`Exception.value` がない）。
4. 引数を持たない。
5. `effects` も `reads` も持たない（デコレータが運ぶものを含めて。「計算」の「デコレータが運ぶ reads と effects」）。
6. `catches` を持つデコレータが付いていない。

この op の中身は「その例外を投げる」と決まる（中身を出すのはトランスパイラ）。呼び出しの式の型は「返らない」で（`docs/types.md`「式の型」）、どの型の `match` のアームにも、どの型の `return` にも書ける。文として書けるのは本体の最後だけ（WIRE-9）。

### throw

アームの本体の `throw E`（`default => throw type.Unauthorized;`）は、その例外を投げる。

- `E` は例外一つ。どの名前空間の例外でもよい。例外のユニオンやデコレータの引数は書けない（RES-4）。
- `E` が値を運ぶ（`Exception.value` がある）なら MATCH-8。値を運ぶ例外を投げるのは、葉の op。
- 計算では、例外が `E` だけで効果を持たない呼び出しとして数える（「計算」の手順 0）。`E` は、配線の `throws` に書くか、デコレータが吸収・肩代わりしなければならない（THROWS-1）。
- 式の型は「返らない」。

### 状態の参照

配線の値の位置（`Atom`）に、今の名前空間の状態を名前で書ける。型はその状態の型（`docs/types.md`「式の型」）。

- 同じ名前の引数か束縛があれば、そちらを指す（`docs/resolve.md`「手続き」の 1 が 5 より先）。隠すこと自体は診断しない。状態と同じ名前の入力（`token`）は普通に書くもので、現在の名前空間の宣言がグローバルの宣言を隠すのを診断しないのと同じ。
- 別の名前空間の状態は書けない（RES-5）。その名前空間の op を通して読む。
- 名前で書いた状態は、その配線が読む状態に入る（「計算」の「reads」）。
- 呼び出しの引数に状態を名前で渡した位置は、明示した位置で、`bind` は埋めない（「計算」の「bind」）。`bind` は、書かなくても埋まる方として残る。
- できるのは読むことだけ。変えるのは効果を持つ op。

## 計算

規則の検査に先立って、op ごとに次を計算し、`Op.signature`（`docs/ir.md`「op」）と `Call.bound` に置く。例外・効果・読む状態・純粋さと、`bind` で埋まる位置。計算は op の呼び出しの依存の順に行う。配線の呼び出しが循環していれば WIRE-7 で、循環に含まれる op は本体を無視して葉として計算する。計算の途中で出す診断（THROWS-1、REL-4、READS-2、DEC-5、DEC-16、WIRE-7）は Sig(O, ns(O)) を計算するときだけ出し、ほかの見る側の計算では出さない。

記法: op を O、O の名前空間を ns(O)、O に付いたデコレータの適用を内側から順に d₁ … dₙ（`Op.decorators` の末尾から先頭へ）。各 dᵢ の宣言を D(dᵢ) とし、`$param:` の参照はその適用の引数で置き換えてから使う。見る側の名前空間を v とする。

### デコレータが運ぶ reads と effects

デコレータの宣言の `reads` / `effects` の節（`docs/grammar.md`「decorator の宣言」）は、そのデコレータを付けた op の中身が読むものと持つ効果を、op の代わりに言う（`docs/decisions.md`「決めたこと」98）。op は、同じものを自分の `reads` / `effects` に書かなくてよい。書いてもよく、重複は一つと数える。

- **運ばれるもの**（`Op.carried`。`docs/ir.md`「op」）: O に付いたデコレータを書かれた順（`Op.decorators` の順）に、その宣言の節を書かれた順に見て、節の要素を並べたもの。要素は op の節と同じ形（「状態の形」の「直に書く要素」）で、デコレータの引数を指す要素（`$param:`）は、その適用の引数の値で置き換える: `any read` と `any effect` の引数の値は名前で書いた要素（`named`）に、`any state` の引数の値は直に書いた状態（`direct`）になる。残りの引数なら、渡した値を書いた順にすべて。省略可能な引数を省いていれば、何も足さない。効果のユニオンは展開する。`reads` の要素は `carried.reads` に、`effects` の要素は `carried.effects` に入れ、それぞれ重複を最初の出現に寄せる。
- O が**直に読むもの**は、`Op.reads` の後ろに `Op.carried.reads` を並べ、重複を最初の出現に寄せたもの。**直に読む状態**は、その要素ごとの状態（`read` の宣言ならその宛先、直に書いた状態ならその状態）を同じ順に並べ、重複を最初の出現に寄せたもの。**直に持つ効果**は、`Op.effects` の後ろに `Op.carried.effects` を並べ、重複を最初の出現に寄せたもの。どれも、呼び出し先から伝わるもの（配線）と、`halts` の効果（手順 i）は含まない。
- 運ぶ節を持つデコレータを付けられるのは、葉の op だけ。配線に付ければ DEC-17: 配線が読む状態と効果は、本体の呼び出しから決まる事実で、節で足すものではない。配線の計算は、運ばれるものを使わない。
- 運ばれたものも、op が書いたものと同じに確かめる: 別の名前空間の状態を読むもの（その状態、その状態の `read`）なら READS-1、別の名前空間の効果なら STATE-2。
- EFF-3（葉の op が自分で書ける効果は一つまで）が数えるのは `Op.effects` だけ。運ばれる効果はいくつでもよい: 順序はデコレータの引数に書いた順で、それらをどう行うか（一体か、どの順か）は、そのデコレータを読むトランスパイラが決める（`@db.effects` は、書いた順の、一つのトランザクション。`docs/plugin-go-sqlite.md`「op の中身」）。
- 呼び出し元への伝わり方は、op が書いたものと同じ（EFF-1、「reads」）。デコレータの付いた op は、もともと純粋でない（「純粋」の 1）ので、純粋さは変わらない。

### Sig(O, v)

名前空間 v から O を呼んだときの関数の型。六つの値を持つ。

- E: 投げうる例外の列
- F: 効果の列（効果の数え方は「状態の形」の「直に書く要素」）
- rel(X, e): X ∈ E、e ∈ F ごとの `kept` / `prevented` / `unknown`
- succ(e): 成功のアームでの e の `kept` / `prevented` / `unknown`
- R: 読む状態の列（下の「reads」）
- pure: 純粋かどうか（下の「純粋」）

R と pure は v によらない（どの名前空間から見ても同じ）。

**手順 0（本体の分）**

葉の op:

- E = `Op.throws` の例外（`Declared(O)` と呼ぶ）。
- F = O が直に持つ効果（`Op.effects` と、デコレータが運ぶ `Op.carried.effects`。「デコレータが運ぶ reads と effects」）。
- rel(X, e) = X を含む `ThrowsClause` の `relations` に `prevents` があれば `prevented`、`keeps` があれば `kept`。どちらもなければ `unknown`。両方あれば REL-5。
- succ(e) = `kept`。

配線の op: 本体の行 i = 1..m を順に見る。行 i の呼び出し（`const` / `call` / `return` なら一つ、`match` ならアームごとの呼び出し）c について Sc = Sig(callee(c), ns(O)) とする。アームの本体の `throw X` は、Sc.exceptions が X だけで、Sc.effects が空の呼び出しとして数える（出どころは、その行とその `throw`）。値（`Atom`）の本体と、式が `Atom` の行は、呼び出しを持たない。

- F = 各 Sc.effects を行の順、アームの順に連結し、重複を最初の出現に寄せたもの。
- E = 各 Sc.exceptions の和に `Declared(O)` を足したもの（宣言だけで本体から出ない例外も含む。THROWS-2）。
- X の**出どころ**は、X ∈ Sc.exceptions となる (行 i, 呼び出し c) の組。出どころ一つにおける e の状態は、上から順に最初に当たるもの:
  1. e が行 j < i のどれかの呼び出しの effects にある → `kept`
  2. e ∈ Sc.effects → Sc.rel(X, e)
  3. それ以外（行 i の別のアーム、行 j > i、どこにもない） → `prevented`
- rel(X, e) = すべての出どころで同じ状態ならその状態、違えば `unknown`。出どころのない X（`Declared(O)` だけにあるもの）は葉と同じく `Op.throws` の `relations` から決める。
- succ(e) = e を effects に持つ呼び出し c ごとの Sc.succ(e) を集め、すべて `kept` なら `kept`、すべて `prevented` なら `prevented`、それ以外は `unknown`。`match` の行で、一部のアームにしかない e は `unknown`。ただし本体が「返らない」アーム（`throw`、`never` を返す op の呼び出し。`docs/types.md`「式の型」）は、成功しないので数えない（`Registered => issueSession(...); default => throw X;` の `issueSession` の効果は、すべてのアームにあるものとして扱う）。
- 配線の `Op.throws` に `relations` が書かれていれば、ここで計算した rel と比べる（REL-4）。

**手順 i = 1..n（デコレータ dᵢ を内側から）**

D = D(dᵢ) について、`catches` の節をすべて、次に `throws` の節をすべて適用する。`reads` / `effects` の節は、ここでは適用しない（手順 0 と「reads」で、op の中身の分として数えてある）。

- `catches Xs mode`: 各 X ∈ Xs について、X ∉ E なら DEC-16（吸収する相手がない）。X ∈ E なら:
  - `recovers` / `ignores`: 各 e ∈ F について rel(X, e) ≠ `kept` なら succ(e) = `unknown`（§8.2「効果が不明な例外を `recovers` で吸収すると、成功のアームでもその効果は不明になる」）。X を E から除く。
  - `halts h`: X を E から除く。h ∉ F なら h を F の末尾に足し、残りのすべての Y ∈ E について rel(Y, h) = `prevented`、succ(h) = `prevented`。h ∈ F ならそのまま。
- `throws Ys relations`: D が `external` で v = ns(O) なら飛ばす（DEC-10）。そうでなければ各 Y ∈ Ys について、Y を E の末尾に足し（既にあれば位置はそのまま）、各 e ∈ F について `relations` から状態を決める（葉の rel と同じ規則。目的語の `effects` は F のすべて）。Y が既に E にあって状態が違えば `unknown`。

**結果**: `Signature.exceptions` = E、`effects` = F、`relations` = rel、`successEffects` = succ、`reads` = R、`pure` = pure。`Op.signature` = Sig(O, ns(O))。`View.ops` は Sig(O, viewer)（`docs/ir.md`「その名前空間から見た IR」）。

### reads

op が読む状態。`effects`（書く）と対になる、事実の記録（`docs/decisions.md`「設計書を変える決定」S15）。状態を読むのは op で、`read` の宣言（「状態の形」の「読むことの宛先」）は、op が読むものに名前を付けるだけ。状態を読み（R が空でない）、効果を持たない op を**読む op** と呼ぶ（`op findUser(email: Email): User? reads users;`。デコレータが運ぶ形なら `@db.find(users, "email") op findUser(email: Email): User?;`）。葉の op は、読む状態と効果を両方持ってよい（読んでから書く op）。

R は基底の状態の FQN の列で、op ごとに次のように決まる。デコレータが R に足すのは、`reads` の節が運ぶものだけ（「デコレータが運ぶ reads と effects」）。`throws` / `catches` の節は R を変えない。

葉の op: R = O が直に読む状態（「デコレータが運ぶ reads と effects」。`read` の宣言は、その宛先の状態として数える）。葉の op が読めるのは自分の名前空間の状態だけで、別の名前空間の状態なら READS-1。

配線の op: 本体を行の順に見て、次を出現順に集め、重複を最初の出現に寄せる。一つの行の中の順は 1、2 の順。

1. `match` の行なら、その対象が読む状態: 対象の `Atom` が参照する状態。`match by C` なら、C の対象の状態を宣言の順に。
2. 行の式（`match` の行ならアームの順に、各アームの本体）ごとに、次の順:
   1. `Atom` が参照する状態（書かれた順。呼び出しの引数、レコードリテラルの中の値、`return` の値、アームの値を含む）。
   2. 呼び出しの `Call.bound` の状態（`bound` の順）。
   3. 呼び出し先の Sig の R。

- R は名前空間をまたいで伝わる。`server` の配線が `store.findUser`（`reads users`）を呼べば、その配線の R に `store.users` が入る。効果（EFF-1）と同じ。
- 配線の op は `reads` を書かなくてよい。書いた場合（`Op.reads` が空でない）、書いた要素の状態の集合が R と等しくなければ READS-2。等しくするために、配線の `reads` には別の名前空間の状態（と、その `read`）も書ける（配線の `effects` に別の名前空間の効果を書けるのと同じ）。
- 呼び出しが循環している op（WIRE-7）は、葉として R = `Op.reads` の要素の状態。

### 純粋

op が純粋かどうかを、仕様から割り出す（`docs/decisions.md`「決めたこと」60）。op そのものの性質で、見る側の名前空間にも `hozo.toml` にもよらない。診断は持たず、使うのはトランスパイラ（`Signature.pure`）。

pure(O) が真なのは、次のすべてを満たすとき。

1. O にデコレータが付いていない（`Op.decorators` が空。どのトランスパイラも読まない印のデコレータでも、付いていれば純粋でない）。
2. R が空（状態を読まない。配線なら、状態を名前で参照せず、状態の分類で `match` せず、`bind` で埋まる位置を持たず、状態を読む op を呼ばない）。
3. F が空（効果を持たない）。
4. 配線なら、本体の呼び出し先がすべて純粋。

- `throws`、アームの `throw`、値の分類や op の分類での `match` は、純粋さを変えない。
- 呼び出しが循環している op（WIRE-7）は、葉として 1〜3 だけで決める。
- 外部の入出力をする op が純粋にならないことは、規則ではなく書き方から来る（`docs/requirements.md`「書き方の指針」）。

### bind

`Bind` b（名前空間 A、状態 s、対象 T）と op O について、埋める位置 Pos(b, O):

1. ns(O) が T.namespace と等しくない（`deep` なら T.namespace を接頭辞に持たない）→ Pos は空。ns(O) が A と違い、O に `export` がない（A から呼べない。「op の宣言と本体」の「export」）→ Pos は空。
2. s の `shape` が `list` → BIND-6。
3. τ = s の `element`。
4. O の各引数から、型が τ または `optional(τ)` の位置を集める。型が model（または model の `optional`）ならフィールドを再帰的に辿る。`array` と `record` の中は辿らない。辿っている途中で同じ model に戻ったら、そこで止める。位置はパス（`["in", "token"]`）。
5. 位置が 2 以上 → BIND-1。1 で、s の `shape` が `nullable` で、位置の型が `optional` でない → BIND-2。
6. 二つの `Bind` が同じ op の同じ位置に当たる → BIND-5。

A の配線の呼び出し c（callee O）の `Call.bound` は、A のすべての `Bind` の Pos(b, O) から、c が明示している位置を除いたもの（BIND-3）。位置が明示されているのは、パスの引数が `name` か `state` の `Atom` で渡されている（中身は分からないので明示とみなす）か、レコードリテラルの連なりがパスのフィールドを持つとき。

### 必須の入力（BIND-4）

各呼び出し c について、callee の各引数 p:

- p の型が `optional` でなく、c の `args` に p がなく、`Call.bound` に `[p]` もなければ BIND-4。
- c の `args` に p がレコードリテラルで渡されていれば、`docs/types.md`「当てはまる」の手続きで、必須のフィールドが `Call.bound` のパスも含めて埋まっているかを再帰的に確かめる。足りなければ BIND-4（パスを示す）。

## 診断

各表の「手続き」が条件。「重さ」は error か warning。計算の結果（`Signature`、`Call.bound`）を要する規則は `internal/check` が、要しない規則は IR を作るときに `internal/sema` が出す（S15〜S23 で足した規則では、READS-2 だけが前者）。位置は、表に書いてなければ規則の対象（宣言、フィールド、呼び出し、アーム）の `Span`。設計書にある規則の意味は `docs/requirements.md`「検査の規則の一覧」の節を、ない規則の意味はこの表を持ち主とする。`hozo.toml` がないとき（`docs/config.md`「置き場と探し方」）は、呼び出す側（`internal/project`）が CFG-* を除く。

### 名前空間

| ID | 重さ | 手続き |
|---|---|---|
| NS-1 | error | 解決の手続きそのもの（`docs/resolve.md`「手続き」）。診断は RES-1 として出る |
| NS-2 | — | 正規化で行う。診断なし |
| NS-3 | error | RES-6 のうち、両方の出どころが語彙（`Span.file` が `hozo:` で始まる。`docs/ir.md`「Program」）のもの。メッセージに語彙の出どころ（標準か、どのプラグインか）を出す |
| NS-5 | error | 別の名前空間の、`export` のない op を呼んだ（「op の宣言と本体」の「export」の表の二つ: 配線の呼び出し、デコレータの引数に渡した op）。位置は呼び出しの op の名前（デコレータの引数ならその引数の値）。`Related` にその op の宣言 |
| NAME-1 | warning | `model` / `scalar` / `exception` / `union` / `classify` とそのケースの名前の先頭が大文字でない。`state` / `op` / `effect` / `read` / `decorator` の名前の先頭が小文字でない。`_` で始まる名前も警告。語彙は対象外。warning にするのは、設計書が強さを決めていないため |
| DOC-1 | error | `@param` が引数名にない、`@see` が解決できない（`internal/sema`）。`@throws` の例外が `Signature.exceptions` にも、O に付いた `external` なデコレータの `throws` にもない（`internal/check`。外から見たときだけ起きる例外も書けるようにするため）。デコレータの宣言の `@param` は `Param` の名前 |
| DOC-2 | error | 同じ `@param` 名、同じ `@throws` 例外が二度ある |
| UNION-1 | error | `union` の要素にアーム（`A: T`）と参照（`X;`）が混ざる。または参照の要素に例外と効果が混ざる。または `arms` でアーム名が重複 |

NS-4 は欠番。「`classify` は宣言した名前空間の中でだけ使える」（型の位置の `C.Case<T>` の C と、`match` の C が、現在の名前空間の `classify` でなければエラー）という規則で、分類の置き場を自由にした（`docs/decisions.md`「設計書を変える決定」S18）ときになくなった。番号は別の意味に使い回さない（欠番の一覧は `docs/requirements.md`「検査の規則の一覧」）。

### 設定

`docs/config.md`「診断」。

### 状態と効果

| ID | 重さ | 手続き |
|---|---|---|
| STATE-1 | error | 組み込みの効果の `operation` が、その状態の形にない（「状態の形」の表。`single` に `post` / `delete`）。`effect` の宣言の宛先にも、`effects` に直に書いた `状態.操作`（「直に書く要素」）にも掛かる。位置は操作の語（直に書いた要素なら、その要素） |
| STATE-3 | error | 状態の要素の型（`State.element`）が `optional`。形が型から決まるようになった（「状態の形」）ので、当たるのは `list` の状態だけ（`list<T?>`、`T?[]`）: 列は値を持つ。`single<T?>` と `nullable<T?>` は `T?`（`nullable<T>`）と同じ型なので当たらない（`docs/types.md`「形の書き方」）。当たった状態は、`list<T>` として残りの検査を続ける |
| STATE-4 | error | 遷移の前か後のケースが、宛先の分類のケースでない（宛先が基底の状態なら `Empty` / `HasItems`）。宛先が `single` の状態（組み込みの分類がない）。前と後のどちらかのケースの並びに重複がある |
| STATE-2 | error | 葉の op O が直に持つ効果 e（`Op.effects` と、デコレータが運ぶ `Op.carried.effects`）について、e の名前空間（直に書いた効果なら、その状態の名前空間）が ns(O) と違う。デコレータの `halts` の効果（宣言に書いたもの、`any effect` の引数で渡したもの）についても、デコレータを付けた op の名前空間と違えば同じ（§6.2「持ち主ではない名前空間の葉の op がその効果を `effects` に宣言すればエラー」）。位置は、op が書いた効果ならその名前、デコレータのものならその適用 |
| EFF-1 | — | 計算（Sig の F）で行う。診断なし |
| EFF-2 | error | 配線の O で `Op.effects` が空でなく、集合として Sig(O, ns(O)).effects と等しくない（効果の数え方は「直に書く要素」: 直に書いた効果と、同じ宛先の宣言は別）。足りないもの・余分なものを `Data.missing` / `Data.extra` に（宣言は FQN、直に書いた効果は `<状態の FQN>.<操作>`） |
| READS-1 | error | 葉の op O が直に読む状態（`Op.reads` と、デコレータが運ぶ `Op.carried.reads` の要素の状態。`read` の宣言なら、その宛先）の名前空間が ns(O) と違う（計算「reads」。`Signature` を要しないので `internal/sema` が出す）。位置は、op が書いた要素ならその名前、デコレータが運ぶものならその適用 |
| READS-2 | error | 配線の O で `Op.reads` が空でなく、その要素の状態の集合が Sig(O, ns(O)).reads と等しくない（計算「reads」）。足りないもの・余分なものを `Data.missing` / `Data.extra` に。位置は O の名前 |
| EFF-3 | error | 葉の op が自分で書いた `Op.effects`（ユニオンを展開した後。同じ効果の重複は一つと数える。名前で書いたものも、直に書いたものも数える）が二つ以上（`Signature` を要しないので `internal/sema` が出す）。デコレータが運ぶ効果（`Op.carried.effects`）は数えない（計算「デコレータが運ぶ reads と effects」）。op が自分で書く形で複数の効果を持つ処理は、効果ごとの葉の op を配線でつなぐ（効果は伝播する。§7.2）。複数の基底の状態を一体として変えるなら、それらを対象にした状態の分類の遷移一つにするか（「効果の宛先」）、効果を並べて一体に行うと言うデコレータを付ける |

### throws

| ID | 重さ | 手続き |
|---|---|---|
| THROWS-1 | error | 配線の O について `Missing = (∪ Sc.exceptions) − Declared(O) − ∪Catches(dᵢ) − ∪Throws(dᵢ, ns(O))`（Sc は本体の各呼び出しの Sig(callee, ns(O))。アームの本体の `throw X` は、例外が X だけの呼び出しとして数える（計算の手順 0）。`external` なデコレータの `throws` は Sig と同じく数えない）。空でなければ、位置は O の名前、メッセージに Missing の FQN、`Related` に各例外の出どころの呼び出し、`Data.missing` に FQN の列 |
| THROWS-2 | — | 診断なし。多めの宣言は許す |
| THROWS-3 | error | `never` を返す op が、「op の宣言と本体」の「never を返す op」の条件 1〜6 のどれかを満たさない（`Signature` を要しないので `internal/sema` が出す）。満たさない条件ごとに一つ。位置は、1 は op の名前、2 と 3 は `throws`（なければ op の名前）、4 は最初の引数、5 はその節（デコレータが運ぶものなら、そのデコレータ）、6 はそのデコレータ |
| REL-1 | error | `Op.throws`（またはデコレータの `throws`）の全節を通して同じ例外が二度ある（`Signature` を要しないので `internal/sema` が出す。REL-5 も同じ） |
| REL-2 | warning | Sig(O, ns(O)).effects が空の op の `Op.throws` に `relations` がある。デコレータの `throws` の `relations` は対象外（§7.3） |
| REL-4 | error | 計算の手順 0 の「配線の `Op.throws` に `relations` が書かれていれば」の比較。`prevents` なのに `prevented` でない、`keeps` なのに `kept` でない組を示す |
| REL-5 | error | 一つの `ThrowsClause` に `prevents` と `keeps` の両方がある |

REL-3 と REL-6 は欠番。`relations` の目的語に効果の名前を個別に書く形についての規則（REL-3: 名前の列で書いたら warning、REL-6: その名前が Sig の effects にない）で、目的語を `effects` だけにした（`docs/decisions.md`「設計書を変える決定」S9）ときになくなった。番号は別の意味に使い回さない（欠番の一覧は `docs/requirements.md`「検査の規則の一覧」）。

### デコレータ

| ID | 重さ | 手続き |
|---|---|---|
| DEC-1 | error | `Application` が付いた先の種類が、D の `targets` にない。`union` / `classify` / `decorator` に付けたら常に DEC-1（`namespace` と `bind` は文法で閉じる。`docs/grammar.md`「ファイルと宣言」） |
| DEC-2 | error | `targets` に `op` を含まない宣言が `throws`、`catches`、`reads`、`effects` のどれかの節を持つ |
| DEC-3 | — | 文法で閉じる |
| DEC-4 | error | `catches ... ignores` を持つデコレータが、`returns` が `null` でない op に付いている |
| DEC-5 | warning | 計算の手順 i で `recovers` が succ(e) を `unknown` にした（F が空なら起きない） |
| DEC-6 | warning | 一つのデコレータの宣言で、`catches` の例外と `throws` の例外に同じものがある |
| DEC-7 | error | `docs/types.md`「デコレータの引数」の、`any effect on` と、ユニオンのメンバーの照合 |
| DEC-8 | — | 文法で閉じる |
| DEC-9 | — | 正規化（`docs/ir.md`「デコレータの適用」） |
| DEC-10 | — | 計算（手順 i の `throws`） |
| DEC-11 | error | `docs/types.md`「デコレータの引数」の数と名前 |
| DEC-12 | error | 別名の `target` が別名。または別名の `args` が `target` の `Param` に合わない（DEC-11 と同じ手続き） |
| DEC-13 | error | 同じデコレータが同じ宣言に二度付いている（引数が違っても。名前付きデコレータは展開した汎用デコレータで比べる） |
| DEC-14 | warning | `external` なデコレータが `throws` を持たない |
| DEC-15 | error | `@external` がデコレータの宣言以外に付いている（文法で閉じる。索引として残す） |
| DEC-16 | warning | 計算の手順 i で、`catches` の例外がそこまでの E にない |
| DEC-17 | error | `reads` か `effects` の節を持つデコレータが、配線の op（本体を持つ op）に付いている（計算「デコレータが運ぶ reads と effects」。`Signature` を要しないので `internal/sema` が出す）。位置はそのデコレータの適用 |

### 配線

| ID | 重さ | 手続き |
|---|---|---|
| WIRE-1 | — | 文法で閉じる |
| WIRE-2 | error | `const x = 式` で式が「値がない」か「返らない」。`式;` の行で式に値がある。`return 式` で式が「値がない」。式の型は `docs/types.md`「式の型」 |
| WIRE-3 | error | O の `returns` が `null` なのに `return` がある。`returns` が `null` でないのに、本体の最後の文が `return` でない。`return` が最後以外の文にある（下の行が評価されないため） |
| WIRE-4 | error | 同じ名前を二度束縛。引数と同じ名前を束縛。束縛より上の行で使っている（RES-1 として出る） |
| WIRE-5 | — | 文法で閉じる |
| WIRE-6 | warning | 本体の `const` の数が `hozo.toml` の `[hozo].max_bindings`（`docs/config.md`「形」）を超える。既定は 5。設計書が値を決めていないため、ここで決める |
| WIRE-7 | error | 配線の呼び出しが循環している。循環上の op をすべて `Related` に示す |
| WIRE-8 | warning | 束縛した名前が下の行で使われていない |
| WIRE-9 | error | 「返らない」式の文（`unauthorized();`、すべてのアームが返らない `match ... ;`）が、本体の最後以外にある（下の行が評価されないため。WIRE-3 の `return` と同じ理由）。位置はその文 |

### classify / match

| ID | 重さ | 手続き |
|---|---|---|
| CLS-1 | error | `classify` に `default` が二つ以上、またはケース名が重複 |
| CLS-2 | error | 名前の付いたケースが一つもない |
| CLS-3 | error | `classify` / `condition` の対象の並びが、型一つ、op 一つ、今の名前空間の基底の状態の並び（「分類」の「対象」）のどれでもない: 対象が二つ以上なのに状態でないもの（型、op）がある。型や op と状態が混ざっている。同じ状態が二度ある。対象が別の名前空間の状態、または状態の分類 |
| CLS-4 | error | 分類の対象の op が値を返さない（`returns` が `null` か `never`）。位置は対象の名前 |
| CLS-5 | error | ケースの `as` の前に書いたものが、その分類の対象でない（値の分類なら `subject` の型と等しい型、op の分類ならその op、状態の分類なら対象の状態のどれか）。または一つのケースに同じ対象の `as` が二度ある（「分類」の「ケースの型」）。位置は `as` の前の名前 |
| CLS-6 | error | ケースの `as` の後ろの型が、対象の型から絞り込める型でない（`docs/types.md`「絞り込める型」）。位置は `as` の後ろの型 |
| MATCH-1 | — | 文法で閉じる |
| MATCH-2 | error | アームに並べたケースの集合が分類のケース（標準の分類なら `Empty` / `HasItems` で、`default` はない）と等しくない（足りない・知らない名前・重複）。`hasDefault` が false なのに `default` のアームがある。`hasDefault` が true なのに `default` のアームがない。`Data.missing` / `Data.extra` |
| MATCH-3 | — | 文法で閉じる（アームの本体は、呼び出し一つ、値一つ、`throw` 一つのどれか。`docs/grammar.md`「配線」の `ArmBody`） |
| MATCH-4 | error | 値を対象にした `match` で、アームの本体の呼び出しに、対象と同じ `Atom` をそのまま渡している引数が、アームの中の対象の型（`docs/types.md`「アームの中の対象の型」。型を書かないケースなら `case(C, アームのケース, X)` で、「互換」の規則 4・5）では渡せない。対象を渡していない引数は TYPE-4 |
| MATCH-5 | error | 「値がない」型の `match` を `const` で束縛している（WIRE-2 と同じ条件。索引として残す） |
| MATCH-6 | error | 値の分類か op の分類での `match` で、対象の型が分類の対象の型（「分類」の「対象」の表）に渡せない |
| MATCH-7 | error | `match` の形と分類が合わない（`docs/grammar.md`「配線」の表）: 値の分類・op の分類・標準の分類なのに対象がない、状態の分類なのに対象がある、`by list` の対象の型が `array` でない、`by nullable` の対象の型が `optional` でない |
| MATCH-8 | error | アームの本体の `throw E` の `E` が、値を運ぶ例外（「op の宣言と本体」の「throw」）。位置は `E` |

### bind

| ID | 重さ | 手続き |
|---|---|---|
| BIND-1 | error | 計算「bind」5。位置は `Bind`。`Related` に各位置 |
| BIND-2 | error | 計算「bind」5。位置は `Bind` |
| BIND-3 | — | 計算「bind」の `Call.bound`。診断なし |
| BIND-4 | error | 計算「必須の入力」。位置は呼び出し。`Data.missing` にパス |
| BIND-5 | error | 計算「bind」6。位置は後の `Bind` |
| BIND-6 | error | `list` の状態を `bind` に書いた（`bind` できるのは `single` と `nullable`） |

### 語彙に固有の規則

PAGE-1 / PAGE-2 は hozoc では検査しない。プラグインの `check`（`docs/plugin-protocol.md`「check」）が返す。

## Data

`Diagnostic.Data`（`docs/diagnostics.md`「構造」）を持つ規則。

| ID | Data |
|---|---|
| THROWS-1 | `{ "missing": [FQN] }` |
| EFF-2 | `{ "missing": [効果], "extra": [効果] }`。効果は、宣言なら FQN、直に書いた効果なら `<状態の FQN>.<操作>` |
| READS-2 | `{ "missing": [FQN], "extra": [FQN] }` |
| MATCH-2 | `{ "missing": [ケース名], "extra": [ケース名] }` |
| BIND-4 | `{ "missing": [パス] }` |
| RES-1 | `{ "candidates": [FQN] }`（`docs/resolve.md`「診断」） |
