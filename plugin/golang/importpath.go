package golang

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"

	"github.com/mew-ton/hozo/ir"
)

// ErrNoModule は、出力の設定に、Go のモジュールの名前（基底へのオプション module）がないこと。基底のプラグインは GO-8 にする
// （docs/plugin-go.md「診断」）。
var ErrNoModule = errors.New("no Go module name in the options")

// ModuleOf は、出力の設定 cfg の、Go のモジュールの名前（docs/plugin-go.md「マニフェスト」のオプション module:
// cfg.Options[cfg.Base]["module"]）。書かれていない、文字列でない、空なら ErrNoModule を包んだ誤り（GO-8）。
// ファイル（go.mod）は、読まない（決定 D425）。
func ModuleOf(cfg ir.NamespaceConfig) (string, error) {
	v, written := cfg.Options[cfg.Base]["module"]
	if !written {
		return "", fmt.Errorf("%w: [options.%s] に module がない", ErrNoModule, cfg.Base)
	}
	s, isString := v.(string)
	if !isString || s == "" {
		return "", fmt.Errorf("%w: [options.%s] の module が、空でない文字列でない（%v）", ErrNoModule, cfg.Base, v)
	}
	return s, nil
}

// ImportPath は、モジュール module の、ディレクトリ dir（プロジェクトのルートからの相対パス。出力先 Dir(U, N) か、その下）に置く
// パッケージの import パス（docs/plugin-go.md「パッケージと import パス」）: module に、dir を `/` でつないだもの。dir がルート
// そのもの（"."、""）なら module。モジュールのルートは、プロジェクトのルート。
func ImportPath(module, dir string) string {
	dir = path.Clean(filepath.ToSlash(dir))
	if dir == "." || dir == "" || dir == "/" {
		return module
	}
	return module + "/" + dir
}
