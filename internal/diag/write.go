package diag

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/mew-ton/hozo/internal/source"
)

// Rendered は表示する形にした診断一件: 位置を 1 始まりの行と列に直し、重さを名前にしたもの。
// JSON にすると docs/diagnostics.md「表示」の json の diagnostics の要素になる（位置のないものは start / end を省き、
// ファイルもなければ file も省く。related と data は、なければ省く）。
//
// 表示（text と json）はこの形の上にだけ書く。FileSet を持つ側（WriteText / WriteJSON）は [Render] でこの形にし、
// 行と列に直した診断しか持たない側（internal/app の Presenter。cli.hozo の model Diagnostic）は自分でこの形に写す。
type Rendered struct {
	Severity string            `json:"severity"` // "error" か "warning"
	Code     string            `json:"code"`
	Message  string            `json:"message"`
	File     string            `json:"file,omitempty"`
	Start    *RenderedPos      `json:"start,omitempty"`
	End      *RenderedPos      `json:"end,omitempty"`
	Related  []RenderedRelated `json:"related,omitempty"`
	Data     map[string]any    `json:"data,omitempty"`
}

// RenderedPos は表示する位置（1 始まりの行と、コードポイントで数えた 1 始まりの列。docs/diagnostics.md「位置」）。
type RenderedPos struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

// RenderedRelated は表示する形にした関連する位置。
type RenderedRelated struct {
	File    string      `json:"file"`
	Start   RenderedPos `json:"start"`
	End     RenderedPos `json:"end"`
	Message string      `json:"message"`
}

// Report は docs/diagnostics.md「表示」の json の全体: 診断の一覧と件数。
type Report struct {
	Diagnostics []Rendered `json:"diagnostics"`
	Errors      int        `json:"errors"`
	Warnings    int        `json:"warnings"`
}

// Render は診断の列を表示する形にする（位置は fs で行と列に直す）。nil でも空の列（JSON で []）を返す。
func Render(diags []Diagnostic, fs *source.FileSet) []Rendered {
	out := make([]Rendered, 0, len(diags))
	for i := range diags {
		d := &diags[i]
		r := Rendered{Severity: d.Severity.String(), Code: d.Code, Message: d.Message, File: d.Path(fs), Data: d.Data}
		if d.Span != nil {
			start, end := renderSpan(fs, *d.Span)
			r.Start, r.End = &start, &end
		}
		for _, rel := range d.Related {
			start, end := renderSpan(fs, rel.Span)
			r.Related = append(r.Related, RenderedRelated{File: fs.Path(rel.Span.File), Start: start, End: end, Message: rel.Message})
		}
		out = append(out, r)
	}
	return out
}

func renderSpan(fs *source.FileSet, s source.Span) (start, end RenderedPos) {
	sp := fs.Position(s.File, s.Start)
	ep := fs.Position(s.File, s.End)
	return RenderedPos{Line: sp.Line, Col: sp.Col}, RenderedPos{Line: ep.Line, Col: ep.Col}
}

// NewReport は表示する形の診断の列に件数を添える。
func NewReport(diags []Rendered) Report {
	r := Report{Diagnostics: diags}
	if r.Diagnostics == nil {
		r.Diagnostics = []Rendered{}
	}
	r.Errors, r.Warnings = CountRendered(diags)
	return r
}

// CountRendered は表示する形の診断の error と warning の数を返す。
func CountRendered(diags []Rendered) (errors, warnings int) {
	for i := range diags {
		switch diags[i].Severity {
		case Error.String():
			errors++
		case Warning.String():
			warnings++
		}
	}
	return errors, warnings
}

// WriteText は docs/diagnostics.md「表示」の text の形で書く（[Render] して [WriteRenderedText]）。
func WriteText(w io.Writer, diags []Diagnostic, fs *source.FileSet) error {
	return WriteRenderedText(w, Render(diags, fs))
}

// WriteRenderedText は docs/diagnostics.md「表示」の text の形で書く。
//
//	ファイル:行:列: 重さ[コード]: メッセージ
//	  ファイル:行:列: note: 関連のメッセージ
//	N errors, M warnings
//
// 位置がなければ「ファイル: 」、ファイルもなければ「hozoc: 」で始める。色は付けない。
func WriteRenderedText(w io.Writer, diags []Rendered) error {
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
	errors, warnings := CountRendered(diags)
	_, err := fmt.Fprintf(w, "%s, %s\n", plural(errors, "error"), plural(warnings, "warning"))
	return err
}

// location は一行目の頭。「ファイル:行:列: 」か「ファイル: 」か「hozoc: 」。
func location(d *Rendered) string {
	switch {
	case d.Start != nil:
		return fmt.Sprintf("%s:%d:%d: ", d.File, d.Start.Line, d.Start.Col)
	case d.File != "":
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

// WriteJSON は docs/diagnostics.md「表示」の json の形で書く（[Render] して [NewReport]）。
// 位置のないものは start / end を省き、ファイルもなければ file も省く。
func WriteJSON(w io.Writer, diags []Diagnostic, fs *source.FileSet) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(NewReport(Render(diags, fs)))
}
