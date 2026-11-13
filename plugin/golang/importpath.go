package golang

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrNoGoMod は ImportPath が go.mod を見つけられなかったこと。呼ぶ側は GOEMIT-1 にする。
var ErrNoGoMod = errors.New("no go.mod found")

// ImportPath は `<root>/<out>` に置くパッケージの import パス（docs/emit-go.md「配置」）。
// `<root>/<out>` から親へ向かって最初に見つかる go.mod の `module` に、そのディレクトリから
// `<root>/<out>` への相対パスを `/` でつないだもの。out には、単位での名前空間の出力先 Dir(U, N)
// （View.config.dir。docs/config.md「単位の対象」）を渡す。go.mod が見つからなければ ErrNoGoMod を包んだ誤り（GOEMIT-1）。
// go.mod が読めない、または `module` 行がなければその誤り。ディレクトリは存在しなくてよい。
func ImportPath(root, out string) (string, error) {
	dir, err := filepath.Abs(filepath.Join(root, out))
	if err != nil {
		return "", err
	}
	for d := dir; ; {
		gomod := filepath.Join(d, "go.mod")
		if _, err := os.Stat(gomod); err == nil {
			module, err := modulePath(gomod)
			if err != nil {
				return "", err
			}
			rel, err := filepath.Rel(d, dir)
			if err != nil {
				return "", err
			}
			if rel == "." {
				return module, nil
			}
			return module + "/" + filepath.ToSlash(rel), nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", fmt.Errorf("%w above %s", ErrNoGoMod, dir)
		}
		d = parent
	}
}

// modulePath は go.mod の `module` 行のパスを読む。
func modulePath(gomod string) (string, error) {
	f, err := os.Open(gomod)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.Index(line, "//"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if !strings.HasPrefix(line, "module") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "module"))
		if rest == "" || rest == line {
			continue
		}
		if strings.HasPrefix(rest, `"`) {
			if s, err := strconv.Unquote(rest); err == nil {
				rest = s
			}
		}
		return rest, nil
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("%s: no module line", gomod)
}
