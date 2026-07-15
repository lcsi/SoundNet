# =============================================================================
# Multi-Platform Player Makefile
# =============================================================================
# Provides convenient shortcuts for building via Docker.
#
# Prerequisites:
#   - Docker & Docker Compose
#
# Usage:
#   make build              # Build for all platforms
#   make build-linux        # Build for Linux only
#   make build-android      # Build for Android only
#   make build-windows      # Build for Windows only
#   make clean              # Remove build artifacts
#   make shell-linux        # Open interactive shell in Linux build container
#   make shell-android      # Open interactive shell in Android build container
# =============================================================================

.PHONY: all build build-linux build-android build-windows clean \
        shell-linux shell-android shell-windows

# Default: build all platforms
all: build

# --- Build all platforms ---
build: build-linux build-android build-windows
	@echo "=== All platforms built successfully ==="
	@ls -lh build/output/linux/ build/output/android/ build/output/windows/

# --- Build for Linux ---
build-linux:
	@mkdir -p build/output/linux
	cd build/docker && docker compose run --rm build-linux

# --- Build for Android ---
build-android:
	@mkdir -p build/output/android
	cd build/docker && docker compose run --rm build-android

# --- Build for Windows ---
build-windows:
	@mkdir -p build/output/windows
	cd build/docker && docker compose run --rm build-windows

# --- Clean build artifacts ---
clean:
	rm -rf build/output/linux build/output/android build/output/windows
	@echo "Cleaned build output directories"

# --- Interactive shells for debugging ---
shell-linux:
	cd build/docker && docker compose run --rm build-linux sh

shell-android:
	cd build/docker && docker compose run --rm build-android sh

shell-windows:
	cd build/docker && docker compose run --rm build-windows sh
