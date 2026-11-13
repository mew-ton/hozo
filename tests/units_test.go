package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUnitsCorpus は tests/corpus/units（単位のコーパス。README.md）を実物の `hozoc emit` で展開し、hozo.toml の単位の一覧
// （docs/config.md「単位」「単位の対象」「in」）がそのとおりに出力になることを確かめる。トランスパイラは tests/plugins/pick の
// 四つ（一つのプラグインが提供する。要求の transpiler で規則が分かれる）。出力の全体は TestCorpusEmitGolden のゴールデンが、
// 書き出した Go がコンパイルできることは同じテストの go vet が確かめる。
func TestUnitsCorpus(t *testing.T) {
	requireGo(t)
	root, before := copyCorpusAsModule(t, "units")
	emit := func(t *testing.T, args ...string) (map[string][]byte, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if code := hozocMainIn(root, append([]string{"emit"}, args...), &stdout, &stderr); code != 0 {
			t.Fatalf("hozoc emit %v の終了コード %d\n%s%s", args, code, stdout.String(), stderr.String())
		}
		got := readTree(t, root)
		for p := range before {
			delete(got, p)
		}
		return got, stdout.String()
	}
	got, _ := emit(t)
	file := func(p string) string { return string(got[p]) }
	want := func(t *testing.T, name string, wants, nots []string) {
		t.Helper()
		content := file(name)
		if content == "" {
			t.Errorf("%s がない", name)
			return
		}
		for _, w := range wants {
			if !strings.Contains(content, w) {
				t.Errorf("%s に %q がない", name, w)
			}
		}
		for _, n := range nots {
			if strings.Contains(content, n) {
				t.Errorf("%s に %q がある", name, n)
			}
		}
	}

	t.Run("config.md 単位/同じ名前空間の二つの単位は、別のトランスパイラで別の out に独立に出力する", func(t *testing.T) {
		// 単位 2（pick-api、gen/api）: 起点は @t.api の open
		want(t, "gen/api/ops.go", []string{"type OpenFunc func(", "type SaveFunc func(", "type IssueFunc func("}, []string{"ReportFunc", "SweepFunc"})
		// 単位 3（pick-batch、gen/batch）: 起点は @t.batch の nightly と report。open は選んでいない
		want(t, "gen/batch/ops.go", []string{"type NightlyFunc func(", "type SweepFunc func(", "type ReportFunc func("}, []string{"OpenFunc", "IssueFunc"})
		// それぞれが自分の Item を持つ。同じ名前空間を出力する別の単位のパッケージは import しない
		want(t, "gen/api/types.go", []string{"type Item struct {", "type Receipt struct {"}, []string{"gen/batch", "type Report struct"})
		want(t, "gen/batch/types.go", []string{"type Item struct {", "type Report struct {"}, []string{"gen/api", "type Receipt struct"})
		for _, dir := range []string{"gen/stock", "gen/api", "gen/batch", "gen/admin"} {
			if file(dir+"/.hozoc-outputs.json") == "" {
				t.Errorf("%s/.hozoc-outputs.json がない（記録は出力ごと）", dir)
			}
		}
	})
	t.Run("config.md in と emit-go.md 出力の対象 2/in の外の op は口だけを出す", func(t *testing.T) {
		// nightly は shop/batch.hozo の配線。単位 2 の in は shop/api.hozo だけなので、open から辿られて選ばれるが、
		// 本体は出さず Leaves の口になる。本体が呼ぶ sweep は選ばれない
		want(t, "gen/api/ops.go", []string{"type NightlyFunc func(item Item) error", "return o.leaves.Nightly(item)"}, []string{"Sweep"})
		want(t, "gen/api/wiring.go", []string{"func (o *ops) Open(item Item) (Receipt, error) {", "o.Nightly(item)"}, []string{"func (o *ops) Nightly("})
		// 単位 3 は in を省いているので、同じ nightly の本体を出す
		want(t, "gen/batch/wiring.go", []string{"func (o *ops) Nightly(item Item) error {", "o.Sweep(item)"}, nil)
		// pick-api は @t.batch も読む（pick.json）が、SDK の plugin.OpsWith は単位の対象の op だけを返すので、
		// in の外の nightly と report は起点にならない（PLG-3 にもならない）
		want(t, "gen/api/ops.go", nil, []string{"ReportFunc"})
	})
	t.Run("emit-go.md 出力の対象/選ばれた宣言が参照する型は in の外でも出力する", func(t *testing.T) {
		// 単位 3 の nightly（batch.hozo）の引数の型 Item は api.hozo の宣言。in を絞った単位でも同じ規則（app の TestEmitIn）
		want(t, "gen/batch/types.go", []string{"type Item struct {"}, nil)
	})
	t.Run("config.md 単位の対象/別の単位が名指しした子孫は外れる", func(t *testing.T) {
		want(t, "gen/admin/ops.go", []string{"package admin", "type BanFunc func("}, nil)
		for p := range got {
			if strings.HasPrefix(p, "gen/api/admin/") || strings.HasPrefix(p, "gen/batch/admin/") {
				t.Errorf("祖先の単位が shop.admin を出力した: %s", p)
			}
		}
	})
	t.Run("emit-go.md 配置/単位をまたぐ import は一方向で、設定の順で最初にその型を持つ単位から", func(t *testing.T) {
		// shop（gen/api、gen/batch）→ stock（gen/stock）: Item.id の型 stock.ItemId
		want(t, "gen/api/types.go", []string{`"github.com/mew-ton/hozo/gen/stock"`, "stock.ItemId"}, []string{"type ItemId string"})
		want(t, "gen/batch/types.go", []string{`"github.com/mew-ton/hozo/gen/stock"`}, nil)
		// shop.admin（gen/admin）→ shop: shop を出力する単位は 2（gen/api）と 3（gen/batch）で、どちらの出力も Item を持つ。
		// 設定の順で最初の 2 から import する
		want(t, "gen/admin/ops.go", []string{`"github.com/mew-ton/hozo/gen/api"`, "item shop.Item"}, []string{"gen/batch"})
		// stock は誰も import しない
		for p, content := range got {
			if strings.HasPrefix(p, "gen/stock/") && (strings.Contains(string(content), "gen/api") || strings.Contains(string(content), "gen/batch")) {
				t.Errorf("%s が shop の出力を import している", p)
			}
		}
	})
	t.Run("cli.md emit 1/引数は名前空間で、それを出力するすべての単位の分を書き出す", func(t *testing.T) {
		full := got
		if err := os.RemoveAll(filepath.Join(root, "gen")); err != nil {
			t.Fatal(err)
		}
		// shop: shop を出力する単位 2 と 3、子孫 shop.admin を出力する単位 4。stock は書かない
		only, stdout := emit(t, "shop")
		for p, content := range full {
			switch {
			case strings.HasPrefix(p, "gen/stock/"):
				if _, wrote := only[p]; wrote {
					t.Errorf("対象でない %s を書いた", p)
				}
			case !bytes.Equal(only[p], content):
				t.Errorf("%s が、すべてを対象にしたときと違う", p)
			}
		}
		if strings.Contains(stdout, "gen/stock/") {
			t.Errorf("stdout に gen/stock がある:\n%s", stdout)
		}
		// どの単位の対象でもない名前空間（語彙の t）は UnknownNamespace（64）
		var out, errOut bytes.Buffer
		if code := hozocMainIn(root, []string{"emit", "t"}, &out, &errOut); code != 64 || !strings.Contains(errOut.String(), "どの単位の対象でもない") {
			t.Errorf("emit t: code=%d stderr=%q", code, errOut.String())
		}
		emit(t) // 以降のサブテストのために、すべてを書き直す
	})
	t.Run("plugin-protocol.md 選択/単位の対象でない宣言を選べば PLG-3", func(t *testing.T) {
		// pick の fqnRoots は FQN を確かめずに roots に入れる。report は batch.hozo の宣言で、単位 2 の in の外
		rules := filepath.Join(root, "plugins", "pick", "pick.json")
		orig, err := os.ReadFile(rules)
		if err != nil {
			t.Fatal(err)
		}
		bad := strings.Replace(string(orig), `"pick-api": { "roots": ["t.api", "t.batch"] }`, `"pick-api": { "roots": ["t.api"], "fqnRoots": ["shop.report"] }`, 1)
		if bad == string(orig) {
			t.Fatal("pick.json の pick-api の規則を書き換えられない")
		}
		if err := os.WriteFile(rules, []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		code := hozocMainIn(root, []string{"emit"}, &stdout, &stderr)
		if code != 70 || stdout.String() != "" || !strings.Contains(stderr.String(), "PLG-3") ||
			!strings.Contains(stderr.String(), `roots の "shop.report" は単位の対象でない（in の外の shop/batch.hozo で宣言されている）`) {
			t.Errorf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
		// 同じ FQN でも、in を省いた単位 3（pick-batch）なら選べる
		ok := strings.Replace(string(orig), `"pick-batch": { "roots": ["t.batch"] }`, `"pick-batch": { "fqnRoots": ["shop.report", "shop.nightly"] }`, 1)
		if err := os.WriteFile(rules, []byte(ok), 0o644); err != nil {
			t.Fatal(err)
		}
		again, _ := emit(t)
		if !bytes.Equal(again["gen/batch/ops.go"], got["gen/batch/ops.go"]) {
			t.Errorf("FQN で選んだ gen/batch/ops.go が、デコレータで選んだときと違う")
		}
	})
}
