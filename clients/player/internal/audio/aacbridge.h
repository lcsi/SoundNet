/*
 * AAC Bridge — decode AAC (ADTS / M4A) files to WAV using FAAD2.
 *
 * FAAD2 (Free AAC Audio Decoder 2) is a well-established, open-source AAC
 * decoder that supports all common AAC profiles (LC, HE, HEv2, LD, ELD).
 *
 * The library is cross-compiled for each target platform in the Docker build step.
 * See build/docker/<platform>/Dockerfile.
 *
 * Exposed functions
 * ─────────────────
 *   aac_decode_adts_to_wav  — decode a complete ADTS (.aac) file to WAV
 *   aac_decode_raw_to_wav   — decode raw AAC frames to WAV (for M4A demux)
 */

#ifndef AAC_BRIDGE_H
#define AAC_BRIDGE_H

#ifdef __cplusplus
extern "C" {
#endif

/*
 * Decode an ADTS AAC file (.aac) to a WAV file (.wav).
 *
 *   aac_path   – input AAC file in ADTS format
 *   wav_path   – output WAV file to write (16-bit signed PCM)
 *   out_sr     – filled with sample rate (e.g. 44100)
 *   out_ch     – filled with channel count (e.g. 2)
 *
 * Returns 0 on success, -1 on failure.
 */
int aac_decode_adts_to_wav(const char* aac_path, const char* wav_path,
                            int* out_sr, int* out_ch);

/*
 * Decode raw AAC frames (with ADTS headers already attached) to WAV.
 * Used for M4A — the Go layer (mp4.go) demuxes the MP4 container and
 * constructs ADTS headers for each raw AAC frame, then calls this.
 *
 *   adts_data    – buffer of concatenated ADTS AAC frames
 *   adts_len     – length of adts_data in bytes
 *   wav_path     – output WAV file
 *   out_sr       – filled with sample rate
 *   out_ch       – filled with channel count
 *
 * Returns 0 on success, -1 on failure.
 */
int aac_decode_raw_to_wav(const unsigned char* adts_data, int adts_len,
                           const char* wav_path,
                           int* out_sr, int* out_ch);

#ifdef __cplusplus
}
#endif

#endif /* AAC_BRIDGE_H */
