package plugin

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// engine は、WASM の実行環境一つ: 機械の言葉に直したものの置き場（docs/plugin-protocol.md「起動」）と、途中で止められる形か
// どうかの組ごとに、プロセスに一つ。runMu が守る。
//
// 途中で止められる形（wazero の WithCloseOnContextDone）は、WASM の中に、止まったかを見る点を入れるので、遅い。なので、
// 止める口（Context）を渡した起動だけが、その形を使う。二つの形は、機械の言葉が違うので、置き場も、分ける: wazero の
// 置き場は、形の違いを、鍵に入れていない。
type engine struct {
	rt       wazero.Runtime
	compiled map[[sha256.Size]byte]wazero.CompiledModule // WASM の内容 → 機械の言葉に直したもの
	files    map[string]wasmFile                         // WASM のファイルのパス → 前に読んだときの内容の鍵
}

// wasmFile は、前に読んだ WASM のファイルの、大きさと、変更の時刻と、内容の鍵。同じなら、読み直さない。
type wasmFile struct {
	size    int64
	modTime int64
	key     [sha256.Size]byte
}

// engineKey は、engine の鍵: 置き場（"" は、覚えない）と、途中で止められる形か。
type engineKey struct {
	place     string
	stoppable bool
}

// engines は、鍵ごとの engine。runMu が守る。
var engines = map[engineKey]*engine{}

// cachePlace は、機械の言葉に直したものの置き場: cacheDir（HOZO_CACHE_DIR）が "" でなければ、その下、"" なら、
// os.UserCacheDir の下の hozo の下の、wasm（途中で止められる形は wasm-stoppable）。決められなければ ""。
func cachePlace(cacheDir string, stoppable bool) string {
	if cacheDir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return ""
		}
		cacheDir = filepath.Join(base, "hozo")
	}
	if stoppable {
		return filepath.Join(cacheDir, "wasm-stoppable")
	}
	return filepath.Join(cacheDir, "wasm")
}

// engineFor は、cacheDir の engine。stoppable なら、途中で止められる形。置き場を作れなければ、覚えずに動く engine。
func engineFor(cacheDir string, stoppable bool) (*engine, error) {
	key := engineKey{cachePlace(cacheDir, stoppable), stoppable}
	if e := engines[key]; e != nil {
		return e, nil
	}
	var cache wazero.CompilationCache
	if key.place != "" {
		c, err := wazero.NewCompilationCacheWithDir(key.place)
		if err != nil {
			// 置き場を作れない: 覚えずに動く
			return engineFor0(engineKey{"", stoppable}, wazero.NewCompilationCache())
		}
		cache = c
	} else {
		cache = wazero.NewCompilationCache()
	}
	return engineFor0(key, cache)
}

// engineFor0 は、鍵 key の engine を、なければ、cache で作る。
func engineFor0(key engineKey, cache wazero.CompilationCache) (*engine, error) {
	if e := engines[key]; e != nil {
		return e, nil
	}
	ctx := context.Background()
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithCloseOnContextDone(key.stoppable).WithCompilationCache(cache))
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, err
	}
	e := &engine{rt: rt, compiled: map[[sha256.Size]byte]wazero.CompiledModule{}, files: map[string]wasmFile{}}
	engines[key] = e
	return e, nil
}

// module は、WASM のファイル path を、機械の言葉に直したもの。同じ内容は、プロセスの中で、一度だけ直す。
func (e *engine) module(path string) (wazero.CompiledModule, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("マニフェストの wasm のファイル %s がない: %w", path, err)
	}
	if f, ok := e.files[path]; ok && f.size == info.Size() && f.modTime == info.ModTime().UnixNano() {
		if m := e.compiled[f.key]; m != nil {
			return m, nil
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("マニフェストの wasm のファイル %s を読めない: %w", path, err)
	}
	key := sha256.Sum256(data)
	e.files[path] = wasmFile{size: info.Size(), modTime: info.ModTime().UnixNano(), key: key}
	if m := e.compiled[key]; m != nil {
		return m, nil
	}
	// 機械の言葉に直すのは、関数ごとに、並べて行える（はじめての実行を、速くする）
	m, err := e.rt.CompileModule(experimental.WithCompilationWorkers(context.Background(), runtime.GOMAXPROCS(0)), data)
	if err != nil {
		return nil, fmt.Errorf("%s を WASM として読めない: %w", path, err)
	}
	e.compiled[key] = m
	return m, nil
}

// runWasm は、WASM のファイル path を、サンドボックスの中で、一度動かす（WASI の _start）。終了コードを返す。誤りは、起動の
// 失敗（ファイルがない、WASM として読めない）、WASM の罠、ctx で止めたとき。途中で止められるのは、ctx が、止める口を
// 持つとき（ctx.Done() が nil でない）だけ。
//
// サンドボックス（docs/plugin-protocol.md「起動」）: 渡すのは、標準入力、標準出力、標準エラーだけ。wazero の ModuleConfig は、
// 言わなければ、ファイルも、環境変数も、引数も、渡さず、時計は、決まった値から、読むたびに、決まった幅で進む、にせもの、乱数は、
// 決まった並び、眠りは、待たない。ネットワークは、WASI preview 1 に、つなぐ口がなく、待ち受けるソケットも、渡していない。
func runWasm(ctx context.Context, cacheDir, path string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}
	e, err := engineFor(cacheDir, ctx.Done() != nil)
	if err != nil {
		return 0, fmt.Errorf("WASM の実行環境を作れない: %w", err)
	}
	compiled, err := e.module(abs)
	if err != nil {
		return 0, err
	}
	cfg := wazero.NewModuleConfig().WithName("").WithStdin(stdin).WithStdout(stdout).WithStderr(stderr)
	if dir, ok := testMount(abs); ok {
		cfg = cfg.WithFSConfig(wazero.NewFSConfig().WithDirMount(dir, "/"))
	}
	mod, err := e.rt.InstantiateModule(ctx, compiled, cfg)
	if mod != nil {
		_ = mod.Close(context.Background())
	}
	if err == nil {
		return 0, nil
	}
	var exit *sys.ExitError
	if errors.As(err, &exit) {
		switch exit.ExitCode() {
		case sys.ExitCodeContextCanceled, sys.ExitCodeDeadlineExceeded:
			return 0, err
		}
		return int(exit.ExitCode()), nil
	}
	return 0, err
}

// testMounts は、テストが、にせもののプラグインに見せるディレクトリ: WASM のファイルの絶対パス → ディレクトリ。
var (
	testMountsMu sync.Mutex
	testMounts   = map[string]string{}
)

// MountForTest は、テストだけの口: WASM のファイル wasm のプラグインを動かすとき、ディレクトリ dir を、その中の "/" として
// 見せる（docs/testing.md「環境からの独立」: にせもののプラグインが、振る舞いを決めるファイルと、記録を、読み書きする）。
// 返す関数で、取り消す。テストのプロセスでなければ panic する。マニフェストからも、設定からも、コマンドからも、ここには届かない。
func MountForTest(wasm, dir string) (undo func()) {
	if !testing.Testing() {
		panic("plugin.MountForTest は、テストの中でだけ呼べる")
	}
	abs, err := filepath.Abs(wasm)
	if err != nil {
		panic(err)
	}
	testMountsMu.Lock()
	testMounts[abs] = dir
	testMountsMu.Unlock()
	return func() {
		testMountsMu.Lock()
		delete(testMounts, abs)
		testMountsMu.Unlock()
	}
}

func testMount(abs string) (string, bool) {
	testMountsMu.Lock()
	defer testMountsMu.Unlock()
	dir, ok := testMounts[abs]
	return dir, ok
}
