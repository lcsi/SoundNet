// Package audio provides a cross-platform audio engine using miniaudio.
//
// The underlying C implementation (bridge.c + bridge.h) wraps miniaudio,
// which handles decoding (MP3, FLAC, Vorbis, WAV) and output via
// platform-specific backends:
//   - Android: OpenSL ES
//   - Linux: ALSA, PulseAudio
//   - Windows: WASAPI, DirectSound
//
// The backend is selected at compile time via CFLAGS:
//   -DMA_ENABLE_OPENSL   (Android)
//   -DMA_ENABLE_ALSA     (Linux)
//   -DMA_ENABLE_WASAPI   (Windows)
//
// Usage:
//
//	audioInit()
//	defer audioDestroy()
//
//	audioLoadFile("/path/to/song.mp3")
//	audioPlay()
//	audioSetVolume(80)
//	time.Sleep(5 * time.Second)
//	audioPause()
package audio
