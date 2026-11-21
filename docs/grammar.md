# Hozo の文法

## この文書が決めること

`.hozo` の字句と構文を決める。パーサー（`internal/syntax`）とシンタックスハイライトの文法（`editors/vscode/syntaxes/`）はこの文書から書く。構文の意味（何が正しい宣言か）の持ち主は、この文書が節ごとに名指しする文書（`docs/check.md`、`docs/types.md`、`docs/resolve.md`）。構文の形は、設計書（出発点のアイデア集で、仕様の持ち主ではない。`docs/README.md`「設計書という呼び名」。「§n」はその節）の例から始めたもので、この文書で決めたことと、その理由は「決めたこと」にある。設計書の例にない構文（`reads`、`export`、`extends` を持つ model、`as`、`throw`、`never`、形の語を型として書くこと）は、`docs/decisions.md`「設計書から離れた決定」の S15〜S23 が足したもので、意味の持ち主はその表の「持ち主」の列の文書。デコレータの宣言の `reads` / `effects` の節、残りの引数（`...`）、対象 `effect`、予約語のデコレータ名は、同「決めたこと」98 が足したもので、意味の持ち主は `docs/check.md`「デコレータが運ぶ reads と effects」と `docs/types.md`「デコレータの引数」。引数を持つ `exception` / `effect` / `read` の宣言、配線からの `read` と効果の呼び出し、引数を持つ `throw`、`reads` / `effects` に直に書く要素、`throws` / `catches` / `reads` / `effects` の並びの区切り `,`、対象 `read`、核の印 `@repeatable` は、例外、効果、`read` を同じ書き方にする決定（`docs/decisions.md`。段階 R4: `docs/requirements.md`「作り直しの段階」）が足したもので、意味の持ち主は `docs/check.md`「状態に触れるもの」「op の宣言と本体」と `docs/types.md`「呼び出しの引数」。`match` の `else` のアームと、ケースの並びの区切り `,` は、分類を閉じる決定（`docs/decisions.md`「決めたこと」154〜157）が足したもので、同じ決定が、名前のないケース `default`、`condition` の宣言、`default` のアーム、ケースの並びの `|` をなくした。意味の持ち主は `docs/check.md`「分類」と `docs/types.md`「アームの中の対象の型」。行の遷移（`effect … on 状態.操作 as …`）と、ケースの `as none` は、表の一行の状態を遷移で書けるようにする決定（`docs/decisions.md`「決めたこと」163〜173）が足したもので、意味の持ち主は `docs/check.md`「行の遷移」と、同「分類」の「ない側のケース」。配線の `val`、`=`、`disjoint`、`for`、`collect`、文の終わりの `;` の決まり、`match` の頭の丸括弧、対象のない分類（`classify … of never`）、分類のケースの前のデコレータ、デコレータの宣言の `classify … case` は、IDE の設計から出た決定（`docs/decisions.md`「決めたこと」294〜343。段階 R6: `docs/requirements.md`「作り直しの段階」）が足したもので、同じ決定が、配線の `const` と、丸括弧のない `match` の頭を、なくした。意味の持ち主は `docs/check.md`「値の入れ物」「繰り返し」「disjoint」と、同「分類」の「対象のない分類」、`docs/types.md`「式の型」「デコレータの引数」。

記法は EBNF。`=` で定義、`|` で選択、`[ ]` は省略可、`{ }` は 0 回以上の繰り返し、`" "` は終端記号、`( )` はまとまり。

## 字句

### 空白とコメント

空白と改行は区切りとしてだけ意味を持つ。文の終わりは `;` で、改行では終わらない。

```
LineComment  = "//" { 改行以外の文字 } ;
BlockComment = "/*" { "*/" 以外 } "*/" ;              (* 入れ子にできない *)
DocComment   = "/**" { "*/" 以外 } "*/" ;
```

`LineComment` と `BlockComment` は捨てる（§3.4「`//` と `/* ... */` はただのコメントで、IR に入らない」）。`DocComment` は字句として残し、直後の宣言・フィールド（`override` のメンバーを含む。「model」）・ユニオンのメンバー・分類のケースに付く（§3.4「直後の宣言の『説明』として IR に入る」）。付く先がない `DocComment` は構文エラー（「決めたこと」）。

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
| 宣言の頭 | `namespace` `model` `scalar` `union` `state` `fn` `op` `exception` `effect` `read` `classify` `decorator` `bind` |
| 修飾 | `export` `override` |
| 節の頭 | `extends` `of` `on` `to` `by` `for` `as` `effects` `reads` `throws` `prevents` `keeps` `catches` `recovers` `halts` `ignores` `any` `case` |
| 文の頭と値 | `val` `return` `match` `disjoint` `collect` `default` `else` `none` `throw` |
| 核の型と形 | `string` `boolean` `int` `float` `datetime` `date` `time` `duration` `void` `never` `list` `nullable` `single` |
| リテラル | `true` `false` |

`fn` は、関数の宣言の頭。`read` と `effect` は、その前に書く、関数の種類の語（「関数の宣言」）。`op` は、種類の名前で、宣言の頭には書かない: 書くのは、デコレータの宣言の、付ける先の並び（`for model, state, op`）と `any op`（「decorator の宣言」）。関数に付けるデコレータの宣言の頭は、`decorator fn`（同）。

`for` と `of` は、節の頭（デコレータの宣言の `for`、`classify` の `of`）のほかに、配線の繰り返し（「配線」の `For`）の頭にも書く、同じ字句。`case` は、デコレータの宣言で、分類のケースを言う語（「decorator の宣言」の `CaseKind`）。

`default` は、どの生成規則も使わない予約語: 書けるのは `FieldName` と `DecoratorName` の位置（`@http.default(1)`）だけ。予約したままにする理由は「決めたこと」。

次の語は予約しない。決まった位置でだけ意味を持ち、それ以外では `Ident` になる（文脈キーワード）。

| 語 | 位置 |
|---|---|
| `field` `variant` | デコレータの `for` の対象（`field`）と `any` の種類（`variant`）。`for` の対象の `effect` と `read` は予約語（宣言の頭）で、その位置でも同じ字句 |
| `post` `put` `delete` | `effect ... on 状態.操作`、`any effect on 状態.操作`、`effects` の要素に直に書く `状態.操作`（「op」）、配線で直に呼ぶ `状態.操作(…)`（「配線」）の操作 |

`@external` と `@repeatable` は予約語でもデコレータの宣言でもなく、核の印。デコレータの宣言の上にだけ書ける（「decorator の宣言」）。

`reads` `read` `as` `never` `throw` `override` `export` の七つは仮の綴りで、後で別の語に変えうる（「決めたこと」）。`is` と `omit` は予約語ではない（`model A is B omit …` をなくしたときに外した。「model」）。`const` も予約語ではない（配線の束縛を `val` に替えたときに外した。「配線」）。

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

`Duration`（字句）は、時間の長さのリテラル（`5m`、`30s`）で、型は、核の型 `duration`（「型」）。単位の選び方と、`datetime`、`date`、`time` のリテラルがないことは「決めたこと」。

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
            | ExceptionDecl | EffectDecl | ReadDecl | ClassifyDecl | DecoratorDecl ;

NamespaceDecl = "namespace" QualifiedName "{" { Decl } "}" ;
```

- `namespace` の外の宣言はグローバル（§3.2）。
- `namespace server.admin { }` と `namespace server { namespace admin { } }` はどちらも書ける（§3.2）。同じ名前の `namespace` は何度でも書ける。
- デコレータはどの `Declaration` の前にも書けるが、付けられる対象はデコレータの宣言の `for` で決まる（検査 DEC-1）。宣言のほかに、フィールド（「model」）と、分類のケース（「classify」）の前にも書ける。`namespace` と `bind` には付けられない（文法で閉じる。`namespace` の理由は「決めたこと」）。
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
- `ArgTerm` の `|` は、一つの引数に渡す、その場で書いた union（`http.NetworkError | http.ServerError`: 「このどれか」。§11.2。`docs/decisions.md`「決めたこと」358）。名前を付けた union（`union U { A; B; }`）を渡すのと、同じ意味。引数の並びの `,` は引数を区切るので、一つの引数の中の union は `|` で書く（節の並びの区切りは `,`。「op」）。`[ ]` はリスト（`["orders", query]`。§11.1）。
- `QualifiedName` が何を指すか（宣言、op の引数、デコレータの引数、ユニオンのアーム、分類のケース）は名前の解決が決める。分類のケースは、分類の名前に続けて書く（`@usecase.of(Actor.Customer)`）。
- 宣言が残りの引数（「decorator の宣言」の `RestParam`）を持つデコレータには、ほかの引数の数を超えて位置で渡した値が、書いた順に、残りの引数に入る（`decorator effects(...targets: any effect)` に `@effects(createOrder, notifyUser)`）。0 個でもよい。残りの引数には、名前では渡せない（DEC-11。`docs/types.md`「デコレータの引数」）。

### model

```
ModelDecl   = "model" Ident [ "extends" QualifiedName ] ModelBody ;
ModelBody   = "{" ( { Member } | Index ) "}" ;
Member      = Field | Override ;
Index       = "[" Ident ":" Type "]" ":" Type ";" ;
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
  | `override hash: never;` | そのフィールドをなくす: 入る値が一つもない型（`never`）にするので、フィールドがない（`docs/types.md`「型の種類」の「never」） |

- メンバーの頭（`DocComment` とデコレータの後ろ）の `override` は、直後が `:` ならフィールドの名前（`override: string;`。`FieldName` は予約語も書ける）、そうでなければ `Override` の頭。`override override: never;` は、`override` という名前のフィールドをなくす。
- `Override` は文法ではどの `ModelBody` にも書けるが、`extends` のない model と、その場で書く model（「union」）には元の model がないので、検査でエラーになる（TYPE-10。`docs/types.md`「診断」）。
- `Index` を書いた model は、**インデックスの model**: キーが決まっていない構造で、キーから、値を引く（`model Attrs { [name: string]: string[]; }`。`docs/decisions.md`「決めたこと」360）。`[` の後ろの名前は、キーが何かを言う、説明のための名前。型の規則は `docs/types.md`「インデックスの model」。
- 本体に書けるのは、`Member` の並びか、`Index` 一つの、どちらか。`Index` と `Member` を混ぜる、`Index` を二つ書く、`Index` にデコレータか `DocComment` を付ける、`extends` を持つ model に `Index` を書く、その場で書く model（「union」の `ArmType`）に `Index` を書く、は、どれも SYNTAX。
- `Record<K, V>` は書けない（なくした書き方。「決めたこと」）。パーサーは、型の位置に、`Record` という名前と、その直後の `<` が来たら、インデックスの model を宣言して、その名前を書くことをメッセージで示して SYNTAX にする。`Record` は、予約語ではなく、ふつうの名前として宣言できる。
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

### 関数の宣言

```
FnDecl     = OpDecl | ReadDecl | EffectDecl ;       (* OpDecl は「op」。ReadDecl と EffectDecl は「exception / effect / read」 *)
FnHead     = "fn" Ident OpParams ;
OpParams   = "(" [ OpParam { "," OpParam } ] ")" ;
OpParam    = Ident ":" Type ;
ReturnType = Type | "void" | "never" ;
```

op、`read`、効果は、どれも関数で、同じ形で宣言する（`docs/decisions.md`「決めたこと」347、348、350、351）: 種類の語、`fn`、名前、引数の並び（`FnHead`）と、その後ろの、関数の型。違うのは、`fn` の前に書く種類の語と、関数の型に書ける節。

**関数の型**は、`)` の後ろの、`:` から始まる部分: 戻り値と、節（投げるもの、読むもの、変えるもの）。

- `)` の後ろに、戻り値か節を書くなら、`:` で始め、戻り値、節の順に書く（`fn register(in: Input): User throws Taken reads users`）。
- 戻り値は省けて、省けば `void`（`fn save(item: Item): throws Broken { … }`）。`: void throws Broken` と書いても、同じ（IR で同じ。§7.1）。
- 何も書かなければ、`:` も書かない（`fn ping();`、`fn clearAll() { … }`）。`:` の後ろに、戻り値も節もない形（`fn f():;`）は、構文エラー。

| 種類 | 頭 | 戻り値 | 節と、その後ろ |
|---|---|---|---|
| op | `fn`。別の名前空間から呼べるものは `export fn` | 省ける。型、`void`、`never` | `throws`、`reads`、`effects` を、いくつでも。その後ろに、`;` か本体（「op」） |
| `read` | `read fn` | 型。省けない（TYPE-18） | `reads 状態` を、ちょうど一つ。その後ろに `;`（「exception / effect / read」） |
| 効果 | `effect fn` | 省く。`void` と書いてもよい（TYPE-18） | `throws` を、書くなら一つ。その後ろに、`effects 宛先` を、ちょうど一つ。その後ろに `;`（同上） |

```
export fn login(in: LoginInput): Session throws InvalidCredentials { … }
fn findUser(email: Email): User?;
fn ping();
read fn userByEmail(email: Email): User? reads users;
effect fn createUser(user: User): effects users.post;
effect fn addUser(user: User): throws EmailTaken effects users.post;
```

- 種類を書かない `fn` は、op。`op fn f()` とは書かない。
- 種類ごとに書ける節は、文法で閉じる: `read fn` と `effect fn` に、本体は書けない。`read fn` に、`throws` の節は書けない。`read fn` の `reads` と、`effect fn` の `effects` は、状態に直に触れることの宣言で、要素は一つだけ。
- `effect fn` の `throws` は、その効果が投げる、仕様の例外（`docs/decisions.md`「決めたこと」352。意味は `docs/check.md`「状態に触れるもの」の「read と効果」）。`effects` の前に、一つだけ書ける。要素は op の `throws` と同じ（「decorator の宣言」の `ExcRef`）で、関係の節（`prevents effects`、`keeps effects`）は書けない: 投げたとき、その効果は、いつも、起きていない。
- `reads` と `effects` は、どの種類でも、同じ関係を言う: その関数が、状態を読むこと、変えること（`docs/check.md`「状態に触れるもの」）。`read fn` と `effect fn` では、その関数が直に触れる先。op では、本体が呼ぶものの合わせたもので、葉（本体のない op）には書けない（STATE-5。「op」）。
- 引数がなくても、`()` を書く（配線から `名前()` と呼ぶものだから）。
- 効果が返せるのは `void` だけで、`read` は `void` と `never` を返せない（TYPE-18。`docs/types.md`「診断」）。
- なくした書き方は、新しい書き方をメッセージで示して SYNTAX にする:

  | なくした書き方 | 新しい書き方 |
  |---|---|
  | 頭に `fn` のない宣言（`op f()`、`read r()`、`effect e()`） | `fn f()`、`read fn r()`、`effect fn e()` |
  | `op fn f()` | `fn f()`（種類を書かない `fn` が op） |
  | 節の前に `:` のない形（`fn f() throws E`、`effect fn e() effects s.post`） | `fn f(): throws E`、`effect fn e(): effects s.post` |
  | `read fn r(): T on s;`、`effect fn e() on s.post;`（`on`） | `read fn r(): T reads s;`、`effect fn e(): effects s.post;` |

### exception / effect / read

```
ExceptionDecl = "exception" Ident [ "(" OpParam { "," OpParam } ")" ] ";" ;
EffectDecl    = "effect" FnHead ":" [ ReturnType ] [ EffectThrows ] "effects" ( BuiltinEffect [ RowClause ] | Transition ) ";" ;   (* FnHead と ReturnType は「関数の宣言」 *)
EffectThrows  = "throws" ExcRef { "," ExcRef } ;        (* ExcRef は「decorator の宣言」。関係の節（Relation）は書けない *)
ReadDecl      = "read" FnHead ":" [ ReturnType ] "reads" QualifiedName ";" ;
BuiltinEffect = QualifiedName "." StateOp ;
StateOp       = "post" | "put" | "delete" ;                 (* 文脈キーワード *)
Transition    = QualifiedName "as" CaseSet "->" CaseSet ;
CaseSet       = "any" | Ident { "," Ident } ;
RowClause     = "as" ( RowCase | "else" ) ;                 (* 効果の後の、行のケース *)
RowCase       = QualifiedName ;                             (* 分類の名前 "." ケースの名前: Stage.Accepted *)
```

三つは、op が使う最小の単位で、同じ形で書く: 名前、引数の並び（`OpParams` と `OpParam` は「関数の宣言」）、宛先（`read` は `reads` の状態、効果は `effects` の宛先。例外には、ない）。どれも本体を持たず、ドキュメンテーションコメントとデコレータを付けられる（対象 `exception` / `effect` / `read`。「decorator の宣言」）。意味は `docs/check.md`「状態に触れるもの」と「op の宣言と本体」の「throw」。

| 宣言 | 例 | 引数 | 結果 |
|---|---|---|---|
| `exception` | `exception InvalidCredentials;`、`exception LimitExceeded(wait: duration);` | その例外が運ぶ値 | なし |
| `effect` | `effect fn createOrder(order: Order): effects orders.post;`、`effect fn clearCart(): effects carts as HasItems -> Empty;`、`effect fn startProduction(order: Order on Stage.Accepted): effects orders.put as Stage.InProduction;`、`effect fn createUser(user: User): throws EmailTaken effects users.post;` | その効果が受け取る値 | なし |
| `read` | `read fn userByEmail(email: Email): User? reads users;`、`read fn allOrders(): Order[] reads orders;` | 読むときに受け取る値 | `:` の後ろの型。省けない（「関数の宣言」） |

- `effect fn` と `read fn` は、引数がなくても `()` を書く（「関数の宣言」）。`exception` は、引数がなければ括弧を書かない（`throw InvalidCredentials` と投げ、`throws` に名前で並べるもの）。`exception X();` は構文エラー（同じ意味の書き方を二つ持たない）。
- 効果の宛先は一つだけで、`effects` の後ろに書く。基底の状態の組み込みの効果（`effects carts.post`）か、状態の遷移（`effects Checkout as Shopping -> Ordered`）。`Transition` の `QualifiedName` は、状態を対象にした `classify` か、組み込みの分類を持つ基底の状態（`effect fn clearCart(): effects carts as HasItems -> Empty;`）。
- 宛先の `QualifiedName` の最後の要素が `StateOp`（`post` / `put` / `delete`）なら `BuiltinEffect`、そうでなければ `Transition`。最後の要素が `post` / `put` / `delete` の名前の状態や分類は、状態の遷移の宛先に書けない（`effects ns.delete as A -> B` は、状態 `ns` の `delete` と、行の遷移として読み、行の遷移に矢印は書けないので、SYNTAX になる）。
- **`as` の後ろは、いつも、ケース。** 宛先が `状態.操作` なら、行の遷移（`RowClause`）: 効果の後の、行のケースを、一つ書く。宛先が、状態か、状態の分類なら、状態の遷移（`Transition` の `CaseSet`）: 前と後を、矢印で書く。行の遷移は、分類の名前から書き（`Stage.Draft`）、状態の遷移は、宛先が分類を決めているので、ケースの名前だけを書く（`HasItems`）。行の遷移に矢印がないのは、前のケースを、行の引数の型（「型」の `OnType`）で言うから。状態の遷移は、引数を持たないので、前も、ここに書く。
- `CaseSet` のケースの並びの区切りは `,`（`effect fn touchCheckout(): effects Checkout as Shopping, Ordered -> any;`）。`match` のアーム（「配線」の `MatchArm`）と同じ区切り。
- `RowClause` は**行の遷移**: 組み込みの効果の後、受け取った行（状態の要素一つ）が、どのケースにあるかを言う。宛先（どこに、何の操作で書くか）とは別のことで、`BuiltinEffect` の後ろにだけ書ける（`Transition` は、それ自身が `as` を持つ。文法で閉じる）。意味、操作ごとに書ける形、分類と引数の結びつきは `docs/check.md`「行の遷移」。

  | 形 | 例 | 読み方 |
  |---|---|---|
  | `as <後>` | `effect fn startProduction(order: Order on Stage.Accepted): effects orders.put as Stage.InProduction;` | 効果の後、行は `<後>` にある |
  | `as else` | `effect fn revise(order: Order on Stage.Accepted): effects orders.put as else;` | 後のケースは、決まっていない |

- `RowCase` は、行の分類の名前と、そのケースの名前を `.` でつないだもの（`Stage.Accepted`。別の名前空間の分類なら `ordering.Stage.Accepted`）。要素が一つだけの名前（`Accepted`）は構文エラー: どの分類のケースかを、いつも書く。
- 効果の前の、行のケースは、ここには書かない。行の引数の型で言う（`order: Order on Stage.Draft | Stage.Accepted`。「型」の `OnType`）。
- `else` は、どのケースになるかは決まっていない、と言う（`as else`）。分類を名指ししない。
- ケースを二つ以上並べた `as A, B;` は構文エラー。
- `read fn` の `reads` の後ろは基底の状態一つで、操作（`.get` のようなもの）は書かない: 状態を読むことは一種類しかない。
- `sessions.post` は字句では一つの `QualifiedName` になる（`post` は文脈キーワードで、字句としては `Ident`）。`EffectDecl`（`effects sessions.post`）と `AnyType`（`on location.put`）では、構文解析で最後の要素を `StateOp` として切り出す。`effects` の要素（「op」の `EffectElem`）と、配線の呼び出し（「配線」の `Call`）では切り出さず、名前の解決が決める。
- なくした書き方は、新しい書き方をメッセージで示して SYNTAX にする（関数の頭と、`:`、`on` は「関数の宣言」の表）:

  | なくした書き方 | 新しい書き方 |
  |---|---|
  | `exception X: T;`（例外の名前の直後の `:`） | 運ぶ値は引数で書く: `exception X(value: T);` |
  | `effect fn e: effects s.post;`（名前の直後に、引数の括弧がない） | `effect fn e(): effects s.post;` |
  | `effects Checkout: Shopping -> Ordered`（宛先の後ろの `:`） | 状態の遷移は `as` で書く: `effects Checkout as Shopping -> Ordered` |
  | `effects Checkout as Shopping \| Ordered -> any`（`CaseSet` のケースの後ろの `\|`） | 区切りは `,`: `effects Checkout as Shopping, Ordered -> any` |
  | `effects orders.put as Stage.Draft -> Stage.Accepted`、`as Stage.Draft, Stage.Accepted -> …`（行の遷移の矢印と、前のケース） | 前のケースは、行の引数の型で言い、`as` は、後のケースだけ: `effect fn accept(order: Order on Stage.Draft): effects orders.put as Stage.Accepted;` |
  | `effects orders.put as any -> Stage.Shipped`、`effects orders.post as any`（行の遷移の `any`） | 前がどのケースでもよければ、行の引数を絞り込まない。後を言わないなら、`as` を書かない（`put` で、後のケースを約束しないなら `as else`） |

### classify

```
ClassifyDecl    = "classify" Ident "of" ClassifySubject "{" { ClassifyCase } "}" ;
ClassifyCase    = [ DocComment ] { Decorator } Ident [ ":" CaseTyping { "," CaseTyping } ] ";" ;
CaseTyping      = Type "as" ( Type | "else" | "none" ) ;
ClassifySubject = "never" | Type { "," Type } ;
```

対象は、`never` か、三つの種類のどれか。三つの種類は、名前の解決で決まる（`docs/resolve.md`「位置ごとに受け付ける種類」）。合わない並びはエラー（検査 CLS-3）。

| 対象 | 例 | 分類の種類 |
|---|---|---|
| 一つで、型を指す | `classify WarrantyKind of server.Item { ... }`、`classify Presence of nullable<User> { ... }` | 値の分類（§10.1） |
| 一つで、op を指す | `classify Registration of findUser { ... }` | op の分類（その op が返した値の分類） |
| すべてが今の名前空間の状態の名前 | `classify Checkout of carts, orders { ... }` | 状態の分類 |
| `never` | `classify Actor of never { Customer; Staff; }` | 対象のない分類（当たる値がない。`docs/check.md`「分類」の「対象のない分類」） |

- ケースは、どれも名前を持つ（`classify Role of Member { Admin; Regular; }`）。宣言したケースが、その分類のすべて（`docs/check.md`「閉じた分類」）。
- `CaseTyping` は、そのケースのときに対象が何の型になるかを書く（`Registered: findUser as type.User;`）。`as` の前は対象の一つで、宣言の `of` の後ろと同じに書く（値の分類なら同じ型、op の分類ならその op、状態の分類なら対象の状態の一つ）。`as` の後ろはそのケースでの型。意味と、書ける型は `docs/check.md`「分類」と `docs/types.md`「絞り込み」。
- `as` の後ろの `else` は、型の代わりに「残り」を言う（`Unregistered: findUser as else;`）: 対象が、ほかのケースが `as` に書いた型のどれにも当たらないとき。`else` は予約語で、書けるのは、この位置と、`match` の最後のアームの頭（「配線」の `ElseArm`）と、行の遷移の矢印の後ろ（「exception / effect / read」の `RowClause`）だけ。書ける分類と意味は `docs/check.md`「分類」の「残りのケース」。
- `as` の後ろの `none` は、型の代わりに「値がない」を言う（`Absent: Order? as none;`）: 対象に値がないとき。`none` は、配線の値（「配線」の `Atom`）と同じ予約語。書ける分類と意味は `docs/check.md`「分類」の「ない側のケース」。
- 状態の分類は、一つのケースに複数の対象の型を `,` で並べられる（`Ready: token as Token, profile as Profile;`）。
- 対象のない分類は、`of` を省かず、`of never` と書く。ケースは、名前だけ（`CaseTyping` を書けば CLS-9）。その分類は、名前を並べるだけで、型にも、`match` にも、効果の遷移にも、書けない（書けない所と診断は `docs/check.md`「分類」の「対象のない分類」）。
- ケースの前に、デコレータを書ける（`@actor.human Customer;`）。順は、`DocComment` → デコレータ → ケースの名前。どの種類の分類のケースにも書け、付けられるデコレータは、宣言の `for` が、分類のケース（「decorator の宣言」の `CaseKind`）を並べたもの（検査 DEC-1）。
- なくした書き方は、新しい書き方をメッセージで示して SYNTAX にする: ケースの位置の `default;`（名前のないケース。ケースには名前を付ける。値がない側や、union の残りのアームなら、残りのケース `<名前>: <対象> as else;`）、宣言の頭の `condition C of T;`（両側に名前を付けた `classify C of T { <成り立つ側>; <成り立たない側>; }` と書く）。

### decorator の宣言

```
DecoratorDecl = { CoreMark } "decorator" "fn" DecoratorName Params [ ":" DecoratorClause { DecoratorClause } ] ( ";" | Block )
              | { CoreMark } "decorator" DecoratorName [ Params ] "for" Target { "," Target } ";"
              | "decorator" DecoratorName "=" DecoratorRef Args ";" ;
CoreMark      = "@external" | "@repeatable" ;          (* 核の印。それぞれ一度まで *)

Params    = "(" [ Param { "," Param } [ "," RestParam ] | RestParam ] ")" ;
Param     = Ident ":" ParamType ;
RestParam = "..." Ident ":" RestType ;                 (* 残りの引数。最後に一つだけ *)
ParamType = AnyType [ "?" ] | Type { "|" Type } ;      (* 省略可能は型の ? で書く。any の種類にも付けられる（from: any state?） *)
RestType  = AnyType | Type { "|" Type } ;              (* 渡す値一つの型。? も [] も付けない *)
AnyType   = "any" AnyKind [ "on" QualifiedName "." StateOp ] ;
AnyKind   = "exception" | "effect" | "state" | "op" | "model" | "variant" | CaseKind ;   (* variant は文脈キーワード *)
Target    = "op" | "field" | "state" | "exception" | "scalar" | "model" | "effect" | "read" | CaseKind ;   (* field は文脈キーワード *)
CaseKind  = "case" [ "of" ( "any" | "never" | Type ) ] ;   (* 分類のケース。Type は、型か、分類の名前 *)

DecoratorClause = ThrowsClause | CatchesClause | DecoReadsClause | DecoEffectsClause ;
ThrowsClause    = "throws" ExcRef { "," ExcRef } { Relation } ;
Relation        = ( "prevents" | "keeps" ) "effects" ;
CatchesClause   = "catches" ExcRef { "," ExcRef } ( "recovers" | "halts" QualifiedName | "ignores" ) ;
ExcRef          = QualifiedName ;
DecoReadsClause   = "reads" ReadElem { "," ReadElem } ;       (* ReadElem と EffectElem は「op」 *)
DecoEffectsClause = "effects" EffectElem { "," EffectElem } ;
```

宣言の形は、三つ（`docs/decisions.md`「決めたこと」392〜394）。

| 形 | 書き方 | 付ける先 | 節 | 本体 |
|---|---|---|---|---|
| 関数に付けるデコレータ | `decorator fn limited(): throws LimitExceeded prevents effects;` | op（種類を書かない `fn`）だけ | 書ける | 書ける |
| そのほかのデコレータ | `decorator table for state;`、`decorator context(name: string) for model, state, op;` | `for` の後ろに並べた種類 | 書けない | 書けない |
| 名前付きのデコレータ | `decorator redirectToLogin = redirectOn(server.Unauthorized, transitionToLogin);` | 汎用の側の宣言に従う | 書けない | 書けない |

- **関数に付けるデコレータは、`decorator fn` と書く。** `fn` が、付ける先（op）を言うので、`for` は書かない。後ろの形は、関数の宣言（「関数の宣言」の `FnType`）と、同じ決まり: `()` は、引数がなくても書く。`)` の後ろに節を書くなら、`:` で始める。節がなければ、`:` も書かない（`decorator fn post(path: string);`、`decorator fn audited() { … }`）。戻り値は、書かない（包んだ op の結果が、そのまま返る）。`:` のない節（`decorator fn f() throws E;`）と、`:` の後ろに節のない形は、SYNTAX。
- **`decorator` の直後の `fn` は、いつも、この頭**: `fn` という名前のデコレータは、宣言できない（`DecoratorName` に書ける予約語の、ただ一つの例外）。
- **本体のないものは、プラグインが解釈する**（同 393）。op の中身を包む（継ぎ目。`docs/config.md`「継ぎ目」）か、語彙として読むだけかは、そのプラグインが決める。宣言では、言い分けない。
- **本体のあるものは、包み方が、仕様にある。** 本体は、配線のブロック（「配線」の `Block`）で、包んだ op の中身が動く所に、`proceed();` を書く（下の「デコレータの本体」）。
- **op だけに付けるデコレータを、`for` の形で書くことは、なくした**: `decorator x for op …;` と、`for` を省いた `decorator x …;`（前は、op の意味）。パーサーは、新しい書き方（`decorator fn x(…)`）をメッセージで示して SYNTAX にし、`decorator fn` として読み進める。op と、ほかの種類の、両方に付けるデコレータ（`for model, state, op`）と、`read fn`・`effect fn` に付けるデコレータ（`for read`、`for effect`）は、`for` の形のまま。
- **`for` の形は、節も、本体も、持てない**（文法で閉じる。前は、検査の DEC-2）。`for` の対象の後ろに、節の頭（`throws`、`catches`、`reads`、`effects`）か、`{` が来たら、SYNTAX: メッセージは、節と本体を書けるのは `decorator fn` だけであることを示す。

- `@external` と `@repeatable` は核の印（`CoreMark`）で、デコレータの宣言にだけ付く。`decorator` の直前に置き、引数を取らず、別名の形には付けられない。二つを並べてよく、順は問わない。同じ印を二度書く、ほかの位置に書く、は構文エラー。
  - `@external`: そのデコレータの `throws` の例外は、境界の外から見たときだけ加わる（§8.2「境界の外から見たときの型の変化」。DEC-10）。
  - `@repeatable`: そのデコレータを、一つの宣言に何度でも付けられる（`@repeatable decorator where(column: string, value: string | int | float | boolean) for read;`）。印のないデコレータは、一つの宣言に一度だけ（DEC-13。`docs/check.md`「デコレータ」）。何度も付けた適用は、書いた順に IR に並ぶ（`docs/ir.md`「デコレータの適用」）。
- 三つ目の形が名前付きのデコレータ（§8.1 `decorator redirectToLogin = redirectOn(server.Unauthorized, transitionToLogin);`）。`for` も節も持たない（汎用の側の宣言に従う）。
- `ExcRef` は例外、例外のユニオン、またはデコレータの引数の名前（`catches error recovers`。§4.2）。
- `|` は、union を、その場で書く記号で、「A か B」を言う（`docs/decisions.md`「決めたこと」358、368）。宣言の中で書けるのは、デコレータの引数の型（`value: int | float`。§4.2 `valid.min`）と、絞り込んだ型の `on` の後ろのケース（「型」の `OnType`）。model のフィールドや、関数の引数の、型そのものには、書けない。適用の側の `|` は「デコレータの適用」の `ArgValue`。
- `for` の後ろは、型ではなく、付ける先の種類の一覧で、区切りは `,`（`for scalar, field`）。節の並び（`throws`、`catches`、`reads`、`effects`。「op」）と、同じ決まり。パーサーは、`for` の対象の後ろに `|` が来たら、区切りは `,` であることをメッセージで示して SYNTAX にし、`,` として読み進める（なくした書き方。「決めたこと」）。
- `any` は、「どれでも」を言う語（`docs/decisions.md`「決めたこと」357）。`for` の後ろに書くのは、種類（`for effect`）。`:` の後ろに書くのは、型。`any <種類>` は、種類から、型を作る: その種類の宣言なら、どれでも（`error: any exception`）。なので、付ける先には `any` を書かず、引数の種類には、書く。
- `ThrowsClause` と `CatchesClause` の順序と回数は文法では縛らない（`catches X recovers throws Y`。§8.1）。差し引きは検査が計算する（DEC-6）。
- `Target` の `effect` は、効果の宣言に付けるデコレータ（`@db.insert effect fn createOrder(order: Order): effects orders.post;`）。`read` は、`read` の宣言に付けるデコレータ（`@db.find read fn userByEmail(email: Email): User? reads users;`）。`exception` は、例外の宣言に付けるデコレータ（`@http.status(401) exception Unauthorized;`）。
- `CaseKind` は、分類のケースを言う（`docs/decisions.md`「決めたこと」349）。`for` の後ろに書けば、ケースに付けるデコレータ（`decorator human for case of never;`）、`any` の後ろに書けば、ケースを受け取る引数（`decorator fn of(actor: any case of never);`）。もとの形は、`case of …` で、`of` の後ろが、絞り。

  | 書き方 | 言うもの |
  |---|---|
  | `case of any` | どの分類でもよい。そのケース（対象が、op や状態の分類も含む） |
  | `case` | `case of any` の省略。同じ意味 |
  | `case of never` | 対象のない分類（「classify」）の、ケース |
  | `case of Order` | 対象の型が `Order` の分類の、ケース。当たるのは、値の分類で、対象の型が、書いた型と等しいもの（`docs/types.md`「型」の「等しい」） |
  | `case of Actor` | 分類 `Actor` の、ケース（別の名前空間の分類は、修飾して書く） |

  - `of` の後ろの名前が、型か、分類かは、名前の解決が決める（`docs/resolve.md`「位置ごとに受け付ける種類」）。op や、状態の名前は、書けない。
  - ここの `any` は、型ではなく、「どの型でも」を言う語（遷移の `as any`、`-> any` と、同じ役）。ほかの型の位置（フィールド、引数、戻り値）に、`any` は書けない。
  - **引数の種類の、いちばん後ろの `?` は、引数を省ける印**（`ParamType` の `?`。`docs/decisions.md`「決めたこと」354）: `any case?`、`any case of any?`、`any case of never?`、`any case of Order?` は、どれも、省ける引数。`any case of Order?` の `?` は、対象の型の一部ではない（対象は `Order`）: 引数の型の全体を、`nullable` で包んだ形の、省略（`docs/types.md`「形の書き方」）。内側の `?`（`any case of Order?[]`）は、型の一部。
  - **型で絞る形の対象に、省略可能な型は、書けない**（いちばん外が `?` か、`nullable<…>` の型。SYNTAX）: 付ける先の `for case of Order?`、`for case of nullable<Order>`、引数の種類の `any case of nullable<Order>`、`any case of Order??`。省略可能な型を対象にする分類（`classify Known of User?`）のケースは、名前で絞る（`case of Known`）。メッセージは、その書き方を示す。
- 分類そのものを、付ける先や、引数の種類にする形（`for classify`、`any classify`）は、まだ、書けない。パーサーは、`Target` か `AnyKind` の位置に `classify` が来たら、SYNTAX にする: メッセージは、分類そのものは、まだ、書けないことと、なくした書き方（`classify case`、`classify of never case`、`classify Actor case`）の、新しい書き方（`case of any`、`case of never`、`case of Actor`）を、示す。
- `DecoReadsClause` と `DecoEffectsClause` は、そのデコレータの処理が読むものと、持つ効果を言う節（`decorator fn limit(counter: any state, by: any effect): reads counter effects by;`）。要素は op の節と同じもの（`ReadElem`、`EffectElem`）か、デコレータの引数の名前（`any state` の引数は読む状態、`any effect` の引数は効果）。何度でも書ける。意味は `docs/check.md`「デコレータが運ぶ reads と effects」。
- `Relation` の `effects` は、直前の `prevents` / `keeps` の目的語。`DecoEffectsClause` の頭の `effects` と字句は同じだが、`prevents` / `keeps` の直後かどうかで分かれる（`throws X prevents effects effects e` は、`Relation` の後ろに `DecoEffectsClause`）。
- `RestParam` は**残りの引数**: 適用が、ほかの引数の後ろに位置で渡した 0 個以上の値を、まとめて受け取る（`decorator fn effects(...targets: any effect): effects targets;`）。型は、渡す値一つの型を書く。引数の並びの最後に一つだけ書け、省略可能にはできない（文法で閉じる）。節の中で名前を書けば、渡した値のすべてを指す。

#### デコレータの本体

`docs/decisions.md`「決めたこと」392。`decorator fn` の本体は、配線のブロックに、`proceed()` を足したもの。

```
decorator fn limited(): throws LimitExceeded {
  val allowance = currentAllowance();
  match (allowance by Allowance) {
    Allowed => proceed();
    Denied  => throw LimitExceeded;
  }
}

decorator fn audited() {
  proceed();
  recordAudit();
}
```

- **`proceed()` は、包んだ op の中身を呼ぶ。** 二つ以上のデコレータを重ねた op では、外側の `proceed()` は、内側のデコレータごと、先へ進める。
- **`proceed` は、文脈キーワード**: 予約語ではなく、デコレータの本体の中の、修飾のない呼び出し `proceed(…)` だけが、この意味になる。本体の外では、ふつうの名前。
- **書ける位置は、二つだけ**: 文（`proceed();`）と、`match` のアームの本体（`Allowed => proceed();`）。引数は、書かない。値は、ない: `val` に入れる、`return` する、呼び出しの引数にする、は、書けない（`docs/check.md`「デコレータ」の DEC-19）。`for` と `disjoint` の中にも、書けない（同）。
- **本体に、`return` は、書けない**（同 DEC-21）。op の結果は、そのまま、呼び元に返る。
- **どの道も、`proceed()` を、ちょうど一度呼ぶか、例外を投げる**（同 DEC-20）。
- **本体の中の名前は、デコレータを宣言した名前空間で、解決する**（`docs/resolve.md`「スコープ」）。引数のうち、値の型のもの（`max: int`）は、本体の中で、値として読める。`any <種類>` の引数は、本体では、まだ、使えない（同 DEC-23）。
- 本体の意味（付けた所で、広げたものとして計算する）の持ち主は、`docs/check.md`「デコレータの本体」。

### op

```
OpDecl     = [ "export" ] FnHead [ ":" ( ReturnType { OpClause } | OpClause { OpClause } ) ] ( ";" | Block ) ;   (* FnHead と ReturnType は「関数の宣言」 *)
OpClause   = "effects" EffectElem { "," EffectElem }
           | "reads" ReadElem { "," ReadElem }
           | ThrowsClause ;
EffectElem = QualifiedName ;      (* 効果の名前か、直に書く「状態.操作」。どちらかは名前の解決が決める *)
ReadElem   = QualifiedName ;      (* read の名前か、直に書く状態の名前。同上 *)
```

- `;` で終われば葉、`Block` を持てば配線（§7.1）。葉は、人が中身を書く op で、状態に触れない: `reads` と `effects` の節を書けない（STATE-5。`docs/check.md`「状態に触れるもの」）。`throws` は書ける。
- `never` は、決して値を返さない op（`fn unauthorized(): never throws Unauthorized;`）。宣言の規則は `docs/check.md`「never を返す op」。
- `export` は、別の名前空間から呼べる op の印（`export fn login(in: LoginInput): LoginOk`）。`DocComment` とデコレータの後ろ、`fn` の直前に書く。付けられるのは、種類を書かない `fn`（op）だけで、ほかの宣言の前に書いたら構文エラー。意味は `docs/check.md`「export」。
- `throws`、`effects`、`reads` の三つは、その op を呼んだときに起きうることの並びで、区切りはどれも `,`（`throws Unauthorized, NotFound`、`effects createOrder, notifyUser`、`reads users, sessions`）。どの節も何度でも書け、同じ要素の重複は一つと数える。
- 配線の op の `throws`、`effects`、`reads` は、書かなくてよい: 本体が投げる例外、行う効果、読む状態は、本体の呼び出しと、デコレータから決まる。節に書いたものは、それに、足される（照合は、しない。決まりの持ち主は `docs/check.md`「状態に触れるもの」の「節に書く要素」）。葉の op は、`throws` を書く（そこが、定義）。
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
Type        = TypePrefix { "[" "]" | "?" } [ OnType ] ;
TypePrefix  = CoreType
            | ( "list" | "nullable" | "single" ) "<" Type ">"
            | QualifiedName ;
OnType      = "on" CaseRef { "|" CaseRef } ;               (* 絞り込み。ケースは、どれも、同じ分類のもの *)
CaseRef     = QualifiedName ;                              (* 分類の名前 "." ケースの名前: Stage.Accepted *)
CoreType    = "string" | "boolean" | "int" | "float" | "datetime" | "date" | "time" | "duration" ;
```

- 核の型は、言語が持つ、素の型で、どれも小文字（`docs/decisions.md`「決めたこと」359）。大文字で始まる型は、宣言された型。日時の四つの意味:

  | 核の型 | 意味 |
  |---|---|
  | `datetime` | ある瞬間 |
  | `date` | 日付（暦の一日。時刻帯を持たない） |
  | `time` | 一日の中の時刻（時刻帯を持たない） |
  | `duration` | 時間の長さ。リテラル `5m`、`30s` の型（「字句」の `Duration`） |

  その言語の型、通信の上の形、保存の形に写すのは、基底とプラグイン（Go は `docs/plugin-go.md`「型」、`docs/plugin-go-http.md`、`docs/plugin-go-sqlite.md`「しまい方」）。
- `Time` と `Duration` は、核の型の名前ではなくなった。ふつうの名前として読み、宣言がなければ、RES-1 が、今の語を示す（`docs/resolve.md`「手続き」の 1.7）。
- 形の語で書いた `single<T>`、`nullable<T>`、`list<T>` が、もとの形で、`T`、`T?`、`T[]` は、その省略形（`docs/decisions.md`「決めたこと」355、361）。`Type` を書けるどの位置にも、どちらでも書け、IR では同じ型になる（`docs/types.md`「形の書き方」）。
- 型の中に、その場で書ける、構造を持つ型は、形の語だけ。キーが決まっていない構造は、インデックスの model を宣言して、その名前を書く（「model」）。

- `T on C.K` は、分類のケースで**絞り込んだ型**: 「`T` で、分類 `C` の、ケース `K` にあるもの」（`fn capture(payment: Payment on Standing.Held)`。`docs/decisions.md`「決めたこと」368）。いくつかのケースは、`|` で並べる（`Payment on Standing.Held | Standing.Prepaid`: 「`Held` か `Prepaid`」）。意味と、渡し方は `docs/types.md`「型の種類」「互換」。
- `on` は、型の後ろ（`[]` と `?` の後ろ）に、一度だけ書ける。`on` の後ろには、`[]` も `?` も続けない: 絞り込んだ型の並びは、`list<Order on Stage.Draft>` と書く。`CaseRef` は、分類の名前と、ケースの名前を `.` でつないだもの（要素が一つだけの名前は、構文エラー）。
- `on` の後ろの `|` は、「A か B」（「decorator の宣言」の `|` と、同じ読み方）。型そのものの union（`x: A | B`）は、書けない: `T on K1 | K2` は、型は `T` のままで、出力する言語では、ただの `T` になる。
- `T on C.K` は書けない（なくした書き方。「決めたこと」）。パーサーは、型の位置に、名前と、その直後の `<` が来たら、`T on C.K` と書くことをメッセージで示して SYNTAX にする。`<>` は、形の語（`list<T>`、`nullable<T>`、`single<T>`）だけのもの。
- その場で書く model（`ModelBody`）は `Type` ではなく、`ArmType`（ユニオンのアーム）にだけ置ける。
- `void` は `ReturnType` にだけ書ける。
- `never` は、値が一つもない型。書ける所は、四つ: `ReturnType`、`Override` の型（「model」）、`classify` の対象（「classify」）、`CaseKind` の `of` の後ろ（「decorator の宣言」）。どれも、同じ意味から読む（`docs/types.md`「型の種類」の「never」）。
- `match` の `by` の後ろの `list` / `nullable` は型ではなく、標準の分類の名前（「配線」）。

## 配線

```
Block = "{" { Stmt } "}" ;
Stmt  = ValDecl | Assign | Collect
      | "return" Expr End
      | Call End
      | Match End
      | Disjoint End
      | [ "disjoint" ] For End ;

ValDecl = "val" ValName { "," ValName } End          (* 空で宣言する。名前を並べられる *)
        | "val" Ident [ ":" Type ] "=" Expr End ;    (* 値と一緒に宣言する *)
ValName = Ident [ ":" Type ] ;
Assign  = Ident "=" Expr End ;                       (* 空で宣言した val に、値を入れる *)
Collect = Ident "collect" Expr End ;                 (* 並びの val に、繰り返しの中から、値を集める *)
End     = [ ";" ] ;                                  (* 省けるのは、直前の字句が "}" のときだけ *)

Expr  = Call | Match | Atom ;
Call  = QualifiedName "(" [ Atom { "," Atom } ] ")" ;      (* op、read、効果の名前か、直に呼ぶ「状態.操作」 *)
Atom  = Path | Literal | "none" | RecordLit ;
RecordLit   = "{" [ RecordField { "," RecordField } [ "," ] ] "}" ;
RecordField = FieldName ":" Atom | Ident ;

Match    = "match" "(" [ Atom ] "by" ( QualifiedName | "list" | "nullable" ) ")" "{" MatchArm { MatchArm } [ ElseArm ] "}" ;
MatchArm = Ident { "," Ident } "=>" ArmBody ";" ;
ElseArm  = "else" "=>" ArmBody ";" ;
ArmBody  = Call | Atom | Throw ;
Throw    = "throw" QualifiedName [ "(" Atom { "," Atom } ")" ] ;

Disjoint = "disjoint" "{" Arm Arm { Arm } "}" ;      (* アームは二つ以上 *)
Arm      = Block End | ArmStmt ;
ArmStmt  = Assign | Collect | Call End | Match End | Disjoint End | [ "disjoint" ] For End ;
For      = "for" "(" "val" Ident "of" Atom ")" Block ;
```

文は七つ。意味の持ち主は、右の列。

| 文 | 例 | 意味の持ち主 |
|---|---|---|
| 値の入れ物の宣言（`ValDecl`） | `val session = authenticate(token);`、`val held;`、`val held: Held;`、`val user, latest, orders;` | `docs/check.md`「値の入れ物」 |
| 値を入れる（`Assign`） | `held = holdStock(stock);` | 同上 |
| 集める（`Collect`） | `summaries collect summarize(order);` | `docs/check.md`「繰り返し」 |
| `return` | `return place(order, held, credit);` | `docs/check.md`「op の宣言と本体」 |
| 呼び出しの文、`match` の文 | `createOrder(order);`、`match (offer by nullable) { … }` | 同上 |
| 互いに触れないアームの並び（`Disjoint`） | `disjoint { a = f(x); b = g(x); }` | `docs/check.md`「disjoint」 |
| 繰り返し（`For`）。前に `disjoint` を書ける | `for (val line of order.lines) { … }`、`disjoint for (val order of orders) { … }` | `docs/check.md`「繰り返し」「disjoint」 |

- **`val` は、値が一度だけ入る入れ物の宣言。** 値と一緒に宣言する（`val x = 式;`）か、空で宣言して（`val x;`）、後で、一度だけ、値を入れる（`x = 式;`）。空の宣言は、名前を `,` で並べられる（`val user, latest, orders;`）。値と一緒の宣言は、一つの文に、一つ（`val a = f(), b = g();` は、構文エラー）。型は、どの形でも、名前ごとに、書いてもよい（`val held: Held;`、`val held: Held, credit;`、`val x: T = f();`）。書かなければ、入れる式の型から導く（`docs/types.md`「式の型」）。
- **`;` は、文の終わりを、明示する印**（`End`）。文の最後の字句が `}` なら、省ける（書いても、同じ意味）。そうでなければ、要る。`}` で終わるのは、`match`、`disjoint`、`for`、レコードリテラルで終わる文（`return { id, name }`）。`;` は、前の文の一部で、「何もしない文」は、ない（`;;` は、構文エラー）。改行に、意味はない。`MatchArm` と `ElseArm` の `;` は、文の終わりではなく、アームの終わりで、省けない。
- **`disjoint` は、互いに触れないアームを、二つ以上、並べる。** アームは、ブロック（`{ … }`）か、文の一つ（`ArmStmt`: 値を入れる、集める、呼び出し、`match`、`disjoint`、`for`）。文の一つのアームに、`val` の宣言と、`return` は、書けない（宣言は、ブロックのアームの中に書く）。アームが一つだけの `disjoint` は、構文エラー。`disjoint` は、式ではない（`val x = disjoint { … }` は、書けない）: 値は、外で宣言した `val` に、アームの中で入れて、渡す。
- **`for` は、並びの要素の一つずつに、ブロックを行う。** 頭は、`(val 名前 of 並び)` の形だけ。並びは `Atom`（呼び出しは、先に `val` に受ける）。数を増やしながら回す形（`for (初め; 条件; 増やし方)`）は、ない。前に `disjoint` を書くと、回どうしが、互いに触れない繰り返し（`disjoint for (…) { … }`）。
- `for` と `disjoint` は、互いの中に、どの深さにも、書ける。`match` のアームの本体は、ブロックを持たない（`ArmBody`）ので、`match` のアームの中に、`for` と `disjoint` は、書けない。
- `return` は、`Block` のどこにでも、構文としては書けるが、書いてよいのは、op の本体の、いちばん外だけ（`for` のブロックと、`disjoint` のアームの中の `return` は WIRE-13。`docs/check.md`「繰り返し」「disjoint」）。
- 文の頭が `Ident` のときは、次の字句で分かれる: `=` なら `Assign`、`collect` なら `Collect`、ほか（`.` か `(`）なら `Call`。`disjoint` のアームの頭が `{` なら、ブロック（レコードリテラルは、文の頭に来ない）。
- なくした書き方は、新しい書き方をメッセージで示して SYNTAX にする: 文の頭の `const 名前 = 式;`（`val 名前 = 式;` と書く）、丸括弧のない `match` の頭（`match x by C { … }`。頭の全体を、丸括弧で囲む: `match (x by C) { … }`）。

`match` の形は三つ（どれに当たるかの検査は MATCH-7）。

| 形 | 対象 | 分類 |
|---|---|---|
| `match (item by WarrantyKind) { ... }`、`match (user by store.Registration) { ... }` | 値 | 値の分類か op の分類 |
| `match (carts by list) { ... }`、`match (in.token by nullable) { ... }` | 値。型が `list<T>`（`by list`）か `nullable<T>`（`by nullable`） | 標準の分類（ケースは、`by list` が `Empty` / `HasItems`、`by nullable` が `None` / `Some`。`docs/check.md`「分類」の「標準の分類」） |
| `match (by Checkout) { ... }` | 書かない（分類の宣言が対象を持つ） | 状態の分類 |

- `Atom` の `Path` は、`val` の名前・`for` の変数・op の引数・今の名前空間の状態と、そのフィールド（`query.page`）のどれか。状態は名前で値として書ける（`f(token)`、`match (token by nullable) { ... }`）。同じ名前の `val`、`for` の変数、引数があれば、そちらを指す。何を指すかは名前の解決が決める（`docs/resolve.md`「手続き」）。
- 値を省いた `RecordField`（`{ item }`）は `{ item: item }` の略（§10.2 `server.addToCart({ item })`）で、`item` は `Atom` の `Path` と同じに解決する。予約語の名前の束縛や引数は作れないので、予約語のキーは `{ op: 値 }` と書く。
- `Call` の引数は `Atom` だけ（§9「呼び出しの引数に呼び出しは書けない（A 正規形）」）。文法で閉じる。
- `Call` の `QualifiedName` が指すものは四つで、どれかは名前の解決が決める（`docs/resolve.md`「呼び出しの読み分け」）。意味は `docs/check.md`「状態に触れるもの」の「配線からの呼び出し」。

  | 呼び出し | 例 | 式として |
  |---|---|---|
  | op | `val session = issueSession(user);` | その op の戻り値（`docs/types.md`「式の型」） |
  | `read` | `val user = userByEmail(email);`、`return userByEmail(email);` | その `read` の結果の型の値 |
  | 効果（`effect` の宣言） | `createOrder(order);` | 値がない（文か、アームの本体に書く） |
  | 組み込みの効果を直に（`状態.操作`） | `users.post(user);` | 値がない。名前を付けない効果。書ける形と引数は `docs/check.md`「直に呼ぶ組み込みの効果」 |

- 状態を名前で値として書くこと（上の `Atom` の `Path`）が、名前を付けない `read`。
- `match` の分類の名前は修飾できる（`by store.Registration`）。分類はどの名前空間からも使える（`docs/check.md`「分類」）。
- `MatchArm` は、受けるケースを名前で並べる。複数なら `,` で区切る（`Draft, Cancelled => throw NotAccepted;`）。
- `ElseArm` は、ほかのアームが並べなかったケースのすべてを受ける（`Accepted => ship(order); else => throw NotAccepted;`）。書けるのは、`MatchArm` を一つ以上書いた後ろに、最後に一つだけ（文法で閉じる）。受けるケースと規則は `docs/check.md`「アームが受けるケース」。
- なくした書き方は、新しい書き方をメッセージで示して SYNTAX にする: `default => …`（`default` のアーム。受けるケースを名前で並べるか、最後のアームを `else => …` と書く）、`A | B => …`（ケースの後ろの `|`。区切りは `,`: `A, B => …`）。
- アームの本体（`ArmBody`）は三つのどれか一つ。文法で閉じる。

  | 本体 | 例 | 意味 |
  |---|---|---|
  | `Call` | `Registered => issueSession(user, in.body.password);` | 呼び出し一つ |
  | `Atom` | `LoggedIn => session;`、`None => none;`、`Standard => 1;` | 値そのもの。`none` もここに入る |
  | `Throw` | `Absent => throw type.Unauthorized;`、`Exceeded => throw rate.LimitExceeded(wait);` | その例外を投げる。引数は、例外の宣言の引数に、位置で渡す値。引数のない例外には括弧を書かない（`throw X()` は構文エラー）。書ける例外と引数の規則は `docs/check.md`「throw」 |

- `Path` の直後が `(` なら `Call`、そうでなければ `Atom`。
- `throw` はアームの本体にだけ書ける。文（`Stmt`）や `return` の式には書けない。
- `match` の頭は、全体を、丸括弧で囲む（`for` と同じ並び: キーワード、丸括弧に入った頭、ブロック）。`val warranty = match (…) { … }` の、`}` の後ろの `;` は、省ける（`End`）。

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
| 形の語（`single<T>` / `nullable<T>` / `list<T>`）は、型の、もとの形で（`T`、`T?`、`T[]` は、その省略形。`docs/decisions.md`「決めたこと」355、361）、型を書けるどの位置にも書ける。状態の宣言は `state 名前: Type;` | 形と型を揃えた（`docs/decisions.md`「設計書から離れた決定」S21）。状態を名前で参照したときの型が、宣言に書いた型そのものになる。位置ごとに書ける綴りを分けると、同じ型なのに位置で書き方が変わる規則が要る。以前の「空になりうる状態は `nullable<T>` とだけ書き、`single<T?>` は書けない」はなくした: `single<T>` は `T` なので、`single<T?>` は `T?`、つまり `nullable<T>` と同じ型で、「空」は型の一か所にしか現れない |
| 状態の分類での `match` は対象を書かない（`match (by C)`） | 対象は分類の宣言が持っていて、書かせると二か所になる |
| `prevents` / `keeps` の目的語は `effects` だけ | 効果を名前で書き分ける場面がない: 葉の op は自分では効果を持たず（STATE-5）、配線の op の、例外ごとの効果の状態は、検査器が計算する（REL-4）。デコレータが運ぶ複数の効果（`docs/check.md`「デコレータが運ぶ reads と effects」）も、名前では書き分けない: 目的語の `effects` は、その op のすべての効果 |
| 予約語をデコレータの名前に書ける（`DecoratorName`） | 語彙のデコレータは、することの名前で呼ぶ（`@db.list`、`@db.delete`）。その語が言語の節の頭や形の語と同じでも、語彙の側の名前を曲げさせない。デコレータの名前の位置は、`decorator` の直後か `@` の後ろ（修飾していれば最後の要素）で、予約語が来ても構文があいまいにならない。フィールド名（`FieldName`）と同じ扱い |
| デコレータの宣言に `reads` / `effects` の節を書ける。要素と区切りは op の節と同じ | デコレータの処理（op の前後に足すもの）が状態に触れることを、デコレータの引数から決められるようにする（`docs/decisions.md`「決めたこと」98）。`throws` と同じく、付いた op の関数の型に加わる。書き方を op の節とそろえて、同じ意味の節の綴りを二つ持たない |
| 残りの引数は `...名前: 型` と書き、型は値一つの型。最後に一つだけ、省略可能にできない | 並べる数が決まらない引数（op が行う効果の並び）を、リスト（`[a, b]`）で囲ませない。TS の書き方に寄せるが、型は配列でなく値一つの型で書く: デコレータの引数の型には `any effect` のような、配列にできない種類がある |
| デコレータの対象は、op が使う三つの単位のどれも（`exception`、`effect`、`read`） | 宣言が、それを使う op の実装の材料を言えるようにする（`@http.status(401)`、`@db.insert`、`@db.find`）。`union` や `classify` と違い、op が投げる、行う、読む「こと」の名前で、語彙が意味を足す相手になる |
| デコレータは `namespace` に付けられない。`for namespace` という対象もない | 名前空間は、複数のファイルの複数のブロックに分けて宣言できる（TypeScript の `interface` と同じく、同じ名前のブロックは一つにまとまる）。名前空間に付けたデコレータは、ファイルをまたいで一つにまとめて管理することになり、どのブロックに何が付いているかが直感的でない。印は中の宣言（op、model など）に付ける |
| `@external` と `@repeatable` は宣言ではなく核の印 | 語彙でも宣言できず、付く先がデコレータの宣言だけなので、文法で閉じる |
| 同じデコレータを一つの宣言に重ねられるのは、宣言が `@repeatable` を持つときだけ | 条件を一つずつ足す語彙（`@db.where(…)` を二つ）のように、重ねることに意味があるデコレータと、重ねれば書き間違いであるデコレータ（`@http.status` を二つ）を、宣言の側で分ける。既定は今までどおり一度だけ（DEC-13） |
| その場で書く model はユニオンのアームにだけ置ける | 合成する名前（`docs/ir.md`「正規化」）が、そこにしか決まらない。例外の値に置けた形（`exception X: { … }`）は、例外が引数を持つ形になってなくなった |
| 省略可能は型の `?` だけで表し、名前に付ける `x?: T` は書けない | 同じ意味の書き方を二つ持たない。型の `?` は `Item?[]` や、インデックスの値の型（`[name: string]: int?;`）のように、名前のない型の位置でも要るので、名前の側を落とす。設計書の例の `x?: T` は、`x: T?` と書く |
| 識別子は ASCII | §3.3 の命名規則が ASCII の大文字小文字で書かれている |
| `duration` のリテラルの単位は `ms` `s` `m` `h` `d` | 例にあるのは `m` と `s`。秒未満と日は使う場面が見えるので足す。週以上は月と年が一定でないので持たない |
| `datetime`、`date`、`time` のリテラルはない | 例にない。日時を仕様に直接書く場面がない |
| 付く先のない `DocComment` はエラー | 説明が IR から落ちることに気づけるようにする（§2「書き忘れ・曖昧さは構文や検査器で検出し」） |
| `model A is B omit f;` をなくし、`model A extends B { override f: never; }` と書く。`is` と `omit` は予約語から外す | 継承した型でできること（足す、なくす、型を変える）を一つの書き方にまとめた（`docs/decisions.md`「設計書から離れた決定」S22）。どの生成規則も使わない語を予約したままにすると、名前に使える語を減らすだけになる |
| 本体が空の `model A extends B { }` は書ける | 継承した model は元の model とは別の型（`docs/types.md`「継承した model」）で、別名ではない。以前の「`omit` のない `is` は書けない」の理由（別名にしかならない）が当たらない |
| `override` の型は省略でき、省略は「型を変えない」。なくす書き方は `override f: never;` | デコレータを足すだけのときに、元の型を書き写させない（元の型を変えたときに、書き写した側が食い違う）。`never` は「そのフィールドに入る値がない」と読める |
| フィールドの既定値（`page: PosInt = 1`）をなくした | 共有する型に書いた既定値は、その型を使うすべての名前空間が埋めることになる。値がないときに誰が何を入れるかは振る舞いで、型ではなく、境界のデコレータが言う（`@http.default`、`@cli.default`。`docs/vocab.md`「`http` の語彙」、`docs/plugin-go-cli.md`「語彙」。`docs/decisions.md`「設計書から離れた決定」S24）。呼び出しの位置を状態で埋める `bind`（「bind」）は別の仕組みで、変えない |
| `reads` `read` `as` `never` `throw` `override` `export` は予約語。綴りは仮で、後で別の語に変えうる | どれも構文の骨（宣言の頭、節の頭、文の頭、核の型、修飾）で、識別子が来る位置の名詞ではないので、文脈キーワードの基準（下の行）に当たらない。`never` は `void` と同じく型の位置に来るので、予約しないと同じ名前の型と区別できない。`override` と `read` は予約してもフィールド名には書け（`FieldName`）、`read` はデコレータの名前にも書ける（`DecoratorName`）。綴りを変えるときは、この表と「予約語」の表を変える |
| `export` と `override` は、予約語の表の「修飾」の行に置く | 宣言やフィールドの頭の語の前に置いて性質を足す語で、宣言の頭でも節の頭でもない。色分けも分ける（`docs/editors.md`「TextMate の文法」） |
| `export` を書けるのは `op` の前だけ | 別の名前空間から隠せるのは op だけ（`docs/check.md`「export」）。ほかの宣言に書ける形にしておくと、意味のない `export` が検査まで残る |
| `throws`、`catches`、`effects`、`reads` の並びの区切りは、op の節でもデコレータの宣言の節でも `,`。`throws A \| B`、`effects a \| b` はなくした（`docs/decisions.md`「設計書から離れた決定」S25） | 三つの節（`throws`、`effects`、`reads`）は、どれも「その op を呼んだときに起きうること」の集合を言うので、同じ書き方にする。記法は「どれか」と「すべて」を書き分けない: 例外は呼び出しを終わらせるので、起きるのは高々一つ。効果と読むことは「このうちのいくつか」。「すべてを行う」と言うのは、デコレータの引数だけ（残りの引数に並べた `@effects(a, b)`）。`,` は `throws` の出どころ（Java）の書き方に近い。`\|` は、型の選択（`int \| float`）、`for` の対象の並び、一つの引数に渡す宣言の集合、ケースの並び（遷移と `match` のアーム）に残る |
| `exception`、`effect`、`read` は、同じ形で書く: 名前、引数、（`read` は結果の型、）宛先。`exception X: T` と、引数の括弧のない効果の宣言（`effect fn e: effects …`）はなくした | 三つは、op が使う最小の単位で、op はそれを組み合わせる。配線から op と同じに使えるように（`userByEmail(email)`、`createOrder(order)`、`throw LimitExceeded(wait)`）、op と同じ引数の並びを持たせる。例外の値を一つの model に包ませない: 運ぶ値は引数として並べる |
| `effect` と `read` は引数がなくても `()` を書き、`exception` は引数がなければ括弧を書かない | `effect` と `read` は呼び出すもので、呼び出しはいつも括弧を持つ（op と同じ）。例外は名前で並べ（`throws A, B`）、名前で投げる（`throw A`）もので、値を運ばない例外が普通。どちらも、同じ意味の書き方を二つ持たない |
| `read` の結果の型は省略できない。`reads` の後ろに操作は書かない | 読むことは、値を得ること。何を得るかを宣言に書かせ、配線の式の型を宣言だけから決める。操作を書かないのは、状態を読むことが一種類だから |
| 名前を付けない形は、配線にある: 状態を名前で読むこと（`f(users)`）と、組み込みの効果を直に呼ぶこと（`users.post(user);`）。配線の op の `reads` と `effects` の要素も、名前でも、直にでも書ける（`reads userByEmail` と `reads users`、`effects addUser` と `effects users.post`） | 名前を付けるかどうかを、書く人が選べるようにする。説明もデコレータも要らない読み書きに、宣言を一つ足させない。分類の遷移だけは直に書けない: 前後のケースを節や呼び出しの中に書くと、並びが読めなくなる |
| `as` の前には対象を、宣言の `of` の後ろと同じに書く。対象が一つでも省略できない | どの対象が何の型になるかを、対象が複数ある状態の分類と同じ形で読めるようにする |
| `as` の後ろの `else` は予約語（`docs/decisions.md`「決めたこと」152） | 型の位置に来るので、予約しないと同じ名前の型と区別できない（`never` と同じ理由）。フィールド名には書ける（`FieldName`）。アームの頭の `else` も、ケースの名前（`Ident`）の位置に来るので、同じ予約語で足りる |
| 分類のケースは、どれも名前を持つ。名前のないケース `default;` と、`condition` の宣言は、なくした（`docs/decisions.md`「決めたこと」154、155） | 残りが意味のある状態なら、名前が要る。値がない側なら、残りのケース（`as else`）がある。仕様が使わない場合は、宣言しない: 宣言したケースの外の値は、分類の誤り（`docs/check.md`「閉じた分類」）。`default` が受ける正当な残りが、なくなった。`condition` は、成り立たない側が `default` になる糖衣だったので、一緒になくなる |
| `default` は予約語のまま。`condition` は予約語から外す | `default` が書かれていた位置（ケースの名前、アームの頭）は `Ident` の位置で、予約を外すと、なくした書き方 `default;` が「`default` という名前のケース」として通る。`condition` が書かれていた位置は宣言の頭で、`Ident` で始まる宣言はないので、予約を外しても、なくした書き方が別の意味で通ることはない（`is` / `omit` と同じ扱い。パーサーは、宣言の頭の `condition` を、メッセージのために見分ける） |
| `match` の「ほかのすべて」は `else => …`。`MatchArm` の後ろに、最後に一つだけ（`docs/decisions.md`「決めたこと」156） | 残りを言う語は、分類のケースの `as else` と同じ語にする。「ほかの」は、先に名前で並べたアームがあって決まるので、`else` だけの `match` と、`else` の後ろのアームは、文法で閉じる |
| ケースの並びの区切りは `,`（アームの `A, B => …`、遷移の `effects C as A, B -> any`）。`\|` は、デコレータの丸括弧の中の、引数の型（`string \| int`）と、適用の引数の値（`ArgValue`）、絞り込んだ型の `on` の後ろのケース（`Payment on Standing.Held \| Standing.Prepaid`）にだけ書ける（`docs/decisions.md`「決めたこと」157、358、368） | `\|` は、その場で書く union（「このどれか」）。デコレータは、言語の基底によらない、プラグインが読む値なので、許す。`on` の後ろは、型は `T` のままで、ケースを絞るだけなので、許す。変数やフィールドの、型そのものには、書けない: 出力する言語が union を持たないことがある。ケースの並びは、「これらのケースのとき」を並べるだけで、節の並び（`throws A, B`。`docs/decisions.md`「設計書から離れた決定」S25）と同じ。ケースを並べる位置は二つ（アーム、遷移）なので、同じ区切りにする |
| 絞り込んだ型は `T on C.K`。いくつかのケースは `\|`。`T on C.K` は、なくした（`docs/decisions.md`「決めたこと」368） | `<>` は、型を作る語（形の語）の書き方で、絞り込みは、それとは別のもの。もとの型が、先に来る。`on` は、「〜に限る」（`any effect on users.post` と同じ役）。`as` は、分類のケースの型（`X: Order? as Order;`）と重なるので、使わない |
| 行の遷移は、宛先の後ろの `as` の節に、後のケースだけを書く（`effects orders.put as Stage.InProduction`）。前のケースは、行の引数の型で言う（`order: Order on Stage.Accepted`）。状態の遷移（`effects Checkout as A -> B`）とは、宛先で分かれる（`docs/decisions.md`「決めたこと」163、350、369） | 効果は、二つの別のことを言う: どこに、何の操作で書くか（宛先）と、受け取った行の状態がどうなるか。前は、引数の性質で、後は、効果の約束。前を、型で言えば、呼ぶ側の確かめが、型の検査一つで済む。状態の遷移は、引数を持たないので、前も、`as` に書く |
| 行の遷移のケースは、分類の名前から書く（`Stage.Accepted`）。ケースの名前だけの形（`as Accepted`）にしない | 絞り込んだ型の `Order on Stage.Accepted` と同じ綴りにする。行の型には分類がいくつもありうるので、ケースの名前だけでは、どの分類の話かが読めない |
| 行の遷移で、後のケースを約束しないことは、`as else` と書く。`any` は、書かない | 後ろは、受け取るものではなく、決まっていない残りなので、残りを言う語（`as else`、`else => …`）にする。`any` を書けると、「どのケースにもなる」と読める。前が「どのケースでも」は、行の引数を絞り込まないことで言う |
| ケースの `as none` は、「値がない」を言う（`docs/decisions.md`「決めたこと」165） | 「値がない」を言う語は、配線の値の `none` で、もうある。`null` を足さない。`as else`（残り）は、残りがたまたま「値がない」ときにも書けるが、「ない」ことを名指しする語ではない |
| `throw` はアームの本体にだけ書ける | 「この場合は失敗」を書くためのもの。本体のどこにでも書ける文にすると、下の行が評価されない配線を書ける |
| `throw` は、例外の引数に値を渡せる（`throw E(a, b)`）。値は `Atom` | 値を運ぶ例外も、配線から投げられるようにする（以前は、値を運ぶ例外を投げられるのは葉の op だけだった）。引数は呼び出しと同じ形（位置で渡す `Atom`）にして、書ける値を位置ごとに分けない |
| アームの本体の値は `Atom` のすべて（名前、リテラル、`none`、レコードリテラル） | 呼び出しの引数と同じ集合にして、位置ごとに書ける値を分けない |
| `match` の分類の名前は修飾できる | 分類はどの名前空間からも使える（`docs/decisions.md`「設計書から離れた決定」S18） |
| 名前で渡す引数の後ろに位置で渡す引数は書けない | 対応が読めなくなる。TS や Python と同じ |
| `return` の後ろに `Atom` も書ける | `return me;` のような、束縛した値をそのまま返す配線を禁じる理由がない |
| 配線の束縛は、`val`（値が一度だけ入る入れ物）。空で宣言して、後で入れられる。`const` は、なくした（`docs/decisions.md`「決めたこと」309、310） | 配線は、値を受けて渡す意図を書くもので、実装のコードではない。束縛は、定数でも、変数でもない。空の宣言は、`disjoint` のアームや、`for` の中から、外へ、値を出すために要る |
| `;` は、文の終わりを明示する印で、文が `}` で終われば、省ける（同 313） | ブロックで終わる文（`match`、`disjoint`、`for`）の終わりの書き方を、一つの決まりにする。決まりは、字句だけで決まる（直前が `}` か） |
| `match` の頭は、全体を、丸括弧で囲む（同 315、316） | 配線の、ブロックを持つ文の頭を、同じ見え方にする（`for (…) { … }` と `match (…) { … }`） |
| 互いに触れないアームの並びのキーワードは、`disjoint`（同 343）。アームに、名前は、付けない（同 311）。`=>` は、「〜のとき」を言う記号で、名前を付ける所には、使わない（同 303） | 実行の手段（並列）ではなく、コードの性質を言う語にする。名前は、`val` に付いているので、アームの順を入れ替えても、名前の指す値は、変わらない |
| 繰り返しは、TS 風の `for (val x of xs)`。数を増やしながら回す形は、持たない。集めるのは、`並びの val collect 値;`（同 308、314、318） | 図の繰り返しは、「並びの一つずつに、同じ流れを行う」だけを言う。集める文は、入れる先を、先に書く（`=` と同じ順） |
| `val`、`disjoint`、`collect`、`case` は、予約語。`const` は、予約語から外す（同 341） | どれも、構文の骨（文の頭、節の語）。`const` は、どの生成規則も使わず、書かれていた位置（文の頭）は、`Ident` が、単独では文にならない位置なので、予約を外しても、なくした書き方として、見分けられる |
| 対象のない分類は、`of` を省かず、`of never` と書く（同 328、329） | 当たる値のない型（`never`）を、分類している、と捉える。対象の種類を、増やさない。`of` を省く形にすると、書き忘れと、区別できない |
| デコレータの宣言で、分類のケースは、`case of …` と書き、`of` の後ろに、`any`、型、分類の名前を書く。`case` は、`case of any` の省略（同 331、346、349）。分類そのもの（`for classify`）は、まだ、書けない（同 341） | 絞りが、いつも、`of` の後ろに来る。`for model` と `for field`、`any variant` と、同じ並び: 部分を言う語（`case`）と、全体を言う語（`classify`）が、別なので、後で、分類そのものを足しても、綴りが、ぶつからない。`any` を型にしないのは、仕様に、型のない値を、持ち込まないため |
| 関数の宣言の頭は、`fn`。種類（`read`、`effect`）は、その前に書き、書かなければ、op（同 347）。`fn` は、予約語。`op`、`read`、`effect` は、予約語のまま | op、`read`、効果は、どれも、基底は関数。頭が一つなら、新しい種類が生まれても、文法の規則は、一つで済み、種類の語を、`fn` の前の、文脈キーワードにできる。状態に触れる末端だけに、印が付く。今ある三つの語を、文脈キーワードに変えないのは、`for op`、`any effect` の位置で、今のまま読むため |
| 関数の戻り値は、どの種類も、`: 型` で、省けば `void`（同 348） | 効果は `void` のほかを返せず、省いた形が、ふつうになる。`read` と効果に、戻り値の、別の文法を持たない |
| `)` の後ろに、戻り値か節を書くなら、`:` で始める。何も書かなければ、`:` も書かない（同 351） | `:` の後ろが、関数の型: 戻り値と、投げるもの、読むもの、変えるもの。`(引数): 戻り値` の形と、後ろに並ぶ節の、継ぎ目がなくなる。`:` の意味は、引数や `val` と同じ、「〜の型は」のまま。効果だけ `: void` を省く、という種類ごとの別扱いが、要らない |
| 関数が、状態を読むこと、変えることは、どの種類も、`reads` と `effects` で書く。`read fn` と `effect fn` の `on` は、なくした（同 350） | 同じ関係を、同じ語で言う。`read fn … reads users` は、語が重なるが、修飾子は、行の頭の目印で、節が、中身。`on` は、デコレータの引数の絞り（`any effect on users.post`）と、絞り込んだ型（`Order on Stage.Draft`。「型」の `OnType`）に残る。関数の戻り値の後ろの `on` は、後ろが `<分類>.<ケース>` の形のときだけ、絞り込んだ型として読む。それ以外（`read fn r(): T on 状態;`、`fn f(): T on 状態;`）は、なくした書き方として、メッセージが、`reads` と `effects` を示す |
| 状態の遷移は、宛先の後ろの `as` で書く（`effects carts as HasItems -> Empty`）。宛先の後ろの `:` は、なくした（同 350） | `as` の後ろは、いつも、ケースの遷移（行の遷移と、同じ語）。`:` は、型を言う記号だけになる。宛先が `状態.操作` か、状態（状態の分類）かは、名前の最後の要素が `post` / `put` / `delete` かで分ける: `:` で分けていたときと違い、その三つの語で終わる名前の状態や分類は、状態の遷移の宛先に書けない |
| 日時の核の型は、`datetime`、`date`、`time`、`duration`。核の型は、どれも小文字。`Time` と `Duration` は、なくした（同 359） | 言語が持つ型は小文字、宣言された型は大文字で始まる、と一つの決まりで言える。日時は、設計する人が、そのまま考える概念なので、素の型にする。表現（文字列か、数か）は、仕様が言うことではなく、基底とプラグインが、決まった表で写す |
| デコレータの宣言の、付ける先の並びの区切りは `,`（`for model, state, op`）。`for model \| op` は、なくした（同 358） | `for` の後ろは、型ではなく、修飾する先の一覧。節の並びと同じ `,` にすると、`\|` は、「その場で書く union」の一つの意味になる |
| 引数の種類には `any` を書き（`error: any exception`）、付ける先には書かない（`for effect`）（同 357） | `any <種類>` は、種類から、型を作る。`for` の後ろは種類、`:` の後ろは型。引数の種類から `any` を取ると、種類の語が、そのまま型として扱われる |
| キーが決まっていない構造は、model の本体のインデックスで書く（`model Attrs { [name: string]: string[]; }`）。`Record<K, V>` と、予約語 `Record` は、なくした（同 360） | 構造を持つ型は、どれも、名前を付けて宣言する（union と同じ）。「何を、何で引くのか」に、名前と、説明と、キーの名前が付く。大文字で始まる、言語の語が、なくなる |
