#ifndef AUDIO_BRIDGE_H
#define AUDIO_BRIDGE_H

#include <stdbool.h>
#include <stdint.h>

/*
 * Audio Bridge - Cross-platform C interface for miniaudio
 *
 * This layer wraps miniaudio (https://miniaud.io) which handles:
 *   - Audio decoding: MP3, FLAC, Vorbis, WAV, PCM, etc.
 *   - Audio output: Platform-specific backends
 *     * Android: OpenSL ES (via dlopen)
 *     * Linux: ALSA, PulseAudio
 *     * Windows: WASAPI, DirectSound
 *
 * The audio backend is selected at compile time via CFLAGS:
 *   -DMA_ENABLE_OPENSL   (Android)
 *   -DMA_ENABLE_ALSA     (Linux)
 *   -DMA_ENABLE_WASAPI   (Windows)
 *
 * Usage:
 *   1. audio_init()     - Initialize the audio engine
 *   2. audio_load()     - Load a local audio file (.mp3, .wav, etc.)
 *   3. audio_play()     - Start playback
 *   4. audio_get_position() / audio_get_duration() - Query state
 *   5. audio_pause() / audio_stop() / audio_seek() - Control
 *   6. audio_destroy()  - Cleanup on exit
 */

// --- Lifecycle ---

// Initialize the audio engine.
// Must be called once before any other audio functions.
// Returns 0 on success, -1 on failure.
int audio_init(void);

// Load an audio file from a local file path.
// Stops and unloads any previously loaded sound.
// Returns 0 on success, -1 on failure.
int audio_load(const char* filepath);

// Cleanup all audio resources.
void audio_destroy(void);

// --- Playback Control ---

// Start or resume playback.
void audio_play(void);

// Pause playback (current position preserved).
void audio_pause(void);

// Stop playback and reset to beginning.
void audio_stop(void);

// Seek to the specified position (in seconds from start).
void audio_seek(float seconds);

// Set volume (0.0 = silent, 1.0 = max).
void audio_set_volume(float volume);

// --- State Queries ---

// Get current playback position in seconds.
float audio_get_position(void);

// Get total duration of the loaded audio in seconds.
// Returns 0 if no audio is loaded.
float audio_get_duration(void);

// Returns true if audio is currently playing.
bool audio_is_playing(void);

// --- Device Change Handling ---

// Call this periodically (e.g., from main loop) to handle device changes.
// This must NOT be called from audio callbacks.
void audio_update(void);

// --- Callbacks ---

// Callback type for audio playback completion.
typedef void (*audio_finished_callback)(void);

// Register a callback that will be invoked when playback finishes.
void audio_set_finished_callback(audio_finished_callback cb);

#endif /* AUDIO_BRIDGE_H */
