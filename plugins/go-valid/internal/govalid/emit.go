package govalid

import (
	"errors"
	"fmt"
	"path"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
	"github.com/mew-ton/hozo/plugin/golang"
	"github.com/mew-ton/hozo/plugin/govalid"
	"github.com/mew-ton/hozo/plugin/validvocab"
)

// 診断のコード（docs/plugin-go-valid.md「診断」）。
const (
	codeValidHome   = "GOVALID-3" // Validate を持つ別の名前空間の型の置き場の単位が、go-valid を並べていない
	codeValidUnion  = "GOVALID-4" // フィールドの型が union で、そのアームに Validate を持つ型がある
	codeValidCustom = "GOVALID-5" // 標準でない valid の制約が付いている
	codeValidField  = "GOVALID-6" // Validate を持つ model が、validate という名前のフィールドを持つ
	codeValidOption = "GOVALID-7" // [emit.options.go-valid] に、知らないキーがある
)

// FileName は、このトランスパイラが出すただ一つのファイルの名前（docs/plugin-go.md「相乗りのファイル」）。定義のパッケージに置く。
const FileName = "validate.go"

// vocabulary は、制約のデコレータを持つ語彙の名前空間。
const vocabulary = "valid"

// method は、このトランスパイラが型に足すメソッドの名前。
const method = "Validate"

// Emit は、要求の View の出力 (U, N) の validate.go を作る（docs/plugin-go-valid.md「出力」）。読むのは、要求だけ
// （ファイルも、環境変数も、読まない）。診断があれば、ファイルは返さない。`Validate` を持つ型が N に一つもなければ、何も返さない。
// 戻り値の error は、利用者の仕様の誤りでないもの（View が欠けている、生成したコードを整形できない）。
func Emit(req *plugin.EmitRequest) ([]plugin.File, []plugin.Diagnostic, error) {
	view := req.View
	if view == nil || view.Program == nil || view.Program.Namespace(view.Viewer) == nil {
		return nil, nil, fmt.Errorf("View に、名前空間 %s の Program がない", viewerOf(view))
	}
	g := &generator{view: view, p: view.Program, ns: view.Program.Namespace(view.Viewer)}
	g.prepare()

	// GOVALID-7: オプションはない
	var none struct{}
	unknown, err := plugin.Options(req, &none)
	if err != nil {
		g.errorf(codeValidOption, "", "%d 番目の [[emit]] の [emit.options.%s] を読めない: %v", view.Config.Index, TranspilerName, err)
	}
	for _, key := range unknown {
		g.errorf(codeValidOption, "", "%d 番目の [[emit]] の [emit.options.%s] に、知らないキー %s がある（%s にオプションはない）",
			view.Config.Index, TranspilerName, key, TranspilerName)
	}

	// 置き場。基底のオプション module がなければ、基底が GO-8 を返すので、ここでは、仕様だけで決まる診断を返して終わる
	layout, layoutErr := golang.LayoutOf(view)
	if layoutErr != nil && !errors.Is(layoutErr, golang.ErrNoModule) {
		return nil, nil, fmt.Errorf("名前空間 %s の出力先 %q の import パスを求められない: %w", view.Viewer, view.Config.Dir, layoutErr)
	}
	g.layout = layout

	targets := g.check()
	if g.fatal != nil {
		return nil, nil, g.fatal
	}
	if len(g.errs) > 0 {
		return nil, g.errs, nil
	}
	if len(targets) == 0 || layout == nil {
		return nil, nil, nil
	}
	content, err := g.render(targets)
	if err != nil {
		return nil, nil, err
	}
	return []plugin.File{{Path: path.Join(layout.TypesDir, FileName), Content: content}}, nil, nil
}

func viewerOf(view *ir.View) string {
	if view == nil {
		return ""
	}
	return view.Viewer
}

// generator は一つの出力 (U, N) の validate.go を作る状態。
type generator struct {
	view   *ir.View
	p      *ir.Program
	ns     *ir.Namespace  // N
	layout *golang.Layout // module がなければ nil

	declNS map[string]string // 宣言の FQN → 宣言した名前空間
	vocab  map[string]bool   // valid の語彙のデコレータの FQN（標準のものと、利用者が足したもの）
	has    map[string]bool   // govalid.HasValidate の結果

	errs  []plugin.Diagnostic
	seen  map[string]bool
	fatal error
}

func (g *generator) errorf(code, at, format string, args ...any) {
	d := plugin.Diagnostic{Code: code, At: at, Message: fmt.Sprintf(format, args...)}
	key := d.Code + "\x00" + d.At + "\x00" + d.Message
	if g.seen == nil {
		g.seen = map[string]bool{}
	}
	if g.seen[key] {
		return
	}
	g.seen[key] = true
	g.errs = append(g.errs, d)
}

// prepare は、View から、検査と生成の間ずっと使う表を作る。
func (g *generator) prepare() {
	g.declNS, g.vocab, g.has = map[string]string{}, map[string]bool{}, map[string]bool{}
	for i := range g.p.Namespaces {
		for _, d := range g.p.Namespaces[i].Decls {
			if d == nil {
				continue
			}
			if _, dup := g.declNS[d.Common().Name]; !dup {
				g.declNS[d.Common().Name] = g.p.Namespaces[i].Name
			}
			if _, ok := d.(*ir.Decorator); ok && g.p.Namespaces[i].Name == vocabulary {
				g.vocab[d.Common().Name] = true
			}
		}
	}
}

// validated は、宣言 fqn が Validate を持つか（govalid.HasValidate。docs/plugin-go-valid.md「Validate を持つ型」）。
func (g *generator) validated(fqn string) bool {
	v, ok := g.has[fqn]
	if !ok {
		v = govalid.HasValidate(g.p, fqn)
		g.has[fqn] = v
	}
	return v
}

// validatedRef は、型 t が、Validate を持つ型への ref なら、その FQN。
func (g *generator) validatedRef(t ir.Type) (string, bool) {
	r, ok := t.(ir.RefType)
	if !ok || !g.validated(r.Target) {
		return "", false
	}
	return r.Target, true
}

// checked は、型 t（通して見たもの）の値に、確かめるものがあるか: Validate を持つ型への ref か、アームが Validate を持つ union。
func (g *generator) checked(t ir.Type) bool {
	if _, ok := g.validatedRef(t); ok {
		return true
	}
	return g.unionValidated(t)
}

// through は、フィールドの型を、optional、array と、インデックスの model の値を通して見た型
// （docs/plugin-go-valid.md「Validate を持つ型」の 4）。
func (g *generator) through(t ir.Type) ir.Type { return govalid.Through(g.p, t) }

// target は、Validate を出す型一つ: N の scalar か model か、union のアームを包む型（union と、そのアーム）。
type target struct {
	scalar *ir.Scalar
	model  *ir.Model
	union  *ir.Union
	arm    *ir.UnionArm
}

// check は、N の scalar と model を宣言順に見て、診断（GOVALID-3〜GOVALID-6）を集め、Validate を出す型を返す。
func (g *generator) check() []target {
	var targets []target
	for _, d := range g.ns.Decls {
		switch x := d.(type) {
		case *ir.Scalar:
			g.checkScalar(x)
			if g.validated(x.Name) {
				targets = append(targets, target{scalar: x})
			}
		case *ir.Model:
			if x.SynthesizedFrom != "" {
				if _, isOp := g.p.Lookup(x.SynthesizedFrom).(*ir.Op); isOp {
					continue // View が見る側のために合成した model は、Go の型にならない（docs/plugin-go.md「出力の対象」）
				}
			}
			g.checkModel(x)
			if g.validated(x.Name) {
				targets = append(targets, target{model: x})
			}
		case *ir.Union:
			if x.Variant == ir.UnionArms {
				for _, a := range g.wrappedArms(x) {
					targets = append(targets, target{union: x, arm: a})
				}
			}
		}
	}
	return targets
}

// unchecked は、制約の使い方の誤り problems（VALID-*）を見つけたとき: 語彙のプラグイン valid の検査を通っていない仕様なので、
// 診断にせず、異常終了にする（docs/plugin-go-valid.md「診断」）。
func (g *generator) unchecked(problems []validvocab.Problem) {
	if len(problems) > 0 && g.fatal == nil {
		g.fatal = fmt.Errorf("valid の語彙の検査（%s）を通っていない仕様を受け取った: %s", problems[0].Code, problems[0].Message)
	}
}

// scalarConstraints は、scalar s に付いた valid の標準の制約（書かれた順。validvocab.Scalar）。
func (g *generator) scalarConstraints(s *ir.Scalar) []constraint {
	cons, problems := validvocab.Scalar(g.p, s)
	g.unchecked(problems)
	return wrap(cons)
}

// checkScalar は scalar s の診断: 自分に付いた、標準でない制約（GOVALID-5）と、元の scalar の置き場（GOVALID-3）。
func (g *generator) checkScalar(s *ir.Scalar) {
	subject := "scalar " + s.Name
	for i := range s.Decorators {
		app := &s.Decorators[i]
		if g.vocab[app.Decorator] && !validvocab.Standard(app.Decorator) {
			g.custom(app, s.Name, subject)
		}
	}
	g.scalarConstraints(s)
	if base, ok := g.validatedRef(s.Base); ok {
		g.checkHome(base, s.Name, fmt.Sprintf("scalar %s の元の型 %s", s.Name, base))
	}
}

// custom は GOVALID-5: 標準でない valid の制約 app が、at（subject）に付いている。
func (g *generator) custom(app *ir.Application, at, subject string) {
	g.errorf(codeValidCustom, at, "%s に、標準でない %s の制約 @%s が付いている（%s の、標準の五つのほかの制約を確かめる形は、まだない）",
		subject, vocabulary, app.Decorator, vocabulary)
}

// checkModel は model m の診断を、フィールドの順に: フィールドに付いた、標準でない制約（GOVALID-5）、フィールドの型
// （GOVALID-4、GOVALID-3）。最後に、フィールドの名前（GOVALID-6）。
func (g *generator) checkModel(m *ir.Model) {
	for i := range m.Fields {
		f := &m.Fields[i]
		subject := fmt.Sprintf("model %s のフィールド %s", m.Name, f.Name)
		for j := range f.Decorators {
			app := &f.Decorators[j]
			if g.vocab[app.Decorator] && !validvocab.Standard(app.Decorator) {
				g.custom(app, m.Name, subject)
			}
		}
		g.fieldConstraints(m, f)
		if t, isRef := g.through(f.Type).(ir.RefType); isRef {
			if u, ok := g.p.Lookup(t.Target).(*ir.Union); ok && u.Variant == ir.UnionArms {
				g.checkUnion(u, m.Name, subject)
			} else if g.validated(t.Target) {
				g.checkHome(t.Target, m.Name, fmt.Sprintf("%s の型 %s", subject, t.Target))
			}
		}
	}
	if !g.validated(m.Name) {
		return
	}
	for _, f := range m.Fields {
		if golang.FieldName(f.Name) == method {
			g.errorf(codeValidField, m.Name, "model %s は %s を持つが、フィールド %s が、Go でメソッド %s と同じ名前になる（直すのは仕様の名前）",
				m.Name, method, f.Name, method)
		}
	}
}

// fieldConstraints は、model m のフィールド f に付いた valid の標準の制約（書かれた順。validvocab.Field）。
// 配列のフィールドに付けた length は、要素の数に掛かる（Count。違反の名前は count）。
func (g *generator) fieldConstraints(m *ir.Model, f *ir.Field) []constraint {
	cons, problems := validvocab.Field(g.p, m, f)
	g.unchecked(problems)
	return wrap(cons)
}

// checkUnion は、model at のフィールド（subject）の型の union u の診断（docs/plugin-go-valid.md「Validate を持つ型」の、
// union のアームの検証）: アームの型が、Validate を持つ型への ref なら、その型の置き場（GOVALID-3。包む型の Validate は、
// union を宣言した名前空間の、定義のパッケージに出るので、その単位も）。確かめる形のないアームは GOVALID-4。
func (g *generator) checkUnion(u *ir.Union, at, subject string) {
	direct := false
	for _, a := range u.Arms {
		if r, isRef := a.Type.(ir.RefType); isRef && g.validated(r.Target) {
			direct = true
			g.checkHome(r.Target, at, fmt.Sprintf("%s の型 union %s のアーム %s の型 %s", subject, u.Name, a.Name, r.Target))
			continue
		}
		if fqn, found := g.validatedIn(a.Type, map[string]bool{u.Name: true}); found {
			g.errorf(codeValidUnion, at, "%s の型が union %s で、そのアーム %s の型 %s の中に %s を持つ型 %s がある（アームの型が、%s を持つ型そのものでないとき、確かめる形は、まだない）",
				subject, u.Name, a.Name, golang.TypeDisplay(a.Type), method, fqn, method)
		}
	}
	if direct {
		g.checkHome(u.Name, at, fmt.Sprintf("%s の型 union %s（アームを包む型）", subject, u.Name))
	}
}

// validatedIn は、型 t（アームの型）の中（optional、array、インデックスの model の値を通して見たもの。union なら、そのアームも）に、
// Validate を持つ型があれば、その FQN。
func (g *generator) validatedIn(t ir.Type, visited map[string]bool) (string, bool) {
	r, ok := g.through(t).(ir.RefType)
	if !ok {
		return "", false
	}
	if g.validated(r.Target) {
		return r.Target, true
	}
	if inner, ok := g.p.Lookup(r.Target).(*ir.Union); ok && inner.Variant == ir.UnionArms && !visited[inner.Name] {
		visited[inner.Name] = true
		for _, a := range inner.Arms {
			if fqn, found := g.validatedIn(a.Type, visited); found {
				return fqn, true
			}
		}
	}
	return "", false
}

// wrappedArms は、N の union u の、包む型のアームのうち、Validate を出すもの（docs/plugin-go-valid.md「出力」）: アームの型が、
// Validate を持つ型への ref で、アームの値が、包む型（u と同じ名前空間の model でない。docs/plugin-go.md「union」）。
func (g *generator) wrappedArms(u *ir.Union) []*ir.UnionArm {
	var out []*ir.UnionArm
	ns := g.declNS[u.Name]
	for i := range u.Arms {
		r, isRef := u.Arms[i].Type.(ir.RefType)
		if !isRef || !g.validated(r.Target) {
			continue
		}
		if _, isModel := g.p.Lookup(r.Target).(*ir.Model); isModel && g.declNS[r.Target] == ns {
			continue
		}
		out = append(out, &u.Arms[i])
	}
	return out
}

// unionValidated は、型 t が、union（arms）への ref で、そのアームの型のどれかが、Validate を持つ型への ref か
// （docs/plugin-go-valid.md「Validate を持つ型」の 5）。
func (g *generator) unionValidated(t ir.Type) bool {
	r, ok := t.(ir.RefType)
	if !ok {
		return false
	}
	u, ok := g.p.Lookup(r.Target).(*ir.Union)
	if !ok || u.Variant != ir.UnionArms {
		return false
	}
	for _, a := range u.Arms {
		if ar, isRef := a.Type.(ir.RefType); isRef && g.validated(ar.Target) {
			return true
		}
	}
	return false
}

// checkHome は GOVALID-3: Validate を持つ型 fqn が別の名前空間 M のものなら、M の型の置き場（docs/config.md「型の置き場」）の
// 単位が、transpilers に go-valid を並べていなければならない（メソッドは、型の定義のパッケージにしか書けない）。
// at は診断の位置、what は、その型を使う場所（メッセージ用）。
func (g *generator) checkHome(fqn, at, what string) {
	m := g.declNS[fqn]
	if m == g.view.Viewer || g.layout == nil {
		return
	}
	_, ok, err := golang.HomeOf(g.view, m)
	if err != nil {
		if !errors.Is(err, golang.ErrNoModule) && g.fatal == nil {
			g.fatal = fmt.Errorf("名前空間 %s の型の置き場を求められない: %w", m, err)
		}
		return
	}
	if !ok {
		if g.fatal == nil {
			g.fatal = fmt.Errorf("名前空間 %s の型の置き場がない（CFG-22 が通したはずのもの）", m)
		}
		return
	}
	// 置き場の単位が go-valid を並べているかは、Validate を呼ぶほかの相乗り（go-http、go-cli）と同じ関数で見る
	if unit, unlisted := govalid.Unlisted(g.view, m); unlisted {
		g.errorf(codeValidHome, at, "%s は %s を持つが、名前空間 %s の型の置き場の単位（%d 番目の [[emit]]）が、transpilers に %s を並べていない（メソッドは、型の定義のパッケージにしか書けない）",
			what, method, m, unit, TranspilerName)
	}
}
