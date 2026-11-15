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
	decorators, external := p.parseDecorators()
	if p.at(token.DocComment) {
		p.fail(p.tok().Span, "ドキュメンテーションコメントはデコレータの前に置く（設計書 §3.4）")
	}
	if p.at(token.Export) {
		export := p.advance().Span
		if !p.at(token.Op) {
			p.fail(export, "export を付けられるのは op だけで、op の直前に書く（docs/grammar.md「op」）")
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
	case token.Op:
		return p.parseOp(start, doc, decorators)
	case token.Exception:
		return p.parseException(start, doc, decorators)
	case token.Effect:
		return p.parseEffect(start, doc, decorators)
	case token.Classify:
		return p.parseClassify(start, doc, decorators)
	case token.Condition:
		return p.parseCondition(start, doc, decorators)
	case token.Decorator:
		return p.parseDecoratorDecl(start, doc, decorators, external)
	}
	if doc != nil && len(decorators) == 0 {
		p.fail(doc.Span, "付く先のないドキュメンテーションコメント（docs/grammar.md「決めたこと」）")
	}
	p.failExpected("宣言")
	return nil
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

// parseDecorators はデコレータの適用の列を読む（docs/grammar.md「デコレータの適用」）。
// "@external" は核の印なので列には入れず、その範囲を返す。直後が "decorator" でなければ誤り。
func (p *parser) parseDecorators() ([]*ast.Decorator, *source.Span) {
	var decorators []*ast.Decorator
	var external *source.Span
	for p.at(token.At) {
		start := p.advance().Span.Start
		name := p.parseQualifiedName()
		if name.String() == "external" {
			if p.at(token.LParen) {
				p.fail(p.tok().Span, "@external は引数を取らない")
			}
			if !p.at(token.Decorator) {
				p.fail(p.spanFrom(start), "@external はデコレータの宣言の直前にだけ書ける（docs/grammar.md「decorator の宣言」）")
			}
			s := p.spanFrom(start)
			external = &s
			continue
		}
		d := &ast.Decorator{Name: name}
		if p.at(token.LParen) {
			d.Args = p.parseArgs()
		}
		d.Span = p.spanFrom(start)
		decorators = append(decorators, d)
	}
	return decorators, external
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
	m.Body = p.parseModelBody()
	m.Span = p.spanFrom(start)
	return m
}

// parseModelBody は ModelBody = "{" { Member } "}" を読む。誤ったメンバーは読み飛ばし、読めたものを残す。
func (p *parser) parseModelBody() *ast.ModelBody {
	start := p.expect(token.LBrace).Span.Start
	body := &ast.ModelBody{Fields: []*ast.Field{}}
	p.parseMembers(func() {
		f, o := p.parseMember()
		if f != nil {
			body.Fields = append(body.Fields, f)
		}
		if o != nil {
			body.Overrides = append(body.Overrides, o)
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

// parseMember は Member = Field | Override を読む。どちらか一方を返す。
//
//	Field    = [ DocComment ] { Decorator } FieldName ":" Type [ "=" Literal ] ";" ;
//	Override = [ DocComment ] { Decorator } "override" FieldName [ ":" ( Type | "never" ) ] ";" ;
//
// フィールドの名前は FieldName なので予約語も書ける（`op: string;`。docs/grammar.md「識別子」）。
// メンバーの頭の override は、直後が ":" ならフィールドの名前、そうでなければ Override の頭。
func (p *parser) parseMember() (*ast.Field, *ast.Override) {
	doc := p.parseLeadingDoc()
	start := p.tok().Span.Start
	decorators, external := p.parseDecorators()
	if external != nil {
		// parseDecorators は直後の字句が decorator なら @external を通すが、ここでの decorator はフィールド名。
		p.fail(*external, "@external はデコレータの宣言の直前にだけ書ける（docs/grammar.md「decorator の宣言」）")
	}
	if doc != nil && p.at(token.RBrace) {
		p.fail(doc.Span, "付く先のないドキュメンテーションコメント（docs/grammar.md「決めたこと」）")
	}
	if p.at(token.Override) && p.peek(1).Kind != token.Colon {
		p.advance()
		o := &ast.Override{Doc: doc, Decorators: decorators, Name: p.parseFieldName()}
		if p.got(token.Colon) {
			if p.at(token.Never) {
				o.Never, o.NeverSpan = true, p.advance().Span
			} else {
				o.Type = p.parseType()
			}
		}
		if p.at(token.Assign) {
			p.fail(p.tok().Span, "override に既定値は書けない（docs/grammar.md「model」）")
		}
		p.expect(token.Semicolon)
		o.Span = p.spanFrom(start)
		return nil, o
	}
	f := &ast.Field{Doc: doc, Decorators: decorators, Name: p.parseFieldName()}
	p.expect(token.Colon)
	f.Type = p.parseType()
	if p.got(token.Assign) {
		f.Default = p.parseLiteral()
	}
	p.expect(token.Semicolon)
	f.Span = p.spanFrom(start)
	return f, nil
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
		a.Body = p.parseModelBody()
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

// parseException は ExceptionDecl = "exception" Ident [ ":" ArmType ] ";" を読む（docs/grammar.md「exception / effect」）。
func (p *parser) parseException(start source.Offset, doc *ast.DocComment, decorators []*ast.Decorator) *ast.ExceptionDecl {
	p.expect(token.Exception)
	e := &ast.ExceptionDecl{Doc: doc, Decorators: decorators, Name: p.parseIdent()}
	if p.got(token.Colon) {
		e.Value = p.parseArmType()
	}
	p.expect(token.Semicolon)
	e.Span = p.spanFrom(start)
	return e
}

// parseEffect は EffectDecl = "effect" Ident "on" ( BuiltinEffect | Transition ) ";" を読む（docs/grammar.md「exception / effect」）。
// QualifiedName の直後が ":" なら Transition、そうでなければ最後の要素が StateOp の BuiltinEffect。
func (p *parser) parseEffect(start source.Offset, doc *ast.DocComment, decorators []*ast.Decorator) *ast.EffectDecl {
	p.expect(token.Effect)
	e := &ast.EffectDecl{Doc: doc, Decorators: decorators, Name: p.parseIdent()}
	p.expect(token.On)
	if p.atName() && p.peek(1).Kind == token.Colon {
		e.Transition = p.parseTransition()
	} else {
		ref := p.parseStateOpRef()
		e.On = &ref
	}
	p.expect(token.Semicolon)
	e.Span = p.spanFrom(start)
	return e
}

// parseTransition は Transition = QualifiedName ":" CaseSet "->" CaseSet を読む。
func (p *parser) parseTransition() *ast.Transition {
	start := p.tok().Span.Start
	t := &ast.Transition{Of: p.parseQualifiedName()}
	p.expect(token.Colon)
	t.From = p.parseCaseSet()
	p.expect(token.ThinArrow)
	t.To = p.parseCaseSet()
	t.Span = p.spanFrom(start)
	return t
}

// parseCaseSet は CaseSet = "any" | Ident { "|" Ident } を読む。
func (p *parser) parseCaseSet() *ast.CaseSet {
	start := p.tok().Span.Start
	c := &ast.CaseSet{}
	if p.got(token.Any) {
		c.Any = true
	} else {
		if !p.atIdent() {
			p.failExpected("遷移のケース（any かケース名）")
		}
		c.Cases = append(c.Cases, p.parseIdent())
		for p.got(token.Pipe) {
			c.Cases = append(c.Cases, p.parseIdent())
		}
	}
	c.Span = p.spanFrom(start)
	return c
}

// parseStateOpRef は "状態.操作" を読む。字句では一つの QualifiedName なので、最後の要素を StateOp として切り出す
// （docs/grammar.md「exception / effect」）。
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

// parseClassify は ClassifyDecl = "classify" Ident "of" ClassifySubject "{" { ClassifyCase } "}" を読む（docs/grammar.md「classify / condition」）。
func (p *parser) parseClassify(start source.Offset, doc *ast.DocComment, decorators []*ast.Decorator) *ast.ClassifyDecl {
	p.expect(token.Classify)
	c := &ast.ClassifyDecl{Doc: doc, Decorators: decorators, Name: p.parseIdent(), Cases: []*ast.ClassifyCase{}}
	p.expect(token.Of)
	c.Of = p.parseClassifySubject()
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
//	ClassifyCase = [ DocComment ] ( Ident [ ":" CaseTyping { "," CaseTyping } ] | "default" ) ";" ;
//	CaseTyping   = Type "as" Type ;
func (p *parser) parseClassifyCase() *ast.ClassifyCase {
	c := &ast.ClassifyCase{Doc: p.parseLeadingDoc()}
	start := p.tok().Span.Start
	if c.Doc != nil && p.at(token.RBrace) {
		p.fail(c.Doc.Span, "付く先のないドキュメンテーションコメント（docs/grammar.md「決めたこと」）")
	}
	if p.got(token.Default) {
		c.Default = true
		if p.at(token.Colon) {
			p.fail(p.tok().Span, "default には as を書けない（docs/grammar.md「classify / condition」）")
		}
	} else {
		name := p.parseIdent()
		c.Name = &name
		if p.got(token.Colon) {
			c.As = append(c.As, p.parseCaseTyping())
			for p.got(token.Comma) {
				c.As = append(c.As, p.parseCaseTyping())
			}
		}
	}
	p.expect(token.Semicolon)
	c.Span = p.spanFrom(start)
	return c
}

// parseCaseTyping は CaseTyping = Type "as" Type を読む。as の前は対象（宣言の of の後ろと同じに書く）。
func (p *parser) parseCaseTyping() *ast.CaseTyping {
	start := p.tok().Span.Start
	t := &ast.CaseTyping{Subject: p.parseType()}
	p.expect(token.As)
	t.Type = p.parseType()
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

// parseCondition は ConditionDecl = "condition" Ident "of" ClassifySubject ";" を読む。
func (p *parser) parseCondition(start source.Offset, doc *ast.DocComment, decorators []*ast.Decorator) *ast.ConditionDecl {
	p.expect(token.Condition)
	c := &ast.ConditionDecl{Doc: doc, Decorators: decorators, Name: p.parseIdent()}
	p.expect(token.Of)
	c.Of = p.parseClassifySubject()
	if p.at(token.Colon) || p.at(token.As) {
		p.fail(p.tok().Span, "condition には as を書けない（docs/grammar.md「classify / condition」）")
	}
	p.expect(token.Semicolon)
	c.Span = p.spanFrom(start)
	return c
}

// parseDecoratorDecl は DecoratorDecl を読む（docs/grammar.md「decorator の宣言」）。
//
//	DecoratorDecl = [ "@external" ] "decorator" Ident [ Params ] [ "for" Target { "|" Target } ] { DecoratorClause } ";"
//	              | "decorator" Ident "=" QualifiedName Args ";" ;
func (p *parser) parseDecoratorDecl(start source.Offset, doc *ast.DocComment, decorators []*ast.Decorator, external *source.Span) *ast.DecoratorDecl {
	p.expect(token.Decorator)
	d := &ast.DecoratorDecl{Doc: doc, Decorators: decorators, Name: p.parseIdent()}
	if external != nil {
		d.External = true
		d.ExternalSpan = *external
	}
	if p.at(token.Assign) {
		if external != nil {
			p.errorAt(*external, "名前付きのデコレータに @external は付けられない（docs/grammar.md「decorator の宣言」）")
		}
		aliasStart := p.advance().Span.Start
		alias := &ast.DecoratorAlias{Name: p.parseQualifiedName()}
		alias.Args = p.parseArgs()
		alias.Span = p.spanFrom(aliasStart)
		d.Alias = alias
		p.expect(token.Semicolon)
		d.Span = p.spanFrom(start)
		return d
	}
	if p.at(token.LParen) {
		d.Params = p.parseParams()
	}
	if p.got(token.For) {
		d.For = append(d.For, p.parseTarget())
		for p.got(token.Pipe) {
			d.For = append(d.For, p.parseTarget())
		}
	}
	for p.at(token.Throws) || p.at(token.Catches) {
		if p.at(token.Throws) {
			d.Clauses = append(d.Clauses, p.parseThrowsClause())
		} else {
			d.Clauses = append(d.Clauses, p.parseCatchesClause())
		}
	}
	p.expect(token.Semicolon)
	d.Span = p.spanFrom(start)
	return d
}

// parseParams は Params = "(" [ Param { "," Param } ] ")" を読む。
func (p *parser) parseParams() []*ast.Param {
	p.expect(token.LParen)
	params := []*ast.Param{}
	for !p.at(token.RParen) {
		start := p.tok().Span.Start
		param := &ast.Param{Name: p.parseIdent()}
		p.expect(token.Colon)
		param.Type = p.parseParamType()
		param.Span = p.spanFrom(start)
		params = append(params, param)
		if !p.got(token.Comma) {
			break
		}
	}
	p.expect(token.RParen)
	return params
}

// parseParamType は ParamType = AnyType [ "?" ] | Type { "|" Type } を読む。
func (p *parser) parseParamType() *ast.ParamType {
	start := p.tok().Span.Start
	t := &ast.ParamType{}
	if p.at(token.Any) {
		t.Any = p.parseAnyType()
		t.Optional = p.got(token.Question)
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
	case p.atContextual("variant"):
		a.Kind = ast.AnyVariant
	default:
		p.failExpected("any の種類（exception / effect / state / op / model / variant）")
	}
	p.advance()
	if p.got(token.On) {
		ref := p.parseStateOpRef()
		a.On = &ref
	}
	a.Span = p.spanFrom(start)
	return a
}

// parseTarget は Target = "op" | "field" | "state" | "exception" | "scalar" | "model" を読む。field は文脈キーワード。
func (p *parser) parseTarget() *ast.Target {
	t := &ast.Target{Span: p.tok().Span}
	switch {
	case p.at(token.Op):
		t.Kind = ast.TargetOp
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
	default:
		p.failExpected("for の対象（op / field / state / exception / scalar / model）")
	}
	p.advance()
	return t
}

// parseThrowsClause は ThrowsClause = "throws" ExcRef { "|" ExcRef } { Relation } を読む。
func (p *parser) parseThrowsClause() *ast.ThrowsClause {
	start := p.expect(token.Throws).Span.Start
	c := &ast.ThrowsClause{Exceptions: p.parseNameList()}
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

// parseCatchesClause は CatchesClause = "catches" ExcRef { "|" ExcRef } ( "recovers" | "halts" QualifiedName | "ignores" ) を読む。
func (p *parser) parseCatchesClause() *ast.CatchesClause {
	start := p.expect(token.Catches).Span.Start
	c := &ast.CatchesClause{Exceptions: p.parseNameList()}
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

// parseNameList は QualifiedName { "|" QualifiedName } を読む。
func (p *parser) parseNameList() []ast.QualifiedName {
	names := []ast.QualifiedName{p.parseQualifiedName()}
	for p.got(token.Pipe) {
		names = append(names, p.parseQualifiedName())
	}
	return names
}

// parseOp は OpDecl を読む（docs/grammar.md「op」）。
//
//	OpDecl = [ "export" ] "op" Ident OpParams [ ":" ReturnType ] { OpClause } ( ";" | Block ) ;
//
// export は parseDecl が読む。
func (p *parser) parseOp(start source.Offset, doc *ast.DocComment, decorators []*ast.Decorator) *ast.OpDecl {
	p.expect(token.Op)
	o := &ast.OpDecl{Doc: doc, Decorators: decorators, Name: p.parseIdent()}
	o.Params = p.parseOpParams()
	if p.got(token.Colon) {
		o.Return = p.parseReturnType()
	}
	for {
		switch p.tok().Kind {
		case token.Effects:
			s := p.advance().Span.Start
			c := &ast.EffectsClause{Effects: p.parseNameList()}
			c.Span = p.spanFrom(s)
			o.Clauses = append(o.Clauses, c)
			continue
		case token.Reads:
			o.Clauses = append(o.Clauses, p.parseReadsClause())
			continue
		case token.Throws:
			o.Clauses = append(o.Clauses, p.parseThrowsClause())
			continue
		case token.Catches:
			p.fail(p.tok().Span, "catches はデコレータの宣言にだけ書ける（docs/grammar.md「op」）")
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

// parseReadsClause は "reads" QualifiedName { "," QualifiedName } を読む（docs/grammar.md「op」）。区切りは ","。
func (p *parser) parseReadsClause() *ast.ReadsClause {
	start := p.expect(token.Reads).Span.Start
	c := &ast.ReadsClause{States: []ast.QualifiedName{p.parseQualifiedName()}}
	for p.got(token.Comma) {
		c.States = append(c.States, p.parseQualifiedName())
	}
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
	if p.at(token.Void) {
		return &ast.ReturnType{Span: p.advance().Span, Void: true}
	}
	if p.at(token.Never) {
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
	switch p.tok().Kind {
	case token.StringType, token.BooleanType, token.IntType, token.FloatType, token.TimeType, token.DurationType:
		tok := p.advance()
		t = &ast.CoreType{Span: tok.Span, Name: tok.Text}
	case token.Record:
		p.advance()
		p.expect(token.Less)
		r := &ast.RecordType{Key: p.parseType()}
		p.expect(token.Comma)
		r.Value = p.parseType()
		p.expect(token.Greater)
		r.Span = p.spanFrom(start)
		t = r
	case token.Ident, token.QualifiedName:
		n := &ast.NamedType{Name: p.parseQualifiedName()}
		if p.got(token.Less) {
			n.Arg = p.parseType()
			p.expect(token.Greater)
		}
		n.Span = p.spanFrom(start)
		t = n
	case token.List, token.Nullable, token.Single:
		shape := p.advance()
		p.expect(token.Less)
		st := &ast.ShapeType{Shape: shape.Text, ShapeSpan: shape.Span, Elem: p.parseType()}
		p.expect(token.Greater)
		st.Span = p.spanFrom(start)
		t = st
	case token.Void:
		p.fail(p.tok().Span, "void は op の戻り値の型にだけ書ける（docs/grammar.md「型」）")
	case token.Never:
		p.fail(p.tok().Span, "never は op の戻り値の型と、override のなくす印にだけ書ける（docs/grammar.md「型」）")
	case token.LBrace:
		p.fail(p.tok().Span, "その場で書く model はユニオンのアームと例外の値にだけ置ける（docs/grammar.md「決めたこと」）")
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
		default:
			return t
		}
	}
}

// --- 配線（docs/grammar.md「配線」） ---

// parseBlock は Block = "{" { Stmt } "}" を読む。誤った文は次の ";" まで読み飛ばし、読めた文を残す。
func (p *parser) parseBlock() *ast.Block {
	start := p.expect(token.LBrace).Span.Start
	b := &ast.Block{Stmts: []ast.Stmt{}}
	p.parseMembers(func() {
		b.Stmts = append(b.Stmts, p.parseStmt())
	})
	p.expect(token.RBrace)
	b.Span = p.spanFrom(start)
	return b
}

// parseStmt は Stmt = "const" Ident "=" Expr ";" | "return" Expr ";" | Call ";" | Match ";" を読む。
func (p *parser) parseStmt() ast.Stmt {
	start := p.tok().Span.Start
	switch p.tok().Kind {
	case token.Const:
		p.advance()
		s := &ast.ConstStmt{Name: p.parseIdent()}
		p.expect(token.Assign)
		s.Value = p.parseExpr()
		p.expect(token.Semicolon)
		s.Span = p.spanFrom(start)
		return s
	case token.Return:
		p.advance()
		s := &ast.ReturnStmt{Value: p.parseExpr()}
		p.expect(token.Semicolon)
		s.Span = p.spanFrom(start)
		return s
	case token.Match:
		s := &ast.MatchStmt{Match: p.parseMatch()}
		p.expect(token.Semicolon)
		s.Span = p.spanFrom(start)
		return s
	case token.Ident, token.QualifiedName:
		s := &ast.CallStmt{Call: p.parseCall()}
		p.expect(token.Semicolon)
		s.Span = p.spanFrom(start)
		return s
	case token.DocComment:
		p.fail(p.tok().Span, "付く先のないドキュメンテーションコメント（docs/grammar.md「決めたこと」）")
	case token.Throw:
		p.fail(p.tok().Span, "throw は match のアームの本体にだけ書ける（docs/grammar.md「配線」）")
	}
	p.failExpected("文（const / return / 呼び出し / match）")
	return nil
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
		p.fail(p.tok().Span, "throw は match のアームの本体にだけ書ける（docs/grammar.md「配線」）")
	}
	return p.parseAtom()
}

// parseCall は Call = QualifiedName "(" [ Atom { "," Atom } ] ")" を読む。引数は Atom だけ（設計書 §9「A 正規形」）。
func (p *parser) parseCall() *ast.Call {
	start := p.tok().Span.Start
	c := &ast.Call{Callee: p.parseQualifiedName(), Args: []ast.Atom{}}
	p.expect(token.LParen)
	for !p.at(token.RParen) {
		if p.atName() && p.peek(1).Kind == token.LParen {
			p.fail(p.tok().Span, "呼び出しの引数に呼び出しは書けない。中間結果に const で名前を付ける（設計書 §9）")
		}
		c.Args = append(c.Args, p.parseAtom())
		if !p.got(token.Comma) {
			break
		}
	}
	p.expect(token.RParen)
	c.Span = p.spanFrom(start)
	return c
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

// parseMatch は Match = "match" [ Atom ] "by" ( QualifiedName | "list" | "nullable" ) "{" MatchArm { MatchArm } "}" を読む。
// 対象を書かない形（match by C）は状態の分類、by list / by nullable は標準の分類。
// どの形がどの分類に合うかは検査（MATCH-7）が決める。
func (p *parser) parseMatch() *ast.Match {
	start := p.expect(token.Match).Span.Start
	m := &ast.Match{}
	if !p.at(token.By) {
		m.Subject = p.parseAtom()
	}
	p.expect(token.By)
	switch p.tok().Kind {
	case token.List, token.Nullable:
		t := p.advance()
		m.Classifier = ast.QualifiedName{Span: t.Span, Segments: []ast.Ident{{Span: t.Span, Name: t.Text}}}
		m.Builtin = true
	case token.Single:
		p.fail(p.tok().Span, "single は標準の分類ではないので、match の by には書けない（標準の分類は list と nullable。docs/check.md「標準の分類」）")
	default:
		m.Classifier = p.parseQualifiedName()
	}
	p.expect(token.LBrace)
	p.parseMembers(func() {
		m.Arms = append(m.Arms, p.parseMatchArm())
	})
	closing := p.expect(token.RBrace)
	if len(m.Arms) == 0 {
		p.errorAt(closing.Span, "match にはアームが一つ以上要る（docs/grammar.md「配線」）")
	}
	m.Span = p.spanFrom(start)
	return m
}

// parseMatchArm は MatchArm = ( Ident { "|" Ident } | "default" ) "=>" ArmBody ";" を読む。
//
//	ArmBody = Call | Atom | Throw ;
//	Throw   = "throw" QualifiedName ;
//
// Path の直後が "(" なら Call、そうでなければ Atom。
func (p *parser) parseMatchArm() *ast.MatchArm {
	start := p.tok().Span.Start
	a := &ast.MatchArm{}
	if p.got(token.Default) {
		a.Default = true
	} else {
		a.Cases = append(a.Cases, p.parseIdent())
		for p.got(token.Pipe) {
			a.Cases = append(a.Cases, p.parseIdent())
		}
	}
	p.expect(token.Arrow)
	switch {
	case p.at(token.Throw):
		tstart := p.advance().Span.Start
		t := &ast.Throw{Exception: p.parseQualifiedName()}
		t.Span = p.spanFrom(tstart)
		a.Body = t
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
