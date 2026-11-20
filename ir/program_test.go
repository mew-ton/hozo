package ir

import "testing"

func TestLookup(t *testing.T) {
	p := sampleProgram()
	d := p.Lookup("server.User")
	m, ok := d.(*Model)
	if !ok {
		t.Fatalf("Lookup(server.User) = %T, want *Model", d)
	}
	if m.Common().Name != "server.User" || m.Kind() != "model" || len(m.Fields) != 6 {
		t.Errorf("unexpected model: %+v", m)
	}
	if s, ok := p.Lookup("db").(*State); !ok || s.Shape != StateList {
		t.Errorf("Lookup(db) = %#v", p.Lookup("db"))
	}
	if p.Lookup("server.nothing") != nil {
		t.Error("Lookup of unknown must be nil")
	}
	if p.Lookup("User") != nil {
		t.Error("Lookup must match the FQN, not the short name")
	}
	// 戻り値は Program の要素を指すので、書き換えが反映する。
	if op, ok := p.Lookup("server.notify").(*Op); ok {
		op.Signature.Effects = NamedEffects("x")
	}
	if got := p.Lookup("server.notify").(*Op).Signature.Effects; len(got) != 1 || got[0] != (NamedEffect{Effect: "x"}) {
		t.Error("Lookup must return a pointer into the program")
	}
	var nilP *Program
	if nilP.Lookup("x") != nil || nilP.Namespace("") != nil {
		t.Error("nil program must answer nil")
	}
}

func TestNamespace(t *testing.T) {
	p := sampleProgram()
	g := p.Namespace("")
	if g == nil || g.Name != "" || len(g.Decls) != 6 {
		t.Fatalf("global namespace: %+v", g)
	}
	s := p.Namespace("server")
	if s == nil || s.Description == nil || s.Description.Summary != "The server." {
		t.Fatalf("server namespace: %+v", s)
	}
	if s != &p.Namespaces[1] {
		t.Error("Namespace must return a pointer into Namespaces")
	}
	if p.Namespace("browser") != nil {
		t.Error("unknown namespace must be nil")
	}
}

func TestViewSignature(t *testing.T) {
	v := sampleView()
	sig := v.Signature("server.findUser")
	if sig == nil {
		t.Fatal("Signature(server.findUser) = nil")
	}
	if len(sig.Bound) != 1 || sig.Bound[0].State != "db" || len(sig.Params) != 1 || sig.Params[0].Name != "in" {
		t.Errorf("unexpected signature: %+v", sig)
	}
	if sig != v.Ops["server.findUser"] {
		t.Error("Signature must return the map's pointer")
	}
	if v.Signature("server.unknown") != nil {
		t.Error("unknown op must be nil")
	}
	var nilV *View
	if nilV.Signature("x") != nil {
		t.Error("nil view must answer nil")
	}
	if (&View{}).Signature("x") != nil {
		t.Error("nil ops must answer nil")
	}
}

func TestVersion(t *testing.T) {
	if Version != "0.11" {
		t.Errorf("Version = %q", Version)
	}
}
