package lsp

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/mew-ton/hozo/internal/vocab"
)

// pathToURI はプロジェクトのファイルのパス（docs/ir.md「Program」の Span.file の三つの形）を URI にする。
// 語彙のファイル（"hozo:" で始まる）はそのまま URI（docs/lsp.md「プロジェクト」）。ユーザーのファイルはルートからの
// 相対パスなので root と合わせて file: の URI にする。絶対パス（ルートの外のマニフェストなど）もそのまま file: にする。
func pathToURI(root, path string) string {
	if vocab.IsVocab(path) {
		return path
	}
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, filepath.FromSlash(path))
	}
	return fileURI(abs)
}

// fileURI は絶対パスを file: の URI にする。
func fileURI(abs string) string {
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // Windows のドライブ文字
	}
	u := url.URL{Scheme: "file", Path: p}
	return u.String()
}

// uriToPath は URI をプロジェクトのファイルのパスにする。"hozo:" の URI はそのまま、file: の URI は root からの
// 相対パス（"/" 区切り）にする。ルートの外なら絶対パス。読めない URI なら空文字列と false。
func uriToPath(root, uri string) (string, bool) {
	if vocab.IsVocab(uri) {
		return uri, true
	}
	abs, ok := uriToFilePath(uri)
	if !ok {
		return "", false
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return abs, true
	}
	return filepath.ToSlash(rel), true
}

// uriToFilePath は file: の URI を OS の絶対パスにする。
func uriToFilePath(uri string) (string, bool) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return "", false
	}
	p := u.Path
	if p == "" {
		p = u.Opaque
	}
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' { // /c:/... （Windows）
		p = p[1:]
	}
	return filepath.Clean(filepath.FromSlash(p)), true
}
