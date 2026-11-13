// Package app は hozoc の葉の op の実装と表示（docs/cli.md「手で書くもの」）。
//
// 骨組み（引数の解釈、例外の見分けと終了コード、使い方の表示）は cli.hozo から生成した
// internal/gen/hozoc（既定の出力。docs/emit-go.md）と internal/gen/hozoc/cli（go-cli。docs/plugin-go-cli.md）が持ち、
// ここは生成した実装の単位を埋める [App] と、cli.Presenter の実装 [Presenter] を持つ。go-cli が選ぶのは @cli.command の
// check / emit / lsp と、emit の配線から辿られる writeOutputs:
//
//   - 効果を持たない葉の check と lsp は hozoc.Leaves のフィールド（[App.Leaves]）。
//   - 効果 syncOutputs を持つ葉の writeOutputs は、その宛先の状態の単位 hozoc.States.OutputsSync の命令。同じ単位の
//     問い合わせ（分類 OutputsSync の述語 UpToDate）と一緒に [App.States] が返す。
//   - 配線の emit は生成した ops が持つ。
//   - どの配線からも呼ばれない runEmit は生成の対象にならないので、このパッケージの中の関数（emit.go）。
//
// [App.Ops] がこれらを組み立てる（hozoc.States に入れるのはそこだけ）。cmd/hozoc/main.go はその結果を cli.Main に渡す。
//
// 境界の型は生成したもの（hozoc.CheckInput、hozoc.Diagnostic、hozoc.CheckFailed など）で、中の豊かな型
// （internal/project.Project、internal/diag.Diagnostic）は App の中にとどめる。例外は生成した exception の型で
// 返す（hozoc.CheckFailed{Value: hozoc.Diagnostics{...}} など）。cli.go はそれを errors.As で見分けて
// @cli.exitCode の終了コードにする（docs/plugin-go-cli.md「実行と表示」）。
package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/gen/hozoc"
	"github.com/mew-ton/hozo/internal/lsp"
	hostplugin "github.com/mew-ton/hozo/internal/plugin"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/internal/source"
)

// App は葉の op の実装（docs/cli.md「手で書くもの」の internal/app）と、それらが共有するもの。零値で使える。
//
// cli.hozo の model は境界だけを書く（docs/requirements.md「自己展開」）ので、op をまたいで要る中身は
// App の中に持つ: Check が解析した Project（WriteOutputs が使う）。プラグインに渡す IR（cli.hozo の model View は
// 中身を持たない）は、その Project から作る。hozoc は一度の実行で一つのコマンドを走らせるので、同時に使われることはない。
type App struct {
	// Dir は作業ディレクトリ。hozo.toml を探し始める場所で、--config の相対パスの基準。空なら os の作業ディレクトリ。
	Dir string
	// Stderr はプラグインの標準エラーを流す先（docs/plugin-protocol.md「起動」）。nil なら os.Stderr。
	Stderr io.Writer
	// Env はプラグインに渡す環境変数の元。nil なら os.Environ()。
	Env []string
	// Stdin と Stdout は lsp が話す相手（docs/lsp.md「通信」）。nil なら os.Stdin / os.Stdout。
	Stdin  io.Reader
	Stdout io.Writer

	// checked は直前の Check が解析した Project。WriteOutputs の引数 checked（hozoc.CheckResult）はその warning だけ
	// なので、Program・設定・マニフェストはここから取る。Check が失敗すれば nil。
	checked *project.Project
}

// Leaves は効果を持たない葉の op（check、lsp）の実装を入れた hozoc.Leaves。
func (a *App) Leaves() hozoc.Leaves {
	return hozoc.Leaves{Check: a.Check, Lsp: a.Lsp}
}

// States は hozoc の状態の単位の実装: 分類の状態 OutputsSync の述語 UpToDate（問い合わせ）と、
// 効果 syncOutputs を持つ葉の op writeOutputs（命令）。
func (a *App) States() hozoc.StateSet {
	return hozoc.StateSet{OutputsSync: hozoc.OutputsSyncState{UpToDate: a.UpToDate, WriteOutputs: a.WriteOutputs}}
}

// Ops は a の実装で hozoc の Ops を組み立てる（docs/cli.md「手で書くもの」の cmd/hozoc/main.go の中身）:
// パッケージの変数 hozoc.States に a.States() を入れ、hozoc.New(a.Leaves()) を返す。hozoc.States に入れるのはここだけ。
// 変数は一つなので、最後に Ops を呼んだ App が効く（hozoc は一度の実行で App を一つだけ作る）。
func (a *App) Ops() hozoc.Ops {
	hozoc.States = a.States()
	return hozoc.New(a.Leaves())
}

// Lsp は cli.hozo の op lsp: 言語サーバーとして標準入出力で動く（docs/lsp.md、docs/cli.md「lsp」）。
// internal/lsp.Serve を呼び、クライアントが exit を送るまで戻らない。config（--config / HOZO_CONFIG）は
// hozo.toml の場所で、なければクライアントの rootUri から探す。log（--log / HOZO_LSP_LOG）はログの書き出し先で、
// 開けなければその誤りを返す（宣言にない例外なので 70。docs/diagnostics.md「終了コード」）。
// 標準エラーには書かない（docs/lsp.md「通信」）。
func (a *App) Lsp(in hozoc.LspInput) error {
	opts := lsp.Options{Root: a.Dir}
	if in.Config != nil && *in.Config != "" {
		opts.ConfigPath = string(*in.Config)
		if !filepath.IsAbs(opts.ConfigPath) && a.Dir != "" {
			opts.ConfigPath = filepath.Join(a.Dir, opts.ConfigPath)
		}
	}
	if in.Log != nil && *in.Log != "" {
		f, err := os.OpenFile(string(*in.Log), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return fmt.Errorf("hozoc lsp: ログを開けない: %w", err)
		}
		defer f.Close()
		opts.Log = f
	}
	var stdin io.Reader = a.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	var stdout io.Writer = a.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	return lsp.Serve(stdin, stdout, opts)
}

// 例外の値と誤りの写し（cli.hozo の exception ↔ 中の型）。

// diagnostics は診断の列を cli.hozo の model Diagnostics にする。
func diagnostics(diags []diag.Diagnostic, fs *source.FileSet) hozoc.Diagnostics {
	return hozoc.Diagnostics{Items: toDiagnostics(diags, fs)}
}

// toDiagnostics は診断の列を cli.hozo の model Diagnostic の列にする。位置は行と列に直す（docs/diagnostics.md「構造」）。
func toDiagnostics(diags []diag.Diagnostic, fs *source.FileSet) []hozoc.Diagnostic {
	out := make([]hozoc.Diagnostic, 0, len(diags))
	for i := range diags {
		out = append(out, toDiagnostic(&diags[i], fs))
	}
	return out
}

func toDiagnostic(d *diag.Diagnostic, fs *source.FileSet) hozoc.Diagnostic {
	out := hozoc.Diagnostic{Severity: d.Severity.String(), Code: d.Code, Message: d.Message}
	if p := d.Path(fs); p != "" {
		file := hozoc.Path(p)
		out.File = &file
	}
	if d.Span != nil {
		start, end := positions(fs, *d.Span)
		out.Start, out.End = &start, &end
	}
	for _, r := range d.Related {
		start, end := positions(fs, r.Span)
		out.Related = append(out.Related, hozoc.RelatedInfo{File: hozoc.Path(fs.Path(r.Span.File)), Start: start, End: end, Message: r.Message})
	}
	if len(d.Data) > 0 {
		out.Data = map[string][]string{}
		for k, v := range d.Data {
			out.Data[k] = stringList(v)
		}
	}
	return out
}

// positions は Span の始まりと終わりを 1 始まりの行と列にする。
func positions(fs *source.FileSet, s source.Span) (hozoc.Position, hozoc.Position) {
	sp, ep := fs.Position(s.File, s.Start), fs.Position(s.File, s.End)
	return hozoc.Position{Line: int64(sp.Line), Col: int64(sp.Col)}, hozoc.Position{Line: int64(ep.Line), Col: int64(ep.Col)}
}

// stringList は Diagnostic.Data の値（docs/check.md「Data」では FQN・ケース名・パスの列）を文字列の列にする。
func stringList(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, len(x))
		for i, e := range x {
			out[i] = fmt.Sprint(e)
		}
		return out
	case nil:
		return nil
	}
	return []string{fmt.Sprint(v)}
}

// unknownNamespace は hozoc.UnknownNamespace（値を運ばない）に、表示のための名前を添えたもの。
// errors.As は Unwrap を通して hozoc.UnknownNamespace を見つける。
type unknownNamespace struct{ names []string }

func (e *unknownNamespace) Error() string {
	return fmt.Sprintf("名前空間 %s は、hozo.toml のどの単位の対象でもない", strings.Join(e.names, "、"))
}

func (e *unknownNamespace) Unwrap() error { return hozoc.UnknownNamespace{} }

// procError は internal/plugin の誤り（ErrCrashed / ErrMalformedOutput）を cli.hozo の proc.Crashed /
// proc.MalformedOutput（hozoc のパッケージに複製された型。docs/emit-go.md「配置」）としても見分けられるようにしたもの。
// メッセージは元の誤りのまま。errors.Is / errors.As は両方を辿る。
type procError struct {
	exc   error // hozoc.Crashed{} か hozoc.MalformedOutput{}
	cause error
}

func (e *procError) Error() string   { return e.cause.Error() }
func (e *procError) Unwrap() []error { return []error{e.exc, e.cause} }

// asProc は err が internal/plugin の proc.Crashed / proc.MalformedOutput なら procError に包む。それ以外はそのまま。
func asProc(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, hostplugin.ErrCrashed):
		return &procError{exc: hozoc.Crashed{}, cause: err}
	case errors.Is(err, hostplugin.ErrMalformedOutput):
		return &procError{exc: hozoc.MalformedOutput{}, cause: err}
	}
	return err
}

// str は optional の文字列の値。nil なら空。
func str[T ~string](p *T) string {
	if p == nil {
		return ""
	}
	return string(*p)
}
