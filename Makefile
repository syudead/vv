# 開発者向けコマンドの契約:
#   specs/001-initial-setup/contracts/developer-commands.md
# ここにある目標名がそのまま開発者との契約になる。README と CI は必ずこれらの目標を呼び、
# CI の検査を同じ目標で手元でも再現できる状態にする（CI 専用の検査を作らない）。

# リリース名。既定は dev で、ビルド時に上書きできる（research.md R-007）。
VERSION ?= dev

# ツールの版はここで固定する（R-005 / R-010）。生成物は版管理に含め、手編集しない。
GOLANGCI_LINT_VERSION      = $(shell jq -er .golangciLint scripts/tool-versions.json)
OAPI_CODEGEN_VERSION       = $(shell jq -er .oapiCodegen scripts/tool-versions.json)
OPENAPI_TYPESCRIPT_VERSION = $(shell jq -er .openapiTypescript scripts/tool-versions.json)

GOLANGCI_LINT = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
NPM           := npm --prefix web

# make dev 用のデータ置き場。
DEV_DATA_DIR  ?= $(CURDIR)/.local/data

# 生成物。make generate の再実行で差分が出る状態は失敗とみなす。
GENERATED := internal/httpapi/gen/api.gen.go web/src/api/gen/openapi.ts

.DEFAULT_GOAL := help
.PHONY: help setup up down dev build generate fmt lint test check test-local-dev
.PHONY: fmt-check fmt-check-go fmt-check-web generate-check
.PHONY: lint-go lint-web test-go test-web test-e2e

help: ## 目標の一覧を表示する
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

setup: ## 依存と開発ツールを先に取得する（Claude Code の SessionStart フックが呼ぶ）
	go mod download
	$(NPM) install
	@# 初回の make lint は golangci-lint の取得とビルドに数分かかるので先に済ませる。
	$(GOLANGCI_LINT) --version
	@# 依存のビルドキャッシュも温める（modernc.org/sqlite が大きい）。
	go build ./...

up: ## Docker でイメージを構築して起動する（導入手順で最初に実行する唯一のコマンド）
	docker compose up --build

down: ## 起動したものを停止・削除する
	docker compose down --remove-orphans

dev: web/node_modules ## Go サーバーと Vite 開発サーバーを起動する
	@mkdir -p "$(DEV_DATA_DIR)"
	@echo "Go: http://localhost:8080 / Vite: http://localhost:5173（/api は :8080 へ中継）"
	@trap 'kill 0' EXIT INT TERM; \
	MDM_DATA_DIR="$(DEV_DATA_DIR)" go run ./cmd/mdm & \
	$(NPM) run dev & \
	wait

build: web/node_modules ## SPA をビルドして埋め込み、単一バイナリを生成する
	$(NPM) run build
	@# Vite が出力先を空にするため、版管理しているプレースホルダを戻す。
	@touch web/dist/.gitkeep
	CGO_ENABLED=0 go build -trimpath \
		-ldflags "-s -w -X main.version=$(VERSION)" \
		-o bin/mdm ./cmd/mdm
	@echo "bin/mdm を生成しました（version=$(VERSION)）"

generate: ## api/openapi.yaml から Go と TypeScript の型を生成する
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION) \
		-config api/oapi-codegen.yaml api/openapi.yaml
	npx --yes openapi-typescript@$(OPENAPI_TYPESCRIPT_VERSION) \
		api/openapi.yaml -o web/src/api/gen/openapi.ts

fmt: web/node_modules ## 書式を整える
	gofmt -w $$(go list -f '{{.Dir}}' ./...)
	$(NPM) run format

lint: lint-go lint-web ## golangci-lint（depguard を含む）と Web の静的検査

test: test-go test-web ## Go と Web の検証

check: ## fmt の差分確認 → lint → test → 生成物の差分確認
	@$(MAKE) --no-print-directory test-local-dev
	@$(MAKE) --no-print-directory fmt-check
	@$(MAKE) --no-print-directory lint
	@$(MAKE) --no-print-directory test
	@$(MAKE) --no-print-directory generate-check
	@echo "check: すべて成功しました"

test-local-dev: ## PowerShell のローカル開発スクリプトを検証する
	pwsh -NoLogo -NoProfile -File scripts/local-dev.tests.ps1

fmt-check: fmt-check-go fmt-check-web ## 書式の差分を確認する（書き換えない）

# 以下の -go / -web は、CI で Go と Web のジョブを分けて並行させるための入口である。
# 手元の make lint / test / fmt-check はこれらをまとめて呼ぶので、判定は一致する。

fmt-check-go: ## Go の書式の差分を確認する
	@unformatted="$$(gofmt -l $$(go list -f '{{.Dir}}' ./...))"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt の差分があります。make fmt を実行してください:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

fmt-check-web: web/node_modules ## Web の書式の差分を確認する
	$(NPM) run format:check

lint-go: ## golangci-lint（depguard を含む）を実行する
	$(GOLANGCI_LINT) run

lint-web: web/node_modules ## Web の静的検査（型検査）を実行する
	$(NPM) run lint

test-go: ## Go のテストを実行する
	go test ./...

# $(NPM) run test はビルド検証（vite build）と単体テスト（vitest run）の両方を走らせる。
test-web: web/node_modules ## Web のビルド検証と単体テストを実行する
	$(NPM) run test

test-e2e: web/node_modules ## Go + Vite + Chromium で主要操作をE2E検証する
	$(NPM) run test:e2e

generate-check: ## 生成物が api/openapi.yaml と一致しているか確認する
	@$(MAKE) --no-print-directory generate
	@if [ -n "$$(git status --porcelain -- $(GENERATED))" ]; then \
		echo "生成物が api/openapi.yaml と一致していません。make generate の結果をコミットしてください:"; \
		git --no-pager diff --stat HEAD -- $(GENERATED); \
		exit 1; \
	fi

# npm の依存は package-lock.json が変わったときだけ取り直す。
web/node_modules: web/package.json web/package-lock.json
	$(NPM) ci
	@touch $@
