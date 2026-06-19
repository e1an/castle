# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Castle is a self-hosted NVR (Network Video Recorder). It ingests RTSP camera streams via FFmpeg, runs motion detection (and optionally YOLOv8 object detection), records HLS clips, stores events in SQLite, and serves a React UI — all as a single Go binary or Docker container.

## Build commands

```bash
make build          # build UI then Go binary (most common)
make ui             # build React → web/dist, copy to internal/api/ui/
make go             # CGO_ENABLED=1 go build -tags onnx ./cmd/castle
make docker         # build Docker image (ONNX + FFmpeg included)
make docker-up      # docker compose up --build -d
make clean          # remove web/dist and internal/api/ui
```

The UI **must be built before the Go binary** because `internal/api/api.go` embeds `internal/api/ui/` via `//go:embed ui`. Running `make go` without `make ui` first will fail if `internal/api/ui/` is absent.

ONNX is always compiled in (`-tags onnx`). Object and face detection activate automatically when their model paths are configured; each camera can individually disable them.

Frontend dev (with hot reload, proxied to a running backend on :8080):
```bash
cd web && npm run dev
```

Lint the frontend:
```bash
cd web && npm run lint
```

There are no Go tests currently. Run the binary directly for manual testing:
```bash
go run ./cmd/castle -config castle.yaml
```

Requirements: Node 20+, Go 1.26+. FFmpeg must be installed at runtime (not build time).

## Architecture

### Go backend

**`cmd/castle/main.go`** — entry point. Wires all packages together: loads config, opens the SQLite store, creates the stream manager, starts per-camera goroutines, starts a daily retention purge goroutine, and starts the HTTP server. The `reloadFn` closure stops all streams and restarts them with new config; it is passed into the API server and triggered by `PUT /api/config`.

**`config/config.go`** — YAML config struct with `Load`, `Save`, and `Default`. The config file path defaults to `castle.yaml` and is passed in via `-config` flag.

**`internal/stream/stream.go`** — `Manager` owns all camera `Stream`s. Each `Stream` starts up to three FFmpeg subprocesses in goroutines:
1. **HLS segmenter** — writes a rolling `live.m3u8` + 2-second `.ts` segments to `{recDir}/{camID}/`. Used by the live view and as the source for motion clips.
2. **Frame pipe** — pipes raw RGB24 frames at 2 fps via stdout for motion/object detection.
3. **Continuous recorder** — (optional, `continuous_mode: true`) writes 5-minute MP4 chunks to `{recDir}/{camID}/continuous/`. Opens a separate RTSP connection.

All FFmpeg processes auto-restart on exit (3–5 s delay).

**`internal/detect/motion.go`** — pure-Go motion detector using mean pixel difference on grayscale frames. Runs first to gate the more expensive object detection.

**`internal/detect/detector.go`** — `ObjectDetector` interface.

**`internal/detect/object.go`** — YOLOv8n ONNX implementation, compiled only with `-tags onnx`. `internal/detect/object_stub.go` provides the no-op fallback for standard builds. The ONNX Runtime session is **not goroutine-safe**; all inference is serialized behind `detMu` in `main.go`.

**`internal/record/record.go`** — `Recorder.Clip()` assembles a motion clip by copying the pre-event HLS `.ts` segments already on disk (avoiding a second RTSP connection), then polling for post-event segments, then concating everything into an MP4 via FFmpeg's concat demuxer. `Purge` and `PurgeContinuous` delete files older than `retention_days`.

**`internal/events/events.go`** — SQLite event store (using `modernc.org/sqlite`, CGO-free). Schema auto-migrated on `Open`. Events are either `motion` or `object` type; object events include a COCO label and confidence score.

**`internal/api/api.go`** — HTTP server. Routes:
- `GET /health`
- `GET /api/events?camera_id=&limit=`
- `GET /api/config` / `PUT /api/config` (saves YAML + triggers reload)
- `POST /api/reload` (reload from disk)
- `POST /api/test-stream` (runs `ffprobe` against a URL, 8 s timeout)
- `GET /recordings/` (file server for clips and HLS segments)
- `/` — serves embedded React SPA; unknown paths fall back to `index.html`

Basic auth is enabled by setting `CASTLE_USER` / `CASTLE_PASS` environment variables.

**`internal/notify/notify.go`** — fires webhook POST (JSON) and/or ntfy push notification asynchronously on each event.

### React frontend (`web/`)

Vite + React 19 + TypeScript. No state management library. `hls.js` handles HLS playback in `LiveView`. During development, Vite proxies `/api` and `/recordings` to `http://localhost:8080`.

Key components:
- **`App.tsx`** — root layout, polls `/api/events` every 10 s, manages view state (`live` | `config`) and camera selection
- **`LiveView.tsx`** — hls.js player pointing at `/recordings/{cameraID}/live.m3u8`; auto-recovers on fatal HLS errors
- **`ClipPlayer.tsx`** — `<video>` for recorded MP4 clips
- **`ConfigPanel.tsx`** — full config editor; "Save & Reload" calls `PUT /api/config` which saves YAML and hot-reloads streams
- **`AddCameraWizard.tsx`** — multi-step wizard that test-streams via `POST /api/test-stream` before adding
- **`web/src/api.ts`** — all fetch calls; `VITE_API_URL` env var overrides the base URL (empty = same origin)
- **`web/src/types.ts`** — TypeScript mirrors of Go config/event structs

### Recording layout on disk

```
{record.path}/
  castle.db               ← SQLite event store
  {camera-id}/
    live.m3u8             ← rolling HLS playlist (live view)
    seg00001.ts           ← 2-second HLS segments (deleted_segments flag keeps ~6 on disk)
    clips/
      {id}_{ts}.mp4       ← motion-triggered clips
    continuous/
      20240101T120000Z.mp4 ← 5-minute chunks (continuous_mode only)
```

### Object and face detection

`internal/detect/object.go` and `internal/detect/face.go` have `//go:build onnx`. The standard build always uses `-tags onnx`. Stub files (`object_stub.go`, `face_stub.go`) provide no-op fallbacks so the package compiles without the tag too. Models are loaded at startup only when their paths are configured (`model_path`, `face_model_path`). If a model path is set, detection is on by default for every camera; individual cameras can disable it via `enable_object_detect: false` or `enable_face_detect: false` in their detect config. Set `ONNX_RUNTIME_LIB` to the `libonnxruntime.so` path or put it on `LD_LIBRARY_PATH`.
