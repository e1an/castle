# Configuration Reference

Castle is configured via a YAML file (default: `/config/castle.yaml`).

Pass a custom path with `-config /path/to/castle.yaml`.

## server

| Key | Default | Description |
|-----|---------|-------------|
| `host` | `0.0.0.0` | Bind address |
| `port` | `8080` | HTTP port |

## cameras

A list of camera objects.

| Key | Required | Description |
|-----|----------|-------------|
| `id` | yes | Unique slug used in API paths and filenames |
| `name` | yes | Human-readable label |
| `url` | yes | RTSP stream URL |
| `enable` | yes | `true` to activate the camera |

## record

| Key | Default | Description |
|-----|---------|-------------|
| `path` | `/recordings` | Directory for HLS segments and clips |
| `segment_duration` | `10` | Seconds per HLS segment |
| `retention_days` | `7` | Delete clips older than N days |
| `continuous_mode` | `false` | `true` = always record; `false` = motion-triggered |

## detect

| Key | Default | Description |
|-----|---------|-------------|
| `motion_threshold` | `0.02` | Fraction of changed pixels (lower = more sensitive) |
| `min_object_score` | `0.5` | Minimum YOLO confidence to emit a detection |
| `model_path` | `/config/yolov8n.onnx` | Path to YOLOv8n ONNX model |
| `enable_object_detect` | `false` | Enable ONNX object detection (requires model file and `onnx` build tag) |

## notify

| Key | Default | Description |
|-----|---------|-------------|
| `webhook_url` | `""` | POST JSON event payload to this URL |
| `ntfy_topic` | `""` | ntfy.sh topic URL, e.g. `https://ntfy.sh/my-castle` |
