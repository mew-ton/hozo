package runtime

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestValidationErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  *ValidationError
		want string
	}{
		{"scalar", &ValidationError{Violations: []Violation{{Path: "", Constraint: "format"}}}, "validation failed: format"},
		{"model", &ValidationError{Violations: []Violation{{Path: "body.email", Constraint: "format"}, {Path: "items[2].id", Constraint: "min"}}},
			"validation failed: body.email: format; items[2].id: min"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidationErrorIsError(t *testing.T) {
	var err error = &ValidationError{Violations: []Violation{{Constraint: "min"}}}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatal("errors.As failed for *ValidationError")
	}
}

func TestClassificationErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  *ClassificationError
		want string
	}{
		{"none", &ClassificationError{Classify: "server.WarrantyKind"}, "classification server.WarrantyKind: no case matched"},
		{"many", &ClassificationError{Classify: "server.WarrantyKind", Hits: []string{"Standard", "Yamada"}},
			"classification server.WarrantyKind: 2 cases matched (Standard, Yamada)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

type fakeException struct{}

func (fakeException) Error() string         { return "server.InvalidCredentials" }
func (fakeException) HozoException() string { return "server.InvalidCredentials" }

func TestExceptionShape(t *testing.T) {
	var e Exception = fakeException{}
	if e.HozoException() != "server.InvalidCredentials" || e.Error() != "server.InvalidCredentials" {
		t.Fatal("Exception must be an error with HozoException()")
	}
	const eff Effect = "server.createSession"
	if string(eff) != "server.createSession" {
		t.Fatal("Effect is a string")
	}
}

func TestCheckFormat(t *testing.T) {
	tests := []struct {
		format string
		valid  []string
		bad    []string
	}{
		{
			format: "email",
			valid:  []string{"a@b.co", "first.last@example.com", "user+tag@sub.example.org", "o'neil@x.io", "A_B-c@x-y.example"},
			bad: []string{"", "plain", "@x.y", "a@", "a@b", "a@@b.c", "a b@x.y", ".a@x.y", "a.@x.y", "a..b@x.y",
				"a@-x.y", "a@x-.y", "a@x..y", "a@x.y.", "a@.x.y", "\"a b\"@x.y", "a@[1.2.3.4]", "a@x.y\n", "日本@x.y"},
		},
		{
			format: "uri",
			valid:  []string{"https://example.com/a?b=c#d", "mailto:a@b.c", "urn:isbn:0451450523", "file:///tmp/x", "a+b-c.d://host"},
			bad:    []string{"", "/relative/path", "example.com", "http://a b", "1http://x", "https://例.jp", "http://x/\x01", ":noscheme"},
		},
		{
			format: "uuid",
			valid:  []string{"123e4567-e89b-12d3-a456-426614174000", "00000000-0000-0000-0000-000000000000", "A1B2C3D4-E5F6-7890-ABCD-EF1234567890"},
			bad: []string{"", "123e4567e89b12d3a456426614174000", "123e4567-e89b-12d3-a456-42661417400", "123e4567-e89b-12d3-a456-4266141740000",
				"g23e4567-e89b-12d3-a456-426614174000", "123e4567_e89b_12d3_a456_426614174000", "{123e4567-e89b-12d3-a456-426614174000}"},
		},
		{
			format: "date-time",
			valid: []string{"2024-01-02T03:04:05Z", "2024-01-02T03:04:05.123456789Z", "2024-01-02T03:04:05+09:00", "2024-01-02T03:04:05.5-07:00",
				"2024-01-02t03:04:05z", "2016-12-31T23:59:60Z", "2024-02-29T00:00:00Z"},
			bad: []string{"", "2024-01-02", "03:04:05Z", "2024-01-02T03:04:05", "2024-01-02 03:04:05Z", "2024-13-02T03:04:05Z", "2024-01-32T03:04:05Z",
				"2023-02-29T00:00:00Z", "2024-01-02T24:00:00Z", "2024-01-02T03:60:05Z", "2024-01-02T03:04:61Z", "2024-01-02T03:04:05+0900", "24-01-02T03:04:05Z"},
		},
	}
	for _, tt := range tests {
		for _, v := range tt.valid {
			if !CheckFormat(tt.format, v) {
				t.Errorf("CheckFormat(%q, %q) = false, want true", tt.format, v)
			}
		}
		for _, v := range tt.bad {
			if CheckFormat(tt.format, v) {
				t.Errorf("CheckFormat(%q, %q) = true, want false", tt.format, v)
			}
		}
	}
	if CheckFormat("unknown", "x") {
		t.Error("unknown format must be false")
	}
}

// TestDBTxKey は、ctx が運ぶトランザクションの鍵（docs/plugin-go-sqlite.md「トランザクション」）: 同じ *sql.DB から作った鍵は等しく
// （どのパッケージで作っても、同じ値を引く）、別の *sql.DB の鍵は別。
func TestDBTxKey(t *testing.T) {
	db, other := new(sql.DB), new(sql.DB)
	tx := new(sql.Tx)
	ctx := context.WithValue(context.Background(), DBTxKey{DB: db}, tx)
	if got, ok := ctx.Value(DBTxKey{DB: db}).(*sql.Tx); !ok || got != tx {
		t.Errorf("同じ db の鍵で引いた値 = %v, %v（期待は、置いた *sql.Tx）", got, ok)
	}
	if got := ctx.Value(DBTxKey{DB: other}); got != nil {
		t.Errorf("別の db の鍵で引いた値 = %v（期待は nil）", got)
	}
	if got := ctx.Value(DBTxKey{}); got != nil {
		t.Errorf("db のない鍵で引いた値 = %v（期待は nil）", got)
	}
}

func TestCaseErrorMessage(t *testing.T) {
	err := &CaseError{Effect: "pay.capture", Param: "payment", Classify: "pay.Standing", Want: []string{"Held", "Prepaid"}, Got: "Settled"}
	if got, want := err.Error(), "effect pay.capture: argument payment is in case Settled of pay.Standing, want Held, Prepaid"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	// 仕様の例外ではない
	var e error = err
	if _, ok := e.(Exception); ok {
		t.Error("CaseError が Exception を満たしている")
	}
}
