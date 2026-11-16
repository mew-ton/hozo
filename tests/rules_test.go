package tests

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/internal/vocab"
)

// このファイルは docs/testing.md「規則ごとのテスト」をリポジトリの段で行う。
//
// tests/rules/<ID>/ に、規則 ID が通る例 ok_* と落ちる例 ng_* を置く。例は一つの .hozo（ok_1.hozo）か、
// 複数のファイルが要るときはディレクトリ（ng_2/ の中に .hozo と、あれば hozo.toml）。例ごとに一時ディレクトリに
// プロジェクトを組み、internal/project.Load（解析・解決・検査・設定・プラグインの探索まで全部）に通し、
// 診断の集合を「ファイル、行、重さ、ID」の組で注釈と比べる。期待にない診断、出なかった期待はどちらも失敗。
// ok の例は注釈を持たず、診断が一つも出ないこと（その規則の診断だけでなく、ほかの診断も失敗）。
//
// 例ごとに一つのプロジェクトにするのは、ok と ng を同じプロジェクトに入れると宣言の名前が衝突する（RES-6）ためと、
// hozo.toml の側の期待（CFG-*）が例ごとに違うため。
//
// 設定は、例の隣の <名前>.toml（ng_1.hozo なら ng_1.toml）、なければ規則のディレクトリの hozo.toml、なければ
// tests/rules/hozo.toml（既定: 名前空間 a を、トランスパイラ go で gen/a に出力する単位が一つ）。ディレクトリの例は中の
// hozo.toml が最優先。プラグインの探索のための fixture は二段: tests/rules/plugins/（既定。トランスパイラ go / go-b / ts を
// 提供する std）と、規則のディレクトリの plugins/（その規則のためのプラグイン）。どちらも各例のプロジェクトの plugins/ に写す
// （[hozo].plugin_dirs = ["plugins"] と組み合わせて CFG-6 / CFG-11 / CFG-14〜CFG-16 / NS-3 を起こす）。
//
// 注釈の形は internal/sema/annot_test.go と同じ: `// ERROR: A, B`、`// WARN: A`、`// ERROR(-1): A`（相対の行）。
// 加えて `// ERROR(hozo:vocab/localstorage.hozo:2): NS-3` で別のファイルの行を指せる（埋め込まれた語彙に出る診断のため）。
// hozo.toml の診断は位置（行）を持たないので、hozo.toml の中では `# ERROR: CFG-12` と書き、ファイル hozo.toml の
// 行 0 の期待にする。
//
// 規則のディレクトリに skip.txt があれば、その内容を理由にして飛ばす（実装か文書の誤りを見つけた例を、直るまで
// 残しておくため。中身は理由と TODO(bug)）。ディレクトリの名前は <ID> か <ID>.<補足>（THROWS-1.external）で、
// 補足は同じ規則の例を分けて skip できるようにするためのもの。

const rulesDir = "rules"

// annotRE は `// ERROR: A, B` / `// WARN: A` / `// ERROR(-1): A`（相対の行）/ `// ERROR(hozo:vocab/localstorage.hozo:2): A`
// （別のファイルの行。注釈を書けない語彙のファイルに出る診断のため）。hozo.toml では `#` で始める。
var annotRE = regexp.MustCompile(`(?://|#)\s*(ERROR|WARN)(?:\(([+-]\d+)\)|\(([^()\s]+):(\d+)\))?:\s*([A-Z0-9-]+(?:\s*,\s*[A-Z0-9-]+)*)`)

// expectation は注釈一つ分の期待: ファイル（ルートからの相対パス）、行（hozo.toml は 0）、重さ、ID。
type expectation struct {
	file string
	line int
	sev  diag.Severity
	code string
}

func (e expectation) String() string {
	return fmt.Sprintf("%s:%d %s[%s]", e.file, e.line, e.sev, e.code)
}

// sample は一つの例。root は一時ディレクトリに組んだプロジェクトのルート、files はルートからの相対パス → 中身
// （注釈を読む対象: .hozo と hozo.toml）。
type sample struct {
	name  string
	root  string
	files map[string]string
}

// expectations は注釈を読む。hozo.toml（`#` のコメント）の期待は行 0。
func (s *sample) expectations() map[expectation]bool {
	out := map[expectation]bool{}
	for path, content := range s.files {
		isToml := strings.HasSuffix(path, ".toml")
		for i, line := range strings.Split(content, "\n") {
			for _, m := range annotRE.FindAllStringSubmatch(line, -1) {
				sev := diag.Error
				if m[1] == "WARN" {
					sev = diag.Warning
				}
				file, ln := path, i+1
				switch {
				case m[2] != "":
					offset, _ := strconv.Atoi(m[2])
					ln += offset
				case m[3] != "":
					file = m[3]
					ln, _ = strconv.Atoi(m[4])
				}
				if isToml {
					ln = 0
				}
				for _, code := range strings.Split(m[5], ",") {
					out[expectation{file: file, line: ln, sev: sev, code: strings.TrimSpace(code)}] = true
				}
			}
		}
	}
	return out
}

// actual は診断を期待の形にする。Span のない診断（hozo.toml のもの）は File と行 0。
func actual(p *project.Project) map[expectation]bool {
	out := map[expectation]bool{}
	for _, d := range p.Diags {
		e := expectation{file: d.File, sev: d.Severity, code: d.Code}
		if d.Span != nil {
			e.file = p.Files.Path(d.Span.File)
			e.line = p.Files.Position(d.Span.File, d.Span.Start).Line
		}
		out[e] = true
	}
	return out
}

// copyTree は src の木を dst に写す（dst はあるものとする）。
func copyTree(t testing.TB, src, dst string) {
	t.Helper()
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
}

// readTree は dir の下の .hozo と hozo.toml を、dir からの相対パス（"/" 区切り）→ 中身で返す。
// plugins/ の下は読まない（プラグインの語彙やマニフェストには注釈を書かない）。
func readSampleTree(t testing.TB, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(rel) != ".hozo" && rel != config.FileName {
			return nil
		}
		key := rel
		if parts := strings.Split(rel, "/"); len(parts) >= 3 && parts[0] == "plugins" {
			// プラグインの語彙（plugins/<名前>/<相対パス>.hozo）は hozo:plugin/<名前>/<相対パス> で読まれる
			key = vocab.PluginPath(parts[1], strings.Join(parts[2:], "/"))
		} else if strings.Contains(rel, "/") && filepath.Base(rel) == config.FileName {
			return nil // プラグインの下などの hozo.toml は設定ではない
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[key] = string(content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// buildSample は規則のディレクトリ ruleDir の例 entry（ファイルかディレクトリ）から一時ディレクトリにプロジェクトを組む。
func buildSample(t testing.TB, ruleDir string, entry os.DirEntry) *sample {
	t.Helper()
	root := t.TempDir()
	name := strings.TrimSuffix(entry.Name(), ".hozo")
	src := filepath.Join(ruleDir, entry.Name())

	// プラグインの fixture: 既定のもの（tests/rules/plugins）と、規則のディレクトリのもの
	if err := os.Mkdir(filepath.Join(root, "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(rulesDir, "plugins"), filepath.Join(ruleDir, "plugins")} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			copyTree(t, dir, filepath.Join(root, "plugins"))
		}
	}

	// 例の本体
	if entry.IsDir() {
		copyTree(t, src, root)
	} else {
		content, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, entry.Name()), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// 設定: 例の hozo.toml（ディレクトリの例）→ <名前>.toml → 規則のディレクトリの hozo.toml → 既定
	if _, err := os.Stat(filepath.Join(root, config.FileName)); err != nil {
		candidates := []string{
			filepath.Join(ruleDir, name+".toml"),
			filepath.Join(ruleDir, config.FileName),
			filepath.Join(rulesDir, config.FileName),
		}
		for _, c := range candidates {
			content, err := os.ReadFile(c)
			if err != nil {
				continue
			}
			if err := os.WriteFile(filepath.Join(root, config.FileName), content, 0o644); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	return &sample{name: name, root: root, files: readSampleTree(t, root)}
}

// ruleID はディレクトリの名前から規則の ID（<ID> か <ID>.<補足> の ID の部分）を返す。
func ruleID(dirName string) string {
	if i := strings.IndexByte(dirName, '.'); i >= 0 {
		return dirName[:i]
	}
	return dirName
}

// grammarClosed は「文法で閉じる」規則。例は構文エラー（SYNTAX）で確かめる（docs/testing.md「規則ごとのテスト」）。
var grammarClosed = map[string]bool{
	"SYN-1": true, "SYN-2": true, "DEC-3": true, "DEC-8": true, "DEC-15": true,
	"WIRE-1": true, "WIRE-5": true, "MATCH-1": true, "MATCH-3": true,
}

// reportedAs は、規則の診断が別の ID で出るもの（docs/check.md「診断」の表の「診断は RES-1 として出る」など）。
var reportedAs = map[string]string{
	"NS-1": "RES-1", // 解決の手続きそのもの
}

// codeFor は規則 ID の ng の例に少なくとも一つは現れるべき診断の ID。
func codeFor(id string) string {
	if grammarClosed[id] {
		return "SYNTAX"
	}
	if alt, ok := reportedAs[id]; ok {
		return alt
	}
	return id
}

// runSample は例を project.Load に通し、注釈と比べる。ng なら期待の集合と診断の集合が等しいこと。
// ok なら注釈がなく、診断も一つもないこと。code は ng の例に現れるべき ID で、現れたら true を返す。
func runSample(t *testing.T, s *sample, ok bool, code string) bool {
	t.Helper()
	p := project.Load(s.root, "", nil)
	if p.RunErr != nil {
		t.Fatalf("プラグインの check が失敗: %v", p.RunErr)
	}
	want := s.expectations()
	got := actual(p)
	var problems []string
	if ok {
		for e := range want {
			problems = append(problems, "ok の例に注釈がある: "+e.String())
		}
		want = map[expectation]bool{}
	} else if len(want) == 0 {
		problems = append(problems, "ng の例に注釈がない")
	}
	for e := range want {
		if !got[e] {
			problems = append(problems, "出なかった期待: "+e.String())
		}
	}
	for e := range got {
		if !want[e] {
			problems = append(problems, "期待にない診断: "+e.String())
		}
	}
	seen := false
	for e := range want {
		if e.code == code {
			seen = true
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		var buf bytes.Buffer
		_ = diag.WriteText(&buf, p.Diags, p.Files)
		t.Errorf("%s\n診断:\n%s", strings.Join(problems, "\n"), buf.String())
	}
	return seen
}

// TestRules は tests/rules/<ID>/ の各例を全パイプライン（project.Load）に通して注釈と比べる。
func TestRules(t *testing.T) {
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "plugins" { // plugins/ は既定の fixture
			continue
		}
		e := e
		t.Run(e.Name(), func(t *testing.T) {
			ruleDir := filepath.Join(rulesDir, e.Name())
			if reason, err := os.ReadFile(filepath.Join(ruleDir, "skip.txt")); err == nil {
				t.Skipf("skip.txt: %s", strings.TrimSpace(string(reason)))
			}
			id := ruleID(e.Name())
			code := codeFor(id)
			samples, err := os.ReadDir(ruleDir)
			if err != nil {
				t.Fatal(err)
			}
			nOK, nNG := 0, 0
			seen := false
			for _, s := range samples {
				name := s.Name()
				var ok bool
				switch {
				case strings.HasPrefix(name, "ok_"):
					ok = true
				case strings.HasPrefix(name, "ng_"):
				default:
					continue // hozo.toml、<名前>.toml、plugins/、README など
				}
				if !s.IsDir() && filepath.Ext(name) != ".hozo" {
					continue
				}
				if ok {
					nOK++
				} else {
					nNG++
				}
				s := s
				t.Run(strings.TrimSuffix(name, ".hozo"), func(t *testing.T) {
					if runSample(t, buildSample(t, ruleDir, s), ok, code) {
						seen = true
					}
				})
			}
			if nOK == 0 || nNG == 0 {
				t.Errorf("%s には ok_* と ng_* が少なくとも一つずつ要る（ok %d、ng %d）", e.Name(), nOK, nNG)
			}
			if nNG > 0 && !seen {
				t.Errorf("%s の ng の例のどれにも %s の注釈がない", e.Name(), code)
			}
		})
	}
}

// ruleDocs は規則の ID を持つ設計文書（docs/testing.md「規則ごとのテスト」の対象）。
var ruleDocs = []string{"check.md", "resolve.md", "types.md", "config.md"}

// idRowRE は「診断」の表の行の先頭の列（`| THROWS-1 | error | ... |`）。
var idRowRE = regexp.MustCompile(`^\|\s*([A-Z]+-\d+)\s*\|`)

// notTestable は .hozo と hozo.toml だけでは起こせない、または診断を持たない規則と、その理由。
// docs/testing.md「規則ごとのテスト」の「文法で閉じている規則（— のもの）は … SYNTAX で確かめる」に従い、
// 文法で閉じる規則はここに入れず、SYNTAX の例を置く。
var notTestable = map[string]string{
	"NS-2":     "正規化で行う。診断なし（docs/check.md「名前空間」）",
	"EFF-1":    "計算（Sig の F）で行う。診断なし（docs/check.md「状態と効果」）",
	"THROWS-2": "診断なし。多めの宣言は許す（docs/check.md「throws」）",
	"DEC-9":    "正規化（docs/ir.md「デコレータの適用」）。診断なし",
	"DEC-10":   "計算（手順 i の throws）。診断なし。DEC-10 の意味は THROWS-1 / DOC-1 の external の例で確かめる",
	"BIND-3":   "計算（Call.bound）。診断なし。BIND-4 の ok の例で確かめる",
	"CFG-13":   "対象の手続き。診断なし（docs/config.md「単位の対象」）。CFG-10 の ok の例（子孫は対象、対象でない名前空間は診断なし）と CFG-17 の例、internal/config の TestCovering で確かめる",
	"TYPE-2":   "DEC-1 で検出される。ID は索引として残す（docs/types.md「診断」）",
	"MATCH-5":  "WIRE-2 と同じ条件。ID は索引として残す（docs/check.md「classify / match」）",
	"CFG-5":    "書き出すファイルの衝突は emit でしか起きない（.hozo と hozo.toml だけでは出ない。internal/app のテスト）",
}

// tableIDs は文書 path の表の先頭の列にある規則の ID を集める。
func tableIDs(t *testing.T, path string) map[string]bool {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, line := range strings.Split(string(content), "\n") {
		if m := idRowRE.FindStringSubmatch(line); m != nil {
			ids[m[1]] = true
		}
	}
	return ids
}

// docRuleIDs は設計文書の「診断」の表から規則の ID を集める（ID → 文書の名前）。
func docRuleIDs(t *testing.T) map[string]string {
	t.Helper()
	ids := map[string]string{}
	for _, name := range ruleDocs {
		for id := range tableIDs(t, filepath.Join("..", "docs", name)) {
			if _, dup := ids[id]; !dup {
				ids[id] = name
			}
		}
	}
	return ids
}

// TestRulesCoverDocs は、設計文書の「診断」の表のすべての ID に tests/rules/<ID>/ があることを確かめる。
// notTestable の ID は除く（理由を持つ）。逆に、tests/rules のディレクトリの ID は表のどれかにあること。
func TestRulesCoverDocs(t *testing.T) {
	ids := docRuleIDs(t)
	if len(ids) == 0 {
		t.Fatal("設計文書から規則の ID を一つも拾えなかった")
	}
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		t.Fatal(err)
	}
	dirs := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() && e.Name() != "plugins" {
			dirs[ruleID(e.Name())] = true
			if _, exact := os.Stat(filepath.Join(rulesDir, ruleID(e.Name()))); exact != nil {
				t.Errorf("tests/rules/%s があるのに tests/rules/%s がない（補足付きのディレクトリは本体の補助）", e.Name(), ruleID(e.Name()))
			}
		}
	}
	var missing []string
	for id, doc := range ids {
		if _, excluded := notTestable[id]; excluded {
			if dirs[id] {
				t.Errorf("%s は notTestable にあるのに tests/rules/%s がある。どちらかを直す", id, id)
			}
			continue
		}
		if !dirs[id] {
			missing = append(missing, id+"（"+doc+"）")
		}
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("設計文書の規則 %s に tests/rules のディレクトリがない", m)
	}
	for id := range notTestable {
		if _, ok := ids[id]; !ok {
			t.Errorf("notTestable の %s は設計文書の表にない", id)
		}
	}
	// 逆向き: ディレクトリの ID は四つの文書か、docs/requirements.md「検査の規則の一覧」（SYN-1 / SYN-2 はそこにだけある）の表にあること
	known := map[string]bool{"SYNTAX": true} // 構文エラーと、誤りのあるファイルでも残りを検査すること（docs/check.md「この文書が決めること」）
	for id := range ids {
		known[id] = true
	}
	for id := range tableIDs(t, filepath.Join("..", "docs", "requirements.md")) {
		known[id] = true
	}
	for id := range dirs {
		if !known[id] {
			t.Errorf("tests/rules/%s は設計文書の表にない規則", id)
		}
	}
}
