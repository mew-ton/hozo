// run() の終了コードと出力（docs/plugin-protocol.md「起動」。Go の plugin.Run と同じ）。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { Readable } from "node:stream";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import { PROTOCOL, UnsupportedDirectionError, opsWith, run, type Handler, type View } from "../src/index.js";

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

/** 何も選ばない select（run は四つの列を [] で書く。docs/plugin-protocol.md「emit」の例）。 */
const noSelect = { roots: [], stops: [], states: [], wraps: [] };

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
  const r = await exec(full, { protocol: PROTOCOL, direction: "emit", view });
  assert.equal(r.code, 0);
  assert.equal(r.stderr, "");
  assert.ok(r.stdout.endsWith("\n"));
  assert.deepEqual(JSON.parse(r.stdout), {
    files: [{ path: "gen/browser/out.ts", content: "// browser\n" }],
    warnings: [],
    select: noSelect,
  });
});

test("emit: writes select with all four lists (docs/plugin-protocol.md「選択」)", async () => {
  const handler: Handler = {
    emit: () => ({
      files: [],
      warnings: [],
      select: { roots: ["browser.openMe"], wraps: [{ op: "browser.openMe", decorator: "browser.retry" }] },
    }),
  };
  const r = await exec(handler, { protocol: PROTOCOL, direction: "emit", view });
  assert.equal(r.code, 0);
  assert.deepEqual(JSON.parse(r.stdout), {
    files: [],
    warnings: [],
    select: {
      roots: ["browser.openMe"],
      stops: [],
      states: [],
      wraps: [{ op: "browser.openMe", decorator: "browser.retry" }],
    },
  });
});

test("emit: fills files/warnings with [] and drops empty errors (like Go)", async () => {
  const handler: Handler = { emit: () => ({ errors: [] }) as unknown as never };
  const r = await exec(handler, { protocol: PROTOCOL, direction: "emit", view });
  assert.equal(r.code, 0);
  assert.deepEqual(JSON.parse(r.stdout), { files: [], warnings: [], select: noSelect });
});

test("emit: keeps non-empty errors", async () => {
  const handler: Handler = {
    emit: () => ({ files: [], warnings: [], errors: [{ code: "CLI-1", at: "a.b", message: "bad" }] }),
  };
  const r = await exec(handler, { protocol: PROTOCOL, direction: "emit", view });
  assert.equal(r.code, 0);
  assert.deepEqual(JSON.parse(r.stdout), {
    files: [],
    warnings: [],
    errors: [{ code: "CLI-1", at: "a.b", message: "bad" }],
    select: noSelect,
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
  assert.equal(out.ir.version, "0.1");
  assert.deepEqual(out.ir, view.program);
});

test("protocol mismatch: {error:{code:protocol}} on stdout and exit 2", async () => {
  const r = await exec(full, { protocol: "0.2", direction: "emit", view });
  assert.equal(r.code, 2);
  assert.equal(r.stderr, "");
  const out = JSON.parse(r.stdout);
  assert.equal(out.error.code, "protocol");
  assert.match(out.error.message, /0\.2/);
  assert.match(out.error.message, /0\.1/);
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
  const r = await exec(handler, { protocol: PROTOCOL, direction: "emit", view });
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

test("opsWith: ops of the viewer namespace with the decorator, in declaration order (via counts)", () => {
  // ゴールデンの View は browser から見たものだが、op を持つのは server なので、見る側を server にして確かめる。
  const server: View = { ...view, viewer: "server" };
  assert.deepEqual(opsWith(server, "server.retry"), ["server.findUser"]);
  assert.deepEqual(opsWith(server, "server.retryThrice"), ["server.findUser"]); // 名前付きデコレータ（via）
  assert.deepEqual(opsWith(server, "server.nope"), []);
  assert.deepEqual(opsWith(view, "server.retry"), []); // viewer（browser）の op だけを見る
});
