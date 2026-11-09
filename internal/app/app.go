// Package app は hozoc の葉の op の実装と表示（docs/cli.md「手で書くもの」）。
//
// cli.hozo（docs/cli.md「cli.hozo」）の op を同じ名前の Go の関数で持つ: Check / WriteOutputs / RunEmit が葉で、
// Emit は配線（check を呼んでから writeOutputs）。例外は cli.hozo と同じ名前の Go の誤りの型（CheckFailed など）で、
// 終了コードは ExitCode が docs/diagnostics.md「終了コード」の表で決める。段階 3 で cli.hozo から生成した骨組み
// （hozoc.Leaves）にこれらをつなぐとき、ここで変わるのは関数の形（context と生成した model の型）だけにする。
//
// 段階 3 まで（骨組みを生成する前）の引数の解釈は Main が持ち（docs/cli.md「手で書くもの」の最後の項）、
// cmd/hozoc/main.go はそれを呼ぶだけ。フラグと環境変数は cli.hozo の @cli.flag / @cli.env / @cli.arg を手で写した。
package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/internal/source"
	sdk "github.com/mew-ton/hozo/plugin"
)

// Diagnostics は cli.hozo の model Diagnostics（指摘の一覧）。Files は表示（diag.WriteText / WriteJSON）のための
// ファイルの集合で、Items の Span のファイルを引く。
type Diagnostics struct {
	Items []diag.Diagnostic
	Files *source.FileSet
}

// CheckInput は cli.hozo の model CheckInput。
type CheckInput struct {
	// Config は設定ファイルの場所（--config / HOZO_CONFIG）。空なら作業ディレクトリから親へ探す。
	Config string
	// Format は表示の形式（--format）。"text" か "json"。空なら text。表示にだけ使い、op の中身は見ない。
	Format string
}

// EmitInput は cli.hozo の model EmitInput。
type EmitInput struct {
	Config string
	Format string
	// Namespaces は生成する名前空間（位置引数）。一つもなければすべて。
	Namespaces []string
	// DryRun なら書き出さずに、書き出す予定のファイルだけを返す。
	DryRun bool
}

// CheckResult は cli.hozo の model CheckResult（誤りではない指摘）。
// Project は Go だけの補足で、WriteOutputs が check の結果（Program、設定、マニフェスト）を受け取るために持つ。
type CheckResult struct {
	Warnings []diag.Diagnostic
	Project  *project.Project
}

// Files は Warnings の表示に使うファイルの集合。
func (r CheckResult) Files() *source.FileSet {
	if r.Project == nil {
		return source.NewFileSet()
	}
	return r.Project.Files
}

// EmitResult は cli.hozo の model EmitResult。Written / Removed はルートからの相対パス（"/" 区切り）で辞書順。
type EmitResult struct {
	Warnings []diag.Diagnostic
	Written  []string
	Removed  []string
	files    *source.FileSet
}

// Files は Warnings の表示に使うファイルの集合。
func (r EmitResult) Files() *source.FileSet {
	if r.files == nil {
		return source.NewFileSet()
	}
	return r.files
}

// Plugin は cli.hozo の model Plugin（プラグインの起動のしかた）。Dir はマニフェストのディレクトリ（絶対パス）。
type Plugin struct {
	Name    string
	Command []string
	Dir     string
}

// EmitRequest は cli.hozo の model EmitRequest（docs/plugin-protocol.md「emit」の要求）。SDK の型と同じ。
type EmitRequest = sdk.EmitRequest

// EmitResponse は cli.hozo の model EmitResponse（同「emit」の応答）。SDK の型と同じ。
type EmitResponse = sdk.EmitResponse

// 例外（cli.hozo の exception）。終了コードは ExitCode。

// CheckFailed は「仕様に構文か型の誤りがある」。値は error と warning のすべて。@cli.exitCode(1)。
type CheckFailed struct{ Diagnostics }

// ConfigInvalid は「hozo.toml がない、または設定が誤っている（CFG-*、MAN-*）」。@cli.exitCode(2)。
type ConfigInvalid struct{ Diagnostics }

// LanguageMismatch は「プラグインの language が名前空間の language と合わない（CFG-4）」。@cli.exitCode(2)。
type LanguageMismatch struct{ Diagnostics }

// OutputConflict は「複数の出どころが同じファイルに書こうとした（CFG-5）」。@cli.exitCode(3)。
type OutputConflict struct{ Diagnostics }

// UnknownNamespace は「emit の引数に、hozo.toml にない名前空間が指定された」。@cli.exitCode(64)。
// Names は Go だけの補足（表示のため）。
type UnknownNamespace struct{ Names []string }

func (e *CheckFailed) Error() string      { return "hozoc.CheckFailed: " + e.summary() }
func (e *ConfigInvalid) Error() string    { return "hozoc.ConfigInvalid: " + e.summary() }
func (e *LanguageMismatch) Error() string { return "hozoc.LanguageMismatch: " + e.summary() }
func (e *OutputConflict) Error() string   { return "hozoc.OutputConflict: " + e.summary() }
func (e *UnknownNamespace) Error() string {
	return "hozoc.UnknownNamespace: " + strings.Join(e.Names, ", ")
}

// summary は「N errors, M warnings」。
func (d Diagnostics) summary() string {
	errs, warns := diag.Count(d.Items)
	return fmt.Sprintf("%d errors, %d warnings", errs, warns)
}

// Leaves は葉の op の実装（docs/cli.md「手で書くもの」の internal/app）。零値で使える。
type Leaves struct {
	// Dir は作業ディレクトリ。hozo.toml を探し始める場所で、--config の相対パスの基準。空なら os の作業ディレクトリ。
	Dir string
	// Stderr はプラグインの標準エラーを流す先（docs/plugin-protocol.md「起動」）。nil なら os.Stderr。
	Stderr io.Writer
	// Env はプラグインに渡す環境変数の元。nil なら os.Environ()。
	Env []string

	// root はプロジェクトのルート。WriteOutputs が RunEmit のために置く（cli.hozo の runEmit は plugin と req しか
	// 受け取らず、起動の作業ディレクトリ（ルート）はここから渡す）。
	root string
}

// Emit は cli.hozo の op emit（配線）: check を呼んでから writeOutputs を呼ぶ。
func (l *Leaves) Emit(in EmitInput) (EmitResult, error) {
	checked, err := l.Check(CheckInput{Config: in.Config, Format: in.Format})
	if err != nil {
		return EmitResult{}, err
	}
	return l.WriteOutputs(in, checked)
}

// Lsp は cli.hozo の op lsp。段階 4（docs/requirements.md「段階」）まで未実装で、常にその誤りを返す。
func (l *Leaves) Lsp(_ LspInput) error {
	return errNotImplemented
}

// LspInput は cli.hozo の model LspInput。
type LspInput struct {
	Config string
	Log    string
}

// errNotImplemented は段階 4 までの lsp の答え。終了コードは 64（使い方の誤り）。
var errNotImplemented = fmt.Errorf("hozoc lsp: not implemented")
