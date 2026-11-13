// ir/testdata のゴールデンを生成した型（src/ir.ts）で読み、往復する。
// 型のレベル: ゴールデンの JSON を Program / View として扱い、kind のユニオンを網羅する switch を
// `never` で閉じる（生成した型に変種が増減すればコンパイルが通らない）。
// 実行時: すべての節を歩いて kind が switch に届くこと、JSON の往復がバイト単位で同じことを確かめる。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import type {
  ArgValue,
  ArmBody,
  Atom,
  CaseSet,
  ClassifySubject,
  Decl,
  EffectTarget,
  Expr,
  Literal,
  Match,
  MatchSubject,
  ParamType,
  Program,
  Statement,
  Type,
  View,
} from "../src/index.js";

// build/test/ir.test.js から見たリポジトリのルート。
const root = resolve(dirname(fileURLToPath(import.meta.url)), "..", "..", "..", "..");
const golden = (name: string) => readFileSync(resolve(root, "ir", "testdata", name), "utf8");

function unreachable(x: never): never {
  throw new Error(`unhandled: ${JSON.stringify(x)}`);
}

const seen = new Set<string>();

function walkType(t: Type): void {
  seen.add(`type:${t.kind}`);
  switch (t.kind) {
    case "core":
      assert.ok(["string", "boolean", "int", "float", "Time", "Duration"].includes(t.name));
      return;
    case "ref":
      assert.equal(typeof t.target, "string");
      return;
    case "optional":
    case "array":
      walkType(t.of);
      return;
    case "record":
      walkType(t.key);
      walkType(t.value);
      return;
    case "case":
      assert.ok(Array.isArray(t.cases));
      walkType(t.of);
      return;
    default:
      unreachable(t);
  }
}

function walkLiteral(l: Literal): void {
  seen.add(`literal:${l.kind}`);
  switch (l.kind) {
    case "string":
      assert.equal(typeof l.value, "string");
      return;
    case "int":
    case "duration":
      assert.ok(Number.isInteger(l.value), `${l.kind} must be an integer`);
      return;
    case "float":
      assert.equal(typeof l.value, "number");
      return;
    case "boolean":
      assert.equal(typeof l.value, "boolean");
      return;
    default:
      unreachable(l);
  }
}

function walkArgValue(v: ArgValue): void {
  seen.add(`argValue:${v.kind}`);
  switch (v.kind) {
    case "literal":
      walkLiteral(v.literal);
      return;
    case "ref":
      assert.equal(typeof v.target, "string");
      return;
    case "refs":
      assert.ok(Array.isArray(v.targets));
      return;
    case "variant":
      assert.equal(typeof v.union, "string");
      assert.equal(typeof v.arm, "string");
      return;
    case "list":
      v.items.forEach(walkArgValue);
      return;
    default:
      unreachable(v);
  }
}

function walkParamType(p: ParamType): void {
  seen.add(`paramType:${p.kind}`);
  switch (p.kind) {
    case "type":
      p.types.forEach(walkType);
      return;
    case "any":
      assert.ok(["exception", "effect", "state", "op", "model", "variant"].includes(p.of));
      if (p.on) assert.ok(["post", "put", "delete"].includes(p.on.operation));
      return;
    default:
      unreachable(p);
  }
}

function walkAtom(a: Atom): void {
  seen.add(`atom:${a.kind}`);
  switch (a.kind) {
    case "name":
      assert.ok(a.path.length > 0);
      return;
    case "literal":
      walkLiteral(a.literal);
      return;
    case "none":
      return;
    case "record":
      for (const f of a.fields) walkAtom(f.value);
      return;
    default:
      unreachable(a);
  }
}

// docs/ir.md「配線」の Match.subject: 値、基底の状態、分類の宣言が持つ、の三つ。
function walkMatchSubject(s: MatchSubject, classify: string): void {
  seen.add(`matchSubject:${s.kind}`);
  switch (s.kind) {
    case "value":
      walkAtom(s.atom);
      return;
    case "state":
      assert.equal(typeof s.state, "string");
      // 組み込みの分類なら classify は状態の形
      assert.ok(["list", "nullable"].includes(classify), `classify of a state match: ${classify}`);
      return;
    case "classify":
      assert.deepEqual(Object.keys(s), ["kind"]);
      return;
    default:
      unreachable(s);
  }
}

function walkCaseSet(c: CaseSet): void {
  if (c === "any") {
    seen.add("caseSet:any");
    return;
  }
  seen.add("caseSet:list");
  assert.ok(Array.isArray(c));
  for (const name of c) assert.equal(typeof name, "string");
}

// docs/ir.md「state / exception / effect」の EffectTarget: 組み込みの効果か、分類の遷移。
function walkEffectTarget(t: EffectTarget): void {
  seen.add(`effectTarget:${t.kind}`);
  switch (t.kind) {
    case "builtin":
      assert.equal(typeof t.state, "string");
      assert.ok(["post", "put", "delete"].includes(t.operation));
      return;
    case "transition":
      assert.equal(typeof t.of, "string");
      walkCaseSet(t.from);
      walkCaseSet(t.to);
      return;
    default:
      unreachable(t);
  }
}

// docs/ir.md「classify」の subject: 値の分類なら型、状態の分類なら基底の状態の FQN。
function walkClassifySubject(s: ClassifySubject): void {
  seen.add(`classifySubject:${s.kind}`);
  switch (s.kind) {
    case "type":
      walkType(s.type);
      return;
    case "states":
      assert.ok(s.states.length > 0);
      for (const st of s.states) assert.equal(typeof st, "string");
      return;
    default:
      unreachable(s);
  }
}

function walkMatch(m: Match): void {
  walkMatchSubject(m.subject, m.classify);
  for (const arm of m.arms) {
    if (arm.cases !== "default") assert.ok(Array.isArray(arm.cases));
    const body: ArmBody = arm.body;
    seen.add(`armBody:${body.kind}`);
    switch (body.kind) {
      case "call":
        body.call.args.forEach((x) => walkAtom(x.value));
        break;
      case "none":
        break;
      default:
        unreachable(body);
    }
  }
}

function walkExpr(e: Expr): void {
  seen.add(`expr:${e.kind}`);
  switch (e.kind) {
    case "call":
      e.call.args.forEach((x) => walkAtom(x.value));
      return;
    case "match":
      walkMatch(e.match);
      return;
    case "atom":
      walkAtom(e.atom);
      return;
    default:
      unreachable(e);
  }
}

function walkStatement(s: Statement): void {
  seen.add(`statement:${s.kind}`);
  switch (s.kind) {
    case "const":
      walkExpr(s.expr);
      return;
    case "return":
      walkExpr(s.expr);
      return;
    case "call":
      s.call.args.forEach((x) => walkAtom(x.value));
      return;
    case "match":
      walkMatch(s.match);
      return;
    default:
      unreachable(s);
  }
}

function walkDecl(d: Decl): void {
  seen.add(`decl:${d.kind}`);
  assert.equal(typeof d.name, "string");
  for (const app of d.decorators) app.args.forEach((a) => walkArgValue(a.value));
  switch (d.kind) {
    case "model":
      for (const f of d.fields) {
        walkType(f.type);
        if (f.default) walkLiteral(f.default);
      }
      return;
    case "scalar":
      walkType(d.base);
      return;
    case "union":
      d.arms.forEach((a) => walkType(a.type));
      return;
    case "state":
      seen.add(`stateShape:${d.shape}`);
      assert.ok(["list", "nullable", "single"].includes(d.shape));
      assert.notEqual(d.element.kind, "optional", "State.element is never optional (STATE-3)");
      walkType(d.element);
      return;
    case "exception":
      if (d.value) walkType(d.value);
      return;
    case "effect":
      walkEffectTarget(d.target);
      assert.ok(d.writes.length > 0);
      for (const st of d.writes) assert.equal(typeof st, "string");
      return;
    case "classify":
      walkClassifySubject(d.subject);
      return;
    case "decorator":
      d.params.forEach((p) => walkParamType(p.type));
      for (const t of d.throws) {
        // Relation は kind だけ（目的語は常にその op のすべての効果。docs/ir.md「decorator」）
        for (const r of t.relations) {
          assert.ok(r.kind === "prevents" || r.kind === "keeps");
          assert.deepEqual(Object.keys(r), ["kind"]);
        }
      }
      for (const target of d.targets) {
        assert.ok(["op", "field", "state", "exception", "scalar", "model", "namespace"].includes(target));
      }
      if (d.alias) d.alias.args.forEach((a) => walkArgValue(a.value));
      return;
    case "op":
      d.params.forEach((p) => walkType(p.type));
      if (d.returns !== null) walkType(d.returns);
      if (d.body !== null) d.body.statements.forEach(walkStatement);
      return;
    default:
      unreachable(d);
  }
}

function walkProgram(p: Program): void {
  assert.equal(p.version, "0.1");
  assert.equal(p.namespaces[0]?.name, "");
  for (const ns of p.namespaces) {
    // Namespace.decorators は常にある（空でも []。docs/ir.md「Program」）
    assert.ok(Array.isArray(ns.decorators));
    for (const a of ns.decorators) a.args.forEach((arg) => walkArgValue(arg.value));
    ns.decls.forEach(walkDecl);
  }
  for (const b of p.binds) assert.equal(typeof b.target.deep, "boolean");
}

test("program.golden.json reads as Program and round-trips", () => {
  const text = golden("program.golden.json");
  const program: Program = JSON.parse(text);
  walkProgram(program);
  assert.equal(JSON.stringify(program, null, 2) + "\n", text);
});

test("view.golden.json reads as View and round-trips", () => {
  const text = golden("view.golden.json");
  const view: View = JSON.parse(text);
  assert.equal(view.version, "0.1");
  assert.equal(typeof view.viewer, "string");
  assert.ok(view.namespaces[view.viewer]);
  walkProgram(view.program);
  for (const sig of Object.values(view.ops)) {
    sig.params.forEach((p) => walkType(p.type));
    for (const r of sig.relations) assert.ok(["prevented", "kept", "unknown"].includes(r.state));
  }
  assert.equal(JSON.stringify(view, null, 2) + "\n", text);
});

test("goldens cover every kind of every union", () => {
  const want = [
    ...["core", "ref", "optional", "array", "record", "case"].map((k) => `type:${k}`),
    ...["model", "scalar", "union", "state", "exception", "effect", "classify", "decorator", "op"].map((k) => `decl:${k}`),
    ...["string", "int", "float", "boolean", "duration"].map((k) => `literal:${k}`),
    ...["literal", "ref", "refs", "variant", "list"].map((k) => `argValue:${k}`),
    ...["type", "any"].map((k) => `paramType:${k}`),
    ...["name", "literal", "none", "record"].map((k) => `atom:${k}`),
    ...["call", "none"].map((k) => `armBody:${k}`),
    ...["call", "match", "atom"].map((k) => `expr:${k}`),
    ...["const", "return", "call", "match"].map((k) => `statement:${k}`),
    ...["list", "nullable", "single"].map((k) => `stateShape:${k}`),
    ...["builtin", "transition"].map((k) => `effectTarget:${k}`),
    ...["any", "list"].map((k) => `caseSet:${k}`),
    ...["type", "states"].map((k) => `classifySubject:${k}`),
    ...["value", "state", "classify"].map((k) => `matchSubject:${k}`),
  ];
  for (const k of want) assert.ok(seen.has(k), `goldens contain no ${k}`);
});
