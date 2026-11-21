// プラグインのプロトコルの要求と応答（docs/plugin-protocol.md「要求と応答」）。
// 形は Go の plugin パッケージ（docs/packages.md「`plugin`（公開）」）と同じで、
// schema/ir.schema.json の $defs（EmitRequest など）がそれを記述する。このファイルはその $defs を手で写したもので、
// 写しが合っていること（項目の名前、必須かどうか、型）は test/protocol.test.ts が確かめる（docs/packages.md「生成するもの」）。
import type { Program, View } from "./ir.js";

/** プロトコルの版。docs/ir.md の version（view.version）と同じ値。 */
export const PROTOCOL = "0.18" as const;

export type Direction = "emit" | "check" | "import" | "coverage";

/**
 * 書き出す（または取り込む）ファイル。path はプロジェクトのルートからの相対パス。emit の応答では、その起動の出力先
 * （req.view.config.dir。Dir(U, N)）の下でなければならず、そうでないパス（`..`、絶対パス、別のディレクトリ）は PLG-1
 * （docs/plugin-protocol.md「emit」）。content は UTF-8 の文字列。
 */
export interface File {
  path: string;
  content: string;
}

/** warning の診断。at は FQN か "file:line:col"。 */
export interface Warning {
  at: string;
  message: string;
}

export type Severity = "error" | "warning";

/** check の diagnostics の要素。 */
export interface Diagnostic {
  severity: Severity;
  code: string;
  at: string;
  message: string;
}

/** emit の errors の要素。常に error なので severity は省ける。 */
export interface EmitError {
  severity?: Severity;
  code: string;
  at: string;
  message: string;
}

export interface EmitRequest {
  protocol: typeof PROTOCOL;
  direction: "emit";
  /**
   * 起動されたトランスパイラの名前（マニフェストの [[transpiler]] の name）。複数のトランスパイラを提供する
   * プラグインは、この名前で処理を分ける。知らない名前なら UnsupportedDirectionError を投げる。
   */
  transpiler: string;
  /**
   * 対象の名前空間は view.viewer。その単位での設定は view.config（オプションは view.config.options。index の options(req) で読む）、
   * 単位の対象のファイルは view.files。
   */
  view: View;
}

/**
 * emit の応答。何を出すかを hozoc に伝える項目（版 0.3 までの select）はない（docs/plugin-protocol.md「選択をなくしたこと」）。
 */
export interface EmitResponse {
  files: File[];
  warnings: Warning[];
  /** 一つでもあれば hozoc は files を書き出さない。 */
  errors?: EmitError[];
}

export interface CheckRequest {
  protocol: typeof PROTOCOL;
  direction: "check";
  view: View;
}

export interface CheckResponse {
  diagnostics: Diagnostic[];
}

export interface ImportRequest {
  protocol: typeof PROTOCOL;
  direction: "import";
  /** マニフェストの [import].formats の一つ。 */
  format: string;
  files: File[];
}

export interface ImportResponse {
  ir: Program;
  warnings: Warning[];
}

/** 行の範囲: 始まりの行と、終わりの行（1 始まりで、両端を含む）。 */
export type LineRange = [number, number];

/** coverage の要求（docs/plugin-protocol.md「coverage」）。 */
export interface CoverageRequest {
  protocol: typeof PROTOCOL;
  direction: "coverage";
  /** hozoc tests に渡された、カバレッジのファイル（path は絶対パス）。 */
  coverage: File;
  /** 実行された行を知りたいファイル（プロジェクトのルートからの相対パス）。 */
  files: string[];
}

/** coverage の応答。recognized が false なら、このプラグインが読める形のファイルではなかった。 */
export interface CoverageResponse {
  recognized: boolean;
  /** 要求の files のうち、カバレッジが測っていたファイルごとに一つ。 */
  files: CoveredFile[];
}

/** 測られていたファイル一つの、実行された所の行の範囲。一度も実行されていなければ lines は空。 */
export interface CoveredFile {
  path: string;
  lines: LineRange[];
}

/**
 * テストの対応表（docs/plugin-protocol.md「テストの対応表」）: テストを受け取る口を出すトランスパイラが、emit の応答の files に
 * 足すファイル hozo-tests.<トランスパイラの名前>.json の中身。
 */
export interface TestMap {
  tests: TestEntry[];
}

/** 対応表のテスト一つ。file は、そのテストの行を持つファイル（プロジェクトのルートからの相対パス）。 */
export interface TestEntry {
  name: string;
  file: string;
  lines: LineRange;
}

/** トランスパイラ transpiler の対応表のファイルの名前（Go の plugin.TestMapName）。 */
export function testMapName(transpiler: string): string {
  return `hozo-tests.${transpiler}.json`;
}

export type Request = EmitRequest | CheckRequest | ImportRequest | CoverageRequest;
export type Response = EmitResponse | CheckResponse | ImportResponse | CoverageResponse;

/** 終了コード 2 のときの標準出力（docs/plugin-protocol.md「起動」）。 */
export interface PluginError {
  error: {
    code: "protocol" | "unsupported";
    message: string;
  };
}

type MaybePromise<T> = T | Promise<T>;

/**
 * プラグインが実装する四つの方向。実装しない方向はメソッドを持たないか、
 * UnsupportedDirectionError を投げる。emit で知らないトランスパイラ（req.transpiler）のときも同じものを投げる。
 * どれも code "unsupported"（終了コード 2）になる。
 */
export interface Handler {
  emit?(req: EmitRequest): MaybePromise<EmitResponse>;
  check?(req: CheckRequest): MaybePromise<CheckResponse>;
  import?(req: ImportRequest): MaybePromise<ImportResponse>;
  coverage?(req: CoverageRequest): MaybePromise<CoverageResponse>;
}

/** Handler がその direction か、要求のトランスパイラを実装していないことを表す（Go の plugin.ErrUnsupported）。 */
export class UnsupportedDirectionError extends Error {
  constructor(message = "direction not supported") {
    super(message);
    this.name = "UnsupportedDirectionError";
  }
}
