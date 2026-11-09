package jsonrpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// TestRoundTrip は要求・通知・応答が Content-Length の枠を往復しても同じであること（docs/testing.md「単体テスト」）。
func TestRoundTrip(t *testing.T) {
	req, err := NewRequest(NumberID(1), "initialize", map[string]any{"rootUri": "file:///x"})
	if err != nil {
		t.Fatal(err)
	}
	note, _ := NewNotification("initialized", struct{}{})
	resp, _ := NewResponse(StringID("a"), []int{1, 2})
	nullResp, _ := NewResponse(NumberID(2), nil)
	errResp := NewErrorResponse(NumberID(3), NewError(CodeMethodNotFound, "unknown %s", "x"))

	var buf bytes.Buffer
	w := NewWriter(&buf)
	for _, m := range []*Message{req, note, resp, nullResp, errResp} {
		if err := w.WriteMessage(m); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.HasPrefix(buf.String(), "Content-Length: ") {
		t.Fatalf("header: %q", buf.String()[:30])
	}

	r := NewReader(&buf)
	got, err := r.ReadMessage()
	if err != nil || !got.IsRequest() || got.Method != "initialize" || got.ID.String() != "1" {
		t.Fatalf("request: %+v %v", got, err)
	}
	var params struct{ RootURI string }
	if err := got.UnmarshalParams(&params); err != nil || params.RootURI != "file:///x" {
		t.Fatalf("params: %+v %v", params, err)
	}
	got, err = r.ReadMessage()
	if err != nil || !got.IsNotification() || got.Method != "initialized" {
		t.Fatalf("notification: %+v %v", got, err)
	}
	got, err = r.ReadMessage()
	if err != nil || !got.IsResponse() || !got.ID.Equal(StringID("a")) || string(got.Result) != "[1,2]" {
		t.Fatalf("response: %+v %v", got, err)
	}
	got, err = r.ReadMessage()
	if err != nil || !got.IsResponse() || string(got.Result) != "null" {
		t.Fatalf("null response: %+v %v", got, err)
	}
	got, err = r.ReadMessage()
	if err != nil || got.Error == nil || got.Error.Code != CodeMethodNotFound || got.Error.Message != "unknown x" {
		t.Fatalf("error response: %+v %v", got, err)
	}
	if _, err := r.ReadMessage(); err != io.EOF {
		t.Fatalf("want EOF, got %v", err)
	}
}

// TestNullResultIsWritten は result が null の応答で "result" のキーが省かれないこと（shutdown の応答）。
func TestNullResultIsWritten(t *testing.T) {
	m, _ := NewResponse(NumberID(5), nil)
	b, err := Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	body := b[bytes.Index(b, []byte("\r\n\r\n"))+4:]
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["result"]) != "null" {
		t.Fatalf("result = %s", raw["result"])
	}
	if _, ok := raw["method"]; ok {
		t.Fatal("response must not carry method")
	}
}

// TestHeaders は Content-Type のヘッダを読み飛ばし、大文字小文字を区別しないこと。
func TestHeaders(t *testing.T) {
	body := `{"jsonrpc":"2.0","method":"x"}`
	in := "content-length: " + itoa(len(body)) + "\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n" + body
	m, err := NewReader(strings.NewReader(in)).ReadMessage()
	if err != nil || m.Method != "x" {
		t.Fatalf("%+v %v", m, err)
	}
}

// TestParseError は本体が JSON でなければ CodeParseError の *Error を返し、次のメッセージへ進めること。
func TestParseError(t *testing.T) {
	bad := "{not json"
	good := `{"jsonrpc":"2.0","method":"ok"}`
	in := "Content-Length: " + itoa(len(bad)) + "\r\n\r\n" + bad + "Content-Length: " + itoa(len(good)) + "\r\n\r\n" + good
	r := NewReader(strings.NewReader(in))
	_, err := r.ReadMessage()
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeParseError {
		t.Fatalf("want parse error, got %v", err)
	}
	m, err := r.ReadMessage()
	if err != nil || m.Method != "ok" {
		t.Fatalf("%+v %v", m, err)
	}
}

// TestInvalidHeader はヘッダがなければ ErrInvalidHeader。
func TestInvalidHeader(t *testing.T) {
	_, err := NewReader(strings.NewReader("\r\n")).ReadMessage()
	if !errors.Is(err, ErrInvalidHeader) {
		t.Fatalf("want ErrInvalidHeader, got %v", err)
	}
	_, err = NewReader(strings.NewReader("Content-Length: 10\r\n\r\nshort")).ReadMessage()
	if !errors.Is(err, ErrInvalidHeader) {
		t.Fatalf("want ErrInvalidHeader for short body, got %v", err)
	}
}

// TestID は数値と文字列の id を区別し、null は id なしとすること。
func TestID(t *testing.T) {
	var id ID
	if err := json.Unmarshal([]byte(`"7"`), &id); err != nil || !id.Equal(StringID("7")) || id.Equal(NumberID(7)) {
		t.Fatalf("string id: %v %v", id, err)
	}
	if err := json.Unmarshal([]byte(`null`), &id); err != nil || !id.IsZero() {
		t.Fatalf("null id: %v %v", id, err)
	}
	if err := json.Unmarshal([]byte(`true`), &id); err == nil {
		t.Fatal("bool id must fail")
	}
	m, err := Decode([]byte(`{"jsonrpc":"2.0","id":null,"result":1}`))
	if err != nil || !m.IsResponse() {
		t.Fatalf("response with null id: %+v %v", m, err)
	}
	if _, err := Decode([]byte(`{"jsonrpc":"2.0"}`)); err == nil {
		t.Fatal("message without method and id must fail")
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
