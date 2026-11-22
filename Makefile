.PHONY: build plugins generate vet test
# hozoc を bin/ に作り、このリポジトリのプラグインを、WASM にビルドする
build: plugins
	go build -o bin/ ./cmd/...
# plugins/ のプラグインを、WASM にビルドして、それぞれのディレクトリの plugin.wasm（マニフェストの wasm）に置く
# （docs/plugin-protocol.md「マニフェスト」、docs/testing.md「プラグインの WASM」。テストと make generate は、自分でビルドする）
plugins:
	for p in go go-valid go-sqlite go-http go-cli go-design http; do (cd plugins/$$p && GOOS=wasip1 GOARCH=wasm go build -o plugin.wasm .) || exit 1; done
# cli.hozo から internal/gen/hozoc/** と internal/gen/proc/** を生成し直す（docs/packages.md「生成するもの」「自己生成の手順」、docs/testing.md「生成物の一致」）。
# 一時ディレクトリに生成し、コンパイルできると確かめてから置き換える（生成器の不具合で hozoc 自身がビルドできなくなるのを防ぐ）
generate:
	go run ./internal/tools/selfgen
vet:
	go vet ./...
test:
	go test ./...
