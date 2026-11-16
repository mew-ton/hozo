# Hozo の文法

## この文書が決めること

`.hozo` の字句と構文を決める。パーサー（`internal/syntax`）とシンタックスハイライトの文法（`editors/vscode/syntaxes/`）はこの文書から書く。構文の意味（何が正しい宣言か）の持ち主は、この文書が節ごとに名指しする文書（`docs/check.md`、`docs/types.md`、`docs/resolve.md`）。構文の形は、設計書（出発点のアイデア集で、仕様の持ち主ではない。`docs/README.md`「設計書という呼び名」。「§n」はその節）の例から始めたもので、この文書で決めたことと、その理由は「決めたこと」にある。設計書の例にない構文（`reads`、`export`、`extends` を持つ model、`as`、`throw`、`never`、形の語を型として書くこと）は、`docs/decisions.md`「設計書から離れた決定」の S15〜S23 が足したもので、意味の持ち主はその表の「持ち主」の列の文書。デコレータの宣言の `reads` / `effects` の節、残りの引数（`...`）、対象 `effect`、予約語のデコレータ名は、同「決めたこと」98 が足したもので、意味の持ち主は `docs/check.md`「デコレータが運ぶ reads と effects」と `docs/types.md`「デコレータの引数」。引数を持つ `exception` / `effect` / `read` の宣言、配線からの `read` と効果の呼び出し、引数を持つ `throw`、`reads` / `effects` に直に書く要素、`throws` / `catches` / `reads` / `effects` の並びの区切り `,`、対象 `read`、核の印 `@repeatable` は、例外、効果、`read` を同じ書き方にする決定（`docs/decisions.md`。段階 R4: `docs/requirements.md`「作り直しの段階」）が足したもので、意味の持ち主は `docs/check.md`「状態に触れるもの」「op の宣言と本体」と `docs/types.md`「呼び出しの引数」。

記法は EBNF。`=` で定義、`|` で選択、`[ ]` は省略可、`{ }` は 0 回以上の繰り返し、`" "` は終端記号、`( )` はまとまり。

## 字句

### 空白とコメント

空白と改行は区切りとしてだけ意味を持つ。文の終わりは `;` で、改行では終わらない。

```
LineComment  = "//" { 改行以外の文字 } ;
BlockComment = "/*" { "*/" 以外 } "*/" ;              (* 入れ子にできない *)
DocComment   = "/**" { "*/" 以外 } "*/" ;
```

`LineComment` と `BlockComment` は捨てる（§3.4「`//` と `/* ... */` はただのコメントで、IR に入らない」）。`DocComment` は字句として残し、直後の宣言・フィールド（`override` のメンバーを含む。「model」）・ユニオンのメンバー・分類のケース（`default` を含む）に付く（§3.4「直後の宣言の『説明』として IR に入る」）。付く先がない `DocComment` は構文エラー（「決めたこと」）。

### 識別子

```
Ident         = ( 英字 | "_" ) { 英字 | 数字 | "_" } ;
QualifiedName = Ident { "." Ident } ;
FieldName     = Ident | 予約語 ;
Path          = Ident { "." FieldName } ;
DecoratorName = Ident | 予約語 ;
DecoratorRef  = [ QualifiedName "." ] DecoratorName ;
```

- 英字と数字は ASCII に限る（「決めたこと」）。予約語は `Ident` にならない。
- `FieldName` はフィールド名を書く位置の名前で、予約語も書ける（`op: string;`、`{ op: x }`、`res.op`）。使う位置は、model のフィールド（「model」の `Field`。その場で書く model も同じ）、`override` の後ろの名前（「model」の `Override`）、レコードリテラルのキー、値のパスの二つ目以降の要素（「配線」）。理由は「決めたこと」。
- `DecoratorName` はデコレータの名前を書く位置の名前で、予約語も書ける（`decorator effects(…)`、`decorator list(…)`）。`DecoratorRef` はデコレータを指す位置の名前で、最後の要素にだけ予約語を書ける（`@db.list`、`@db.delete`。同じ名前空間の中なら `@effects(…)`）。使う位置は、デコレータの宣言の名前、適用の `@` の後ろ、別名の `=` の後ろ（「decorator の宣言」）、ドキュメンテーションコメントの `@see` の後ろ。理由は「決めたこと」。
- `Path` は値を書く位置の名前。先頭は束縛か引数か状態の名前（`Ident`）、残りはその値のフィールド名。構文木の形は `QualifiedName` と同じ。何を指すかは `docs/resolve.md`「手続き」。
- 字句解析は、先頭が予約語でなければ `.` で続く要素を予約語も含めて一つの字句にまとめ、`QualifiedName` か `Path` かは構文解析が位置で決める。`QualifiedName` の位置で二つ目以降の要素が予約語なら構文エラー。`DecoratorRef` の位置では、最後の要素だけが予約語でよく、要素が一つで予約語のとき（`@effects`）は、その予約語の字句をそのまま名前として読む。

### 予約語

構文の骨になる語を予約する。宣言の頭、宣言やフィールドの前に置く修飾、節の頭、文の頭、核の型、リテラル。予約語は `Ident` の位置（デコレータを除く宣言の名前、引数の名前、束縛の名前、名前空間や型の名前の要素、呼び出す op・`read`・効果の名前）には書けず、`FieldName` と `DecoratorName` の位置にだけ書ける（「識別子」）。

| 種類 | 語 |
|---|---|
| 宣言の頭 | `namespace` `model` `scalar` `union` `state` `op` `exception` `effect` `read` `classify` `condition` `decorator` `bind` |
| 修飾 | `export` `override` |
| 節の頭 | `extends` `of` `on` `to` `by` `for` `as` `effects` `reads` `throws` `prevents` `keeps` `catches` `recovers` `halts` `ignores` `any` |
| 文の頭と値 | `const` `return` `match` `default` `none` `throw` |
| 核の型と形 | `string` `boolean` `int` `float` `Time` `Duration` `void` `never` `Record` `list` `nullable` `single` |
| リテラル | `true` `false` |

次の語は予約しない。決まった位置でだけ意味を持ち、それ以外では `Ident` になる（文脈キーワード）。

| 語 | 位置 |
|---|---|
| `field` `variant` | デコレータの `for` の対象（`field`）と `any` の種類（`variant`）。`for` の対象の `effect` と `read` は予約語（宣言の頭）で、その位置でも同じ字句 |
| `post` `put` `delete` | `effect ... on 状態.操作`、`any effect on 状態.操作`、`effects` の要素に直に書く `状態.操作`（「op」）、配線で直に呼ぶ `状態.操作(…)`（「配線」）の操作 |

`@external` と `@repeatable` は予約語でもデコレータの宣言でもなく、核の印。デコレータの宣言の上にだけ書ける（「decorator の宣言」）。

`reads` `read` `as` `never` `throw` `override` `export` の七つは仮の綴りで、後で別の語に変えうる（「決めたこと」）。`is` と `omit` は予約語ではない（`model A is B omit …` をなくしたときに外した。「model」）。

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
"{" "}" "(" ")" "[" "]" "<" ">" ":" ";" "," "." "=" "?" "|" "@" "*" "**" "=>" "->" "..."
```

`T[]` の `[]` は `"["` `"]"` の並び。`**` は `bind` の対象（§5.3 `server.**`）でだけ、`...` はデコレータの宣言の残りの引数（「decorator の宣言」）でだけ使う。

## ファイルと宣言

```
File = { Decl } ;

Decl = [ DocComment ] { Decorator } Declaration
     | [ DocComment ] NamespaceDecl
     | BindDecl ;

Declaration = ModelDecl | ScalarDecl | UnionDecl | StateDecl | OpDecl
            | ExceptionDecl | EffectDecl | ReadDecl | ClassifyDecl | ConditionDecl | DecoratorDecl ;

NamespaceDecl = "namespace" QualifiedName "{" { Decl } "}" ;
```

- `namespace` の外の宣言はグローバル（§3.2）。
- `namespace server.admin { }` と `namespace server { namespace admin { } }` はどちらも書ける（§3.2）。同じ名前の `namespace` は何度でも書ける。
- デコレータはどの `Declaration` の前にも書けるが、付けられる対象はデコレータの宣言の `for` で決まる（検査 DEC-1）。`namespace` と `bind` には付けられない（文法で閉じる。`namespace` の理由は「決めたこと」）。
- `DocComment` → デコレータ → 宣言の順（§3.4）。デコレータの後ろに `DocComment` は置けない。

### デコレータの適用

```
Decorator = "@" DecoratorRef [ Args ] ;
Args      = "(" [ Arg { "," Arg } ] ")" ;
Arg       = [ Ident ":" ] ArgValue ;
ArgValue  = ArgTerm { "|" ArgTerm }
          | "[" [ ArgValue { "," ArgValue } ] "]" ;
ArgTerm   = Literal | QualifiedName ;
```

- 位置で渡す引数（`@http.status(401)`）と名前で渡す引数（`@http.header(name: "Authorization")`）を混ぜられる（`@page.route("/orders", from: query, onInvalid: transitionToOrders)`）。名前で渡す引数の後ろに位置で渡す引数は書けない（「決めたこと」）。
- `ArgTerm` の `|` は、一つの引数に渡す宣言の集合（`http.NetworkError | http.ServerError`。§11.2）。引数の並びの `,` は引数を区切るので、一つの引数の中の集合は `|` のまま（節の並びの区切りは `,`。「op」）。`[ ]` はリスト（`["orders", query]`。§11.1）。
- `QualifiedName` が何を指すか（宣言、op の引数、デコレータの引数）は名前の解決が決める。
- 宣言が残りの引数（「decorator の宣言」の `RestParam`）を持つデコレータには、ほかの引数の数を超えて位置で渡した値が、書いた順に、残りの引数に入る（`decorator effects(...targets: any effect)` に `@effects(createOrder, notifyUser)`）。0 個でもよい。残りの引数には、名前では渡せない（DEC-11。`docs/types.md`「デコレータの引数」）。

### model

```
ModelDecl   = "model" Ident [ "extends" QualifiedName ] ModelBody ;
ModelBody   = "{" { Member } "}" ;
Member      = Field | Override ;
Field       = [ DocComment ] { Decorator } FieldName ":" Type ";" ;
Override    = [ DocComment ] { Decorator } "override" FieldName [ ":" ( Type | "never" ) ] ";" ;
```

- 省略可能なフィールドは型の `?` で書く（`token: Token?`）。名前に `?` を付ける書き方はない（「決めたこと」）。
- フィールドに既定値は書けない（`page: PosInt = 1`。なくした書き方。「決めたこと」）。パーサーは、型の直後に `=` が来たら、値がないときに何を入れるかは境界のデコレータで言うこと（`@http.default(1)`、`@cli.default(1)`）をメッセージで示して SYNTAX にする。
- `extends` を持つ model は、既存の model を継承した別の型（`model PublicUser extends User { override hash: never; }`）。本体が空でもよい（`model Admin extends User { }`）。何を受け継ぎ、本体に何を書けるかは `docs/types.md`「継承した model」。
- `Override` は、元の model のフィールドに触れるメンバー。形は三つ。

  | 形 | 意味 |
  |---|---|
  | `@db.key override id;` | 型を変えず、デコレータを足す |
  | `override status: StatusOrPending;` | 型を変える（変えられる形は `docs/types.md`「継承した model」） |
  | `override hash: never;` | そのフィールドをなくす。`never` はここでは型ではなく、なくす印 |

- メンバーの頭（`DocComment` とデコレータの後ろ）の `override` は、直後が `:` ならフィールドの名前（`override: string;`。`FieldName` は予約語も書ける）、そうでなければ `Override` の頭。`override override: never;` は、`override` という名前のフィールドをなくす。
- `Override` は文法ではどの `ModelBody` にも書けるが、`extends` のない model と、その場で書く model（「union」）には元の model がないので、検査でエラーになる（TYPE-10。`docs/types.md`「診断」）。
- `model A is B omit f;` は書けない（なくした書き方。「決めたこと」）。パーサーは、`model` の名前の直後に `is` という識別子が来たら、`extends` と `override f: never;` で書くことをメッセージで示して SYNTAX にする。

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
StateDecl = "state" Ident ":" Type ";" ;
```

状態は、その名前空間が持つ、型 `Type` の値。`state users: list<User>;`、`state token: nullable<Token>;`、`state title: single<string>;` のように形の語（「型」）で書け、`state token: Token?;`（`nullable<Token>` と同じ）、`state title: string;`（`single<string>` と同じ）とも書ける。形（`list` / `nullable` / `single`）は型から決まる（`docs/check.md`「状態の形」）。

### exception / effect / read

```
ExceptionDecl = "exception" Ident [ "(" OpParam { "," OpParam } ")" ] ";" ;
EffectDecl    = "effect" Ident OpParams "on" ( BuiltinEffect | Transition ) ";" ;
ReadDecl      = "read" Ident OpParams ":" Type "on" QualifiedName ";" ;
BuiltinEffect = QualifiedName "." StateOp ;
StateOp       = "post" | "put" | "delete" ;                 (* 文脈キーワード *)
Transition    = QualifiedName ":" CaseSet "->" CaseSet ;
CaseSet       = "any" | Ident { "|" Ident } ;
```

三つは、op が使う最小の単位で、同じ形で書く: 名前、引数の並び（`OpParams` と `OpParam` は「op」）、宛先。どれも本体を持たず、ドキュメンテーションコメントとデコレータを付けられる（対象 `exception` / `effect` / `read`。「decorator の宣言」）。意味は `docs/check.md`「状態に触れるもの」と「op の宣言と本体」の「throw」。

| 宣言 | 例 | 引数 | 結果 |
|---|---|---|---|
| `exception` | `exception InvalidCredentials;`、`exception LimitExceeded(wait: Duration);` | その例外が運ぶ値 | なし |
| `effect` | `effect createOrder(order: Order) on orders.post;`、`effect clearCart() on carts: HasItems -> Empty;` | その効果が受け取る値 | なし |
| `read` | `read userByEmail(email: Email): User? on users;`、`read allOrders(): Order[] on orders;` | 読むときに受け取る値 | `:` の後ろの型。省略できない |

- `effect` と `read` は、引数がなくても `()` を書く（配線から `名前()` と呼ぶものなので、op と同じ）。`exception` は、引数がなければ括弧を書かない（`throw InvalidCredentials` と投げ、`throws` に名前で並べるもの）。`exception X();` は構文エラー（同じ意味の書き方を二つ持たない）。
- 効果の宛先は一つだけ。基底の状態の組み込みの効果（`on carts.post`）か、分類の遷移（`on Checkout: Shopping -> Ordered`）。`Transition` の `QualifiedName` は、状態を対象にした `classify` か、組み込みの分類を持つ基底の状態（`effect clearCart() on carts: HasItems -> Empty;`）。`on` の後ろの `QualifiedName` の直後が `:` なら `Transition`、そうでなければ最後の要素が `StateOp`。
- `read` の `on` の後ろは基底の状態一つで、操作（`.get` のようなもの）は書かない: 状態を読むことは一種類しかない。
- `read` の結果の型に `void` と `never` は書けない（`Type` だけ）。
- `sessions.post` は字句では一つの `QualifiedName` になる（`post` は文脈キーワードで、字句としては `Ident`）。`EffectDecl` と `AnyType`（`on location.put`）では、構文解析で最後の要素を `StateOp` として切り出す。`effects` の要素（「op」の `EffectElem`）と、配線の呼び出し（「配線」の `Call`）では切り出さず、名前の解決が決める。
- なくした書き方は、新しい書き方をメッセージで示して SYNTAX にする: `exception X: T;`（例外の名前の直後の `:`。運ぶ値は引数で書く: `exception X(value: T);`）、`effect e on s.post;`（効果の名前の直後の `on`。引数の括弧が要る: `effect e() on s.post;`）。

### classify / condition

```
ClassifyDecl    = "classify" Ident "of" ClassifySubject "{" { ClassifyCase } "}" ;
ClassifyCase    = [ DocComment ] ( Ident [ ":" CaseTyping { "," CaseTyping } ] | "default" ) ";" ;
CaseTyping      = Type "as" Type ;
ConditionDecl   = "condition" Ident "of" ClassifySubject ";" ;
ClassifySubject = Type { "," Type } ;
```

対象の種類は三つで、名前の解決で決まる（`docs/resolve.md`「位置ごとに受け付ける種類」）。合わない並びはエラー（検査 CLS-3）。

| 対象 | 例 | 分類の種類 |
|---|---|---|
| 一つで、型を指す | `classify WarrantyKind of server.Item { ... }`、`classify Presence of nullable<User> { ... }` | 値の分類（§10.1） |
| 一つで、op を指す | `classify Registration of findUser { ... }` | op の分類（その op が返した値の分類） |
| すべてが今の名前空間の状態の名前 | `classify Checkout of carts, orders { ... }` | 状態の分類 |

- `CaseTyping` は、そのケースのときに対象が何の型になるかを書く（`Registered: findUser as type.User;`）。`as` の前は対象の一つで、宣言の `of` の後ろと同じに書く（値の分類なら同じ型、op の分類ならその op、状態の分類なら対象の状態の一つ）。`as` の後ろはそのケースでの型。意味と、書ける型は `docs/check.md`「分類」と `docs/types.md`「絞り込み」。
- 状態の分類は、一つのケースに複数の対象の型を `,` で並べられる（`Ready: token as Token, profile as Profile;`）。
- `default` と `condition` には `as` を書けない（文法で閉じる）。

### decorator の宣言

```
DecoratorDecl = { CoreMark } "decorator" DecoratorName [ Params ] [ "for" Target { "|" Target } ] { DecoratorClause } ";"
              | "decorator" DecoratorName "=" DecoratorRef Args ";" ;
CoreMark      = "@external" | "@repeatable" ;          (* 核の印。それぞれ一度まで *)

Params    = "(" [ Param { "," Param } [ "," RestParam ] | RestParam ] ")" ;
Param     = Ident ":" ParamType ;
RestParam = "..." Ident ":" RestType ;                 (* 残りの引数。最後に一つだけ *)
ParamType = AnyType [ "?" ] | Type { "|" Type } ;      (* 省略可能は型の ? で書く。any の種類にも付けられる（from: any state?） *)
RestType  = AnyType | Type { "|" Type } ;              (* 渡す値一つの型。? も [] も付けない *)
AnyType   = "any" AnyKind [ "on" QualifiedName "." StateOp ] ;
AnyKind   = "exception" | "effect" | "state" | "op" | "model" | "variant" ;   (* variant は文脈キーワード *)
Target    = "op" | "field" | "state" | "exception" | "scalar" | "model" | "effect" | "read" ;   (* field は文脈キーワード *)

DecoratorClause = ThrowsClause | CatchesClause | DecoReadsClause | DecoEffectsClause ;
ThrowsClause    = "throws" ExcRef { "," ExcRef } { Relation } ;
Relation        = ( "prevents" | "keeps" ) "effects" ;
CatchesClause   = "catches" ExcRef { "," ExcRef } ( "recovers" | "halts" QualifiedName | "ignores" ) ;
ExcRef          = QualifiedName ;
DecoReadsClause   = "reads" ReadElem { "," ReadElem } ;       (* ReadElem と EffectElem は「op」 *)
DecoEffectsClause = "effects" EffectElem { "," EffectElem } ;
```

- `@external` と `@repeatable` は核の印（`CoreMark`）で、デコレータの宣言にだけ付く。`decorator` の直前に置き、引数を取らず、別名の形には付けられない。二つを並べてよく、順は問わない。同じ印を二度書く、ほかの位置に書く、は構文エラー。
  - `@external`: そのデコレータの `throws` の例外は、境界の外から見たときだけ加わる（§8.2「境界の外から見たときの型の変化」。DEC-10）。
  - `@repeatable`: そのデコレータを、一つの宣言に何度でも付けられる（`@repeatable decorator where(column: string, value: string | int | float | boolean) for read;`）。印のないデコレータは、一つの宣言に一度だけ（DEC-13。`docs/check.md`「デコレータ」）。何度も付けた適用は、書いた順に IR に並ぶ（`docs/ir.md`「デコレータの適用」）。
- 二つ目の形が名前付きのデコレータ（§8.1 `decorator redirectToLogin = redirectOn(server.Unauthorized, transitionToLogin);`）。`for` も節も持たない（汎用の側の宣言に従う）。
- `ExcRef` は例外、例外のユニオン、またはデコレータの引数の名前（`catches error recovers`。§4.2）。
- `int | float`（§4.2 `valid.min`）のように、デコレータの引数の型にだけ `|` を書ける。model のフィールドや op の引数の型には書けない。`for` の対象の並びも `|`（`for scalar | field`）。節の並び（`throws`、`catches`、`reads`、`effects`）の区切りは `,` で、`|` ではない（「op」）。
- `ThrowsClause` と `CatchesClause` の順序と回数は文法では縛らない（`catches X recovers throws Y`。§8.1）。差し引きは検査が計算する（DEC-6）。
- `Target` の `effect` は、効果の宣言に付けるデコレータ（`@db.insert effect createOrder(order: Order) on orders.post;`）。`read` は、`read` の宣言に付けるデコレータ（`@db.find read userByEmail(email: Email): User? on users;`）。`exception` は、例外の宣言に付けるデコレータ（`@http.status(401) exception Unauthorized;`）。
- `DecoReadsClause` と `DecoEffectsClause` は、そのデコレータの処理が読むものと、持つ効果を言う節（`decorator limit(counter: any state, by: any effect) for op reads counter effects by;`）。要素は op の節と同じもの（`ReadElem`、`EffectElem`）か、デコレータの引数の名前（`any state` の引数は読む状態、`any effect` の引数は効果）。何度でも書ける。意味は `docs/check.md`「デコレータが運ぶ reads と effects」。
- `Relation` の `effects` は、直前の `prevents` / `keeps` の目的語。`DecoEffectsClause` の頭の `effects` と字句は同じだが、`prevents` / `keeps` の直後かどうかで分かれる（`throws X prevents effects effects e` は、`Relation` の後ろに `DecoEffectsClause`）。
- `RestParam` は**残りの引数**: 適用が、ほかの引数の後ろに位置で渡した 0 個以上の値を、まとめて受け取る（`decorator effects(...targets: any effect) for op effects targets;`）。型は、渡す値一つの型を書く。引数の並びの最後に一つだけ書け、省略可能にはできない（文法で閉じる）。節の中で名前を書けば、渡した値のすべてを指す。

### op

```
OpDecl     = [ "export" ] "op" Ident OpParams [ ":" ReturnType ] { OpClause } ( ";" | Block ) ;
OpParams   = "(" [ OpParam { "," OpParam } ] ")" ;
OpParam    = Ident ":" Type ;
ReturnType = Type | "void" | "never" ;
OpClause   = "effects" EffectElem { "," EffectElem }
           | "reads" ReadElem { "," ReadElem }
           | ThrowsClause ;
EffectElem = QualifiedName ;      (* 効果の名前か、直に書く「状態.操作」。どちらかは名前の解決が決める *)
ReadElem   = QualifiedName ;      (* read の名前か、直に書く状態の名前。同上 *)
```

- `;` で終われば葉、`Block` を持てば配線（§7.1）。葉は、人が中身を書く op で、状態に触れない: `reads` と `effects` の節を書けない（STATE-5。`docs/check.md`「状態に触れるもの」）。`throws` は書ける。
- `ReturnType` を省略するのと `void` は IR で同じ（§7.1）。
- `never` は、決して値を返さない op（`op unauthorized(): never throws Unauthorized;`）。宣言の規則は `docs/check.md`「never を返す op」。
- `export` は、別の名前空間から呼べる op の印（`export op login(in: LoginInput): LoginOk`）。`DocComment` とデコレータの後ろ、`op` の直前に書く。付けられるのは `op` だけで、ほかの宣言の前に書いたら構文エラー。意味は `docs/check.md`「export」。
- `throws`、`effects`、`reads` の三つは、その op を呼んだときに起きうることの並びで、区切りはどれも `,`（`throws Unauthorized, NotFound`、`effects createOrder, notifyUser`、`reads users, sessions`）。どの節も何度でも書け、同じ要素の重複は一つと数える。
- 配線の op の `effects` と `reads` は、書かなくてよい: 本体が行う効果と読む状態は、本体の呼び出しから決まる。書いた場合は、それと照合する（EFF-2、READS-2。`docs/check.md`「状態と効果」）。
- `effects` の要素（`EffectElem`）は、`effect` の宣言の名前（効果のユニオンの名前でもよい）か、基底の状態の組み込みの効果を直に書いた `状態.操作`（`effects users.post`）。分類の遷移は、直には書けない（名前の付いた `effect` の宣言にだけ書ける）。
- `reads` の要素（`ReadElem`）は、`read` の宣言の名前か、直に書いた状態の名前（`reads users`）。
- 要素が名前か、直に書いたものかは、名前の解決が決める（`docs/resolve.md`「位置ごとに受け付ける種類」）。節の要素と、配線の本体が行ったものとの対応は `docs/check.md`「節に書く要素」、`reads` の意味は同「reads」。
- パーサーは、`throws` / `catches` / `effects` / `reads` の並びの要素の後ろに `|` が来たら、区切りは `,` であることをメッセージで示して SYNTAX にし、`,` として読み進める（なくした書き方。「決めたこと」）。
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
            | ( "list" | "nullable" | "single" ) "<" Type ">"
            | QualifiedName [ "<" Type ">" ] ;
CoreType    = "string" | "boolean" | "int" | "float" | "Time" | "Duration" ;
```

- 形の語は型の別の書き方: `list<T>` は `T[]`、`nullable<T>` は `T?`、`single<T>` は `T`。`Type` を書けるどの位置にも書け、IR では同じ型になる（`docs/types.md`「形の書き方」）。

- `QualifiedName "<" Type ">"` は分類のケースで絞り込んだ型（`WarrantyKind.Yamada<server.Item>`。§10.2）にだけ使う。ほかの名前に型引数を付けたらエラー（名前の解決で判定する）。
- その場で書く model（`ModelBody`）は `Type` ではなく、`ArmType`（ユニオンのアーム）にだけ置ける。
- `void` と `never` は `ReturnType` にだけ書ける（`never` は、ほかに `Override` のなくす印として書く。「model」）。`match` の `by` の後ろの `list` / `nullable` は型ではなく、標準の分類の名前（「配線」）。

## 配線

```
Block = "{" { Stmt } "}" ;
Stmt  = "const" Ident "=" Expr ";"
      | "return" Expr ";"
      | Call ";"
      | Match ";" ;

Expr  = Call | Match | Atom ;
Call  = QualifiedName "(" [ Atom { "," Atom } ] ")" ;      (* op、read、効果の名前か、直に呼ぶ「状態.操作」 *)
Atom  = Path | Literal | "none" | RecordLit ;
RecordLit   = "{" [ RecordField { "," RecordField } [ "," ] ] "}" ;
RecordField = FieldName ":" Atom | Ident ;

Match    = "match" [ Atom ] "by" ( QualifiedName | "list" | "nullable" ) "{" MatchArm { MatchArm } "}" ;
MatchArm = ( Ident { "|" Ident } | "default" ) "=>" ArmBody ";" ;
ArmBody  = Call | Atom | Throw ;
Throw    = "throw" QualifiedName [ "(" Atom { "," Atom } ")" ] ;
```

`match` の形は三つ（どれに当たるかの検査は MATCH-7）。

| 形 | 対象 | 分類 |
|---|---|---|
| `match item by WarrantyKind { ... }`、`match user by store.Registration { ... }` | 値 | 値の分類か op の分類 |
| `match carts by list { ... }`、`match in.token by nullable { ... }` | 値。型が `list<T>`（`by list`）か `nullable<T>`（`by nullable`） | 標準の分類（ケースは `Empty` / `HasItems`。`docs/check.md`「分類」） |
| `match by Checkout { ... }` | 書かない（分類の宣言が対象を持つ） | 状態の分類 |

- `Atom` の `Path` は、束縛した名前・op の引数・今の名前空間の状態と、そのフィールド（`query.page`）のどれか。状態は名前で値として書ける（`f(token)`、`match token by nullable { ... }`）。同じ名前の束縛か引数があれば、そちらを指す。何を指すかは名前の解決が決める（`docs/resolve.md`「手続き」）。
- 値を省いた `RecordField`（`{ item }`）は `{ item: item }` の略（§10.2 `server.addToCart({ item })`）で、`item` は `Atom` の `Path` と同じに解決する。予約語の名前の束縛や引数は作れないので、予約語のキーは `{ op: 値 }` と書く。
- `Call` の引数は `Atom` だけ（§9「呼び出しの引数に呼び出しは書けない（A 正規形）」）。文法で閉じる。
- `Call` の `QualifiedName` が指すものは四つで、どれかは名前の解決が決める（`docs/resolve.md`「呼び出しの読み分け」）。意味は `docs/check.md`「状態に触れるもの」の「配線からの呼び出し」。

  | 呼び出し | 例 | 式として |
  |---|---|---|
  | op | `const session = issueSession(user);` | その op の戻り値（`docs/types.md`「式の型」） |
  | `read` | `const user = userByEmail(email);`、`return userByEmail(email);` | その `read` の結果の型の値 |
  | 効果（`effect` の宣言） | `createOrder(order);` | 値がない（文か、アームの本体に書く） |
  | 組み込みの効果を直に（`状態.操作`） | `users.post(user);` | 値がない。名前を付けない効果。書ける形と引数は `docs/check.md`「直に呼ぶ組み込みの効果」 |

- 状態を名前で値として書くこと（上の `Atom` の `Path`）が、名前を付けない `read`。
- `match` の分類の名前は修飾できる（`by store.Registration`）。分類はどの名前空間からも使える（`docs/check.md`「分類」）。
- アームの本体（`ArmBody`）は三つのどれか一つ。文法で閉じる。

  | 本体 | 例 | 意味 |
  |---|---|---|
  | `Call` | `Registered => issueSession(user, in.body.password);` | 呼び出し一つ |
  | `Atom` | `LoggedIn => session;`、`Empty => none;`、`Standard => 1;` | 値そのもの。`none` もここに入る |
  | `Throw` | `default => throw type.Unauthorized;`、`Exceeded => throw rate.LimitExceeded(wait);` | その例外を投げる。引数は、例外の宣言の引数に、位置で渡す値。引数のない例外には括弧を書かない（`throw X()` は構文エラー）。書ける例外と引数の規則は `docs/check.md`「throw」 |

- `Path` の直後が `(` なら `Call`、そうでなければ `Atom`。
- `throw` はアームの本体にだけ書ける。文（`Stmt`）や `return` の式には書けない。
- `const warranty = match ... { ... };` のように、`match` の `}` の後にも `;` が要る。

## ドキュメンテーションコメント

```
DocComment の中身 = { 本文の行 } { タグの行 } ;
タグの行 = "@param" Ident 説明 | "@throws" QualifiedName 説明 | "@see" ( QualifiedName | DecoratorRef ) ;
```

- 各行の先頭の空白と `*` と、その後ろの空白一つを落としてから読む。
- 本文は Markdown で、最初の段落が要約（§3.4）。
- タグは `@param` / `@throws` / `@see` だけ（§3.4）。`@param` を書けるのは、引数を持つ宣言（op、デコレータ、`exception`、`effect`、`read`）。ほかの `@` で始まる行は本文として扱う。タグの行の後ろに続くタグでない行は、直前のタグの説明の続き。
- 説明は省略できる。
- タグの形が崩れている行（名前がない、`@param` の名前が修飾されている、`@see` に説明がある）は構文エラーだが、宣言は捨てない。
- `bind` には付けられない（「ファイルと宣言」の `Decl`）。

## 決めたこと

この文書で決めたことと、その理由。

| 決めたこと | 理由 |
|---|---|
| `to` を予約語にする | `bind x to ...` の節の頭だから。ほかの節の頭と同じ扱い |
| 予約語をフィールド名に書ける（`FieldName`） | `{ "op": "foo" }` のようなオブジェクトを返す API を、そのままの名前で書けるようにする。フィールド名の位置は `:` の前か `.` の後ろで、予約語が来ても構文があいまいにならない |
| `field` `variant` `post` `put` `delete` は文脈キーワード | 節の操作対象（名詞）で、識別子が来る位置と同じ。§6.1 の `model Violation { field: string; }` が通る |
| 形の語（`list<T>` / `nullable<T>` / `single<T>`）は型の別の書き方で、型を書けるどの位置にも書ける。状態の宣言は `state 名前: Type;` | 形と型を揃えた（`docs/decisions.md`「設計書から離れた決定」S21）。状態を名前で参照したときの型が、宣言に書いた型そのものになる。位置ごとに書ける綴りを分けると、同じ型なのに位置で書き方が変わる規則が要る。以前の「空になりうる状態は `nullable<T>` とだけ書き、`single<T?>` は書けない」はなくした: `single<T>` は `T` なので、`single<T?>` は `T?`、つまり `nullable<T>` と同じ型で、「空」は型の一か所にしか現れない |
| 状態の分類での `match` は対象を書かない（`match by C`） | 対象は分類の宣言が持っていて、書かせると二か所になる |
| `prevents` / `keeps` の目的語は `effects` だけ | 効果を名前で書き分ける場面がない: 葉の op は自分では効果を持たず（STATE-5）、配線の op の、例外ごとの効果の状態は、検査器が計算する（REL-4）。デコレータが運ぶ複数の効果（`docs/check.md`「デコレータが運ぶ reads と effects」）も、名前では書き分けない: 目的語の `effects` は、その op のすべての効果 |
| 予約語をデコレータの名前に書ける（`DecoratorName`） | 語彙のデコレータは、することの名前で呼ぶ（`@db.list`、`@db.delete`）。その語が言語の節の頭や形の語と同じでも、語彙の側の名前を曲げさせない。デコレータの名前の位置は、`decorator` の直後か `@` の後ろ（修飾していれば最後の要素）で、予約語が来ても構文があいまいにならない。フィールド名（`FieldName`）と同じ扱い |
| デコレータの宣言に `reads` / `effects` の節を書ける。要素と区切りは op の節と同じ | デコレータの処理（op の前後に足すもの）が状態に触れることを、デコレータの引数から決められるようにする（`docs/decisions.md`「決めたこと」98）。`throws` と同じく、付いた op の関数の型に加わる。書き方を op の節とそろえて、同じ意味の節の綴りを二つ持たない |
| 残りの引数は `...名前: 型` と書き、型は値一つの型。最後に一つだけ、省略可能にできない | 並べる数が決まらない引数（op が行う効果の並び）を、リスト（`[a, b]`）で囲ませない。TS の書き方に寄せるが、型は配列でなく値一つの型で書く: デコレータの引数の型には `any effect` のような、配列にできない種類がある |
| デコレータの対象は、op が使う三つの単位のどれも（`exception`、`effect`、`read`） | 宣言が、それを使う op の実装の材料を言えるようにする（`@http.status(401)`、`@db.insert`、`@db.find`）。`union` や `classify` と違い、op が投げる、行う、読む「こと」の名前で、語彙が意味を足す相手になる |
| デコレータは `namespace` に付けられない。`for namespace` という対象もない | 名前空間は、複数のファイルの複数のブロックに分けて宣言できる（TypeScript の `interface` と同じく、同じ名前のブロックは一つにまとまる）。名前空間に付けたデコレータは、ファイルをまたいで一つにまとめて管理することになり、どのブロックに何が付いているかが直感的でない。印は中の宣言（op、model など）に付ける |
| `@external` と `@repeatable` は宣言ではなく核の印 | 語彙でも宣言できず、付く先がデコレータの宣言だけなので、文法で閉じる |
| 同じデコレータを一つの宣言に重ねられるのは、宣言が `@repeatable` を持つときだけ | 条件を一つずつ足す語彙（`@db.where(…)` を二つ）のように、重ねることに意味があるデコレータと、重ねれば書き間違いであるデコレータ（`@http.status` を二つ）を、宣言の側で分ける。既定は今までどおり一度だけ（DEC-13） |
| その場で書く model はユニオンのアームにだけ置ける | 合成する名前（`docs/ir.md`「正規化」）が、そこにしか決まらない。例外の値に置けた形（`exception X: { … }`）は、例外が引数を持つ形になってなくなった |
| 省略可能は型の `?` だけで表し、名前に付ける `x?: T` は書けない | 同じ意味の書き方を二つ持たない。型の `?` は `Record<string, int?>` や `Item?[]` のように、名前のない型の位置でも要るので、名前の側を落とす。設計書の例の `x?: T` は、`x: T?` と書く |
| 識別子は ASCII | §3.3 の命名規則が ASCII の大文字小文字で書かれている |
| `Duration` の単位は `ms` `s` `m` `h` `d` | 例にあるのは `m` と `s`。秒未満と日は使う場面が見えるので足す。週以上は月と年が一定でないので持たない |
| `Time` のリテラルはない | 例にない。時刻を仕様に直接書く場面がない |
| 付く先のない `DocComment` はエラー | 説明が IR から落ちることに気づけるようにする（§2「書き忘れ・曖昧さは構文や検査器で検出し」） |
| `model A is B omit f;` をなくし、`model A extends B { override f: never; }` と書く。`is` と `omit` は予約語から外す | 継承した型でできること（足す、なくす、型を変える）を一つの書き方にまとめた（`docs/decisions.md`「設計書から離れた決定」S22）。どの生成規則も使わない語を予約したままにすると、名前に使える語を減らすだけになる |
| 本体が空の `model A extends B { }` は書ける | 継承した model は元の model とは別の型（`docs/types.md`「継承した model」）で、別名ではない。以前の「`omit` のない `is` は書けない」の理由（別名にしかならない）が当たらない |
| `override` の型は省略でき、省略は「型を変えない」。なくす書き方は `override f: never;` | デコレータを足すだけのときに、元の型を書き写させない（元の型を変えたときに、書き写した側が食い違う）。`never` は「そのフィールドに入る値がない」と読める |
| フィールドの既定値（`page: PosInt = 1`）をなくした | 共有する型に書いた既定値は、その型を使うすべての名前空間が埋めることになる。値がないときに誰が何を入れるかは振る舞いで、型ではなく、境界のデコレータが言う（`@http.default`、`@cli.default`。`docs/vocab.md`「`http` の語彙」、`docs/plugin-go-cli.md`「語彙」。`docs/decisions.md`「設計書から離れた決定」S24）。呼び出しの位置を状態で埋める `bind`（「bind」）は別の仕組みで、変えない |
| `reads` `read` `as` `never` `throw` `override` `export` は予約語。綴りは仮で、後で別の語に変えうる | どれも構文の骨（宣言の頭、節の頭、文の頭、核の型、修飾）で、識別子が来る位置の名詞ではないので、文脈キーワードの基準（下の行）に当たらない。`never` は `void` と同じく型の位置に来るので、予約しないと同じ名前の型と区別できない。`override` と `read` は予約してもフィールド名には書け（`FieldName`）、`read` はデコレータの名前にも書ける（`DecoratorName`）。綴りを変えるときは、この表と「予約語」の表を変える |
| `export` と `override` は、予約語の表の「修飾」の行に置く | 宣言やフィールドの頭の語の前に置いて性質を足す語で、宣言の頭でも節の頭でもない。色分けも分ける（`docs/editors.md`「TextMate の文法」） |
| `export` を書けるのは `op` の前だけ | 別の名前空間から隠せるのは op だけ（`docs/check.md`「export」）。ほかの宣言に書ける形にしておくと、意味のない `export` が検査まで残る |
| `throws`、`catches`、`effects`、`reads` の並びの区切りは、op の節でもデコレータの宣言の節でも `,`。`throws A \| B`、`effects a \| b` はなくした（`docs/decisions.md`「設計書から離れた決定」S25） | 三つの節（`throws`、`effects`、`reads`）は、どれも「その op を呼んだときに起きうること」の集合を言うので、同じ書き方にする。記法は「どれか」と「すべて」を書き分けない: 例外は呼び出しを終わらせるので、起きるのは高々一つ。効果と読むことは「このうちのいくつか」。「すべてを行う」と言うのは、デコレータの引数だけ（残りの引数に並べた `@effects(a, b)`）。`,` は `throws` の出どころ（Java）の書き方に近い。`\|` は、型の選択（`int \| float`）、`for` の対象の並び、一つの引数に渡す宣言の集合、ケースの並び（遷移と `match` のアーム）に残る |
| `exception`、`effect`、`read` は、同じ形で書く: 名前、引数、（`read` は結果の型、）宛先。`exception X: T` と、引数のない `effect e on …` はなくした | 三つは、op が使う最小の単位で、op はそれを組み合わせる。配線から op と同じに使えるように（`userByEmail(email)`、`createOrder(order)`、`throw LimitExceeded(wait)`）、op と同じ引数の並びを持たせる。例外の値を一つの model に包ませない: 運ぶ値は引数として並べる |
| `effect` と `read` は引数がなくても `()` を書き、`exception` は引数がなければ括弧を書かない | `effect` と `read` は呼び出すもので、呼び出しはいつも括弧を持つ（op と同じ）。例外は名前で並べ（`throws A, B`）、名前で投げる（`throw A`）もので、値を運ばない例外が普通。どちらも、同じ意味の書き方を二つ持たない |
| `read` の結果の型は省略できない。`on` の後ろに操作は書かない | 読むことは、値を得ること。何を得るかを宣言に書かせ、配線の式の型を宣言だけから決める。操作を書かないのは、状態を読むことが一種類だから |
| 名前を付けない形は、配線にある: 状態を名前で読むこと（`f(users)`）と、組み込みの効果を直に呼ぶこと（`users.post(user);`）。配線の op の `reads` と `effects` の要素も、名前でも、直にでも書ける（`reads userByEmail` と `reads users`、`effects addUser` と `effects users.post`） | 名前を付けるかどうかを、書く人が選べるようにする。説明もデコレータも要らない読み書きに、宣言を一つ足させない。分類の遷移だけは直に書けない: 前後のケースを節や呼び出しの中に書くと、並びが読めなくなる |
| `as` の前には対象を、宣言の `of` の後ろと同じに書く。対象が一つでも省略できない | どの対象が何の型になるかを、対象が複数ある状態の分類と同じ形で読めるようにする。`default` と `condition` には書けない（`default` は「どれでもない」で、型を言えない。`condition` は名前と対象だけの糖衣） |
| `throw` はアームの本体にだけ書ける | 「この場合は失敗」を書くためのもの。本体のどこにでも書ける文にすると、下の行が評価されない配線を書ける |
| `throw` は、例外の引数に値を渡せる（`throw E(a, b)`）。値は `Atom` | 値を運ぶ例外も、配線から投げられるようにする（以前は、値を運ぶ例外を投げられるのは葉の op だけだった）。引数は呼び出しと同じ形（位置で渡す `Atom`）にして、書ける値を位置ごとに分けない |
| アームの本体の値は `Atom` のすべて（名前、リテラル、`none`、レコードリテラル） | 呼び出しの引数と同じ集合にして、位置ごとに書ける値を分けない |
| `match` の分類の名前は修飾できる | 分類はどの名前空間からも使える（`docs/decisions.md`「設計書から離れた決定」S18） |
| 名前で渡す引数の後ろに位置で渡す引数は書けない | 対応が読めなくなる。TS や Python と同じ |
| `return` の後ろに `Atom` も書ける | `return me;` のような、束縛した値をそのまま返す配線を禁じる理由がない |
