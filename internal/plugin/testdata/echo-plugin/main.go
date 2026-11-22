// Command echo-plugin は internal/plugin のテストに使う小さなプラグイン（docs/testing.md「プラグインのプロトコル」）。
// WASM（WASI）にビルドして使う。標準入力の JSON の "mode" で振る舞いを変える:
//
//	echo（既定）  要求をそのまま応答に写す
//	probe         サンドボックスの外に、手を伸ばしてみた結果を返す（下の probe）
//	spin          応答を書かずに、回り続ける（終わらない）
//	listen        要求の "addr" で、待ち受けてみて、結果を、標準エラーに一行書き、回り続ける（終わらない）
//	sleep         応答を書かずに、眠り続ける（終わらない）
//	crash         終了コード 1 で終わる
//	panic         panic する（終了コード 2 と、標準エラー）
//	garbage       JSON でないものを書いて 0 で終わる
//	trailing      JSON の後ろに余計なものを書いて 0 で終わる
//	protocol      { "error": { "code": "protocol" } } で 2
//	unsupported   { "error": { "code": "unsupported" } } で 2
//	exit2-other   { "error": { "code": "weird" } } で 2
//	exit2-garbage JSON でないものを書いて 2
//
// "stderr" が配列なら、その各行を標準エラーに書く。
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"net"
	"os"
	"time"
)

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(3)
	}
	var req map[string]any
	if err := json.Unmarshal(data, &req); err != nil {
		fmt.Fprintln(os.Stderr, "bad request:", err)
		os.Exit(3)
	}
	if lines, ok := req["stderr"].([]any); ok {
		for _, l := range lines {
			fmt.Fprintln(os.Stderr, l)
		}
	}
	mode, _ := req["mode"].(string)
	switch mode {
	case "", "echo":
		os.Stdout.Write(data)
	case "probe":
		json.NewEncoder(os.Stdout).Encode(probe(req))
	case "spin":
		for {
		}
	case "listen":
		addr, _ := req["addr"].(string)
		if _, err := net.Listen("tcp", addr); err != nil {
			fmt.Fprintln(os.Stderr, "listen: "+err.Error())
		} else {
			fmt.Fprintln(os.Stderr, "listen: ok")
		}
		for {
		}
	case "sleep":
		for {
			time.Sleep(time.Hour)
		}
	case "crash":
		os.Exit(1)
	case "panic":
		panic("boom")
	case "garbage":
		fmt.Print("this is not json")
	case "trailing":
		fmt.Print(`{"ok": true} {"again": true}`)
	case "protocol":
		fmt.Print(`{"error": {"code": "protocol", "message": "want 9.9"}}`)
		os.Exit(2)
	case "unsupported":
		fmt.Print(`{"error": {"code": "unsupported", "message": "no import"}}`)
		os.Exit(2)
	case "exit2-other":
		fmt.Print(`{"error": {"code": "weird", "message": "?"}}`)
		os.Exit(2)
	case "exit2-garbage":
		fmt.Print("boom")
		os.Exit(2)
	default:
		fmt.Fprintln(os.Stderr, "unknown mode", mode)
		os.Exit(3)
	}
}

// probe は、サンドボックスの外に、手を伸ばしてみた結果。どの項目も、できなければ、誤りの文字列（できたら ""）。
//
//	args、env   見えた引数と、環境変数
//	cwd         作業ディレクトリ（分からなければ ""）
//	open        要求の "paths" の、それぞれを、開いてみた結果
//	list        同じく、ディレクトリとして、一覧してみた結果
//	write       同じく、その下に、ファイルを作ってみた結果
//	dial        要求の "addr" に、つないでみた結果
//	now、mono、rand、mrand  時計と乱数を読んだ値
func probe(req map[string]any) map[string]any {
	msg := func(err error) string {
		if err != nil {
			return err.Error()
		}
		return ""
	}
	out := map[string]any{"args": os.Args, "env": os.Environ()}
	cwd, _ := os.Getwd()
	out["cwd"] = cwd
	open, list, write := map[string]string{}, map[string]string{}, map[string]string{}
	paths, _ := req["paths"].([]any)
	for _, p := range paths {
		path, _ := p.(string)
		f, err := os.Open(path)
		if err == nil {
			f.Close()
		}
		open[path] = msg(err)
		_, err = os.ReadDir(path)
		list[path] = msg(err)
		write[path] = msg(os.WriteFile(path+"/written-by-plugin", []byte("x"), 0o644))
	}
	out["open"], out["list"], out["write"] = open, list, write
	addr, _ := req["addr"].(string)
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err == nil {
		conn.Close()
	}
	out["dial"] = msg(err)
	start := time.Now()
	out["now"] = start.UnixNano()
	out["mono"] = time.Since(start).Nanoseconds()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		out["rand"] = "error: " + err.Error()
	} else {
		out["rand"] = hex.EncodeToString(b[:])
	}
	out["mrand"] = mrand.Uint64()
	return out
}
