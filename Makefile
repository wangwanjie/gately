VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
# 默认构建标签：去掉 Coraza 的 @validateSchema 操作符及其依赖（JSON Schema、YAML 库），程序约小 2.3 MB；
# CRS 与内置规则都不使用它。定制构建的 no_<模块> / lang_* 标签追加在后面，例如 make build GO_TAGS=coraza.disabled_operators.validateSchema,no_smb
GO_TAGS ?= coraza.disabled_operators.validateSchema
LDFLAGS := -s -w \
	-X github.com/wangwanjie/gately/internal/version.Version=$(VERSION) \
	-X github.com/wangwanjie/gately/internal/version.Commit=$(COMMIT) \
	-X github.com/wangwanjie/gately/internal/version.BuildDate=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: all web build test lint run clean docker api-docs

all: web build

web:
	cd web && pnpm install --frozen-lockfile && pnpm build

build:
	CGO_ENABLED=0 go build -trimpath -tags "$(GO_TAGS)" -ldflags "$(LDFLAGS)" -o bin/gately ./cmd/gately

test:
	go test -race ./...

# 导出 API 文档（openapi.json 与 API.md），供官网 https://gately.cn 发布
api-docs:
	go run ./cmd/gately -api-docs docs/api

lint:
	gofmt -l . | (! grep .)
	go vet ./...
	cd web && pnpm typecheck

run: build
	./bin/gately -c ./data

clean:
	rm -rf bin

docker:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t gately:$(VERSION) .
