# hozoc

## この文書が決めること

`hozoc` のサブコマンドの振る舞いと、hozoc 自身の仕様 `cli.hozo`（`docs/requirements.md`「自己展開」）。骨組み（引数の解釈、表示、終了コード）は `cli.hozo` から `go-cli`（`docs/plugin-go-cli.md`）が生成する。ここで決めるのは葉の op の中身。

## cli.hozo

設計書の `examples/hozoc.hozo` を元に、次を変えたもの。変えた理由は `docs/decisions.md`。

- `cli` / `fs` / `proc` の語彙は標準の語彙（`docs/vocab.md`）になったので宣言しない。
- `lsp` を足す。
- `emit` を配線にし、`check` を呼んでから `writeOutputs` を呼ぶ。
- `outputs` の状態の `@fs.files("{out}/**")` は、置き場が設定で決まるので `@fs.files` を付けず説明に書く。
- `runEmit` の `@proc.stdio("{plugin.command}")` は、呼び先を引数 `plugin` で渡す形にし、`proc.stdio` は引数を持たない（`docs/vocab.md`「同梱する語彙」）。`runImport` は段階 6 で足す。
- 出力の形式を選ぶ `format` のフラグと、知らない名前空間を指したときの `UnknownNamespace` を足す。
- プラグインとやり取りする model（`EmitRequest` / `EmitResponse`）は `docs/plugin-protocol.md`「要求と応答」の JSON をそのまま写す。

```ts
/** hozo の仕様を検査し、各言語のコードに展開する CLI */
namespace hozoc {
  /** ファイルのパス。相対パスの基準は使う場所が決める（docs/config.md「置き場と探し方」など） */
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
    file: Path?;
    start: Position?;
    end: Position?;
    related: RelatedInfo[];
    /** 機械が読む補足（docs/check.md「Data」）。json にだけ出す */
    data: Record<string, string[]>?;
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
  state config: nullable<SourceFile>;

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

  /** emit の引数に、hozo.toml にない名前空間が指定された */
  @cli.exitCode(64)
  exception UnknownNamespace;

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
    @cli.flag("config") @cli.env("HOZO_CONFIG") config: Path?;
    /** 表示の形式 */
    @cli.flag("format") format: OutputFormat?;
  }

  /** emit の入力 */
  model EmitInput {
    /** 設定ファイルの場所 */
    @cli.flag("config") @cli.env("HOZO_CONFIG") config: Path?;
    /** 表示の形式 */
    @cli.flag("format") format: OutputFormat?;
    /** 生成する名前空間（一つもなければすべて） */
    @cli.arg namespaces: string[];
    /** 書き出さずに、書き出す予定のファイルだけを表示する */
    @cli.flag("dry-run") dryRun: boolean?;
  }

  /** lsp の入力 */
  model LspInput {
    /** 設定ファイルの場所 */
    @cli.flag("config") @cli.env("HOZO_CONFIG") config: Path?;
    /** ログの書き出し先 */
    @cli.flag("log") @cli.env("HOZO_LSP_LOG") log: Path?;
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
   * @throws LanguageMismatch プラグインのマニフェストの language が名前空間の language と合わない（docs/config.md の CFG-4）
   * @throws proc.Crashed check を持つプラグインが異常終了した
   * @see emit
   */
  @cli.command("check")
  op check(in: CheckInput): CheckResult
    throws CheckFailed | ConfigInvalid | LanguageMismatch
    throws proc.Crashed | proc.MalformedOutput;

  /**
   * 検査を通った仕様から、名前空間ごとに既定の出力とプラグインの出力を作り、書き出す。
   *
   * @param in コマンドの入力
   * @param checked 検査の結果
   * @throws CheckFailed プラグインが errors を返した、または既定の出力が作れない（GOEMIT-*）。ファイルは書き出さない
   * @throws OutputConflict 同じファイルに二つの出どころが書こうとした。ファイルは書き出さない
   * @throws UnknownNamespace 引数の名前空間が hozo.toml にない。ファイルは書き出さない
   * @throws proc.Crashed プラグインが異常終了した。書き出しは始めていない
   */
  op writeOutputs(in: EmitInput, checked: CheckResult): EmitResult
    effects addOutputs | replaceOutputs | removeStaleOutputs
    throws CheckFailed | OutputConflict | UnknownNamespace prevents effects
    throws proc.Crashed | proc.MalformedOutput prevents effects;

  /**
   * 仕様を検査し、名前空間ごとに設定されたプラグインを呼んで、生成したファイルを書き出す。
   *
   * @param in コマンドの入力
   * @see check
   */
  @cli.command("emit")
  op emit(in: EmitInput): EmitResult
    throws CheckFailed | ConfigInvalid | LanguageMismatch | OutputConflict | UnknownNamespace prevents effects
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

  /** プラグインの起動のしかた（docs/plugin-protocol.md「起動」）。root は作業ディレクトリにするプロジェクトのルート、dir はマニフェストのディレクトリ */
  model Plugin { name: string; command: string[]; dir: Path; root: Path; }

  /** IR（docs/ir.md「その名前空間から見た IR」）。中身はこの仕様では書かない */
  model View {}

  /** 展開のときのプラグインへの入力（docs/plugin-protocol.md「emit」の要求） */
  model EmitRequest { protocol: string; direction: string; view: View; }

  /** プラグインの warning（同「emit」の応答の warnings の要素） */
  model PluginWarning { at: string; message: string; }

  /** プラグインの error（同「emit」の応答の errors の要素） */
  model PluginError { code: string; at: string; message: string; }

  /** 展開のときのプラグインからの出力（同「emit」の応答） */
  model EmitResponse { files: OutputFile[]; warnings: PluginWarning[]; errors: PluginError[]; }

  /**
   * プラグインを一つ起動して展開させる（docs/plugin-protocol.md「起動」）。
   *
   * @param plugin 起動するプラグイン
   * @param req プラグインへの入力
   */
  @proc.stdio
  op runEmit(plugin: Plugin, req: EmitRequest): EmitResponse;
}
```

`hozo.toml`:

```toml
[hozoc]
language = "go"
plugins  = ["go-cli"]
out      = "internal/gen/hozoc"

[hozo]
sources     = ["cli.hozo"]      # vocab/ と tests/ の .hozo を読まないため
plugin_dirs = ["plugins"]
```

`writeOutputs` の `throws proc.Crashed | proc.MalformedOutput prevents effects` は、プラグインをすべて呼び終えて衝突を確かめてから書き始めるため（下の「emit」）。

## check

1. `hozo.toml` を読み、プラグインのマニフェストを探して読む（`docs/config.md`「置き場と探し方」、`docs/plugin-protocol.md`「探索」）。設定とマニフェストの error は、CFG-4 なら `LanguageMismatch`、それ以外は `ConfigInvalid`（`docs/config.md`「診断」、`docs/plugin-protocol.md`「診断」）。
2. 標準の語彙、設定されたプラグインの語彙、`sources` のファイルの順に読み、解析する（`docs/grammar.md`）。
3. 解決（`docs/resolve.md`）、正規化（`docs/ir.md`）、計算と検査（`docs/check.md`）を行う。
4. `[check] enabled` のプラグインに `direction: "check"` を送り、返った診断を足す。プラグインの異常終了は `proc.Crashed` / `proc.MalformedOutput`。
5. error が一つでもあれば例外（値は error と warning のすべて）。設定かマニフェストの error（CFG-*、MAN-*）が一つでもあれば `ConfigInvalid`、ただしそれがすべて CFG-4 なら `LanguageMismatch`。それ以外は `CheckFailed`。error がなければ `CheckResult`（warning）。

構文エラーのあるファイルも、読めた宣言は解決・検査に進める。診断の順序は `docs/diagnostics.md`。

## emit

`check` の後、`writeOutputs`:

1. `namespaces` が空でなければ、その名前空間（と子孫）だけを対象にする。`hozo.toml` にない名前なら `UnknownNamespace`。
2. 対象の各名前空間 N について、Conf(N) を解決し、既定の出力（`docs/emit-go.md`）を作る。GOEMIT-* があれば `CheckFailed`。
3. Conf(N).plugins の各プラグインを `runEmit` で起動し（順序は `docs/plugin-protocol.md`「起動」）、`files` を集める。`errors` があれば `CheckFailed`。
4. すべての名前空間のファイルを集めてから、パスの重複を確かめる（CFG-5 → `OutputConflict`）。
5. `dryRun` なら、書く予定のパスを `written` に入れて返す（書かない）。
6. 対象の各名前空間の `<Dir(N)>/.hozoc-outputs.json`（前回書いたパスの一覧。ルートからの相対パスの JSON の配列、辞書順）を読み、今回書かないパスのファイルを消す（`removeStaleOutputs`）。ファイルがなければ何もしない。`hozo.toml` から名前空間のテーブルを消した場合、その出力先はもう分からないので消さない（手で消す）。
7. ファイルを書く。内容が同じなら書き直さない（`written` には入れる）。ディレクトリは作る。
8. `.hozoc-outputs.json` を書く。

`EmitResult.warnings` は `check` の warning に、既定の出力とプラグインの warning を足したもの。

書く順序はパスの辞書順。途中で失敗したら、そこまでに書いたものは残る（原子性は持たない。`proc.Crashed` は 3 で起きるので、書き出しには入っていない）。

## lsp

`docs/lsp.md`。`config` が与えられなければ、クライアントの `rootUri` から探す。

## 手で書くもの

- `cmd/hozoc/main.go`: `internal/app` の `Leaves` を `hozoc.New` で `Ops` にし、`Presenter` と一緒に `cli.Main`（`docs/plugin-go-cli.md`「出力」）に渡す。それだけ。骨組み（引数の解釈、例外の見分けと終了コード、使い方）は `cli.hozo` から生成した `internal/gen/hozoc`（`docs/emit-go.md`）と `internal/gen/hozoc/cli`（go-cli）が持ち、変えるときは `cli.hozo` を変えて `make generate`（`go run ./cmd/hozoc emit`）。生成物はコミットし、`tests/generated_test.go` と CI が遅れていないことを確かめる（`docs/testing.md`「生成物の一致」）。
- `internal/app`: 生成した `hozoc.Leaves`（`docs/emit-go.md`「op」）の実装。`check` は `internal/project.Load`、`writeOutputs` は `internal/emit/golang` と `runEmit`、`runEmit` は `internal/plugin.Runner`、`lsp` は `internal/lsp.Serve`（`docs/packages.md`）。境界の型は生成したもの（`hozoc.CheckInput`、`hozoc.Diagnostic`。中の `internal/diag.Diagnostic` は行と列に直して写す）で、例外は生成した exception の型（`hozoc.CheckFailed{Value: hozoc.Diagnostics{...}}` など。`proc.Crashed` / `proc.MalformedOutput` は hozoc のパッケージに複製された `hozoc.Crashed` / `hozoc.MalformedOutput`。`docs/emit-go.md`「配置」）。`cli.hozo` の model は境界だけ（`docs/requirements.md`「自己展開」）なので、op をまたいで要る中身（`check` が解析した `Project`、`runEmit` に渡す IR。`View` は中身を持たない）は `Leaves` の中に持つ。`Wrappers` はない（`@cli.command` は go-cli の `entry`、`@proc.stdio` は `stop`。`docs/emit-go.md`「デコレータ」）。
- `Presenter`（`cli.Presenter` の実装）: 入力（`CheckInput` / `EmitInput`）の `format` に応じて text か JSON（`docs/diagnostics.md`「表示」）。結果は標準出力、例外は標準エラー（`docs/plugin-go-cli.md`「実行と表示」）。text の `CheckResult` は診断の一覧と件数、`EmitResult` は warning の一覧（あれば）に続けて `written: <path>` / `removed: <path>` を一行ずつ、診断を運ぶ例外はその一覧と件数、それ以外の例外は `hozoc: <Failure.Err のメッセージ>`。json の `CheckResult` は `{"warnings": [...]}`、`EmitResult` は `{"warnings": [...], "written": [...], "removed": [...]}`、例外は `{"error": "<FQN>", "value": {...}}` で、診断を運ぶ例外の `value` は `docs/diagnostics.md`「json」の形（`{"diagnostics", "errors", "warnings"}`）、それ以外は `cli.JSONPresenter` と同じ。診断の要素は `docs/diagnostics.md`「json」と同じ形。
- `hozo.toml` がなければ、`check` は CFG-7 を一つ作って `ConfigInvalid` にする（`internal/project` は設定なしでも動くが、CLI は設定を要求する）。
- `lsp` は段階 4 まで未実装で、宣言にない例外として 70 で終わる（`docs/diagnostics.md`「終了コード」）。
