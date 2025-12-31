FRONTEND_DIR := web
BINARY := bin/cups-web

.PHONY: all frontend build clean docker-build
all: frontend build

frontend:
	@echo "Building frontend (expects Bun)..."
	cd $(FRONTEND_DIR) && bun install || true
	cd $(FRONTEND_DIR) && bunx vite build || bun run build

build:
	@echo "Building Go binary..."
	go build -o $(BINARY) ./cmd/server

clean:
	rm -f $(BINARY)

docker-build:
	docker build -t cups-web:latest -f Dockerfile .
	docker build -t cups:latest -f cups/Dockerfile cups

