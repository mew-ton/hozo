# 型

## この文書が決めること

値がどの位置に渡せるか（互換）と、式の型の決め方。`docs/ir.md` の `Type` の上で定義する。違反はこの文書の「診断」の `TYPE-*`。

## 型の種類

`docs/ir.md`「型」の通り。加えて、式の型として `none` の型（どの `optional` にも入る）と、レコードリテラルの型（フィールド名と値の型の組の列。model に照合して初めて型が決まる）がある。

## 等しい

二つの `Type` は、構造が等しければ等しい。`ref` は FQN が等しいとき。`case` は `classify` と `cases` の集合と `of` が等しいとき。

## 互換

S の値を T の位置に渡せるのは、次のどれかを満たすとき。再帰的に適用する。

1. S と T が等しい。
2. T が `optional(T')` で、S が T' に渡せる。
3. S が `none` の型で、T が `optional`。
4. S が `case(C, cases, of)` で、T が `of`（絞り込みを捨てる）。
5. S が `case(C, s, of)`、T が `case(C, t, of)` で、s ⊆ t（§10.2「まとめたアームではケースの和の型」。狭い方は広い方に渡せる）。
6. S が `array(S')`、T が `array(T')` で、S' が T' に渡せる。
7. S が `record(K1, V1)`、T が `record(K2, V2)` で、K1 = K2 かつ V1 が V2 に渡せる。
8. S と T がともに model への `ref` で、S が T に**射影**できる（下）。
9. S がレコードリテラルで、T が model への `ref`（または model の `optional`）で、リテラルが T に**当てはまる**（下）。
10. S がリテラルの型で、T が対応する核の型（`string`→`string`、`int`→`int` または `float`、`float`→`float`、`boolean`→`boolean`、`duration`→`Duration`）。
11. S が `scalar` への `ref` で、S の `base` が T に渡せる（`Email` を `string` に。値の型は元の型の部分集合）。

渡せないもの（明示）:

- 核の型の値を `scalar` の位置に渡す（TYPE-1。§6.5「元の型の値を値の型の位置に渡すには、検証を通す必要がある」）。配線にはその手段がないので、配線では常にエラー。
- `scalar` の値を別の `scalar` の位置に渡す（`Token` を `UserId` に）。元の型が同じでも別の型。
- `union`（`arms`）に、アームの型の値を直接渡す。配線にユニオンの値を作る手段はなく、ユニオンを返すのは葉の op（§11.2 の VM 関数）。
- `union`（`set`）は値の型ではない。型の位置に書いたら RES-4。

### 射影（model S を model T に）

§7.1「出力は宣言した型に自動で射影される」。T の各フィールド f について、次のどちらか。

- S に同じ名前のフィールドがあり、S.f の型が T.f の型に渡せ、T.f が必須（`optional` でなく `default` もない）なら S.f も必須。
- S に同じ名前のフィールドがなく、T.f に `default` がある（既定値で埋める）。

S にあって T にないフィールドは落ちる。

### 既定値

フィールドの `default`（`page: PosInt = 1`。§11.2）のリテラルは、フィールドの型が `scalar` への `ref` ならその `base`（`scalar` の継承を辿った核の型）に、そうでなければフィールドの型に渡せること（規則 10）。`scalar` の位置にリテラルを渡せる唯一の場所で、値は設計者が書く定数だから。`valid` の制約に合うかは既定の出力が生成時に確かめる（`docs/emit-go.md`「model」）。合わなければ TYPE-13。

### 当てはまる（レコードリテラルを model T に）

- リテラルの各フィールドが T にあり、値の型が T のフィールドの型に渡せる。T にないフィールドがあれば TYPE-5。
- T の必須のフィールド（`optional` でなく、`default` もない）が、リテラルにあるか、`bind` で埋まる位置（`Call.bound` のパス）である。なければ BIND-4（`docs/check.md`）。
- 同じフィールドが二度あれば TYPE-6。

## 式の型

| 式 | 型 |
|---|---|
| 束縛名 | 束縛した式の型 |
| 引数名 | 引数の型 |
| フィールド参照 `x.f` | x の型が model への `ref` なら、そのフィールドの型。x の型が `optional(model)` なら `optional(フィールドの型)`。それ以外は RES-3 |
| リテラル | リテラルの型 |
| `none` | `none` の型 |
| レコードリテラル | 当てはめる先が決まるまで未定。引数の位置か `return` の位置で決まる。位置が決まらない（`const x = { ... }`）なら TYPE-7 |
| 呼び出し | 呼び出した op の `returns`。`null`（void）なら「値がない」（WIRE-2 で区別する） |
| `match`（値でも状態でも同じ。状態の `match` にはアームでの絞り込みがない） | アームの本体の型を集めて: 値のある呼び出しがなければ（「値がない」と `none` だけなら）「値がない」。値のある呼び出しの型がすべて等しい型 T なら、`none` のアームがあれば `optional(T)`、なければ T。等しくなければ TYPE-8。「値がない」と値のある呼び出しが混ざれば TYPE-8 |

## デコレータの引数

`Application.args` の値が `Param.type` に合うかの規則。`docs/resolve.md` で種類が決まった後に適用する。

| `ParamType` | 受け付ける `ArgValue` |
|---|---|
| `type` の `Type` | `literal` で、リテラルの型が渡せるもの（`scalar` の位置には渡せない。TYPE-1）。`list` は `array` の位置に。`ref` で op の引数（`$param:`。`docs/resolve.md`「手続き」1.3）なら、その引数の型が渡せること |
| `any exception` | `ref`（例外）、`refs`（例外の集合）、`ref`（`set` のユニオンで要素が例外。展開して `refs` にする） |
| `any effect` | `ref`（効果）、`refs`、ユニオン（同上） |
| `any effect on s.op` | 上のうち、すべての効果の宛先（`Effect.target`）が組み込みの効果で、その `state` が `s`、`operation` が `op` のもの。遷移の効果や、違う状態・操作なら DEC-7 |
| `any state` | `ref`（状態） |
| `any op` | `ref`（op） |
| `any model` | `ref`（model） |
| `any variant` | `variant` |
| ユニオンへの `ref`（`error: ApiError`） | `ref` で、そのユニオンの `members` に含まれる例外か効果。`refs` なら全要素が含まれる。含まれなければ DEC-7 |

引数の数と名前:

- 必須の `Param` が渡されていなければ DEC-11。
- 宣言にない引数名、または位置で渡した引数が `Param` の数を超えれば DEC-11。
- 同じ引数に二度渡せば DEC-11。

## 診断

| ID | 意味 |
|---|---|
| TYPE-1 | 核の型（リテラルを含む）の値を `scalar` の位置に渡した |
| TYPE-2 | `valid` の制約を `scalar` とフィールド以外に付けた（DEC-1 で検出されるので、ID は索引として残す） |
| TYPE-3 | `return` の値の型が op の `returns` に渡せない（射影もできない） |
| TYPE-4 | 引数の値の型が引数の型に渡せない。どの規則で失敗したかを示す |
| TYPE-5 | レコードリテラルに、当てはめる model にないフィールドがある |
| TYPE-6 | レコードリテラルに同じフィールドが二度ある |
| TYPE-7 | レコードリテラルの当てはめる先が決まらない |
| TYPE-8 | `match` のアームの型がそろわない |
| TYPE-9 | `scalar X extends B` の `B` が核の型でも `scalar` でもない。または `scalar` の継承が循環している |
| TYPE-10 | `model P is U omit f` の `f` が `U` にない |
| TYPE-11 | `Record<K, V>` の `K` が `string` でも、元が `string` の `scalar` でもない |
| TYPE-12 | `C.Case<T>` の `T` が `C` の `subject` に渡せない |
| TYPE-13 | 既定値のリテラルがフィールドの型（`scalar` なら元の型）に渡せない |
| LIT-1 | 整数リテラルが 64 ビット符号付きの範囲を超える。`Duration` がナノ秒で同じ範囲を超える |
