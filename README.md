# Castle

Lightweight, self-hosted network video recorder (NVR). RTSP stream ingestion, motion detection, optional YOLOv8 object detection, HLS recording, and a React web UI — packaged as a single Docker container.

## Quick start

```bash
mkdir -p config recordings
cp castle.example.yaml config/castle.yaml
# edit config/castle.yaml with your RTSP URLs
docker compose up --build -d
```

Open **http://localhost:8080**.

See [docs/quickstart.md](docs/quickstart.md) for full setup instructions and [docs/configuration.md](docs/configuration.md) for all config options.

## Building from source

```bash
make build          # UI + Go binary
make docker         # Docker image
make go-onnx        # with YOLOv8 object detection
```

Requires Node 20+ and Go 1.26+.

## Object detection (optional)

Build with `-tags onnx` and place `yolov8n.onnx` in `/config/`. Set `detect.enable_object_detect: true` in your config. See [docs/configuration.md](docs/configuration.md).

## License

MIT
