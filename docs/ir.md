# IR

## この文書が決めること

IR（中間表現）の構造と、表面の構文から IR への正規化、JSON での形、「その名前空間から見た IR」を決める。IR を正とし、表面の構文は糖衣を許すが、IR は一つの形に正規化する（出どころは設計書 §2）。Go の型は `ir` パッケージがこの文書を写したもので、JSON スキーマ（`schema/ir.schema.json`）はそこから生成する（`docs/requirements.md`「プラグインの書き方」）。

構文の形は `docs/grammar.md`、名前の解決は `docs/resolve.md`、型の互換は `docs/types.md`、IR の上で計算する集合（例外・効果）は `docs/check.md`「計算」が持ち主。この文書はそれらの結果を IR のどこに置くかを決める。

## 原則

- **参照はすべて完全修飾名（FQN）**。`server.User` のように、ルートからの名前空間のパスと宣言名を `.` でつないだ文字列。グローバルの宣言は宣言名だけ（`db`）。表面の構文の「同じ名前空間なら修飾しない」は IR に残さない。
- **宣言の順序は出現順**。ファイルの順は、ユーザーのファイルをパスの辞書順に並べ、その後に語彙のファイル（`Span.file` が `hozo:` のもの）を同じく辞書順に並べたもの。ファイルの中は位置順。同じ名前空間が複数の場所にあれば、この順で並べて一つにする（§3.2）。出力の決定性（`docs/requirements.md`「品質の要件」）のため、IR のどの列も集合ではなく順序付きの列。診断の並び（`docs/diagnostics.md`「順序」）もこのファイルの順に従う。
- **すべての宣言は位置（`span`）を持つ**。言語サーバーと診断のため。JSON にも出す。
- **計算した結果も IR に持つ**。例外の集合・効果の集合・読む状態の集合・純粋かどうか・`bind` で埋まる位置は、プラグインが自分で計算しなくて済むように、`docs/check.md` の計算結果を IR に置く。宣言から決まること（状態の形、分類が型で決まるか、アームの中で絞り込まれた型）も同じ。

## 構造

TypeScript 風の記法で書く。`?` は省略可（JSON での扱いは「JSON」）。

### Program

```ts
interface Program {
  version: "0.20";                 // この文書の版。Program か View の構造が変わったら上げる（「版」）
  namespaces: Namespace[];        // グローバルは name "" の Namespace として先頭に置く
  binds: Bind[];
}

interface Namespace {
  name: string;                   // FQN。"server.admin"。グローバルは ""
  description?: Doc;              // namespace ブロックの直前の DocComment。複数あれば最初のもの
  decls: Decl[];                  // 出現順
}

type Decl = Model | Scalar | Union | State | Exception | Effect | Read | Classify | Decorator | Op;

interface DeclBase {
  kind: "model" | "scalar" | "union" | "state" | "exception" | "effect" | "read" | "classify" | "decorator" | "op";
  name: string;                   // FQN
  description?: Doc;
  decorators: Application[];      // 付いたデコレータ。順序の意味は「デコレータの適用」。付けられる宣言は Decorator.targets の種類（DEC-1）
  span: Span;
}

interface Span { file: string; start: Pos; end: Pos; }
// file は三つの形のどれか: ユーザーのファイルは hozo.toml からの相対パス、
// 標準の語彙は "hozo:vocab/<名前>.hozo"、プラグインの語彙は "hozo:plugin/<プラグイン名>/<マニフェストからの相対パス>"
interface Pos { line: number; col: number; }              // 1 始まり。col は UTF-8 のバイトではなく Unicode のコードポイント数
```

### Doc

```ts
interface Doc {
  summary: string;                // 最初の段落（Markdown）
  body: string;                   // 全文（Markdown。summary を含む。タグの行は含まない）
  params: { name: string; text: string }[];      // @param。name は解決済み（op の引数名）
  throws: { exception: string; text: string }[]; // @throws。exception は FQN
  see: string[];                  // @see。FQN
}
```

### 型

```ts
type Type =
  | { kind: "core"; name: "string" | "boolean" | "int" | "float" | "datetime" | "date" | "time" | "duration" }
  | { kind: "ref"; target: string }                    // model / scalar / union の FQN
  | { kind: "optional"; of: Type }
  | { kind: "array"; of: Type }
  | { kind: "case"; classify: string; cases: string[]; of: Type }    // 絞り込んだ型 `of on classify.cases`（docs/types.md「型の種類」の「絞り込んだ型」）。
                                                                    // cases は昇順ではなく、書かれた順（アームの中の対象なら、アームが受ける順）
  | { kind: "never" };                                               // 値が一つもない型。Op.returns（決して値を返さない）、対象のない分類の
                                                                     // Classify.subject.type、CaseKind.subject（`case of never`）にだけ現れる
```

- `void` は型ではなく、`Op.returns` が `null` であること。
- `never` は、値が一つもない型（意味と、書ける所は `docs/types.md`「型の種類」の「never」）。IR で現れるのは、`Op.returns`、対象のない分類の対象（`Classify.subject.type`）、デコレータの宣言の `CaseKind.subject` だけ。ほかの位置（フィールド、引数、`as` など）には現れない。`override f: never;` は、型として載らず、`Model.extends.removed` になる。
- 形の語（`list<T>` / `nullable<T>` / `single<T>`）は IR にない。`array(T)` / `optional(T)` / T になる（「正規化」）。
- その場で書いた model（`{ ... }`）は IR にはない（「正規化」）。

### model / scalar / union

```ts
interface Model extends DeclBase {
  kind: "model";
  fields: Field[];                // 継承した model なら、受け継いだ分も入れた展開済みの並び（docs/types.md「継承した model」の「フィールド」）。
                                  // インデックスの model なら []
  index?: {                       // インデックスの model（`model Attrs { [name: string]: string[]; }`。docs/types.md「インデックスの model」）
    name: string;                 // キーの名前（説明のための名前）
    key: Type;                    // キーの型（string か、元が string の scalar）
    value: Type;                  // 値の型
    span: Span;                   // インデックスの位置（`[` から `;` まで）
  };
  extends?: {                     // 継承した model（`model D extends B { ... }`）の記録
    base: string;                 // 元の model の FQN（直接の元）
    removed: string[];            // `override f: never;` でなくしたフィールドの名前（書かれた順）
    overridden: string[];         // `override` で型を変えたか、デコレータを足したフィールドの名前（書かれた順）。これらのフィールドの説明は、`override` に書いたもの（なければなし）
  };                              // 足したフィールドは、fields のうち、元の model の fields に名前がないもの
  synthesizedFrom?: string;       // その場で書いた model の出どころ。"server.OrderPage.Ready" のように持ち主の FQN
}

interface Field {
  name: string;
  description?: Doc;
  type: Type;                     // 省略可能なら optional
  decorators: Application[];
  span: Span;                     // 受け継いだままのフィールドは元の model のフィールドの位置。`override` したフィールドは `override` の位置
}

interface Scalar extends DeclBase {
  kind: "scalar";
  base: Type;                     // core か、別の scalar への ref
}

interface Union extends DeclBase {
  kind: "union";
  variant: "arms" | "set";        // arms: model のバリアント。set: 例外か効果の集合。混在は検査 UNION-1 のエラー
  arms: { name: string; type: Type; description?: Doc; span: Span }[];   // variant が arms のとき
  members: { target: string; description?: Doc; span: Span }[];   // variant が set のとき。target は例外か効果の FQN。メンバーがユニオンなら展開済み
}
```

### state / exception / effect

`exception`、`effect`、`read`（次の節）は、op が使う最小の単位で、どれも引数の並び `params` を持つ（`docs/grammar.md`「exception / effect / read」）。

```ts
interface State extends DeclBase {
  kind: "state";
  type: Type;                     // 状態の型。配線が名前で参照したときの値の型（docs/types.md「形の書き方」）
  shape: "list" | "nullable" | "single";   // 列数の範囲。type の最も外側から決まる（docs/check.md「状態の形」）:
                                           // array なら list、optional なら nullable、それ以外は single
  element: Type;                  // type から決まる: list と nullable なら type の of、single なら type そのもの。
                                  // optional にはならない（STATE-3。当たった状態は optional を外して載せる）
}

interface Exception extends DeclBase {
  kind: "exception";
  params: OpParam[];              // 運ぶ値（`exception X(a: T, b: U);`）。値を運ばない例外は []。OpParam は「op」
}

interface Effect extends DeclBase {
  kind: "effect";
  params: OpParam[];              // 受け取る値（`effect fn e(order: Order): effects …;`）。なければ []
  throws: string[];               // この効果が投げうる例外の FQN（`: throws E effects …`。docs/check.md「read と効果」）。ユニオンは展開済み。
                                  // 書いた順で、なければ []
  target: EffectTarget;           // 宛先は一つだけ
  writes: string[];               // この効果が書き込む基底の状態の FQN（docs/check.md「状態の形」の「効果が書き込む状態」）
  row?: RowTransition;            // 行の遷移（`as` の節。docs/check.md「行の遷移」）。書いていなければ持たない。
                                  // 持つのは、target が builtin の効果だけ
}

type EffectTarget =
  | { kind: "builtin"; state: string; operation: "post" | "put" | "delete" }   // 基底の状態の組み込みの効果
  | { kind: "transition"; of: string; from: CaseSet; to: CaseSet };            // 分類の遷移。of は状態の分類の FQN か、
                                                                               // 組み込みの分類を持つ基底の状態の FQN
type CaseSet = "any" | string[];  // ケース名の列（書かれた順）

interface RowTransition {         // 効果の後、受け取った行が、どのケースにあるか。書き込む状態と操作は、Effect.target にある
  param: string;                  // 行の引数の名前（Effect.params のうち、型が、状態の要素の型か、それを絞り込んだ型の、ただ一つのもの）。
                                  // 効果の前のケースは、この引数の型が言う（kind "case" なら、その cases。そうでなければ、どのケースでも）
  classify?: string;              // 行の分類の FQN（値の分類。対象の型は、状態の要素の型 T か T?）。`as else` では持たない
  to?: string;                    // 効果の後のケースの名前。持たなければ、決まっていない（put の `as else`）。post では、いつも持つ。
                                  // delete では、いつも、分類のない側のケース（Classify.absent）
}
```

- `RowTransition` は、行の遷移を、生成したテストで確かめる段階（`docs/requirements.md`「段階」）が要るものを持つ: 状態と操作（`Effect.target`）、行の引数（その型が、前のケース）、分類、後のケース。
- `row` を持たない効果の意味は、操作から決まる（`docs/check.md`「行の遷移」の「操作ごとに書ける形」の「`as` を書かない」の列）。

### read

```ts
interface Read extends DeclBase {
  kind: "read";
  params: OpParam[];              // 読むときに受け取る値（`read fn r(email: Email): User? reads users;`）。なければ []
  returns: Type;                  // 結果の型。null にも never にもならない
  state: string;                  // 読む基底の状態の FQN。宛先は一つだけ（docs/check.md「read の宛先」）
}
```

### classify

```ts
interface Classify extends DeclBase {
  kind: "classify";
  subject: { kind: "type"; type: Type }             // 値の分類。対象のない分類（`of never`）は、type が never
         | { kind: "op"; op: string; type: Type }   // op の分類。op は FQN、type はその op の returns（対象の型）
         | { kind: "states"; states: string[] }     // 状態の分類。基底の状態の FQN（書かれた順）
         | { kind: "exception" };                   // 例外の分類（`of any exception`。docs/check.md「例外の分類」）。対象は、catch が受けた例外
  cases: { name: string; description?: Doc; decorators: Application[]; as: CaseTyping[]; exceptions?: string[]; rest?: boolean; span: Span }[];
                                  // 宣言したケースのすべて（docs/check.md「閉じた分類」）。宣言順。as は書かれた順（なければ []）。
                                  // decorators は、ケースの前に書いたデコレータ（書かれた順。なければ []）。付けられるのは、
                                  // Decorator.caseTargets に、そのケースを含む CaseKind を持つデコレータ（DEC-1）
                                  // exceptions と rest は、例外の分類のケースだけが持つ（as は []）: exceptions は、そのケースに当たる例外の
                                  // FQN（書かれた順。例外のユニオンは、展開済み）。rest は、`any exception as else` と書いたケースで true
  decidedBy: "type" | "predicate" | "none";   // ケースが何で決まるか（docs/check.md「ケースの判定」）。type なら述語を持たない。
                                  // none は、対象のない分類: 判定するものがなく、述語も持たない（docs/check.md「対象のない分類」）
  absent?: string;                // ない側のケース（docs/check.md「ない側のケース」）の名前。なければ持たない。
                                  // decidedBy が predicate でも、このケースだけは述語を持たず、値がないことで決まる
}

interface CaseTyping {            // ケースの `対象 as T`、`対象 as else`、`対象 as none`（docs/check.md「ケースの型」「残りのケース」「ない側のケース」）
  state?: string;                 // 状態の分類のとき、その対象の状態の FQN。値の分類と op の分類では持たない（対象は一つ）
  type: Type;                     // そのケースでの対象の型。`as else` と `as none` なら、書いた型がないので、docs/types.md「残りのケースでの対象の型」
  by: "presence" | "variant" | "predicate" | "else" | "none";   // 絞り込みの種類（docs/types.md「型で決まる絞り込み」）。else は `as else`、none は `as none`
  span: Span;
}
```

### decorator

```ts
interface Decorator extends DeclBase {
  kind: "decorator";
  params: Param[];
  fn: boolean;                    // 関数に付けるデコレータ（`decorator fn`）か。真なら targets は ["op"]。
                                  // 節（throws / catches / reads / effects）と body を持てるのは、fn が真の宣言だけ
  targets: ("op" | "field" | "state" | "exception" | "scalar" | "model" | "effect" | "read" | "case")[];   // `decorator fn` なら ["op"]。case は、分類のケース
  caseTargets: CaseKind[];        // targets の "case" ごとの、絞り（書かれた順）。なければ []
  entry: boolean;                 // @entry が付いているか（このデコレータの付いた fn は、境界の入口。docs/grammar.md「decorator の宣言」）
  hint: boolean;                  // @hint が付いているか（解釈されなくても、よい。DEC-26）
  repeatable: boolean;            // @repeatable が付いているか。偽なら、一つの宣言に一度だけ付けられる（DEC-13）
  throws: ThrowsClause[];         // `throws` の節（包むことで、その fn が投げる）
  outerThrows: ThrowsClause[];    // `outer throws` の節（fn の外で起きる。境界の外から見たときだけ、加わる: DEC-10）。書かれた順。なければ []
  catches: CatchesClause[];
  reads: ReadRef[];               // `reads` の節の要素（書かれた順。節が複数なら連結）。なければ []。形は「op」の ReadRef で、FQN の位置に
                                  // デコレータの引数名 "$param:名前" も来る: any state の引数は direct の state に
  effects: EffectRef[];           // `effects` の節の要素（同じ）。形は「op」の EffectRef（ユニオンは展開済み）で、any effect の引数は
                                  // named の effect に "$param:名前"。なければ []
                                  // 付いた op に何が運ばれるかは Op.carried（docs/check.md「デコレータが運ぶ reads と effects」）
  body?: Wiring;                  // 本体（docs/grammar.md「デコレータの本体」）。本体のない宣言には、ない。形は「配線」の Wiring で、
                                  // 包んだ op の中身を呼ぶ所は、Callee が proceed の呼び出し。値の型の引数（Param）は、
                                  // Atom の name の、path の頭の名前で読む。意味は docs/check.md「デコレータの本体」
  alias?: { target: string; args: Arg[] };   // 名前付きデコレータ。target は汎用デコレータの FQN。
                                             // alias を持つ宣言は params / targets / throws / outerThrows / catches / reads / effects を持たず、
                                             // entry と hint と repeatable は false で、どれも target のものを使う
}

interface CaseKind {              // 分類のケースの絞り（docs/grammar.md「decorator の宣言」の CaseKind）。持つのは、どちらか一つか、
                                  // なし（`case of any` と、その省略の `case`）
  classify?: string;              // 分類の名前で絞ったとき（`case of Actor`）、その分類の FQN
  subject?: Type;                 // 対象の型で絞ったとき（`case of Order`、`case of never`）、その型。`of never` は、kind が never の型
}

interface Param {
  name: string;
  type: ParamType;
  optional: boolean;              // 説明は DeclBase.description.params から引く
  rest: boolean;                  // 残りの引数（`...名前: 型`。docs/grammar.md「decorator の宣言」）。params の最後の一つだけが true になりうる。
                                  // true なら optional は false で、type は渡す値一つの型
}

type ParamType =
  | { kind: "type"; types: Type[] }                   // `int | float` は types が二つ。普通は一つ
  | { kind: "any"; of: "exception" | "effect" | "state" | "op" | "model" | "variant"; on?: { state: string; operation: "post" | "put" | "delete" } }
  | { kind: "any"; of: "case"; case: CaseKind };     // `any case`、`any case of never`、`any case of Actor`。分類のケースを受け取る

interface ThrowsClause {
  exceptions: string[];           // FQN。ユニオンは展開済み。デコレータの引数名なら "$param:名前"
  relations: Relation[];
}
interface Relation { kind: "prevents" | "keeps"; }   // 目的語は常にその op のすべての効果（`effects`）

interface CatchesClause {
  exceptions: string[];           // 同上
  mode: "recovers" | "halts" | "ignores";
  halts?: string;                 // mode が halts のときの効果。FQN か "$param:名前"
}
```

### op

```ts
interface Op extends DeclBase {
  kind: "op";
  exported: boolean;              // `export` が付いているか（docs/check.md「export」）
  params: OpParam[];
  returns: Type | null;           // null は void。決して返さない op は { kind: "never" }
  effects: EffectRef[];           // 節に書かれた effects（書かれた順。ユニオンは展開済み）。書いていなければ []。
                                  // 書けるのは配線の op だけ（葉は STATE-5）。本体が行う効果は、body の呼び出しと signature にある。
                                  // デコレータが運ぶものは含まない
  reads: ReadRef[];               // 節に書かれた reads（書かれた順、重複は最初の出現に寄せる）。書いていなければ []。同上
  carried: {                      // デコレータの reads / effects の節が、この op に運ぶもの（docs/check.md「デコレータが運ぶ reads と effects」）。
    reads: ReadRef[];             // decorators の順、節の順、引数に書いた順。重複は最初の出現に寄せる。なければ []
    effects: EffectRef[];         // ユニオンは展開済み。同じ順。なければ []
  };                              // 葉の op が状態に触れるのは、この分だけ（docs/check.md「葉の op」）。配線の op にも運ばれ、どちらも signature に入る
  throws: ThrowsClause[];         // 書かれた throws
  body: Wiring | null;            // null は葉
  signature: Signature;           // 計算結果。自分の名前空間から見た関数の型（docs/check.md「計算」）
}

interface OpParam { name: string; type: Type; span: Span; }   // op、exception、effect、read の引数。説明は、その宣言の description.params から引く

// reads と effects の要素。名前で書いたか、直に書いたかを要素ごとに持つ（docs/check.md「節に書く要素」）
type ReadRef =
  | { kind: "named"; read: string }                   // read の宣言の FQN。読む状態はその宣言の state
  | { kind: "direct"; state: string };                // 直に書いた基底の状態の FQN
type EffectRef =
  | { kind: "named"; effect: string }                 // effect の宣言の FQN
  | { kind: "direct"; state: string; operation: "post" | "put" | "delete" };   // 直に書いた（節）か、直に呼んだ（配線）、
                                                                               // 基底の状態の組み込みの効果。同じ宛先の宣言とは別の効果（同上）

interface Signature {                   // docs/check.md「計算」の Sig(O, v) の結果。Op.signature は v = op の名前空間
  exceptions: string[];           // v から見て投げうる例外
  effects: EffectRef[];           // 伝播を含むすべての効果（配線が呼んだものと、デコレータが運ぶもの）と、デコレータの halts の効果
  relations: { exception: string; effect: EffectRef; state: "prevented" | "kept" | "unknown" }[];
                                  // exceptions × effects のすべての組
  successEffects: { effect: EffectRef; state: "kept" | "prevented" | "unknown" }[];   // 成功のアームでの効果の状態
  reads: string[];                // 読む基底の状態の FQN。伝播と、デコレータが運ぶものを含む（docs/check.md「reads」の R）。v によらない。
                                  // read の宣言は、その state として入る（どの read を呼んだかは、配線の body の Call.callee にある）
  pure: boolean;                  // 純粋か（docs/check.md「純粋」）。v によらない
}
```

### 配線

```ts
interface Wiring { statements: Statement[]; }

type Statement =
  | { kind: "val"; name: string; type?: Type; expr?: Expr; collected: boolean; span: Span }
                                  // 値の入れ物の宣言（docs/check.md「値の入れ物」）。名前を並べた宣言（`val a, b;`）は、名前ごとの文に分ける。
                                  // type は、解決した型（書いた型か、入れる式から導いた型。docs/types.md「val の型」）。プラグインは、型を導かない。
                                  // 型が決まらない val（`val x = none;`）だけ、持たない。
                                  // expr は、値と一緒に宣言したときだけ。collected は、集める文で値が入る val か（expr があれば false）
  | { kind: "assign"; name: string; expr: Expr; span: Span }      // 値を入れる文 `x = 式;`
  | { kind: "collect"; name: string; expr: Expr; span: Span }     // 集める文 `xs collect 式;`（docs/check.md「繰り返し」の「集める」）
  | { kind: "return"; expr: Expr; span: Span }
  | { kind: "call"; call: Call; span: Span }
  | { kind: "match"; match: Match; span: Span }
  | { kind: "disjoint"; arms: Arm[]; span: Span }                 // 互いに触れないアームの並び（docs/check.md「disjoint」）。書かれた順。二つ以上
  | { kind: "for"; var: string; varType: Type; of: Atom; body: Statement[]; disjoint: boolean; span: Span }
                                  // 繰り返し（docs/check.md「繰り返し」）。var は変数の名前、varType はその型（並びの要素の型）、of は並び。
                                  // disjoint は、前に `disjoint` を書いたか（回どうしが、互いに触れない）
  | { kind: "try"; body: Statement[]; catch: Catch; span: Span }  // 例外を受ける文（docs/check.md「例外を受ける」）。body は try のブロック
  | { kind: "throw"; throw: Throw; span: Span };                  // catch のブロックの、最後の文の `throw`

interface Catch {
  name: string;                   // 受けた例外の名前（`catch (e)` の e）
  body: Statement[];              // catch のブロック。`catch (e by C) { アーム … }` は、`match (e by C) { アーム … }` の文一つにして入れる
  never: boolean;                 // ブロックが「返らない」か（最後の文が throw か、「返らない」式の文。docs/check.md「例外を受ける」）
  span: Span;
}

interface Throw {                 // `throw E`、`throw E(値, …)`、`throw e`。アームの本体（MatchArm.body）と、同じ形
  kind: "throw";
  exception: string;              // 例外の FQN。`throw e`（受けた例外を、そのまま投げ直す）なら ""
  args: { param: string; value: Atom }[];   // 位置で書いた値を、例外の params の名前に対応付けたもの（なければ []）
  caught?: string;                // `throw e` のとき、受けた例外の名前（Catch.name）。ほかは、持たない
  span: Span;
}

interface Arm {                   // disjoint のアーム。ブロックでも、文の一つでも、文の並びにする（文の一つなら、要素が一つ）
  block: boolean;                 // ブロック `{ … }` で書いたか（中の文が一つでも true。出力の形に効く: docs/plugin-go.md「文」）
  statements: Statement[];
  span: Span;
}

type Expr = { kind: "call"; call: Call } | { kind: "match"; match: Match } | { kind: "atom"; atom: Atom };

interface Call {
  callee: Callee;                 // 呼び出し先（docs/check.md「配線からの呼び出し」）
  args: { param: string; value: Atom }[];   // 位置で書いた引数を、呼び出し先の引数名に対応付けたもの（docs/types.md「呼び出しの引数」）。
                                  // 直に呼ぶ組み込みの効果の、値を取る操作の引数名は "value"
  bound: BoundPosition[];         // bind で埋まる位置（docs/check.md「bind」）。callee が op でなければ []
  span: Span;
}

type Callee =
  | { kind: "op"; op: string }                        // op の FQN
  | { kind: "read"; read: string }                    // read の宣言の FQN。名前を付けない read は、呼び出しではなく Atom の state
  | { kind: "effect"; effect: EffectRef }             // named: effect の宣言の呼び出し。direct: 組み込みの効果を直に呼んだもの（`users.post(user)`）
  | { kind: "proceed" };                              // デコレータの本体の `proceed()`: 包んだ op の中身。現れるのは Decorator.body の中だけ。args は []

interface BoundPosition {
  path: string[];                 // ["in", "token"] のように、引数名から始まるフィールドのパス
  state: string;                  // 埋める状態の FQN
}

type Atom =
  | { kind: "name"; path: string[]; span: Span }      // ["query", "page"]。先頭は val の名前か、for の変数か、引数名
  | { kind: "state"; state: string; path: string[]; span: Span }   // 状態の参照（docs/check.md「状態の参照」）。state は FQN、
                                                                    // path はその値のフィールドのパス（状態そのものなら []）
  | { kind: "literal"; literal: Literal; span: Span }
  | { kind: "none"; span: Span }
  | { kind: "record"; fields: { name: string; value: Atom }[]; span: Span };   // { item } は { item: item } に展開済み

interface Match {
  subject: { kind: "value"; atom: Atom }            // 値を対象にした match（値の分類、op の分類、標準の分類）。
                                                    // 状態を名前で書いた対象（match token by nullable）は、atom が state
         | { kind: "classify" }                     // 状態の分類での match（対象は classify が持つ）
         | { kind: "caught"; name: string };        // 受けた例外の match（`match (e by C)`、`catch (e by C)`）。name は Catch.name。
                                                    // classify は、例外の分類。どのアームも、narrowed は []
  classify: string;               // FQN。標準の分類なら "list" か "nullable"
  arms: MatchArm[];
  span: Span;
}

interface MatchArm {
  cases: string[];                // このアームが受けるケース（docs/check.md「アームが受けるケース」）。名前で並べたアームなら、書かれた順。
                                  // else のアームなら、ほかのアームが並べなかったケースを、分類の宣言の順に（標準の分類は、list が Empty、HasItems、nullable が None、Some の順）
  else: boolean;                  // `else => …` と書いたアームか。プラグインは、cases だけを読めば足りる
  body: { kind: "call"; call: Call }
      | { kind: "atom"; atom: Atom }                // 値（docs/grammar.md「配線」の ArmBody の Atom）。none はここに入れず、下の none にする
      | { kind: "none" }
      | Throw;                                      // `throw E`、`throw E(値, …)`、`throw e`（上の Throw）
  narrowed: Narrowing[];          // このアームの中で、外と型が違う対象（docs/types.md「アームの中の対象の型」）。なければ []
  span: Span;
}

interface Narrowing {
  state?: string;                 // 状態の分類での match（subject が classify）のとき、絞り込まれる状態の FQN。
                                  // subject が value なら持たない（絞り込まれるのは subject の atom）
  type: Type;                     // アームの中での型
}

type Literal =
  | { kind: "string"; value: string }
  | { kind: "int"; value: number }
  | { kind: "float"; value: number }
  | { kind: "boolean"; value: boolean }
  | { kind: "duration"; value: number };    // ナノ秒。Go の time.Duration と同じ単位
```

### デコレータの適用

```ts
interface Application {
  decorator: string;              // 汎用デコレータの FQN。名前付きなら展開済み
  via?: string;                   // 名前付きデコレータを通して付けたなら、その FQN
  args: Arg[];                    // 引数名に対応付け済み。省略された省略可能な引数は含まない。
                                  // 残りの引数（Param.rest）は、いつも一つの Arg で、value は list（渡した値を書いた順に。0 個なら items が []）
  span: Span;
}

interface Arg {
  param: string;
  value: ArgValue;
}

type ArgValue =
  | { kind: "literal"; literal: Literal }
  | { kind: "ref"; target: string }                 // 宣言の FQN、または op の引数名（"$param:名前"）
  | { kind: "refs"; targets: string[] }             // `A | B`。ユニオンは展開済み
  | { kind: "variant"; union: string; arm: string } // `ProductPage.Ready`
  | { kind: "case"; classify: string; case: string } // 分類のケース `Actor.Customer`。classify は FQN
  | { kind: "list"; items: ArgValue[] };
```

`DeclBase.decorators` の並びは書かれた順（上から下）。`repeatable` なデコレータは、同じ `decorator` の `Application` が、付けた数だけ、書かれた順に並ぶ。§8.2「適用順は op に近いものから」なので、**最後の要素が最も内側で、最初に適用される**。入れ子の構造にはせず、この順序付きの列で表す（`docs/decisions.md`）。

### bind

```ts
interface Bind {
  namespace: string;              // bind を書いた名前空間の FQN
  state: string;                  // FQN
  target: { namespace: string; deep: boolean };   // deep は `**`
  span: Span;
}
```

## 正規化

表面の構文から IR を作るときに行うこと。一つの意味は一つの形になる。

| 表面 | IR |
|---|---|
| `namespace a.b { }` と `namespace a { namespace b { } }`、複数回の `namespace a { }` | `name: "a.b"` の一つの `Namespace`。`decls` は出現順に連結 |
| 修飾なしの参照 | FQN |
| 戻り値の型の省略 | `returns: null` |
| `T??` | `T?`（`optional` は入れ子にしない） |
| `list<T>` / `nullable<T>` / `single<T>`（どの型の位置でも） | `array(T)` / `optional(T)` / T（`docs/types.md`「形の書き方」）。`nullable<T?>` と `single<T?>` は `optional(T)` |
| `state s: T;`（形の語でも、`?` / `[]` でも） | `type` に正規化した型を置き、`shape` と `element` をそこから決める |
| `model D extends B { override a: never; override b: T; c: U; }` | `fields` に、`B` の `fields` を写して `Override` を当て、足したフィールドを末尾に足したもの（`docs/types.md`「継承した model」の「フィールド」）。`extends` に `base`、`removed`、`overridden` を記録。受け継いだフィールドのデコレータ・説明も写す |
| `export fn f()` | `exported: true` |
| `reads a, b`、複数の `reads` | `ReadRef` の列一つ（重複は最初の出現に寄せる）。`read` の宣言の名前は `named`、状態の名前は `direct` |
| ケースの `X as T` | `as` の `CaseTyping`。`X` が状態なら `state` にその FQN、型か op なら `state` を持たない |
| ケースの `X as else`（残りのケース） | `as` の `CaseTyping`。`by` は `else`、`type` は、そのケースでの対象の型（`docs/types.md`「残りのケースでの対象の型」）。`state` は上の行と同じ。そのケースは、ほかのケースと同じに `cases` に入る |
| 配線の値の位置の、状態の名前（`f(token)`、`token.value`、`match (token by nullable)`） | `Atom` の `state`。修飾して書いても同じ |
| `val a, b: T;`（名前を並べた、空の宣言） | 名前ごとの `val` の文（書かれた順）。`span` は、それぞれの名前（と、書いた型） |
| `disjoint` の、文の一つのアーム（`a = f(x);`） | `statements` が一つで、`block` が `false` の `Arm`。ブロックで書いた、文が一つのアーム（`{ a = f(x); }`）は、`block` が `true` |
| ケースの `X as none`（ない側のケース） | `as` の `CaseTyping`。`by` は `none`、`type` は対象の型のまま。`Classify.absent` に、そのケースの名前。ない側のケースを `X as else` で書いた分類（`docs/check.md`「ない側のケース」の表の二行目）も、`absent` を持つ（`by` は `else` のまま） |
| `effect fn e(o: T on C.A \| C.B): effects s.put as C.D` | `row` に `{ param: "o", classify: C の FQN, to: "D" }`。前のケースは、`params` の `o` の型（`case(C, ["A", "B"], T)`）にある |
| `effect fn e(o: T): effects s.post as C.X` | `row` に `{ param: "o", classify: C の FQN, to: "X" }` |
| `effect fn e(o: T): effects s.put as else` | `row` に `{ param: "o" }`（`classify` も `to` も持たない） |
| `effect fn e(o: T): effects s.delete as C.N` | `row` に `{ param: "o", classify: C の FQN, to: "N" }` |
| アームの `A, B => …` | `cases: ["A", "B"]`（書かれた順）、`else: false` |
| アームの `else => …` | `cases` に、ほかのアームが並べなかったケースを宣言の順で、`else: true`。受けるケースがなければ（MATCH-9）`cases: []` |
| アームの本体の `none` | `body` の `none`（`atom` にはしない） |
| ユニオンのアームのその場の model `Ready: { ... }` | `<ユニオンの FQN>.Ready` という FQN の `Model`（`synthesizedFrom` にユニオンの FQN）を同じ名前空間に足し、アームの型を `ref` にする。宣言の位置はユニオンの直後 |
| `exception X;` | `params: []` |
| 配線の呼び出し `f(a)`、`r(a)`、`e(a)`、`s.post(a)` | `Call`。`callee` は、行き着いた宣言の種類で決まる（`docs/resolve.md`「呼び出しの読み分け」）: `op`、`read`、`effect` の `named`、`effect` の `direct` |
| `throw E(a, b)` | `body` の `throw`。`args` は例外の `params` に対応付け |
| `Unavailable: Unavailable` | そのまま `ref`（新しい model は作らない） |
| ユニオンのメンバーにユニオン | メンバーを展開して平らにする |
| `throws A, B` にユニオン | 例外の FQN に展開 |
| `effects a, b`、複数の `effects` | `EffectRef` の列一つ（書かれた順に連結）。`effect` の宣言の名前は `named`（ユニオンは展開）、`状態.操作` は `direct` |
| `@redirectToLogin`（名前付き） | `decorator: "browser.redirectOn"`、`via: "browser.redirectToLogin"`、`args` は固定した引数 |
| デコレータの位置で書いた引数 | `param` に対応付け |
| 残りの引数に渡した値（`decorator effects(...targets: any effect)` に `@effects(a, b)`） | 残りの引数の名前の `Arg` 一つ。`value` は `list` で、`items` は渡した値を書いた順に（一つずつ `ref` か `refs`） |
| デコレータの宣言の `reads a, b`、`effects a, b`、複数の節 | `Decorator.reads` / `Decorator.effects` の列一つずつ（書かれた順に連結。効果のユニオンは展開。引数名は `$param:名前`） |
| op に付いたデコレータの `reads` / `effects` の節 | `Op.carried`（`$param:` を適用の引数で置き換えたもの）。`Op.reads` / `Op.effects` には足さない |
| `{ item }` | `{ item: item }` |
| 呼び出しと `throw` の引数（位置だけ。`docs/grammar.md`「配線」） | 順に、呼び出し先の引数の名前（`param`）に対応付け |
| 複数行の `throws` | `ThrowsClause` の列（まとめない。`Relation` が節ごとに違うため） |
| `5m` | `duration` のナノ秒 |

正規化しないもの:

- op の引数の並び（§7.1「引数の並び（名前と型の組）で、IR もそのまま持つ」）。
- デコレータの `throws` と `catches` の節の順序。差し引きは `Signature` に計算結果として置く。

## その名前空間から見た IR

トランスパイラに渡す IR（§4.6「その名前空間から見た IR」）は、`Program` 全体に「見る側」の情報を足したもの。単位（`docs/config.md`「単位」）と、その単位が出力する名前空間の組ごとに一つ作る。

```ts
interface View {
  version: "0.20";
  viewer: string;                 // 見る側の名前空間の FQN
  config: NamespaceConfig;        // この View の単位での viewer の設定
  files: string[];                // この View の単位の in に合ったユーザーのファイルと、読み込んだすべての語彙のファイル
                                  // （docs/config.md「in」）。Span.file と同じ形で、ユーザーのファイルを辞書順に、その後に語彙のファイルを辞書順に。
                                  // in を省いた単位なら、読み込んだすべてのファイル。
                                  // viewer の宣言のうち span.file がここにあるものが、単位の対象の宣言
  namespaces: Record<string, NamespaceConfig[]>; // 出力のあるすべての名前空間（viewer を含む）。キーは FQN、順序は辞書順。
                                                 // 値は、その名前空間を出力する単位ごとの設定（設定の順）。
                                                 // トランスパイラが別の名前空間（と別の単位）の言語と出力先を知るため
  program: Program;               // 全体そのまま（各 op の signature は op 自身の名前空間から見たもの）。
                                  // ただし下の params が合成する model を、viewer の Namespace.decls の末尾に足してある
  ops: Record<string, ViewedSignature>;   // FQN → viewer から見た関数の型。すべての op について
}

interface NamespaceConfig {       // 単位 U での名前空間 N の設定
  index: number;                  // U が何番目の [[emit]] か（1 始まり）。診断のメッセージと、単位どうしを見分けるのに使う
  language: string;               // U の言語（docs/config.md「基底と相乗り」）
  base: string;                   // U の基底のトランスパイラの名前（同上）
  out: string;                    // U の out
  dir: string;                    // Dir(U, N)。U での N のファイルを置くディレクトリ（ルートからの相対パス。docs/config.md「単位の対象」）
  transpilers: string[];          // U の transpilers（書かれた順）
  options: Record<string, Record<string, unknown>>;   // U の options（docs/config.md「オプション」）。キーはトランスパイラの名前、
                                  // 値は hozo.toml のテーブルを JSON に写したもの（整数と小数は number）。オプションを書いていないトランスパイラのキーはない。
                                  // 順序は、キーの辞書順（入れ子のテーブルも）
  seams: Record<string, string>;  // U の継ぎ目（docs/config.md「継ぎ目」）。キーは、継ぎ目を持つデコレータの FQN、値は、それをマニフェストの seams で
                                  // 宣言した U のトランスパイラの名前（一つに決まる。CFG-23）。順序は、キーの辞書順。なければ {}
}

interface ViewedSignature extends Signature {   // Signature は Sig(O, viewer)
  params: OpParam[];              // bind で埋まる位置（docs/check.md「bind」の Pos(b, O)）を取り除いた引数。
                                  // 位置が model のフィールドなら、そのフィールドを除いた model を
                                  // `<viewer>.<op 名>$<引数名>` の FQN（synthesizedFrom は op の FQN）で合成し、ref にする。
                                  // 位置が入れ子の model の中なら、パスのセグメントを `$` でつないだ名前（`browser.deep$in$inner`）で、親を先に合成する
  bound: BoundPosition[];         // 取り除いた位置
}
```

- `Program` 全体を渡すのは、トランスパイラが参照先（別の名前空間の型、語彙の宣言、`in` の外の宣言）を自分で探さなくて済むようにするため。何を出力するかは、基底が `View` だけから決める（Go なら `docs/plugin-go.md`「出力の対象」）。`View` のうち、相乗りが基底の形を知るのに使う項目は `docs/plugin-protocol.md`「相乗りが基底の形を知る方法」。
- `config.seams` は、仕様（`Program`）からではなく、単位のトランスパイラのマニフェストから作る。同じ名前空間でも、単位ごとに違いうる（`namespaces[N]` の要素ごとに、その単位のものを持つ）。
- `files` は、トランスパイラが単位の対象の宣言を見分けるための最小の情報。宣言ごとの印を `Program` に足さないのは、`Program` を単位によらず同じにするため。合成した model（`<viewer>.<op 名>$<引数名>`）の `span` は元の引数のもので、op と同じファイルになる。
- `namespaces` に入るのは、どれかの単位が出力する名前空間（`docs/config.md`「単位の対象」「in」）。同じ名前空間を複数の単位が出力していれば、値は複数の要素を持つ。`namespaces[viewer]` のうち `index` が `config.index` と同じ要素が、この View の単位のもの。別の名前空間の型の置き場（`docs/config.md`「型の置き場」）は、`namespaces[M]` の要素のうち、`index` が `config.index` と同じもの、なければ `base` が `config.base` と同じ最初のもの。
- `ViewedSignature.params` と、それが合成する model（`<viewer>.<op 名>$<引数名>`）は、`bind` で埋まる位置を呼ぶ側の引数から消した形が要るトランスパイラ（別のプロセスの op を呼ぶ口を作るもの）のためのもの。Go の基底は使わない（`docs/plugin-go.md`「出力の対象」）。
- `ops` の `Signature` は `docs/check.md`「計算」の Sig(O, viewer)。viewer が op の名前空間と同じなら `Program` の `signature` と一致する。`reads` と `pure` は viewer によらず、`Program` の `signature` と同じ値。
- `ops` は、`export` の有無によらずすべての op を持つ。viewer が呼べる op かどうかは `Op.exported` と名前空間から分かる（`docs/check.md`「export」）。
- `params` に反映する `bind` は `viewer` の名前空間の `Bind` だけ（`bind` は呼び出す側の宣言。§5.3）。
- `viewer` が同じなら、`config`・`files` 以外は単位によらず同じ。

## JSON

- フィールド名はこの文書の通り（lowerCamelCase）。`kind` が判別子。Go の `ir` パッケージの JSON タグはここから写す。
- 省略可能（`?`）のフィールドは、値がなければキーごと省く。空の列は `[]` で出す（省かない）。`null` を出すのは `Op.returns`（void）と `Op.body`（葉）の二つだけ。
- 数値は JSON の number。`duration` は整数（ナノ秒）。`int` は 64 ビットの範囲に収まることを検査 LIT-1（`docs/types.md`「診断」）で保証する。
- 列の順序はこの文書の「原則」の通りで、同じ入力から同じバイト列になる。`ops` のキーの順序は FQN の辞書順。
- 版の照合は `docs/plugin-protocol.md`「要求と応答」。

## 版

`Program.version` と `View.version` は同じ値で、どちらかの構造か、プラグインとのやり取りの形（`docs/plugin-protocol.md`「要求と応答」）が変わったら上げる。IR が決めている名前のうち、プラグインが、その値で出力を分けるもの（核の型の名前、標準の分類のケースの名前）が変わったときも、上げる。今の版は `"0.20"`。構造もやり取りの形も同じで、値の意味だけが変わる決定では、上げない: 配線の op の `Op.carried` が空でないことがあり、その `Signature` が運ばれたものを含むこと（`docs/decisions.md`「決めたこと」149）は、0.6 の中にある。

| 版 | 変わったこと |
|---|---|
| 0.20 | 配線の文（`Statement`）に `try`（`Catch`）と `throw` を足した。`Match.subject` に `caught`。アームの本体の `throw` に `caught`（`throw e`）。`Classify.subject` に `exception`、`Classify.cases[]` に `exceptions` と `rest`（例外の分類）（`docs/check.md`「例外を受ける」「例外の分類」。`docs/decisions.md`「決めたこと」419、422）。`View` とやり取りの形は 0.19 のまま |
| 0.19 | `Decorator.external` をなくし、`Decorator.outerThrows`（節 `outer throws`）と、`Decorator.entry`、`Decorator.hint`（核の印 `@entry`、`@hint`）を足した（`docs/grammar.md`「decorator の宣言」。`docs/decisions.md`「決めたこと」407、408、420） |
| 0.18 | `Decorator.fn` と `Decorator.body` を足した（関数に付けるデコレータ `decorator fn` と、その本体。`docs/grammar.md`「decorator の宣言」「デコレータの本体」、`docs/decisions.md`「決めたこと」392〜394）。`Callee` に `proceed` を足した（デコレータの本体の `proceed()`）。`View` とやり取りの形は 0.17 と同じ |
| 0.17 | やり取りに、direction `coverage` を足した（`docs/plugin-protocol.md`「coverage」、`docs/decisions.md`「決めたこと」380）。マニフェストの `[coverage]`。`emit` の応答の `files` のうち、名前が `hozo-tests.<トランスパイラの名前>.json` のものを、テストの対応表として読み、形を確かめる（同「テストの対応表」、PLG-4）。`Program` と `View` の構造、`emit` / `check` / `import` の要求と応答の形は、0.16 と同じ |
| 0.16 | `RowTransition.from` をなくした: 効果の前のケースは、行の引数の型（`case`）が言う（`docs/check.md`「行の遷移」、`docs/decisions.md`「決めたこと」369）。値の分類と op の分類の `match` のアームの `narrowed` の型は、いつも `case`（型を書いたケースのアームと、`else` のアームでも。`docs/types.md`「アームの中の対象の型」、同 370）。`View` とやり取りの形は 0.15 と同じ |
| 0.15 | 標準の分類 `nullable` のケースの名前が、`Empty` / `HasItems` から、`None` / `Some` になった（`docs/check.md`「分類」の「標準の分類」、`docs/decisions.md`「決めたこと」362）。変わるのは、`Match.classify` が `"nullable"` の `MatchArm.cases` と、形が `nullable` の状態を宛先にする `TransitionTarget` の `from` / `to` に載る、名前の値。構造は、0.14 と同じ。値だけの変更で版を上げるのは、基底が、この名前でアームを選ぶため: 前の版の名前を待つプラグインは、アームを見つけられない |
| 0.14 | 型の種類 `record` をなくし、`Model.index`（インデックスの model: キーの名前、キーの型、値の型）を足した。`Record<K, V>` は、インデックスの model を宣言して、その名前（`ref`）で書く（`docs/grammar.md`「model」、`docs/types.md`「インデックスの model」） |
| 0.13 | 核の型の名前（`Type` の `core` の `name`）: `Time` を `datetime` に、`Duration` を `duration` に替え、`date` と `time` を足した（`docs/grammar.md`「型」）。デコレータの宣言の、付ける先の並びの区切りが `,` になったこと（`docs/grammar.md`「decorator の宣言」）は、綴りだけで、`Decorator.targets` の形は、変わらない |
| 0.12 | `Effect.throws`（効果が投げうる例外。`docs/check.md`「read と効果」）。効果の呼び出しと、運ばれた効果が、`Signature.exceptions` に例外を足しうる。`View` とやり取りの形は 0.11 と同じ |
| 0.11 | `CaseKind.subject` が、文字列の `"never"` から、`Type` になった（対象の型で絞る `case of <型>`。`docs/grammar.md`「decorator の宣言」）。`of never` は、kind が `never` の型。`View` とやり取りの形は 0.10 と同じ |
| 0.10 | 配線の文（`Statement`）: `const` をやめて `val`（`type`、`collected`。`expr` は省ける）。`assign`、`collect`、`disjoint`（`Arm`）、`for`。`Classify.cases[].decorators`。`Classify.decidedBy` に `none`（対象のない分類。`subject` の `type` が `never`）。`Decorator.targets` に `case` と、`Decorator.caseTargets`（`CaseKind`）。`ParamType` の `any` の `of` に `case` と、`case`（`CaseKind`）。`ArgValue` に `case`。`View` とやり取りの形は 0.9 と同じ |
| 0.9 | `Effect.row`（`RowTransition`: 行の遷移。`docs/check.md`「行の遷移」）。`CaseTyping.by` に `none`（ない側のケース `対象 as none`。同「ない側のケース」）。`Classify.absent`。述語で決まる分類（`decidedBy` が `predicate`）が、述語を持たないケース（`absent` のケース）を持ちうる。`View` とやり取りの形は 0.8 と同じ |
| 0.8 | `Classify.hasDefault`、`Classify.defaultDescription`、`Classify.fromCondition` をなくした（分類を閉じた。`docs/check.md`「閉じた分類」）。`MatchArm.cases` は、いつもケース名の列（`"default"` をなくした）。`MatchArm.else`（`else` のアーム。`cases` は、それが受けるケース）。`View` とやり取りの形は 0.7 と同じ |
| 0.7 | `CaseTyping.by` に `else`（残りのケース `対象 as else`。`docs/check.md`「残りのケース」）。そのときの `CaseTyping.type` は、書いた型ではなく、そのケースでの対象の型（`docs/types.md`「残りのケースでの対象の型」）。残りのケースは `Classify.cases` に入り、`hasDefault` には数えない。`View` とやり取りの形は 0.6 と同じ |
| 0.6 | 宣言の種類 `read`（`Read`: `params`、`returns`、`state`）。`Exception.value` をやめて `Exception.params`（例外のその場の model `<例外の FQN>.Value` は、もう合成しない）。`Effect.params`。`Call.op` をやめて `Call.callee`（`op` / `read` / `effect`）。`Match.arms[].body` の `throw` に `args`。`Decorator.targets` に `read`。`Decorator.repeatable`（同じデコレータの `Application` が、一つの宣言に複数並びうる）。`reads` と `effects` の要素が、文字列（FQN）から `ReadRef` / `EffectRef` に変わった: `Op.reads`、`Op.effects`、`Op.carried`、`Decorator.reads`、`Decorator.effects`、`Signature.effects`、`Signature.relations[].effect`、`Signature.successEffects[].effect`。`Signature.reads` は、基底の状態の FQN の列のまま。葉の op の `Op.reads` / `Op.effects` は、いつも空になった（STATE-5）。`View` とやり取りの形は 0.5 と同じ |
| 0.5 | `Decorator.targets` に `effect`（効果の宣言の `decorators` が空でなくなりうる）。`Decorator.reads`、`Decorator.effects`。`Param.rest` と、残りの引数の `Arg`（`list`）。`Op.carried`。`Signature.reads` / `Signature.effects` が、デコレータが運ぶものを含む。`Field.default` をなくした（フィールドの既定値を、言語からなくした。`docs/grammar.md`「決めたこと」）。`View` とやり取りの形は 0.4 と同じ |
| 0.4 | `NamespaceConfig` に `index`、`base`、`options`、`seams`。`View.files` に語彙のファイル。`Program` の構造は 0.3 と同じ。`emit` の応答から `select` をなくした（`docs/plugin-protocol.md`「選択をなくしたこと」） |
| 0.3 | `Type` に `never`。`Model.derivedFrom` をやめて `Model.extends`。`State.type`。`Classify.subject` に `op`、`cases[].as`、`decidedBy`。`Op.exported`、`Op.reads`。`Signature.reads`、`Signature.pure`。`Atom` に `state`。`Match.subject` の `state` をやめる（`value` の `atom` が `state`）。`Match.arms[].body` に `atom` と `throw`、`Match.arms[].narrowed` |
