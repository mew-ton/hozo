package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUnitsCorpus は tests/corpus/units（単位のコーパス。README.md）を実物の `hozoc build` で展開し、hozo.toml の単位の一覧
// （docs/config.md「単位」「単位の対象」「in」「型の置き場」）がそのとおりに出力になることを確かめる。トランスパイラは、四つの単位とも、
// リポジトリの plugins/ の基底 go。出力の全体は TestCorpusEmitGolden のゴールデンが、書き出した Go がコンパイルできることは
// 同じテストの go vet が確かめる。
func TestUnitsCorpus(t *testing.T) {
	requireGo(t)
	root, before := copyCorpusAsModule(t, "units")
	emit := func(t *testing.T, args ...string) (map[string][]byte, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if code := hozocCorpus(t, root, append([]string{"build"}, args...), &stdout, &stderr); code != 0 {
			t.Fatalf("hozoc build %v の終了コード %d\n%s%s", args, code, stdout.String(), stderr.String())
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

	t.Run("config.md 単位/同じ名前空間の二つの単位は、別の out に独立に出力する", func(t *testing.T) {
		// 単位 2（gen/api）と単位 3（gen/batch）は、どちらも shop。shop は関数を持つので、型は、型のパッケージ types
		// （docs/plugin-go.md「型のパッケージ」）。内部の op も持つので、定義は内側のフォルダ inner（同「内側のフォルダと入口」）で、
		// 内側は、型のパッケージを、入口は、内側を、再公開する。それぞれが自分の型をすべて持ち、同じ名前空間を出力する別の単位の
		// パッケージは import しない
		for _, dir := range []string{"gen/api", "gen/batch"} {
			other := map[string]string{"gen/api": "gen/batch", "gen/batch": "gen/api"}[dir]
			want(t, dir+"/types/types.go", []string{"package types", "type Item struct {", "type Receipt struct {", "type Report struct {"}, []string{"gen/api", "gen/batch"})
			want(t, dir+"/inner/types.go", []string{"type Item = types.Item", "type Report = types.Report", dir + "/types\""}, []string{other})
			want(t, dir+"/types.go", []string{"type Item = inner.Item", "type Report = inner.Report"}, nil)
		}
		for _, dir := range []string{"gen/stock", "gen/api", "gen/batch", "gen/admin"} {
			if file(dir+"/.hozoc-outputs.json") == "" {
				t.Errorf("%s/.hozoc-outputs.json がない（記録は出力ごと）", dir)
			}
		}
	})
	t.Run("config.md in と plugin-go.md 出力の対象/in が絞るのは op だけ", func(t *testing.T) {
		// 単位 2 の in は shop/api.hozo だけ。型（Report は batch.hozo の宣言）と状態は、in によらずすべて出る（上のサブテスト）。
		// op は、単位の対象のもの（open、save、issue）と、単位の対象の配線 open が呼んでいる nightly（batch.hozo の配線）。
		// nightly は、本体を出さず、人が実装する葉（Leaves の口）として出す。nightly が呼ぶ sweep と、呼ばれていない report は出ない
		want(t, "gen/api/inner/ops.go", []string{"\tNightly(ctx context.Context, item Item) error", "return o.leaves.Nightly(ctx, item)"}, []string{"Sweep", "Report"})
		want(t, "gen/api/inner/wiring.go", []string{"func (o *ops) Open(ctx context.Context, item Item) (Receipt, error) {", "o.Nightly(ctx, item)"}, []string{"func (o *ops) Nightly("})
		// 単位 3 は in を省いているので、同じ nightly の本体を出し、sweep（純粋で throws のない葉。それでも、ctx を取り、error を返す:
		// docs/plugin-go.md「op の形」）と report も出す
		want(t, "gen/batch/inner/wiring.go", []string{"func (o *ops) Nightly(ctx context.Context, item Item) error {", "o.Sweep(ctx, item)"}, nil)
		want(t, "gen/batch/inner/ops.go", []string{"\tSweep(ctx context.Context, item Item) error\n", "\tReport(ctx context.Context) (Report, error)"}, nil)
	})
	t.Run("config.md 単位の対象/別の単位が名指しした子孫は外れる", func(t *testing.T) {
		want(t, "gen/admin/inner/ops.go", []string{"package inner", "\tBan(ctx context.Context, item shop.Item) error"}, nil)
		want(t, "gen/admin/doc.go", []string{"package admin"}, nil)
		for p := range got {
			if strings.HasPrefix(p, "gen/api/admin/") || strings.HasPrefix(p, "gen/batch/admin/") {
				t.Errorf("祖先の単位が shop.admin を出力した: %s", p)
			}
		}
	})
	t.Run("config.md 型の置き場/単位をまたぐ import は一方向で、置き場は設定の順で最初の単位", func(t *testing.T) {
		// shop（gen/api、gen/batch）→ stock（gen/stock）: Item.id の型 stock.ItemId。複製しない
		// stock も、関数を持つので、import するのは、その、型のパッケージ
		want(t, "gen/api/types/types.go", []string{`stock "github.com/mew-ton/hozo/gen/stock/types"`, "stock.ItemId"}, []string{"type ItemId string", `"github.com/mew-ton/hozo/gen/stock"`})
		want(t, "gen/batch/types/types.go", []string{`stock "github.com/mew-ton/hozo/gen/stock/types"`}, []string{"type ItemId string"})
		// shop.admin（gen/admin）→ shop: shop を出力する単位は 2（gen/api）と 3（gen/batch）。設定の順で最初の 2 の、型のパッケージを
		// import する（入口も、内側も、import しない）
		want(t, "gen/admin/inner/ops.go", []string{`shop "github.com/mew-ton/hozo/gen/api/types"`, "item shop.Item"}, []string{"gen/batch", "gen/api/inner", `"github.com/mew-ton/hozo/gen/api"`})
		// stock は誰も import しない
		for p, content := range got {
			if strings.HasPrefix(p, "gen/stock/") && (strings.Contains(string(content), "gen/api") || strings.Contains(string(content), "gen/batch")) {
				t.Errorf("%s が shop の出力を import している", p)
			}
		}
	})
	t.Run("plugin-go.md 内側のフォルダと入口/内部の op を持たない出力は分けない", func(t *testing.T) {
		// stock の op は reserve だけで、export。一つのパッケージがすべての定義を持つ
		want(t, "gen/stock/ops.go", []string{"package stock", "\tReserve(ctx context.Context, id ItemId) error"}, nil)
		for p := range got {
			if strings.HasPrefix(p, "gen/stock/inner/") {
				t.Errorf("内部の op を持たない出力に、内側のフォルダがある: %s", p)
			}
		}
		// 入口の Ops は export の op だけ。shop の op はどれも内部なので、入口の Ops は空
		want(t, "gen/api/ops.go", []string{"type Ops interface{}", "return inner.New(leaves, items, stock)"}, nil)
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
		if code := hozocCorpus(t, root, []string{"build", "t"}, &out, &errOut); code != 64 || !strings.Contains(errOut.String(), "どの単位の対象でもない") {
			t.Errorf("emit t: code=%d stderr=%q", code, errOut.String())
		}
	})
}
