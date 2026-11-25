# hozoc

## この文書が決めること

この文書の細部は、決定 D603、D703 が決めた。

`hozoc` のサブコマンドの振る舞いと、hozoc 自身の仕様 `cli.hozo`（`docs/requirements.md`「自己展開」）。骨組みは `cli.hozo` から三つのトランスパイラが生成する: 型と op の形と配線は基底 `go`（`docs/plugin-go.md`）、入力の制約を確かめる関数は `go-valid`（`docs/plugin-go-valid.md`）、引数の解釈と表示と終了コードは `go-cli`（`docs/plugin-go-cli.md`）。ここで決めるのは葉の op の中身。hozoc 本体は、コードを出力しない（`docs/plugin-protocol.md`「この文書が決めること」）。

## cli.hozo

hozoc 自身の仕様は、リポジトリ直下の `cli.hozo`。下のコードブロックはその写しで、`hozo.toml` のブロックも同じ（バイト単位で同じであることを `tests/parse_test.go` の `TestDocsEmbedRootFiles` が確かめる）。この文書と `docs/` のほかの文書が、修飾なしに `cli.hozo` と書けばこのファイルを指す。go-cli プラグインの語彙のファイルは `plugins/go-cli/cli.hozo` と書く（`docs/plugin-go-cli.md`「マニフェスト」）。

業務の流れ（ユースケースを、順に呼ぶ配線）は、別のファイル `flows.hozo` の、名前空間 `hozo.flows` が持つ（決定 D475。写しは、ここには、置かない）。hozoc の外の手順（利用者か CI が、テストを走らせる）は、同じファイルの `hozo.toolchain` の op として言う。どちらの名前空間も、単位の対象ではなく、コードには、ならない。

プラグインとのやり取り（`plugin.hozo` の `hozo.plugin`）と、IR（`ir.hozo` の `hozo.ir`）も、Hozo で写してある（同 468）。**写しで、持ち主は、文書**（`docs/plugin-protocol.md`、`docs/ir.md`）: 今の言語で、JSON の上の形を、言い切れない所があり（`docs/open.md`「サンプルと、hozoc 自身の仕様」の 14〜20。印 `kind` などの、決まった値の項目と、列挙は、リテラルの型で書いてある: 決定 D482）、生成には、使っていない。単位の対象ではなく、コードには、ならない。

言語サーバー（`lsp.hozo`）も、同じに、写してある（同 468）: 入口（LSP の要求と通知）の `hozo.lsp`、入口から入る操作の `hozo.operation`、相手（文面のエディタ）の `hozo.editor`。**写しで、持ち主は、文書**（`docs/lsp.md`、`docs/editors.md`、`docs/tools.md`「操作」）。単位の対象ではなく、コードには、ならない。ユースケース `serve`（下の `hozo.compiler`。葉）は、その入口と操作の全体を、一つの op にしたもので、重なっている（`docs/open.md`「サンプルと、hozoc 自身の仕様」）。

IDE と、MCP サーバー（`ide.hozo`）も、同じに、写してある（同 468）: 画面の入口 `hozo.ide`（語彙 `browser.page`）、AI の道具の入口 `hozo.mcp`、チャットの AI（外）の `hozo.assistant`、操作の続きの `hozo.operation`。**写しで、持ち主は、文書**（`docs/tools.md`、`docs/ide-screens.md`）。IDE も、MCP サーバーも、実装は、まだ、ない。単位の対象ではなく、コードには、ならない。担い手「設計する人」（下の `Actor.Designer`）は、この写しと一緒に、足した（同 474）。

診断（`diagnostics.hozo` の `hozo.diagnostics`）も、要求として、写してある（同 442、468、469）: 診断の ID が、対象のない分類のケースで、重さ、出どころの決定、持ち主の文書と見出し、欠番、文言に埋める値を、自前の印で結ぶ。写したのは、代表の 24 個。**写しで、持ち主は、文書**（`docs/requirements.md`「検査の規則の一覧」と、それぞれの文書の「診断」の表）。単位の対象ではなく、コードには、ならない。書けなかった所は `docs/open.md`「サンプルと、hozoc 自身の仕様」の 30〜35。

決定の記録（`decisions/` の `hozo.decisions`）も、Hozo で書いてある（決定 D471、D476〜D481、D533、D534、D541）: 決定が、対象のない分類（百の位ごとの `Decision000`〜、設計書から離れた決定の `DecisionS`）のケースで、理由、結果、採らなかった案、決めた人の言葉を、説明のコメントの中のタグ（`@adr.context` など）で、状態（提案、採用、却下、廃止、置き換え済み）と、一部を直す決定を、印で書く。語彙 `adr` は `decisions/adr.hozo` に、自前のものとして宣言してある（同梱の語彙では、ない）。形を決めた宣言の側には、`@adr.decided(…)` を付けてある（この文書の写しの `cli.hozo` にも、現れる）。**決定の持ち主は、`decisions/` のケース**。`docs/decisions.md` の行は、そのまま本文に移した（D541）。単位の対象ではなく、コードには、ならない。書けなかった所は `docs/open.md`「サンプルと、hozoc 自身の仕様」の 36〜39。

設計書の `examples/hozoc.hozo` を元に、次を変えたもの。変えた理由はそれぞれの項目に書く（決定の一覧での番号は 決定 D20 と D36）。

`examples/hozoc.hozo` が「書けなかったもの」として挙げた三つは、次のように解く。

- **状態の宣言から設定の値を参照できない**（`@fs.files("{out}/**")`）: `outputs` の状態には `@fs.files` を付けず、置き場が `hozo.toml` の各単位の `out` で決まることを説明に書く。置き場は設定で決まり、仕様の宣言の時点では決まらないため。
- **`runEmit` を呼ぶ繰り返しを配線で書けない**（配線はループを持たない。§9）: 繰り返しは葉の op の中に置く（プラグインを、一つずつ起動するのは `check`。その答えから、ファイルを集めるのは `collectOutputs`）。書き出すユースケース `write` は配線にし、`check` を呼んでから、葉 `collectOutputs`（`check` が受け取った、トランスパイラの答えから、ファイルを集める。ファイルには触れない）、`read` の `outputChanges`（書くファイルと消すファイルを、今の `outputs` と比べて決める）、効果 `syncOutputs`（書き出す）を、この順に呼ぶ: 状態に触れるのは `read` と効果だけで、葉の op は触れない（`docs/check.md`「葉の op」）。検査を通った結果だけを書き出しに渡すことと、`check` の例外が、ユースケースと、コマンド `build` に伝わることを、境界の記述のまま検査器に確かめさせるため。`runEmit` はどの配線からも呼ばれないが、プラグインとの境界の記述として残す（「手で書くもの」）。
- **デコレータの引数に実行時の値を書けない**（`@proc.stdio("{plugin.command}")`）: 呼び先を引数 `plugin` で渡す。デコレータ `proc.stdio` は、付けない: 語彙 `proc`（外部プロセスの起動）は、プラグインを WASM で動かすようにした後、なくした（決定 D434）。`runEmit` は、例外を、op に直に書く。`runImport` は段階 6 で足す。

ほかに変えたこと:

- 名前空間は `hozo.compiler`（出典は `hozoc`）。Hozo 自身の仕様は、`hozo` の下の、入れ子の名前空間に書く（決定 D468）。生成物の置き場は `internal/gen/hozo/compiler` で、Go のパッケージ名は `compiler`。例外の名前の表示（`--format json` の `"error"`）は、`hozo.compiler.CheckFailed` のように、名前空間から書く。コマンドの名前（`hozoc`）と、バイナリの置き場（`cmd/hozoc`）は、変えていない。
- **入口（コマンドの行）と、ユースケースを、二つの名前空間に分ける**（決定 D472、D473）。`hozo.cli` が、コマンドの op（`@cli.command` の `build`、`tests`、`lsp`）と、その入力（`@cli.flag` などを持つ `BuildInput`、`TestsInput`、`LspInput`）を持ち、`hozo.compiler` が、ユースケースの op（`verify`: 検査する。`preview`: 書き出す予定を見る。`write`: 書き出す。`confirmTests`: 求めるテストが走ったかを確かめる。`serve`: 言語サーバーとして動く）を持つ。フラグ（`--no-emit`、`--dry-run`、`--format`）を知っているのは、`hozo.cli` だけ: ユースケースの入力（`EmitInput`、`CoverageInput`、`ServeInput`）は、それに当たる項目を持たず、コマンドの op が、入力から、レコードリテラルで組んで渡す。`build` は、フラグ（`noEmit`、`dryRun`。真偽の値）を、型で決まる分類 `Flag`（`On`: `boolean as true`、`Off`: `boolean as false`。リテラルの型: 決定 D483）で分け、三つのユースケースの、どれかを呼ぶ（「build」の表）: フラグの値から、ユースケースへの対応は、仕様の配線が言い、人が書く述語は、ない。分かれ道は二つ（`--no-emit` か。そうでなければ、`--dry-run` か）で、アームの本体に `match` は書けないので、二つ目は、内部の op `writeOrPreview` に分けてある（`docs/open.md`「サンプルと、hozoc 自身の仕様」）。フラグは、省略可能でない `boolean`: 付いていなければ、偽（`docs/plugin-go-cli.md`「語彙」の、値が与えられなかったときの扱い）。生成物の置き場は、`internal/gen/hozo/cli`（コマンドの行）と `internal/gen/hozo/compiler`（ユースケース）。
- 例外は、投げるユースケースの側（`hozo.compiler`）に宣言し、終了コード（`@cli.exitCode`）も、その宣言に付ける（`@http.status` を、別の名前空間の例外に付けるのと、同じ形。`go-cli` は、コマンドの op が投げうる例外を、どの名前空間のものでも、見分ける: `docs/plugin-go-cli.md`「コマンドごとの関数」）。終了コードは、入口の側の事情だが、デコレータは、宣言に付けるものなので、ユースケースの側の宣言に、残る（同「サンプルと、hozoc 自身の仕様」）。
- 担い手（分類 `Actor`）と、要求（分類 `Requirement`）を足し、ユースケースの op に、行う担い手（`@prd.usecase.of`）と、要求（`@prd.ref`）を結んだ（`docs/vocab.md`「設計の図の語彙」。決定 D468）。中身は、文書から写した: 担い手は `docs/requirements.md`「書く人と、読む人」「言語サーバー」、この文書の「tests」、`docs/testing.md`「CI」。要求は `docs/requirements.md`「ただ一つの正」「もとの考えと、Hozo が守るもの」（決定 373）「言語サーバー」「品質の要件」。四つのデコレータは `@hint` で、今の単位のトランスパイラは読まない: 生成物は、変わらない。業務フロー（`@prd.business.flow`）は、書いていない（`docs/open.md`「サンプルと、hozoc 自身の仕様」）。
- `fs` の語彙は標準の語彙（`docs/vocab.md`「同梱する語彙」）に、`cli` の語彙は go-cli プラグインが提供する語彙（同「同梱しない語彙」）になったので、どちらも `cli.hozo` では宣言しない。出典の語彙 `proc` の二つの例外（`Crashed`、`MalformedOutput`）は、名前空間 `hozoc` の例外として、`cli.hozo` に宣言する（同 434）。
- `lsp` を足す。言語サーバーを別のバイナリにしない（`docs/requirements.md`「言語サーバー」）ので、hozoc のサブコマンドとして境界に書く。
- 書き出しの三つの変化（追加・書き換え・削除）は、`outputs` を対象にした分類 `OutputsSync` の遷移 `syncOutputs` 一つにする（一つの効果が変えるのは、一つの状態の一つの変化。`docs/check.md`「効果の宛先」）。ケースは `UpToDate` と `OutOfDate` の二つで、どちらも述語を持つ（`docs/check.md`「ケースの判定」）。述語 `UpToDate` は「生成し直して差分がない」で、`docs/testing.md`「生成物の一致」の検査と同じ中身。述語 `OutOfDate` は、その否定。
- 出力の形式を選ぶ `format` のフラグを足す。診断を機械が読めるようにするため（`docs/diagnostics.md`「json」）。
- 例外 `UnknownNamespace` を足す。`build` の引数の名前空間の書き間違いを、何も書かずに成功したことにしないため（「書き出し」の 1）。
- 環境変数 `HOZO_PLUGIN_PATH` を、`build` / `tests` / `lsp` の入力のフィールド `pluginPath` として足す（内部の op `check` には、配線が、入力から渡す）。プラグインの探索が見るプロジェクトの外の場所（`docs/plugin-protocol.md`「探索」の 2）も、ほかの環境変数（`HOZO_CONFIG`）と同じく境界の入力として書き、葉の op の実装がプロセスの環境を自分で読まないようにするため（同じ入力から同じ結果になり、テストが実行する人の環境に左右されない）。
- 環境変数 `HOZO_CACHE_DIR` も、同じ流儀で、`build` / `tests` / `lsp` の入力のフィールド `cacheDir` として足す（`check` には、配線が渡す）。プラグインの WASM を、機械の言葉に直したものを、覚える置き場（`docs/plugin-protocol.md`「起動」）。
- model `Plugin` は、プラグインの名前と、WASM のファイルの場所だけを持つ。プラグインは、hozoc の中の、サンドボックスで動き、作業ディレクトリも、引数も、渡さない（同「起動」）ので、前にあった `command`、`dir`、`root` は、ない。
- 設定を単位の一覧にした（`docs/config.md`「単位」）のに合わせて、`EmitRequest` に `transpiler` を足し、`LanguageMismatch` と `UnknownNamespace` の意味を単位の言葉に直す。
- プラグインとやり取りする model（`EmitRequest` / `EmitResponse` と、その中の `EmitOutput`、`OutputResult`、`TranspilerResult`）は `docs/plugin-protocol.md`「要求と応答」の JSON をそのまま写す（一つの要求に、仕様を一度と、受け持つ出力の並び。決定 D411）。IR の中身（`Spec`、`OutputView`）は、書かない。応答の `select` をなくした（同「選択をなくしたこと」）のに合わせて、`Select` と `Wrap` を持たない。
- **コマンドは `build`、`tests`、`lsp` の三つ**（決定 D417）。前にあった `check` と `emit` の二つのコマンドは、`build` 一つにまとめた（「build」）: `build` は、検査して、書き出す。`build --no-emit` は、検査だけ。検査の op `check` は、コマンドでも、ユースケースでもない、内部の葉の op として残る（ユースケース `verify`、`preview`、`write`、`confirmTests` の配線が、はじめに呼ぶ）。その入力 `CheckInput` は、配線が、ユースケースの入力から作る。
- `build` / `tests` / `lsp` に `export` を付ける。`@cli.command` の付いた op は、`go-cli` の `Main` が、名前空間の外から入口の `Ops` を通して呼ぶ（`docs/plugin-go-cli.md`「対象」の CLI-6）。ユースケースの op（`hozo.compiler` の `verify`、`preview`、`write`、`confirmTests`、`serve`）にも、`export` を付ける: 別の名前空間（`hozo.cli`）から、呼ぶ。`check`、`collectOutputs`、`verifyOutputs`、`plannedChanges`、`matchTests`、`runEmit` は内部の op。
- 例外が運ぶ値は、例外の引数で書く（`exception CheckFailed(diagnostics: Diagnostic[]);`。`docs/grammar.md`「exception / effect / read」）。値を包むだけの model `Diagnostics` は持たない。
- `tests` を足す（決定 D380）。ユースケース `confirmTests` は配線にして、`check` を呼んでから、葉の op `matchTests` を呼ぶ（`write` と同じ理由: 検査を通った結果だけを渡すことを、境界の記述のまま、検査器に確かめさせる）。対応表を集めるところは、`collectOutputs` を呼ばず、`matchTests` の中に置く: 配線は、リストのリテラルを書けず（`docs/grammar.md`「配線」）、`EmitInput`（`namespaces` を持つ）を、配線の中で作れない。入力の `coverage` は、なければ使い方の誤りにする（`@cli.required`。`docs/plugin-go-cli.md`「語彙」）。
- `BuildInput` の位置引数は、位置を引数で言う（`@cli.arg(1) namespaces: string[];`。`docs/plugin-go-cli.md`「語彙」）。フラグと環境変数の行は、名前を書いている。

```ts
/**
 * hozo の仕様を検査し、各言語のコードに展開するコンパイラの、ユースケース。
 * コマンドの行（入口）は、名前空間 hozo.cli で、フラグを知っているのは、そちらだけ（docs/cli.md「cli.hozo」）。
 */
namespace hozo.compiler {
  /**
   * 担い手: hozoc を使うもの。どの担い手が、どのユースケースを使うかは、ユースケースの op の @prd.usecase.of が言う
   * （docs/tools.md「担い手」、docs/requirements.md「書く人と、読む人」、docs/cli.md「tests」、docs/testing.md「CI」）。
   * 担い手は、していることで分ける。道具（エディタ、CLI、MCP サーバー、IDE）は、入口で、担い手ではない。
   * 設計する人が使う操作と、画面は、ide.hozo
   */
  @adr.decided(hozo.decisions.Decision400.D474)
  classify Actor of never {
    /** 仕様（.hozo）の文面を書く AI。文面を書くのは、主に AI */
    @prd.actor.ai Agent;
    /** 仕様の文面と、中身のコードを、自分のエディタで、直に書くエンジニア。エディタと言語サーバー、CLI は、道具（入口）で、担い手ではない */
    @prd.actor.human Engineer;
    /** CI。検査を走らせる。プロジェクトのやり方でテストを走らせ、カバレッジのファイルを渡す */
    @prd.actor.bot Ci;
    /** 設計する人。作りたいものを、仕様として決める。AI に書かせ、読み、直し、中身が仕様どおりかを見る。エンジニアでない人を含む。道具は、IDE（ide.hozo） */
    @prd.actor.human Designer;
  }

  /**
   * 要求: hozoc が満たすこと。どの宣言が、どの要求のためのものかは、@prd.ref が言う
   * （docs/requirements.md「ただ一つの正」「もとの考えと、Hozo が守るもの」「言語サーバー」「品質の要件」）
   */
  classify Requirement of never {
    /** ただ一つの正: 設計を言うものは、仕様（.hozo）だけ。生成するコードの枠と、テストは、仕様から作る */
    SingleSource;
    /** 共有: 検査だけの実行と、言語サーバーの診断は、同じ検査器の、同じ呼び出しで作る。同じ入力に対する診断が、二つの入口で食い違わない */
    SharedDiagnostics;
    /** 決定性: 同じ入力から、同じバイトの出力を作る。出力の並びは、宣言の並びに従い、ハッシュの順序に依存しない */
    Deterministic;
    /** 診断: 漏れを指摘するときは、何が漏れているかを、名指しする */
    NamedOmission;
    /** 配布: OS ごとの、単一のバイナリ。実行環境を要求しない。言語サーバーも、別のバイナリにしない */
    @adr.decided(hozo.decisions.Decision400.D470)
    SingleBinary;
    /** ハーネス: 必要なテストケースが、用意されていて、それが、通っていること */
    RequiredTestsRan;
  }

  /** ファイルのパス。相対パスの基準は使う場所が決める（docs/config.md「置き場と探し方」など） */
  scalar Path extends string;

  /** 読み込むファイル */
  model SourceFile { path: Path; content: string; }

  /** 書き出すファイル */
  model OutputFile { path: Path; content: string; }

  /** 位置（1 始まりの行と列） */
  model Position { line: int; col: int; }

  /** 指摘の、機械が読む補足（docs/check.md「Data」）。項目の名前から、値の並びを引く */
  model DiagnosticData { [key: string]: string[]; }

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
    data: DiagnosticData?;
  }

  /** 指摘に関連する位置 */
  model RelatedInfo { file: Path; start: Position; end: Position; message: string; }

  /** 仕様のファイル */
  @fs.files("**/*.hozo")
  state sources: list<SourceFile>;

  /** プロジェクトの設定（docs/config.md） */
  @fs.files("hozo.toml")
  state config: nullable<SourceFile>;

  /** 生成したファイル。置き場は hozo.toml の各単位の out（docs/config.md「単位の対象」） */
  state outputs: list<OutputFile>;

  /**
   * 仕様に構文か型の誤りがある
   *
   * @param diagnostics error と warning のすべて
   */
  @cli.exitCode(1)
  exception CheckFailed(diagnostics: Diagnostic[]);

  /**
   * hozo.toml がない、または設定が誤っている（docs/config.md の CFG-*）
   *
   * @param diagnostics error と warning のすべて
   */
  @cli.exitCode(2)
  exception ConfigInvalid(diagnostics: Diagnostic[]);

  /**
   * 一つの単位のトランスパイラの language がそろっていない
   *
   * @param diagnostics error と warning のすべて
   */
  @cli.exitCode(2)
  exception LanguageMismatch(diagnostics: Diagnostic[]);

  /**
   * 複数の出どころが同じファイルに書こうとした
   *
   * @param diagnostics error と warning のすべて
   */
  @cli.exitCode(3)
  exception OutputConflict(diagnostics: Diagnostic[]);

  /**
   * 求めるテストのうち、走ったと確かめられないものがある（docs/cli.md「tests」の TESTS-1）
   *
   * @param diagnostics error と warning のすべて
   */
  @cli.exitCode(4)
  exception TestsNotRun(diagnostics: Diagnostic[]);

  /**
   * カバレッジを、求めるテストと突き合わせられない（docs/cli.md「tests」の TESTS-2、TESTS-3）
   *
   * @param diagnostics error と warning のすべて
   */
  @cli.exitCode(5)
  exception CoverageUnusable(diagnostics: Diagnostic[]);

  /** 書き出す名前空間として指定されたものも、その子孫も、どの単位の対象でもない */
  @cli.exitCode(64)
  exception UnknownNamespace;

  /** プラグインが異常終了した（WASM の実行環境で、0 と 2 のほかの終了コードで終わった、途中で止まった、起動できなかった。docs/plugin-protocol.md「起動」） */
  exception Crashed;

  /** プラグインの出力が、約束した形（JSON）でなかった（docs/plugin-protocol.md「応答の形」） */
  exception MalformedOutput;

  /** 検査の結果（誤りではない指摘） */
  model CheckResult { warnings: Diagnostic[]; }

  /** 検査と書き出しのユースケース（verify、preview、write）の結果。検査だけ（verify）なら、written も removed も空 */
  model BuildResult {
    warnings: Diagnostic[];
    /** 書いたファイル（新規と更新） */
    written: Path[];
    /** 消したファイル */
    removed: Path[];
  }

  /** 突き合わせの結果 */
  model TestsResult {
    warnings: Diagnostic[];
    /** 走ったと確かめた、求めるテスト（`<トランスパイラの名前>: <テストの名前>`。対応表の順） */
    ran: string[];
  }

  /** 検査（check）の入力。ユースケースの入力（EmitInput、CoverageInput）から、配線が作る */
  model CheckInput {
    /** 設定ファイルの場所 */
    config: Path?;
    /** プラグインを探すディレクトリ（OS のパス区切りで複数） */
    pluginPath: string?;
    /** プラグインの WASM を、機械の言葉に直したものを、覚える置き場 */
    cacheDir: Path?;
  }

  /** 検査と書き出しのユースケース（verify、preview、write）の入力 */
  model EmitInput {
    /** 設定ファイルの場所 */
    config: Path?;
    /** 生成する名前空間（一つもなければすべて）。その名前空間と子孫を出力する単位の分を書き出す */
    namespaces: string[];
    /** プラグインを探すディレクトリ（OS のパス区切りで複数） */
    pluginPath: string?;
    /** プラグインの WASM を、機械の言葉に直したものを、覚える置き場 */
    cacheDir: Path?;
  }

  /** 求めるテストが走ったかを確かめるユースケース（confirmTests）の入力 */
  model CoverageInput {
    /** 設定ファイルの場所 */
    config: Path?;
    /** テストを走らせたときに取った、カバレッジのファイル */
    coverage: Path;
    /** プラグインを探すディレクトリ（OS のパス区切りで複数） */
    pluginPath: string?;
    /** プラグインの WASM を、機械の言葉に直したものを、覚える置き場 */
    cacheDir: Path?;
  }

  /** 言語サーバーとして動くユースケース（serve）の入力 */
  model ServeInput {
    /** 設定ファイルの場所 */
    config: Path?;
    /** ログの書き出し先 */
    log: Path?;
    /** プラグインを探すディレクトリ（OS のパス区切りで複数） */
    pluginPath: string?;
    /** プラグインの WASM を、機械の言葉に直したものを、覚える置き場 */
    cacheDir: Path?;
  }

  /** 生成したファイルが、今の仕様と設定から生成し直したものと一致しているか */
  classify OutputsSync of outputs {
    /** 一致している */
    UpToDate;
    /** 足りない、古い、または余分なファイルがある */
    OutOfDate;
  }

  /** 書き出す予定: 単位ごとにトランスパイラを呼んで集めたファイルと、そこまでの warning */
  model EmitPlan {
    warnings: Diagnostic[];
    files: OutputFile[];
  }

  /**
   * 書き出す予定を、今の生成したファイルと比べて、書くファイルと消すファイルを決める。
   *
   * @param plan 書き出す予定
   */
  read fn outputChanges(plan: EmitPlan): BuildResult reads outputs;

  /**
   * 生成したファイルを今の仕様と設定に合わせる（足りないものを書き、古いものを書き換え、余分なものを消す）。
   *
   * @param plan 書き出す予定
   */
  effect fn syncOutputs(plan: EmitPlan): effects OutputsSync as any -> UpToDate;

  /**
   * 仕様を検査する。使うプラグインに、展開を頼み、その診断も集める。ファイルは書き出さない。
   *
   * @param in 検査の入力
   * @throws LanguageMismatch 一つの単位のトランスパイラの language がそろっていない（docs/config.md の CFG-16）
   * @throws CheckFailed 仕様に誤りがある、または、トランスパイラが errors を返した
   * @throws Crashed プラグインが異常終了した
   * @see verify
   */
  @prd.ref(Requirement.SharedDiagnostics, Requirement.NamedOmission)
  fn check(in: CheckInput): CheckResult
    throws CheckFailed, ConfigInvalid, LanguageMismatch
    throws Crashed, MalformedOutput;

  /**
   * 検査がプラグインから受け取った答えから、出力ごとのファイルを集める。プラグインは、起動しない。ファイルには触れない。
   *
   * @param in ユースケースの入力
   * @param checked 検査の結果
   * @throws OutputConflict 同じファイルに二つの出どころが書こうとした
   * @throws UnknownNamespace 書き出す名前空間として指定されたものも、その子孫も、どの単位の対象でもない
   * @throws MalformedOutput トランスパイラの返したファイルのパスか、テストの対応表が、約束の形でない
   */
  fn collectOutputs(in: EmitInput, checked: CheckResult): EmitPlan
    throws OutputConflict, UnknownNamespace
    throws MalformedOutput;

  /**
   * 検査がプラグインから受け取った答えから、出力ごとのファイルを集めて、確かめる（パスと、ファイルの衝突）。集めたファイルは、返さない:
   * 返すのは、warning だけの結果（written も removed も空）。プラグインは、起動しない。ファイルには触れない。
   * hozo.toml がなければ、集めるものがなく、検査の warning だけを返す。
   *
   * @param in ユースケースの入力
   * @param checked 検査の結果
   * @throws OutputConflict 同じファイルに二つの出どころが書こうとした
   * @throws UnknownNamespace 書き出す名前空間として指定されたものも、その子孫も、どの単位の対象でもない
   * @throws MalformedOutput トランスパイラの返したファイルのパスか、テストの対応表が、約束の形でない
   */
  fn verifyOutputs(in: EmitInput, checked: CheckResult): BuildResult
    throws OutputConflict, UnknownNamespace
    throws MalformedOutput;

  /**
   * 仕様を検査する。プラグインに展開を頼み、返ってきたファイルを集めて確かめるところまで行う。ファイルには、触れない（読みも、書きも、消しもしない）。
   *
   * @param in ユースケースの入力
   * @see write
   */
  @prd.usecase.of(Actor.Agent, Actor.Engineer, Actor.Ci)
  @prd.ref(Requirement.SharedDiagnostics)
  @adr.decided(hozo.decisions.Decision400.D472, hozo.decisions.Decision400.D473)
  export fn verify(in: EmitInput): BuildResult {
    val checked = check({ config: in.config, pluginPath: in.pluginPath, cacheDir: in.cacheDir });
    return verifyOutputs(in, checked);
  }

  /**
   * 書き出す予定を、結果の形にする: written は、書く予定のファイルのパス（内容が同じで、書き直さないものも入る）。removed は、空
   * （消すファイルは、今の生成したファイルを読まないと決まらないので、決めない）。ファイルには触れない。
   *
   * @param plan 書き出す予定
   */
  fn plannedChanges(plan: EmitPlan): BuildResult;

  /**
   * 仕様を検査し、書き出す予定のファイルを返す。書き出さない。今の生成したファイルも、読まない（消すファイルは、決めない）。
   *
   * @param in ユースケースの入力
   * @see write
   */
  @prd.usecase.of(Actor.Agent, Actor.Engineer)
  @adr.decided(hozo.decisions.Decision400.D472, hozo.decisions.Decision400.D473)
  export fn preview(in: EmitInput): BuildResult {
    val checked = check({ config: in.config, pluginPath: in.pluginPath, cacheDir: in.cacheDir });
    val plan = collectOutputs(in, checked);
    return plannedChanges(plan);
  }

  /**
   * 仕様を検査し、プラグインから受け取ったファイルを書き出す。例外は、どれも書き出しを始める前に起きる（集めるのが先）。
   *
   * @param in ユースケースの入力
   * @see verify
   * @see preview
   */
  @prd.usecase.of(Actor.Agent, Actor.Engineer)
  @prd.ref(Requirement.SingleSource, Requirement.Deterministic)
  @adr.decided(hozo.decisions.Decision400.D472, hozo.decisions.Decision400.D473, hozo.decisions.Decision700.D743)
  export fn write(in: EmitInput): BuildResult
    effects syncOutputs
    throws CheckFailed, ConfigInvalid, LanguageMismatch, OutputConflict, UnknownNamespace prevents effects
    throws Crashed, MalformedOutput prevents effects
  {
    val checked = check({ config: in.config, pluginPath: in.pluginPath, cacheDir: in.cacheDir });
    val plan = collectOutputs(in, checked);
    val result = outputChanges(plan);
    syncOutputs(plan);
    return result;
  }

  /**
   * 検査がプラグインから受け取った答えから、テストの対応表を集め、カバレッジの「実行された行」と突き合わせる。ファイルは書き出さない。
   *
   * @param in ユースケースの入力
   * @param checked 検査の結果
   * @throws ConfigInvalid hozo.toml がない（docs/config.md の CFG-7）
   * @throws OutputConflict 同じファイルに二つの出どころが書こうとした
   * @throws TestsNotRun 求めるテストのうち、走ったと確かめられないものがある
   * @throws CoverageUnusable 置き場の出力が今の仕様からのものと違う、または、カバレッジを読めない
   * @throws Crashed カバレッジを読むプラグインが異常終了した
   */
  fn matchTests(in: CoverageInput, checked: CheckResult): TestsResult
    throws ConfigInvalid, OutputConflict, TestsNotRun, CoverageUnusable
    throws Crashed, MalformedOutput;

  /**
   * 仕様を検査し、求めるテストが走ったかを、カバレッジと突き合わせて確かめる。テストは走らせない。
   *
   * @param in ユースケースの入力
   * @see write
   */
  @prd.usecase.of(Actor.Agent, Actor.Engineer, Actor.Ci)
  @prd.ref(Requirement.RequiredTestsRan)
  @adr.decided(hozo.decisions.Decision400.D472, hozo.decisions.Decision400.D473)
  export fn confirmTests(in: CoverageInput): TestsResult {
    val checked = check({ config: in.config, pluginPath: in.pluginPath, cacheDir: in.cacheDir });
    return matchTests(in, checked);
  }

  /**
   * 言語サーバーとして標準入出力で動く（docs/lsp.md）。クライアントが exit を送るまで戻らない。
   *
   * @param in ユースケースの入力
   */
  @prd.usecase.of(Actor.Engineer)
  @prd.ref(Requirement.SharedDiagnostics)
  @adr.decided(hozo.decisions.Decision400.D472, hozo.decisions.Decision400.D473, hozo.decisions.Decision400.D474)
  export fn serve(in: ServeInput);

  /** プラグインの起動のしかた（docs/plugin-protocol.md「起動」）。wasm は、プラグインの本体（WASM）のファイル */
  model Plugin { name: string; wasm: Path; }

  /** 仕様: IR の、出力に依らない部分（docs/ir.md「まとめた形」の Spec）。中身はこの仕様では書かない */
  model Spec {}

  /** IR の、出力ごとの部分（同 OutputView）。中身はこの仕様では書かない */
  model OutputView {}

  /** プラグインが受け持つ出力一つ（docs/plugin-protocol.md「emit」の要求の outputs の要素）。transpilers は、その出力で頼むトランスパイラの名前。単位のオプションは view の中にある */
  model EmitOutput { transpilers: string[]; view: OutputView; }

  /** 展開のときのプラグインへの入力（同「emit」の要求）。仕様を一度と、プラグインが受け持つ出力の並び */
  model EmitRequest { protocol: string; direction: string; spec: Spec; outputs: EmitOutput[]; }

  /** トランスパイラの warning（同「emit」の応答の warnings の要素） */
  model PluginWarning { at: string; message: string; }

  /** トランスパイラの error（同「emit」の応答の errors の要素） */
  model PluginError { code: string; at: string; message: string; }

  /** トランスパイラ一つの、出力一つへの答え（同「emit」の応答の results の要素） */
  model TranspilerResult { files: OutputFile[]; warnings: PluginWarning[]; errors: PluginError[]; }

  /** 出力一つへの答え（同「emit」の応答の outputs の要素）。results は、要求の、その出力の transpilers と同じ順 */
  model OutputResult { results: TranspilerResult[]; }

  /** 展開のときのプラグインからの出力（同「emit」の応答）。outputs は、要求の outputs と同じ順 */
  model EmitResponse { outputs: OutputResult[]; }

  /**
   * プラグインを一つ、hozoc の中の WASM の実行環境で動かして、受け持つ出力のすべてを、まとめて展開させる
   * （docs/plugin-protocol.md「起動」）。入力は、標準入力に JSON で渡し、標準出力の JSON を、出力として受け取る。
   *
   * @param plugin 動かすプラグイン
   * @param req プラグインへの入力
   * @throws Crashed プラグインが異常終了した
   * @throws MalformedOutput プラグインの出力が、約束した形でない
   */
  fn runEmit(plugin: Plugin, req: EmitRequest): EmitResponse
    throws Crashed, MalformedOutput;
}

/**
 * hozoc のコマンドの行（入口）。引数とフラグを読んで、名前空間 hozo.compiler のユースケースに入る。
 * フラグ（--no-emit、--dry-run、--format）を知っているのは、ここだけ（docs/cli.md「cli.hozo」）。
 */
namespace hozo.cli {
  /** 表示の形式。text か json */
  @valid.pattern("^(text|json)$")
  scalar OutputFormat extends string;

  /** build の入力 */
  model BuildInput {
    /** 設定ファイルの場所 */
    @cli.flag("config") @cli.env("HOZO_CONFIG") config: hozo.compiler.Path?;
    /** 表示の形式 */
    @cli.flag("format") format: OutputFormat?;
    /** 生成する名前空間（一つもなければすべて）。その名前空間と子孫を出力する単位の分を書き出す */
    @cli.arg(1) namespaces: string[];
    /** 検査だけを行い、ファイルを書き出さない */
    @cli.flag("no-emit") noEmit: boolean;
    /** 書き出さずに、書き出す予定のファイルだけを表示する */
    @cli.flag("dry-run") dryRun: boolean;
    /** プラグインを探すディレクトリ（OS のパス区切りで複数） */
    @cli.env("HOZO_PLUGIN_PATH") pluginPath: string?;
    /** プラグインの WASM を、機械の言葉に直したものを、覚える置き場 */
    @cli.env("HOZO_CACHE_DIR") cacheDir: hozo.compiler.Path?;
  }

  /**
   * フラグが、付いているか。真偽の値の、型で決まる分類（docs/check.md「分類」。リテラルの型: 決定 D483）。
   * build は、これで、--no-emit と --dry-run を見て、入るユースケースを分ける: フラグの値から、ユースケースへの対応は、
   * 人が書く述語ではなく、下の二つの op の配線が、言う（docs/cli.md「build」）。
   */
  @adr.decided(hozo.decisions.Decision400.D472, hozo.decisions.Decision400.D483)
  classify Flag of boolean {
    /** 付いている */
    On: boolean as true;
    /** 付いていない */
    Off: boolean as false;
  }

  /**
   * 仕様を検査し、プラグインから受け取ったファイルを書き出す。--no-emit なら、検査だけを行い、ファイルを書き出さない。
   *
   * @param in コマンドの入力
   * @see tests
   */
  @cli.command("build")
  @adr.decided(hozo.decisions.Decision400.D472, hozo.decisions.Decision400.D473)
  export fn build(in: BuildInput): hozo.compiler.BuildResult {
    return match (in.noEmit by Flag) {
      On => hozo.compiler.verify({ config: in.config, namespaces: in.namespaces, pluginPath: in.pluginPath, cacheDir: in.cacheDir });
      Off => writeOrPreview(in);
    };
  }

  /**
   * --no-emit の付いていない build: --dry-run なら、書き出す予定を見る。そうでなければ、書き出す。
   * build の、二つ目の分かれ道。アームの本体に、match は書けないので、op を、一つ、分けてある。
   *
   * @param in コマンドの入力
   */
  @adr.decided(hozo.decisions.Decision400.D472, hozo.decisions.Decision400.D473)
  fn writeOrPreview(in: BuildInput): hozo.compiler.BuildResult {
    return match (in.dryRun by Flag) {
      On => hozo.compiler.preview({ config: in.config, namespaces: in.namespaces, pluginPath: in.pluginPath, cacheDir: in.cacheDir });
      Off => hozo.compiler.write({ config: in.config, namespaces: in.namespaces, pluginPath: in.pluginPath, cacheDir: in.cacheDir });
    };
  }

  /** tests の入力 */
  model TestsInput {
    /** 設定ファイルの場所 */
    @cli.flag("config") @cli.env("HOZO_CONFIG") config: hozo.compiler.Path?;
    /** 表示の形式 */
    @cli.flag("format") format: OutputFormat?;
    /** テストを走らせたときに取った、カバレッジのファイル */
    @cli.required @cli.flag("coverage") coverage: hozo.compiler.Path;
    /** プラグインを探すディレクトリ（OS のパス区切りで複数） */
    @cli.env("HOZO_PLUGIN_PATH") pluginPath: string?;
    /** プラグインの WASM を、機械の言葉に直したものを、覚える置き場 */
    @cli.env("HOZO_CACHE_DIR") cacheDir: hozo.compiler.Path?;
  }

  /**
   * 仕様を検査し、求めるテストが走ったかを、カバレッジと突き合わせて確かめる。テストは走らせない。
   *
   * @param in コマンドの入力
   * @see build
   */
  @cli.command("tests")
  export fn tests(in: TestsInput): hozo.compiler.TestsResult {
    return hozo.compiler.confirmTests({ config: in.config, coverage: in.coverage, pluginPath: in.pluginPath, cacheDir: in.cacheDir });
  }

  /** lsp の入力 */
  model LspInput {
    /** 設定ファイルの場所 */
    @cli.flag("config") @cli.env("HOZO_CONFIG") config: hozo.compiler.Path?;
    /** ログの書き出し先 */
    @cli.flag("log") @cli.env("HOZO_LSP_LOG") log: hozo.compiler.Path?;
    /** プラグインを探すディレクトリ（OS のパス区切りで複数） */
    @cli.env("HOZO_PLUGIN_PATH") pluginPath: string?;
    /** プラグインの WASM を、機械の言葉に直したものを、覚える置き場 */
    @cli.env("HOZO_CACHE_DIR") cacheDir: hozo.compiler.Path?;
  }

  /**
   * 言語サーバーとして標準入出力で動く（docs/lsp.md）。クライアントが exit を送るまで戻らない。
   *
   * @param in コマンドの入力
   */
  @cli.command("lsp")
  export fn lsp(in: LspInput) {
    hozo.compiler.serve({ config: in.config, log: in.log, pluginPath: in.pluginPath, cacheDir: in.cacheDir });
  }
}
```

`hozo.toml`（単位は二つ）:

```toml
[hozo]
sources     = ["cli.hozo", "decisions/*.hozo", "diagnostics.hozo", "flows.hozo", "ide.hozo", "ir.hozo", "lsp.hozo", "plugin.hozo"]   # vocab/ と tests/ の .hozo を読まないため
plugin_dirs = ["plugins"]

# Go のモジュールの名前（go.mod の module と同じもの。docs/plugin-go.md「マニフェスト」）。プロジェクト全体のオプション
[options.go]
module = "github.com/mew-ton/hozo"

# hozoc 自身（docs/cli.md「cli.hozo」）。1: ユースケース（名前空間 hozo.compiler）。型と op の形は基底 go、制約の検査は go-valid
[[emit]]
namespace   = "hozo.compiler"
transpilers = ["go", "go-valid"]
out         = "internal/gen/hozo/compiler"

# 2: コマンドの行（入口。名前空間 hozo.cli）。コマンドラインは go-cli
[[emit]]
namespace   = "hozo.cli"
transpilers = ["go", "go-valid", "go-cli"]
out         = "internal/gen/hozo/cli"
```

- **検査だけの結果と、書き出す予定の結果を組むところは、葉の op**（`verifyOutputs`、`plannedChanges`）。どちらの結果（`BuildResult`）も、空の並び（`written`、`removed`）か、ファイルの並びから作るパスの並びを持つ。配線は、リストのリテラルも、並びの写しも、書けないので、葉にした（`docs/open.md`「サンプルと、hozoc 自身の仕様」）。三つのユースケース（`verify`、`preview`、`write`）の結果の型を、同じ `BuildResult` にしているのも、配線の都合: `build` の `match` のアームの型は、そろっていなければならない（TYPE-8）。

`write` の `throws MalformedOutput prevents effects` は、ファイルをすべて集めて、パスと衝突を確かめてから書き始めるため（下の「書き出し」）。`write` の `Crashed` は、`check` が、プラグインを起動するところから来る。`write` の本体で、例外を投げる `collectOutputs` が、効果 `syncOutputs` より上の行にあることから、検査器が確かめる（REL-4）。

この節の細部は、決定 D649、D694、D742、D751 が決めた。

## build

`hozoc build [--no-emit] [--dry-run] [--format text|json] [--config <パス>] [<名前空間>…]`。仕様を検査し、プラグインから受け取ったファイルを書き出す（決定 D417）。TypeScript の `tsc` と `tsc --noEmit` と、同じ形。

| 書き方 | フラグ（分類 `Flag`） | 入るユースケース | すること |
|---|---|---|---|
| `hozoc build` | `noEmit` が `Off`、`dryRun` が `Off` | `write` | 検査して（「検査」）、書き出す（「書き出し」） |
| `hozoc build --no-emit` | `noEmit` が `On` | `verify` | 検査だけ。ファイルには、触れない（読みも、書きも、消しもしない）。「書き出し」の 1〜3（ファイルを集め、`path` と衝突を確かめる）までは、行う |
| `hozoc build --dry-run` | `noEmit` が `Off`、`dryRun` が `On` | `preview` | 書き出さずに、書き出す予定のファイルを表示する（「書き出し」の 4） |

- **入口が、フラグで、道を分ける**（決定 D472）: コマンドの op `hozo.cli.build` は、入力のフラグを、分類 `Flag` の `match` で見て、三つのユースケースの、どれかを呼ぶ（`build` が `noEmit` を見て、`On` なら `verify`。`Off` なら、内部の op `writeOrPreview` が `dryRun` を見て、`On` なら `preview`、`Off` なら `write`）。フラグは、ユースケースには、渡さない（ユースケースの入力 `EmitInput` は、`noEmit` も `dryRun` も持たず、書き出す予定 `EmitPlan` も、持たない）。三つのユースケースは、どれも、はじめに `check` を呼ぶ: `verify` は、その後に `verifyOutputs`、`preview` は `collectOutputs` と `plannedChanges`、`write` は `collectOutputs`、`outputChanges`、`syncOutputs`（「cli.hozo」）。
- **`--no-emit` の表示**は、検査の結果だけ: text は、warning と、数の行（`0 errors, 0 warnings`。`docs/diagnostics.md`「text」）。warning だけなら、終了コードは 0（hozoc 自身の仕様では、CFG-26 が二つ出る: `docs/vocab.md`「解釈されなくてよいデコレータ」）。json は、`{"warnings": [...]}`。`written` と `removed` は、出さない（いつも空）。`--no-emit` のない `build` は、warning と、`written: <パス>` / `removed: <パス>` の行（json は、`warnings`、`written`、`removed`）。
- `--no-emit` と `--dry-run` を、両方書けば、`--no-emit`（書き出す予定も、表示しない）: `build` が、先に `noEmit` を見て、`verify` に入るので、`dryRun` は、見ない。
- 名前空間の引数は、`--no-emit` でも、集めて確かめるファイルの範囲を選ぶ（「書き出し」の 1。`UnknownNamespace` も、同じ）。
- **`hozo.toml` がないとき**: `build --no-emit` は、設定なしで検査する（「検査」の 1）。`build` は、書き出す単位がないので、CFG-7 で `ConfigInvalid`（「書き出し」の 0）。
- **古い名前**: `hozoc check` と `hozoc emit` は、なくした。その名前で呼ばれたら、何もせずに、新しい書き方を標準エラーに示して、使い方の誤りの終了コード（64。`docs/diagnostics.md`「終了コード」）で終わる。示すのは、残りの引数を付けたもの: `hozoc check --format json` には `hozoc build --no-emit --format json`、`hozoc emit --dry-run server` には `hozoc build --dry-run server`。

この節の細部は、決定 D694、D751、D770 が決めた。

### 検査

内部の op `check`（コマンドでも、ユースケースでもない。ユースケース `verify`、`preview`、`write`、`confirmTests` の配線が、はじめに呼ぶ）。

1. `hozo.toml` を読み、入っているプラグインのマニフェストを探して読み、各単位の `transpilers` の名前をトランスパイラに解決する（`docs/config.md`「置き場と探し方」「単位」、`docs/plugin-protocol.md`「探索」）。探索が見るプロジェクトの外の場所は、入力の `pluginPath`（`HOZO_PLUGIN_PATH`）と、`cmd/hozoc` が渡すホームディレクトリ（「手で書くもの」）。`hozo.toml` がなければ設定なしで進む（作業ディレクトリをルートにし、`sources` は既定。プラグインは探さない）。設定とマニフェストの error は、CFG-16 なら `LanguageMismatch`、それ以外は `ConfigInvalid`（`docs/config.md`「診断」、`docs/plugin-protocol.md`「診断」）。
2. 標準の語彙、使うプラグイン（`docs/plugin-protocol.md`「探索」）の語彙、`sources` のファイルの順に読み、解析する（`docs/grammar.md`）。`in` は読むファイルを変えない（`docs/config.md`「in」）。
3. 解決（`docs/resolve.md`）、正規化（`docs/ir.md`）、計算と検査（`docs/check.md`）を行う。
4. 1〜3 に error が一つもなければ、プラグインに頼む。はじめに、語彙のプラグインごとに一回、`direction: "read"` を送る（相手は `docs/plugin-protocol.md`「語彙のプラグイン」: 仕様が使っている語彙のもの。単位がなくても、頼む）。その応答に error がなければ、使うプラグインごとに一回、`direction: "emit"` を送る（相手と、受け持つ出力は同「起動」）。どちらの応答も、返った `errors` を error の診断として、`warnings` を warning の診断（`PLUGIN`）として、足す（同じ診断は、一度だけ。同「emit」）。語彙の使い方の誤り（HTTP-5 など）と、トランスパイラが返す誤り（GOHTTP-16 など）は、ここで出る（決定 D404、D410、D416）。返ったファイルは、書かない: 同じ実行の、後の段（「emit」の `collectOutputs`、「tests」の `matchTests`）が使う。プラグインの異常終了は `Crashed`、形の違う応答は `MalformedOutput`。
5. error が一つでもあれば例外（値は error と warning のすべて）。設定かマニフェストの error（CFG-*、MAN-*）が一つでもあれば `ConfigInvalid`、ただしそれがすべて CFG-16 なら `LanguageMismatch`。それ以外は `CheckFailed`。error がなければ `CheckResult`（warning）。

構文エラーのあるファイルも、読めた宣言は解決・検査に進める。診断の順序は `docs/diagnostics.md`。

### 書き出し

ユースケース `write` の、`check` の後。下の手順の 1〜3 が `collectOutputs`（ファイルを集める）、5〜7 の、書くファイルと消すファイルを決めるところが `outputChanges`、書き出すところが `syncOutputs`。ユースケース `verify`（検査だけ）は、1〜3 を、確かめるためだけに行い（`verifyOutputs`）、`preview`（書き出す予定を見る）は、1〜3 の後、4 で終わる（`plannedChanges`）。出力（単位 U と、U が出力する名前空間 N の組）と、その置き場 Dir(U, N) は `docs/config.md`「単位の対象」「in」。一度の `build` が書き出す出力の集まりを **書き出しの対象**と呼ぶ。書き出すファイルは、どれもトランスパイラが返したもので、hozoc が自分で作るファイルは、出力先ごとの記録だけ。プラグインは、`check` の 4 で、一度、起動してあり、`collectOutputs` は、起動しない（`docs/plugin-protocol.md`「起動」: 一度の実行で、同じプラグインに、同じことを、二度、頼まない）。

0. `hozo.toml` がなければ（設定なし）、`verify` は、集めるものがないので、検査の warning だけを返す（`verifyOutputs`）。`preview` と `write` は、CFG-7 で `ConfigInvalid`（`collectOutputs`）。
1. `namespaces` が空なら、すべての出力が書き出しの対象。空でなければ、引数 n ごとに、名前空間が n と同じか n の子孫である出力が書き出しの対象（その名前空間を出力するすべての単位の分。引数は単位ではなく名前空間を選ぶ）。n は単位の `namespace` に書いた名前である必要はない: n 自身がどの単位の対象でなくても、n の子孫を対象にする単位があればその出力が選ばれ、単位が名指しする名前空間の子孫を n に書けば、その子孫の出力だけが選ばれる。引数 n について、仕様にある名前空間のうち n と n の子孫のどれも、どの単位の対象（`docs/config.md`「単位の対象」）でもなければ `UnknownNamespace`（仕様にない名前を書いたときも、これに当たる）。単位の対象ではあるが `in` のせいで出力でない名前空間だけを選ぶ引数は `UnknownNamespace` ではなく、何も書かずに 5 の後始末だけを行う。
2.書き出しの対象の各出力 (U, N) について、U の `transpilers` の各トランスパイラの答え（`check` の 4 で受け取ったもの）から、`files` を集める（順序は、単位の設定の順、名前空間の IR の順、`transpilers` に書いた順）。`files` の `path` の誤り（PLG-1 / PLG-2）と、テストの対応表の誤り（PLG-4）は `MalformedOutput`（`docs/plugin-protocol.md`「応答の形」「診断」）。`errors` は、ここには、ない（一つでもあれば、`check` が `CheckFailed` で終わっている）。
3. すべての出力のファイルを集めてから、パスの重複を確かめる（CFG-5 → `OutputConflict`）。
4. `verify` は、ここで終わり: `written` も `removed` も空で返す（置き場は、読まない）。`preview` も、ここで終わり: 書く予定のパスを `written` に入れて返す（書かない。置き場は、読まないので、`removed` は、空）。
5.書き出しの対象の各出力の `<Dir(U, N)>/.hozoc-outputs.json`（前回書いたパスの一覧。ルートからの相対パスの JSON の配列、辞書順）を読み、今回書かないパスのファイルを消す。ファイルがなければ何もしない。U の対象の名前空間が `in` のせいで出力でなくなったとき（`docs/config.md`「in」）も、その Dir(U, N) は同じに扱う（前回のファイルを消す）。`hozo.toml` から単位を消した、または `namespace` か `out` を変えた場合、前の出力先はもう分からないので消さない（手で消す）。
   - 記録のパスに従って消すのは、その記録の持ち主の Dir(U, N) の下のファイルだけ。記録に、Dir(U, N) の下でないパス（`..` で外へ出るもの、絶対パス、ルートの中の別のディレクトリ、別の出力の Dir）が書かれていても、そのファイルは消さず、`removed` にも warning にも出さない。hozoc がこの記録に書くのは Dir(U, N) の下のパスだけ（PLG-1 を通ったトランスパイラのファイル）なので、それ以外は hozoc の書いたものではなく、記録が壊れているか手で直されている。壊れた記録で、出力先の外やプロジェクトの外のファイルを消さないための規則。黙って無視するのは、単位を消したときの前の出力先と同じ扱い（hozoc が持ち主だと言えないファイルには触れず、何も言わない）。
   - 消したファイルのディレクトリが空になれば、Dir(U, N) の下の範囲で、そのディレクトリも消す（Go の基底の内側のフォルダ `inner` が要らなくなったとき。`docs/plugin-go.md`「内側のフォルダと入口」）。
6. ファイルを書く。内容が同じなら書き直さない（`written` には入れる）。ディレクトリは作る。
7. `.hozoc-outputs.json` を書く。書くものが一つもない出力（どのトランスパイラもファイルを返さなかった）には作らず、前回のものがあれば消す。

-書き出しの対象を絞っても、書き出す出力の内容は、すべてを対象にしたときと同じになる。どのトランスパイラも、一つの出力の答えを、その出力の `View` だけから作り（`docs/plugin-protocol.md`「起動」）、ほかの出力の答えを要らないため。
-書き出しの対象を絞っても、プラグインには、すべての出力を頼む（`check` の 4 が、すべての出力の診断を要る）。絞るのは、集めて、書き出すファイルだけ。対象でない出力の、`path` の誤り（PLG-1 / PLG-2 / PLG-4）と、衝突（CFG-5）は、見ない。
- `BuildResult.warnings` は `check` の warning（トランスパイラの warning を含む）。
- 書く順序はパスの辞書順。途中で失敗したら、そこまでに書いたものは残る（原子性は持たない。`Crashed` は 2 で起きるので、書き出しには入っていない）。

この節の細部は、決定 D752 が決めた。

## tests

`hozoc tests --coverage <パス>` は、求めるテスト（テストを受け取る口が、ケースを受け取って走らせるテスト。`docs/plugin-protocol.md`「テストの対応表」）が、走ったかを、カバレッジのファイルと突き合わせて確かめる。hozoc は、テストを走らせない: 利用者（か CI）が、プロジェクトのやり方で、カバレッジを取りながらテストを走らせ、そのファイルの場所を渡す。分かるのは「走ったか」までで、「通ったか」は、テストの実行の終了コードが言う（`docs/ide-chain.md`「テストの規約」の「担保すること」）。

ユースケース `confirmTests`: `check` の後、`matchTests`。

1. すべての出力について、トランスパイラの答えから、ファイルを集める（「書き出し」の 2 と 3 と同じ。`OutputConflict`、`MalformedOutput` も同じ）。ファイルは、書き出さない。`hozo.toml` がなければ、CFG-7 で `ConfigInvalid`（「手で書くもの」の最後の項目）。
2. 集めたファイルから、テストの対応表を取り出す。その並び（出力の順、トランスパイラの順、対応表の中の順）が、**求めるテスト**。一つもなければ、ここで終わり（`ran` は空）。
3. 対応表と、対応表が指すファイルが、置き場にあり、集めたものと同じ内容であることを確かめる。ないか、違えば、ファイルごとに TESTS-2（置き場の出力が、今の仕様からのものでない。行が、合わない）。一つでもあれば `CoverageUnusable`。
4. `coverage` のファイルを読み（相対パスの基準は、作業ディレクトリ）、対応表を持つ単位の基底のプラグインごとに、`direction: "coverage"` を送る（相手と、渡すものは `docs/plugin-protocol.md`「coverage」）。次のどれかなら、TESTS-3 で、一つでもあれば `CoverageUnusable`: ファイルを読めない（一つ）。単位の基底のプラグインが `[coverage] enabled` でない（プラグインごとに一つ）。プラグインの応答が `recognized: false`（プラグインごとに一つ）。
5. 求めるテストごとに、応答の、そのファイルの `lines` のどれかが、テストの `lines` の範囲と、一行でも重なれば、**走った**。重ならなければ、走っていない。応答に、そのファイルがなければ、カバレッジが、そのファイルを測っていない。どちらも、テストごとに TESTS-1 で、一つでもあれば `TestsNotRun`。
6. すべて走っていれば、`TestsResult`（`warnings` は、`check` の warning（トランスパイラの warning を含む）。`ran` は、求めるテストの `<トランスパイラの名前>: <テストの名前>`）。

診断:

| ID | 重さ | 意味 |
|---|---|---|
| TESTS-1 | error | 求めるテストが、走ったと確かめられない。二つの場合がある。（1）カバレッジが、そのテストの行を、実行済みと言っていない: テストを受け取る口を呼ぶテストを、書いていない。絞って走らせた。カバレッジを取るときに、測る範囲に、口のパッケージを入れていない（Go なら `-coverpkg`。`docs/plugin-go.md`「カバレッジを読む」）。（2）カバレッジが、そのファイルを測っていない。位置は、対応表の `file` と、`lines` の始まりの行 |
| TESTS-2 | error | 対応表か、対応表が指すファイルが、置き場にない、または、今の仕様から集めたものと、内容が違う（`hozoc build` をやり直して、テストを走らせ直す）。位置は、そのファイル |
| TESTS-3 | error | カバレッジを読めない: `coverage` のファイルを開けない、UTF-8 の文字列でない、単位の基底のプラグインが `[coverage] enabled` でない、プラグインが、その形を読めなかった（`recognized: false`）。位置は、`coverage` のファイル |

- メッセージは、テストを `<トランスパイラの名前>: <テストの名前>` で名指す（`go-http: server.login/OK`）。同じ名前のテストが、二つのトランスパイラの対応表にあってよい（葉の op に経路を付ければ、単体テストと、インテグレーションテストが、同じ `<op の FQN>/<結果>` になる）。
- **確かめられないこと**: カバレッジを取った後に、`hozoc build` をやり直して、同じ内容でない出力を書いた場合、3 は、今の置き場を見るので、通る。カバレッジは、前の出力の行を言っているので、突き合わせは、当てにならない。カバレッジと出力の、新しさは、比べない（ファイルの時刻は、チェックアウトで変わる）。CI では、`emit` の後の一致（`docs/testing.md`「生成物の一致」）、テスト、`hozoc tests` を、この順に、同じ作業ツリーで走らせる。
- 一度に渡せるカバレッジのファイルは、一つ（言語の違う単位を持つプロジェクトの扱いは、決めていない: `docs/open.md`「語彙とプラグイン」）。

## lsp

`docs/lsp.md`。`config` が与えられなければ、クライアントの `rootUri` から探す。

## 手で書くもの

`cli.hozo` から生成するのは、二つの名前空間の出力。ユースケースの `hozo.compiler` は、`internal/gen/hozo/compiler`（基底 `go` の出力。パッケージ名は、名前空間 `hozo.compiler` の最後の要素の `compiler`。`hozo.compiler` は内部の op を持つので、入口のパッケージと、内側の `internal/gen/hozo/compiler/inner` に分かれる。`docs/plugin-go.md`「内側のフォルダと入口」。型は、`internal/gen/hozo/compiler/types` にある: 同「型のパッケージ」）、`internal/gen/hozo/compiler/compilertest`（基底 `go` の、テストのパッケージ: 七つの葉の op の、単体テストの口と、その、テストの対応表。`docs/plugin-go.md`「単体テストの口」。この口を満たすテストは、まだ書いていない: `docs/open.md`「サンプルと、hozoc 自身の仕様」）、コマンドの行の `hozo.cli` は、`internal/gen/hozo/cli`（基底 `go` の出力。パッケージ名は `cli`。内部の op `writeOrPreview` を持つので、ここも、入口のパッケージと、内側の `internal/gen/hozo/cli/inner` に分かれる。型は `internal/gen/hozo/cli/types` で、`go-valid` の `validate.go` も、そこ: 入力の `format` が、制約を持つ）、`internal/gen/hozo/cli/clitest`（にせもの。`docs/plugin-go.md`「テストのにせもの」。分類 `Flag` は、型で決まるので、述語も、分類の判定の口も、ない）、`internal/gen/hozo/cli/cli`（`go-cli`。パッケージ名は、これも `cli`: 手で書くコードは、名前空間のパッケージの方を、`hozocli` という名前で import する）。変えるときは `cli.hozo` を変えて `make generate`（`docs/packages.md`「自己生成の手順」）。生成物はコミットし、`tests/generated_test.go` と CI が遅れていないことを確かめる（`docs/testing.md`「生成物の一致」）。

基底が出す、実装を受け取る口（`docs/plugin-go.md`「op の置き場」「New」）と、それを満たすもの:

| 口 | メソッド | 実装（`internal/app`） |
|---|---|---|
| `compiler.Leaves` | `Check`、`CollectOutputs`、`VerifyOutputs`、`PlannedChanges`、`MatchTests`、`Serve`、`RunEmit`（葉の op。状態に触れない。七つとも、最初の引数に `ctx` を取り、最後の戻り値に `error` を返す: `docs/plugin-go.md`「op の形」） | `*app.App` |
| `compiler.SourcesState`（`sources: list<SourceFile>`） | `IsEmpty`、`Get` | `app.App` の `Sources()` が返す値: `sources` の glob に合うファイル |
| `compiler.ConfigState`（`config: nullable<SourceFile>`） | `IsEmpty`、`Get` | 同 `Config()`: `hozo.toml`（設定なしなら、ない） |
| `compiler.OutputsState`（`outputs: list<OutputFile>`） | `IsEmpty`、`Get`、`OutputChanges`（`read outputChanges` の宛先が、この状態） | 同 `Outputs()`: 出力先ごとの記録（`.hozoc-outputs.json`）に載っているファイル。`OutputChanges` は、書き出す予定を、その記録とディスクと比べる |
| `compiler.OutputsSyncState`（述語で決まる状態の分類 `OutputsSync`） | 述語 `UpToDate` と `OutOfDate`、命令 `SyncOutputs`（効果 `syncOutputs` の宛先が、この分類の遷移） | 同 `OutputsSync()`。`OutOfDate` は `UpToDate` の否定で、同じ比べ方を使う |
| `hozocli.HozoCompilerOps`（コマンドの行の配線が呼ぶ、ユースケースの op） | `Verify`、`Preview`、`Write`、`ConfirmTests`、`Serve` | `compiler.New` の戻り値（`compiler.Ops`）を、そのまま渡す |

`compiler.New(leaves, sources, config, outputs, outputsSync)` の引数は、この順（`Leaves`、状態の単位を宣言順に）。コマンドの行は、`hozocli.New(hozoCompiler)`（別の名前空間の op。フラグで道を分ける所は、分類 `Flag` の `match` で、生成した本体が持つ: 人が書く述語は、ない）。一つでも渡し忘れれば、コンパイルできない。口はどれもインターフェースなので、メソッドが一つでも足りない実装（`cli.hozo` に葉の op、`read`、効果を足して、`internal/app` に書いていない）も、渡すところでコンパイルできない。配線は、生成した本体が持つ。ユースケース（`internal/gen/hozo/compiler/inner/wiring.go`）: `verify` は、`Check`、`VerifyOutputs`。`preview` は、`Check`、`CollectOutputs`、`PlannedChanges`。`write` は、`Check`、`CollectOutputs`、`OutputChanges`、`SyncOutputs`。`confirmTests` は、`Check`、`MatchTests`。コマンドの行（`internal/gen/hozo/cli/inner/wiring.go`）: `build` は、`in.NoEmit` が真なら `Verify`、偽なら、内部の `WriteOrPreview`。`WriteOrPreview` は、`in.DryRun` が真なら `Preview`、偽なら `Write`（真偽の値の分類の `match` は、Go の `if` になる: `docs/plugin-go.md`「分類」）。`tests` は `ConfirmTests`、`lsp` は `Serve`。

- `cmd/hozoc/main.go`: `internal/app` の `App` から、上の表の五つを取り出して `compiler.New` に渡し（`app.App` の `Usecases()`）、その戻り値を、`hozocli.New` に渡し（`app.App` の `Ops()`）、その戻り値を、`Presenter` と一緒に `cli.Main`（`docs/plugin-go-cli.md`「出力」）に渡す。その前に、古い名前のコマンド（`check`、`emit`。「build」の「古い名前」）なら、`internal/app` の `Renamed` が返す文を、標準エラーに書いて、64 で終わる（`cli.hozo` に書けない、手で書く一か所: 仕様にないコマンドの名前は、`go-cli` の `Main` には、知らないコマンドとしか、見えない）。最初の引数の `ctx` は `context.Background()`。プロセスの環境を読むのはここだけ: 環境変数は `os.Getenv` を `cli.Main` に渡し（`cli.hozo` の `@cli.env` のフィールドに入る）、ホームディレクトリ（`os.UserHomeDir()`。プラグインの探索の `<ホーム>/.hozo/plugins`）は `app.App` の `Home` に入れる。それだけ。
- `internal/app`: 葉の op と、状態の単位の実装。`Check` は `internal/project.LoadWith`（プラグインの起動も、ここ: `internal/plugin.Runner`）、`CollectOutputs` は「書き出し」の手順の 1〜3（`Check` が受け取った答えを使い、プラグインは、起動しない。ファイルには触れない）、`VerifyOutputs` は、同じ 1〜3 を、確かめるためだけに行って、warning だけの結果を返す（設定なしなら、集めずに、検査の warning だけ）。`PlannedChanges` は、同 4 の、書く予定のパスを結果にするところ（ファイルには触れない）。`RunEmit` は下の項目、`Serve` は `internal/lsp.Serve`（`docs/packages.md`）。`MatchTests` は「tests」の手順の 1〜6: ファイルを集めるところは `CollectOutputs` と同じ関数、対応表の形と、突き合わせは `internal/testmap`、プラグインの起動は `internal/plugin.Runner`（置き場のファイルと、カバレッジのファイルを、読むだけ。状態 `outputs` は、境界に書いていない: 「cli.hozo」の `tests` の項目）。状態に触れるのは、状態の単位のメソッドだけ: `Outputs()` の `OutputChanges` は、同 4〜7 の、書くファイルと消すファイルを決めるところ（読むだけで、何も変えない。`written` は書く予定のすべて、`removed` は、記録にあって今回は書かないパスのうち、ディスクにあるもの）、`OutputsSync()` の `SyncOutputs` は、書き出すところ（消す、書く、記録を書く）。どちらも、呼ぶのは、ユースケース `write` だけ。`OutputsSync` の述語 `UpToDate` は、生成し直した内容とディスクの内容を比べて実装する。状態の単位の、形で決まる問い合わせ（`IsEmpty`、`Get`）は、作業ディレクトリから探した設定で読む（配線は呼ばない。テストが、効果の結果の判定に使う）。
- `Check` は、受け取った `ctx` で、プラグインを起動する: `ctx` が中断されていれば、次のプラグインを起動せずに、その `error` を返す。`cmd/hozoc` が渡す `ctx` は `context.Background()` で、中断されることはない。
- `runEmit` は、どの配線からも呼ばれない内部の op だが、基底は単位の対象の op をすべて出す（`docs/plugin-go.md`「出力の対象」）ので、`compiler.Leaves` のメソッド `RunEmit` になり、実装が要る。実装は、`plugin` を起動し、`req` を JSON にして送り、応答を `hozoc.EmitResponse` に読んで返す。`hozoc.Spec` と `hozoc.OutputView` は中身のない model なので、この入口から送る要求の `spec` と、出力の `view` は空のオブジェクトになる（プラグインとの境界の記述としての op で、hozoc 自身はこの入口を呼ばない）。`Check`（`internal/project`）は、同じ起動を、本物の仕様（`ir.Spec`、`ir.OutputView`）を持つ要求で、`internal/plugin.Runner` を直に使って行う。
- 境界の型は生成したもの（`compiler.CheckInput`、`compiler.Diagnostic`。中の `internal/diag.Diagnostic` は行と列に直して写す）で、例外は生成した exception の型を**値で**返す。`Crashed` / `MalformedOutput` も、`hozoc` の例外の型（`hozo.compiler.Crashed{}`）。`cli.hozo` の model は境界だけ（`docs/requirements.md`「自己展開」）なので、op をまたいで要る中身は `internal/app` の中に持つ: `check` が解析した `Project`（プラグインから受け取った答えを含む）と、書き出す予定の置き場（ルートと、出力ごとの Dir(U, N)。境界の `EmitPlan` は、warning と、ファイルだけ）。`OutputChanges`、`SyncOutputs`、`PlannedChanges` に渡す予定が、直前の `CollectOutputs` の返したものでなければ、誤り（宣言にない誤り。70）。
- **診断を運ぶ六つの例外**（`CheckFailed`、`ConfigInvalid`、`LanguageMismatch`、`OutputConflict`、`TestsNotRun`、`CoverageUnusable`。生成した構造体は、引数 `diagnostics` のフィールドを一つ持つ。`docs/plugin-go.md`「exception」）を作るのと、その診断を取り出すのは、`internal/app/exceptions.go` の関数だけ。引数の名前を書くのは、そのファイルの一か所（六つの構造体と同じ形の型 `carried` と、そのフィールドを読む一行）で、`cli.hozo` の引数の名前を変えたら、そこを合わせる（合っていなければ、コンパイルできない）。ほかのコードとテストは、名前を書かない。
- 内部の op（`check`、`collectOutputs`、`verifyOutputs`、`plannedChanges`、`matchTests`、`runEmit`）のテストは、内側のパッケージ `internal/gen/hozo/compiler/inner` の `New` の戻り値から呼ぶ。`build` が、フラグの組から、入るユースケースを分けること（「build」の表）は、生成した本体に、呼ばれたことを覚える、ユースケースの代わりを渡して、フラグの四つの組で確かめる（`internal/app/buildflags_test.go` の `TestBuildFlags`）。
- `Presenter`（`cli.Presenter` の実装）: コマンドの入力（`hozo.cli` の `BuildInput` / `TestsInput`）の `format` に応じて text か JSON（表示は、コマンドの行の側の仕事なので、フラグ `format` と `noEmit` を、ここで、コマンドの入力から読む。ユースケースの結果は、検査だけでも、`BuildResult`）（`docs/diagnostics.md`「表示」。診断の text と json の形は `internal/diag` が持ち、`Presenter` は `cli.hozo` の model `Diagnostic` をその形に写して呼ぶ）。結果は標準出力、例外は標準エラー（`docs/plugin-go-cli.md`「実行と表示」）。text の `CheckResult` は診断の一覧と件数、`EmitResult` は warning の一覧（あれば）に続けて `written: <path>` / `removed: <path>` を一行ずつ、`TestsResult` は warning の一覧（あれば）に続けて `ran: <トランスパイラの名前>: <テストの名前>` を一行ずつと、最後に件数（`3 tests ran`）、診断を運ぶ例外はその一覧と件数、それ以外の例外は `hozoc: <Failure.Err のメッセージ>`。json の `CheckResult` は `{"warnings": [...]}`、`EmitResult` は `{"warnings": [...], "written": [...], "removed": [...]}`、`TestsResult` は `{"warnings": [...], "ran": [...]}`、例外は `{"error": "<FQN>", "value": {...}}` で、診断を運ぶ例外の `value` は `docs/diagnostics.md`「json」の形（`{"diagnostics", "errors", "warnings"}`）、それ以外は `cli.JSONPresenter` と同じ。診断の要素は `docs/diagnostics.md`「json」と同じ形。診断を運ぶ例外の `value` を書くのは `Presenter` で、キー `diagnostics` は `docs/diagnostics.md`「json」のもの（`internal/diag` の `Report`）: 例外の引数の名前から作るのではない（`cli.JSONPresenter` なら、引数の名前をキーにしたオブジェクトで、件数の `errors` / `warnings` を持たない。`docs/plugin-go-cli.md`「実行と表示」）。引数の名前が同じ `diagnostics` なので、二つの形は、キー `diagnostics` で重なる。
- `hozo.toml` がないとき、`check` と、ユースケース `verify` は、設定なしで検査し、`preview`、`write`、`confirmTests` は CFG-7 を一つ作って `ConfigInvalid` にする（`docs/config.md`「置き場と探し方」）。

この節の細部は、決定 D649、D742、D753、D754、D770 が決めた。
