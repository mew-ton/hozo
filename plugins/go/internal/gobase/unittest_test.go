package gobase

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは、単体テストの口（docs/plugin-go.md「単体テストの口」）を、生成したコードを実際にコンパイルして、走らせて確かめる:
// ケースの漏れは、コンパイルエラーになる。テストの名前は、<FQN>/<結果>。結果がケースのものと違えば、口が、テストを落とす。
// 口を満たす型が、すべてのケースで通ることと、効果の入口の確かめは、入力 unittest の behavior_test.go（TestBehavior）。

// unittestCases は、入力 unittest の、口を満たすはずの型のメソッドの、すべての実装（<型>.<メソッド> → ソース）。
var unittestCases = map[string]string{
	// apptest.LeafCases
	"leafCases.Submit": "func (leafCases) Submit() apptest.SubmitCases { return submitCases{} }",
	"leafCases.Log":    "func (leafCases) Log() apptest.LogCases { return nil }",
	"leafCases.Send":   "func (leafCases) Send() apptest.SendCases { return nil }",
	"leafCases.Refill": "func (leafCases) Refill() apptest.RefillCases { return nil }",
	"leafCases.Reopen": "func (leafCases) Reopen() apptest.ReopenCases { return nil }",
	"leafCases.Find":   "func (leafCases) Find() apptest.FindCases { return nil }",
	// apptest.SubmitCases: 結果ごと
	"submitCases.OK":        "func (submitCases) OK() apptest.SubmitCase { return apptest.SubmitCase{} }",
	"submitCases.Rejected":  "func (submitCases) Rejected() apptest.SubmitCase { return apptest.SubmitCase{} }",
	"submitCases.AppBusy":   "func (submitCases) AppBusy() apptest.SubmitCase { return apptest.SubmitCase{} }",
	"submitCases.StockBusy": "func (submitCases) StockBusy() apptest.SubmitCase { return apptest.SubmitCase{} }",
	// apptest.StageExamples: ケースごと
	"stageExamples.Draft": "func (stageExamples) Draft() app.Order { return app.Order{} }",
	"stageExamples.Sent":  "func (stageExamples) Sent() app.Order { return app.Order{} }",
	"stageExamples.Done":  "func (stageExamples) Done() app.Order { return app.Order{} }",
	// apptest.LifeExamples: ない側のケース Absent は、メソッドがない
	"lifeExamples.Open":   "func (lifeExamples) Open() *app.Order { return nil }",
	"lifeExamples.Closed": "func (lifeExamples) Closed() *app.Order { return nil }",
}

// casesImpl は、口を満たすはずの型のパッケージのソース。omit のメソッドだけを持たない。口に渡すところは、RunLeaves と Run<C> を
// 呼ぶ関数（人が書くテストと、同じ渡し方）。
func casesImpl(omit string) string {
	var names []string
	for name := range unittestCases {
		if name != omit {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("package impl\n\nimport (\n\t\"testing\"\n\n\t\"" + modulePath + "/gen/app\"\n\t\"" + modulePath + "/gen/app/apptest\"\n)\n\n")
	b.WriteString("type (\n\tleafCases     struct{}\n\tsubmitCases   struct{}\n\tstageExamples struct{}\n\tlifeExamples  struct{}\n)\n\n")
	b.WriteString("// Run は、人が書くテストと同じに、口を満たす型を、走らせる関数に渡す\n")
	b.WriteString("func Run(t *testing.T, leaves app.Leaves, life app.LifeCases, stage app.StageCases) {\n")
	b.WriteString("\tapptest.RunLeaves(t, leaves, life, stage, nil, leafCases{})\n\tvar _ apptest.SubmitCases = submitCases{}\n")
	b.WriteString("\tapptest.RunStage(t, stage, stageExamples{})\n\tapptest.RunLife(t, life, lifeExamples{})\n}\n\n")
	for _, name := range names {
		b.WriteString(unittestCases[name] + "\n")
	}
	return b.String()
}

// TestIncompleteCasesDoNotCompile は、単体テストのケースの漏れが、コンパイルエラーになること（同「葉の op」「分類の判定」）:
// 葉を一つ欠いた LeafCases、結果（正常、例外、名前が当たって名前空間を前に付けた例外）を一つ欠いた <Op>Cases、ケースを一つ
// 欠いた <C>Examples は、走らせる関数に渡すところで、コンパイルできない。
func TestIncompleteCasesDoNotCompile(t *testing.T) {
	root, files := emitFixture(t, "unittest")
	writeFiles(t, root, files)
	impl := filepath.Join(root, "impl", "impl.go")

	// すべてのメソッドを持つ型は、コンパイルできる（下の失敗が、ほかの理由でないこと）。ない側のケース Absent の例は、要らない
	writeFile(t, impl, casesImpl(""))
	if out, err := runGo(t, root, "vet", "./impl"); err != nil {
		t.Fatalf("すべてのケースを持つ実装が、コンパイルできない: %v\n%s", err, out)
	}
	for _, tt := range []struct{ omit, what string }{
		{"leafCases.Submit", "葉 submit のケース"},
		{"leafCases.Find", "葉 find のケース"},
		{"submitCases.OK", "葉 submit の、正常のケース"},
		{"submitCases.Rejected", "葉 submit の、例外 app.Rejected のケース"},
		{"submitCases.StockBusy", "葉 submit の、例外 stock.Busy のケース"},
		{"stageExamples.Sent", "分類 app.Stage の、ケース Sent の例"},
		{"lifeExamples.Closed", "分類 app.Life の、ケース Closed の例"},
	} {
		method := tt.omit[strings.IndexByte(tt.omit, '.')+1:]
		writeFile(t, impl, casesImpl(tt.omit))
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

// writeBehavior は、入力 unittest の出力と、その behavior_test.go を、一時のモジュールに置いて、ルートを返す。
func writeBehavior(t *testing.T) string {
	t.Helper()
	root, files := emitFixture(t, "unittest")
	writeFiles(t, root, files)
	test, err := os.ReadFile(filepath.Join("testdata", "unittest", "behavior_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "behavior_test.go"), string(test))
	return root
}

// TestUnitTestNames は、走らせる関数が付けるテストの名前（ラベル）が、<op の FQN>/<結果> と、<分類の FQN>/<ケース> であること
// （同「葉の op」「分類の判定」。docs/ide-chain.md「テストの規約」）。ない側のケースのテストは、例なしで、基底が作る。
func TestUnitTestNames(t *testing.T) {
	root := writeBehavior(t)
	out, err := runGo(t, root, "test", "-count=1", "-v", "-run", "^(TestLeafCases|TestStage|TestLife|TestLookup|TestLevel)$", ".")
	if err != nil {
		t.Fatalf("口を満たす型のテストが落ちた: %v\n%s", err, out)
	}
	for _, name := range []string{
		"TestLeafCases/app.submit/OK", "TestLeafCases/app.submit/Rejected", "TestLeafCases/app.submit/AppBusy", "TestLeafCases/app.submit/StockBusy",
		"TestLeafCases/app.log/OK", "TestLeafCases/app.send/OK", "TestLeafCases/app.refill/OK", "TestLeafCases/app.reopen/OK", "TestLeafCases/app.find/OK",
		"TestStage/app.Stage/Draft", "TestStage/app.Stage/Sent", "TestStage/app.Stage/Done",
		"TestLife/app.Life/Open", "TestLife/app.Life/Closed", "TestLife/app.Life/Absent",
		"TestLookup/app.Lookup/Hit", "TestLookup/app.Lookup/Stale", "TestLookup/app.Lookup/Miss",
		"TestLevel/stock.Level/Plenty", "TestLevel/stock.Level/Few", "TestLevel/stock.Level/Out",
	} {
		if !strings.Contains(out, "--- PASS: "+name+" ") {
			t.Errorf("テスト %s が、通っていない（名前が違うか、走っていない）:\n%s", name, out)
		}
	}
	if n := strings.Count(out, "    --- PASS: "); n != 9+3+3+3+3+6+4 {
		// 9 + 12 は、結果とケースのテスト。6 + 4 は、その親（op ごと、分類ごと）
		t.Errorf("通ったサブテストの数 = %d, want %d:\n%s", n, 9+3+3+3+3+6+4, out)
	}
}

// unittestFaults は、口が落とすはずのテスト（入力 unittest の behavior_test.go の型を使う）。それぞれ、結果が、ケースのものと違う。
const unittestFaults = `package behavior

import (
	"context"
	"errors"
	"testing"

	"example.com/app/gen/app"
	"example.com/app/gen/app/apptest"
	"example.com/app/gen/stock"
)

// faulty は、言ったとおりに動かない葉: submit は、いつも正常に終わる。log は、いつも失敗する。send は、Draft のまま返す。
// refill は、数を 0 にして返す（Out）。reopen は、closed にして返す。
type faulty struct{ leaves }

func (faulty) Submit(_ context.Context, order app.Order, then string) (app.Order, error) {
	order.Note = then
	return order, nil
}
func (faulty) Log(context.Context, string) error { return errors.New("disk full") }
func (faulty) Send(_ context.Context, order app.Order) (app.Order, error) { return order, nil }
func (faulty) Refill(_ context.Context, item stock.Item) (stock.Item, error) {
	item.Count = 0
	return item, nil
}
func (faulty) Reopen(_ context.Context, order app.Order) (app.Order, error) {
	order.Note = "closed"
	return order, nil
}

func TestFaultyLeaves(t *testing.T) {
	thens = nil
	t.Cleanup(func() { t.Logf("thens=%v", thens) })
	apptest.RunLeaves(t, faulty{}, life{}, stage{}, level{}, leafCases{})
}

// 例が、ケースに当たらない: Draft の例が Sent、Open の例が closed
type wrongStage struct{ stageExamples }

func (wrongStage) Draft() app.Order { return app.Order{Note: "sent"} }

type wrongLife struct{ lifeExamples }

func (wrongLife) Open() *app.Order { return &app.Order{Note: "closed"} }

func TestWrongStage(t *testing.T) { apptest.RunStage(t, stage{}, wrongStage{}) }
func TestWrongLife(t *testing.T)  { apptest.RunLife(t, life{}, wrongLife{}) }

// 述語が、どれも真にならない値: 判定関数の誤りで、落ちる
type noStage struct{ stageExamples }

func (noStage) Done() app.Order { return app.Order{Note: "?"} }

func TestNoStage(t *testing.T) { apptest.RunStage(t, stage{}, noStage{}) }
`

// TestUnitTestSeamFails は、結果が、ケースのものと違えば、走らせる関数が、そのケースのテストを落とすこと（同「葉の op」の
// 「走らせる側が確かめること」、「分類の判定」）: 例外のケースで、失敗がないか、別の失敗。正常のケースで、失敗がある。絞り込んだ
// 戻り値が、言ったケースにない。例が、そのケースに当たらない。落ちたケースの Then は、呼ばれない。
func TestUnitTestSeamFails(t *testing.T) {
	root := writeBehavior(t)
	writeFile(t, filepath.Join(root, "faults_test.go"), unittestFaults)
	out, err := runGo(t, root, "test", "-count=1", "-v", "-run", "^(TestFaultyLeaves|TestWrongStage|TestWrongLife|TestNoStage)$", ".")
	if err == nil {
		t.Fatalf("落ちるはずのテストが、通った:\n%s", out)
	}
	for _, name := range []string{
		"TestFaultyLeaves/app.submit/Rejected", "TestFaultyLeaves/app.submit/AppBusy", "TestFaultyLeaves/app.submit/StockBusy",
		"TestFaultyLeaves/app.log/OK", "TestFaultyLeaves/app.send/OK", "TestFaultyLeaves/app.refill/OK", "TestFaultyLeaves/app.reopen/OK",
		"TestWrongStage/app.Stage/Draft", "TestWrongLife/app.Life/Open", "TestNoStage/app.Stage/Done",
	} {
		if !strings.Contains(out, "--- FAIL: "+name+" ") {
			t.Errorf("テスト %s が、落ちていない:\n%s", name, out)
		}
	}
	// 言ったとおりの結果のケースは、同じ走りの中でも、通る
	for _, name := range []string{
		"TestFaultyLeaves/app.submit/OK", "TestFaultyLeaves/app.find/OK",
		"TestWrongStage/app.Stage/Sent", "TestWrongLife/app.Life/Closed", "TestWrongLife/app.Life/Absent", "TestNoStage/app.Stage/Draft",
	} {
		if !strings.Contains(out, "--- PASS: "+name+" ") {
			t.Errorf("テスト %s が、通っていない:\n%s", name, out)
		}
	}
	for _, msg := range []string{
		"want exception app.Rejected, got <nil>",
		"want exception app.Busy, got <nil>",
		"want exception stock.Busy, got <nil>",
		"want no error, got disk full",
		"want case Sent of app.Stage, got Draft",
		"want case Plenty or Few of stock.Level, got Out",
		"want case Open of app.Life, got Closed",
		"want case Draft, got Sent",
		"want case Open, got Closed",
		"want case Done, got error ",
	} {
		if !strings.Contains(out, msg) {
			t.Errorf("落ちたテストの文面に、%q がない:\n%s", msg, out)
		}
	}
	// 落ちたケースの Then は、呼ばれない: 呼ばれたのは、通った submit/OK と find/OK だけ
	if want := "thens=[submit/OK find/OK]"; !strings.Contains(out, want) {
		t.Errorf("呼ばれた Then が、%s でない:\n%s", want, out)
	}
}

// TestUnitTestSplitOutput は、分けた出力（docs/plugin-go.md「内側のフォルダと入口」）でも、単体テストの口と、効果の入口の確かめが
// 同じに働くこと: 入力 unittest の葉 log を、内部の op にする。包む型は、内側のパッケージの states.go に出て、テストのパッケージは、
// 入口のパッケージだけを import する。同じ behavior_test.go が、入口の New と、テストのパッケージを通して、通る。
func TestUnitTestSplitOutput(t *testing.T) {
	views := loadViews(t, "unittest")
	internal := 0
	for _, v := range views {
		for i := range v.Program.Namespaces {
			for _, d := range v.Program.Namespaces[i].Decls {
				if o, ok := d.(*ir.Op); ok && o.Name == "app.log" {
					o.Exported = false
					internal++
				}
			}
		}
	}
	if internal == 0 {
		t.Fatal("op app.log がない")
	}
	root := newRoot(t)
	files, diags := emitViews(t, root, views)
	if len(diags) > 0 {
		t.Fatalf("診断 %v", diags)
	}
	if states := files["gen/app/inner/states.go"]; !strings.Contains(states, "type checkedOrdersState struct") {
		t.Errorf("包む型が、内側のパッケージの states.go にない:\n%s", states)
	}
	if strings.Contains(files["gen/app/states.go"], "checked") {
		t.Errorf("包む型が、入口のパッケージに出ている:\n%s", files["gen/app/states.go"])
	}
	for _, name := range []string{"gen/app/apptest/leaves.go", "gen/app/apptest/classify.go"} {
		content, ok := files[name]
		if !ok {
			t.Fatalf("%s がない", name)
		}
		if strings.Contains(content, modulePath+"/gen/app/inner") || !strings.Contains(content, "app \""+modulePath+"/gen/app\"") {
			t.Errorf("%s が、入口のパッケージだけを import していない:\n%s", name, content)
		}
	}
	if _, ok := files["gen/app/inner/apptest/leaves.go"]; ok {
		t.Error("テストのパッケージが、内側のフォルダの下にある")
	}
	writeFiles(t, root, files)
	test, err := os.ReadFile(filepath.Join("testdata", "unittest", "behavior_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "behavior_test.go"), string(test))
	if out, err := runGo(t, root, "vet", "./..."); err != nil {
		t.Fatalf("分けた出力が go vet を通らない: %v\n%s", err, out)
	}
	if out, err := runGo(t, root, "test", "-count=1", "."); err != nil {
		t.Fatalf("分けた出力の上で、振る舞いのテストが落ちた: %v\n%s", err, out)
	}
}
