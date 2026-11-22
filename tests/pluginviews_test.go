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
	"github.com/mew-ton/hozo/ir"
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
// サンプルの中のプラグイン（tests/samples/<名前>/ の下。tests/samples/README.md「置き方」）の `testdata/<名前>/` も同じに扱う。
// その入力は、自分のプラグイン（語彙とトランスパイラ）を使うので、リポジトリの plugins/ に加えて、そのプラグインを置いた
// ディレクトリも探す（samplePluginPath）。
//
// views.json は、View の JSON の配列。並びは、単位の順、その中は名前空間の辞書順。一つの View を一行に書く。
// 仕様は hozoc の検査を通ること（error があれば、このテストが落ちる。プラグインが返す診断の条件は、検査を通る仕様で書く）。
func TestPluginViewFixtures(t *testing.T) {
	var dirs []string
	for _, root := range []string{repoPluginsDir, samplesDir} {
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
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
	}
	if len(dirs) == 0 {
		t.Fatal("plugins/ の下に、単体テストの入力（testdata/<名前>/hozo.toml）が一つもない")
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		rel, err := filepath.Rel(repoPluginsDir, dir)
		if err != nil {
			rel = dir // サンプルの中のプラグインの入力は、tests/ からの相対パスのまま
		}
		t.Run(filepath.ToSlash(rel), func(t *testing.T) {
			// プラグインは、起動しない: ここで要るのは View だけで、入力には、プラグインが診断を返すための仕様もある
			// （その診断が、hozoc check で出ることは、TestPluginDiagnosticsAtCheck）
			p := project.LoadWith(project.Options{Root: dir, PluginPath: repoPlugins(t) + samplePluginPath(t, dir), NoPlugins: true})
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
					splitRoundTrip(t, views[ns], data)
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

// samplePluginPath は、単体テストの入力 dir が、サンプルの中のプラグインのものなら、そのプラグインを置いたディレクトリ
// （サンプルの plugin_dirs のディレクトリ）を、HOZO_PLUGIN_PATH の続きの形（パス区切りと絶対パス）で返す。そうでなければ ""。
// プラグインのディレクトリは、dir から親へ向かって最初に見つかる、マニフェストを持つディレクトリ。
func samplePluginPath(t *testing.T, dir string) string {
	t.Helper()
	if !strings.HasPrefix(filepath.ToSlash(dir), samplesDir+"/") {
		return ""
	}
	for d := filepath.Dir(dir); d != samplesDir && d != "."; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "hozo-plugin.toml")); err == nil {
			abs, err := filepath.Abs(filepath.Dir(d))
			if err != nil {
				t.Fatal(err)
			}
			return string(filepath.ListSeparator) + abs
		}
	}
	t.Fatalf("%s の上に、プラグインのマニフェスト（hozo-plugin.toml）がない", dir)
	return ""
}

// splitRoundTrip は、View v を、やり取りの形（docs/ir.md「まとめた形」: 出力に依らない Spec と、出力ごとの OutputView）に分け、
// JSON にして読み直し、合わせたものが、もとの View と、同じ JSON（want）になることを確かめる: プラグインが受け取る View は、
// 分けて送っても、中身が変わらない。Spec の Program は、合成した model を含まない。
func splitRoundTrip(t *testing.T, v *ir.View, want []byte) {
	t.Helper()
	spec, out := v.Split()
	specJSON, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	outJSON, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(specJSON, []byte(`$`+`"`)) && bytes.Contains(specJSON, []byte(`"name":"`+v.Viewer+`.`)) {
		for i := range spec.Program.Namespaces {
			for _, d := range spec.Program.Namespaces[i].Decls {
				if m, ok := d.(*ir.Model); ok && m.SynthesizedFrom != "" && strings.Contains(m.Name, "$") {
					t.Errorf("%s: Spec の Program に、合成した model %s が残っている", v.Viewer, m.Name)
				}
			}
		}
	}
	var spec2 ir.Spec
	var out2 ir.OutputView
	if err := json.Unmarshal(specJSON, &spec2); err != nil {
		t.Fatalf("%s: Spec を読み直せない: %v", v.Viewer, err)
	}
	if err := json.Unmarshal(outJSON, &out2); err != nil {
		t.Fatalf("%s: OutputView を読み直せない: %v", v.Viewer, err)
	}
	got, err := json.Marshal(spec2.View(&out2))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s: 分けて、合わせた View が、もとの View と違う（%d バイト、もとは %d バイト）", v.Viewer, len(got), len(want))
	}
}
