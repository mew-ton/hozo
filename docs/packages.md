# Go のパッケージ

## この文書が決めること

モジュール `github.com/mew-ton/hozo` のパッケージの切り方、各パッケージの責務と主な API、依存の向き。ディレクトリの一覧の持ち主は `docs/requirements.md`「リポジトリの構成」で、ここはその中身。

## 依存の向き

```
cmd/hozoc → internal/app → internal/project → internal/{config, vocab, syntax, sema, check, emit/golang, plugin}
                                              → ir, plugin/golang, runtime
internal/lsp → internal/project
plugins/go-cli → plugin, plugin/golang, ir
ir → (標準ライブラリだけ)
plugin → ir
runtime → (標準ライブラリだけ)
```

- `internal/` は `cmd/hozoc` と `internal/` の中からだけ使う。
- プラグインが使えるのは `ir`、`plugin`、`plugin/golang`、`runtime`（`docs/requirements.md`「リポジトリの構成」）。
- `ir` と `runtime` は外部のモジュールに依存しない。`internal/config` だけが `github.com/BurntSushi/toml` に依存する。ほかに外部のモジュールは使わない（`docs/lsp.md`「通信」）。

## パッケージ

### `ir`（公開）

`docs/ir.md` の型。`encoding/json` のタグはこの文書のフィールド名。`Program` / `View` と、`Type` などの判別付きユニオンは `kind` で分ける（Go では interface と `UnmarshalJSON`）。

```go
func (p *Program) Lookup(fqn string) Decl          // 宣言を引く。なければ nil
func (p *Program) Namespace(fqn string) *Namespace
func (v *View) Signature(op string) *ViewedSignature
```

### `plugin`（公開）

プラグインの SDK。`docs/plugin-protocol.md`。

```go
type Handler interface {
    Emit(req *EmitRequest) (*EmitResponse, error)
    Check(req *CheckRequest) (*CheckResponse, error)
    Import(req *ImportRequest) (*ImportResponse, error)
}
type EmitRequest struct { Namespace string; View *ir.View }
type EmitResponse struct { Files []File; Warnings []Warning; Errors []Diagnostic }
// CheckRequest / CheckResponse / ImportRequest / ImportResponse / File / Warning / Diagnostic も同じ形

// Main は標準入力の要求を読み、protocol を確かめ、h を呼び、標準出力に応答を書く。終了コードを返す。
func Main(h Handler) int
// 実装しない方向は、ErrUnsupported を返す。Main は終了コード 2 で { "error": ... } を返す
var ErrUnsupported error
```

### `plugin/golang`（公開）

`docs/emit-go.md`「名前」「型」の規則。既定の出力と Go のプラグインが共有する。

```go
func TypeName(fqn, viewer string) string          // 合成 model の連結を含む
func FieldName(name string) string
func MethodName(opName string) string
func EffectConst(name string) string
func CaseConst(classify, caseName string) string
func GoType(t ir.Type, imports *Imports) string   // 型の文字列。必要な import を imports に足す
type Imports struct{ ... }                         // import パスと別名の管理。Render() で import ブロック
func PackageName(namespace string) string
func ImportPath(root, out string) (string, error) // go.mod を探す。GOEMIT-1
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
type Ref struct { Span source.Span; Target Target }   // Target は宣言の FQN、フィールド、束縛、引数のどれか
func Analyze(fs *source.FileSet, files []*ast.File, cfg *config.Config) *Result
```

解決と型の誤りがあっても `Program` は作る。作れなかった宣言は落とし、参照は未解決のまま残す。

### `internal/check`

`docs/check.md`。

```go
func Compute(p *ir.Program)                                  // Signature と Call.bound を埋める
func Check(p *ir.Program, cfg *config.Config) []diag.Diagnostic
func Views(p *ir.Program, cfg *config.Config) map[string]*ir.View   // 名前空間ごとの View
```

規則ごとに一つのファイル（`throws.go`、`bind.go`、…）。各関数のコメントに規則の ID。

### `internal/config`

`docs/config.md`。

```go
type Config struct { Root string; Hozo HozoTable; Namespaces map[string]NamespaceTable }
func Load(path string) (*Config, []diag.Diagnostic)
func Find(startDir string) (string, bool)
func (c *Config) Resolve(namespace string) (NamespaceConf, bool)   // 継承を解決
```

### `internal/vocab`

`docs/vocab.md`。`embed` した `vocab/*.hozo` を `source.FileSet` に足す `Add(fs *source.FileSet)`。パスは `hozo:vocab/<名前>.hozo`。

### `internal/emit/golang`

`docs/emit-go.md`。`func Emit(view *ir.View) ([]File, []diag.Diagnostic)`。

### `internal/plugin`

ホスト側。`docs/plugin-protocol.md`。

```go
type Manifest struct{ ... }
func Discover(cfg *config.Config, name string) (*Manifest, string, []diag.Diagnostic)   // マニフェストとそのディレクトリ
func Run(m *Manifest, dir string, req any, resp any) error                              // 起動して JSON をやり取り。proc.Crashed / MalformedOutput に対応するエラー型
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
func Load(root string, configPath string, overlay map[string][]byte) *Project
func (p *Project) Errors() int
```

### `internal/app`

`docs/cli.md`「手で書くもの」。`hozoc.Leaves` の実装と `Presenter`。段階 3 までは、引数の解釈も持つ。

### `internal/lsp`

`docs/lsp.md`。`func Serve(in io.Reader, out io.Writer, opts Options) error`。

### `plugins/go-cli`

`docs/plugin-go-cli.md`。`main` パッケージ。`plugin.Main(handler{})`。

## 生成するもの

| 元 | 生成物 | 道具 |
|---|---|---|
| `ir` の型 | `schema/ir.schema.json` | `internal/tools/irschema`（`go generate`） |
| `schema/ir.schema.json` | `sdk/ts/src/ir.ts` | `json-schema-to-typescript`（`sdk/ts` の `npm run generate`） |
| `token.Keywords` | `editors/vscode/syntaxes/hozo.tmLanguage.json` の予約語の部分 | `internal/tools/tmgrammar`（`go generate`） |
| `cli.hozo` | `internal/gen/hozoc/**` | `hozoc emit`（段階 3 から） |

生成物はコミットする。CI で生成し直して差分がないことを確かめる（`docs/testing.md`）。
