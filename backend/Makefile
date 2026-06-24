.PHONY: build generate test test-unit test-integration test-e2e

VERSION   ?= $(shell tr -d '\r\n' < ./cmd/server/VERSION)
# BUILD 自研发布版本号：构建时从 git commit date + short SHA 自动推导（CalVer），
# 不依赖任何远程服务、无需手改文件。同一 commit 在任何机器构建结果一致（可复现）。
GIT_DATE  := $(shell git log -1 --format=%cd --date=format:'%Y.%m.%d')
GIT_SHORT := $(shell git rev-parse --short=8 HEAD)
BUILD     ?= $(GIT_DATE)-$(GIT_SHORT)
# 用 GO_LDFLAGS 而非 LDFLAGS：避免被 shell/环境里的 LDFLAGS（如 homebrew ruby 的
# -L/opt/.../lib）污染，导致 -X main.* 版本号注入丢失。
GO_LDFLAGS := -s -w -X main.Version=$(VERSION) -X main.Build=$(BUILD)

build:
	CGO_ENABLED=0 go build -ldflags="$(GO_LDFLAGS)" -trimpath -o bin/server ./cmd/server

generate:
	go generate ./ent
	go generate ./cmd/server

test:
	go test ./...
	golangci-lint run ./...

test-unit:
	go test -tags=unit ./...

test-integration:
	go test -tags=integration ./...

test-e2e:
	./scripts/e2e-test.sh

test-e2e-local:
	go test -tags=e2e -v -timeout=300s ./internal/integration/...
