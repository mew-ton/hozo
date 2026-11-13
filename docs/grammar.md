# Hozo の文法

## この文書が決めること

`.hozo` の字句と構文を決める。パーサー（`internal/syntax`）とシンタックスハイライトの文法（`editors/vscode/syntaxes/`）はこの文書から書く。構文の意味（何が正しい宣言か）は設計書（`mew-ton/drafts` の `hozo/README.md`。「§n」はその節）が持ち主で、この文書は設計書の例がすべて通る形になるように書いた。例から決められなかったことは「決めたこと」に、設計書の例と両立しなかったことは「設計書へ返すこと」にある。

記法は EBNF。`=` で定義、`|` で選択、`[ ]` は省略可、`{ }` は 0 回以上の繰り返し、`" "` は終端記号、`( )` はまとまり。

## 字句

### 空白とコメント

空白と改行は区切りとしてだけ意味を持つ。文の終わりは `;` で、改行では終わらない。

```
LineComment  = "//" { 改行以外の文字 } ;
BlockComment = "/*" { "*/" 以外 } "*/" ;              (* 入れ子にできない *)
DocComment   = "/**" { "*/" 以外 } "*/" ;
```

`LineComment` と `BlockComment` は捨てる（§3.4「`//` と `/* ... */` はただのコメントで、IR に入らない」）。`DocComment` は字句として残し、直後の宣言・フィールド・ユニオンのメンバー・分類のケース（`default` を含む）に付く（§3.4「直後の宣言の『説明』として IR に入る」）。付く先がない `DocComment` は構文エラー（「決めたこと」）。

### 識別子

```
Ident         = ( 英字 | "_" ) { 英字 | 数字 | "_" } ;
QualifiedName = Ident { "." Ident } ;
FieldName     = Ident | 予約語 ;
Path          = Ident { "." FieldName } ;
```

- 英字と数字は ASCII に限る（「決めたこと」）。予約語は `Ident` にならない。
- `FieldName` はフィールド名を書く位置の名前で、予約語も書ける（`op: string;`、`{ op: x }`、`res.op`）。使う位置は、model のフィールド（「model」の `Field`。その場で書く model も同じ）、`omit` の並び、レコードリテラルのキー、値のパスの二つ目以降の要素（「配線」）。理由は「決めたこと」。
- `Path` は値を書く位置の名前。先頭は束縛か引数の名前（`Ident`）、残りはその値のフィールド名。構文木の形は `QualifiedName` と同じ。
- 字句解析は、先頭が予約語でなければ `.` で続く要素を予約語も含めて一つの字句にまとめ、`QualifiedName` か `Path` かは構文解析が位置で決める。`QualifiedName` の位置で二つ目以降の要素が予約語なら構文エラー。

### 予約語

構文の骨になる語を予約する。宣言の頭、節の頭、文の頭、核の型、リテラル。予約語は `Ident` の位置（宣言の名前、引数の名前、束縛の名前、名前空間や型の名前の要素、呼び出す op の名前）には書けず、`FieldName` の位置にだけ書ける（「識別子」）。

| 種類 | 語 |
|---|---|
| 宣言の頭 | `namespace` `model` `scalar` `union` `state` `op` `exception` `effect` `classify` `condition` `decorator` `bind` |
| 節の頭 | `is` `omit` `extends` `of` `on` `to` `by` `for` `effects` `throws` `prevents` `keeps` `catches` `recovers` `halts` `ignores` `any` |
| 文の頭と値 | `const` `return` `match` `default` `none` |
| 核の型と状態の形 | `string` `boolean` `int` `float` `Time` `Duration` `void` `Record` `list` `nullable` `single` |
| リテラル | `true` `false` |

次の語は予約しない。決まった位置でだけ意味を持ち、それ以外では `Ident` になる（文脈キーワード）。

| 語 | 位置 |
|---|---|
| `field` `variant` | デコレータの `for` の対象（`field`）と `any` の種類（`variant`） |
| `post` `put` `delete` | `effect ... on 状態.操作` と `any effect on 状態.操作` の操作 |

`@external` は予約語でもデコレータの宣言でもなく、核の印。デコレータの宣言の上にだけ書ける（「decorator の宣言」）。

### リテラル

```
String   = '"' { '"' と "\" 以外の文字 | Escape } '"' ;
Escape   = "\" ( '"' | "\" | "n" | "t" | "r" ) | "\u{" 16進数 { 16進数 } "}" ;
Int      = [ "-" ] 数字 { 数字 } ;
Float    = [ "-" ] 数字 { 数字 } "." 数字 { 数字 } ;
Bool     = "true" | "false" ;
Duration = 数字 { 数字 } ( "ms" | "s" | "m" | "h" | "d" ) ;
Literal  = String | Int | Float | Bool | Duration ;
```

`Duration` は §4.1「`5m` / `30s` のようなリテラルを核の構文として書くため」のもの。単位の選び方と、`Time` のリテラルがないことは「決めたこと」。

### 記号

```
"{" "}" "(" ")" "[" "]" "<" ">" ":" ";" "," "." "=" "?" "|" "@" "*" "**" "=>" "->"
```

`T[]` の `[]` は `"["` `"]"` の並び。`**` は `bind` の対象（§5.3 `server.**`）でだけ使う。

## ファイルと宣言

```
File = { Decl } ;

Decl = [ DocComment ] { Decorator } Declaration
     | [ DocComment ] NamespaceDecl
     | BindDecl ;

Declaration = ModelDecl | ScalarDecl | UnionDecl | StateDecl | OpDecl
            | ExceptionDecl | EffectDecl | ClassifyDecl | ConditionDecl | DecoratorDecl ;

NamespaceDecl = "namespace" QualifiedName "{" { Decl } "}" ;
```

- `namespace` の外の宣言はグローバル（§3.2）。
- `namespace server.admin { }` と `namespace server { namespace admin { } }` はどちらも書ける（§3.2）。同じ名前の `namespace` は何度でも書ける。
- デコレータはどの `Declaration` の前にも書けるが、付けられる対象はデコレータの宣言の `for` で決まる（検査 DEC-1）。`namespace` と `bind` には付けられない（文法で閉じる。`namespace` の理由は「決めたこと」）。
- `DocComment` → デコレータ → 宣言の順（§3.4）。デコレータの後ろに `DocComment` は置けない。

### デコレータの適用

```
Decorator = "@" QualifiedName [ Args ] ;
Args      = "(" [ Arg { "," Arg } ] ")" ;
Arg       = [ Ident ":" ] ArgValue ;
ArgValue  = ArgTerm { "|" ArgTerm }
          | "[" [ ArgValue { "," ArgValue } ] "]" ;
ArgTerm   = Literal | QualifiedName ;
```

- 位置で渡す引数（`@http.status(401)`）と名前で渡す引数（`@http.header(name: "Authorization")`）を混ぜられる（`@page.route("/orders", from: query, onInvalid: transitionToOrders)`）。名前で渡す引数の後ろに位置で渡す引数は書けない（「決めたこと」）。
- `ArgTerm` の `|` は宣言の集合（`http.NetworkError | http.ServerError`。§11.2）。`[ ]` はリスト（`["orders", query]`。§11.1）。
- `QualifiedName` が何を指すか（宣言、op の引数、デコレータの引数）は名前の解決が決める。

### model

```
ModelDecl   = "model" Ident ( ModelBody | "is" QualifiedName "omit" FieldName { "," FieldName } ";" ) ;
ModelBody   = "{" { Field } "}" ;
Field       = [ DocComment ] { Decorator } FieldName ":" Type [ "=" Literal ] ";" ;
```

- `model PublicUser is User omit hash;`（§5.2）。`omit` なしの `is` は書けない（「決めたこと」）。
- 省略可能なフィールドは型の `?` で書く（`token: Token?`）。名前に `?` を付ける書き方はない（「決めたこと」）。
- 既定値（`page: PosInt = 1`。§11.2）は `Literal` だけ。

### scalar

```
ScalarDecl = "scalar" Ident "extends" Type ";" ;
```

### union

```
UnionDecl   = "union" Ident "{" { UnionMember } "}" ;
UnionMember = [ DocComment ] ( Ident ":" ArmType ";" | QualifiedName ";" ) ;
ArmType     = Type | ModelBody ;
```

`Ready: { userName: string; }`（アームと型）、`Unavailable: OrdersUnavailable;`（アームと宣言済みの model）、`server.Unauthorized;`（例外や効果の集合。§6.4）の三つの書き方。

### state

```
StateDecl = "state" Ident ":" ( "list" | "nullable" | "single" ) "<" Type ">" ";" ;
```

形は列数の範囲（`list` は 0 以上、`nullable` は 0 か 1、`single` はちょうど 1）。要素の型に `?` は付けられず（検査 STATE-3）、空になりうる状態は `nullable<T>` と書く（`docs/check.md`「状態の形」）。

### exception / effect

```
ExceptionDecl = "exception" Ident [ ":" ArmType ] ";" ;
EffectDecl    = "effect" Ident "on" ( BuiltinEffect | Transition ) ";" ;
BuiltinEffect = QualifiedName "." StateOp ;
StateOp       = "post" | "put" | "delete" ;                 (* 文脈キーワード *)
Transition    = QualifiedName ":" CaseSet "->" CaseSet ;
CaseSet       = "any" | Ident { "|" Ident } ;
```

効果の宛先は一つだけ。基底の状態の組み込みの効果（`effect addCartLine on carts.post;`）か、分類の遷移（`effect placeOrder on Checkout: Shopping -> Ordered;`）。`Transition` の `QualifiedName` は、状態を対象にした `classify` か、組み込みの分類を持つ基底の状態（`effect clearCart on carts: HasItems -> Empty;`）。`QualifiedName` の直後が `:` なら `Transition`、そうでなければ最後の要素が `StateOp`。

`exception ClientError: Violations;` と `exception X: { items: Violation[] };`（§6.1）。その場で書く model（`ModelBody`）を置けるのは、ユニオンのアームとここの二か所だけ（「決めたこと」）。

`sessions.post` は字句では一つの `QualifiedName` になる（`post` は文脈キーワードで、字句としては `Ident`）。構文解析で最後の要素を `StateOp` として切り出す。`AnyType` の `on location.put` も同じ。

### classify / condition

```
ClassifyDecl    = "classify" Ident "of" ClassifySubject "{" { ClassifyCase } "}" ;
ClassifyCase    = [ DocComment ] ( Ident | "default" ) ";" ;
ConditionDecl   = "condition" Ident "of" ClassifySubject ";" ;
ClassifySubject = Type { "," Type } ;
```

対象が一つで型を指せば値の分類（§10.1）。対象のすべてが今の名前空間の状態の名前なら状態の分類（`classify Checkout of carts, orders { ... }`）。どちらかは名前の解決で決まり、混ざっていればエラー（検査 CLS-3）。

### decorator の宣言

```
DecoratorDecl = [ "@external" ] "decorator" Ident [ Params ] [ "for" Target { "|" Target } ] { DecoratorClause } ";"
              | "decorator" Ident "=" QualifiedName Args ";" ;

Params    = "(" [ Param { "," Param } ] ")" ;
Param     = Ident ":" ParamType ;
ParamType = AnyType [ "?" ] | Type { "|" Type } ;      (* 省略可能は型の ? で書く。any の種類にも付けられる（from: any state?） *)
AnyType   = "any" AnyKind [ "on" QualifiedName "." StateOp ] ;
AnyKind   = "exception" | "effect" | "state" | "op" | "model" | "variant" ;   (* variant は文脈キーワード *)
Target    = "op" | "field" | "state" | "exception" | "scalar" | "model" ;     (* field は文脈キーワード *)

DecoratorClause = ThrowsClause | CatchesClause ;
ThrowsClause    = "throws" ExcRef { "|" ExcRef } { Relation } ;
Relation        = ( "prevents" | "keeps" ) "effects" ;
CatchesClause   = "catches" ExcRef { "|" ExcRef } ( "recovers" | "halts" QualifiedName | "ignores" ) ;
ExcRef          = QualifiedName ;
```

- `@external` は核の印で、デコレータの宣言にだけ付く（§8.2「境界の外から見たときの型の変化」）。`decorator` の直前に置き、引数を取らず、別名の形には付けられない。ほかの位置に書いたら構文エラー。
- 二つ目の形が名前付きのデコレータ（§8.1 `decorator redirectToLogin = redirectOn(server.Unauthorized, transitionToLogin);`）。`for` も節も持たない（汎用の側の宣言に従う）。
- `ExcRef` は例外、例外のユニオン、またはデコレータの引数の名前（`catches error recovers`。§4.2）。
- `int | float`（§4.2 `valid.min`）のように、デコレータの引数の型にだけ `|` を書ける。model のフィールドや op の引数の型には書けない。
- `ThrowsClause` と `CatchesClause` の順序と回数は文法では縛らない（`catches X recovers throws Y`。§8.1）。差し引きは検査が計算する（DEC-6）。

### op

```
OpDecl     = "op" Ident OpParams [ ":" ReturnType ] { OpClause } ( ";" | Block ) ;
OpParams   = "(" [ OpParam { "," OpParam } ] ")" ;
OpParam    = Ident ":" Type ;
ReturnType = Type | "void" ;
OpClause   = "effects" QualifiedName { "|" QualifiedName }
           | ThrowsClause ;
```

- `;` で終われば葉、`Block` を持てば配線（§7.1）。
- `ReturnType` を省略するのと `void` は IR で同じ（§7.1）。
- `ThrowsClause` は何度でも書ける（§7.3「状態の違う例外は `throws` を複数行に分けて書く」）。`Relation` は、その節に並べた例外すべてに掛かる。

### bind

```
BindDecl = "bind" Ident "to" QualifiedName "." ( "*" | "**" ) ";" ;
```

## 型

```
Type        = TypePrefix { "[" "]" | "?" } ;
TypePrefix  = CoreType
            | "Record" "<" Type "," Type ">"
            | QualifiedName [ "<" Type ">" ] ;
CoreType    = "string" | "boolean" | "int" | "float" | "Time" | "Duration" ;
```

- `QualifiedName "<" Type ">"` は分類のケースで絞り込んだ型（`WarrantyKind.Yamada<server.Item>`。§10.2）にだけ使う。ほかの名前に型引数を付けたらエラー（名前の解決で判定する）。
- その場で書く model（`ModelBody`）は `Type` ではなく、`ArmType`（ユニオンのアームと例外の値）にだけ置ける。
- `void` は `ReturnType` にだけ書ける。`list` / `nullable` / `single` は `StateDecl` と、`match` の `by` の後ろ（`list` / `nullable`）にだけ書ける。

## 配線

```
Block = "{" { Stmt } "}" ;
Stmt  = "const" Ident "=" Expr ";"
      | "return" Expr ";"
      | Call ";"
      | Match ";" ;

Expr  = Call | Match | Atom ;
Call  = QualifiedName "(" [ Atom { "," Atom } ] ")" ;
Atom  = Path | Literal | "none" | RecordLit ;
RecordLit   = "{" [ RecordField { "," RecordField } [ "," ] ] "}" ;
RecordField = FieldName ":" Atom | Ident ;

Match = "match" [ Atom ] "by" ( Ident | "list" | "nullable" ) "{" MatchArm { MatchArm } "}" ;
MatchArm = ( Ident { "|" Ident } | "default" ) "=>" ( Call | "none" ) ";" ;
```

`match` の形は三つ（どれに当たるかの検査は MATCH-7）。

| 形 | 対象 | 分類 |
|---|---|---|
| `match item by WarrantyKind { ... }` | 値 | 値の分類 |
| `match carts by list { ... }`、`match token by nullable { ... }` | 今の名前空間の基底の状態 | その形の組み込みの分類（ケースは `Empty` / `HasItems`） |
| `match by Checkout { ... }` | 書かない（分類の宣言が対象を持つ） | 状態の分類 |

- 値を省いた `RecordField`（`{ item }`）は、同じ名前の束縛か引数を指す。予約語の名前の束縛や引数は作れないので、予約語のキーは `{ op: 値 }` と書く。
- `Call` の引数は `Atom` だけ（§9「呼び出しの引数に呼び出しは書けない（A 正規形）」）。文法で閉じる。
- `Atom` の `Path` は、束縛した名前・op の引数・そのフィールド（`query.page`）のどれか。何を指すかは名前の解決が決める。
- `RecordField` の `{ item }` は `{ item: item }` の略（§10.2 `server.addToCart({ item })`）。
- `match` のアームの本体は `Call` か `none` だけ（§10.2「各アームの本体は呼び出し一つか `none`」）。文法で閉じる。
- `const warranty = match ... { ... };` のように、`match` の `}` の後にも `;` が要る。

## ドキュメンテーションコメント

```
DocComment の中身 = { 本文の行 } { タグの行 } ;
タグの行 = "@param" Ident 説明 | "@throws" QualifiedName 説明 | "@see" QualifiedName ;
```

- 各行の先頭の空白と `*` と、その後ろの空白一つを落としてから読む。
- 本文は Markdown で、最初の段落が要約（§3.4）。
- タグは `@param` / `@throws` / `@see` だけ（§3.4）。ほかの `@` で始まる行は本文として扱う。タグの行の後ろに続くタグでない行は、直前のタグの説明の続き。
- 説明は省略できる。
- タグの形が崩れている行（名前がない、`@param` の名前が修飾されている、`@see` に説明がある）は構文エラーだが、宣言は捨てない。
- `bind` には付けられない（「ファイルと宣言」の `Decl`）。

## 決めたこと

設計書の例からは決められず、この文書で決めたこと。

| 決めたこと | 理由 |
|---|---|
| `to` を予約語にする | `bind x to ...` の節の頭だから。ほかの節の頭と同じ扱い |
| 予約語をフィールド名に書ける（`FieldName`） | `{ "op": "foo" }` のようなオブジェクトを返す API を、そのままの名前で書けるようにする。フィールド名の位置は `:` の前か `.` の後ろで、予約語が来ても構文があいまいにならない |
| `field` `variant` `post` `put` `delete` は文脈キーワード | 節の操作対象（名詞）で、識別子が来る位置と同じ。§6.1 の `model Violation { field: string; }` が通る |
| 空になりうる状態は `nullable<T>` とだけ書き、`single<T?>` は書けない | 同じ意味の書き方を二つ持たない。「空」が入れ物と値の型の二か所に現れないようにする |
| 状態の分類での `match` は対象を書かない（`match by C`） | 対象は分類の宣言が持っていて、書かせると二か所になる |
| `prevents` / `keeps` の目的語は `effects` だけ | 葉の op が直に持つ効果は高々一つ（`docs/check.md` EFF-3）なので、効果を名前で書き分ける場面がない |
| デコレータは `namespace` に付けられない。`for namespace` という対象もない | 名前空間は、複数のファイルの複数のブロックに分けて宣言できる（TypeScript の `interface` と同じく、同じ名前のブロックは一つにまとまる）。名前空間に付けたデコレータは、ファイルをまたいで一つにまとめて管理することになり、どのブロックに何が付いているかが直感的でない。印は中の宣言（op、model など）に付ける |
| `@external` は宣言ではなく核の印 | 語彙でも宣言できず、付く先がデコレータの宣言だけなので、文法で閉じる |
| その場で書く model はユニオンのアームと例外の値にだけ置ける | 合成する名前（`docs/ir.md`「正規化」）がその二つにしか決まらない |
| 省略可能は型の `?` だけで表し、名前に付ける `x?: T` は書けない | 同じ意味の書き方を二つ持たない。型の `?` は `Record<string, int?>` や `Item?[]` のように、名前のない型の位置でも要るので、名前の側を落とす。設計書の例の `x?: T` は `x: T?` に直す（`docs/decisions.md`「設計書へ返すこと」） |
| 識別子は ASCII | §3.3 の命名規則が ASCII の大文字小文字で書かれている |
| `Duration` の単位は `ms` `s` `m` `h` `d` | 例にあるのは `m` と `s`。秒未満と日は使う場面が見えるので足す。週以上は月と年が一定でないので持たない |
| `Time` のリテラルはない | 例にない。時刻を仕様に直接書く場面がない |
| 付く先のない `DocComment` はエラー | 説明が IR から落ちることに気づけるようにする（§2「書き忘れ・曖昧さは構文や検査器で検出し」） |
| `model A is B;`（`omit` なし）は書けない | 別名にしかならず、§2「名前を持つ概念は…名前と説明と関係だけを宣言する」のどれでもない |
| 名前で渡す引数の後ろに位置で渡す引数は書けない | 対応が読めなくなる。TS や Python と同じ |
| `return` の後ろに `Atom` も書ける | `return me;` のような、束縛した値をそのまま返す配線を禁じる理由がない |

## 設計書へ返すこと

設計書の例と両立しなかったことは、`docs/decisions.md`「設計書へ返すこと」の「省略可能の書き方」「ユニオンのアームの中の `;`」「予約語のフィールド名」。`to` を予約語にしたことと衝突する §8.1 の `redirectOn` の引数名 `to` は、例ごと今のスコープの外に置いた（同「スコープ外にした設計書の例」）。
