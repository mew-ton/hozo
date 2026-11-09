package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/mew-ton/hozo/internal/diag"
	hostplugin "github.com/mew-ton/hozo/internal/plugin"
	"github.com/mew-ton/hozo/internal/source"
)

// 終了コード（docs/diagnostics.md「終了コード」）。
const (
	ExitOK               = 0
	ExitCheckFailed      = 1  // CheckFailed
	ExitConfigInvalid    = 2  // ConfigInvalid / LanguageMismatch
	ExitOutputConflict   = 3  // OutputConflict
	ExitUsage            = 64 // UnknownNamespace、使い方の誤り
	ExitSoftwareOrCrash  = 70 // proc.Crashed / proc.MalformedOutput、宣言にない例外
	exitNotImplementedOp = ExitUsage
)

// ExitCode は誤りを終了コードにする（docs/diagnostics.md「終了コード」、cli.hozo の @cli.exitCode）。nil なら 0。
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var (
		cf *CheckFailed
		ci *ConfigInvalid
		lm *LanguageMismatch
		oc *OutputConflict
		un *UnknownNamespace
	)
	switch {
	case errors.As(err, &cf):
		return ExitCheckFailed
	case errors.As(err, &ci), errors.As(err, &lm):
		return ExitConfigInvalid
	case errors.As(err, &oc):
		return ExitOutputConflict
	case errors.As(err, &un), errors.Is(err, errNotImplemented), errors.Is(err, errUsage):
		return ExitUsage
	case errors.Is(err, hostplugin.ErrCrashed), errors.Is(err, hostplugin.ErrMalformedOutput):
		return ExitSoftwareOrCrash
	}
	return ExitSoftwareOrCrash
}

// Presenter は結果と例外の表示（docs/cli.md「手で書くもの」の Presenter、docs/plugin-go-cli.md「実行と表示」）。
// 結果は標準出力に、例外は標準エラーに出す（呼ぶ側が w を選ぶ）。
type Presenter interface {
	PresentCheck(w io.Writer, r CheckResult) error
	PresentEmit(w io.Writer, r EmitResult) error
	PresentError(w io.Writer, err error) error
}

// NewPresenter は format（"text" か "json"。docs/diagnostics.md「表示」）の Presenter。空なら text。
func NewPresenter(format string) Presenter {
	if format == "json" {
		return JSONPresenter{}
	}
	return TextPresenter{}
}

// TextPresenter は text の表示（docs/diagnostics.md「表示」の text、docs/cli.md「手で書くもの」）。
//
//   - CheckResult: 診断（warning）の一覧と件数（diag.WriteText）。
//   - EmitResult: warning があればその一覧と件数、続けて `written: <path>` と `removed: <path>` の行。
//   - 診断を運ぶ例外: その一覧と件数。UnknownNamespace: `hozoc: 名前空間 X は hozo.toml にない`。
//     それ以外（proc.Crashed など）: `hozoc: <メッセージ>`。
type TextPresenter struct{}

func (TextPresenter) PresentCheck(w io.Writer, r CheckResult) error {
	return diag.WriteText(w, r.Warnings, r.Files())
}

func (TextPresenter) PresentEmit(w io.Writer, r EmitResult) error {
	if len(r.Warnings) > 0 {
		if err := diag.WriteText(w, r.Warnings, r.Files()); err != nil {
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

func (TextPresenter) PresentError(w io.Writer, err error) error {
	if d, _, ok := diagnosticsOf(err); ok {
		return diag.WriteText(w, d.Items, d.Files)
	}
	var un *UnknownNamespace
	if errors.As(err, &un) {
		for _, n := range un.Names {
			if _, err := fmt.Fprintf(w, "hozoc: 名前空間 %s は hozo.toml にない\n", n); err != nil {
				return err
			}
		}
		return nil
	}
	_, werr := fmt.Fprintf(w, "hozoc: %v\n", err)
	return werr
}

// JSONPresenter は json の表示。
//
//   - CheckResult: {"warnings": [診断...]}。
//   - EmitResult: {"warnings": [診断...], "written": [パス...], "removed": [パス...]}。
//   - 例外: docs/plugin-go-cli.md「実行と表示」の JSONPresenter と同じ {"error": "<例外の FQN>", "value": <値>}。
//     診断を運ぶ例外の value は docs/diagnostics.md「表示」の json（{"diagnostics", "errors", "warnings"}）。
//     UnknownNamespace は値を持たないので {"error": "hozoc.UnknownNamespace"}。宣言にない誤りは {"error": "<メッセージ>"}。
//
// 診断一件の形は docs/diagnostics.md「表示」の json の diagnostics の要素（diag.WriteJSON と同じ）。
type JSONPresenter struct{}

func (JSONPresenter) PresentCheck(w io.Writer, r CheckResult) error {
	items, err := diagnosticsJSON(r.Warnings, r.Files())
	if err != nil {
		return err
	}
	return writeJSON(w, struct {
		Warnings []json.RawMessage `json:"warnings"`
	}{items})
}

func (JSONPresenter) PresentEmit(w io.Writer, r EmitResult) error {
	items, err := diagnosticsJSON(r.Warnings, r.Files())
	if err != nil {
		return err
	}
	written, removed := r.Written, r.Removed
	if written == nil {
		written = []string{}
	}
	if removed == nil {
		removed = []string{}
	}
	return writeJSON(w, struct {
		Warnings []json.RawMessage `json:"warnings"`
		Written  []string          `json:"written"`
		Removed  []string          `json:"removed"`
	}{items, written, removed})
}

func (JSONPresenter) PresentError(w io.Writer, err error) error {
	type failure struct {
		Error string          `json:"error"`
		Value json.RawMessage `json:"value,omitempty"`
	}
	if d, name, ok := diagnosticsOf(err); ok {
		var buf bytes.Buffer
		if err := diag.WriteJSON(&buf, d.Items, d.Files); err != nil {
			return err
		}
		return writeJSON(w, failure{Error: name, Value: bytes.TrimSpace(buf.Bytes())})
	}
	var un *UnknownNamespace
	if errors.As(err, &un) {
		return writeJSON(w, failure{Error: "hozoc.UnknownNamespace"})
	}
	return writeJSON(w, failure{Error: err.Error()})
}

// diagnosticsOf は診断を運ぶ例外の診断と例外の FQN。
func diagnosticsOf(err error) (Diagnostics, string, bool) {
	var (
		cf *CheckFailed
		ci *ConfigInvalid
		lm *LanguageMismatch
		oc *OutputConflict
	)
	switch {
	case errors.As(err, &cf):
		return cf.Diagnostics, "hozoc.CheckFailed", true
	case errors.As(err, &ci):
		return ci.Diagnostics, "hozoc.ConfigInvalid", true
	case errors.As(err, &lm):
		return lm.Diagnostics, "hozoc.LanguageMismatch", true
	case errors.As(err, &oc):
		return oc.Diagnostics, "hozoc.OutputConflict", true
	}
	return Diagnostics{}, "", false
}

// diagnosticsJSON は診断の列を docs/diagnostics.md「表示」の json の要素にする。形を一つにするため diag.WriteJSON を通す。
func diagnosticsJSON(diags []diag.Diagnostic, fs *source.FileSet) ([]json.RawMessage, error) {
	var buf bytes.Buffer
	if err := diag.WriteJSON(&buf, diags, fs); err != nil {
		return nil, err
	}
	var out struct {
		Diagnostics []json.RawMessage `json:"diagnostics"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		return nil, err
	}
	if out.Diagnostics == nil {
		out.Diagnostics = []json.RawMessage{}
	}
	return out.Diagnostics, nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
