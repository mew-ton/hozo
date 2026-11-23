package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/app"
	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/fakeplugin"
	hostplugin "github.com/mew-ton/hozo/internal/plugin"
	"github.com/mew-ton/hozo/internal/wasmbuild"
	sdk "github.com/mew-ton/hozo/plugin"
)

// このファイルは、サンプル（tests/samples/<名前>/。持ち主は tests/samples/README.md）を確かめる。サンプルは
// 「仕様（.hozo と hozo.toml）と、手で書いた完成したコード」の組で、トランスパイラのブラックボックスのテストの
// 仕様になる（docs/testing.md「トランスパイラのテスト」）。
//
// テストは最後の形で書いてある。サンプルは、まだないトランスパイラと、今の hozoc がまだ読めない設定で書かれて
// いてよい。まだないトランスパイラは、ファイルを返さない代役（sampleProject.standIns）に置き換えて、あるトランスパイラの分だけを
// 走らせる。それでも通せないサブテストは、足りないもの（sampleProject.blocked、sampleProject.absent）を名指しして飛ばし、
// 足りないものがそろえば、このファイルを変えずに走る（tests/samples/README.md「テスト」）。
//
// 仕様だけのサンプル（sample.json の "specOnly": true。sampleProject.specOnly）は、期待する出力を持たないと申告したサンプルで、
// 確かめるのは check だけ。出力についてのサブテスト（files、go、transpilers）は、当てはまらないので、作らない
// （tests/samples/README.md「仕様だけのサンプル」）。
//
// サンプルは、設計文書の版の言語と出力で書く。文書の版（docs/ir.md「版」）が、今の hozoc の版より先にある間
// （sampleProject.ahead）は、今の hozoc はサンプルの仕様を読めず、文書の通りの出力も作れないので、hozoc を走らせる
// サブテスト（check と transpilers/*）を、二つの版を名指しして飛ばす（tests/samples/README.md「飛ばす条件」）。

// samplesDir はサンプルの置き場（tests/ からの相対パス）。
const samplesDir = "samples"

// sampleManifestFile は、サンプルのトランスパイラごとの事実を書いたファイル（tests/samples/README.md「sample.json」）。
const sampleManifestFile = "sample.json"

// repoPluginsDir は、リポジトリのプラグインの置き場（tests/ からの相対パス）。hozoc には HOZO_PLUGIN_PATH として渡す。
var repoPluginsDir = filepath.Join("..", "plugins")

// transpilersAll は、hozoc build の出力の全体を比べるサブテストの名前（transpilers/all）。トランスパイラの名前には使えない。
const transpilersAll = "all"

// sampleProject はサンプル一つ。
type sampleProject struct {
	name string
	dir  string // サンプルのルート（tests/ からの相対パス）

	// specOnly は、sample.json の "specOnly": 仕様だけのサンプル（期待する出力を持たないと、サンプルが申告している）。
	// 真なら、transpilers は書けず、hozo.toml に出力の単位（[[emit]]）を置けない。確かめるのは check だけ。
	specOnly bool

	// files は sample.json: トランスパイラの名前 → 期待する出力のうち、そのトランスパイラが出すファイル
	// （サンプルのルートからの相対パス）。
	files map[string][]string
	// owner は、期待する出力のファイル → それを出すトランスパイラの名前（files の逆）。
	owner map[string]string
	// tests は sample.json の tests（サブテスト tests の missing の入力）。期待する出力に、テストの対応表があるサンプルだけが持つ。
	tests *sampleTests

	// cfg は、今の hozoc（internal/config）が読んだ、サンプルの本物の hozo.toml。
	cfg *config.Config
	// unreadable は、cfg を読んだときの診断。空でなければ、今の hozoc はこの hozo.toml をまだ読めず、cfg の単位は欠けている。
	unreadable []string
	// absent は、サンプルのトランスパイラ（sample.json の名前）のうち、今は動かせないものと、その理由: プラグインがまだない、
	// マニフェストを読めない、実装がまだない。
	absent map[string]string
	// standIns は、absent のトランスパイラを、ファイルを返さない代役に置き換える手順（tests/samples/README.md「飛ばす条件」）。
	// copyInputs の後で、写したプロジェクトに当てる。
	standIns []standIn
	// blocked は、このサンプルに、本物の hozo.toml で hozoc build --no-emit / emit を掛けられない理由: unreadable と、代役を置けない
	// トランスパイラ（その単位に、動かせる基底がない）。空なら、hozoc を本物の hozo.toml で走らせるサブテストが走る。
	blocked []string
	// ahead は、設計文書の版が、今の hozoc の版より先にあるときの、その理由（docsAhead）。空でなければ、hozoc を走らせる
	// サブテスト（check と transpilers/*）は、言語の検査にも落とさず、飛ばす: 仕様そのものが、今の hozoc の読めない書き方で
	// 書かれていてよい。
	ahead string
}

// sampleTests は sample.json の tests（tests/samples/README.md「sample.json」）。
type sampleTests struct {
	// Skip は、除いて走らせる Go のテストのパターン（go test -skip に渡す）。
	Skip string `json:"skip"`
	// Missing は、そのとき、走っていないと名指しされるはずのテスト（<トランスパイラの名前>: <テストの名前>）。
	Missing []string `json:"missing"`
}

// irDoc は、IR の版の持ち主の文書（tests/ からの相対パス）。
var irDoc = filepath.Join("..", "docs", "ir.md")

// docVersion は、docs/ir.md「版」の「今の版は `"<版>"`」。
var docVersion = regexp.MustCompile("今の版は `\"([^\"]+)\"`")

// docsAhead は、設計文書の版（docs/ir.md「版」）が、今の hozoc の版（internal/plugin の Protocol）と違えば、その理由を返す。
// 文書とサンプルは、実装より先に書く（docs/testing.md「トランスパイラのテスト」）。言語や IR を変える決定では、文書の版が
// 先に上がり、サンプルは、その版の言語と出力で書かれる。同じなら ""。
func docsAhead(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(irDoc)
	if err != nil {
		t.Fatal(err)
	}
	m := docVersion.FindSubmatch(data)
	if m == nil {
		t.Fatalf("%s に「今の版は `\"<版>\"`」がない", irDoc)
	}
	if doc := string(m[1]); doc != hostplugin.Protocol {
		return fmt.Sprintf("設計文書の版（docs/ir.md「版」）は %s、今の hozoc の版（internal/plugin の Protocol）は %s: 文書とサンプルが、実装より先にある。"+
			"サンプルは文書の版の言語と出力で書いてあるので、今の hozoc では、仕様を読めず、文書の通りの出力も作れない", doc, hostplugin.Protocol)
	}
	return ""
}

// skipIfAhead は、hozoc を走らせるサブテストを、設計文書の版が今の hozoc より先にある間、二つの版を名指しして飛ばす。
func (s *sampleProject) skipIfAhead(t *testing.T, what string) {
	t.Helper()
	if s.ahead != "" {
		t.Skipf("飛ばす（%s）。足りないもの:\n  - %s\nhozoc がその版になれば、何も変えずに走る", what, s.ahead)
	}
}

// standIn は、まだ動かせないトランスパイラ一つを、ファイルを返さない代役に置き換える手順。
type standIn struct {
	transpiler string
	// manifest は、実装のないプラグインのマニフェスト（サンプルのルートからの相対パス）。その wasm の場所に、代役の WASM を置く
	// （マニフェストは、書き換えない: 語彙、requires、seams は本物のまま）。プラグインそのものがなければ ""。
	manifest string
	// language と requires は、プラグインそのものがないときに作る代役のマニフェストの中身: その単位の基底の言語と、基底の名前。
	language string
	requires []string
}

// 代役のプラグインの中身は、にせもののプラグインの WASM（internal/fakeplugin の Place）: 要求を読み、頼まれたトランスパイラごとに、
// ファイルも warning も返さない答えを返す。

// loadSample は、サンプルの sample.json と hozo.toml を読み、足りないもの（absent、blocked）を調べる。
func loadSample(t *testing.T, name string) *sampleProject {
	t.Helper()
	s := &sampleProject{name: name, dir: filepath.Join(samplesDir, name), files: map[string][]string{}, owner: map[string]string{}, absent: map[string]string{}}
	data, err := os.ReadFile(filepath.Join(s.dir, sampleManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		SpecOnly    bool `json:"specOnly"`
		Transpilers map[string]struct {
			Files []string `json:"files"`
		} `json:"transpilers"`
		Tests *sampleTests `json:"tests"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields() // ほかのキー（これまでの pending、select）は書けない
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("%s/%s を読めない: %v", s.dir, sampleManifestFile, err)
	}
	s.specOnly = m.SpecOnly
	switch {
	case s.specOnly && m.Transpilers != nil:
		t.Fatalf("%s/%s: specOnly のサンプルは、期待する出力を持たないので、transpilers を書けない", s.dir, sampleManifestFile)
	case !s.specOnly && len(m.Transpilers) == 0:
		t.Fatalf("%s/%s に transpilers がない（期待する出力を持たないサンプルなら \"specOnly\": true と書く）", s.dir, sampleManifestFile)
	}
	for name, tr := range m.Transpilers {
		if name == transpilersAll {
			t.Errorf("%s: トランスパイラの名前 %q は、サブテスト transpilers/%s と当たる", sampleManifestFile, name, transpilersAll)
		}
		s.files[name] = tr.Files
		for _, f := range tr.Files {
			if prev, dup := s.owner[f]; dup {
				t.Errorf("%s: %s が %s と %s の両方のファイルになっている", sampleManifestFile, f, prev, name)
			}
			s.owner[f] = name
		}
	}
	// tests は、期待する出力に、テストの対応表があるサンプルだけが、書く
	s.tests = m.Tests
	switch maps := s.testMaps(); {
	case len(maps) > 0 && (s.tests == nil || s.tests.Skip == "" || len(s.tests.Missing) == 0):
		t.Errorf("%s: 期待する出力にテストの対応表（%s）があるので、tests（skip と missing）が要る", sampleManifestFile, strings.Join(maps, "、"))
	case len(maps) == 0 && s.tests != nil:
		t.Errorf("%s: 期待する出力にテストの対応表がないので、tests は書けない", sampleManifestFile)
	}
	if t.Failed() {
		t.FailNow()
	}

	// 本物の hozo.toml を、今の hozoc が読む。読めない書き方（まだ実装のない設定）があれば、診断が返る
	cfg, diags := config.Load(filepath.Join(s.dir, config.FileName))
	s.cfg = cfg
	for _, d := range diags {
		s.unreadable = append(s.unreadable, fmt.Sprintf("今の hozoc（internal/config）が、%s/%s を読めない: %s", s.dir, config.FileName, diagText(d)))
	}
	s.blocked = append(s.blocked, s.unreadable...)
	if s.specOnly {
		s.requireNoOutputs(t)
	}
	s.findTranspilers(t)
	s.ahead = docsAhead(t)
	return s
}

// requireNoOutputs は、仕様だけのサンプルが、出力を持たないことを確かめる: hozo.toml に出力の単位（[[emit]]）がなく、
// Go のモジュール（go.mod）でもない。どちらかがあれば、確かめられない出力か、走らせないコードを持つサンプルが、check だけで
// 通ってしまう。hozo.toml を読めないときは、単位が分からないので、それも失敗にする（仕様だけのサンプルは、今の hozoc が
// 読める設定で書く。まだないトランスパイラを待つ理由がない）。
func (s *sampleProject) requireNoOutputs(t *testing.T) {
	t.Helper()
	for _, why := range s.unreadable {
		t.Errorf("specOnly のサンプルの %s を読めない: %s", config.FileName, why)
	}
	if n := len(s.cfg.Units); n > 0 {
		t.Errorf("%s/%s: specOnly のサンプルなのに、出力の単位（[[emit]]）が %d つある。期待する出力を持つサンプルなら、%s に transpilers を書く",
			s.dir, config.FileName, n, sampleManifestFile)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "go.mod")); err == nil {
		t.Errorf("%s: specOnly のサンプルなのに、go.mod がある（仕様だけのサンプルは、コードを持たない）", s.dir)
	}
	if t.Failed() {
		t.FailNow()
	}
}

// diagText は診断一つを、理由として読める一行にする。
func diagText(d diag.Diagnostic) string {
	return d.Code + ": " + strings.Join(strings.Fields(d.Message), " ")
}

// pluginDirs は、hozoc がこのサンプルのプラグインを探す場所（docs/plugin-protocol.md「探索」の順）: サンプルの
// hozo.toml の plugin_dirs（サンプルの中のプラグイン）と、リポジトリの plugins/（HOZO_PLUGIN_PATH として渡す）。
// tests/ からの相対パス。
func (s *sampleProject) pluginDirs() []string {
	var dirs []string
	for _, d := range s.cfg.Hozo.PluginDirs {
		dirs = append(dirs, filepath.Join(s.dir, filepath.FromSlash(d)))
	}
	return append(dirs, repoPluginsDir)
}

// findTranspilers は、サンプルのトランスパイラ（sample.json の名前）のうち、今は動かせないもの（absent）を調べ、それぞれを
// 代役に置き換える手順（standIns）を作る。動かせるのは、pluginDirs のどれかのプラグインのマニフェストを今の hozoc が読め、
// そのマニフェストがその名前のトランスパイラを提供し、プラグインに実装があるとき。
//
// 代役は、ファイルを返さないトランスパイラ: マニフェストはあるが実装のないプラグインは、wasm の場所に、代役を置く。プラグイン
// そのものがなければ、その単位の（動かせる）基底に相乗りするマニフェストを作る。単位に動かせる基底がなければ、代役を置けない
// （blocked）。
func (s *sampleProject) findTranspilers(t *testing.T) {
	t.Helper()
	var unreadable []string                         // マニフェストを読めなかったプラグイン
	provided := map[string]*hostplugin.Transpiler{} // 動かせるトランスパイラ
	unimplemented := map[string]string{}            // 実装のないプラグインのトランスパイラ → マニフェスト（サンプルのルートからの相対パス）
	unimplementedPlugin := map[string]string{}      // 同じく → プラグインのディレクトリ（理由に書く）
	seen := map[string]bool{}                       // プラグインの名前。近い場所のものを優先する（docs/plugin-protocol.md「探索」）
	for _, dir := range s.pluginDirs() {
		manifests, err := filepath.Glob(filepath.Join(dir, "*", hostplugin.ManifestFileName))
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(manifests)
		for _, p := range manifests {
			pluginDir := filepath.Dir(p)
			plugin := filepath.Base(pluginDir)
			if seen[plugin] {
				continue
			}
			seen[plugin] = true
			m, diags := hostplugin.LoadManifest(p)
			if len(diags) > 0 || m == nil {
				// 読めない（MAN-4）か、protocol が合わない（MAN-2）。どちらも、今の hozoc はこのプラグインを使えない
				for _, d := range diags {
					unreadable = append(unreadable, fmt.Sprintf("今の hozoc（internal/plugin）が、マニフェスト %s を読めない: %s", filepath.ToSlash(filepath.Clean(p)), diagText(d)))
				}
				continue
			}
			implemented := pluginImplemented(t, m, pluginDir)
			for i := range m.Transpilers {
				tr := &m.Transpilers[i]
				switch {
				case implemented:
					provided[tr.Name] = tr
				case dir != repoPluginsDir:
					if rel, err := filepath.Rel(s.dir, p); err == nil {
						unimplemented[tr.Name] = filepath.ToSlash(rel)
						unimplementedPlugin[tr.Name] = filepath.ToSlash(pluginDir)
					}
				}
			}
		}
	}
	var dirs []string
	for _, d := range s.pluginDirs() {
		dirs = append(dirs, filepath.ToSlash(filepath.Clean(d)))
	}
	for _, name := range s.names() {
		if provided[name] != nil {
			continue
		}
		if manifest, ok := unimplemented[name]; ok {
			s.absent[name] = fmt.Sprintf("プラグインの実装がまだない（%s に、テストでない Go のファイルも、マニフェストの wasm のファイルもない）", unimplementedPlugin[name])
			s.standIns = append(s.standIns, standIn{transpiler: name, manifest: manifest})
			continue
		}
		s.absent[name] = fmt.Sprintf("トランスパイラ %s がまだない（%s の、今の hozoc が動かせるどのプラグインも提供していない）", name, strings.Join(dirs, " と "))
		if len(unreadable) > 0 {
			s.absent[name] += "\n    " + strings.Join(unreadable, "\n    ")
		}
		// その単位の、動かせる基底に相乗りする代役
		in := standIn{transpiler: name}
		for _, u := range s.cfg.Units {
			listed := false
			for _, n := range u.Transpilers {
				listed = listed || n == name
			}
			if !listed {
				continue
			}
			base := ""
			for _, n := range u.Transpilers {
				if tr := provided[n]; tr != nil && tr.IsBase() {
					base, in.language = n, tr.Language
				}
			}
			if base == "" {
				s.blocked = append(s.blocked, fmt.Sprintf("%s（%d 番目の [[emit]] に、動かせる基底がないので、代役も置けない）", s.absent[name], u.Index))
				continue
			}
			if !containsString(in.requires, base) {
				in.requires = append(in.requires, base)
			}
		}
		s.standIns = append(s.standIns, in)
	}
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// absentReasons は、動かせないトランスパイラの理由（名前の辞書順）。
func (s *sampleProject) absentReasons() []string {
	var out []string
	for _, name := range s.names() {
		if why, ok := s.absent[name]; ok {
			out = append(out, why)
		}
	}
	return out
}

// placeStandIns は、root に写したプロジェクトに、代役を置く（tests/samples/README.md「飛ばす条件」）: 実装のないプラグインは、
// マニフェストの wasm の場所に、代役の WASM を置き、プラグインそのものがないトランスパイラは、dir（HOZO_PLUGIN_PATH で渡す
// ディレクトリ）に、代役のプラグインを作る。
func (s *sampleProject) placeStandIns(t *testing.T, root, dir string) {
	t.Helper()
	for _, in := range s.standIns {
		if in.manifest != "" {
			p := filepath.Join(root, filepath.FromSlash(in.manifest))
			m, diags := hostplugin.LoadManifest(p)
			if m == nil {
				t.Fatalf("%s を読めない: %+v", in.manifest, diags)
			}
			fakeplugin.Place(t, m.WasmPath(filepath.Dir(p)))
			continue
		}
		quoted := make([]string, len(in.requires))
		for i, r := range in.requires {
			quoted[i] = fmt.Sprintf("%q", r)
		}
		manifest := fmt.Sprintf("# TestSamples が置いた代役（tests/samples/README.md「飛ばす条件」）。ファイルを返さない\nname = %q\nwasm = %s\nprotocol = %q\n\n[[transpiler]]\nname = %q\nlanguage = %q\nrequires = [%s]\n",
			in.transpiler, fakeplugin.WasmTOML, hostplugin.Protocol, in.transpiler, in.language, strings.Join(quoted, ", "))
		p := filepath.Join(dir, in.transpiler, hostplugin.ManifestFileName)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
		fakeplugin.Place(t, filepath.Join(filepath.Dir(p), fakeplugin.WasmFile))
	}
}

// pluginImplemented は、マニフェスト m のプラグインに、動かすものがあるか。サンプルは、マニフェストと語彙と期待する出力を、
// プラグインの実装より先に書く（docs/testing.md「サンプル」）。マニフェストのディレクトリ（main パッケージの置き場。
// docs/packages.md「plugins/<名前>」）に、テストでない Go のファイル（TestMain が、WASM にビルドする）か、マニフェストの wasm の
// ファイルがあれば、実装があるとする。
func pluginImplemented(t *testing.T, m *hostplugin.Manifest, dir string) bool {
	t.Helper()
	if wasmbuild.HasGoSources(dir) {
		return true
	}
	_, err := os.Stat(m.WasmPath(dir))
	return err == nil
}

// skipIfBlocked は、hozoc を本物の hozo.toml で走らせるサブテストを、走らせられない理由があれば、それを名指しして飛ばす。
func (s *sampleProject) skipIfBlocked(t *testing.T, what string) {
	t.Helper()
	if len(s.blocked) > 0 {
		t.Skipf("飛ばす（%s）。足りないもの:\n  - %s\nそろえば、何も変えずに走る", what, strings.Join(s.blocked, "\n  - "))
	}
}

// outs は、hozo.toml の単位の out（設定の順。ルートからの相対パス）。
func (s *sampleProject) outs() []string {
	var outs []string
	for _, u := range s.cfg.Units {
		outs = append(outs, config.CleanRel(u.Out))
	}
	return outs
}

// names はサンプルのトランスパイラの名前（辞書順）。
func (s *sampleProject) names() []string {
	var names []string
	for name := range s.files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// expected は、サンプルの期待する出力（単位の out の下のすべてのファイル）を、サンプルのルートからの相対パス → 内容で返す。
func (s *sampleProject) expected(t *testing.T) map[string][]byte {
	t.Helper()
	want := map[string][]byte{}
	for _, out := range s.outs() {
		for p, content := range readTree(t, filepath.Join(s.dir, filepath.FromSlash(out))) {
			want[path.Join(out, p)] = content
		}
	}
	return want
}

// copyInputs は、サンプルの入力を root に写す: hozo.toml、.hozo、go.mod、go.sum と、サンプルの中のプラグイン（hozo.toml の
// plugin_dirs の下のすべて）。期待する出力（単位の out の下）と、人が書いたコードは写さない。
// go.mod は、基底が import パスを決めるのに要る（docs/plugin-go.md「パッケージと import パス」。なければ GO-1）。その replace は
// リポジトリへの相対パスなので、絶対パスに書き換える（root で、生成したコードを、go の道具で確かめられるように）。
func (s *sampleProject) copyInputs(t *testing.T, root string) {
	t.Helper()
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	var pluginDirs, outs []string
	for _, d := range s.cfg.Hozo.PluginDirs {
		pluginDirs = append(pluginDirs, config.CleanRel(d))
	}
	outs = s.outs()
	under := func(rel string, dirs []string) bool {
		for _, d := range dirs {
			if rel == d || config.Under(rel, d) {
				return true
			}
		}
		return false
	}
	err = filepath.WalkDir(s.dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(s.dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if under(rel, outs) {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		isInput := name == config.FileName || name == "go.mod" || name == "go.sum" || strings.HasSuffix(name, ".hozo")
		if !isInput && !under(rel, pluginDirs) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if rel == "go.mod" {
			lines := strings.Split(string(data), "\n")
			for i, l := range lines {
				if strings.HasPrefix(l, "replace "+modulePath+" => ") {
					lines[i] = "replace " + modulePath + " => " + repo
				}
			}
			data = []byte(strings.Join(lines, "\n"))
		}
		dst := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// copySpecs は、サンプルのすべての .hozo（サンプルの中のプラグインの語彙を含む）を、同じ相対パスで root に写す。
// hozo.toml は写さない（testCheck の、言語の検査だけの形）。設定がなければ、hozoc はルートの下のすべての .hozo を
// 仕様として読むので、プラグインの語彙も、宣言として読まれる。
func (s *sampleProject) copySpecs(t *testing.T, root string) {
	t.Helper()
	n := 0
	err := filepath.WalkDir(s.dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".hozo") {
			return err
		}
		rel, err := filepath.Rel(s.dir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		dst := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		n++
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatalf("%s に .hozo がない", s.dir)
	}
}

// pluginPathEnv は、HOZO_PLUGIN_PATH だけが dir である環境。
func pluginPathEnv(dir string) func(string) string {
	return func(name string) string {
		if name == hostplugin.PluginPathEnv {
			return dir
		}
		return ""
	}
}

// hozocMainEnv は hozocMainIn と同じ組み立てで、環境変数を env から読む（プロセスの環境は読まない）。
func hozocMainEnv(dir string, env func(string) string, args []string, stdout, stderr io.Writer) int {
	a := &app.App{Dir: dir, Stderr: stderr}
	return app.Main(context.Background(), a, args, env, stdout, stderr)
}

// project は、サンプルの入力を一時ディレクトリに写し、そのルートと、hozoc に渡す環境（リポジトリの plugins/ を
// HOZO_PLUGIN_PATH に持つ。docs/plugin-protocol.md「探索」の 2）を返す。hozo.toml は本物のまま。まだ動かせないトランスパイラが
// あれば、代役を置く（placeStandIns）: 代役のプラグインのディレクトリは、HOZO_PLUGIN_PATH の、リポジトリの plugins/ の後ろ。
func (s *sampleProject) project(t *testing.T) (string, func(string) string) {
	t.Helper()
	tmp := t.TempDir()
	root := filepath.Join(tmp, "project")
	s.copyInputs(t, root)
	plugins, err := filepath.Abs(repoPluginsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.standIns) == 0 {
		return root, pluginPathEnv(plugins)
	}
	standIns := filepath.Join(tmp, "stand-ins")
	s.placeStandIns(t, root, standIns)
	return root, pluginPathEnv(plugins + string(filepath.ListSeparator) + standIns)
}

// TestSamples は tests/samples/ の各サンプルについて、次を確かめる（tests/samples/README.md「テスト」）。
//
//   - check: サンプルの仕様と設定が、本物の hozo.toml とプラグインで、hozoc build --no-emit を error なしで通る。
//   - files: 期待する出力のファイル、sample.json、hozo.toml、出力先ごとの記録が合っている（hozoc を動かさない）。
//   - go: 期待する出力と人が書いたコードが、サンプルのモジュールで go vet と go test を通る。
//   - transpilers/<名前>: hozoc build の出力のうち、そのトランスパイラのファイルが、期待する出力とバイト単位で同じ。
//   - transpilers/all: hozoc build の出力の全体が、期待する出力と同じ（余分なファイルがなく、記録が同じ）。
//
// go は飛ばさない。設計文書の版が今の hozoc より先にある間は、check と transpilers/* を、版を名指しして飛ばす（docsAhead）。
// まだないトランスパイラは、ファイルを返さない代役に置き換えて、check と、あるトランスパイラの
// transpilers/<名前> を走らせる。まだないトランスパイラの transpilers/<名前> と transpilers/all は、足りないものを名指しして
// 飛ばす。本物の hozo.toml を読めないときは、どれも飛ばし、check だけは、設定を使わない言語の検査に落として通す（testCheck）。
//
// 仕様だけのサンプル（sample.json の "specOnly": true）は、check だけを持つ: 期待する出力がないと申告しているので、
// files、go、transpilers は、飛ばすのではなく、作らない。申告と食い違うもの（出力の単位、go.mod、transpilers）があれば、
// そのサンプルは失敗する（loadSample）。
func TestSamples(t *testing.T) {
	requireGo(t)
	manifests, err := filepath.Glob(filepath.Join(samplesDir, "*", sampleManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) == 0 {
		t.Fatalf("%s にサンプルがない", samplesDir)
	}
	for _, m := range manifests {
		name := filepath.Base(filepath.Dir(m))
		t.Run(name, func(t *testing.T) {
			s := loadSample(t, name)
			t.Run("check", s.testCheck)
			if s.specOnly {
				return
			}
			t.Run("files", s.testFiles)
			t.Run("go", s.testGo)
			t.Run("transpilers", s.testTranspilers)
			if len(s.testMaps()) > 0 {
				t.Run("tests", s.testTests)
			}
		})
	}
}

// testCheck は、サンプルの仕様と設定が hozoc build --no-emit を error なしで通ること。サンプルの入力を一時ディレクトリに写し、
// 本物の hozo.toml のまま、サンプルの中のプラグインと、リポジトリの plugins/ のプラグインで確かめる。warning は許し、表示する。
// まだないトランスパイラは、代役に置き換える（project）。代役は、マニフェストだけが要る（check はトランスパイラを起動しない）。
//
// 本物の hozo.toml を今の hozoc が読めない（blocked）間は、**言語の検査だけ**に落とす: すべての .hozo（サンプルの中の
// プラグインの語彙を含む）だけを写し、設定なしで hozoc build --no-emit を通す（hozo.toml がなければ、hozoc は設定の検査 CFG-* を
// しない。docs/config.md「置き場と探し方」）。落としたことと、足りないものは、ログに書く。足りないものがそろえば、本物の設定での
// 検査に戻る。
func (s *sampleProject) testCheck(t *testing.T) {
	s.skipIfAhead(t, "hozoc build --no-emit を、このサンプルに掛けられない")
	root, env := "", func(string) string { return "" }
	if len(s.blocked) > 0 {
		t.Logf("言語の検査だけに落とした（設定 %s と、プラグインの検査は、まだ確かめていない）。足りないもの:\n  - %s\nそろえば、本物の hozo.toml で走る",
			config.FileName, strings.Join(s.blocked, "\n  - "))
		root = filepath.Join(t.TempDir(), "project")
		s.copySpecs(t, root)
	} else {
		root, env = s.project(t)
		if len(s.absent) > 0 {
			t.Logf("まだないトランスパイラを、ファイルを返さない代役に置き換えた:\n  - %s", strings.Join(s.absentReasons(), "\n  - "))
		}
	}
	var stdout, stderr bytes.Buffer
	if code := hozocMainEnv(root, env, []string{"build", "--no-emit"}, &stdout, &stderr); code != 0 {
		t.Fatalf("hozoc build --no-emit の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
	}
	t.Logf("hozoc build --no-emit:\n%s", stdout.String())
}

// testFiles は、hozoc を動かさずに確かめられること: 期待する出力のファイルと、sample.json と、hozo.toml と、
// 出力先ごとの記録（.hozoc-outputs.json）が合っている。
//
//   - 単位に書いたトランスパイラは、どれも sample.json にあり、sample.json のトランスパイラは、どれかの単位が使っている。
//   - sample.json のファイルは、どれもあり、どれかの単位の out の下にある。
//   - 単位の out の下のファイルは、記録を除いて、どれも sample.json のどれかのトランスパイラのファイル
//     （hozoc 本体はコードを出力しない。出どころのないファイルを置かせない）。
//   - 記録は、そのディレクトリの下のファイル（もっと深い記録のあるディレクトリの下のものを除く）のパスを、
//     辞書順に並べた JSON の配列（docs/cli.md「書き出し」の 6）。記録の下にないファイルはない。
//
// 単位（out と transpilers）は、今の hozoc が読んだ hozo.toml から取る。読めなければ、単位が欠けるので、理由を示して飛ばす。
func (s *sampleProject) testFiles(t *testing.T) {
	if len(s.unreadable) > 0 {
		t.Skipf("飛ばす（単位の out と transpilers が分からない）。足りないもの:\n  - %s\nそろえば、何も変えずに走る", strings.Join(s.unreadable, "\n  - "))
	}
	used := map[string]bool{}
	outs := s.outs()
	for _, u := range s.cfg.Units {
		for _, name := range u.Transpilers {
			used[name] = true
			if _, ok := s.files[name]; !ok {
				t.Errorf("%s が単位に書いたトランスパイラ %s が、%s にない", config.FileName, name, sampleManifestFile)
			}
		}
	}
	for _, name := range s.names() {
		if !used[name] {
			t.Errorf("%s のトランスパイラ %s を、%s のどの単位も使っていない", sampleManifestFile, name, config.FileName)
		}
		for _, f := range s.files[name] {
			underOut := false
			for _, out := range outs {
				underOut = underOut || config.Under(f, out)
			}
			if !underOut {
				t.Errorf("%s: %s のファイル %s が、どの単位の out の下にもない", sampleManifestFile, name, f)
			}
		}
	}

	want := s.expected(t)
	listed := map[string][]string{} // 記録のあるディレクトリ → その記録が挙げるはずのファイル
	for p := range want {
		if path.Base(p) == app.OutputsFile {
			listed[path.Dir(p)] = nil
		}
	}
	for _, p := range sortedKeys(want) {
		if path.Base(p) == app.OutputsFile {
			continue
		}
		if s.owner[p] == "" {
			t.Errorf("%s/%s は、%s のどのトランスパイラのファイルでもない", s.dir, p, sampleManifestFile)
		}
		dir, found := "", false
		for d := range listed {
			if config.Under(p, d) && (!found || len(d) > len(dir)) {
				dir, found = d, true
			}
		}
		if !found {
			t.Errorf("%s/%s を下に持つ記録（%s）がない", s.dir, p, app.OutputsFile)
			continue
		}
		listed[dir] = append(listed[dir], p)
	}
	for f, name := range s.owner {
		if _, ok := want[f]; !ok {
			t.Errorf("%s/%s: %s が出すはずのファイルがない（%s）", s.dir, f, name, sampleManifestFile)
		}
	}
	for dir, paths := range listed {
		sort.Strings(paths)
		data, err := json.MarshalIndent(paths, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		record := path.Join(dir, app.OutputsFile)
		if got := append(data, '\n'); !bytes.Equal(want[record], got) {
			t.Errorf("%s/%s が、その下のファイルの一覧と違う:\n%s", s.dir, record, firstDiff(string(want[record]), string(got)))
		}
	}
}

// testGo は、サンプルが動くこと: サンプルのモジュール（リポジトリのモジュールとは別。自分の go.mod を持つ）で
// `go vet ./...` と `go test ./...` を通す。期待する出力がコンパイルでき、人が書いたコードと合わせて仕様の通りに動く。
// 飛ばさない: 動くと確かめていないサンプルを、仕様として通さない。
//
// 依存はサンプルの go.sum の通りのもの（-mod=readonly）を、モジュールキャッシュから使う。キャッシュにあれば
// ネットワークは要らない。なければ go が GOPROXY から取りに行き、取れなければ（オフラインなど）このテストは失敗する。
// ツールチェーンは取りに行かない（GOTOOLCHAIN=local）。
func (s *sampleProject) testGo(t *testing.T) {
	if _, err := os.Stat(filepath.Join(s.dir, "go.mod")); err != nil {
		t.Skipf("%s は Go のモジュールではない", s.dir)
	}
	for _, args := range [][]string{{"vet", "./..."}, {"test", "./..."}} {
		cmd := exec.Command("go", args...)
		cmd.Dir = s.dir
		cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOFLAGS=-mod=readonly", "GOWORK=off")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s で go %s が通らない: %v\n%s\n"+
				"（依存を取れないという誤りなら: サンプルの依存がモジュールキャッシュになく、ネットワークからも取れなかった。"+
				"ネットワークのあるところで、%s で `go mod download` を一度走らせる）",
				s.dir, strings.Join(args, " "), err, out, s.dir)
		}
		t.Logf("go %s:\n%s", strings.Join(args, " "), out)
	}
}

// testMaps は、期待する出力のうち、テストの対応表（docs/plugin-protocol.md「テストの対応表」）のパス（辞書順）。
func (s *sampleProject) testMaps() []string {
	var out []string
	for p := range s.owner {
		if _, ok := sdk.TestMapTranspiler(p); ok {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// testTests は、求めるテストが走ったかを、カバレッジと突き合わせられること（docs/cli.md「tests」、tests/samples/README.md
// 「テスト」）。サンプルのモジュールで、カバレッジを取りながらテストを走らせ、サンプルのディレクトリ（期待する出力が、置き場に
// ある）で hozoc tests を走らせる。hozoc は、何も書き出さない。
//
//   - all: すべてのテストを走らせたカバレッジで、終了コード 0。ran が、期待する出力の対応表のテストのすべて。
//   - missing: sample.json の tests.skip の Go のテストを除いて走らせたカバレッジ（口を渡す一行を、書かなかったのと同じ）で、
//     終了コード 4。走っていないと名指しされるテストが、ちょうど tests.missing。
func (s *sampleProject) testTests(t *testing.T) {
	const what = "hozoc tests を、このサンプルに掛けられない"
	s.skipIfAhead(t, what)
	s.skipIfBlocked(t, what)
	if len(s.absent) > 0 {
		t.Skipf("飛ばす（求めるテストは、すべてのトランスパイラの対応表がそろってから比べる）。足りないもの:\n  - %s\nそろえば、何も変えずに走る",
			strings.Join(s.absentReasons(), "\n  - "))
	}
	plugins, err := filepath.Abs(repoPluginsDir)
	if err != nil {
		t.Fatal(err)
	}
	// 求めるテスト: 期待する出力の対応表（手で書いたもの）の、<トランスパイラの名前>: <テストの名前>
	var want []string
	for _, p := range s.testMaps() {
		data, err := os.ReadFile(filepath.Join(s.dir, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		var m sdk.TestMap
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("%s/%s が JSON でない: %v", s.dir, p, err)
		}
		transpiler, _ := sdk.TestMapTranspiler(p)
		for _, e := range m.Tests {
			want = append(want, transpiler+": "+e.Name)
		}
	}
	sort.Strings(want)
	// cover は、サンプルのモジュールで、カバレッジを取りながらテストを走らせ、カバレッジのファイルを返す。skip が空でなければ、
	// そのパターンの Go のテストを除く。-coverpkg が要る理由は docs/plugin-go.md「カバレッジを読む」。
	cover := func(t *testing.T, skip string) string {
		t.Helper()
		file := filepath.Join(t.TempDir(), "cover.out")
		args := []string{"test", "-count=1", "-coverprofile=" + file, "-coverpkg=./..."}
		if skip != "" {
			args = append(args, "-skip", skip)
		}
		cmd := exec.Command("go", append(args, "./...")...)
		cmd.Dir = s.dir
		cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOFLAGS=-mod=readonly", "GOWORK=off")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s で go %s が通らない: %v\n%s", s.dir, strings.Join(args, " "), err, out)
		}
		return file
	}
	t.Run("all", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := hozocMainEnv(s.dir, pluginPathEnv(plugins), []string{"tests", "--coverage", cover(t, ""), "--format=json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("hozoc tests が終了コード %d で終わった（want 0: 求めるテストが、すべて走っている）:\n%s%s", code, stdout.String(), stderr.String())
		}
		var result struct {
			Ran []string `json:"ran"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatalf("hozoc tests の出力が JSON でない: %v\n%s", err, stdout.String())
		}
		sort.Strings(result.Ran)
		if strings.Join(result.Ran, "\n") != strings.Join(want, "\n") {
			t.Errorf("走ったと言ったテストが、期待する出力の対応表のテストと違う:\n got:\n  %s\nwant:\n  %s", strings.Join(result.Ran, "\n  "), strings.Join(want, "\n  "))
		}
		t.Logf("%d tests ran", len(result.Ran))
	})
	t.Run("missing", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := hozocMainEnv(s.dir, pluginPathEnv(plugins), []string{"tests", "--coverage", cover(t, s.tests.Skip), "--format=json"}, &stdout, &stderr)
		if code != 4 {
			t.Fatalf("hozoc tests が終了コード %d で終わった（want 4: TestsNotRun）:\n%s%s", code, stdout.String(), stderr.String())
		}
		var failure struct {
			Error string `json:"error"`
			Value struct {
				Diagnostics []struct {
					Severity string `json:"severity"`
					Code     string `json:"code"`
					Message  string `json:"message"`
				} `json:"diagnostics"`
			} `json:"value"`
		}
		if err := json.Unmarshal(stderr.Bytes(), &failure); err != nil || failure.Error != "hozo.compiler.TestsNotRun" {
			t.Fatalf("hozoc tests の例外が hozo.compiler.TestsNotRun でない: %v\n%s", err, stderr.String())
		}
		var named []string
		for _, d := range failure.Value.Diagnostics {
			if d.Severity != "error" {
				continue
			}
			label, _, ok := strings.Cut(strings.TrimPrefix(d.Message, "求めるテスト "), " が、")
			if d.Code != "TESTS-1" || !strings.HasPrefix(d.Message, "求めるテスト ") || !ok {
				t.Errorf("TESTS-1 でない error: %s: %s", d.Code, d.Message)
				continue
			}
			named = append(named, label)
		}
		sort.Strings(named)
		missing := append([]string{}, s.tests.Missing...)
		sort.Strings(missing)
		if strings.Join(named, "\n") != strings.Join(missing, "\n") {
			t.Errorf("走っていないと名指しされたテストが、%s の tests.missing と違う:\n got:\n  %s\nwant:\n  %s", sampleManifestFile, strings.Join(named, "\n  "), strings.Join(missing, "\n  "))
		}
		for _, m := range missing {
			if !containsString(want, m) {
				t.Errorf("%s の tests.missing の %q が、期待する出力の対応表にない", sampleManifestFile, m)
			}
		}
	})
}

// testTranspilers は、トランスパイラのブラックボックスのテスト（docs/testing.md「トランスパイラのテスト」）:
// サンプルの入力を一時ディレクトリに写し、本物の hozo.toml のまま、サンプルの中のプラグインと、リポジトリの plugins/ の
// プラグインのトランスパイラで、hozoc build を一度だけ走らせる。書き出されたファイルを、サブテストが分けて、
// サンプルの期待する出力とバイト単位で比べる。
//
//   - transpilers/<名前>: sample.json の、そのトランスパイラの files が、どれも書き出され、期待する出力と同じ。
//     失敗が、どのトランスパイラのものかが分かり、一つ実装するごとに、その分が通る。
//   - transpilers/all: 出力の全体が同じ。どのトランスパイラの files にもないファイルが書かれていない。期待する出力の
//     ファイルが、すべて書かれている。出力先ごとの記録が同じ。
//
// まだないトランスパイラ（absent）は、ファイルを返さない代役に置き換えて、hozoc build を走らせる（どのトランスパイラも、自分の
// 出力の View だけから応答を作るので、あるトランスパイラの出力は、代役がいても変わらない。docs/plugin-protocol.md「起動」）。
// その transpilers/<名前> と、transpilers/all は、足りないものを名指しして飛ばす。本物の hozo.toml で走らせられない（blocked）か、
// 動かせるトランスパイラが一つもなければ、どのサブテストも飛ばす。期待する出力が仕様なので、-update では書き直さない。
func (s *sampleProject) testTranspilers(t *testing.T) {
	var got, want map[string][]byte
	runnable := s.ahead == "" && len(s.blocked) == 0 && len(s.absent) < len(s.files)
	if runnable {
		got, want = s.emit(t), s.expected(t)
	}
	for _, name := range s.names() {
		t.Run(name, func(t *testing.T) {
			s.skipIfAhead(t, "hozoc build を、このサンプルに掛けられない")
			s.skipIfBlocked(t, "hozoc build を、このサンプルに掛けられない")
			if why, absent := s.absent[name]; absent {
				t.Skipf("飛ばす（トランスパイラ %s を動かせない）。足りないもの:\n  - %s\nそろえば、何も変えずに走る", name, why)
			}
			for _, p := range s.files[name] {
				w, inWant := want[p]
				g, inGot := got[p]
				switch {
				case !inWant:
					t.Errorf("%s/%s: %s が出すはずのファイルが、期待する出力にない（%s）", s.dir, p, name, sampleManifestFile)
				case !inGot:
					t.Errorf("%s/%s は期待する出力にあるが、emit は書かない（%s のファイル）", s.dir, p, name)
				case !bytes.Equal(w, g):
					t.Errorf("%s/%s が期待する出力と違う（%s のファイル）:\n%s", s.dir, p, name, firstDiff(string(w), string(g)))
				}
			}
			// このトランスパイラのファイルでないものを、このトランスパイラの名前で書いていないこと（ほかのトランスパイラが代役の間も、
			// 余分なファイルを見逃さない）: 書かれたファイルは、どれも、動かせるどれかのトランスパイラのもの
			if len(s.absent) > 0 {
				for _, p := range sortedKeys(got) {
					if path.Base(p) == app.OutputsFile {
						continue
					}
					if owner := s.owner[p]; owner == "" || s.absent[owner] != "" {
						t.Errorf("%s/%s は emit が書くが、動かせるどのトランスパイラのファイルでもない（%s）", s.dir, p, sampleManifestFile)
					}
				}
			}
		})
	}
	t.Run(transpilersAll, func(t *testing.T) {
		s.skipIfAhead(t, "hozoc build を、このサンプルに掛けられない")
		s.skipIfBlocked(t, "hozoc build を、このサンプルに掛けられない")
		if len(s.absent) > 0 {
			t.Skipf("飛ばす（出力の全体は、すべてのトランスパイラがそろってから比べる）。足りないもの:\n  - %s\nそろえば、何も変えずに走る",
				strings.Join(s.absentReasons(), "\n  - "))
		}
		for _, p := range sortedKeys(want, got) {
			w, inWant := want[p]
			g, inGot := got[p]
			isRecord := path.Base(p) == app.OutputsFile
			switch {
			case !inWant && s.owner[p] == "":
				t.Errorf("%s/%s は emit が書くが、%s のどのトランスパイラのファイルでもない", s.dir, p, sampleManifestFile)
			case !inWant:
				t.Errorf("%s/%s は emit が書くが、期待する出力にない", s.dir, p)
			case !inGot:
				t.Errorf("%s/%s は期待する出力にあるが、emit は書かない", s.dir, p)
			case isRecord && !bytes.Equal(w, g):
				t.Errorf("%s/%s（出力先ごとの記録）が期待する出力と違う:\n%s", s.dir, p, firstDiff(string(w), string(g)))
			}
			// 記録でないファイルの内容の違いは、transpilers/<名前> が、持ち主のトランスパイラの名前で報告する
		}
	})
}

// emit は、サンプルの入力を一時ディレクトリに写して hozoc build を走らせ、書き出されたファイル（入力になかったもの）を、
// サンプルのルートからの相対パス → 内容で返す。
func (s *sampleProject) emit(t *testing.T) map[string][]byte {
	t.Helper()
	root, env := s.project(t)
	before := readTree(t, root)
	var stdout, stderr bytes.Buffer
	if code := hozocMainEnv(root, env, []string{"build"}, &stdout, &stderr); code != 0 {
		t.Fatalf("hozoc build の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
	}
	got := readTree(t, root)
	for p := range before {
		delete(got, p)
	}
	return got
}
