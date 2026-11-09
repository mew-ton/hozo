// Command echo-plugin は internal/plugin のテストに使う小さなプラグイン（docs/testing.md「プラグインのプロトコル」）。
// 標準入力の JSON の "mode" で振る舞いを変える:
//
//	echo（既定）  要求をそのまま応答に写す
//	env           作業ディレクトリと HOZO_PROJECT_ROOT / HOZO_PLUGIN_DIR と引数を返す
//	crash         終了コード 1 で終わる
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
	"encoding/json"
	"fmt"
	"io"
	"os"
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
	case "env":
		cwd, _ := os.Getwd()
		json.NewEncoder(os.Stdout).Encode(map[string]any{
			"cwd":  cwd,
			"root": os.Getenv("HOZO_PROJECT_ROOT"),
			"dir":  os.Getenv("HOZO_PLUGIN_DIR"),
			"args": os.Args[1:],
		})
	case "crash":
		os.Exit(1)
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
