package plugin

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/mew-ton/hozo/internal/diag"
)

// ErrCrashed はプラグインの異常終了（例外 proc.Crashed。docs/plugin-protocol.md「起動」の「それ以外」の終了コード、
// または起動できなかった）。*CrashedError が Unwrap する。
var ErrCrashed = errors.New("proc.Crashed")

// ErrMalformedOutput はプラグインの出力が約束した形（JSON）でなかった（例外 proc.MalformedOutput）。
// *MalformedOutputError が Unwrap する。
var ErrMalformedOutput = errors.New("proc.MalformedOutput")

// CrashedError は proc.Crashed の中身。
type CrashedError struct {
	Plugin   string
	ExitCode int   // 起動できなかったときは -1
	Err      error // 起動の失敗など。なければ nil
}

func (e *CrashedError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("プラグイン %s を起動できない: %v", e.Plugin, e.Err)
	}
	return fmt.Sprintf("プラグイン %s が異常終了した（終了コード %d）", e.Plugin, e.ExitCode)
}

func (e *CrashedError) Unwrap() error { return ErrCrashed }

// MalformedOutputError は proc.MalformedOutput の中身。
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
	// Root はプロジェクトのルート。起動したプロセスの作業ディレクトリ。
	Root string
	// Stderr はプラグインの標準エラーを流す先。nil なら os.Stderr。各行の先頭に "[<プラグイン名>] " を付ける。
	Stderr io.Writer
	// Env は起動したプロセスの環境変数。nil なら os.Environ()。足すものはない（プラグインは、要求のほかに頼らない）。
	Env []string
}

// runMu は起動を直列にする（docs/plugin-protocol.md「起動」: 並列には起動しない）。
var runMu sync.Mutex

// Run はプラグインを一度起動し、req を JSON にして標準入力に書き、標準出力の JSON を resp に読む
// （docs/plugin-protocol.md「起動」）。dir はマニフェストのディレクトリ。
//
// 終了コードと返すエラー: 0 で JSON が読め、形が合えば nil。0 で JSON でない・形が違えば *MalformedOutputError
// （形を確かめるのは、resp が *sdk.EmitBatchResponse か *sdk.CoverageResponse のとき。docs/plugin-protocol.md「応答の形」。shape.go）。
// 2 で { "error": { "code": "protocol" | "unsupported" } } なら *PluginError。2 でそれ以外なら *MalformedOutputError。
// ほかの終了コードと起動の失敗は *CrashedError。
func (r Runner) Run(m *Manifest, dir string, req any, resp any) error {
	runMu.Lock()
	defer runMu.Unlock()

	input, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("plugin: 要求を JSON にできない: %w", err)
	}
	absRoot, err := filepath.Abs(r.Root)
	if err != nil {
		return &CrashedError{Plugin: m.Name, ExitCode: -1, Err: err}
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return &CrashedError{Plugin: m.Name, ExitCode: -1, Err: err}
	}

	cmd := exec.Command(m.ResolveCommand(absDir), m.Command[1:]...)
	cmd.Dir = absRoot
	env := r.Env
	if env == nil {
		env = os.Environ()
	}
	cmd.Env = append([]string{}, env...)
	cmd.Stdin = bytes.NewReader(input)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return &CrashedError{Plugin: m.Name, ExitCode: -1, Err: err}
	}
	stderr := r.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		return &CrashedError{Plugin: m.Name, ExitCode: -1, Err: err}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		prefixLines(stderr, stderrPipe, "["+m.Name+"] ")
	}()
	<-done
	waitErr := cmd.Wait()

	exitCode := 0
	if waitErr != nil {
		var ee *exec.ExitError
		if !errors.As(waitErr, &ee) {
			return &CrashedError{Plugin: m.Name, ExitCode: -1, Err: waitErr}
		}
		exitCode = ee.ExitCode()
	}
	switch exitCode {
	case 0:
		// JSON として一つ読めること。次に、形（必須の項目と型。shape.go）。形を先に確かめてから resp に読むのは、
		// encoding/json が、ない項目と null を黙って零値にするため。
		var raw json.RawMessage
		if err := decodeOne(stdout.Bytes(), &raw); err != nil {
			return &MalformedOutputError{Plugin: m.Name, ExitCode: 0, Reason: "標準出力を応答の JSON として読めない", Err: err}
		}
		if err := checkShape(raw, resp); err != nil {
			return &MalformedOutputError{Plugin: m.Name, ExitCode: 0, Reason: "応答の形が違う", Err: err}
		}
		if err := json.Unmarshal(raw, resp); err != nil {
			return &MalformedOutputError{Plugin: m.Name, ExitCode: 0, Reason: "標準出力を応答の JSON として読めない", Err: err}
		}
		return nil
	case 2:
		var er errorResponse
		if err := decodeOne(stdout.Bytes(), &er); err != nil || er.Error == nil {
			return &MalformedOutputError{Plugin: m.Name, ExitCode: 2, Reason: "終了コード 2 だが標準出力が { \"error\": { \"code\", \"message\" } } でない", Err: err}
		}
		switch er.Error.Code {
		case CodeProtocol, CodeUnsupported:
			return &PluginError{Plugin: m.Name, Code: er.Error.Code, Message: er.Error.Message}
		default:
			return &MalformedOutputError{Plugin: m.Name, ExitCode: 2, Reason: fmt.Sprintf("終了コード 2 の error の code %q を知らない", er.Error.Code)}
		}
	default:
		return &CrashedError{Plugin: m.Name, ExitCode: exitCode}
	}
}

// Run は Runner{Root: root}.Run。
func Run(root string, m *Manifest, dir string, req any, resp any) error {
	return Runner{Root: root}.Run(m, dir, req, resp)
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

// prefixLines は r の各行の先頭に prefix を付けて w に書く。最後の行に改行がなくても付けて書く。
func prefixLines(w io.Writer, r io.Reader, prefix string) {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			if line[len(line)-1] != '\n' {
				line += "\n"
			}
			_, _ = io.WriteString(w, prefix+line)
		}
		if err != nil {
			return
		}
	}
}
