package godesign

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// このファイルは docs/plugin-go-design.md のテスト: テストのパッケージの design.go の中身（ゴールデン）と、生成した口を、一時のモジュールで
// 動かしたときの振る舞いと、診断（GODESIGN-*）と、warning。入力と、ゴールデンの比べ方は fixtures_test.go。

// goldenFixtures は、出力をゴールデンと比べる入力（testdata/<名前>/）と、それが確かめる docs/plugin-go-design.md の見出し。
var goldenFixtures = []struct {
	name   string
	covers string
}{
	{name: "flows", covers: "「出力」「E2E テストの口」「要求の、テストの入れ物の口」: 値を返すフローと、返さないフロー、引数のないフロー、説明のないフロー、" +
		"Given・Then と当たる引数の名前、match・for・disjoint の中から操作の op を呼ぶフロー、二つの要求の分類、説明のない要求、どこからも" +
		"指されていない要求、op のほかの宣言からの @prd.ref、要求だけの単位、フローだけの単位、基底が、テストのパッケージを作る出力と、作らない出力"},
}

// TestGolden は、入力ごとに、出力がゴールデン testdata/<名前>/expected/ とバイト単位で同じこと、go/format を通した形で
// あること、同じ View から同じバイト列を返すこと、依存するトランスパイラ（基底 go）の出力と合わせてコンパイルできる
// （一時のモジュールで go vet を通る）ことを確かめる。
func TestGolden(t *testing.T) {
	for _, fx := range goldenFixtures {
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()
			views := loadViews(t, fx.name)
			root, files := emitFixture(t, fx.name)
			checkFormatted(t, files)
			compareTree(t, fx.name, files)
			// 決定性: もう一度、別のルート（同じモジュールのパス）で出力しても、同じバイト列
			again, _, _ := emitViews(t, newRoot(t), views)
			if len(again) != len(files) {
				t.Errorf("二度目の出力のファイルの数が違う: %d と %d", len(files), len(again))
			}
			for p, content := range files {
				if again[p] != content {
					t.Errorf("%s が、二度目の出力で変わった", p)
				}
			}
			vet(t, root, views, files)
		})
	}
}

// writeFlows は、入力 flows の出力（依存するトランスパイラの出力と合わせたもの）と、テストのファイル tests を、一時のモジュールに
// 置いて、ルートを返す。
func writeFlows(t *testing.T, tests ...string) string {
	t.Helper()
	root, files := emitFixture(t, "flows")
	compose(t, root, loadViews(t, "flows"), files)
	for _, name := range tests {
		test, err := os.ReadFile(filepath.Join("testdata", "flows", name))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(root, name), string(test))
	}
	return root
}

// TestBehavior は、生成した口を、一時のモジュールで、基底の出力に当てて走らせる（testdata/flows/behavior_test.go。確かめることは、
// そのファイルの頭）: すべてのケースと、すべての入れ物を満たす型を渡せば、通る。テストの名前（ラベル）は、フローの FQN と、要求の名前
// （docs/ide-chain.md「テストの規約」）。
func TestBehavior(t *testing.T) {
	t.Parallel()
	root := writeFlows(t, "behavior_test.go")
	out, err := runGo(t, root, "test", "-count=1", "-v", ".")
	if err != nil {
		t.Fatalf("口を満たす型のテストが落ちた: %v\n%s", err, out)
	}
	for _, name := range []string{
		"TestFlows/flows.checkout", "TestFlows/flows.ping", "TestFlows/flows.rename", "TestFlows/flows.branch", "TestFlows/flows.lookup",
		"TestRequirements/design.Req.R1", "TestRequirements/design.Req.R1/first", "TestRequirements/design.Req.R1/second",
		"TestRequirements/design.Req.R2", "TestRequirements/design.Req.r3", "TestRequirements/design.Extra.X1",
		"TestMixed/mixed.run", "TestMixed/design.Extra.X1", "TestK/k.go",
		"TestReqOnly/design.Req.R1", "TestReqOnly/design.Req.R2", "TestReqOnly/design.Req.r3",
	} {
		if !strings.Contains(out, "--- PASS: "+name+" ") {
			t.Errorf("テスト %s が、通っていない:\n%s", name, out)
		}
	}
}

// TestMismatch は、走らせる側の確かめ（「E2E テストの口」の「口の形」）: Given が失敗を返せば、フローを呼ばずに落ちる。フローが失敗を
// 返せば、落ちる。どちらも、Then は呼ばれない（testdata/flows/mismatch_test.go）。差し替えていないケースは、通る。
func TestMismatch(t *testing.T) {
	t.Parallel()
	root := writeFlows(t, "behavior_test.go", "mismatch_test.go")
	out, err := runGo(t, root, "test", "-count=1", "-v", "-run", "^TestFlowMismatch$", ".")
	if err == nil {
		t.Fatalf("Given とフローが失敗を返したのに、テストが通った:\n%s", out)
	}
	for name, message := range map[string]string{
		"flows.checkout": "given: boom",
		"flows.ping":     "want no error, got boom",
	} {
		if !strings.Contains(out, "--- FAIL: TestFlowMismatch/"+name+" ") {
			t.Errorf("テスト %s が、落ちていない:\n%s", name, out)
		}
		if !strings.Contains(out, message) {
			t.Errorf("テスト %s の文面 %q がない:\n%s", name, message, out)
		}
	}
	for _, banned := range []string{"Then was called", "フローが呼ばれた"} {
		if strings.Contains(out, banned) {
			t.Errorf("落ちたケースで、%q が出た:\n%s", banned, out)
		}
	}
	for _, name := range []string{"flows.rename", "flows.branch", "flows.lookup"} {
		if !strings.Contains(out, "--- PASS: TestFlowMismatch/"+name+" ") {
			t.Errorf("差し替えていないテスト %s が、通っていない:\n%s", name, out)
		}
	}
	if n := strings.Count(out, "\n    --- FAIL: "); n != 2 {
		t.Errorf("落ちた、フローのテストが %d 個（2 のはず）:\n%s", n, out)
	}
}

// implMethods は、口を満たす型のメソッド（<型>.<メソッド> → 宣言）。TestIncompleteDoNotCompile が、一つずつ欠く。
var implMethods = map[string]string{
	// flowstest.FlowCases: フローごと
	"flowCases.Checkout": "func (flowCases) Checkout() flowstest.CheckoutFlowCase { return flowstest.CheckoutFlowCase{} }",
	"flowCases.Ping":     "func (flowCases) Ping() flowstest.PingFlowCase { return flowstest.PingFlowCase{} }",
	"flowCases.Rename":   "func (flowCases) Rename() flowstest.RenameFlowCase { return flowstest.RenameFlowCase{} }",
	"flowCases.Branch":   "func (flowCases) Branch() flowstest.BranchFlowCase { return flowstest.BranchFlowCase{} }",
	"flowCases.Lookup":   "func (flowCases) Lookup() flowstest.LookupFlowCase { return flowstest.LookupFlowCase{} }",
	// flowstest.Requirements: 要求ごと
	"requirements.R1": "func (requirements) R1(*testing.T) {}",
	"requirements.R2": "func (requirements) R2(*testing.T) {}",
	"requirements.R3": "func (requirements) R3(*testing.T) {}",
	"requirements.X1": "func (requirements) X1(*testing.T) {}",
	// reqonlytest.Requirements: 要求だけの単位
	"onlyRequirements.R1": "func (onlyRequirements) R1(*testing.T) {}",
	"onlyRequirements.R2": "func (onlyRequirements) R2(*testing.T) {}",
	"onlyRequirements.R3": "func (onlyRequirements) R3(*testing.T) {}",
}

// implSource は、口を満たす型を持つパッケージのテキスト。omit の宣言を、書かない。
func implSource(omit string) string {
	var b strings.Builder
	b.WriteString(`package impl

import (
	"testing"

	"example.com/app/gen/flows/flowstest"
	"example.com/app/gen/reqonly/reqonlytest"
)

type (
	flowCases        struct{}
	requirements     struct{}
	onlyRequirements struct{}
)

// Run は、口を満たす型を、走らせる関数に渡す。口のメソッドが、一つでも足りなければ、ここで、コンパイルできない。
func Run(t *testing.T) {
	flowstest.RunFlows(t, nil, flowCases{})
	flowstest.RunRequirements(t, requirements{})
	reqonlytest.RunRequirements(t, onlyRequirements{})
}

`)
	names := make([]string, 0, len(implMethods))
	for name := range implMethods {
		names = append(names, name)
	}
	sort.Strings(names) // 決まった順に書く
	for _, name := range names {
		if name != omit {
			b.WriteString(implMethods[name] + "\n")
		}
	}
	return b.String()
}

// TestIncompleteDoNotCompile は、ケースと入れ物の漏れが、コンパイルエラーになること（「E2E テストの口」「要求の、テストの入れ物の口」の
// 「口の形」。CLAUDE.md「実装の漏れは、コンパイルエラーにする」）: フローのケースを一つ欠いた FlowCases と、要求の入れ物を一つ欠いた
// Requirements は、走らせる関数に渡すところで、コンパイルできない。
func TestIncompleteDoNotCompile(t *testing.T) {
	t.Parallel()
	root := writeFlows(t)
	impl := filepath.Join(root, "impl", "impl.go")

	// すべてのメソッドを持つ型は、コンパイルできる（下の失敗が、ほかの理由でないこと）
	writeFile(t, impl, implSource(""))
	if out, err := runGo(t, root, "vet", "./impl"); err != nil {
		t.Fatalf("すべてのケースを持つ実装が、コンパイルできない: %v\n%s", err, out)
	}
	for _, tt := range []struct{ omit, what string }{
		{"flowCases.Checkout", "フロー flows.checkout のケース"},
		{"flowCases.Ping", "値を返さないフロー flows.ping のケース"},
		{"flowCases.Lookup", "最後のフロー flows.lookup のケース"},
		{"requirements.R1", "要求 design.Req.R1 の入れ物"},
		{"requirements.R3", "どこからも指されていない要求 design.Req.r3 の入れ物"},
		{"requirements.X1", "二つ目の分類の要求 design.Extra.X1 の入れ物"},
		{"onlyRequirements.R2", "要求だけの単位の、要求 design.Req.R2 の入れ物"},
	} {
		method := tt.omit[strings.IndexByte(tt.omit, '.')+1:]
		writeFile(t, impl, implSource(tt.omit))
		out, err := runGo(t, root, "vet", "./impl")
		if err == nil {
			t.Errorf("%sを持たない実装が、コンパイルできた", tt.what)
			continue
		}
		if !strings.Contains(out, "missing method "+method) {
			t.Errorf("%sを持たない実装のコンパイルエラーが、足りないメソッド %s を示さない:\n%s", tt.what, method, out)
		}
	}
}

// TestContents は、design.go の中身（「出力」「口の形」「指している設計」）のうち、行として確かめるもの。
func TestContents(t *testing.T) {
	_, files := emitFixture(t, "flows")
	// 出すファイルは、go-design を並べた単位ごとに、テストのパッケージの design.go と、その、テストの対応表（act の単位は、
	// 並べていない）
	if got := strings.Join(sortedPaths(files), " "); got != "gen/flows/flowstest/design.go gen/flows/flowstest/hozo-tests.go-design.json "+
		"gen/k/ktest/design.go gen/k/ktest/hozo-tests.go-design.json gen/mixed/mixedtest/design.go gen/mixed/mixedtest/hozo-tests.go-design.json "+
		"gen/reqonly/reqonlytest/design.go gen/reqonly/reqonlytest/hozo-tests.go-design.json" {
		t.Errorf("出したファイル = %s", got)
	}
	flows := files["gen/flows/flowstest/design.go"]
	for _, line := range []string{
		// N の入口のパッケージは、いつも、名前を書いて import する。別の名前空間の型は、その置き場の入口のパッケージから
		"\tflows \"example.com/app/gen/flows\"\n", "\t\"example.com/app/gen/act\"\n",
		// 引数のフィールド: Given、Then と当たれば、末尾に _
		"\tGiven_ string\n", "\tThen_  string\n", "got, err := ops.Rename(ctx, k.Given_, k.Then_)\n",
		// Then の形: 値を返すフローと、返さないフロー
		"\tThen func(t *testing.T, got act.Receipt)\n", "\tThen func(t *testing.T)\n", "\tThen func(t *testing.T, got *string)\n",
		// 走らせる側: フローの FQN の名前で、Given、フロー、正常に終わったことの確かめ、Then
		"t.Run(\"flows.checkout\", func(t *testing.T) {\n", "if err := k.Given(ctx); err != nil {\n", "t.Fatalf(\"given: %v\", err)\n",
		"err := ops.Ping(ctx)\n", "t.Fatalf(\"want no error, got %v\", err)\n", "k.Then(t, got)\n", "k.Then(t)\n",
		"func RunFlows(t *testing.T, ops flows.Ops, c FlowCases) {\n",
		// フローの説明: 二行以上なら、行ごとに続ける。説明がなければ、一行目だけ
		"// フロー: 二行以上の説明を持つフロー。\n// 引数の名前が、Given、Then と当たる。\ntype RenameFlowCase struct {\n",
		"足りない確かめ。\ntype PingFlowCase struct {\n",
		// 要求: 分類の順、その中は、ケースの宣言順。メソッドの名前は、先頭を大文字に。テストの名前は、もとのケースの名前
		"// Requirements は、要求ごとの、テストの入れ物。要求（design.Req、design.Extra のケース）ごとに、メソッド一つ（宣言順）。中身は、人か AI が書く。\n",
		// 走らせる側は、入れ物を、その場の関数で包んで呼ぶ（「要求の、テストの入れ物の口」の「口の形」）
		"\tt.Run(\"design.Req.r3\", func(t *testing.T) {\n\t\tc.R3(t)\n\t})\n", "func RunRequirements(t *testing.T, c Requirements) {\n",
		// 指している設計: 名前空間の FQN の辞書順、その中は宣言順。同じ宣言は、一度だけ。op のほかの宣言（model、状態、read、効果）も
		"\t// 指している設計: act.buy、act.count、flows.checkout\n", "\t// 指している設計: data.Row、data.rows、data.first、data.add\n",
		"\t// 指している設計: なし\n", "\t// 指している設計: data.first、flows.checkout\n",
		// 説明のない要求と、二行以上の説明
		"\t// R2 は、要求 design.Req.R2。\n", "\t// X1 は、要求 design.Extra.X1: 二行以上の説明を持つ要求。\n\t// 二行目。\n\t//\n\t// 空行の後の行。\n",
	} {
		if !strings.Contains(flows, line) {
			t.Errorf("gen/flows/flowstest/design.go に、%q がない", line)
		}
	}
	// 公開の名前は、<Op>FlowCase、FlowCases、RunFlows、Requirements、RunRequirements だけ。非公開の名前は、ない
	for _, line := range strings.Split(flows, "\n") {
		for _, prefix := range []string{"func ", "type "} {
			rest, ok := strings.CutPrefix(line, prefix)
			if !ok {
				continue
			}
			name := rest[:strings.IndexAny(rest, "( ")]
			if !strings.HasSuffix(name, "FlowCase") && name != "FlowCases" && name != "RunFlows" && name != "Requirements" && name != "RunRequirements" {
				t.Errorf("gen/flows/flowstest/design.go が、文書にない名前 %s を宣言している", name)
			}
		}
	}
	// 要求だけの単位: フローの口も、N の入口のパッケージの import も、ない
	only := files["gen/reqonly/reqonlytest/design.go"]
	for _, banned := range []string{"FlowCases", "RunFlows", "context", "example.com/app/gen/reqonly"} {
		if strings.Contains(only, banned) {
			t.Errorf("gen/reqonly/reqonlytest/design.go に、%q がある", banned)
		}
	}
	// フローだけの単位: 要求の口は、ない。名前空間のパッケージの名前 k は、本体の局所の名前と当たるので、k2
	kfile := files["gen/k/ktest/design.go"]
	if strings.Contains(kfile, "Requirements") || !strings.Contains(kfile, "\tk2 \"example.com/app/gen/k\"\n") || !strings.Contains(kfile, "ops k2.Ops") {
		t.Errorf("gen/k/ktest/design.go =\n%s", kfile)
	}
	// 基底が、テストのパッケージを作る出力でも、同じパッケージに出す
	if mixed := files["gen/mixed/mixedtest/design.go"]; !strings.Contains(mixed, "\npackage mixedtest\n") {
		t.Errorf("gen/mixed/mixedtest/design.go =\n%s", mixed)
	}
}

// TestWarnings は、二つの warning（「対象」「指している設計」）: フローも、要求もない単位は、ファイルを出さず warning を一つ。
// どこからも指されていない要求は、要求ごとに、それを並べた単位ごとに、warning を一つ（位置は、その分類）で、口は、出す。
func TestWarnings(t *testing.T) {
	root := newRoot(t)
	files, diags, warnings := emitViews(t, root, loadViews(t, "empty"))
	if len(files) != 0 || len(diags) != 0 || len(warnings) != 2 {
		t.Fatalf("empty: files=%v 診断=%+v warnings=%+v, want warning が二つだけ", sortedPaths(files), diags, warnings)
	}
	for i, at := range []string{"app", "other"} {
		if warnings[i].At != at || !strings.Contains(warnings[i].Message, "@business.flow") || !strings.Contains(warnings[i].Message, "requirements") {
			t.Errorf("empty: warning = %+v", warnings[i])
		}
	}
	resp, err := Handler{}.Emit(request(loadViews(t, "empty")[0], TranspilerName))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 0 || len(resp.Warnings) != 1 || len(resp.Errors) != 0 {
		t.Errorf("empty: 応答 = %+v, want warning が一つだけ", resp)
	}

	files, diags, warnings = emitViews(t, newRoot(t), loadViews(t, "flows"))
	if len(diags) != 0 || len(warnings) != 2 {
		t.Fatalf("flows: 診断=%+v warnings=%+v, want warning が二つだけ（design.Req を並べた、flows と reqonly の単位）", diags, warnings)
	}
	for _, w := range warnings {
		if w.At != "design.Req" || !strings.Contains(w.Message, "design.Req.r3") {
			t.Errorf("flows: warning = %+v", w)
		}
	}
	if !strings.Contains(files["gen/reqonly/reqonlytest/design.go"], "\tR3(t *testing.T)\n") {
		t.Error("flows: どこからも指されていない要求の、入れ物の口がない")
	}
}

// diagnosticFixtures は、診断を返す入力と、そのコード。期待する診断は testdata/<名前>/expected/errors.txt
// （一行に、コード、位置、メッセージをタブで区切って。View の順、その中は返した順）。
var diagnosticFixtures = []struct{ name, code string }{
	{"godesign1", codeFlowExport}, // @business.flow の付いた op に export がない
	{"godesign2", codeFlowShape},  // @business.flow の付いた op が、本体を持たない、または never を返す
	{"godesign3", codeFlowCalls},  // フローの op の配線が、操作の op でないものを呼ぶ
	{"godesign4", codeFlowHome},   // フローを持つ名前空間に、@usecase.of の付いた op がある
	{"godesign5", codeOption},     // [emit.options.go-design] の誤り
	{"godesign6", codeTestName},   // テストのパッケージに足す名前が、当たる、または Go の公開の名前にならない
	{"godesign7", codeTestDir},    // テストのパッケージのディレクトリが、同じ単位が出力する子の名前空間の出力先と重なる
}

// TestDiagnosticCodes は、「診断」の表の ID（GODESIGN-1〜GODESIGN-7。欠番はない）のどれもが、入力を一つ持つこと。
func TestDiagnosticCodes(t *testing.T) {
	seen := map[string]bool{}
	for _, fx := range diagnosticFixtures {
		seen[fx.code] = true
	}
	for n := 1; n <= 7; n++ {
		if code := "GODESIGN-" + strconv.Itoa(n); !seen[code] {
			t.Errorf("%s の入力がない", code)
		}
	}
	if len(seen) != 7 {
		t.Errorf("診断の入力のコードが %d 種類（7 のはず）", len(seen))
	}
}

// TestDiagnostics は docs/plugin-go-design.md「診断」の GODESIGN-1〜GODESIGN-7: 診断のコード・位置・メッセージがゴールデンと同じで、
// 診断を返した出力がファイルも warning も返さないこと（emitViews が確かめる）。
func TestDiagnostics(t *testing.T) {
	for _, fx := range diagnosticFixtures {
		t.Run(fx.name, func(t *testing.T) {
			_, diags, _ := emitViews(t, newRoot(t), loadViews(t, fx.name))
			if len(diags) == 0 {
				t.Fatal("診断がない")
			}
			var b bytes.Buffer
			for _, d := range diags {
				if d.Code != fx.code {
					t.Errorf("コード %s の診断がある（%s だけのはず）: %s", d.Code, fx.code, d.Message)
				}
				b.WriteString(d.Code + "\t" + d.At + "\t" + d.Message + "\n")
			}
			compareTree(t, fx.name, map[string]string{"errors.txt": b.String()})
		})
	}
}

// TestDiagnosticPositions は、診断の位置（docs/plugin-go-design.md「診断」の表）と、その数。GODESIGN-5 は位置なし。
func TestDiagnosticPositions(t *testing.T) {
	for name, want := range map[string]map[string]int{
		"godesign1": {"flows.hidden": 1, "flows.also": 1},
		// 葉と、never を返す op
		"godesign2": {"flows.leaf": 1, "flows.fail": 1},
		// 呼んだ先ごとに、一つ（同じ先を二度呼んでも、一つ）。位置は、フローの op
		"godesign3": {"flows.callsPlain": 1, "flows.callsHelper": 1, "flows.callsFlow": 1, "flows.callsState": 3, "flows.nested": 3},
		// @usecase.of の付いた op（フローでもある op を含む）。フローを持たない名前空間の op は、当たらない
		"godesign4": {"flows.local": 1, "flows.both": 1},
		"godesign5": {"": 8},
		// 当たった自分の名前を生んだフローの op か、要求の分類。決まった名前が当たったなら、相手の名前を生んだ宣言（葉の op、分類）
		"godesign6": {"leafy.flow": 1, "leafy.x": 1, "typed.Flows": 1, "typed.Requirements": 1, "fields.clash": 1, "fields.caps": 1,
			"hidden._secret": 1, "design.Hidden": 1, "design.Other": 2},
		// 最初のフローの op。フローがなければ、最初の要求の分類
		"godesign7": {"n.first": 1, "design.Req": 1},
	} {
		_, diags, _ := emitViews(t, newRoot(t), loadViews(t, name))
		got := map[string]int{}
		for _, d := range diags {
			got[d.At]++
		}
		if len(got) != len(want) {
			t.Errorf("%s: 位置 = %v, want %v", name, got, want)
		}
		for at, n := range want {
			if got[at] != n {
				t.Errorf("%s: 位置 %q の診断が %d 個（%d のはず。%v）", name, at, got[at], n, got)
			}
		}
	}
}

// TestFlowShapeMessages は GODESIGN-2 の二つの条件: 本体を持たない op と、never を返す op は、別のメッセージ（never を返す op も、
// 本体を持たないので、never を先に見る）。
func TestFlowShapeMessages(t *testing.T) {
	_, diags, _ := emitViews(t, newRoot(t), loadViews(t, "godesign2"))
	for _, d := range diags {
		switch d.At {
		case "flows.leaf":
			if !strings.Contains(d.Message, "本体を持たない") {
				t.Errorf("flows.leaf: %s", d.Message)
			}
		case "flows.fail":
			if !strings.Contains(d.Message, "never を返す") {
				t.Errorf("flows.fail: %s", d.Message)
			}
		}
	}
}

// TestTestDir は GODESIGN-7 の条件: 基底も、テストのパッケージを作る出力（葉の op がある）では、基底が GO-4 を返すので、重ねて返さない。
// 置き場が要るので、module がないときも、返さない（基底が GO-8 を返す）。
func TestTestDir(t *testing.T) {
	views := loadViews(t, "godesign7")
	_, diags, _ := emitViews(t, newRoot(t), views)
	if len(diags) != 2 || diags[0].Code != codeTestDir || diags[0].At != "n.first" || diags[1].At != "design.Req" {
		t.Errorf("診断 = %+v, want n.first と design.Req の GODESIGN-7（m は、基底が GO-4 を返す）", diags)
	}
	if _, diags, _ = emitViews(t, "", noModule(views)); len(diags) != 0 {
		t.Errorf("module がないとき: 診断 = %+v, want なし", diags)
	}
}

// TestNoModule: 基底のオプション module がないとき（基底が GO-8 を返す）、go-design は、ファイルを返さない。仕様だけで決まる診断と、
// warning は、module がなくても返す。
func TestNoModule(t *testing.T) {
	files, diags, warnings := emitViews(t, "", noModule(loadViews(t, "flows")))
	if len(files) != 0 || len(diags) != 0 || len(warnings) != 2 {
		t.Errorf("files=%v 診断=%+v warnings=%+v, want ファイルも診断もなく、warning が二つ（GO-8 は基底が返す）", sortedPaths(files), diags, warnings)
	}
	_, diags, _ = emitViews(t, "", noModule(loadViews(t, "godesign1")))
	if len(diags) == 0 || diags[0].Code != codeFlowExport {
		t.Errorf("診断 = %+v, want GODESIGN-1", diags)
	}
}

// TestUnitTargets は「対象」: フローは、単位の対象の op（View.files にあるファイルの op。docs/config.md「in」）だけ。
// 単位の対象でない op は、@business.flow が付いていても、フローにならない。
func TestUnitTargets(t *testing.T) {
	var flows *ir.View
	for _, v := range loadViews(t, "flows") {
		if v.Viewer == "k" {
			copied := *v
			flows = &copied
		}
	}
	if flows == nil {
		t.Fatal("入力 flows に、名前空間 k の View がない")
	}
	// 単位の対象から、仕様のファイルを外す（語彙のファイルだけを残す）
	var kept []string
	for _, f := range flows.Files {
		if !strings.HasSuffix(f, "app.hozo") {
			kept = append(kept, f)
		}
	}
	if len(kept) == len(flows.Files) {
		t.Fatalf("View.files に、app.hozo がない: %v", flows.Files)
	}
	flows.Files = kept
	out, err := Emit(request(flows, TranspilerName))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 0 || len(out.Errors) != 0 || len(out.Warnings) != 1 {
		t.Errorf("単位の対象でないフロー: files=%d errors=%+v warnings=%+v, want warning が一つだけ", len(out.Files), out.Errors, out.Warnings)
	}
}

// TestBrokenView: View が欠けている（Program がない、viewer の名前空間が Program にない）のは、利用者の仕様の誤りではないので、
// 診断ではなく error で終わる（hozoc からは hozoc.Crashed）。
func TestBrokenView(t *testing.T) {
	view := loadViews(t, "flows")[1]
	for name, v := range map[string]*ir.View{
		"Program がない":          {Version: ir.Version, Viewer: "flows", Config: view.Config},
		"viewer が Program にない": {Version: ir.Version, Viewer: "nowhere", Config: view.Config, Program: view.Program},
	} {
		out, err := Emit(request(v, TranspilerName))
		if err == nil || out != nil {
			t.Errorf("%s: out=%+v err=%v, want error だけ", name, out, err)
		}
	}
}

// TestHandler は、プラグインの入口（Handler）: 要求の View から出力し、診断は応答の errors で返す
// （docs/plugin-protocol.md「emit」）。import は持たない。
func TestHandler(t *testing.T) {
	var view *ir.View
	for _, v := range loadViews(t, "flows") {
		if v.Viewer == "flows" {
			view = v
		}
	}
	if view == nil {
		t.Fatal("入力 flows に、名前空間 flows の View がない")
	}
	root := newRoot(t)
	h := Handler{}
	resp, err := h.Emit(request(view, TranspilerName))
	if err != nil {
		t.Fatal(err)
	}
	// どこからも指されていない要求の warning は、ファイルと一緒に返す
	if len(resp.Files) != 2 || len(resp.Errors) != 0 || len(resp.Warnings) != 1 {
		t.Fatalf("files=%d errors=%v warnings=%v", len(resp.Files), resp.Errors, resp.Warnings)
	}
	want, _, _ := emitViews(t, root, []*ir.View{view})
	// 並びは、design.go、その、テストの対応表
	for i, p := range []string{"gen/flows/flowstest/design.go", "gen/flows/flowstest/hozo-tests.go-design.json"} {
		if f := resp.Files[i]; f.Path != p || want[f.Path] != f.Content {
			t.Errorf("%d 番目のファイル %s が、Emit の結果（%s）と違う", i+1, f.Path, p)
		}
	}
	// 診断は errors で返し、ファイルも warning も返さない
	resp, err = h.Emit(request(loadViews(t, "godesign5")[1], TranspilerName))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 0 || len(resp.Warnings) != 0 || len(resp.Errors) == 0 || resp.Errors[0].Code != codeOption {
		t.Errorf("files=%d warnings=%v errors=%+v, want GODESIGN-5", len(resp.Files), resp.Warnings, resp.Errors)
	}
	// ほかのトランスパイラの名前は unsupported
	if _, err := h.Emit(request(view, "go-http")); !errors.Is(err, plugin.ErrUnsupported) {
		t.Errorf("知らない transpiler の err = %v, want ErrUnsupported", err)
	}
	if _, err := h.Emit(&plugin.EmitRequest{Transpiler: TranspilerName}); err == nil {
		t.Error("View のない要求で、エラーにならない")
	}
	if _, err := h.Import(&plugin.ImportRequest{}); err != plugin.ErrUnsupported {
		t.Errorf("Import の err = %v, want ErrUnsupported", err)
	}
}
