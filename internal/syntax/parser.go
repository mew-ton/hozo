package syntax

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mew-ton/hozo/internal/diag"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax/ast"
	"github.com/mew-ton/hozo/internal/syntax/token"
)

// Parse はファイルを構文解析し、構文木と診断を返す（docs/packages.md「internal/syntax/token、internal/syntax/ast、internal/syntax」）。
// 構文エラーの Code は "SYNTAX"。誤りがあっても読めた宣言は構文木に残す。
// 誤りからの回復は、宣言の中なら次の ";" か "}" まで、ブロックの中なら次の ";" まで読み飛ばし、SYNTAX を一つ出して続ける。
func Parse(fs *source.FileSet, f source.FileID) (*ast.File, []diag.Diagnostic) {
	src := fs.Content(f)
	toks, diags := lex(f, src)
	p := &parser{file: f, toks: toks, diags: diags}
	file := &ast.File{Span: source.Span{File: f, Start: 0, End: source.Offset(len(src))}}
	for !p.at(token.EOF) {
		file.Decls = append(file.Decls, p.parseDecls(token.EOF)...)
		if p.at(token.RBrace) {
			p.errorAt(p.advance().Span, "対応する「{」のない「}」")
		}
	}
	return file, p.diags
}

// parser は再帰下降の構文解析器。
type parser struct {
	file  source.FileID
	toks  []token.Token
	pos   int
	diags []diag.Diagnostic
	// openRecords は読みかけのレコードリテラルの "{" の数。誤りからの回復（skip）が、文の途中の "}" を
	// ブロックの終わりと取り違えないために数える。
	openRecords int
	// noOnType は、型の後ろの on を、絞り込んだ型（OnType）として読まないか: 関数の戻り値の後ろの、
	// なくした書き方の on（`read fn r(): T on 状態;`、`fn f(): T on 状態;`）を、呼んだ側が、メッセージにするため。on の後ろが
	// 「分類.ケース」の形なら、このときも、絞り込んだ型として読む。
	noOnType bool
}

// bailout は誤りからの回復のために投げる印。宣言・メンバー・文の単位で受ける。
type bailout struct{}

// --- 字句の読み ---

func (p *parser) tok() token.Token { return p.toks[p.pos] }

func (p *parser) peek(n int) token.Token {
	if p.pos+n < len(p.toks) {
		return p.toks[p.pos+n]
	}
	return p.toks[len(p.toks)-1]
}

func (p *parser) at(kind token.Kind) bool { return p.tok().Kind == kind }

// atIdent は tok が識別子（文脈キーワードを含む）かを返す。
func (p *parser) atIdent() bool { return p.at(token.Ident) }

// atName は tok が Ident か QualifiedName かを返す。
func (p *parser) atName() bool { return p.at(token.Ident) || p.at(token.QualifiedName) }

// atContextual は tok が指定の文脈キーワードかを返す。
func (p *parser) atContextual(word string) bool { return p.at(token.Ident) && p.tok().Text == word }

// atTypeWord は tok が、型の位置の語 word（核の型と形の名前。docs/grammar.md「予約語」「型」）かを返す。
// 字句としては Ident で、型の位置でだけ、この読み方をする。
func (p *parser) atTypeWord(word string) bool { return p.at(token.Ident) && p.tok().Text == word }

// gotTypeWord は、型の位置の語 word があれば読んで true。
func (p *parser) gotTypeWord(word string) bool {
	if p.atTypeWord(word) {
		p.advance()
		return true
	}
	return false
}

func (p *parser) advance() token.Token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

// prevEnd は直前に読んだ字句の終わり。節の Span の End に使う。
func (p *parser) prevEnd() source.Offset {
	if p.pos == 0 {
		return 0
	}
	return p.toks[p.pos-1].Span.End
}

func (p *parser) spanFrom(start source.Offset) source.Span {
	return source.Span{File: p.file, Start: start, End: p.prevEnd()}
}

// expect は kind の字句を読む。違えば誤りにして回復へ抜ける。
func (p *parser) expect(kind token.Kind) token.Token {
	if !p.at(kind) {
		p.failExpected("「" + kind.String() + "」")
	}
	return p.advance()
}

// got は kind の字句があれば読んで true。
func (p *parser) got(kind token.Kind) bool {
	if p.at(kind) {
		p.advance()
		return true
	}
	return false
}

// --- 誤り ---

// errorAt は回復なしで誤りを記録する。解析を続けられる誤り（名前付き引数の後ろの位置引数など）に使う。
func (p *parser) errorAt(span source.Span, message string) {
	p.diags = append(p.diags, syntaxError(span, message))
}

// fail は誤りを記録して回復へ抜ける。
func (p *parser) fail(span source.Span, message string) {
	p.errorAt(span, message)
	panic(bailout{})
}

// failLiteral はリテラルが 64 ビットに収まらない誤り（LIT-1。docs/types.md「診断」）を記録して回復へ抜ける。
func (p *parser) failLiteral(span source.Span, message string) {
	p.diags = append(p.diags, codedError("LIT-1", span, message))
	panic(bailout{})
}

// failExpected は「X が必要だが Y がある」の誤り。Illegal の字句なら字句解析器が出しているので重ねて出さない。
func (p *parser) failExpected(what string) {
	t := p.tok()
	if t.Kind == token.Illegal {
		panic(bailout{})
	}
	p.fail(t.Span, fmt.Sprintf("%sが必要だが%sがある", what, describe(t)))
}

// describe は字句を「「;」」「識別子 foo」「ファイルの終わり」のように表す。
func describe(t token.Token) string {
	switch t.Kind {
	case token.EOF:
		return "ファイルの終わり"
	case token.Ident, token.QualifiedName, token.String, token.Int, token.Float, token.Duration:
		return fmt.Sprintf("%s %s", t.Kind, t.Text)
	case token.DocComment:
		return t.Kind.String()
	}
	return "「" + t.Kind.String() + "」"
}

// recover は fn を実行し、bailout が起きたら skip で読み飛ばして false を返す。
func (p *parser) recover(fn func()) (ok bool) {
	outer := p.openRecords
	p.openRecords = 0
	defer func() {
		if r := recover(); r != nil {
			if _, is := r.(bailout); !is {
				panic(r)
			}
			p.skip()
			ok = false
		}
		p.openRecords = outer
	}()
	fn()
	return true
}

// skip は次の ";"（読む）か、対応のない "}"（読まない）か、ファイルの終わりまで読み飛ばす
// （docs/packages.md「internal/syntax/token、internal/syntax/ast、internal/syntax」の回復）。
// 読み飛ばす途中で開いた "{" は、閉じるまで ";" を区切りにしない。閉じたらそこで止まる
// （本体を持つ宣言や match の終わり）。その直後の ";" は一緒に読む。
// 誤りがレコードリテラルの中で起きたときは、先に読みかけのレコードリテラルを閉じるまで読む
// （その "}" は文の途中で、ブロックの終わりではない）。
func (p *parser) skip() {
	for p.openRecords > 0 && !p.at(token.EOF) {
		switch p.tok().Kind {
		case token.LBrace:
			p.openRecords++
		case token.RBrace:
			p.openRecords--
		}
		p.advance()
	}
	p.openRecords = 0
	depth := 0
	for {
		switch p.tok().Kind {
		case token.EOF:
			return
		case token.Semicolon:
			p.advance()
			if depth == 0 {
				return
			}
		case token.LBrace:
			depth++
			p.advance()
		case token.RBrace:
			if depth == 0 {
				return
			}
			depth--
			p.advance()
			if depth == 0 {
				p.got(token.Semicolon)
				return
			}
		default:
			p.advance()
		}
	}
}

// --- ファイルと宣言（docs/grammar.md「ファイルと宣言」） ---

// parseDecls は until（EOF か "}"）まで宣言を読む。誤った宣言は読み飛ばし、読めた宣言だけを返す。
func (p *parser) parseDecls(until token.Kind) []ast.Decl {
	var decls []ast.Decl
	for !p.at(token.EOF) && !p.at(until) {
		if p.at(token.RBrace) {
			// until が EOF のときの余った "}"。Parse が誤りにする。
			return decls
		}
		before := p.pos
		var d ast.Decl
		if p.recover(func() { d = p.parseDecl() }) && d != nil {
			decls = append(decls, d)
		}
		if p.pos == before {
			// 進まなければ無限ループになるので一つ捨てる。
			p.advance()
		}
	}
	return decls
}

// parseDecl は Decl を一つ読む。
//
//	Decl = [ DocComment ] { Decorator } Declaration | [ DocComment ] NamespaceDecl | BindDecl ;
//
// 節の Span は DocComment を含まず、最初のデコレータか宣言の頭（op なら export）から始まる。
func (p *parser) parseDecl() ast.Decl {
	doc := p.parseLeadingDoc()
	start := p.tok().Span.Start
	decorators, marks := p.parseDecorators()
	if p.at(token.DocComment) {
		p.fail(p.tok().Span, "ドキュメンテーションコメントはデコレータの前に置く")
	}
	if !p.at(token.Namespace) && !p.at(token.Bind) {
		// 説明のコメントの中の、デコレータのタグは、宣言の前の適用と、同じ（docs/grammar.md「ドキュメンテーションコメント」）
		decorators = docDecorators(doc, decorators)
	}
	if p.at(token.Export) {
		export := p.advance().Span
		if p.at(token.Op) {
			p.failOldFnHead()
		}
		if !p.at(token.Fn) {
			p.fail(export, "export を付けられるのは、種類を書かない fn（op）だけで、fn の直前に書く（docs/grammar.md「op」）")
		}
		o := p.parseOp(start, doc, decorators)
		o.Export, o.ExportSpan = true, export
		return o
	}
	switch p.tok().Kind {
	case token.Namespace:
		if len(decorators) > 0 {
			p.fail(decorators[0].Span, "namespace にデコレータは付けられない（docs/grammar.md「決めたこと」）")
		}
		if tag := firstDocApply(doc); tag != nil {
			p.errorAt(tag.Span, "namespace にデコレータは付けられない（説明のコメントの中の、デコレータのタグも、同じ。docs/grammar.md「ドキュメンテーションコメント」）")
		}
		return p.parseNamespace(start, doc)
	case token.Bind:
		if doc != nil {
			p.fail(doc.Span, "bind に説明は付けられない。付く先のないドキュメンテーションコメント")
		}
		if len(decorators) > 0 {
			p.fail(decorators[0].Span, "bind にデコレータは付けられない")
		}
		return p.parseBind(start)
	case token.Model:
		return p.parseModel(start, doc, decorators)
	case token.Scalar:
		return p.parseScalar(start, doc, decorators)
	case token.Union:
		return p.parseUnion(start, doc, decorators)
	case token.State:
		return p.parseState(start, doc, decorators)
	case token.Fn:
		return p.parseOp(start, doc, decorators)
	case token.Op:
		// なくした書き方: op f()、op fn f()（docs/grammar.md「関数の宣言」）
		p.failOldFnHead()
	case token.Exception:
		return p.parseException(start, doc, decorators)
	case token.Effect:
		return p.parseEffect(start, doc, decorators)
	case token.Read:
		return p.parseRead(start, doc, decorators)
	case token.Classify:
		return p.parseClassify(start, doc, decorators)
	case token.Decorator:
		return p.parseDecoratorDecl(start, doc, decorators, marks)
	}
	// `condition C of T;` はなくした書き方で、宣言の頭の識別子 condition は、今の書き方を示して誤りにする
	// （condition は予約語ではない。docs/grammar.md「classify」「決めたこと」）。
	if p.atContextual("condition") {
		p.fail(p.tok().Span, "condition の宣言は書けない。両側に名前を付けた classify で書く（classify C of T { <成り立つ側>; <成り立たない側>; }。docs/grammar.md「classify」）")
	}
	if doc != nil && len(decorators) == 0 {
		p.fail(doc.Span, "付く先のないドキュメンテーションコメント（docs/grammar.md「決めたこと」）")
	}
	p.failExpected("宣言")
	return nil
}

// expectFn は、関数の種類の語（read、effect）の後ろの fn を読む。なくした書き方（read r()、effect e(): 種類の語の直後に名前）は、
// 新しい書き方をメッセージで示して SYNTAX にする（docs/grammar.md「関数の宣言」）。
func (p *parser) expectFn(kind string) {
	if p.at(token.Fn) {
		p.advance()
		return
	}
	if p.atIdent() {
		name := p.tok().Text
		p.fail(p.tok().Span, fmt.Sprintf("%s %s(…) という書き方はなくなった。関数の宣言の頭は fn で、種類は、その前に書く（%s fn %s(…) と書く。docs/grammar.md「関数の宣言」）", kind, name, kind, name))
	}
	p.failExpected("fn（" + kind + " fn 名前(…) と書く）")
}

// failOldFnHead は、宣言の頭の op を、新しい書き方を示して誤りにする: op f() は fn f()、op fn f() も fn f()
// （種類を書かない fn が op。docs/grammar.md「関数の宣言」）。
func (p *parser) failOldFnHead() {
	op := p.tok().Span
	if p.peek(1).Kind == token.Fn {
		p.fail(op, "op fn とは書かない。種類を書かない fn が op（fn 名前(…) と書く。docs/grammar.md「関数の宣言」）")
	}
	name := "名前"
	if n := p.peek(1); n.Kind == token.Ident {
		name = n.Text
	}
	p.fail(op, fmt.Sprintf("op %s(…) という書き方はなくなった。関数の宣言の頭は fn（fn %s(…) と書く。docs/grammar.md「関数の宣言」）", name, name))
}

// parseLeadingDoc は先頭の DocComment を読む。二つ以上続いたら、最後のもの以外は付く先がないので誤り。
func (p *parser) parseLeadingDoc() *ast.DocComment {
	var doc *ast.DocComment
	for p.at(token.DocComment) {
		if doc != nil {
			p.errorAt(doc.Span, "付く先のないドキュメンテーションコメント（docs/grammar.md「決めたこと」）")
		}
		doc = p.parseDocComment(p.advance())
	}
	return doc
}

// coreMarks は核の印（docs/grammar.md「decorator の宣言」の CoreMark）の範囲。書かれていなければ nil。
type coreMarks struct {
	repeatable *source.Span
	entry      *source.Span
	hint       *source.Span
	// external は、なくした印 @external（SYNTAX にして、読み進める。その宣言の throws は、outer throws として読む）。
	external *source.Span
}

// coreMarkNames は、核の印の名前（"external" は、なくした印）。
var coreMarkNames = map[string]bool{"repeatable": true, "entry": true, "hint": true, "external": true}

// slot は、印の名前の置き場。
func (m *coreMarks) slot(mark string) **source.Span {
	switch mark {
	case "repeatable":
		return &m.repeatable
	case "entry":
		return &m.entry
	case "hint":
		return &m.hint
	}
	return &m.external
}

// each は、書かれた印を、名前と一緒に、決まった順で渡す。
func (m *coreMarks) each(f func(mark string, span source.Span)) {
	for _, mark := range []string{"repeatable", "entry", "hint", "external"} {
		if sp := *m.slot(mark); sp != nil {
			f(mark, *sp)
		}
	}
}

// parseDecorators はデコレータの適用の列を読む（docs/grammar.md「デコレータの適用」）。
// "@repeatable"、"@entry"、"@hint" は核の印なので列には入れず、その範囲を返す（なくした "@external" も、ここで拾う）。印の後ろは、もう一つの印か "decorator" で
// なければ誤り。同じ印を二度書いても誤り。
func (p *parser) parseDecorators() ([]*ast.Decorator, coreMarks) {
	var decorators []*ast.Decorator
	var marks coreMarks
	for p.at(token.At) {
		start := p.advance().Span.Start
		name := p.parseDecoratorRef()
		if mark := name.String(); coreMarkNames[mark] {
			if p.at(token.LParen) {
				p.fail(p.tok().Span, "@"+mark+" は引数を取らない")
			}
			s := p.spanFrom(start)
			slot := marks.slot(mark)
			if *slot != nil {
				p.fail(s, "@"+mark+" を二度書いている（docs/grammar.md「decorator の宣言」）")
			}
			if !p.at(token.Decorator) && !p.atCoreMark() {
				p.fail(s, "@"+mark+" はデコレータの宣言の直前にだけ書ける（docs/grammar.md「decorator の宣言」）")
			}
			*slot = &s
			continue
		}
		d := &ast.Decorator{Name: name}
		if p.at(token.LParen) {
			d.Args = p.parseArgs()
		}
		d.Span = p.spanFrom(start)
		decorators = append(decorators, d)
	}
	return decorators, marks
}

// atCoreMark は今の位置が核の印（"@repeatable"、"@entry"、"@hint" と、なくした "@external"）の頭か。
func (p *parser) atCoreMark() bool {
	if !p.at(token.At) {
		return false
	}
	next := p.peek(1)
	return next.Kind == token.Ident && coreMarkNames[next.Text]
}

// parseArgs は "( Arg, ... )" を読む。名前で渡す引数の後ろに位置で渡す引数は書けない（docs/grammar.md「決めたこと」）。
func (p *parser) parseArgs() []*ast.Arg {
	p.expect(token.LParen)
	args := []*ast.Arg{}
	named := false
	for !p.at(token.RParen) {
		a := p.parseArg()
		if a.Name != nil {
			named = true
		} else if named {
			p.errorAt(a.Span, "名前で渡す引数の後ろに位置で渡す引数は書けない（docs/grammar.md「決めたこと」）")
		}
		args = append(args, a)
		if !p.got(token.Comma) {
			break
		}
	}
	p.expect(token.RParen)
	return args
}

// parseArg は Arg = [ Ident ":" ] ArgValue を読む。
func (p *parser) parseArg() *ast.Arg {
	start := p.tok().Span.Start
	a := &ast.Arg{}
	// 引数名はデコレータの Param の名前で、フィールド名ではないので予約語は書けない（parseIdent が誤りにする）。
	if (p.atIdent() || p.tok().Kind.IsKeyword()) && p.peek(1).Kind == token.Colon {
		id := p.parseIdent()
		a.Name = &id
		p.advance()
	}
	a.Value = p.parseArgValue()
	a.Span = p.spanFrom(start)
	return a
}

// parseArgValue は ArgValue = ArgTerm { "|" ArgTerm } | "[" [ ArgValue { "," ArgValue } ] "]" を読む。
func (p *parser) parseArgValue() *ast.ArgValue {
	start := p.tok().Span.Start
	v := &ast.ArgValue{}
	if p.got(token.LBracket) {
		v.IsList = true
		v.List = []*ast.ArgValue{}
		for !p.at(token.RBracket) {
			v.List = append(v.List, p.parseArgValue())
			if !p.got(token.Comma) {
				break
			}
		}
		p.expect(token.RBracket)
	} else {
		v.Terms = append(v.Terms, p.parseArgTerm())
		for p.got(token.Pipe) {
			v.Terms = append(v.Terms, p.parseArgTerm())
		}
	}
	v.Span = p.spanFrom(start)
	return v
}

// parseArgTerm は ArgTerm = Literal | QualifiedName を読む。
func (p *parser) parseArgTerm() *ast.ArgTerm {
	t := &ast.ArgTerm{}
	switch {
	case p.tok().Kind.IsLiteral():
		t.Literal = p.parseLiteral()
		t.Span = t.Literal.Span
	case p.atName():
		qn := p.parseQualifiedName()
		t.Name = &qn
		t.Span = qn.Span
	default:
		p.failExpected("引数の値（リテラルか名前）")
	}
	return t
}

// parseNamespace は NamespaceDecl = "namespace" QualifiedName "{" { Decl } "}" を読む。
func (p *parser) parseNamespace(start source.Offset, doc *ast.DocComment) *ast.NamespaceDecl {
	p.expect(token.Namespace)
	ns := &ast.NamespaceDecl{Doc: doc, Name: p.parseQualifiedName()}
	p.expect(token.LBrace)
	ns.Decls = p.parseDecls(token.RBrace)
	if !p.at(token.RBrace) {
		p.errorAt(p.tok().Span, fmt.Sprintf("namespace %s の「}」がない", ns.Name.String()))
	} else {
		p.advance()
	}
	ns.Span = p.spanFrom(start)
	return ns
}

// parseBind は BindDecl = "bind" Ident "to" QualifiedName "." ( "*" | "**" ) ";" を読む（docs/grammar.md「bind」）。
func (p *parser) parseBind(start source.Offset) *ast.BindDecl {
	p.expect(token.Bind)
	b := &ast.BindDecl{Name: p.parseIdent()}
	p.expect(token.To)
	b.Target = p.parseQualifiedName()
	p.expect(token.Dot)
	switch p.tok().Kind {
	case token.Star:
		p.advance()
	case token.StarStar:
		p.advance()
		b.Deep = true
	default:
		p.failExpected("「*」か「**」")
	}
	p.expect(token.Semicolon)
	b.Span = p.spanFrom(start)
	return b
}

// parseModel は ModelDecl を読む（docs/grammar.md「model」）。
//
//	ModelDecl = "model" Ident [ "extends" QualifiedName ] ModelBody ;
//
// `model A is B omit f;` はなくした書き方で、名前の直後の識別子 is は、今の書き方を示して誤りにする。
func (p *parser) parseModel(start source.Offset, doc *ast.DocComment, decorators []*ast.Decorator) *ast.ModelDecl {
	p.expect(token.Model)
	m := &ast.ModelDecl{Doc: doc, Decorators: decorators, Name: p.parseIdent()}
	if p.atContextual("is") {
		p.fail(p.tok().Span, fmt.Sprintf("model %s is ... omit ... は書けない。model %s extends 元の model { override フィールド: never; } と書く（docs/grammar.md「model」）", m.Name.Name, m.Name.Name))
	}
	if p.got(token.Extends) {
		base := p.parseQualifiedName()
		m.Extends = &base
	}
	if !p.at(token.LBrace) {
		if m.Extends != nil {
			p.failExpected("「{」")
		}
		p.failExpected("「{」か「extends」")
	}
	m.Body = p.parseModelBody(indexDeclared)
	if idx := m.Body.Index; idx != nil && m.Extends != nil {
		p.errorAt(idx.Span, "extends を持つ model に、インデックスは書けない（インデックスの model は、継承できない。docs/grammar.md「model」）")
	}
	m.Span = p.spanFrom(start)
	return m
}

// indexPlace は、ModelBody の置き場が、インデックスを書ける所か。
type indexPlace int

const (
	// indexDeclared は、名前を付けて宣言する model の本体（インデックスを書ける）。
	indexDeclared indexPlace = iota
	// indexInline は、その場で書く model（ArmType）の本体（インデックスを書けない）。
	indexInline
)

// parseModelBody は ModelBody = "{" ( { Member } | Index ) "}" を読む。誤ったメンバーは読み飛ばし、読めたものを残す。
//
// 本体に書けるのは、Member の並びか、Index 一つの、どちらか（docs/grammar.md「model」）: Index と Member を混ぜる、
// Index を二つ書く、その場で書く model に Index を書く、は誤り。誤りのあった Index も、最初の一つは残す。
func (p *parser) parseModelBody(place indexPlace) *ast.ModelBody {
	start := p.expect(token.LBrace).Span.Start
	body := &ast.ModelBody{Fields: []*ast.Field{}}
	mixed := false
	p.parseMembers(func() {
		f, o, idx := p.parseMember()
		if f != nil {
			body.Fields = append(body.Fields, f)
		}
		if o != nil {
			body.Overrides = append(body.Overrides, o)
		}
		switch {
		case idx == nil:
		case place == indexInline:
			p.errorAt(idx.Span, "その場で書く model に、インデックスは書けない。インデックスの model は、名前を付けて宣言する（docs/grammar.md「model」）")
		case body.Index != nil:
			p.errorAt(idx.Span, "インデックスは、一つの model に一つだけ（docs/grammar.md「model」）")
		default:
			body.Index = idx
		}
		if !mixed && body.Index != nil && len(body.Fields)+len(body.Overrides) > 0 {
			mixed = true
			p.errorAt(body.Index.Span, "インデックスと、フィールドは、一つの model に混ぜられない。本体は、フィールドの並びか、インデックス一つ（docs/grammar.md「model」）")
		}
	})
	p.expect(token.RBrace)
	body.Span = p.spanFrom(start)
	return body
}

// parseMembers は "}" まで parseOne を繰り返す。誤ったメンバーは次の ";" か "}" まで読み飛ばす。
func (p *parser) parseMembers(parseOne func()) {
	for !p.at(token.RBrace) && !p.at(token.EOF) {
		before := p.pos
		p.recover(parseOne)
		if p.pos == before {
			p.advance()
		}
	}
}

// parseMember は Member = Field | Override か、Index を読む。どれか一つを返す。
//
//	Field    = [ DocComment ] { Decorator } FieldName ":" Type ";" ;
//	Override = [ DocComment ] { Decorator } "override" FieldName [ ":" ( Type | "never" ) ] ";" ;
//	Index    = "[" Ident ":" Type "]" ":" Type ";" ;
//
// Index には、デコレータも DocComment も付けられない（model の宣言に付ける）。
//
// フィールドの名前は FieldName なので予約語も書ける（`op: string;`。docs/grammar.md「識別子」）。
// メンバーの頭の override は、直後が ":" ならフィールドの名前、そうでなければ Override の頭。
// フィールドの既定値（`page: int = 1`）はなくした書き方で、型の直後の "=" は、今の書き方を示して誤りにする
// （docs/grammar.md「model」「決めたこと」）。
func (p *parser) parseMember() (*ast.Field, *ast.Override, *ast.Index) {
	doc := p.parseLeadingDoc()
	start := p.tok().Span.Start
	decorators, marks := p.parseDecorators()
	decorators = docDecorators(doc, decorators)
	// parseDecorators は直後の字句が decorator なら核の印を通すが、ここでの decorator はフィールド名。
	marks.each(func(mark string, span source.Span) {
		p.fail(span, "@"+mark+" はデコレータの宣言の直前にだけ書ける（docs/grammar.md「decorator の宣言」）")
	})
	if doc != nil && p.at(token.RBrace) {
		p.fail(doc.Span, "付く先のないドキュメンテーションコメント（docs/grammar.md「決めたこと」）")
	}
	if p.at(token.LBracket) {
		if doc != nil || len(decorators) > 0 {
			p.errorAt(p.tok().Span, "インデックスには、デコレータも、説明も、付けられない。model の宣言に付ける（docs/grammar.md「model」）")
		}
		p.advance()
		idx := &ast.Index{Name: p.parseIdent()}
		p.expect(token.Colon)
		idx.Key = p.parseType()
		p.expect(token.RBracket)
		p.expect(token.Colon)
		idx.Value = p.parseType()
		p.expect(token.Semicolon)
		idx.Span = p.spanFrom(start)
		return nil, nil, idx
	}
	if p.at(token.Override) && p.peek(1).Kind != token.Colon {
		p.advance()
		o := &ast.Override{Doc: doc, Decorators: decorators, Name: p.parseFieldName()}
		if p.got(token.Colon) {
			if p.atTypeWord(token.NeverName) {
				o.Never, o.NeverSpan = true, p.advance().Span
			} else {
				o.Type = p.parseType()
			}
		}
		if p.at(token.Assign) {
			p.failFieldDefault(o.Name)
		}
		p.expect(token.Semicolon)
		o.Span = p.spanFrom(start)
		return nil, o, nil
	}
	f := &ast.Field{Doc: doc, Decorators: decorators, Name: p.parseFieldName()}
	p.expect(token.Colon)
	f.Type = p.parseType()
	if p.at(token.Assign) {
		p.failFieldDefault(f.Name)
	}
	p.expect(token.Semicolon)
	f.Span = p.spanFrom(start)
	return f, nil, nil
}

// failFieldDefault は、フィールドの型の直後の "="（なくした書き方の既定値）を、今の書き方を示して誤りにする。
func (p *parser) failFieldDefault(name ast.Ident) {
	p.fail(p.tok().Span, fmt.Sprintf("フィールド %s に既定値は書けない。値がないときに何を入れるかは、境界のデコレータで言う（@http.default(値)、@cli.default(値)。docs/grammar.md「model」）", name.Name))
}

// parseScalar は ScalarDecl = "scalar" Ident "extends" Type ";" を読む（docs/grammar.md「scalar」）。
func (p *parser) parseScalar(start source.Offset, doc *ast.DocComment, decorators []*ast.Decorator) *ast.ScalarDecl {
	p.expect(token.Scalar)
	s := &ast.ScalarDecl{Doc: doc, Decorators: decorators, Name: p.parseIdent()}
	p.expect(token.Extends)
	s.Base = p.parseType()
	p.expect(token.Semicolon)
	s.Span = p.spanFrom(start)
	return s
}

// parseUnion は UnionDecl = "union" Ident "{" { UnionMember } "}" を読む（docs/grammar.md「union」）。
func (p *parser) parseUnion(start source.Offset, doc *ast.DocComment, decorators []*ast.Decorator) *ast.UnionDecl {
	p.expect(token.Union)
	u := &ast.UnionDecl{Doc: doc, Decorators: decorators, Name: p.parseIdent(), Members: []*ast.UnionMember{}}
	p.expect(token.LBrace)
	p.parseMembers(func() {
		u.Members = append(u.Members, p.parseUnionMember())
	})
	p.expect(token.RBrace)
	u.Span = p.spanFrom(start)
	return u
}

// parseUnionMember は UnionMember = [ DocComment ] ( Ident ":" ArmType ";" | QualifiedName ";" ) を読む。
func (p *parser) parseUnionMember() *ast.UnionMember {
	m := &ast.UnionMember{Doc: p.parseLeadingDoc()}
	if tag := firstDocApply(m.Doc); tag != nil {
		p.errorAt(tag.Span, "union のアームにデコレータは付けられない（説明のコメントの中の、デコレータのタグも、同じ。docs/grammar.md「ドキュメンテーションコメント」）")
	}
	start := p.tok().Span.Start
	if m.Doc != nil && p.at(token.RBrace) {
		p.fail(m.Doc.Span, "付く先のないドキュメンテーションコメント（docs/grammar.md「決めたこと」）")
	}
	if p.atIdent() && p.peek(1).Kind == token.Colon {
		arm := p.parseIdent()
		m.Arm = &arm
		p.advance()
		m.Type = p.parseArmType()
	} else {
		if !p.atName() {
			p.failExpected("ユニオンのメンバー（アームの名前か宣言の名前）")
		}
		target := p.parseQualifiedName()
		m.Target = &target
	}
	p.expect(token.Semicolon)
	m.Span = p.spanFrom(start)
	return m
}

// parseArmType は ArmType = Type | ModelBody を読む。
func (p *parser) parseArmType() *ast.ArmType {
	a := &ast.ArmType{}
	if p.at(token.LBrace) {
		a.Body = p.parseModelBody(indexInline)
		a.Span = a.Body.Span
	} else {
		a.Type = p.parseType()
		a.Span = a.Type.NodeSpan()
	}
	return a
}

// parseState は StateDecl = "state" Ident ":" Type ";" を読む（docs/grammar.md「state」）。
func (p *parser) parseState(start source.Offset, doc *ast.DocComment, decorators []*ast.Decorator) *ast.StateDecl {
	p.expect(token.State)
	s := &ast.StateDecl{Doc: doc, Decorators: decorators, Name: p.parseIdent()}
	p.expect(token.Colon)
	s.Type = p.parseType()
	p.expect(token.Semicolon)
	s.Span = p.spanFrom(start)
	return s
}

// parseException は ExceptionDecl = "exception" Ident [ "(" OpParam { "," OpParam } ")" ] ";" を読む
// （docs/grammar.md「exception / effect / read」）。引数は例外が運ぶ値。引数がなければ括弧を書かない（`exception X();` は誤り）。
// なくした書き方 `exception X: T;` は、引数で書くことをメッセージで示して SYNTAX にする。
func (p *parser) parseException(start source.Offset, doc *ast.DocComment, decorators []*ast.Decorator) *ast.ExceptionDecl {
	p.expect(token.Exception)
	e := &ast.ExceptionDecl{Doc: doc, Decorators: decorators, Name: p.parseIdent()}
	if p.at(token.Colon) {
		p.fail(p.tok().Span, fmt.Sprintf("exception %s: T という書き方はなくなった。例外が運ぶ値は引数で書く（exception %s(value: T);。docs/grammar.md「exception / effect / read」）", e.Name.Name, e.Name.Name))
	}
	if p.at(token.LParen) {
		open := p.tok().Span
		e.Params = p.parseOpParams()
		if len(e.Params) == 0 {
			p.errorAt(source.Span{File: open.File, Start: open.Start, End: p.prevEnd()}, fmt.Sprintf("引数のない例外は括弧を書かない（exception %s; と書く。docs/grammar.md「exception / effect / read」）", e.Name.Name))
		}
	}
	p.expect(token.Semicolon)
	e.Span = p.spanFrom(start)
	return e
}

// parseFnType は、関数の「)」の後ろの「:」と、戻り値を読む（docs/grammar.md「関数の宣言」の FnType の頭）。
//
//	":" ( ReturnType { 節 } | 節 { 節 } )
//
// 「)」の後ろに、戻り値か節を書くなら、「:」で始める。戻り値を省けば、nil（void と同じ）。節そのものは、呼ぶ側が読む。
// なくした書き方（「:」のない節: fn f() throws E）と、「:」の後ろに何もない形（fn f():;）は、SYNTAX。
// head は、メッセージに出す宣言の頭（"fn f"、"read fn r"、"effect fn e"）。
func (p *parser) parseFnType(head string) *ast.ReturnType {
	if !p.at(token.Colon) {
		switch p.tok().Kind {
		case token.Throws, token.Reads, token.Effects:
			clause := p.tok().Text
			p.fail(p.tok().Span, fmt.Sprintf("「)」の後ろに、戻り値か節を書くなら、「:」で始める（%s(…): %s … と書く。docs/grammar.md「関数の宣言」）", head, clause))
		}
		return nil
	}
	colon := p.advance().Span
	switch p.tok().Kind {
	case token.Throws, token.Reads, token.Effects, token.Catches, token.On:
		return nil
	case token.Semicolon, token.LBrace:
		p.fail(colon, fmt.Sprintf("「:」の後ろに、戻り値か節が要る。何も書かないなら、「:」も書かない（%s(…); か %s(…) { … } と書く。docs/grammar.md「関数の宣言」）", head, head))
	}
	p.noOnType = true
	defer func() { p.noOnType = false }()
	return p.parseReturnType()
}

// failStateTouchingClause は、read fn と effect fn に書けない節を、誤りにする: read fn の throws、catches、on
// （docs/grammar.md「関数の宣言」の表。docs/check.md「状態に触れるもの」）。effect fn の throws は、書ける。
func (p *parser) failStateTouchingClause(kind, name, want string) {
	switch p.tok().Kind {
	case token.Throws, token.Catches:
		if kind == "effect" && p.at(token.Throws) {
			return // 効果は、effects の前に、throws を一つ書ける（parseEffectThrows）
		}
		if kind == "effect" {
			p.fail(p.tok().Span, fmt.Sprintf("effect fn %s に %s の節は書けない（効果に書ける節は、throws と effects。docs/grammar.md「関数の宣言」）", name, p.tok().Text))
		}
		p.fail(p.tok().Span, fmt.Sprintf("read fn %s に %s の節は書けない（read は、例外を投げない。docs/check.md「状態に触れるもの」）", name, p.tok().Text))
	case token.On:
		p.fail(p.tok().Span, fmt.Sprintf("%s fn %s(…) on … という書き方はなくなった。状態との関係は、%s で書く（%s fn %s(…): … %s … と書く。docs/grammar.md「exception / effect / read」）", kind, name, want, kind, name, want))
	}
}

// parseEffect は EffectDecl = "effect" FnHead ":" [ ReturnType ] [ EffectThrows ] "effects" ( BuiltinEffect [ RowClause ] | Transition ) ";" を読む
// （docs/grammar.md「関数の宣言」「exception / effect / read」）。引数がなくても括弧を書く。戻り値は、書いたものを、そのまま持つ
// （void のほかは TYPE-18。意味の解析が出す）。
// 宛先の QualifiedName の最後の要素が StateOp なら BuiltinEffect、そうでなければ Transition（状態か、状態の分類の、遷移）。
// 行の遷移（RowClause）は BuiltinEffect の後ろにだけ書ける。
// なくした書き方（括弧のない effect fn e: …、on、宛先の後ろの「:」）は、新しい書き方をメッセージで示して SYNTAX にする。
func (p *parser) parseEffect(start source.Offset, doc *ast.DocComment, decorators []*ast.Decorator) *ast.EffectDecl {
	p.expect(token.Effect)
	p.expectFn("effect")
	e := &ast.EffectDecl{Doc: doc, Decorators: decorators, Name: p.parseIdent()}
	if !p.at(token.LParen) {
		p.fail(p.tok().Span, fmt.Sprintf("effect fn %s の名前の後ろに、引数の括弧が要る（引数がなくても effect fn %s(): effects … と書く。docs/grammar.md「exception / effect / read」）", e.Name.Name, e.Name.Name))
	}
	e.Params = p.parseOpParams()
	p.failStateTouchingClause("effect", e.Name.Name, "effects")
	e.Return = p.parseFnType("effect fn " + e.Name.Name)
	p.failStateTouchingClause("effect", e.Name.Name, "effects")
	if p.at(token.Throws) {
		e.Throws = p.parseEffectThrows(e.Name.Name)
	}
	if !p.at(token.Effects) {
		p.failExpected(fmt.Sprintf("「: effects 宛先」（effect fn %s(…): effects 状態.操作; と書く）", e.Name.Name))
	}
	p.advance()
	if p.atName() && p.peek(1).Kind == token.Colon {
		p.fail(p.peek(1).Span, "状態の遷移を、宛先の後ろの「:」で書く形はなくなった。as で書く（effects C as A -> B と書く。docs/grammar.md「exception / effect / read」）")
	}
	if p.atTransitionTarget() {
		e.Transition = p.parseTransition()
	} else {
		ref := p.parseStateOpRef()
		e.On = &ref
		if p.at(token.As) {
			e.Row = p.parseRowClause()
		}
	}
	if p.at(token.Comma) {
		p.fail(p.tok().Span, fmt.Sprintf("effect fn %s の宛先は、一つだけ（docs/grammar.md「exception / effect / read」）", e.Name.Name))
	}
	if p.at(token.Throws) {
		p.fail(p.tok().Span, fmt.Sprintf("effect fn %s の throws は、effects の前に書く（effect fn %s(…): throws E effects 宛先; と書く。docs/grammar.md「関数の宣言」）", e.Name.Name, e.Name.Name))
	}
	p.expect(token.Semicolon)
	e.Span = p.spanFrom(start)
	return e
}

// parseEffectThrows は EffectThrows = "throws" ExcRef { "," ExcRef } を読む（docs/grammar.md「exception / effect / read」）。
// 効果が投げる例外。関係の節（prevents effects、keeps effects）は書けない: 投げたとき、その効果は、いつも、起きていない
// （docs/check.md「read と効果」）。節は、一つだけ。
func (p *parser) parseEffectThrows(name string) *ast.ThrowsClause {
	start := p.expect(token.Throws).Span.Start
	c := &ast.ThrowsClause{Exceptions: p.parseNameList("throws")}
	c.Span = p.spanFrom(start)
	switch p.tok().Kind {
	case token.Prevents, token.Keeps:
		p.fail(p.tok().Span, fmt.Sprintf("effect fn %s の throws に、%s の節は書けない（効果が例外を投げたとき、その効果は、いつも、起きていない。docs/check.md「状態に触れるもの」）", name, p.tok().Text))
	case token.Throws:
		p.fail(p.tok().Span, fmt.Sprintf("effect fn %s の throws は、一つだけ（例外は「,」で並べる: throws A, B。docs/grammar.md「関数の宣言」）", name))
	}
	return c
}

// atTransitionTarget は、効果の宛先が、状態の遷移（Transition）の頭か: 名前の最後の要素が StateOp でなく、直後が as。
// 最後の要素が post / put / delete の名前は、いつも「状態.操作」として読む（docs/grammar.md「exception / effect / read」）。
func (p *parser) atTransitionTarget() bool {
	if !p.atName() || p.peek(1).Kind != token.As {
		return false
	}
	if p.at(token.QualifiedName) {
		text := p.tok().Text
		switch text[strings.LastIndexByte(text, '.')+1:] {
		case "post", "put", "delete":
			return false
		}
	}
	return true
}

// parseRowClause は行の遷移を読む（docs/grammar.md「exception / effect / read」）。
//
//	RowClause = "as" ( RowCase | "else" ) ;
//	RowCase   = QualifiedName ;      (* 分類の名前 "." ケースの名前 *)
//
// 書くのは、効果の後のケースだけ。効果の前のケースは、行の引数の型（parseType の OnType）が言う。矢印の形（as <前> -> <後>）と、
// any は、なくした書き方で、新しい書き方をメッセージで示す。
func (p *parser) parseRowClause() *ast.RowClause {
	const removed = "行の遷移の、前のケースは、行の引数の型で言い（order: Order on C.A | C.B）、as には、後のケースだけを書く（effects s.put as C.D。docs/grammar.md「exception / effect / read」）"
	start := p.tok().Span.Start
	r := &ast.RowClause{AsSpan: p.expect(token.As).Span}
	afterStart := p.tok().Span.Start
	switch {
	case p.at(token.Any):
		p.fail(p.tok().Span, "行の遷移に any は書けない。前がどのケースでもよければ、行の引数を絞り込まない。後のケースを言わないなら、as を書かない（put で、後のケースを約束しないなら as else）。"+removed)
	case p.got(token.Else):
		r.After.Else = true
	default:
		c := p.parseRowCase()
		r.After.Case = &c
	}
	r.After.Span = p.spanFrom(afterStart)
	switch p.tok().Kind {
	case token.ThinArrow:
		p.fail(p.tok().Span, "行の遷移の矢印（as <前> -> <後>）はなくなった。"+removed)
	case token.Comma, token.Pipe:
		p.fail(p.tok().Span, "行の遷移の as に書けるケースは、一つだけ（効果の後のケース）。"+removed)
	}
	r.Span = p.spanFrom(start)
	return r
}

// parseRowCase は RowCase（分類の名前とケースの名前を "." でつないだ QualifiedName）を読む。要素が一つだけの名前は誤り。
func (p *parser) parseRowCase() ast.QualifiedName {
	if p.atIdent() {
		p.fail(p.tok().Span, fmt.Sprintf("行の遷移のケースは、分類の名前から書く（<分類>.%s。docs/grammar.md「exception / effect / read」）", p.tok().Text))
	}
	if !p.at(token.QualifiedName) {
		p.failExpected("行の遷移のケース（<分類>.<ケース> か、else）")
	}
	return p.parseQualifiedName()
}

// parseRead は ReadDecl = "read" FnHead ":" [ ReturnType ] "reads" QualifiedName ";" を読む
// （docs/grammar.md「関数の宣言」「exception / effect / read」）。引数がなくても括弧を書く。戻り値は、書いたものを、そのまま持つ
// （省いた形、void、never は TYPE-18。意味の解析が出す）。reads の後ろは状態の名前一つで、操作は書かない。
func (p *parser) parseRead(start source.Offset, doc *ast.DocComment, decorators []*ast.Decorator) *ast.ReadDecl {
	p.expect(token.Read)
	p.expectFn("read")
	r := &ast.ReadDecl{Doc: doc, Decorators: decorators, Name: p.parseIdent()}
	if !p.at(token.LParen) {
		p.fail(p.tok().Span, fmt.Sprintf("read fn %s の名前の後ろに、引数の括弧が要る（read fn %s(…): T reads 状態; と書く。docs/grammar.md「exception / effect / read」）", r.Name.Name, r.Name.Name))
	}
	r.Params = p.parseOpParams()
	p.failStateTouchingClause("read", r.Name.Name, "reads")
	if ret := p.parseFnType("read fn " + r.Name.Name); ret != nil {
		r.Return = ret
		r.Result = ret.Type
	}
	p.failStateTouchingClause("read", r.Name.Name, "reads")
	if !p.at(token.Reads) {
		p.failExpected(fmt.Sprintf("「reads 状態」（read fn %s(…): T reads 状態; と書く）", r.Name.Name))
	}
	p.advance()
	if !p.atName() {
		p.failExpected("読む状態の名前")
	}
	r.On = p.parseQualifiedName()
	if p.at(token.Comma) {
		p.fail(p.tok().Span, fmt.Sprintf("read fn %s が読む状態は、一つだけ（docs/grammar.md「exception / effect / read」）", r.Name.Name))
	}
	p.expect(token.Semicolon)
	r.Span = p.spanFrom(start)
	return r
}

// parseTransition は Transition = QualifiedName "as" CaseSet "->" CaseSet を読む。
func (p *parser) parseTransition() *ast.Transition {
	start := p.tok().Span.Start
	t := &ast.Transition{Of: p.parseQualifiedName()}
	p.expect(token.As)
	t.From = p.parseCaseSet()
	p.expect(token.ThinArrow)
	t.To = p.parseCaseSet()
	t.Span = p.spanFrom(start)
	return t
}

// parseCaseSet は CaseSet = "any" | Ident { "," Ident } を読む。
func (p *parser) parseCaseSet() *ast.CaseSet {
	start := p.tok().Span.Start
	c := &ast.CaseSet{}
	if p.got(token.Any) {
		c.Any = true
	} else {
		if !p.atIdent() {
			p.failExpected("遷移のケース（any かケース名）")
		}
		c.Cases = p.parseCaseList("遷移のケース", "effects C as A, B -> any", "exception / effect / read")
	}
	c.Span = p.spanFrom(start)
	return c
}

// parseCaseList はケースの並び Ident { "," Ident } を読む（アームの頭と、遷移の CaseSet）。what は並びの呼び名、
// example は今の書き方の例、section は docs/grammar.md の見出し。ケースの後ろに "|" が来たら（なくした書き方）、
// 区切りが "," であることをメッセージで示して SYNTAX にし、"," として読み進める。
func (p *parser) parseCaseList(what, example, section string) []ast.Ident {
	cases := []ast.Ident{p.parseIdent()}
	for {
		if p.at(token.Pipe) {
			p.errorAt(p.tok().Span, fmt.Sprintf("%sの並びの区切りは「,」で、「|」ではない（%s と書く。docs/grammar.md「%s」）", what, example, section))
			p.advance()
		} else if !p.got(token.Comma) {
			return cases
		}
		cases = append(cases, p.parseIdent())
	}
}

// parseStateOpRef は "状態.操作" を読む。字句では一つの QualifiedName なので、最後の要素を StateOp として切り出す
// （docs/grammar.md「exception / effect / read」）。
func (p *parser) parseStateOpRef() ast.StateOpRef {
	if !p.at(token.QualifiedName) {
		p.failExpected("「状態.操作」（操作は post / put / delete）")
	}
	qn := p.parseQualifiedName()
	last := qn.Last()
	ref := ast.StateOpRef{Span: qn.Span, OpSpan: last.Span}
	switch last.Name {
	case "post":
		ref.Op = ast.Post
	case "put":
		ref.Op = ast.Put
	case "delete":
		ref.Op = ast.Delete
	default:
		p.fail(last.Span, fmt.Sprintf("状態の操作は post / put / delete のどれかで、%s ではない", last.Name))
	}
	ref.State = ast.QualifiedName{
		Span:     source.Span{File: p.file, Start: qn.Span.Start, End: qn.Segments[len(qn.Segments)-2].Span.End},
		Segments: qn.Segments[:len(qn.Segments)-1],
	}
	return ref
}

// parseClassify は ClassifyDecl = "classify" Ident "of" ClassifySubject "{" { ClassifyCase } "}" を読む（docs/grammar.md「classify」）。
func (p *parser) parseClassify(start source.Offset, doc *ast.DocComment, decorators []*ast.Decorator) *ast.ClassifyDecl {
	p.expect(token.Classify)
	c := &ast.ClassifyDecl{Doc: doc, Decorators: decorators, Name: p.parseIdent(), Cases: []*ast.ClassifyCase{}}
	p.expect(token.Of)
	if p.atTypeWord(token.NeverName) {
		c.Never, c.NeverSpan = true, p.advance().Span
		if p.at(token.Comma) {
			p.fail(p.tok().Span, "never は、ほかの対象と並べられない（対象のない分類は of never だけ。docs/grammar.md「classify」）")
		}
	} else if p.at(token.Any) && p.peek(1).Kind == token.Exception {
		start := p.advance().Span.Start
		p.advance()
		c.Exception, c.ExceptionSpan = true, p.spanFrom(start)
	} else {
		c.Of = p.parseClassifySubject()
	}
	p.expect(token.LBrace)
	p.parseMembers(func() {
		c.Cases = append(c.Cases, p.parseClassifyCase())
	})
	p.expect(token.RBrace)
	c.Span = p.spanFrom(start)
	return c
}

// parseClassifyCase は ClassifyCase を読む。
//
//	ClassifyCase = [ DocComment ] Ident [ ":" CaseTyping { "," CaseTyping } ] ";" ;
//	CaseTyping   = Type "as" ( Type | "else" | "none" ) ;
//
// 名前のないケース `default;` はなくした書き方で、ケースの位置の default は、今の書き方を示して誤りにする。
func (p *parser) parseClassifyCase() *ast.ClassifyCase {
	c := &ast.ClassifyCase{Doc: p.parseLeadingDoc()}
	start := p.tok().Span.Start
	if c.Doc != nil && p.at(token.RBrace) {
		p.fail(c.Doc.Span, "付く先のないドキュメンテーションコメント（docs/grammar.md「決めたこと」）")
	}
	c.Decorators, _ = p.parseDecorators()
	c.Decorators = docDecorators(c.Doc, c.Decorators)
	if p.at(token.DocComment) {
		p.fail(p.tok().Span, "ドキュメンテーションコメントは、デコレータより上に書く（docs/grammar.md「ファイルと宣言」）")
	}
	if p.at(token.Default) {
		p.fail(p.tok().Span, "名前のないケース default は書けない。ケースには名前を付ける（値がない側や、union の残りのアームなら <名前>: <対象> as else;。仕様が使わない場合は、宣言しない。docs/grammar.md「classify」）")
	}
	c.Name = p.parseIdent()
	if p.got(token.Colon) {
		c.As = append(c.As, p.parseCaseTyping())
		for p.got(token.Comma) {
			c.As = append(c.As, p.parseCaseTyping())
		}
	}
	p.expect(token.Semicolon)
	c.Span = p.spanFrom(start)
	return c
}

// parseCaseTyping は CaseTyping = Type "as" ( Type | "else" | "none" ) を読む。as の前は対象（宣言の of の後ろと同じに書く）。
// as の後ろの else は、型の代わりに「残り」を言い（docs/check.md「残りのケース」）、none は「値がない」を言う（同「ない側のケース」）。
func (p *parser) parseCaseTyping() *ast.CaseTyping {
	start := p.tok().Span.Start
	if p.at(token.Any) && p.peek(1).Kind == token.Exception {
		// 例外の分類のケース: "any" "exception" "as" ( ExcRef { "|" ExcRef } | "else" )
		p.advance()
		p.advance()
		t := &ast.CaseTyping{AnyException: true}
		p.expect(token.As)
		switch {
		case p.at(token.Else):
			t.Else, t.ElseSpan = true, p.advance().Span
		case p.at(token.None):
			p.fail(p.tok().Span, "例外の分類のケースに、as none は書けない（受けた例外に、「値がない」は、ない。docs/grammar.md「classify」）")
		default:
			t.Exceptions = append(t.Exceptions, p.parseQualifiedName())
			for p.got(token.Pipe) {
				t.Exceptions = append(t.Exceptions, p.parseQualifiedName())
			}
		}
		t.Span = p.spanFrom(start)
		return t
	}
	t := &ast.CaseTyping{Subject: p.parseType()}
	p.expect(token.As)
	switch {
	case p.at(token.Else):
		t.Else, t.ElseSpan = true, p.advance().Span
	case p.at(token.None):
		t.None, t.NoneSpan = true, p.advance().Span
	default:
		t.Type = p.parseType()
	}
	t.Span = p.spanFrom(start)
	return t
}

// parseClassifySubject は ClassifySubject = Type { "," Type } を読む。型か op か状態の名前かは名前の解決が決める。
func (p *parser) parseClassifySubject() []ast.Type {
	var subject []ast.Type
	for {
		if p.at(token.LBrace) || p.at(token.Semicolon) {
			p.failExpected("分類の対象（型か op か状態の名前）")
		}
		subject = append(subject, p.parseType())
		if !p.got(token.Comma) {
			return subject
		}
	}
}

// parseDecoratorDecl は DecoratorDecl を読む（docs/grammar.md「decorator の宣言」）。
//
//	DecoratorDecl = { CoreMark } "decorator" "fn" DecoratorName Params [ ":" DecoratorClause { DecoratorClause } ] ( ";" | Block )
//	              | { CoreMark } "decorator" DecoratorName [ Params ] "for" Target { "," Target } ";"
//	              | "decorator" DecoratorName "=" DecoratorRef Args ";" ;
//
//	DecoratorClause = ThrowsClause | CatchesClause | DecoReadsClause | DecoEffectsClause ;
//	ThrowsClause    = [ "outer" ] "throws" ExcRef { "," ExcRef } { Relation } ;
//
// デコレータの名前は DecoratorName なので予約語も書ける（`decorator default(…)`。docs/grammar.md「識別子」）。
// reads / effects の節は op の節と同じ形で、同じ構文木（ReadsClause / EffectsClause）にする。
func (p *parser) parseDecoratorDecl(start source.Offset, doc *ast.DocComment, decorators []*ast.Decorator, marks coreMarks) *ast.DecoratorDecl {
	p.expect(token.Decorator)
	d := &ast.DecoratorDecl{Doc: doc, Decorators: decorators}
	if p.at(token.Fn) {
		d.Fn, d.FnSpan = true, p.advance().Span
	}
	d.Name = p.parseDecoratorName()
	if marks.entry != nil {
		d.Entry, d.EntrySpan = true, *marks.entry
	}
	if marks.hint != nil {
		d.Hint, d.HintSpan = true, *marks.hint
	}
	if marks.external != nil {
		// なくした印: 新しい書き方（節 outer throws）を示して、読み進める。この宣言の throws は、outer throws として読む（下の outerOld）
		p.errorAt(*marks.external, "@external はなくなった。fn の外で起きる例外は、節 outer throws で書く（decorator fn "+d.Name.Name+"(…): outer throws …。境界の入口なら、印 @entry を付ける。docs/grammar.md「decorator の宣言」）")
	}
	outerOld := func() {
		if marks.external == nil {
			return
		}
		for _, c := range d.Clauses {
			if t, ok := c.(*ast.ThrowsClause); ok {
				t.Outer = true
			}
		}
	}
	if marks.repeatable != nil {
		d.Repeatable = true
		d.RepeatableSpan = *marks.repeatable
	}
	if p.at(token.Assign) {
		if d.Fn {
			p.errorAt(d.FnSpan, "名前付きのデコレータに fn は書かない（decorator 名前 = 元(…); と書く。docs/grammar.md「decorator の宣言」）")
			d.Fn, d.FnSpan = false, source.Span{}
		}
		for _, mark := range []string{"repeatable", "entry", "hint"} {
			if sp := *marks.slot(mark); sp != nil {
				p.errorAt(*sp, "名前付きのデコレータに @"+mark+" は付けられない（docs/grammar.md「decorator の宣言」）")
			}
		}
		d.Entry, d.EntrySpan, d.Hint, d.HintSpan = false, source.Span{}, false, source.Span{}
		aliasStart := p.advance().Span.Start
		alias := &ast.DecoratorAlias{Name: p.parseDecoratorRef()}
		alias.Args = p.parseArgs()
		alias.Span = p.spanFrom(aliasStart)
		d.Alias = alias
		p.expect(token.Semicolon)
		d.Span = p.spanFrom(start)
		return d
	}
	name := d.Name.Name
	if d.Fn {
		if !p.at(token.LParen) {
			p.fail(p.tok().Span, fmt.Sprintf("decorator fn の引数の「(」が要る。引数がなくても書く（decorator fn %s() と書く。docs/grammar.md「decorator の宣言」）", name))
		}
		d.Params, d.Wrapped = p.parseParams(true)
		p.parseDecoratorFnTail(d)
		outerOld()
		d.Span = p.spanFrom(start)
		return d
	}
	if d.Entry {
		p.errorAt(d.EntrySpan, "@entry を付けられるのは、関数に付けるデコレータ（decorator fn "+d.Name.Name+"(…)）だけ（docs/grammar.md「decorator の宣言」）")
		d.Entry, d.EntrySpan = false, source.Span{}
	}
	if p.at(token.LParen) {
		d.Params, _ = p.parseParams(false)
	}
	if !p.at(token.For) {
		// なくした書き方: for を省いた形（前は、op の意味）。decorator fn として読み進める
		p.errorAt(d.Name.Span, fmt.Sprintf("付ける先を書かない decorator %s … という書き方はなくなった。関数に付けるデコレータは decorator fn %s(…) と書き、そのほかは for で付ける先を書く（docs/grammar.md「decorator の宣言」）", name, name))
		d.Fn = true
		p.parseOldDecoratorClauses(d)
		outerOld()
		p.expect(token.Semicolon)
		d.Span = p.spanFrom(start)
		return d
	}
	forSpan := p.advance().Span
	// 付ける先の並び: Target { "," Target }。"|" は、なくした書き方（docs/grammar.md「decorator の宣言」）:
	// 区切りが "," であることをメッセージで示して SYNTAX にし、"," として読み進める
	d.For = append(d.For, p.parseTarget())
	for {
		if p.at(token.Pipe) {
			p.errorAt(p.tok().Span, "for の対象の並びの区切りは「,」で、「|」ではない（for model, state, op と書く。docs/grammar.md「decorator の宣言」）")
			p.advance()
		} else if !p.got(token.Comma) {
			break
		}
		d.For = append(d.For, p.parseTarget())
	}
	if len(d.For) == 1 && d.For[0] != nil && d.For[0].Kind == ast.TargetOp {
		// なくした書き方: op だけに付ける for の形。decorator fn として読み進める
		p.errorAt(p.spanFrom(forSpan.Start), fmt.Sprintf("decorator %s for op … という書き方はなくなった。関数に付けるデコレータは decorator fn %s(…) と書く（節は「:」の後ろ。docs/grammar.md「decorator の宣言」）", name, name))
		d.Fn, d.For = true, nil
		p.parseOldDecoratorClauses(d)
		outerOld()
		p.expect(token.Semicolon)
		d.Span = p.spanFrom(start)
		return d
	}
	if p.atOuterThrows() {
		p.fail(p.tok().Span, fmt.Sprintf("for で付ける先を書いたデコレータは、節も本体も持てない。節と本体を書けるのは、関数に付けるデコレータ（decorator fn %s(…): 節 { … }）だけ（docs/grammar.md「decorator の宣言」）", name))
	}
	switch p.tok().Kind {
	case token.Throws, token.Catches, token.Reads, token.Effects, token.Colon, token.LBrace:
		p.fail(p.tok().Span, fmt.Sprintf("for で付ける先を書いたデコレータは、節も本体も持てない。節と本体を書けるのは、関数に付けるデコレータ（decorator fn %s(…): 節 { … }）だけ（docs/grammar.md「decorator の宣言」）", name))
	}
	p.expect(token.Semicolon)
	d.Span = p.spanFrom(start)
	return d
}

// parseDecoratorFnTail は decorator fn の「)」の後ろを読む（docs/grammar.md「decorator の宣言」）。
//
//	[ ":" DecoratorClause { DecoratorClause } ] ( ";" | Block )
//
// 節を書くなら「:」で始め、節がなければ「:」も書かない（関数の宣言の FnType と、同じ決まり）。戻り値は、書かない。
func (p *parser) parseDecoratorFnTail(d *ast.DecoratorDecl) {
	name := d.Name.Name
	atClause := func() bool {
		switch p.tok().Kind {
		case token.Throws, token.Catches, token.Reads, token.Effects:
			return true
		}
		return p.atOuterThrows()
	}
	switch {
	case p.at(token.Colon):
		colon := p.advance().Span
		if !atClause() {
			switch p.tok().Kind {
			case token.Semicolon, token.LBrace:
				p.fail(colon, fmt.Sprintf("「:」の後ろに、節が要る。何も書かないなら、「:」も書かない（decorator fn %s(…); か decorator fn %s(…) { … } と書く。docs/grammar.md「decorator の宣言」）", name, name))
			}
			p.fail(p.tok().Span, "decorator fn に、戻り値は書かない。「:」の後ろは、節（throws / outer throws / catches / reads / effects）だけ（包んだ op の結果が、そのまま返る。docs/grammar.md「decorator の宣言」）")
		}
	case atClause():
		p.fail(p.tok().Span, fmt.Sprintf("「)」の後ろに、節を書くなら、「:」で始める（decorator fn %s(…): %s … と書く。docs/grammar.md「decorator の宣言」）", name, p.tok().Text))
	case p.at(token.For):
		p.fail(p.tok().Span, "decorator fn に for は書かない。fn が、付ける先（op）を言う（docs/grammar.md「decorator の宣言」）")
	}
	p.parseOldDecoratorClauses(d)
	switch p.tok().Kind {
	case token.Semicolon:
		p.advance()
	case token.LBrace:
		d.Body = p.parseBlock()
	case token.For:
		p.fail(p.tok().Span, "decorator fn に for は書かない。fn が、付ける先（op）を言う（docs/grammar.md「decorator の宣言」）")
	default:
		p.failExpected("「;」かデコレータの本体の「{」")
	}
}

// atOuterThrows は、今の位置が、節 outer throws の頭か（outer は文脈キーワード: 直後が throws のときだけ、この意味）。
func (p *parser) atOuterThrows() bool {
	return p.at(token.Ident) && p.tok().Text == "outer" && p.peek(1).Kind == token.Throws
}

// parseOldDecoratorClauses は DecoratorClause の並びを読む（頭の「:」は、呼ぶ側が読む）。
func (p *parser) parseOldDecoratorClauses(d *ast.DecoratorDecl) {
	for {
		if p.atOuterThrows() {
			start := p.advance().Span.Start
			c := p.parseThrowsClause()
			c.Outer = true
			c.Span.Start = start
			d.Clauses = append(d.Clauses, c)
			continue
		}
		switch p.tok().Kind {
		case token.Throws:
			d.Clauses = append(d.Clauses, p.parseThrowsClause())
		case token.Catches:
			d.Clauses = append(d.Clauses, p.parseCatchesClause())
		case token.Reads:
			d.Clauses = append(d.Clauses, p.parseReadsClause())
		case token.Effects:
			d.Clauses = append(d.Clauses, p.parseEffectsClause())
		default:
			return
		}
	}
}

// parseParams は Params を読む。
//
//	Params    = "(" [ Param { "," Param } [ "," RestParam ] | RestParam ] ")" ;
//	Param     = Ident ":" ParamType ;
//	RestParam = "..." Ident ":" RestType ;
//	RestType  = AnyType | Type { "|" Type } ;
//
// 残りの引数（RestParam）は最後に一つだけ書け、型は渡す値一つの型で、? も [] も付けない（文法で閉じる）。
//
// fnParams が true なら、decorator fn の FnParams: 最初の引数に、包んだ関数の引数（WrappedParam = Ident ":" "fn"）を書ける
// （docs/grammar.md「デコレータの本体」）。それを、二つ目の戻り値で返す（なければ nil）。Params には、入れない。
// 型の位置の fn は、そこにしか書けない: ほかの引数、残りの引数、for の形のデコレータの引数に書けば、SYNTAX。
func (p *parser) parseParams(fnParams bool) ([]*ast.Param, *ast.WrappedParam) {
	p.expect(token.LParen)
	params := []*ast.Param{}
	var wrapped *ast.WrappedParam
	first := true
	for !p.at(token.RParen) {
		start := p.tok().Span.Start
		param := &ast.Param{}
		if p.at(token.Ellipsis) {
			param.Rest, param.RestSpan = true, p.advance().Span
		}
		param.Name = p.parseIdent()
		p.expect(token.Colon)
		if p.at(token.Fn) {
			fnSpan := p.advance().Span
			switch {
			case !fnParams:
				p.fail(fnSpan, "for で付ける先を書いたデコレータは、包んだ関数の引数（"+param.Name.Name+": fn）を持てない。持てるのは、本体のある、関数に付けるデコレータ（decorator fn 名前("+param.Name.Name+": fn) { … }）だけ（docs/grammar.md「デコレータの本体」）")
			case param.Rest || !first:
				p.fail(fnSpan, "包んだ関数の引数（"+param.Name.Name+": fn）は、decorator fn の、最初の引数に、一つだけ書ける（docs/grammar.md「デコレータの本体」）")
			case p.at(token.Question) || p.at(token.LBracket):
				p.fail(p.tok().Span, "包んだ関数の引数の型 fn に、? も [] も付けられない（docs/grammar.md「デコレータの本体」）")
			}
			wrapped = &ast.WrappedParam{Span: p.spanFrom(start), Name: param.Name, FnSpan: fnSpan}
			first = false
			if !p.got(token.Comma) {
				break
			}
			continue
		}
		first = false
		param.Type = p.parseParamType()
		param.Span = p.spanFrom(start)
		if param.Rest {
			p.checkRestType(param)
		}
		params = append(params, param)
		if !p.got(token.Comma) {
			break
		}
		if param.Rest {
			p.fail(param.Span, fmt.Sprintf("残りの引数 ...%s は引数の並びの最後に一つだけ書ける（docs/grammar.md「decorator の宣言」）", param.Name.Name))
		}
	}
	p.expect(token.RParen)
	return params, wrapped
}

// checkRestType は RestType の形を確かめる: 渡す値一つの型で、省略可能（?）にも配列（[]）にもできない。
func (p *parser) checkRestType(param *ast.Param) {
	t := param.Type
	if t.Any != nil {
		if t.Optional {
			p.errorAt(t.Span, fmt.Sprintf("残りの引数 ...%s は省略可能にできない（0 個でも渡せる。docs/grammar.md「decorator の宣言」）", param.Name.Name))
			t.Optional = false
		}
		return
	}
	for i, typ := range t.Types {
		switch x := typ.(type) {
		case *ast.OptionalType:
			p.errorAt(x.Span, fmt.Sprintf("残りの引数 ...%s は省略可能にできない（0 個でも渡せる。docs/grammar.md「decorator の宣言」）", param.Name.Name))
			t.Types[i] = x.Elem
		case *ast.ArrayType:
			p.errorAt(x.Span, fmt.Sprintf("残りの引数 ...%s の型には、渡す値一つの型を書く（[] は付けない。docs/grammar.md「decorator の宣言」）", param.Name.Name))
			t.Types[i] = x.Elem
		}
	}
}

// parseParamType は ParamType = AnyType [ "?" ] | Type { "|" Type } を読む。
func (p *parser) parseParamType() *ast.ParamType {
	start := p.tok().Span.Start
	t := &ast.ParamType{}
	if p.at(token.Any) {
		t.Any = p.parseAnyType()
		// 分類のケースの、型で絞る形（any case of Order?）は、型が、いちばん後ろの ? を読んでいる: それも、引数を省ける印
		t.Optional = p.got(token.Question) || (t.Any.Case != nil && t.Any.Case.Optional)
	} else {
		t.Types = append(t.Types, p.parseType())
		for p.got(token.Pipe) {
			t.Types = append(t.Types, p.parseType())
		}
	}
	t.Span = p.spanFrom(start)
	return t
}

// parseAnyType は AnyType = "any" AnyKind [ "on" QualifiedName "." StateOp ] を読む。variant は文脈キーワード。
func (p *parser) parseAnyType() *ast.AnyType {
	start := p.expect(token.Any).Span.Start
	a := &ast.AnyType{}
	switch {
	case p.at(token.Exception):
		a.Kind = ast.AnyException
	case p.at(token.Effect):
		a.Kind = ast.AnyEffect
	case p.at(token.State):
		a.Kind = ast.AnyState
	case p.at(token.Op):
		a.Kind = ast.AnyOp
	case p.at(token.Model):
		a.Kind = ast.AnyModel
	case p.atCaseKind():
		a.Kind, a.Case = ast.AnyCase, p.parseCaseKind(true)
		a.Span = p.spanFrom(start)
		return a
	case p.atContextual("variant"):
		a.Kind = ast.AnyVariant
	case p.at(token.Classify):
		p.failClassifyKind()
	default:
		p.failExpected("any の種類（exception / effect / state / op / model / variant / case）")
	}
	p.advance()
	if p.got(token.On) {
		ref := p.parseStateOpRef()
		a.On = &ref
	}
	a.Span = p.spanFrom(start)
	return a
}

// parseTarget は Target = "op" | "field" | "state" | "exception" | "scalar" | "model" | "union" | "effect" | "read" |
// "classify" | "decorator" | CaseKind を読む。
// field は文脈キーワード。
func (p *parser) parseTarget() *ast.Target {
	t := &ast.Target{Span: p.tok().Span}
	switch {
	case p.at(token.Op):
		t.Kind = ast.TargetOp
	case p.atCaseKind():
		t.Kind, t.Case = ast.TargetCase, p.parseCaseKind(false)
		t.Span = t.Case.Span
		return t
	case p.atContextual("field"):
		t.Kind = ast.TargetField
	case p.at(token.State):
		t.Kind = ast.TargetState
	case p.at(token.Exception):
		t.Kind = ast.TargetException
	case p.at(token.Scalar):
		t.Kind = ast.TargetScalar
	case p.at(token.Model):
		t.Kind = ast.TargetModel
	case p.at(token.Effect):
		t.Kind = ast.TargetEffect
	case p.at(token.Read):
		t.Kind = ast.TargetRead
	case p.at(token.Union):
		t.Kind = ast.TargetUnion
	case p.at(token.Decorator):
		t.Kind = ast.TargetDecorator
	case p.at(token.Classify):
		// 分類そのもの（classify の宣言）。後ろが case / of / 名前なら、なくした書き方（classify … case）
		if next := p.peek(1).Kind; next == token.Case || next == token.Of || next == token.Ident {
			p.failClassifyKind()
		}
		t.Kind = ast.TargetClassify
	default:
		p.failExpected("for の対象（op / field / state / exception / scalar / model / union / effect / read / classify / decorator / case）")
	}
	p.advance()
	return t
}

// atCaseKind は、tok が CaseKind（分類のケース）の頭か。
func (p *parser) atCaseKind() bool { return p.at(token.Case) }

// parseCaseKind は CaseKind = "case" [ "of" ( "any" | "never" | Type ) ] を読む（分類のケース）。
// case は case of any の省略。of の後ろの型は、型として読む（分類の名前も、型の名前として読み、名前の解決が分ける。
// docs/grammar.md「decorator の宣言」）。
//
// 型で絞る形の対象に、省略可能な型（いちばん外が ? か nullable<…>）は、書けない（決定 D354）。
// 引数の種類（param が true。any の後ろ）では、いちばん後ろの ? を、引数を省ける印として読み（CaseKind.Optional）、対象の型から外す。
// 付ける先（for の後ろ）には、省ける印がないので、SYNTAX。
func (p *parser) parseCaseKind(param bool) *ast.CaseKind {
	start := p.expect(token.Case).Span.Start
	k := &ast.CaseKind{}
	end := p.spanFrom(start)
	if p.got(token.Of) {
		switch {
		case p.got(token.Any):
		case p.gotTypeWord(token.NeverName):
			k.Never = true
		default:
			k.Of = p.parseType()
			if opt, ok := k.Of.(*ast.OptionalType); ok && param {
				k.Of, k.Optional = opt.Elem, true
				end = source.Span{File: end.File, Start: start, End: opt.Elem.NodeSpan().End}
			}
			if optionalAtTop(k.Of) {
				p.fail(k.Of.NodeSpan(), "型で絞る形（case of 型）の対象に、省略可能な型は書けない。省略可能な型を対象にする分類のケースは、名前で絞る（case of 分類の名前）と書く（docs/grammar.md「decorator の宣言」）")
			}
		}
		if !k.Optional {
			end = p.spanFrom(start)
		}
	}
	k.Span = end
	return k
}

// optionalAtTop は、型 t の、いちばん外が、省略可能か（T? か nullable<T>）。
func optionalAtTop(t ast.Type) bool {
	switch x := t.(type) {
	case *ast.OptionalType:
		return true
	case *ast.ShapeType:
		return x.Shape == "nullable"
	}
	return false
}

// failClassifyKind は、any の後ろの classify（引数の種類としての、分類そのものは、まだ、書けない）と、for の後ろの、
// なくした書き方（classify … case）: 新しい書き方を、示す（docs/grammar.md「decorator の宣言」）。for の後ろの classify は、
// 後ろが case / of / 名前でなければ、付ける先（parseTarget）。
func (p *parser) failClassifyKind() {
	p.fail(p.tok().Span, "分類そのもの（classify）を、引数の種類（any classify）には、まだ、書けない。分類のケースは、case of any（省略して case）、case of never、case of 分類の名前 と書く（classify … case は、なくした書き方。docs/grammar.md「decorator の宣言」）")
}

// parseThrowsClause は ThrowsClause = "throws" ExcRef { "," ExcRef } { Relation } を読む。
func (p *parser) parseThrowsClause() *ast.ThrowsClause {
	start := p.expect(token.Throws).Span.Start
	c := &ast.ThrowsClause{Exceptions: p.parseNameList("throws")}
	for p.at(token.Prevents) || p.at(token.Keeps) {
		c.Relations = append(c.Relations, p.parseRelation())
	}
	c.Span = p.spanFrom(start)
	return c
}

// parseRelation は Relation = ( "prevents" | "keeps" ) "effects" を読む。
// 目的語は `effects` だけで、効果の名前は書けない（docs/grammar.md「決めたこと」）。
func (p *parser) parseRelation() *ast.Relation {
	start := p.tok().Span.Start
	r := &ast.Relation{}
	if p.got(token.Keeps) {
		r.Kind = ast.Keeps
	} else {
		p.expect(token.Prevents)
		r.Kind = ast.Prevents
	}
	if !p.at(token.Effects) {
		if p.atName() {
			p.fail(p.tok().Span, fmt.Sprintf("%s の目的語は「effects」だけ。効果の名前は書けない（docs/grammar.md「決めたこと」）", r.Kind))
		}
		p.failExpected("「effects」")
	}
	p.advance()
	r.Span = p.spanFrom(start)
	return r
}

// parseCatchesClause は CatchesClause = "catches" ExcRef { "," ExcRef } ( "recovers" | "halts" QualifiedName | "ignores" ) を読む。
func (p *parser) parseCatchesClause() *ast.CatchesClause {
	start := p.expect(token.Catches).Span.Start
	c := &ast.CatchesClause{Exceptions: p.parseNameList("catches")}
	switch p.tok().Kind {
	case token.Recovers:
		p.advance()
		c.Outcome = ast.Recovers
	case token.Halts:
		p.advance()
		c.Outcome = ast.Halts
		e := p.parseQualifiedName()
		c.Halts = &e
	case token.Ignores:
		p.advance()
		c.Outcome = ast.Ignores
	default:
		p.failExpected("catches の後の振る舞い（recovers / halts 効果 / ignores）")
	}
	c.Span = p.spanFrom(start)
	return c
}

// parseNameList は節の並び QualifiedName { "," QualifiedName } を読む。clause は節の頭の語（throws / catches / effects / reads）。
// 区切りはどの節も ","（docs/grammar.md「op」）。要素の後ろに "|" が来たら（なくした書き方）、区切りが "," であることを
// メッセージで示して SYNTAX にし、"," として読み進める。
func (p *parser) parseNameList(clause string) []ast.QualifiedName {
	names := []ast.QualifiedName{p.parseQualifiedName()}
	for {
		if p.at(token.Pipe) {
			p.errorAt(p.tok().Span, fmt.Sprintf("%s の並びの区切りは「,」で、「|」ではない（%s A, B と書く。docs/grammar.md「op」）", clause, clause))
			p.advance()
		} else if !p.got(token.Comma) {
			return names
		}
		names = append(names, p.parseQualifiedName())
	}
}

// parseOp は OpDecl を読む（docs/grammar.md「op」）。
//
//	OpDecl = [ "export" ] FnHead [ ":" ( ReturnType { OpClause } | OpClause { OpClause } ) ] ( ";" | Block ) ;
//	FnHead = "fn" Ident OpParams ;                              （docs/grammar.md「関数の宣言」）
//
// export は parseDecl が読む。
func (p *parser) parseOp(start source.Offset, doc *ast.DocComment, decorators []*ast.Decorator) *ast.OpDecl {
	p.expect(token.Fn)
	o := &ast.OpDecl{Doc: doc, Decorators: decorators, Name: p.parseIdent()}
	o.Params = p.parseOpParams()
	o.Return = p.parseFnType("fn " + o.Name.Name)
	for {
		switch p.tok().Kind {
		case token.Effects:
			o.Clauses = append(o.Clauses, p.parseEffectsClause())
			continue
		case token.Reads:
			o.Clauses = append(o.Clauses, p.parseReadsClause())
			continue
		case token.Throws:
			o.Clauses = append(o.Clauses, p.parseThrowsClause())
			continue
		case token.Catches:
			p.fail(p.tok().Span, "catches はデコレータの宣言にだけ書ける（docs/grammar.md「op」）")
		case token.On:
			p.fail(p.tok().Span, fmt.Sprintf("種類を書かない fn に on は書けない。状態を読み書きするのは、read fn と effect fn（read fn %s(…): T reads 状態;、effect fn %s(…): effects 状態.操作;。docs/grammar.md「関数の宣言」）", o.Name.Name, o.Name.Name))
		}
		break
	}
	switch p.tok().Kind {
	case token.Semicolon:
		p.advance()
	case token.LBrace:
		o.Body = p.parseBlock()
	default:
		p.failExpected("「;」か配線の「{」")
	}
	o.Span = p.spanFrom(start)
	return o
}

// parseEffectsClause は "effects" EffectElem { "," EffectElem } を読む（docs/grammar.md「op」の OpClause と、
// 「decorator の宣言」の DecoEffectsClause）。要素が効果の名前か、直に書いた「状態.操作」かは、名前の解決が決める。
func (p *parser) parseEffectsClause() *ast.EffectsClause {
	start := p.expect(token.Effects).Span.Start
	c := &ast.EffectsClause{Effects: p.parseNameList("effects")}
	c.Span = p.spanFrom(start)
	return c
}

// parseReadsClause は "reads" ReadElem { "," ReadElem } を読む（docs/grammar.md「op」の OpClause と、
// 「decorator の宣言」の DecoReadsClause）。要素が read の名前か、直に書いた状態の名前かは、名前の解決が決める。
func (p *parser) parseReadsClause() *ast.ReadsClause {
	start := p.expect(token.Reads).Span.Start
	c := &ast.ReadsClause{Reads: p.parseNameList("reads")}
	c.Span = p.spanFrom(start)
	return c
}

// parseOpParams は OpParams = "(" [ OpParam { "," OpParam } ] ")" を読む。
func (p *parser) parseOpParams() []*ast.OpParam {
	p.expect(token.LParen)
	params := []*ast.OpParam{}
	for !p.at(token.RParen) {
		start := p.tok().Span.Start
		param := &ast.OpParam{Name: p.parseIdent()}
		p.expect(token.Colon)
		param.Type = p.parseType()
		param.Span = p.spanFrom(start)
		params = append(params, param)
		if !p.got(token.Comma) {
			break
		}
	}
	p.expect(token.RParen)
	return params
}

// parseReturnType は ReturnType = Type | "void" | "never" を読む。
func (p *parser) parseReturnType() *ast.ReturnType {
	if p.atTypeWord(token.VoidName) {
		return &ast.ReturnType{Span: p.advance().Span, Void: true}
	}
	if p.atTypeWord(token.NeverName) {
		return &ast.ReturnType{Span: p.advance().Span, Never: true}
	}
	t := p.parseType()
	return &ast.ReturnType{Span: t.NodeSpan(), Type: t}
}

// --- 型（docs/grammar.md「型」） ---

// parseType は Type = TypePrefix { "[" "]" | "?" } を読む。形の語（list<T> / nullable<T> / single<T>）は
// 型の別の書き方で、どの型の位置にも書ける。
func (p *parser) parseType() ast.Type {
	start := p.tok().Span.Start
	var t ast.Type
	// 型の位置では、要素が一つの名前が、核の型と形の名前なら、いつも、その型（名前の解決を通らない。docs/grammar.md「型」）
	word := ""
	if p.at(token.Ident) && token.IsTypeName(p.tok().Text) {
		word = p.tok().Text
	}
	switch {
	case token.IsCoreTypeName(word):
		tok := p.advance()
		t = &ast.CoreType{Span: tok.Span, Name: tok.Text}
	case token.IsShapeName(word):
		shape := p.advance()
		p.expect(token.Less)
		st := &ast.ShapeType{Shape: shape.Text, ShapeSpan: shape.Span, Elem: p.parseType()}
		p.expect(token.Greater)
		st.Span = p.spanFrom(start)
		t = st
	case word == token.VoidName:
		p.fail(p.tok().Span, "void は op の戻り値の型にだけ書ける（docs/grammar.md「型」）")
	case word == token.NeverName:
		p.fail(p.tok().Span, "never は op の戻り値の型と、override のなくす印にだけ書ける（docs/grammar.md「型」）")
	case p.at(token.Ident), p.at(token.QualifiedName):
		if p.tok().Kind == token.Ident && p.tok().Text == "Record" && p.peek(1).Kind == token.Less {
			// なくした書き方（docs/grammar.md「model」「決めたこと」）
			p.fail(p.tok().Span, "Record<K, V> という書き方はなくなった。キーが決まっていない構造は、インデックスの model を宣言して、その名前を書く（model 名前 { [キーの名前: K]: V; }。docs/grammar.md「model」）")
		}
		n := &ast.NamedType{Name: p.parseQualifiedName()}
		if p.at(token.Less) {
			// なくした書き方（docs/grammar.md「型」「決めたこと」）
			p.fail(p.tok().Span, fmt.Sprintf("%s<T> という書き方はなくなった。分類のケースで絞り込んだ型は、T on %s と書く（<> は、list<T>、nullable<T>、single<T> だけのもの。docs/grammar.md「型」）", n.Name.String(), n.Name.String()))
		}
		n.Span = p.spanFrom(start)
		t = n
	case p.at(token.String), p.at(token.Int), p.at(token.Float), p.at(token.True), p.at(token.False):
		// リテラルの型（docs/grammar.md「型」の LiteralType）: その値だけを持つ型
		lit := p.parseLiteral()
		t = &ast.LiteralType{Span: lit.Span, Lit: lit}
	case p.at(token.Duration):
		p.fail(p.tok().Span, "期間のリテラルは、型の位置には書けない（リテラルの型は、文字列、整数、小数、真偽だけ。docs/grammar.md「型」）")
	case p.at(token.LBrace):
		p.fail(p.tok().Span, "その場で書く model はユニオンのアームと例外の値にだけ置ける（docs/grammar.md「決めたこと」）")
	case p.at(token.Fn):
		p.fail(p.tok().Span, "fn は、型ではない（関数を、値として渡す書き方は、ない）。型の位置に書けるのは、decorator fn の、最初の引数（包んだ関数の引数）だけ（docs/grammar.md「デコレータの本体」）")
	default:
		p.failExpected("型")
	}
	for {
		switch p.tok().Kind {
		case token.LBracket:
			p.advance()
			p.expect(token.RBracket)
			t = &ast.ArrayType{Span: p.spanFrom(start), Elem: t}
		case token.Question:
			p.advance()
			t = &ast.OptionalType{Span: p.spanFrom(start), Elem: t}
		case token.On:
			if p.noOnType && p.peek(1).Kind != token.QualifiedName {
				return t // なくした書き方 `read fn r(): T on 状態;`、`fn f(): T on 状態;` の on。呼んだ側が、メッセージを出す
			}
			return p.parseOnType(start, t)
		default:
			return t
		}
	}
}

// parseOnType は OnType = "on" CaseRef { "|" CaseRef } を読む（docs/grammar.md「型」）: 分類のケースで絞り込んだ型 T on C.K。
// of は、on の前までに読んだ型で、start は、その頭。on の後ろには、[] も ? も続けない。
func (p *parser) parseOnType(start source.Offset, of ast.Type) ast.Type {
	t := &ast.OnType{Of: of, OnSpan: p.expect(token.On).Span}
	for {
		if p.atIdent() {
			p.fail(p.tok().Span, fmt.Sprintf("絞り込んだ型のケースは、分類の名前から書く（T on <分類>.%s。docs/grammar.md「型」）", p.tok().Text))
		}
		if !p.at(token.QualifiedName) {
			p.failExpected("絞り込んだ型のケース（T on <分類>.<ケース>）")
		}
		t.Cases = append(t.Cases, p.parseQualifiedName())
		if p.at(token.Comma) && p.peek(1).Kind == token.QualifiedName && p.peek(2).Kind != token.Colon && onTypeListEnd(p.peek(2).Kind) {
			p.fail(p.tok().Span, "絞り込んだ型の、いくつかのケースの区切りは「|」で、「,」ではない（T on C.A | C.B と書く。docs/grammar.md「型」）")
		}
		if !p.got(token.Pipe) {
			break
		}
	}
	switch p.tok().Kind {
	case token.LBracket, token.Question:
		p.fail(p.tok().Span, "on の後ろに [] と ? は続けられない。絞り込んだ型の並びは list<T on C.K>、省略可能は nullable<T on C.K> と書く（docs/grammar.md「型」）")
	case token.On:
		p.fail(p.tok().Span, "on は、一つの型に、一度だけ書ける（二つの分類の絞り込みを、同時に持つ型は、ない。docs/types.md「アームの中の対象の型」）")
	}
	t.Span = p.spanFrom(start)
	return t
}

// onTypeListEnd は、絞り込んだ型のケースを「,」で並べた、と読める形の、終わりの字句か（「,」の後ろの名前の、次の字句）。
// 引数の並びの「,」（次が「名前 :」）とは、名前の後ろの「:」で見分ける。
func onTypeListEnd(k token.Kind) bool {
	switch k {
	case token.RParen, token.Comma, token.Pipe, token.Semicolon, token.Greater:
		return true
	}
	return false
}

// --- 配線（docs/grammar.md「配線」） ---

// parseBlock は Block = "{" { Stmt } "}" を読む。誤った文は次の ";" まで読み飛ばし、読めた文を残す。
func (p *parser) parseBlock() *ast.Block {
	start := p.expect(token.LBrace).Span.Start
	b := &ast.Block{Stmts: []ast.Stmt{}}
	p.parseMembers(func() {
		b.Stmts = append(b.Stmts, p.parseStmt(false)...)
	})
	p.expect(token.RBrace)
	b.Span = p.spanFrom(start)
	return b
}

// end は End = [ ";" ] を読む。";" を省けるのは、直前の字句が "}" のときだけ（docs/grammar.md「配線」）。
func (p *parser) end() {
	if p.got(token.Semicolon) {
		return
	}
	if p.pos > 0 && p.toks[p.pos-1].Kind == token.RBrace {
		return
	}
	p.expect(token.Semicolon)
}

// parseStmt は文を一つ読む（docs/grammar.md「配線」の Stmt）。名前を並べた空の val は、名前ごとの文になるので、並びを返す。
// arm が真なら、disjoint の、文の一つのアーム（ArmStmt）: val の宣言と return は書けない。
//
//	Stmt = ValDecl | Assign | Collect | "return" Expr End | Call End | Match End | Disjoint End | [ "disjoint" ] For End ;
func (p *parser) parseStmt(arm bool) []ast.Stmt {
	start := p.tok().Span.Start
	switch p.tok().Kind {
	case token.Val:
		if arm {
			p.fail(p.tok().Span, "disjoint の、文の一つのアームに、val の宣言は書けない（宣言は、ブロックのアーム { … } の中に書く。docs/grammar.md「配線」）")
		}
		return p.parseVal()
	case token.Return:
		if arm {
			p.fail(p.tok().Span, "disjoint のアームに、return は書けない（docs/grammar.md「配線」）")
		}
		p.advance()
		s := &ast.ReturnStmt{Value: p.parseExpr()}
		p.end()
		s.Span = p.spanFrom(start)
		return []ast.Stmt{s}
	case token.Match:
		s := &ast.MatchStmt{Match: p.parseMatch()}
		p.end()
		s.Span = p.spanFrom(start)
		return []ast.Stmt{s}
	case token.Disjoint:
		if p.peek(1).Kind == token.For {
			return []ast.Stmt{p.parseFor()}
		}
		return []ast.Stmt{p.parseDisjoint()}
	case token.For:
		return []ast.Stmt{p.parseFor()}
	case token.Try:
		if arm {
			p.fail(p.tok().Span, "disjoint のアームに、try は書けない（docs/grammar.md「配線」）")
		}
		return []ast.Stmt{p.parseTry()}
	case token.Catch:
		p.fail(p.tok().Span, "catch は、try のブロックの直後にだけ書ける（try { … } catch (e) { … }。docs/grammar.md「配線」）")
	case token.Ident, token.QualifiedName:
		if p.at(token.Ident) {
			switch {
			case p.tok().Text == "const" && p.peek(1).Kind == token.Ident:
				p.fail(p.tok().Span, "const は、なくした書き方。val と書く: val 名前 = 式;（docs/grammar.md「配線」）")
			case p.peek(1).Kind == token.Assign:
				s := &ast.AssignStmt{Name: p.parseIdent()}
				p.expect(token.Assign)
				s.Value = p.parseExpr()
				p.end()
				s.Span = p.spanFrom(start)
				return []ast.Stmt{s}
			case p.peek(1).Kind == token.Collect:
				s := &ast.CollectStmt{Name: p.parseIdent()}
				p.expect(token.Collect)
				s.Value = p.parseExpr()
				p.end()
				s.Span = p.spanFrom(start)
				return []ast.Stmt{s}
			}
		}
		s := &ast.CallStmt{Call: p.parseCall()}
		p.end()
		s.Span = p.spanFrom(start)
		return []ast.Stmt{s}
	case token.DocComment:
		p.fail(p.tok().Span, "付く先のないドキュメンテーションコメント（docs/grammar.md「決めたこと」）")
	case token.Throw:
		p.fail(p.tok().Span, "throw を書けるのは、match のアームの本体と、catch のブロックの最後の文だけ（docs/grammar.md「配線」）")
	case token.Semicolon:
		p.fail(p.tok().Span, "「何もしない文」は、ない。; は、文の終わりに、一つだけ書く（docs/grammar.md「配線」）")
	}
	if arm {
		p.failExpected("disjoint のアーム（ブロック { … } か、文の一つ: 値を入れる / 集める / 呼び出し / match / disjoint / for）")
	}
	p.failExpected("文（val / 値を入れる / 集める / return / 呼び出し / match / disjoint / for / try）")
	return nil
}

// parseVal は ValDecl を読む。名前を並べた空の宣言は、名前ごとの ValStmt にする。
//
//	ValDecl = "val" ValName { "," ValName } End | "val" Ident [ ":" Type ] "=" Expr End ;
//	ValName = Ident [ ":" Type ] ;
func (p *parser) parseVal() []ast.Stmt {
	start := p.expect(token.Val).Span.Start
	var out []ast.Stmt
	for {
		nameStart := p.tok().Span.Start
		if len(out) == 0 {
			nameStart = start
		}
		s := &ast.ValStmt{Name: p.parseIdent()}
		if p.got(token.Colon) {
			s.Type = p.parseType()
		}
		if p.at(token.Assign) {
			if len(out) > 0 {
				p.fail(p.tok().Span, "値と一緒に宣言する val は、一つの文に、一つ（val a = …; val b = …; と分ける。名前を並べられるのは、空の宣言だけ。docs/grammar.md「配線」）")
			}
			p.advance()
			s.Value = p.parseExpr()
			if p.at(token.Comma) {
				p.fail(p.tok().Span, "値と一緒に宣言する val は、一つの文に、一つ（val a = …; val b = …; と分ける。名前を並べられるのは、空の宣言だけ。docs/grammar.md「配線」）")
			}
			p.end()
			s.Span = p.spanFrom(start)
			return []ast.Stmt{s}
		}
		s.Span = p.spanFrom(nameStart)
		out = append(out, s)
		if !p.got(token.Comma) {
			break
		}
	}
	p.end()
	return out
}

// parseDisjoint は Disjoint = "disjoint" "{" Arm Arm { Arm } "}" を読む。Arm = Block End | ArmStmt。
func (p *parser) parseDisjoint() ast.Stmt {
	kw := p.expect(token.Disjoint)
	s := &ast.DisjointStmt{Keyword: kw.Span}
	p.expect(token.LBrace)
	p.parseMembers(func() {
		armStart := p.tok().Span.Start
		arm := &ast.Arm{}
		if p.at(token.LBrace) {
			b := p.parseBlock()
			p.got(token.Semicolon)
			arm.Block, arm.Stmts = true, b.Stmts
		} else {
			arm.Stmts = p.parseStmt(true)
		}
		arm.Span = p.spanFrom(armStart)
		s.Arms = append(s.Arms, arm)
	})
	p.expect(token.RBrace)
	if len(s.Arms) < 2 {
		p.errorAt(kw.Span, "disjoint には、アームが二つ以上要る（docs/grammar.md「配線」）")
	}
	p.end()
	s.Span = p.spanFrom(kw.Span.Start)
	return s
}

// parseFor は [ "disjoint" ] For を読む。For = "for" "(" "val" Ident "of" Atom ")" Block。
func (p *parser) parseFor() ast.Stmt {
	start := p.tok().Span.Start
	s := &ast.ForStmt{Keyword: p.tok().Span}
	if p.got(token.Disjoint) {
		s.Disjoint = true
	}
	p.expect(token.For)
	p.expect(token.LParen)
	if !p.at(token.Val) {
		p.fail(p.tok().Span, "for の頭は (val 名前 of 並び) の形だけ（数を増やしながら回す形は、ない。docs/grammar.md「配線」）")
	}
	p.advance()
	s.Var = p.parseIdent()
	p.expect(token.Of)
	if p.atName() && p.peek(1).Kind == token.LParen {
		p.fail(p.tok().Span, "for の並びに、呼び出しは書けない（先に val に受ける: val xs = f(…); for (val x of xs) { … }。docs/grammar.md「配線」）")
	}
	s.Seq = p.parseAtom()
	p.expect(token.RParen)
	s.Body = p.parseBlock()
	p.end()
	s.Span = p.spanFrom(start)
	return s
}

// parseExpr は Expr = Call | Match | Atom を読む。
func (p *parser) parseExpr() ast.Expr {
	if p.at(token.Match) {
		return p.parseMatch()
	}
	if p.atName() && p.peek(1).Kind == token.LParen {
		return p.parseCall()
	}
	if p.at(token.Throw) {
		p.fail(p.tok().Span, "throw を書けるのは、match のアームの本体と、catch のブロックの最後の文だけ（docs/grammar.md「配線」）")
	}
	return p.parseAtom()
}

// parseCall は Call = QualifiedName "(" [ Atom { "," Atom } ] ")" を読む。引数は Atom だけ（設計書 §9「A 正規形」）。
func (p *parser) parseCall() *ast.Call {
	start := p.tok().Span.Start
	c := &ast.Call{Callee: p.parseQualifiedName()}
	c.Args = p.parseCallArgs()
	c.Span = p.spanFrom(start)
	return c
}

// parseCallArgs は呼び出しと throw の引数 "(" [ Atom { "," Atom } ] ")" を読む。引数は Atom だけ。
func (p *parser) parseCallArgs() []ast.Atom {
	args := []ast.Atom{}
	p.expect(token.LParen)
	for !p.at(token.RParen) {
		if p.atName() && p.peek(1).Kind == token.LParen {
			p.fail(p.tok().Span, "呼び出しの引数に呼び出しは書けない。中間結果に val で名前を付ける")
		}
		args = append(args, p.parseAtom())
		if !p.got(token.Comma) {
			break
		}
	}
	p.expect(token.RParen)
	return args
}

// parseAtom は Atom = Path | Literal | "none" | RecordLit を読む。
func (p *parser) parseAtom() ast.Atom {
	switch {
	case p.atName():
		qn := p.parsePath()
		return &ast.NameAtom{Span: qn.Span, Name: qn}
	case p.tok().Kind.IsLiteral():
		return p.parseLiteral()
	case p.at(token.None):
		return &ast.NoneAtom{Span: p.advance().Span}
	case p.at(token.LBrace):
		return p.parseRecordLit()
	}
	p.failExpected("値（名前・リテラル・none・レコード）")
	return nil
}

// parseRecordLit は RecordLit = "{" [ RecordField { "," RecordField } [ "," ] ] "}" を読む。
//
//	RecordField = FieldName ":" Atom | Ident ;
//
// キーは FieldName なので予約語も書ける（`{ op: x }`）。値を省いた形（`{ item }`）は同じ名前の束縛か引数を
// 指すので Ident だけで、予約語は書けない（その名前の束縛や引数は作れない）。
func (p *parser) parseRecordLit() *ast.RecordLit {
	start := p.expect(token.LBrace).Span.Start
	p.openRecords++
	r := &ast.RecordLit{Fields: []*ast.RecordField{}}
	for !p.at(token.RBrace) {
		fstart := p.tok().Span.Start
		if p.tok().Kind.IsKeyword() && p.peek(1).Kind != token.Colon {
			p.fail(p.tok().Span, fmt.Sprintf("%s は予約語なので、値を省いた形（{ %s }）では書けない。その名前の束縛や引数は作れないので、{ %s: 値 } と書く（docs/grammar.md「配線」）", p.tok().Text, p.tok().Text, p.tok().Text))
		}
		f := &ast.RecordField{Name: p.parseFieldName()}
		if p.got(token.Colon) {
			f.Value = p.parseAtom()
		}
		f.Span = p.spanFrom(fstart)
		r.Fields = append(r.Fields, f)
		if !p.got(token.Comma) {
			break
		}
	}
	p.expect(token.RBrace)
	p.openRecords--
	r.Span = p.spanFrom(start)
	return r
}

// parseMatch は Match = "match" [ Atom ] "by" ( QualifiedName | "list" | "nullable" ) "{" MatchArm { MatchArm } [ ElseArm ] "}" を読む。
// 対象を書かない形（match by C）は状態の分類、by list / by nullable は標準の分類。
// どの形がどの分類に合うかは検査（MATCH-7）が決める。
// ElseArm は、MatchArm を一つ以上書いた後ろに、最後に一つだけ（文法で閉じる）: else のアームの後ろのアームは、誤りにして捨てる。
// MatchArm より前の else のアームは、誤りにして残す（ほかのアームがないので、すべてのケースを受ける）。
func (p *parser) parseMatch() *ast.Match {
	start := p.expect(token.Match).Span.Start
	m := &ast.Match{}
	if !p.at(token.LParen) {
		p.fail(p.tok().Span, "match の頭は、全体を、丸括弧で囲む: match (x by C) { … }（丸括弧のない頭は、なくした書き方。docs/grammar.md「配線」）")
	}
	p.advance()
	if !p.at(token.By) {
		m.Subject = p.parseAtom()
	}
	p.expect(token.By)
	// by の後ろの、要素が一つの list / nullable は、いつも、標準の分類（その名前の分類は、宣言できない。docs/grammar.md「型」）
	switch {
	case p.atTypeWord("list"), p.atTypeWord("nullable"):
		t := p.advance()
		m.Classifier = ast.QualifiedName{Span: t.Span, Segments: []ast.Ident{{Span: t.Span, Name: t.Text}}}
		m.Builtin = true
	case p.atTypeWord("single"):
		p.fail(p.tok().Span, "single は標準の分類ではないので、match の by には書けない（標準の分類は list と nullable。docs/check.md「標準の分類」）")
	default:
		m.Classifier = p.parseQualifiedName()
	}
	p.expect(token.RParen)
	p.expect(token.LBrace)
	sawElse := false
	p.parseMembers(func() {
		arm := p.parseMatchArm()
		head := source.Span{File: arm.Span.File, Start: arm.Span.Start, End: arm.Span.Start}
		switch {
		case sawElse:
			head.End = p.armHeadEnd(arm)
			p.errorAt(head, "else のアームの後ろに、アームは書けない（else は、最後に一つだけ。docs/grammar.md「配線」）")
			return
		case arm.Else && len(m.Arms) == 0:
			p.errorAt(arm.ElseSpan, "else のアームは、ケースを名前で並べたアームの後ろに書く（else は「ほかのケースのすべて」。docs/grammar.md「配線」）")
		}
		sawElse = sawElse || arm.Else
		m.Arms = append(m.Arms, arm)
	})
	closing := p.expect(token.RBrace)
	if len(m.Arms) == 0 {
		p.errorAt(closing.Span, "match にはアームが一つ以上要る（docs/grammar.md「配線」）")
	}
	m.Span = p.spanFrom(start)
	return m
}

// armHeadEnd はアームの頭（ケースの並びか else）の終わり。診断の位置に使う。
func (p *parser) armHeadEnd(a *ast.MatchArm) source.Offset {
	if a.Else {
		return a.ElseSpan.End
	}
	if n := len(a.Cases); n > 0 {
		return a.Cases[n-1].Span.End
	}
	return a.Span.End
}

// parseThrow は Throw = "throw" QualifiedName [ "(" Atom { "," Atom } ")" ] を読む。
func (p *parser) parseThrow() *ast.Throw {
	tstart := p.expect(token.Throw).Span.Start
	t := &ast.Throw{Exception: p.parseQualifiedName()}
	if p.at(token.LParen) {
		open := p.tok().Span
		t.Args = p.parseCallArgs()
		if len(t.Args) == 0 {
			p.errorAt(source.Span{File: open.File, Start: open.Start, End: p.prevEnd()}, fmt.Sprintf("引数のない例外は括弧を書かない（throw %s と書く。docs/grammar.md「配線」）", t.Exception.String()))
		}
	}
	t.Span = p.spanFrom(tstart)
	return t
}

// parseTry は Try を読む（docs/grammar.md「配線」）。
//
//	Try        = "try" Block "catch" "(" Ident [ "by" QualifiedName ] ")" ( CatchBlock | CatchArms ) ;
//	CatchBlock = "{" { Stmt } [ Throw ";" ] "}" ;
//	CatchArms  = "{" MatchArm { MatchArm } [ ElseArm ] "}" ;
func (p *parser) parseTry() ast.Stmt {
	kw := p.expect(token.Try)
	s := &ast.TryStmt{Keyword: kw.Span}
	s.Body = p.parseBlock()
	if !p.at(token.Catch) {
		p.failExpected("catch（try のブロックの後ろには、catch (e) { … } が要る。docs/grammar.md「配線」）")
	}
	cstart := p.advance().Span.Start
	p.expect(token.LParen)
	s.Name = p.parseIdent()
	if p.got(token.By) {
		if p.atTypeWord("list") || p.atTypeWord("nullable") {
			p.fail(p.tok().Span, "catch (e by …) の分類は、例外の分類（classify … of any exception）。標準の分類は書けない（docs/grammar.md「配線」）")
		}
		name := p.parseQualifiedName()
		s.By = &name
	}
	p.expect(token.RParen)
	if s.By != nil {
		p.expect(token.LBrace)
		sawElse := false
		p.parseMembers(func() {
			arm := p.parseMatchArm()
			switch {
			case sawElse:
				head := source.Span{File: arm.Span.File, Start: arm.Span.Start, End: p.armHeadEnd(arm)}
				p.errorAt(head, "else のアームの後ろに、アームは書けない（else は、最後に一つだけ。docs/grammar.md「配線」）")
				return
			case arm.Else && len(s.Arms) == 0:
				p.errorAt(arm.ElseSpan, "else のアームは、ケースを名前で並べたアームの後ろに書く（else は「ほかのケースのすべて」。docs/grammar.md「配線」）")
			}
			sawElse = sawElse || arm.Else
			s.Arms = append(s.Arms, arm)
		})
		closing := p.expect(token.RBrace)
		if len(s.Arms) == 0 {
			p.errorAt(closing.Span, "catch (e by 分類) にはアームが一つ以上要る（docs/grammar.md「配線」）")
		}
	} else {
		bstart := p.expect(token.LBrace).Span.Start
		b := &ast.Block{Stmts: []ast.Stmt{}}
		p.parseMembers(func() {
			if p.at(token.Throw) {
				tstart := p.tok().Span.Start
				t := &ast.ThrowStmt{Throw: p.parseThrow()}
				p.expect(token.Semicolon)
				t.Span = p.spanFrom(tstart)
				if !p.at(token.RBrace) {
					p.fail(t.Span, "throw の文は、catch のブロックの最後にだけ書ける（下の文が、評価されない。docs/grammar.md「配線」）")
				}
				b.Stmts = append(b.Stmts, t)
				return
			}
			b.Stmts = append(b.Stmts, p.parseStmt(false)...)
		})
		p.expect(token.RBrace)
		b.Span = p.spanFrom(bstart)
		s.Catch = b
	}
	s.CatchSpan = p.spanFrom(cstart)
	p.end()
	s.Span = p.spanFrom(kw.Span.Start)
	return s
}

// parseMatchArm は MatchArm と ElseArm を読む。
//
//	MatchArm = Ident { "," Ident } "=>" ArmBody ";" ;
//	ElseArm  = "else" "=>" ArmBody ";" ;
//	ArmBody  = Call | Atom | Throw ;
//	Throw    = "throw" QualifiedName ;
//
// Path の直後が "(" なら Call、そうでなければ Atom。`default => …` はなくした書き方で、今の書き方を示して SYNTAX にし、
// else のアームとして読み進める。ケースの後ろの "|" も、なくした書き方（parseCaseList）。
func (p *parser) parseMatchArm() *ast.MatchArm {
	start := p.tok().Span.Start
	a := &ast.MatchArm{}
	switch {
	case p.at(token.Default):
		p.errorAt(p.tok().Span, "default のアームは書けない。受けるケースを名前で並べるか、最後のアームを else => … と書く（else は、並べなかったケースのすべてを受ける。docs/grammar.md「配線」）")
		a.Else, a.ElseSpan = true, p.advance().Span
	case p.at(token.Else):
		a.Else, a.ElseSpan = true, p.advance().Span
	default:
		a.Cases = p.parseCaseList("アームのケース", "A, B => …", "配線")
	}
	p.expect(token.Arrow)
	switch {
	case p.at(token.Throw):
		a.Body = p.parseThrow()
	case p.atName() && p.peek(1).Kind == token.LParen:
		a.Body = p.parseCall()
	case p.atName(), p.tok().Kind.IsLiteral(), p.at(token.None), p.at(token.LBrace):
		a.Body = p.parseAtom()
	case p.at(token.Match):
		p.fail(p.tok().Span, "アームの本体に match は書けない（呼び出し一つか、値一つか、throw 一つ。docs/grammar.md「配線」）")
	default:
		p.failExpected("アームの本体（呼び出しか値か throw）")
	}
	p.expect(token.Semicolon)
	a.Span = p.spanFrom(start)
	return a
}

// --- 識別子とリテラル ---

// parseIdent は Ident を読む。文脈キーワードも Ident。QualifiedName なら誤り。
func (p *parser) parseIdent() ast.Ident {
	if p.at(token.QualifiedName) {
		p.fail(p.tok().Span, fmt.Sprintf("ここには修飾しない識別子を書く: %s", p.tok().Text))
	}
	if p.tok().Kind.IsKeyword() {
		p.fail(p.tok().Span, fmt.Sprintf("%s は予約語で、識別子には使えない（docs/grammar.md「予約語」）", p.tok().Text))
	}
	t := p.expect(token.Ident)
	return ast.Ident{Span: t.Span, Name: t.Text}
}

// parseFieldName は FieldName = Ident | 予約語 を読む（docs/grammar.md「識別子」）。フィールド名を書く位置
// （model のフィールド、override の後ろの名前、レコードリテラルのキー）で使う。QualifiedName なら誤り。
func (p *parser) parseFieldName() ast.Ident {
	if p.tok().Kind.IsKeyword() {
		t := p.advance()
		return ast.Ident{Span: t.Span, Name: t.Text}
	}
	if p.at(token.QualifiedName) {
		p.fail(p.tok().Span, fmt.Sprintf("ここには修飾しない識別子を書く: %s", p.tok().Text))
	}
	if !p.at(token.Ident) {
		p.failExpected("フィールド名")
	}
	t := p.advance()
	return ast.Ident{Span: t.Span, Name: t.Text}
}

// parseQualifiedName は QualifiedName を読む。字句は Ident か QualifiedName で、"." で分けて各要素の Span を付ける。
// 字句解析器は二つ目以降の要素が予約語でもつなげる（lexName）ので、ここで誤りにする。
// 予約語を書けるのはフィールド名の位置だけ（parseFieldName、parsePath）。
func (p *parser) parseQualifiedName() ast.QualifiedName {
	if !p.atName() {
		if p.tok().Kind.IsKeyword() {
			p.fail(p.tok().Span, fmt.Sprintf("%s は予約語で、名前には使えない（docs/grammar.md「予約語」）", p.tok().Text))
		}
		p.failExpected("名前")
	}
	qn := splitName(p.tok(), p.file)
	for _, seg := range qn.Segments[1:] {
		if _, isKeyword := token.Keywords[seg.Name]; isKeyword {
			p.fail(seg.Span, fmt.Sprintf("%s は予約語で、名前には使えない（予約語を書けるのはフィールド名の位置だけ。docs/grammar.md「予約語」）", seg.Name))
		}
	}
	p.advance()
	return qn
}

// parseDecoratorName は DecoratorName = Ident | 予約語 を読む（docs/grammar.md「識別子」）。デコレータの宣言の名前の位置で使う。
// QualifiedName なら誤り。
func (p *parser) parseDecoratorName() ast.Ident {
	if p.tok().Kind.IsKeyword() {
		t := p.advance()
		return ast.Ident{Span: t.Span, Name: t.Text}
	}
	if p.at(token.QualifiedName) {
		p.fail(p.tok().Span, fmt.Sprintf("ここには修飾しない識別子を書く: %s", p.tok().Text))
	}
	if !p.at(token.Ident) {
		p.failExpected("デコレータの名前")
	}
	t := p.advance()
	return ast.Ident{Span: t.Span, Name: t.Text}
}

// parseDecoratorRef は DecoratorRef = [ QualifiedName "." ] DecoratorName を読む（docs/grammar.md「識別子」）。
// デコレータを指す位置（適用の "@" の後ろ、別名の "=" の後ろ）の名前で、最後の要素にだけ予約語を書ける
// （`@db.effects`、`@db.list`）。要素が一つで予約語のとき（`@effects`）は、その予約語の字句をそのまま名前として読む。
// 構文木の形は QualifiedName と同じ。
func (p *parser) parseDecoratorRef() ast.QualifiedName {
	if p.tok().Kind.IsKeyword() {
		t := p.advance()
		return ast.QualifiedName{Span: t.Span, Segments: []ast.Ident{{Span: t.Span, Name: t.Text}}}
	}
	if !p.atName() {
		p.failExpected("デコレータの名前")
	}
	qn := splitName(p.tok(), p.file)
	for i, seg := range qn.Segments {
		if i == 0 || i == len(qn.Segments)-1 {
			continue
		}
		if _, isKeyword := token.Keywords[seg.Name]; isKeyword {
			p.fail(seg.Span, fmt.Sprintf("%s は予約語で、名前には使えない（デコレータを指す名前で予約語を書けるのは、最後の要素だけ。docs/grammar.md「識別子」）", seg.Name))
		}
	}
	p.advance()
	return qn
}

// parsePath は Path = Ident { "." FieldName } を読む（docs/grammar.md「識別子」）。値を書く位置（Atom）の名前で、
// 先頭は束縛か引数か状態の名前（Ident）、残りはその値のフィールド名なので予約語も書ける（`res.op`）。
// 構文木の形は QualifiedName と同じ。
func (p *parser) parsePath() ast.QualifiedName {
	if !p.atName() {
		if p.tok().Kind.IsKeyword() {
			p.fail(p.tok().Span, fmt.Sprintf("%s は予約語で、名前には使えない（docs/grammar.md「予約語」）", p.tok().Text))
		}
		p.failExpected("名前")
	}
	return splitName(p.advance(), p.file)
}

// splitName は Ident か QualifiedName の字句を "." で分け、各要素の Span を付ける。予約語の要素もそのまま通す。
func splitName(t token.Token, file source.FileID) ast.QualifiedName {
	qn := ast.QualifiedName{Span: t.Span}
	off := t.Span.Start
	for _, seg := range strings.Split(t.Text, ".") {
		end := off + source.Offset(len(seg))
		qn.Segments = append(qn.Segments, ast.Ident{Span: source.Span{File: file, Start: off, End: end}, Name: seg})
		off = end + 1
	}
	return qn
}

// parseLiteral は Literal を読み、値に直す（docs/grammar.md「リテラル」）。
func (p *parser) parseLiteral() *ast.Literal {
	t := p.tok()
	lit := &ast.Literal{Span: t.Span, Raw: t.Text}
	switch t.Kind {
	case token.String:
		lit.Kind = ast.StringLit
		s, err := unquote(t.Text)
		if err != nil {
			p.fail(t.Span, err.Error())
		}
		lit.Str = s
	case token.Int:
		lit.Kind = ast.IntLit
		n, err := strconv.ParseInt(t.Text, 10, 64)
		if err != nil {
			p.failLiteral(t.Span, "整数が 64 ビットに収まらない: "+t.Text)
		}
		lit.Int = n
	case token.Float:
		lit.Kind = ast.FloatLit
		f, err := strconv.ParseFloat(t.Text, 64)
		if err != nil {
			p.fail(t.Span, "小数として読めない: "+t.Text)
		}
		lit.Float = f
	case token.True, token.False:
		lit.Kind = ast.BoolLit
		lit.Bool = t.Kind == token.True
	case token.Duration:
		lit.Kind = ast.DurationLit
		d, err := parseDuration(t.Text)
		if err != nil {
			p.failLiteral(t.Span, err.Error())
		}
		lit.Duration = d
	default:
		p.failExpected("リテラル")
	}
	p.advance()
	return lit
}
