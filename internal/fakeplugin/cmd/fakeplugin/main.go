// Command fakeplugin は、テストのための、にせもののプラグイン（docs/testing.md「環境からの独立」）。本物の SDK を通さずに、
// docs/plugin-protocol.md「emit」のまとめた要求を読み、決まった応答を返す: テストが、hozoc の側（起動、要求の形、応答の読み方）を、
// プラグインの実装と切り離して確かめるためのもの。
//
// WASM（WASI）にビルドして、hozoc の、サンドボックスの中で動かす。サンドボックスは、プラグインに、ファイルを見せない
// （docs/plugin-protocol.md「起動」）ので、テストが、internal/plugin の MountForTest で、プラグインのディレクトリ
// （マニフェストのある場所）を、"/" として見せる。振る舞いは、そのディレクトリのファイルで決める:
//
//	launches        起動のたびに、一行足す（起動の回数）
//	requests.jsonl  direction "emit" なら、要求の、出力とトランスパイラの組ごとに、一行足す:
//	                {"protocol":…,"direction":"emit","transpiler":<名前>,"view":<その出力の view>}。
//	                ほかの direction なら、要求を、そのまま一行足す
//	raw.jsonl       要求を、そのまま一行足す（どの direction でも）
//	response.json   emit の、トランスパイラ一つの答えのひな形。@DIR@ は、その出力の view.config.dir、@NS@ は view.viewer、
//	                @T@ はトランスパイラの名前、@RUN@ は、起動の回数に置き換える。response.<トランスパイラの名前>.json が
//	                あれば、そのトランスパイラには、それを使う。JSON として読まずに、応答に、そのまま並べる（形の違う
//	                答えも、返せる）
//	raw-response    あれば、emit の応答の全体を、これにする（outputs の数が合わない応答など）
//	coverage.json   direction "coverage" の応答
//	stderr          あれば、標準エラーに書く
//	gate、open      gate がある間は、open ができるまで待つ（起動したプラグインが、終わらない間を作る）
//	crash           あれば、応答を書かずに、終了コード 3
//	exit            終了コード（なければ 0）。応答を書いてから、そのコードで終わる
//	spin            あれば、応答を書かずに、終わらない（途中で止められるまで、回り続ける）
//
// ディレクトリが見えなければ（MountForTest なしで置かれた、代役）、何も記録せず、emit には、頼まれたトランスパイラごとに、
// ファイルを返さない答え {"files":[],"warnings":[]} を返す。
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
	const dir = "/"
	mounted := exists(dir)
	file := func(name string) string { return filepath.Join(dir, name) }
	read := func(name string) (string, bool) {
		data, err := os.ReadFile(file(name))
		return string(data), err == nil
	}
	appendLine := func(name string, line []byte) {
		if !mounted {
			return
		}
		f, err := os.OpenFile(file(name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			fmt.Fprintln(os.Stderr, "fakeplugin:", err)
			os.Exit(9)
		}
		defer f.Close()
		f.Write(append(bytes.TrimSpace(line), '\n'))
	}
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(9)
	}
	appendLine("launches", []byte("x"))
	launches, _ := read("launches")
	run := strings.Count(launches, "\n")
	var compact bytes.Buffer
	if json.Compact(&compact, raw) != nil {
		compact.Write(bytes.ReplaceAll(raw, []byte("\n"), []byte(" ")))
	}
	appendLine("raw.jsonl", compact.Bytes())
	if text, ok := read("stderr"); ok {
		fmt.Fprint(os.Stderr, text)
	}
	for exists(file("gate")) && !exists(file("open")) {
		time.Sleep(10 * time.Millisecond)
	}
	if exists(file("spin")) {
		for {
		}
	}
	if exists(file("crash")) {
		os.Exit(3)
	}
	code := 0
	if text, ok := read("exit"); ok {
		code, _ = strconv.Atoi(strings.TrimSpace(text))
	}

	var req struct {
		Protocol  json.RawMessage `json:"protocol"`
		Direction string          `json:"direction"`
		Outputs   []struct {
			Transpilers []string        `json:"transpilers"`
			View        json.RawMessage `json:"view"`
		} `json:"outputs"`
	}
	_ = json.Unmarshal(raw, &req)
	if req.Direction != "emit" {
		appendLine("requests.jsonl", compact.Bytes())
		if req.Direction == "coverage" {
			text, _ := read("coverage.json")
			fmt.Print(text)
			os.Exit(0)
		}
		text, _ := read("response.json")
		fmt.Print(text)
		os.Exit(code)
	}

	var out strings.Builder
	out.WriteString(`{"outputs":[`)
	for i, o := range req.Outputs {
		var view struct {
			Viewer string `json:"viewer"`
			Config struct {
				Dir string `json:"dir"`
			} `json:"config"`
		}
		_ = json.Unmarshal(o.View, &view)
		if i > 0 {
			out.WriteByte(',')
		}
		out.WriteString(`{"results":[`)
		for j, name := range o.Transpilers {
			call, _ := json.Marshal(map[string]any{"protocol": req.Protocol, "direction": "emit", "transpiler": name, "view": o.View})
			appendLine("requests.jsonl", call)
			template, ok := read("response." + name + ".json")
			if !ok {
				template, ok = read("response.json")
			}
			if !ok && !mounted {
				template = `{"files":[],"warnings":[]}`
			}
			answer := strings.NewReplacer("@DIR@", view.Config.Dir, "@NS@", view.Viewer, "@T@", name, "@RUN@", strconv.Itoa(run)).Replace(template)
			if j > 0 {
				out.WriteByte(',')
			}
			out.WriteString(strings.TrimSpace(answer))
		}
		out.WriteString(`]}`)
	}
	out.WriteString(`]}`)
	if text, ok := read("raw-response"); ok {
		fmt.Print(text)
	} else {
		fmt.Print(out.String())
	}
	os.Exit(code)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
