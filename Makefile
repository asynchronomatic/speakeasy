

build: test
	go mod tidy
	go build -tags assert -o build/speakeasy github.com/asynchronomatic/speakeasy/cmd/mesh
	go build -tags assert -o build/speakeasy-cli github.com/asynchronomatic/speakeasy/cmd/admincli
.PHONY: build

build-all:
	GOARCH=amd64 GOOS=windows go build -o build/speakeasy.windows.amd64 github.com/asynchronomatic/speakeasy/cmd/mesh
	GOARCH=amd64 GOOS=linux   go build -o build/speakeasy.linux.amd64 github.com/asynchronomatic/speakeasy/cmd/mesh
	GOARCH=arm64 GOOS=linux   go build -o build/speakeasyh.linux.arm64 github.com/asynchronomatic/speakeasy/cmd/mesh
	GOARCH=arm64 GOOS=darwin  go build -o build/speakeasy.darwin.arm64 github.com/asynchronomatic/speakeasy/cmd/mesh
.PHONY: build-all

run-proxy:
	go run -tags assert github.com/asynchronomatic/speakeasy/cmd/mesh proxy start
.PHONY: run-proxy

run-hybrid:
	go run -tags assert github.com/asynchronomatic/speakeasy/cmd/mesh hybrid start
.PHONY: run-proxy

test:
	go test -tags assert github.com/asynchronomatic/speakeasy/...
	- staticcheck ./...
.PHONY: test

test-verbose:
	go test -v -tags assert github.com/asynchronomatic/speakeasy/...
.PHONY: test-verbose

# This is for development use, feel free to put custom targets in this file, it should never be checked in
-include Makefile.local

