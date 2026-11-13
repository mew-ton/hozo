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

// module は「生成物 gen を main が import する」小さなモジュールを作る（hozoc の internal/gen/hozoc と cmd/hozoc の関係）。
func module(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go がない")
	}
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "go.mod"), "module example.com/self\n\ngo 1.24\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nimport \"example.com/self/gen\"\n\nfunc main() { println(gen.Name()) }\n")
	writeFile(t, filepath.Join(repo, "gen", "gen.go"), "package gen\n\nfunc Name() string { return \"old\" }\n")
	writeFile(t, filepath.Join(repo, "gen", "stale.go"), "package gen\n\nfunc Stale() {}\n")
	return repo
}

// workDir は生成先の一時ディレクトリ（リポジトリと同じ go.mod を持つ）。
func workDir(t *testing.T) string {
	t.Helper()
	work := t.TempDir()
	writeFile(t, filepath.Join(work, "go.mod"), "module example.com/self\n\ngo 1.24\n")
	return work
}

// docs/packages.md「自己生成の手順」の 3: コンパイルできない生成物は、置き場に置く前に見つかる。置き場は変わらない。
func TestVerifyRejectsBrokenOutput(t *testing.T) {
	repo := module(t)
	before, err := tree(filepath.Join(repo, "gen"))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]map[string]string{
		"構文の誤り":         {"gen.go": "package gen\n\nfunc Name() string { return \n"},
		"型の誤り":          {"gen.go": "package gen\n\nfunc Name() string { return 1 }\n"},
		"使う側が要る関数がない":   {"gen.go": "package gen\n\nfunc Other() string { return \"x\" }\n"},
		"新しいファイルが壊れている": {"gen.go": "package gen\n\nfunc Name() string { return \"new\" }\n", "sub/sub.go": "package sub\n\nvar x int = \"s\"\n"},
		"何も生成されなかった":    {},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			work := workDir(t)
			for f, content := range files {
				writeFile(t, filepath.Join(work, "gen", f), content)
			}
			var stderr bytes.Buffer
			if err := verify(repo, "gen", work, []string{"./..."}, t.TempDir(), &stderr); err == nil {
				t.Fatalf("verify が通った\n%s", stderr.String())
			}
			after, err := tree(filepath.Join(repo, "gen"))
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Errorf("置き場が変わった: %v", err)
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
	var stderr bytes.Buffer
	if err := verify(repo, "gen", work, []string{"./..."}, t.TempDir(), &stderr); err != nil {
		t.Fatalf("verify: %v\n%s", err, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, "gen", "sub")); !os.IsNotExist(err) {
		t.Fatal("verify が置き場に書いた")
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
	if err != nil || !strings.Contains(string(out), "new") {
		t.Errorf("置き換えた後に動かない: %v\n%s", err, out)
	}
	// 二度目は何も変えない（不動点）
	if changed, err := replace(filepath.Join(repo, "gen"), fresh); err != nil || len(changed) != 0 {
		t.Errorf("二度目: changed = %v err = %v", changed, err)
	}
}
