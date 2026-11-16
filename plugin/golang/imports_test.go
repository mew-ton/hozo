package golang

import (
	"strings"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// docs/plugin-go.md「パッケージと import パス」の「import の名前」: import のブロックは、標準ライブラリ、空行、それ以外の順で、
// それぞれパスの辞書順。一つなら一行。名前がパスの最後の要素と違えば、名前を書く。
func TestImportsRender(t *testing.T) {
	im := &Imports{}
	if got := im.Render(); got != "" {
		t.Errorf("empty Render = %q", got)
	}
	if name := im.Add("time", "time"); name != "time" {
		t.Errorf("Add = %q", name)
	}
	if got := im.Render(); got != "import \"time\"\n" {
		t.Errorf("一つなら一行: %q", got)
	}
	im.Add("github.com/mew-ton/hozo/runtime", "runtime")
	im.Add("context", "context")
	im.Add("example.com/shop-server/gen/type", "pkgtype")
	im.Add("example.com/shop-server/gen/server/inner", "inner")
	want := "import (\n\t\"context\"\n\t\"time\"\n\n\t\"example.com/shop-server/gen/server/inner\"\n\tpkgtype \"example.com/shop-server/gen/type\"\n\t\"github.com/mew-ton/hozo/runtime\"\n)\n"
	if got := im.Render(); got != want {
		t.Errorf("Render = %q\nwant %q", got, want)
	}
	if im.Name("time") != "time" || im.Name("example.com/shop-server/gen/type") != "pkgtype" || im.Name("nope") != "" {
		t.Errorf("Name: %q %q %q", im.Name("time"), im.Name("example.com/shop-server/gen/type"), im.Name("nope"))
	}
	if got := strings.Join(im.Paths(), ","); got != "context,example.com/shop-server/gen/server/inner,example.com/shop-server/gen/type,github.com/mew-ton/hozo/runtime,time" {
		t.Errorf("Paths = %s", got)
	}
	if got := strings.Join(im.Names(), ","); got != "context,inner,pkgtype,runtime,time" {
		t.Errorf("Names = %s", got)
	}
	// 同じパスを二度足しても同じ名前
	if im.Add("time", "time") != "time" || len(im.Paths()) != 5 {
		t.Error("Add twice")
	}
}

// collisionView は、パッケージ名の当たる名前空間（a.store と b.store、標準ライブラリと同じ time、内側と同じ x.inner）の型を使う
// 見る側 server の View。
func collisionView() *ir.View {
	v := viewOf("server",
		namespace("server", with(op("server.f", nil), internal())),
		namespace("a.store", model("a.store.A")), namespace("b.store", model("b.store.B")),
		namespace("time", model("time.Slot")), namespace("x.inner", model("x.inner.I")),
	)
	for i, ns := range []string{"a.store", "b.store", "time", "x.inner"} {
		v.Namespaces[ns] = []ir.NamespaceConfig{conf(i+2, "gen/"+strings.ReplaceAll(ns, ".", "/"))}
	}
	return v
}

// 一つのファイルの中で名前が当たるときは、名前空間の FQN の辞書順に名前を決めていき、当たった方に 2、3、… を付ける。
// 標準ライブラリ、runtime、inner の名前は変えない。Canonical が、足した順によらず、その名前にする。
func TestImportsCollision(t *testing.T) {
	root := projectRoot(t)
	v := collisionView()
	l, err := LayoutOf(v, root)
	if err != nil || !l.HasInner {
		t.Fatalf("layout = %+v, %v", l, err)
	}
	// 入口のパッケージのファイル。型を書いた順は、辞書順と逆
	im := NewImports(v, l, root, false)
	for _, fqn := range []string{"x.inner.I", "time.Slot", "b.store.B", "a.store.A"} {
		GoType(ir.Ref(fqn), im)
	}
	GoType(ir.Time(), im) // 標準ライブラリの time を、名前空間 time の後で足す
	im.Add(l.DefPath, l.DefName)
	if err := im.Err(); err != nil {
		t.Fatal(err)
	}
	c := im.Canonical()
	for fqn, want := range map[string]string{
		"a.store.A": "store.A", "b.store.B": "store2.B", // FQN の辞書順で a.store が先
		"time.Slot": "time2.Slot", // 標準ライブラリの time の名前は変えない
		"x.inner.I": "inner2.I",   // 内側のパッケージ inner の名前は変えない
	} {
		if got := GoType(ir.Ref(fqn), c); got != want {
			t.Errorf("GoType(%s) = %q, want %q", fqn, got, want)
		}
	}
	if got := GoType(ir.Time(), c); got != "time.Time" {
		t.Errorf("GoType(Time) = %q", got)
	}
	got := c.Render()
	for _, line := range []string{
		"\t\"time\"\n\n",
		"\t\"example.com/app/gen/a/store\"\n",
		"\tstore2 \"example.com/app/gen/b/store\"\n",
		"\t\"example.com/app/gen/server/inner\"\n",
		"\ttime2 \"example.com/app/gen/time\"\n",
		"\tinner2 \"example.com/app/gen/x/inner\"\n",
	} {
		if !strings.Contains(got, line) {
			t.Errorf("Render に %q がない:\n%s", line, got)
		}
	}
	// 同じ import の集まりなら、Canonical は足した順によらず同じ
	im2 := NewImports(v, l, root, false)
	im2.Add(l.DefPath, l.DefName)
	GoType(ir.Time(), im2)
	for _, fqn := range []string{"a.store.A", "b.store.B", "time.Slot", "x.inner.I"} {
		GoType(ir.Ref(fqn), im2)
	}
	if im2.Canonical().Render() != got {
		t.Errorf("Canonical が足した順による:\n%s\n%s", im2.Canonical().Render(), got)
	}
	// 入口のファイルでは、内側のパッケージを import していなくても、inner の名前は使わせない
	im3 := NewImports(v, l, root, false)
	if got := GoType(ir.Ref("x.inner.I"), im3); got != "inner2.I" {
		t.Errorf("入口のファイルの x.inner: %q", got)
	}
	// 定義のパッケージ（内側）のファイルでは、当たるものがなければそのまま
	im4 := NewImports(v, l, root, true)
	if got := GoType(ir.Ref("x.inner.I"), im4); got != "inner.I" {
		t.Errorf("定義のファイルの x.inner: %q", got)
	}
}

// 基底が定義のパッケージに宣言する非公開の名前（New が返す構造体 ops、補助 ptr）は、import の名前に使わない: パッケージ名が
// それと同じ名前空間は、どのファイルでも（定義でも入口でも）2、3、… を付けて import する。
func TestImportsBasePrivateNames(t *testing.T) {
	root := projectRoot(t)
	v := viewOf("server",
		namespace("server", with(op("server.f", nil), internal())),
		namespace("ops", model("ops.Job")), namespace("more.ops", model("more.ops.Step")), namespace("ptr", model("ptr.Target")),
	)
	for i, ns := range []string{"ops", "more.ops", "ptr"} {
		v.Namespaces[ns] = []ir.NamespaceConfig{conf(i+2, "gen/"+strings.ReplaceAll(ns, ".", "/"))}
	}
	l, err := LayoutOf(v, root)
	if err != nil {
		t.Fatal(err)
	}
	for _, inDef := range []bool{true, false} {
		im := NewImports(v, l, root, inDef)
		for _, fqn := range []string{"ptr.Target", "ops.Job", "more.ops.Step"} {
			GoType(ir.Ref(fqn), im)
		}
		c := im.Canonical()
		for fqn, want := range map[string]string{
			"more.ops.Step": "ops2.Step", // FQN の辞書順で more.ops が先
			"ops.Job":       "ops3.Job",
			"ptr.Target":    "ptr2.Target",
		} {
			if got := GoType(ir.Ref(fqn), c); got != want {
				t.Errorf("inDef=%v: GoType(%s) = %q, want %q", inDef, fqn, got, want)
			}
		}
		if got := strings.Join(c.Names(), ","); got != "ops2,ops3,ptr2" {
			t.Errorf("inDef=%v: Names = %s", inDef, got)
		}
		if got := c.Render(); !strings.Contains(got, "\tops3 \"example.com/app/gen/ops\"\n") || !strings.Contains(got, "\tptr2 \"example.com/app/gen/ptr\"\n") {
			t.Errorf("inDef=%v: Render =\n%s", inDef, got)
		}
	}
}

// Reserve は、ファイルを書くトランスパイラが、関数の本体の中で決まった名前で使う識別子を、import の名前に使わせない。
// Canonical にも引き継ぐ。Reserve していないファイルの名前は変わらない。
func TestImportsReserve(t *testing.T) {
	root := projectRoot(t)
	v := viewOf("server",
		namespace("server", op("server.f", nil)),
		namespace("ctx", model("ctx.Info")), namespace("a.err", model("a.err.Fault")), namespace("err", model("err.Fault")),
	)
	for i, ns := range []string{"ctx", "a.err", "err"} {
		v.Namespaces[ns] = []ir.NamespaceConfig{conf(i+2, "gen/"+strings.ReplaceAll(ns, ".", "/"))}
	}
	l, err := LayoutOf(v, root)
	if err != nil {
		t.Fatal(err)
	}
	im := NewImports(v, l, root, true)
	im.Reserve("o", "ctx", "err")
	for _, fqn := range []string{"err.Fault", "ctx.Info", "a.err.Fault"} {
		GoType(ir.Ref(fqn), im)
	}
	for _, c := range []*Imports{im.Canonical(), im.Canonical().Canonical()} {
		for fqn, want := range map[string]string{"ctx.Info": "ctx2.Info", "a.err.Fault": "err2.Fault", "err.Fault": "err3.Fault"} {
			if got := GoType(ir.Ref(fqn), c); got != want {
				t.Errorf("GoType(%s) = %q, want %q", fqn, got, want)
			}
		}
		if got := strings.Join(c.Names(), ","); got != "ctx2,err2,err3" {
			t.Errorf("Names = %s（Reserve した名前そのものは、import の名前ではない）", got)
		}
	}
	// Reserve していないファイルでは、そのままの名前
	plain := NewImports(v, l, root, true)
	if got := GoType(ir.Ref("ctx.Info"), plain); got != "ctx.Info" {
		t.Errorf("Reserve していないファイル: %q", got)
	}
}

// Qualifier は、別の名前空間の宣言を書くときの前置き。N の宣言は修飾なし。置き場がなければ Err。
func TestImportsQualifier(t *testing.T) {
	root := projectRoot(t)
	v := typesView()
	l, _ := LayoutOf(v, root)
	im := NewImports(v, l, root, true)
	if got := im.Qualifier("server"); got != "" {
		t.Errorf("Qualifier(viewer) = %q", got)
	}
	if got := im.Qualifier("type"); got != "pkgtype." || im.Err() != nil {
		t.Errorf("Qualifier(type) = %q, %v", got, im.Err())
	}
	// 置き場のない名前空間: 修飾なしの名前を返し、Err が報告する（CFG-22 が通したはずのもの）
	v.Program.Namespaces = append(v.Program.Namespaces, namespace("lost", model("lost.M")))
	im = NewImports(v, l, root, true)
	if got := GoType(ir.Ref("lost.M"), im); got != "M" || im.Err() == nil || !strings.Contains(im.Err().Error(), "lost") {
		t.Errorf("置き場がない: %q, %v", got, im.Err())
	}
}
