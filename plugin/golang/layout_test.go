package golang

import (
	"errors"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// plugins/go/spec/go.hozo の Requirement.Layout の「パッケージと import パス」: import パスは、オプション module に、Dir(U, N) を `/` でつないだもの
// （Dir が `.` なら、module そのもの）。ファイル（go.mod）は、読まない。
func TestImportPath(t *testing.T) {
	for dir, want := range map[string]string{
		"gen/server": "example.com/app/gen/server", "gen/server/inner": "example.com/app/gen/server/inner",
		".": "example.com/app", "": "example.com/app", "./gen//type/": "example.com/app/gen/type",
	} {
		if got := ImportPath("example.com/app", dir); got != want {
			t.Errorf("ImportPath(%q) = %q, want %q", dir, got, want)
		}
	}
}

// plugins/go/hozo-plugin.tomlのオプション module: 出力の設定の、基底へのオプションから取る。書かれていない、文字列でない、
// 空なら ErrNoModule（GO-8）。
func TestModuleOf(t *testing.T) {
	c := conf(1, "gen/a")
	if got, err := ModuleOf(c); err != nil || got != "example.com/app" {
		t.Errorf("ModuleOf = %q, %v", got, err)
	}
	for name, options := range map[string]map[string]map[string]any{
		"オプションがない":     {},
		"module がない":   {"go": {}},
		"文字列でない":       {"go": {"module": 1}},
		"空":            {"go": {"module": ""}},
		"別のトランスパイラのもの": {"go-http": {"module": "example.com/app"}},
	} {
		c.Options = options
		if _, err := ModuleOf(c); !errors.Is(err, ErrNoModule) {
			t.Errorf("%s: err = %v, want ErrNoModule", name, err)
		}
	}
	// 基底の名前で引く
	c.Base, c.Options = "go2", map[string]map[string]any{"go2": {"module": "example.com/two"}}
	if got, err := ModuleOf(c); err != nil || got != "example.com/two" {
		t.Errorf("基底 go2: ModuleOf = %q, %v", got, err)
	}
}

// plugins/go/spec/go.hozo の Requirement.Layout の「内側のフォルダと入口」: N の op に内部の op が一つでもある出力は、内側（<Dir>/inner、パッケージ inner）と
// 入口に分ける。一つもない出力（op を持たない、型だけの名前空間を含む）は分けない。
// テストのパッケージ（同「単体テストの口」の「テストのパッケージ」）は、分けても分けなくても、<Dir>/<入口のパッケージ名>test。
func TestLayoutOf(t *testing.T) {
	// 型だけの名前空間。パッケージ名は名前空間から決まり、予約語なら pkg を前に付ける
	v := viewOf("type", namespace("type", model("type.User")))
	l, err := LayoutOf(v)
	if err != nil {
		t.Fatal(err)
	}
	want := Layout{Namespace: "type", Dir: "gen/type", EntryPath: "example.com/app/gen/type", EntryName: "pkgtype",
		DefDir: "gen/type", DefPath: "example.com/app/gen/type", DefName: "pkgtype",
		// 型だけの名前空間は、型のパッケージを、分けない（plugins/go/spec/go.hozo の Requirement.Layout の「型のパッケージ」）: 入口と同じ
		TypesDir: "gen/type", TypesPath: "example.com/app/gen/type", TypesName: "pkgtype",
		TestDir: "gen/type/pkgtypetest", TestPath: "example.com/app/gen/type/pkgtypetest", TestName: "pkgtypetest"}
	if *l != want {
		t.Errorf("型だけの名前空間: %+v, want %+v", *l, want)
	}
	// すべての op が export: 分けない
	v = viewOf("store", namespace("store", op("store.find", nil), op("store.add", nil)))
	if l, _ = LayoutOf(v); l.HasInner || l.DefDir != "gen/store" || l.DefName != "store" {
		t.Errorf("すべて export: %+v", *l)
	}
	// 内部の op が一つでもあれば分ける。入口の名前は変わらない
	v = viewOf("server", namespace("server", op("server.login", nil), with(op("server.check", nil), internal())))
	l, _ = LayoutOf(v)
	want = Layout{Namespace: "server", Dir: "gen/server", EntryPath: "example.com/app/gen/server", EntryName: "server", HasInner: true,
		DefDir: "gen/server/inner", DefPath: "example.com/app/gen/server/inner", DefName: "inner",
		// 関数（op）を持つ名前空間は、型のパッケージを、分ける
		HasTypes: true, TypesDir: "gen/server/types", TypesPath: "example.com/app/gen/server/types", TypesName: "types",
		TestDir: "gen/server/servertest", TestPath: "example.com/app/gen/server/servertest", TestName: "servertest"}
	if *l != want {
		t.Errorf("内部の op を持つ: %+v, want %+v", *l, want)
	}
	// 単位の対象でない内部の op は、呼ばれていなければ「N の op」でない（出さない）ので、分けない
	v = viewOf("server", namespace("server", op("server.login", nil), with(op("server.check", nil), internal(), inFile("batch.hozo"))))
	v.Files = []string{"server.hozo"}
	if l, _ = LayoutOf(v); l.HasInner {
		t.Errorf("出さない内部の op で分けた: %+v", *l)
	}
	// 単位の対象の配線が呼んでいれば、本体のない op として出すので、分ける
	v.Program.Namespaces[0].Decls = append(v.Program.Namespaces[0].Decls, with(op("server.run", nil), wired(call("server.check"))))
	if l, _ = LayoutOf(v); !l.HasInner {
		t.Errorf("呼ばれている内部の op で分けていない: %+v", *l)
	}
	v.Config.Options = map[string]map[string]any{}
	if _, err := LayoutOf(v); !errors.Is(err, ErrNoModule) {
		t.Errorf("module がない: err = %v", err)
	}
}

// docs/config.md「型の置き場」: view の単位自身がその名前空間を出力するならその出力、そうでなければ、基底が同じで、
// その名前空間を出力する単位のうち、設定の順で最初のもの。
func TestHomeOf(t *testing.T) {
	v := viewOf("server",
		namespace("server", op("server.f", ir.Ref("type.User"))),
		namespace("type", model("type.User")),
		namespace("store", with(op("store.hidden", nil), internal())),
	)
	v.Config.Index = 3
	other := func(index int, base, dir string) ir.NamespaceConfig {
		c := conf(index, dir)
		c.Base = base
		return c
	}
	// type を出力するのは、基底の違う単位 1、基底 go の単位 2 と 4
	v.Namespaces["type"] = []ir.NamespaceConfig{other(1, "ts", "ts/type"), other(2, "go", "gen/type"), other(4, "go", "gen/type4")}
	l, ok, err := HomeOf(v, "type")
	if err != nil || !ok || l.Dir != "gen/type" || l.EntryPath != "example.com/app/gen/type" || l.EntryName != "pkgtype" || l.Namespace != "type" {
		t.Errorf("設定の順で最初の、同じ基底の単位: %+v, %v, %v", l, ok, err)
	}
	// 自分の単位（index が同じ）が出力していれば、そちら
	v.Namespaces["type"] = append(v.Namespaces["type"], other(3, "go", "gen/server-type"))
	if l, ok, _ = HomeOf(v, "type"); !ok || l.Dir != "gen/server-type" {
		t.Errorf("自分の単位の出力: %+v", l)
	}
	// 同じ基底の出力がなければ、ない
	v.Namespaces["type"] = []ir.NamespaceConfig{other(1, "ts", "ts/type")}
	if l, ok, err = HomeOf(v, "type"); ok || l != nil || err != nil {
		t.Errorf("置き場がない: %+v, %v, %v", l, ok, err)
	}
	if _, ok, _ = HomeOf(v, "nowhere"); ok {
		t.Error("出力のない名前空間に置き場がある")
	}
	// 内部の op を持つ名前空間の置き場も、使う側が import するのは入口
	v.Namespaces["store"] = []ir.NamespaceConfig{other(2, "go", "gen/store")}
	if l, ok, _ = HomeOf(v, "store"); !ok || !l.HasInner || l.EntryPath != "example.com/app/gen/store" || l.DefPath != "example.com/app/gen/store/inner" {
		t.Errorf("内部の op を持つ置き場: %+v", l)
	}
	// 見る側の名前空間なら LayoutOf と同じ
	self, ok, err := HomeOf(v, "server")
	want, _ := LayoutOf(v)
	if err != nil || !ok || *self != *want {
		t.Errorf("HomeOf(viewer) = %+v, want %+v", self, want)
	}
}
