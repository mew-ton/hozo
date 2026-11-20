/* eslint-disable */
// Code generated from schema/ir.schema.json by `npm run generate` (json-schema-to-typescript); DO NOT EDIT.
// IR の構造の持ち主は docs/ir.md、JSON の形は Go の ir パッケージ（docs/requirements.md「プラグインの書き方」）。

/**
 * Hozo の IR（docs/ir.md）の JSON の形。Program（hozoc の出力）か View（トランスパイラに渡すもの）。$defs にはプラグインのプロトコル（docs/plugin-protocol.md「要求と応答」）の要求と応答も含む。Go の ir / plugin パッケージから internal/tools/irschema が生成する。手で編集しない。
 */
export type HozoIR = Program | View;
/**
 * 宣言（docs/ir.md「Program」の Decl）。kind で分ける。
 */
export type Decl =
  Model | Scalar | Union | State | Exception | Effect | Read | Classify | Decorator | Op;
/**
 * デコレータの引数の値の種類（docs/ir.md「デコレータの適用」）。kind で分ける。
 */
export type ArgValue = LiteralValue | RefValue | RefsValue | VariantValue | ListValue;
/**
 * リテラル（docs/ir.md「配線」）。duration は整数のナノ秒。
 */
export type Literal = StringLiteral | IntLiteral | FloatLiteral | BooleanLiteral | DurationLiteral;
/**
 * 型（docs/ir.md「型」）。void は型ではなく Op.returns が null であること。never は Op.returns にだけ現れる。
 */
export type Type =
  CoreType | RefType | OptionalType | ArrayType | RecordType | CaseType | NeverType;
export type CoreName = "string" | "boolean" | "int" | "float" | "Time" | "Duration";
export type UnionVariant = "arms" | "set";
/**
 * 状態の形（docs/ir.md「state / exception / effect」の shape）。列数の範囲: list は 0 以上、nullable は 0 か 1、single はちょうど 1。状態の型の最も外側から決まる（docs/check.md「状態の形」）。
 */
export type StateShape = "list" | "nullable" | "single";
/**
 * 遷移の前か後のケースの集合。"any" か、ケース名の列（書かれた順。docs/ir.md「state / exception / effect」の CaseSet）。
 */
export type CaseSet = "any" | string[];
/**
 * 効果の宛先（docs/ir.md「state / exception / effect」の EffectTarget）。kind で分ける。
 */
export type EffectTarget = BuiltinTarget | TransitionTarget;
export type Operation = "post" | "put" | "delete";
/**
 * 絞り込みの種類（docs/ir.md「classify」の CaseTyping の by、docs/types.md「型で決まる絞り込み」）。else は残りのケース `対象 as else`、none はない側のケース `対象 as none`。
 */
export type NarrowBy = "presence" | "variant" | "predicate" | "else" | "none";
/**
 * 分類のケースが何で決まるか（docs/ir.md「classify」の decidedBy、docs/check.md「ケースの判定」）。type なら述語を持たない。
 */
export type DecidedBy = "type" | "predicate";
/**
 * 分類の対象（docs/ir.md「classify」の subject）。kind で分ける。
 */
export type ClassifySubject = TypeSubject | OpSubject | StatesSubject;
export type CatchMode = "recovers" | "halts" | "ignores";
export type EffectRef = NamedEffect | DirectEffect;
/**
 * デコレータの引数の型（docs/ir.md「decorator」の ParamType）。
 */
export type ParamType = TypeParam | AnyParam;
export type AnyOf = "exception" | "effect" | "state" | "op" | "model" | "variant";
export type ReadRef = NamedRead | DirectRead;
export type DecoratorTarget =
  "op" | "field" | "state" | "exception" | "scalar" | "model" | "effect" | "read";
export type RelationKind = "prevents" | "keeps";
/**
 * 配線の文（docs/ir.md「配線」）。kind で分ける。
 */
export type Statement = ConstStmt | ReturnStmt | CallStmt | MatchStmt;
/**
 * 式（docs/ir.md「配線」）。kind で分ける。
 */
export type Expr = CallExpr | MatchExpr | AtomExpr;
/**
 * 式の葉（docs/ir.md「配線」）。kind で分ける。
 */
export type Atom = NameAtom | StateAtom | LiteralAtom | NoneAtom | RecordAtom;
export type Callee = OpCallee | ReadCallee | EffectCallee;
/**
 * match のアームの本体（docs/ir.md「配線」）。call、atom、none、throw のどれか。
 */
export type ArmBody = CallExpr | AtomExpr | NoneBody | ThrowBody;
/**
 * match の対象（docs/ir.md「配線」の Match の subject）。kind で分ける。
 */
export type MatchSubject = ValueSubject | ClassifyOwnedSubject;
export type EffectState = "prevented" | "kept" | "unknown";

/**
 * IR 全体（docs/ir.md「Program」）。namespaces の先頭はグローバル（name ""）。
 */
export interface Program {
  binds: Bind[];
  namespaces: Namespace[];
  version: "0.10";
}
/**
 * bind 宣言（docs/ir.md「bind」）。
 */
export interface Bind {
  namespace: string;
  span: Span;
  state: string;
  target: BindTarget;
}
/**
 * 宣言や文の位置（docs/ir.md「Program」）。file はユーザーのファイルの相対パスか hozo:vocab/... か hozo:plugin/...。
 */
export interface Span {
  end: Pos;
  file: string;
  start: Pos;
}
/**
 * ファイルの中の位置（docs/ir.md「Program」）。1 始まり。col はコードポイント数。
 */
export interface Pos {
  col: number;
  line: number;
}
/**
 * bind の対象（docs/ir.md「bind」）。deep は `**`。
 */
export interface BindTarget {
  deep: boolean;
  namespace: string;
}
/**
 * 一つの名前空間（docs/ir.md「Program」）。name は FQN。decls は出現順。
 */
export interface Namespace {
  decls: Decl[];
  description?: Doc;
  name: string;
}
/**
 * model 宣言（docs/ir.md「model / scalar / union」）。
 */
export interface Model {
  decorators: Application[];
  description?: Doc;
  extends?: ModelExtends;
  fields: Field[];
  kind: "model";
  name: string;
  span: Span;
  synthesizedFrom?: string;
}
/**
 * デコレータの適用（docs/ir.md「デコレータの適用」）。
 */
export interface Application {
  args: Arg[];
  decorator: string;
  span: Span;
  via?: string;
}
/**
 * デコレータの引数の値（docs/ir.md「デコレータの適用」）。
 */
export interface Arg {
  param: string;
  value: ArgValue;
}
export interface LiteralValue {
  kind: "literal";
  literal: Literal;
}
export interface StringLiteral {
  kind: "string";
  value: string;
}
export interface IntLiteral {
  kind: "int";
  value: number;
}
export interface FloatLiteral {
  kind: "float";
  value: number;
}
export interface BooleanLiteral {
  kind: "boolean";
  value: boolean;
}
export interface DurationLiteral {
  kind: "duration";
  value: number;
}
export interface RefValue {
  kind: "ref";
  target: string;
}
export interface RefsValue {
  kind: "refs";
  targets: string[];
}
export interface VariantValue {
  arm: string;
  kind: "variant";
  union: string;
}
export interface ListValue {
  items: ArgValue[];
  kind: "list";
}
/**
 * DocComment の内容（docs/ir.md「Doc」）。
 */
export interface Doc {
  body: string;
  params: DocParam[];
  see: string[];
  summary: string;
  throws: DocThrows[];
}
/**
 * Doc の @param（docs/ir.md「Doc」）。
 */
export interface DocParam {
  name: string;
  text: string;
}
/**
 * Doc の @throws（docs/ir.md「Doc」）。
 */
export interface DocThrows {
  exception: string;
  text: string;
}
/**
 * 継承した model（`model D extends B { ... }`）の記録（docs/ir.md「model / scalar / union」の extends）。fields は展開済み。
 */
export interface ModelExtends {
  base: string;
  overridden: string[];
  removed: string[];
}
/**
 * model のフィールド（docs/ir.md「model / scalar / union」の Field）。
 */
export interface Field {
  decorators: Application[];
  description?: Doc;
  name: string;
  span: Span;
  type: Type;
}
/**
 * 核の型（docs/ir.md「型」の core）。
 */
export interface CoreType {
  kind: "core";
  name: CoreName;
}
/**
 * model / scalar / union への参照（docs/ir.md「型」の ref）。
 */
export interface RefType {
  kind: "ref";
  target: string;
}
/**
 * T?（docs/ir.md「型」の optional）。入れ子にはしない。
 */
export interface OptionalType {
  kind: "optional";
  of: Type;
}
/**
 * T[]（docs/ir.md「型」の array）。
 */
export interface ArrayType {
  kind: "array";
  of: Type;
}
/**
 * Record<K, V>（docs/ir.md「型」の record）。
 */
export interface RecordType {
  key: Type;
  kind: "record";
  value: Type;
}
/**
 * 分類のケースで絞り込まれた型（docs/ir.md「型」の case）。cases は match のアームに書かれた順。
 */
export interface CaseType {
  cases: string[];
  classify: string;
  kind: "case";
  of: Type;
}
/**
 * 決して値を返さない（docs/ir.md「型」の never）。Op.returns にだけ現れる。
 */
export interface NeverType {
  kind: "never";
}
/**
 * scalar 宣言（docs/ir.md「model / scalar / union」）。
 */
export interface Scalar {
  base: Type;
  decorators: Application[];
  description?: Doc;
  kind: "scalar";
  name: string;
  span: Span;
}
/**
 * union 宣言（docs/ir.md「model / scalar / union」）。arms と members は常に出し、使わない方は []。
 */
export interface Union {
  arms: UnionArm[];
  decorators: Application[];
  description?: Doc;
  kind: "union";
  members: UnionMember[];
  name: string;
  span: Span;
  variant: UnionVariant;
}
/**
 * arms のユニオンのアーム（docs/ir.md「model / scalar / union」）。
 */
export interface UnionArm {
  description?: Doc;
  name: string;
  span: Span;
  type: Type;
}
/**
 * set のユニオンのメンバー（docs/ir.md「model / scalar / union」）。
 */
export interface UnionMember {
  description?: Doc;
  span: Span;
  target: string;
}
/**
 * state 宣言（docs/ir.md「state / exception / effect」）。type は状態の型。shape と element は type から決まり、element は optional にならない。
 */
export interface State {
  decorators: Application[];
  description?: Doc;
  element: Type;
  kind: "state";
  name: string;
  shape: StateShape;
  span: Span;
  type: Type;
}
/**
 * exception 宣言（docs/ir.md「state / exception / effect」）。
 */
export interface Exception {
  decorators: Application[];
  description?: Doc;
  kind: "exception";
  name: string;
  params: OpParam[];
  span: Span;
}
/**
 * op の引数（docs/ir.md「op」）。
 */
export interface OpParam {
  name: string;
  span: Span;
  type: Type;
}
/**
 * effect 宣言（docs/ir.md「state / exception / effect」）。宛先（target）は一つだけ。writes は書き込む基底の状態の FQN。row は行の遷移（書いたときだけ持つ）。
 */
export interface Effect {
  decorators: Application[];
  description?: Doc;
  kind: "effect";
  name: string;
  params: OpParam[];
  row?: RowTransition;
  span: Span;
  target: EffectTarget;
  writes: string[];
}
/**
 * 行の遷移（docs/ir.md「state / exception / effect」の RowTransition、docs/check.md「行の遷移」）。param は行の引数の名前。classify は行の分類の FQN（名指ししない形では持たない）。from は効果の前のケース（"any" かケース名の列）。to は効果の後のケース（決まっていなければ持たない）。
 */
export interface RowTransition {
  classify?: string;
  from: CaseSet;
  param: string;
  to?: string;
}
/**
 * 基底の状態の組み込みの効果（docs/ir.md「state / exception / effect」の EffectTarget kind "builtin"）。
 */
export interface BuiltinTarget {
  kind: "builtin";
  operation: Operation;
  state: string;
}
/**
 * 分類の遷移（docs/ir.md「state / exception / effect」の EffectTarget kind "transition"）。of は状態の分類の FQN か、組み込みの分類を持つ基底の状態の FQN。
 */
export interface TransitionTarget {
  from: CaseSet;
  kind: "transition";
  of: string;
  to: CaseSet;
}
export interface Read {
  decorators: Application[];
  description?: Doc;
  kind: "read";
  name: string;
  params: OpParam[];
  returns: Type;
  span: Span;
  state: string;
}
/**
 * classify 宣言（docs/ir.md「classify」）。cases は、宣言したケースのすべて。absent は、ない側のケースの名前（あるときだけ持つ）。
 */
export interface Classify {
  absent?: string;
  cases: ClassifyCase[];
  decidedBy: DecidedBy;
  decorators: Application[];
  description?: Doc;
  kind: "classify";
  name: string;
  span: Span;
  subject: ClassifySubject;
}
/**
 * classify のケース（docs/ir.md「classify」）。as は `対象 as T` の並び（なければ []）。
 */
export interface ClassifyCase {
  as: CaseTyping[];
  description?: Doc;
  name: string;
  span: Span;
}
/**
 * ケースの `対象 as T`、`対象 as else`、`対象 as none`（docs/ir.md「classify」の CaseTyping）。state は状態の分類のときだけ持つ。
 */
export interface CaseTyping {
  by: NarrowBy;
  span: Span;
  state?: string;
  type: Type;
}
/**
 * 値の分類の対象の型（docs/ir.md「classify」の subject kind "type"）。
 */
export interface TypeSubject {
  kind: "type";
  type: Type;
}
/**
 * op の分類の対象（docs/ir.md「classify」の subject kind "op"）。op は FQN、type はその op の returns。
 */
export interface OpSubject {
  kind: "op";
  op: string;
  type: Type;
}
/**
 * 状態の分類の対象（docs/ir.md「classify」の subject kind "states"）。基底の状態の FQN（書かれた順）。
 */
export interface StatesSubject {
  kind: "states";
  states: string[];
}
/**
 * decorator 宣言（docs/ir.md「decorator」）。alias を持つ宣言は params / targets / throws / catches が []。
 */
export interface Decorator {
  alias?: DecoratorAlias;
  catches: CatchesClause[];
  decorators: Application[];
  description?: Doc;
  effects: EffectRef[];
  external: boolean;
  kind: "decorator";
  name: string;
  params: Param[];
  reads: ReadRef[];
  repeatable: boolean;
  span: Span;
  targets: DecoratorTarget[];
  throws: ThrowsClause[];
}
/**
 * 名前付きデコレータの中身（docs/ir.md「decorator」の alias）。
 */
export interface DecoratorAlias {
  args: Arg[];
  target: string;
}
/**
 * catches の一節（docs/ir.md「decorator」）。
 */
export interface CatchesClause {
  exceptions: string[];
  halts?: string;
  mode: CatchMode;
}
export interface NamedEffect {
  effect: string;
  kind: "named";
}
export interface DirectEffect {
  kind: "direct";
  operation: Operation;
  state: string;
}
/**
 * デコレータの引数（docs/ir.md「decorator」の Param）。
 */
export interface Param {
  name: string;
  optional: boolean;
  rest: boolean;
  type: ParamType;
}
/**
 * 型で受ける引数（docs/ir.md「decorator」の ParamType kind "type"）。
 */
export interface TypeParam {
  kind: "type";
  types: Type[];
}
/**
 * `any exception` のように宣言の参照で受ける引数（docs/ir.md「decorator」の ParamType kind "any"）。
 */
export interface AnyParam {
  kind: "any";
  of: AnyOf;
  on?: EffectOn;
}
/**
 * `any effect on <state>.<operation>` の制約（docs/ir.md「decorator」）。
 */
export interface EffectOn {
  operation: Operation;
  state: string;
}
export interface NamedRead {
  kind: "named";
  read: string;
}
export interface DirectRead {
  kind: "direct";
  state: string;
}
/**
 * throws の一節（docs/ir.md「decorator」）。exceptions はデコレータの引数名なら "$param:名前"。
 */
export interface ThrowsClause {
  exceptions: string[];
  relations: Relation[];
}
/**
 * 例外が効果に及ぼす関係（docs/ir.md「decorator」）。目的語は常にその op のすべての効果（`effects`）。
 */
export interface Relation {
  kind: RelationKind;
}
/**
 * op 宣言（docs/ir.md「op」）。returns の null は void、never は決して返さない op。body の null は葉。
 */
export interface Op {
  body: Wiring | null;
  carried: Carried;
  decorators: Application[];
  description?: Doc;
  effects: EffectRef[];
  exported: boolean;
  kind: "op";
  name: string;
  params: OpParam[];
  reads: ReadRef[];
  returns: Type | null;
  signature: Signature;
  span: Span;
  throws: ThrowsClause[];
}
/**
 * op の本体（docs/ir.md「配線」）。
 */
export interface Wiring {
  statements: Statement[];
}
export interface ConstStmt {
  expr: Expr;
  kind: "const";
  name: string;
  span: Span;
}
export interface CallExpr {
  call: Call;
  kind: "call";
}
/**
 * op の呼び出し（docs/ir.md「配線」）。args は引数名に対応付け済み。bound は計算結果。
 */
export interface Call {
  args: CallArg[];
  bound: BoundPosition[];
  callee: Callee;
  span: Span;
}
/**
 * 呼び出しの引数（docs/ir.md「配線」の Call.args）。
 */
export interface CallArg {
  param: string;
  value: Atom;
}
export interface NameAtom {
  kind: "name";
  path: string[];
  span: Span;
}
/**
 * 状態の参照（docs/ir.md「配線」の Atom kind "state"）。state は FQN、path はその値のフィールドのパス（状態そのものなら []）。
 */
export interface StateAtom {
  kind: "state";
  path: string[];
  span: Span;
  state: string;
}
export interface LiteralAtom {
  kind: "literal";
  literal: Literal;
  span: Span;
}
export interface NoneAtom {
  kind: "none";
  span: Span;
}
export interface RecordAtom {
  fields: RecordField[];
  kind: "record";
  span: Span;
}
export interface RecordField {
  name: string;
  value: Atom;
}
/**
 * bind で埋まる位置（docs/ir.md「配線」、docs/check.md「bind」）。
 */
export interface BoundPosition {
  path: string[];
  state: string;
}
export interface OpCallee {
  kind: "op";
  op: string;
}
export interface ReadCallee {
  kind: "read";
  read: string;
}
export interface EffectCallee {
  effect: EffectRef;
  kind: "effect";
}
export interface MatchExpr {
  kind: "match";
  match: Match;
}
/**
 * 分類による分岐（docs/ir.md「配線」）。classify は FQN。標準の分類なら "list" か "nullable"。
 */
export interface Match {
  arms: MatchArm[];
  classify: string;
  span: Span;
  subject: MatchSubject;
}
/**
 * match のアーム（docs/ir.md「配線」）。cases は、このアームが受けるケース（else のアームなら、ほかのアームが並べなかったケース）。narrowed は、アームの中で外と型が違う対象（なければ []）。
 */
export interface MatchArm {
  body: ArmBody;
  cases: string[];
  else: boolean;
  narrowed: Narrowing[];
  span: Span;
}
export interface AtomExpr {
  atom: Atom;
  kind: "atom";
}
/**
 * アームの本体の none（docs/ir.md「配線」）。
 */
export interface NoneBody {
  kind: "none";
}
/**
 * アームの本体の throw（docs/ir.md「配線」）。exception は FQN。
 */
export interface ThrowBody {
  args: CallArg[];
  exception: string;
  kind: "throw";
  span: Span;
}
/**
 * アームの中で絞り込まれた対象の型（docs/ir.md「配線」の Narrowing）。state は状態の分類での match のときだけ持つ。
 */
export interface Narrowing {
  state?: string;
  type: Type;
}
/**
 * 値を対象にした match の対象（docs/ir.md「配線」の subject kind "value"）。状態を名前で書いた対象は atom が state。
 */
export interface ValueSubject {
  atom: Atom;
  kind: "value";
}
/**
 * 状態の分類での match（docs/ir.md「配線」の subject kind "classify"）。対象は classify が持つ。
 */
export interface ClassifyOwnedSubject {
  kind: "classify";
}
export interface ReturnStmt {
  expr: Expr;
  kind: "return";
  span: Span;
}
export interface CallStmt {
  call: Call;
  kind: "call";
  span: Span;
}
export interface MatchStmt {
  kind: "match";
  match: Match;
  span: Span;
}
export interface Carried {
  effects: EffectRef[];
  reads: ReadRef[];
}
/**
 * 計算結果の関数の型 Sig(O, v)（docs/ir.md「op」、docs/check.md「計算」）。
 */
export interface Signature {
  effects: EffectRef[];
  exceptions: string[];
  pure: boolean;
  reads: string[];
  relations: SignatureRelation[];
  successEffects: SuccessEffect[];
}
/**
 * 例外が起きたときの効果の状態（docs/ir.md「op」の Signature.relations）。
 */
export interface SignatureRelation {
  effect: EffectRef;
  exception: string;
  state: EffectState;
}
/**
 * 成功のアームでの効果の状態（docs/ir.md「op」の Signature.successEffects）。
 */
export interface SuccessEffect {
  effect: EffectRef;
  state: EffectState;
}
/**
 * その名前空間から見た IR（docs/ir.md「その名前空間から見た IR」）。トランスパイラに渡すもの。単位と名前空間の組ごとに一つ。
 */
export interface View {
  config: NamespaceConfig;
  files: string[];
  namespaces: {
    [k: string]: NamespaceConfig[];
  };
  ops: {
    [k: string]: ViewedSignature;
  };
  program: Program;
  version: "0.10";
  viewer: string;
}
/**
 * 単位での一つの名前空間の設定（docs/ir.md「その名前空間から見た IR」）。index は何番目の [[emit]] か、base は単位の基底、options は単位のオプション（トランスパイラの名前 → テーブル）、seams は単位の継ぎ目（デコレータの FQN → 宣言したトランスパイラ）。
 */
export interface NamespaceConfig {
  base: string;
  dir: string;
  index: number;
  language: string;
  options: {
    [k: string]: {
      [k: string]: unknown;
    };
  };
  out: string;
  seams: {
    [k: string]: string;
  };
  transpilers: string[];
}
/**
 * viewer から見た op の関数の型（docs/ir.md「その名前空間から見た IR」）。
 */
export interface ViewedSignature {
  bound: BoundPosition[];
  effects: EffectRef[];
  exceptions: string[];
  params: OpParam[];
  pure: boolean;
  reads: string[];
  relations: SignatureRelation[];
  successEffects: SuccessEffect[];
}
