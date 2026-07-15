package codec

/*
#cgo LDFLAGS: -lfaad -lm
#cgo CFLAGS: -I../audio
#include <stdlib.h>
#include "aacbridge.h"
*/
import "C"
import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

// ============================================================================
// AAC / M4A → WAV Decode Orchestrator
//
// Flow:
//   .aac  → aac_decode_adts_to_wav() → .wav  (FAAD2 handles ADTS frames)
//   .m4a  → mp4 demux → construct ADTS .aac → decode to .wav → clean temp
// ============================================================================

// DecodeToWAV converts an AAC or M4A file to WAV format and returns the WAV path.
func DecodeToWAV(audioPath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(audioPath))
	wavPath := audioPath[:len(audioPath)-len(ext)] + ".wav"

	switch ext {
	case ".aac":
		return decodeAACToWAV(audioPath, wavPath)
	case ".m4a":
		return decodeM4AToWAV(audioPath, wavPath)
	default:
		return "", fmt.Errorf("DecodeToWAV: unsupported format %q", ext)
	}
}

// --------------------------------------------------------------------------
// ADTS AAC (.aac) → WAV
// --------------------------------------------------------------------------

// decodeAACToWAV decodes an ADTS AAC file to WAV using FAAD2.
func decodeAACToWAV(aacPath, wavPath string) (string, error) {
	caac := C.CString(aacPath)
	cwav := C.CString(wavPath)
	defer C.free(unsafe.Pointer(caac))
	defer C.free(unsafe.Pointer(cwav))

	var sr, ch C.int
	ret := C.aac_decode_adts_to_wav(caac, cwav, &sr, &ch)
	if ret != 0 {
		return "", fmt.Errorf("aac_decode_adts_to_wav returned %d for %s", int(ret), aacPath)
	}

	log.Printf("[Decode] AAC → WAV: %s → %s (%d Hz, %d ch)", aacPath, wavPath, int(sr), int(ch))
	return wavPath, nil
}

// --------------------------------------------------------------------------
// M4A (.m4a) → WAV   (MP4 demux + ADTS construction + FAAD2 decode)
// --------------------------------------------------------------------------

// decodeM4AToWAV demuxes an M4A file, constructs ADTS headers for each raw
// AAC frame, writes a temporary .aac file, and decodes it to WAV.
func decodeM4AToWAV(m4aPath, wavPath string) (string, error) {
	log.Printf("[Decode] M4A → demux → AAC → WAV: %s", m4aPath)

	// 1. Parse MP4 and build ADTS buffer
	adtsData, _, _, _, err := BuildADTSBufferFromM4A(m4aPath)
	if err != nil {
		return "", fmt.Errorf("m4a demux: %w", err)
	}

	// 2. Write temporary ADTS file (alongside the M4A in cache)
	tmpAAC := m4aPath + ".aac"
	if err := os.WriteFile(tmpAAC, adtsData, 0644); err != nil {
		return "", fmt.Errorf("write temp .aac: %w", err)
	}

	// 3. Decode ADTS → WAV
	wavPath, err = decodeAACToWAV(tmpAAC, wavPath)
	if err != nil {
		os.Remove(tmpAAC)
		os.Remove(tmpAAC + ".meta")
		return "", fmt.Errorf("decode temp .aac: %w", err)
	}

	// 4. Clean up temp AAC file
	os.Remove(tmpAAC)
	os.Remove(tmpAAC + ".meta")

	return wavPath, nil
}
