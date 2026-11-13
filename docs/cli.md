# hozoc

## この文書が決めること

`hozoc` のサブコマンドの振る舞いと、hozoc 自身の仕様 `cli.hozo`（`docs/requirements.md`「自己展開」）。骨組み（引数の解釈、表示、終了コード）は `cli.hozo` から `go-cli`（`docs/plugin-go-cli.md`）が生成する。ここで決めるのは葉の op の中身。

## cli.hozo

hozoc 自身の仕様は、リポジトリ直下の `cli.hozo`。下のコードブロックはその写しで、`hozo.toml` のブロックも同じ（バイト単位で同じであることを `tests/parse_test.go` の `TestDocsEmbedRootFiles` が確かめる）。この文書と `docs/` のほかの文書が、修飾なしに `cli.hozo` と書けばこのファイルを指す。go-cli プラグインの語彙のファイルは `plugins/go-cli/cli.hozo` と書く（`docs/plugin-go-cli.md`「マニフェスト」）。

設計書の `examples/hozoc.hozo` を元に、次を変えたもの。変えた理由はそれぞれの項目に書く（決定の一覧での番号は `docs/decisions.md` の 20 と 36）。

`examples/hozoc.hozo` が「書けなかったもの」として挙げた三つは、次のように解く。

- **状態の宣言から設定の値を参照できない**（`@fs.files("{out}/**")`）: `outputs` の状態には `@fs.files` を付けず、置き場が `hozo.toml` の各単位の `out` で決まることを説明に書く。置き場は設定で決まり、仕様の宣言の時点では決まらないため。
- **`runEmit` を呼ぶ繰り返しを配線で書けない**（配線はループを持たない。§9）: 繰り返しは葉の op `writeOutputs` の中に置く。`emit` そのものは配線にし、`check` を呼んでから `writeOutputs` を呼ぶ。検査を通った結果だけを書き出しに渡すことと、`check` の例外が `emit` に伝わることを、境界の記述のまま検査器に確かめさせるため。`runEmit` はどの配線からも呼ばれないが、プラグインとの境界の記述として残す（「手で書くもの」）。
- **デコレータの引数に実行時の値を書けない**（`@proc.stdio("{plugin.command}")`）: 呼び先を引数 `plugin` で渡し、`proc.stdio` は引数を持たない（理由は `docs/vocab.md`「同梱する語彙」）。`runImport` は段階 6 で足す。

ほかに変えたこと:

- `fs` / `proc` の語彙は標準の語彙（`docs/vocab.md`「同梱する語彙」）に、`cli` の語彙は go-cli プラグインが提供する語彙（同「同梱しない語彙」）になったので、どれも `cli.hozo` では宣言しない。
- `lsp` を足す。言語サーバーを別のバイナリにしない（`docs/requirements.md`「言語サーバー」）ので、hozoc のサブコマンドとして境界に書く。
- `writeOutputs` の三つの効果（追加・書き換え・削除）は、葉の op が持てる効果が一つまで（`docs/check.md` EFF-3）なので、`outputs` を対象にした分類 `OutputsSync` の遷移 `syncOutputs` 一つにする。述語 `UpToDate` は「生成し直して差分がない」で、`docs/testing.md`「生成物の一致」の検査と同じ中身。
- 出力の形式を選ぶ `format` のフラグを足す。診断を機械が読めるようにするため（`docs/diagnostics.md`「json」）。
- 例外 `UnknownNamespace` を足す。`emit` の引数の名前空間の書き間違いを、何も書かずに成功したことにしないため（「emit」の 1）。
- 環境変数 `HOZO_PLUGIN_PATH` を、`check` / `emit` / `lsp` の入力のフィールド `pluginPath` として足す。プラグインの探索が見るプロジェクトの外の場所（`docs/plugin-protocol.md`「探索」の 2）も、ほかの環境変数（`HOZO_CONFIG`）と同じく境界の入力として書き、葉の op の実装がプロセスの環境を自分で読まないようにするため（同じ入力から同じ結果になり、テストが実行する人の環境に左右されない）。
- 設定を単位の一覧にした（`docs/config.md`「単位」）のに合わせて、`EmitRequest` に `transpiler` を足し、`LanguageMismatch` と `UnknownNamespace` の意味を単位の言葉に直す。
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

  /** 生成したファイル。置き場は hozo.toml の各単位の out（docs/config.md「単位の対象」） */
  state outputs: list<OutputFile>;

  /** 仕様に構文か型の誤りがある */
  @cli.exitCode(1)
  exception CheckFailed: Diagnostics;

  /** hozo.toml がない、または設定が誤っている（docs/config.md の CFG-*） */
  @cli.exitCode(2)
  exception ConfigInvalid: Diagnostics;

  /** 一つの単位のトランスパイラの language がそろっていない */
  @cli.exitCode(2)
  exception LanguageMismatch: Diagnostics;

  /** 複数の出どころが同じファイルに書こうとした */
  @cli.exitCode(3)
  exception OutputConflict: Diagnostics;

  /** emit の引数の名前空間も、その子孫も、どの単位の対象でもない */
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
    /** プラグインを探すディレクトリ（OS のパス区切りで複数） */
    @cli.env("HOZO_PLUGIN_PATH") pluginPath: string?;
  }

  /** emit の入力 */
  model EmitInput {
    /** 設定ファイルの場所 */
    @cli.flag("config") @cli.env("HOZO_CONFIG") config: Path?;
    /** 表示の形式 */
    @cli.flag("format") format: OutputFormat?;
    /** 生成する名前空間（一つもなければすべて）。その名前空間と子孫を出力する単位の分を書き出す */
    @cli.arg namespaces: string[];
    /** 書き出さずに、書き出す予定のファイルだけを表示する */
    @cli.flag("dry-run") dryRun: boolean?;
    /** プラグインを探すディレクトリ（OS のパス区切りで複数） */
    @cli.env("HOZO_PLUGIN_PATH") pluginPath: string?;
  }

  /** lsp の入力 */
  model LspInput {
    /** 設定ファイルの場所 */
    @cli.flag("config") @cli.env("HOZO_CONFIG") config: Path?;
    /** ログの書き出し先 */
    @cli.flag("log") @cli.env("HOZO_LSP_LOG") log: Path?;
    /** プラグインを探すディレクトリ（OS のパス区切りで複数） */
    @cli.env("HOZO_PLUGIN_PATH") pluginPath: string?;
  }

  /** 生成したファイルが、今の仕様と設定から生成し直したものと一致しているか */
  classify OutputsSync of outputs {
    /** 一致している */
    UpToDate;
    /** 足りない、古い、または余分なファイルがある */
    default;
  }

  /** 生成したファイルを今の仕様と設定に合わせる（足りないものを書き、古いものを書き換え、余分なものを消す） */
  effect syncOutputs on OutputsSync: any -> UpToDate;

  /**
   * 仕様を検査する。ファイルは書き出さない。
   *
   * @param in コマンドの入力
   * @throws LanguageMismatch 一つの単位のトランスパイラの language がそろっていない（docs/config.md の CFG-16）
   * @throws proc.Crashed check を持つプラグインが異常終了した
   * @see emit
   */
  @cli.command("check")
  op check(in: CheckInput): CheckResult
    throws CheckFailed | ConfigInvalid | LanguageMismatch
    throws proc.Crashed | proc.MalformedOutput;

  /**
   * 検査を通った仕様から、単位ごとに既定の出力とトランスパイラの出力を作り、書き出す。
   *
   * @param in コマンドの入力
   * @param checked 検査の結果
   * @throws CheckFailed トランスパイラが errors を返した、または既定の出力が作れない（GOEMIT-*）。ファイルは書き出さない
   * @throws OutputConflict 同じファイルに二つの出どころが書こうとした。ファイルは書き出さない
   * @throws UnknownNamespace 引数の名前空間も、その子孫も、どの単位の対象でもない。ファイルは書き出さない
   * @throws proc.Crashed プラグインが異常終了した。書き出しは始めていない
   */
  op writeOutputs(in: EmitInput, checked: CheckResult): EmitResult
    effects syncOutputs
    throws CheckFailed | OutputConflict | UnknownNamespace prevents effects
    throws proc.Crashed | proc.MalformedOutput prevents effects;

  /**
   * 仕様を検査し、単位ごとにトランスパイラを呼んで、生成したファイルを書き出す。
   *
   * @param in コマンドの入力
   * @see check
   */
  @cli.command("emit")
  op emit(in: EmitInput): EmitResult
    throws CheckFailed | ConfigInvalid | LanguageMismatch | OutputConflict | UnknownNamespace prevents effects
    throws proc.Crashed | proc.MalformedOutput prevents effects
  {
    const checked = check({ config: in.config, format: in.format, pluginPath: in.pluginPath });
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

  /** 展開のときのプラグインへの入力（docs/plugin-protocol.md「emit」の要求）。transpiler は起動するトランスパイラの名前 */
  model EmitRequest { protocol: string; direction: string; transpiler: string; view: View; }

  /** トランスパイラの warning（同「emit」の応答の warnings の要素） */
  model PluginWarning { at: string; message: string; }

  /** トランスパイラの error（同「emit」の応答の errors の要素） */
  model PluginError { code: string; at: string; message: string; }

  /** トランスパイラが包む関数を実装するデコレータの適用（同「選択」の wraps の要素） */
  model Wrap { op: string; decorator: string; }

  /** トランスパイラが選んだ出力の対象（同「選択」） */
  model Select { roots: string[]; stops: string[]; states: string[]; wraps: Wrap[]; }

  /** 展開のときのプラグインからの出力（同「emit」の応答） */
  model EmitResponse { files: OutputFile[]; warnings: PluginWarning[]; errors: PluginError[]; select: Select; }

  /**
   * プラグインを一つ起動して、トランスパイラ一つに展開させる（docs/plugin-protocol.md「起動」）。
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
[hozo]
sources     = ["cli.hozo"]      # vocab/ と tests/ の .hozo を読まないため
plugin_dirs = ["plugins"]

[[emit]]
namespace   = "hozoc"
transpilers = ["go-cli"]
out         = "internal/gen/hozoc"
```

`writeOutputs` の `throws proc.Crashed | proc.MalformedOutput prevents effects` は、プラグインをすべて呼び終えて衝突を確かめてから書き始めるため（下の「emit」）。

## check

1. `hozo.toml` を読み、入っているプラグインのマニフェストを探して読み、各単位の `transpilers` の名前をトランスパイラに解決する（`docs/config.md`「置き場と探し方」「単位」、`docs/plugin-protocol.md`「探索」）。探索が見るプロジェクトの外の場所は、入力の `pluginPath`（`HOZO_PLUGIN_PATH`）と、`cmd/hozoc` が渡すホームディレクトリ（「手で書くもの」）。`hozo.toml` がなければ設定なしで進む（作業ディレクトリをルートにし、`sources` は既定。プラグインは探さない）。設定とマニフェストの error は、CFG-16 なら `LanguageMismatch`、それ以外は `ConfigInvalid`（`docs/config.md`「診断」、`docs/plugin-protocol.md`「診断」）。
2. 標準の語彙、使うプラグイン（`docs/plugin-protocol.md`「探索」）の語彙、`sources` のファイルの順に読み、解析する（`docs/grammar.md`）。`in` は読むファイルを変えない（`docs/config.md`「in」）。
3. 解決（`docs/resolve.md`）、正規化（`docs/ir.md`）、計算と検査（`docs/check.md`）を行う。
4. 使うプラグインのうち `[check] enabled` のものに `direction: "check"` を送り（相手と回数は `docs/plugin-protocol.md`「起動」）、返った診断を足す。プラグインの異常終了は `proc.Crashed` / `proc.MalformedOutput`。
5. error が一つでもあれば例外（値は error と warning のすべて）。設定かマニフェストの error（CFG-*、MAN-*）が一つでもあれば `ConfigInvalid`、ただしそれがすべて CFG-16 なら `LanguageMismatch`。それ以外は `CheckFailed`。error がなければ `CheckResult`（warning）。

構文エラーのあるファイルも、読めた宣言は解決・検査に進める。診断の順序は `docs/diagnostics.md`。

## emit

`check` の後、`writeOutputs`。出力（単位 U と、U が出力する名前空間 N の組）と、その置き場 Dir(U, N) は `docs/config.md`「単位の対象」「in」。一度の `emit` が書き出す出力の集まりを **emit の対象**と呼ぶ。

1. `namespaces` が空なら、すべての出力が emit の対象。空でなければ、引数 n ごとに、名前空間が n と同じか n の子孫である出力が emit の対象（その名前空間を出力するすべての単位の分。引数は単位ではなく名前空間を選ぶ）。n は単位の `namespace` に書いた名前である必要はない: n 自身がどの単位の対象でなくても、n の子孫を対象にする単位があればその出力が選ばれ、単位が名指しする名前空間の子孫を n に書けば、その子孫の出力だけが選ばれる。引数 n について、仕様にある名前空間のうち n と n の子孫のどれも、どの単位の対象（`docs/config.md`「単位の対象」）でもなければ `UnknownNamespace`（仕様にない名前を書いたときも、これに当たる）。単位の対象ではあるが `in` のせいで出力でない名前空間だけを選ぶ引数は `UnknownNamespace` ではなく、何も書かずに 6 の後始末だけを行う。
2. emit の対象の各出力 (U, N) について、U の `transpilers` の各トランスパイラを `runEmit` で起動して（順序は `docs/plugin-protocol.md`「起動」）、`files` と `select` を集める。`errors` があれば `CheckFailed`。応答の形の誤り、`files` の `path` の誤り（PLG-1 / PLG-2）、`select` の誤り（PLG-3）は `proc.MalformedOutput`（`docs/plugin-protocol.md`「応答の形」「診断」）。
3. emit の対象の各出力について、U のトランスパイラの `select` を合わせて既定の出力（`docs/emit-go.md`「出力の対象」）を作る。GOEMIT-* があれば `CheckFailed`。出力が別の名前空間 M の型を使うときは、M を出力する Go の単位のうち emit の対象でないもののトランスパイラも起動して、`select` だけを使う（どの単位の出力がその型を持つかで、import するか複製するかが決まる。`docs/emit-go.md`「配置」）。import の循環（GOEMIT-5）は、emit の対象の出力から import を辿って届くすべての出力について確かめる（emit の対象でない出力も、`select` から既定の出力を作って import だけを見る）。emit の対象を絞っても、書き出す出力の内容は、すべてを対象にしたときと同じになる。
4. すべての出力のファイルを集めてから、パスの重複を確かめる（CFG-5 → `OutputConflict`）。
5. `dryRun` なら、書く予定のパスを `written` に入れて返す（書かない）。
6. emit の対象の各出力の `<Dir(U, N)>/.hozoc-outputs.json`（前回書いたパスの一覧。ルートからの相対パスの JSON の配列、辞書順）を読み、今回書かないパスのファイルを消す。ファイルがなければ何もしない。U の対象の名前空間が `in` のせいで出力でなくなったとき（`docs/config.md`「in」）も、その Dir(U, N) は同じに扱う（前回のファイルを消す）。`hozo.toml` から単位を消した、または `namespace` か `out` を変えた場合、前の出力先はもう分からないので消さない（手で消す）。
   - 記録のパスに従って消すのは、その記録の持ち主の Dir(U, N) の下のファイルだけ。記録に、Dir(U, N) の下でないパス（`..` で外へ出るもの、絶対パス、ルートの中の別のディレクトリ、別の出力の Dir）が書かれていても、そのファイルは消さず、`removed` にも warning にも出さない。hozoc がこの記録に書くのは Dir(U, N) の下のパスだけ（既定の出力のファイルと、PLG-1 を通ったトランスパイラのファイル）なので、それ以外は hozoc の書いたものではなく、記録が壊れているか手で直されている。壊れた記録で、出力先の外やプロジェクトの外のファイルを消さないための規則。黙って無視するのは、単位を消したときの前の出力先と同じ扱い（hozoc が持ち主だと言えないファイルには触れず、何も言わない）。
7. ファイルを書く。内容が同じなら書き直さない（`written` には入れる）。ディレクトリは作る。
8. `.hozoc-outputs.json` を書く。書くものが一つもない出力（何も選ばれていない）には作らず、前回のものがあれば消す。

`EmitResult.warnings` は `check` の warning に、既定の出力とトランスパイラの warning を足したもの。

書く順序はパスの辞書順。途中で失敗したら、そこまでに書いたものは残る（原子性は持たない。`proc.Crashed` は 2 と 3 で起きるので、書き出しには入っていない）。

## lsp

`docs/lsp.md`。`config` が与えられなければ、クライアントの `rootUri` から探す。

## 手で書くもの

- `cmd/hozoc/main.go`: `internal/app` が用意した葉の実装（`hozoc.Leaves` と `hozoc.States` のフィールド。`docs/emit-go.md`「実装の単位」）を入れ、`hozoc.New` で `Ops` にし、`Presenter` と一緒に `cli.Main`（`docs/plugin-go-cli.md`「出力」）に渡す。プロセスの環境を読むのはここだけ: 環境変数は `os.Getenv` を `cli.Main` に渡し（`cli.hozo` の `@cli.env` のフィールドに入る）、ホームディレクトリ（`os.UserHomeDir()`。プラグインの探索の `<ホーム>/.hozo/plugins`）は `app.App` の `Home` に入れる。それだけ。骨組み（引数の解釈、例外の見分けと終了コード、使い方）は `cli.hozo` から生成した `internal/gen/hozoc`（`docs/emit-go.md`）と `internal/gen/hozoc/cli`（go-cli）が持ち、変えるときは `cli.hozo` を変えて `make generate`（`docs/packages.md`「自己生成の手順」）。生成物はコミットし、`tests/generated_test.go` と CI が遅れていないことを確かめる（`docs/testing.md`「生成物の一致」）。
- `internal/app`: 葉の op の実装。go-cli が選ぶのは `@cli.command` の `check` / `emit` / `lsp` と、`emit` の配線から辿られる `writeOutputs`（`docs/plugin-go-cli.md`「マニフェスト」、`docs/emit-go.md`「出力の対象」）。どの配線からも呼ばれない `runEmit` は生成の対象にならず、`internal/app` の中の関数になる（`cli.hozo` には境界の記述として残る）。効果を持たない `check` / `lsp` は `hozoc.Leaves` のフィールドに、効果 `syncOutputs` を持つ `writeOutputs` は、その宛先の状態の単位 `hozoc.States.OutputsSync` の命令に入る（`docs/emit-go.md`「state」）。`check` は `internal/project.LoadWith`、`writeOutputs` は `internal/emit/golang` とプラグインの起動（`internal/plugin.Runner`）、`lsp` は `internal/lsp.Serve`（`docs/packages.md`）。`OutputsSync` の述語 `UpToDate` は、生成し直した内容とディスクの内容を比べて実装する。境界の型は生成したもの（`hozoc.CheckInput`、`hozoc.Diagnostic`。中の `internal/diag.Diagnostic` は行と列に直して写す）で、例外は生成した exception の型（`hozoc.CheckFailed{Value: hozoc.Diagnostics{...}}` など。`proc.Crashed` / `proc.MalformedOutput` は hozoc のパッケージに複製された `hozoc.Crashed` / `hozoc.MalformedOutput`。`docs/emit-go.md`「配置」）。`cli.hozo` の model は境界だけ（`docs/requirements.md`「自己展開」）なので、op をまたいで要る中身（`check` が解析した `Project`、`runEmit` に渡す IR。`View` は中身を持たない）は `internal/app` の中に持つ。包む関数はない（go-cli は `wraps` を返さない。`docs/emit-go.md`「デコレータ」）。
- `Presenter`（`cli.Presenter` の実装）: 入力（`CheckInput` / `EmitInput`）の `format` に応じて text か JSON（`docs/diagnostics.md`「表示」。診断の text と json の形は `internal/diag` が持ち、`Presenter` は `cli.hozo` の model `Diagnostic` をその形に写して呼ぶ）。結果は標準出力、例外は標準エラー（`docs/plugin-go-cli.md`「実行と表示」）。text の `CheckResult` は診断の一覧と件数、`EmitResult` は warning の一覧（あれば）に続けて `written: <path>` / `removed: <path>` を一行ずつ、診断を運ぶ例外はその一覧と件数、それ以外の例外は `hozoc: <Failure.Err のメッセージ>`。json の `CheckResult` は `{"warnings": [...]}`、`EmitResult` は `{"warnings": [...], "written": [...], "removed": [...]}`、例外は `{"error": "<FQN>", "value": {...}}` で、診断を運ぶ例外の `value` は `docs/diagnostics.md`「json」の形（`{"diagnostics", "errors", "warnings"}`）、それ以外は `cli.JSONPresenter` と同じ。診断の要素は `docs/diagnostics.md`「json」と同じ形。
- `hozo.toml` がないとき、`check` は設定なしで検査し、`emit` は CFG-7 を一つ作って `ConfigInvalid` にする（`docs/config.md`「置き場と探し方」）。
