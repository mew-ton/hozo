# 検査

## この文書が決めること

`hozoc check` と `hozoc lsp` が IR（`docs/ir.md`）の上で行う計算と、規則ごとの検査の手続き・診断を決める。規則の ID は `docs/requirements.md`「検査の規則の一覧」のもので、規則の意味の持ち主は設計書の節。ここでは手続きだけを書く。設計書にない規則はこの文書の「診断」の各表が持ち主。名前の解決（`RES-*`）と型（`TYPE-*`、`LIT-1`）はそれぞれ `docs/resolve.md`「診断」、`docs/types.md`「診断」。

検査は IR が作れた後に行う。IR を作る途中（解析、解決、正規化）の誤りがあるファイルでも、作れた部分には検査を行う（`docs/requirements.md`「言語サーバー」）。一つの検査で見つかる誤りはすべて報告する。

## 計算

規則の検査に先立って、op ごとに次を計算し、`Op.signature`（`docs/ir.md`「op」）と `Call.bound` に置く。計算は op の呼び出しの依存の順に行う。配線の呼び出しが循環していれば WIRE-7 で、循環に含まれる op は本体を無視して葉として計算する。計算の途中で出す診断（THROWS-1、REL-4、DEC-5、DEC-16、WIRE-7）は Sig(O, ns(O)) を計算するときだけ出し、ほかの見る側の計算では出さない。

記法: op を O、O の名前空間を ns(O)、O に付いたデコレータの適用を内側から順に d₁ … dₙ（`Op.decorators` の末尾から先頭へ）。各 dᵢ の宣言を D(dᵢ) とし、`$param:` の参照はその適用の引数で置き換えてから使う。見る側の名前空間を v とする。

### Sig(O, v)

名前空間 v から O を呼んだときの関数の型。四つの値を持つ。

- E: 投げうる例外の列
- F: 効果の列
- rel(X, e): X ∈ E、e ∈ F ごとの `kept` / `prevented` / `unknown`
- succ(e): 成功のアームでの e の `kept` / `prevented` / `unknown`

**手順 0（本体の分）**

葉の op:

- E = `Op.throws` の例外（`Declared(O)` と呼ぶ）。
- F = `Op.effects`。
- rel(X, e) = X を含む `ThrowsClause` の `relations` に、e を含む（`all` か名前で）`prevents` があれば `prevented`、`keeps` があれば `kept`。どちらもなければ `unknown`。両方あれば REL-5。
- succ(e) = `kept`。

配線の op: 本体の行 i = 1..m を順に見る。行 i の呼び出し（`const` / `call` / `return` なら一つ、`match` ならアームごとの呼び出し）c について Sc = Sig(callee(c), ns(O)) とする。

- F = 各 Sc.effects を行の順、アームの順に連結し、重複を最初の出現に寄せたもの。
- E = 各 Sc.exceptions の和に `Declared(O)` を足したもの（宣言だけで本体から出ない例外も含む。THROWS-2）。
- X の**出どころ**は、X ∈ Sc.exceptions となる (行 i, 呼び出し c) の組。出どころ一つにおける e の状態は、上から順に最初に当たるもの:
  1. e が行 j < i のどれかの呼び出しの effects にある → `kept`
  2. e ∈ Sc.effects → Sc.rel(X, e)
  3. それ以外（行 i の別のアーム、行 j > i、どこにもない） → `prevented`
- rel(X, e) = すべての出どころで同じ状態ならその状態、違えば `unknown`。出どころのない X（`Declared(O)` だけにあるもの）は葉と同じく `Op.throws` の `relations` から決める。
- succ(e) = e を effects に持つ呼び出し c ごとの Sc.succ(e) を集め、すべて `kept` なら `kept`、すべて `prevented` なら `prevented`、それ以外は `unknown`。`match` の行で、一部のアームにしかない e は `unknown`。
- 配線の `Op.throws` に `relations` が書かれていれば、ここで計算した rel と比べる（REL-4）。

**手順 i = 1..n（デコレータ dᵢ を内側から）**

D = D(dᵢ) について、`catches` の節をすべて、次に `throws` の節をすべて適用する。

- `catches Xs mode`: 各 X ∈ Xs について、X ∉ E なら DEC-16（吸収する相手がない）。X ∈ E なら:
  - `recovers` / `ignores`: 各 e ∈ F について rel(X, e) ≠ `kept` なら succ(e) = `unknown`（§8.2「効果が不明な例外を `recovers` で吸収すると、成功のアームでもその効果は不明になる」）。X を E から除く。
  - `halts h`: X を E から除く。h ∉ F なら h を F の末尾に足し、残りのすべての Y ∈ E について rel(Y, h) = `prevented`、succ(h) = `prevented`。h ∈ F ならそのまま。
- `throws Ys relations`: D が `external` で v = ns(O) なら飛ばす（DEC-10）。そうでなければ各 Y ∈ Ys について、Y を E の末尾に足し（既にあれば位置はそのまま）、各 e ∈ F について `relations` から状態を決める（葉の rel と同じ規則。`all` は F のすべて）。Y が既に E にあって状態が違えば `unknown`。

**結果**: `Signature.exceptions` = E、`effects` = F、`relations` = rel、`successEffects` = succ。`Op.signature` = Sig(O, ns(O))。`View.ops` は Sig(O, viewer)（`docs/ir.md`「その名前空間から見た IR」）。

### bind

`Bind` b（名前空間 A、状態 s、対象 T）と op O について、埋める位置 Pos(b, O):

1. ns(O) が T.namespace と等しくない（`deep` なら T.namespace を接頭辞に持たない）→ Pos は空。
2. s の `shape` が `list` → BIND-6。
3. τ = s の `element`（`optional` なら中身）。
4. O の各引数から、型が τ または `optional(τ)` の位置を集める。型が model（または model の `optional`）ならフィールドを再帰的に辿る。`array` と `record` の中は辿らない。辿っている途中で同じ model に戻ったら、そこで止める。位置はパス（`["in", "token"]`）。
5. 位置が 2 以上 → BIND-1。1 で、s の `element` が `optional` で、位置の型が `optional` でなく `default` もない → BIND-2。
6. 二つの `Bind` が同じ op の同じ位置に当たる → BIND-5。

A の配線の呼び出し c（callee O）の `Call.bound` は、A のすべての `Bind` の Pos(b, O) から、c が明示している位置を除いたもの（BIND-3）。位置が明示されているのは、パスの引数が `name` の `Atom` で渡されている（中身は分からないので明示とみなす）か、レコードリテラルの連なりがパスのフィールドを持つとき。

### 必須の入力（BIND-4）

各呼び出し c について、callee の各引数 p:

- p の型が `optional` でなく、c の `args` に p がなく、`Call.bound` に `[p]` もなければ BIND-4。
- c の `args` に p がレコードリテラルで渡されていれば、`docs/types.md`「当てはまる」の手続きで、必須のフィールドが `Call.bound` のパスも含めて埋まっているかを再帰的に確かめる。足りなければ BIND-4（パスを示す）。

## 診断

各表の「手続き」が条件。「重さ」は error か warning。位置は、表に書いてなければ規則の対象（宣言、フィールド、呼び出し、アーム）の `Span`。設計書にある規則の意味は `docs/requirements.md`「検査の規則の一覧」の節を、ない規則の意味はこの表を持ち主とする。`hozo.toml` がないとき（`docs/lsp.md`「プロジェクト」）は、呼び出す側（`internal/project`）が CFG-* を除く。

### 名前空間

| ID | 重さ | 手続き |
|---|---|---|
| NS-1 | error | 解決の手続きそのもの（`docs/resolve.md`「手続き」）。診断は RES-1 として出る |
| NS-2 | — | 正規化で行う。診断なし |
| NS-3 | error | RES-6 のうち、両方の出どころが語彙（`Span.file` が `hozo:` で始まる。`docs/ir.md`「Program」）のもの。メッセージに語彙の出どころ（標準か、どのプラグインか）を出す |
| NS-4 | error | 型の位置の `C.Case<T>` の C が現在の名前空間の `classify` でない（`match` の C は文法上修飾できない） |
| NAME-1 | warning | `model` / `scalar` / `exception` / `union` / `classify` とそのケースの名前の先頭が大文字でない。`state` / `op` / `effect` / `decorator` の名前の先頭が小文字でない。`_` で始まる名前も警告。語彙は対象外。warning にするのは、設計書が強さを決めていないため |
| DOC-1 | error | `@param` が引数名にない、`@see` が解決できない（`internal/sema`）。`@throws` の例外が `Signature.exceptions` にも、O に付いた `external` なデコレータの `throws` にもない（`internal/check`。外から見たときだけ起きる例外も書けるようにするため）。デコレータの宣言の `@param` は `Param` の名前 |
| DOC-2 | error | 同じ `@param` 名、同じ `@throws` 例外が二度ある |
| UNION-1 | error | `union` の要素にアーム（`A: T`）と参照（`X;`）が混ざる。または参照の要素に例外と効果が混ざる。または `arms` でアーム名が重複 |

### 設定

`docs/config.md`「診断」。

### 状態と効果

| ID | 重さ | 手続き |
|---|---|---|
| STATE-1 | error | `Effect.operation` が、`State.shape` が `single` なのに `post` か `delete` |
| STATE-2 | error | 葉の op O の `Op.effects` の効果 e について、e の名前空間が ns(O) と違う。デコレータの `halts` の効果（宣言に書いたもの、`any effect` の引数で渡したもの）についても、デコレータを付けた op の名前空間と違えば同じ（§6.2「持ち主ではない名前空間の葉の op がその効果を `effects` に宣言すればエラー」） |
| EFF-1 | — | 計算（Sig の F）で行う。診断なし |
| EFF-2 | error | 配線の O で `Op.effects` が空でなく、集合として Sig(O, ns(O)).effects と等しくない。足りないもの・余分なものを `Data.missing` / `Data.extra` に |

### throws

| ID | 重さ | 手続き |
|---|---|---|
| THROWS-1 | error | 配線の O について `Missing = (∪ Sc.exceptions) − Declared(O) − ∪Catches(dᵢ) − ∪Throws(dᵢ, ns(O))`（Sc は本体の各呼び出しの Sig(callee, ns(O))。`external` なデコレータの `throws` は Sig と同じく数えない）。空でなければ、位置は O の名前、メッセージに Missing の FQN、`Related` に各例外の出どころの呼び出し、`Data.missing` に FQN の列 |
| THROWS-2 | — | 診断なし。多めの宣言は許す |
| REL-1 | error | `Op.throws`（またはデコレータの `throws`）の全節を通して同じ例外が二度ある（`Signature` を要しないので `internal/sema` が出す。REL-5 も同じ） |
| REL-2 | warning | Sig(O, ns(O)).effects が空の op の `Op.throws` に `relations` がある。デコレータの `throws` の `relations` は対象外（§7.3） |
| REL-3 | warning | `relations.effects` が `all` でなく名前の列 |
| REL-4 | error | 計算の手順 0 の「配線の `Op.throws` に `relations` が書かれていれば」の比較。`prevents` なのに `prevented` でない、`keeps` なのに `kept` でない組を示す |
| REL-5 | error | 同じ例外・同じ効果に `prevents` と `keeps` の両方がある |
| REL-6 | error | `relations` の効果名が Sig(O, ns(O)).effects にない |

### デコレータ

| ID | 重さ | 手続き |
|---|---|---|
| DEC-1 | error | `Application` が付いた先の種類が、D の `targets` にない。`union` / `effect` / `classify` / `decorator` / `namespace` / `bind` に付けたら常に DEC-1 |
| DEC-2 | error | `targets` に `op` を含まない宣言が `throws` か `catches` を持つ |
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

### 配線

| ID | 重さ | 手続き |
|---|---|---|
| WIRE-1 | — | 文法で閉じる |
| WIRE-2 | error | `const x = 式` で式が「値がない」。`式;` の行で式に値がある。`return 式` で式が「値がない」。式の型は `docs/types.md`「式の型」 |
| WIRE-3 | error | O の `returns` が `null` なのに `return` がある。`returns` が `null` でないのに、本体の最後の文が `return` でない。`return` が最後以外の文にある（下の行が評価されないため） |
| WIRE-4 | error | 同じ名前を二度束縛。引数と同じ名前を束縛。束縛より上の行で使っている（RES-1 として出る） |
| WIRE-5 | — | 文法で閉じる |
| WIRE-6 | warning | 本体の `const` の数が `hozo.toml` の `[hozo].max_bindings`（`docs/config.md`「形」）を超える。既定は 5。設計書が値を決めていないため、ここで決める |
| WIRE-7 | error | 配線の呼び出しが循環している。循環上の op をすべて `Related` に示す |
| WIRE-8 | warning | 束縛した名前が下の行で使われていない |

### classify / match

| ID | 重さ | 手続き |
|---|---|---|
| CLS-1 | error | `classify` に `default` が二つ以上、またはケース名が重複 |
| CLS-2 | error | 名前の付いたケースが一つもない |
| MATCH-1 | — | 文法で閉じる |
| MATCH-2 | error | アームに並べたケースの集合が `Classify.cases` と等しくない（足りない・知らない名前・重複）。`hasDefault` が false なのに `default` のアームがある。`hasDefault` が true なのに `default` のアームがない。`Data.missing` / `Data.extra` |
| MATCH-3 | — | 文法で閉じる |
| MATCH-4 | error | アームの本体の呼び出しで、`subject` をそのまま渡している引数の型が `case(C, アームのケース, T)` に対して `docs/types.md`「互換」の規則 4・5 で渡せない。`subject` を渡していない引数は TYPE-4 |
| MATCH-5 | error | 「値がない」型の `match` を `const` で束縛している（WIRE-2 と同じ条件。索引として残す） |
| MATCH-6 | error | `subject` の型が `Classify.subject` に渡せない |

### bind

| ID | 重さ | 手続き |
|---|---|---|
| BIND-1 | error | 計算「bind」5。位置は `Bind`。`Related` に各位置 |
| BIND-2 | error | 計算「bind」5。位置は `Bind` |
| BIND-3 | — | 計算「bind」の `Call.bound`。診断なし |
| BIND-4 | error | 計算「必須の入力」。位置は呼び出し。`Data.missing` にパス |
| BIND-5 | error | 計算「bind」6。位置は後の `Bind` |
| BIND-6 | error | `list` の状態を `bind` に書いた |

### 語彙に固有の規則

PAGE-1 / PAGE-2 は hozoc では検査しない。プラグインの `check`（`docs/plugin-protocol.md`「check」）が返す。

## Data

`Diagnostic.Data`（`docs/diagnostics.md`「構造」）を持つ規則。

| ID | Data |
|---|---|
| THROWS-1 | `{ "missing": [FQN] }` |
| EFF-2 | `{ "missing": [FQN], "extra": [FQN] }` |
| MATCH-2 | `{ "missing": [ケース名], "extra": [ケース名] }` |
| BIND-4 | `{ "missing": [パス] }` |
| RES-1 | `{ "candidates": [FQN] }`（`docs/resolve.md`「診断」） |
