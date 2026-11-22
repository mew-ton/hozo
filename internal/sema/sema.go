package sema

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/ir"
)

// Result は Analyze の結果（docs/packages.md「internal/sema」）。
type Result struct {
	// Program は正規化した IR。誤りがあっても作る。Op.Signature と Call.Bound は零値（internal/check が埋める）。
	Program *ir.Program
	// Refs は解決した参照。定義へジャンプとホバーが使う。解決した名前の節（セグメント）ごとに一つ。
	Refs []Ref
	// Diags は解決・型・正規化の診断。docs/diagnostics.md「順序」で並べ替え済み。
	Diags []diag.Diagnostic
}

// Ref はトークンの範囲から解決先への対応。
type Ref struct {
	Span   source.Span
	Target Target
}

// TargetKind は解決先の種類。
type TargetKind int

// 解決先の種類（docs/packages.md「internal/sema」の Target）。
const (
	// TargetDecl は宣言。Decl がその FQN。
	TargetDecl TargetKind = iota
	// TargetField は model のフィールド。Decl は model の FQN、Name はフィールド名。
	TargetField
	// TargetArm はユニオンのアーム。Decl はユニオンの FQN、Name はアーム名。
	TargetArm
	// TargetCase は classify のケース。Decl は classify の FQN、Name はケース名。
	TargetCase
	// TargetDecoratorParam はデコレータの Param。Decl はデコレータの FQN、Name は引数名。
	TargetDecoratorParam
	// TargetBinding は配線の束縛。Decl は op の FQN、Name は束縛名。
	TargetBinding
	// TargetOpParam は op の引数。Decl は op の FQN、Name は引数名。
	TargetOpParam
	// TargetNamespace は名前空間（修飾の先頭と bind の対象）。Decl はその FQN。
	TargetNamespace
)

// String は種類の名前。
func (k TargetKind) String() string {
	switch k {
	case TargetDecl:
		return "decl"
	case TargetField:
		return "field"
	case TargetArm:
		return "arm"
	case TargetCase:
		return "case"
	case TargetDecoratorParam:
		return "decoratorParam"
	case TargetBinding:
		return "binding"
	case TargetOpParam:
		return "opParam"
	case TargetNamespace:
		return "namespace"
	}
	return "?"
}

// Target は解決先。
type Target struct {
	Kind TargetKind
	// Decl は宣言の FQN（TargetDecl ならそれ自身、ほかは持ち主）。TargetNamespace なら名前空間の FQN。
	Decl string
	// Name はフィールド・アーム・ケース・引数・束縛の名前。TargetDecl / TargetNamespace では空。
	Name string
	// Span は定義の位置（名前のトークン）。
	Span source.Span
}

// Analyze は構文木の列から IR を作る（docs/packages.md「internal/sema」）。
//
// files の順は docs/ir.md「原則」に従って並べ直す（ユーザーのファイルをパスの辞書順、その後に "hozo:" のファイルを辞書順）。
// 語彙のファイルもほかのファイルと同じ規則で扱う（docs/vocab.md「読み込み」）。設定は見ない
// （CFG-* は internal/config、internal/check、internal/project）。
func Analyze(fs *source.FileSet, files []*ast.File) *Result {
	return analyze(fs, files).result()
}

// analyze は Analyze の中身。テストが analyzer の表と互換の判定に触るために分けてある。
func analyze(fs *source.FileSet, files []*ast.File) *analyzer {
	a := newAnalyzer(fs, files)
	a.buildTable()
	a.lowerAll()
	a.lowerApplications()
	a.finalizeModels()
	a.checkFiniteTypes()
	a.checkNeverOps()
	a.lowerWirings()
	a.checkPendingCaseTypes()
	return a
}

// analyzer は一度の解析の状態。
type analyzer struct {
	fs        *source.FileSet
	files     []*ast.File
	fileOrder map[source.FileID]int

	global  *namespace
	nss     map[string]*namespace // FQN → 名前空間（暗黙の親を含む）
	nsOrder []*namespace          // 出現順。先頭はグローバル
	// vocabNS は、語彙のファイルが宣言した名前空間の FQN → はじめに宣言した語彙のファイル（RES-8）。
	vocabNS map[string]source.FileID
	syms    map[string]*symbol   // FQN → 表に残った宣言
	models  map[string]*ir.Model // すべての model（合成したものを含む）
	fields  map[string]map[string]source.Span

	pendingBinds []pendingBind
	binds        []ir.Bind
	sites        []*appSite
	wirings      []*wiringJob
	bodies       []*modelJob
	neverOps     []*neverOp
	// carriers は、op ごとの、reads か effects を運んだデコレータの適用（書かれた順）。
	carriers         map[*ir.Op][]carrier
	pendingCaseTypes []pendingCaseType

	diags []diag.Diagnostic
	refs  []Ref
}

func newAnalyzer(fs *source.FileSet, files []*ast.File) *analyzer {
	a := &analyzer{
		fs:        fs,
		fileOrder: map[source.FileID]int{},
		nss:       map[string]*namespace{},
		syms:      map[string]*symbol{},
		models:    map[string]*ir.Model{},
		fields:    map[string]map[string]source.Span{},
		carriers:  map[*ir.Op][]carrier{},
	}
	a.files = sortFiles(fs, files)
	for i, f := range a.files {
		a.fileOrder[f.Span.File] = i
	}
	a.global = &namespace{declared: true, children: map[string]*namespace{}, syms: map[string]*symbol{}}
	a.nss[""] = a.global
	a.nsOrder = []*namespace{a.global}
	return a
}

// sortFiles は docs/ir.md「原則」のファイルの順に並べた複製を返す。nil のファイルは除く。
func sortFiles(fs *source.FileSet, files []*ast.File) []*ast.File {
	out := make([]*ast.File, 0, len(files))
	for _, f := range files {
		if f != nil {
			out = append(out, f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return diag.PathOrder(fs.Path(out[i].Span.File), fs.Path(out[j].Span.File)) < 0
	})
	return out
}

// result は Program を組み立て、診断と参照を並べ替えて返す。
func (a *analyzer) result() *Result {
	p := &ir.Program{Version: ir.Version, Binds: a.binds}
	for _, ns := range a.nsOrder {
		if !ns.declared && ns != a.global {
			continue
		}
		n := ir.Namespace{Name: ns.fqn, Description: ns.description}
		for _, s := range ns.order {
			if s.decl != nil {
				n.Decls = append(n.Decls, s.decl)
			}
			n.Decls = append(n.Decls, s.synthesized...)
		}
		p.Namespaces = append(p.Namespaces, n)
	}
	diag.Sort(a.diags, a.fs, nil)
	sort.SliceStable(a.refs, func(i, j int) bool {
		ri, rj := a.refs[i].Span, a.refs[j].Span
		if ri.File != rj.File {
			return a.fileOrder[ri.File] < a.fileOrder[rj.File]
		}
		return ri.Start < rj.Start
	})
	return &Result{Program: p, Refs: a.refs, Diags: a.diags}
}

// --- 診断と位置 ---

func (a *analyzer) report(sev diag.Severity, code string, span source.Span, format string, args ...any) *diag.Diagnostic {
	s := span
	a.diags = append(a.diags, diag.Diagnostic{Severity: sev, Code: code, Message: fmt.Sprintf(format, args...), Span: &s})
	return &a.diags[len(a.diags)-1]
}

func (a *analyzer) errorf(code string, span source.Span, format string, args ...any) *diag.Diagnostic {
	return a.report(diag.Error, code, span, format, args...)
}

func (a *analyzer) warnf(code string, span source.Span, format string, args ...any) *diag.Diagnostic {
	return a.report(diag.Warning, code, span, format, args...)
}

// irSpan はバイトの範囲を IR の行・列の位置にする（docs/ir.md「Program」の Span）。
func (a *analyzer) irSpan(s source.Span) ir.Span {
	return ir.Span{
		File:  a.fs.Path(s.File),
		Start: irPos(a.fs.Position(s.File, s.Start)),
		End:   irPos(a.fs.Position(s.File, s.End)),
	}
}

func irPos(p source.Pos) ir.Pos { return ir.Pos{Line: p.Line, Col: p.Col} }

// isVocab は語彙のファイル（Span.file が "hozo:" で始まる。docs/ir.md「Program」）かどうか。
func (a *analyzer) isVocab(f source.FileID) bool {
	return strings.HasPrefix(a.fs.Path(f), "hozo:")
}

// addRef は解決した参照を記録する。
func (a *analyzer) addRef(span source.Span, t Target) {
	a.refs = append(a.refs, Ref{Span: span, Target: t})
}

// text は範囲の元の文字列（診断に、書かれたままの形を出すため）。
func (a *analyzer) text(s source.Span) string {
	content := a.fs.Content(s.File)
	if int(s.Start) < 0 || int(s.End) > len(content) || s.Start > s.End {
		return ""
	}
	return string(content[s.Start:s.End])
}

// join は FQN の表示用に名前の列をつなぐ。
func join(parts []string) string { return strings.Join(parts, ", ") }
