package ir

import "testing"

func TestTypeEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b Type
		want bool
	}{
		{"core same", String(), String(), true},
		{"core diff", String(), Int(), false},
		{"core vs ref", String(), Ref("string"), false},
		{"ref same", Ref("a.X"), Ref("a.X"), true},
		{"ref diff", Ref("a.X"), Ref("b.X"), false},
		{"optional same", Optional(Int()), Optional(Int()), true},
		{"optional vs plain", Optional(Int()), Int(), false},
		{"array same", Array(Ref("X")), Array(Ref("X")), true},
		{"array vs optional", Array(Int()), Optional(Int()), false},
		{"nested", Array(Optional(Array(Ref("X")))), Array(Optional(Array(Ref("X")))), true},
		{"case same order", Case("C", []string{"A", "B"}, Ref("T")), Case("C", []string{"A", "B"}, Ref("T")), true},
		{"case set order", Case("C", []string{"A", "B"}, Ref("T")), Case("C", []string{"B", "A"}, Ref("T")), true},
		{"case duplicate", Case("C", []string{"A", "A"}, Ref("T")), Case("C", []string{"A"}, Ref("T")), true},
		{"case subset", Case("C", []string{"A"}, Ref("T")), Case("C", []string{"A", "B"}, Ref("T")), false},
		{"case classify diff", Case("C", []string{"A"}, Ref("T")), Case("D", []string{"A"}, Ref("T")), false},
		{"case of diff", Case("C", []string{"A"}, Ref("T")), Case("C", []string{"A"}, Ref("U")), false},
		{"case vs of", Case("C", []string{"A"}, Ref("T")), Ref("T"), false},
		{"void void", nil, nil, true},
		{"void vs type", nil, Int(), false},
		{"type vs void", Int(), nil, false},
	}
	for _, c := range cases {
		if got := TypeEqual(c.a, c.b); got != c.want {
			t.Errorf("%s: TypeEqual(%v, %v) = %v, want %v", c.name, c.a, c.b, got, c.want)
		}
		if got := TypeEqual(c.b, c.a); got != c.want {
			t.Errorf("%s (swapped): TypeEqual(%v, %v) = %v, want %v", c.name, c.b, c.a, got, c.want)
		}
	}
}

func TestTypeString(t *testing.T) {
	cases := []struct {
		t    Type
		want string
	}{
		{String(), "string"},
		{Boolean(), "boolean"},
		{Int(), "int"},
		{Float(), "float"},
		{Datetime(), "datetime"},
		{Duration(), "duration"},
		{Ref("server.Token"), "server.Token"},
		{Optional(Ref("Token")), "Token?"},
		{Array(String()), "string[]"},
		{Optional(Array(String())), "string[]?"},
		{Array(Optional(String())), "string?[]"},
		{Case("server.WarrantyKind", []string{"Yamada"}, Ref("server.Item")), "server.WarrantyKind.Yamada<server.Item>"},
		{Case("C", []string{"A", "B"}, Int()), "C.(A, B)<int>"},
		{Case("C", nil, Int()), "C.()<int>"},
		{Optional(Case("C", []string{"A"}, Ref("T"))), "C.A<T>?"},
		{OptionalType{}, "void?"},
	}
	for _, c := range cases {
		if got := c.t.String(); got != c.want {
			t.Errorf("String() = %q, want %q", got, c.want)
		}
	}
}

func TestOptionalCollapses(t *testing.T) {
	o := Optional(Optional(Int()))
	if !TypeEqual(o, Optional(Int())) || o.String() != "int?" {
		t.Errorf("Optional(Optional(int)) = %v", o)
	}
	if _, ok := Optional(Array(Int())).(OptionalType); !ok {
		t.Error("Optional of a non-optional must wrap")
	}
}
