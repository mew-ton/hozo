package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestSelectCorpus は tests/corpus/select（出力の対象のコーパス。README.md）を実物の `hozoc emit` で展開し、
// 既定の出力が、プラグインの選んだ宣言だけを出していることを確かめる（docs/emit-go.md「出力の対象」、
// docs/plugin-protocol.md「選択」）。プラグイン pick（tests/plugins/pick）は、テストの語彙 t のデコレータで選ぶ:
// roots は @t.entry、stops は @t.stop、states は @t.store、wraps は @t.cache。
// サブテストの名前は docs/emit-go.md の見出しと手順の番号。出力の全体は TestCorpusEmitGolden のゴールデンが確かめる。
func TestSelectCorpus(t *testing.T) {
	requireGo(t)
	root, before := copyCorpusAsModule(t, "select")
	var stdout, stderr bytes.Buffer
	if code := hozocMainIn(root, []string{"emit"}, &stdout, &stderr); code != 0 {
		t.Fatalf("hozoc emit の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
	}
	got := readTree(t, root)
	for p := range before {
		delete(got, p)
	}
	file := func(name string) string { return string(got["gen/shop/"+name]) }
	ops, states, wiring, types := file("ops.go"), file("states.go"), file("wiring.go"), file("types.go")
	// has は content に s があるか。`re:` で始まれば正規表現（gofmt が桁をそろえる構造体のフィールド）。
	has := func(content, s string) bool {
		if re, ok := strings.CutPrefix(s, "re:"); ok {
			return regexp.MustCompile(re).MatchString(content)
		}
		return strings.Contains(content, s)
	}
	want := func(t *testing.T, name, content string, wants, nots []string) {
		t.Helper()
		for _, w := range wants {
			if !has(content, w) {
				t.Errorf("%s に %q がない", name, w)
			}
		}
		for _, n := range nots {
			if has(content, n) {
				t.Errorf("%s に %q がある", name, n)
			}
		}
	}
	field := func(name, typ string) string {
		return `re:(?m)^\t` + regexp.QuoteMeta(name) + ` +` + regexp.QuoteMeta(typ) + `$`
	}

	t.Run("出力の対象 1〜2: 起点から配線の呼び出しを辿る。選ばれなかった op は出力しない", func(t *testing.T) {
		// 起点は @t.entry の checkout と openCart。辿って選ばれるのは placeOrder、notify、issue、showEmpty、showCart
		want(t, "ops.go", ops,
			[]string{"\tCheckout() (Receipt, error)\n", "\tPlaceOrder() (Order, error)\n", "\tNotify(order Order) error\n", "\tIssue(order Order) (Receipt, error)\n",
				"\tOpenCart() error\n", "\tShowEmpty() error\n", "\tShowCart() error\n"},
			// どこからも辿られない saveForLater と addToCart（@t.note が付いていても、印では選ばれない）
			[]string{"SaveForLater", "AddToCart"})
		want(t, "wiring.go", wiring, []string{"func (o *ops) Checkout() (Receipt, error) {", "func (o *ops) OpenCart() error {"}, nil)
	})
	t.Run("出力の対象 2: stops の op は口だけを出し、本体は辿らない", func(t *testing.T) {
		// notify は配線だが、Leaves のフィールド（葉と同じ扱い）。本体が呼ぶ sendMail と logSent は選ばれない
		want(t, "ops.go", ops,
			[]string{"type NotifyFunc func(order Order) error", field("Notify", "NotifyFunc"), "return o.leaves.Notify(order)"},
			[]string{"SendMail", "LogSent"})
		want(t, "wiring.go", wiring, []string{"if err := o.Notify(order); err != nil {"}, []string{"func (o *ops) Notify(", "SendMail", "LogSent"})
	})
	t.Run("デコレータ: wraps で引き受けた適用だけが包む関数になり、印は何も生成しない", func(t *testing.T) {
		want(t, "ops.go", ops,
			[]string{
				"type IssueTCacheWrapper func(order Order, next IssueFunc) (Receipt, error)",
				"type TCacheWrappers struct {\n\tIssue IssueTCacheWrapper\n}",
				"type Wrappers struct {\n\tTCache TCacheWrappers\n}",
				"return o.wrappers.TCache.Issue(order, f0)",
				"func New(leaves Leaves, deps Deps, wrappers Wrappers) Ops {",
			},
			// @t.note（checkout、issue）と @t.entry / @t.stop は、どのプラグインも包む関数を引き受けていない
			[]string{"Note", "TEntry", "TStop"})
	})
	t.Run("出力の対象 3 と state: 状態は、葉の効果の宛先、配線の match、states から選ばれる", func(t *testing.T) {
		want(t, "states.go", states,
			[]string{
				// carts: openCart が match で読む
				"type CartsState struct {\n\t// 問い合わせ: 組み込みの分類。空（Empty）なら true。HasItems はその否定。\n\tIsEmpty func() (bool, error)\n}",
				// orders: 葉の placeOrder の効果 createOrder（on orders.post）の宛先。placeOrder はその命令
				field("PlaceOrder", "PlaceOrderFunc"),
				// audits: @t.store（select の states）。どの op も触れない
				"type AuditsState struct {",
				"type StateSet struct {\n\tCarts  CartsState\n\tOrders OrdersState\n\tAudits AuditsState\n}",
				"var States StateSet\n",
			},
			[]string{"Drafts"}) // どこからも選ばれない状態
		// 効果を持つ葉は States の命令で、Leaves には載らない
		want(t, "ops.go", ops, []string{"return States.Orders.PlaceOrder()"}, []string{field("PlaceOrder", "PlaceOrderFunc")})
		want(t, "wiring.go", wiring, []string{"empty, err := States.Carts.IsEmpty()"}, nil)
	})
	t.Run("出力の対象 5: 別の名前空間の op は Deps に載り、辿らない", func(t *testing.T) {
		want(t, "ops.go", ops,
			[]string{"type Deps struct {\n\tStock StockOps\n}", "type StockOps struct {\n\tReserve StockReserveFunc\n}", "type StockReserveFunc func(order Order) error"},
			[]string{"Count", "Keep"}) // stock.reserve の本体が呼ぶ op
		want(t, "wiring.go", wiring, []string{"if err := o.deps.Stock.Reserve(order); err != nil {"}, nil)
	})
	t.Run("出力の対象 6: 型は選ばれた op と状態が参照するものだけ。効果の定数は選ばれた op のもの", func(t *testing.T) {
		want(t, "types.go", types,
			[]string{"type Line struct {", "type Order struct {", "type Receipt struct {", "type Audit struct {", "const EffectCreateOrder "},
			[]string{"Draft", "EffectAddLine", "EffectSaveDraft"})
	})
	t.Run("配置: 出力がその型を持たない Go の名前空間の型は複製する", func(t *testing.T) {
		// stock は Go の単位が出力する名前空間だが、その単位のトランスパイラが何も選ばず、出力がその型を持たないので、import できない
		want(t, "types.go", types, []string{"type ItemId string", "type Short struct{}", `return "stock.Short"`}, []string{"gen/stock"})
	})
	t.Run("実装の単位: 生成するコードは context を引数に取らない", func(t *testing.T) {
		for p, content := range got {
			if strings.HasSuffix(p, ".go") && strings.Contains(string(content), "context") {
				t.Errorf("%s に context がある", p)
			}
		}
	})
	t.Run("出力の対象: トランスパイラが何も選ばなかった名前空間は何も出力しない", func(t *testing.T) {
		for p := range got {
			if !strings.HasPrefix(p, "gen/shop/") {
				t.Errorf("gen/shop の外に書いた: %s", p)
			}
		}
		if _, err := os.Stat(filepath.Join(root, "gen", "stock")); !os.IsNotExist(err) {
			t.Errorf("gen/stock がある（stock の単位は何も選んでいない）")
		}
		if strings.Contains(stdout.String(), "warning") {
			t.Errorf("選ばれなかった宣言に warning が出た:\n%s", stdout.String())
		}
	})
	t.Run("選択を変えれば出力も変わる", func(t *testing.T) {
		// @t.stop を読まなくすれば、notify の本体を出し、sendMail と logSent まで辿る
		rules := filepath.Join(root, "plugins", "pick", "pick.json")
		if err := os.WriteFile(rules, []byte(`{"roots": ["t.entry"]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		stdout.Reset()
		stderr.Reset()
		if code := hozocMainIn(root, []string{"emit"}, &stdout, &stderr); code != 0 {
			t.Fatalf("hozoc emit の終了コード %d\n%s%s", code, stdout.String(), stderr.String())
		}
		tree := readTree(t, root)
		ops, wiring := string(tree["gen/shop/ops.go"]), string(tree["gen/shop/wiring.go"])
		want(t, "ops.go", ops, []string{field("SendMail", "SendMailFunc"), field("LogSent", "LogSentFunc")},
			[]string{field("Notify", "NotifyFunc"), "Wrapper", "wrappers"}) // wraps も states も選ばない
		want(t, "wiring.go", wiring, []string{"func (o *ops) Notify(order Order) error {", "return o.LogSent()"}, nil)
		if strings.Contains(string(tree["gen/shop/states.go"]), "Audits") {
			t.Errorf("states を選んでいないのに audits の単位がある")
		}
	})
}
