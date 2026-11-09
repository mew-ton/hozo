package golang

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestImportsRender(t *testing.T) {
	var im Imports
	if got := im.Render(); got != "" {
		t.Errorf("empty Render() = %q", got)
	}
	if got := im.Add("time", "time"); got != "time" {
		t.Errorf("Add(time) = %q", got)
	}
	if got := im.Render(); got != "import \"time\"\n" {
		t.Errorf("single Render() = %q", got)
	}
	im.Add("github.com/mew-ton/hozo/runtime", "runtime")
	im.Add("context", "context")
	im.Add("example.com/app/gen/server", "server")
	want := "import (\n\t\"context\"\n\t\"time\"\n\n\t\"example.com/app/gen/server\"\n\t\"github.com/mew-ton/hozo/runtime\"\n)\n"
	if got := im.Render(); got != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
	if got := im.Paths(); len(got) != 4 || got[0] != "context" || got[3] != "time" {
		t.Errorf("Paths() = %v", got)
	}
}

func TestImportsAlias(t *testing.T) {
	var im Imports
	// パッケージ名がパスの最後の要素と違う。
	if got := im.Add("example.com/app/gen/my-api", "myapi"); got != "myapi" {
		t.Errorf("Add = %q", got)
	}
	// 同じ名前のパッケージが二つ。
	if got := im.Add("example.com/app/gen/a/server", "server"); got != "server" {
		t.Errorf("Add first server = %q", got)
	}
	if got := im.Add("example.com/app/gen/b/server", "server"); got != "server2" {
		t.Errorf("Add second server = %q", got)
	}
	if got := im.Add("example.com/app/gen/a/server", "server"); got != "server" {
		t.Errorf("Add same path again = %q", got)
	}
	if got := im.Name("example.com/app/gen/b/server"); got != "server2" {
		t.Errorf("Name = %q", got)
	}
	if got := im.Name("missing"); got != "" {
		t.Errorf("Name(missing) = %q", got)
	}
	want := "import (\n\t\"example.com/app/gen/a/server\"\n\tserver2 \"example.com/app/gen/b/server\"\n\tmyapi \"example.com/app/gen/my-api\"\n)\n"
	if got := im.Render(); got != want {
		t.Errorf("Render() =\n%s\nwant\n%s", got, want)
	}
}

func TestImportsDeterministic(t *testing.T) {
	var a, b Imports
	for _, p := range []string{"time", "example.com/x/b", "example.com/x/a", "context"} {
		a.Add(p, filepath.Base(p))
	}
	for _, p := range []string{"context", "example.com/x/a", "example.com/x/b", "time"} {
		b.Add(p, filepath.Base(p))
	}
	if a.Render() != b.Render() {
		t.Errorf("Render depends on insertion order:\n%s\n%s", a.Render(), b.Render())
	}
}

func TestImportPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("// comment\nmodule example.com/app // trailing\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct{ out, want string }{
		{"gen", "example.com/app/gen"},
		{"gen/server/admin", "example.com/app/gen/server/admin"},
		{".", "example.com/app"},
		{"", "example.com/app"},
	}
	for _, tt := range tests {
		got, err := ImportPath(root, tt.out)
		if err != nil || got != tt.want {
			t.Errorf("ImportPath(root, %q) = %q, %v; want %q", tt.out, got, err, tt.want)
		}
	}
	// 下のディレクトリに別の go.mod があれば近いほうを使う。
	sub := filepath.Join(root, "gen", "cli")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "go.mod"), []byte("module \"example.com/cli\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ImportPath(root, "gen/cli/internal"); err != nil || got != "example.com/cli/internal" {
		t.Errorf("nested = %q, %v", got, err)
	}
	// 存在しないディレクトリでも上へ辿る。
	if got, err := ImportPath(root, "not/yet/created"); err != nil || got != "example.com/app/not/yet/created" {
		t.Errorf("missing dir = %q, %v", got, err)
	}
}

func TestImportPathNoGoMod(t *testing.T) {
	root := t.TempDir() // t.TempDir は /tmp の下で、その上に go.mod はないものとする
	_, err := ImportPath(root, "gen")
	if !errors.Is(err, ErrNoGoMod) {
		t.Errorf("err = %v, want ErrNoGoMod", err)
	}
}

func TestImportPathNoModuleLine(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("go 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportPath(root, "gen"); err == nil || errors.Is(err, ErrNoGoMod) {
		t.Errorf("err = %v, want a 'no module line' error", err)
	}
}
