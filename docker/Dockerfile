# ── build stage ──────────────────────────────────────────────────────────────
FROM node:20-slim AS ui-builder
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-bookworm AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui-builder /src/web/dist internal/api/ui
RUN CGO_ENABLED=0 GOOS=linux go build -o /castle ./cmd/castle

# ── runtime stage ─────────────────────────────────────────────────────────────
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
        ffmpeg \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=go-builder /castle /usr/local/bin/castle

VOLUME ["/config", "/recordings"]
EXPOSE 8080

ENTRYPOINT ["castle", "-config", "/config/castle.yaml"]
