// Package app は hozoc の葉の op の実装と表示（docs/cli.md「手で書くもの」）。
//
// 骨組み（型、op の形、実装を受け取る口、配線、引数の解釈、例外の見分けと終了コード、使い方の表示）は cli.hozo から生成した
// internal/gen/hozo/compiler（基底 go と go-valid。docs/plugin-go.md、docs/plugin-go-valid.md）と internal/gen/hozo/compiler/cli（go-cli。
// docs/plugin-go-cli.md）が持ち、ここは、生成した口を満たす [App] と、cli.Presenter の実装 [Presenter] を持つ:
//
//   - 葉の op（状態に触れない）check、collectOutputs、matchTests、lsp、runEmit は compiler.Leaves のメソッドで、*App がそれを満たす。
//   - 状態に触れるのは、状態の単位のメソッドだけ（docs/plugin-go.md「状態の単位」）。read outputChanges は、その状態 outputs の単位
//     （compiler.OutputsState）の問い合わせで、形で決まる問い合わせ（IsEmpty と Get）と一緒に、[App.Outputs] が返す値が持つ。
//     効果 syncOutputs は、その宛先の状態の分類 OutputsSync の単位（compiler.OutputsSyncState）の命令で、同じ単位の問い合わせ
//     （述語 UpToDate と OutOfDate）と一緒に、[App.OutputsSync] が返す値が持つ。
//   - 状態 sources、config の単位（形で決まる問い合わせ IsEmpty と Get だけ）は、[App.Sources]、[App.Config] が返す値。
//   - 配線の writeOutputs（collectOutputs、outputChanges、syncOutputs を、この順に呼ぶ）と emit は、生成した本体が持つ。
//
// [App.Ops] が、これらを compiler.New に渡す。口はどれもインターフェースで、実装は New の位置引数なので、メソッドの書き忘れも、
// 渡し忘れも、コンパイルできない。cmd/hozoc/main.go は、その結果を cli.Main に渡す。
//
// 境界の型は生成したもの（compiler.CheckInput、compiler.Diagnostic、compiler.CheckFailed など）で、中の豊かな型
// （internal/project.Project、internal/diag.Diagnostic）は App の中にとどめる。例外は生成した exception の型を値で
// 返す（診断を運ぶ六つの例外は、このパッケージの exceptions.go の関数で作る。Crashed と MalformedOutput も、生成した hozoc の型）。cli.go は
// それを errors.As で見分けて @cli.exitCode の終了コードにする（docs/plugin-go-cli.md「実行と表示」）。
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/gen/hozo/compiler"
	"github.com/mew-ton/hozo/internal/lsp"
	hostplugin "github.com/mew-ton/hozo/internal/plugin"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/internal/source"
)

// App は葉の op の実装（docs/cli.md「手で書くもの」の internal/app）と、それらが共有するもの。零値で使える。
//
// cli.hozo の model は境界だけを書く（docs/requirements.md「自己展開」）ので、op をまたいで要る中身は
// App の中に持つ: Check が解析した Project（CollectOutputs が使う）と、CollectOutputs が集めた書き出す予定の、置き場を含む形
// （OutputChanges と SyncOutputs が使う）。プラグインに渡す IR（cli.hozo の model View は中身を持たない）は、その Project から
// 作る。hozoc は一度の実行で一つのコマンドを走らせるので、同時に使われることはない。
type App struct {
	// Dir は作業ディレクトリ。hozo.toml を探し始める場所で、--config の相対パスの基準。空なら os の作業ディレクトリ。
	Dir string
	// Stderr はプラグインの標準エラーを流す先（docs/plugin-protocol.md「起動」）。nil なら os.Stderr。
	Stderr io.Writer
	// Home はホームディレクトリ。プラグインの探索が <ホーム>/.hozo/plugins を見るのに使う（docs/plugin-protocol.md「探索」の 3）。
	// 空なら探さない。App はプロセスの環境から読まない: cmd/hozoc/main.go が os.UserHomeDir() を入れる。
	// 探索のもう一つの外の場所 HOZO_PLUGIN_PATH は、cli.hozo の入力の pluginPath で届く（cli.Main に渡した env から）。
	Home string
	// Stdin と Stdout は lsp が話す相手（docs/lsp.md「通信」）。nil なら os.Stdin / os.Stdout。
	Stdin  io.Reader
	Stdout io.Writer

	// pluginPath は直前の Check か Lsp の入力の pluginPath（HOZO_PLUGIN_PATH）。入力を取らない問い合わせ（UpToDate と、
	// 状態の単位の IsEmpty / Get）が、自分で解析するときに使う。
	pluginPath string
	// cacheDir は、同じく、直前の Check か Lsp の入力の cacheDir（HOZO_CACHE_DIR。docs/plugin-protocol.md「起動」）。
	cacheDir string

	// checked は直前の Check が解析した Project。CollectOutputs の引数 checked（compiler.CheckResult）はその warning だけ
	// なので、Program・設定・マニフェストはここから取る。Check が失敗すれば nil。
	checked *project.Project

	// planned は直前の CollectOutputs が集めた書き出す予定。それが返す compiler.EmitPlan は warning とファイルだけなので、
	// OutputChanges と SyncOutputs は、置き場（ルートと、出力ごとの Dir(U, N)）をここから取る（[App.plannedFor]）。
	// Check をやり直すか、CollectOutputs が失敗すれば nil。
	planned *outputPlan
}

// *App は、人が実装する葉の op（check、collectOutputs、matchTests、lsp、runEmit）の口を満たす。メソッドが一つでも足りなければ、ここで
// コンパイルできない。
var _ compiler.Leaves = (*App)(nil)

// Ops は a の実装で hozoc の Ops を組み立てる（docs/cli.md「手で書くもの」）: compiler.New に、葉の op（a 自身）と、状態の単位を
// 宣言順に渡す。返る値は a だけを持ち、パッケージの変数は使わない（App を二つ作れば、互いに何も共有しない）。
func (a *App) Ops() compiler.Ops {
	return compiler.New(a, a.Sources(), a.Config(), a.Outputs(), a.OutputsSync())
}

// Lsp は cli.hozo の op lsp: 言語サーバーとして標準入出力で動く（docs/lsp.md、docs/cli.md「lsp」）。
// internal/lsp.Serve を呼び、クライアントが exit を送るまで戻らない。config（--config / HOZO_CONFIG）は
// hozo.toml の場所で、なければクライアントの rootUri から探す。log（--log / HOZO_LSP_LOG）はログの書き出し先で、
// 開けなければその誤りを返す（宣言にない例外なので 70。docs/diagnostics.md「終了コード」）。
// 標準エラーには書かない（docs/lsp.md「通信」）。
func (a *App) Lsp(ctx context.Context, in compiler.LspInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.pluginPath = str(in.PluginPath)
	a.cacheDir = str(in.CacheDir)
	opts := lsp.Options{Root: a.Dir, CacheDir: a.cacheDir, PluginPath: a.pluginPath, Home: a.Home}
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

// 診断と誤りの写し（cli.hozo の model と exception ↔ 中の型）。診断を運ぶ例外そのものは exceptions.go。

// toDiagnostics は診断の列を cli.hozo の model Diagnostic の列にする。位置は行と列に直す（docs/diagnostics.md「構造」）。
func toDiagnostics(diags []diag.Diagnostic, fs *source.FileSet) []compiler.Diagnostic {
	out := make([]compiler.Diagnostic, 0, len(diags))
	for i := range diags {
		out = append(out, toDiagnostic(&diags[i], fs))
	}
	return out
}

func toDiagnostic(d *diag.Diagnostic, fs *source.FileSet) compiler.Diagnostic {
	out := compiler.Diagnostic{Severity: d.Severity.String(), Code: d.Code, Message: d.Message}
	if p := d.Path(fs); p != "" {
		file := compiler.Path(p)
		out.File = &file
	}
	if d.Span != nil {
		start, end := positions(fs, *d.Span)
		out.Start, out.End = &start, &end
	}
	for _, r := range d.Related {
		start, end := positions(fs, r.Span)
		out.Related = append(out.Related, compiler.RelatedInfo{File: compiler.Path(fs.Path(r.Span.File)), Start: start, End: end, Message: r.Message})
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
func positions(fs *source.FileSet, s source.Span) (compiler.Position, compiler.Position) {
	sp, ep := fs.Position(s.File, s.Start), fs.Position(s.File, s.End)
	return compiler.Position{Line: int64(sp.Line), Col: int64(sp.Col)}, compiler.Position{Line: int64(ep.Line), Col: int64(ep.Col)}
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

// unknownNamespace は compiler.UnknownNamespace（引数を持たない）に、表示のための名前を添えたもの。
// errors.As は Unwrap を通して compiler.UnknownNamespace を見つける。
type unknownNamespace struct{ names []string }

func (e *unknownNamespace) Error() string {
	return fmt.Sprintf("名前空間 %s も、その子孫も、hozo.toml のどの単位の対象でもない（仕様にない名前空間も同じ）", strings.Join(e.names, "、"))
}

func (e *unknownNamespace) Unwrap() error { return compiler.UnknownNamespace{} }

// procError は internal/plugin の誤り（ErrCrashed / ErrMalformedOutput）を cli.hozo の compiler.Crashed /
// compiler.MalformedOutput（生成した hozoc の例外の型。docs/cli.md「手で書くもの」）としても見分けられるようにしたもの。メッセージは元の誤りのまま。errors.Is / errors.As は両方を辿る。
type procError struct {
	exc   error // compiler.Crashed{} か compiler.MalformedOutput{}
	cause error
}

func (e *procError) Error() string   { return e.cause.Error() }
func (e *procError) Unwrap() []error { return []error{e.exc, e.cause} }

// asProc は err が internal/plugin の compiler.Crashed / compiler.MalformedOutput なら procError に包む。それ以外はそのまま。
func asProc(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, hostplugin.ErrCrashed):
		return &procError{exc: compiler.Crashed{}, cause: err}
	case errors.Is(err, hostplugin.ErrMalformedOutput):
		return &procError{exc: compiler.MalformedOutput{}, cause: err}
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
