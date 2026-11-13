# Go のパッケージ

## この文書が決めること

モジュール `github.com/mew-ton/hozo` のディレクトリとパッケージの切り方、各パッケージの責務と主な API、依存の向き。リポジトリの構成の持ち主はこの文書。

## ディレクトリ

```
go.mod                 module github.com/mew-ton/hozo
hozo.toml, cli.hozo    hozoc 自身の仕様と設定（docs/cli.md）
Makefile               build / generate（「自己生成の手順」）/ vet / test
cmd/hozoc/             CLI の main。app.App の Ops()（hozoc.States に実装を入れて hozoc.New）と app.Presenter を、生成した cli.Main に渡すだけ
ir/                    IR の型（公開）
plugin/                Go 向けプラグイン SDK（公開）。testdata/ に、二つの SDK のテストが読む表
plugin/golang/         Go の名前と型の規則（公開）
runtime/               生成した Go のコードが import する型（公開）
vocab/                 標準の語彙の .hozo と、それを embed する公開パッケージ vocab（go:embed はパッケージのディレクトリの外を参照できないため）
internal/source/       ファイルと位置
internal/diag/         診断
internal/syntax/       字句・構文解析（token/、ast/ は字句と構文木）
internal/sema/         解決、型、正規化
internal/check/        検査
internal/config/       hozo.toml
internal/vocab/        埋め込んだ標準の語彙
internal/emit/         言語ごとの既定の出力の置き場（このパッケージ自体はコードを持たない）
internal/emit/golang/  Go の既定の出力
internal/plugin/       プラグインの探索と起動
internal/project/      一度の解析のすべて
internal/app/          hozoc の葉の op の実装と表示
internal/lsp/          言語サーバー（jsonrpc/ は JSON-RPC、protocol/ は LSP の型のうち使うもの）
internal/gen/hozoc/    cli.hozo からの生成物（既定の出力。cli/ は go-cli の出力）
internal/tools/        生成の道具: irschema/ と tmgrammar/（go generate）、selfgen/（make generate）
plugins/<name>/        プラグイン。一つのディレクトリが一つの実行ファイルと hozo-plugin.toml（提供するトランスパイラの一覧を持つ）。Go なら main パッケージ、TS なら npm パッケージ
plugins/go-cli/        go-cli プラグイン（main。中身は internal/gocli/）と、その語彙 cli.hozo
sdk/ts/                TS 向けプラグイン SDK
schema/                IR の JSON スキーマ（go generate の置き場のパッケージ schema）
editors/vscode/        VS Code 拡張（syntaxes/ は TextMate の文法と、go generate の置き場のパッケージ syntaxes）
tests/                 リポジトリをまたぐテスト（パッケージ tests。テストのファイルだけ）
tests/rules/, tests/corpus/   テストの入力（docs/testing.md）
tests/plugins/pick/, tests/corpus/view/plugins/view-dump/   テスト用のプラグイン（main パッケージ。docs/testing.md「コーパス」）
docs/                  設計文書
```

## 依存の向き

Go のパッケージごとの、このモジュールの中で import しているパッケージ（テストでないファイル）。この表が持ち主で、実際の import と同じであることを `tests/deps_test.go` が確かめる。import を足すときは、足してよい向きかを下の規則で確かめてから、この表にも足す。

| パッケージ | import する |
|---|---|
| `cmd/hozoc` | `internal/app`、`internal/gen/hozoc/cli` |
| `internal/app` | `internal/config`、`internal/diag`、`internal/emit/golang`、`internal/gen/hozoc`、`internal/gen/hozoc/cli`、`internal/lsp`、`internal/plugin`、`internal/project`、`internal/source`、`ir`、`plugin` |
| `internal/lsp` | `internal/diag`、`internal/lsp/jsonrpc`、`internal/lsp/protocol`、`internal/project`、`internal/sema`、`internal/source`、`internal/syntax`、`internal/syntax/ast`、`internal/vocab`、`ir` |
| `internal/lsp/jsonrpc` | — |
| `internal/lsp/protocol` | — |
| `internal/project` | `internal/check`、`internal/config`、`internal/diag`、`internal/plugin`、`internal/sema`、`internal/source`、`internal/syntax`、`internal/syntax/ast`、`internal/vocab`、`ir`、`plugin` |
| `internal/plugin` | `internal/config`、`internal/diag`、`ir`、`plugin` |
| `internal/emit` | — |
| `internal/emit/golang` | `internal/diag`、`ir`、`plugin`、`plugin/golang`、`runtime` |
| `internal/check` | `internal/config`、`internal/diag`、`internal/source`、`ir` |
| `internal/sema` | `internal/diag`、`internal/source`、`internal/syntax/ast`、`ir` |
| `internal/syntax` | `internal/diag`、`internal/source`、`internal/syntax/ast`、`internal/syntax/token` |
| `internal/syntax/ast` | `internal/source` |
| `internal/syntax/token` | `internal/source` |
| `internal/config` | `internal/diag` |
| `internal/diag` | `internal/source` |
| `internal/source` | — |
| `internal/vocab` | `vocab` |
| `internal/gen/hozoc` | `runtime` |
| `internal/gen/hozoc/cli` | `internal/gen/hozoc`、`runtime` |
| `internal/tools/irschema` | `ir`、`plugin` |
| `internal/tools/tmgrammar` | `internal/syntax/token` |
| `internal/tools/selfgen` | — |
| `ir` | — |
| `plugin` | `ir` |
| `plugin/golang` | `ir` |
| `runtime` | — |
| `vocab` | — |
| `plugins/go-cli` | `plugin`、`plugins/go-cli/internal/gocli` |
| `plugins/go-cli/internal/gocli` | `ir`、`plugin`、`plugin/golang` |
| `tests/plugins/pick` | `ir`、`plugin` |
| `tests/corpus/view/plugins/view-dump` | `plugin` |
| `schema` | — |
| `editors/vscode/syntaxes` | — |
| `tests` | — |

- `internal/` は `cmd/hozoc` と `internal/` の中からだけ使う。例外は、リポジトリをまたぐテストのパッケージ `tests` だけ（テストのファイルから `internal/app`、`internal/project` などを import する。`tests` にテストでないファイルはない）。
- プラグインが使えるのは `ir`、`plugin`、`plugin/golang`、`runtime`（と、自分のディレクトリの下のパッケージ）。テスト用のプラグインも同じ。依存先を約束した形だけにするため（§4.3「どのプラグインも既定のインターフェースに依存する」と同じ考え方）。
- 公開のパッケージ（`ir`、`plugin`、`plugin/golang`、`runtime`、`vocab`）は `internal/` に依存しない。
- 解析（`internal/project`）は出力（`internal/emit/golang`）に依存しない。解析した結果から出力を作って書き出すのは `internal/app`。言語サーバーは `internal/project` までで、出力もプラグインの起動（`emit`）も行わない。
- `internal/sema` は設定（`internal/config`）に依存しない。設定に関わる検査は `internal/config`、`internal/check`、`internal/project` が行う（「`internal/config`」）。
- `internal/tools/selfgen` はこのモジュールのどのパッケージも import しない（生成物が壊れていても動くため。「自己生成の手順」）。
- 外部のモジュールは `github.com/BurntSushi/toml` だけ（`internal/config` と `internal/plugin` が使う）。依存を最小にするため、言語サーバーの JSON-RPC も自前で書く。

## パッケージ

### `ir`（公開）

`docs/ir.md` の型。`encoding/json` のタグはこの文書のフィールド名。`Program` / `View` と、`Type` などの判別付きユニオンは `kind` で分ける（Go では interface と `UnmarshalJSON`）。

```go
func (p *Program) Lookup(fqn string) Decl          // 宣言を引く。なければ nil
func (p *Program) Namespace(fqn string) *Namespace
func (v *View) Signature(op string) *ViewedSignature
func (v *View) InUnit(fqn string) bool             // viewer の名前空間の宣言で、単位の対象か（span.file が View.files にある。docs/config.md「in」）
```

TS の SDK（`sdk/ts`）は同じ引き方を関数で持つ: `lookup(program, fqn)`、`namespace(program, fqn)`、`signature(view, op)`、`inUnit(view, fqn)`（なければ `undefined`）。

### `plugin`（公開）

プラグインの SDK。JSON の形は `docs/plugin-protocol.md`「要求と応答」で、型はそれを写す。

```go
type Handler interface {
    Emit(req *EmitRequest) (*EmitResponse, error)
    Check(req *CheckRequest) (*CheckResponse, error)
    Import(req *ImportRequest) (*ImportResponse, error)
}
type EmitRequest struct { Protocol string; Direction string; Transpiler string; View *ir.View }   // Transpiler は起動されたトランスパイラの名前
type EmitResponse struct { Files []File; Warnings []Warning; Errors []Diagnostic; Select Select }
type Select struct { Roots, Stops, States []string; Wraps []Wrap }   // docs/plugin-protocol.md「選択」
type Wrap struct { Op, Decorator string }
func OpsWith(view *ir.View, decorator string) []string             // view.viewer の op のうち、単位の対象（View.InUnit）で、そのデコレータ（FQN。via も見る）が付いたものの FQN
// CheckRequest / CheckResponse / ImportRequest / ImportResponse / File / Warning / Diagnostic も同じ形

// Main は標準入力の要求を読み、protocol を確かめ、h を呼び、標準出力に応答を書く。終了コードを返す
// （docs/plugin-protocol.md「要求と応答」「起動」。文字列でない protocol も「合わない」で code "protocol"。TS の run も同じ）。
func Main(h Handler) int
func Run(h Handler, in io.Reader, out, errOut io.Writer) int   // Main の中身。テスト用
// 実装しない方向と、Emit で知らない Transpiler は ErrUnsupported を返す。Main は docs/plugin-protocol.md「起動」の code "unsupported" で終わる
var ErrUnsupported error
```

### `plugin/golang`（公開）

`docs/emit-go.md`「名前」「型」の規則。既定の出力と Go のプラグインが共有する。公開の関数はここに並べたものがすべて。

```go
func TypeName(fqn, namespace string) string       // namespace は宣言した名前空間。合成 model（`.` と `$`）の連結を含む
func FieldName(name string) string
func MethodName(opName string) string
func LocalName(name string) string                // 引数名・束縛名。Go の予約語か事前宣言の識別子なら末尾に _
func EffectConst(name string) string
func CaseConst(classify, caseName string) string
func GoType(t ir.Type, imports *Imports) string   // 型の文字列。必要な import を imports に足す
func IsNillable(t ir.Type) bool                   // Go の零値 nil が「なし」を表せる型か（array、record）。optional(T) を *T にしない型
func Capitalize(s string) string                  // 先頭の一文字を大文字に（上の名前の規則の部品）
func IsKeyword(name string) bool                  // Go の予約語か
func IsPredeclared(name string) bool              // Go の事前宣言の識別子（error、string、nil、len など）か
type Resolver func(fqn string) (namespace, pkgPath, pkgName string, local bool)   // ref の行き先が同じパッケージか、どのパッケージか
type Imports struct{ Viewer string; Resolve Resolver }   // import パスと別名の管理。Add / Name / Paths / Render()
func PackageName(namespace string) string
func ImportPath(root, out string) (string, error) // go.mod を探す。見つからなければ ErrNoGoMod（GOEMIT-1）
var ErrNoGoMod error
```

### `runtime`（公開）

`docs/emit-go.md`「runtime パッケージ」。

### `internal/source`

ファイルの集合と位置。

```go
type FileSet struct{ ... }
func (fs *FileSet) Add(path string, content []byte) FileID
type Span struct { File FileID; Start, End Offset }      // バイトのオフセット
func (fs *FileSet) Position(FileID, Offset) Pos          // 1 始まりの行・列（コードポイント）
func (fs *FileSet) Offset(FileID, Pos) Offset            // Position の逆。行の先頭は Add が作った行の表から引く（行数によらず一定の手間）
func (fs *FileSet) Path(FileID) string
func (fs *FileSet) Content(FileID) []byte
func (fs *FileSet) Len() int
```

IR の `Span`（行・列）は `Position` で作る。構文木はバイトのオフセットで持つ。行・列とオフセットの変換はこのパッケージだけが持つ: IR の `Span` やプラグインの `"file:line:col"` を診断の位置に戻すとき（`internal/check` の `FileSetLocator`、`internal/project` の `Locate`）も `Offset` を使い、自分では数えない。

### `internal/diag`

`docs/diagnostics.md` の `Diagnostic`、`Severity`、並べ替え（`Sort`）、text と JSON の表示。表示の形の持ち主はこのパッケージだけで、ほかのパッケージは診断を自分で文字列にしない。

```go
type Rendered struct { Severity, Code, Message, File string; Start, End *RenderedPos; Related []RenderedRelated; Data map[string]any }
                                                         // 表示する形にした診断（位置は行と列）。JSON にすると docs/diagnostics.md「json」の要素
type Report struct { Diagnostics []Rendered; Errors, Warnings int }   // 同「json」の全体
func Render(diags []Diagnostic, fs *source.FileSet) []Rendered
func NewReport(diags []Rendered) Report
func WriteRenderedText(w io.Writer, diags []Rendered) error           // 同「text」
func WriteText(w io.Writer, diags []Diagnostic, fs *source.FileSet) error   // Render して WriteRenderedText
func WriteJSON(w io.Writer, diags []Diagnostic, fs *source.FileSet) error   // Render して NewReport
func Count(diags []Diagnostic) (errors, warnings int)
```

`internal/app` の `Presenter` は、`cli.hozo` の model `Diagnostic`（位置は既に行と列）を `Rendered` に写して `WriteRenderedText` と `Report` を使う（`docs/cli.md`「手で書くもの」）。

### `internal/syntax/token`、`internal/syntax/ast`、`internal/syntax`

`docs/grammar.md`。

```go
// token
type Kind int                        // 予約語、記号、Ident、リテラル、DocComment、EOF
var Keywords map[string]Kind         // 予約語の一覧（docs/grammar.md「予約語」）。docs/testing.md の一致テストの元
var Contextual = []string{"field", "variant", "post", "put", "delete"}

// ast: 文法の非終端記号ごとの構造体。すべて Span を持つ。DocComment は付いた先の節が持つ
// syntax
func Parse(fs *source.FileSet, f source.FileID) (*ast.File, []diag.Diagnostic)
```

パーサーは再帰下降。誤りからの回復は、宣言の中なら次の `;` か `}` まで、ブロックの中なら次の `;` まで読み飛ばし、`SYNTAX` を一つ出して続ける。読めた宣言は `ast.File` に残す（`docs/requirements.md`「言語サーバー」の前提条件）。

### `internal/sema`

解決（`docs/resolve.md`）、型（`docs/types.md`）、正規化（`docs/ir.md`）。

```go
type Result struct {
    Program *ir.Program
    Refs    []Ref            // トークンの Span → 解決先。定義へジャンプとホバーが使う
    Diags   []diag.Diagnostic
}
type Ref struct { Span source.Span; Target Target }
type Target struct { Kind TargetKind; Decl string; Name string; Span source.Span }
// TargetKind: 宣言、フィールド、ユニオンのアーム、classify のケース、デコレータの Param、束縛、op の引数、名前空間
func Analyze(fs *source.FileSet, files []*ast.File) *Result   // Op.Signature と Call.Bound は空のまま（internal/check が埋める）。設定は見ない
```

解決と型の誤りがあっても `Program` は作る。作れなかった宣言は落とす。解決できなかった参照（型、`throws` / `effects` の要素、デコレータの適用、呼び出しなど）は、IR に残さず、その参照を落とす（診断は RES-* で出ている）。未解決の印を IR に置くことはしていない。

### `internal/check`

`docs/check.md`。

```go
func Compute(p *ir.Program)                                  // Signature と Call.bound を埋める
type Locator func(ir.Span) (source.Span, bool)               // IR の行・列を FileSet のバイトの位置に戻す
func FileSetLocator(fs *source.FileSet) Locator
type Checker struct { Locate Locator }
func (c Checker) Check(p *ir.Program, cfg *config.Config) []diag.Diagnostic   // Compute を呼んでから規則を検査する。Locate が nil なら Span は nil で File だけ
func (c Checker) CheckUnsupported(p *ir.Program, ns string, files []string, unsupported []string) []diag.Diagnostic   // CFG-6。files は単位の in に合ったファイル、unsupported はマニフェストから
type Unit struct { Language string; Files []string }         // 設定の外で決まる単位の情報: 言語（マニフェストから）と、in に合ったファイル
func Views(p *ir.Program, cfg *config.Config, units []Unit) []map[string]*ir.View   // 単位ごと（cfg.Units と同じ順）の、出力する名前空間 → View
```

設定に関わる検査のうち、プログラムが要る CFG-8 と CFG-10 は `Check` が（`cfg` を見て）出す。CFG-6 は `CheckUnsupported`。

規則ごとに一つのファイル（`throws.go`、`bind.go`、…）。各関数のコメントに規則の ID。

### `internal/config`

`docs/config.md`。

```go
type Config struct { Root, Path string; Hozo HozoTable; Units []Unit }
type Unit struct { Index int; In []string; Namespace string; Transpilers []string; Out string }   // [[emit]] 一つ。Index は何番目か（1 始まり）。In は省略なら nil
func Load(path string) (*Config, []diag.Diagnostic)        // CFG-7 / CFG-12 / CFG-17 はここで出る。Config は常に返す（既定値で補う。CFG-7 か CFG-12 の単位は Units に入れない）
func Find(startDir string) (string, bool)
func Default(root string) *Config                           // hozo.toml がないとき（Project.NoConfig）
func (c *Config) Covering(namespace string) []int           // 名前空間を対象にする単位（Units の添字、設定の順）。CFG-13（docs/config.md「単位の対象」）
func (c *Config) Dir(unit int, namespace string) string     // Dir(U, N)。ルートからの相対パス
func (c *Config) UnitNamespaces() []string                  // 単位の namespace（重複なし、辞書順）。CFG-8 / CFG-10 のため
func CleanRel(p string) string                              // ルートからの相対パスを比べられる形に（/ 区切り、./ や余分な / を除く。ルートは ""）
func Under(a, dir string) bool                              // 相対パス a が dir の下（真に深い）か。CFG-17、PLG-1、.hozoc-outputs.json の判定
```

ルートからの相対パスの正規化と「下にあるか」の判定はこのパッケージだけが持つ（`out`、`plugin_dirs`、glob、書き出すファイルのパス、`.hozoc-outputs.json` のパスのどれも `CleanRel` を通す）。

設定に関わる検査（CFG-*）を出すパッケージは、何が要るかで分かれる:

| 規則 | 出すパッケージ | 要るもの |
|---|---|---|
| CFG-7、CFG-12、CFG-17 | `internal/config`（`Load`） | `hozo.toml` だけ |
| CFG-8、CFG-10 | `internal/check`（`Check`） | プログラム（名前空間と、その `state` / `op`） |
| CFG-6 | `internal/check`（`CheckUnsupported`。呼ぶのは `internal/project`） | プログラムと、マニフェストの `unsupported` |
| CFG-11、CFG-14〜CFG-16、CFG-18 | `internal/project` | マニフェストと、読んだファイル |
| CFG-5 と、設定なしで `emit` したときの CFG-7 | `internal/app` | 書き出すファイルの一覧 |

設定なし（`Project.NoConfig`）のときは、`internal/project` が CFG-* をまとめて除く。

### `internal/vocab`

`docs/vocab.md`。公開パッケージ `vocab` が `embed` した `vocab/*.hozo` を、`Files() []File` と `AddTo(func(path string, content []byte))` で渡す。パスは `hozo:vocab/<名前>.hozo`。プラグインの語彙は `PluginFiles(plugin, dir, rels)` / `AddPluginTo` で `hozo:plugin/<名前>/<相対パス>`。

### `internal/emit/golang`

`docs/emit-go.md`。

```go
type File struct { Path string; Content []byte }            // Path はルートからの相対パス（View.config.dir の下）
type Options struct {
    Root     string
    Select   plugin.Select       // 単位のトランスパイラの select を合わせたもの（docs/emit-go.md「出力の対象」）
    Exported map[string]string   // 別の名前空間の型の FQN → それを持つ Go の出力のディレクトリ（Dir(U', M)）。ここにある型は import し、ない型は複製する（docs/emit-go.md「配置」）
}                                                            // どの単位の出力がその型を持つかは、呼ぶ側（internal/app）が Exports から決める
func Emit(view *ir.View, opts Options) ([]File, []diag.Diagnostic)   // error があればファイルは返さない。何も選ばれていなければ何も返さない
func Exports(view *ir.View, sel plugin.Select) []string     // sel から決まる出力の対象のうち、view.viewer の名前空間の型の FQN
func Uses(view *ir.View, sel plugin.Select) []string        // 出力の対象が型を参照している、別の名前空間
func Imports(view *ir.View, opts Options) []string          // Emit が作るファイルが import する、別の出力のディレクトリ（Exported の値）。GOEMIT-5 の判定は呼ぶ側
```

### `internal/plugin`

ホスト側。`docs/plugin-protocol.md`。

```go
type Manifest struct{ ...; Transpilers []Transpiler }
type Transpiler struct { Name, Language string; Unsupported []string }                   // [[transpiler]] 一つ
func LoadManifest(path string) (*Manifest, []diag.Diagnostic)                           // MAN-2 / MAN-4
type Plugin struct { Name, Dir string; Manifest *Manifest; Mismatch []diag.Diagnostic } // 入っているプラグイン一つ。Dir は絶対パス。Mismatch は MAN-2（使うときにだけ報告する）
type SearchEnv struct { PluginPath, Home string }                                       // 探索が見るプロジェクトの外の場所（HOZO_PLUGIN_PATH の値、ホーム）。呼ぶ側が渡す。零値なら plugin_dirs だけ
func Discover(cfg *config.Config, env SearchEnv) ([]*Plugin, []diag.Diagnostic)         // 入っているプラグインのすべて（docs/plugin-protocol.md「探索」の順）。MAN-3 / MAN-4。プロセスの環境は読まない
type Runner struct { Root string; Stderr io.Writer; Env []string }
func (r Runner) Run(m *Manifest, dir string, req any, resp any) error                   // 起動して JSON をやり取り（docs/plugin-protocol.md「起動」）。resp が *sdk.EmitResponse か *sdk.CheckResponse なら応答の形も確かめる（同「応答の形」）
var ErrCrashed, ErrMalformedOutput error                                                // errors.Is で判定。*PluginError は exit 2 の code を持ち、MAN-2 / MAN-5 の診断にする
func CheckSelect(plugin string, view *ir.View, sel sdk.Select) error                    // emit の応答の select を確かめる。誤りは PLG-3（ErrMalformedOutput）。sdk は公開の plugin パッケージ
func MergeSelect(sels ...sdk.Select) sdk.Select                                         // 単位のトランスパイラの select を合わせる
```

### `internal/project`

一度の解析のすべて。CLI と言語サーバーの共通の入口。

```go
type Project struct {
    Config  *config.Config          // 設定なしなら既定値で、NoConfig が true
    NoConfig bool
    Files   *source.FileSet
    Sema    *sema.Result
    Plugins []*plugin.Plugin         // 入っているプラグイン（internal/plugin.Discover）
    Units   []Unit                   // 解決した単位（Config.Units と同じ順）
    Diags   []diag.Diagnostic        // 解析・解決・検査・設定・プラグインの check のすべて。並べ替え済み
}
type Unit struct { *config.Unit; Language string; Transpilers []Transpiler; Files []string }   // 言語、解決できたトランスパイラ、in に合ったファイル
type Transpiler struct { Name string; Plugin *plugin.Plugin; Decl *plugin.Transpiler }
type Options struct {
    Root, ConfigPath string          // hozo.toml を探し始めるディレクトリと、--config / HOZO_CONFIG
    Overlay map[string][]byte        // ディスクの代わりに使う内容（言語サーバーの開いている文書）
    Stderr io.Writer; Env []string   // プラグインの標準エラーの流し先と、プラグインに渡す環境変数の元
    PluginPath, Home string          // プラグインの探索が見るプロジェクトの外の場所（internal/plugin.SearchEnv）。空なら探さない
}
func LoadWith(opts Options) *Project                                            // 解析の本体。NoConfig なら CFG-* の診断を除く
func Load(root string, configPath string, overlay map[string][]byte) *Project   // LoadWith の短い形。プラグインは plugin_dirs だけから探す
func (p *Project) Errors() int
func (p *Project) Views() []map[string]*ir.View                                 // 単位ごとの、出力する名前空間 → View（internal/check.Views）
func (p *Project) Covered(unit int) []string                                    // 単位の対象の名前空間（Program の順。in で出力しなくなったものも含む）
func (p *Project) ManifestPath(pl *plugin.Plugin) string                        // 診断の File に使う形のマニフェストのパス
func (p *Project) Locate(at string) (*source.Span, string)                      // プラグインの at（FQN か "file:line:col"）を診断の位置に
func (p *Project) PluginDiagnostic(plugin string, d sdk.Diagnostic) diag.Diagnostic   // プラグインの診断（check の diagnostics、emit の errors）を診断に
func (p *Project) PluginWarning(transpiler string, w sdk.Warning) diag.Diagnostic     // トランスパイラの warning を PLUGIN の診断に
```

`hozoc` の `check` / `emit`（`internal/app`）と言語サーバー（`internal/lsp`）は `LoadWith` を呼ぶ。`Project` は `Root`、`Sources`、`RunErr`（プラグインの `check` が `proc.Crashed` / `proc.MalformedOutput` で終わったときの誤り）も持つ。`Covered` 以下の五つは、`internal/app` が `emit` の手順（`docs/cli.md`「emit」）で使うためのもの。

### `internal/app`

`docs/cli.md`「手で書くもの」。葉の op の実装を持つ `App`（`Leaves()` が生成した `hozoc.Leaves` を、`States()` が `hozoc.StateSet` を返し、`Ops()` が `hozoc.States` に入れて `hozoc.New` を呼ぶ。`hozoc.States` に入れるのはここだけ）と、`cli.Presenter`（`internal/gen/hozoc/cli`）の実装 `Presenter`。引数の解釈は持たない（生成した `cli.Main`）。`App` はプロセスの環境を自分で読まない: ホームディレクトリは `App.Home`（`cmd/hozoc` が入れる）、環境変数は `cli.hozo` の入力のフィールドで届く。

### `internal/lsp`

`docs/lsp.md`。`func Serve(in io.Reader, out io.Writer, opts Options) error`。`Options` は `Root`、`ConfigPath`、`Log` と、`internal/project.Options` にそのまま渡す `Env`、`PluginPath`、`Home`。`internal/lsp/jsonrpc`（`Content-Length` 付きの JSON-RPC 2.0 の読み書き）と `internal/lsp/protocol`（LSP の型のうち使うもの）は、このパッケージの中だけで使う。

### `plugins/go-cli`

`docs/plugin-go-cli.md`。`main` パッケージ。`plugin.Main(gocli.Handler{})`。中身（View の読み取り、CLI-* の検査、`cli.go` の生成）は `plugins/go-cli/internal/gocli`。

## 生成するもの

| 元 | 生成物 | 道具 |
|---|---|---|
| `ir` と `plugin` の型 | `schema/ir.schema.json`（`Program` / `View` と、`$defs` にプロトコルの要求と応答） | `internal/tools/irschema`（`go generate ./schema/...`。構造体は reflect で写し、`kind` のユニオンと列挙は道具の表で持つ） |
| `schema/ir.schema.json` | `sdk/ts/src/ir.ts`（IR の型） | `json-schema-to-typescript`（`sdk/ts` の `npm run generate`） |
| `token.Keywords` | `editors/vscode/syntaxes/hozo.tmLanguage.json`（予約語の部分は `token.Keywords` から、それ以外は道具の中の表から） | `internal/tools/tmgrammar`（`go generate ./editors/...`） |
| `cli.hozo` | `internal/gen/hozoc/**` | `internal/tools/selfgen`（`make generate`。下の「自己生成の手順」） |

生成物はコミットする。CI で生成し直して差分がないことを確かめる（`docs/testing.md`「生成物の一致」）。

生成せずに手で写すものは、写しが合っていることをテストで確かめる: プロトコルの要求と応答の TS の型 `sdk/ts/src/protocol.ts`（スキーマの `$defs` を写す。項目の名前、必須かどうか、型が合っていることを `sdk/ts/test/protocol.test.ts` が確かめる）と、`docs/cli.md` に埋め込んだ `cli.hozo` と `hozo.toml`（`docs/testing.md`「コーパス」）。

### 自己生成の手順

`internal/gen/hozoc` は `cmd/hozoc` が import するパッケージなので、hozoc がそこへ直に書くと、生成器の不具合でコンパイルできない Go が出たときに hozoc 自身がビルドできなくなり、生成し直すこともできなくなる。そうならないように、`make generate`（`go run ./internal/tools/selfgen`）は、置き場に、コンパイルできると確かめたものしか置かない。

1. 今のソース（今の `internal/gen/hozoc` を含む）で `hozoc` を一時ディレクトリにビルドする。
2. 別の一時ディレクトリに `cli.hozo`、`hozo.toml`、`go.mod`、`go.sum` を写し、go-cli を `go run` で動かすのに要る `ir`、`plugin`、`plugins`、`runtime` をシンボリックリンクで足して、そこで 1 の `hozoc emit` を走らせる（`tests/generated_test.go` と同じ組み立て。`go.mod` が同じなので import パスも同じになる）。プラグインの探索が見るプロジェクトの外の場所（`HOZO_PLUGIN_PATH`、ホーム）は空にして、生成物が実行する人の環境に左右されないようにする。
3. 生成した `internal/gen/hozoc/**` を、その一時ディレクトリで `go vet` に通し、次に、リポジトリの今のものの代わりに置いたつもりで（`go build -overlay`。ディスクの上の置き場には触れない）、それを import するパッケージ（`cmd/hozoc`、`internal/app`）ごと `go build` する。
4. 3 が通ったときだけ、`internal/gen/hozoc` の中身を生成したものに置き換える（なくなったファイルは消し、同じ内容のファイルには触れない）。通らなければ何も変えず、生成したものを残した一時ディレクトリの場所を示して失敗する。

- 生成は不動点: 生成し直した直後にもう一度 `make generate` しても、何も変わらない。置き換えた後の `hozoc emit` も同じ内容を書く。
- リポジトリのルートで `go run ./cmd/hozoc emit` を直に走らせれば、確かめずに置き場へ書く。生成器を変えている間は `make generate` を使う。
- 置き場が壊れて 1 のビルドが通らないときは、`internal/gen/hozoc` を git で元に戻してからやり直す。`internal/tools/selfgen` 自身はこのモジュールのどのパッケージも import しないので、そのときも動く。
