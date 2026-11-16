// Command irschema は ir パッケージと plugin パッケージの Go の型から
// schema/ir.schema.json（JSON Schema draft 2020-12）を生成する
// （docs/packages.md「生成するもの」、docs/requirements.md「プラグインの書き方」）。
//
// 構造体のフィールド（名前、省略可能か、型）は reflect で機械的に写す。
// reflect では分からないもの、kind で分ける判別付きユニオンの変種の一覧、
// 名前付き文字列型の取りうる値、"all" / "default" のような特別な JSON、
// null を許す Op.returns と Op.body、要求の protocol / direction の定数は、
// このファイルの表で手で持つ。表に漏れがあれば生成は失敗する
// （知らない名前付き文字列型、ユニオンの表にない interface）。
//
// 使い方: go run ./internal/tools/irschema -o schema/ir.schema.json
// （schema/generate.go の go:generate が同じことをする）。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/mew-ton/hozo/ir"
	"github.com/mew-ton/hozo/plugin"
)

func main() {
	out := flag.String("o", "", "書き出す先。省略すれば標準出力")
	flag.Parse()
	b, err := Generate()
	if err != nil {
		fmt.Fprintln(os.Stderr, "irschema:", err)
		os.Exit(1)
	}
	if *out == "" {
		os.Stdout.Write(b)
		return
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "irschema:", err)
		os.Exit(1)
	}
}

// Generate はスキーマの JSON を返す。キーは辞書順、2 スペースのインデント、末尾に改行。
func Generate() ([]byte, error) {
	root, err := Schema()
	if err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Schema はスキーマをマップの木として組む。
func Schema() (map[string]any, error) {
	g := &gen{defs: map[string]any{}, kinds: map[reflect.Type]string{}, types: map[string]reflect.Type{}}
	if err := g.build(); err != nil {
		return nil, err
	}
	return map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "https://github.com/mew-ton/hozo/schema/ir.schema.json",
		"title":   "Hozo IR",
		"description": "Hozo の IR（docs/ir.md）の JSON の形。Program（hozoc の出力）か View（トランスパイラに渡すもの）。" +
			"$defs にはプラグインのプロトコル（docs/plugin-protocol.md「要求と応答」）の要求と応答も含む。" +
			"Go の ir / plugin パッケージから internal/tools/irschema が生成する。手で編集しない。",
		"oneOf": []any{ref("Program"), ref("View")},
		"$defs": g.defs,
	}, nil
}

// ---- 手で持つ表 ----

// unions は kind で分ける判別付きユニオン: interface 型と、その変種の具体型（docs/ir.md「JSON」）。
// 変種の kind は具体型の Kind() から取る。
var unions = []struct {
	iface    reflect.Type
	variants []reflect.Type
}{
	{typeOf[ir.Type](), []reflect.Type{
		typeOf[ir.CoreType](), typeOf[ir.RefType](), typeOf[ir.OptionalType](),
		typeOf[ir.ArrayType](), typeOf[ir.RecordType](), typeOf[ir.CaseType](), typeOf[ir.NeverType](),
	}},
	{typeOf[ir.Decl](), []reflect.Type{
		typeOf[ir.Model](), typeOf[ir.Scalar](), typeOf[ir.Union](), typeOf[ir.State](),
		typeOf[ir.Exception](), typeOf[ir.Effect](), typeOf[ir.Read](), typeOf[ir.Classify](), typeOf[ir.Decorator](), typeOf[ir.Op](),
	}},
	{typeOf[ir.EffectTarget](), []reflect.Type{typeOf[ir.BuiltinTarget](), typeOf[ir.TransitionTarget]()}},
	{typeOf[ir.ReadRef](), []reflect.Type{typeOf[ir.NamedRead](), typeOf[ir.DirectRead]()}},
	{typeOf[ir.EffectRef](), []reflect.Type{typeOf[ir.NamedEffect](), typeOf[ir.DirectEffect]()}},
	{typeOf[ir.Callee](), []reflect.Type{typeOf[ir.OpCallee](), typeOf[ir.ReadCallee](), typeOf[ir.EffectCallee]()}},
	{typeOf[ir.ClassifySubject](), []reflect.Type{typeOf[ir.TypeSubject](), typeOf[ir.OpSubject](), typeOf[ir.StatesSubject]()}},
	{typeOf[ir.ParamType](), []reflect.Type{typeOf[ir.TypeParam](), typeOf[ir.AnyParam]()}},
	{typeOf[ir.Statement](), []reflect.Type{
		typeOf[ir.ConstStmt](), typeOf[ir.ReturnStmt](), typeOf[ir.CallStmt](), typeOf[ir.MatchStmt](),
	}},
	{typeOf[ir.Expr](), []reflect.Type{typeOf[ir.CallExpr](), typeOf[ir.MatchExpr](), typeOf[ir.AtomExpr]()}},
	{typeOf[ir.Atom](), []reflect.Type{
		typeOf[ir.NameAtom](), typeOf[ir.StateAtom](), typeOf[ir.LiteralAtom](), typeOf[ir.NoneAtom](), typeOf[ir.RecordAtom](),
	}},
	{typeOf[ir.MatchSubject](), []reflect.Type{
		typeOf[ir.ValueSubject](), typeOf[ir.ClassifyOwnedSubject](),
	}},
	{typeOf[ir.ArmBody](), []reflect.Type{
		typeOf[ir.CallExpr](), typeOf[ir.AtomExpr](), typeOf[ir.NoneBody](), typeOf[ir.ThrowBody](),
	}},
	{typeOf[ir.Literal](), []reflect.Type{
		typeOf[ir.StringLiteral](), typeOf[ir.IntLiteral](), typeOf[ir.FloatLiteral](),
		typeOf[ir.BooleanLiteral](), typeOf[ir.DurationLiteral](),
	}},
	{typeOf[ir.ArgValue](), []reflect.Type{
		typeOf[ir.LiteralValue](), typeOf[ir.RefValue](), typeOf[ir.RefsValue](),
		typeOf[ir.VariantValue](), typeOf[ir.ListValue](),
	}},
}

// enums は名前付き文字列型の取りうる値（docs/ir.md「構造」の文字列リテラルのユニオン）。
// ir パッケージの定数と同じ並び。ここにない名前付き文字列型が現れたら生成は失敗する。
var enums = map[reflect.Type][]string{
	typeOf[ir.CoreName]():        {"string", "boolean", "int", "float", "Time", "Duration"},
	typeOf[ir.UnionVariant]():    {"arms", "set"},
	typeOf[ir.StateShape]():      {"list", "nullable", "single"},
	typeOf[ir.Operation]():       {"post", "put", "delete"},
	typeOf[ir.DecoratorTarget](): {"op", "field", "state", "exception", "scalar", "model", "effect", "read"},
	typeOf[ir.AnyOf]():           {"exception", "effect", "state", "op", "model", "variant"},
	typeOf[ir.RelationKind]():    {"prevents", "keeps"},
	typeOf[ir.CatchMode]():       {"recovers", "halts", "ignores"},
	typeOf[ir.EffectState]():     {"prevented", "kept", "unknown"},
	typeOf[ir.DecidedBy]():       {"type", "predicate"},
	typeOf[ir.NarrowBy]():        {"presence", "variant", "predicate", "else"},
}

// specials は MarshalJSON が構造体の形を出さない型。手で書いたスキーマを $defs に置く。
var specials = map[reflect.Type]map[string]any{
	typeOf[ir.CaseSet](): {
		"description": "遷移の前か後のケースの集合。\"any\" か、ケース名の列（書かれた順。docs/ir.md「state / exception / effect」の CaseSet）。",
		"oneOf":       []any{map[string]any{"const": "any"}, arrayOf(map[string]any{"type": "string"})},
	},
	typeOf[ir.ArmCases](): {
		"description": "match のアームが受けるケース。\"default\" か、ケース名の列（docs/ir.md「配線」の Match）。",
		"oneOf":       []any{map[string]any{"const": "default"}, arrayOf(map[string]any{"type": "string"})},
	},
}

// overrides はフィールドごとの手書きのスキーマ。キーは "<Go の型名>.<JSON のフィールド名>"。
var overrides = map[string]any{
	// docs/ir.md「Program」「その名前空間から見た IR」: version はこの文書の版。
	"Program.version": map[string]any{"const": ir.Version},
	"View.version":    map[string]any{"const": ir.Version},
	// docs/ir.md「op」: null は void / 葉。null を出すのはこの二つだけ（docs/ir.md「JSON」）。
	"Op.returns": nullable(ref("Type")),
	"Op.body":    nullable(ref("Wiring")),
	// docs/plugin-protocol.md「要求と応答」: protocol は docs/ir.md の version と同じ値。
	"EmitRequest.protocol":    map[string]any{"const": ir.Version},
	"EmitRequest.direction":   map[string]any{"const": "emit"},
	"CheckRequest.protocol":   map[string]any{"const": ir.Version},
	"CheckRequest.direction":  map[string]any{"const": "check"},
	"ImportRequest.protocol":  map[string]any{"const": ir.Version},
	"ImportRequest.direction": map[string]any{"const": "import"},
	// docs/plugin-protocol.md「check」: severity は "error" か "warning"。emit の errors では省く。
	"Diagnostic.severity": map[string]any{"type": "string", "enum": []string{"error", "warning"}},
	// check の診断は severity を必ず持つ。
	"CheckResponse.diagnostics": arrayOf(ref("CheckDiagnostic")),
}

// descriptions は $defs の説明。docs の見出しを指す。
var descriptions = map[string]string{
	"Program":              "IR 全体（docs/ir.md「Program」）。namespaces の先頭はグローバル（name \"\"）。",
	"Namespace":            "一つの名前空間（docs/ir.md「Program」）。name は FQN。decls は出現順。",
	"Span":                 "宣言や文の位置（docs/ir.md「Program」）。file はユーザーのファイルの相対パスか hozo:vocab/... か hozo:plugin/...。",
	"Pos":                  "ファイルの中の位置（docs/ir.md「Program」）。1 始まり。col はコードポイント数。",
	"Doc":                  "DocComment の内容（docs/ir.md「Doc」）。",
	"DocParam":             "Doc の @param（docs/ir.md「Doc」）。",
	"DocThrows":            "Doc の @throws（docs/ir.md「Doc」）。",
	"Decl":                 "宣言（docs/ir.md「Program」の Decl）。kind で分ける。",
	"Type":                 "型（docs/ir.md「型」）。void は型ではなく Op.returns が null であること。never は Op.returns にだけ現れる。",
	"NeverType":            "決して値を返さない（docs/ir.md「型」の never）。Op.returns にだけ現れる。",
	"CoreType":             "核の型（docs/ir.md「型」の core）。",
	"RefType":              "model / scalar / union への参照（docs/ir.md「型」の ref）。",
	"OptionalType":         "T?（docs/ir.md「型」の optional）。入れ子にはしない。",
	"ArrayType":            "T[]（docs/ir.md「型」の array）。",
	"RecordType":           "Record<K, V>（docs/ir.md「型」の record）。",
	"CaseType":             "分類のケースで絞り込まれた型（docs/ir.md「型」の case）。cases は match のアームに書かれた順。",
	"Model":                "model 宣言（docs/ir.md「model / scalar / union」）。",
	"ModelExtends":         "継承した model（`model D extends B { ... }`）の記録（docs/ir.md「model / scalar / union」の extends）。fields は展開済み。",
	"Field":                "model のフィールド（docs/ir.md「model / scalar / union」の Field）。",
	"Scalar":               "scalar 宣言（docs/ir.md「model / scalar / union」）。",
	"Union":                "union 宣言（docs/ir.md「model / scalar / union」）。arms と members は常に出し、使わない方は []。",
	"UnionArm":             "arms のユニオンのアーム（docs/ir.md「model / scalar / union」）。",
	"UnionMember":          "set のユニオンのメンバー（docs/ir.md「model / scalar / union」）。",
	"State":                "state 宣言（docs/ir.md「state / exception / effect」）。type は状態の型。shape と element は type から決まり、element は optional にならない。",
	"StateShape":           "状態の形（docs/ir.md「state / exception / effect」の shape）。列数の範囲: list は 0 以上、nullable は 0 か 1、single はちょうど 1。状態の型の最も外側から決まる（docs/check.md「状態の形」）。",
	"Exception":            "exception 宣言（docs/ir.md「state / exception / effect」）。",
	"Effect":               "effect 宣言（docs/ir.md「state / exception / effect」）。宛先（target）は一つだけ。writes は書き込む基底の状態の FQN。",
	"EffectTarget":         "効果の宛先（docs/ir.md「state / exception / effect」の EffectTarget）。kind で分ける。",
	"BuiltinTarget":        "基底の状態の組み込みの効果（docs/ir.md「state / exception / effect」の EffectTarget kind \"builtin\"）。",
	"TransitionTarget":     "分類の遷移（docs/ir.md「state / exception / effect」の EffectTarget kind \"transition\"）。of は状態の分類の FQN か、組み込みの分類を持つ基底の状態の FQN。",
	"Classify":             "classify 宣言（docs/ir.md「classify」）。cases に default は含まない。",
	"ClassifyCase":         "classify のケース（docs/ir.md「classify」）。as は `対象 as T` の並び（なければ []）。",
	"CaseTyping":           "ケースの `対象 as T` と `対象 as else`（docs/ir.md「classify」の CaseTyping）。state は状態の分類のときだけ持つ。",
	"DecidedBy":            "分類のケースが何で決まるか（docs/ir.md「classify」の decidedBy、docs/check.md「ケースの判定」）。type なら述語を持たない。",
	"NarrowBy":             "絞り込みの種類（docs/ir.md「classify」の CaseTyping の by、docs/types.md「型で決まる絞り込み」）。else は残りのケース `対象 as else`。",
	"OpSubject":            "op の分類の対象（docs/ir.md「classify」の subject kind \"op\"）。op は FQN、type はその op の returns。",
	"ClassifySubject":      "分類の対象（docs/ir.md「classify」の subject）。kind で分ける。",
	"TypeSubject":          "値の分類の対象の型（docs/ir.md「classify」の subject kind \"type\"）。",
	"StatesSubject":        "状態の分類の対象（docs/ir.md「classify」の subject kind \"states\"）。基底の状態の FQN（書かれた順）。",
	"Decorator":            "decorator 宣言（docs/ir.md「decorator」）。alias を持つ宣言は params / targets / throws / catches が []。",
	"DecoratorAlias":       "名前付きデコレータの中身（docs/ir.md「decorator」の alias）。",
	"Param":                "デコレータの引数（docs/ir.md「decorator」の Param）。",
	"ParamType":            "デコレータの引数の型（docs/ir.md「decorator」の ParamType）。",
	"TypeParam":            "型で受ける引数（docs/ir.md「decorator」の ParamType kind \"type\"）。",
	"AnyParam":             "`any exception` のように宣言の参照で受ける引数（docs/ir.md「decorator」の ParamType kind \"any\"）。",
	"EffectOn":             "`any effect on <state>.<operation>` の制約（docs/ir.md「decorator」）。",
	"ThrowsClause":         "throws の一節（docs/ir.md「decorator」）。exceptions はデコレータの引数名なら \"$param:名前\"。",
	"Relation":             "例外が効果に及ぼす関係（docs/ir.md「decorator」）。目的語は常にその op のすべての効果（`effects`）。",
	"CatchesClause":        "catches の一節（docs/ir.md「decorator」）。",
	"Op":                   "op 宣言（docs/ir.md「op」）。returns の null は void、never は決して返さない op。body の null は葉。",
	"OpParam":              "op の引数（docs/ir.md「op」）。",
	"Signature":            "計算結果の関数の型 Sig(O, v)（docs/ir.md「op」、docs/check.md「計算」）。",
	"SignatureRelation":    "例外が起きたときの効果の状態（docs/ir.md「op」の Signature.relations）。",
	"SuccessEffect":        "成功のアームでの効果の状態（docs/ir.md「op」の Signature.successEffects）。",
	"Wiring":               "op の本体（docs/ir.md「配線」）。",
	"Statement":            "配線の文（docs/ir.md「配線」）。kind で分ける。",
	"Expr":                 "式（docs/ir.md「配線」）。kind で分ける。",
	"Call":                 "op の呼び出し（docs/ir.md「配線」）。args は引数名に対応付け済み。bound は計算結果。",
	"CallArg":              "呼び出しの引数（docs/ir.md「配線」の Call.args）。",
	"BoundPosition":        "bind で埋まる位置（docs/ir.md「配線」、docs/check.md「bind」）。",
	"Atom":                 "式の葉（docs/ir.md「配線」）。kind で分ける。",
	"Match":                "分類による分岐（docs/ir.md「配線」）。classify は FQN。標準の分類なら \"list\" か \"nullable\"。",
	"MatchSubject":         "match の対象（docs/ir.md「配線」の Match の subject）。kind で分ける。",
	"ValueSubject":         "値を対象にした match の対象（docs/ir.md「配線」の subject kind \"value\"）。状態を名前で書いた対象は atom が state。",
	"StateAtom":            "状態の参照（docs/ir.md「配線」の Atom kind \"state\"）。state は FQN、path はその値のフィールドのパス（状態そのものなら []）。",
	"Narrowing":            "アームの中で絞り込まれた対象の型（docs/ir.md「配線」の Narrowing）。state は状態の分類での match のときだけ持つ。",
	"ThrowBody":            "アームの本体の throw（docs/ir.md「配線」）。exception は FQN。",
	"ClassifyOwnedSubject": "状態の分類での match（docs/ir.md「配線」の subject kind \"classify\"）。対象は classify が持つ。",
	"MatchArm":             "match のアーム（docs/ir.md「配線」）。narrowed は、アームの中で外と型が違う対象（なければ []）。",
	"ArmBody":              "match のアームの本体（docs/ir.md「配線」）。call、atom、none、throw のどれか。",
	"NoneBody":             "アームの本体の none（docs/ir.md「配線」）。",
	"Literal":              "リテラル（docs/ir.md「配線」）。duration は整数のナノ秒。",
	"Application":          "デコレータの適用（docs/ir.md「デコレータの適用」）。",
	"Arg":                  "デコレータの引数の値（docs/ir.md「デコレータの適用」）。",
	"ArgValue":             "デコレータの引数の値の種類（docs/ir.md「デコレータの適用」）。kind で分ける。",
	"Bind":                 "bind 宣言（docs/ir.md「bind」）。",
	"BindTarget":           "bind の対象（docs/ir.md「bind」）。deep は `**`。",
	"View":                 "その名前空間から見た IR（docs/ir.md「その名前空間から見た IR」）。トランスパイラに渡すもの。単位と名前空間の組ごとに一つ。",
	"NamespaceConfig":      "単位での一つの名前空間の設定（docs/ir.md「その名前空間から見た IR」）。index は何番目の [[emit]] か、base は単位の基底、options は単位のオプション（トランスパイラの名前 → テーブル）、seams は単位の継ぎ目（デコレータの FQN → 宣言したトランスパイラ）。",
	"ViewedSignature":      "viewer から見た op の関数の型（docs/ir.md「その名前空間から見た IR」）。",
	"EmitRequest":          "direction \"emit\" の要求（docs/plugin-protocol.md「emit」）。transpiler は起動するトランスパイラの名前。",
	"EmitResponse":         "\"emit\" の応答（docs/plugin-protocol.md「emit」）。errors が一つでもあれば書き出さない。",
	"CheckRequest":         "direction \"check\" の要求（docs/plugin-protocol.md「check」）。",
	"CheckResponse":        "\"check\" の応答（docs/plugin-protocol.md「check」）。",
	"ImportRequest":        "direction \"import\" の要求（docs/plugin-protocol.md「import」）。",
	"ImportResponse":       "\"import\" の応答（docs/plugin-protocol.md「import」）。",
	"File":                 "書き出す（取り込む）ファイル（docs/plugin-protocol.md「emit」）。path はルートからの相対パス（emit の応答では、その起動の view.config.dir の下。そうでなければ PLG-1）、content は UTF-8 の文字列。",
	"Warning":              "warning の診断（docs/plugin-protocol.md「emit」）。at は FQN か \"file:line:col\"。",
	"Diagnostic":           "emit の errors の要素（docs/plugin-protocol.md「emit」）。severity は省ける（常に error）。",
	"CheckDiagnostic":      "check の diagnostics の要素（docs/plugin-protocol.md「check」）。severity を必ず持つ。",
	"PluginError":          "終了コード 2 のときの標準出力（docs/plugin-protocol.md「起動」）。",
}

// roots は $defs に置く起点。到達できる型をすべて写す。
var roots = []reflect.Type{
	typeOf[ir.Program](), typeOf[ir.View](),
	typeOf[plugin.EmitRequest](), typeOf[plugin.EmitResponse](),
	typeOf[plugin.CheckRequest](), typeOf[plugin.CheckResponse](),
	typeOf[plugin.ImportRequest](), typeOf[plugin.ImportResponse](),
}

// ---- 生成 ----

type gen struct {
	defs  map[string]any
	kinds map[reflect.Type]string // 変種の具体型 → kind
	types map[string]reflect.Type // $defs の名前 → Go の型
}

type kinder interface{ Kind() string }

func (g *gen) build() error {
	for _, u := range unions {
		refs := make([]any, 0, len(u.variants))
		for _, v := range u.variants {
			k, ok := reflect.New(v).Interface().(kinder)
			if !ok {
				return fmt.Errorf("%s: variant %s has no Kind()", u.iface.Name(), v.Name())
			}
			kind := k.Kind()
			if prev, dup := g.kinds[v]; dup && prev != kind {
				return fmt.Errorf("%s: kind %q conflicts with %q", v.Name(), kind, prev)
			}
			g.kinds[v] = kind
			refs = append(refs, ref(v.Name()))
		}
		if _, err := g.claim(u.iface.Name(), u.iface); err != nil {
			return err
		}
		g.defs[u.iface.Name()] = describe(u.iface.Name(), map[string]any{"oneOf": refs})
	}
	// kind の表が揃ってから変種を写す（CallExpr のように二つのユニオンに属する型があるため）。
	for _, u := range unions {
		for _, v := range u.variants {
			if _, err := g.schemaOf(v, ""); err != nil {
				return err
			}
		}
	}
	for _, r := range roots {
		if _, err := g.schemaOf(r, ""); err != nil {
			return err
		}
	}
	// 手で足す定義。
	cd, err := g.structDef(typeOf[plugin.Diagnostic](), "CheckDiagnostic")
	if err != nil {
		return err
	}
	cd["required"] = append([]string{"severity"}, cd["required"].([]string)...)
	g.defs["CheckDiagnostic"] = cd
	g.defs["PluginError"] = describe("PluginError", object(map[string]any{
		"error": object(map[string]any{
			"code":    map[string]any{"type": "string", "enum": []string{"protocol", "unsupported"}},
			"message": map[string]any{"type": "string"},
		}, "code", "message"),
	}, "error"))
	return nil
}

// claim は $defs の名前 name を型 t のものにする。ir と plugin で同じ名前の型があれば誤り
// （$defs は名前で引くので、黙って混ざるのを防ぐ）。すでに同じ型のものなら false。
func (g *gen) claim(name string, t reflect.Type) (bool, error) {
	if prev, ok := g.types[name]; ok {
		if prev != t {
			return false, fmt.Errorf("two types named %s: %s and %s", name, prev.PkgPath(), t.PkgPath())
		}
		return false, nil
	}
	g.types[name] = t
	return true, nil
}

// schemaOf は型 t のスキーマ（$ref か、その場のスキーマ）を返す。
// fieldKey は "<型名>.<フィールド名>" で、overrides を引くのに使う（フィールドでなければ ""）。
func (g *gen) schemaOf(t reflect.Type, fieldKey string) (any, error) {
	if o, ok := overrides[fieldKey]; ok {
		// 手書きのスキーマが $ref する定義も写しておく（Op.body の Wiring など）。
		if _, err := g.schemaOf(t, ""); err != nil {
			return nil, err
		}
		return o, nil
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if s, ok := specials[t]; ok {
		if fresh, err := g.claim(t.Name(), t); err != nil {
			return nil, err
		} else if fresh {
			g.defs[t.Name()] = s
		}
		return ref(t.Name()), nil
	}
	switch t.Kind() {
	case reflect.Interface:
		if t.Name() == "" && t.NumMethod() == 0 {
			// any: 形を決めない値（NamespaceConfig.options のテーブルの値。docs/ir.md の unknown）
			return map[string]any{}, nil
		}
		if _, ok := g.defs[t.Name()]; !ok {
			return nil, fmt.Errorf("interface %s is not in the unions table", t.Name())
		}
		return ref(t.Name()), nil
	case reflect.Struct:
		fresh, err := g.claim(t.Name(), t)
		if err != nil {
			return nil, err
		}
		if fresh {
			d, err := g.structDef(t, t.Name())
			if err != nil {
				return nil, err
			}
			g.defs[t.Name()] = d
		}
		return ref(t.Name()), nil
	case reflect.Slice:
		items, err := g.schemaOf(t.Elem(), "")
		if err != nil {
			return nil, err
		}
		return arrayOf(items), nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("map with non-string key: %s", t)
		}
		vals, err := g.schemaOf(t.Elem(), "")
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "object", "additionalProperties": vals}, nil
	case reflect.String:
		if t.Name() == "string" && t.PkgPath() == "" {
			return map[string]any{"type": "string"}, nil
		}
		values, ok := enums[t]
		if !ok {
			return nil, fmt.Errorf("named string type %s is not in the enums table", t.Name())
		}
		if fresh, err := g.claim(t.Name(), t); err != nil {
			return nil, err
		} else if fresh {
			g.defs[t.Name()] = describe(t.Name(), map[string]any{"type": "string", "enum": values})
		}
		return ref(t.Name()), nil
	case reflect.Bool:
		return map[string]any{"type": "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}, nil // time.Duration（ナノ秒）もここ
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}, nil
	}
	return nil, fmt.Errorf("unsupported type %s", t)
}

// structDef は構造体 t のオブジェクトのスキーマ。埋め込んだ構造体のフィールドは平らにする。
// omitempty も omitzero もないフィールドは required（Go は常に書く。空の列も []）。
// t がユニオンの変種なら kind の定数を足す。
func (g *gen) structDef(t reflect.Type, name string) (map[string]any, error) {
	props := map[string]any{}
	var required []string
	if kind, ok := g.kinds[t]; ok {
		props["kind"] = map[string]any{"const": kind}
		required = append(required, "kind")
	}
	var walk func(t reflect.Type) error
	walk = func(st reflect.Type) error {
		for i := 0; i < st.NumField(); i++ {
			f := st.Field(i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				if err := walk(f.Type); err != nil {
					return err
				}
				continue
			}
			if f.PkgPath != "" { // unexported
				continue
			}
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			jsonName, opts, _ := strings.Cut(tag, ",")
			if jsonName == "" {
				jsonName = f.Name
			}
			s, err := g.schemaOf(f.Type, t.Name()+"."+jsonName)
			if err != nil {
				return fmt.Errorf("%s.%s: %w", t.Name(), f.Name, err)
			}
			props[jsonName] = s
			if !strings.Contains(","+opts+",", ",omitempty,") && !strings.Contains(","+opts+",", ",omitzero,") {
				required = append(required, jsonName)
			}
		}
		return nil
	}
	if err := walk(t); err != nil {
		return nil, err
	}
	return describe(name, object(props, required...)), nil
}

// ---- 小さな道具 ----

func typeOf[T any]() reflect.Type { return reflect.TypeOf((*T)(nil)).Elem() }

func ref(name string) map[string]any { return map[string]any{"$ref": "#/$defs/" + name} }

func arrayOf(items any) map[string]any { return map[string]any{"type": "array", "items": items} }

func nullable(s any) map[string]any {
	return map[string]any{"oneOf": []any{s, map[string]any{"type": "null"}}}
}

func object(props map[string]any, required ...string) map[string]any {
	o := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		o["required"] = required
	}
	return o
}

func describe(name string, s map[string]any) map[string]any {
	if d, ok := descriptions[name]; ok {
		s["description"] = d
	}
	return s
}
