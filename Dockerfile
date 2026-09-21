# syntax=docker/dockerfile:1

# make up の実体。SPA のビルド → 単一バイナリのビルド → 実行環境の 3 段に分ける
# （research.md R-003 / R-008）。

# 1) SPA をビルドする。
FROM node:22-alpine AS web
WORKDIR /src/web
# 依存の取得だけを先に行い、ソースの変更でこの層が無駄にならないようにする。
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2) 単一バイナリをビルドする。
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 版管理のプレースホルダを、実際のビルド成果物で置き換える。
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
# CGO_ENABLED=0 を維持する（modernc.org/sqlite は CGO を必要としない）。
# .dockerignore が .git を除くため、コミット情報の自動埋め込みは無効にする。
# リリース名だけ ldflags で渡す（commit / builtAt は契約上省略可）。
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/mdm ./cmd/mdm

# 3) 実行する。ffmpeg／ffprobe を同梱し、起動前確認（FR-008）が通る状態にする。
FROM alpine:3.22
RUN apk add --no-cache ffmpeg ca-certificates tzdata \
    && mkdir -p /media /data
COPY --from=build /out/mdm /usr/local/bin/mdm

ENV MDM_ADDR=":8080" \
    MDM_DATA_DIR="/data" \
    MDM_LOG_LEVEL="info"

EXPOSE 8080
VOLUME ["/data"]

# 稼働確認はアプリケーション自身の経路を使う。
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/api/health >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/mdm"]
