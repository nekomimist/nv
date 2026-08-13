package imgdecode

import "encoding/binary"

// pngFastPathInfo is what the libdeflate-backed PNG fast path (native_linux.go,
// build-tagged linux && cgo && native_decode) needs to decode an eligible PNG:
// dimensions, the channel count implied by its color type, and the IDAT
// payload ready to hand to libdeflate's whole-buffer zlib decompressor.
type pngFastPathInfo struct {
	width, height int
	colorType     int    // 0 (gray), 2 (RGB), 4 (gray+alpha), or 6 (RGBA)
	channels      int    // 1, 3, 2, or 4, matching colorType above
	idat          []byte // concatenated IDAT payload; aliases data with no
	// copy when there was exactly one IDAT chunk
}

// parsePNGFastPath walks data's PNG chunk stream in pure Go -- no cgo, no
// decompression, just byte parsing -- and reports whether the libdeflate
// fast path can handle it. It accepts only the common case the fast path
// implements: 8-bit depth, color type 0/2/4/6, no interlacing, no tRNS
// chunk (colour-key transparency needs extra handling that types 4/6 can't
// even carry), and at least one IDAT chunk. Anything else -- including any
// malformed or truncated chunk stream -- reports ok == false so the caller
// falls back to the libpng path; this function never panics and never
// reads outside data's bounds.
func parsePNGFastPath(data []byte) (info pngFastPathInfo, ok bool) {
	if !isPNGData(data) {
		return pngFastPathInfo{}, false
	}

	_, typ, ihdr, next, ok := readPNGChunk(data, 8)
	if !ok || typ != pngChunkIHDR || len(ihdr) != 13 {
		return pngFastPathInfo{}, false
	}

	width := binary.BigEndian.Uint32(ihdr[0:4])
	height := binary.BigEndian.Uint32(ihdr[4:8])
	bitDepth := ihdr[8]
	colorType := ihdr[9]
	interlace := ihdr[12]

	channels, okColor := pngFastPathChannels(colorType)
	if bitDepth != 8 || !okColor || interlace != 0 {
		return pngFastPathInfo{}, false
	}
	if width == 0 || height == 0 {
		return pngFastPathInfo{}, false
	}

	off := next
	var idatParts [][]byte
	for {
		_, typ, chunkData, next, ok := readPNGChunk(data, off)
		if !ok {
			return pngFastPathInfo{}, false
		}
		switch typ {
		case pngChunkIDAT:
			idatParts = append(idatParts, chunkData)
		case pngChunkTRNS:
			return pngFastPathInfo{}, false
		case pngChunkIEND:
			idat, ok := concatIDAT(idatParts)
			if !ok {
				return pngFastPathInfo{}, false
			}
			return pngFastPathInfo{
				width:     int(width),
				height:    int(height),
				colorType: int(colorType),
				channels:  channels,
				idat:      idat,
			}, true
		}
		off = next
	}
}

// concatIDAT returns parts[0] directly with no copy when there is exactly
// one IDAT chunk, and only allocates and concatenates when there are
// several. It reports ok == false when the total payload is empty (no
// bytes for libdeflate to decompress), which can happen if every IDAT
// chunk in the stream was zero-length.
func concatIDAT(parts [][]byte) ([]byte, bool) {
	switch len(parts) {
	case 0:
		return nil, false
	case 1:
		if len(parts[0]) == 0 {
			return nil, false
		}
		return parts[0], true
	default:
		total := 0
		for _, p := range parts {
			total += len(p)
		}
		if total == 0 {
			return nil, false
		}
		idat := make([]byte, 0, total)
		for _, p := range parts {
			idat = append(idat, p...)
		}
		return idat, true
	}
}

// pngFastPathChannels maps a PNG color type to the fast path's bytes-per-
// pixel-at-8-bit channel count, reporting ok == false for any color type
// the fast path does not handle (3 == palette, or any reserved value).
func pngFastPathChannels(colorType byte) (channels int, ok bool) {
	switch colorType {
	case 0: // gray
		return 1, true
	case 2: // RGB
		return 3, true
	case 4: // gray+alpha
		return 2, true
	case 6: // RGBA
		return 4, true
	default:
		return 0, false
	}
}

// pngChunkType is a PNG chunk's 4-byte type name, kept as a fixed-size,
// directly comparable array rather than a string: readPNGChunk is called
// once per chunk, and some real PNGs split their image data across well
// over a hundred IDAT chunks (schrenshot.png, one of this package's own
// test fixtures, has 124), so converting the type field to a string on
// every call would be a per-chunk heap allocation for no benefit here --
// nothing needs a string, every use is a comparison against a known type.
type pngChunkType [4]byte

var (
	pngChunkIHDR = pngChunkType{'I', 'H', 'D', 'R'}
	pngChunkIDAT = pngChunkType{'I', 'D', 'A', 'T'}
	pngChunkIEND = pngChunkType{'I', 'E', 'N', 'D'}
	pngChunkTRNS = pngChunkType{'t', 'R', 'N', 'S'}
)

// readPNGChunk reads one PNG chunk starting at off (the offset of its
// 4-byte length field) within data. It returns the chunk's declared
// length, its 4-byte type, its data slice (referencing data directly, no
// copy), the offset of the following chunk, and ok == false for anything
// that would require reading outside data's bounds -- a truncated length
// field, a declared length longer than the remaining bytes, or a
// missing/truncated CRC. It never reads or returns a slice past len(data).
func readPNGChunk(data []byte, off int) (length uint32, typ pngChunkType, chunkData []byte, next int, ok bool) {
	if off < 0 || off+8 > len(data) {
		return 0, pngChunkType{}, nil, 0, false
	}
	length = binary.BigEndian.Uint32(data[off : off+4])
	typ = pngChunkType(data[off+4 : off+8])
	dataStart := off + 8
	if length > uint32(len(data)) {
		// Cannot possibly fit; also guarantees int(length) below cannot
		// overflow, since len(data) already fits in int.
		return 0, pngChunkType{}, nil, 0, false
	}
	dataEnd := dataStart + int(length)
	if dataEnd < dataStart || dataEnd+4 > len(data) {
		return 0, pngChunkType{}, nil, 0, false
	}
	return length, typ, data[dataStart:dataEnd], dataEnd + 4, true
}
