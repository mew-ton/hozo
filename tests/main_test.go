package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mew-ton/hozo/internal/fakeplugin"
	"github.com/mew-ton/hozo/internal/wasmbuild"
)

// TestMain は、テストの初めに、このリポジトリのプラグインを、WASM にビルドする（docs/testing.md「プラグインの WASM」）。hozoc が
// 起動するのは WASM だけで、WASM はリポジトリに入れないので、コーパス、サンプル、生成物の一致のテストは、これが置いたものを使う。
// コーパス model の、何も出さないプラグイン none の中身は、にせもののプラグイン（internal/fakeplugin）。
func TestMain(m *testing.M) {
	repo, err := filepath.Abs("..")
	if err == nil {
		err = wasmbuild.Repo(repo)
	}
	if err == nil {
		err = fakeplugin.PlaceAt(filepath.Join(modelCorpus, "plugins", "none", fakeplugin.WasmFile))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "プラグインの WASM を用意できない:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
