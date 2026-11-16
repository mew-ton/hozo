package ir

import "time"

func span(file string, l1, c1, l2, c2 int) Span {
	return Span{File: file, Start: Pos{Line: l1, Col: c1}, End: Pos{Line: l2, Col: c2}}
}

// sampleProgram は docs/ir.md「構造」のすべての種類の節を一つずつ以上含む小さな Program。
// 空の列は nil で書く（読むと nil になるので、往復の比較が成り立つ）。
func sampleProgram() *Program {
	const f = "server.hozo"
	return &Program{
		Version: Version,
		Namespaces: []Namespace{
			{
				Name: "",
				Decls: []Decl{
					&State{
						DeclBase: DeclBase{Name: "db", Span: span("global.hozo", 1, 1, 1, 20)},
						Type:     Array(Ref("server.User")),
						Shape:    StateList,
						Element:  Ref("server.User"),
					},
					&State{
						DeclBase: DeclBase{Name: "session", Span: span("global.hozo", 2, 1, 2, 40)},
						Type:     Optional(Ref("server.Token")),
						Shape:    StateNullable,
						Element:  Ref("server.Token"),
					},
					&State{
						DeclBase: DeclBase{Name: "clock", Span: span("global.hozo", 3, 1, 3, 30)},
						Type:     Time(),
						Shape:    StateSingle,
						Element:  Time(),
					},
					&Classify{
						DeclBase: DeclBase{Name: "Login", Span: span("global.hozo", 4, 1, 7, 2)},
						Subject:  StatesSubject{States: []string{"db", "session"}},
						Cases: []ClassifyCase{
							{Name: "Anonymous", Span: span("global.hozo", 5, 3, 5, 13)},
							{
								Name: "LoggedIn",
								As: []CaseTyping{
									{State: "session", Type: Ref("server.Token"), By: NarrowByPresence, Span: span("global.hozo", 6, 13, 6, 40)},
									{State: "db", Type: Array(Ref("server.User")), By: NarrowByPredicate, Span: span("global.hozo", 6, 42, 6, 60)},
								},
								Span: span("global.hozo", 6, 3, 6, 61),
							},
						},
						DecidedBy: DecidedByPredicate,
					},
					&Effect{
						DeclBase: DeclBase{Name: "login", Span: span("global.hozo", 8, 1, 8, 50)},
						Target:   TransitionTarget{Of: "Login", From: Cases("Anonymous"), To: Cases("LoggedIn")},
						Writes:   []string{"db", "session"},
					},
					&Effect{
						DeclBase: DeclBase{Name: "logout", Span: span("global.hozo", 9, 1, 9, 50)},
						Target:   TransitionTarget{Of: "session", From: AnyCase(), To: Cases(CaseEmpty)},
						Writes:   []string{"session"},
					},
				},
			},
			{
				Name: "server",
				Description: &Doc{
					Summary: "The server.",
					Body:    "The server.\n\nMore.",
				},
				Decls: []Decl{
					&Scalar{
						DeclBase: DeclBase{Name: "server.Token", Span: span(f, 3, 1, 3, 30)},
						Base:     String(),
					},
					&Scalar{
						DeclBase: DeclBase{
							Name: "server.Email",
							Span: span(f, 4, 1, 4, 30),
							Decorators: []Application{{
								Decorator: "valid.pattern",
								Args: []Arg{{
									Param: "pattern",
									Value: LiteralValue{Literal: StringLiteral{Value: ".+@.+"}},
								}},
								Span: span(f, 4, 1, 4, 20),
							}},
						},
						Base: String(),
					},
					&Model{
						DeclBase: DeclBase{
							Name: "server.User",
							Description: &Doc{
								Summary: "A user.",
								Body:    "A user.",
								See:     []string{"server.Token"},
							},
							Span: span(f, 6, 1, 12, 2),
						},
						Fields: []Field{
							{Name: "id", Type: Int(), Span: span(f, 7, 3, 7, 10)},
							{
								Name:        "email",
								Description: &Doc{Summary: "Mail.", Body: "Mail."},
								Type:        Ref("server.Email"),
								Span:        span(f, 8, 3, 8, 16),
							},
							{Name: "token", Type: Optional(Ref("server.Token")), Span: span(f, 9, 3, 9, 16)},
							{Name: "tags", Type: Array(String()), Span: span(f, 10, 3, 10, 17)},
							{
								Name: "limit",
								Type: Int(),
								Decorators: []Application{{
									Decorator: "valid.min",
									Args:      []Arg{{Param: "min", Value: LiteralValue{Literal: FloatLiteral{Value: 0.5}}}},
									Span:      span(f, 11, 3, 11, 20),
								}},
								Span: span(f, 11, 3, 11, 30),
							},
							{Name: "attrs", Type: Record(String(), String()), Span: span(f, 12, 3, 12, 30)},
						},
					},
					&Model{
						DeclBase: DeclBase{Name: "server.PublicUser", Span: span(f, 14, 1, 14, 40)},
						Fields:   []Field{{Name: "id", Type: Int(), Span: span(f, 14, 3, 14, 20)}, {Name: "nick", Type: String(), Span: span(f, 14, 22, 14, 35)}},
						Extends:  &ModelExtends{Base: "server.User", Removed: []string{"email", "token", "tags", "limit", "attrs"}, Overridden: []string{"id"}},
					},
					&Union{
						DeclBase: DeclBase{Name: "server.OrderPage", Span: span(f, 16, 1, 19, 2)},
						Variant:  UnionArms,
						Arms: []UnionArm{
							{Name: "Ready", Type: Ref("server.OrderPage.Ready"), Span: span(f, 17, 3, 17, 30)},
							{
								Name:        "Unavailable",
								Type:        Ref("server.Unavailable"),
								Description: &Doc{Summary: "No orders.", Body: "No orders."},
								Span:        span(f, 18, 3, 18, 40),
							},
						},
					},
					&Model{
						DeclBase:        DeclBase{Name: "server.OrderPage.Ready", Span: span(f, 17, 10, 17, 30)},
						Fields:          []Field{{Name: "userName", Type: String(), Span: span(f, 17, 12, 17, 28)}},
						SynthesizedFrom: "server.OrderPage",
					},
					&Exception{
						DeclBase: DeclBase{Name: "server.Unauthorized", Span: span(f, 21, 1, 21, 30)},
					},
					&Exception{
						DeclBase: DeclBase{Name: "server.Unavailable", Span: span(f, 22, 1, 22, 40)},
						Value:    Ref("server.Unavailable.Value"),
					},
					&Model{
						DeclBase:        DeclBase{Name: "server.Unavailable.Value", Span: span(f, 22, 25, 22, 40)},
						Fields:          []Field{{Name: "reason", Type: String(), Span: span(f, 22, 27, 22, 38)}},
						SynthesizedFrom: "server.Unavailable",
					},
					&Effect{
						DeclBase: DeclBase{
							Name: "server.UserCreated",
							// 効果の宣言に付けたデコレータ（Decorator.targets の effect）
							Decorators: []Application{{
								Decorator: "server.inserts",
								Args:      []Arg{{Param: "table", Value: RefValue{Target: "db"}}},
								Span:      span(f, 24, 1, 24, 20),
							}},
							Span: span(f, 24, 1, 24, 40),
						},
						Target: BuiltinTarget{State: "db", Operation: OpPost},
						Writes: []string{"db"},
					},
					&Union{
						DeclBase: DeclBase{Name: "server.ApiError", Span: span(f, 26, 1, 28, 2)},
						Variant:  UnionSet,
						Members: []UnionMember{
							{Target: "server.Unauthorized", Span: span(f, 27, 3, 27, 22)},
							{Target: "server.Unavailable", Span: span(f, 27, 25, 27, 44)},
						},
					},
					&Classify{
						DeclBase: DeclBase{Name: "server.Status", Span: span(f, 30, 1, 33, 2)},
						Subject:  TypeSubject{Type: Ref("server.User")},
						Cases: []ClassifyCase{
							{Name: "Active", Span: span(f, 31, 3, 31, 10)},
							{Name: "Banned", Description: &Doc{Summary: "Banned.", Body: "Banned."}, Span: span(f, 32, 3, 32, 10)},
						},
						HasDefault:         true,
						DefaultDescription: &Doc{Summary: "Others.", Body: "Others."},
						DecidedBy:          DecidedByPredicate,
					},
					&Classify{
						DeclBase: DeclBase{Name: "server.Registration", Span: span(f, 34, 1, 34, 80)},
						Subject:  OpSubject{Op: "server.findUser", Type: Optional(Ref("server.User"))},
						Cases: []ClassifyCase{{
							Name: "Registered",
							As:   []CaseTyping{{Type: Ref("server.User"), By: NarrowByPresence, Span: span(f, 34, 50, 34, 70)}},
							Span: span(f, 34, 38, 34, 71),
						}},
						HasDefault: true,
						DecidedBy:  DecidedByType,
					},
					&Classify{
						DeclBase:      DeclBase{Name: "server.IsAdmin", Span: span(f, 35, 1, 35, 30)},
						Subject:       TypeSubject{Type: Ref("server.User")},
						Cases:         []ClassifyCase{{Name: "IsAdmin", Span: span(f, 35, 11, 35, 18)}},
						HasDefault:    true,
						FromCondition: true,
						DecidedBy:     DecidedByPredicate,
					},
					&Decorator{
						DeclBase: DeclBase{Name: "server.retry", Span: span(f, 37, 1, 41, 2)},
						Params: []Param{
							{Name: "times", Type: TypeParam{Types: []Type{Int(), Float()}}},
							{Name: "on", Type: AnyParam{Of: AnyException}, Optional: true},
							{Name: "after", Type: AnyParam{Of: AnyEffect, On: &EffectOn{State: "db", Operation: OpPost}}, Optional: true},
						},
						Targets: []DecoratorTarget{TargetOp, TargetField},
						Throws: []ThrowsClause{
							{Exceptions: []string{"server.Unavailable"}, Relations: []Relation{{Kind: Prevents}}},
							{Exceptions: []string{"$param:on"}, Relations: []Relation{{Kind: Keeps}}},
						},
						Catches: []CatchesClause{
							{Exceptions: []string{"server.Unauthorized"}, Mode: Recovers},
							{Exceptions: []string{"$param:on"}, Mode: Halts, Halts: "$param:after"},
							{Exceptions: []string{"server.Unavailable"}, Mode: Ignores},
						},
					},
					&Decorator{
						DeclBase: DeclBase{Name: "server.retryThrice", Span: span(f, 43, 1, 43, 50)},
						External: true,
						Alias: &DecoratorAlias{
							Target: "server.retry",
							Args: []Arg{
								{Param: "times", Value: LiteralValue{Literal: IntLiteral{Value: 3}}},
								{Param: "on", Value: RefsValue{Targets: []string{"server.Unauthorized", "server.Unavailable"}}},
							},
						},
					},
					&Decorator{
						DeclBase: DeclBase{Name: "server.inserts", Span: span(f, 44, 1, 44, 50)},
						Params:   []Param{{Name: "table", Type: AnyParam{Of: AnyState}}},
						Targets:  []DecoratorTarget{TargetEffect},
					},
					// reads / effects の節と、残りの引数を持つデコレータ
					&Decorator{
						DeclBase: DeclBase{Name: "server.stores", Span: span(f, 44, 52, 44, 140)},
						Params: []Param{
							{Name: "table", Type: AnyParam{Of: AnyState}, Optional: true},
							{Name: "targets", Type: AnyParam{Of: AnyEffect}, Rest: true},
						},
						Targets: []DecoratorTarget{TargetOp},
						Reads:   []string{"$param:table", "session"},
						Effects: []string{"$param:targets"},
					},
					&Op{
						DeclBase: DeclBase{
							Name: "server.findUser",
							Description: &Doc{
								Summary: "Find a user.",
								Body:    "Find a user.",
								Params:  []DocParam{{Name: "id", Text: "The id."}},
								Throws:  []DocThrows{{Exception: "server.Unauthorized", Text: "No token."}},
							},
							Decorators: []Application{
								{
									Decorator: "server.retry",
									Via:       "server.retryThrice",
									Args: []Arg{
										{Param: "times", Value: LiteralValue{Literal: IntLiteral{Value: 3}}},
										{Param: "on", Value: RefsValue{Targets: []string{"server.Unauthorized", "server.Unavailable"}}},
									},
									Span: span(f, 45, 1, 45, 13),
								},
								{
									Decorator: "server.retry",
									Args: []Arg{
										{Param: "times", Value: LiteralValue{Literal: DurationLiteral{Value: 5 * time.Minute}}},
										{Param: "on", Value: RefValue{Target: "server.ApiError"}},
										{Param: "after", Value: ListValue{Items: []ArgValue{
											VariantValue{Union: "server.OrderPage", Arm: "Ready"},
											LiteralValue{Literal: BooleanLiteral{Value: true}},
										}}},
									},
									Span: span(f, 46, 1, 46, 40),
								},
							},
							Span: span(f, 47, 1, 47, 60),
						},
						Exported: true,
						Params:   []OpParam{{Name: "id", Type: Int(), Span: span(f, 47, 15, 47, 22)}},
						Returns:  Optional(Ref("server.User")),
						Effects:  []string{"server.UserCreated"},
						Reads:    []string{"db"},
						Throws:   []ThrowsClause{{Exceptions: []string{"server.Unauthorized"}}},
						Body:     nil,
						Signature: Signature{
							Exceptions: []string{"server.Unauthorized", "server.Unavailable"},
							Effects:    []string{"server.UserCreated"},
							Relations: []SignatureRelation{
								{Exception: "server.Unauthorized", Effect: "server.UserCreated", State: Prevented},
								{Exception: "server.Unavailable", Effect: "server.UserCreated", State: Unknown},
							},
							SuccessEffects: []SuccessEffect{{Effect: "server.UserCreated", State: Kept}},
							Reads:          []string{"db"},
						},
					},
					&Op{
						DeclBase:  DeclBase{Name: "server.unauthorized", Span: span(f, 48, 1, 48, 50)},
						Returns:   Never(),
						Throws:    []ThrowsClause{{Exceptions: []string{"server.Unauthorized"}}},
						Signature: Signature{Exceptions: []string{"server.Unauthorized"}, Pure: true},
					},
					// デコレータが reads と effects を運ぶ葉の op（Op.carried）。残りの引数は一つの list
					&Op{
						DeclBase: DeclBase{
							Name: "server.record",
							Decorators: []Application{{
								Decorator: "server.stores",
								Args: []Arg{
									{Param: "table", Value: RefValue{Target: "db"}},
									{Param: "targets", Value: ListValue{Items: []ArgValue{
										RefValue{Target: "server.UserCreated"},
										RefsValue{Targets: []string{"login", "logout"}},
									}}},
								},
								Span: span(f, 48, 52, 48, 90),
							}},
							Span: span(f, 48, 52, 48, 130),
						},
						Params: []OpParam{{Name: "in", Type: Ref("server.User"), Span: span(f, 48, 100, 48, 115)}},
						Carried: Carried{
							Reads:   []string{"db", "session"},
							Effects: []string{"server.UserCreated", "login", "logout"},
						},
						Signature: Signature{
							Effects: []string{"server.UserCreated", "login", "logout"},
							SuccessEffects: []SuccessEffect{
								{Effect: "server.UserCreated", State: Kept},
								{Effect: "login", State: Kept},
								{Effect: "logout", State: Kept},
							},
							Reads: []string{"db", "session"},
						},
					},
					&Op{
						DeclBase: DeclBase{Name: "server.notify", Span: span(f, 49, 1, 49, 30)},
						Params:   []OpParam{{Name: "in", Type: Ref("server.User"), Span: span(f, 49, 12, 49, 25)}},
						Returns:  nil,
					},
					&Op{
						DeclBase: DeclBase{Name: "server.page", Span: span(f, 51, 1, 60, 2)},
						Params: []OpParam{
							{Name: "id", Type: Int(), Span: span(f, 51, 9, 51, 16)},
							{Name: "kind", Type: Case("server.Status", []string{"Active"}, Ref("server.User")), Span: span(f, 51, 18, 51, 50)},
						},
						Returns: Ref("server.OrderPage"),
						Body: &Wiring{Statements: []Statement{
							ConstStmt{
								Name: "user",
								Expr: CallExpr{Call: Call{
									Op:   "server.findUser",
									Args: []CallArg{{Param: "id", Value: NameAtom{Path: []string{"id"}, Span: span(f, 52, 32, 52, 34)}}},
									Span: span(f, 52, 16, 52, 35),
								}},
								Span: span(f, 52, 3, 52, 36),
							},
							CallStmt{
								Call: Call{
									Op: "server.notify",
									Args: []CallArg{{Param: "in", Value: RecordAtom{
										Fields: []RecordField{
											{Name: "id", Value: NameAtom{Path: []string{"user", "id"}, Span: span(f, 53, 22, 53, 29)}},
											{Name: "token", Value: NoneAtom{Span: span(f, 53, 38, 53, 42)}},
											{Name: "limit", Value: LiteralAtom{Literal: IntLiteral{Value: 5}, Span: span(f, 53, 51, 53, 52)}},
										},
										Span: span(f, 53, 17, 53, 54),
									}}},
									Bound: []BoundPosition{{Path: []string{"in", "email"}, State: "db"}},
									Span:  span(f, 53, 3, 53, 55),
								},
								Span: span(f, 53, 3, 53, 56),
							},
							MatchStmt{
								Match: Match{
									Subject:  ValueSubject{Atom: NameAtom{Path: []string{"user"}, Span: span(f, 54, 9, 54, 13)}},
									Classify: "server.IsAdmin",
									Arms: []MatchArm{
										{Cases: ArmCases{Cases: []string{"IsAdmin"}}, Body: CallExpr{Call: Call{Op: "server.notify", Args: []CallArg{{Param: "in", Value: NameAtom{Path: []string{"user"}, Span: span(f, 55, 30, 55, 34)}}}, Span: span(f, 55, 16, 55, 35)}}, Span: span(f, 55, 5, 55, 36)},
										{Cases: DefaultArm(), Body: NoneBody{}, Span: span(f, 56, 5, 56, 20)},
									},
									Span: span(f, 54, 3, 57, 4),
								},
								Span: span(f, 54, 3, 57, 5),
							},
							ReturnStmt{
								Expr: MatchExpr{Match: Match{
									Subject:  ValueSubject{Atom: NameAtom{Path: []string{"user"}, Span: span(f, 58, 16, 58, 20)}},
									Classify: "server.Status",
									Arms: []MatchArm{
										{Cases: ArmCases{Cases: []string{"Active", "Banned"}}, Body: CallExpr{Call: Call{Op: "server.findUser", Args: []CallArg{{Param: "id", Value: LiteralAtom{Literal: StringLiteral{Value: "x"}, Span: span(f, 59, 40, 59, 43)}}}, Span: span(f, 59, 24, 59, 44)}}, Span: span(f, 59, 5, 59, 45)},
									},
									Span: span(f, 58, 10, 60, 4),
								}},
								Span: span(f, 58, 3, 60, 5),
							},
							ReturnStmt{
								Expr: AtomExpr{Atom: LiteralAtom{Literal: FloatLiteral{Value: 1.5}, Span: span(f, 61, 10, 61, 13)}},
								Span: span(f, 61, 3, 61, 14),
							},
							MatchStmt{
								Match: Match{
									Subject:  ValueSubject{Atom: StateAtom{State: "session", Span: span(f, 62, 9, 62, 16)}},
									Classify: ClassifyNullable,
									Arms: []MatchArm{
										{Cases: ArmCases{Cases: []string{CaseEmpty}}, Body: NoneBody{}, Span: span(f, 63, 5, 63, 19)},
										{
											Cases:    ArmCases{Cases: []string{CaseHasItems}},
											Body:     CallExpr{Call: Call{Op: "server.notify", Span: span(f, 64, 17, 64, 32)}},
											Narrowed: []Narrowing{{Type: Ref("server.Token")}},
											Span:     span(f, 64, 5, 64, 33),
										},
									},
									Span: span(f, 62, 3, 65, 4),
								},
								Span: span(f, 62, 3, 65, 5),
							},
							MatchStmt{
								Match: Match{
									Subject:  ClassifyOwnedSubject{},
									Classify: "Login",
									Arms: []MatchArm{
										{Cases: ArmCases{Cases: []string{"Anonymous"}}, Body: ThrowBody{Exception: "server.Unauthorized", Span: span(f, 67, 18, 67, 43)}, Span: span(f, 67, 5, 67, 44)},
										{
											Cases:    ArmCases{Cases: []string{"LoggedIn"}},
											Body:     AtomExpr{Atom: StateAtom{State: "session", Path: []string{"value"}, Span: span(f, 67, 58, 67, 71)}},
											Narrowed: []Narrowing{{State: "session", Type: Ref("server.Token")}, {State: "db", Type: Array(Ref("server.User"))}},
											Span:     span(f, 67, 46, 67, 72),
										},
									},
									Span: span(f, 66, 3, 68, 4),
								},
								Span: span(f, 66, 3, 68, 5),
							},
						}},
						Signature: Signature{},
					},
				},
			},
		},
		Binds: []Bind{
			{Namespace: "browser", State: "db", Target: BindTarget{Namespace: "server", Deep: true}, Span: span("browser.hozo", 1, 1, 1, 30)},
			{Namespace: "browser", State: "db", Target: BindTarget{Namespace: "server.admin"}, Span: span("browser.hozo", 2, 1, 2, 30)},
		},
	}
}

func sampleView() *View {
	p := sampleProgram()
	return &View{
		Version: Version,
		Viewer:  "browser",
		Config: NamespaceConfig{Index: 1, Language: "go", Base: "go", Out: "gen/browser", Dir: "gen/browser", Transpilers: []string{"go", "go-cli"},
			Options: map[string]map[string]any{"go-cli": {"z": true, "a": float64(1), "nested": map[string]any{"y": "s", "x": 1.5}}},
			Seams:   map[string]string{"rate.limited": "go-ratelimit"}},
		Files: []string{"browser.hozo", "server.hozo", "hozo:vocab/http.hozo"},
		// server は二つの単位が出力する（設定の順）。server.admin は server の単位の対象で、Dir(U, server.admin) は
		// <out>/admin（docs/config.md「単位の対象」）
		Namespaces: map[string][]NamespaceConfig{
			"browser": {{Index: 1, Language: "go", Base: "go", Out: "gen/browser", Dir: "gen/browser", Transpilers: []string{"go", "go-cli"},
				Options: map[string]map[string]any{"go-cli": {"z": true, "a": float64(1), "nested": map[string]any{"y": "s", "x": 1.5}}},
				Seams:   map[string]string{"rate.limited": "go-ratelimit"}}},
			"server": {
				{Index: 2, Language: "go", Base: "go", Out: "gen/server", Dir: "gen/server", Transpilers: []string{"go", "go-http"}},
				{Index: 3, Language: "go", Base: "go", Out: "cmd/gen", Dir: "cmd/gen", Transpilers: []string{"go", "go-cli"}},
			},
			"server.admin": {
				{Index: 2, Language: "go", Base: "go", Out: "gen/server", Dir: "gen/server/admin", Transpilers: []string{"go", "go-http"}},
				{Index: 3, Language: "go", Base: "go", Out: "cmd/gen", Dir: "cmd/gen/admin", Transpilers: []string{"go", "go-cli"}},
			},
		},
		Program: p,
		Ops: map[string]*ViewedSignature{
			"server.page": {
				Signature: Signature{Exceptions: []string{"server.Unauthorized"}},
				Params:    []OpParam{{Name: "id", Type: Int(), Span: span("server.hozo", 51, 9, 51, 16)}},
			},
			"server.findUser": {
				Signature: p.Lookup("server.findUser").(*Op).Signature,
				Params:    []OpParam{{Name: "in", Type: Ref("browser.findUser$in"), Span: span("server.hozo", 47, 15, 47, 22)}},
				Bound:     []BoundPosition{{Path: []string{"in", "token"}, State: "db"}},
			},
			"server.notify": {},
			"server.record": {
				Signature: p.Lookup("server.record").(*Op).Signature,
				Params:    []OpParam{{Name: "in", Type: Ref("server.User"), Span: span("server.hozo", 48, 100, 48, 115)}},
			},
		},
	}
}
