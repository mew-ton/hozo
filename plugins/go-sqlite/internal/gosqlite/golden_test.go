package gosqlite

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

// goldenFixtures は、出力をゴールデンと比べる入力（testdata/<名前>/）と、それが確かめる docs/plugin-go-sqlite.md の見出し。
// behavior は、生成したコードを本物の SQLite の上で動かすテスト testdata/<名前>/behavior_test.go を持つもの。
var goldenFixtures = []struct {
	name     string
	covers   string
	behavior bool
}{
	{name: "columns", covers: "「しまい方」の表の各行（六つのしまい方。核の型と scalar）、省略可能なフィールドの列（しまい方ごと）、「表」の制約（複数の @db.key、@db.unique、@db.ref）", behavior: true},
	{name: "reads", covers: "「read の中身」の各行（@db.find、@db.list、@db.where、@db.latest と、その組み合わせ。二つ以上の引数）、「列と引数の型」、「メソッドの形とコメント」の read の句", behavior: true},
	{name: "effects", covers: "「効果の中身」の表の各行（@db.insert、@db.update、一つと二つ以上の列の @db.delete）と、@db.conflict（補助 dbConflict）、「状態ごとの型」の、形で決まる命令 Post", behavior: true},
	{name: "transaction", covers: "「トランザクション」（継ぎ目のメソッドの二つの形、確定と取り消し、入れ子、同じ db の上の二つの名前空間、var _ Seams を出すとき）", behavior: true},
	{name: "partial", covers: "「状態ごとの型」の、デコレータを付けていない read と効果があるとき（var _ を出さない、付けていないものの句、warning を出さない。人が埋め込んで書き足し、Conn を通す）と、型を出さない単位", behavior: true},
	{name: "rows", covers: "「効果の中身」の、行の遷移を持つ効果（SQL は変わらない。コメントの「確かめない」の一文）と、デコレータを付けられない行の遷移の効果（人が書き足す）", behavior: true},
	{name: "names", covers: "「補助」の、当たる名前（引数、同じ名前の model、局所の名前と同じ名前のパッケージ）と、内側のフォルダを持つ出力", behavior: true},
	{name: "minimal", covers: "「補助」の、使うものだけを出すこと（read も効果も持たない表。出る補助と import が、いちばん少ない）"},
}

// TestGolden は、入力ごとに、出力（db.go と warning）がゴールデン testdata/<名前>/expected/ とバイト単位で同じこと、go/format を
// 通した形であること、同じ View から同じバイト列を返すこと（docs/plugin-protocol.md「emit」の決定性）、生成したコードが、基底の
// 出力と合わせてコンパイルできる（一時のモジュールで go vet を通る）ことを確かめる。
func TestGolden(t *testing.T) {
	for _, fx := range goldenFixtures {
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()
			root, res := emitFixture(t, fx.name)
			if len(res.files) == 0 {
				t.Fatal("ファイルを一つも出力しなかった")
			}
			checkFormatted(t, res.files)
			got := map[string]string{}
			for p, content := range res.files {
				got[p] = content
			}
			// warning は出さない: db の語彙のデコレータを付けていない read と効果は、仕様を書いた人が、中身を人が書くと選んだもの
			for _, w := range res.warnings {
				t.Errorf("warning を返した（%s）: %s", w.At, w.Message)
			}
			compareTree(t, fx.name, got)
			// 決定性: もう一度、別のルート（同じモジュールのパス）で出力しても、同じバイト列
			again := emitViews(t, newRoot(t), ours(t, loadViews(t, fx.name)))
			if len(again.files) != len(res.files) {
				t.Errorf("二度目の出力のファイルの数が違う: %d と %d", len(res.files), len(again.files))
			}
			for p, content := range res.files {
				if again.files[p] != content {
					t.Errorf("%s が、二度目の出力で変わった", p)
				}
			}
			if warningsText(again.warnings) != warningsText(res.warnings) {
				t.Error("warning が、二度目の出力で変わった")
			}
			vet(t, root, fx.name, res.files)
		})
	}
}

// TestBehavior は、生成したコードを、一時のモジュールで、本物の SQLite の上で動かす（docs/testing.md「単体テスト」）: 入力の
// behavior_test.go を、基底の出力と go-sqlite の出力を置いたモジュールのルートに置いて、go test を走らせる。ドライバは、
// サンプル shop-server と同じもの（addDriver）。確かめることは、それぞれのファイルの頭。
func TestBehavior(t *testing.T) {
	for _, fx := range goldenFixtures {
		if !fx.behavior {
			continue
		}
		t.Run(fx.name, func(t *testing.T) {
			t.Parallel()
			root, res := emitFixture(t, fx.name)
			addDriver(t, root)
			writeProject(t, root, fx.name, res.files)
			test, err := os.ReadFile(filepath.Join("testdata", fx.name, "behavior_test.go"))
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(root, "behavior_test.go"), string(test))
			out, err := runGo(t, root, "test", "-count=1", "-v", ".")
			if err != nil {
				t.Fatalf("生成したコードのテストが落ちた: %v\n%s", err, out)
			}
			if !strings.Contains(out, "--- PASS: Test") {
				t.Fatalf("生成したコードのテストが一つも走らなかった:\n%s", out)
			}
		})
	}
}

// TestSeamsUniform は「型に依らない形の確かめ」（docs/testing.md「単体テスト」、docs/requirements.md「出力を推論しない」の、op を包む側）:
// @db.transaction の継ぎ目のメソッドは、op のデコレータの分なので、op の中身にも、型にも依らない。入力 transaction の bank の継ぎ目の
// メソッド（引数と戻り値の型が、どれも違う: model、省略可能な model、配列、scalar、省略可能な値、別の名前空間の model。配線と葉）は、
// 名前と、基底の形の写しである引数と戻り値の型のほかは、同じテキスト。本体の形は、戻り値の有無の二つだけ。
func TestSeamsUniform(t *testing.T) {
	_, res := emitFixture(t, "transaction")
	src, ok := res.files["gen/bank/db.go"]
	if !ok {
		t.Fatal("gen/bank/db.go を出力しなかった")
	}
	signature := regexp.MustCompile(`^func \(s DbSeams\) (\w+)\(ctx context\.Context(.*), body func\(ctx context\.Context(.*)\) (.+)\) (.+) \{$`)
	const (
		returning = "\treturn dbTransaction(ctx, s.db, func(ctx context.Context) (RESULT, error) {\n\t\treturn body(ctxARGS)\n\t})\n}"
		void      = "\t_, err := dbTransaction(ctx, s.db, func(ctx context.Context) (struct{}, error) {\n\t\treturn struct{}{}, body(ctxARGS)\n\t})\n\treturn err\n}"
	)
	lines := strings.Split(src, "\n")
	forms := map[string]int{}
	for i, l := range lines {
		m := signature.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		name, params, results := m[1], m[2], m[5]
		if m[3] != params || m[4] != results {
			t.Errorf("%s の body の形が、メソッドの形（op の形）と違う: %s", name, l)
		}
		// 引数の並び（`, a T, b U`）から、名前だけを取り出す。型に `,` を含むもの（Record）は、この入力にない
		args := ""
		for _, p := range strings.Split(strings.TrimPrefix(params, ", "), ", ") {
			if p != "" {
				args += ", " + strings.Fields(p)[0]
			}
		}
		end := i + 1
		for lines[end] != "}" {
			end++
		}
		body := strings.Join(lines[i+1:end+1], "\n")
		want, form := strings.ReplaceAll(void, "ARGS", args), "戻り値がない"
		if results != "error" {
			result := strings.TrimSuffix(strings.TrimPrefix(results, "("), ", error)")
			want, form = strings.ReplaceAll(strings.ReplaceAll(returning, "ARGS", args), "RESULT", result), "戻り値がある"
		}
		if body != want {
			t.Errorf("%s（%s）の本体が、決まった形でない:\n%s\n---- 期待 ----\n%s", name, form, body, want)
		}
		forms[form]++
	}
	// bank の継ぎ目を持つ op は九つ: 戻り値のあるものが三つ（省略可能な model、配列、scalar）、ないものが六つ
	if forms["戻り値がある"] != 3 || forms["戻り値がない"] != 6 {
		t.Errorf("継ぎ目のメソッドの数が %v（期待は、戻り値がある 3、戻り値がない 6。入力が、形を変えていない）", forms)
	}
}

// TestIncomplete は、db の語彙のデコレータを付けていない read か効果のある型が、基底の単位を満たさないこと（docs/plugin-go-sqlite.md
// 「状態ごとの型」: 足りなければコンパイルエラーになる。warning は出さない）: 入力 partial の BooksTable（read newestBooks、countBooks と
// 効果 retitle、clearBooks を持たない）を lib.BooksState の位置に置いたファイルは、コンパイルできない。すべてを持つ StampsTable は、通る。
func TestIncomplete(t *testing.T) {
	root, res := emitFixture(t, "partial")
	writeProject(t, root, "partial", res.files)
	probe := func(unit, impl string) string {
		return "package probe\n\nimport \"example.com/app/gen/lib\"\n\nvar _ lib." + unit + " = lib." + impl + "{}\n"
	}
	writeFile(t, filepath.Join(root, "probe", "probe.go"), probe("StampsState", "StampsTable"))
	if out, err := runGo(t, root, "build", "./probe"); err != nil {
		t.Fatalf("すべてのメソッドを持つ型が、単位を満たさない: %v\n%s", err, out)
	}
	// Go が言うのは、足りないメソッドのうち、名前の順で最初のもの
	for impl, missing := range map[string]string{"BooksTable": "ClearBooks", "NotesTable": "FirstNote"} {
		unit := strings.TrimSuffix(impl, "Table") + "State"
		writeFile(t, filepath.Join(root, "probe", "probe.go"), probe(unit, impl))
		out, err := runGo(t, root, "build", "./probe")
		if err == nil {
			t.Fatalf("メソッドの足りない型 %s が、単位 %s を満たした（コンパイルできた）", impl, unit)
		}
		if !strings.Contains(out, "does not implement") || !strings.Contains(out, "missing method "+missing) {
			t.Errorf("%s のコンパイルエラーが、足りないメソッド %s を言っていない:\n%s", impl, missing, out)
		}
	}
}

// TestNoTables は、表の状態が一つもない名前空間（docs/plugin-go-sqlite.md「出力」）: ファイルを出さず、warning を一つ返す。
func TestNoTables(t *testing.T) {
	res := emitViews(t, newRoot(t), ours(t, loadViews(t, "none")))
	if len(res.files) != 0 || len(res.errors) != 0 {
		t.Fatalf("files=%v errors=%v, want どちらも空", sortedPaths(res.files), res.errors)
	}
	if len(res.warnings) != 1 {
		t.Fatalf("warning が %d 件（期待は 1）: %v", len(res.warnings), res.warnings)
	}
	compareTree(t, "none", map[string]string{"warnings.txt": warningsText(res.warnings)})
}

// TestOtherDir は、同じ名前空間を別の out に出力しても、ファイルの中身が、パスを除いて同じこと（db.go は、自分のパッケージの
// import パスを書かない）。
func TestOtherDir(t *testing.T) {
	view := ours(t, loadViews(t, "reads"))[0]
	root := newRoot(t)
	first := emitViews(t, root, []*ir.View{view})
	other := *view
	other.Config.Out, other.Config.Dir = "elsewhere/out", "elsewhere/out"
	second := emitViews(t, root, []*ir.View{&other})
	if len(first.files) != 1 || len(second.files) != 1 {
		t.Fatalf("ファイルの数が %d と %d（期待は、どちらも 1）", len(first.files), len(second.files))
	}
	if a, b := first.files[view.Config.Dir+"/"+FileName], second.files["elsewhere/out/"+FileName]; a == "" || a != b {
		t.Errorf("別の out の db.go が違う:\n%s", firstDiff(a, b))
	}
}

// diagnosticFixtures は、診断を返す入力（testdata/<名前>/）。期待する診断は testdata/<名前>/expected/errors.txt
// （一行に、コード、位置、メッセージをタブで区切って。View の順、その中は返した順）。docs/plugin-go-sqlite.md「診断」の表の行ごとに一つ
// （GOSQL-5、GOSQL-14、GOSQL-18、GOSQL-19、GOSQL-21、GOSQL-23 は欠番）。
var diagnosticFixtures = []struct{ name, code string }{
	{"gosql1", codeSQLElement},      // @db.table の状態の要素の型が、model でない
	{"gosql2", codeSQLShape},        // @db.table の状態の形が list でない
	{"gosql3", codeSQLColumn},       // どのしまい方でも列にできない型のフィールドがある
	{"gosql4", codeSQLCollision},    // 公開の名前が当たる（条件ごとに名前空間が一つ。当たらない名前空間には出ない）
	{"gosql6", codeSQLOption},       // [emit.options.go-sqlite] に、知らないキーがある
	{"gosql7", codeSQLTable},        // read の on の状態か、効果の宛先の状態が、N の表の状態でない
	{"gosql8", codeSQLName},         // 列を名指しするものが、表の model のフィールドにない
	{"gosql9", codeSQLParams},       // 引数が、デコレータの言うものと合わない
	{"gosql10", codeSQLReturns},     // read の結果の型が、デコレータの言うものと合わない
	{"gosql11", codeSQLTarget},      // 効果のデコレータが、効果の宛先と合わない
	{"gosql12", codeSQLEffect},      // 一つの効果が、効果のデコレータを二つ以上持つ
	{"gosql13", codeSQLUnique},      // @db.find の read が、一行に決まらない
	{"gosql15", codeSQLUpdate},      // @db.update の表の model が、キーか、キーでない列を持たない
	{"gosql16", codeSQLStorage},     // フィールドが、しまい方のデコレータを、ちょうど一つ持たない
	{"gosql17", codeSQLFit},         // しまい方が、フィールドの型に合わない
	{"gosql20", codeSQLRef},         // @db.ref の指す先が、一つに決まらない
	{"gosql22", codeSQLBoth},        // 一つの read が、@db.find と @db.list の両方を持つ
	{"gosql24", codeSQLNeither},     // @db.where か @db.latest の付いた read が、@db.find も @db.list も持たない
	{"gosql25", codeSQLTwice},       // 一つの read の条件が、同じ列を二度名指ししている
	{"gosql26", codeSQLLiteral},     // @db.where のリテラルが、列の型に渡せない
	{"gosql27", codeSQLLatest},      // @db.latest の列のしまい方が、@db.time / @db.int / @db.float のどれでもない
	{"gosql28", codeSQLDeleteAll},   // @db.delete の効果が、引数を一つも持たない
	{"gosql29", codeSQLTransaction}, // op に @db.transaction が付いているのに、N に表の状態が一つもない
	{"gosql30", codeSQLRow},         // @db.update の効果の行の遷移の、後のケースが、前のケースの並びにない
	{"gosql31", codeSQLConflictOn},  // @db.conflict を付けた効果が、@db.insert も @db.update も持たない
	{"gosql32", codeSQLConflictExc}, // @db.conflict の例外が、効果の throws にないか、引数を持つ
	{"gosql33", codeSQLThrows},      // db のデコレータで中身を作る効果の throws に、@db.conflict が名指ししていない例外がある
}

// TestDiagnostics は docs/plugin-go-sqlite.md「診断」の表の各行: 診断のコード・位置・メッセージがゴールデンと同じで、
// 診断を返した出力がファイルを返さないこと（emitViews が確かめる）。入力は、そのコードの診断だけを返す。
func TestDiagnostics(t *testing.T) {
	for _, fx := range diagnosticFixtures {
		t.Run(fx.name, func(t *testing.T) {
			res := emitViews(t, newRoot(t), ours(t, loadViews(t, fx.name)))
			if len(res.errors) == 0 {
				t.Fatal("診断がない")
			}
			// 診断を返すとき、warning は返さない（表の状態が一つもないことの warning は、GOSQL-29 のときは出さない）
			for _, w := range res.warnings {
				t.Errorf("warning を返した（%s）: %s", w.At, w.Message)
			}
			for _, d := range res.errors {
				if d.Code != fx.code {
					t.Errorf("コード %s の診断がある（%s だけのはず）: %s", d.Code, fx.code, d.Message)
				}
			}
			compareTree(t, fx.name, map[string]string{"errors.txt": errorsText(res.errors)})
		})
	}
}

// TestDiagnosedOutputsOnly は、診断が、条件に当たった出力にだけ出ること: GOSQL-4 の入力の、当たらない名前空間 clean は、
// ファイルを返す（model DbSeams を持つが、DbSeams を出さないので、当たらない）。GOSQL-29 の入力の、表の状態を持つ名前空間 ok も同じ。
func TestDiagnosedOutputsOnly(t *testing.T) {
	for fixture, clean := range map[string]string{"gosql4": "clean", "gosql29": "ok"} {
		for _, v := range ours(t, loadViews(t, fixture)) {
			res := emitViews(t, newRoot(t), []*ir.View{v})
			if ok := v.Viewer == clean; ok != (len(res.errors) == 0) || ok != (len(res.files) == 1) {
				t.Errorf("%s の %s: files=%d errors=%d", fixture, v.Viewer, len(res.files), len(res.errors))
			}
		}
	}
}

// TestNoGoMod は、Dir(U, N) の上に go.mod がないとき: 報告するのは基底（GO-1。docs/plugin-go.md「診断」）で、go-sqlite は、
// ファイルも診断も返さない（型のパッケージの import パスを求められないので、何も出せない）。
func TestNoGoMod(t *testing.T) {
	res := emitViews(t, t.TempDir(), ours(t, loadViews(t, "reads")))
	if len(res.files) != 0 || len(res.errors) != 0 || len(res.warnings) != 0 {
		t.Fatalf("files=%d errors=%v warnings=%v, want どれも空", len(res.files), res.errors, res.warnings)
	}
}

// TestMalformedView は、View が欠けている（Program がない、viewer の名前空間が Program にない）とき: 利用者の仕様の誤りではないので、
// 診断ではなく誤りを返す（プラグインは終了コード 1 で終わる。docs/plugin-protocol.md「起動」）。
func TestMalformedView(t *testing.T) {
	view := ours(t, loadViews(t, "reads"))[0]
	for name, v := range map[string]*ir.View{
		"Program がない":          {Version: ir.Version, Viewer: "shop", Config: view.Config},
		"viewer が Program にない": {Version: ir.Version, Viewer: "nowhere", Config: view.Config, Program: view.Program},
	} {
		if resp, err := Emit(request(v), newRoot(t)); err == nil {
			t.Errorf("%s: 誤りを返さない（応答 %+v）", name, resp)
		}
	}
}

// TestHandler は、プラグインの入口（Handler）: HOZO_PROJECT_ROOT をルートにして出力し、診断は応答の errors で、warning は
// warnings で返す（docs/plugin-protocol.md「emit」）。check と import は持たない。
func TestHandler(t *testing.T) {
	root := newRoot(t)
	h := Handler{Getenv: func(key string) string {
		if key == "HOZO_PROJECT_ROOT" {
			return root
		}
		return ""
	}}
	view := ours(t, loadViews(t, "reads"))[0]
	resp, err := h.Emit(request(view))
	if err != nil {
		t.Fatal(err)
	}
	want := emitViews(t, root, []*ir.View{view})
	if len(resp.Files) != 1 || len(resp.Errors) != 0 || len(resp.Warnings) != 0 {
		t.Fatalf("files=%d errors=%v warnings=%v", len(resp.Files), resp.Errors, resp.Warnings)
	}
	if f := resp.Files[0]; f.Path != "gen/shop/db.go" || want.files[f.Path] != f.Content {
		t.Errorf("%s が Emit の結果と違う", f.Path)
	}
	// 診断は errors で返し、ファイルは返さない
	resp, err = h.Emit(request(ours(t, loadViews(t, "gosql6"))[0]))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 0 || len(resp.Errors) != 2 || resp.Errors[0].Code != codeSQLOption {
		t.Errorf("files=%d errors=%+v, want GOSQL-6 が二つ", len(resp.Files), resp.Errors)
	}
	// warning は warnings で返す
	resp, err = h.Emit(request(ours(t, loadViews(t, "none"))[0]))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Files) != 0 || len(resp.Errors) != 0 || len(resp.Warnings) != 1 {
		t.Errorf("files=%d errors=%v warnings=%v, want warning が一つ", len(resp.Files), resp.Errors, resp.Warnings)
	}
	// ほかのトランスパイラの名前は unsupported
	other := request(view)
	other.Transpiler = "go"
	if _, err := h.Emit(other); err == nil {
		t.Error("知らない transpiler で、エラーにならない")
	}
	if _, err := h.Check(&plugin.CheckRequest{}); err != plugin.ErrUnsupported {
		t.Errorf("Check の err = %v, want ErrUnsupported", err)
	}
	if _, err := h.Import(&plugin.ImportRequest{}); err != plugin.ErrUnsupported {
		t.Errorf("Import の err = %v, want ErrUnsupported", err)
	}
}
