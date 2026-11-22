// Package wasmbuild は、このリポジトリのプラグインを、WASM にビルドする（docs/testing.md「プラグインの WASM」）。hozoc が
// 起動するのは WASM だけ（docs/plugin-protocol.md「マニフェスト」の wasm）で、WASM はリポジトリに入れないので、テストが、
// これで、自分で作る。hozoc 本体は、このパッケージを使わない。
package wasmbuild

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	hostplugin "github.com/mew-ton/hozo/internal/plugin"
)

// Build は、ディレクトリ pkgDir の main パッケージを、WASM（WASI preview 1 の、コマンドの形）にビルドして、out に置く。
// 中身が変わっていなければ、go build は、作り直さない。テストのパッケージは、並んで走るので、同じ out を、同時には作らない
// （鍵のファイルで、順にする）。
func Build(pkgDir, out string) error {
	out, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	unlock, err := lock(out)
	if err != nil {
		return err
	}
	defer unlock()
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = pkgDir
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	if msg, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s を WASM にビルドできない: %v\n%s", pkgDir, err, msg)
	}
	return nil
}

// Repo は、root の下の、マニフェスト（hozo-plugin.toml）のあるディレクトリのうち、テストでない Go のファイルを持つもの
// すべてを、WASM にビルドして、それぞれのマニフェストの wasm の場所に置く。
func Repo(root string) error {
	type target struct{ dir, out string }
	var targets []target
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != hostplugin.ManifestFileName {
			return nil
		}
		dir := filepath.Dir(path)
		if !hasGoSources(dir) {
			return nil
		}
		m, diags := hostplugin.LoadManifest(path)
		if m == nil {
			var msgs []string
			for _, d := range diags {
				msgs = append(msgs, d.Code+": "+d.Message)
			}
			return fmt.Errorf("%s を読めない: %s", path, strings.Join(msgs, "; "))
		}
		targets = append(targets, target{dir, m.WasmPath(dir)})
		return nil
	})
	if err != nil {
		return err
	}
	errs := make([]error, len(targets))
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = Build(t.dir, t.out)
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

// hasGoSources は、dir に、テストでない Go のファイルがあるか（プラグインの実装がある: main パッケージの置き場）。
func hasGoSources(dir string) bool {
	sources, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	for _, f := range sources {
		if !strings.HasSuffix(f, "_test.go") {
			return true
		}
	}
	return false
}

// HasGoSources は、プラグインのディレクトリ dir に、ビルドする実装（テストでない Go のファイル）があるか。
func HasGoSources(dir string) bool { return hasGoSources(dir) }
