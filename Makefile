

build: test
	go build -tags assert -o build/mesh github.com/asynchronomatic/speakeasy/cmd/mesh
	go build -tags assert -o build/admincli github.com/asynchronomatic/speakeasy/cmd/admincli
.PHONY: build

build-all:
	GOARCH=amd64 GOOS=windows go build -o build/mesh.win github.com/asynchronomatic/speakeasy/cmd/mesh
	GOARCH=amd64 GOOS=linux   go build -o build/mesh.linux.amd64 github.com/asynchronomatic/speakeasy/cmd/mesh
	GOARCH=arm64 GOOS=linux   go build -o build/mesh.linux.arm64 github.com/asynchronomatic/speakeasy/cmd/mesh
	GOARCH=arm64 GOOS=darwin  go build -o build/mesh.darwin.arm64 github.com/asynchronomatic/speakeasy/cmd/mesh
.PHONY: build-all

run-admin:
	export ADMIN_DB_PATH="tests/admin.jkv"
	ADMIN_DB_PATH="tests/admin.kjv" go run github.com/asynchronomatic/speakeasy/cmd/mesh admin
.PHONY: run-admin

run-proxy:
	go run -tags assert github.com/asynchronomatic/speakeasy/cmd/mesh proxy start
.PHONY: run-proxy

run-hybrid:
	go run github.com/asynchronomatic/speakeasy/cmd/mesh hybrid
.PHONY: run-proxy


test:
	go test -tags assert github.com/asynchronomatic/speakeasy/...
	- staticcheck ./...
.PHONY: test

test-verbose:
	go test -v -tags assert github.com/asynchronomatic/speakeasy/...
.PHONY: test-verbose


-include Makefile.local

