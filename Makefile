# ──────────────────────────────
# Makefile for KENT exporter
# ──────────────────────────────

APP_NAME := kent
IMAGE := staslevchenko/$(APP_NAME)
TAG ?= $(shell git describe --tags --always --dirty)
PLATFORMS ?= linux/amd64,linux/arm64
GOOS ?= linux
GOARCH ?= amd64
OUTPUT_DIR := bin
BINARY := $(OUTPUT_DIR)/$(APP_NAME)-$(GOOS)-$(GOARCH)

DOCKER_BUILDX := docker buildx

.PHONY: all build docker push clean lint test

# ────────────────
# Local build
# ────────────────
build:
	@echo "→ Building for $(GOOS)/$(GOARCH)..."
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -ldflags="-s -w" -o $(BINARY) ./cmd
	@echo "✔ Binary created: $(BINARY)"

# ────────────────
# Multi-arch image build & push
# ────────────────
docker:
	@echo "→ Building multi-arch Docker image ($(PLATFORMS)) with tag $(TAG)"
	$(DOCKER_BUILDX) build \
		--platform $(PLATFORMS) \
		-t $(IMAGE):$(TAG) \
		-t $(IMAGE):latest \
		--push .

# ────────────────
# Optional lint/test targets (will be added in a while)
# ────────────────
lint:
	@golangci-lint run ./...

test:
	@go test ./... -v

# ────────────────
# Clean artifacts
# ────────────────
clean:
	rm -rf $(OUTPUT_DIR)
