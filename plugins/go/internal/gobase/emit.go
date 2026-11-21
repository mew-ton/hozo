package gobase

import (
	"errors"
	"fmt"
	"go/format"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
)

// 診断のコード（docs/plugin-go.md「診断」）。
const (
	codeNoGoMod     = "GO-1"        // Dir(U, N) の上に go.mod がない
	codeCollision   = "GO-2"        // 生成する名前が当たる
	codeImportCycle = "GO-3"        // 出力どうしの import が循環する
	codeInnerDir    = "GO-4"        // 内側のフォルダが、子の名前空間 N.inner の出力先と重なる
	codeOption      = "GO-5"        // [emit.options.go] に知らないキーがある
	codeVariant     = "GO-6"        // variant の絞り込みの as の型と等しい型のアームが、union に二つ以上ある
	codeInternal    = "GO-INTERNAL" // 生成器の不具合
)

// runtimePath は、生成したコードが import する runtime パッケージ（docs/plugin-go.md「runtime パッケージ」）。
const runtimePath = "github.com/mew-ton/hozo/runtime"

// 基底のファイル名（docs/plugin-go.md「ファイル」）。
const (
	fileDoc      = "doc.go"
	fileTypes    = "types.go"
	fileOps      = "ops.go"
	fileStates   = "states.go"
	fileClassify = "classify.go"
	fileWiring   = "wiring.go"
)

// Emit は、要求の View の出力 (U, N) のファイルを作る（docs/plugin-go.md）。root はプロジェクトのルート（go.mod を探す起点）、
// stderr は、整形に失敗したときに整形前の内容を書く先。診断があれば、ファイルは返さない。ファイルはパスの辞書順。
func Emit(req *plugin.EmitRequest, root string, stderr io.Writer) ([]plugin.File, []plugin.Diagnostic) {
	view := req.View
	e := &emitter{view: view, root: root, stderr: stderr}
	if view.Program == nil || view.Program.Namespace(view.Viewer) == nil {
		e.errorf(codeInternal, "", "View に、名前空間 %s の Program がない", view.Viewer)
		return nil, e.errs
	}
	// GO-5: オプションはない
	var none struct{}
	unknown, err := plugin.Options(req, &none)
	if err != nil {
		e.errorf(codeOption, "", "[emit.options.go] を読めない: %v", err)
	}
	for _, key := range unknown {
		e.errorf(codeOption, "", "%d 番目の [[emit]] の [emit.options.go] に、知らないキー %s がある（go にオプションはない）", view.Config.Index, key)
	}
	// GO-1
	layout, err := golang.LayoutOf(view, root)
	if err != nil {
		if errors.Is(err, golang.ErrNoGoMod) {
			e.errorf(codeNoGoMod, "", "名前空間 %s の出力先 %q の上に go.mod がない（%v）", view.Viewer, view.Config.Dir, err)
		} else {
			e.errorf(codeInternal, "", "名前空間 %s の出力先 %q の import パスを求められない: %v", view.Viewer, view.Config.Dir, err)
		}
		return nil, e.errs
	}
	e.layout = layout
	kit, err := golang.TestKitOf(view, root)
	if err != nil {
		e.errorf(codeInternal, "", "名前空間 %s の、テストの中の組み立てに使う出力の import パスを求められない: %v", view.Viewer, err)
		return nil, e.errs
	}
	e.kit = kit
	e.prepare()
	e.checkInnerDir()
	e.checkTestDir()
	e.checkNames()
	e.checkTestNames()
	e.checkImportCycle()
	if len(e.errs) > 0 {
		return nil, e.errs
	}
	files := e.generate()
	if len(e.errs) > 0 {
		return nil, e.errs
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// emitter は一つの出力 (U, N) を作る状態。
type emitter struct {
	view   *ir.View
	root   string
	stderr io.Writer
	layout *golang.Layout
	shape  *golang.Shape
	// kit は、テストのにせものと、テストの中の組み立ての形（docs/plugin-go.md「テストのにせもの」）。
	kit *golang.TestKit
	p   *ir.Program
	ns  *ir.Namespace // N

	declNS map[string]string           // 宣言の FQN → 宣言した名前空間
	ops    map[string]golang.OpShape   // N の op（Shape.Ops）を FQN で引く
	units  map[string]golang.StateUnit // 状態の単位を FQN で引く
	// unitOf は、N の read と効果の FQN → それをメソッドに持つ状態の単位（read は読む状態の単位、効果は宛先の単位）。
	unitOf map[string]golang.StateUnit
	fields fieldNames // New の引数（= 構造体 ops のフィールド）の名前

	// reserved は、局所の名前が避ける名前（docs/plugin-go.md「名前」の「局所の名前」）: o、ctx、err、ptr と、定義のパッケージの
	// ops.go と wiring.go が import するパッケージの名前。
	reserved map[string]bool
	// imports は、ファイル（"def/ops.go"、"entry/ops.go" の形のキー）ごとの、名前を決めた import（一度目の生成の後で作る）。
	imports map[string]*golang.Imports

	errs []plugin.Diagnostic
	seen map[string]bool // 同じ診断を二度出さない（生成を二度走らせるため）
}

// fieldNames は、New の引数のうち、種類ごとに一つのものの名前と、分類の述語の引数の名前。
type fieldNames struct {
	leaves, seams string
	classify      map[string]string // 分類の FQN → 引数の名前
	deps          map[string]string // 別の名前空間の FQN → 引数の名前
}

func (e *emitter) errorf(code, at, format string, args ...any) {
	d := plugin.Diagnostic{Code: code, At: at, Message: fmt.Sprintf(format, args...)}
	key := d.Code + "\x00" + d.At + "\x00" + d.Message
	if e.seen == nil {
		e.seen = map[string]bool{}
	}
	if e.seen[key] {
		return
	}
	e.seen[key] = true
	e.errs = append(e.errs, d)
}

// prepare は、View から、生成の間ずっと使う表を作る。
func (e *emitter) prepare() {
	e.p = e.view.Program
	e.ns = e.p.Namespace(e.view.Viewer)
	e.shape = golang.ShapeOf(e.view)
	e.declNS = map[string]string{}
	for i := range e.p.Namespaces {
		for _, d := range e.p.Namespaces[i].Decls {
			if d == nil {
				continue
			}
			if _, dup := e.declNS[d.Common().Name]; !dup {
				e.declNS[d.Common().Name] = e.p.Namespaces[i].Name
			}
		}
	}
	e.ops = map[string]golang.OpShape{}
	for _, o := range e.shape.Ops {
		e.ops[o.FQN] = o
	}
	e.units = map[string]golang.StateUnit{}
	e.unitOf = map[string]golang.StateUnit{}
	for _, u := range e.shape.States {
		e.units[u.FQN] = u
		for _, fqn := range u.Reads {
			e.unitOf[fqn] = u
		}
		for _, fqn := range u.Effects {
			e.unitOf[fqn] = u
		}
	}
	// N の read と効果は、どれも、N の状態の単位のメソッド（docs/plugin-go.md「状態の単位」）。単位が決まらないものは、検査を通った
	// IR には来ない（read の宛先も、効果の宛先も、宣言した名前空間の状態）
	for _, d := range e.ns.Decls {
		switch d.(type) {
		case *ir.Read, *ir.Effect:
			if _, placed := e.unitOf[d.Common().Name]; !placed {
				e.errorf(codeInternal, d.Common().Name, "%s %s の宛先の状態の単位が、名前空間 %s にない（検査を通った IR には来ないはずの形）", d.Kind(), d.Common().Name, e.view.Viewer)
			}
		}
	}
	// New の引数の名前を、Shape.New の並び（leaves、状態の単位、分類の述語、別の名前空間、seams）から引く
	e.fields = fieldNames{classify: map[string]string{}, deps: map[string]string{}}
	i := 0
	next := func() string { name := e.shape.New[i].Name; i++; return name }
	if len(e.shape.Leaves) > 0 {
		e.fields.leaves = next()
	}
	i += len(e.shape.States)
	for _, fqn := range e.shape.Classify {
		e.fields.classify[fqn] = next()
	}
	for _, d := range e.shape.Deps {
		e.fields.deps[d.Namespace] = next()
	}
	if len(e.shape.Seams) > 0 {
		e.fields.seams = next()
	}
	e.reserved = baseReserved()
	e.imports = map[string]*golang.Imports{}
}

// baseReserved は、生成したコードがメソッドの中で使う名前（docs/plugin-go.md「名前」の「局所の名前」）。
func baseReserved() map[string]bool {
	return map[string]bool{"o": true, "ctx": true, "err": true, "ptr": true}
}

// lookup は FQN の宣言。なければ nil。
func (e *emitter) lookup(fqn string) ir.Decl { return e.p.Lookup(fqn) }

// op は FQN の op。op でなければ nil。
func (e *emitter) op(fqn string) *ir.Op {
	o, _ := e.lookup(fqn).(*ir.Op)
	return o
}

// read は FQN の read。read でなければ nil。
func (e *emitter) read(fqn string) *ir.Read {
	r, _ := e.lookup(fqn).(*ir.Read)
	return r
}

// effect は FQN の効果。効果でなければ nil。
func (e *emitter) effect(fqn string) *ir.Effect {
	x, _ := e.lookup(fqn).(*ir.Effect)
	return x
}

// inUnit は、N の op o が単位の対象か（docs/config.md「in」）。
func (e *emitter) inUnit(o *ir.Op) bool {
	for _, f := range e.view.Files {
		if f == o.Span.File {
			return true
		}
	}
	return false
}

// short は FQN の最後のセグメント。
func short(fqn string) string {
	if i := strings.LastIndexByte(fqn, '.'); i >= 0 {
		return fqn[i+1:]
	}
	return fqn
}

// typeName は宣言 fqn の Go の型の名前（修飾なし）。
func (e *emitter) typeName(fqn string) string { return golang.TypeName(fqn, e.declNS[fqn]) }

// checkInnerDir は GO-4: 内側のフォルダ inner が、同じ単位が出力する子の名前空間 N.inner の出力先と重なる。
// 位置は、名前空間の最初の内部の op。
func (e *emitter) checkInnerDir() {
	if !e.layout.HasInner {
		return
	}
	child := e.view.Viewer + ".inner"
	for _, c := range e.view.Namespaces[child] {
		if c.Index != e.view.Config.Index {
			continue
		}
		at := ""
		for _, o := range e.shape.Ops {
			if !o.Exported {
				at = o.FQN
				break
			}
		}
		e.errorf(codeInnerDir, at, "名前空間 %s は内部の op を持つので、定義を内側のフォルダ %s に置くが、同じ単位（%d 番目の [[emit]]）が出力する子の名前空間 %s の出力先と重なる",
			e.view.Viewer, e.layout.DefDir, c.Index, child)
		return
	}
}

// generate は、すべてのファイルを作る。import の名前と、局所の名前が避ける名前を決めるために、二度走らせる
// （docs/plugin-go.md「パッケージと import パス」の「import の名前」、「名前」の「局所の名前」）: 一度目で、ファイルごとの import の集まりを
// 知り、名前を文書の順で決め直す（golang.Imports.Canonical）。二度目は、その名前で作る。
func (e *emitter) generate() []plugin.File {
	e.build()
	if len(e.errs) > 0 {
		return nil
	}
	for key, im := range e.imports {
		e.imports[key] = im.Canonical()
	}
	e.reserved = baseReserved()
	for _, name := range []string{fileOps, fileWiring} {
		if im := e.imports["def/"+name]; im != nil {
			for _, n := range im.Names() {
				e.reserved[n] = true
			}
		}
	}
	return e.build()
}

// build は、ファイルを一通り作る。
func (e *emitter) build() []plugin.File {
	var files []plugin.File
	add := func(dir string, f *fileGen) {
		if f == nil {
			return
		}
		content, ok := f.render()
		if !ok {
			return
		}
		files = append(files, plugin.File{Path: path.Join(dir, f.name), Content: content})
	}
	def := e.layout.DefDir
	add(def, e.genDoc(true))
	add(def, e.genTypes())
	add(def, e.genOps())
	add(def, e.genStates())
	add(def, e.genClassify())
	add(def, e.genWiring())
	if e.layout.HasInner {
		entry := e.layout.Dir
		add(entry, e.genDoc(false))
		add(entry, e.genEntryTypes())
		add(entry, e.genEntryOps())
		add(entry, e.genEntryStates())
		add(entry, e.genEntryClassify())
	}
	// テストのパッケージ（docs/plugin-go.md「単体テストの口」）
	test := e.layout.TestDir
	add(test, e.genTestDoc())
	add(test, e.genTestLeaves())
	add(test, e.genTestClassify())
	add(test, e.genTestFakes())
	// テストの対応表（docs/plugin-go.md「テストの対応表」）: 出したテキストから作る
	var tests []plugin.TestEntry
	for _, f := range files {
		if path.Dir(f.Path) != test {
			continue
		}
		entries, err := golang.TestMap(f.Path, f.Content)
		if err != nil {
			e.errorf(codeInternal, "", "テストの対応表を作れない（生成器の不具合）: %v", err)
			continue
		}
		tests = append(tests, entries...)
	}
	if len(tests) > 0 {
		files = append(files, plugin.TestMapFile(test, TranspilerName, tests))
	}
	return files
}

// fileGen は生成するファイル一つ。
type fileGen struct {
	e     *emitter
	name  string
	key   string // "def/<name>" か "entry/<name>" か "test/<name>"
	inDef bool
	// inTest は、テストのパッケージのファイルか（docs/plugin-go.md「単体テストの口」）。
	inTest bool
	pkg    string
	doc    string // パッケージのコメント（doc.go だけ）
	im     *golang.Imports
	body   strings.Builder
	// usesPtr は、このファイルが補助 ptr を使ったか（wiring.go だけが使う。docs/plugin-go.md「値」）。
	usesPtr bool
}

// newFile は、定義のパッケージ（inDef）か、入口のパッケージのファイル name を作り始める。import は、一度目なら空から集め、
// 二度目なら、一度目で決めた名前を使う。
func (e *emitter) newFile(name string, inDef bool) *fileGen {
	f := &fileGen{e: e, name: name, inDef: inDef, pkg: e.layout.DefName, key: "def/" + name}
	if !inDef {
		f.pkg, f.key = e.layout.EntryName, "entry/"+name
	}
	f.im = e.imports[f.key]
	if f.im == nil {
		f.im = golang.NewImports(e.view, e.layout, e.root, inDef)
		f.im.Reserve(bodyNames...)
		e.imports[f.key] = f.im
	}
	return f
}

// bodyNames は、基底が、関数の本体の中で決まった名前で使う識別子のうち、本体が名前空間のパッケージの型を書きうるもの: メソッドの
// レシーバ o と、ctx、err。その名前の局所の変数が、同じ名前のパッケージを隠すので、基底のファイルでは、import の名前に使わない
// （docs/plugin-go.md「パッケージと import パス」の「import の名前」）。パッケージに宣言する ops と ptr は、plugin/golang が、
// どのファイルでも避ける。
var bodyNames = []string{"o", "ctx", "err"}

func (f *fileGen) printf(format string, args ...any) { fmt.Fprintf(&f.body, format, args...) }
func (f *fileGen) print(s string)                    { f.body.WriteString(s) }

// goType は型 t の、このファイルでの Go の型。
func (f *fileGen) goType(t ir.Type) string { return golang.GoType(t, f.im) }

// zero は型 t の零値の式。
func (f *fileGen) zero(t ir.Type) string { return golang.Zero(t, f.im) }

// ptr は、式 expr を補助 ptr で包み、このファイルが ptr を使ったことを覚える（docs/plugin-go.md「値」）。
func (f *fileGen) ptr(expr string) string {
	f.usesPtr = true
	return "ptr(" + expr + ")"
}

// render は、ヘッダ、package 節、import、本体をつなぎ、go/format で整形する（docs/plugin-go.md「ファイル」）。
// 整形に失敗したら GO-INTERNAL で、整形前の内容を標準エラーに出す。
func (f *fileGen) render() (string, bool) {
	if len(f.e.errs) > 0 {
		// 診断が出ている（GO-6 など）: ファイルは返さないので、整形もしない（途中で止めた本体を、整形の失敗として重ねて報告しない）
		return "", false
	}
	if err := f.im.Err(); err != nil {
		f.e.errorf(codeInternal, "", "%s: %v（CFG-22 が通したはずのもの）", path.Join(f.dir(), f.name), err)
		return "", false
	}
	var b strings.Builder
	b.WriteString("// Code generated by hozoc go " + plugin.Version + "; DO NOT EDIT.\n\n")
	b.WriteString(f.doc)
	b.WriteString("package " + f.pkg + "\n")
	if imports := f.im.Render(); imports != "" {
		b.WriteString("\n" + imports)
	}
	if body := strings.TrimRight(f.body.String(), "\n"); body != "" {
		b.WriteString("\n" + body + "\n")
	}
	src, err := format.Source([]byte(b.String()))
	if err != nil {
		f.e.errorf(codeInternal, "", "生成した %s を整形できない（生成器の不具合）: %v", path.Join(f.dir(), f.name), err)
		if f.e.stderr != nil {
			fmt.Fprintf(f.e.stderr, "---- %s（整形前）----\n%s\n", path.Join(f.dir(), f.name), b.String())
		}
		return "", false
	}
	return string(src), true
}

func (f *fileGen) dir() string {
	if f.inTest {
		return f.e.layout.TestDir
	}
	if f.inDef {
		return f.e.layout.DefDir
	}
	return f.e.layout.Dir
}

// genDoc は doc.go（docs/plugin-go.md「コメント」のパッケージの行）。def は、定義のパッケージのものか。
func (e *emitter) genDoc(def bool) *fileGen {
	f := e.newFile(fileDoc, def)
	if def && e.layout.HasInner {
		f.doc = fmt.Sprintf("// Package inner は名前空間 %s の定義: 型と、内部の op を含むすべての op。使う側は入口のパッケージ %s を import する。テストは、内部の op をここから呼ぶ。\n",
			e.view.Viewer, e.layout.EntryPath)
		return f
	}
	f.doc = comment(describe("Package "+e.layout.EntryName, e.ns.Description, "名前空間 "+e.view.Viewer+"。"))
	return f
}
