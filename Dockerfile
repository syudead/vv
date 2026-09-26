# syntax=docker/dockerfile:1

# task up と公開イメージ（ghcr.io/syudead/vv）の実体。SPA のビルド → 単一バイナリの
# ビルド → 実行環境の 3 段に分ける。前の 2 段はビルドする側のアーキテクチャで動かし、
# 別アーキテクチャ向けのイメージでもエミュレーションを使わずにクロスビルドする。

# 1) SPA をビルドする。
FROM --platform=$BUILDPLATFORM node:24-alpine@sha256:ebfe2f90462722a7a4de65e91990e97fe0d401c70e0e762c5b53302f905ec1c1 AS web
WORKDIR /src/web
# 依存の取得だけを先に行い、ソースの変更でこの層が無駄にならないようにする。
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2) 単一バイナリをビルドする。
FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 埋め込み先へ SPA の成果物を置く。手元の web/dist は .dockerignore で除いてあるので、
# ここで入るのは web ステージが作ったものだけである。
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
# CGO_ENABLED=0 を維持する（modernc.org/sqlite は CGO を必要としない）。
# .dockerignore が .git を除くため、コミット情報の自動埋め込みは無効にする。
# リリース名だけ ldflags で渡す（commit / builtAt は契約上省略可）。
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -buildvcs=false \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/mdm ./cmd/mdm

# 3) 実行する。ffmpeg／ffprobe を同梱し、起動前確認が通る状態にする。
FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6
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
