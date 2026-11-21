package gobase

import (
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin/golang"
)

// このファイルは、テストのパッケージの fakes.go（docs/plugin-go.md「テストのにせもの」）: 実装を受け取る口のにせものと、
// にせものの上に、この名前空間の Ops を組み立てる NewOps。形（何を、にせものにし、何を、本物で通すか）は、golang.TestKitOf が
// 決める（その組み立てを使う相乗りと、同じ値）。

const fileFakes = "fakes.go"

// fakeBodyNames は、fakes.go の本体が、決まった名前で使う識別子（docs/plugin-go.md「テストのにせもの」の「出すとき」）。
// import の名前に使わない。
var fakeBodyNames = []string{"f", "o", "ctx", "k", "body"}

// hasFakes は、fakes.go を出すか（docs/plugin-go.md「テストのにせもの」の「出すとき」）。
func (e *emitter) hasFakes() bool { return e.kit.HasFakes() }

// fakeMethod は、にせものにする口のメソッド一つ: 名前、引数（ctx を除く。フィールドの名前、局所の名前、Go の型）、戻り値の型
// （値を返さなければ ""）、口のメソッドの「名前(引数) 戻り値」。
type fakeMethod struct {
	name    string
	params  []ir.OpParam
	returns string
	sig     string
	// args は、本物を呼ぶときの引数の並び（ctx と、局所の名前）。
	args string
}

// fakeArgs は、呼ばれた引数を覚える、名前のない構造体の型と、その値: `struct{ User pkgtype.User }` と `struct{ User pkgtype.User }{user}`。
func (f *fileGen) fakeArgs(m fakeMethod, reserved map[string]bool) (typ, value string) {
	if len(m.params) == 0 {
		return "struct{}", "struct{}{}"
	}
	var fields, values []string
	for _, p := range m.params {
		fields = append(fields, golang.FieldName(p.Name)+" "+f.goType(p.Type))
		values = append(values, golang.LocalName(p.Name, reserved))
	}
	typ = "struct{ " + strings.Join(fields, "; ") + " }"
	return typ, typ + "{" + strings.Join(values, ", ") + "}"
}

// fakeField は、にせものの型のフィールドの型: 値を返すなら runtime.Fake[A, R]、返さないなら runtime.FakeVoid[A]。
func (f *fileGen) fakeField(m fakeMethod, reserved map[string]bool) string {
	rt := f.im.Add(runtimePath, "runtime")
	a, _ := f.fakeArgs(m, reserved)
	if m.returns == "" {
		return rt + ".FakeVoid[" + a + "]"
	}
	return rt + ".Fake[" + a + ", " + m.returns + "]"
}

// opMethod は、op o のメソッド（葉、別の名前空間の op）を、にせものにするメソッドにする。
func (e *emitter) opMethod(f *fileGen, o *ir.Op, reserved map[string]bool) fakeMethod {
	params, results := golang.Signature(o, f.im, reserved)
	m := fakeMethod{name: golang.MethodName(o.Name), params: o.Params, sig: method(golang.MethodName(o.Name), params, results)}
	if hasValue(o) {
		m.returns = f.goType(o.Returns)
	}
	as := []string{"ctx"}
	for _, p := range o.Params {
		as = append(as, golang.LocalName(p.Name, reserved))
	}
	m.args = strings.Join(as, ", ")
	return m
}

// unitMethods は、状態の単位 u のメソッド（docs/plugin-go.md「状態の単位」の並び）。
func (e *emitter) unitMethods(f *fileGen, u golang.StateUnit, reserved map[string]bool) []fakeMethod {
	ctx := "ctx " + f.im.Add("context", "context") + ".Context"
	var out []fakeMethod
	if st, ok := e.lookup(u.FQN).(*ir.State); ok {
		if st.Shape != ir.StateSingle {
			out = append(out, fakeMethod{name: "IsEmpty", returns: "bool", sig: "IsEmpty(" + ctx + ") (bool, error)"})
		}
		get := f.goType(st.Type)
		out = append(out, fakeMethod{name: "Get", returns: get, sig: "Get(" + ctx + ") (" + get + ", error)"})
		for _, operation := range u.Builtins {
			name := golang.BuiltinMethod(operation)
			params, results := golang.BuiltinSignature(st, operation, f.im)
			m := fakeMethod{name: name, sig: method(name, params, results)}
			if golang.BuiltinTakesValue(operation) {
				m.params = []ir.OpParam{{Name: ir.DirectEffectParam, Type: st.Element}}
			}
			out = append(out, m)
		}
	}
	for _, c := range u.Predicates {
		name := golang.Capitalize(c)
		out = append(out, fakeMethod{name: name, returns: "bool", sig: name + "(" + ctx + ") (bool, error)"})
	}
	for _, fqn := range u.Reads {
		r := e.read(fqn)
		params, results := golang.ReadSignature(r, f.im, reserved)
		out = append(out, fakeMethod{name: golang.MethodName(fqn), params: r.Params, returns: f.goType(r.Returns), sig: method(golang.MethodName(fqn), params, results)})
	}
	for _, fqn := range u.Effects {
		x := e.effect(fqn)
		params, results := golang.EffectSignature(x, f.im, reserved)
		out = append(out, fakeMethod{name: golang.MethodName(fqn), params: x.Params, sig: method(golang.MethodName(fqn), params, results)})
	}
	return out
}

// lowerFirst は、名前の先頭を小文字にする（口を満たす、非公開の型の名前）。
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// genFakeType は、にせものの型と、口を満たす非公開の型と、その確かめと、メソッドを書く。thing は「<もの>」、iface は、口の
// Go の型、fields は「<フィールドの言い方>」、unsaid は、言われていないときの振る舞いの文。wraps は、本物（impl）を包むか。
// impl は、メソッド m の、本物を呼ぶ式（包まないなら、使わない）。
func (e *emitter) genFakeType(f *fileGen, fake, thing, iface, fields, unsaid string, wraps bool, ms []fakeMethod, reserved map[string]bool, impl func(m fakeMethod) string) {
	f.print(comment(fake + " は、" + thing + "（" + iface + "）のにせもの。" + fields + "。\n" + unsaid))
	f.printf("type %s struct {\n", fake)
	for _, m := range ms {
		f.printf("\t%s %s\n", m.name, f.fakeField(m, reserved))
	}
	f.print("}\n\n")
	private := lowerFirst(fake)
	f.print(comment(private + " は、" + fake + " を、" + iface + " にする。"))
	if wraps {
		f.printf("type %s struct {\n\tf *%s\n\timpl %s\n}\n\n", private, fake, iface)
	} else {
		f.printf("type %s struct{ f *%s }\n\n", private, fake)
	}
	f.printf("var _ %s = %s{}\n\n", iface, private)
	for _, m := range ms {
		_, value := f.fakeArgs(m, reserved)
		real := "nil"
		if impl != nil {
			results := "error"
			if m.returns != "" {
				results = "(" + m.returns + ", error)"
			}
			real = "func() " + results + " { return " + impl(m) + " }"
		}
		f.printf("func (o %s) %s {\n\treturn o.f.%s.Call(%s, %s)\n}\n\n", private, m.sig, m.name, value, real)
	}
}

// genTestFakes はテストのパッケージの fakes.go。出さない出力（docs/plugin-go.md「テストのにせもの」の「出すとき」）なら nil。
func (e *emitter) genTestFakes() *fileGen {
	if !e.hasFakes() {
		return nil
	}
	k := e.kit
	s := e.shape
	f := e.newTestFile(fileFakes, fakeBodyNames)
	entry := f.im.Qualifier(e.view.Viewer)
	// 局所の名前が避ける名前: レシーバ o、ctx と、このファイルが import するパッケージの名前
	reserved := map[string]bool{"o": true, "ctx": true}
	for _, n := range f.im.Names() {
		reserved[n] = true
	}
	units := map[string]golang.StateUnit{}
	for _, u := range s.States {
		units[u.FQN] = u
	}
	deps := map[string]golang.Dep{}
	for _, d := range s.Deps {
		deps[d.Namespace] = d
	}
	const zero = "言われていないメソッドは、零値を返す。"
	for _, p := range k.Params {
		switch p.Kind {
		case golang.KitLeaves:
			var ms []fakeMethod
			for _, fqn := range s.Leaves {
				ms = append(ms, e.opMethod(f, e.op(fqn), reserved))
			}
			e.genFakeType(f, p.Fake, "人が実装する葉の op", entry+"Leaves", "葉ごとに、フィールド一つ", "言われていない葉は、本物（NewOps に渡したもの）を呼ぶ。", true, ms, reserved,
				func(m fakeMethod) string { return "o.impl." + m.name + "(" + m.args + ")" })
		case golang.KitState:
			u := units[p.FQN]
			e.genFakeType(f, p.Fake, "状態 "+p.FQN+" の単位", entry+u.TypeName, "メソッドごとに、フィールド一つ", zero, false, e.unitMethods(f, u, reserved), reserved, nil)
		case golang.KitClassify:
			e.genCasesFake(f, p)
		case golang.KitExternal:
			d := deps[p.FQN]
			var ms []fakeMethod
			for _, fqn := range d.Ops {
				ms = append(ms, e.opMethod(f, e.op(fqn), reserved))
			}
			e.genFakeType(f, p.Fake, "名前空間 "+p.FQN+" の op", entry+d.TypeName, "メソッドごとに、フィールド一つ", zero, false, ms, reserved, nil)
		case golang.KitSeams:
			var ms []fakeMethod
			bodies := map[string]string{}
			for _, sm := range s.Seams {
				o := e.op(sm.Op)
				m := e.opMethod(f, o, reserved)
				params, results := golang.SeamSignature(o, f.im, reserved)
				m.name, m.sig = sm.Method, method(sm.Method, params, results)
				bodies[sm.Method] = golang.SeamBodyParam(o, reserved)
				ms = append(ms, m)
			}
			e.genFakeType(f, p.Fake, "継ぎ目", entry+"Seams", "継ぎ目ごとに、フィールド一つ", "言われていない継ぎ目は、op の中身を、そのまま呼ぶ。", false, ms, reserved,
				func(m fakeMethod) string { return bodies[m.name] + "(" + m.args + ")" })
		}
	}

	// Fakes
	f.print(comment("Fakes は、名前空間 " + e.view.Viewer + " の、テストの中の組み立て（NewOps）に使う、にせもの。フィールドは、New の引数の順。零値のまま、使える。"))
	f.print("type Fakes struct {\n")
	testPkg := func(u *golang.TestKit) string { return f.im.Add(u.Layout.TestPath, u.Layout.TestName) }
	for _, p := range k.Params {
		if p.Field == "" {
			continue
		}
		switch p.Kind {
		case golang.KitLeaves:
			f.printf("\t// %s は、葉のにせもの。言われていない葉は、本物を呼ぶ。\n\t%s %s\n", p.Field, p.Field, p.Fake)
		case golang.KitState:
			f.printf("\t// %s は、状態 %s の単位のにせもの。\n\t%s %s\n", p.Field, p.FQN, p.Field, p.Fake)
		case golang.KitClassify:
			f.printf("\t// %s は、分類 %s の述語のにせもの。言われていなければ、本物の述語を呼ぶ。\n\t%s %s\n", p.Field, p.FQN, p.Field, p.Fake)
		case golang.KitExternal:
			f.printf("\t// %s は、名前空間 %s（単位がない）の op のにせもの。\n\t%s %s\n", p.Field, p.FQN, p.Field, p.Fake)
		case golang.KitUnit:
			f.printf("\t// %s は、名前空間 %s の出力（%s）の、にせもの。その配線は、本物を通す。\n\t%s %s.Fakes\n", p.Field, p.FQN, p.Unit.Dir, p.Field, testPkg(p.Unit))
		case golang.KitSeams:
			f.printf("\t// %s は、継ぎ目のにせもの。言われていない継ぎ目は、op の中身を、そのまま呼ぶ。\n\t%s %s\n", p.Field, p.Field, p.Fake)
		}
	}
	f.print("}\n\n")

	// NewOps。人が書いたものの引数の名前は、このファイルの import の名前を避ける
	taken := map[string]bool{"f": true}
	for _, n := range f.im.Names() {
		taken[n] = true
	}
	realName := map[string]string{} // TestKit の名前 → このファイルでの名前
	params := []string{"f *Fakes"}
	var names []string
	for _, r := range k.Reals {
		name := r.Name
		for taken[name] {
			name += "_"
		}
		taken[name] = true
		realName[r.Name] = name
		names = append(names, name)
		typ := f.im.Qualifier(r.Namespace) + "Leaves"
		if r.Kind == golang.KitClassify {
			typ = f.im.Qualifier(e.declNS[r.Classify]) + e.typeName(r.Classify) + "Cases"
		}
		params = append(params, name+" "+typ)
	}
	text := "NewOps は、にせもの f の上に、名前空間 " + e.view.Viewer + " の Ops を組み立てる: 配線は、本物。流れの端と、継ぎ目は、にせもの。"
	if len(names) > 0 {
		text += "\n人が書いたもの（" + joinJa(names) + "）は、引数で受け取り、にせものが包む。"
	}
	f.print(comment(text))
	var args []string
	own := 0 // k.Reals のうち、この出力のもの（葉、分類の述語）を、順に使う
	for _, p := range k.Params {
		switch p.Kind {
		case golang.KitLeaves, golang.KitClassify:
			args = append(args, lowerFirst(p.Fake)+"{&f."+p.Field+", "+realName[k.Reals[own].Name]+"}")
			own++
		case golang.KitUnit:
			if !p.Unit.HasFakes() {
				args = append(args, f.im.Qualifier(p.FQN)+"New()")
				continue
			}
			sub := []string{"&f." + p.Field}
			for _, r := range p.Unit.Reals {
				sub = append(sub, realName[p.Param+golang.Capitalize(r.Name)])
			}
			args = append(args, testPkg(p.Unit)+".NewOps("+strings.Join(sub, ", ")+")")
		default:
			args = append(args, lowerFirst(p.Fake)+"{&f."+p.Field+"}")
		}
	}
	f.printf("func NewOps(%s) %sOps {\n\treturn %sNew(%s)\n}\n", strings.Join(params, ", "), entry, entry, strings.Join(args, ", "))
	return f
}

// genCasesFake は、分類の述語のにせもの（docs/plugin-go.md「テストのにせもの」の <C>CasesFake）: メソッド Is と、本物を包む、口を満たす型。
func (e *emitter) genCasesFake(f *fileGen, p golang.KitParam) {
	c, _ := e.lookup(p.FQN).(*ir.Classify)
	q := f.im.Qualifier(e.declNS[p.FQN])
	name := e.typeName(p.FQN)
	iface := q + name + "Cases"
	ctx := f.im.Add("context", "context")
	f.print(comment(p.Fake + " は、分類 " + p.FQN + " の述語（" + iface + "）のにせもの。言われていなければ、本物の述語を呼ぶ。"))
	f.printf("type %s struct {\n\tset bool\n\tkind %s%s\n}\n\n", p.Fake, q, name)
	f.print(comment("Is は、これからの判定が、値によらず、ケース k になるようにする。"))
	f.printf("func (f *%s) Is(k %s%s) { f.set, f.kind = true, k }\n\n", p.Fake, q, name)
	private := lowerFirst(p.Fake)
	f.print(comment(private + " は、" + p.Fake + " を、" + iface + " にする。"))
	f.printf("type %s struct {\n\tf *%s\n\timpl %s\n}\n\n", private, p.Fake, iface)
	f.printf("var _ %s = %s{}\n\n", iface, private)
	subject := f.goType(c.SubjectType())
	for _, cs := range c.Cases {
		if cs.Name == c.Absent {
			continue
		}
		m := golang.Capitalize(cs.Name)
		f.printf("func (o %s) %s(ctx %s.Context, v %s) (bool, error) {\n\tif o.f.set {\n\t\treturn o.f.kind == %s%s, nil\n\t}\n\treturn o.impl.%s(ctx, v)\n}\n\n",
			private, m, ctx, subject, q, golang.CaseConst(name, cs.Name), m)
	}
}
