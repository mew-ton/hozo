# hozoc

## この文書が決めること

`hozoc` のサブコマンドの振る舞いと、hozoc 自身の仕様 `cli.hozo`（`docs/requirements.md`「自己展開」）。骨組み（引数の解釈、表示、終了コード）は `cli.hozo` から `go-cli`（`docs/plugin-go-cli.md`）が生成する。ここで決めるのは葉の op の中身。

## cli.hozo

設計書の `examples/hozoc.hozo` を元に、次を変えたもの。変えた理由は `docs/decisions.md`。

- `cli` / `fs` / `proc` の語彙は標準の語彙（`docs/vocab.md`）になったので宣言しない。
- `lsp` を足す。
- `emit` を配線にし、`check` を呼んでから `writeOutputs` を呼ぶ。
- `outputs` の状態の `@fs.files("{out}/**")` は、置き場が設定で決まるので `@fs.files` を付けず説明に書く。
- `runEmit` / `runImport` の `@proc.stdio("{plugin.command}")` は、呼び先を入力の `plugin` で渡す形にし、`proc.stdio` は引数を持たない。
- 出力の形式を選ぶ `format` のフラグを足す。

```ts
/** hozo の仕様を検査し、各言語のコードに展開する CLI */
namespace hozoc {
  /** ファイルのパス（プロジェクトのルートからの相対パス） */
  scalar Path extends string;

  /** 表示の形式。text か json */
  @valid.pattern("^(text|json)$")
  scalar OutputFormat extends string;

  /** 読み込むファイル */
  model SourceFile { path: Path; content: string; }

  /** 書き出すファイル */
  model OutputFile { path: Path; content: string; }

  /** 位置（1 始まりの行と列） */
  model Position { line: int; col: int; }

  /** 検査やプラグインが報告する一件の指摘（docs/diagnostics.md） */
  model Diagnostic {
    /** error か warning */
    severity: string;
    /** 規則の ID */
    code: string;
    message: string;
    file?: Path;
    start?: Position;
    end?: Position;
    related: RelatedInfo[];
  }

  /** 指摘に関連する位置 */
  model RelatedInfo { file: Path; start: Position; end: Position; message: string; }

  /** 指摘の一覧 */
  model Diagnostics { items: Diagnostic[]; }

  /** 仕様のファイル */
  @fs.files("**/*.hozo")
  state sources: list<SourceFile>;

  /** プロジェクトの設定（docs/config.md） */
  @fs.files("hozo.toml")
  state config: single<SourceFile?>;

  /** 生成したファイル。置き場は hozo.toml の各名前空間の out */
  state outputs: list<OutputFile>;

  /** 仕様に構文か型の誤りがある */
  @cli.exitCode(1)
  exception CheckFailed: Diagnostics;

  /** hozo.toml がない、または設定が誤っている（docs/config.md の CFG-*） */
  @cli.exitCode(2)
  exception ConfigInvalid: Diagnostics;

  /** プラグインの language が名前空間の language と合わない */
  @cli.exitCode(2)
  exception LanguageMismatch: Diagnostics;

  /** 複数の出どころが同じファイルに書こうとした */
  @cli.exitCode(3)
  exception OutputConflict: Diagnostics;

  /** 検査の結果（誤りではない指摘） */
  model CheckResult { warnings: Diagnostic[]; }

  /** 書き出しの結果 */
  model EmitResult {
    warnings: Diagnostic[];
    /** 書いたファイル（新規と更新） */
    written: Path[];
    /** 消したファイル */
    removed: Path[];
  }

  /** check の入力 */
  model CheckInput {
    /** 設定ファイルの場所 */
    @cli.flag("config") @cli.env("HOZO_CONFIG") config?: Path;
    /** 表示の形式 */
    @cli.flag("format") format?: OutputFormat;
  }

  /** emit の入力 */
  model EmitInput {
    /** 設定ファイルの場所 */
    @cli.flag("config") @cli.env("HOZO_CONFIG") config?: Path;
    /** 表示の形式 */
    @cli.flag("format") format?: OutputFormat;
    /** 生成する名前空間（省略時はすべて） */
    @cli.arg namespaces?: string[];
    /** 書き出さずに、書き出す予定のファイルだけを表示する */
    @cli.flag("dry-run") dryRun?: boolean;
  }

  /** lsp の入力 */
  model LspInput {
    /** 設定ファイルの場所 */
    @cli.flag("config") @cli.env("HOZO_CONFIG") config?: Path;
    /** ログの書き出し先 */
    @cli.flag("log") @cli.env("HOZO_LSP_LOG") log?: Path;
  }

  /** 新しく生成したファイルを書き出す */
  effect addOutputs on outputs.post;
  /** 既にあるファイルを、生成し直した内容で書き換える */
  effect replaceOutputs on outputs.put;
  /** 生成しなくなったファイルを消す */
  effect removeStaleOutputs on outputs.delete;

  /**
   * 仕様を検査する。ファイルは書き出さない。
   *
   * @param in コマンドの入力
   * @see emit
   */
  @cli.command("check")
  op check(in: CheckInput): CheckResult
    throws CheckFailed | ConfigInvalid;

  /**
   * 検査を通った仕様から、名前空間ごとに既定の出力とプラグインの出力を作り、書き出す。
   *
   * @param in コマンドの入力
   * @param checked 検査の結果
   * @throws LanguageMismatch プラグインの language が合わない。ファイルは書き出さない
   * @throws OutputConflict 同じファイルに二つの出どころが書こうとした。ファイルは書き出さない
   * @throws proc.Crashed プラグインが異常終了した。書き出しは始めていない
   */
  op writeOutputs(in: EmitInput, checked: CheckResult): EmitResult
    effects addOutputs | replaceOutputs | removeStaleOutputs
    throws LanguageMismatch | OutputConflict prevents effects
    throws proc.Crashed | proc.MalformedOutput prevents effects;

  /**
   * 仕様を検査し、名前空間ごとに設定されたプラグインを呼んで、生成したファイルを書き出す。
   *
   * @param in コマンドの入力
   * @see check
   */
  @cli.command("emit")
  op emit(in: EmitInput): EmitResult
    throws CheckFailed | ConfigInvalid | LanguageMismatch | OutputConflict prevents effects
    throws proc.Crashed | proc.MalformedOutput prevents effects
  {
    const checked = check({ config: in.config, format: in.format });
    return writeOutputs(in, checked);
  }

  /**
   * 言語サーバーとして標準入出力で動く（docs/lsp.md）。クライアントが exit を送るまで戻らない。
   *
   * @param in コマンドの入力
   */
  @cli.command("lsp")
  op lsp(in: LspInput);

  /** プラグインの起動のしかた（docs/plugin-protocol.md「マニフェスト」の command） */
  model Plugin { name: string; command: string[]; dir: Path; }

  /** IR（docs/ir.md）。中身はこの仕様では書かない */
  model View {}

  /** 展開のときのプラグインへの入力 */
  model EmitRequest { plugin: Plugin; namespace: string; view: View; }

  /** 展開のときのプラグインからの出力 */
  model EmitResponse { files: OutputFile[]; warnings: Diagnostic[]; errors: Diagnostic[]; }

  /**
   * プラグインを一つ呼んで展開させる。
   *
   * @param req プラグインへの入力
   */
  @proc.stdio
  op runEmit(req: EmitRequest): EmitResponse;
}
```

`hozo.toml`:

```toml
[hozoc]
language = "go"
plugins  = ["go-cli"]
out      = "internal/gen/hozoc"

[hozo]
plugin_dirs = ["plugins"]
```

`proc.stdio` の `writeOutputs` が `prevents effects` なのは、プラグインをすべて呼び終えて衝突を確かめてから書き始めるため（下の「emit」）。

## check

1. `hozo.toml` を読む（`docs/config.md`「置き場と探し方」）。なければ、または CFG-* の error があれば `ConfigInvalid`（CFG-4 は `LanguageMismatch`）。
2. 標準の語彙、設定されたプラグインの語彙、`sources` のファイルの順に読み、解析する（`docs/grammar.md`）。
3. 解決（`docs/resolve.md`）、正規化（`docs/ir.md`）、計算と検査（`docs/check.md`）を行う。
4. `[check] enabled` のプラグインに `direction: "check"` を送り、返った診断を足す。
5. error が一つでもあれば `CheckFailed`（値は error と warning のすべて）。なければ `CheckResult`（warning）。

構文エラーのあるファイルも、読めた宣言は解決・検査に進める。診断の順序は `docs/diagnostics.md`。

## emit

`check` の後、`writeOutputs`:

1. `namespaces` が与えられていれば、その名前空間（と子孫）だけを対象にする。知らない名前なら使い方の誤り（64）。
2. 対象の各名前空間 N について、Conf(N) を解決し、既定の出力（`docs/emit-go.md`）を作る。
3. Conf(N).plugins の各プラグインを `docs/plugin-protocol.md` の順で起動し、`files` を集める。`errors` があれば `CheckFailed`。
4. すべての名前空間のファイルを集めてから、パスの重複を確かめる（CFG-5 → `OutputConflict`）。
5. `dryRun` なら、書く予定のパスを `written` に入れて返す（書かない）。
6. 各名前空間の `<out>/.hozoc-outputs.json`（前回書いたパスの一覧。JSON の文字列の配列、辞書順）を読み、今回書かないパスのファイルを消す（`removeStaleOutputs`）。ファイルがなければ何もしない。
7. ファイルを書く。内容が同じなら書き直さない（`written` には入れる）。ディレクトリは作る。
8. `.hozoc-outputs.json` を書く。

書く順序はパスの辞書順。途中で失敗したら、そこまでに書いたものは残る（原子性は持たない。`proc.Crashed` は 3 で起きるので、書き出しには入っていない）。

## lsp

`docs/lsp.md`。`config` が与えられなければ、クライアントの `rootUri` から探す。

## 手で書くもの

- `cmd/hozoc/main.go`: `internal/app` の実装を組み立てて `cli.Main` を呼ぶ。
- `internal/app`: `hozoc.Leaves` の実装（`check` / `writeOutputs` / `lsp` / `runEmit`）と、`format` に応じて text か JSON で表示する `Presenter`（`docs/diagnostics.md`「表示」）。`text` では `CheckResult` は診断の一覧と件数、`EmitResult` は `written` / `removed` のパスの一覧を出す。
- 段階 3 まで（骨組みを生成する前）は、`cmd/hozoc/main.go` が引数の解釈も手で持つ。
