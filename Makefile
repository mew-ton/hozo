.PHONY: build generate vet test
# hozoc と、配布用のプラグインの実行ファイル（docs/plugin-go-cli.md「マニフェスト」の hozoc-plugin-go-cli）を bin/ に作る
build:
	go build -o bin/ ./cmd/...
	go build -o bin/hozoc-plugin-go-cli ./plugins/go-cli
# cli.hozo から internal/gen/hozoc/** を生成し直す（docs/packages.md「生成するもの」「自己生成の手順」、docs/testing.md「生成物の一致」）。
# 一時ディレクトリに生成し、コンパイルできると確かめてから置き換える（生成器の不具合で hozoc 自身がビルドできなくなるのを防ぐ）
generate:
	go run ./internal/tools/selfgen
vet:
	go vet ./...
test:
	go test ./...
