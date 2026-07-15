# =============================================================================
# Windows Player Build Environment
# =============================================================================
# Cross-compiles the player binary for Windows AMD64 with WASAPI audio.
#
# Usage (via docker-compose):
#   docker compose run --rm build-windows
#
# Note: This builds a Windows executable using MinGW cross-compiler.
# The resulting .exe can run on Windows with WASAPI audio support.
# =============================================================================

FROM golang:1.22-bullseye AS base

# --- Install system dependencies ---
RUN apt-get update && apt-get install -y --no-install-recommends \
    cmake \
    make \
    wget \
    unzip 

# =============================================================================
# Stage 1: Build FAAD2 (AAC decoder) for Windows
# =============================================================================
ARG FAAD2_VERSION=2.11.1

RUN wget -qO /tmp/faad2.tar.gz \
    "https://github.com/knik0/faad2/archive/refs/tags/${FAAD2_VERSION}.tar.gz" \
    && mkdir -p /opt/faad2-src \
    && tar xzf /tmp/faad2.tar.gz -C /opt/faad2-src --strip-components=1 \
    && rm /tmp/faad2.tar.gz

ARG MINIAUDIO_VERSION=0.11.25

# Download miniaudio header
RUN mkdir /opt/miniaudio && wget -qO /opt/miniaudio/miniaudio.h \
    "https://raw.githubusercontent.com/mackron/miniaudio/${MINIAUDIO_VERSION}/miniaudio.h"

# --- Install system dependencies ---
RUN apt-get install -y --no-install-recommends \
    gcc-mingw-w64-x86-64 \
    && rm -rf /var/lib/apt/lists/*
    
# Cross-compile FAAD2 for Windows using MinGW
RUN cd /opt/faad2-src && mkdir build && cd build \
    && cmake .. \
       -G "Unix Makefiles" \
       -DCMAKE_SYSTEM_NAME=Windows \
       -DCMAKE_C_COMPILER=x86_64-w64-mingw32-gcc \
       -DCMAKE_CXX_COMPILER=x86_64-w64-mingw32-g++ \
       -DBUILD_SHARED_LIBS=OFF \
       -DCMAKE_INSTALL_PREFIX="/opt/faad2-install" \
    && make -j$(nproc) \
    && make install \
    && echo "FAAD2 built successfully for Windows"

WORKDIR /app

ENV GOOS=windows
ENV GOARCH=amd64
ENV CGO_ENABLED=1
ENV CC=x86_64-w64-mingw32-gcc
ENV CXX=x86_64-w64-mingw32-g++

# Build environment for Windows with WASAPI
# Note: Windows audio libraries are linked automatically by miniaudio
ENV CGO_CFLAGS="-DMA_ENABLE_WASAPI -I/opt/faad2-install/include -I/opt/miniaudio"
ENV CGO_LDFLAGS="-L/opt/faad2-install/lib -lfaad -lole32 -loleaut32 -lwinmm -lm"

# Build the binary
#RUN go mod tidy && go build -ldflags='-s -w' -o /output/player.exe .

# Verify
#RUN ls -lh /output/player.exe && echo "=== Windows build complete ==="
