package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// engine は、機械の言葉に直したものの置き場（docs/plugin-protocol.md「起動」）と、途中で止められる形かどうかの組ごとに、
// プロセスに一つ。runMu が守る。
//
// 置き場の下は、WASM の内容ごとのディレクトリに分ける（最後に使った時刻を、hozoc が、ディレクトリの変更の時刻として持てる
// ように: wazero の置き場のファイルの名前は、外から求められない）。wazero の置き場は、実行環境一つに一つなので、WASM の内容
// ごとに、実行環境を一つ作る。
//
// 途中で止められる形（wazero の WithCloseOnContextDone）は、WASM の中に、止まったかを見る点を入れるので、遅い。なので、
// 止める口（Context）を渡した起動だけが、その形を使う。二つの形は、機械の言葉が違うので、置き場も、分ける: wazero の
// 置き場は、形の違いを、鍵に入れていない。
type engine struct {
	place     string // 置き場（"" は、覚えない）
	stoppable bool
	loaded    map[[sha256.Size]byte]*loaded // WASM の内容 → 機械の言葉に直したもの
	files     map[string]wasmFile           // WASM のファイルのパス → 前に読んだときの内容の鍵
}

// loaded は、WASM の内容一つの、実行環境と、機械の言葉に直したもの。
type loaded struct {
	rt       wazero.Runtime
	compiled wazero.CompiledModule
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

// cacheLimit は、置き場の大きさの上限（バイト。docs/plugin-protocol.md「起動」）。変えるのは、テストだけ。
var cacheLimit int64 = 1 << 30

// usedEntries は、このプロセスが使った、置き場のもの（WASM の内容のディレクトリ）のパス。上限を超えても、消さない。runMu が守る。
var usedEntries = map[string]bool{}

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

// engineFor は、cacheDir の engine。stoppable なら、途中で止められる形。
func engineFor(cacheDir string, stoppable bool) *engine {
	key := engineKey{cachePlace(cacheDir, stoppable), stoppable}
	if e := engines[key]; e != nil {
		return e
	}
	e := &engine{place: key.place, stoppable: stoppable, loaded: map[[sha256.Size]byte]*loaded{}, files: map[string]wasmFile{}}
	engines[key] = e
	return e
}

// module は、WASM のファイル path を、機械の言葉に直したものと、その実行環境。同じ内容は、プロセスの中で、一度だけ直す。
func (e *engine) module(path string) (*loaded, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("マニフェストの wasm のファイル %s がない: %w", path, err)
	}
	if f, ok := e.files[path]; ok && f.size == info.Size() && f.modTime == info.ModTime().UnixNano() {
		if l := e.loaded[f.key]; l != nil {
			return l, nil
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("マニフェストの wasm のファイル %s を読めない: %w", path, err)
	}
	key := sha256.Sum256(data)
	e.files[path] = wasmFile{size: info.Size(), modTime: info.ModTime().UnixNano(), key: key}
	if l := e.loaded[key]; l != nil {
		return l, nil
	}
	entry := ""
	if e.place != "" {
		entry = filepath.Join(e.place, hex.EncodeToString(key[:]))
	}
	rt, err := e.runtime(entry)
	if err != nil {
		return nil, fmt.Errorf("WASM の実行環境を作れない: %w", err)
	}
	// 機械の言葉に直すのは、関数ごとに、並べて行える（はじめての実行を、速くする）
	m, err := rt.CompileModule(experimental.WithCompilationWorkers(context.Background(), runtime.GOMAXPROCS(0)), data)
	if err != nil {
		_ = rt.Close(context.Background())
		return nil, fmt.Errorf("%s を WASM として読めない: %w", path, err)
	}
	l := &loaded{rt: rt, compiled: m}
	e.loaded[key] = l
	if entry != "" {
		// 最後に使った時刻を、今にして、上限を超えた分を、消す（docs/plugin-protocol.md「起動」）
		usedEntries[entry] = true
		now := time.Now()
		_ = os.Chtimes(entry, now, now)
		trimCache(filepath.Dir(e.place))
	}
	return l, nil
}

// runtime は、WASM の内容一つのための実行環境。entry（WASM の内容のディレクトリ）が "" か、作れなければ、覚えない。
func (e *engine) runtime(entry string) (wazero.Runtime, error) {
	var cache wazero.CompilationCache
	if entry != "" {
		if c, err := wazero.NewCompilationCacheWithDir(entry); err == nil {
			cache = c
		}
	}
	if cache == nil {
		cache = wazero.NewCompilationCache()
	}
	ctx := context.Background()
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithCloseOnContextDone(e.stoppable).WithCompilationCache(cache))
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, err
	}
	return rt, nil
}

// cacheEntry は、置き場のもの一つ: パス、大きさ、最後に使った時刻。
type cacheEntry struct {
	path    string
	size    int64
	modTime time.Time
}

// trimCache は、base（HOZO_CACHE_DIR か、ふだんの置き場）の下の wasm/ と wasm-stoppable/ の、ものの大きさの合計が、
// cacheLimit を超えていれば、最後に使った時刻の古いものから消して、上限の内に戻す（docs/plugin-protocol.md「起動」）。
// このプロセスが使ったものと、消せないものは、飛ばす。誤りは、返さない。
func trimCache(base string) {
	var entries []cacheEntry
	var total int64
	for _, sub := range []string{"wasm", "wasm-stoppable"} {
		dir := filepath.Join(base, sub)
		list, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, d := range list {
			info, err := d.Info()
			if err != nil {
				continue
			}
			c := cacheEntry{path: filepath.Join(dir, d.Name()), modTime: info.ModTime()}
			_ = filepath.WalkDir(c.path, func(_ string, f os.DirEntry, err error) error {
				if err == nil && f.Type().IsRegular() {
					if fi, err := f.Info(); err == nil {
						c.size += fi.Size()
					}
				}
				return nil
			})
			entries = append(entries, c)
			total += c.size
		}
	}
	if total <= cacheLimit {
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].modTime.Equal(entries[j].modTime) {
			return entries[i].modTime.Before(entries[j].modTime)
		}
		return entries[i].path < entries[j].path
	})
	for _, c := range entries {
		if total <= cacheLimit {
			return
		}
		if usedEntries[c.path] {
			continue
		}
		if os.RemoveAll(c.path) == nil {
			total -= c.size
		}
	}
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
	l, err := engineFor(cacheDir, ctx.Done() != nil).module(abs)
	if err != nil {
		return 0, err
	}
	cfg := wazero.NewModuleConfig().WithName("").WithStdin(stdin).WithStdout(stdout).WithStderr(stderr)
	if dir, ok := testMount(abs); ok {
		cfg = cfg.WithFSConfig(wazero.NewFSConfig().WithDirMount(dir, "/"))
	}
	mod, err := l.rt.InstantiateModule(ctx, l.compiled, cfg)
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
