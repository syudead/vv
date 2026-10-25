# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

# Build SPA and Go stages on the builder architecture to cross-compile without emulation; the final stage runs the published image.

FROM --platform=$BUILDPLATFORM node:24-alpine@sha256:06d7adf3e0ef61a13f1d28682726366f97a15aa1888c934d3ecec86c33918b53 AS web
WORKDIR /src/web
# Cache dependency downloads independently of source changes and retain npm downloads across lockfile updates.
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:b8c9b82affbbb82a020bbcf998dad2ba9516944ac7e883524642a481a9505e9c AS build
WORKDIR /src
COPY go.mod go.sum ./
# Reuse module and build cache mounts in every Go step; mounts are not persisted in layers.
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
# Copy only binary inputs so documentation and SPA source changes do not invalidate Go compilation.
COPY cmd/ cmd/
COPY internal/ internal/
COPY web/embed.go web/
# Embed only output from the web stage; .dockerignore excludes local dist files.
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
# task up and CI supply commit and RFC 3339 timestamp; omitted values stay absent from /api/health. Late use limits cache invalidation to compilation.
ARG COMMIT=
ARG BUILT_AT=
ARG TARGETOS
ARG TARGETARCH
# Keep CGO disabled; modernc.org/sqlite does not need it. With .git excluded, disable VCS probing and inject metadata through ldflags.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -buildvcs=false \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.builtAt=${BUILT_AT}" \
    -o /out/mdm ./cmd/mdm

# Include ffmpeg and ffprobe for startup checks.
FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a
RUN apk add --no-cache ffmpeg ca-certificates tzdata \
    && mkdir -p /media /data
COPY --from=build /out/mdm /usr/local/bin/mdm

ENV MDM_ADDR=":8080" \
    MDM_DATA_DIR="/data" \
    MDM_LOG_LEVEL="info"

EXPOSE 8080
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/api/health >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/mdm"]
