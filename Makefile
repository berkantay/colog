# Colog Makefile
# Build and release automation for Docker container log viewer

# Variables
APP_NAME := colog
VERSION := $(shell git describe --tags --always --dirty)
COMMIT := $(shell git rev-parse --short HEAD)
DATE := $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Build flags
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.Date=$(DATE)"

# Directories
BUILD_DIR := releases
DIST_DIR := dist

# Platforms
PLATFORMS := \
	darwin/amd64 \
	darwin/arm64 \
	linux/amd64 \
	linux/arm64 \
	linux/386 \
	windows/amd64 \
	windows/386

.PHONY: all build clean test deps help install release compress checksums

# Default target
all: clean deps test build

# Help target
help:
	@echo "Colog Build System"
	@echo ""
	@echo "Available targets:"
	@echo "  build       - Build binary for current platform"
	@echo "  build-all   - Build binaries for all platforms"
	@echo "  clean       - Remove build artifacts"
	@echo "  test        - Run tests"
	@echo "  deps        - Download dependencies"
	@echo "  install     - Install binary to GOPATH/bin"
	@echo "  release     - Create GitHub release with all binaries"
	@echo "  compress    - Compress all binaries"
	@echo "  checksums   - Generate checksums for binaries"
	@echo "  docker      - Build Docker image"
	@echo "  sdk-test    - Test SDK functionality"
	@echo "  help        - Show this help message"
	@echo ""
	@echo "Version: $(VERSION)"
	@echo "Commit:  $(COMMIT)"

# Download dependencies
deps:
	@echo "📦 Downloading dependencies..."
	go mod download
	go mod tidy

# Run tests
test:
	@echo "🧪 Running tests..."
	go test -v ./...

# Build for current platform
build: deps
	@echo "🔨 Building $(APP_NAME) for current platform..."
	go build $(LDFLAGS) -o $(APP_NAME) .

# Build for all platforms
build-all: clean deps $(BUILD_DIR)
	@echo "🔨 Building $(APP_NAME) for all platforms..."
	@$(foreach platform,$(PLATFORMS), \
		$(call build_platform,$(platform)) \
	)
	@echo "✅ All builds completed"

# Build function for each platform
define build_platform
	$(eval GOOS := $(word 1,$(subst /, ,$1)))
	$(eval GOARCH := $(word 2,$(subst /, ,$1)))
	$(eval EXT := $(if $(filter windows,$(GOOS)),.exe,))
	$(eval OUTPUT := $(BUILD_DIR)/$(APP_NAME)-$(GOOS)-$(GOARCH)$(EXT))
	@echo "Building $(OUTPUT)..."
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go build $(LDFLAGS) -o $(OUTPUT) .
endef

# Create build directory
$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

# Create distribution directory
$(DIST_DIR):
	mkdir -p $(DIST_DIR)

# Compress binaries
compress: build-all $(DIST_DIR)
	@echo "📦 Compressing binaries..."
	@for binary in $(BUILD_DIR)/*; do \
		basename=$$(basename $$binary); \
		if [[ $$basename == *".exe" ]]; then \
			zip -j $(DIST_DIR)/$$basename.zip $$binary; \
		else \
			tar -czf $(DIST_DIR)/$$basename.tar.gz -C $(BUILD_DIR) $$basename; \
		fi; \
		echo "✅ Compressed $$basename"; \
	done

# Generate checksums
checksums: compress
	@echo "🔐 Generating checksums..."
	@cd $(DIST_DIR) && sha256sum * > checksums.txt
	@echo "✅ Checksums generated in $(DIST_DIR)/checksums.txt"

# Clean build artifacts
clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -rf $(BUILD_DIR) $(DIST_DIR) $(APP_NAME)

# Install binary
install: build
	@echo "📥 Installing $(APP_NAME) to GOPATH/bin..."
	go install $(LDFLAGS) .

# Test SDK functionality
sdk-test: build
	@echo "🧪 Testing SDK functionality..."
	@echo "Testing SDK help..."
	./$(APP_NAME) sdk --help
	@echo ""
	@echo "Testing SDK list command..."
	./$(APP_NAME) sdk list --help
	@echo ""
	@echo "✅ SDK tests passed"

# Docker build
docker:
	@echo "🐳 Building Docker image..."
	docker build -t $(APP_NAME):$(VERSION) .
	docker build -t $(APP_NAME):latest .
	@echo "✅ Docker image built: $(APP_NAME):$(VERSION)"

# Quick release (build, compress, checksums)
quick-release: build-all compress checksums
	@echo "🚀 Quick release ready in $(DIST_DIR)/"
	@ls -la $(DIST_DIR)/

# Full GitHub release
release: quick-release
	@echo "🚀 Creating GitHub release v$(VERSION)..."
	@if command -v gh >/dev/null 2>&1; then \
		gh release create v$(VERSION) \
			--title "Colog v$(VERSION) - Docker Log Viewer with SDK" \
			--notes "$$(cat <<'EOF'\n## 🎉 Colog v$(VERSION)\n\n### New in this release:\n- 🔧 **SDK Mode**: Programmatic access to Docker container logs\n- 🤖 **LLM Integration**: Export logs in JSON/Markdown for AI analysis\n- 📊 **Batch Operations**: Process multiple containers simultaneously\n- 🔍 **Smart Filtering**: Filter containers by name, image, status, labels\n- 💻 **CLI Commands**: SDK subcommands for automation\n- 📚 **Comprehensive Docs**: Complete SDK documentation and examples\n\n### SDK Usage:\n\`\`\`bash\n# List containers\ncolog sdk list\n\n# Export logs for LLM analysis\ncolog sdk export --format markdown\n\n# Filter and get logs\ncolog sdk filter --image nginx\n\`\`\`\n\n### Downloads:\nChoose the binary for your platform:\n- **macOS**: colog-darwin-amd64.tar.gz (Intel) or colog-darwin-arm64.tar.gz (Apple Silicon)\n- **Linux**: colog-linux-amd64.tar.gz (x64) or colog-linux-arm64.tar.gz (ARM64)\n- **Windows**: colog-windows-amd64.exe.zip\n\n### Installation:\n\`\`\`bash\n# macOS/Linux\ntar -xzf colog-*.tar.gz && sudo mv colog-* /usr/local/bin/colog\n\n# Or via Go\ngo install github.com/berkantay/colog@v$(VERSION)\n\`\`\`\n\nFull documentation: [SDK_README.md](https://github.com/berkantay/colog/blob/main/SDK_README.md)\nEOF\n)" \
			$(DIST_DIR)/*; \
		echo "✅ GitHub release created successfully"; \
	else \
		echo "❌ GitHub CLI (gh) not found. Please install it or create the release manually."; \
		echo "Upload these files to GitHub releases:"; \
		ls -la $(DIST_DIR)/; \
	fi

# Development targets
dev-build:
	@echo "🛠️  Building development version..."
	go build -race $(LDFLAGS) -o $(APP_NAME)-dev .

dev-run: dev-build
	@echo "🏃 Running development version..."
	./$(APP_NAME)-dev

# Lint code
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "🔍 Linting code..."; \
		golangci-lint run; \
	else \
		echo "⚠️  golangci-lint not found, using go vet"; \
		go vet ./...; \
	fi

# Format code
fmt:
	@echo "💅 Formatting code..."
	go fmt ./...

# Update version and create tag
version:
	@if [ -z "$(V)" ]; then \
		echo "❌ Please specify version: make version V=1.3.0"; \
		exit 1; \
	fi
	@echo "🏷️  Creating version $(V)..."
	git tag v$(V)
	git push origin v$(V)
	@echo "✅ Version v$(V) created and pushed"

# Show build info
info:
	@echo "Build Information:"
	@echo "  App Name: $(APP_NAME)"
	@echo "  Version:  $(VERSION)"
	@echo "  Commit:   $(COMMIT)"
	@echo "  Date:     $(DATE)"
	@echo "  Platforms: $(words $(PLATFORMS)) platforms"
	@echo ""
	@go version

# Benchmark
benchmark:
	@echo "📈 Running benchmarks..."
	go test -bench=. -benchmem ./...

.PHONY: dev-build dev-run lint fmt version info benchmark quick-release