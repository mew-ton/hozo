package gosqlite

import "strings"

// このファイルは db.go の末尾の補助（docs/plugin-go-sqlite.md「補助」）。列ごと、メソッドごとに出すコードは同じ形で、値で変わること
// （値がなければ NULL、列の値への変え方と、その逆）と、文を実行する先（トランザクションか、db か）は、ここにある共通のコードが、
// 実行時に決める。テキストは、サンプル tests/samples/shop-server の gen/store/db.go の通り（dbInt、dbFloat、dbBool は、サンプルが
// 使わないので、単体テストのゴールデンが固定する）。

// helperText は、補助一つ: 名前、それが宣言する非公開の名前（メソッドの引数の局所の名前が避ける）、それが import する標準ライブラリ、テキスト。
type helperText struct {
	name    string
	names   []string
	imports []string
	text    string
}

// helperTexts は、補助のすべて（「補助」の表の順。使うものだけを、この順で出す）。dbOption、dbNullable、dbCol は、一緒に、いつも出す。
var helperTexts = []helperText{
	{name: helperRow, names: []string{helperRow}, text: `// dbRow は *sql.Row と *sql.Rows が共に持つ読み出し。
type dbRow interface {
	Scan(dest ...any) error
}
`},
	{name: helperCol, names: []string{helperOption, helperNullable, helperCol}, imports: []string{"database/sql/driver", "fmt", "reflect", "slices"}, text: `// dbOption は、列の読み書きに、フィールドの宣言が足すこと。
type dbOption int

// dbNullable は、省略可能な（T?）フィールドの列: 値がなくてよい（NULL）。
const dbNullable dbOption = 1

// dbCol は列一つ: しまい方（@db.text など。write と read）と、Go の値 v と、フィールドの宣言が足したこと opts。
// v は、書くときはフィールドの値（か、列と比べる引数）、読むときはフィールドへのポインタ。
// database/sql が、書くときに Value を、読むときに Scan を呼ぶ。
type dbCol struct {
	v     any
	opts  []dbOption
	write func(v reflect.Value) (driver.Value, error) // 値のある Go の値を、列の値にする
	read  func(src any, dst reflect.Value) error      // NULL でない列の値を、Go の値に読む
}

// Value は、列に書く値。値があるかどうかは、実行時に見る: 値のないポインタは NULL。
// 省略可能なフィールド（dbNullable）の列では、値のないスライスとマップも NULL。
func (c dbCol) Value() (driver.Value, error) {
	v := reflect.ValueOf(c.v)
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return nil, nil
		}
		v = v.Elem()
	case reflect.Slice, reflect.Map:
		if v.IsNil() && slices.Contains(c.opts, dbNullable) {
			return nil, nil
		}
	}
	return c.write(v)
}

// Scan は、列の値 src を、読む先に読む。NULL は、省略可能なフィールド（dbNullable）の列でだけ読めて、読む先は、値がない（nil）。
// ほかの列（NOT NULL）に NULL があれば、エラー。読む先がポインタかどうかは、実行時に見る。
func (c dbCol) Scan(src any) error {
	dst := reflect.ValueOf(c.v).Elem()
	if src == nil {
		if !slices.Contains(c.opts, dbNullable) {
			return fmt.Errorf("db: NULL in a NOT NULL column (read into %s)", dst.Type())
		}
		dst.SetZero()
		return nil
	}
	if dst.Kind() == reflect.Pointer {
		dst.Set(reflect.New(dst.Type().Elem()))
		dst = dst.Elem()
	}
	return c.read(src, dst)
}
`},
	{name: helperArgs, names: []string{helperArgs}, text: `// dbArgs は、列の並びを、文に渡す値の並びにする。
func dbArgs(cols ...dbCol) ([]any, error) {
	args := make([]any, len(cols))
	for i, c := range cols {
		v, err := c.Value()
		if err != nil {
			return nil, err
		}
		args[i] = v
	}
	return args, nil
}
`},
	{name: helperString, names: []string{helperString}, text: `// dbString は、列の値 src を文字列として読む。
func dbString(src any) (string, error) {
	var s sql.NullString
	err := s.Scan(src)
	return s.String, err
}
`},
	{name: "dbText", names: []string{"dbText"}, text: `// dbText は @db.text の列: 文字列を、そのまま。
func dbText(v any, opts ...dbOption) dbCol {
	return dbCol{v, opts, func(v reflect.Value) (driver.Value, error) {
		return v.String(), nil
	}, func(src any, dst reflect.Value) error {
		s, err := dbString(src)
		if err != nil {
			return err
		}
		dst.SetString(s)
		return nil
	}}
}
`},
	{name: "dbInt", names: []string{"dbInt"}, text: `// dbInt は @db.int の列: 整数を、そのまま（Duration は、ナノ秒の数）。
func dbInt(v any, opts ...dbOption) dbCol {
	return dbCol{v, opts, func(v reflect.Value) (driver.Value, error) {
		return v.Int(), nil
	}, func(src any, dst reflect.Value) error {
		var n sql.NullInt64
		if err := n.Scan(src); err != nil {
			return err
		}
		dst.SetInt(n.Int64)
		return nil
	}}
}
`},
	{name: "dbFloat", names: []string{"dbFloat"}, text: `// dbFloat は @db.float の列: 小数を、そのまま。
func dbFloat(v any, opts ...dbOption) dbCol {
	return dbCol{v, opts, func(v reflect.Value) (driver.Value, error) {
		return v.Float(), nil
	}, func(src any, dst reflect.Value) error {
		var n sql.NullFloat64
		if err := n.Scan(src); err != nil {
			return err
		}
		dst.SetFloat(n.Float64)
		return nil
	}}
}
`},
	{name: "dbBool", names: []string{"dbBool"}, text: `// dbBool は @db.bool の列: 真偽値を、0 か 1 の整数で。
func dbBool(v any, opts ...dbOption) dbCol {
	return dbCol{v, opts, func(v reflect.Value) (driver.Value, error) {
		if v.Bool() {
			return int64(1), nil
		}
		return int64(0), nil
	}, func(src any, dst reflect.Value) error {
		var b sql.NullBool
		if err := b.Scan(src); err != nil {
			return err
		}
		dst.SetBool(b.Bool)
		return nil
	}}
}
`},
	{name: "dbTime", names: []string{helperTimeLayout, "dbTime"}, imports: []string{"time"}, text: `// dbTimeLayout は @db.time の列の形: RFC 3339、UTC、小数部はいつも 9 桁（文字列の順が時刻の順になる）。
const dbTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// dbTime は @db.time の列: 時刻を、dbTimeLayout の文字列で。
func dbTime(v any, opts ...dbOption) dbCol {
	return dbCol{v, opts, func(v reflect.Value) (driver.Value, error) {
		t := v.Convert(reflect.TypeFor[time.Time]()).Interface().(time.Time)
		return t.UTC().Format(dbTimeLayout), nil
	}, func(src any, dst reflect.Value) error {
		s, err := dbString(src)
		if err != nil {
			return err
		}
		t, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(t).Convert(dst.Type()))
		return nil
	}}
}
`},
	{name: "dbJSON", names: []string{"dbJSON"}, imports: []string{"encoding/json"}, text: `// dbJSON は @db.json の列: 値を、encoding/json の JSON の文字列で。
func dbJSON(v any, opts ...dbOption) dbCol {
	return dbCol{v, opts, func(v reflect.Value) (driver.Value, error) {
		data, err := json.Marshal(v.Interface())
		return string(data), err
	}, func(src any, dst reflect.Value) error {
		s, err := dbString(src)
		if err != nil {
			return err
		}
		return json.Unmarshal([]byte(s), dst.Addr().Interface())
	}}
}
`},
	{name: helperTransaction, names: []string{helperTransaction}, text: `// dbTransaction は、body を、db の一つのトランザクションの中で呼ぶ: 始めて、body を呼び、確定する。
// トランザクションは、body に渡す ctx が運ぶ（鍵は runtime.DBTxKey）。body が失敗すれば（確定に失敗しても）取り消して、零値とそのエラーを返す。
// ctx が、すでに db のトランザクションを運んでいれば（入れ子。別の名前空間の継ぎ目が始めたものでも）、新しく始めずに、それに加わる: 確定も取り消しも、いちばん外側がする。
func dbTransaction[R any](ctx context.Context, db *sql.DB, body func(ctx context.Context) (R, error)) (R, error) {
	if _, ok := ctx.Value(runtime.DBTxKey{DB: db}).(*sql.Tx); ok {
		return body(ctx)
	}
	var zero R
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}
	defer tx.Rollback()
	out, err := body(context.WithValue(ctx, runtime.DBTxKey{DB: db}, tx))
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(); err != nil {
		return zero, err
	}
	return out, nil
}
`},
	{name: helperExec, names: []string{helperExec}, text: `// dbExec は行を返さない文を実行する。
func dbExec(ctx context.Context, db *sql.DB, query string, args ...any) error {
	_, err := Conn(ctx, db).ExecContext(ctx, query, args...)
	return err
}
`},
	{name: helperIsEmpty, names: []string{helperIsEmpty}, text: `// dbIsEmpty は、行の数を返す問い合わせの結果が 0 かどうか。
func dbIsEmpty(ctx context.Context, db *sql.DB, query string) (bool, error) {
	var n int64
	if err := Conn(ctx, db).QueryRowContext(ctx, query).Scan(&n); err != nil {
		return false, err
	}
	return n == 0, nil
}
`},
	{name: helperGet, names: []string{helperGet}, imports: []string{"errors"}, text: `// dbGet は問い合わせの一行を scan で読む。行がなければ nil。
func dbGet[R any](ctx context.Context, db *sql.DB, scan func(dbRow) (R, error), query string, args ...any) (*R, error) {
	row, err := scan(Conn(ctx, db).QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}
`},
	{name: helperList, names: []string{helperList}, text: `// dbList は問い合わせのすべての行を scan で読む。
func dbList[R any](ctx context.Context, db *sql.DB, scan func(dbRow) (R, error), query string, args ...any) ([]R, error) {
	rows, err := Conn(ctx, db).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []R
	for rows.Next() {
		row, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
`},
}

// helpers は補助（docs/plugin-go-sqlite.md「補助」の表の順）。使ったものだけを出す。dbOption、dbNullable、dbCol は、いつも出す。
// dbString は、それを使うしまい方の補助（dbText、dbTime、dbJSON）を出すときに出す。
func (f *fileGen) helpers() {
	f.used[helperCol] = true
	if f.used["dbText"] || f.used["dbTime"] || f.used["dbJSON"] {
		f.used[helperString] = true
	}
	first := true
	for _, h := range helperTexts {
		if !f.used[h.name] {
			continue
		}
		for _, p := range h.imports {
			f.im.Add(p, short(strings.ReplaceAll(p, "/", ".")))
		}
		if !first {
			f.printf("\n")
		}
		first = false
		f.printf("%s", h.text)
	}
}
