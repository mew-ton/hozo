package runtime

import (
	"fmt"
	"time"
)

// Date は、暦の一日（仕様の核の型 date。docs/plugin-go.md「runtime パッケージ」）。時刻帯を持たない。
// 零値は 0001-01-01。== で比べられ、map のキーにできる。文字列の形は "2006-01-02"。
type Date struct {
	// 零値を、正しい日付（0001-01-01）にするために、年・月・日から 1 を引いて持つ。
	year  int32
	month uint8
	day   uint8
}

const dateLayout = "2006-01-02"

// NewDate は、年・月・日の日付。範囲の外の月と日は、time.Date と同じに、繰り上げる（13 月は、翌年の 1 月）。
func NewDate(year int, month time.Month, day int) Date {
	return DateOf(time.Date(year, month, day, 0, 0, 0, 0, time.UTC))
}

// DateOf は、時刻 t の、その時刻帯での日付。
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{year: int32(y - 1), month: uint8(m - 1), day: uint8(d - 1)}
}

// ParseDate は、"2006-01-02" の形の文字列を読む。
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return Date{}, fmt.Errorf("runtime: date %q: 2006-01-02 の形でない", s)
	}
	return DateOf(t), nil
}

// Year は、年。
func (d Date) Year() int { return int(d.year) + 1 }

// Month は、月。
func (d Date) Month() time.Month { return time.Month(d.month) + 1 }

// Day は、日。
func (d Date) Day() int { return int(d.day) + 1 }

// In は、その日の 0 時の時刻（時刻帯 loc での）。
func (d Date) In(loc *time.Location) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
}

// Compare は、d が o より前なら -1、同じなら 0、後なら +1。
func (d Date) Compare(o Date) int {
	switch {
	case d.year != o.year:
		return compareInt(int64(d.year), int64(o.year))
	case d.month != o.month:
		return compareInt(int64(d.month), int64(o.month))
	}
	return compareInt(int64(d.day), int64(o.day))
}

// String は、"2006-01-02" の形。
func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year(), int(d.Month()), d.Day())
}

// MarshalText は、String の形（encoding/json は、これを、文字列として書く）。
func (d Date) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalText は、ParseDate で読む。
func (d *Date) UnmarshalText(text []byte) error {
	v, err := ParseDate(string(text))
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// TimeOfDay は、一日の中の時刻（仕様の核の型 time。docs/plugin-go.md「runtime パッケージ」）。時刻帯を持たない。
// 零値は 00:00:00。== で比べられ、map のキーにできる。文字列の形は "15:04:05" で、秒より細かい部分は、小数部。
type TimeOfDay struct {
	ns int64 // 0 時からのナノ秒。0 以上、一日より小さい
}

const dayNanos = int64(24 * time.Hour)

// NewTimeOfDay は、時・分・秒・ナノ秒の時刻。一日の外の値は、一日の長さで割った余りにする（24 時は 0 時、-1 秒は 23:59:59）。
func NewTimeOfDay(hour, minute, second, nanosecond int) TimeOfDay {
	ns := (int64(hour)*int64(time.Hour) + int64(minute)*int64(time.Minute) + int64(second)*int64(time.Second) + int64(nanosecond)) % dayNanos
	if ns < 0 {
		ns += dayNanos
	}
	return TimeOfDay{ns: ns}
}

// TimeOfDayOf は、時刻 t の、その時刻帯での、一日の中の時刻。
func TimeOfDayOf(t time.Time) TimeOfDay {
	return NewTimeOfDay(t.Hour(), t.Minute(), t.Second(), t.Nanosecond())
}

// ParseTimeOfDay は、"15:04:05" の形の文字列を読む。小数部は、0 桁から 9 桁。
func ParseTimeOfDay(s string) (TimeOfDay, error) {
	bad := func() (TimeOfDay, error) {
		return TimeOfDay{}, fmt.Errorf("runtime: time %q: 15:04:05 の形でない", s)
	}
	// 時・分・秒は、二桁ずつ。小数部は、あれば、"." と、1 桁から 9 桁の数字
	if len(s) < 8 || s[2] != ':' || s[5] != ':' || len(s) == 9 || len(s) > 18 || (len(s) > 8 && s[8] != '.') {
		return bad()
	}
	t, err := time.Parse("15:04:05.999999999", s)
	if err != nil {
		return bad()
	}
	return TimeOfDayOf(t), nil
}

// Hour は、時（0〜23）。
func (t TimeOfDay) Hour() int { return int(t.ns / int64(time.Hour)) }

// Minute は、分（0〜59）。
func (t TimeOfDay) Minute() int { return int(t.ns % int64(time.Hour) / int64(time.Minute)) }

// Second は、秒（0〜59）。
func (t TimeOfDay) Second() int { return int(t.ns % int64(time.Minute) / int64(time.Second)) }

// Nanosecond は、秒より細かい部分（0〜999999999）。
func (t TimeOfDay) Nanosecond() int { return int(t.ns % int64(time.Second)) }

// SinceMidnight は、0 時からの長さ。
func (t TimeOfDay) SinceMidnight() time.Duration { return time.Duration(t.ns) }

// Compare は、t が o より前なら -1、同じなら 0、後なら +1。
func (t TimeOfDay) Compare(o TimeOfDay) int { return compareInt(t.ns, o.ns) }

// String は、"15:04:05" の形。秒より細かい部分があれば、末尾の 0 を除いた小数部を付ける（"09:30:00.5"）。
func (t TimeOfDay) String() string {
	s := fmt.Sprintf("%02d:%02d:%02d", t.Hour(), t.Minute(), t.Second())
	if n := t.Nanosecond(); n != 0 {
		frac := fmt.Sprintf("%09d", n)
		for frac[len(frac)-1] == '0' {
			frac = frac[:len(frac)-1]
		}
		s += "." + frac
	}
	return s
}

// Fixed は、小数部を、いつも 9 桁で書いた形（"09:30:00.000000000"）。文字列の順が、時刻の順になる。
func (t TimeOfDay) Fixed() string {
	return fmt.Sprintf("%02d:%02d:%02d.%09d", t.Hour(), t.Minute(), t.Second(), t.Nanosecond())
}

// MarshalText は、String の形（encoding/json は、これを、文字列として書く）。
func (t TimeOfDay) MarshalText() ([]byte, error) { return []byte(t.String()), nil }

// UnmarshalText は、ParseTimeOfDay で読む。
func (t *TimeOfDay) UnmarshalText(text []byte) error {
	v, err := ParseTimeOfDay(string(text))
	if err != nil {
		return err
	}
	*t = v
	return nil
}

func compareInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
