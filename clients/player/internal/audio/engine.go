package audio

/*
#include "bridge.h"
#include "aacbridge.h"
#include <stdlib.h>

// Forward declaration of the Go callback exported below.
// CGO generates the actual C function body in _cgo_export.c.
extern void goAudioFinishedCallback(void);

// These C helpers are implemented in bridge.c (NOT here) so they are
// compiled only once and won't cause duplicate-symbol linker errors.
// CGO's preamble is compiled into two object files per Go file,
// so any function defined here would appear twice at link time.
extern void registerGoCallback(void);
extern void clearGoCallback(void);
extern void audio_update(void);
*/
import "C"
import (
	"fmt"
	"sync"
	"unsafe"
)

// Global state
var (
	audioMu         sync.Mutex
	audioFinishedFn func()
)

//export goAudioFinishedCallback
func goAudioFinishedCallback() {
	audioMu.Lock()
	fn := audioFinishedFn
	audioMu.Unlock()
	if fn != nil {
		fn()
	}
}

// Init initializes the audio engine.
// Must be called once before any other audio function.
func Init() error {
	audioMu.Lock()
	defer audioMu.Unlock()

	ret := C.audio_init()
	if ret != 0 {
		return fmt.Errorf("audio_init failed (returned %d)", int(ret))
	}
	return nil
}

// LoadFile loads an audio file from the given local file path.
// Supported formats: MP3, FLAC, Vorbis, WAV. Any previously loaded sound is unloaded.
func LoadFile(filepath string) error {
	audioMu.Lock()
	defer audioMu.Unlock()

	fmt.Printf("[Audio Go] LoadFile: %q", filepath)

	cpath := C.CString(filepath)
	defer C.free(unsafe.Pointer(cpath))

	ret := C.audio_load(cpath)
	if ret != 0 {
		return fmt.Errorf("audio_load(%q) failed (returned %d)", filepath, int(ret))
	}
	return nil
}

// Play starts or resumes playback.
func Play() {
	audioMu.Lock()
	defer audioMu.Unlock()
	C.audio_play()
}

// Pause pauses playback. Current position is preserved.
func Pause() {
	audioMu.Lock()
	defer audioMu.Unlock()
	C.audio_pause()
}

// Stop stops playback and resets to the beginning.
func Stop() {
	audioMu.Lock()
	defer audioMu.Unlock()
	C.audio_stop()
}

// Seek sets the playback position to the given number of seconds from the start.
func Seek(seconds float64) {
	audioMu.Lock()
	defer audioMu.Unlock()
	C.audio_seek(C.float(seconds))
}

// SetVolume sets the playback volume (0-100).
// Values outside the range are clamped.
func SetVolume(vol int) {
	audioMu.Lock()
	defer audioMu.Unlock()

	// Convert 0-100 scale to 0.0-1.0 for the C layer
	fvol := float32(vol) / 100.0
	if fvol < 0 {
		fvol = 0
	}
	if fvol > 1.0 {
		fvol = 1.0
	}
	C.audio_set_volume(C.float(fvol))
}

// GetPosition returns the current playback position in seconds.
func GetPosition() float64 {
	audioMu.Lock()
	defer audioMu.Unlock()
	return float64(C.audio_get_position())
}

// GetDuration returns the total duration of the loaded audio in seconds.
func GetDuration() float64 {
	audioMu.Lock()
	defer audioMu.Unlock()
	return float64(C.audio_get_duration())
}

// IsPlaying returns true if audio is currently playing.
func IsPlaying() bool {
	audioMu.Lock()
	defer audioMu.Unlock()
	return bool(C.audio_is_playing())
}

// SetOnFinished registers a callback that will be invoked when the current
// audio playback finishes. Only one callback can be registered at a time;
// subsequent calls replace the previous callback.
func SetOnFinished(fn func()) {
	audioMu.Lock()
	defer audioMu.Unlock()
	audioFinishedFn = fn
	if fn != nil {
		// Call the C wrapper — it registers goAudioFinishedCallback
		// (the //export'd Go function) as the miniaudio callback.
		C.registerGoCallback()
	} else {
		C.clearGoCallback()
	}
}

// Update should be called periodically from the main loop to handle
// device changes. This must NOT be called from audio callbacks.
func Update() {
	audioMu.Lock()
	defer audioMu.Unlock()
	C.audio_update()
}

// Destroy cleans up all audio resources. After calling this, Init()
// must be called again before any other audio function.
func Destroy() {
	audioMu.Lock()
	defer audioMu.Unlock()
	audioFinishedFn = nil
	C.clearGoCallback()
	C.audio_destroy()
}
