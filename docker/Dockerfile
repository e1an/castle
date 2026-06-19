# syntax=docker/dockerfile:1
# ── UI build ──────────────────────────────────────────────────────────────────
FROM node:20-slim AS ui-builder
WORKDIR /src/web
COPY web/package*.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --cache /root/.npm
COPY web/ ./
RUN npm run build

# ── YOLOv8n model export ──────────────────────────────────────────────────────
FROM python:3.12-slim AS model-fetcher
RUN pip install --no-cache-dir 'ultralytics==8.3.*'
WORKDIR /models
RUN python - <<'EOF'
from ultralytics import YOLO
YOLO('yolov8n.pt').export(format='onnx', imgsz=640)
EOF

# ── fetch ONNX Runtime shared library ─────────────────────────────────────────
FROM debian:bookworm-slim AS onnx-fetcher
ARG ONNX_VERSION=1.26.0
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL \
    "https://github.com/microsoft/onnxruntime/releases/download/v${ONNX_VERSION}/onnxruntime-linux-x64-${ONNX_VERSION}.tgz" \
    | tar -xz -C /tmp \
    && mv /tmp/onnxruntime-linux-x64-${ONNX_VERSION}/lib/libonnxruntime.so.${ONNX_VERSION} \
          /tmp/libonnxruntime.so

# ── Go build ──────────────────────────────────────────────────────────────────
FROM golang:1.26-bookworm AS go-builder
WORKDIR /src

COPY --from=onnx-fetcher /tmp/libonnxruntime.so /usr/lib/libonnxruntime.so
RUN ldconfig

COPY go.mod go.sum ./
COPY . .
COPY --from=ui-builder /src/web/dist/ internal/api/ui/
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod tidy

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux go build -tags onnx -o /castle ./cmd/castle

# ── Runtime ───────────────────────────────────────────────────────────────────
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
        ffmpeg \
        ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=onnx-fetcher /tmp/libonnxruntime.so /usr/lib/libonnxruntime.so
RUN ldconfig

COPY --from=go-builder /castle /usr/local/bin/castle
COPY --from=model-fetcher /models/yolov8n.onnx /models/yolov8n.onnx

VOLUME ["/config", "/recordings"]
EXPOSE 8080

ENV ONNX_RUNTIME_LIB=/usr/lib/libonnxruntime.so

ENTRYPOINT ["castle", "-config", "/config/castle.yaml"]
