package runtime

import (
	"encoding/json"
	"testing"
	"time"
)

// TestDate は、Date の、文字列の形、零値、繰り上げ、比較、JSON（パッケージ runtime の説明（runtime/doc.go））。
func TestDate(t *testing.T) {
	if got := (Date{}).String(); got != "0001-01-01" {
		t.Errorf("零値 = %q, want 0001-01-01", got)
	}
	d := NewDate(2026, time.November, 21)
	if d.Year() != 2026 || d.Month() != time.November || d.Day() != 21 || d.String() != "2026-11-21" {
		t.Errorf("NewDate = %v", d)
	}
	if got := NewDate(2026, 13, 1); got != NewDate(2027, time.January, 1) {
		t.Errorf("13 月 = %v, want 2027-01-01", got)
	}
	jst := time.FixedZone("JST", 9*60*60)
	if got := DateOf(time.Date(2026, 11, 21, 23, 30, 0, 0, time.UTC).In(jst)); got.String() != "2026-11-22" {
		t.Errorf("DateOf（その時刻帯での日付）= %v, want 2026-11-22", got)
	}
	if got := d.In(time.UTC); !got.Equal(time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("In = %v", got)
	}
	for _, tt := range []struct {
		a, b Date
		want int
	}{
		{NewDate(2026, 1, 1), NewDate(2026, 1, 1), 0},
		{NewDate(2025, 12, 31), NewDate(2026, 1, 1), -1},
		{NewDate(2026, 2, 1), NewDate(2026, 1, 31), 1},
		{NewDate(2026, 1, 2), NewDate(2026, 1, 1), 1},
	} {
		if got := tt.a.Compare(tt.b); got != tt.want {
			t.Errorf("%v.Compare(%v) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
	for _, bad := range []string{"", "2026-1-1", "2026-13-01", "2026-11-21T00:00:00Z", "20261121"} {
		if _, err := ParseDate(bad); err == nil {
			t.Errorf("ParseDate(%q) が通った", bad)
		}
	}
	// JSON: 文字列。map のキーにもなる
	type box struct {
		On   Date           `json:"on"`
		Opt  *Date          `json:"opt,omitempty"`
		Days map[Date]int64 `json:"days"`
	}
	data, err := json.Marshal(box{On: d, Days: map[Date]int64{d: 1}})
	if err != nil || string(data) != `{"on":"2026-11-21","days":{"2026-11-21":1}}` {
		t.Fatalf("Marshal = %s, %v", data, err)
	}
	var back box
	if err := json.Unmarshal(data, &back); err != nil || back.On != d || back.Days[d] != 1 || back.Opt != nil {
		t.Errorf("Unmarshal = %+v, %v", back, err)
	}
	if err := json.Unmarshal([]byte(`{"on":"2026-02-30"}`), &back); err == nil {
		t.Error("ない日付が、読めた")
	}
}

// TestTimeOfDay は、TimeOfDay の、文字列の形、零値、一日の外の値、比較、JSON。
func TestTimeOfDay(t *testing.T) {
	if got := (TimeOfDay{}).String(); got != "00:00:00" {
		t.Errorf("零値 = %q, want 00:00:00", got)
	}
	v := NewTimeOfDay(9, 30, 0, 500_000_000)
	if v.Hour() != 9 || v.Minute() != 30 || v.Second() != 0 || v.Nanosecond() != 500_000_000 {
		t.Errorf("NewTimeOfDay = %v", v)
	}
	if v.String() != "09:30:00.5" || v.Fixed() != "09:30:00.500000000" {
		t.Errorf("String = %q, Fixed = %q", v.String(), v.Fixed())
	}
	if got := NewTimeOfDay(24, 0, 0, 0); got != (TimeOfDay{}) {
		t.Errorf("24 時 = %v, want 00:00:00", got)
	}
	if got := NewTimeOfDay(0, 0, -1, 0).String(); got != "23:59:59" {
		t.Errorf("-1 秒 = %v, want 23:59:59", got)
	}
	if got := v.SinceMidnight(); got != 9*time.Hour+30*time.Minute+500*time.Millisecond {
		t.Errorf("SinceMidnight = %v", got)
	}
	if got := TimeOfDayOf(time.Date(2026, 11, 21, 18, 5, 7, 9, time.UTC)).String(); got != "18:05:07.000000009" {
		t.Errorf("TimeOfDayOf = %q", got)
	}
	if NewTimeOfDay(9, 0, 0, 0).Compare(NewTimeOfDay(10, 0, 0, 0)) != -1 || v.Compare(v) != 0 || NewTimeOfDay(10, 0, 0, 1).Compare(NewTimeOfDay(10, 0, 0, 0)) != 1 {
		t.Error("Compare")
	}
	for in, want := range map[string]string{"09:30:00": "09:30:00", "23:59:59.999999999": "23:59:59.999999999", "00:00:00.120": "00:00:00.12"} {
		got, err := ParseTimeOfDay(in)
		if err != nil || got.String() != want {
			t.Errorf("ParseTimeOfDay(%q) = %v, %v, want %s", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "9:30:00", "09:30", "24:00:00", "09:60:00", "09:30:00.", "09:30:00.1234567890", "09:30:00Z", "09:30:00+09:00"} {
		if _, err := ParseTimeOfDay(bad); err == nil {
			t.Errorf("ParseTimeOfDay(%q) が通った", bad)
		}
	}
	data, err := json.Marshal(struct {
		At TimeOfDay `json:"at"`
	}{v})
	if err != nil || string(data) != `{"at":"09:30:00.5"}` {
		t.Fatalf("Marshal = %s, %v", data, err)
	}
	var back struct {
		At TimeOfDay `json:"at"`
	}
	if err := json.Unmarshal(data, &back); err != nil || back.At != v {
		t.Errorf("Unmarshal = %+v, %v", back, err)
	}
}
