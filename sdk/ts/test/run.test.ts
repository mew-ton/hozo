// run() の終了コードと出力（docs/plugin-protocol.md「起動」。Go の plugin.Run と同じ）。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { Readable } from "node:stream";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import {
  PROTOCOL,
  UnsupportedDirectionError,
  calledOp,
  directEffects,
  directReads,
  effectWrites,
  inUnit,
  lookup,
  namespace,
  options,
  opsWith,
  readState,
  readStates,
  run,
  sameRef,
  signature,
  type Callee,
  type EffectRef,
  type EmitRequest,
  type Handler,
  type Op,
  type ReadRef,
  type View,
} from "../src/index.js";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..", "..", "..", "..");
const view: View = JSON.parse(readFileSync(resolve(root, "ir", "testdata", "view.golden.json"), "utf8"));

class Collector {
  text = "";
  write(chunk: string) {
    this.text += chunk;
  }
}

async function exec(handler: Handler, input: string | object) {
  const stdout = new Collector();
  const stderr = new Collector();
  const raw = typeof input === "string" ? input : JSON.stringify(input);
  const code = await run(handler, Readable.from([raw]), stdout, stderr);
  return { code, stdout: stdout.text, stderr: stderr.text };
}

const full: Handler = {
  emit: (req) => ({
    files: [{ path: `${req.view.config.dir}/out.ts`, content: `// ${req.view.viewer}\n` }],
    warnings: [],
  }),
  check: (req) => ({
    diagnostics: [{ severity: "warning", code: "T-1", at: req.view.viewer, message: "m" }],
  }),
  import: async (req) => ({
    ir: view.program,
    warnings: [{ at: req.files[0]?.path ?? "", message: "w" }],
  }),
};

test("emit: writes the response and exits 0", async () => {
  const r = await exec(full, { protocol: PROTOCOL, direction: "emit", transpiler: "t", view });
  assert.equal(r.code, 0);
  assert.equal(r.stderr, "");
  assert.ok(r.stdout.endsWith("\n"));
  assert.deepEqual(JSON.parse(r.stdout), {
    files: [{ path: "gen/browser/out.ts", content: "// browser\n" }],
    warnings: [],
  });
});

test("emit: the response has no select (docs/plugin-protocol.md「選択をなくしたこと」)", async () => {
  // 版 0.3 までの select を返す Handler があっても、run は書かない（応答は files / warnings / errors だけ）
  const handler: Handler = {
    emit: () => ({ files: [], warnings: [], select: { roots: ["browser.openMe"] } }) as unknown as never,
  };
  const r = await exec(handler, { protocol: PROTOCOL, direction: "emit", transpiler: "t", view });
  assert.equal(r.code, 0);
  assert.deepEqual(JSON.parse(r.stdout), { files: [], warnings: [] });
});

test("options: the options of the invoked transpiler (docs/config.md「オプション」)", () => {
  const withOptions: View = {
    ...view,
    config: { ...view.config, options: { "go-http": { validate: true, limits: { max: 3 } }, go: {} } },
  };
  const req = (transpiler: string, v: View): EmitRequest => ({ protocol: PROTOCOL, direction: "emit", transpiler, view: v });
  assert.deepEqual(options(req("go-http", withOptions)), { validate: true, limits: { max: 3 } });
  assert.deepEqual(options(req("go", withOptions)), {});
  // 書かれていなければ、空のテーブル
  assert.deepEqual(options(req("go-valid", withOptions)), {});
  assert.deepEqual(options(req("toString", withOptions)), {});
});

test("emit: fills files/warnings with [] and drops empty errors (like Go)", async () => {
  const handler: Handler = { emit: () => ({ errors: [] }) as unknown as never };
  const r = await exec(handler, { protocol: PROTOCOL, direction: "emit", transpiler: "t", view });
  assert.equal(r.code, 0);
  assert.deepEqual(JSON.parse(r.stdout), { files: [], warnings: [] });
});

test("emit: keeps non-empty errors", async () => {
  const handler: Handler = {
    emit: () => ({ files: [], warnings: [], errors: [{ code: "CLI-1", at: "a.b", message: "bad" }] }),
  };
  const r = await exec(handler, { protocol: PROTOCOL, direction: "emit", transpiler: "t", view });
  assert.equal(r.code, 0);
  assert.deepEqual(JSON.parse(r.stdout), {
    files: [],
    warnings: [],
    errors: [{ code: "CLI-1", at: "a.b", message: "bad" }],
  });
});

test("check: writes diagnostics and exits 0", async () => {
  const r = await exec(full, { protocol: PROTOCOL, direction: "check", view });
  assert.equal(r.code, 0);
  assert.deepEqual(JSON.parse(r.stdout), {
    diagnostics: [{ severity: "warning", code: "T-1", at: "browser", message: "m" }],
  });
});

test("import: writes ir and warnings and exits 0", async () => {
  const r = await exec(full, {
    protocol: PROTOCOL,
    direction: "import",
    format: "openapi",
    files: [{ path: "openapi.yaml", content: "" }],
  });
  assert.equal(r.code, 0);
  const out = JSON.parse(r.stdout);
  assert.deepEqual(out.warnings, [{ at: "openapi.yaml", message: "w" }]);
  assert.equal(out.ir.version, PROTOCOL);
  assert.deepEqual(out.ir, view.program);
});

test("protocol mismatch: {error:{code:protocol}} on stdout and exit 2", async () => {
  const r = await exec(full, { protocol: "0.1", direction: "emit", transpiler: "t", view });
  assert.equal(r.code, 2);
  assert.equal(r.stderr, "");
  const out = JSON.parse(r.stdout);
  assert.equal(out.error.code, "protocol");
  assert.match(out.error.message, /0\.1/);
  assert.match(out.error.message, /0\.14/);
});

test("protocol missing: exit 2 with code protocol (checked before direction)", async () => {
  const r = await exec(full, { direction: "nope" });
  assert.equal(r.code, 2);
  assert.equal(JSON.parse(r.stdout).error.code, "protocol");
});

test("unknown direction: exit 2 with code unsupported", async () => {
  const r = await exec(full, { protocol: PROTOCOL, direction: "lint", view });
  assert.equal(r.code, 2);
  const out = JSON.parse(r.stdout);
  assert.equal(out.error.code, "unsupported");
  assert.match(out.error.message, /lint/);
});

test("handler without the method: exit 2 with code unsupported", async () => {
  const emitOnly: Handler = { emit: () => ({ files: [], warnings: [] }) };
  const r = await exec(emitOnly, { protocol: PROTOCOL, direction: "check", view });
  assert.equal(r.code, 2);
  assert.equal(JSON.parse(r.stdout).error.code, "unsupported");
});

test("handler throwing UnsupportedDirectionError: exit 2 with code unsupported", async () => {
  const handler: Handler = {
    import: () => {
      throw new UnsupportedDirectionError();
    },
  };
  const r = await exec(handler, { protocol: PROTOCOL, direction: "import", format: "x", files: [] });
  assert.equal(r.code, 2);
  assert.equal(JSON.parse(r.stdout).error.code, "unsupported");
});

test("handler throwing another error: message on stderr, nothing on stdout, exit 1", async () => {
  const handler: Handler = {
    emit: async () => {
      throw new Error("boom");
    },
  };
  const r = await exec(handler, { protocol: PROTOCOL, direction: "emit", transpiler: "t", view });
  assert.equal(r.code, 1);
  assert.equal(r.stdout, "");
  assert.equal(r.stderr, "error: emit: boom\n");
});

test("request that is not JSON: exit 1", async () => {
  const r = await exec(full, "{ not json");
  assert.equal(r.code, 1);
  assert.equal(r.stdout, "");
  assert.match(r.stderr, /^error: decode request: /);
});

test("request that is not an object: exit 1", async () => {
  const r = await exec(full, "[1, 2]");
  assert.equal(r.code, 1);
  assert.match(r.stderr, /not a JSON object/);
});

test("unreadable stdin: exit 1", async () => {
  const broken = new Readable({
    read() {
      this.destroy(new Error("pipe closed"));
    },
  });
  const stdout = new Collector();
  const stderr = new Collector();
  const code = await run(full, broken, stdout, stderr);
  assert.equal(code, 1);
  assert.equal(stderr.text, "error: read request: pipe closed\n");
});

test("stdout that cannot be written: exit 1", async () => {
  const stderr = new Collector();
  const code = await run(
    full,
    Readable.from([JSON.stringify({ protocol: PROTOCOL, direction: "check", view })]),
    {
      write() {
        throw new Error("EPIPE");
      },
    },
    stderr,
  );
  assert.equal(code, 1);
  assert.equal(stderr.text, "error: write response: EPIPE\n");
});

test("emit: the request carries the transpiler; an unknown one is refused with the handler's message", async () => {
  // 複数のトランスパイラを提供するプラグインは、要求の transpiler で処理を分ける（docs/plugin-protocol.md「emit」）
  const handler: Handler = {
    emit: (req) => {
      if (req.transpiler === "a") return { files: [{ path: "a.txt", content: "a" }], warnings: [] };
      if (req.transpiler === "b") return { files: [], warnings: [{ at: "server.findUser", message: "b" }] };
      throw new UnsupportedDirectionError(`transpiler ${JSON.stringify(req.transpiler)} is not provided`);
    },
  };
  const a = await exec(handler, { protocol: PROTOCOL, direction: "emit", transpiler: "a", view });
  assert.deepEqual(JSON.parse(a.stdout).files, [{ path: "a.txt", content: "a" }]);
  const b = await exec(handler, { protocol: PROTOCOL, direction: "emit", transpiler: "b", view });
  assert.deepEqual(JSON.parse(b.stdout).warnings, [{ at: "server.findUser", message: "b" }]);
  const c = await exec(handler, { protocol: PROTOCOL, direction: "emit", transpiler: "c", view });
  assert.equal(c.code, 2);
  const out = JSON.parse(c.stdout);
  assert.equal(out.error.code, "unsupported");
  assert.match(out.error.message, /"c"/);
});

test("opsWith: ops of the viewer namespace in the unit with the decorator, in declaration order (via counts)", () => {
  // ゴールデンの View は browser から見たものだが、op を持つのは server なので、見る側を server にして確かめる。
  const server: View = { ...view, viewer: "server" };
  assert.deepEqual(opsWith(server, "server.retry"), ["server.findUser"]);
  assert.deepEqual(opsWith(server, "server.retryThrice"), ["server.findUser"]); // 名前付きデコレータ（via）
  assert.deepEqual(opsWith(server, "server.nope"), []);
  assert.deepEqual(opsWith(view, "server.retry"), []); // viewer（browser）の op だけを見る
  // 単位の対象でない op（span.file が view.files にない。docs/config.md「in」）は返さない
  const narrowed: View = { ...server, files: ["browser.hozo"] };
  assert.deepEqual(opsWith(narrowed, "server.retry"), []);
});

test("inUnit: declarations of the viewer namespace whose file is in view.files", () => {
  const server: View = { ...view, viewer: "server" };
  assert.equal(inUnit(server, "server.findUser"), true);
  assert.equal(inUnit(view, "server.findUser"), false); // viewer（browser）の宣言ではない
  assert.equal(inUnit(server, "server.nope"), false);
  assert.equal(inUnit({ ...server, files: ["browser.hozo"] }, "server.findUser"), false);
});

// Go の SDK（plugin.Run）と同じ表を読む: 要求の protocol と direction の読み方と終了コード
// （docs/plugin-protocol.md「要求と応答」「起動」）。文字列でない protocol は「合わない」で code "protocol"（終了コード 2）。
// Go の側は plugin/plugin_test.go の TestRequestHead。
const headCases: { cases: { name: string; request: string; exit: number; code: string }[] } = JSON.parse(
  readFileSync(resolve(root, "plugin", "testdata", "request_head.json"), "utf8"),
);

for (const c of headCases.cases) {
  test(`request head (shared with the Go SDK): ${c.name}`, async () => {
    let called = "";
    const handler: Handler = {
      emit: () => ((called = "emit"), { files: [], warnings: [] }),
      check: () => ((called = "check"), { diagnostics: [] }),
      import: () => ((called = "import"), { ir: view.program, warnings: [] }),
    };
    const r = await exec(handler, c.request);
    assert.equal(r.code, c.exit, `stdout ${r.stdout} stderr ${r.stderr}`);
    if (r.code === 2) {
      const out = JSON.parse(r.stdout);
      assert.equal(out.error.code, c.code);
      assert.notEqual(out.error.message, "");
      assert.equal(called, "", "断った要求で handler を呼んだ");
    }
    if (r.code === 1) {
      assert.equal(r.stdout, "");
      assert.notEqual(r.stderr, "");
    }
  });
}

test("request head table is not empty", () => {
  assert.ok(headCases.cases.length > 10);
});

// Go の ir.Program.Lookup / ir.Program.Namespace / ir.View.Signature と同じ引き方（docs/packages.md「`ir`（公開）」）。
test("lookup / namespace / signature: same lookups as the Go ir package", () => {
  for (const ns of view.program.namespaces) {
    assert.equal(namespace(view.program, ns.name), ns);
  }
  assert.equal(namespace(view.program, "")?.name, ""); // グローバル
  assert.equal(namespace(view.program, "no.such.namespace"), undefined);

  // すべての宣言が FQN で引け、引いたものはその宣言そのもの
  let decls = 0;
  for (const n of view.program.namespaces) {
    for (const d of n.decls) {
      assert.equal(lookup(view.program, d.name), d);
      decls++;
    }
  }
  assert.ok(decls > 0);
  assert.equal(lookup(view.program, "no.such.Decl"), undefined);
  assert.equal(lookup(view.program, view.viewer), undefined); // 名前空間は宣言ではない

  // すべての op に viewer から見た関数の型がある
  const ops = Object.keys(view.ops);
  assert.ok(ops.length > 0);
  for (const fqn of ops) {
    assert.equal(signature(view, fqn), view.ops[fqn]);
    assert.equal(lookup(view.program, fqn)?.kind, "op");
  }
  assert.equal(signature(view, "no.such.op"), undefined);
  assert.equal(signature(view, "toString"), undefined); // Object.prototype の名前を拾わない
});

// Go の ir.Op.DirectReads / DirectEffects と同じ（docs/packages.md「`ir`（公開）」、docs/check.md「デコレータが運ぶ reads と effects」）。
test("directReads / directEffects: written first, then carried, duplicates folded to the first", () => {
  const record = lookup(view.program, "server.record");
  assert.equal(record?.kind, "op");
  if (record?.kind !== "op") return;
  const state = (s: string): ReadRef => ({ kind: "direct", state: s });
  const named = (e: string): EffectRef => ({ effect: e, kind: "named" });
  assert.deepEqual(directReads(record), [state("db"), state("session")]);
  assert.deepEqual(directEffects(record), [named("server.UserCreated"), named("login"), named("logout")]);
  const both: Op = { ...record, reads: [state("session"), state("x")], effects: [named("logout")] };
  assert.deepEqual(directReads(both), [state("session"), state("x"), state("db")]);
  assert.deepEqual(directEffects(both), [named("logout"), named("server.UserCreated"), named("login")]);
  // 元の op は変えない
  assert.deepEqual(record.reads, []);
  // デコレータが何も運ばない op は、書いたものだけ
  const findUser = lookup(view.program, "server.findUser");
  if (findUser?.kind !== "op") return assert.fail("server.findUser is not an op");
  assert.deepEqual(directReads(findUser), findUser.reads);
  assert.deepEqual(directEffects(findUser), findUser.effects);
});

// 要素は値として比べられないので sameRef で見る（docs/packages.md「`ir`（公開）」）。
test("sameRef: named and direct elements are different even for the same name", () => {
  const namedRead: ReadRef = { kind: "named", read: "s.x" };
  const directRead: ReadRef = { kind: "direct", state: "s.x" };
  const namedEffect: EffectRef = { effect: "s.x", kind: "named" };
  const post: EffectRef = { kind: "direct", operation: "post", state: "s.x" };
  const put: EffectRef = { kind: "direct", operation: "put", state: "s.x" };
  const all = [namedRead, directRead, namedEffect, post, put];
  all.forEach((a, i) => all.forEach((b, j) => assert.equal(sameRef(a, b), i === j, `${i} / ${j}`)));
  assert.ok(sameRef(post, { ...post }));
  // 同じ要素は、directEffects で一つに寄る（参照が違っても）
  const record = lookup(view.program, "server.record");
  if (record?.kind !== "op") return assert.fail("server.record is not an op");
  assert.deepEqual(directEffects({ ...record, effects: [post, { ...post }], carried: { reads: [], effects: [{ ...post }] } }), [post]);
});

// Go の ir.Program.ReadState / ReadStates / EffectWrites と同じ（docs/packages.md「`ir`（公開）」）。
test("readState / readStates / effectWrites: named elements follow the declaration, direct ones carry the state", () => {
  const p = view.program;
  assert.equal(readState(p, { kind: "named", read: "server.userById" }), "db");
  assert.equal(readState(p, { kind: "direct", state: "session" }), "session");
  assert.equal(readState(p, { kind: "named", read: "no.such" }), undefined);
  assert.equal(readState(p, { kind: "named", read: "server.findUser" }), undefined); // read の宣言でない
  const findUser = lookup(p, "server.findUser");
  if (findUser?.kind !== "op") return assert.fail("server.findUser is not an op");
  assert.deepEqual(readStates(p, findUser.reads), ["db", "session"]);
  // 重複は最初の出現に寄せ、宣言のない要素は飛ばす
  assert.deepEqual(
    readStates(p, [{ kind: "direct", state: "session" }, { kind: "named", read: "no.such" }, ...findUser.reads]),
    ["session", "db"],
  );
  assert.deepEqual(effectWrites(p, { effect: "server.UserCreated", kind: "named" }), ["db"]);
  assert.deepEqual(effectWrites(p, { kind: "direct", operation: "delete", state: "session" }), ["session"]);
  assert.equal(effectWrites(p, { effect: "no.such", kind: "named" }), undefined);
});

// Go の ir.Call.CalledOp に当たる（docs/ir.md「配線」の Callee）。
test("calledOp: only an op callee has a name", () => {
  const callees: Callee[] = [];
  for (const ns of view.program.namespaces) {
    for (const d of ns.decls) {
      if (d.kind !== "op" || d.body === null) continue;
      for (const s of d.body.statements) {
        const call = s.kind === "call" ? s.call : (s.kind === "val" || s.kind === "return") && s.expr !== undefined && s.expr.kind === "call" ? s.expr.call : undefined;
        if (call === undefined) continue;
        callees.push(call.callee);
        assert.equal(calledOp(call), call.callee.kind === "op" ? call.callee.op : undefined);
      }
    }
  }
  // ゴールデンには、三つの呼び出し先がすべてある
  assert.deepEqual([...new Set(callees.map((c) => c.kind))].sort(), ["effect", "op", "read"]);
});
