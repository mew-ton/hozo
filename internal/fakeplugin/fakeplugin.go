// Package fakeplugin は、テストが使う、にせもののプラグインの置き方（docs/testing.md「環境からの独立」）。にせものの中身は
// cmd/fakeplugin（一度だけビルドする）で、振る舞いは、プラグインのディレクトリのファイルで決める（そのコマンドの説明）。
package fakeplugin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var (
	buildOnce sync.Once
	binary    string
	buildErr  error
)

// Binary は、にせもののプラグインの実行ファイルのパス。はじめて呼ばれたときに、一時ディレクトリにビルドする
// （テストのプロセスが終わるまで、残る）。
func Binary(t testing.TB) string {
	t.Helper()
	buildOnce.Do(func() {
		_, self, _, _ := runtime.Caller(0)
		dir, err := os.MkdirTemp("", "hozo-fakeplugin")
		if err != nil {
			buildErr = err
			return
		}
		binary = filepath.Join(dir, "fakeplugin")
		if runtime.GOOS == "windows" {
			binary += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", binary, "./cmd/fakeplugin")
		cmd.Dir = filepath.Dir(self)
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("fakeplugin をビルドできない: %v\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return binary
}

// CommandTOML は、マニフェストの command に書く TOML の値: Install が置く、起動のスクリプト。実行ファイルを、マニフェストの
// ディレクトリからの相対パスで指す（docs/plugin-protocol.md「マニフェスト」）ので、プラグインのディレクトリを、動かしても、動く。
const CommandTOML = `["./plugin.sh"]`

// CommandArgs は、CommandTOML と同じ起動の、引数の並び。
var CommandArgs = []string{"./plugin.sh"}

// Install は、プラグインのディレクトリ pluginDir に、にせものを起動するスクリプト plugin.sh を置く。prelude は、にせものを
// 起動する前に走らせる、シェルの行（"" なら、なし。その中では、$d が、プラグインのディレクトリ）。hozoc は、プラグインに、
// 環境変数を足さない（docs/plugin-protocol.md「起動」）ので、プラグインのディレクトリは、起動されたパス（$0）から求める。
// sh がなければ、テストを飛ばす。
func Install(t testing.TB, pluginDir, prelude string) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh がない")
	}
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nd=$(dirname \"$0\")\n" + prelude
	if prelude != "" && prelude[len(prelude)-1] != '\n' {
		script += "\n"
	}
	script += "exec " + shellQuote(Binary(t)) + " \"$d\"\n"
	path := filepath.Join(pluginDir, "plugin.sh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// shellQuote は、s を、sh の、一重引用符の文字列にする。
func shellQuote(s string) string {
	out := "'"
	for _, r := range s {
		if r == '\'' {
			out += `'\''`
		} else {
			out += string(r)
		}
	}
	return out + "'"
}
