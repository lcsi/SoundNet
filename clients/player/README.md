# Player - Cross-Platform Music Playback Client

A cross-platform music playback client that connects to a music-player backend server via WebSocket and plays audio from remote commands.

## Supported Platforms

| Platform | Architecture | Audio Backend | Binary |
|----------|--------------|---------------|--------|
| Linux | amd64 | ALSA | `player-linux-amd64` |
| Android | arm64 | OpenSL ES | `player-android-arm64` |
| Windows | amd64 | WASAPI | `player-windows-amd64.exe` |

## Features

- WebSocket connection with automatic reconnection
- HTTP API for fetching song URLs
- Audio format detection (MP3, FLAC, OGG, WAV, AAC, M4A)
- AAC/M4A decoding to WAV via FAAD2
- Local caching with conditional HTTP requests
- Cross-platform audio playback via miniaudio

## Prerequisites

- Docker & Docker Compose
- Make (optional, for convenience commands)

## Quick Start

### Build for all platforms

```bash
make build
```

### Build for specific platform

```bash
make build-linux
make build-android
make build-windows
```

### Or use Docker Compose directly

```bash
# Linux
docker compose run --rm build-linux

# Android
docker compose run --rm build-android

# Windows
docker compose run --rm build-windows
```

### Build output

Built binaries are placed in:

```
build/output/
├── linux/
│   └── player-linux-amd64
├── android/
│   └── player-android-arm64
└── windows/
    └── player-windows-amd64.exe
```

## Usage

```bash
# Basic usage
./player-linux-amd64 --server ws://192.168.1.100:8080

# With channel
./player-linux-amd64 --server ws://192.168.1.100:8080 --channel my-music

# With custom name
./player-linux-amd64 --server ws://192.168.1.100:8080 --name LivingRoom --channel main

# Verbose logging
./player-linux-amd64 --server ws://192.168.1.100:8080 -v
```

### Command-line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--server` | `ws://localhost:8080` | Backend server WebSocket address |
| `--player-id` | (auto-generated) | Unique player identifier |
| `--name` | (same as player-id) | Display name for this player |
| `--channel` | (empty) | Channel to join immediately |
| `-v` | `false` | Enable verbose logging |

## Project Structure

```
player/
├── cmd/
│   └── player/
│       └── main.go              # Entry point
├── internal/
│   ├── audio/                   # Audio engine (CGO + miniaudio)
│   │   ├── engine.go
│   │   ├── bridge.c
│   │   ├── bridge.h
│   │   ├── aacbridge.c
│   │   └── aacbridge.h
│   ├── player/                  # Player core logic
│   │   ├── player.go
│   │   └── state.go
│   ├── network/                 # Network communication
│   │   ├── websocket.go
│   │   ├── httpapi.go
│   │   └── download.go
│   └── codec/                   # Audio codecs
│       ├── mp4.go
│       ├── format.go
│       └── decode.go
├── build/
│   ├── miniaudio.h              # Third-party library
│   ├── docker/                  # Docker build environments
│   │   ├── android/
│   │   ├── linux/
│   │   └── windows/
│   └── output/                  # Build artifacts
├── Makefile                     # Build commands
├── docker-compose.yml           # Docker services
├── go.mod
└── README.md
```

## Development

### Local development (Linux only)

```bash
# Install dependencies
sudo apt-get install libasound2-dev

# Build
go build -o player ./cmd/player

# Run
./player --server ws://localhost:8080
```

### Adding a new platform

1. Create a new Dockerfile in `build/docker/<platform>/`
2. Add build service to `docker-compose.yml`
3. Add Makefile target
4. Update README.md

## Architecture

### Audio Backend Selection

The audio backend is selected at compile time via C preprocessor defines:

- **Linux**: `-DMA_ENABLE_ALSA`
- **Android**: `-DMA_ENABLE_OPENSL`
- **Windows**: `-DMA_ENABLE_WASAPI`

These are passed via `CGO_CFLAGS` in the Docker build environment.

### Dependencies

- **miniaudio**: Single-header C library for audio playback
- **FAAD2**: AAC decoder library
- **gorilla/websocket**: WebSocket client for Go

## License

See LICENSE file for details.
