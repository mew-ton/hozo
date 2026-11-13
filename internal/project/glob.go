package project

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// matchGlob は "/" 区切りの相対パス name が glob pattern に合うかどうか。
// セグメントごとに path.Match で比べ、"**" のセグメントは 0 個以上のセグメントに合う（docs/config.md「形」の
// `sources = ["**/*.hozo"]`）。pattern は "./" や余分な "/" を取り除いて比べる。
func matchGlob(pattern, name string) bool {
	return matchSegments(strings.Split(cleanRel(pattern), "/"), strings.Split(name, "/"))
}

func matchSegments(p, n []string) bool {
	for len(p) > 0 {
		if p[0] == "**" {
			if len(p) == 1 {
				return true
			}
			for i := 0; i <= len(n); i++ {
				if matchSegments(p[1:], n[i:]) {
					return true
				}
			}
			return false
		}
		if len(n) == 0 {
			return false
		}
		ok, err := path.Match(p[0], n[0])
		if err != nil || !ok {
			return false
		}
		p, n = p[1:], n[1:]
	}
	return len(n) == 0
}

// hasMeta は glob のメタ文字を含むか。含まなければ pattern はただのパス。
func hasMeta(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

// cleanRel はパスを "/" 区切りに正規化し、先頭の "./" と "/" を落とす。
func cleanRel(p string) string {
	p = path.Clean(filepath.ToSlash(p))
	p = strings.TrimPrefix(p, "/")
	if p == "." {
		return ""
	}
	return p
}

// excluder は除くディレクトリの集合（各単位の out と plugin_dirs。docs/config.md「形」）。
type excluder []string

// excluded は rel（"/" 区切りの相対パス）が除くディレクトリの下にあるか。
func (e excluder) excluded(rel string) bool {
	for _, d := range e {
		if d == "" {
			continue
		}
		if rel == d || strings.HasPrefix(rel, d+"/") {
			return true
		}
	}
	return false
}

// collectSources は root の下で patterns に合うファイルの相対パス（"/" 区切り）を辞書順に返す。
// メタ文字のない pattern はそのパスのファイルがあるかだけを見る。メタ文字のある pattern があれば root を一度だけ歩く。
// 除くディレクトリ（ex）の中には入らない。
func collectSources(root string, patterns []string, ex excluder) ([]string, error) {
	seen := map[string]bool{}
	var walk []string
	for _, pat := range patterns {
		clean := cleanRel(pat)
		if clean == "" {
			continue
		}
		if hasMeta(clean) {
			walk = append(walk, clean)
			continue
		}
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(clean)))
		if err != nil || info.IsDir() || ex.excluded(clean) {
			continue
		}
		seen[clean] = true
	}
	if len(walk) > 0 {
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				if p == root {
					return err
				}
				return nil // 読めないディレクトリは飛ばす
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if rel == "." {
				return nil
			}
			if d.IsDir() {
				if ex.excluded(rel) {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() || ex.excluded(rel) {
				return nil
			}
			for _, pat := range walk {
				if matchGlob(pat, rel) {
					seen[rel] = true
					break
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// matchesAny は rel が patterns のどれかに合い、除くディレクトリの下にないか。overlay のファイルに使う。
func matchesAny(patterns []string, ex excluder, rel string) bool {
	if ex.excluded(rel) {
		return false
	}
	for _, pat := range patterns {
		if matchGlob(pat, rel) {
			return true
		}
	}
	return false
}
