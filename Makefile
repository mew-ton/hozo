.PHONY: build generate vet test
# hozoc と、配布用のプラグインの実行ファイル（docs/plugin-go-cli.md「マニフェスト」の hozoc-plugin-go-cli）を bin/ に作る
build:
	go build -o bin/ ./cmd/...
	go build -o bin/hozoc-plugin-go-cli ./plugins/go-cli
# cli.hozo から internal/gen/hozoc/** を生成し直す（docs/packages.md「生成するもの」、docs/testing.md「生成物の一致」）
generate:
	go run ./cmd/hozoc emit
vet:
	go vet ./...
test:
	go test ./...
