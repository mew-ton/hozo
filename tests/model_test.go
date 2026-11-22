package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/fakeplugin"
	"github.com/mew-ton/hozo/internal/project"
	"github.com/mew-ton/hozo/ir"
)

// modelCorpus は tests/corpus/model（その README.md に見出し → 構文の対応がある）。
const modelCorpus = "corpus/model"

// designCorpus は tests/corpus/design（設計書の例）。
const designCorpus = "corpus/design"

// loadClean はコーパス root を internal/project で解析する。CFG-26 の warning のほかに、診断が一つでもあれば失敗。
func loadClean(t *testing.T, root string) *project.Project {
	t.Helper()
	p := project.LoadWith(project.Options{Root: root, Stderr: io.Discard, PluginPath: repoPlugins(t)})
	// CFG-26（解釈するトランスパイラがいないデコレータ。docs/config.md「解釈するトランスパイラ」）は、コーパスの想定のうち: 単位の
	// トランスパイラは、基底だけで、語彙のデコレータを解釈しない。その一覧は、ゴールデン testdata/expected/check.txt が持つ
	var b strings.Builder
	for _, d := range p.Diags {
		if d.Code == "CFG-26" && d.Severity == diag.Warning {
			continue
		}
		fmt.Fprintf(&b, "%s[%s]: %s\n", d.Severity, d.Code, d.Message)
	}
	if b.Len() > 0 {
		t.Fatalf("tests/%s に、CFG-26 の warning のほかの診断がある（想定しない）:\n%s", root, b.String())
	}
	return p
}

// lineIn は path のファイルで needle を含む最初の行（1 始まり）。
func lineIn(t *testing.T, path, needle string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i, l := range strings.Split(string(data), "\n") {
		if strings.Contains(l, needle) {
			return i + 1
		}
	}
	t.Fatalf("%s に %q がない", path, needle)
	return 0
}

// copyHozo は src の直下の .hozo を dst に写す（hozo.toml は写さない）。
func copyHozo(t *testing.T, src, dst string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(src, "*.hozo"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("%s に .hozo がない: %v", src, err)
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, filepath.Base(p)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestModelCorpus は「モデルが先」の決定（docs/decisions.md「設計書から離れた決定」S10〜S12）が IR と検査に現れることを、
// tests/corpus/model で確かめる: 宣言に付けた印のデコレータ（名前空間には付けられない）、どの単位の対象でもない名前空間（出力しない。検査は同じ）、
// どのプラグインも読まない印のデコレータ（throws を宣言していれば Signature に効く）。
// サブテストの名前は tests/corpus/model/README.md の「見出し → 構文」の表の見出し。
func TestModelCorpus(t *testing.T) {
	p := loadClean(t, modelCorpus)
	prog := p.Sema.Program

	t.Run("grammar.md ファイルと宣言/デコレータは名前空間に付けられない、ir.md デコレータの適用", func(t *testing.T) {
		// 印は名前空間ではなく宣言（model / state / op）に付ける。出力する名前空間でも、出力しない名前空間でも同じ
		expect(t, "shop.Order のデコレータ", renderApplications(declOf(t, prog, "shop.Order").Common().Decorators), []string{`@marks.context(name: "販売")`})
		expect(t, "shop.orders のデコレータ", renderApplications(declOf(t, prog, "shop.orders").Common().Decorators), []string{`@marks.context(name: "販売")`})
		expect(t, "carrier.parcels のデコレータ", renderApplications(declOf(t, prog, "carrier.parcels").Common().Decorators), []string{`@marks.context(name: "配送")`})
		expect(t, "carrier.pickup のデコレータ", renderApplications(opOf(t, prog, "carrier.pickup").Decorators), []string{`@marks.context(name: "配送")`})
		// 印を付けていない宣言は []
		expect(t, "shop.Line のデコレータ", renderApplications(declOf(t, prog, "shop.Line").Common().Decorators), []string{})
		expect(t, "shop の説明", docOf(prog.Namespace("shop").Description), "店（コードを生成する側）")
		// 同じ名前の namespace のブロックは一つの Namespace にまとまる（customer.hozo の二つのブロック）
		expect(t, "customer は一つの Namespace", countNamespaces(prog, "customer"), 1)
		// Namespace は decorators を持たない（docs/ir.md「Program」）
		data, err := json.Marshal(prog)
		if err != nil {
			t.Fatal(err)
		}
		raw := decodeJSON(t, data)
		for _, n := range raw["namespaces"].([]any) {
			ns := n.(map[string]any)
			if _, ok := ns["decorators"]; ok {
				t.Errorf("namespace %q の JSON に decorators がある: %v", ns["name"], ns["decorators"])
			}
		}
		// 位置は適用そのもの（@ から引数の閉じ括弧まで）
		app := declOf(t, prog, "shop.Order").Common().Decorators[0]
		expect(t, "適用の位置", []any{app.Span.File, app.Span.Start.Line, app.Span.Start.Col}, []any{"shop.hozo", lineIn(t, filepath.Join(modelCorpus, "shop.hozo"), `@marks.context("販売")`), 3})
		// 読み戻して書いても同じバイト列
		var back ir.Program
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatal(err)
		}
		again, _ := json.Marshal(&back)
		if !bytes.Equal(again, data) {
			t.Errorf("Program の JSON を読み戻して書くと変わる:\n%s", firstDiff(string(data), string(again)))
		}
	})

	t.Run("grammar.md decorator の宣言/Target、ir.md decorator/targets", func(t *testing.T) {
		dec := func(name string) *ir.Decorator { return declOf(t, prog, name).(*ir.Decorator) }
		expect(t, "context の targets（書いた順）", dec("marks.context").Targets, []ir.DecoratorTarget{ir.TargetModel, ir.TargetState, ir.TargetOp})
		expect(t, "manual の targets", dec("marks.manual").Targets, []ir.DecoratorTarget{ir.TargetOp})
		expect(t, "requirement の targets（省略は op）", dec("marks.requirement").Targets, []ir.DecoratorTarget{ir.TargetOp})
		expect(t, "requirement の throws", renderThrows(dec("marks.requirement").Throws), []string{"throws marks.NotApproved prevents effects"})
		// 人の操作は、担い手ごとの名前空間（customer）の中の op に印を付けて表す
		expect(t, "pick のデコレータ", renderApplications(opOf(t, prog, "customer.pick").Decorators), []string{"@marks.manual"})
		expect(t, "leave のデコレータ", renderApplications(opOf(t, prog, "customer.leave").Decorators), []string{"@marks.manual"})
	})

	t.Run("config.md 単位の対象", func(t *testing.T) {
		// View を持つ（出力する）のは、単位の対象の名前空間: shop と、shop が使う型の置き場の carrier と marks
		// （docs/config.md「型の置き場」）。View.namespaces にも、どの単位の対象でもない名前空間（customer）は入らない
		views := viewsOf(p)
		expect(t, "単位の数", len(p.Views()), 3)
		expect(t, "View を持つ名前空間", keysOf(views), []string{"carrier", "marks", "shop"})
		expect(t, "View.namespaces", keysOf(views["shop"].Namespaces), []string{"carrier", "marks", "shop"})
		if n := len(p.Config.Covering("customer")); n != 0 {
			t.Errorf("customer を対象にする単位が %d ある（単位の対象でない名前空間は出力先を持たない）", n)
		}
		for _, ns := range []string{"marks", "carrier", "customer"} {
			// どの名前空間も Program にはそのまま入る（トランスパイラは全体を読める）
			if views["shop"].Program.Namespace(ns) == nil {
				t.Errorf("View(shop).program に名前空間 %s がない", ns)
			}
		}
		// View.ops はすべての op（単位の対象でない名前空間のものも）を shop から見た Sig で持つ
		for _, op := range []string{"shop.addToCart", "shop.placeOrder", "shop.fulfill", "carrier.pickup", "customer.pick", "customer.buy", "customer.leave"} {
			if views["shop"].Ops[op] == nil {
				t.Errorf("View(shop).ops に %s がない", op)
			}
		}
		expect(t, "View(shop) の carrier.pickup", sigOf(views["shop"].Ops["carrier.pickup"].Signature), sig{
			E: []string{"carrier.Closed"}, F: []string{"carrier.accept"},
			Rel: []string{"carrier.Closed / carrier.accept = prevented"}, Succ: []string{"carrier.accept = kept"},
		})
	})

	t.Run("check.md 計算/手順 i の throws（印のデコレータ）", func(t *testing.T) {
		// @marks.requirement はどのプラグインも読まない印だが、throws を宣言しているので付けた op の Signature に効く。
		// op 自身の throws は空のまま
		place := opOf(t, prog, "shop.placeOrder")
		expect(t, "placeOrder の throws（宣言）", renderThrows(place.Throws), []string{})
		expect(t, "placeOrder のデコレータ", renderApplications(place.Decorators), []string{`@marks.requirement(id: "PRD-12")`})
		expect(t, "placeOrder の Signature", sigOf(place.Signature), sig{
			E: []string{"marks.NotApproved"}, F: []string{"shop.createOrder"},
			Rel: []string{"marks.NotApproved / shop.createOrder = prevented"}, Succ: []string{"shop.createOrder = kept"},
		})
		// throws を持たない印（@marks.manual）は Signature を変えない
		expect(t, "pick の Signature", sigOf(opOf(t, prog, "customer.pick").Signature), sig{
			E: []string{}, F: []string{"customer.forget"}, Rel: []string{}, Succ: []string{"customer.forget = kept"},
		})
	})

	t.Run("check.md 計算/手順 0（配線）: 名前空間をまたぐ伝播", func(t *testing.T) {
		// 単位の対象の shop の配線が、対象でない carrier の op を呼ぶ
		expect(t, "fulfill の Signature", sigOf(opOf(t, prog, "shop.fulfill").Signature), sig{
			E: []string{"marks.NotApproved", "carrier.Closed"}, F: []string{"shop.createOrder", "carrier.accept"},
			Rel: []string{
				"marks.NotApproved / shop.createOrder = prevented", "marks.NotApproved / carrier.accept = prevented",
				"carrier.Closed / shop.createOrder = kept", "carrier.Closed / carrier.accept = prevented",
			},
			Succ: []string{"shop.createOrder = kept", "carrier.accept = kept"},
		})
		// 単位の対象でない customer の配線が、対象の shop の op を呼ぶ。検査は同じに掛かり、Signature も計算される
		expect(t, "buy の Signature", sigOf(opOf(t, prog, "customer.buy").Signature), sig{
			E: []string{"shop.OutOfStock", "marks.NotApproved", "carrier.Closed"},
			F: []string{"customer.forget", "shop.addLine", "shop.createOrder", "carrier.accept"},
			Rel: []string{
				"shop.OutOfStock / customer.forget = kept", "shop.OutOfStock / shop.addLine = prevented",
				"shop.OutOfStock / shop.createOrder = prevented", "shop.OutOfStock / carrier.accept = prevented",
				"marks.NotApproved / customer.forget = kept", "marks.NotApproved / shop.addLine = kept",
				"marks.NotApproved / shop.createOrder = prevented", "marks.NotApproved / carrier.accept = prevented",
				"carrier.Closed / customer.forget = kept", "carrier.Closed / shop.addLine = kept",
				"carrier.Closed / shop.createOrder = kept", "carrier.Closed / carrier.accept = prevented",
			},
			Succ: []string{"customer.forget = kept", "shop.addLine = kept", "shop.createOrder = kept", "carrier.accept = kept"},
		})
	})

	t.Run("cli.md emit/どのトランスパイラもファイルを返さなければ、何も書かない", func(t *testing.T) {
		// shop は単位の対象（出力する名前空間）だが、その単位のトランスパイラ none はファイルを返さない。hozoc 本体は
		// コードを出力しない（docs/plugin-protocol.md「この文書が決めること」）ので、emit は何も書かない。印のデコレータ
		// （@marks.*）も何も生成しない。前回の出力（.hozoc-outputs.json にあるもの）は古いファイルとして消え、記録も残らない
		// （docs/cli.md「書き出し」の 5 と 7）。
		dir := t.TempDir()
		fakeplugin.Place(t, filepath.Join(dir, "plugins", "none", fakeplugin.WasmFile))
		copyHozo(t, modelCorpus, dir)
		manifest, err := os.ReadFile(filepath.Join(modelCorpus, "plugins", "none", "hozo-plugin.toml"))
		if err != nil {
			t.Fatal(err)
		}
		toml, err := os.ReadFile(filepath.Join(modelCorpus, "hozo.toml"))
		if err != nil {
			t.Fatal(err)
		}
		files := map[string]string{
			"hozo.toml":                     string(toml),
			"plugins/none/hozo-plugin.toml": string(manifest),
			"gen/shop/ops.go":               "package shop // 前回の emit が書いたもの\n",
			"gen/shop/.hozoc-outputs.json":  "[\n  \"gen/shop/ops.go\"\n]\n",
			"gen/shop/hand.go":              "package shop // 人が書いたもの。記録にないので触らない\n",
		}
		for name, content := range files {
			path := filepath.Join(dir, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		var stdout, stderr bytes.Buffer
		code := hozocMainIn(dir, []string{"build"}, &stdout, &stderr)
		// warning は、@marks.requirement の CFG-26 が一つ（README.md の marks.hozo の行）
		rest, warnings := splitUninterpreted(stdout.String())
		if code != 0 || rest != "removed: gen/shop/ops.go\n" || len(warnings) != 1 || !strings.Contains(warnings[0], "@marks.requirement ") || stderr.String() != "" {
			t.Fatalf("hozoc build: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
		var left []string
		for p := range readTree(t, filepath.Join(dir, "gen")) {
			left = append(left, p)
		}
		expect(t, "gen に残るファイル", left, []string{"shop/hand.go"})
		// 二度目は何も書かず、何も消さない
		stdout.Reset()
		code = hozocMainIn(dir, []string{"build"}, &stdout, &stderr)
		if rest, warnings := splitUninterpreted(stdout.String()); code != 0 || rest != "" || len(warnings) != 1 {
			t.Errorf("二度目の hozoc build: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	})

	t.Run("config.md 置き場と探し方/設定なし", func(t *testing.T) {
		// 同じ .hozo を hozo.toml なしで検査しても診断はなく、どの名前空間も出力しない
		dir := t.TempDir()
		copyHozo(t, modelCorpus, dir)
		q := project.LoadWith(project.Options{Root: dir, Stderr: io.Discard})
		if !q.NoConfig || len(q.Diags) != 0 {
			t.Fatalf("NoConfig=%v diags=%v", q.NoConfig, q.Diags)
		}
		expect(t, "View を持つ名前空間", keysOf(viewsOf(q)), []string{})
		expect(t, "buy の Signature.effects", effectStrings(q.Sema.Program.Lookup("customer.buy").(*ir.Op).Signature.Effects),
			[]string{"customer.forget", "shop.addLine", "shop.createOrder", "carrier.accept"})
		var stdout, stderr bytes.Buffer
		if code := hozocMainIn(dir, []string{"build", "--no-emit"}, &stdout, &stderr); code != 0 || stdout.String() != "0 errors, 0 warnings\n" {
			t.Errorf("hozoc build --no-emit（設定なし）: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
		stdout.Reset()
		stderr.Reset()
		if code := hozocMainIn(dir, []string{"build"}, &stdout, &stderr); code != 2 || !strings.Contains(stderr.String(), "error[CFG-7]") {
			t.Errorf("hozoc build（設定なし）: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
	})
}

// TestDesignPlaceOrderRelations は設計書 §7.3 の placeOrder（tests/corpus/design/README.md の表の 14）。
// 原文は葉の op に `effects createOrder | notifyUser throws NotificationFailed keeps createOrder prevents notifyUser` と
// 書くが、葉の op が直に持つ効果は一つまで（docs/decisions.md S8）で、prevents / keeps の目的語は effects だけ（S9）なので、
// 効果ごとの葉の op（saveOrder、sendOrderNotice）を配線でつないだ。原文が手で書いていた関係
// （NotificationFailed のとき createOrder は起きていて、notifyUser は起きていない）が、本体から計算した
// Signature.relations にそのまま現れることを確かめる（docs/check.md「計算」手順 0 の配線）。
func TestDesignPlaceOrderRelations(t *testing.T) {
	p := loadClean(t, designCorpus)
	prog := p.Sema.Program
	place := opOf(t, prog, "server.placeOrder")
	expect(t, "placeOrder は配線", renderBody(place.Body), []string{
		"val order = server.saveOrder(in: in)",
		"server.sendOrderNotice(order: order)",
		"return order",
	})
	// 配線には effects も relations も書いていない（計算される）
	expect(t, "placeOrder の effects（宣言）", effectStrings(place.Effects), []string{})
	expect(t, "placeOrder の throws（宣言）", renderThrows(place.Throws), []string{"throws server.NotificationFailed"})
	want := sig{
		E: []string{"server.NotificationFailed"},
		F: []string{"server.createOrder", "server.notifyUser"},
		Rel: []string{
			"server.NotificationFailed / server.createOrder = kept",
			"server.NotificationFailed / server.notifyUser = prevented",
		},
		Succ: []string{"server.createOrder = kept", "server.notifyUser = kept"},
	}
	expect(t, "placeOrder の Signature", sigOf(place.Signature), want)
	// 葉の op はそれぞれ効果を一つだけ持つ
	expect(t, "saveOrder の Signature", sigOf(opOf(t, prog, "server.saveOrder").Signature), sig{
		E: []string{}, F: []string{"server.createOrder"}, Rel: []string{}, Succ: []string{"server.createOrder = kept"},
	})
	expect(t, "sendOrderNotice の Signature", sigOf(opOf(t, prog, "server.sendOrderNotice").Signature), sig{
		E: []string{"server.NotificationFailed"}, F: []string{"server.notifyUser"},
		Rel: []string{"server.NotificationFailed / server.notifyUser = prevented"}, Succ: []string{"server.notifyUser = kept"},
	})
	// browser から見ても同じ（external なデコレータは付いていない）
	expect(t, "View(browser) の placeOrder", sigOf(viewsOf(p)["browser"].Ops["server.placeOrder"].Signature), want)
}
