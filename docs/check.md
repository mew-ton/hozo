# 検査

## この文書が決めること

`hozoc check` と `hozoc lsp` が IR（`docs/ir.md`）の上で行う計算と、規則ごとの検査の手続き・診断を決める。規則の ID は `docs/requirements.md`「検査の規則の一覧」のもので、規則の意味の持ち主は設計書の節。ここでは手続きだけを書く。名前の解決（`RES-*`）と型（`TYPE-*`）は `docs/resolve.md`、`docs/types.md` が持ち主。

検査は IR が作れた後に行う。IR を作る途中（解析、解決、正規化）の誤りがあるファイルでも、作れた部分には検査を行う（言語サーバーのため。`docs/requirements.md`「言語サーバー」）。一つの検査で見つかる誤りはすべて報告する。

## 計算

規則の検査に先立って、op ごとに次を計算し、`Op.signature` に置く。計算は op の呼び出しの依存の順に行う。配線の呼び出しが循環していれば WIRE-7 で、循環に含まれる op の `signature` は宣言から計算する（本体を無視する）。

記法: op を O、O の名前空間を ns(O)、O に付いたデコレータの適用を内側から順に d₁ … dₙ（`Op.decorators` の逆順。`docs/ir.md`「デコレータの適用」）。各 dᵢ の宣言を D(dᵢ) とし、`$param:` の参照はその適用の引数で置き換えてから使う。

### 例外の集合 Exc(O, v)

名前空間 v から O を呼んだときに受け取りうる例外。

```
E₀ = 葉なら Declared(O)、配線なら Raised(O)
Eᵢ = (Eᵢ₋₁ − Catches(dᵢ)) ∪ Throws(dᵢ, v)
Exc(O, v) = Eₙ
```

- `Declared(O)`: `Op.throws` の例外をすべて集めたもの。
- `Raised(O)`: 本体の各呼び出し c について `Exc(callee(c), ns(O))` を集めたもの（`match` のアームの呼び出しも含む）。配線に `Declared(O)` も足す（多めに書いた分。THROWS-2）。
- `Catches(d)`: D(d) の `catches` の例外。
- `Throws(d, v)`: D(d) の `throws` の例外。ただし D(d) が `external` なら、v ≠ ns(O) のときだけ（それ以外は ∅）。DEC-10。

`Op.signature.exceptions = Exc(O, ns(O))`。`View.ops` では `Exc(O, viewer)`。

### 効果の集合 Eff(O)

```
Eff(葉) = Op.effects ∪ Halts(d₁..dₙ)
Eff(配線) = ∪ Eff(callee(c)) over c ∪ Halts(d₁..dₙ)
```

`Halts(d)`: D(d) の `catches` で `mode: halts` の効果。`Op.signature.effects = Eff(O)`。順序は出現順（本体の行の順、アームの順、デコレータの順）。

### 例外ごとの効果の状態 Rel(O)

例外 X ∈ Exc(O, ns(O)) と効果 e ∈ Eff(O) の組ごとに `prevented` / `kept` / `unknown`。

**葉の op**: 本体がないので宣言から。

- X が `Op.throws` にあり、その `ThrowsClause` の `relations` に e を含む（`all` か名前で）`prevents` があれば `prevented`、`keeps` があれば `kept`。どちらもなければ `unknown`。両方あれば REL-5。
- X がデコレータ dᵢ の `Throws` から来たなら、D(dᵢ) のその `ThrowsClause` の `relations` から同じように決める。`all` はこのとき Eff(O) のすべて。
- X が `Op.throws` にもデコレータにもない（配線でだけありうる）なら「出どころ」から。

**配線の op**: 本体の行から計算する（§9）。行 i の呼び出し c の callee を C とする。X が行 i から出る（X ∈ Exc(C, ns(O))）とき、効果 e の状態は

| e の出どころ | 状態 |
|---|---|
| 行 j < i の呼び出しの Eff | `kept` |
| 行 i の呼び出し C 自身 | Rel(C) における (X, e) の状態。X が C のデコレータ由来なら、そのデコレータの宣言から（葉と同じ） |
| 行 i の `match` の別のアーム | `prevented`（アームは排他） |
| 行 j > i | `prevented` |
| O 自身のデコレータの `halts` | `prevented`（例外が出た時点では起きていない。`halts` で吸収されれば X は Exc から消えるので、ここに来るのは吸収されなかった X） |

X が複数の行から出るなら、状態が行ごとに違いうる。すべての行で同じならその状態、違えば `unknown`。

デコレータが X を `catches ... recovers` / `ignores` で吸収した場合、X は Exc(O) から消えるので Rel には現れない。代わりに「成功のアームでの状態」に影響する（下）。

**O の宣言との照合**（REL-4）: 配線の O に `Op.throws` の `relations` が書かれていれば、計算した状態と比べ、`prevents` なのに `prevented` でない、`keeps` なのに `kept` でない組があれば REL-4。

`Op.signature.relations` には X × e のすべての組を置く。

### 成功のアームでの効果の状態 Succ(O)

効果 e ∈ Eff(O) ごとに `kept` / `unknown`。

```
Succ(葉)(e) = kept（宣言した効果は成功なら起きている）
Succ(配線)(e) = e の出どころの呼び出し C について Succ(C)(e)。複数あれば、すべて kept なら kept、それ以外は unknown
```

その上で、O のデコレータ dᵢ（内側から）が X を `recovers` か `ignores` で吸収するなら、X が吸収される直前の集合に含まれていれば、各 e について Rel の (X, e) が `kept` でなければ Succ(e) = `unknown`。§8.2「効果が不明な例外を recovers で吸収すると、成功のアームでもその効果は不明になる」。`halts` は成功のアームに戻らないので変えない。

### bind

`Bind` b（名前空間 A、状態 s、対象 T）と、A の配線の中の各呼び出し c（callee C、ns(C) が T.namespace と等しい、`deep` なら T.namespace を接頭辞に持つ）について:

1. 埋める型 τ = s の `element`（`optional` なら中身）。
2. C の引数の中で型が τ と等しい位置をすべて集める。探す範囲は、各引数の型と、型が model（または model の `optional`）ならそのフィールドを再帰的に辿ったもの。`array` と `record` の中は辿らない。位置はパス（`["in", "token"]`）で表す。
3. 位置が 0 → 何もしない。2 以上 → BIND-1（b の位置で、C と位置の一覧を示す）。
4. 位置が 1 で、s の `element` が `optional` で、位置の型が `optional` でなく既定値もない → BIND-2。
5. 呼び出し c の引数でその位置が明示されていれば何もしない（BIND-3）。パス上の途中のレコードリテラルが位置のフィールドを含むかで判定する。引数そのものを名前（束縛・引数）で渡しているなら、その中は分からないので「明示されている」とみなす。
6. それ以外なら `Call.bound` に `{ path, state: s }` を足す。

同じ呼び出しの同じ位置に二つの `Bind` が当たれば BIND-5。

### 必須の入力（BIND-4）

各呼び出し c について、callee の各引数 p:

- p の型が `optional` でなく、c の `args` に p がなく、`Call.bound` に `[p]` もなければ BIND-4。
- c の `args` に p がレコードリテラルで渡されていれば、`docs/types.md`「当てはまる」の手続きで、必須のフィールドが `Call.bound` のパスも含めて埋まっているかを再帰的に確かめる。足りなければ BIND-4（パスを示す）。

## 規則

各規則について、見る場所、条件、診断の位置と内容を書く。「重さ」は error か warning。ID の規則の意味は `docs/requirements.md`「検査の規則の一覧」の節を見よ。

### 名前空間

| ID | 重さ | 手続き |
|---|---|---|
| NS-1 | error | 解決の手続きそのもの（`docs/resolve.md`）。修飾なしで別の名前空間の宣言を指したときは RES-1 の候補にその FQN を出す |
| NS-2 | — | 正規化で行う。診断なし |
| NS-3 | error | RES-6 のうち、両方の出どころが標準の語彙かプラグインの語彙（`Span.file` が `hozo:` で始まる）のもの。メッセージにプラグイン名を出す |
| NS-4 | error | `match x by C` の C が現在の名前空間の `classify` でない。型の位置の `C.Case<T>` も同じ |
| NAME-1 | warning | `model` / `scalar` / `exception` / `union` / `classify` とそのケースの名前の先頭が大文字でない。`state` / `op` / `effect` / `decorator` の名前の先頭が小文字でない。`_` で始まる名前も警告。標準の語彙は対象外 |
| DOC-1 | error | `@param` が引数名にない、`@throws` が `Signature.exceptions` にない、`@see` が解決できない（RES-1 と同じ探し方）。デコレータの宣言の `@param` は `Param` の名前 |
| DOC-2 | error | 同じ `@param` 名、同じ `@throws` 例外が二度ある |
| UNION-1 | error | `union` の要素にアーム（`A: T`）と参照（`X;`）が混ざる。または参照の要素に例外と効果が混ざる。または `arms` でアーム名が重複 |
| LIT-1 | error | `docs/types.md` |

### 設定

`docs/config.md` が持ち主。CFG-1 〜 CFG-6 の手続きはそこにある。

### 状態と効果

| ID | 重さ | 手続き |
|---|---|---|
| STATE-1 | error | `Effect.operation` が、`State.shape` が `single` なのに `post` か `delete` |
| STATE-2 | error | 葉の op O の `Op.effects` の効果 e について、e の `state` の名前空間が ns(O) と違う（効果の宣言自体は RES-5 で同じ名前空間に限っているので、効果の名前空間 ≠ ns(O) で判定できる） |
| STATE-3 | warning | どの `effect` からも参照されず、どの `bind` にも使われない `state` |
| EFF-1 | — | 計算（Eff）で行う。診断なし |
| EFF-2 | error | 配線の O で `Op.effects` が空でなく、集合として Eff(O) と等しくない。足りないもの・余分なものを示す |
| EFF-3 | warning | `effect` の宣言が、どの op の `Op.effects` にも、どのデコレータの `halts` にも、どのデコレータの引数（`any effect`）にも使われていない |

### throws

| ID | 重さ | 手続き |
|---|---|---|
| THROWS-1 | error | 配線の O について `Missing = Raised(O) − Declared(O) − ∪Catches(dᵢ) − ∪Throws(dᵢ, ns(O))`。空でなければ、位置は O の名前、メッセージに Missing の FQN と、それぞれがどの行のどの呼び出しから来たかを並べる。`Raised` に足した `Declared(O)` はここでは除いて計算する |
| THROWS-2 | — | 診断なし。多めの宣言は許す |
| REL-1 | error | `Op.throws`（またはデコレータの `throws`）の全節を通して同じ例外が二度ある |
| REL-2 | warning | Eff(O) が空の op の `throws` に `relations` がある。デコレータの `throws` の `relations` は対象外（§7.3「デコレータの宣言は、効果を持たない op に付けた場合は単に使われない」） |
| REL-3 | warning | `relations.effects` が `all` でなく名前の列 |
| REL-4 | error | 計算の「O の宣言との照合」 |
| REL-5 | error | 同じ例外・同じ効果に `prevents` と `keeps` の両方がある |
| REL-6 | error | `relations` の効果名が Eff(O) にない |

### デコレータ

| ID | 重さ | 手続き |
|---|---|---|
| DEC-1 | error | `Application` が付いた先の種類（op / field / state / exception / scalar / model）が、D の `targets` にない。`union` / `effect` / `classify` / `decorator` に付けたら常に DEC-1（対象にできない） |
| DEC-2 | error | `targets` に `op` を含まない宣言が `throws` か `catches` を持つ |
| DEC-3 | — | 文法で閉じる |
| DEC-4 | error | `catches ... ignores` を持つデコレータが、`returns` が `null` でない op に付いている |
| DEC-5 | warning | O のデコレータ dᵢ が X を `recovers` で吸収し、Eff(O) が空でなく、ある e で Rel の (X, e) が `kept` でない |
| DEC-6 | warning | 一つのデコレータの宣言で、`catches` の例外と `throws` の例外に同じものがある |
| DEC-7 | error | `docs/types.md`「デコレータの引数」の、`any effect on` と、ユニオンのメンバーの照合。加えて、`any effect` の引数に渡した効果の `state` の名前空間が、デコレータを付けた op の名前空間と違う（§8.2「`onInvalid` に渡した効果が、今の名前空間が持ち主ではない状態に書き込むならエラー」） |
| DEC-8 | — | 文法で閉じる（別名の形は `= generic(args)` だけ） |
| DEC-9 | — | 正規化 |
| DEC-10 | — | 計算（Throws(d, v)） |
| DEC-11 | error | `docs/types.md`「デコレータの引数」の数と名前 |
| DEC-12 | error | 別名の `target` が別名（別名の別名は作れない。IR を一段で展開するため）。または別名の `args` が `target` の `Param` に合わない（DEC-11 と同じ手続き） |
| DEC-13 | error | 同じデコレータが同じ宣言に二度付いている（引数が違っても） |
| DEC-14 | warning | `external` なデコレータが `throws` を持たない（印として意味がない） |

### 配線

| ID | 重さ | 手続き |
|---|---|---|
| WIRE-1 | — | 文法で閉じる |
| WIRE-2 | error | `const x = call` で callee の `returns` が `null`。`call;` の行で callee の `returns` が `null` でない。`match` も同じ（式の型が「値がない」かどうか）。`return call` で `returns` が `null` |
| WIRE-3 | error | O の `returns` が `null` なのに `return` がある。`returns` が `null` でないのに、本体の最後の文が `return` でない（`return` は最後の文にだけ書ける。途中に書いても下の行が評価されないため） |
| WIRE-4 | error | 同じ名前を二度束縛。引数と同じ名前を束縛。束縛より上の行で使っている（RES-1 として出る） |
| WIRE-5 | — | 文法で閉じる |
| WIRE-6 | warning | 本体の `const` の数が `hozo.toml` の `[hozo].max_bindings`（既定 5）を超える |
| WIRE-7 | error | 配線の呼び出しが循環している。循環上の op をすべて示す |
| WIRE-8 | warning | 束縛した名前が下の行で使われていない |

### classify / match

| ID | 重さ | 手続き |
|---|---|---|
| CLS-1 | error | `classify` に `default` が二つ以上、またはケース名が重複 |
| CLS-2 | error | ケースが一つもない（`default` だけも含む） |
| MATCH-1 | — | 文法で閉じる |
| MATCH-2 | error | アームに並べたケースの集合が `Classify.cases` と等しくない（足りない・知らない名前・重複）。`hasDefault` が false なのに `default` のアームがある。`hasDefault` が true なのに `default` のアームがない |
| MATCH-3 | — | 文法で閉じる |
| MATCH-4 | error | アームの本体の呼び出しで、`subject` をそのまま渡している引数の型が `case(C, アームのケース, T)` に対して `docs/types.md` の規則 4・5 で渡せない。`subject` を渡していない引数は普通の TYPE-4 |
| MATCH-5 | error | アームの本体がすべて「値がない」か `none` の `match` を `const` で束縛している。逆に、値のある `match` を文として書いている（WIRE-2 で出る） |
| MATCH-6 | error | `subject` の型が `Classify.subject` に渡せない |

### 語彙に固有の規則

PAGE-1 / PAGE-2 は hozoc では検査しない。プラグインの `check`（`docs/plugin-protocol.md`）が返す。

## 診断の順序

ファイルのパスの辞書順、位置の順。同じ位置なら ID の辞書順。`docs/diagnostics.md`。
