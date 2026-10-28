# 常用操作入口。目标都是"一条命令说清楚一件事"，参数细节留给底层脚本。
#
# 说明：这里刻意不放 install / deploy 之类的目标——部署方式差异太大（离线包、
# docker、裸机 systemd），用一个 make 目标糊住反而会误导。

SHELL := /bin/bash
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: help build web verify offline docker-up docker-down docker-logs fmt vet test clean

help: ## 列出所有目标
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## 编译当前平台（含前端静态导出）
	./build.sh

web: ## 只重建前端静态资源（server/webui/dist）
	cd web && npm ci && npm run build:static

verify: ## 编译 + vet + 无库单测（有 ARTEX_PG_DSN 时自动带上数据库用例）
	./scripts/verify.sh

test: ## 跑单元测试（没有 ARTEX_PG_DSN 时数据库用例自动跳过）
	go test ./...

fmt: ## gofmt 所有 Go 源码
	gofmt -w $$(git ls-files '*.go')

vet: ## go vet 全仓库
	go vet ./...

offline: ## 打离线安装包（当前平台）
	./scripts/build-offline.sh --version $(VERSION)

docker-up: ## 本机 docker 起一套（自带 PostgreSQL）
	./scripts/docker-local.sh up

docker-down: ## 停掉本机 docker 部署
	./scripts/docker-local.sh down

docker-logs: ## 跟踪容器日志
	./scripts/docker-local.sh logs

clean: ## 清理构建产物
	rm -rf dist out
