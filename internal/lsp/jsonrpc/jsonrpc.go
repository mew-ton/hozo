// Package jsonrpc は Content-Length ヘッダ付きの JSON-RPC 2.0 の読み書き（docs/lsp.md「通信」）。
// 外部のモジュールに頼らず自前で書く（docs/packages.md「依存の向き」）。
//
// 一つのメッセージは、`Content-Length: <バイト数>\r\n`（と任意の `Content-Type`）のヘッダ、空行 `\r\n`、
// その長さの JSON の本体からなる。[Reader] がそれを [Message] に、[Writer] が [Message] をそれに直す。
// 要求（ID と Method）、通知（Method だけ）、応答（ID と Result か Error）は同じ [Message] で表し、
// [Message.IsRequest] などで見分ける。`$/cancelRequest` の扱いはサーバー側（internal/lsp）で、ここは運ぶだけ。
package jsonrpc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// JSON-RPC 2.0 と LSP 3.17 の誤りのコード。
const (
	// CodeParseError は JSON として読めない。
	CodeParseError = -32700
	// CodeInvalidRequest は JSON-RPC の形でない。
	CodeInvalidRequest = -32600
	// CodeMethodNotFound は知らないメソッドの要求。
	CodeMethodNotFound = -32601
	// CodeInvalidParams は params が読めない。
	CodeInvalidParams = -32602
	// CodeInternalError はサーバーの内部の誤り。
	CodeInternalError = -32603
	// CodeServerNotInitialized は initialize の前に届いた要求（LSP）。
	CodeServerNotInitialized = -32002
	// CodeRequestCancelled は取り消された要求（LSP）。
	CodeRequestCancelled = -32800
	// CodeContentModified は内容が変わって答えが無効になった要求（LSP）。
	CodeContentModified = -32801
)

// Error は応答の error。
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error は error の実装。
func (e *Error) Error() string { return fmt.Sprintf("jsonrpc: %d %s", e.Code, e.Message) }

// NewError は Error を作る。
func NewError(code int, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// ID は要求の id。数値でも文字列でもよいので、JSON のまま持つ。
type ID struct {
	raw json.RawMessage
}

// NumberID は数値の id を作る。
func NumberID(n int64) ID { return ID{raw: json.RawMessage(strconv.FormatInt(n, 10))} }

// StringID は文字列の id を作る。
func StringID(s string) ID {
	b, _ := json.Marshal(s)
	return ID{raw: b}
}

// IsZero は id がないこと（通知）。
func (id ID) IsZero() bool { return len(id.raw) == 0 }

// String は id の JSON の表記。
func (id ID) String() string { return string(id.raw) }

// Equal は同じ id か。数値と文字列は区別する。
func (id ID) Equal(o ID) bool { return bytes.Equal(id.raw, o.raw) }

// MarshalJSON は json.Marshaler の実装。
func (id ID) MarshalJSON() ([]byte, error) {
	if id.IsZero() {
		return []byte("null"), nil
	}
	return id.raw, nil
}

// UnmarshalJSON は json.Unmarshaler の実装。数値か文字列だけを受け付ける。null は id なしとして扱う。
func (id *ID) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		id.raw = nil
		return nil
	}
	if len(data) == 0 || !(data[0] == '"' || data[0] == '-' || (data[0] >= '0' && data[0] <= '9')) {
		return fmt.Errorf("jsonrpc: id must be a number or a string: %s", data)
	}
	id.raw = append(json.RawMessage(nil), data...)
	return nil
}

// Message は要求・通知・応答のどれか。
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      ID              `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`

	// hasID は読んだメッセージに id のキーがあったか（応答の `"result": null` と区別するため）。
	hasID bool
}

// IsRequest は id と method を持つメッセージ。
func (m *Message) IsRequest() bool { return m.Method != "" && !m.ID.IsZero() }

// IsNotification は method を持ち id を持たないメッセージ。
func (m *Message) IsNotification() bool { return m.Method != "" && m.ID.IsZero() }

// IsResponse は method を持たないメッセージ（result か error）。
func (m *Message) IsResponse() bool { return m.Method == "" }

// UnmarshalParams は params を v に読む。params がなければ何もしない。
func (m *Message) UnmarshalParams(v any) error {
	if len(m.Params) == 0 || bytes.Equal(bytes.TrimSpace(m.Params), []byte("null")) {
		return nil
	}
	return json.Unmarshal(m.Params, v)
}

// NewRequest は要求を作る。params は JSON にする。
func NewRequest(id ID, method string, params any) (*Message, error) {
	raw, err := marshalParams(params)
	if err != nil {
		return nil, err
	}
	return &Message{JSONRPC: "2.0", ID: id, Method: method, Params: raw}, nil
}

// NewNotification は通知を作る。
func NewNotification(method string, params any) (*Message, error) {
	raw, err := marshalParams(params)
	if err != nil {
		return nil, err
	}
	return &Message{JSONRPC: "2.0", Method: method, Params: raw}, nil
}

// NewResponse は成功の応答を作る。result が nil なら `"result": null`。
func NewResponse(id ID, result any) (*Message, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &Message{JSONRPC: "2.0", ID: id, Result: raw}, nil
}

// NewErrorResponse は誤りの応答を作る。
func NewErrorResponse(id ID, e *Error) *Message {
	return &Message{JSONRPC: "2.0", ID: id, Error: e}
}

func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return nil, nil
	}
	return json.Marshal(params)
}

// marshal は応答の result が null でも省かれないように、Message を手で組み立てる。
func (m *Message) marshal() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`{"jsonrpc":"2.0"`)
	if !m.ID.IsZero() {
		buf.WriteString(`,"id":`)
		buf.Write(m.ID.raw)
	}
	if m.Method != "" {
		buf.WriteString(`,"method":`)
		b, _ := json.Marshal(m.Method)
		buf.Write(b)
		if len(m.Params) > 0 {
			buf.WriteString(`,"params":`)
			buf.Write(m.Params)
		}
	} else if m.Error != nil {
		buf.WriteString(`,"error":`)
		b, err := json.Marshal(m.Error)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	} else {
		buf.WriteString(`,"result":`)
		if len(m.Result) == 0 {
			buf.WriteString("null")
		} else {
			buf.Write(m.Result)
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Reader は Content-Length で区切られたメッセージを読む。
type Reader struct {
	r *bufio.Reader
}

// NewReader は r から読む Reader を作る。
func NewReader(r io.Reader) *Reader {
	return &Reader{r: bufio.NewReaderSize(r, 64*1024)}
}

// ErrInvalidHeader はヘッダが読めないこと。
var ErrInvalidHeader = errors.New("jsonrpc: invalid header")

// ReadMessage は次のメッセージを読む。入力の終わりなら io.EOF。ヘッダが壊れていれば ErrInvalidHeader
// （これ以上読み進められないので、呼び手は接続を閉じる）。本体が JSON として読めなければ *Error
// （CodeParseError）を返し、次のメッセージへ進める。
func (rd *Reader) ReadMessage() (*Message, error) {
	length := -1
	for {
		line, err := rd.r.ReadString('\n')
		if err != nil {
			if err == io.EOF && line == "" && length < 0 {
				return nil, io.EOF
			}
			return nil, fmt.Errorf("%w: %v", ErrInvalidHeader, err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if length < 0 {
				return nil, fmt.Errorf("%w: missing Content-Length", ErrInvalidHeader)
			}
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrInvalidHeader, line)
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || n < 0 {
				return nil, fmt.Errorf("%w: Content-Length %q", ErrInvalidHeader, value)
			}
			length = n
		}
		// Content-Type などほかのヘッダは読み飛ばす
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(rd.r, body); err != nil {
		return nil, fmt.Errorf("%w: body: %v", ErrInvalidHeader, err)
	}
	return Decode(body)
}

// Decode は一つのメッセージの JSON を読む。
func Decode(body []byte) (*Message, error) {
	var probe struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
		Result  json.RawMessage `json:"result"`
		Error   *Error          `json:"error"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil, NewError(CodeParseError, "invalid JSON: %v", err)
	}
	m := &Message{JSONRPC: probe.JSONRPC, Method: probe.Method, Params: probe.Params, Result: probe.Result, Error: probe.Error}
	if len(probe.ID) > 0 {
		m.hasID = true
		if err := m.ID.UnmarshalJSON(probe.ID); err != nil {
			return nil, NewError(CodeInvalidRequest, "%v", err)
		}
	}
	if m.Method == "" && !m.hasID {
		return nil, NewError(CodeInvalidRequest, "message has neither method nor id")
	}
	return m, nil
}

// Writer はメッセージを Content-Length 付きで書く。複数の goroutine から使える。
type Writer struct {
	mu sync.Mutex
	w  io.Writer
}

// NewWriter は w に書く Writer を作る。
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// WriteMessage は一つのメッセージを書く。
func (wr *Writer) WriteMessage(m *Message) error {
	body, err := m.marshal()
	if err != nil {
		return err
	}
	wr.mu.Lock()
	defer wr.mu.Unlock()
	if _, err := fmt.Fprintf(wr.w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = wr.w.Write(body)
	return err
}

// Encode はテスト向けに、メッセージをヘッダ付きのバイト列にする。
func Encode(m *Message) ([]byte, error) {
	var buf bytes.Buffer
	if err := NewWriter(&buf).WriteMessage(m); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
