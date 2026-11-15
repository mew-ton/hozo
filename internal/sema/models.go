package sema

import (
	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/ir"
)

// modelJob は model の本体を仕上げる仕事（docs/types.md「継承した model」）。宣言した model と、その場で書いた model
// （ユニオンのアーム、例外の値）のどちらにも一つずつある。すべての宣言とデコレータの適用が下りてから
// （finalizeModels）行う: 元の model のフィールドを、デコレータごと写すため。
type modelJob struct {
	// sym は宣言。その場で書いた model なら nil。
	sym   *symbol
	model *ir.Model
	body  *ast.ModelBody
	// extends は `extends B` の名前。なければ nil。
	extends *ast.QualifiedName
	sc      *scope

	// overrides は Override ごとの、下ろしたデコレータの置き場。body.Overrides と同じ並び。
	overrides [][]ir.Application
	// base は元の model の仕事。元を持たない（extends がない、解決できない、循環している）なら nil。
	base *modelJob
	// cyclic は extends が循環しているか（TYPE-10 を出した）。本体の Override は重ねて報告しない。
	cyclic bool
	done   bool
}

// addModelJob は model の仕事を登録し、Override に付いたデコレータを下ろす場所を用意する。
func (a *analyzer) addModelJob(j *modelJob) {
	if j.body != nil {
		j.overrides = make([][]ir.Application, len(j.body.Overrides))
		for i, o := range j.body.Overrides {
			if o != nil {
				a.addSite(o.Decorators, siteField, j.sc, &j.overrides[i])
			}
		}
	}
	a.bodies = append(a.bodies, j)
}

// finalizeModels は、継承した model のフィールドを作り（docs/types.md「継承した model」の「フィールド」）、
// 本体に書けるものを確かめる（TYPE-10、TYPE-14、TYPE-15、TYPE-16）。
func (a *analyzer) finalizeModels() {
	byName := map[string]*modelJob{}
	for _, j := range a.bodies {
		byName[j.model.Name] = j
	}
	// 元の model を解決する
	for _, j := range a.bodies {
		if j.extends == nil || j.sym == nil {
			continue
		}
		r, ok := a.resolve(*j.extends, j.sc, acModel)
		if !ok {
			continue
		}
		j.base = byName[r.sym.fqn]
	}
	// 循環（TYPE-10）: extends を辿って自分に戻る model は、元の model を持たないものとして続ける
	for _, j := range a.bodies {
		for b, steps := j.base, 0; b != nil && steps <= len(a.bodies); b, steps = b.base, steps+1 {
			if b == j {
				j.cyclic = true
				break
			}
		}
	}
	for _, j := range a.bodies {
		if j.cyclic {
			a.errorf("TYPE-10", j.extends.Span, "model %s の extends が循環している（%s）", j.model.Name, j.base.model.Name)
		}
	}
	for _, j := range a.bodies {
		if j.cyclic {
			j.base = nil
		}
	}
	for _, j := range a.bodies {
		a.finalizeModel(j)
	}
}

// finalizeModel は一つの model を仕上げる。元の model を先に仕上げる。
func (a *analyzer) finalizeModel(j *modelJob) {
	if j.done {
		return
	}
	j.done = true
	if j.base == nil {
		a.rejectOverrides(j)
		return
	}
	a.finalizeModel(j.base)
	base := j.base.model
	own := j.model.Fields
	name := j.model.Name

	// 1. 元の model のフィールドを、元の並びのまま写す
	fields := make([]ir.Field, len(base.Fields))
	for i, f := range base.Fields {
		f.Decorators = append([]ir.Application(nil), f.Decorators...)
		fields[i] = f
	}
	spans := map[string]source.Span{}
	for _, f := range base.Fields {
		spans[f.Name] = a.fields[base.Name][f.Name]
	}

	// 2. Override を当てる
	ext := &ir.ModelExtends{Base: base.Name}
	touched := map[string]source.Span{}
	for i, o := range j.body.Overrides {
		if o == nil {
			continue
		}
		bf := findField(base, o.Name.Name)
		if bf == nil {
			a.errorf("TYPE-10", o.Name.Span, "元の model %s にフィールド %s はないので override できない（足すフィールドには override を書かない）", base.Name, o.Name.Name)
			continue
		}
		a.addRef(o.Name.Span, Target{Kind: TargetField, Decl: base.Name, Name: o.Name.Name, Span: a.fields[base.Name][o.Name.Name]})
		if first, dup := touched[o.Name.Name]; dup {
			a.errorf("TYPE-16", o.Name.Span, "model %s でフィールド %s への override が二度ある", name, o.Name.Name).Related = []diag.Related{related(first, "最初の override")}
			continue
		}
		touched[o.Name.Name] = o.Name.Span
		at := fieldIndex(fields, o.Name.Name)
		if at < 0 {
			continue
		}
		if o.Never {
			if len(o.Decorators) > 0 || o.Doc != nil {
				a.errorf("TYPE-16", o.Name.Span, "なくすフィールド %s には、デコレータも説明も付けられない", o.Name.Name)
			}
			fields = append(fields[:at], fields[at+1:]...)
			delete(spans, o.Name.Name)
			ext.Removed = append(ext.Removed, o.Name.Name)
			continue
		}
		f := &fields[at]
		// changed は型が変わったか。bad は型を当てられなかったか（解決できない、変えられない型）。
		changed, bad := false, false
		if o.Type != nil {
			t, ok := a.lowerType(o.Type, j.sc)
			switch {
			case !ok || bf.Type == nil:
				bad = true
			default:
				widened, can := a.changeable(bf.Type, t)
				switch {
				case !can:
					a.errorf("TYPE-15", o.Type.NodeSpan(), "フィールド %s の型 %s を %s には変えられない（変えられるのは、元の型をアームに持つ union に広げるか、元の型を継承した型に差し替えるか。docs/types.md「変えられる型」）", o.Name.Name, typeName(bf.Type), typeName(t))
					bad = true
				case !ir.TypeEqual(bf.Type, t):
					f.Type = t
					changed = true
					if widened {
						f.Default = nil // 広げた型（union）のフィールドは既定値を持てない
					}
				}
			}
		}
		if bad && len(o.Decorators) == 0 {
			continue
		}
		if !changed && len(o.Decorators) == 0 {
			what := "型を書かない override には、デコレータが一つ以上要る"
			if o.Type != nil {
				what = "元と等しい型の override には、デコレータが一つ以上要る"
			}
			a.errorf("TYPE-16", o.Name.Span, "フィールド %s への override は何も変えない（%s。説明だけを変えることはできない）", o.Name.Name, what)
			continue
		}
		a.appendOverrideDecorators(f, o, j.overrides[i])
		// 説明は、override に書いたものに置き換える（書いていなければ、説明なしになる）
		f.Description = nil
		if o.Doc != nil {
			f.Description = a.lowerDoc(o.Doc, docContext{ns: j.sc.ns, span: o.Name.Span})
		}
		f.Span = a.irSpan(o.Span)
		spans[o.Name.Name] = o.Name.Span
		ext.Overridden = append(ext.Overridden, o.Name.Name)
	}

	// 3. 足すフィールドを、書かれた順に末尾に足す
	for _, f := range own {
		if findField(base, f.Name) != nil {
			span := a.fields[name][f.Name]
			a.errorf("TYPE-14", span, "フィールド %s は元の model %s にある。元のフィールドに触れるときは override を書く（override %s ...）", f.Name, base.Name, f.Name)
			continue
		}
		fields = append(fields, f)
		spans[f.Name] = a.fields[name][f.Name]
	}
	j.model.Fields = fields
	j.model.Extends = ext
	a.fields[name] = spans
}

// rejectOverrides は TYPE-10 の「元の model を持たない model に書いた override」。extends のない model と、
// その場で書いた model に当たる。循環に入った model の Override は重ねて報告しない。
func (a *analyzer) rejectOverrides(j *modelJob) {
	if j.body == nil || j.cyclic {
		return
	}
	for _, o := range j.body.Overrides {
		if o == nil {
			continue
		}
		what := "extends のない model"
		if j.sym == nil {
			what = "その場で書いた model"
		}
		a.errorf("TYPE-10", o.Name.Span, "%s %s には元の model がないので、override は書けない", what, j.model.Name)
	}
}

// appendOverrideDecorators は Override に付けたデコレータを、受け継いだものの後ろに書かれた順に足す。
// 同じデコレータを重ねれば DEC-13（docs/types.md「継承した model」の「受け継ぐもの」）。
func (a *analyzer) appendOverrideDecorators(f *ir.Field, o *ast.Override, apps []ir.Application) {
	inherited := map[string]bool{}
	for _, app := range f.Decorators {
		inherited[app.Decorator] = true
	}
	for _, app := range apps {
		if inherited[app.Decorator] {
			a.errorf("DEC-13", a.decoratorSpan(o.Decorators, app), "デコレータ %s は、フィールド %s が元の model から受け継いでいる（同じデコレータは重ねられない）", app.Decorator, o.Name.Name)
			continue
		}
		f.Decorators = append(f.Decorators, app)
	}
}

// decoratorSpan は、下ろした適用 app に当たるデコレータの名前の位置。
func (a *analyzer) decoratorSpan(decos []*ast.Decorator, app ir.Application) source.Span {
	for _, d := range decos {
		if a.irSpan(d.Span) == app.Span {
			return d.Name.Span
		}
	}
	if len(decos) > 0 {
		return decos[0].Name.Span
	}
	return source.Span{}
}

func fieldIndex(fields []ir.Field, name string) int {
	for i := range fields {
		if fields[i].Name == name {
			return i
		}
	}
	return -1
}

// changeable は docs/types.md「変えられる型」: 元の型 o を n に変えられるか。widened は、変更が「広げる」（規則 2）を
// 含むか（含めば、既定値は受け継がない）。
func (a *analyzer) changeable(o, n ir.Type) (widened, ok bool) {
	// 1. 等しい
	if ir.TypeEqual(o, n) {
		return false, true
	}
	return a.changeableStrict(o, n)
}

// changeableStrict は規則 2〜6（等しいだけでは通さない）。
func (a *analyzer) changeableStrict(o, n ir.Type) (widened, ok bool) {
	// 2. 広げる: n が union（arms）への ref で、そのアームのどれかの型が o と等しい
	if a.armOf(n, o) {
		return true, true
	}
	// 3. 差し替える: n が o を継承している
	if a.inherits(n, o) {
		return false, true
	}
	switch ov := o.(type) {
	case ir.OptionalType: // 4
		if nv, isOpt := n.(ir.OptionalType); isOpt {
			return a.changeableStrict(ov.Of, nv.Of)
		}
	case ir.ArrayType: // 5
		if nv, isArr := n.(ir.ArrayType); isArr {
			return a.changeableStrict(ov.Of, nv.Of)
		}
	case ir.RecordType: // 6
		if nv, isRec := n.(ir.RecordType); isRec && ir.TypeEqual(ov.Key, nv.Key) {
			return a.changeableStrict(ov.Value, nv.Value)
		}
	}
	return false, false
}

// inherits は docs/types.md「継承した model」の「継承している」: x と y が model で、x の元の model を一回以上辿ると
// y に着く。または x が scalar で、x の extends を一回以上辿ると y（scalar か核の型）に着く。等しいときは含まない。
func (a *analyzer) inherits(x, y ir.Type) bool {
	xr, ok := x.(ir.RefType)
	if !ok {
		return false
	}
	if a.models[xr.Target] != nil {
		yr, ok := y.(ir.RefType)
		if !ok {
			return false
		}
		for j, steps := a.modelJobOf(xr.Target), 0; j != nil && j.base != nil && steps <= len(a.bodies); steps++ {
			j = j.base
			if j.model.Name == yr.Target {
				return true
			}
		}
		return false
	}
	seen := map[string]bool{}
	for s := a.scalarSym(xr.Target); s != nil && !seen[s.fqn]; {
		seen[s.fqn] = true
		sc, ok := a.lower(s).(*ir.Scalar)
		if !ok || sc.Base == nil {
			return false
		}
		if ir.TypeEqual(sc.Base, y) {
			return true
		}
		r, ok := sc.Base.(ir.RefType)
		if !ok {
			return false
		}
		s = a.scalarSym(r.Target)
	}
	return false
}

// modelJobOf は FQN の model の仕事。
func (a *analyzer) modelJobOf(fqn string) *modelJob {
	for _, j := range a.bodies {
		if j.model.Name == fqn {
			return j
		}
	}
	return nil
}
