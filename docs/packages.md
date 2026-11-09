# Go のパッケージ

## この文書が決めること

モジュール `github.com/mew-ton/hozo` のディレクトリとパッケージの切り方、各パッケージの責務と主な API、依存の向き。リポジトリの構成の持ち主はこの文書。

## ディレクトリ

```
go.mod                 module github.com/mew-ton/hozo
hozo.toml, cli.hozo    hozoc 自身の仕様と設定（docs/cli.md）
cmd/hozoc/             CLI の main。hozoc.New(app.Leaves) と app.Presenter を、生成した cli.Main に渡すだけ
ir/                    IR の型（公開）
plugin/                Go 向けプラグイン SDK（公開）
plugin/golang/         Go の名前と型の規則（公開）
runtime/               生成した Go のコードが import する型（公開）
internal/source/       ファイルと位置
internal/diag/         診断
internal/syntax/       字句・構文解析と構文木（token、ast）
internal/sema/         解決、型、正規化
internal/check/        検査
internal/config/       hozo.toml
internal/vocab/        埋め込んだ標準の語彙
internal/emit/golang/  Go の既定の出力
internal/plugin/       プラグインの探索と起動
internal/project/      一度の解析のすべて
internal/app/          hozoc の葉の op の実装と表示
internal/lsp/          言語サーバー
internal/gen/hozoc/    cli.hozo からの生成物（段階 3 から）
internal/tools/        go generate の道具
plugins/<name>/        プラグイン。一つのディレクトリが一つの実行ファイルと hozo-plugin.toml。Go なら main パッケージ、TS なら npm パッケージ
sdk/ts/                TS 向けプラグイン SDK
schema/                IR の JSON スキーマ
vocab/                 標準の語彙の .hozo と、それを embed する公開パッケージ vocab（go:embed はパッケージのディレクトリの外を参照できないため）
editors/vscode/        VS Code 拡張
tests/rules/, tests/corpus/   テストの入力（docs/testing.md）
docs/                  設計文書
```

## 依存の向き

```
cmd/hozoc → internal/gen/hozoc/cli → internal/gen/hozoc → runtime
cmd/hozoc → internal/app → internal/gen/hozoc
                         → internal/project → internal/{config, vocab, syntax, sema, check, emit/golang, plugin}
                                            → ir, plugin/golang, runtime
internal/lsp → internal/project, internal/syntax（シンボルと定義へのジャンプのために構文木を読み直す）
internal/vocab → vocab
plugins/go-cli → plugin, plugin/golang, ir
ir → (標準ライブラリだけ)
plugin → ir
runtime → (標準ライブラリだけ)
```

- `internal/` は `cmd/hozoc` と `internal/` の中からだけ使う。
- プラグインが使えるのは `ir`、`plugin`、`plugin/golang`、`runtime`。依存先を約束した形だけにするため（§4.3「どのプラグインも既定のインターフェースに依存する」と同じ考え方）。
- 外部のモジュールは `github.com/BurntSushi/toml` だけ（`internal/config` と `internal/plugin` が使う）。依存を最小にするため、言語サーバーの JSON-RPC も自前で書く。

## パッケージ

### `ir`（公開）

`docs/ir.md` の型。`encoding/json` のタグはこの文書のフィールド名。`Program` / `View` と、`Type` などの判別付きユニオンは `kind` で分ける（Go では interface と `UnmarshalJSON`）。

```go
func (p *Program) Lookup(fqn string) Decl          // 宣言を引く。なければ nil
func (p *Program) Namespace(fqn string) *Namespace
func (v *View) Signature(op string) *ViewedSignature
```

### `plugin`（公開）

プラグインの SDK。JSON の形は `docs/plugin-protocol.md`「要求と応答」で、型はそれを写す。

```go
type Handler interface {
    Emit(req *EmitRequest) (*EmitResponse, error)
    Check(req *CheckRequest) (*CheckResponse, error)
    Import(req *ImportRequest) (*ImportResponse, error)
}
type EmitRequest struct { Protocol string; Direction string; View *ir.View }
type EmitResponse struct { Files []File; Warnings []Warning; Errors []Diagnostic }
// CheckRequest / CheckResponse / ImportRequest / ImportResponse / File / Warning / Diagnostic も同じ形

// Main は標準入力の要求を読み、protocol を確かめ、h を呼び、標準出力に応答を書く。終了コードを返す。
func Main(h Handler) int
func Run(h Handler, in io.Reader, out, errOut io.Writer) int   // Main の中身。テスト用
// 実装しない方向は ErrUnsupported を返す。Main は docs/plugin-protocol.md「起動」の code "unsupported" で終わる
var ErrUnsupported error
```

### `plugin/golang`（公開）

`docs/emit-go.md`「名前」「型」の規則。既定の出力と Go のプラグインが共有する。

```go
func TypeName(fqn, namespace string) string       // namespace は宣言した名前空間。合成 model（`.` と `$`）の連結を含む
func FieldName(name string) string
func MethodName(opName string) string
func LocalName(name string) string                // 引数名・束縛名。予約語なら末尾に _
func EffectConst(name string) string
func CaseConst(classify, caseName string) string
func GoType(t ir.Type, imports *Imports) string   // 型の文字列。必要な import を imports に足す
type Resolver func(fqn string) (namespace, pkgPath, pkgName string, local bool)   // ref の行き先が同じパッケージか、どのパッケージか
type Imports struct{ Viewer string; Resolve Resolver }   // import パスと別名の管理。Add / Name / Paths / Render()
func PackageName(namespace string) string
func ImportPath(root, out string) (string, error) // go.mod を探す。見つからなければ ErrNoGoMod（GOEMIT-1）
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
func (fs *FileSet) Path(FileID) string
```

IR の `Span`（行・列）は `Position` で作る。構文木はバイトのオフセットで持つ。

### `internal/diag`

`docs/diagnostics.md` の `Diagnostic`、`Severity`、並べ替え（`Sort`）、text と JSON の表示（`WriteText`、`WriteJSON`）。

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
func Analyze(fs *source.FileSet, files []*ast.File, cfg *config.Config) *Result   // Op.Signature と Call.Bound は空のまま（internal/check が埋める）
```

解決と型の誤りがあっても `Program` は作る。作れなかった宣言は落とし、参照は未解決のまま残す。

### `internal/check`

`docs/check.md`。

```go
func Compute(p *ir.Program)                                  // Signature と Call.bound を埋める
type Locator func(ir.Span) (source.Span, bool)               // IR の行・列を FileSet のバイトの位置に戻す
func FileSetLocator(fs *source.FileSet) Locator
type Checker struct { Locate Locator }
func (c Checker) Check(p *ir.Program, cfg *config.Config) []diag.Diagnostic   // Compute を呼んでから規則を検査する。Locate が nil なら Span は nil で File だけ
func (c Checker) CheckUnsupported(p *ir.Program, ns string, unsupported []string) []diag.Diagnostic   // CFG-6。unsupported はマニフェストから
func Views(p *ir.Program, cfg *config.Config) map[string]*ir.View   // 設定のある名前空間ごとの View
```

規則ごとに一つのファイル（`throws.go`、`bind.go`、…）。各関数のコメントに規則の ID。

### `internal/config`

`docs/config.md`。

```go
type Config struct { Root, Path string; Hozo HozoTable; Namespaces map[string]NamespaceTable }
func Load(path string) (*Config, []diag.Diagnostic)        // CFG-2 / CFG-7 / CFG-11 はここで出る。Config は常に返す（既定値で補う）
func Find(startDir string) (string, bool)
func Default(root string) *Config                           // hozo.toml がないとき（Project.NoConfig）
func (c *Config) Resolve(namespace string) (NamespaceConf, bool)   // 継承を解決した Conf(N)
func (c *Config) Dir(namespace string) (string, bool)       // Dir(N)（docs/config.md「名前空間の設定の解決」）。ルートからの相対パス。Resolve が false なら false
func (c *Config) NamespaceNames() []string                  // CFG-8 / CFG-10 のため
func IsReservedNamespaceName(name string) bool              // CFG-2 の .hozo 側のため
```

### `internal/vocab`

`docs/vocab.md`。公開パッケージ `vocab` が `embed` した `vocab/*.hozo` を、`Files() []File` と `AddTo(func(path string, content []byte))` で渡す。パスは `hozo:vocab/<名前>.hozo`。プラグインの語彙は `PluginFiles(plugin, dir, rels)` / `AddPluginTo` で `hozo:plugin/<名前>/<相対パス>`。

### `internal/emit/golang`

`docs/emit-go.md`。

```go
type File struct { Path string; Content []byte }            // Path はルートからの相対パス（View.config.dir の下）
type ManifestInfo struct { Name string; Entry, Stop, Decorate []string }   // 設定されたプラグインのマニフェストから（Wrappers の判定）
type Options struct { Root string; Manifests []ManifestInfo }   // 別の名前空間の言語と出力先は View.namespaces から読む（docs/emit-go.md「配置」）
func Emit(view *ir.View, opts Options) ([]File, []diag.Diagnostic)   // error があればファイルは返さない
```

### `internal/plugin`

ホスト側。`docs/plugin-protocol.md`。

```go
type Manifest struct{ ... }
func LoadManifest(path string) (*Manifest, []diag.Diagnostic)                           // MAN-1 / MAN-2 / MAN-4
func Discover(cfg *config.Config, name string) (*Manifest, string, []diag.Diagnostic)   // マニフェストとそのディレクトリ（絶対パス）。CFG-9 / MAN-3
type Runner struct { Root string; Stderr io.Writer; Env []string }
func (r Runner) Run(m *Manifest, dir string, req any, resp any) error                   // 起動して JSON をやり取り（docs/plugin-protocol.md「起動」）
var ErrCrashed, ErrMalformedOutput error                                                // errors.Is で判定。*PluginError は exit 2 の code を持ち、MAN-2 / MAN-5 の診断にする
```

### `internal/project`

一度の解析のすべて。CLI と言語サーバーの共通の入口。

```go
type Project struct {
    Config  *config.Config          // 設定なしなら既定値で、NoConfig が true
    NoConfig bool
    Files   *source.FileSet
    Sema    *sema.Result
    Diags   []diag.Diagnostic        // 解析・解決・検査・設定・プラグインの check のすべて。並べ替え済み
}
func Load(root string, configPath string, overlay map[string][]byte) *Project   // NoConfig なら CFG-* の診断を除く
func (p *Project) Errors() int
```

### `internal/app`

`docs/cli.md`「手で書くもの」。生成した `hozoc.Leaves`（`internal/gen/hozoc`）の実装 `Leaves` と、`cli.Presenter`（`internal/gen/hozoc/cli`）の実装 `Presenter`。引数の解釈は持たない（生成した `cli.Main`）。

### `internal/lsp`

`docs/lsp.md`。`func Serve(in io.Reader, out io.Writer, opts Options) error`。

### `plugins/go-cli`

`docs/plugin-go-cli.md`。`main` パッケージ。`plugin.Main(handler{})`。

## 生成するもの

| 元 | 生成物 | 道具 |
|---|---|---|
| `ir` と `plugin` の型 | `schema/ir.schema.json`（`Program` / `View` と、`$defs` にプロトコルの要求と応答） | `internal/tools/irschema`（`go generate ./schema/...`。構造体は reflect で写し、`kind` のユニオンと列挙は道具の表で持つ） |
| `schema/ir.schema.json` | `sdk/ts/src/ir.ts`（IR の型。プロトコルの要求と応答の型は `sdk/ts/src/protocol.ts` に手で写す） | `json-schema-to-typescript`（`sdk/ts` の `npm run generate`） |
| `token.Keywords` | `editors/vscode/syntaxes/hozo.tmLanguage.json` の予約語の部分 | `internal/tools/tmgrammar`（`go generate`） |
| `cli.hozo` | `internal/gen/hozoc/**` | `hozoc emit`（`make generate`。段階 3 から） |

生成物はコミットする。CI で生成し直して差分がないことを確かめる（`docs/testing.md`）。
