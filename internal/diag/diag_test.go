package diag

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/mew-ton/hozo/internal/source"
)

func span(f source.FileID, start, end int) *source.Span {
	return &source.Span{File: f, Start: source.Offset(start), End: source.Offset(end)}
}

func TestSeverityString(t *testing.T) {
	if Error.String() != "error" || Warning.String() != "warning" {
		t.Errorf("String: %s / %s", Error, Warning)
	}
}

func TestSort(t *testing.T) {
	fs := source.NewFileSet()
	b := fs.Add("b.hozo", []byte("0123456789"))
	a := fs.Add("a.hozo", []byte("0123456789"))
	v := fs.Add("hozo:vocab/valid.hozo", []byte("0123456789"))
	diags := []Diagnostic{
		{Code: "Z", Message: "vocab", Span: span(v, 0, 1)},
		{Code: "B", Message: "b at 5", Span: span(b, 5, 6)},
		{Code: "B", Message: "a at 5 B", Span: span(a, 5, 6)},
		{Code: "A", Message: "a at 5 A", Span: span(a, 5, 6)},
		{Code: "A", Message: "a at 5 A second", Span: span(a, 5, 6)},
		{Code: "C", Message: "a at 2", Span: span(a, 2, 3)},
		{Code: "CFG-12", Message: "a no span", File: "a.hozo"},
		{Code: "CFG-12", Message: "toml", File: "hozo.toml"},
		{Code: "X", Message: "no file"},
	}
	Sort(diags, fs, nil)
	want := []string{"no file", "a no span", "a at 2", "a at 5 A", "a at 5 A second", "a at 5 B", "b at 5", "toml", "vocab"}
	for i, w := range want {
		if diags[i].Message != w {
			t.Errorf("[%d] = %q, want %q", i, diags[i].Message, w)
		}
	}
}

func TestSortFileSetOrder(t *testing.T) {
	fs := source.NewFileSet()
	b := fs.Add("b.hozo", []byte("0123456789"))
	a := fs.Add("a.hozo", []byte("0123456789"))
	diags := []Diagnostic{
		{Code: "A", Message: "a", Span: span(a, 0, 1)},
		{Code: "A", Message: "b", Span: span(b, 0, 1)},
		{Code: "A", Message: "unknown", File: "z.toml"},
		{Code: "A", Message: "unknown2", File: "y.toml"},
	}
	Sort(diags, fs, FileSetOrder(fs))
	want := []string{"b", "a", "unknown2", "unknown"}
	for i, w := range want {
		if diags[i].Message != w {
			t.Errorf("[%d] = %q, want %q", i, diags[i].Message, w)
		}
	}
}

func TestPathOrder(t *testing.T) {
	tests := []struct {
		a, b string
		neg  bool
	}{
		{"a.hozo", "b.hozo", true},
		{"z.hozo", "hozo:vocab/a.hozo", true},
		{"hozo:vocab/b.hozo", "hozo:vocab/a.hozo", false},
	}
	for _, tt := range tests {
		got := PathOrder(tt.a, tt.b)
		if (got < 0) != tt.neg {
			t.Errorf("PathOrder(%q, %q) = %d", tt.a, tt.b, got)
		}
	}
}

func TestWriteText(t *testing.T) {
	fs := source.NewFileSet()
	f := fs.Add("examples/orders.hozo", []byte("line1\n  x\n\n\n\n\n\n\n\nabc\n"))
	diags := []Diagnostic{
		{Severity: Error, Code: "THROWS-1", Message: "openOrders は http.NetworkError を投げうるが throws に書かれていない", Span: span(f, 8, 9),
			Related: []Related{{Span: *span(f, 17, 20), Message: "出どころ: fetchMe の呼び出し"}}},
		{Severity: Error, Code: "CFG-12", Message: "2 番目の [[emit]]（namespace = \"server\"）に out がない", File: "hozo.toml"},
		{Severity: Warning, Code: "NAME-1", Message: "warn"},
	}
	var buf bytes.Buffer
	if err := WriteText(&buf, diags, fs); err != nil {
		t.Fatal(err)
	}
	want := "examples/orders.hozo:2:3: error[THROWS-1]: openOrders は http.NetworkError を投げうるが throws に書かれていない\n" +
		"  examples/orders.hozo:10:1: note: 出どころ: fetchMe の呼び出し\n" +
		"hozo.toml: error[CFG-12]: 2 番目の [[emit]]（namespace = \"server\"）に out がない\n" +
		"hozoc: warning[NAME-1]: warn\n" +
		"2 errors, 1 warning\n"
	if buf.String() != want {
		t.Errorf("WriteText =\n%s\nwant\n%s", buf.String(), want)
	}
}

func TestWriteTextEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteText(&buf, nil, source.NewFileSet()); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "0 errors, 0 warnings\n" {
		t.Errorf("WriteText(empty) = %q", buf.String())
	}
}

func TestWriteJSON(t *testing.T) {
	fs := source.NewFileSet()
	f := fs.Add("examples/orders.hozo", []byte("line1\n  xyz\n"))
	diags := []Diagnostic{
		{Severity: Error, Code: "THROWS-1", Message: "m", Span: span(f, 8, 11),
			Related: []Related{{Span: *span(f, 0, 5), Message: "r"}},
			Data:    map[string]any{"missing": []string{"http.NetworkError"}}},
		{Severity: Error, Code: "CFG-12", Message: "c", File: "hozo.toml"},
		{Severity: Warning, Code: "W", Message: "w"},
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, diags, fs); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, buf.String())
	}
	if got["errors"] != float64(2) || got["warnings"] != float64(1) {
		t.Errorf("counts = %v / %v", got["errors"], got["warnings"])
	}
	list := got["diagnostics"].([]any)
	if len(list) != 3 {
		t.Fatalf("len = %d", len(list))
	}
	d0 := list[0].(map[string]any)
	if d0["severity"] != "error" || d0["code"] != "THROWS-1" || d0["file"] != "examples/orders.hozo" {
		t.Errorf("d0 = %v", d0)
	}
	start := d0["start"].(map[string]any)
	end := d0["end"].(map[string]any)
	if start["line"] != float64(2) || start["col"] != float64(3) || end["line"] != float64(2) || end["col"] != float64(6) {
		t.Errorf("d0 start/end = %v / %v", start, end)
	}
	rel := d0["related"].([]any)[0].(map[string]any)
	if rel["file"] != "examples/orders.hozo" || rel["message"] != "r" {
		t.Errorf("related = %v", rel)
	}
	if d0["data"].(map[string]any)["missing"].([]any)[0] != "http.NetworkError" {
		t.Errorf("data = %v", d0["data"])
	}
	d1 := list[1].(map[string]any)
	if d1["file"] != "hozo.toml" {
		t.Errorf("d1 file = %v", d1["file"])
	}
	for _, k := range []string{"start", "end", "related", "data"} {
		if _, ok := d1[k]; ok {
			t.Errorf("d1 has %s", k)
		}
	}
	d2 := list[2].(map[string]any)
	if _, ok := d2["file"]; ok {
		t.Errorf("d2 has file")
	}
	if d2["severity"] != "warning" {
		t.Errorf("d2 severity = %v", d2["severity"])
	}
}
