package diag

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/mew-ton/hozo/internal/source"
)

// WriteText は docs/diagnostics.md「表示」の text の形で書く。
//
//	ファイル:行:列: 重さ[コード]: メッセージ
//	  ファイル:行:列: note: 関連のメッセージ
//	N errors, M warnings
//
// 位置がなければ「ファイル: 」、ファイルもなければ「hozoc: 」で始める。色は付けない。
func WriteText(w io.Writer, diags []Diagnostic, fs *source.FileSet) error {
	for i := range diags {
		d := &diags[i]
		if _, err := fmt.Fprintf(w, "%s%s[%s]: %s\n", location(d, fs), d.Severity, d.Code, d.Message); err != nil {
			return err
		}
		for _, r := range d.Related {
			pos := fs.Position(r.Span.File, r.Span.Start)
			if _, err := fmt.Fprintf(w, "  %s:%d:%d: note: %s\n", fs.Path(r.Span.File), pos.Line, pos.Col, r.Message); err != nil {
				return err
			}
		}
	}
	errors, warnings := Count(diags)
	_, err := fmt.Fprintf(w, "%s, %s\n", plural(errors, "error"), plural(warnings, "warning"))
	return err
}

// location は一行目の頭。「ファイル:行:列: 」か「ファイル: 」か「hozoc: 」。
func location(d *Diagnostic, fs *source.FileSet) string {
	if d.Span != nil {
		pos := fs.Position(d.Span.File, d.Span.Start)
		return fmt.Sprintf("%s:%d:%d: ", fs.Path(d.Span.File), pos.Line, pos.Col)
	}
	if d.File != "" {
		return d.File + ": "
	}
	return "hozoc: "
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// jsonPos は JSON の start / end。
type jsonPos struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

type jsonRelated struct {
	File    string  `json:"file"`
	Start   jsonPos `json:"start"`
	End     jsonPos `json:"end"`
	Message string  `json:"message"`
}

type jsonDiagnostic struct {
	Severity Severity       `json:"severity"`
	Code     string         `json:"code"`
	Message  string         `json:"message"`
	File     string         `json:"file,omitempty"`
	Start    *jsonPos       `json:"start,omitempty"`
	End      *jsonPos       `json:"end,omitempty"`
	Related  []jsonRelated  `json:"related,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

type jsonOutput struct {
	Diagnostics []jsonDiagnostic `json:"diagnostics"`
	Errors      int              `json:"errors"`
	Warnings    int              `json:"warnings"`
}

// WriteJSON は docs/diagnostics.md「表示」の json の形で書く。
// 位置のないものは start / end を省き、ファイルもなければ file も省く。
func WriteJSON(w io.Writer, diags []Diagnostic, fs *source.FileSet) error {
	out := jsonOutput{Diagnostics: make([]jsonDiagnostic, 0, len(diags))}
	for i := range diags {
		d := &diags[i]
		jd := jsonDiagnostic{
			Severity: d.Severity,
			Code:     d.Code,
			Message:  d.Message,
			File:     d.Path(fs),
			Data:     d.Data,
		}
		if d.Span != nil {
			jd.Start, jd.End = jsonSpan(fs, *d.Span)
		}
		for _, r := range d.Related {
			start, end := jsonSpan(fs, r.Span)
			jd.Related = append(jd.Related, jsonRelated{
				File:    fs.Path(r.Span.File),
				Start:   *start,
				End:     *end,
				Message: r.Message,
			})
		}
		out.Diagnostics = append(out.Diagnostics, jd)
	}
	out.Errors, out.Warnings = Count(diags)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func jsonSpan(fs *source.FileSet, s source.Span) (start, end *jsonPos) {
	sp := fs.Position(s.File, s.Start)
	ep := fs.Position(s.File, s.End)
	return &jsonPos{Line: sp.Line, Col: sp.Col}, &jsonPos{Line: ep.Line, Col: ep.Col}
}
