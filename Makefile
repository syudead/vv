# 開発者向けコマンドの契約:
#   specs/001-initial-setup/contracts/developer-commands.md
# ここにある目標名がそのまま開発者との契約になる。README と CI は必ずこの目標を呼び、
# 手元と CI が同じ判定になるようにする（CI でしか動かない検査を作らない）。

# 生成器の版はここで固定する（R-010）。生成物は版管理に含め、手編集しない。
OAPI_CODEGEN_VERSION       := v2.8.0
OPENAPI_TYPESCRIPT_VERSION := 7.13.0

.DEFAULT_GOAL := help
.PHONY: help up down dev build generate fmt lint test check

help: ## 目標の一覧を表示する
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

up: ## Docker でイメージを構築して起動する（導入手順で最初に実行する唯一のコマンド）
	@echo "TODO(T027): docker compose up --build"

down: ## 起動したものを停止・削除する
	@echo "TODO(T027)"

dev: ## Go サーバーと Vite 開発サーバーを起動する
	@echo "TODO(T027)"

build: ## SPA をビルドして埋め込み、単一バイナリを生成する
	@echo "TODO(T027)"

generate: ## api/openapi.yaml から Go と TypeScript の型を生成する
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION) \
		-config api/oapi-codegen.yaml api/openapi.yaml
	npx --yes openapi-typescript@$(OPENAPI_TYPESCRIPT_VERSION) \
		api/openapi.yaml -o web/src/api/gen/openapi.ts

fmt: ## 書式を整える
	@echo "TODO(T030)"

lint: ## golangci-lint（depguard を含む）と Web の静的検査
	@echo "TODO(T030)"

test: ## Go のテストと Web の検証
	@echo "TODO(T030)"

check: ## fmt の差分確認 → lint → test → 生成物の差分確認
	@echo "TODO(T030)"
