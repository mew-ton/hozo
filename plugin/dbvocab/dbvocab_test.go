package dbvocab

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// このファイルは、db の語彙の、書いたものの整合の検査（docs/plugin-db.md「診断」の DB-1〜DB-19）と、読み解きの結果（同「表」
// 「キーと参照」「read」「効果」）の単体テスト（docs/testing.md「単体テスト」の `plugin/dbvocab`）。
//
// 入力は testdata/<名前>/ の .hozo。このパッケージは internal/ を import できない（docs/packages.md「依存の向き」）ので、
// .hozo を自分では解析しない: hozoc が語彙のプラグインに渡すのと同じ仕様（docs/plugin-protocol.md「read」の spec）を、
// tests/pluginviews_test.go の TestPluginSpecFixtures が testdata/<名前>/spec.json に置き、ここではそれを読む。
// 期待する診断は testdata/<名前>/expected/errors.txt（一行に、コード、位置、メッセージをタブで区切って。返した順）。

// loadSpec は、入力 name の仕様（testdata/<name>/spec.json）。
func loadSpec(t *testing.T, name string) *ir.Spec {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name, "spec.json"))
	if err != nil {
		t.Fatalf("%v（go test ./tests -run TestPluginSpecFixtures -update で作る）", err)
	}
	var spec ir.Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.Program == nil {
		t.Fatalf("testdata/%s/spec.json に program がない", name)
	}
	return &spec
}

// codes は、spec/diagnostics/db.hozo の ID（DB-1〜DB-19。欠番はない）。入力は testdata/db<n>/ で、DB-<n> の診断だけを返す。
const codes = 19

// TestDiagnostics は docs/plugin-db.md「診断」の DB-1〜DB-19: どの ID も入力を一つ持ち、診断のコード・位置・メッセージが
// ゴールデンと同じこと。入力は、その ID の診断だけを返す。同じ仕様から、同じ診断を返すこと（決定性）も確かめる。
func TestDiagnostics(t *testing.T) {
	for n := 1; n <= codes; n++ {
		name, code := "db"+strconv.Itoa(n), "DB-"+strconv.Itoa(n)
		t.Run(name, func(t *testing.T) {
			diags := Check(loadSpec(t, name).Program)
			if len(diags) == 0 {
				t.Fatal("診断がない")
			}
			var b bytes.Buffer
			for _, d := range diags {
				if d.Code != code {
					t.Errorf("コード %s の診断がある（%s だけのはず）: %s", d.Code, code, d.Message)
				}
				b.WriteString(d.Code + "\t" + d.At + "\t" + d.Message + "\n")
			}
			path := filepath.Join("testdata", name, "expected", "errors.txt")
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if b.String() != string(want) {
				t.Errorf("%s と違う:\n--- 期待\n%s--- 実際\n%s", path, want, b.String())
			}
			if again := Check(loadSpec(t, name).Program); !reflect.DeepEqual(diags, again) {
				t.Error("二度目の検査で、診断が変わった")
			}
		})
	}
}

// TestClean は「対象」: 正しい使い方は、診断にならない。デコレータのない read と効果、@db.transaction、表の model でない model の
// フィールドのデコレータは、確かめない。名前空間ごとに、その名前空間の表の状態で見る。
func TestClean(t *testing.T) {
	p := loadSpec(t, "clean").Program
	if diags := Check(p); len(diags) != 0 {
		t.Fatalf("診断になった: %+v", diags)
	}
	if Check(nil) != nil {
		t.Error("仕様がないのに、診断がある")
	}
	if r := ReadNamespace(p, "nowhere"); r.Marked != 0 || len(r.Tables) != 0 || len(r.Errors) != 0 {
		t.Errorf("ない名前空間の読み解きが、空でない: %+v", r)
	}
	if r := ReadNamespace(nil, "shop"); len(r.Tables) != 0 || len(r.Errors) != 0 {
		t.Errorf("仕様のない読み解きが、空でない: %+v", r)
	}
}

// TestReading は「表」「キーと参照」「read」「効果」: 読み解きの結果（go-sqlite が、出力を作るのに使う）。
func TestReading(t *testing.T) {
	p := loadSpec(t, "clean").Program
	r := ReadNamespace(p, "shop")
	if r.Name != "shop" || r.Marked != 2 {
		t.Fatalf("Name = %q, Marked = %d", r.Name, r.Marked)
	}
	// 表の状態は宣言順。表の名前は、状態の名前。表の model は、状態の要素の型
	var tables, models []string
	for _, x := range r.Tables {
		tables = append(tables, x.Name+"="+x.State.Name+":"+x.Model.Decl.Name)
	}
	for _, m := range r.Models {
		models = append(models, m.Decl.Name)
	}
	if want := []string{"users=shop.users:shop.User", "orders=shop.orders:shop.Order"}; !reflect.DeepEqual(tables, want) {
		t.Errorf("表の状態 = %v, want %v", tables, want)
	}
	if want := []string{"shop.User", "shop.Order"}; !reflect.DeepEqual(models, want) {
		t.Errorf("表の model = %v, want %v", models, want)
	}
	if r.TableOf("shop.drafts") != nil || r.ModelOf("shop.Draft") != nil || r.TableOf("shop.users") != r.Tables[0] || r.ModelOf("shop.Order") != r.Models[1] {
		t.Error("TableOf / ModelOf が、表の状態と表の model だけを返さない")
	}

	// 列: フィールドの順。値がなくてよいか、キー、一意、参照の先（しまい方は、読まない）
	user, order := r.Models[0], r.Models[1]
	type col struct {
		name                string
		nullable, key, uniq bool
		ref                 string
	}
	cols := func(m *Model) []col {
		var out []col
		for _, c := range m.Columns {
			out = append(out, col{c.Name, c.Nullable, c.Key, c.Unique, c.Ref + ">" + c.RefTable + "." + c.RefColumn})
		}
		return out
	}
	if got, want := cols(user), []col{
		{"id", false, true, false, ">."},
		{"email", false, false, true, ">."},
		{"nickname", true, false, false, ">."},
		{"age", false, false, false, ">."},
		{"joinedAt", false, false, false, ">."},
		{"tags", false, false, false, ">."},
	}; !reflect.DeepEqual(got, want) {
		t.Errorf("shop.User の列 = %+v, want %+v", got, want)
	}
	if got, want := cols(order)[1], (col{"userId", false, false, false, "shop.User>users.id"}); got != want {
		t.Errorf("shop.Order.userId = %+v, want %+v", got, want)
	}
	if keys := user.Keys(); len(keys) != 1 || keys[0] != user.Column("id") || user.Column("nowhere") != nil {
		t.Errorf("shop.User のキー = %+v", keys)
	}

	// read: デコレータの付いたものだけ。条件は、引数の後ろに @db.where
	if len(r.Reads) != 4 || r.Reads["shop.allDrafts"] != nil {
		t.Fatalf("read = %d 個（4 のはず。デコレータのない read は、読まない）", len(r.Reads))
	}
	paid := r.Reads["shop.paidOrdersOf"]
	if !paid.OK || paid.Kind != List || paid.Table != r.Tables[1] || paid.Latest != order.Column("placedAt") || len(paid.Apps) != 3 {
		t.Fatalf("shop.paidOrdersOf = %+v", paid)
	}
	if len(paid.Conds) != 2 || paid.Conds[0].Column != order.Column("userId") || paid.Conds[0].Param == nil || paid.Conds[0].Literal != nil ||
		paid.Conds[1].Column != order.Column("paid") || paid.Conds[1].Param != nil || paid.Conds[1].Literal != (ir.BooleanLiteral{Value: true}) {
		t.Errorf("shop.paidOrdersOf の条件 = %+v", paid.Conds)
	}
	if latest := r.Reads["shop.latestOrderOf"]; !latest.OK || latest.Kind != Find || latest.Latest == nil {
		t.Errorf("shop.latestOrderOf = %+v", latest)
	}

	// 効果: 効果のデコレータを、ちょうど一つ付けたものだけ
	if len(r.Effects) != 3 || r.Effects["shop.addDraft"] != nil {
		t.Fatalf("効果 = %d 個（3 のはず）", len(r.Effects))
	}
	if x := r.Effects["shop.createUser"]; !x.OK || x.Kind != Insert || x.Table != r.Tables[0] || x.Conflict != "shop.EmailTaken" || len(x.Conds) != 0 {
		t.Errorf("shop.createUser = %+v", x)
	}
	if x := r.Effects["shop.saveUser"]; !x.OK || x.Kind != Update || x.Conflict != "" {
		t.Errorf("shop.saveUser = %+v", x)
	}
	if x := r.Effects["shop.removeOrder"]; !x.OK || x.Kind != Delete || len(x.Conds) != 1 || x.Conds[0].Column != order.Column("id") {
		t.Errorf("shop.removeOrder = %+v", x)
	}

	// 別の名前空間は、その名前空間の表の状態で見る: 同じ model を、表の model にしてもよい
	plan := ReadNamespace(p, "plan")
	if plan.Marked != 2 || len(plan.Models) != 2 || plan.Models[1].Decl.Name != "shop.User" || plan.TableOf("shop.users") != nil || len(plan.Reads) != 1 {
		t.Errorf("plan の読み解き = %+v", plan)
	}
}

// TestUnreadable は、診断を返した宣言の、読み解きの結果: 食い違いのある read と効果は OK でなく、デコレータの言うことが決まらない
// もの（DB-8、DB-12、DB-13、DB-18）は、Reads と Effects に入らない。
func TestUnreadable(t *testing.T) {
	for name, want := range map[string]struct{ reads, effects int }{
		"db8":  {0, 0}, // 効果のデコレータを二つ以上持つ効果
		"db12": {0, 0}, // @db.find と @db.list の両方を持つ read
		"db13": {0, 0}, // @db.find も @db.list も持たない read
		"db18": {0, 0}, // @db.insert も @db.update も持たない、@db.conflict の効果
	} {
		p := loadSpec(t, name).Program
		reads, effects := 0, 0
		for i := range p.Namespaces {
			r := ReadNamespace(p, p.Namespaces[i].Name)
			for _, d := range r.Errors {
				if x := r.Reads[d.At]; x != nil {
					reads++
				}
				if x := r.Effects[d.At]; x != nil {
					effects++
				}
			}
		}
		if reads != want.reads || effects != want.effects {
			t.Errorf("%s: 診断を返した宣言のうち、Reads に %d、Effects に %d", name, reads, effects)
		}
	}
	// 食い違いのある read は、OK でない
	p := loadSpec(t, "db6").Program
	r := ReadNamespace(p, "app")
	for _, d := range r.Errors {
		if x := r.Reads[d.At]; x == nil || x.OK {
			t.Errorf("DB-6 を返した read %s が、OK", d.At)
		}
	}
}

// TestNames は「read」: 型の種類は、scalar を、元の核の型まで辿る。リテラルは、仕様での書き方で書く。
func TestNames(t *testing.T) {
	p := loadSpec(t, "clean").Program
	for typ, want := range map[ir.Type]string{
		ir.Ref("shop.Email"):                "string",
		ir.Type(ir.ArrayType{Of: ir.Int()}): "array",
		ir.Ref("shop.User"):                 "model",
		ir.Ref("shop.nowhere"):              "",
	} {
		if got := ClassOf(p, typ); got != want {
			t.Errorf("ClassOf(%s) = %q, want %q", typ, got, want)
		}
	}
	for lit, want := range map[ir.Literal]string{
		ir.StringLiteral{Value: "a\"b\n"}: `"a\"b\n"`,
		ir.IntLiteral{Value: 3}:           "3",
		ir.FloatLiteral{Value: 2}:         "2.0",
		ir.BooleanLiteral{Value: true}:    "true",
	} {
		if got := LiteralText(lit); got != want {
			t.Errorf("LiteralText(%v) = %s, want %s", lit, got, want)
		}
	}
}
