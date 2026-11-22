package app

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/gen/hozoc"
	"github.com/mew-ton/hozo/internal/gen/hozoc/cli"
)

// Presenter は結果と例外の表示（docs/cli.md「手で書くもの」の Presenter、docs/plugin-go-cli.md「実行と表示」）。
// go-cli が生成した cli.Presenter の実装で、cli.Main が結果を標準出力に、例外を標準エラーに出すときに呼ぶ。
//
// 形式は input（hozoc.CheckInput / hozoc.BuildInput / hozoc.TestsInput）の format で選ぶ（docs/diagnostics.md「表示」）。なければ text。
//
//   - text: CheckResult は診断（warning）の一覧と件数。EmitResult は warning があればその一覧と件数、続けて
//     `written: <path>` と `removed: <path>` を一行ずつ。TestsResult は warning があればその一覧と件数、続けて
//     `ran: <トランスパイラの名前>: <テストの名前>` を一行ずつと、最後に件数（`3 tests ran`）。診断を運ぶ例外はその一覧と件数。それ以外の例外は
//     `hozoc: <メッセージ>`（Failure.Err のメッセージ。なければ例外の FQN）。
//   - json: CheckResult は {"warnings": [...]}、EmitResult は {"warnings": [...], "written": [...], "removed": [...]}。
//     例外は {"error": "<FQN>", "value": {...}} で、診断を運ぶ例外の value は docs/diagnostics.md「表示」の json
//     （{"diagnostics", "errors", "warnings"}）。例外の引数の名前は、表示に出さない。それ以外は cli.JSONPresenter と同じ。
//
// 診断の text と json の形は internal/diag が持つ（diag.Rendered、diag.WriteRenderedText、diag.Report）。ここは、
// cli.hozo の model Diagnostic をその形に写して呼び、結果と例外ごとの包み（warnings / written / removed、error / value）
// だけを書く。
type Presenter struct{}

var _ cli.Presenter = Presenter{}

// Present は成功の結果を w に出す。知らない result（void の nil を含む）は cli.JSONPresenter に委ねる（nil なら何も出さない）。
func (Presenter) Present(w io.Writer, command string, input any, result any) error {
	switch r := result.(type) {
	case hozoc.BuildResult:
		// 検査だけ（hozoc build --no-emit）: 検査の結果だけを出す（docs/cli.md「build」の「--no-emit の表示」）
		if in, ok := input.(hozoc.BuildInput); ok && in.NoEmit != nil && *in.NoEmit {
			if isJSON(input) {
				return writeJSON(w, struct {
					Warnings []diag.Rendered `json:"warnings"`
				}{rendered(r.Warnings)})
			}
			return diag.WriteRenderedText(w, rendered(r.Warnings))
		}
		if isJSON(input) {
			return writeJSON(w, struct {
				Warnings []diag.Rendered `json:"warnings"`
				Written  []hozoc.Path    `json:"written"`
				Removed  []hozoc.Path    `json:"removed"`
			}{rendered(r.Warnings), orEmpty(r.Written), orEmpty(r.Removed)})
		}
		if len(r.Warnings) > 0 {
			if err := diag.WriteRenderedText(w, rendered(r.Warnings)); err != nil {
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
	case hozoc.TestsResult:
		ran := r.Ran
		if ran == nil {
			ran = []string{}
		}
		if isJSON(input) {
			return writeJSON(w, struct {
				Warnings []diag.Rendered `json:"warnings"`
				Ran      []string        `json:"ran"`
			}{rendered(r.Warnings), ran})
		}
		if len(r.Warnings) > 0 {
			if err := diag.WriteRenderedText(w, rendered(r.Warnings)); err != nil {
				return err
			}
		}
		for _, name := range ran {
			if _, err := fmt.Fprintf(w, "ran: %s\n", name); err != nil {
				return err
			}
		}
		_, err := fmt.Fprintf(w, "%d tests ran\n", len(ran))
		return err
	}
	return cli.JSONPresenter{}.Present(w, command, input, result)
}

// PresentError は例外を w に出す。
func (Presenter) PresentError(w io.Writer, command string, input any, f cli.Failure) error {
	// f.Value は、引数を持つ例外の、値そのもの（docs/plugin-go-cli.md「コマンドごとの関数」）。診断を運ぶ例外かどうかは carriedBy が見る
	diags, carries := carriedBy(f.Value)
	if isJSON(input) {
		if !carries {
			return cli.JSONPresenter{}.PresentError(w, command, input, f)
		}
		return writeJSON(w, struct {
			Error string      `json:"error"`
			Value diag.Report `json:"value"`
		}{f.Name, diag.NewReport(rendered(diags))})
	}
	if carries {
		return diag.WriteRenderedText(w, rendered(diags))
	}
	msg := f.Name
	if f.Err != nil {
		msg = f.Err.Error()
	}
	_, err := fmt.Fprintf(w, "hozoc: %s\n", msg)
	return err
}

// isJSON は入力の format が json か（cli.hozo の CheckInput / EmitInput / TestsInput の format。lsp にはない）。
func isJSON(input any) bool {
	switch in := input.(type) {
	case hozoc.BuildInput:
		return in.Format != nil && *in.Format == "json"
	case hozoc.TestsInput:
		return in.Format != nil && *in.Format == "json"
	}
	return false
}

// rendered は cli.hozo の model Diagnostic の列を、表示する形（internal/diag.Rendered）に写す。位置は既に行と列なので
// 写すだけ。text と json の形そのものは internal/diag が持つ（docs/packages.md「internal/diag」）。
func rendered(diags []hozoc.Diagnostic) []diag.Rendered {
	out := make([]diag.Rendered, 0, len(diags))
	for i := range diags {
		d := &diags[i]
		r := diag.Rendered{Severity: d.Severity, Code: d.Code, Message: d.Message}
		if d.File != nil {
			r.File = string(*d.File)
		}
		if d.Start != nil {
			r.Start = &diag.RenderedPos{Line: int(d.Start.Line), Col: int(d.Start.Col)}
		}
		if d.End != nil {
			r.End = &diag.RenderedPos{Line: int(d.End.Line), Col: int(d.End.Col)}
		}
		for _, rel := range d.Related {
			r.Related = append(r.Related, diag.RenderedRelated{File: string(rel.File),
				Start: diag.RenderedPos{Line: int(rel.Start.Line), Col: int(rel.Start.Col)},
				End:   diag.RenderedPos{Line: int(rel.End.Line), Col: int(rel.End.Col)}, Message: rel.Message})
		}
		if len(d.Data) > 0 {
			r.Data = make(map[string]any, len(d.Data))
			for k, v := range d.Data {
				r.Data[k] = v
			}
		}
		out = append(out, r)
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
