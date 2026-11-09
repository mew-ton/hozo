// Package plugin は Go で書くプラグインのための SDK。
//
// プラグインは標準入出力で JSON をやり取りする実行ファイル（設計書 §4.6）。
// JSON の形は docs/plugin-protocol.md「要求と応答」で、このパッケージの型はそれを写す。
// 起動・終了コード・標準出力の読み方は同「起動」。API の形は docs/packages.md「`plugin`（公開）」。
// プラグインの main は `os.Exit(plugin.Main(handler{}))` と書く。
package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mew-ton/hozo/ir"
)

// Handler はプラグインが実装する三つの方向（direction）。
// 実装しない方向は ErrUnsupported を返す。Main はそれを
// docs/plugin-protocol.md「起動」の code "unsupported"（終了コード 2）にする。
type Handler interface {
	Emit(req *EmitRequest) (*EmitResponse, error)
	Check(req *CheckRequest) (*CheckResponse, error)
	Import(req *ImportRequest) (*ImportResponse, error)
}

// ErrUnsupported は、Handler がその direction を実装していないときに返す誤り。
// errors.Is で比べるので、包んで返してもよい。
var ErrUnsupported = errors.New("plugin: direction not supported")

// EmitRequest は direction "emit" の要求（docs/plugin-protocol.md「emit」）。
// View.Viewer が対象の名前空間。
type EmitRequest struct {
	Protocol  string   `json:"protocol"`
	Direction string   `json:"direction"`
	View      *ir.View `json:"view"`
}

// EmitResponse は "emit" の応答。Errors が一つでもあれば hozoc は Files を書き出さない。
type EmitResponse struct {
	Files    []File       `json:"files"`
	Warnings []Warning    `json:"warnings"`
	Errors   []Diagnostic `json:"errors,omitempty"`
}

// CheckRequest は direction "check" の要求（docs/plugin-protocol.md「check」）。
type CheckRequest struct {
	Protocol  string   `json:"protocol"`
	Direction string   `json:"direction"`
	View      *ir.View `json:"view"`
}

// CheckResponse は "check" の応答。
type CheckResponse struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// ImportRequest は direction "import" の要求（docs/plugin-protocol.md「import」）。
// Format はマニフェストの [import].formats の一つ。
type ImportRequest struct {
	Protocol  string `json:"protocol"`
	Direction string `json:"direction"`
	Format    string `json:"format"`
	Files     []File `json:"files"`
}

// ImportResponse は "import" の応答。IR は docs/ir.md の Program。
type ImportResponse struct {
	IR       *ir.Program `json:"ir"`
	Warnings []Warning   `json:"warnings"`
}

// File は書き出す（または取り込む）ファイル。Path は view.config.out からの相対パスで、
// `..` や絶対パスは PLG-1。Content は UTF-8 の文字列（バイナリは出せない）。
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Warning は warning の診断。At は FQN か "file:line:col"。
type Warning struct {
	At      string `json:"at"`
	Message string `json:"message"`
}

// Diagnostic は "check" の diagnostics と "emit" の errors の要素。
// Severity は "error" か "warning"（emit の errors では常に error なので省く）。
// Code はプラグインが決める規則の ID（PAGE-1、CLI-1 など）。At は FQN か "file:line:col"。
type Diagnostic struct {
	Severity string `json:"severity,omitempty"`
	Code     string `json:"code"`
	At       string `json:"at"`
	Message  string `json:"message"`
}

// Main は標準入力の要求を読み、protocol を確かめ、h を呼び、標準出力に応答を書く。
// 終了コードを返す（docs/plugin-protocol.md「起動」）: 成功なら 0、protocol が合わないか
// direction を実装していなければ 2（標準出力に {"error":{"code":...,"message":...}}）、
// それ以外の誤り（要求が読めない、h が誤りを返した、書き出せない）なら 1（標準エラーにメッセージ）。
func Main(h Handler) int {
	return Run(h, os.Stdin, os.Stdout, os.Stderr)
}

// Run は Main の本体で、入出力を差し替えられる。in から要求を一つ読み、out に応答を、
// errOut に誤りのメッセージを書く。
func Run(h Handler, in io.Reader, out, errOut io.Writer) int {
	raw, err := io.ReadAll(in)
	if err != nil {
		return fail(errOut, fmt.Errorf("read request: %w", err))
	}
	var head struct {
		Protocol  string `json:"protocol"`
		Direction string `json:"direction"`
	}
	if err := json.Unmarshal(raw, &head); err != nil {
		return fail(errOut, fmt.Errorf("decode request: %w", err))
	}
	if head.Protocol != ir.Version {
		return refuse(out, errOut, "protocol",
			fmt.Sprintf("protocol %q is not supported (this plugin speaks %q)", head.Protocol, ir.Version))
	}

	var resp any
	switch head.Direction {
	case "emit":
		var req EmitRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return fail(errOut, fmt.Errorf("decode emit request: %w", err))
		}
		r, err := h.Emit(&req)
		if err != nil {
			return handlerError(out, errOut, head.Direction, err)
		}
		if r == nil {
			r = &EmitResponse{}
		}
		if r.Files == nil {
			r.Files = []File{}
		}
		if r.Warnings == nil {
			r.Warnings = []Warning{}
		}
		resp = r
	case "check":
		var req CheckRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return fail(errOut, fmt.Errorf("decode check request: %w", err))
		}
		r, err := h.Check(&req)
		if err != nil {
			return handlerError(out, errOut, head.Direction, err)
		}
		if r == nil {
			r = &CheckResponse{}
		}
		if r.Diagnostics == nil {
			r.Diagnostics = []Diagnostic{}
		}
		resp = r
	case "import":
		var req ImportRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return fail(errOut, fmt.Errorf("decode import request: %w", err))
		}
		r, err := h.Import(&req)
		if err != nil {
			return handlerError(out, errOut, head.Direction, err)
		}
		if r == nil {
			r = &ImportResponse{}
		}
		if r.Warnings == nil {
			r.Warnings = []Warning{}
		}
		resp = r
	default:
		return refuse(out, errOut, "unsupported", fmt.Sprintf("direction %q is not supported", head.Direction))
	}

	if err := writeJSON(out, resp); err != nil {
		return fail(errOut, fmt.Errorf("write response: %w", err))
	}
	return 0
}

// handlerError は Handler の誤りを終了コードにする。ErrUnsupported なら 2、それ以外は 1。
func handlerError(out, errOut io.Writer, direction string, err error) int {
	if errors.Is(err, ErrUnsupported) {
		return refuse(out, errOut, "unsupported", fmt.Sprintf("direction %q is not supported", direction))
	}
	return fail(errOut, fmt.Errorf("%s: %w", direction, err))
}

// refuse は docs/plugin-protocol.md「起動」の終了コード 2 の形を out に書く。
func refuse(out, errOut io.Writer, code, message string) int {
	body := struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{}
	body.Error.Code = code
	body.Error.Message = message
	if err := writeJSON(out, body); err != nil {
		return fail(errOut, fmt.Errorf("write response: %w", err))
	}
	return 2
}

// fail は終了コード 1。メッセージは標準エラーへ（hozoc が `[<プラグイン名>] ` を付けて流す）。
func fail(errOut io.Writer, err error) int {
	fmt.Fprintf(errOut, "error: %v\n", err)
	return 1
}

func writeJSON(out io.Writer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return err
	}
	_, err := out.Write(buf.Bytes())
	return err
}
