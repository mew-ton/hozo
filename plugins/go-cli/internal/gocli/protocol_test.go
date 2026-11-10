package gocli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// TestProtocolRun は JSON の要求を plugin.Run に通し、応答の形（docs/plugin-protocol.md「emit」）を確かめる。
func TestProtocolRun(t *testing.T) {
	root := repoRoot(t)
	req, err := json.Marshal(plugin.EmitRequest{Protocol: ir.Version, Direction: "emit", View: hozocView()})
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := plugin.Run(handlerFor(root), bytes.NewReader(req), &out, &errOut)
	if code != 0 || errOut.Len() != 0 {
		t.Fatalf("code = %d, stderr = %q", code, errOut.String())
	}
	var resp struct {
		Files    []plugin.File       `json:"files"`
		Warnings []plugin.Warning    `json:"warnings"`
		Errors   []plugin.Diagnostic `json:"errors"`
	}
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out.String())
	}
	if len(resp.Files) != 1 || resp.Files[0].Path != "internal/gen/hozoc/cli/cli.go" {
		t.Fatalf("files = %+v", resp.Files)
	}
	if !strings.Contains(out.String(), `"warnings":[]`) || strings.Contains(out.String(), `"errors"`) {
		t.Errorf("warnings must be [] and errors omitted: %s", out.String()[:200])
	}
	golden, err := os.ReadFile(filepath.Join("testdata", "hozoc_cli.go.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Files[0].Content != string(golden) {
		t.Errorf("content differs from golden")
	}
}

func TestProtocolErrors(t *testing.T) {
	root := repoRoot(t)
	ns := hozocNamespace()
	ns.Decls = append(ns.Decls, opDecl("hozoc.dup", "", nil, nil, nil, cliCommand("check")))
	req, _ := json.Marshal(plugin.EmitRequest{Protocol: ir.Version, Direction: "emit", View: view(ns, "internal/gen/hozoc")})
	var out, errOut bytes.Buffer
	code := plugin.Run(handlerFor(root), bytes.NewReader(req), &out, &errOut)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, errOut.String())
	}
	want := `{"files":[],"warnings":[],"errors":[{"code":"CLI-5","at":"hozoc.dup","message":`
	if !strings.HasPrefix(out.String(), want) {
		t.Errorf("stdout = %s\nwant prefix %s", out.String(), want)
	}
}

func TestProtocolUnsupportedDirections(t *testing.T) {
	for _, dir := range []string{"check", "import"} {
		var out, errOut bytes.Buffer
		code := plugin.Run(Handler{}, strings.NewReader(`{"protocol":"`+ir.Version+`","direction":"`+dir+`"}`), &out, &errOut)
		if code != 2 || !strings.Contains(out.String(), `"code":"unsupported"`) {
			t.Errorf("%s: code = %d, stdout = %s", dir, code, out.String())
		}
	}
}

// TestPluginBinary はプラグインの main を go run で起動し、標準入出力で一往復する（docs/plugin-protocol.md「起動」）。
func TestPluginBinary(t *testing.T) {
	goBin := goTool(t)
	root := repoRoot(t)
	req, _ := json.Marshal(plugin.EmitRequest{Protocol: ir.Version, Direction: "emit", View: hozocView()})
	cmd := exec.Command(goBin, "run", "github.com/mew-ton/hozo/plugins/go-cli")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "HOZO_PROJECT_ROOT="+root, "HOZO_PLUGIN_DIR="+filepath.Join(root, "plugins", "go-cli"))
	cmd.Stdin = bytes.NewReader(req)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("go run: %v\n%s", err, errOut.String())
	}
	var resp plugin.EmitResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out.String())
	}
	golden, _ := os.ReadFile(filepath.Join("testdata", "hozoc_cli.go.golden"))
	if len(resp.Files) != 1 || resp.Files[0].Path != "internal/gen/hozoc/cli/cli.go" || resp.Files[0].Content != string(golden) {
		t.Errorf("response differs: %d files", len(resp.Files))
	}
}

// TestManifest はマニフェスト（docs/plugin-go-cli.md「マニフェスト」）を確かめる。外部の TOML は使えないので行で見る。
func TestManifest(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "hozo-plugin.toml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	checks := map[string]*regexp.Regexp{
		"name":     regexp.MustCompile(`(?m)^name\s*=\s*"go-cli"$`),
		"command":  regexp.MustCompile(`(?m)^command\s*=\s*\["go", "run", "github.com/mew-ton/hozo/plugins/go-cli"\]$`),
		"language": regexp.MustCompile(`(?m)^language\s*=\s*"go"$`),
		"protocol": regexp.MustCompile(`(?m)^protocol\s*=\s*"` + regexp.QuoteMeta(ir.Version) + `"$`),
		"emit":     regexp.MustCompile(`(?m)^\[emit\]$`),
		"entry":    regexp.MustCompile(`(?m)^entry\s*=\s*\["cli.command"\]$`),
		"stop":     regexp.MustCompile(`(?m)^stop\s*=\s*\["proc.stdio"\]$`),
	}
	for name, re := range checks {
		if !re.MatchString(s) {
			t.Errorf("manifest lacks %s:\n%s", name, s)
		}
	}
	if !strings.Contains(s, `vocabulary = ["cli.hozo"]`) {
		t.Error("manifest must provide the cli vocabulary (docs/plugin-go-cli.md「マニフェスト」)")
	}
	if _, err := os.Stat(filepath.Join("..", "..", "cli.hozo")); err != nil {
		t.Errorf("plugins/go-cli/cli.hozo is missing: %v", err)
	}
}
