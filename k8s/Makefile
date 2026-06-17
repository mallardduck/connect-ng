.PHONY: sync-deps help tidy build test

# sync-deps: Synchronize a dependency version across k8s and examples/rancher
# Usage: make sync-deps PKG=k8s.io/api@v0.33.0
# Usage: make sync-deps PKG="k8s.io/api@v0.33.0 k8s.io/apimachinery@v0.33.0"
sync-deps:
	@if [ -z "$(PKG)" ]; then \
		echo "Error: PKG is required"; \
		echo "Usage: make sync-deps PKG=package@version"; \
		echo "Example: make sync-deps PKG=k8s.io/api@v0.33.0"; \
		echo "Example: make sync-deps PKG=\"k8s.io/api@v0.33.0 k8s.io/apimachinery@v0.33.0\""; \
		exit 1; \
	fi
	@echo "Updating dependencies in k8s module..."
	go get $(PKG)
	@echo "Updating dependencies in examples/rancher module..."
	cd examples/rancher && go get $(PKG)
	@echo "Running go mod tidy in both modules..."
	go mod tidy
	cd examples/rancher && go mod tidy
	@echo "✓ Dependencies synchronized"

# sync-k8s-deps: Sync common k8s.io dependencies to a specific version
# Usage: make sync-k8s-deps VERSION=v0.33.0
sync-k8s-deps:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required"; \
		echo "Usage: make sync-k8s-deps VERSION=v0.33.0"; \
		exit 1; \
	fi
	@echo "Syncing k8s.io packages to $(VERSION)..."
	$(MAKE) sync-deps PKG="k8s.io/api@$(VERSION) k8s.io/apimachinery@$(VERSION) k8s.io/client-go@$(VERSION)"

# sync-controller-runtime: Sync controller-runtime across modules
# Usage: make sync-controller-runtime VERSION=v0.21.0
sync-controller-runtime:
	@if [ -z "$(VERSION)" ]; then \
		echo "Error: VERSION is required"; \
		echo "Usage: make sync-controller-runtime VERSION=v0.21.0"; \
		exit 1; \
	fi
	@echo "Syncing controller-runtime to $(VERSION)..."
	$(MAKE) sync-deps PKG="sigs.k8s.io/controller-runtime@$(VERSION)"

# tidy: Run go mod tidy in both modules
tidy:
	@echo "Running go mod tidy in k8s module..."
	go mod tidy
	@echo "Running go mod tidy in examples/rancher module..."
	cd examples/rancher && go mod tidy
	@echo "✓ Tidy complete"

# build: Build all packages
build:
	@echo "Building k8s packages..."
	go build ./...
	@echo "Building examples/rancher..."
	cd examples/rancher && go build ./...
	@echo "✓ Build complete"

# test: Run tests in all modules
test:
	@echo "Testing k8s packages..."
	go test ./...
	@echo "Testing examples/rancher..."
	cd examples/rancher && go test ./...
	@echo "✓ Tests complete"

# help: Show this help message
help:
	@echo "k8s Module Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  sync-deps PKG=package@version    - Sync a dependency across k8s and examples/rancher"
	@echo "  sync-k8s-deps VERSION=vX.Y.Z     - Sync all k8s.io/* packages to a version"
	@echo "  sync-controller-runtime VERSION  - Sync controller-runtime to a version"
	@echo "  tidy                              - Run go mod tidy in all modules"
	@echo "  build                             - Build all packages"
	@echo "  test                              - Run tests in all modules"
	@echo "  help                              - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make sync-deps PKG=k8s.io/api@v0.33.0"
	@echo "  make sync-deps PKG=\"k8s.io/api@v0.33.0 k8s.io/apimachinery@v0.33.0\""
	@echo "  make sync-k8s-deps VERSION=v0.33.0"
	@echo "  make sync-controller-runtime VERSION=v0.21.0"
