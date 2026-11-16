package runtime

import (
	"net/url"
	"strings"
	"time"
)

// CheckFormat は valid.format(name) の実装（docs/plugin-go-valid.md「runtime」）。
// name は "email" / "uri" / "uuid" / "date-time" のどれかで、v がその形なら true。
// 知らない name は false を返す（生成の時点で診断になるので、ここに来るのは生成器の不具合）。
//
//   - "email": `local@domain`。local は RFC 5322 の dot-atom（atext を `.` でつないだもの。先頭・末尾・連続の `.` は不可）、
//     domain は `.` でつないだ 2 つ以上のラベル（英数字とハイフン。先頭・末尾のハイフンは不可、各 63 文字以内）。
//     引用した local（`"a b"@x.y`）とアドレスリテラル（`@[1.2.3.4]`）は受け付けない。
//   - "uri": RFC 3986 の絶対 URI。scheme を持ち、ASCII の可視文字だけ。相対参照（`/a/b`）は不可。
//   - "uuid": RFC 4122 の文字列の形（8-4-4-4-12 の 16 進。大文字小文字は問わない）。版は確かめない。
//   - "date-time": RFC 3339 の date-time。`T` と `Z` は小文字でもよく、閏秒（秒が 60）も受け付ける。
//     時刻帯は必須（`2024-01-02T03:04:05` だけでは false）。
func CheckFormat(name, v string) bool {
	switch name {
	case "email":
		return checkEmail(v)
	case "uri":
		return checkURI(v)
	case "uuid":
		return checkUUID(v)
	case "date-time":
		return checkDateTime(v)
	}
	return false
}

func checkEmail(v string) bool {
	at := strings.LastIndexByte(v, '@')
	if at <= 0 || at == len(v)-1 {
		return false
	}
	local, domain := v[:at], v[at+1:]
	if len(local) > 64 || !isDotAtom(local) {
		return false
	}
	return isHostname(domain, true)
}

// isDotAtom は RFC 5322 の dot-atom-text か。
func isDotAtom(s string) bool {
	if s == "" || s[0] == '.' || s[len(s)-1] == '.' || strings.Contains(s, "..") {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '.' || isAlnum(c) || strings.IndexByte("!#$%&'*+-/=?^_`{|}~", c) >= 0 {
			continue
		}
		return false
	}
	return true
}

// isHostname は `.` でつないだラベルの列か。requireDot なら 2 つ以上のラベルを求める。
func isHostname(s string, requireDot bool) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	labels := strings.Split(s, ".")
	if requireDot && len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return false
		}
		for i := 0; i < len(l); i++ {
			if c := l[i]; !isAlnum(c) && c != '-' {
				return false
			}
		}
	}
	return true
}

func isAlnum(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}

func checkURI(v string) bool {
	for i := 0; i < len(v); i++ {
		if c := v[i]; c <= ' ' || c >= 0x7f {
			return false
		}
	}
	u, err := url.Parse(v)
	if err != nil || u.Scheme == "" {
		return false
	}
	// scheme = ALPHA *( ALPHA / DIGIT / "+" / "-" / "." )。url.Parse は先頭の数字を通すので確かめる。
	c := u.Scheme[0]
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

func checkUUID(v string) bool {
	if len(v) != 36 {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
				return false
			}
		}
	}
	return true
}

func checkDateTime(v string) bool {
	// RFC 3339 §5.6 の NOTE: T と Z は小文字でもよい。time.Parse は大文字しか読まないので直す。
	if len(v) < len("2006-01-02T15:04:05Z") {
		return false
	}
	b := []byte(v)
	if b[10] == 't' {
		b[10] = 'T'
	}
	if last := len(b) - 1; b[last] == 'z' {
		b[last] = 'Z'
	}
	// 閏秒。time.Parse は秒の 60 を拒むので 59 にして形だけ確かめる。
	if b[17] == '6' && b[18] == '0' {
		b[17], b[18] = '5', '9'
	}
	_, err := time.Parse(time.RFC3339Nano, string(b))
	return err == nil
}
