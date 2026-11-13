package golang

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mew-ton/hozo/ir"
	gol "github.com/mew-ton/hozo/plugin/golang"
	"github.com/mew-ton/hozo/runtime"
)

const (
	validNS      = "valid"
	runtimePath  = "github.com/mew-ton/hozo/runtime"
	validMin     = "valid.min"
	validMax     = "valid.max"
	validLength  = "valid.length"
	validPattern = "valid.pattern"
	validFormat  = "valid.format"
)

// knownFormats は runtime.CheckFormat が知っている name（docs/emit-go.md「runtime パッケージ」）。
var knownFormats = map[string]bool{"email": true, "uri": true, "uuid": true, "date-time": true}

// constraint は scalar かフィールドに付いた valid.* の制約一つ。
type constraint struct {
	// name は違反の Constraint に書く名前（"min"、"length"、ユーザーの制約なら宣言名）。
	name string
	app  ir.Application
	// user は valid に足した制約（標準の五つ以外）。decl はその宣言。
	user bool
	decl *ir.Decorator
	// patternVar は pattern のパッケージ変数の名前。
	patternVar string
	regex      string
	// min / max / length の値
	hasMin, hasMax bool
	min, max       float64
	minInt, maxInt int64
	intArgs        bool // min / max の引数が int リテラルか
	format         string
	// userCore はユーザーの制約が付いた先の元の型。
	userCore ir.CoreName
}

// constraintsOf は decos の valid.* を制約にする。core は付いた先の元の型（ok が false なら核の型を持たない）。
// owner は pattern 変数の名前の元（型名か型名+フィールド名）、subject は診断に書く付いた先の名前（FQN）。
// report なら GOEMIT-3 を出し、違反した制約は列から外す。
func (e *emitter) constraintsOf(decos []ir.Application, core ir.CoreName, coreOK bool, owner, subject string, report bool) []constraint {
	var out []constraint
	bad := func(app *ir.Application, format string, args ...any) {
		if report {
			e.errorf(CodeBadConstraint, &app.Span, format, args...)
		}
	}
	for i := range decos {
		app := &decos[i]
		if !strings.HasPrefix(app.Decorator, validNS+".") {
			continue
		}
		c := constraint{name: localName(app.Decorator), app: *app}
		switch app.Decorator {
		case validMin, validMax:
			lit, ok := argLiteral(app, "value")
			if !ok {
				bad(app, "%s の引数 value がリテラルでない", app.Decorator)
				continue
			}
			if !coreOK || !isNumeric(core) {
				bad(app, "%s は数値の型にだけ付けられるが、%s の元の型は %s", app.Decorator, subject, coreName(core, coreOK))
				continue
			}
			v, isInt := numberOf(lit)
			if app.Decorator == validMin {
				c.hasMin, c.min = true, v
				c.minInt = int64(v)
			} else {
				c.hasMax, c.max = true, v
				c.maxInt = int64(v)
			}
			c.intArgs = isInt
		case validLength:
			if !coreOK || core != ir.CoreString {
				bad(app, "%s は string の型にだけ付けられるが、%s の元の型は %s", app.Decorator, subject, coreName(core, coreOK))
				continue
			}
			if lit, ok := argLiteral(app, "min"); ok {
				v, _ := numberOf(lit)
				c.hasMin, c.minInt = true, int64(v)
			}
			if lit, ok := argLiteral(app, "max"); ok {
				v, _ := numberOf(lit)
				c.hasMax, c.maxInt = true, int64(v)
			}
		case validPattern:
			if !coreOK || core != ir.CoreString {
				bad(app, "%s は string の型にだけ付けられるが、%s の元の型は %s", app.Decorator, subject, coreName(core, coreOK))
				continue
			}
			lit, ok := argLiteral(app, "regex")
			s, isStr := lit.(ir.StringLiteral)
			if !ok || !isStr {
				bad(app, "%s の引数 regex が文字列でない", app.Decorator)
				continue
			}
			if _, err := regexp.Compile(s.Value); err != nil {
				bad(app, "%s の %q は RE2 として解釈できない: %v", app.Decorator, s.Value, err)
				continue
			}
			c.regex = s.Value
			c.patternVar = "pattern" + owner
		case validFormat:
			if !coreOK || core != ir.CoreString {
				bad(app, "%s は string の型にだけ付けられるが、%s の元の型は %s", app.Decorator, subject, coreName(core, coreOK))
				continue
			}
			lit, ok := argLiteral(app, "name")
			s, isStr := lit.(ir.StringLiteral)
			if !ok || !isStr {
				bad(app, "%s の引数 name が文字列でない", app.Decorator)
				continue
			}
			if !knownFormats[s.Value] {
				bad(app, "%s の名前 %q を知らない（email / uri / uuid / date-time）", app.Decorator, s.Value)
				continue
			}
			c.format = s.Value
		default:
			if !coreOK {
				bad(app, "%s は核の型を元に持つ位置にだけ付けられるが、%s は %s", app.Decorator, subject, coreName(core, coreOK))
				continue
			}
			c.user = true
			c.userCore = core
			c.decl, _ = e.lookup(app.Decorator).(*ir.Decorator)
		}
		out = append(out, c)
	}
	return out
}

func coreName(c ir.CoreName, ok bool) string {
	if !ok {
		return "核の型でない"
	}
	return string(c)
}

// argLiteral は適用の引数 param のリテラル。
func argLiteral(app *ir.Application, param string) (ir.Literal, bool) {
	for _, a := range app.Args {
		if a.Param == param {
			if l, ok := a.Value.(ir.LiteralValue); ok {
				return l.Literal, true
			}
		}
	}
	return nil, false
}

// numberOf はリテラルの数値と、int リテラルだったか。
func numberOf(l ir.Literal) (float64, bool) {
	switch x := l.(type) {
	case ir.IntLiteral:
		return float64(x.Value), true
	case ir.FloatLiteral:
		return x.Value, false
	case ir.DurationLiteral:
		return float64(x.Value), true
	}
	return 0, false
}

// scalarConstraints は scalar の制約を、元の方（継承の根）から順に。report は自分の分だけ出す
// （元の scalar の分はその scalar の Validate を作るときに出る）。
func (e *emitter) scalarConstraints(s *ir.Scalar, report bool) []constraint {
	chain := e.scalarChain(s)
	core, ok := e.coreOf(s.Base)
	var out []constraint
	for i := len(chain) - 1; i >= 0; i-- {
		out = append(out, e.constraintsOf(chain[i].Decorators, core, ok, e.typeName(chain[i].Name), chain[i].Name, report && i == 0)...)
	}
	return out
}

// checkDefault は GOEMIT-4（docs/emit-go.md「model」）: 既定値が scalar の標準の制約と、
// フィールド自身の valid.* に合うか。ユーザーの制約は生成時に評価できないので見ない。
func (e *emitter) checkDefault(m *ir.Model, f *ir.Field) {
	var cs []constraint
	if s := e.scalarOf(f.Type); s != nil {
		cs = append(cs, e.scalarConstraints(s, false)...)
	}
	core, ok := e.coreOf(f.Type)
	cs = append(cs, e.constraintsOf(f.Decorators, core, ok, e.typeName(m.Name)+gol.FieldName(f.Name), "", false)...)
	for _, c := range cs {
		if c.user {
			continue
		}
		if !satisfies(c, f.Default) {
			e.errorf(CodeBadDefault, &f.Span, "model %s のフィールド %s の既定値 %s は制約 %s（%s）に合わない",
				m.Name, f.Name, plainLiteral(f.Default), c.name, describeConstraint(c))
		}
	}
}

func satisfies(c constraint, l ir.Literal) bool {
	switch c.app.Decorator {
	case validMin:
		v, _ := numberOf(l)
		return v >= c.min
	case validMax:
		v, _ := numberOf(l)
		return v <= c.max
	case validLength:
		s, ok := l.(ir.StringLiteral)
		if !ok {
			return true
		}
		n := int64(utf8.RuneCountInString(s.Value))
		return !(c.hasMin && n < c.minInt) && !(c.hasMax && n > c.maxInt)
	case validPattern:
		s, ok := l.(ir.StringLiteral)
		if !ok {
			return true
		}
		re, err := regexp.Compile(c.regex)
		return err != nil || re.MatchString(s.Value)
	case validFormat:
		s, ok := l.(ir.StringLiteral)
		if !ok {
			return true
		}
		return runtime.CheckFormat(c.format, s.Value)
	}
	return true
}

func describeConstraint(c constraint) string {
	switch c.app.Decorator {
	case validMin:
		return "min " + strconv.FormatFloat(c.min, 'g', -1, 64)
	case validMax:
		return "max " + strconv.FormatFloat(c.max, 'g', -1, 64)
	case validLength:
		var parts []string
		if c.hasMin {
			parts = append(parts, "min "+strconv.FormatInt(c.minInt, 10))
		}
		if c.hasMax {
			parts = append(parts, "max "+strconv.FormatInt(c.maxInt, 10))
		}
		return "length " + strings.Join(parts, ", ")
	case validPattern:
		return "pattern " + strconv.Quote(c.regex)
	case validFormat:
		return "format " + c.format
	}
	return c.name
}

// userConstraint は ConstraintFuncs のフィールド一つ（valid に足した制約。docs/emit-go.md「scalar」）。
type userConstraint struct {
	decorator string // FQN
	decl      *ir.Decorator
	core      ir.CoreName
}

// validateGen は validate.go を作る途中の状態。
type validateGen struct {
	g        *genFile
	users    []userConstraint
	patterns []constraint
	needHelp bool // appendViolations が要る
	needUTF8 bool
	needConv bool // strconv が要る
}

// rt は runtime パッケージの修飾名。使うときに import に足す（使わなければ import しない）。
func (v *validateGen) rt() string { return v.g.imports.Add(runtimePath, "runtime") }

// genValidate は validate.go（docs/emit-go.md「scalar」の Validate、「model の Validate」、ConstraintFuncs）。
// pattern の変数、制約ごとの関数の型と ConstraintFuncs、scalar と model の Validate（IR の順）、appendViolations の順に出す。
func (e *emitter) genValidate() *genFile {
	if len(e.scalars) == 0 && len(e.models) == 0 {
		return nil
	}
	v := &validateGen{g: e.newFile("validate.go")}
	var methods strings.Builder
	for _, s := range e.scalars {
		e.genScalarValidate(v, s)
	}
	for _, m := range e.models {
		e.genModelValidate(v, m)
	}
	methods.WriteString(v.g.body.String())
	v.g.body.Reset()
	g := v.g
	if len(v.patterns) > 0 {
		re := g.imports.Add("regexp", "regexp")
		for _, c := range v.patterns {
			g.printf("var %s = %s.MustCompile(%q)\n", c.patternVar, re, c.regex)
		}
		g.println("")
	}
	if len(v.users) > 0 {
		for _, u := range v.users {
			name := constraintFuncName(u.decorator)
			text := "制約 " + u.decorator + " の関数の型。値がその制約を満たせば true。"
			if u.decl != nil && u.decl.Description != nil {
				text += "\n" + strings.TrimSpace(u.decl.Description.Body)
			}
			g.body.WriteString(comment(joinJa(name+" は", text)))
			g.printf("type %s func(%s) bool\n\n", name, e.userConstraintParams(u, &g.imports))
		}
		g.println("// ConstraintFuncs は valid に足した制約の実装（設計書 §6.5）を束ねる。")
		g.println("type ConstraintFuncs struct {")
		for _, u := range v.users {
			g.printf("\t%s %s\n", constraintField(u.decorator), constraintFuncName(u.decorator))
		}
		g.println("}\n")
		g.println("// Constraints は valid に足した制約の実装。人がフィールドを設定する。")
		g.println("// 設定していない制約に Validate が当たったら *runtime.MissingError（Kind は \"constraint\"）。")
		g.println("var Constraints ConstraintFuncs\n")
	}
	if v.needUTF8 {
		g.imports.Add("unicode/utf8", "utf8")
	}
	if v.needConv {
		g.imports.Add("strconv", "strconv")
	}
	g.body.WriteString(methods.String())
	if v.needHelp {
		errs := g.imports.Add("errors", "errors")
		g.println("// appendViolations は err が *runtime.ValidationError なら、その違反の Path に path を前置して dst に足す。")
		g.printf("func appendViolations(dst []%s.Violation, path string, err error) []%s.Violation {\n", v.rt(), v.rt())
		g.println("\tif err == nil {\n\t\treturn dst\n\t}")
		g.printf("\tvar ve *%s.ValidationError\n", v.rt())
		g.printf("\tif !%s.As(err, &ve) {\n\t\treturn append(dst, %s.Violation{Path: path, Constraint: err.Error()})\n\t}\n", errs, v.rt())
		g.println("\tfor _, x := range ve.Violations {\n\t\tp := path\n\t\tif x.Path != \"\" {\n\t\t\tp = path + \".\" + x.Path\n\t\t}")
		g.printf("\t\tdst = append(dst, %s.Violation{Path: p, Constraint: x.Constraint})\n\t}\n\treturn dst\n}\n", v.rt())
	}
	return g
}

// constraintField は ConstraintFuncs のフィールド名: 制約の宣言名を先頭大文字にしたもの。
func constraintField(decorator string) string { return gol.Capitalize(localName(decorator)) }

// constraintFuncName は制約の関数の型の名前 <制約>Func。
func constraintFuncName(decorator string) string { return constraintField(decorator) + "Func" }

// userConstraintParams は制約の関数の型の引数: `v <元の型>` と、デコレータの引数（あれば）。
func (e *emitter) userConstraintParams(u userConstraint, imp *gol.Imports) string {
	parts := []string{"v " + coreGoTypeImp(u.core, imp)}
	if u.decl != nil {
		for _, p := range u.decl.Params {
			parts = append(parts, local(p.Name)+" "+e.paramGoType(p, imp))
		}
	}
	return strings.Join(parts, ", ")
}

// paramGoType はデコレータの引数の Go の型。type なら最初の型、any なら FQN の string。
func (e *emitter) paramGoType(p ir.Param, imp *gol.Imports) string {
	if t, ok := p.Type.(ir.TypeParam); ok && len(t.Types) > 0 {
		return gol.GoType(t.Types[0], imp)
	}
	return "string"
}

func coreGoTypeImp(c ir.CoreName, imp *gol.Imports) string {
	return gol.GoType(ir.CoreType{Name: c}, imp)
}

// indent は各行の先頭に prefix を付ける。
func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// genScalarValidate は scalar の Validate（docs/emit-go.md「scalar」）。継承した制約も確かめる。
func (e *emitter) genScalarValidate(v *validateGen, s *ir.Scalar) {
	g := v.g
	name := e.typeName(s.Name)
	cs := e.scalarConstraints(s, true)
	g.printf("// Validate は %s の制約を確かめる。違反がなければ nil。\n", s.Name)
	g.printf("func (v %s) Validate() error {\n", name)
	if len(cs) == 0 {
		g.println("\treturn nil\n}\n")
		return
	}
	g.printf("\tvar violations []%s.Violation\n", v.rt())
	for _, c := range cs {
		e.genCheck(v, c, "v", `""`, "\t")
	}
	g.println("\tif len(violations) == 0 {\n\t\treturn nil\n\t}")
	g.printf("\treturn &%s.ValidationError{Violations: violations}\n}\n\n", v.rt())
}

// genCheck は制約一つの検査。expr は元の型に変換できる値の式、pathExpr は Path の Go の式。
func (e *emitter) genCheck(v *validateGen, c constraint, expr, pathExpr, ind string) {
	g := v.g
	violate := func() string {
		return fmt.Sprintf("%s\tviolations = append(violations, %s.Violation{Path: %s, Constraint: %q})\n%s}\n", ind, v.rt(), pathExpr, c.name, ind)
	}
	switch c.app.Decorator {
	case validMin, validMax:
		op, val := "<", c.min
		if c.app.Decorator == validMax {
			op, val = ">", c.max
		}
		if c.intArgs {
			g.printf("%sif int64(%s) %s %d {\n", ind, expr, op, int64(val))
		} else {
			g.printf("%sif float64(%s) %s %s {\n", ind, expr, op, strconv.FormatFloat(val, 'g', -1, 64))
		}
		g.body.WriteString(violate())
	case validLength:
		v.needUTF8 = true
		var conds []string
		if c.hasMin {
			conds = append(conds, fmt.Sprintf("l < %d", c.minInt))
		}
		if c.hasMax {
			conds = append(conds, fmt.Sprintf("l > %d", c.maxInt))
		}
		if len(conds) == 0 {
			return
		}
		g.printf("%sif l := utf8.RuneCountInString(string(%s)); %s {\n", ind, expr, strings.Join(conds, " || "))
		g.body.WriteString(violate())
	case validPattern:
		v.patterns = append(v.patterns, c)
		g.printf("%sif !%s.MatchString(string(%s)) {\n", ind, c.patternVar, expr)
		g.body.WriteString(violate())
	case validFormat:
		g.printf("%sif !%s.CheckFormat(%q, string(%s)) {\n", ind, v.rt(), c.format, expr)
		g.body.WriteString(violate())
	default:
		if !c.user {
			return
		}
		u := e.registerUser(v, c)
		args := []string{coreGoTypeImp(u.core, &g.imports) + "(" + expr + ")"}
		if c.decl != nil {
			for _, p := range c.decl.Params {
				args = append(args, e.decoratorArgExpr(&c.app, p, &g.imports))
			}
		}
		// 設定していない制約に当たったら、違反ではなく *runtime.MissingError を返す（docs/emit-go.md「実装の単位」）。
		fn := "Constraints." + constraintField(c.app.Decorator)
		g.printf("%sif %s == nil {\n%s\treturn %s\n%s}\n", ind, fn, ind, missing(&g.imports, missingConstraint, c.app.Decorator), ind)
		g.printf("%sif !%s(%s) {\n", ind, fn, strings.Join(args, ", "))
		g.body.WriteString(violate())
	}
}

// registerUser はユーザーの制約を ConstraintFuncs のフィールドとして登録する。
func (e *emitter) registerUser(v *validateGen, c constraint) userConstraint {
	for _, u := range v.users {
		if u.decorator == c.app.Decorator {
			if u.core != c.userCore {
				e.errorf(CodeBadConstraint, &c.app.Span, "%s は元の型 %s の位置にも %s の位置にも付いているが、制約の関数の型は一つの型しか取れない", c.app.Decorator, u.core, c.userCore)
			}
			return u
		}
	}
	u := userConstraint{decorator: c.app.Decorator, decl: c.decl, core: c.userCore}
	v.users = append(v.users, u)
	return u
}

// decoratorArgExpr は制約の関数に渡す、デコレータの引数 p の値の Go の式。
func (e *emitter) decoratorArgExpr(app *ir.Application, p ir.Param, imp *gol.Imports) string {
	var target ir.Type
	if t, ok := p.Type.(ir.TypeParam); ok && len(t.Types) > 0 {
		target = t.Types[0]
	}
	for _, a := range app.Args {
		if a.Param != p.Name {
			continue
		}
		switch val := a.Value.(type) {
		case ir.LiteralValue:
			return e.literalFor(val.Literal, target, false, imp)
		case ir.RefValue:
			return strconv.Quote(val.Target)
		case ir.RefsValue:
			return strconv.Quote(strings.Join(val.Targets, " | "))
		case ir.VariantValue:
			return strconv.Quote(val.Union + "." + val.Arm)
		}
		return `""`
	}
	if target != nil {
		return e.zeroExpr(target, imp)
	}
	return `""`
}

// genModelValidate は model の Validate（docs/emit-go.md「model の Validate」）。
func (e *emitter) genModelValidate(v *validateGen, m *ir.Model) {
	g := v.g
	name := e.typeName(m.Name)
	// 一度別のバッファに書き、何もなければ return nil だけにする。
	var cs [][]constraint
	for _, f := range m.Fields {
		core, ok := e.coreOf(f.Type)
		cs = append(cs, e.constraintsOf(f.Decorators, core, ok, name+gol.FieldName(f.Name), m.Name+" のフィールド "+f.Name, true))
	}
	mark := g.body.Len()
	for i, f := range m.Fields {
		e.genFieldValidate(v, "m."+gol.FieldName(f.Name), strconv.Quote(f.Name), f.Type, cs[i], "\t", 0)
	}
	body := string(g.body.Bytes()[mark:])
	g.body.Truncate(mark)
	g.printf("// Validate は %s の各フィールドの制約を確かめる。違反の Path はフィールド名を . でつないだもの。\n", m.Name)
	g.printf("func (m %s) Validate() error {\n", name)
	if body == "" {
		g.println("\treturn nil\n}\n")
		return
	}
	g.printf("\tvar violations []%s.Violation\n", v.rt())
	g.body.WriteString(body)
	g.println("\tif len(violations) == 0 {\n\t\treturn nil\n\t}")
	g.printf("\treturn &%s.ValidationError{Violations: violations}\n}\n\n", v.rt())
}

// needsValidate は型の値に Validate を呼ぶ（か辿る）必要があるか。
func (e *emitter) needsValidate(t ir.Type) bool {
	t, _ = stripOptional(t)
	switch x := t.(type) {
	case ir.RefType:
		return e.model(x.Target) != nil || e.scalar(x.Target) != nil
	case ir.ArrayType:
		return e.needsValidate(x.Of)
	case ir.RecordType:
		return e.needsValidate(x.Value)
	}
	return false
}

// concatPath は Path の Go の式をつなぐ。隣り合う文字列リテラルは一つにする（`"items"+"["` → `"items["`）。
func concatPath(a, b string) string {
	if strings.HasSuffix(a, `"`) && strings.HasPrefix(b, `"`) {
		return a[:len(a)-1] + b[1:]
	}
	return a + "+" + b
}

// genFieldValidate はフィールド（か要素）一つの検査。expr は値の式、pathExpr は Path の Go の式。
// cs はフィールドに付いた制約（要素には付かない）。
func (e *emitter) genFieldValidate(v *validateGen, expr, pathExpr string, t ir.Type, cs []constraint, ind string, depth int) {
	g := v.g
	inner, opt := stripOptional(t)
	if !e.needsValidate(t) && len(cs) == 0 {
		return
	}
	if opt && !e.isNillable(inner) {
		x := fmt.Sprintf("x%d", depth)
		g.printf("%sif %s != nil {\n%s\t%s := *%s\n", ind, expr, ind, x, expr)
		e.genValueValidate(v, x, pathExpr, inner, cs, ind+"\t", depth+1)
		g.printf("%s}\n", ind)
		return
	}
	e.genValueValidate(v, expr, pathExpr, inner, cs, ind, depth)
}

func (e *emitter) genValueValidate(v *validateGen, expr, pathExpr string, t ir.Type, cs []constraint, ind string, depth int) {
	g := v.g
	switch x := t.(type) {
	case ir.RefType:
		if e.model(x.Target) != nil || e.scalar(x.Target) != nil {
			v.needHelp = true
			if e.mayMiss(x.Target, map[string]bool{}) {
				// 中の Validate が設定していない制約に当たったら、その *runtime.MissingError をそのまま返す。
				errs := g.imports.Add("errors", "errors")
				g.printf("%sif err := %s.Validate(); err != nil {\n", ind, expr)
				g.printf("%s\tvar miss *%s.MissingError\n%s\tif %s.As(err, &miss) {\n%s\t\treturn err\n%s\t}\n", ind, v.rt(), ind, errs, ind, ind)
				g.printf("%s\tviolations = appendViolations(violations, %s, err)\n%s}\n", ind, pathExpr, ind)
			} else {
				g.printf("%sviolations = appendViolations(violations, %s, %s.Validate())\n", ind, pathExpr, expr)
			}
		}
	case ir.ArrayType:
		if e.needsValidate(x.Of) {
			v.needConv = true
			i, el := fmt.Sprintf("i%d", depth), fmt.Sprintf("x%d", depth)
			g.printf("%sfor %s, %s := range %s {\n", ind, i, el, expr)
			e.genFieldValidate(v, el, concatPath(pathExpr, `"["+strconv.Itoa(`+i+`)+"]"`), x.Of, nil, ind+"\t", depth+1)
			g.printf("%s}\n", ind)
		}
	case ir.RecordType:
		if e.needsValidate(x.Value) {
			k, el := fmt.Sprintf("k%d", depth), fmt.Sprintf("x%d", depth)
			g.printf("%sfor %s, %s := range %s {\n", ind, k, el, expr)
			e.genFieldValidate(v, el, concatPath(pathExpr, `"["+string(`+k+`)+"]"`), x.Value, nil, ind+"\t", depth+1)
			g.printf("%s}\n", ind)
		}
	}
	for _, c := range cs {
		e.genCheck(v, c, expr, pathExpr, ind)
	}
}

// mayMiss は、型 fqn（scalar か model）の Validate が *runtime.MissingError を返しうるか: valid に足した制約を
// （継承やフィールドを通して）確かめるか。seen は辿っている途中の型（model が自分に戻る参照を止める）。
func (e *emitter) mayMiss(fqn string, seen map[string]bool) bool {
	if seen[fqn] {
		return false
	}
	seen[fqn] = true
	if s := e.scalar(fqn); s != nil {
		for _, c := range e.scalarConstraints(s, false) {
			if c.user {
				return true
			}
		}
		return false
	}
	m := e.model(fqn)
	if m == nil {
		return false
	}
	for _, f := range m.Fields {
		core, ok := e.coreOf(f.Type)
		for _, c := range e.constraintsOf(f.Decorators, core, ok, "", "", false) {
			if c.user {
				return true
			}
		}
		miss := false
		forEachRef(f.Type, func(target string) {
			if e.mayMiss(target, seen) {
				miss = true
			}
		})
		if miss {
			return true
		}
	}
	return false
}
