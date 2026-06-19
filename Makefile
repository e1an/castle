.PHONY: build ui go docker docker-up docker-down clean

build: ui go

ui:
	cd web && npm run build
	rm -rf internal/api/ui
	cp -r web/dist internal/api/ui

go:
	CGO_ENABLED=1 go build -tags onnx ./cmd/castle

docker:
	docker build -f docker/Dockerfile -t castle:latest .

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

clean:
	rm -rf web/dist internal/api/ui
	go clean
