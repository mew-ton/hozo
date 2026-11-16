package lsp

import (
	"strings"

	"github.com/mew-ton/hozo/internal/sema"
	"github.com/mew-ton/hozo/internal/source"
	"github.com/mew-ton/hozo/internal/syntax"
	"github.com/mew-ton/hozo/internal/syntax/ast"
)

// declEntry は宣言（とその中身）の名前のトークンの位置。宣言の名前の上でのホバーと、名前空間のブロックの位置に使う。
// sema の Ref は参照の側だけを持つ（docs/packages.md「internal/sema」）ので、宣言の側は構文木から引く。
type declEntry struct {
	// span は名前のトークン。
	span source.Span
	// target は sema.Ref と同じ形の解決先（Span は span）。
	target sema.Target
}

// fileAST は FileID の構文木。解析ごとに一度だけ構文解析し直す（Project は構文木を持たない）。
func (s *server) fileAST(id source.FileID) *ast.File {
	if f, ok := s.asts[id]; ok {
		return f
	}
	f, _ := syntax.Parse(s.proj.Files, id) // 診断は Project.Diags に出ている
	s.asts[id] = f
	return f
}

// declIndex は FileID の宣言の名前の索引。
func (s *server) declIndex(id source.FileID) []declEntry {
	if e, ok := s.declIdx[id]; ok {
		return e
	}
	var entries []declEntry
	add := func(span source.Span, t sema.Target) {
		t.Span = span
		entries = append(entries, declEntry{span: span, target: t})
	}
	// docTags はドキュメンテーションコメントの @param / @throws / @see の名前（docs/lsp.md「定義へジャンプ」の表）を
	// 取り置く。sema の Ref には入らない（docs.go は resolveName で解決し、Ref を残さない）ので、ここで構文木と IR から
	// 引く。@throws / @see の解決は宣言の索引が要るので、索引を納めた後に docRefIndex が行う。
	var tags []pendingTag
	docTags := func(doc *ast.DocComment, ns, owner string, params map[string]source.Span, paramKind sema.TargetKind) {
		if doc == nil {
			return
		}
		for i := range doc.Tags {
			tag := &doc.Tags[i]
			switch tag.Kind {
			case ast.DocParam:
				if sp, ok := params[tag.Name.String()]; ok {
					tags = append(tags, pendingTag{span: tag.Name.Span, target: &sema.Target{Kind: paramKind, Decl: owner, Name: tag.Name.String(), Span: sp}})
				}
			case ast.DocThrows, ast.DocSee:
				tags = append(tags, pendingTag{span: tag.Name.Span, ns: ns, name: tag.Name.String()})
			}
		}
	}
	var walk func(decls []ast.Decl, ns string)
	walk = func(decls []ast.Decl, ns string) {
		for _, d := range decls {
			switch d := d.(type) {
			case *ast.NamespaceDecl:
				fqn := joinNS(ns, d.Name.String())
				// 各セグメントは、そこまでの名前空間
				prefix := ns
				for _, seg := range d.Name.Segments {
					prefix = joinNS(prefix, seg.Name)
					add(seg.Span, sema.Target{Kind: sema.TargetNamespace, Decl: prefix})
				}
				walk(d.Decls, fqn)
			case *ast.BindDecl:
				// bind は宣言ではない（名前は状態への参照で、Refs にある）
			case *ast.ModelDecl:
				docTags(d.Doc, ns, "", nil, sema.TargetDecl)
				fqn := joinNS(ns, d.Name.Name)
				add(d.Name.Span, sema.Target{Kind: sema.TargetDecl, Decl: fqn})
				if d.Body != nil {
					for _, f := range d.Body.Fields {
						add(f.Name.Span, sema.Target{Kind: sema.TargetField, Decl: fqn, Name: f.Name.Name})
					}
				}
			case *ast.ScalarDecl:
				docTags(d.Doc, ns, "", nil, sema.TargetDecl)
				add(d.Name.Span, sema.Target{Kind: sema.TargetDecl, Decl: joinNS(ns, d.Name.Name)})
			case *ast.UnionDecl:
				docTags(d.Doc, ns, "", nil, sema.TargetDecl)
				fqn := joinNS(ns, d.Name.Name)
				add(d.Name.Span, sema.Target{Kind: sema.TargetDecl, Decl: fqn})
				for _, m := range d.Members {
					if m.Arm != nil {
						add(m.Arm.Span, sema.Target{Kind: sema.TargetArm, Decl: fqn, Name: m.Arm.Name})
						if m.Type != nil && m.Type.Body != nil {
							// その場で書いた model のフィールド（docs/ir.md「正規化」: <ユニオン>.<アーム> の Model）
							inline := fqn + "." + m.Arm.Name
							for _, f := range m.Type.Body.Fields {
								add(f.Name.Span, sema.Target{Kind: sema.TargetField, Decl: inline, Name: f.Name.Name})
							}
						}
					}
				}
			case *ast.StateDecl:
				docTags(d.Doc, ns, "", nil, sema.TargetDecl)
				add(d.Name.Span, sema.Target{Kind: sema.TargetDecl, Decl: joinNS(ns, d.Name.Name)})
			case *ast.ExceptionDecl:
				docTags(d.Doc, ns, "", nil, sema.TargetDecl)
				fqn := joinNS(ns, d.Name.Name)
				add(d.Name.Span, sema.Target{Kind: sema.TargetDecl, Decl: fqn})
				if d.Value != nil && d.Value.Body != nil {
					for _, f := range d.Value.Body.Fields {
						add(f.Name.Span, sema.Target{Kind: sema.TargetField, Decl: fqn + ".Value", Name: f.Name.Name})
					}
				}
			case *ast.EffectDecl:
				docTags(d.Doc, ns, "", nil, sema.TargetDecl)
				add(d.Name.Span, sema.Target{Kind: sema.TargetDecl, Decl: joinNS(ns, d.Name.Name)})
			case *ast.ReadDecl:
				docTags(d.Doc, ns, "", nil, sema.TargetDecl)
				add(d.Name.Span, sema.Target{Kind: sema.TargetDecl, Decl: joinNS(ns, d.Name.Name)})
			case *ast.ClassifyDecl:
				docTags(d.Doc, ns, "", nil, sema.TargetDecl)
				fqn := joinNS(ns, d.Name.Name)
				add(d.Name.Span, sema.Target{Kind: sema.TargetDecl, Decl: fqn})
				for _, c := range d.Cases {
					if c.Name != nil {
						add(c.Name.Span, sema.Target{Kind: sema.TargetCase, Decl: fqn, Name: c.Name.Name})
					}
				}
			case *ast.ConditionDecl:
				docTags(d.Doc, ns, "", nil, sema.TargetDecl)
				fqn := joinNS(ns, d.Name.Name)
				add(d.Name.Span, sema.Target{Kind: sema.TargetDecl, Decl: fqn})
			case *ast.DecoratorDecl:
				fqn := joinNS(ns, d.Name.Name)
				add(d.Name.Span, sema.Target{Kind: sema.TargetDecl, Decl: fqn})
				params := map[string]source.Span{}
				for _, p := range d.Params {
					add(p.Name.Span, sema.Target{Kind: sema.TargetDecoratorParam, Decl: fqn, Name: p.Name.Name})
					params[p.Name.Name] = p.Name.Span
				}
				docTags(d.Doc, ns, fqn, params, sema.TargetDecoratorParam)
			case *ast.OpDecl:
				fqn := joinNS(ns, d.Name.Name)
				add(d.Name.Span, sema.Target{Kind: sema.TargetDecl, Decl: fqn})
				params := map[string]source.Span{}
				for _, p := range d.Params {
					add(p.Name.Span, sema.Target{Kind: sema.TargetOpParam, Decl: fqn, Name: p.Name.Name})
					params[p.Name.Name] = p.Name.Span
				}
				docTags(d.Doc, ns, fqn, params, sema.TargetOpParam)
				if d.Body != nil {
					for _, st := range d.Body.Stmts {
						if c, ok := st.(*ast.ConstStmt); ok {
							add(c.Name.Span, sema.Target{Kind: sema.TargetBinding, Decl: fqn, Name: c.Name.Name})
						}
					}
				}
			}
		}
	}
	if f := s.fileAST(id); f != nil {
		walk(f.Decls, "")
	}
	s.declIdx[id] = entries
	s.pendingTags[id] = tags
	return entries
}

// pendingTag は docRefIndex が解決するドキュメンテーションコメントのタグ。target があれば解決済み（@param）。
type pendingTag struct {
	span   source.Span
	target *sema.Target
	ns     string
	name   string
}

// docRefIndex は FileID のドキュメンテーションコメントの @param / @throws / @see の名前から解決先への対応。
func (s *server) docRefIndex(id source.FileID) []declEntry {
	if e, ok := s.docRefIdx[id]; ok {
		return e
	}
	s.declIndex(id)
	var entries []declEntry
	for _, tag := range s.pendingTags[id] {
		if tag.target != nil {
			entries = append(entries, declEntry{span: tag.span, target: *tag.target})
			continue
		}
		if fqn, sp, ok := s.resolveDocName(tag.ns, tag.name); ok {
			entries = append(entries, declEntry{span: tag.span, target: sema.Target{Kind: sema.TargetDecl, Decl: fqn, Span: sp}})
		}
	}
	s.docRefIdx[id] = entries
	return entries
}

// joinNS は名前空間の FQN に名前をつなぐ。グローバル（""）ならそのまま。
func joinNS(ns, name string) string {
	if ns == "" {
		return name
	}
	return ns + "." + name
}

// contains は off が範囲の中か。終わりの位置（トークンの直後）も含める（カーソルは文字の間にあるため）。
func contains(sp source.Span, off source.Offset) bool {
	return sp.Start <= off && off <= sp.End
}

// targetAt はカーソル位置のトークンの解決先。参照（sema の Ref。docs/lsp.md「定義へジャンプ」）を先に、
// 次にドキュメンテーションコメントのタグ（docRefIndex）、なければ宣言の名前（declIndex）を見る。同じ位置に複数あれば最も短い範囲のもの。
func (s *server) targetAt(id source.FileID, off source.Offset) (sema.Target, source.Span, bool) {
	var best *sema.Ref
	consider := func(r sema.Ref) {
		if r.Span.File != id || !contains(r.Span, off) {
			return
		}
		// 終わりの位置だけで触れているものより、中にあるものを優先
		if best != nil {
			bestInside := off < best.Span.End
			inside := off < r.Span.End
			if bestInside && !inside {
				return
			}
			if bestInside == inside && r.Span.End-r.Span.Start >= best.Span.End-best.Span.Start {
				return
			}
		}
		rr := r
		best = &rr
	}
	if s.proj.Sema != nil {
		for _, r := range s.proj.Sema.Refs {
			consider(r)
		}
	}
	if best == nil {
		for _, e := range s.docRefIndex(id) {
			consider(sema.Ref{Span: e.span, Target: e.target})
		}
	}
	if best == nil {
		for _, e := range s.declIndex(id) {
			consider(sema.Ref{Span: e.span, Target: e.target})
		}
	}
	if best == nil {
		return sema.Target{}, source.Span{}, false
	}
	return best.Target, best.Span, true
}

// namespaceAt はカーソルを囲む名前空間の FQN（ホバーの「見ている名前空間」。docs/lsp.md「ホバー」）。
// 名前空間の外ならグローバル（""）。
func (s *server) namespaceAt(id source.FileID, off source.Offset) string {
	var find func(decls []ast.Decl, ns string) string
	find = func(decls []ast.Decl, ns string) string {
		for _, d := range decls {
			if n, ok := d.(*ast.NamespaceDecl); ok && contains(n.Span, off) {
				return find(n.Decls, joinNS(ns, n.Name.String()))
			}
		}
		return ns
	}
	f := s.fileAST(id)
	if f == nil {
		return ""
	}
	return find(f.Decls, "")
}

// namespaceBlock は名前空間 fqn の最初のブロックの名前の位置（docs/lsp.md「定義へジャンプ」の
// 「名前空間なら、その名前空間の最初のブロックの名前」）。ファイルの順（docs/ir.md「原則」）に探す。
// `namespace a.b { }` は a と a.b のどちらのブロックでもあり、a なら a のセグメントを返す。
func (s *server) namespaceBlock(fqn string) (source.Span, bool) {
	var find func(decls []ast.Decl, ns string) (source.Span, bool)
	find = func(decls []ast.Decl, ns string) (source.Span, bool) {
		for _, d := range decls {
			n, ok := d.(*ast.NamespaceDecl)
			if !ok {
				continue
			}
			prefix := ns
			for _, seg := range n.Name.Segments {
				prefix = joinNS(prefix, seg.Name)
				if prefix == fqn {
					return seg.Span, true
				}
			}
			if strings.HasPrefix(fqn, prefix+".") {
				if sp, ok := find(n.Decls, prefix); ok {
					return sp, true
				}
			}
		}
		return source.Span{}, false
	}
	for i := 0; i < s.proj.Files.Len(); i++ {
		id := source.FileID(i)
		f := s.fileAST(id)
		if f == nil {
			continue
		}
		if sp, ok := find(f.Decls, ""); ok {
			return sp, true
		}
	}
	return source.Span{}, false
}

// namespaceIndex は宣言の FQN → 宣言した名前空間の FQN（ホバーの修飾に使う）。
func namespaceIndex(res *sema.Result) map[string]string {
	idx := map[string]string{}
	if res == nil || res.Program == nil {
		return idx
	}
	for i := range res.Program.Namespaces {
		ns := &res.Program.Namespaces[i]
		for _, d := range ns.Decls {
			if d != nil {
				idx[d.Common().Name] = ns.Name
			}
		}
	}
	return idx
}

// resolveDocName はドキュメンテーションコメントの @throws / @see の名前を、名前空間 ns から見て宣言の FQN と
// 名前の位置にする。修飾なしなら ns の宣言、次にグローバル（docs/resolve.md「手続き」の 1.5 / 1.6）。修飾ありなら
// FQN そのもの、次に ns の下。
func (s *server) resolveDocName(ns, name string) (string, source.Span, bool) {
	candidates := []string{joinNS(ns, name), name}
	if strings.Contains(name, ".") {
		candidates = []string{name, joinNS(ns, name)}
	}
	for _, fqn := range candidates {
		if s.proj.Sema == nil || s.proj.Sema.Program.Lookup(fqn) == nil {
			continue
		}
		if sp, ok := s.declSpan(fqn); ok {
			return fqn, sp, true
		}
	}
	return "", source.Span{}, false
}

// declSpan は宣言 fqn の名前のトークンの位置。IR の Span（行・列）ではなくバイトの範囲が要るので、
// 宣言のあるファイルの索引（declIndex）から引く。
func (s *server) declSpan(fqn string) (source.Span, bool) {
	d := s.proj.Sema.Program.Lookup(fqn)
	if d == nil {
		return source.Span{}, false
	}
	id, ok := s.fileIDs[d.Common().Span.File]
	if !ok {
		return source.Span{}, false
	}
	for _, e := range s.declIndex(id) {
		if e.target.Kind == sema.TargetDecl && e.target.Decl == fqn {
			return e.span, true
		}
	}
	return source.Span{}, false
}
