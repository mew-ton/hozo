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
	"reflect"
	"sort"
	"strings"

	"github.com/mew-ton/hozo/ir"
)

// Handler はプラグインが実装する四つの方向（direction）。
// 実装しない方向と、Emit で知らないトランスパイラ（EmitRequest.Transpiler）は ErrUnsupported を返す。Main はそれを
// docs/plugin-protocol.md「起動」の code "unsupported"（終了コード 2）にする。
type Handler interface {
	Emit(req *EmitRequest) (*EmitResponse, error)
	Check(req *CheckRequest) (*CheckResponse, error)
	Import(req *ImportRequest) (*ImportResponse, error)
	Coverage(req *CoverageRequest) (*CoverageResponse, error)
}

// ErrUnsupported は、Handler がその direction か、要求のトランスパイラを実装していないときに返す誤り。
// errors.Is で比べるので、包んで返してもよい。
var ErrUnsupported = errors.New("plugin: direction not supported")

// EmitRequest は direction "emit" の要求（docs/plugin-protocol.md「emit」）。
// Transpiler は起動されたトランスパイラの名前（マニフェストの [[transpiler]] の name）。複数のトランスパイラを
// 提供するプラグインは、この名前で処理を分ける。View.Viewer が対象の名前空間で、View.Config がその単位の設定。
type EmitRequest struct {
	Protocol   string   `json:"protocol"`
	Direction  string   `json:"direction"`
	Transpiler string   `json:"transpiler"`
	View       *ir.View `json:"view"`
}

// EmitResponse は "emit" の応答。Errors が一つでもあれば hozoc は Files を書き出さない。
// 何を出すかを hozoc に伝える項目（版 0.3 までの select）はない（docs/plugin-protocol.md「選択をなくしたこと」）。
type EmitResponse struct {
	Files    []File       `json:"files"`
	Warnings []Warning    `json:"warnings"`
	Errors   []Diagnostic `json:"errors,omitempty"`
}

// Version は、このリポジトリのプラグインの版。生成したファイルのヘッダに書く（docs/plugin-go.md「ファイル」）。
const Version = "0.0.0-dev"

// Options は、起動されたトランスパイラへのオプション req.View.Config.Options[req.Transpiler]（docs/config.md「オプション」、
// docs/plugin-protocol.md「emit」）を into に読む。into は構造体へのポインタで、フィールドは json タグで結ぶ。
// オプションが書かれていなければ（キーがない、View がない）into はそのまま。戻り値は、into にないキー（辞書順）。
// 型の違う値は error。知らないキーと error を診断にするのは呼ぶ側（それぞれのプラグインの文書の「オプション」「診断」）。
func Options(req *EmitRequest, into any) ([]string, error) {
	if req == nil || req.View == nil {
		return nil, nil
	}
	table, ok := req.View.Config.Options[req.Transpiler]
	if !ok || len(table) == 0 {
		return nil, nil
	}
	known := map[string]bool{}
	if t := reflect.TypeOf(into); t != nil && t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct {
		for i := 0; i < t.Elem().NumField(); i++ {
			f := t.Elem().Field(i)
			if !f.IsExported() {
				continue
			}
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			known[name] = true
		}
	}
	var unknown []string
	rest := map[string]any{}
	for k, v := range table {
		if known[k] {
			rest[k] = v
		} else {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	data, err := json.Marshal(rest)
	if err != nil {
		return unknown, err
	}
	if err := json.Unmarshal(data, into); err != nil {
		return unknown, fmt.Errorf("options of %s: %w", req.Transpiler, err)
	}
	return unknown, nil
}

// OpsWith は view.Viewer の名前空間の op のうち、単位の対象（Span.File が view.Files にある。docs/config.md「in」）で、
// デコレータ decorator（FQN）が付いたものの FQN を宣言の順で返す。名前付きデコレータを通して付いたもの
// （Application.Via）も数える。相乗りが自分の対象を探すための道具（docs/plugin-protocol.md「選択をなくしたこと」）。
// 一つもなければ空の列（nil ではない）。
func OpsWith(view *ir.View, decorator string) []string {
	out := []string{}
	if view == nil || view.Program == nil {
		return out
	}
	ns := view.Program.Namespace(view.Viewer)
	if ns == nil {
		return out
	}
	files := map[string]bool{}
	for _, f := range view.Files {
		files[f] = true
	}
	for _, d := range ns.Decls {
		op, ok := d.(*ir.Op)
		if !ok || !files[op.Span.File] {
			continue
		}
		for _, app := range op.Decorators {
			if app.Decorator == decorator || app.Via == decorator {
				out = append(out, op.Name)
				break
			}
		}
	}
	return out
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

// CheckByEmit は、direction "check" を、emit と同じ検査で答える（docs/plugin-protocol.md「check」の「emit が errors で返す誤りは、
// check でも返す」）。Handler の Check が、これを呼ぶ: transpilers は、そのプラグインが提供するトランスパイラの名前。
//
// req.View.Config.Transpilers（その単位の transpilers。書かれた順）のうち、transpilers にあるものごとに、同じ View で h.Emit を呼び、
// 応答の Errors を、Severity が "error" の Diagnostics にして、その順に並べる。Files と Warnings は、捨てる（warning は、emit の
// ときにだけ出る）。h.Emit が誤りを返せば、その誤りを返す。
func CheckByEmit(h Handler, transpilers []string, req *CheckRequest) (*CheckResponse, error) {
	if req == nil || req.View == nil {
		return nil, errors.New("check: request has no view")
	}
	provided := map[string]bool{}
	for _, name := range transpilers {
		provided[name] = true
	}
	out := &CheckResponse{Diagnostics: []Diagnostic{}}
	for _, name := range req.View.Config.Transpilers {
		if !provided[name] {
			continue
		}
		resp, err := h.Emit(&EmitRequest{Protocol: req.Protocol, Direction: "emit", Transpiler: name, View: req.View})
		if err != nil {
			return nil, err
		}
		if resp == nil {
			continue
		}
		for _, d := range resp.Errors {
			d.Severity = "error"
			out.Diagnostics = append(out.Diagnostics, d)
		}
	}
	return out, nil
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

// CoverageRequest は direction "coverage" の要求（docs/plugin-protocol.md「coverage」）。
// Coverage は hozoc tests に渡されたカバレッジのファイル（Path は絶対パス）。Files は、実行された行を知りたいファイル
// （プロジェクトのルートからの相対パス）。
type CoverageRequest struct {
	Protocol  string   `json:"protocol"`
	Direction string   `json:"direction"`
	Coverage  File     `json:"coverage"`
	Files     []string `json:"files"`
}

// CoverageResponse は "coverage" の応答。Recognized が false なら、このプラグインが読める形のファイルではなかった。
// Files は、要求の Files のうち、カバレッジが測っていたファイルごとに一つ。
type CoverageResponse struct {
	Recognized bool          `json:"recognized"`
	Files      []CoveredFile `json:"files"`
}

// CoveredFile は、測られていたファイル一つの、実行された所の行の範囲（始まりの行、終わりの行。1 始まりで両端を含む）。
// 一度も実行されていなければ Lines は空。
type CoveredFile struct {
	Path  string   `json:"path"`
	Lines [][2]int `json:"lines"`
}

// File は書き出す（または取り込む）ファイル。Path はプロジェクトのルートからの相対パス（docs/plugin-protocol.md「emit」）。
// emit の応答では、その起動の出力先（EmitRequest.View.Config.Dir。Dir(U, N)）の下でなければならず、そうでないパス
// （`..`、絶対パス、別のディレクトリ）は PLG-1。Content は UTF-8 の文字列（バイナリは出せない）。
type File struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Warning は warning の診断。At は FQN か "file:line:col"（どちらでもなければ位置なし）。
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
// それ以外の誤り（要求が JSON のオブジェクトとして読めない、h が誤りを返した、書き出せない）なら 1（標準エラーにメッセージ）。
// protocol は文字列で、この SDK の版と等しくなければ「合わない」（ない、null、文字列でないものを含む）。
// TS の SDK（sdk/ts の run）も同じ終了コードを返す。
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
	// protocol と direction は、型も確かめずに先に見る: protocol が合わない要求は、ほかの項目の形が違っていても
	// （版が違えば違って当然）code "protocol" で断る。文字列でない protocol も「合わない」（docs/plugin-protocol.md
	// 「要求と応答」）。文字列でない direction は、実装していない direction と同じ。
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fail(errOut, fmt.Errorf("decode request: %w", err))
	}
	if fields == nil {
		return fail(errOut, errors.New("decode request: not a JSON object"))
	}
	var head struct{ Protocol, Direction string }
	if json.Unmarshal(fields["protocol"], &head.Protocol) != nil || head.Protocol != ir.Version {
		got := `""`
		if p, ok := fields["protocol"]; ok {
			got = compact(p)
		}
		return refuse(out, errOut, "protocol",
			fmt.Sprintf("protocol %s is not supported (this plugin speaks %q)", got, ir.Version))
	}
	if json.Unmarshal(fields["direction"], &head.Direction) != nil {
		head.Direction = ""
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
	case "coverage":
		var req CoverageRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return fail(errOut, fmt.Errorf("decode coverage request: %w", err))
		}
		r, err := h.Coverage(&req)
		if err != nil {
			return handlerError(out, errOut, head.Direction, err)
		}
		if r == nil {
			r = &CoverageResponse{}
		}
		if r.Files == nil {
			r.Files = []CoveredFile{}
		}
		for i := range r.Files {
			if r.Files[i].Lines == nil {
				r.Files[i].Lines = [][2]int{}
			}
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
		msg := fmt.Sprintf("direction %q is not supported", direction)
		if err != ErrUnsupported { // 包んであれば、その文（どのトランスパイラを知らないか、など）も伝える
			msg = err.Error()
		}
		return refuse(out, errOut, "unsupported", msg)
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

// compact は JSON の値を、メッセージに出すために一行にする。
func compact(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
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
