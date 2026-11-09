package diag

import (
	"sort"
	"strings"

	"github.com/mew-ton/hozo/internal/source"
)

// FileOrder はファイルの順。a が先なら負、同じなら 0、b が先なら正を返す。
type FileOrder func(a, b string) int

// PathOrder は docs/ir.md「原則」のファイルの順。
// ユーザーのファイルをパスの辞書順に並べ、その後に語彙のファイル（"hozo:" で始まるもの）を辞書順に並べる。
func PathOrder(a, b string) int {
	av, bv := strings.HasPrefix(a, "hozo:"), strings.HasPrefix(b, "hozo:")
	if av != bv {
		if av {
			return 1
		}
		return -1
	}
	return strings.Compare(a, b)
}

// FileSetOrder は FileSet に足した順を返す FileOrder。FileSet にないパスは、あるパスの後ろに辞書順で並べる。
func FileSetOrder(fs *source.FileSet) FileOrder {
	index := make(map[string]int, fs.Len())
	for i := 0; i < fs.Len(); i++ {
		p := fs.Path(source.FileID(i))
		if _, ok := index[p]; !ok {
			index[p] = i
		}
	}
	return func(a, b string) int {
		ai, aok := index[a]
		bi, bok := index[b]
		switch {
		case aok && bok:
			return ai - bi
		case aok:
			return -1
		case bok:
			return 1
		}
		return strings.Compare(a, b)
	}
}

// Sort は docs/diagnostics.md「順序」の順に並べ替える。
// ファイルの順（ファイルのない診断はすべての先頭）、同じファイルでは位置のないものが先、
// 次に Start の順、Code の辞書順、Message の辞書順。
// order が nil なら PathOrder を使う。fs は Span のファイルのパスを引くのに使う。
func Sort(diags []Diagnostic, fs *source.FileSet, order FileOrder) {
	if order == nil {
		order = PathOrder
	}
	sort.SliceStable(diags, func(i, j int) bool {
		return compare(&diags[i], &diags[j], fs, order) < 0
	})
}

func compare(a, b *Diagnostic, fs *source.FileSet, order FileOrder) int {
	ap, bp := a.Path(fs), b.Path(fs)
	if ap != bp {
		switch {
		case ap == "":
			return -1
		case bp == "":
			return 1
		}
		if c := order(ap, bp); c != 0 {
			return c
		}
	}
	switch {
	case a.Span == nil && b.Span != nil:
		return -1
	case a.Span != nil && b.Span == nil:
		return 1
	case a.Span != nil && b.Span != nil && a.Span.Start != b.Span.Start:
		return int(a.Span.Start - b.Span.Start)
	}
	if c := strings.Compare(a.Code, b.Code); c != 0 {
		return c
	}
	return strings.Compare(a.Message, b.Message)
}
