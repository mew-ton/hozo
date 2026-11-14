package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/app"
	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/gen/hozoc/cli"
	hostplugin "github.com/mew-ton/hozo/internal/plugin"
)

// このファイルは、サンプル（tests/samples/<名前>/。持ち主は tests/samples/README.md）を確かめる。サンプルは
// 「仕様（.hozo と hozo.toml）と、手で書いた完成したコード」の組で、まだないトランスパイラのブラックボックスのテストの
// 仕様になる（docs/testing.md「トランスパイラのテスト」）。

// samplesDir はサンプルの置き場（tests/ からの相対パス）。
const samplesDir = "samples"

// sampleManifestFile は、サンプルのトランスパイラごとの事実を書いたファイル（tests/samples/README.md「sample.json」）。
const sampleManifestFile = "sample.json"

// sampleProjectTranspiler は sample.json のトランスパイラ一つ分。
type sampleProjectTranspiler struct {
	// Select は、このトランスパイラが出力の対象に選ぶ宣言。tests/plugins/pick の規則の形（デコレータの FQN）で書く。
	// トランスパイラがまだない間、pick がこの規則でそのトランスパイラの代役をする。
	Select json.RawMessage `json:"select"`
	// Files は、期待する出力のうちこのトランスパイラが出すファイル（サンプルのルートからの相対パス）。
	// 単位の out の下の、ここにないファイルは、共有する形（hozoc の既定の出力）。
	Files []string `json:"files"`
}

// sampleProject はサンプル一つ。
type sampleProject struct {
	name        string
	dir         string                             // サンプルのルート（tests/ からの相対パス）
	transpilers map[string]sampleProjectTranspiler // sample.json
	outs        []string                           // hozo.toml の単位の out（設定の順）
	owner       map[string]string                  // トランスパイラのファイル → トランスパイラの名前
}

// loadSample は、サンプルの sample.json と hozo.toml を読み、二つが合っていることを確かめる:
// 単位に書いたトランスパイラはどれも sample.json にあり、sample.json のファイルはどれも単位の out の下にある。
func loadSample(t *testing.T, name string) *sampleProject {
	t.Helper()
	s := &sampleProject{name: name, dir: filepath.Join(samplesDir, name), owner: map[string]string{}}
	data, err := os.ReadFile(filepath.Join(s.dir, sampleManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Transpilers map[string]sampleProjectTranspiler `json:"transpilers"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("%s/%s を読めない: %v", s.dir, sampleManifestFile, err)
	}
	s.transpilers = m.Transpilers

	// 本物の hozo.toml。トランスパイラがあるかどうかは見ない（見るのは internal/project の CFG-14）
	cfg, diags := config.Load(filepath.Join(s.dir, config.FileName))
	if len(diags) > 0 {
		t.Fatalf("%s/%s: %+v", s.dir, config.FileName, diags)
	}
	used := map[string]bool{}
	for _, u := range cfg.Units {
		s.outs = append(s.outs, config.CleanRel(u.Out))
		for _, name := range u.Transpilers {
			used[name] = true
			if _, ok := s.transpilers[name]; !ok {
				t.Errorf("%s が単位に書いたトランスパイラ %s が、%s にない", config.FileName, name, sampleManifestFile)
			}
		}
	}
	for name, tr := range s.transpilers {
		if !used[name] {
			t.Errorf("%s のトランスパイラ %s を、%s のどの単位も使っていない", sampleManifestFile, name, config.FileName)
		}
		for _, f := range tr.Files {
			if prev, dup := s.owner[f]; dup {
				t.Errorf("%s: %s が %s と %s の両方のファイルになっている", sampleManifestFile, f, prev, name)
			}
			s.owner[f] = name
			if !s.underOut(f) {
				t.Errorf("%s: %s のファイル %s が、どの単位の out の下にもない", sampleManifestFile, name, f)
			}
		}
	}
	if t.Failed() {
		t.FailNow()
	}
	return s
}

// underOut は、サンプルのルートからの相対パス p が、どれかの単位の out の下にあるか。
func (s *sampleProject) underOut(p string) bool {
	for _, out := range s.outs {
		if config.Under(p, out) {
			return true
		}
	}
	return false
}

// names はサンプルのトランスパイラの名前（辞書順）。
func (s *sampleProject) names() []string {
	var names []string
	for name := range s.transpilers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// expected は、サンプルの期待する出力（単位の out の下のすべてのファイル）を、サンプルのルートからの相対パス → 内容で返す。
func (s *sampleProject) expected(t *testing.T) map[string][]byte {
	t.Helper()
	want := map[string][]byte{}
	for _, out := range s.outs {
		for p, content := range readTree(t, filepath.Join(s.dir, filepath.FromSlash(out))) {
			want[path.Join(out, p)] = content
		}
	}
	return want
}

// copyInputs は、サンプルの入力（hozo.toml、.hozo、go.mod、go.sum）を root に写す。期待する出力と人の手のコードは写さない。
// go.mod は、既定の出力が import パスを決めるのに要る（docs/emit-go.md「配置」。なければ GOEMIT-1）。その replace は
// リポジトリへの相対パスなので、絶対パスに書き換える（root から `go run` でプラグインを動かせるように）。
func (s *sampleProject) copyInputs(t *testing.T, root string) {
	t.Helper()
	repo, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(s.dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(s.dir, p)
		if err != nil {
			return err
		}
		name := d.Name()
		if name != config.FileName && name != "go.mod" && name != "go.sum" && !strings.HasSuffix(name, ".hozo") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if name == "go.mod" {
			lines := strings.Split(string(data), "\n")
			for i, l := range lines {
				if strings.HasPrefix(l, "replace "+modulePath+" => ") {
					lines[i] = "replace " + modulePath + " => " + repo
				}
			}
			data = []byte(strings.Join(lines, "\n"))
		}
		dst := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// standIn は、サンプルの入力を一時ディレクトリに写し、トランスパイラの代役を入れたプラグインのディレクトリを作る。
// 戻り値はプロジェクトのルートと、hozoc に渡す環境（HOZO_PLUGIN_PATH だけを持つ。docs/plugin-protocol.md「探索」の 2）。
//
// サンプルの本物の hozo.toml は、まだないトランスパイラの名前を書いている。それを書き換えずに使うために、
// 同じ名前のトランスパイラを提供するプラグイン pick（tests/plugins/pick。ファイルは出さず、sample.json の select の
// 規則で選ぶだけ）を、プロジェクトの外に置く。pick は pickExe（go build したもの）で動かす: 一時ディレクトリは
// リポジトリのモジュールの外なので、コーパスのマニフェストの `go run` は使えない。
func (s *sampleProject) standIn(t *testing.T, pickExe string) (string, func(string) string) {
	t.Helper()
	tmp := t.TempDir()
	root := filepath.Join(tmp, "project")
	s.copyInputs(t, root)

	plugins := filepath.Join(tmp, "plugins")
	pick := filepath.Join(plugins, "pick")
	if err := os.MkdirAll(pick, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf("name     = \"pick\"\ncommand  = [%q]\nprotocol = %q\n", pickExe, hostplugin.Protocol)
	rules := map[string]json.RawMessage{}
	for _, name := range s.names() {
		manifest += fmt.Sprintf("\n[[transpiler]]\nname     = %q\nlanguage = \"go\"\n", name)
		rules[name] = s.transpilers[name].Select
	}
	pickJSON, err := json.Marshal(map[string]any{"transpilers": rules})
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{hostplugin.ManifestFileName: []byte(manifest), "pick.json": pickJSON} {
		if err := os.WriteFile(filepath.Join(pick, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, pluginPathEnv(plugins)
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
	return cli.Main(a.Ops(), app.Presenter{}, args, env, stdout, stderr)
}

// buildPick は tests/plugins/pick を go build し、実行ファイルのパスを返す。
func buildPick(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "pick")
	build := exec.Command("go", "build", "-o", exe, "./plugins/pick")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("tests/plugins/pick を go build できない: %v\n%s", err, out)
	}
	return exe
}

// TestSamples は tests/samples/ の各サンプルについて、四つを確かめる（tests/samples/README.md「テスト」）。
//
//   - check: サンプルの仕様が hozoc check を error なしで通る。
//   - shared: 期待する出力のうち共有する形（既定の出力）のファイルが、今の hozoc emit の出力とバイト単位で同じ。
//   - go: 期待する出力と人の手のコードが、サンプルのモジュールで go vet と go test を通る。
//   - transpilers: 本物のトランスパイラで emit した出力の全体が、期待する出力とバイト単位で同じ。
//     トランスパイラがまだなければ飛ばす。
func TestSamples(t *testing.T) {
	requireGo(t)
	manifests, err := filepath.Glob(filepath.Join(samplesDir, "*", sampleManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(manifests) == 0 {
		t.Fatalf("%s にサンプルがない", samplesDir)
	}
	pickExe := buildPick(t)
	for _, m := range manifests {
		name := filepath.Base(filepath.Dir(m))
		t.Run(name, func(t *testing.T) {
			s := loadSample(t, name)
			t.Run("check", func(t *testing.T) { s.testCheck(t, pickExe) })
			t.Run("shared", func(t *testing.T) { s.testShared(t, pickExe) })
			t.Run("go", s.testGo)
			t.Run("transpilers", func(t *testing.T) { s.testTranspilers(t, filepath.Join("..", "plugins")) })
		})
	}
}

// testCheck は、サンプルの仕様が hozoc check を error なしで通ること。本物の hozo.toml のまま、代役のトランスパイラで
// 確かめる（代役がなければ、単位のトランスパイラが見つからない CFG-14 になる）。warning は許し、表示する。
func (s *sampleProject) testCheck(t *testing.T, pickExe string) {
	root, env := s.standIn(t, pickExe)
	var stdout, stderr bytes.Buffer
	if code := hozocMainEnv(root, env, []string{"check"}, &stdout, &stderr); code != 0 {
		t.Fatalf("hozoc check の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
	}
	t.Logf("hozoc check:\n%s", stdout.String())
}

// testShared は、期待する出力のうち共有する形のファイルが古くなっていないこと。代役のトランスパイラ（sample.json の
// select の通りに選ぶだけの pick）で emit し、書き出されたファイルを、サンプルの同じパスのファイルとバイト単位で比べる。
// 単位の out の下のファイルは、既定の出力が書くか、sample.json がトランスパイラのファイルとして挙げるかのどちらか。
// 出力先ごとの記録（.hozoc-outputs.json）は、代役の記録にトランスパイラのファイルを足したものと比べる。
// -update なら、共有する形のファイルと記録をサンプルに書き直す（トランスパイラのファイルは人が書くので、触らない）。
func (s *sampleProject) testShared(t *testing.T, pickExe string) {
	root, env := s.standIn(t, pickExe)
	before := readTree(t, root)
	var stdout, stderr bytes.Buffer
	if code := hozocMainEnv(root, env, []string{"emit"}, &stdout, &stderr); code != 0 {
		t.Fatalf("hozoc emit の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
	}
	got := readTree(t, root)
	for p := range before {
		delete(got, p)
	}

	// 記録は、その出力のトランスパイラのファイルを足してから比べる。ファイルの持ち主の出力は、記録のあるディレクトリのうち
	// そのファイルを下に持つ最も深いもの（Dir(U, N)。docs/config.md「単位の対象」）
	records := map[string][]string{}
	for p, content := range got {
		if path.Base(p) != app.OutputsFile {
			continue
		}
		var paths []string
		if err := json.Unmarshal(content, &paths); err != nil {
			t.Fatalf("%s を読めない: %v", p, err)
		}
		records[path.Dir(p)] = paths
	}
	for f, name := range s.owner {
		if _, written := got[f]; written {
			t.Errorf("%s は %s のファイルのはずだが、既定の出力が書いた", f, name)
		}
		dir := ""
		for d := range records {
			if config.Under(f, d) && len(d) > len(dir) {
				dir = d
			}
		}
		if dir == "" {
			t.Errorf("%s のファイル %s を下に持つ出力がない（その単位は何も出力しなかった）", name, f)
			continue
		}
		records[dir] = append(records[dir], f)
	}
	for dir, paths := range records {
		sort.Strings(paths)
		data, err := json.MarshalIndent(paths, "", "  ") // docs/cli.md「emit」の 6: 辞書順の JSON の配列
		if err != nil {
			t.Fatal(err)
		}
		got[path.Join(dir, app.OutputsFile)] = append(data, '\n')
	}

	if *update {
		for p, content := range got {
			abs := filepath.Join(s.dir, filepath.FromSlash(p))
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(abs, content, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		for p := range s.expected(t) {
			if _, ok := got[p]; !ok && s.owner[p] == "" {
				if err := os.Remove(filepath.Join(s.dir, filepath.FromSlash(p))); err != nil {
					t.Fatal(err)
				}
			}
		}
	}

	want := s.expected(t)
	for _, p := range sortedKeys(want, got) {
		w, inWant := want[p]
		g, inGot := got[p]
		switch name := s.owner[p]; {
		case name != "" && !inWant:
			t.Errorf("%s/%s: %s が出すはずのファイルがない（%s）", s.dir, p, name, sampleManifestFile)
		case name != "":
			// トランスパイラのファイル。中身を確かめるのは testGo と testTranspilers
		case !inGot:
			t.Errorf("%s/%s は既定の出力が書かず、%s のトランスパイラのファイルにもない（-update で消える）", s.dir, p, sampleManifestFile)
		case !inWant:
			t.Errorf("%s/%s は既定の出力が書くが、サンプルにない（-update で作る）", s.dir, p)
		case !bytes.Equal(w, g):
			t.Errorf("%s/%s が今の既定の出力と違う（-update で更新）:\n%s", s.dir, p, firstDiff(string(w), string(g)))
		}
	}
}

// testGo は、サンプルが動くこと: サンプルのモジュール（リポジトリのモジュールとは別。自分の go.mod を持つ）で
// `go vet ./...` と `go test ./...` を通す。期待する出力がコンパイルでき、人の手のコードと合わせて仕様の通りに動く。
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

// testTranspilers は、トランスパイラのブラックボックスのテスト（docs/testing.md「トランスパイラのテスト」）:
// サンプルの入力を一時ディレクトリに写し、本物の hozo.toml のまま、pluginDir のプラグインのトランスパイラで emit して、
// 書き出されたファイルの木の全体（共有する形、トランスパイラのファイル、記録）が、サンプルの期待する出力と
// バイト単位で同じであることを確かめる。
//
// サンプルのトランスパイラのうち、pluginDir のどのプラグインも提供していないものがあれば飛ばす。
// そのトランスパイラを提供するプラグインを plugins/ に足せば、何も変えずに走るようになる。
// 期待する出力が仕様なので、-update では書き直さない。
func (s *sampleProject) testTranspilers(t *testing.T, pluginDir string) {
	provided := map[string]bool{}
	manifests, err := filepath.Glob(filepath.Join(pluginDir, "*", hostplugin.ManifestFileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range manifests {
		m, _ := hostplugin.LoadManifest(p)
		if m == nil {
			continue
		}
		for _, tr := range m.Transpilers {
			provided[tr.Name] = true
		}
	}
	var missing []string
	for _, name := range s.names() {
		if !provided[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Skipf("保留: トランスパイラ %s がまだない（%s のどのプラグインも提供していない）。"+
			"できたら、hozoc emit の出力の全体を %s の期待する出力とバイト単位で比べる",
			strings.Join(missing, "、"), filepath.ToSlash(filepath.Clean(pluginDir)), s.dir)
	}

	absPlugins, err := filepath.Abs(pluginDir)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "project")
	s.copyInputs(t, root)
	before := readTree(t, root)
	var stdout, stderr bytes.Buffer
	if code := hozocMainEnv(root, pluginPathEnv(absPlugins), []string{"emit"}, &stdout, &stderr); code != 0 {
		t.Fatalf("hozoc emit の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
	}
	got := readTree(t, root)
	for p := range before {
		delete(got, p)
	}
	want := s.expected(t)
	for _, p := range sortedKeys(want, got) {
		w, inWant := want[p]
		g, inGot := got[p]
		from := s.owner[p]
		if from == "" {
			from = "既定の出力"
		}
		switch {
		case !inGot:
			t.Errorf("%s/%s は期待する出力にあるが、emit は書かない（%s）", s.dir, p, from)
		case !inWant:
			t.Errorf("%s/%s は emit が書くが、期待する出力にない", s.dir, p)
		case !bytes.Equal(w, g):
			t.Errorf("%s/%s が期待する出力と違う（%s）:\n%s", s.dir, p, from, firstDiff(string(w), string(g)))
		}
	}
}
