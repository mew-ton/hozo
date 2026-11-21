// Hozo のプラグインを TypeScript（Node）で書くための SDK。
// プラグインは標準入出力で JSON をやり取りする実行ファイル（docs/plugin-protocol.md「起動」）。
// main / run は Go の plugin.Main / plugin.Run と同じ振る舞い（docs/packages.md「`plugin`（公開）」）。
import type { Readable } from "node:stream";
import {
  PROTOCOL,
  UnsupportedDirectionError,
  type CheckRequest,
  type CheckResponse,
  type CoverageRequest,
  type CoverageResponse,
  type EmitRequest,
  type EmitResponse,
  type Handler,
  type ImportRequest,
  type ImportResponse,
  type PluginError,
} from "./protocol.js";
import type { Call, Decl, EffectRef, Namespace, Op, Program, ReadRef, View, ViewedSignature } from "./ir.js";

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
      case "coverage": {
        if (!handler.coverage) return unsupported();
        const r = (await handler.coverage(head as CoverageRequest)) ?? ({} as Partial<CoverageResponse>);
        resp = {
          recognized: r.recognized ?? false,
          files: (r.files ?? []).map((f) => ({ path: f.path, lines: f.lines ?? [] })),
        } satisfies CoverageResponse;
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
 * 二つの要素（reads か effects の要素。docs/ir.md「op」の ReadRef / EffectRef）が同じか。TS の値は === で比べられないので、
 * これで見る（Go では == で比べる。docs/packages.md「`ir`（公開）」）。
 */
export function sameRef(a: ReadRef | EffectRef, b: ReadRef | EffectRef): boolean {
  return refKey(a) === refKey(b);
}

function refKey(r: ReadRef | EffectRef): string {
  if (r.kind === "named") return "read" in r ? `named read ${r.read}` : `named effect ${r.effect}`;
  return "operation" in r ? `direct effect ${r.state} ${r.operation}` : `direct read ${r.state}`;
}

function mergeUnique<T extends ReadRef | EffectRef>(a: T[], b: T[]): T[] {
  const out: T[] = [];
  const keys = new Set<string>();
  for (const r of [...a, ...b]) {
    const key = refKey(r);
    if (keys.has(key)) continue;
    keys.add(key);
    out.push(r);
  }
  return out;
}

/**
 * 葉の op が直に読むもの: 書いた reads の後ろに、デコレータが運ぶ carried.reads を並べ、重複を最初の出現に寄せたもの
 * （docs/check.md「デコレータが運ぶ reads と effects」。Go の ir.Op.DirectReads と同じ）。呼び出し先から伝わるものは含まない。
 */
export function directReads(op: Op): ReadRef[] {
  return mergeUnique(op.reads, op.carried.reads);
}

/**
 * 葉の op が直に持つ効果: 書いた effects の後ろに、デコレータが運ぶ carried.effects を並べ、重複を最初の出現に寄せたもの
 * （同上。Go の ir.Op.DirectEffects と同じ）。呼び出し先から伝わるものと、デコレータの halts の効果は含まない。
 */
export function directEffects(op: Op): EffectRef[] {
  return mergeUnique(op.effects, op.carried.effects);
}

/**
 * 呼び出し先が op なら、その FQN。read か効果の呼び出しなら undefined（docs/ir.md「配線」の Callee。Go の ir.Call.CalledOp に当たる）。
 */
export function calledOp(call: Call): string | undefined {
  return call.callee.kind === "op" ? call.callee.op : undefined;
}

/**
 * reads の要素が読む基底の状態の FQN: named なら read の宣言の state、direct ならその state。宣言がなければ undefined
 * （Go の ir.Program.ReadState に当たる）。
 */
export function readState(program: Program, ref: ReadRef): string | undefined {
  if (ref.kind === "direct") return ref.state;
  const decl = lookup(program, ref.read);
  return decl?.kind === "read" ? decl.state : undefined;
}

/**
 * 要素ごとの readState を同じ順に並べ、重複を最初の出現に寄せたもの（宣言のない要素は飛ばす）。op が直に読む状態は
 * readStates(program, directReads(op))（Go の ir.Program.ReadStates と同じ）。
 */
export function readStates(program: Program, refs: ReadRef[]): string[] {
  const out = new Set<string>();
  for (const ref of refs) {
    const state = readState(program, ref);
    if (state !== undefined) out.add(state);
  }
  return [...out];
}

/**
 * 効果が書き込む基底の状態の FQN: named なら effect の宣言の writes、direct ならその state。宣言がなければ undefined
 * （Go の ir.Program.EffectWrites に当たる）。
 */
export function effectWrites(program: Program, ref: EffectRef): string[] | undefined {
  if (ref.kind === "direct") return [ref.state];
  const decl = lookup(program, ref.effect);
  return decl?.kind === "effect" ? decl.writes : undefined;
}

/**
 * view.viewer の名前空間の op のうち、単位の対象（span.file が view.files にある。docs/config.md「in」）で、
 * デコレータ decorator（FQN）が付いたものの FQN を宣言の順で返す。名前付きデコレータを通して付いたもの
 * （Application の via）も数える。相乗りが自分の対象を探すための道具（docs/plugin-protocol.md「選択をなくしたこと」。
 * Go の plugin.OpsWith と同じ）。
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
 * 起動されたトランスパイラへのオプション request.view.config.options[request.transpiler]（docs/config.md「オプション」、
 * docs/plugin-protocol.md「emit」）。書かれていなければ空のテーブル。キーと値の型を確かめるのは呼ぶ側で、知らないキーと
 * 型の違う値は、emit の応答の errors で返す（Go の plugin.Options に当たる）。
 */
export function options(request: EmitRequest): Record<string, unknown> {
  const all = request.view?.config?.options ?? {};
  return Object.hasOwn(all, request.transpiler) ? (all[request.transpiler] ?? {}) : {};
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
