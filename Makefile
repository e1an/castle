.PHONY: build ui go go-onnx docker clean

build: ui go

ui:
	cd web && npm run build
	rm -rf internal/api/ui
	cp -r web/dist internal/api/ui

go:
	go build ./...

go-onnx:
	go build -tags onnx ./...

docker:
	docker build -f docker/Dockerfile -t castle:latest .

docker-onnx:
	docker build -f docker/Dockerfile.onnx -t castle:onnx .

docker-up:
	docker compose up --build -d

docker-up-onnx:
	docker compose -f docker-compose.yml -f docker-compose.onnx.yml up --build -d

docker-down:
	docker compose down

clean:
	rm -rf web/dist internal/api/ui
	go clean
