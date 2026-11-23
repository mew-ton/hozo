// Command selfgen は `make generate` の中身: hozoc 自身の仕様 cli.hozo から internal/gen/hozo/compiler/** と internal/gen/hozo/cli/**
// （hozo.toml の二つの単位の out。以下、「置き場」）を生成し直す（docs/packages.md「生成するもの」「自己生成の手順」）。
//
// 置き場は cmd/hozoc が import するパッケージなので、hozoc がそこへ直に書くと、プラグインの不具合で
// コンパイルできない Go が出たときに、hozoc 自身がビルドできなくなり、生成し直すこともできなくなる。そうならないように、
// 置き場には、コンパイルできると確かめたものしか置かない:
//
//  1. 今のソース（今の置き場を含む）で hozoc をビルドする。使うプラグイン（plugins/ の go、go-valid、go-cli）を、WASM に
//     ビルドして、それぞれのディレクトリの plugin.wasm に置く（docs/testing.md「プラグインの WASM」）。
//  2. 一時ディレクトリに仕様と設定（cli.hozo、hozo.toml、go.mod、go.sum）を写し、プラグインの置き場 plugins と、3 の vet が
//     要るディレクトリ（ir、plugin、runtime）をシンボリックリンクで足して、そこで hozoc build を走らせる。
//     生成されるコードの import パスは、hozo.toml の [options.go] の module から決まる（docs/plugin-go.md「パッケージと import パス」）
//     ので、リポジトリで生成したときと同じになる。go.mod は、3 の vet とビルドのために写す。
//  3. 生成した置き場の中身を、その一時ディレクトリで vet に通し、次に、リポジトリの今のものの代わりに
//     置いたつもりで（go の -overlay）、それを使うパッケージ（cmd/hozoc、internal/app）ごとビルドする。
//  4. 通ったときだけ、リポジトリの置き場を生成したものに置き換える。通らなければ何も変えず、
//     生成したものを残した一時ディレクトリの場所を示して失敗する。
//
// 使い方（リポジトリのルートで）: go run ./internal/tools/selfgen
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// 自己生成の入力と出力（リポジトリのルートからの相対パス）。
var (
	// genDirs は cli.hozo からの生成物の置き場（hozo.toml の二つの単位の out。docs/cli.md「cli.hozo」）: 名前空間 hozo.compiler
	// （ユースケース）の出力（入口、内側の inner）と、名前空間 hozo.cli（コマンドの行）の出力（入口、go-cli の cli）。
	genDirs = []string{filepath.Join("internal", "gen", "hozo", "compiler"), filepath.Join("internal", "gen", "hozo", "cli")}
	// inputs は一時ディレクトリに写すファイル。ほかに、ルートの仕様のファイル（*.hozo）を、すべて写す（specFiles）。
	inputs = []string{"cli.hozo", "hozo.toml", "go.mod", "go.sum"}
	// links は一時ディレクトリにシンボリックリンクで足すディレクトリ（プラグインの置き場と、生成物の vet が要るパッケージ）。
	links = []string{"ir", "plugin", "plugins", "runtime"}
	// plugins は、cli.hozo の出力に使うプラグイン（hozo.toml の単位の transpilers。plugins/ の下の名前）。生成の前に、WASM に
	// ビルドする: マニフェストの wasm は plugin.wasm（docs/plugin-go.md「マニフェスト」）。
	plugins = []string{"go", "go-valid", "go-cli"}
	// users は、生成物を置き換えたつもりでビルドするパッケージ: 生成物そのものと、それを import するもの。
	users = []string{"./internal/gen/...", "./internal/app/...", "./cmd/hozoc"}
)

func main() {
	repo := flag.String("C", ".", "リポジトリのルート")
	flag.Parse()
	if err := run(*repo, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "selfgen:", err)
		os.Exit(1)
	}
}

func run(repo string, stdout, stderr io.Writer) error {
	repo, err := filepath.Abs(repo)
	if err != nil {
		return err
	}
	for _, name := range inputs[:3] {
		if _, err := os.Stat(filepath.Join(repo, name)); err != nil {
			return fmt.Errorf("%s がない（リポジトリのルートで実行する）: %w", name, err)
		}
	}
	tmp, err := os.MkdirTemp("", "hozoc-selfgen-")
	if err != nil {
		return err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(tmp)
		}
	}()

	// 1. 今のソースで hozoc をビルドする
	hozoc := filepath.Join(tmp, "hozoc")
	if err := command(repo, stdout, stderr, "go", "build", "-o", hozoc, "./cmd/hozoc"); err != nil {
		return fmt.Errorf("hozoc をビルドできない（internal/gen/ が壊れているなら、git で元に戻してからやり直す）: %w", err)
	}

	for _, name := range plugins {
		dir := filepath.Join(repo, "plugins", name)
		wasmEnv := append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
		if err := commandEnv(dir, wasmEnv, stdout, stderr, "go", "build", "-o", filepath.Join(dir, "plugin.wasm"), "."); err != nil {
			return fmt.Errorf("プラグイン %s を WASM にビルドできない: %w", name, err)
		}
	}

	// 2. 一時ディレクトリで emit する
	work := filepath.Join(tmp, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		return err
	}
	for _, name := range inputs {
		data, err := os.ReadFile(filepath.Join(repo, name))
		if err != nil {
			if os.IsNotExist(err) && name == "go.sum" {
				continue
			}
			return err
		}
		if err := os.WriteFile(filepath.Join(work, name), data, 0o644); err != nil {
			return err
		}
	}
	specs, err := specFiles(repo)
	if err != nil {
		return err
	}
	for _, name := range specs {
		data, err := os.ReadFile(filepath.Join(repo, name))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(work, name), data, 0o644); err != nil {
			return err
		}
	}
	for _, dir := range links {
		if err := os.Symlink(filepath.Join(repo, dir), filepath.Join(work, dir)); err != nil {
			return fmt.Errorf("シンボリックリンクを作れない: %w", err)
		}
	}
	env, err := isolatedEnv(repo, filepath.Join(tmp, "home"))
	if err != nil {
		return err
	}
	if err := commandEnv(work, env, io.Discard, stderr, hozoc, "build"); err != nil {
		return fmt.Errorf("hozoc build が失敗した: %w", err)
	}

	// 3. vet と、置き換えたつもりでのビルド
	if err := verify(repo, genDirs, work, users, tmp, stderr); err != nil {
		keep = true
		return fmt.Errorf("生成したコードがコンパイルできないので、%s は置き換えていない（生成したものは %s に残した）: %w", places(), work, err)
	}

	// 4. 置き換える
	for _, dir := range genDirs {
		changed, err := replace(filepath.Join(repo, dir), filepath.Join(work, dir))
		for _, c := range changed {
			fmt.Fprintln(stdout, strings.Replace(c, ": ", ": "+filepath.ToSlash(dir)+"/", 1))
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// places は、置き場の一覧（メッセージ用）。
// specFiles は、リポジトリのルートの仕様のファイル（*.hozo。hozo.toml の sources が並べるもの: cli.hozo と、設計だけのファイル）の
// 名前。cli.hozo の宣言が、設計だけのファイルの宣言を指す（決定の記録への @adr.decided など）ので、生成するときに、すべて要る
// （docs/packages.md「自己生成の手順」の 2）。
func specFiles(repo string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(repo, "*.hozo"))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, filepath.Base(m))
	}
	sort.Strings(names)
	return names, nil
}

func places() string {
	var out []string
	for _, dir := range genDirs {
		out = append(out, filepath.ToSlash(dir))
	}
	return strings.Join(out, " と ")
}

// isolatedEnv は、生成のために hozoc を走らせるときの環境変数: 今の環境から、プラグインの探索が見る場所
// （docs/plugin-protocol.md「探索」の HOZO_PLUGIN_PATH とホーム）を外したもの。生成物が、実行する人の環境に入っている
// プラグインに左右されないようにする（使うのは hozo.toml の plugin_dirs のものだけ）。ホームを空のディレクトリ home に
// 向けるので、ホームから決まるキャッシュの場所は、今の値を明示して渡す: go のものと、hozoc が、プラグインの WASM を、機械の
// 言葉に直したものを覚える置き場（HOZO_CACHE_DIR。docs/plugin-protocol.md「起動」。答えは、変わらない）。
func isolatedEnv(repo, home string) ([]string, error) {
	if err := os.MkdirAll(home, 0o755); err != nil {
		return nil, err
	}
	out, err := exec.Command("go", "env", "-json", "GOCACHE", "GOMODCACHE", "GOPATH").Output()
	if err != nil {
		return nil, fmt.Errorf("go env: %w", err)
	}
	var goEnv map[string]string
	if err := json.Unmarshal(out, &goEnv); err != nil {
		return nil, fmt.Errorf("go env: %w", err)
	}
	env := os.Environ()
	for _, name := range []string{"GOCACHE", "GOMODCACHE", "GOPATH"} {
		env = append(env, name+"="+goEnv[name])
	}
	if os.Getenv("HOZO_CACHE_DIR") == "" {
		if base, err := os.UserCacheDir(); err == nil {
			env = append(env, "HOZO_CACHE_DIR="+filepath.Join(base, "hozo"))
		}
	}
	return append(env, "HOME="+home, "USERPROFILE="+home, "HOZO_PLUGIN_PATH=", "HOZO_CONFIG="), nil
}

// verify は、work の rels（置き場。リポジトリのルートからの相対パス）に生成したもの（work はリポジトリと同じ go.mod を持つ）が
// コンパイルできることを確かめる: まず work で、生成したパッケージを vet に通す。次に、リポジトリ repo の置き場の中身をそれに
// 置き換えたつもりで、パッケージ pkgs（生成物と、それを import するもの）をビルドする。ディスクの上の repo の置き場には触れない
// （go の -overlay で差し替える。なくなるファイルは消えたものとして扱う）。置き場の一つにでも何も生成されていなければ、誤り。
// scratch は overlay の JSON を置く一時ディレクトリ。
func verify(repo string, rels []string, work string, pkgs []string, scratch string, stderr io.Writer) error {
	overlay := struct {
		Replace map[string]string
	}{Replace: map[string]string{}}
	vet := []string{"vet"}
	for _, rel := range rels {
		fresh := filepath.Join(work, rel)
		oldFiles, err := tree(filepath.Join(repo, rel))
		if err != nil {
			return err
		}
		newFiles, err := tree(fresh)
		if err != nil {
			return err
		}
		if len(newFiles) == 0 {
			return fmt.Errorf("%s に何も生成されなかった", filepath.ToSlash(rel))
		}
		for f := range oldFiles {
			overlay.Replace[filepath.Join(repo, rel, f)] = "" // なくなるファイル
		}
		for f := range newFiles {
			overlay.Replace[filepath.Join(repo, rel, f)] = filepath.Join(fresh, f)
		}
		vet = append(vet, "./"+filepath.ToSlash(rel)+"/...")
	}
	if len(rels) == 0 {
		return errors.New("置き場がない")
	}
	if err := command(work, io.Discard, stderr, "go", vet...); err != nil {
		return err
	}
	data, err := json.Marshal(overlay)
	if err != nil {
		return err
	}
	file := filepath.Join(scratch, "overlay.json")
	if err := os.WriteFile(file, data, 0o644); err != nil {
		return err
	}
	return command(repo, io.Discard, stderr, "go", append([]string{"build", "-overlay", file}, pkgs...)...)
}

// replace は dst の中身を src の中身と同じにする: src にないファイルを消し、違うファイルと新しいファイルを書く。
// 同じ内容のファイルには触れない。変えたことを「written: <dst からの相対パス>」「removed: <同>」の行で返す（辞書順）。
func replace(dst, src string) ([]string, error) {
	oldFiles, err := tree(dst)
	if err != nil {
		return nil, err
	}
	newFiles, err := tree(src)
	if err != nil {
		return nil, err
	}
	var changed []string
	for f := range oldFiles {
		if _, ok := newFiles[f]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(dst, f)); err != nil {
			return changed, err
		}
		changed = append(changed, "removed: "+filepath.ToSlash(f))
	}
	for f, content := range newFiles {
		if old, ok := oldFiles[f]; ok && bytes.Equal(old, content) {
			continue
		}
		path := filepath.Join(dst, f)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return changed, err
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return changed, err
		}
		changed = append(changed, "written: "+filepath.ToSlash(f))
	}
	sort.Strings(changed)
	return changed, nil
}

// tree は dir の下の普通のファイルを、dir からの相対パス → 内容で返す。dir がなければ空。
func tree(dir string) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out[rel], err = os.ReadFile(p)
		return err
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return out, nil
}

// command は dir で name を走らせる。失敗したら、何を走らせたかを添えた誤りを返す。
func command(dir string, stdout, stderr io.Writer, name string, args ...string) error {
	return commandEnv(dir, nil, stdout, stderr, name, args...)
}

// commandEnv は command と同じで、環境変数を env にする（nil なら今の環境）。
func commandEnv(dir string, env []string, stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", filepath.Base(name), strings.Join(args, " "), err)
	}
	return nil
}
