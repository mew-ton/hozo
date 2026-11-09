package ir

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "update golden files (docs/testing.md「コーパス」)")

// TestGolden は手で組んだ Program と View が、testdata のゴールデンとバイト単位で一致することを確かめる。
// 更新は go test ./ir -update。
func TestGolden(t *testing.T) {
	cases := []struct {
		file string
		v    any
	}{
		{"program.golden.json", sampleProgram()},
		{"view.golden.json", sampleView()},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			got, err := json.MarshalIndent(c.v, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')
			path := filepath.Join("testdata", c.file)
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
				t.Errorf("%s differs from golden (run with -update to refresh)\n%s", c.file, firstDiff(got, want))
			}
			// ゴールデンを読み戻しても同じバイト列になる。
			target := map[string]any{"program.golden.json": &Program{}, "view.golden.json": &View{}}[c.file]
			if err := json.Unmarshal(want, target); err != nil {
				t.Fatal(err)
			}
			again, err := json.MarshalIndent(target, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(append(again, '\n'), want) {
				t.Errorf("%s: re-marshal of golden differs\n%s", c.file, firstDiff(again, want))
			}
		})
	}
}

func firstDiff(a, b []byte) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			lo := i - 40
			if lo < 0 {
				lo = 0
			}
			return "got:  ..." + string(a[lo:min(i+40, len(a))]) + "\nwant: ..." + string(b[lo:min(i+40, len(b))])
		}
	}
	return "lengths differ"
}
