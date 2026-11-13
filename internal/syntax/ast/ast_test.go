package ast

import (
	"fmt"
	"testing"

	"github.com/mew-ton/hozo/internal/source"
)

func TestQualifiedName(t *testing.T) {
	qn := &QualifiedName{Segments: []Ident{{Name: "server"}, {Name: "admin"}, {Name: "User"}}}
	if qn.String() != "server.admin.User" {
		t.Errorf("String = %q", qn.String())
	}
	if qn.Last().Name != "User" {
		t.Errorf("Last = %+v", qn.Last())
	}
	single := &QualifiedName{Segments: []Ident{{Name: "db"}}}
	if single.String() != "db" || single.Last().Name != "db" {
		t.Errorf("single = %q", single.String())
	}
}

func TestEnumStrings(t *testing.T) {
	tests := []struct {
		v    fmt.Stringer
		want string
	}{
		{StateList, "list"},
		{StateSingle, "single"},
		{Post, "post"},
		{Put, "put"},
		{Delete, "delete"},
		{AnyException, "exception"},
		{AnyEffect, "effect"},
		{AnyState, "state"},
		{AnyOp, "op"},
		{AnyModel, "model"},
		{AnyVariant, "variant"},
		{TargetOp, "op"},
		{TargetField, "field"},
		{TargetState, "state"},
		{TargetException, "exception"},
		{TargetScalar, "scalar"},
		{TargetModel, "model"},
		{Prevents, "prevents"},
		{Keeps, "keeps"},
		{Recovers, "recovers"},
		{Halts, "halts"},
		{Ignores, "ignores"},
		{DocParam, "@param"},
		{DocThrows, "@throws"},
		{DocSee, "@see"},
		{StateOp(99), "?"},
		{AnyKind(99), "?"},
		{TargetKind(99), "?"},
		{CatchOutcome(99), "?"},
		{DocTagKind(99), "@?"},
	}
	for _, tt := range tests {
		if got := tt.v.String(); got != tt.want {
			t.Errorf("%T(%v).String() = %q, want %q", tt.v, tt.v, got, tt.want)
		}
	}
}

// TestNodeSpan はすべての節が Node で、NodeSpan が Span を返すことを確かめる。
func TestNodeSpan(t *testing.T) {
	s := source.Span{File: 1, Start: 2, End: 3}
	nodes := []Node{
		&Ident{Span: s}, &QualifiedName{Span: s}, &DocComment{Span: s}, &DocTag{Span: s}, &Literal{Span: s},
		&File{Span: s}, &Decorator{Span: s}, &Arg{Span: s}, &ArgValue{Span: s}, &ArgTerm{Span: s},
		&NamespaceDecl{Span: s}, &BindDecl{Span: s}, &ModelDecl{Span: s}, &ModelBody{Span: s}, &Field{Span: s},
		&ScalarDecl{Span: s}, &UnionDecl{Span: s}, &UnionMember{Span: s}, &ArmType{Span: s}, &StateDecl{Span: s},
		&ExceptionDecl{Span: s}, &StateOpRef{Span: s}, &EffectDecl{Span: s}, &ClassifyDecl{Span: s}, &ClassifyCase{Span: s},
		&ConditionDecl{Span: s}, &DecoratorDecl{Span: s}, &DecoratorAlias{Span: s}, &Param{Span: s}, &ParamType{Span: s},
		&AnyType{Span: s}, &Target{Span: s}, &ThrowsClause{Span: s}, &Relation{Span: s},
		&CatchesClause{Span: s}, &EffectsClause{Span: s}, &OpDecl{Span: s}, &OpParam{Span: s}, &ReturnType{Span: s},
		&CoreType{Span: s}, &RecordType{Span: s}, &NamedType{Span: s}, &ArrayType{Span: s}, &OptionalType{Span: s},
		&Block{Span: s}, &ConstStmt{Span: s}, &ReturnStmt{Span: s}, &CallStmt{Span: s}, &MatchStmt{Span: s},
		&Call{Span: s}, &NameAtom{Span: s}, &NoneAtom{Span: s}, &RecordLit{Span: s}, &RecordField{Span: s},
		&Match{Span: s}, &MatchArm{Span: s},
	}
	for _, n := range nodes {
		if n.NodeSpan() != s {
			t.Errorf("%T.NodeSpan() = %+v", n, n.NodeSpan())
		}
	}
	// 各 interface の実装。
	var _ Decl = (*NamespaceDecl)(nil)
	var _ Decl = (*BindDecl)(nil)
	var _ Decl = (*ModelDecl)(nil)
	var _ Decl = (*DecoratorDecl)(nil)
	var _ Clause = (*ThrowsClause)(nil)
	var _ Clause = (*CatchesClause)(nil)
	var _ Clause = (*EffectsClause)(nil)
	var _ Type = (*OptionalType)(nil)
	var _ Stmt = (*MatchStmt)(nil)
	var _ Expr = (*Match)(nil)
	var _ Atom = (*Literal)(nil)
	var _ Atom = (*RecordLit)(nil)
}
