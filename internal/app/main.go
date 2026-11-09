package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// errUsage は引数・フラグの解釈の失敗（終了コード 64）。
var errUsage = errors.New("usage")

// Usage は使い方（docs/plugin-go-cli.md「使い方の表示」の形。コマンドの一行は cli.hozo の op の summary）。
const Usage = `usage: hozoc <command> [flags] [args]

commands:
  check   仕様を検査する。ファイルは書き出さない
  emit    検査を通った仕様から、名前空間ごとに既定の出力とプラグインの出力を作り、書き出す
  lsp     言語サーバーとして標準入出力で動く（docs/lsp.md）

flags:
  --config <path>   設定ファイルの場所（HOZO_CONFIG）
  --format <fmt>    表示の形式。text か json
  --dry-run         emit: 書き出さずに、書き出す予定のファイルだけを表示する
  --log <path>      lsp: ログの書き出し先（HOZO_LSP_LOG）

args:
  emit [namespaces...]   生成する名前空間（一つもなければすべて）
`

// formatPattern は cli.hozo の scalar OutputFormat の @valid.pattern。
var formatPattern = regexp.MustCompile(`^(text|json)$`)

// Main は段階 3 までの手書きの骨組み（docs/cli.md「手で書くもの」）: 引数を解釈して葉の op を呼び、結果を表示し、
// 終了コードを返す。args はプログラム名を含まない（os.Args[1:]）。env は環境変数を引く関数（os.Getenv）。
//
// 引数の解釈は docs/plugin-go-cli.md「引数の解釈」に合わせる: フラグは --name value か --name=value、boolean は --name
// （--name=false も取れる）、同じフラグは後のものが勝つ、-x は位置引数（-h を除く）、-- 以降は位置引数、
// @cli.env はフラグがないときだけ（空文字列は「ない」）。知らないフラグ・値の欠落・位置引数の過不足・Validate の違反は
// 使い方を標準エラーに出して 64。help / --help / -h は使い方を標準出力に出して 0。
func Main(args []string, env func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, Usage)
		return ExitUsage
	}
	command := args[0]
	switch command {
	case "help", "--help", "-h":
		fmt.Fprint(stdout, Usage)
		return ExitOK
	case "check", "emit", "lsp":
	default:
		fmt.Fprintf(stderr, "hozoc: 知らないコマンド %q\n\n%s", command, Usage)
		return ExitUsage
	}
	flags, positional, err := parseArgs(args[1:])
	if err != nil {
		if errors.Is(err, errHelp) {
			fmt.Fprint(stdout, Usage)
			return ExitOK
		}
		fmt.Fprintf(stderr, "hozoc: %v\n\n%s", err, Usage)
		return ExitUsage
	}
	if command != "emit" && len(positional) > 0 {
		fmt.Fprintf(stderr, "hozoc: %s は位置引数を取らない: %s\n\n%s", command, strings.Join(positional, " "), Usage)
		return ExitUsage
	}
	known := map[string][]string{
		"check": {"config", "format"},
		"emit":  {"config", "format", "dry-run"},
		"lsp":   {"config", "log"},
	}[command]
	for name := range flags {
		if !contains(known, name) {
			fmt.Fprintf(stderr, "hozoc: %s に知らないフラグ --%s\n\n%s", command, name, Usage)
			return ExitUsage
		}
	}

	// @cli.env: フラグがなければ環境変数（空文字列は「ない」）
	configPath := flags["config"]
	if _, ok := flags["config"]; !ok {
		configPath = env("HOZO_CONFIG")
	}
	format := flags["format"]
	if format != "" && !formatPattern.MatchString(format) {
		fmt.Fprintf(stderr, "hozoc: --format の値 %q は text か json でなければならない（format: valid.pattern）\n\n%s", format, Usage)
		return ExitUsage
	}
	dryRun := false
	if v, ok := flags["dry-run"]; ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			fmt.Fprintf(stderr, "hozoc: --dry-run の値 %q を boolean にできない\n\n%s", v, Usage)
			return ExitUsage
		}
		dryRun = b
	}

	leaves := &Leaves{Stderr: stderr}
	presenter := NewPresenter(format)
	switch command {
	case "check":
		result, err := leaves.Check(CheckInput{Config: configPath, Format: format})
		return finish(presenter, stdout, stderr, err, func(w io.Writer) error { return presenter.PresentCheck(w, result) })
	case "emit":
		result, err := leaves.Emit(EmitInput{Config: configPath, Format: format, Namespaces: positional, DryRun: dryRun})
		return finish(presenter, stdout, stderr, err, func(w io.Writer) error { return presenter.PresentEmit(w, result) })
	default: // lsp
		logPath := flags["log"]
		if _, ok := flags["log"]; !ok {
			logPath = env("HOZO_LSP_LOG")
		}
		err := leaves.Lsp(LspInput{Config: configPath, Log: logPath})
		return finish(presenter, stdout, stderr, err, func(io.Writer) error { return nil })
	}
}

// finish は結果か例外を表示して終了コードを返す（docs/plugin-go-cli.md「実行と表示」の 2〜4）。
func finish(p Presenter, stdout, stderr io.Writer, err error, present func(io.Writer) error) int {
	if err != nil {
		if errors.Is(err, errNotImplemented) {
			fmt.Fprintf(stderr, "%v\n", err)
			return exitNotImplementedOp
		}
		if perr := p.PresentError(stderr, err); perr != nil {
			fmt.Fprintf(stderr, "hozoc: %v\n", perr)
			return ExitSoftwareOrCrash
		}
		return ExitCode(err)
	}
	if perr := present(stdout); perr != nil {
		fmt.Fprintf(stderr, "hozoc: %v\n", perr)
		return ExitSoftwareOrCrash
	}
	return ExitOK
}

// errHelp は --help / -h。
var errHelp = errors.New("help")

// boolFlags は値を取らないフラグ。
var boolFlags = map[string]bool{"dry-run": true}

// parseArgs はフラグと位置引数を分ける（docs/plugin-go-cli.md「引数の解釈」）。
func parseArgs(args []string) (map[string]string, []string, error) {
	flags := map[string]string{}
	var positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			positional = append(positional, args[i+1:]...)
			return flags, positional, nil
		case a == "--help" || a == "-h":
			return nil, nil, errHelp
		case strings.HasPrefix(a, "--"):
			name, value, hasValue := strings.Cut(a[2:], "=")
			if name == "" {
				return nil, nil, fmt.Errorf("%w: フラグの名前がない: %q", errUsage, a)
			}
			if boolFlags[name] {
				if !hasValue {
					value = "true"
				}
				flags[name] = value
				continue
			}
			if !hasValue {
				if i+1 >= len(args) {
					return nil, nil, fmt.Errorf("%w: --%s の値がない", errUsage, name)
				}
				i++
				value = args[i]
			}
			flags[name] = value
		default:
			positional = append(positional, a)
		}
	}
	return flags, positional, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Run は os の入出力で Main を呼ぶ。cmd/hozoc/main.go が呼ぶ。
func Run() int {
	return Main(os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
}
