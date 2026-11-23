package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/mew-ton/hozo/internal/diag"
)

// ErrCrashed はプラグインの異常終了（例外 hozo.compiler.Crashed。docs/plugin-protocol.md「起動」の「それ以外」の終了コード、
// WASM の罠、または起動できなかった）。*CrashedError が Unwrap する。
var ErrCrashed = errors.New("hozo.compiler.Crashed")

// ErrMalformedOutput はプラグインの出力が約束した形（JSON）でなかった（例外 hozo.compiler.MalformedOutput）。
// *MalformedOutputError が Unwrap する。
var ErrMalformedOutput = errors.New("hozo.compiler.MalformedOutput")

// CrashedError は hozo.compiler.Crashed の中身。
type CrashedError struct {
	Plugin   string
	ExitCode int   // 起動できなかったときと、終了コードなしに止まったときは -1
	Err      error // 起動の失敗、WASM の罠など。なければ nil
}

func (e *CrashedError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("プラグイン %s を動かせない: %v", e.Plugin, e.Err)
	}
	return fmt.Sprintf("プラグイン %s が異常終了した（終了コード %d）", e.Plugin, e.ExitCode)
}

func (e *CrashedError) Unwrap() error { return ErrCrashed }

// MalformedOutputError は hozo.compiler.MalformedOutput の中身。
type MalformedOutputError struct {
	Plugin   string
	ExitCode int
	Reason   string
	Err      error
}

func (e *MalformedOutputError) Error() string {
	s := fmt.Sprintf("プラグイン %s の出力が約束した形でない: %s", e.Plugin, e.Reason)
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *MalformedOutputError) Unwrap() error { return ErrMalformedOutput }

// ErrorCode は終了コード 2 の応答 { "error": { "code": ..., "message": ... } } の code。
const (
	CodeProtocol    = "protocol"    // MAN-2
	CodeUnsupported = "unsupported" // MAN-5
)

// PluginError は終了コード 2 で返った { "error": { "code", "message" } }（docs/plugin-protocol.md「起動」）。
// code は CodeProtocol か CodeUnsupported。ほかの code は MalformedOutputError になる。
type PluginError struct {
	Plugin  string
	Code    string
	Message string
}

func (e *PluginError) Error() string {
	return fmt.Sprintf("プラグイン %s: %s: %s", e.Plugin, e.Code, e.Message)
}

// Diagnostic はこのエラーの診断。code "protocol" は MAN-2、"unsupported" は MAN-5（docs/plugin-protocol.md「診断」）。
// file はマニフェストのパス（診断の File）。
func (e *PluginError) Diagnostic(file string) diag.Diagnostic {
	code := "MAN-5"
	msg := fmt.Sprintf("プラグイン %s はこの direction かトランスパイラを実装していない: %s", e.Plugin, e.Message)
	if e.Code == CodeProtocol {
		code = "MAN-2"
		msg = fmt.Sprintf("プラグイン %s の protocol が合わない: %s", e.Plugin, e.Message)
	}
	return diag.Diagnostic{Severity: diag.Error, Code: code, Message: msg, File: file}
}

// errorResponse は終了コード 2 の応答の形。
type errorResponse struct {
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Runner はプラグインを起動する側の設定（docs/plugin-protocol.md「起動」）。
type Runner struct {
	// Stderr はプラグインの標準エラーを流す先。nil なら os.Stderr。各行の先頭に "[<プラグイン名>] " を付ける。
	Stderr io.Writer
	// Context は、動いているプラグインを、途中で止める口。nil なら、止めない。止めたとき、Run は、Context の誤りを返す。
	// 止める口を持つ Context（Done が nil でない）を渡した起動は、途中で止められる形の WASM を使い、その分、遅い
	// （docs/plugin-protocol.md「起動」）。
	Context context.Context
	// CacheDir は、WASM を、機械の言葉に直したものを、覚える置き場（HOZO_CACHE_DIR の値）。"" なら、os.UserCacheDir の下の
	// hozo。どちらも、その下の wasm/ を使う。決められない・作れないときは、覚えずに動く。
	CacheDir string
}

// runMu は起動を直列にする（docs/plugin-protocol.md「起動」: 並列には起動しない）。engines と、その中身も、これが守る。
var runMu sync.Mutex

// Run は、WASM のファイル wasm のプラグイン（名前 name）を、サンドボックスの中で一度動かし、req を JSON にして標準入力に渡し、
// 標準出力の JSON を resp に読む（docs/plugin-protocol.md「起動」）。プラグインに渡すのは、標準入力、標準出力、標準エラーと、
// 決まった値の時計と乱数だけ: ファイル、ネットワーク、環境変数、引数は、渡さない。
//
// 終了コードと返すエラー: 0 で JSON が読め、形が合えば nil。0 で JSON でない・形が違えば *MalformedOutputError
// （形を確かめるのは、resp が *sdk.EmitBatchResponse か *sdk.CoverageResponse のとき。docs/plugin-protocol.md「応答の形」。shape.go）。
// 2 で { "error": { "code": "protocol" | "unsupported" } } なら *PluginError。2 でそれ以外なら *MalformedOutputError。
// ほかの終了コードと、WASM の罠と、起動の失敗（ファイルがない、WASM として読めない）は *CrashedError。
// Context で止めたときは、Context の誤り。
func (r Runner) Run(name, wasm string, req any, resp any) error {
	runMu.Lock()
	defer runMu.Unlock()

	input, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("plugin: 要求を JSON にできない: %w", err)
	}
	ctx := r.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stderr := r.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	var stdout bytes.Buffer
	errOut := &prefixWriter{w: stderr, prefix: "[" + name + "] "}
	exitCode, err := runWasm(ctx, r.CacheDir, wasm, bytes.NewReader(input), &stdout, errOut)
	errOut.flush()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return &CrashedError{Plugin: name, ExitCode: -1, Err: err}
	}
	switch exitCode {
	case 0:
		// JSON として一つ読めること。次に、形（必須の項目と型。shape.go）。形を先に確かめてから resp に読むのは、
		// encoding/json が、ない項目と null を黙って零値にするため。
		var raw json.RawMessage
		if err := decodeOne(stdout.Bytes(), &raw); err != nil {
			return &MalformedOutputError{Plugin: name, ExitCode: 0, Reason: "標準出力を応答の JSON として読めない", Err: err}
		}
		if err := checkShape(raw, resp); err != nil {
			return &MalformedOutputError{Plugin: name, ExitCode: 0, Reason: "応答の形が違う", Err: err}
		}
		if err := json.Unmarshal(raw, resp); err != nil {
			return &MalformedOutputError{Plugin: name, ExitCode: 0, Reason: "標準出力を応答の JSON として読めない", Err: err}
		}
		return nil
	case 2:
		var er errorResponse
		if err := decodeOne(stdout.Bytes(), &er); err != nil || er.Error == nil {
			return &MalformedOutputError{Plugin: name, ExitCode: 2, Reason: "終了コード 2 だが標準出力が { \"error\": { \"code\", \"message\" } } でない", Err: err}
		}
		switch er.Error.Code {
		case CodeProtocol, CodeUnsupported:
			return &PluginError{Plugin: name, Code: er.Error.Code, Message: er.Error.Message}
		default:
			return &MalformedOutputError{Plugin: name, ExitCode: 2, Reason: fmt.Sprintf("終了コード 2 の error の code %q を知らない", er.Error.Code)}
		}
	default:
		return &CrashedError{Plugin: name, ExitCode: exitCode}
	}
}

// decodeOne は data を一つの JSON の値として v に読む。後ろに空白以外があれば誤り。
func decodeOne(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("JSON の後ろに余計な内容がある")
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return errors.New("JSON の後ろに余計な内容がある")
	}
	return nil
}

// prefixWriter は、書かれたものを、行ごとに、先頭に prefix を付けて w に書く。行の途中は、改行が来るまで持ち、flush で、
// 改行を足して書く。
type prefixWriter struct {
	w      io.Writer
	prefix string
	buf    []byte
}

func (p *prefixWriter) Write(data []byte) (int, error) {
	p.buf = append(p.buf, data...)
	for {
		i := bytes.IndexByte(p.buf, '\n')
		if i < 0 {
			return len(data), nil
		}
		_, _ = io.WriteString(p.w, p.prefix+string(p.buf[:i+1]))
		p.buf = p.buf[i+1:]
	}
}

func (p *prefixWriter) flush() {
	if len(p.buf) > 0 {
		_, _ = io.WriteString(p.w, p.prefix+string(p.buf)+"\n")
		p.buf = nil
	}
}
