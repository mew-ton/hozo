package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/mew-ton/hozo/internal/config"
	"github.com/mew-ton/hozo/internal/gen/hozo/compiler"
)

// このファイルは、状態の分類 OutputsSync（spec/cli.hozo の `classify OutputsSync of outputs`）の単位の実装と、生成したファイルと
// 出力先ごとの記録に触れる補助。

// OutputsFile は、出力（単位と名前空間の組）ごとの Dir(U, N) に置く、前回書いたパスの一覧（docs/cli.md「書き出し」の 5 と 7）。
// JSON の文字列の配列で、パスはルートからの相対パス（"/" 区切り）、辞書順。
const OutputsFile = ".hozoc-outputs.json"

// OutputsSync は、状態の分類 OutputsSync の単位の実装: 述語 UpToDate と OutOfDate（問い合わせ）と、効果 syncOutputs（命令）。
// compiler.New の引数 outputsSync に渡す（[App.Ops]）。
func (a *App) OutputsSync() compiler.OutputsSyncState { return outputsSync{a} }

// outputsSync は [App.OutputsSync] が返す値。
type outputsSync struct{ a *App }

// SyncOutputs は spec/cli.hozo の効果 syncOutputs（on OutputsSync: any -> UpToDate。docs/cli.md「書き出し」の 5〜7）: 生成したファイルを、
// 書き出す予定 plan に合わせる。ユースケース write の配線が、collectOutputs と outputChanges の後に呼ぶ。plan は、直前の
// CollectOutputs が返したもの（置き場は [App.plannedFor] から取る）。
//
//  5. 対象の各出力の <Dir(U, N)>/.hozoc-outputs.json を読み、今回書かないパスのファイルを消す（[outputPlan.stale]）。空になった
//     ディレクトリも、Dir(U, N) の下の範囲で消す。
//  6. 辞書順にファイルを書く。内容が同じなら書き直さない。
//  7. .hozoc-outputs.json を書く。書くものが一つもない出力では作らず、前回のものがあれば消す。
//
// 途中で失敗したら、そこまでに書いたものは残る（原子性は持たない）。
func (s outputsSync) SyncOutputs(ctx context.Context, plan compiler.EmitPlan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	pl, err := s.a.plannedFor(plan)
	if err != nil {
		return err
	}
	// 5. 前回の出力のうち今回書かないものを消す
	stale, err := pl.stale()
	if err != nil {
		return err
	}
	for _, f := range stale {
		if err := os.Remove(filepath.Join(pl.root, filepath.FromSlash(f.path))); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("hozoc: %s を消せない: %w", f.path, err)
		}
		removeEmptyDirs(pl.root, f.dir, f.path)
	}

	// 6. 書く（内容が同じなら書き直さない）。plan.Files は、パスの辞書順
	for _, f := range plan.Files {
		if err := writeIfChanged(filepath.Join(pl.root, filepath.FromSlash(string(f.Path))), []byte(f.Content)); err != nil {
			return fmt.Errorf("hozoc: %s を書けない: %w", f.Path, err)
		}
	}

	// 7. .hozoc-outputs.json
	for _, dir := range pl.dirs {
		record := filepath.Join(pl.root, filepath.FromSlash(dir), OutputsFile)
		data := pl.record(dir)
		if data == nil {
			// 何も書かない出力（どのトランスパイラもファイルを返さなかった、in で出力でなくなった）には記録も置かない
			if err := os.Remove(record); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("hozoc: %s/%s を消せない: %w", dir, OutputsFile, err)
			}
			continue
		}
		if err := writeIfChanged(record, data); err != nil {
			return fmt.Errorf("hozoc: %s/%s を書けない: %w", dir, OutputsFile, err)
		}
	}
	return nil
}

// UpToDate は spec/cli.hozo の分類 OutputsSync の述語（docs/cli.md「cli.hozo」「手で書くもの」）: 生成したファイルが、今の仕様と
// 設定から生成し直したものと一致しているか。生成したコードでは、状態の分類 OutputsSync の単位の問い合わせになり、判定関数
// compiler.ClassifyOutputsSync が呼ぶ。
//
// すべての出力について書き出す予定（[App.collect]。トランスパイラの答えから）を作り、ディスクと比べる。一致していると言うのは、
// 書く予定のファイルがすべて同じ内容であり、.hozoc-outputs.json が今回の一覧と同じで（何も書かない出力にはなく）、
// 前回書いて今回は書かないファイルが残っていないとき。emit を走らせても何も書かず何も消さない、ということ。
// 仕様や設定に誤りがあって生成し直せなければ、その例外を返す（check / emit と同じもの）。
//
// 見るのは、直前の Check が解析した Project（同じ App で check か emit を走らせた後。プラグインの答えを含む）。なければ、
// 作業ディレクトリから hozo.toml を探して解析し、プラグインに頼む。直前の CollectOutputs の予定（a.planned）は、使わず、変えない。
func (s outputsSync) UpToDate(ctx context.Context) (bool, error) {
	a := s.a
	if err := ctx.Err(); err != nil {
		return false, err
	}
	p := a.checked
	if p == nil {
		var err error
		if p, err = a.load(true); err != nil {
			return false, err
		}
	}
	if p.NoConfig {
		return false, noConfig(p)
	}
	pl, err := a.collect(ctx, p, nil)
	if err != nil {
		return false, err
	}
	for _, o := range pl.outputs {
		if old, err := os.ReadFile(filepath.Join(pl.root, filepath.FromSlash(o.path))); err != nil || !bytes.Equal(old, o.content) {
			return false, nil // 足りないか、古い
		}
	}
	for _, dir := range pl.dirs {
		record, err := os.ReadFile(filepath.Join(pl.root, filepath.FromSlash(dir), OutputsFile))
		if err != nil && !os.IsNotExist(err) {
			return false, fmt.Errorf("hozoc: %s/%s を読めない: %w", dir, OutputsFile, err)
		}
		if want := pl.record(dir); !bytes.Equal(record, want) {
			// 記録が今回の一覧と違う。前回書いて今回は書かないファイルが残っていれば、それは余分
			return false, nil
		}
	}
	return true, nil
}

// OutOfDate は spec/cli.hozo の分類 OutputsSync の述語（docs/cli.md「cli.hozo」「手で書くもの」）: 生成したファイルに、足りない、
// 古い、または余分なものがあるか。UpToDate の否定で、同じ比べ方を使う（分類のケースは、どれも述語を持つ: docs/check.md
// 「ケースの判定」）。生成し直せなければ、UpToDate と同じ例外を返す。
func (s outputsSync) OutOfDate(ctx context.Context) (bool, error) {
	upToDate, err := s.UpToDate(ctx)
	if err != nil {
		return false, err
	}
	return !upToDate, nil
}

// staleFile は、前回書いて、今回は書かないファイル一つ。
type staleFile struct {
	path string // ルートからの相対パス（"/" 区切り）
	dir  string // 記録の持ち主の Dir(U, N)
}

// stale は、予定 pl の対象の各出力の記録（<Dir(U, N)>/.hozoc-outputs.json。前回書いたパスの一覧）にあって、今回は書かないパスの
// うち、ディスクにあるもの（docs/cli.md「書き出し」の 5 が消すファイル）。並びは、対象の出力の順、その中は記録の順。読むだけで、
// 何も変えない: 何を消すかを決める read outputChanges（[outputsState.OutputChanges]）と、消す効果 syncOutputs
// （[outputsSync.SyncOutputs]）が、同じこの一覧を使う。
//
// 記録のパスに従うのは、その記録の持ち主の Dir(U, N) の下のものだけ（同 5）。それ以外（`..`、絶対パス、別のディレクトリ）は
// hozoc がこの記録に書くはずのないパスなので、消さず、報告もしない。
func (pl *outputPlan) stale() ([]staleFile, error) {
	current := map[string]bool{}
	for _, o := range pl.outputs {
		current[o.path] = true
	}
	var out []staleFile
	seen := map[string]bool{}
	for _, dir := range pl.dirs {
		previous, err := readOutputsFile(pl.root, dir)
		if err != nil {
			return nil, err
		}
		for _, prev := range previous {
			if current[prev] || seen[prev] || !config.Under(prev, dir) {
				continue
			}
			if _, err := os.Lstat(filepath.Join(pl.root, filepath.FromSlash(prev))); os.IsNotExist(err) {
				continue // 記録にあっても、もうないファイル
			}
			seen[prev] = true
			out = append(out, staleFile{path: prev, dir: dir})
		}
	}
	return out, nil
}

// readOutputsFile は <root>/<dir>/.hozoc-outputs.json を読む。ファイルがなければ空。パスは正規化するだけで、
// dir の下かどうかは確かめない（従うかどうかは呼ぶ側が決める。[outputPlan.stale]、[outputsState.Get]）。
func readOutputsFile(root, dir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dir), OutputsFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("hozoc: %s/%s を読めない: %w", dir, OutputsFile, err)
	}
	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return nil, fmt.Errorf("hozoc: %s/%s を読めない: %w", dir, OutputsFile, err)
	}
	for i, p := range paths {
		paths[i] = config.CleanRel(p)
	}
	return paths, nil
}

// removeEmptyDirs は、消したファイル removed（ルートからの相対パス）のディレクトリが空になっていれば、出力先 dir の下の範囲で、
// そのディレクトリも消す（docs/cli.md「書き出し」の 5: Go の基底の内側のフォルダ inner が要らなくなったとき）。親へ向かって、
// 空でないディレクトリか dir そのものに当たるまで続ける。dir そのものは消さない。
func removeEmptyDirs(root, dir, removed string) {
	for d := path.Dir(removed); config.Under(d, dir); d = path.Dir(d) {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(d))); err != nil {
			return // 空でない（か、消せない）。そこで止める
		}
	}
}

// writeIfChanged は内容が違うときだけ書く。ディレクトリは作る。
func writeIfChanged(abs string, content []byte) error {
	if old, err := os.ReadFile(abs); err == nil && bytes.Equal(old, content) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, content, 0o644)
}
