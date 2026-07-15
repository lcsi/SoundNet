# =============================================================================
# Linux Player Build Environment
# =============================================================================
# Builds the player binary for Linux AMD64 with ALSA audio support.
#
# Usage (via docker-compose):
#   docker compose run --rm build-linux
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

RUN apt-get install -y --no-install-recommends \
    libasound2-dev \
    && rm -rf /var/lib/apt/lists/*
    
RUN cd /opt/faad2-src && mkdir build && cd build \
    && cmake .. \
       -G "Unix Makefiles" \
       -DBUILD_SHARED_LIBS=OFF \
       -DCMAKE_INSTALL_PREFIX="/opt/faad2-install" \
    && make -j$(nproc) \
    && make install \
    && echo "FAAD2 built successfully"


# =============================================================================
# Stage 3: Build the player
# =============================================================================
WORKDIR /app

# Build environment for Linux with ALSA
ENV CGO_ENABLED=1
ENV CGO_CFLAGS="-DMA_ENABLE_ALSA -I/opt/faad2-install/include"
ENV CGO_LDFLAGS="-L/opt/faad2-install/lib -lfaad -lasound -lpthread -lm -ldl"

# Build the binary
#RUN go build -ldflags='-s -w' -o /output/player .

# Verify
#RUN ls -lh /output/player && echo "=== Linux build complete ==="
