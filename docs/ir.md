# IR

## この文書が決めること

IR（中間表現）の構造と、表面の構文から IR への正規化、JSON での形、「その名前空間から見た IR」を決める。設計書 §2「IR（中間表現）を正とする。表面の構文は糖衣を許すが、IR は一つの形に正規化する」に従う。Go の型は `ir` パッケージがこの文書を写したもので、JSON スキーマ（`schema/ir.schema.json`）と TS の型（`sdk/ts/`）はそこから生成する（`docs/requirements.md`「プラグインの書き方」）。

構文の形は `docs/grammar.md`、名前の解決は `docs/resolve.md`、型の互換は `docs/types.md`、IR の上で計算する集合（例外・効果）は `docs/check.md`「計算」が持ち主。この文書はそれらの結果を IR のどこに置くかを決める。

## 原則

- **参照はすべて完全修飾名（FQN）**。`server.User` のように、ルートからの名前空間のパスと宣言名を `.` でつないだ文字列。グローバルの宣言は宣言名だけ（`db`）。表面の構文の「同じ名前空間なら修飾しない」は IR に残さない。
- **宣言の順序は出現順**。ファイルの順は、ユーザーのファイルをパスの辞書順に並べ、その後に語彙のファイル（`Span.file` が `hozo:` のもの）を同じく辞書順に並べたもの。ファイルの中は位置順。同じ名前空間が複数の場所にあれば、この順で並べて一つにする（§3.2）。出力の決定性（`docs/requirements.md`「品質の要件」）のため、IR のどの列も集合ではなく順序付きの列。診断の並び（`docs/diagnostics.md`「順序」）もこのファイルの順に従う。
- **すべての宣言は位置（`span`）を持つ**。言語サーバーと診断のため。JSON にも出す。
- **計算した結果も IR に持つ**。例外の集合・効果の集合・`bind` で埋まる位置は、プラグインが自分で計算しなくて済むように、`docs/check.md` の計算結果を IR に置く。

## 構造

TypeScript 風の記法で書く。`?` は省略可（JSON での扱いは「JSON」）。

### Program

```ts
interface Program {
  version: "0.1";                 // この文書の版。構造が変わったら上げる
  namespaces: Namespace[];        // グローバルは name "" の Namespace として先頭に置く
  binds: Bind[];
}

interface Namespace {
  name: string;                   // FQN。"server.admin"。グローバルは ""
  description?: Doc;              // namespace ブロックの直前の DocComment。複数あれば最初のもの
  decls: Decl[];                  // 出現順
}

type Decl = Model | Scalar | Union | State | Exception | Effect | Classify | Decorator | Op;

interface DeclBase {
  kind: "model" | "scalar" | "union" | "state" | "exception" | "effect" | "classify" | "decorator" | "op";
  name: string;                   // FQN
  description?: Doc;
  decorators: Application[];      // 付いたデコレータ。順序の意味は「デコレータの適用」
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
  | { kind: "core"; name: "string" | "boolean" | "int" | "float" | "Time" | "Duration" }
  | { kind: "ref"; target: string }                    // model / scalar / union の FQN
  | { kind: "optional"; of: Type }
  | { kind: "array"; of: Type }
  | { kind: "record"; key: Type; value: Type }
  | { kind: "case"; classify: string; cases: string[]; of: Type };   // 絞り込まれた型。cases は昇順ではなく match のアームに書かれた順
```

- `void` は型ではなく、`Op.returns` が `null` であること。
- その場で書いた model（`{ ... }`）は IR にはない（「正規化」）。

### model / scalar / union

```ts
interface Model extends DeclBase {
  kind: "model";
  fields: Field[];
  derivedFrom?: { base: string; omit: string[] };    // `is X omit ...` の記録。fields は展開済み
  synthesizedFrom?: string;                           // その場で書いた model の出どころ。"server.OrderPage.Ready" のように持ち主の FQN
}

interface Field {
  name: string;
  description?: Doc;
  type: Type;                     // 省略可能なら optional
  default?: Literal;
  decorators: Application[];
  span: Span;
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

```ts
interface State extends DeclBase {
  kind: "state";
  shape: "list" | "single";
  element: Type;
}

interface Exception extends DeclBase {
  kind: "exception";
  value?: Type;                   // 運ぶ値の型。ref か、synthesizedFrom を持つ model への ref
}

interface Effect extends DeclBase {
  kind: "effect";
  state: string;                  // FQN
  operation: "post" | "put" | "delete";
}
```

### classify

```ts
interface Classify extends DeclBase {
  kind: "classify";
  subject: Type;
  cases: { name: string; description?: Doc; span: Span }[];   // default は含まない
  hasDefault: boolean;
  defaultDescription?: Doc;       // default に付いた DocComment
  fromCondition: boolean;         // condition の糖衣から来たか（cases は一つ、hasDefault は true）
}
```

### decorator

```ts
interface Decorator extends DeclBase {
  kind: "decorator";
  params: Param[];
  targets: ("op" | "field" | "state" | "exception" | "scalar" | "model")[];   // 省略時は ["op"]
  external: boolean;              // @external が付いているか（DEC-10）
  throws: ThrowsClause[];
  catches: CatchesClause[];
  alias?: { target: string; args: Arg[] };   // 名前付きデコレータ。target は汎用デコレータの FQN。
                                             // alias を持つ宣言は params / targets / throws / catches を持たず、target のものを使う
}

interface Param {
  name: string;
  type: ParamType;
  optional: boolean;              // 説明は DeclBase.description.params から引く
}

type ParamType =
  | { kind: "type"; types: Type[] }                   // `int | float` は types が二つ。普通は一つ
  | { kind: "any"; of: "exception" | "effect" | "state" | "op" | "model" | "variant"; on?: { state: string; operation: "post" | "put" | "delete" } };

interface ThrowsClause {
  exceptions: string[];           // FQN。ユニオンは展開済み。デコレータの引数名なら "$param:名前"
  relations: Relation[];
}
interface Relation { kind: "prevents" | "keeps"; effects: "all" | string[]; }   // "all" は `effects`。string[] は効果の FQN

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
  params: OpParam[];
  returns: Type | null;           // null は void
  effects: string[];              // 書かれた effects（FQN）。配線で省略していれば []
  throws: ThrowsClause[];         // 書かれた throws
  body: Wiring | null;            // null は葉
  signature: Signature;           // 計算結果。自分の名前空間から見た関数の型（docs/check.md「計算」）
}

interface OpParam { name: string; type: Type; span: Span; }   // 説明は Op.description.params から引く

interface Signature {                   // docs/check.md「計算」の Sig(O, v) の結果。Op.signature は v = op の名前空間
  exceptions: string[];           // v から見て投げうる例外
  effects: string[];              // 伝播を含むすべての効果と、デコレータの halts の効果
  relations: { exception: string; effect: string; state: "prevented" | "kept" | "unknown" }[];
                                  // exceptions × effects のすべての組
  successEffects: { effect: string; state: "kept" | "prevented" | "unknown" }[];   // 成功のアームでの効果の状態
}
```

### 配線

```ts
interface Wiring { statements: Statement[]; }

type Statement =
  | { kind: "const"; name: string; expr: Expr; span: Span }
  | { kind: "return"; expr: Expr; span: Span }
  | { kind: "call"; call: Call; span: Span }
  | { kind: "match"; match: Match; span: Span };

type Expr = { kind: "call"; call: Call } | { kind: "match"; match: Match } | { kind: "atom"; atom: Atom };

interface Call {
  op: string;                     // FQN
  args: { param: string; value: Atom }[];   // 位置で書いた引数を引数名に対応付けたもの
  bound: BoundPosition[];         // bind で埋まる位置（docs/check.md「bind」）
  span: Span;
}

interface BoundPosition {
  path: string[];                 // ["in", "token"] のように、引数名から始まるフィールドのパス
  state: string;                  // 埋める状態の FQN
}

type Atom =
  | { kind: "name"; path: string[]; span: Span }      // ["query", "page"]。先頭は束縛名か引数名
  | { kind: "literal"; literal: Literal; span: Span }
  | { kind: "none"; span: Span }
  | { kind: "record"; fields: { name: string; value: Atom }[]; span: Span };   // { item } は { item: item } に展開済み

interface Match {
  subject: Atom;
  classify: string;               // FQN
  arms: { cases: string[] | "default"; body: { kind: "call"; call: Call } | { kind: "none" }; span: Span }[];
  span: Span;
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
  args: Arg[];                    // 引数名に対応付け済み。省略された省略可能な引数は含まない
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
  | { kind: "list"; items: ArgValue[] };
```

`DeclBase.decorators` の並びは書かれた順（上から下）。§8.2「適用順は op に近いものから」なので、**最後の要素が最も内側で、最初に適用される**。入れ子の構造にはせず、この順序付きの列で表す（`docs/decisions.md`）。

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
| `condition C of T;` | `Classify` で `cases: [C]`、`hasDefault: true`、`fromCondition: true` |
| 戻り値の型の省略 | `returns: null` |
| `T??` | `T?`（`optional` は入れ子にしない） |
| `model P is U omit a, b;` | `fields` に `U` のフィールドから `a`、`b` を除いたものを複製し、`derivedFrom` に記録 |
| ユニオンのアームのその場の model `Ready: { ... }` | `<ユニオンの FQN>.Ready` という FQN の `Model`（`synthesizedFrom` にユニオンの FQN）を同じ名前空間に足し、アームの型を `ref` にする。宣言の位置はユニオンの直後 |
| 例外のその場の model `exception X: { ... }` | `<例外の FQN>.Value` という `Model` を同じく足し、`value` を `ref` にする |
| `Unavailable: Unavailable` | そのまま `ref`（新しい model は作らない） |
| ユニオンのメンバーにユニオン | メンバーを展開して平らにする |
| `throws A \| B` にユニオン | 例外の FQN に展開 |
| `effects A \| B` | FQN の列 |
| `@redirectToLogin`（名前付き） | `decorator: "browser.redirectOn"`、`via: "browser.redirectToLogin"`、`args` は固定した引数 |
| デコレータの位置で書いた引数 | `param` に対応付け |
| `{ item }` | `{ item: item }` |
| 呼び出しの引数（位置だけ。`docs/grammar.md`「配線」） | 順に `param` に対応付け |
| 複数行の `throws` | `ThrowsClause` の列（まとめない。`Relation` が節ごとに違うため） |
| `5m` | `duration` のナノ秒 |

正規化しないもの:

- op の引数の並び（§7.1「引数の並び（名前と型の組）で、IR もそのまま持つ」）。
- デコレータの `throws` と `catches` の節の順序。差し引きは `Signature` に計算結果として置く。

## その名前空間から見た IR

プラグインに渡す IR（§4.6「その名前空間から見た IR」）は、`Program` 全体に「見る側」の情報を足したもの。

```ts
interface View {
  version: "0.1";
  viewer: string;                 // 見る側の名前空間の FQN
  config: { language: string; out: string; plugins: string[] };   // Conf(viewer)（docs/config.md「名前空間の設定の解決」）
  program: Program;               // 全体そのまま（各 op の signature は op 自身の名前空間から見たもの）。
                                  // ただし下の params が合成する model を、viewer の Namespace.decls の末尾に足してある
  ops: Record<string, ViewedSignature>;   // FQN → viewer から見た関数の型。すべての op について
}

interface ViewedSignature extends Signature {   // Signature は Sig(O, viewer)
  params: OpParam[];              // bind で埋まる位置（docs/check.md「bind」の Pos(b, O)）を取り除いた引数。
                                  // 位置が model のフィールドなら、そのフィールドを除いた model を
                                  // `<viewer>.<op 名>$<引数名>` の FQN（synthesizedFrom は op の FQN）で合成し、ref にする
  bound: BoundPosition[];         // 取り除いた位置
}
```

- `Program` 全体を渡すのは、プラグインが参照先（別の名前空間の型、語彙の宣言）を自分で探さなくて済むようにするため。何を出力の対象にするかはプラグインが `entry` / `stop` で決める（§4.3）。
- `ops` の `Signature` は `docs/check.md`「計算」の Sig(O, viewer)。viewer が op の名前空間と同じなら `Program` の `signature` と一致する。
- `params` に反映する `bind` は `viewer` の名前空間の `Bind` だけ（`bind` は呼び出す側の宣言。§5.3）。

## JSON

- フィールド名はこの文書の通り（lowerCamelCase）。`kind` が判別子。Go の `ir` パッケージの JSON タグはここから写す。
- 省略可能（`?`）のフィールドは、値がなければキーごと省く。`null` は出さない。空の列は `[]` で出す（省かない）。
- 数値は JSON の number。`duration` は整数（ナノ秒）。`int` は 64 ビットの範囲に収まることを検査 LIT-1（`docs/types.md`「診断」）で保証する。
- 列の順序はこの文書の「原則」の通りで、同じ入力から同じバイト列になる。`ops` のキーの順序は FQN の辞書順。
- 版の照合は `docs/plugin-protocol.md`「要求と応答」。
