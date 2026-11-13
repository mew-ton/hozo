package sema

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
)

// TestCorpusDesign はコーパス design（docs/testing.md「コーパス」）を標準の語彙と一緒に解析し、error が出ないことを確かめる。
// warning は許し、表示する（README.md の想定は error も warning もなし）。
func TestCorpusDesign(t *testing.T) {
	dir := filepath.Join("..", "..", "tests", "corpus", "design")
	cfg, cfgDiags := config.Load(filepath.Join(dir, "hozo.toml"))
	if n, _ := diag.Count(cfgDiags); n > 0 {
		t.Fatalf("hozo.toml にエラー: %+v", cfgDiags)
	}
	p := loadDir(t, dir)
	fs, files := parseProject(t, p, true)
	res := Analyze(fs, files, cfg)

	var buf bytes.Buffer
	_ = diag.WriteText(&buf, res.Diags, fs)
	if errors, _ := diag.Count(res.Diags); errors > 0 {
		t.Fatalf("コーパスに error がある:\n%s", buf.String())
	}
	t.Logf("診断:\n%s", buf.String())

	// Program は JSON に書けて、読み戻せる（docs/ir.md「JSON」）。
	data, err := json.Marshal(res.Program)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("JSON が空")
	}
	// 設計書の例の要点が IR に残っている。
	for _, fqn := range []string{"server.User", "server.PublicUser", "browser.OrderPage.Ready", "browser.openOrders", "browser.WarrantyAvailable", "db", "page.fallback"} {
		if res.Program.Lookup(fqn) == nil {
			t.Errorf("%s が Program にない", fqn)
		}
	}
	if len(res.Program.Binds) != 1 || res.Program.Binds[0].State != "browser.token" || res.Program.Binds[0].Target.Namespace != "server" {
		t.Errorf("bind が正規化されていない: %+v", res.Program.Binds)
	}
	if len(res.Refs) == 0 {
		t.Error("Refs が空")
	}
}
