# =============================================================================
# Android Player Build Environment
# =============================================================================
# Cross-compiles the player binary for Android ARM64 with OpenSL ES audio.
#
# Usage (via docker-compose):
#   docker compose run --rm build-android
# =============================================================================

FROM golang:1.22-bullseye AS base

# --- Install system dependencies --- 
RUN apt-get update && apt-get install -y --no-install-recommends \
    cmake \
    make \
    wget \
    unzip 

# download
ARG FAAD2_VERSION=2.11.1

RUN wget -qO /tmp/faad2.tar.gz \
    "https://github.com/knik0/faad2/archive/refs/tags/${FAAD2_VERSION}.tar.gz" \
    && mkdir -p /opt/faad2-src \
    && tar xzf /tmp/faad2.tar.gz -C /opt/faad2-src --strip-components=1 \
    && rm /tmp/faad2.tar.gz

ARG MINIAUDIO_VERSION=0.11.21

# Download miniaudio header
RUN mkdir /opt/miniaudio && wget -qO /opt/miniaudio/miniaudio.h \
    "https://raw.githubusercontent.com/mackron/miniaudio/${MINIAUDIO_VERSION}/miniaudio.h"
    
# =============================================================================
# Stage 1: Android NDK
# =============================================================================
ARG NDK_VERSION=r27c

# Download and extract Android NDK (provides cross-compiler for Android)
RUN wget -qO /tmp/ndk.zip \
    "https://dl.google.com/android/repository/android-ndk-${NDK_VERSION}-linux.zip" \
    && unzip -q /tmp/ndk.zip -d /opt \
    && rm /tmp/ndk.zip \
    && mv /opt/android-ndk-* /opt/android-ndk

# --- Install system dependencies ---
RUN apt-get install -y --no-install-recommends \
    ca-certificates xz-utils \
    && rm -rf /var/lib/apt/lists/*

# NDK environment
ENV NDK_HOME=/opt/android-ndk
ENV TOOLCHAIN=$NDK_HOME/toolchains/llvm/prebuilt/linux-x86_64
ENV PATH=$TOOLCHAIN/bin:$PATH

# =============================================================================
# Stage 2: Build FAAD2 (AAC decoder) for Android ARM64
# =============================================================================

# Cross-compile FAAD2 for Android ARM64
RUN cd /opt/faad2-src && mkdir build && cd build \
    && cmake .. \
       -G "Unix Makefiles" \
       -DCMAKE_TOOLCHAIN_FILE="${NDK_HOME}/build/cmake/android.toolchain.cmake" \
       -DANDROID_ABI="arm64-v8a" \
       -DANDROID_PLATFORM="android-21" \
       -DBUILD_SHARED_LIBS=OFF \
       -DCMAKE_INSTALL_PREFIX="/opt/faad2-install" \
    && make -j$(nproc) \
    && make install \
    && echo "FAAD2 built successfully"

# =============================================================================
# Stage 3: Final builder image
# =============================================================================

# Default cross-compilation environment variables
ENV GOOS=android
ENV GOARCH=arm64
ENV CGO_ENABLED=1
ENV CC=aarch64-linux-android21-clang
ENV CXX=aarch64-linux-android21-clang++

# Verify toolchain
RUN aarch64-linux-android21-clang --version && \
    go version && \
    echo "Build environment ready"

WORKDIR /app

# Copy source code
#COPY go.mod go.sum ./
#COPY cmd/ ./cmd/
#COPY internal/ ./internal/

# Copy FAAD2 headers for CGO
#RUN cp /opt/faad2-install/include/neaacdec.h /app/internal/audio/

# Copy miniaudio.h to audio package
#RUN cp /opt/miniaudio.h /app/internal/audio/

# Build environment for Android with OpenSL ES
ENV CGO_CFLAGS="-DMA_ENABLE_OPENSL -I/opt/faad2-install/include -I/opt/miniaudio"
ENV CGO_LDFLAGS="-L/opt/faad2-install/lib -lfaad -lm -ldl -llog"

# Build the binary
#RUN go build -ldflags='-s -w' -o /output/android-player .

# Verify
#RUN ls -lh /output/android-player && echo "=== Android build complete ==="
