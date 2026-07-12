.PHONY: all build web clean dev run help \
       build-amd64 build-arm64 build-arm build-all dist release

BINARY_NAME := goether-sdr
BIN_DIR := bin
DIST_DIR := dist
WEB_DIR := web

# Go build flags
GO_FLAGS := -trimpath -ldflags="-s -w"

# Cross-compilation targets (Docker buildx)
PLATFORMS := amd64 arm64 arm
# Docker platform mapping (arm = arm/v7)
DOCK_PLATFORM_arm := linux/arm/v7
DOCK_PLATFORM_amd64 := linux/amd64
DOCK_PLATFORM_arm64 := linux/arm64

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

## build-amd64: Cross-compile for linux/amd64 (x86_64) via Docker
build-amd64:
	@$(MAKE) build-docker TARGET=amd64

## build-arm64: Cross-compile for linux/arm64 (aarch64) via Docker
build-arm64:
	@$(MAKE) build-docker TARGET=arm64

## build-arm: Cross-compile for linux/arm (armv7) via Docker
build-arm:
	@$(MAKE) build-docker TARGET=arm

## build-all: Cross-compile for all supported architectures
build-all: $(addprefix build-,$(PLATFORMS))

# Internal: build a single platform using Docker buildx + QEMU
build-docker:
	@echo "==> Building for linux/$(TARGET) via Docker..."
	@mkdir -p $(BIN_DIR)
	docker buildx build \
		--platform $(DOCK_PLATFORM_$(TARGET)) \
		--target export \
		--output type=local,dest=$(BIN_DIR)/tmp-$(TARGET) \
		.
	@mv $(BIN_DIR)/tmp-$(TARGET)/$(BINARY_NAME) $(BIN_DIR)/$(BINARY_NAME)-linux-$(TARGET)
	@rm -rf $(BIN_DIR)/tmp-$(TARGET)
	@echo "==> Built $(BIN_DIR)/$(BINARY_NAME)-linux-$(TARGET)"

## dist: Build all architectures and package into tarballs in dist/
dist: build-all
	@echo "==> Packaging distributions..."
	@mkdir -p $(DIST_DIR)
	@for target in $(PLATFORMS); do \
		bin=$(BIN_DIR)/$(BINARY_NAME)-linux-$$target; \
		tarball=$(DIST_DIR)/$(BINARY_NAME)-linux-$$target.tar.gz; \
		tar czf $$tarball -C $(BIN_DIR) $$(basename $$bin); \
		echo "  -> $$tarball"; \
	done
	@echo "==> Done. Distributions in $(DIST_DIR)/"

## release: Same as dist (used by CI; creates release artifacts)
release: dist

## clean: Remove build artifacts
clean:
	@echo "==> Cleaning..."
	rm -rf $(BIN_DIR) $(DIST_DIR) $(WEB_DIR)/dist $(WEB_DIR)/node_modules

## help: Show this help
help:
	@echo "Available targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //; s/:/:\n   /'
