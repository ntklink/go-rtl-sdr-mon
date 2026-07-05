.PHONY: all build web clean dev run help

BINARY_NAME := go-rtl-sdr-mon
BIN_DIR := bin
WEB_DIR := web

# Go build flags
GO_FLAGS := -trimpath -ldflags="-s -w"

# Default target
all: build

## build: Build the complete single binary (frontend + backend) into bin/
build: web
	@echo "==> Building Go binary..."
	@mkdir -p $(BIN_DIR)
	go build $(GO_FLAGS) -o $(BIN_DIR)/$(BINARY_NAME) .
	@echo "==> Built $(BIN_DIR)/$(BINARY_NAME)"

## web: Build the Vue frontend into web/dist/
web:
	@echo "==> Building frontend..."
	cd $(WEB_DIR) && npm install && npm run build

## dev: Start frontend dev server (with API proxy to backend)
dev:
	@echo "==> Starting frontend dev server..."
	cd $(WEB_DIR) && npm run dev

## run: Build and run the server
run: build
	./$(BIN_DIR)/$(BINARY_NAME)

## clean: Remove build artifacts
clean:
	@echo "==> Cleaning..."
	rm -rf $(BIN_DIR)
	rm -rf $(WEB_DIR)/dist
	rm -rf $(WEB_DIR)/node_modules

## help: Show this help
help:
	@echo "Available targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //; s/:/:\n   /'
