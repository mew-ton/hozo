package tests

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/project"
)

// pluginViewsFile は、プラグインの単体テストの入力（View の列）のファイルの名前。
const pluginViewsFile = "views.json"

// TestPluginViewFixtures は、プラグインの単体テストの入力が、その元の仕様から作った View と同じであることを確かめる
// （docs/testing.md「単体テスト」: プラグインの単体テストは、小さな `.hozo` と出力のゴールデン）。
//
// プラグインのパッケージは internal/ を import できない（docs/packages.md「依存の向き」）ので、`.hozo` を自分で解析できない。
// そこで、`plugins/` の下の `testdata/<名前>/`（hozo.toml を持つディレクトリ）ごとに、ここで internal/project に解析させ、
// 単位ごと・名前空間ごとの View（docs/ir.md「その名前空間から見た IR」。hozoc がトランスパイラに渡すものと同じ）を
// `testdata/<名前>/views.json` に置く。プラグインのテストは、それを読んで入力にする。仕様か hozoc を変えて View が変われば、
// このテストが落ちる。更新は `go test ./tests -run TestPluginViewFixtures -update`（その後で、プラグインのゴールデンを更新する）。
//
// views.json は、View の JSON の配列。並びは、単位の順、その中は名前空間の辞書順。一つの View を一行に書く。
// 仕様は hozoc の検査を通ること（error があれば、このテストが落ちる。プラグインが返す診断の条件は、検査を通る仕様で書く）。
func TestPluginViewFixtures(t *testing.T) {
	var dirs []string
	err := filepath.WalkDir(repoPluginsDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "hozo.toml" && strings.Contains(filepath.ToSlash(p), "/testdata/") {
			dirs = append(dirs, filepath.Dir(p))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("plugins/ の下に、単体テストの入力（testdata/<名前>/hozo.toml）が一つもない")
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		rel, err := filepath.Rel(repoPluginsDir, dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(filepath.ToSlash(rel), func(t *testing.T) {
			p := project.LoadWith(project.Options{Root: dir, PluginPath: repoPlugins(t)})
			if n, _ := diag.Count(p.Diags); n > 0 {
				var b strings.Builder
				for _, d := range p.Diags {
					if d.Severity == diag.Error {
						b.WriteString("\n  " + d.Code + ": " + d.Message)
					}
				}
				t.Fatalf("仕様が検査を通らない:%s", b.String())
			}
			var buf bytes.Buffer
			buf.WriteString("[")
			n := 0
			for _, views := range p.Views() {
				names := make([]string, 0, len(views))
				for ns := range views {
					names = append(names, ns)
				}
				sort.Strings(names)
				for _, ns := range names {
					data, err := json.Marshal(views[ns])
					if err != nil {
						t.Fatal(err)
					}
					if n > 0 {
						buf.WriteString(",")
					}
					buf.WriteString("\n")
					buf.Write(data)
					n++
				}
			}
			buf.WriteString("\n]\n")
			if n == 0 {
				t.Fatal("View が一つもない（hozo.toml に単位がない）")
			}
			compareGolden(t, filepath.Join(dir, pluginViewsFile), buf.Bytes())
		})
	}
}
