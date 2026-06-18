# syntax=docker/dockerfile:1
# ── UI build ──────────────────────────────────────────────────────────────────
FROM node:20-slim AS ui-builder
WORKDIR /src/web
COPY web/package*.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --cache /root/.npm
COPY web/ ./
RUN npm run build

# ── Go build ──────────────────────────────────────────────────────────────────
FROM golang:1.26-bookworm AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
COPY --from=ui-builder /src/web/dist/ internal/api/ui/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /castle ./cmd/castle

# ── Runtime ───────────────────────────────────────────────────────────────────
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
        ffmpeg \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=go-builder /castle /usr/local/bin/castle

VOLUME ["/config", "/recordings"]
EXPOSE 8080

ENTRYPOINT ["castle", "-config", "/config/castle.yaml"]
