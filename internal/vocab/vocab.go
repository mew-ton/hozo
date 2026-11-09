// Package vocab は埋め込んだ標準の語彙と、プラグインの語彙の読み込み（docs/vocab.md「読み込み」）。
//
// 標準の語彙 vocab/*.hozo は hozoc のバイナリに埋め込み（埋め込み自体はリポジトリ直下の vocab パッケージ。
// go:embed はパッケージの外を参照できないため）、hozoc check / emit / lsp がプロジェクトのファイルの一部として読む。
// パスの形は docs/ir.md「Program」の Span.file: 標準の語彙は "hozo:vocab/<名前>.hozo"、
// プラグインの語彙は "hozo:plugin/<プラグイン名>/<マニフェストからの相対パス>"。
//
// internal/source には依存しない。FileSet への追加は AddTo に fs.Add を包んだ関数を渡す:
//
//	vocab.AddTo(func(path string, content []byte) { fs.Add(path, content) })
package vocab

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	embedded "github.com/mew-ton/hozo/vocab"
)

// Prefix は語彙のファイルのパスに共通の接頭辞。IR の原則（docs/ir.md「原則」）は
// Span.file がこの接頭辞で始まるファイルを語彙のファイルとして、ユーザーのファイルの後ろに並べる。
const Prefix = "hozo:"

// VocabPrefix は標準の語彙のパスの接頭辞 "hozo:vocab/"。
const VocabPrefix = Prefix + "vocab/"

// PluginPrefix はプラグインの語彙のパスの接頭辞 "hozo:plugin/"。
const PluginPrefix = Prefix + "plugin/"

// File は語彙のファイル一つ。Path は Span.file の形。
type File struct {
	Path    string
	Content []byte
}

// Files は同梱する標準の語彙のすべて（docs/vocab.md「同梱する語彙」）。パスは "hozo:vocab/<名前>.hozo" で、
// 辞書順に並ぶ（docs/ir.md「原則」のファイルの順）。
func Files() []File {
	entries, err := fs.ReadDir(embedded.Files, ".")
	if err != nil {
		panic("vocab: " + err.Error())
	}
	var files []File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".hozo") {
			continue
		}
		content, err := embedded.Files.ReadFile(e.Name())
		if err != nil {
			panic("vocab: " + err.Error())
		}
		files = append(files, File{Path: Path(e.Name()), Content: content})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
}

// Path は標準の語彙のファイル名（"valid.hozo"）から Span.file の形 "hozo:vocab/valid.hozo" を作る。
func Path(name string) string { return VocabPrefix + name }

// PluginPath はプラグインの語彙の Span.file の形 "hozo:plugin/<プラグイン名>/<マニフェストからの相対パス>" を作る。
// 相対パスの区切りは OS に関わらず "/" に正規化する。
func PluginPath(plugin, rel string) string {
	return PluginPrefix + plugin + "/" + path.Clean(filepath.ToSlash(rel))
}

// IsVocab は p が語彙のファイル（標準かプラグイン）のパスかどうか。
func IsVocab(p string) bool { return strings.HasPrefix(p, Prefix) }

// AddTo は標準の語彙のすべてを add に渡す。add は source.FileSet.Add を包んだもの。
func AddTo(add func(path string, content []byte)) {
	for _, f := range Files() {
		add(f.Path, f.Content)
	}
}

// PluginFiles はプラグインのマニフェストの vocabulary（docs/plugin-protocol.md「マニフェスト」）の .hozo を読む。
// plugin はプラグインの名前、dir はマニフェストのディレクトリ、rels はマニフェストからの相対パス。
// パスは "hozo:plugin/<プラグイン名>/<相対パス>"。ディレクトリの外に出る相対パスはエラー。
func PluginFiles(plugin, dir string, rels []string) ([]File, error) {
	var files []File
	for _, rel := range rels {
		clean := filepath.Clean(filepath.FromSlash(rel))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("vocab: プラグイン %s の vocabulary %q はマニフェストのディレクトリの外を指す", plugin, rel)
		}
		content, err := os.ReadFile(filepath.Join(dir, clean))
		if err != nil {
			return nil, fmt.Errorf("vocab: プラグイン %s の vocabulary %q を読めない: %w", plugin, rel, err)
		}
		files = append(files, File{Path: PluginPath(plugin, rel), Content: content})
	}
	return files, nil
}

// AddPluginTo は PluginFiles の結果を add に渡す。
func AddPluginTo(add func(path string, content []byte), plugin, dir string, rels []string) error {
	files, err := PluginFiles(plugin, dir, rels)
	if err != nil {
		return err
	}
	for _, f := range files {
		add(f.Path, f.Content)
	}
	return nil
}
