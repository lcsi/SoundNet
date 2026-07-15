package codec

import (
	"fmt"
	"io"
	"log"
	"os"
)

// ============================================================================
// Audio Format Detection
//
// detectAudioFormat reads the file header (magic bytes) to determine the
// actual audio encoding format, regardless of the file extension.
//
// ----------------------------------------------------------------------------
// Supported formats (miniaudio can play natively):
//   MP3    ID3v2 tags ("ID3") or MPEG sync word (0xFFE)
//   FLAC   "fLaC"
//   OGG    "OggS"
//   WAV    "RIFF" + "WAVE"
//
// Decodable via our AAC → WAV pipeline (FAAD2 decoder + MP4 demux):
//   AAC    ADTS sync word (0xFFF)
//   M4A    "ftyp" in MP4 container (AAC in MP4)
//
// Unsupported:
//   WMA / APE / other compressed audio formats
// ============================================================================

// MiniaudioCompatible returns true if the given format extension is
// natively supported by miniaudio's built-in decoders (MP3, FLAC, OGG, WAV).
func MiniaudioCompatible(ext string) bool {
	switch ext {
	case ".mp3", ".flac", ".ogg", ".wav":
		return true
	}
	return false
}

// DetectAudioFormat opens the file at path, reads its header, and returns
// the correct file extension (including the dot, e.g. ".mp3").
//
// If the format cannot be determined from the header, it returns an empty
// string so the caller can keep the original extension.
func DetectAudioFormat(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "" // can't read → keep whatever we have
	}
	defer f.Close()

	// Read up to 12 bytes. That's enough for all common audio magic signatures.
	buf := make([]byte, 12)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.ErrUnexpectedEOF {
		return ""
	}
	if n < 4 {
		return "" // too short to determine anything
	}

	// ── Check known magic signatures ─────────────────────────────────

	// 1. ID3v2 tag → MP3
	if n >= 3 && buf[0] == 0x49 && buf[1] == 0x44 && buf[2] == 0x33 { // "ID3"
		return ".mp3"
	}

	// 2. FLAC: "fLaC"
	if n >= 4 && buf[0] == 0x66 && buf[1] == 0x4C && buf[2] == 0x61 && buf[3] == 0x43 {
		return ".flac"
	}

	// 3. OGG: "OggS"
	if n >= 4 && buf[0] == 0x4F && buf[1] == 0x67 && buf[2] == 0x67 && buf[3] == 0x53 {
		return ".ogg"
	}

	// 4. WAV / RIFF: "RIFF" + "WAVE" at offset 8
	if n >= 12 &&
		buf[0] == 0x52 && buf[1] == 0x49 && buf[2] == 0x46 && buf[3] == 0x46 && // "RIFF"
		buf[8] == 0x57 && buf[9] == 0x41 && buf[10] == 0x56 && buf[11] == 0x45 { // "WAVE"
		return ".wav"
	}

	// 5. MP4 / M4A container: "ftyp" (contains AAC or ALAC)
	// MP4/M4A: "ftyp" box type at byte offset 4 (after 4-byte box size)
	if n >= 8 && buf[4] == 0x66 && buf[5] == 0x74 && buf[6] == 0x79 && buf[7] == 0x70 {
		return ".m4a"
	}

	// 6. MPEG / ADTS sync word
	//     First byte 0xFF indicates the start of a sync word.
	if buf[0] == 0xFF {
		// Byte 1 top 4 bits == 0xF0 → 12-bit ADTS sync (AAC)
		if buf[1]&0xF0 == 0xF0 {
			// Check layer bits (byte 1 bits 2-1):
			//   layer=00 → AAC (ADTS)
			//   layer=01/10/11 → MPEG Layer I/II/III (MP3/MP2/MP1)
			if buf[1]&0x06 == 0x00 {
				return ".aac" // ADTS AAC
			}
			// layer bits != 00 → MPEG audio (MPEG1/2/2.5, Layer I/II/III)
			// This includes MP3 but also MP2/MP1.  All handled by miniaudio dr_mp3.
			return ".mp3"
		}
		// Byte 1 top 3 bits == 0xE0 → 11-bit MPEG sync word (MP3/MP2/MP1)
		if buf[1]&0xE0 == 0xE0 {
			return ".mp3"
		}
	}

	// Unknown format
	return ""
}

// DetectAndRename reads the magic bytes of the file at tmpPath, determines
// the correct extension, and renames the file accordingly.  Both the audio
// file and its companion .meta sidecar are renamed.
//
// Returns the final path with the correct extension.
// If detection fails, tmpPath is kept as-is and returned unchanged.
func DetectAndRename(tmpPath string) string {
	ext := DetectAudioFormat(tmpPath)
	if ext == "" {
		// Could not determine format — keep the .tmp extension.
		// The caller (and miniaudio) will try to decode it anyway.
		log.Printf("[Format] Could not detect audio format, keeping: %s", tmpPath)
		return tmpPath
	}

	base := tmpPath[:len(tmpPath)-4] // strip ".tmp"
	finalPath := base + ext

	// Only rename if the extension actually changes
	if finalPath == tmpPath {
		return finalPath
	}

	// Rename audio file
	if err := os.Rename(tmpPath, finalPath); err != nil {
		log.Printf("[Format] Failed to rename %s → %s: %v", tmpPath, finalPath, err)
		return tmpPath // keep original path
	}

	// Rename companion .meta file (best-effort)
	oldMeta := tmpPath + ".meta"
	newMeta := finalPath + ".meta"
	if err := os.Rename(oldMeta, newMeta); err != nil {
		// Not fatal — the meta will just be orphaned.
		// Next download for this song will start fresh (no cache hit).
	}

	log.Printf("[Format] Detected %s → %s", ext, finalPath)

	// Log for formats that need external decoding
	if !MiniaudioCompatible(ext) {
		log.Printf("[Format] %s format (%s) — will decode to WAV via AAC pipeline",
			ext, FormatName(ext))
	}

	return finalPath
}

// FormatName returns a human-readable name for the audio format.
func FormatName(ext string) string {
	switch ext {
	case ".mp3":
		return "MPEG Audio (MP3)"
	case ".flac":
		return "FLAC"
	case ".ogg":
		return "Ogg Vorbis"
	case ".wav":
		return "WAV"
	case ".aac":
		return "AAC (ADTS)"
	case ".m4a":
		return "AAC (MP4/M4A)"
	default:
		return fmt.Sprintf("Unknown (%s)", ext)
	}
}

// ReadableSize returns a human-readable file size string.
func ReadableSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
