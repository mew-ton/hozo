// Package plugin は Go で書くプラグインのための SDK。
//
// プラグインは標準入出力で JSON をやり取りするプログラム（設計書 §4.6）。hozoc は、それを WASM（GOOS=wasip1 GOARCH=wasm）に
// ビルドしたものを、サンドボックスの中で動かす（docs/plugin-protocol.md「起動」）。
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

// Handler は、トランスパイラを提供するプラグインが実装する三つの方向（direction）。語彙のプラグインの方向は [Reader]。
// 実装しない方向と、Emit で知らないトランスパイラ（EmitRequest.Transpiler）は ErrUnsupported を返す。Main はそれを
// docs/plugin-protocol.md「起動」の code "unsupported"（終了コード 2）にする。
//
// Emit は、トランスパイラ一つが、出力一つに答える関数。direction "emit" の要求は、プラグインが受け持つ出力と、出力ごとに頼む
// トランスパイラを並べている（同「emit」）が、その並びを回すのは [Run] で、Emit は、一回の呼び出しで、一つの答えを返す。
// 答えは、渡された View だけから作る（同「起動」: 答えは、出力ごとに独立）。
type Handler interface {
	Emit(req *EmitRequest) (*EmitResponse, error)
	Import(req *ImportRequest) (*ImportResponse, error)
	Coverage(req *CoverageRequest) (*CoverageResponse, error)
}

// Reader は、語彙のプラグイン（docs/plugin-protocol.md「語彙のプラグイン」）が実装する、ただ一つの方向（direction "read"）:
// 仕様を受け取り、その語彙の、言語に依らない決まりの診断を返す。[Run] は、渡された [Handler] が、Reader も満たしていれば、
// direction "read" で、これを呼ぶ（満たしていなければ、code "unsupported"）。read だけのプラグインは、[ReadOnly] で Handler にする。
type Reader interface {
	Read(req *ReadRequest) (*ReadResponse, error)
}

// ReadRequest は direction "read" の要求（docs/plugin-protocol.md「read」）: 仕様（emit の要求の spec と同じ形）だけ。
// hozoc が、要求を作るのにも使う。
type ReadRequest struct {
	Protocol  string   `json:"protocol"`
	Direction string   `json:"direction"`
	Spec      *ir.Spec `json:"spec"`
}

// NewReadRequest は、仕様 spec を読み解かせる、direction "read" の要求。
func NewReadRequest(spec *ir.Spec) *ReadRequest {
	return &ReadRequest{Protocol: ir.Version, Direction: "read", Spec: spec}
}

// ReadResponse は direction "read" の応答: error の診断と、warning。どちらも、空でも [] で出る（[Run] が、nil を [] にする）。
type ReadResponse struct {
	Errors   []Diagnostic `json:"errors"`
	Warnings []Warning    `json:"warnings"`
}

// ReadOnly は、read だけを実装するプラグイン（語彙のプラグイン）の [Handler]: emit、import、coverage は [ErrUnsupported]。
func ReadOnly(r Reader) Handler { return readOnly{r} }

type readOnly struct{ Reader }

func (readOnly) Emit(*EmitRequest) (*EmitResponse, error)             { return nil, ErrUnsupported }
func (readOnly) Import(*ImportRequest) (*ImportResponse, error)       { return nil, ErrUnsupported }
func (readOnly) Coverage(*CoverageRequest) (*CoverageResponse, error) { return nil, ErrUnsupported }

// ErrUnsupported は、Handler がその direction か、要求のトランスパイラを実装していないときに返す誤り。
// errors.Is で比べるので、包んで返してもよい。
var ErrUnsupported = errors.New("plugin: direction not supported")

// EmitRequest は、[Handler] の Emit の、一回の呼び出し: 頼まれたトランスパイラ一つと、出力一つ。やり取りの JSON の形
// （[EmitBatch]）そのものではなく、[Run] が、要求の spec と、出力の view を合わせて作る（docs/plugin-protocol.md「emit」）。
// Transpiler は頼まれたトランスパイラの名前（マニフェストの [[transpiler]] の name）。複数のトランスパイラを
// 提供するプラグインは、この名前で処理を分ける。View.Viewer が対象の名前空間で、View.Config がその単位の設定。
type EmitRequest struct {
	Protocol   string   `json:"protocol"`
	Direction  string   `json:"direction"`
	Transpiler string   `json:"transpiler"`
	View       *ir.View `json:"view"`
}

// EmitResponse は、トランスパイラ一つの、出力一つへの答え（"emit" の応答の results の要素）。Errors が一つでもあれば
// hozoc は Files を書き出さない。
// 何を出すかを hozoc に伝える項目（版 0.3 までの select）はない（docs/plugin-protocol.md「選択をなくしたこと」）。
type EmitResponse struct {
	Files    []File       `json:"files"`
	Warnings []Warning    `json:"warnings"`
	Errors   []Diagnostic `json:"errors,omitempty"`
}

// EmitBatch は direction "emit" の要求の、JSON の形（docs/plugin-protocol.md「emit」）: 仕様を一度と、プラグインが受け持つ
// 出力の並び。プラグインの書き手は、触らない（[Run] が読む）。hozoc が、要求を作るのに使う。
type EmitBatch struct {
	Protocol  string       `json:"protocol"`
	Direction string       `json:"direction"`
	Spec      *ir.Spec     `json:"spec"`
	Outputs   []EmitOutput `json:"outputs"`
}

// NewEmitBatch は、View の並び views と、View ごとに頼むトランスパイラの名前 transpilers（views と同じ長さ）から、
// direction "emit" の要求を作る（docs/plugin-protocol.md「emit」）。仕様（spec）は、最初の View のもの: views は、同じ仕様の
// View（出力に依らない部分が、同じ）でなければならない。views が空なら、spec は nil。
func NewEmitBatch(views []*ir.View, transpilers [][]string) *EmitBatch {
	req := &EmitBatch{Protocol: ir.Version, Direction: "emit", Outputs: []EmitOutput{}}
	for i, v := range views {
		spec, out := v.Split()
		if i == 0 {
			req.Spec = spec
		}
		names := []string{}
		if i < len(transpilers) {
			names = append(names, transpilers[i]...)
		}
		req.Outputs = append(req.Outputs, EmitOutput{Transpilers: names, View: out})
	}
	return req
}

// Batch は、この一回の呼び出し（トランスパイラ一つ、出力一つ）だけを頼む、direction "emit" の要求。プラグインのプログラムを、
// テストで、標準入出力から動かすときに使う（応答は [SingleResult] で読む）。
func (r *EmitRequest) Batch() *EmitBatch {
	b := NewEmitBatch([]*ir.View{r.View}, [][]string{{r.Transpiler}})
	if r.Protocol != "" {
		b.Protocol = r.Protocol
	}
	return b
}

// SingleResult は、direction "emit" の応答 data（JSON）の、ただ一つの答え（outputs が一つで、その results が一つ）を返す。
// そういう形でなければ、誤り。[EmitRequest.Batch] の要求への応答を読む。
func SingleResult(data []byte) (*EmitResponse, error) {
	var resp EmitBatchResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	if len(resp.Outputs) != 1 || len(resp.Outputs[0].Results) != 1 {
		return nil, fmt.Errorf("emit response does not hold exactly one result (outputs: %d)", len(resp.Outputs))
	}
	return &resp.Outputs[0].Results[0], nil
}

// EmitOutput は、要求の outputs の要素: 出力一つと、その出力で頼むトランスパイラの名前（単位の transpilers に書いた順）。
type EmitOutput struct {
	Transpilers []string       `json:"transpilers"`
	View        *ir.OutputView `json:"view"`
}

// EmitBatchResponse は direction "emit" の応答の、JSON の形: 要求の outputs と、同じ数、同じ順。
type EmitBatchResponse struct {
	Outputs []OutputResult `json:"outputs"`
}

// OutputResult は、応答の outputs の要素: その出力の transpilers と、同じ数、同じ順の、答え。
type OutputResult struct {
	Results []EmitResponse `json:"results"`
}

// Version は、このリポジトリのプラグインの版。生成したファイルのヘッダに書く（plugins/go/spec/go.hozo の Requirement.Layout の「ファイル」）。
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
// （プロジェクトのルートからの相対パス）。Outputs は、Files を出した出力の設定（単位の設定の順、その中は名前空間の IR の順）。
// プラグインは、ファイルを読まないので、設定から決まるもの（Go のモジュールの名前など）は、ここから取る。
type CoverageRequest struct {
	Protocol  string               `json:"protocol"`
	Direction string               `json:"direction"`
	Coverage  File                 `json:"coverage"`
	Files     []string             `json:"files"`
	Outputs   []ir.NamespaceConfig `json:"outputs"`
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

// Diagnostic は "emit" の errors の要素。
// Severity は、hozoc が読まない（errors は、常に error。省く）。
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
// protocol は文字列で、この SDK の版（ir.Version）と合わなければ「合わない」（ない、null、文字列でないものを含む）。
// 合うのは、大きい版が同じで、要求の小さい版が、この SDK の小さい版以上のとき（docs/plugin-protocol.md「版の合わせ方」）。
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
	if json.Unmarshal(fields["protocol"], &head.Protocol) != nil || !ir.VersionAccepts(head.Protocol, ir.Version) {
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
		// 知らない項目は、どの階層でも、読まずに無視する（encoding/json と ir の読み方が、そうなっている。
		// docs/plugin-protocol.md「要求と応答」）
		var req EmitBatch
		if err := json.Unmarshal(raw, &req); err != nil {
			return fail(errOut, fmt.Errorf("decode emit request: %w", err))
		}
		r, err := emitBatch(h, &req)
		if err != nil {
			return handlerError(out, errOut, head.Direction, err)
		}
		resp = r
	case "read":
		reader, ok := h.(Reader)
		if !ok {
			return refuse(out, errOut, "unsupported", fmt.Sprintf("direction %q is not supported", head.Direction))
		}
		var req ReadRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			return fail(errOut, fmt.Errorf("decode read request: %w", err))
		}
		if req.Spec == nil {
			return fail(errOut, errors.New("read: request has no spec"))
		}
		r, err := reader.Read(&req)
		if err != nil {
			return handlerError(out, errOut, head.Direction, err)
		}
		if r == nil {
			r = &ReadResponse{}
		}
		one := *r
		if one.Errors == nil {
			one.Errors = []Diagnostic{}
		}
		if one.Warnings == nil {
			one.Warnings = []Warning{}
		}
		resp = &one
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

// emitBatch は、direction "emit" の要求 req の、outputs の順、その中は transpilers の順に、h.Emit を呼び、答えを、同じ並びで
// 返す（docs/plugin-protocol.md「emit」の「SDK が、並びを回す」）。h.Emit に渡す View は、req.Spec と、その出力の view を
// 合わせたもの（docs/ir.md「まとめた形」）。h.Emit が誤りを返せば、そこでやめて、その誤りを返す。
func emitBatch(h Handler, req *EmitBatch) (*EmitBatchResponse, error) {
	if req.Spec == nil {
		return nil, errors.New("request has no spec")
	}
	resp := &EmitBatchResponse{Outputs: make([]OutputResult, 0, len(req.Outputs))}
	for i, o := range req.Outputs {
		if o.View == nil {
			return nil, fmt.Errorf("outputs[%d] has no view", i)
		}
		view := req.Spec.View(o.View)
		result := OutputResult{Results: make([]EmitResponse, 0, len(o.Transpilers))}
		for _, name := range o.Transpilers {
			r, err := h.Emit(&EmitRequest{Protocol: req.Protocol, Direction: req.Direction, Transpiler: name, View: view})
			if err != nil {
				return nil, err
			}
			if r == nil {
				r = &EmitResponse{}
			}
			one := *r
			if one.Files == nil {
				one.Files = []File{}
			}
			if one.Warnings == nil {
				one.Warnings = []Warning{}
			}
			result.Results = append(result.Results, one)
		}
		resp.Outputs = append(resp.Outputs, result)
	}
	return resp, nil
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
