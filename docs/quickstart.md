# Castle Quickstart

## Prerequisites

- Docker + Docker Compose
- RTSP camera(s) accessible from the Docker host

## 1. Create a config file

```bash
mkdir -p config recordings
cp castle.example.yaml config/castle.yaml
```

Edit `config/castle.yaml` to add your camera RTSP URLs.

## 2. Start Castle

```bash
docker compose up -d
```

The web UI is available at **http://localhost:8080**.

## 3. Verify it's running

```bash
docker compose logs -f castle
```

Recordings land in `./recordings/`.

## Stopping

```bash
docker compose down
```

## Upgrading

```bash
docker compose pull   # if using a published image
docker compose up -d --build
```
