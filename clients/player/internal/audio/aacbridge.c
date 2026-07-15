/*
 * AAC Bridge — decode AAC to WAV using FAAD2.
 *
 * FAAD2 is a full-featured AAC decoder supporting AAC-LC, HE-AAC,
 * HE-AACv2, AAC-LD, AAC-ELD and more.
 *
 * It is cross-compiled for each target platform in the Docker build step.
 * Link with -lfaad -lm.
 */

#include "aacbridge.h"
#include <neaacdec.h>

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>

/* ==========================================================================
 * WAV writing helpers
 * ========================================================================== */

/* Write the WAV RIFF header (returns offset to data section for later patching). */
static long wav_start(FILE* f, int sample_rate, int channels)
{
    int bits_per_sample = 16;
    int bytes_per_sample = bits_per_sample / 8;
    int byte_rate = sample_rate * channels * bytes_per_sample;
    int block_align = channels * bytes_per_sample;

    uint32_t le32;
    uint16_t le16;

    fwrite("RIFF", 1, 4, f);
    le32 = 0x7FFFFFFF; fwrite(&le32, 4, 1, f);  /* placeholder */
    fwrite("WAVE", 1, 4, f);
    fwrite("fmt ", 1, 4, f);
    le32 = 16;                fwrite(&le32, 4, 1, f);
    le16 = 1;                 fwrite(&le16, 2, 1, f);
    le16 = (uint16_t)channels; fwrite(&le16, 2, 1, f);
    le32 = (uint32_t)sample_rate; fwrite(&le32, 4, 1, f);
    le32 = (uint32_t)byte_rate;   fwrite(&le32, 4, 1, f);
    le16 = (uint16_t)block_align; fwrite(&le16, 2, 1, f);
    le16 = (uint16_t)bits_per_sample; fwrite(&le16, 2, 1, f);
    fwrite("data", 1, 4, f);
    le32 = 0x7FFFFFFF; fwrite(&le32, 4, 1, f);

    return ftell(f);  /* return position right after data header — for patching */
}

/* Patch the RIFF and data chunk sizes. Call at the end once total PCM bytes are known. */
static void wav_patch(FILE* f, long data_start, long file_end)
{
    long data_bytes = file_end - data_start;
    long riff_size = data_bytes + 36;
    uint32_t le32;

    fseek(f, 4, SEEK_SET);
    le32 = (uint32_t)riff_size; fwrite(&le32, 4, 1, f);

    fseek(f, data_start - 4, SEEK_SET);  /* go to data chunk size */
    le32 = (uint32_t)data_bytes; fwrite(&le32, 4, 1, f);
}

/* ==========================================================================
 * ADTS AAC file → WAV
 * ========================================================================== */

int aac_decode_adts_to_wav(const char* aac_path, const char* wav_path,
                            int* out_sr, int* out_ch)
{
    int ret = -1;
    FILE* faac = NULL;
    FILE* fwav = NULL;
    NeAACDecHandle decoder = NULL;
    unsigned char* aac_buf = NULL;
    long aac_size = 0;
    int sample_rate = 0;
    int channels = 0;
    long data_start = 0;

    /* ── 1. Read entire ADTS file ───────────────────────────────────── */
    faac = fopen(aac_path, "rb");
    if (!faac) goto done;

    fseek(faac, 0, SEEK_END);
    aac_size = ftell(faac);
    fseek(faac, 0, SEEK_SET);
    if (aac_size <= 0) goto done;

    aac_buf = (unsigned char*)malloc(aac_size);
    if (!aac_buf) goto done;
    if ((long)fread(aac_buf, 1, aac_size, faac) != aac_size) goto done;
    fclose(faac);
    faac = NULL;

    /* ── 2. Open FAAD2 decoder ──────────────────────────────────────── */
    decoder = NeAACDecOpen();
    if (!decoder) goto done;

    /* Configure output format: 16-bit PCM, downmix 5.1 to stereo, SBR upsampling */
    NeAACDecConfigurationPtr cfg = NeAACDecGetCurrentConfiguration(decoder);
    if (cfg) {
        cfg->outputFormat   = FAAD_FMT_16BIT;
        cfg->downMatrix     = 1;
        cfg->dontUpSampleImplicitSBR = 0;
    }

    /* ── 3. Initialize with ADTS header ─────────────────────────────── */
    {
        unsigned long sr = 0;
        unsigned char ch = 0;
        char err = NeAACDecInit(decoder, aac_buf,
                                 (unsigned long)(aac_size > 8192 ? 8192 : aac_size),
                                 &sr, &ch);
        if (err < 0) goto done;
        sample_rate = (int)sr;
        channels    = (int)ch;
    }

    if (out_sr) *out_sr = sample_rate;
    if (out_ch) *out_ch = channels;

    /* ── 4. Open WAV output file ────────────────────────────────────── */
    fwav = fopen(wav_path, "wb");
    if (!fwav) goto done;
    data_start = wav_start(fwav, sample_rate, channels);

    /* ── 5. Decode frame by frame ───────────────────────────────────── */
    NeAACDecFrameInfo frame_info;
    long consumed = 0;
    long total_pcm_bytes = 0;

    while (consumed < (long)aac_size) {
        unsigned long remaining = (unsigned long)(aac_size - consumed);
        void* pcm = NeAACDecDecode(decoder, &frame_info,
                                    aac_buf + consumed, remaining);
        if (frame_info.error > 0) {
            /* Error — skip frame */
            consumed++;
            continue;
        }
        if (frame_info.bytesconsumed <= 0) {
            consumed++;
            continue;
        }
        consumed += (long)frame_info.bytesconsumed;

        if (pcm && frame_info.samples > 0) {
            int bytes = frame_info.samples * (int)sizeof(short);
            fwrite(pcm, 1, (size_t)bytes, fwav);
            total_pcm_bytes += bytes;
        }
    }

    /* ── 6. Patch WAV sizes ─────────────────────────────────────────── */
    long file_end = ftell(fwav);
    wav_patch(fwav, data_start, file_end);

    ret = 0;

done:
    if (faac) fclose(faac);
    if (fwav) { fflush(fwav); fclose(fwav); }
    if (decoder) NeAACDecClose(decoder);
    free(aac_buf);
    return ret;
}

/* ==========================================================================
 * Raw AAC frames (ADTS-prefixed) → WAV
 * Used for M4A demux → ADTS construction → decode
 * ========================================================================== */

int aac_decode_raw_to_wav(const unsigned char* adts_data, int adts_len,
                           const char* wav_path,
                           int* out_sr, int* out_ch)
{
    int ret = -1;
    FILE* fwav = NULL;
    NeAACDecHandle decoder = NULL;
    int sample_rate = 44100;
    int channels = 2;
    long data_start = 0;

    if (!adts_data || adts_len <= 0) goto done;

    /* ── 1. Open decoder ────────────────────────────────────────────── */
    decoder = NeAACDecOpen();
    if (!decoder) goto done;

    NeAACDecConfigurationPtr cfg = NeAACDecGetCurrentConfiguration(decoder);
    if (cfg) {
        cfg->outputFormat   = FAAD_FMT_16BIT;
        cfg->downMatrix     = 1;
        cfg->dontUpSampleImplicitSBR = 0;
    }

    /* ── 2. Initialize with ADTS header ─────────────────────────────── */
    {
        unsigned long sr = 0;
        unsigned char ch = 0;
        /* Read enough data to parse ADTS header */
        int init_len = adts_len > 8192 ? 8192 : adts_len;
        char err = NeAACDecInit(decoder,
                                 (unsigned char*)adts_data,
                                 (unsigned long)init_len,
                                 &sr, &ch);
        if (err < 0) goto done;
        sample_rate = (int)sr;
        channels    = (int)ch;
    }

    if (out_sr) *out_sr = sample_rate;
    if (out_ch) *out_ch = channels;

    /* ── 3. Open WAV ────────────────────────────────────────────────── */
    fwav = fopen(wav_path, "wb");
    if (!fwav) goto done;
    data_start = wav_start(fwav, sample_rate, channels);

    /* ── 4. Decode frames ───────────────────────────────────────────── */
    NeAACDecFrameInfo frame_info;
    int consumed = 0;
    long total_pcm_bytes = 0;

    while (consumed < adts_len) {
        unsigned long remaining = (unsigned long)(adts_len - consumed);
        void* pcm = NeAACDecDecode(decoder, &frame_info,
                                    (unsigned char*)adts_data + consumed,
                                    remaining);
        if (frame_info.error > 0) {
            consumed++;
            continue;
        }
        if (frame_info.bytesconsumed <= 0) {
            consumed++;
            continue;
        }
        consumed += (int)frame_info.bytesconsumed;

        if (pcm && frame_info.samples > 0) {
            int bytes = frame_info.samples * (int)sizeof(short);
            fwrite(pcm, 1, (size_t)bytes, fwav);
            total_pcm_bytes += bytes;
        }
    }

    /* ── 5. Patch WAV sizes ─────────────────────────────────────────── */
    long file_end = ftell(fwav);
    wav_patch(fwav, data_start, file_end);

    ret = 0;

done:
    if (fwav) { fflush(fwav); fclose(fwav); }
    if (decoder) NeAACDecClose(decoder);
    return ret;
}
