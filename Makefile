.PHONY: build generate vet test
build:
	go build -o bin/ ./cmd/...
# cli.hozo から internal/gen/hozoc/** を生成し直す（docs/packages.md「生成するもの」、docs/testing.md「生成物の一致」）
generate:
	go run ./cmd/hozoc emit
vet:
	go vet ./...
test:
	go test ./...
