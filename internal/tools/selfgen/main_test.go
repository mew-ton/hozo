package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// module は「生成物 gen と gen2 を main が import する」小さなモジュールを作る（置き場が、二つのパッケージを持つ場合。hozoc では
// internal/gen/hozo/compiler と、その下のパッケージと、cmd/hozoc の関係）。
func module(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go がない")
	}
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/self\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nimport (\n\t\"example.com/self/gen\"\n\t\"example.com/self/gen2\"\n)\n\nfunc main() { println(gen.Name(), gen2.Kind) }\n")
	writeFile(t, filepath.Join(repo, "gen", "gen.go"), "package gen\n\nfunc Name() string { return \"old\" }\n")
	writeFile(t, filepath.Join(repo, "gen", "stale.go"), "package gen\n\nfunc Stale() {}\n")
	writeFile(t, filepath.Join(repo, "gen2", "gen2.go"), "package gen2\n\nconst Kind = \"old\"\n")
	return repo
}

// testPlaces は、テストのモジュールの二つの置き場。
var testPlaces = []string{"gen", "gen2"}

// goodGen2 は、二つ目の置き場の、コンパイルできる生成物。
const goodGen2 = "package gen2\n\nconst Kind = \"new\"\n"

// workDir は生成先の一時ディレクトリ（リポジトリと同じ go.mod を持つ）。
func workDir(t *testing.T) string {
	t.Helper()
	work := t.TempDir()
	writeFile(t, filepath.Join(work, "go.mod"), "module example.com/self\n\ngo 1.24\n")
	return work
}

// trees は、二つの置き場の中身（置き場 → 相対パス → 内容）。
func trees(t *testing.T, root string) map[string]map[string][]byte {
	t.Helper()
	out := map[string]map[string][]byte{}
	for _, rel := range testPlaces {
		files, err := tree(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		out[rel] = files
	}
	return out
}

// docs/packages.md「自己生成の手順」の 3: コンパイルできない生成物は、置き場に置く前に見つかる。置き場は、どれも変わらない
// （壊れているのが一方だけでも、もう一方も置き換えない）。
func TestVerifyRejectsBrokenOutput(t *testing.T) {
	repo := module(t)
	before := trees(t, repo)
	good := "package gen\n\nfunc Name() string { return \"new\" }\n"
	cases := map[string]map[string]string{
		"構文の誤り":                {"gen/gen.go": "package gen\n\nfunc Name() string { return \n", "gen2/gen2.go": goodGen2},
		"型の誤り":                 {"gen/gen.go": "package gen\n\nfunc Name() string { return 1 }\n", "gen2/gen2.go": goodGen2},
		"使う側が要る関数がない":          {"gen/gen.go": "package gen\n\nfunc Other() string { return \"x\" }\n", "gen2/gen2.go": goodGen2},
		"新しいファイルが壊れている":        {"gen/gen.go": good, "gen/sub/sub.go": "package sub\n\nvar x int = \"s\"\n", "gen2/gen2.go": goodGen2},
		"二つ目の置き場が壊れている":        {"gen/gen.go": good, "gen2/gen2.go": "package gen2\n\nconst Kind = \n"},
		"二つ目の置き場に、使う側が要る名前がない": {"gen/gen.go": good, "gen2/gen2.go": "package gen2\n\nconst Other = 1\n"},
		"何も生成されなかった":           {},
		"一方の置き場に、何も生成されなかった":   {"gen/gen.go": good},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			work := workDir(t)
			for f, content := range files {
				writeFile(t, filepath.Join(work, filepath.FromSlash(f)), content)
			}
			var stderr bytes.Buffer
			if err := verify(repo, testPlaces, work, []string{"./..."}, t.TempDir(), &stderr); err == nil {
				t.Fatalf("verify が通った\n%s", stderr.String())
			}
			if after := trees(t, repo); !reflect.DeepEqual(before, after) {
				t.Errorf("置き場が変わった")
			}
		})
	}
}

// 同 3 と 4: コンパイルできる生成物は通り、置き換えると、なくなったファイルは消え、同じ内容のファイルには触れない。
// なくなるファイルにしか定義がないものを、使う側が要らないことも、置き換える前に確かめている。
func TestVerifyAndReplace(t *testing.T) {
	repo := module(t)
	work := workDir(t)
	fresh := filepath.Join(work, "gen")
	writeFile(t, filepath.Join(fresh, "gen.go"), "package gen\n\nimport \"example.com/self/gen/sub\"\n\nfunc Name() string { return sub.Name }\n")
	writeFile(t, filepath.Join(fresh, "sub", "sub.go"), "package sub\n\nconst Name = \"new\"\n")
	writeFile(t, filepath.Join(fresh, ".outputs.json"), "[]\n")
	writeFile(t, filepath.Join(work, "gen2", "gen2.go"), goodGen2)
	var stderr bytes.Buffer
	if err := verify(repo, testPlaces, work, []string{"./..."}, t.TempDir(), &stderr); err != nil {
		t.Fatalf("verify: %v\n%s", err, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, "gen", "sub")); !os.IsNotExist(err) {
		t.Fatal("verify が置き場に書いた")
	}
	if changed, err := replace(filepath.Join(repo, "gen2"), filepath.Join(work, "gen2")); err != nil || !reflect.DeepEqual(changed, []string{"written: gen2.go"}) {
		t.Fatalf("二つ目の置き場: changed = %v err = %v", changed, err)
	}

	changed, err := replace(filepath.Join(repo, "gen"), fresh)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"removed: stale.go", "written: .outputs.json", "written: gen.go", "written: sub/sub.go"}; !reflect.DeepEqual(changed, want) {
		t.Errorf("changed = %v, want %v", changed, want)
	}
	got, _ := tree(filepath.Join(repo, "gen"))
	want, _ := tree(fresh)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("置き場が生成したものと違う")
	}
	out, err := exec.Command("go", "run", "-C", repo, ".").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "new new") {
		t.Errorf("置き換えた後に動かない: %v\n%s", err, out)
	}
	// 二度目は何も変えない（不動点）
	if changed, err := replace(filepath.Join(repo, "gen"), fresh); err != nil || len(changed) != 0 {
		t.Errorf("二度目: changed = %v err = %v", changed, err)
	}
}

// TestSpecFiles は、写す仕様のファイルが、spec/ の下の、すべての .hozo で、ほかのディレクトリの .hozo を含まないこと
// （docs/packages.md「自己生成の手順」の 2）。
func TestSpecFiles(t *testing.T) {
	repo := t.TempDir()
	for _, name := range []string{"spec/cli.hozo", "spec/ir.hozo", "spec/decisions/adr.hozo", "spec/decisions/d000.hozo", "spec/diagnostics/rule.hozo", "tests/x.hozo", "vocab/v.hozo", "spec/decisions/note.md"} {
		writeFile(t, filepath.Join(repo, name), "")
	}
	got, err := specFiles(repo)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"spec/cli.hozo", "spec/decisions/adr.hozo", "spec/decisions/d000.hozo", "spec/diagnostics/rule.hozo", "spec/ir.hozo"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("specFiles = %v, want %v", got, want)
	}
}
