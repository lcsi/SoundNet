package codec

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// ============================================================================
// M4A / MP4 AAC Demuxer — Pure Go
//
// Extracts raw AAC frames and AudioSpecificConfig (ASC) from M4A files.
// The raw frames are then wrapped with ADTS headers and decoded by FAAD2.
//
// Only handles the common "AAC-in-MP4" case (AAC-LC, single audio track).
// Does NOT handle: fragmented movies (moof), compressed moov (cmov),
// multiple audio tracks, or encrypted content.
// ============================================================================

// MP4DemuxResult holds the parsed data from an M4A file.
type MP4DemuxResult struct {
	ASC      []byte // AudioSpecificConfig bytes (for FAAD2 init)
	Frames   []byte // Concatenated raw AAC frame data (no ADTS headers)
	Offsets  []int64 // Byte offset of each frame within Frames[]
	Sizes    []int   // Size of each frame in bytes
	FullData []byte // The entire M4A file bytes (for absolute offset lookups)
	// Audio parameters decoded from ASC
	SampleRate    int
	Channels      int
	AOT           int // Audio Object Type (2 = AAC-LC)
	SampleRateIdx int // Sample rate index in standard table
	ChannelCfg    int // Channel configuration
}

// ParseM4A parses an M4A file and returns raw AAC frames + AudioSpecificConfig.
func ParseM4A(path string) (*MP4DemuxResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	// Read entire file into memory for easier parsing
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	data := make([]byte, info.Size())
	if _, err := io.ReadFull(f, data); err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	result := &MP4DemuxResult{
		FullData: data,
	}

	if err := parseBox(data, result); err != nil {
		return nil, fmt.Errorf("parse mp4: %w", err)
	}

	if len(result.Frames) == 0 {
		return nil, fmt.Errorf("no AAC frames found in M4A file")
	}

	return result, nil
}

// readBoxHeader reads the 8-byte box header (size + type).
// Returns size, type string, and remaining payload bytes.
func readBoxHeader(data []byte) (size int, typ string, payload []byte, ok bool) {
	if len(data) < 8 {
		return 0, "", nil, false
	}
	size = int(binary.BigEndian.Uint32(data[0:4]))
	typ = string(data[4:8])

	if size == 0 {
		// Box extends to end of data
		size = len(data)
	} else if size == 1 {
		// 64-bit extended size
		if len(data) < 16 {
			return 0, "", nil, false
		}
		size = int(binary.BigEndian.Uint64(data[8:16]))
		payload = data[16:]
		return size, typ, payload, true
	}

	if size < 8 || size > len(data) {
		return 0, "", nil, false
	}
	payload = data[8:size]
	return size, typ, payload, true
}

// parseBox recursively parses boxes, searching for audio track info and mdat.
func parseBox(data []byte, result *MP4DemuxResult) error {
	pos := 0
	for pos < len(data) {
		// Need at least 8 bytes for header
		if pos+8 > len(data) {
			break
		}

		size, typ, payload, ok := readBoxHeader(data[pos:])
		if !ok {
			break
		}
		nextPos := pos + size
		if nextPos > len(data) {
			break
		}

		switch typ {
		case "ftyp":
			// Verify it's a compatible file type
			// No action needed; we just decode whatever we find

		case "moov":
			if err := parseBox(payload, result); err != nil {
				return err
			}

		case "trak":
			if err := parseTrak(payload, result); err != nil {
				return err
			}

		case "mdat":
			// The mdat contains raw media data.
			// We'll reference the frame offsets later after parsing stbl.
			result.Offsets = append(result.Offsets, int64(nextPos-len(payload))) // not used directly
			_ = payload // raw media data — we access via result later

		case "moof":
			// Fragmented movie — not supported
			return fmt.Errorf("fragmented MP4 (moof) not supported")

		case "cmov":
			// Compressed moov — not supported
			return fmt.Errorf("compressed moov (cmov) not supported")
		}

		pos = nextPos
		if size <= 0 {
			break
		}
	}
	return nil
}

// parseTrak processes a track box looking for the audio (soun) track.
func parseTrak(data []byte, result *MP4DemuxResult) error {
	var tkhdID int64
	var handlerType string
	var stblData []byte

	pos := 0
	for pos < len(data) {
		if pos+8 > len(data) {
			break
		}
		size, typ, payload, ok := readBoxHeader(data[pos:])
		if !ok {
			break
		}
		nextPos := pos + size
		if nextPos > len(data) {
			break
		}

		switch typ {
		case "tkhd":
			// Track header — get track ID (used to find which track in mdat)
			if len(payload) >= 20 {
				version := payload[0]
				if version == 0 {
					tkhdID = int64(binary.BigEndian.Uint32(payload[12:16]))
				} else {
					tkhdID = int64(binary.BigEndian.Uint64(payload[20:28]))
				}
			}

		case "mdia":
			// Media box — look for hdlr
			hdlrType, mdiaStbl, err := parseMdia(payload)
			if err != nil {
				return err
			}
			handlerType = hdlrType
			stblData = mdiaStbl

		case "udta":
			// User data — skip

		case "edts":
			// Edit list — skip
		}

		_ = tkhdID // could use for matching, but we just take the first audio track
		pos = nextPos
	}

	// If this is the audio track (soun), process its sample table
	if handlerType == "soun" && len(stblData) > 0 {
		if err := parseAudioStbl(stblData, result.FullData, result); err != nil {
			return err
		}
	}

	return nil
}

// parseMdia processes an mdia box to find handler type and stbl data.
func parseMdia(data []byte) (handlerType string, stblData []byte, err error) {
	pos := 0
	for pos < len(data) {
		if pos+8 > len(data) {
			break
		}
		size, typ, payload, ok := readBoxHeader(data[pos:])
		if !ok {
			break
		}
		nextPos := pos + size
		if nextPos > len(data) {
			break
		}

		switch typ {
		case "hdlr":
			// Handler reference box — get subtype ('soun', 'vide', etc.)
			if len(payload) >= 12 {
				hdr := payload[8:12]
				handlerType = string(hdr)
			}

		case "minf":
			// Media info box — look for stbl inside
			if err := findStbl(payload, &stblData); err != nil {
				return "", nil, err
			}
		}
		pos = nextPos
	}
	return handlerType, stblData, nil
}

// findStbl recursively descends into boxes to find the stbl (Sample Table) box.
func findStbl(data []byte, stblData *[]byte) error {
	pos := 0
	for pos < len(data) {
		if pos+8 > len(data) {
			break
		}
		size, typ, payload, ok := readBoxHeader(data[pos:])
		if !ok {
			break
		}
		nextPos := pos + size
		if nextPos > len(data) {
			break
		}

		if typ == "stbl" {
			*stblData = payload
			return nil
		}
		// Recurse into container boxes
		switch typ {
		case "dinf", "gmhd", "smhd", "vmhd", "nmhd":
			// Skip leaves and continue looking
		default:
			if err := findStbl(payload, stblData); err == nil {
				return nil
			}
		}
		pos = nextPos
	}
	return nil
}

// parseAudioStbl parses the Sample Table box for audio.
// It extracts the AudioSpecificConfig (from esds) and builds the frame list
// using stsz (sizes), stsc (sample-to-chunk), and stco (chunk offsets).
func parseAudioStbl(stblData []byte, fullFileData []byte, result *MP4DemuxResult) error {
	var stszSizes []uint32
	var stcoOffsets []uint64
	var stscEntries []struct {
		firstChunk       uint32
		samplesPerChunk  uint32
	}
	var isCo64 bool
	ascFound := false

	pos := 0
	for pos < len(stblData) {
		if pos+8 > len(stblData) {
			break
		}
		size, typ, payload, ok := readBoxHeader(stblData[pos:])
		if !ok {
			break
		}
		nextPos := pos + size
		if nextPos > len(stblData) {
			break
		}

		switch typ {
		case "stsd":
			// Sample Description — FullBox: [ver(1)][flags(3)][entry_count(4)][SampleEntry[]...]
			if len(payload) < 8 {
				break
			}
			// payload[0:4] = version+flags (skip)
			// entry_count at payload[4:8], SampleEntry[] starts at payload[8:]
			if asc, err := parseSTSDForASC(payload[8:]); err == nil && len(asc) > 0 {
				result.ASC = asc
				ascFound = true
				decodeASC(asc, result)
			}

		case "stsz":
			// Sample Size Box — FullBox: [ver(1)][flags(3)][sample_size(4)][sample_count(4)][entry_size[]...]
			if len(payload) < 12 {
				break
			}
			// payload[0:4] = version+flags (skip)
			sampleSize := binary.BigEndian.Uint32(payload[4:8])
			sampleCount := binary.BigEndian.Uint32(payload[8:12])
			if sampleSize > 0 {
				stszSizes = make([]uint32, sampleCount)
				for i := range stszSizes {
					stszSizes[i] = sampleSize
				}
			} else {
				stszSizes = make([]uint32, sampleCount)
				entryOffset := 12
				for i := range stszSizes {
					if entryOffset+4 > len(payload) {
						break
					}
					stszSizes[i] = binary.BigEndian.Uint32(payload[entryOffset:])
					entryOffset += 4
				}
			}

		case "stco":
			// Chunk Offset (32-bit) — FullBox: [ver(1)][flags(3)][entry_count(4)][offset[]...]
			if len(payload) < 8 {
				break
			}
			// payload[0:4] = version+flags (skip)
			entryCount := binary.BigEndian.Uint32(payload[4:8])
			stcoOffsets = make([]uint64, entryCount)
			entryOffset := 8
			for i := range stcoOffsets {
				if entryOffset+4 > len(payload) {
					break
				}
				stcoOffsets[i] = uint64(binary.BigEndian.Uint32(payload[entryOffset:]))
				entryOffset += 4
			}

		case "co64":
			// Chunk Offset (64-bit) — FullBox: [ver(1)][flags(3)][entry_count(4)][offset[]...]
			if len(payload) < 8 {
				break
			}
			// payload[0:4] = version+flags (skip)
			entryCount := binary.BigEndian.Uint32(payload[4:8])
			stcoOffsets = make([]uint64, entryCount)
			entryOffset := 8
			for i := range stcoOffsets {
				if entryOffset+8 > len(payload) {
					break
				}
				stcoOffsets[i] = binary.BigEndian.Uint64(payload[entryOffset:])
				entryOffset += 8
			}
			isCo64 = true

		case "stsc":
			// Sample-to-Chunk — FullBox: [ver(1)][flags(3)][entry_count(4)][entries(12 each)...]
			if len(payload) < 8 {
				break
			}
			// payload[0:4] = version+flags (skip)
			entryCount := binary.BigEndian.Uint32(payload[4:8])
			stscEntries = make([]struct {
				firstChunk       uint32
				samplesPerChunk  uint32
			}, entryCount)
			entryOffset := 8
			for i := range stscEntries {
				if entryOffset+12 > len(payload) {
					break
				}
				stscEntries[i].firstChunk = binary.BigEndian.Uint32(payload[entryOffset:])
				stscEntries[i].samplesPerChunk = binary.BigEndian.Uint32(payload[entryOffset+4:])
				// Skip sample_description_index (4 bytes)
				entryOffset += 12
			}

		case "stts":
			// Time-to-sample — not needed for frame extraction
		case "stss":
			// Sync sample — not needed for AAC
		case "stsz2", "stco2":
			// Compact tables — not supported
			return fmt.Errorf("compact sample tables not supported")
		}

		_ = isCo64 // needed for sanity
		pos = nextPos
	}

	if !ascFound || len(stszSizes) == 0 || len(stcoOffsets) == 0 || len(stscEntries) == 0 {
		return fmt.Errorf("incomplete stbl: asc=%v, frames=%d, chunks=%d, stsc=%d",
			ascFound, len(stszSizes), len(stcoOffsets), len(stscEntries))
	}

	// ── Build frame list from stsc + stco + stsz ──────────────────────
	// For each frame, compute its absolute file offset and size.
	type frameInfo struct {
		offset int64
		size   int
	}
	frameInfos := make([]frameInfo, 0, len(stszSizes))

	// Pre-compute chunk start sample indices using stsc
	// stscEntries[i]: chunk firstChunk has samplesPerChunk samples
	// Between stscEntries[i].firstChunk and stscEntries[i+1].firstChunk-1,
	// all chunks have samplesPerChunk samples.
	chunkFirstSample := make([]uint32, len(stcoOffsets))
	var sampleIdx uint32
	entryIdx := 0
	for chunkIdx := uint32(0); chunkIdx < uint32(len(stcoOffsets)); chunkIdx++ {
		chunkFirstSample[chunkIdx] = sampleIdx

		// Determine samplesPerChunk for this chunk
		var samplesInThisChunk uint32
		if entryIdx < len(stscEntries)-1 && chunkIdx+1 >= stscEntries[entryIdx+1].firstChunk {
			entryIdx++
		}
		samplesInThisChunk = stscEntries[entryIdx].samplesPerChunk

		sampleIdx += samplesInThisChunk
		if sampleIdx > uint32(len(stszSizes)) {
			sampleIdx = uint32(len(stszSizes))
		}
	}

	// Build frame list
	for i := 0; i < len(stszSizes); i++ {
		// Find which chunk this sample belongs to
		var chunkIdx int
		for ci := len(chunkFirstSample) - 1; ci >= 0; ci-- {
			if chunkFirstSample[ci] <= uint32(i) {
				chunkIdx = ci
				break
			}
		}

		chunkOffset := int64(stcoOffsets[chunkIdx])
		if chunkOffset <= 0 || chunkOffset >= int64(len(fullFileData)) {
			continue
		}

		// Compute offset within chunk
		firstSampleInChunk := int(chunkFirstSample[chunkIdx])
		internalOffset := 0
		for j := firstSampleInChunk; j < i; j++ {
			internalOffset += int(stszSizes[j])
		}

		frameInfos = append(frameInfos, frameInfo{
			offset: chunkOffset + int64(internalOffset),
			size:   int(stszSizes[i]),
		})
	}

	// ── Read raw frame data ──────────────────────────────────────────
	result.Sizes = make([]int, len(frameInfos))
	var totalSize int
	for i, fi := range frameInfos {
		result.Sizes[i] = fi.size
		totalSize += fi.size
	}

	result.Frames = make([]byte, totalSize)
	offset := 0
	for _, fi := range frameInfos {
		if fi.offset+int64(fi.size) > int64(len(fullFileData)) {
			continue
		}
		copy(result.Frames[offset:], fullFileData[fi.offset:fi.offset+int64(fi.size)])
		offset += fi.size
	}
	// Trim to actual bytes written
	result.Frames = result.Frames[:offset]

	return nil
}

// parseSTSDForASC looks through stsd SampleEntry boxes for mp4a → esds →
// AudioSpecificConfig. The stsdPayload should already skip the FullBox
// header (version+flags) and entry_count — it starts at the first
// SampleEntry (mp4a/enca box).
func parseSTSDForASC(stsdPayload []byte) ([]byte, error) {
	pos := 0
	for pos < len(stsdPayload) {
		if pos+8 > len(stsdPayload) {
			break
		}
		size, typ, payload, ok := readBoxHeader(stsdPayload[pos:])
		if !ok {
			break
		}
		if typ == "mp4a" || typ == "enca" {
			// Inside mp4a, look for esds box
			return findESDS(payload)
		}
		pos += size
		if size <= 0 {
			break
		}
	}
	return nil, fmt.Errorf("no AudioSpecificConfig found")
}

// findESDS looks for the esds (Elementary Stream Descriptor) box and
// extracts the AudioSpecificConfig from the DecoderSpecificInfo descriptor.
//
// data is the mp4a box payload, which starts with SampleEntry fixed fields
// (6 reserved + 2 data_reference_index = 8 bytes) followed by
// AudioSampleEntry fixed fields (20 bytes for version 0). Only after these
// 28 bytes do optional boxes like "esds" appear.
func findESDS(data []byte) ([]byte, error) {
	// Skip the fixed SampleEntry + AudioSampleEntry fields to reach
	// the optional boxes (esds, etc.) at the end.
	//   SampleEntry:       6 reserved + 2 data_reference_index = 8 bytes
	//   AudioSampleEntry:  2 entry_version + 2+4 reserved + 2 ch+2 sz+2 pd+2 rs + 4 sr = 20 bytes
	// Read entry_version to determine any additional fields
	pos := 28 // default for version 0
	if len(data) >= 28 {
		_ = data[27] // bound check
		entryVersion := int(binary.BigEndian.Uint16(data[8:10]))
		if entryVersion == 1 {
			pos = 44 // version 1 has 16 more bytes
		} else if entryVersion == 2 {
			pos = 64 // version 2 has 36 more bytes
		}
	}

	for pos < len(data) {
		if pos+8 > len(data) {
			break
		}
		size, typ, payload, ok := readBoxHeader(data[pos:])
		if !ok {
			break
		}
		if typ == "esds" {
			return parseESDS(payload)
		}
		pos += size
		if size <= 0 {
			break
		}
	}
	return nil, fmt.Errorf("no esds box found")
}

// parseESDS extracts AudioSpecificConfig from the ESDS box payload.
// esds is a FullBox: [version(1)][flags(3)][descriptors...], so we must
// skip the first 4 bytes before reading descriptors.
//
// Descriptor format:
//
//	tag(1) = 0x03, length(1-4 bytes), ES_ID(2), flags(1)
//	DecoderConfigDescriptor:
//	  tag(1) = 0x04, length(1-4 bytes), objectType(1), streamType(1),
//	  bufferSizeDB(3), maxBitrate(4), avgBitrate(4)
//	DecoderSpecificInfo:
//	  tag(1) = 0x05, length(1-4 bytes), AudioSpecificConfig(...)
func parseESDS(payload []byte) ([]byte, error) {
	// esds is a FullBox: skip version(1) + flags(3) = 4 bytes
	if len(payload) < 5 {
		return nil, fmt.Errorf("esds payload too short")
	}

	// Read descriptor using MP4 descriptor format:
	// tag(1 byte) | length(1-4 bytes, dynamically encoded) | data
	readDescriptor := func(data []byte, start int) (tag int, content []byte, next int, err error) {
		if start >= len(data) {
			return 0, nil, start, fmt.Errorf("descriptor: out of bounds")
		}
		tag = int(data[start])
		start++

		// Read length (variable bytes: each byte, top bit=1 means more bytes follow)
		length := 0
		for start < len(data) {
			b := data[start]
			length = (length << 7) | int(b&0x7F)
			start++
			if b&0x80 == 0 {
				break
			}
		}
		if start+length > len(data) {
			return tag, nil, start, fmt.Errorf("descriptor truncated")
		}
		return tag, data[start : start+length], start + length, nil
	}

	// Start parsing descriptors after the FullBox header
	descData := payload[4:]
	pos := 0

	// ES_Descriptor (tag 0x03)
	tag, content, next, err := readDescriptor(descData, pos)
	if err != nil || tag != 0x03 {
		return nil, fmt.Errorf("expected ES_Descriptor (0x03), got 0x%02x", tag)
	}
	pos = next
	_ = content // content = ES_Descriptor body (we skip ES_ID + flags)

	// Inside ES_Descriptor, find DecoderConfigDescriptor (tag 0x04)
	tag, dcdContent, _, err := readDescriptor(content, 3) // skip ES_ID(2) + flags(1)
	if err != nil || tag != 0x04 {
		// Try at start of content
		tag, dcdContent, _, err = readDescriptor(content, 0)
		if err != nil || tag != 0x04 {
			return nil, fmt.Errorf("expected DecoderConfigDescriptor (0x04), got 0x%02x", tag)
		}
	}

	// Skip objectType(1) + streamType(1) + bufferSizeDB(3) + maxBitrate(4) + avgBitrate(4)
	// = 13 bytes
	dcdBody := dcdContent
	dcdPos := 13
	if dcdPos >= len(dcdBody) {
		return nil, fmt.Errorf("DecoderConfigDescriptor too short")
	}

	// DecoderSpecificInfo (tag 0x05)
	tag, asc, _, err := readDescriptor(dcdBody, dcdPos)
	if err != nil || tag != 0x05 {
		return nil, fmt.Errorf("expected DecoderSpecificInfo (0x05), got 0x%02x", tag)
	}

	if len(asc) < 2 {
		return nil, fmt.Errorf("AudioSpecificConfig too short (%d bytes)", len(asc))
	}

	return asc, nil
}

// decodeASC parses the AudioSpecificConfig bytes and fills result fields.
func decodeASC(asc []byte, result *MP4DemuxResult) {
	if len(asc) < 2 {
		return
	}

	// AudioSpecificConfig bit layout (ISO 14496-3):
	// byte 0: audioObjectType (5 bits) | samplingFrequencyIndex_high (3 bits)
	// byte 1: samplingFrequencyIndex_low (1 bit) | channelConfiguration (3 bits) | padding (2 bits)
	result.AOT = int(asc[0] >> 3)
	result.SampleRateIdx = (int(asc[0]&0x07) << 1) | int(asc[1]>>7)
	result.ChannelCfg = int((asc[1] >> 4) & 0x07)

	// Look up sample rate from index
	srTable := []int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}
	if result.SampleRateIdx >= 0 && result.SampleRateIdx < len(srTable) {
		result.SampleRate = srTable[result.SampleRateIdx]
	} else {
		result.SampleRate = 44100 // fallback
	}
	result.Channels = result.ChannelCfg
	if result.Channels < 1 {
		result.Channels = 2 // fallback
	}
}

// BuildADTSBufferFromM4A parses the M4A file and builds a continuous buffer
// of ADTS-prefixed AAC frames suitable for aac_decode_adts_to_wav.
//
// Returns the synthetic ADTS data, sample rate, channels, and AudioSpecificConfig.
func BuildADTSBufferFromM4A(m4aPath string) (adtsData []byte, sampleRate int, channels int, asc []byte, err error) {
	result, err := ParseM4A(m4aPath)
	if err != nil {
		return nil, 0, 0, nil, err
	}

	// Build ADTS header for each frame
	profile := result.AOT - 1
	sfi := result.SampleRateIdx
	channelCfg := result.ChannelCfg

	// Estimate total buffer size
	totalSize := 0
	for _, s := range result.Sizes {
		totalSize += 7 + s // ADTS header (7) + raw frame
	}

	buf := make([]byte, 0, totalSize)
	offset := 0
	for _, size := range result.Sizes {
		frameLength := 7 + size // 7-byte ADTS header + raw frame
		adtsHdr := make([]byte, 7)
		adtsHdr[0] = 0xFF
		adtsHdr[1] = 0xF1
		adtsHdr[2] = byte((profile << 6) | (sfi << 2) | ((channelCfg >> 2) & 0x01))
		adtsHdr[3] = byte(((channelCfg & 0x03) << 6) | ((frameLength >> 11) & 0x1F))
		adtsHdr[4] = byte((frameLength >> 3) & 0xFF)
		adtsHdr[5] = byte(((frameLength & 0x07) << 5) | 0x1F)
		adtsHdr[6] = 0xFC

		buf = append(buf, adtsHdr...)
		buf = append(buf, result.Frames[offset:offset+size]...)
		offset += size
	}

	return buf, result.SampleRate, result.Channels, result.ASC, nil
}
