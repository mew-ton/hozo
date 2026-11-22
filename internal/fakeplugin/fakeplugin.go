// Package fakeplugin は、テストが使う、にせもののプラグインの置き方（docs/testing.md「環境からの独立」）。にせものの中身は
// cmd/fakeplugin（WASM に、一度だけビルドする）で、振る舞いは、プラグインのディレクトリのファイルで決める（そのコマンドの説明）。
package fakeplugin

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	hostplugin "github.com/mew-ton/hozo/internal/plugin"
	"github.com/mew-ton/hozo/internal/wasmbuild"
)

var (
	buildOnce sync.Once
	wasm      string
	buildErr  error
)

// Wasm は、にせもののプラグインの WASM のファイルのパス。はじめて呼ばれたときに、OS の一時ディレクトリ（リポジトリの
// 置き場ごとに、決まった場所）にビルドする。中身が変わっていなければ、作り直さない。
func Wasm(t testing.TB) string {
	t.Helper()
	path, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// Build は、Wasm と同じ（テストの外から呼ぶ形: TestMain）。
func Build() (string, error) {
	buildOnce.Do(func() {
		_, self, _, _ := runtime.Caller(0)
		src := filepath.Join(filepath.Dir(self), "cmd", "fakeplugin")
		sum := sha256.Sum256([]byte(src))
		wasm = filepath.Join(os.TempDir(), "hozo-fakeplugin-"+hex.EncodeToString(sum[:8]), WasmFile)
		buildErr = wasmbuild.Build(src, wasm)
	})
	return wasm, buildErr
}

// WasmFile は、Install が置く、にせものの WASM の、ファイルの名前。マニフェストの wasm には、これを書く
// （マニフェストのディレクトリからの相対パス。docs/plugin-protocol.md「マニフェスト」）。
const WasmFile = "plugin.wasm"

// WasmTOML は、マニフェストの wasm に書く TOML の値。
const WasmTOML = `"` + WasmFile + `"`

// Install は、プラグインのディレクトリ pluginDir に、にせものの WASM（WasmFile）を置き、そのディレクトリを、にせものに見せる
// （振る舞いを決めるファイルと、記録の置き場。internal/plugin の MountForTest。テストが終われば、取り消す）。
func Install(t testing.TB, pluginDir string) {
	t.Helper()
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(pluginDir, WasmFile)
	Place(t, path)
	t.Cleanup(hostplugin.MountForTest(path, pluginDir))
}

// Place は、にせものの WASM を、path に置く（見せるディレクトリは、ない: にせものは、頼まれたトランスパイラごとに、ファイルを
// 返さない答えを返す）。実装のないプラグインの代役に使う。
func Place(t testing.TB, path string) {
	t.Helper()
	if err := PlaceAt(path); err != nil {
		t.Fatal(err)
	}
}

// PlaceAt は、Place と同じ（テストの外から呼ぶ形: TestMain）。
func PlaceAt(path string) error {
	src, err := Build()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	_ = os.Remove(path)
	if os.Link(src, path) == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// DesignDecorators は、設計書のコーパス（tests/corpus/design）が、単位の対象の宣言に付ける、本体のない、@hint でないデコレータの FQN。
// コーパスの .hozo を、にせもののトランスパイラだけの設定で使うテストが、にせものの [[transpiler]] の interprets に書く（[Interprets]）:
// 書かなければ、付けた所ごとに CFG-26（解釈するトランスパイラがいない。docs/config.md「解釈するトランスパイラ」）が出て、
// そのテストが見たい診断に、混ざる。コーパスに、新しいデコレータを足したら、ここに足す（足し忘れは、CFG-26 が、テストに出て分かる）。
// コーパス自身の設定（基底 go だけ）での CFG-26 は、ゴールデン tests/corpus/design/testdata/expected/check.txt が持つ。
var DesignDecorators = []string{
	"browser.ignoreFailure", "browser.unavailableOnFailure",
	"db.key", "db.ref", "db.table", "db.unique",
	"http.body", "http.bodyObj", "http.default", "http.get", "http.header", "http.post", "http.query", "http.status",
	"localStorage",
	"page.dialog", "page.fallback", "page.interaction", "page.path", "page.query", "page.route",
	"server.validated",
	"valid.checkDigit", "valid.format", "valid.length", "valid.min", "valid.pattern",
}

// Interprets は、マニフェストの [[transpiler]] に足す一行: interprets = [fqns...]（docs/plugin-protocol.md「マニフェスト」）。
func Interprets(fqns ...string) string {
	quoted := make([]string, len(fqns))
	for i, f := range fqns {
		quoted[i] = `"` + f + `"`
	}
	return "interprets = [" + strings.Join(quoted, ", ") + "]\n"
}
