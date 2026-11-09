package app

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/mew-ton/hozo/internal/gen/hozoc"
	"github.com/mew-ton/hozo/internal/gen/hozoc/cli"
)

// Presenter は結果と例外の表示（docs/cli.md「手で書くもの」の Presenter、docs/plugin-go-cli.md「実行と表示」）。
// go-cli が生成した cli.Presenter の実装で、cli.Main が結果を標準出力に、例外を標準エラーに出すときに呼ぶ。
//
// 形式は input（hozoc.CheckInput / hozoc.EmitInput）の format で選ぶ（docs/diagnostics.md「表示」）。なければ text。
//
//   - text: CheckResult は診断（warning）の一覧と件数。EmitResult は warning があればその一覧と件数、続けて
//     `written: <path>` と `removed: <path>` を一行ずつ。診断を運ぶ例外はその一覧と件数。それ以外の例外は
//     `hozoc: <メッセージ>`（Failure.Err のメッセージ。なければ例外の FQN）。
//   - json: CheckResult は {"warnings": [...]}、EmitResult は {"warnings": [...], "written": [...], "removed": [...]}。
//     例外は {"error": "<FQN>", "value": {...}} で、診断を運ぶ例外の value は docs/diagnostics.md「表示」の json
//     （{"diagnostics", "errors", "warnings"}）。それ以外は cli.JSONPresenter と同じ。
//
// 診断一件の形は docs/diagnostics.md「表示」の json の diagnostics の要素（start / end / related / data は、なければ省く）。
type Presenter struct{}

var _ cli.Presenter = Presenter{}

// Present は成功の結果を w に出す。知らない result（void の nil を含む）は cli.JSONPresenter に委ねる（nil なら何も出さない）。
func (Presenter) Present(w io.Writer, command string, input any, result any) error {
	switch r := result.(type) {
	case hozoc.CheckResult:
		if isJSON(input) {
			return writeJSON(w, struct {
				Warnings []jsonDiagnostic `json:"warnings"`
			}{jsonDiagnostics(r.Warnings)})
		}
		return writeText(w, r.Warnings)
	case hozoc.EmitResult:
		if isJSON(input) {
			return writeJSON(w, struct {
				Warnings []jsonDiagnostic `json:"warnings"`
				Written  []hozoc.Path     `json:"written"`
				Removed  []hozoc.Path     `json:"removed"`
			}{jsonDiagnostics(r.Warnings), orEmpty(r.Written), orEmpty(r.Removed)})
		}
		if len(r.Warnings) > 0 {
			if err := writeText(w, r.Warnings); err != nil {
				return err
			}
		}
		for _, p := range r.Written {
			if _, err := fmt.Fprintf(w, "written: %s\n", p); err != nil {
				return err
			}
		}
		for _, p := range r.Removed {
			if _, err := fmt.Fprintf(w, "removed: %s\n", p); err != nil {
				return err
			}
		}
		return nil
	}
	return cli.JSONPresenter{}.Present(w, command, input, result)
}

// PresentError は例外を w に出す。
func (Presenter) PresentError(w io.Writer, command string, input any, f cli.Failure) error {
	d, carries := f.Value.(hozoc.Diagnostics)
	if isJSON(input) {
		if !carries {
			return cli.JSONPresenter{}.PresentError(w, command, input, f)
		}
		errs, warns := count(d.Items)
		return writeJSON(w, struct {
			Error string `json:"error"`
			Value any    `json:"value"`
		}{f.Name, struct {
			Diagnostics []jsonDiagnostic `json:"diagnostics"`
			Errors      int              `json:"errors"`
			Warnings    int              `json:"warnings"`
		}{jsonDiagnostics(d.Items), errs, warns}})
	}
	if carries {
		return writeText(w, d.Items)
	}
	msg := f.Name
	if f.Err != nil {
		msg = f.Err.Error()
	}
	_, err := fmt.Fprintf(w, "hozoc: %s\n", msg)
	return err
}

// isJSON は入力の format が json か（cli.hozo の CheckInput / EmitInput の format。lsp にはない）。
func isJSON(input any) bool {
	switch in := input.(type) {
	case hozoc.CheckInput:
		return in.Format != nil && *in.Format == "json"
	case hozoc.EmitInput:
		return in.Format != nil && *in.Format == "json"
	}
	return false
}

// writeText は docs/diagnostics.md「表示」の text: `ファイル:行:列: 重さ[コード]: メッセージ`、関連は `  ...: note: ...`、
// 最後に件数。位置がなければ `ファイル: `、ファイルもなければ `hozoc: `。
func writeText(w io.Writer, diags []hozoc.Diagnostic) error {
	for i := range diags {
		d := &diags[i]
		if _, err := fmt.Fprintf(w, "%s%s[%s]: %s\n", location(d), d.Severity, d.Code, d.Message); err != nil {
			return err
		}
		for _, r := range d.Related {
			if _, err := fmt.Fprintf(w, "  %s:%d:%d: note: %s\n", r.File, r.Start.Line, r.Start.Col, r.Message); err != nil {
				return err
			}
		}
	}
	errs, warns := count(diags)
	_, err := fmt.Fprintf(w, "%s, %s\n", plural(errs, "error"), plural(warns, "warning"))
	return err
}

func location(d *hozoc.Diagnostic) string {
	switch {
	case d.File != nil && d.Start != nil:
		return fmt.Sprintf("%s:%d:%d: ", *d.File, d.Start.Line, d.Start.Col)
	case d.File != nil:
		return string(*d.File) + ": "
	}
	return "hozoc: "
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// count は error と warning の数。
func count(diags []hozoc.Diagnostic) (errs, warns int) {
	for _, d := range diags {
		switch d.Severity {
		case "error":
			errs++
		case "warning":
			warns++
		}
	}
	return errs, warns
}

// jsonDiagnostic は docs/diagnostics.md「表示」の json の diagnostics の要素。hozoc.Diagnostic との違いは
// related を空なら省くこと。
type jsonDiagnostic struct {
	Severity string              `json:"severity"`
	Code     string              `json:"code"`
	Message  string              `json:"message"`
	File     *hozoc.Path         `json:"file,omitempty"`
	Start    *hozoc.Position     `json:"start,omitempty"`
	End      *hozoc.Position     `json:"end,omitempty"`
	Related  []hozoc.RelatedInfo `json:"related,omitempty"`
	Data     map[string][]string `json:"data,omitempty"`
}

func jsonDiagnostics(diags []hozoc.Diagnostic) []jsonDiagnostic {
	out := make([]jsonDiagnostic, 0, len(diags))
	for _, d := range diags {
		out = append(out, jsonDiagnostic{Severity: d.Severity, Code: d.Code, Message: d.Message, File: d.File,
			Start: d.Start, End: d.End, Related: d.Related, Data: d.Data})
	}
	return out
}

func orEmpty(paths []hozoc.Path) []hozoc.Path {
	if paths == nil {
		return []hozoc.Path{}
	}
	return paths
}

// writeJSON は json.MarshalIndent（cli.JSONPresenter と同じ形）で w に出す。
func writeJSON(w io.Writer, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(b))
	return err
}
