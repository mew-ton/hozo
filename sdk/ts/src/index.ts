// Hozo のプラグインを TypeScript（Node）で書くための SDK。
// プラグインは標準入出力で JSON をやり取りする実行ファイル（docs/plugin-protocol.md「起動」）。
// main / run は Go の plugin.Main / plugin.Run と同じ振る舞い（docs/packages.md「`plugin`（公開）」）。
import type { Readable } from "node:stream";
import {
  PROTOCOL,
  UnsupportedDirectionError,
  type CheckRequest,
  type CheckResponse,
  type EmitRequest,
  type EmitResponse,
  type Handler,
  type ImportRequest,
  type ImportResponse,
  type PluginError,
  type Select,
} from "./protocol.js";
import type { Decl, Namespace, Program, View, ViewedSignature } from "./ir.js";

export * from "./ir.js";
export * from "./protocol.js";

/** run が書く先。process.stdout / process.stderr や、テストの収集器。 */
export interface Sink {
  write(chunk: string): unknown;
}

/**
 * 標準入力の要求を読み、protocol を確かめ、handler を呼び、標準出力に応答を書く。
 * 終了コードを process.exitCode に置いて返す。プラグインの main は `await main(handler)` と書く
 * （process.exit は呼ばない。パイプの標準出力を書き切ってから終わるため）。
 */
export async function main(handler: Handler): Promise<number> {
  const code = await run(handler, process.stdin, process.stdout, process.stderr);
  process.exitCode = code;
  return code;
}

/**
 * main の本体で、入出力を差し替えられる。stdin から要求を一つ読み、stdout に応答を、
 * stderr に誤りのメッセージを書く。終了コード（docs/plugin-protocol.md「起動」）:
 * 成功なら 0。protocol が合わないか direction を実装していなければ 2
 * （stdout に {"error":{"code":"protocol"|"unsupported","message":...}}）。
 * それ以外の誤り（要求が JSON のオブジェクトとして読めない、handler が投げた、書けない）なら 1（stderr にメッセージ）。
 * protocol は文字列で PROTOCOL と等しくなければ「合わない」（ない、null、文字列でないものを含む）。文字列でない
 * direction は、実装していない direction と同じ。Go の plugin.Run と同じ終了コードを返すことは、二つの SDK の
 * テストが同じ表（plugin/testdata/request_head.json）で確かめる。
 */
export async function run(handler: Handler, stdin: Readable, stdout: Sink, stderr: Sink): Promise<number> {
  let raw: string;
  try {
    raw = await readAll(stdin);
  } catch (err) {
    return fail(stderr, `read request: ${message(err)}`);
  }
  let head: unknown;
  try {
    head = JSON.parse(raw);
  } catch (err) {
    return fail(stderr, `decode request: ${message(err)}`);
  }
  if (typeof head !== "object" || head === null || Array.isArray(head)) {
    return fail(stderr, "decode request: not a JSON object");
  }
  const req = head as { protocol?: unknown; direction?: unknown };
  if (req.protocol !== PROTOCOL) {
    return refuse(
      stdout,
      stderr,
      "protocol",
      `protocol ${JSON.stringify(req.protocol ?? "")} is not supported (this plugin speaks ${JSON.stringify(PROTOCOL)})`,
    );
  }
  const direction = typeof req.direction === "string" ? req.direction : "";
  const unsupported = () => refuse(stdout, stderr, "unsupported", `direction ${JSON.stringify(direction)} is not supported`);

  let resp: unknown;
  try {
    switch (direction) {
      case "emit": {
        if (!handler.emit) return unsupported();
        const r = (await handler.emit(head as EmitRequest)) ?? ({} as Partial<EmitResponse>);
        const out: EmitResponse = { files: r.files ?? [], warnings: r.warnings ?? [] };
        if (r.errors && r.errors.length > 0) out.errors = r.errors;
        // select は常に四つの列を持つ形で書く（空でも []。docs/plugin-protocol.md「emit」の例）。
        out.select = {
          roots: r.select?.roots ?? [],
          stops: r.select?.stops ?? [],
          states: r.select?.states ?? [],
          wraps: r.select?.wraps ?? [],
        } satisfies Select;
        resp = out;
        break;
      }
      case "check": {
        if (!handler.check) return unsupported();
        const r = (await handler.check(head as CheckRequest)) ?? ({} as Partial<CheckResponse>);
        resp = { diagnostics: r.diagnostics ?? [] } satisfies CheckResponse;
        break;
      }
      case "import": {
        if (!handler.import) return unsupported();
        const r = (await handler.import(head as ImportRequest)) ?? ({} as Partial<ImportResponse>);
        resp = { ir: r.ir ?? null, warnings: r.warnings ?? [] };
        break;
      }
      default:
        return unsupported();
    }
  } catch (err) {
    if (err instanceof UnsupportedDirectionError) {
      // 既定の文でなければ、その文（どのトランスパイラを知らないか、など）を伝える
      if (err.message !== "" && err.message !== "direction not supported") return refuse(stdout, stderr, "unsupported", err.message);
      return unsupported();
    }
    return fail(stderr, `${direction}: ${message(err)}`);
  }
  try {
    stdout.write(JSON.stringify(resp) + "\n");
  } catch (err) {
    return fail(stderr, `write response: ${message(err)}`);
  }
  return 0;
}

/**
 * FQN で宣言を引く。なければ undefined（Go の ir.Program.Lookup と同じ。docs/packages.md「`ir`（公開）」）。
 * 戻り値は kind で分ける（`decl.kind === "op"` など）。
 */
export function lookup(program: Program, fqn: string): Decl | undefined {
  for (const ns of program.namespaces) {
    const decl = ns.decls.find((d) => d.name === fqn);
    if (decl !== undefined) return decl;
  }
  return undefined;
}

/** FQN の名前空間を引く。グローバルは ""。なければ undefined（Go の ir.Program.Namespace と同じ）。 */
export function namespace(program: Program, fqn: string): Namespace | undefined {
  return program.namespaces.find((n) => n.name === fqn);
}

/**
 * op の FQN から、view.viewer から見た関数の型（docs/ir.md「その名前空間から見た IR」の ViewedSignature）を引く。
 * なければ undefined（Go の ir.View.Signature と同じ）。
 */
export function signature(view: View, op: string): ViewedSignature | undefined {
  return Object.hasOwn(view.ops, op) ? view.ops[op] : undefined;
}

/**
 * view.viewer の名前空間の op のうち、単位の対象（span.file が view.files にある。docs/config.md「in」）で、
 * デコレータ decorator（FQN）が付いたものの FQN を宣言の順で返す。名前付きデコレータを通して付いたもの
 * （Application の via）も数える。デコレータで起点を選ぶトランスパイラのための道具で、返す op はどれも select に書ける
 * （docs/plugin-protocol.md「選択」。Go の plugin.OpsWith と同じ）。
 */
export function opsWith(view: View, decorator: string): string[] {
  const out: string[] = [];
  const files = new Set(view.files);
  const ns = namespace(view.program, view.viewer);
  for (const decl of ns?.decls ?? []) {
    if (decl.kind !== "op" || !files.has(decl.span.file)) continue;
    if (decl.decorators.some((a) => a.decorator === decorator || a.via === decorator)) out.push(decl.name);
  }
  return out;
}

/**
 * FQN fqn が view.viewer の名前空間の宣言で、この View の単位の対象か（docs/config.md「in」）: 宣言の span.file が
 * view.files にある。viewer の宣言でなければ false（Go の ir.View.InUnit と同じ）。
 */
export function inUnit(view: View, fqn: string): boolean {
  const ns = namespace(view.program, view.viewer);
  const decl = ns?.decls.find((d) => d.name === fqn);
  return decl !== undefined && view.files.includes(decl.span.file);
}

function refuse(stdout: Sink, stderr: Sink, code: PluginError["error"]["code"], msg: string): number {
  const body: PluginError = { error: { code, message: msg } };
  try {
    stdout.write(JSON.stringify(body) + "\n");
  } catch (err) {
    return fail(stderr, `write response: ${message(err)}`);
  }
  return 2;
}

function fail(stderr: Sink, msg: string): number {
  stderr.write(`error: ${msg}\n`);
  return 1;
}

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

async function readAll(stream: Readable): Promise<string> {
  const chunks: Buffer[] = [];
  for await (const chunk of stream) {
    chunks.push(typeof chunk === "string" ? Buffer.from(chunk) : (chunk as Buffer));
  }
  return Buffer.concat(chunks).toString("utf8");
}
