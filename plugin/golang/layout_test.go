package golang

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mew-ton/hozo/ir"
)

// docs/plugin-go.md「パッケージと import パス」: import パスは、Dir(U, N) から親へ向かって最初に見つかる go.mod の module に、
// そこからの相対パスを足したもの。見つからなければ ErrNoGoMod（GO-1）。
func TestImportPath(t *testing.T) {
	root := projectRoot(t)
	for dir, want := range map[string]string{
		"gen/server": "example.com/app/gen/server", "gen/server/inner": "example.com/app/gen/server/inner",
		".": "example.com/app", "./gen//type/": "example.com/app/gen/type",
	} {
		if got, err := ImportPath(root, dir); err != nil || got != want {
			t.Errorf("ImportPath(%q) = %q, %v, want %q", dir, got, err, want)
		}
	}
	// 途中に go.mod があれば、近い方
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "go.mod"), []byte("// c\nmodule \"example.com/sub\" // x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ImportPath(root, "sub/gen/a"); err != nil || got != "example.com/sub/gen/a" {
		t.Errorf("ImportPath(sub/gen/a) = %q, %v", got, err)
	}
	if _, err := ImportPath(t.TempDir(), "gen/a"); !errors.Is(err, ErrNoGoMod) {
		t.Errorf("go.mod がない: err = %v, want ErrNoGoMod", err)
	}
}

// docs/plugin-go.md「内側のフォルダと入口」: N の op に内部の op が一つでもある出力は、内側（<Dir>/inner、パッケージ inner）と
// 入口に分ける。一つもない出力（op を持たない、型だけの名前空間を含む）は分けない。
// テストのパッケージ（同「単体テストの口」の「テストのパッケージ」）は、分けても分けなくても、<Dir>/<入口のパッケージ名>test。
func TestLayoutOf(t *testing.T) {
	root := projectRoot(t)
	// 型だけの名前空間。パッケージ名は名前空間から決まり、予約語なら pkg を前に付ける
	v := viewOf("type", namespace("type", model("type.User")))
	l, err := LayoutOf(v, root)
	if err != nil {
		t.Fatal(err)
	}
	want := Layout{Namespace: "type", Dir: "gen/type", EntryPath: "example.com/app/gen/type", EntryName: "pkgtype",
		DefDir: "gen/type", DefPath: "example.com/app/gen/type", DefName: "pkgtype",
		// 型だけの名前空間は、型のパッケージを、分けない（docs/plugin-go.md「型のパッケージ」）: 入口と同じ
		TypesDir: "gen/type", TypesPath: "example.com/app/gen/type", TypesName: "pkgtype",
		TestDir: "gen/type/pkgtypetest", TestPath: "example.com/app/gen/type/pkgtypetest", TestName: "pkgtypetest"}
	if *l != want {
		t.Errorf("型だけの名前空間: %+v, want %+v", *l, want)
	}
	// すべての op が export: 分けない
	v = viewOf("store", namespace("store", op("store.find", nil), op("store.add", nil)))
	if l, _ = LayoutOf(v, root); l.HasInner || l.DefDir != "gen/store" || l.DefName != "store" {
		t.Errorf("すべて export: %+v", *l)
	}
	// 内部の op が一つでもあれば分ける。入口の名前は変わらない
	v = viewOf("server", namespace("server", op("server.login", nil), with(op("server.check", nil), internal())))
	l, _ = LayoutOf(v, root)
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
	if l, _ = LayoutOf(v, root); l.HasInner {
		t.Errorf("出さない内部の op で分けた: %+v", *l)
	}
	// 単位の対象の配線が呼んでいれば、本体のない op として出すので、分ける
	v.Program.Namespaces[0].Decls = append(v.Program.Namespaces[0].Decls, with(op("server.run", nil), wired(call("server.check"))))
	if l, _ = LayoutOf(v, root); !l.HasInner {
		t.Errorf("呼ばれている内部の op で分けていない: %+v", *l)
	}
	if _, err := LayoutOf(v, t.TempDir()); !errors.Is(err, ErrNoGoMod) {
		t.Errorf("go.mod がない: err = %v", err)
	}
}

// docs/config.md「型の置き場」: view の単位自身がその名前空間を出力するならその出力、そうでなければ、基底が同じで、
// その名前空間を出力する単位のうち、設定の順で最初のもの。
func TestHomeOf(t *testing.T) {
	root := projectRoot(t)
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
	l, ok, err := HomeOf(v, "type", root)
	if err != nil || !ok || l.Dir != "gen/type" || l.EntryPath != "example.com/app/gen/type" || l.EntryName != "pkgtype" || l.Namespace != "type" {
		t.Errorf("設定の順で最初の、同じ基底の単位: %+v, %v, %v", l, ok, err)
	}
	// 自分の単位（index が同じ）が出力していれば、そちら
	v.Namespaces["type"] = append(v.Namespaces["type"], other(3, "go", "gen/server-type"))
	if l, ok, _ = HomeOf(v, "type", root); !ok || l.Dir != "gen/server-type" {
		t.Errorf("自分の単位の出力: %+v", l)
	}
	// 同じ基底の出力がなければ、ない
	v.Namespaces["type"] = []ir.NamespaceConfig{other(1, "ts", "ts/type")}
	if l, ok, err = HomeOf(v, "type", root); ok || l != nil || err != nil {
		t.Errorf("置き場がない: %+v, %v, %v", l, ok, err)
	}
	if _, ok, _ = HomeOf(v, "nowhere", root); ok {
		t.Error("出力のない名前空間に置き場がある")
	}
	// 内部の op を持つ名前空間の置き場も、使う側が import するのは入口
	v.Namespaces["store"] = []ir.NamespaceConfig{other(2, "go", "gen/store")}
	if l, ok, _ = HomeOf(v, "store", root); !ok || !l.HasInner || l.EntryPath != "example.com/app/gen/store" || l.DefPath != "example.com/app/gen/store/inner" {
		t.Errorf("内部の op を持つ置き場: %+v", l)
	}
	// 見る側の名前空間なら LayoutOf と同じ
	self, ok, err := HomeOf(v, "server", root)
	want, _ := LayoutOf(v, root)
	if err != nil || !ok || *self != *want {
		t.Errorf("HomeOf(viewer) = %+v, want %+v", self, want)
	}
}
