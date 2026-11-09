package check

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

var update = flag.Bool("update", false, "update golden files (docs/testing.md「コーパス」)")

// TestViewsGolden は §9 の例に深い bind を足した小さなプログラムの View（browser から見たもの）がゴールデンと一致すること。
// 更新は go test ./internal/check -update。
func TestViewsGolden(t *testing.T) {
	lineCounter = 1000 // 位置を決定的にする
	p := logoutProgram()
	// 入れ子の位置: server.deep(in: Outer{ inner: Inner{ token: Token? }, note: string })
	server := p.Namespace("server")
	server.Decls = append(server.Decls,
		model("server.Inner", field("token", ir.Optional(ir.Ref("server.Token"))), field("n", ir.Int())),
		model("server.Outer", field("inner", ir.Ref("server.Inner")), field("note", ir.String())),
		leaf("server.deep", ir.String(), param("in", ir.Ref("server.Outer")), param("t", ir.Optional(ir.Ref("server.Token")))),
	)
	cfg := cfgFor("server", "browser")
	plugins := []string{"react-router"}
	nt := cfg.Namespaces["browser"]
	nt.Plugins = plugins
	cfg.Namespaces["browser"] = nt

	views := Views(p, cfg)
	if len(views) != 2 {
		t.Fatalf("views = %d, want 2 (server, browser)", len(views))
	}
	v := views["browser"]
	if v.Viewer != "browser" || v.Config.Language != "go" || v.Config.Out != "gen/browser" || len(v.Config.Plugins) != 1 {
		t.Errorf("config = %+v", v.Config)
	}
	// 合成した model は browser の decls の末尾にだけ足され、元の Program は変わらない
	// logout$in（token を除いた LogoutInput）、deep$in、deep$in$inner の三つ
	if n := len(v.Program.Namespace("browser").Decls); n != len(p.Namespace("browser").Decls)+3 {
		t.Errorf("browser decls = %d, want +3 synthesized", n)
	}
	if m := v.Program.Lookup("browser.logout$in").(*ir.Model); len(m.Fields) != 0 || m.SynthesizedFrom != "server.logout" {
		t.Errorf("logout$in = %+v", m)
	}
	if len(views["server"].Program.Namespace("browser").Decls) != len(p.Namespace("browser").Decls) {
		t.Error("server view has browser's synthesized models")
	}
	deep := v.Ops["server.deep"]
	if len(deep.Params) != 1 || deep.Params[0].Type.String() != "browser.deep$in" {
		t.Errorf("deep params = %+v", deep.Params)
	}
	if len(deep.Bound) != 2 {
		t.Errorf("deep bound = %+v", deep.Bound)
	}
	synth := v.Program.Lookup("browser.deep$in$inner").(*ir.Model)
	if len(synth.Fields) != 1 || synth.Fields[0].Name != "n" || synth.SynthesizedFrom != "server.deep" {
		t.Errorf("synthesized inner = %+v", synth)
	}

	got, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "view.golden.json")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("view differs from golden (run with -update to refresh)\n%s", firstDiff(got, want))
	}
}

func firstDiff(a, b []byte) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			lo := i - 80
			if lo < 0 {
				lo = 0
			}
			hi := i + 80
			if hi > len(a) {
				hi = len(a)
			}
			hj := i + 80
			if hj > len(b) {
				hj = len(b)
			}
			return "got:  …" + string(a[lo:hi]) + "…\nwant: …" + string(b[lo:hj]) + "…"
		}
	}
	return "lengths differ"
}
