package imgdecode

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// pngChunkSpec is one chunk to splice into a hand-built PNG byte stream for
// testing parsePNGFastPath's eligibility rules directly, without going
// through image/png (which cannot produce interlaced, tRNS-bearing, or
// malformed output).
type pngChunkSpec struct {
	typ  string
	data []byte
}

// buildTestPNG assembles a minimal PNG byte stream: signature, an IHDR
// built from the given fields, then extraChunks in order, each chunk
// written with a 4-byte zero CRC -- parsePNGFastPath never validates CRCs
// (a corrupted chunk that passes eligibility simply fails libdeflate's own
// decompression later, which is still a safe fallback), so tests don't need
// to compute real ones.
func buildTestPNG(width, height uint32, bitDepth, colorType, interlace byte, extraChunks []pngChunkSpec) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = bitDepth
	ihdr[9] = colorType
	ihdr[10] = 0 // compression method
	ihdr[11] = 0 // filter method
	ihdr[12] = interlace
	writeTestPNGChunk(&buf, "IHDR", ihdr)

	for _, c := range extraChunks {
		writeTestPNGChunk(&buf, c.typ, c.data)
	}
	writeTestPNGChunk(&buf, "IEND", nil)
	return buf.Bytes()
}

func writeTestPNGChunk(buf *bytes.Buffer, typ string, data []byte) {
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	buf.Write(lenBuf[:])
	buf.WriteString(typ)
	buf.Write(data)
	buf.Write([]byte{0, 0, 0, 0}) // CRC, unchecked by parsePNGFastPath
}

func fakeIDAT(n int) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(i)
	}
	return data
}

// TestParsePNGFastPathEligibleColorTypes covers every color type the fast
// path accepts and checks it reports the correct channel count for each.
func TestParsePNGFastPathEligibleColorTypes(t *testing.T) {
	cases := []struct {
		name         string
		colorType    byte
		wantChannels int
	}{
		{"gray", 0, 1},
		{"RGB", 2, 3},
		{"gray+alpha", 4, 2},
		{"RGBA", 6, 4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := buildTestPNG(4, 3, 8, tc.colorType, 0, []pngChunkSpec{
				{"IDAT", fakeIDAT(16)},
			})

			info, ok := parsePNGFastPath(data)
			if !ok {
				t.Fatalf("parsePNGFastPath: ok = false, want true")
			}
			if info.width != 4 || info.height != 3 {
				t.Errorf("width/height = %d/%d, want 4/3", info.width, info.height)
			}
			if info.colorType != int(tc.colorType) {
				t.Errorf("colorType = %d, want %d", info.colorType, tc.colorType)
			}
			if info.channels != tc.wantChannels {
				t.Errorf("channels = %d, want %d", info.channels, tc.wantChannels)
			}
			if len(info.idat) != 16 {
				t.Errorf("len(idat) = %d, want 16", len(info.idat))
			}
		})
	}
}

// TestParsePNGFastPathRejectsIneligible covers every reason the fast path
// must decline and fall back to libpng: palette, 16-bit depth, interlacing,
// a tRNS chunk, and no IDAT at all.
func TestParsePNGFastPathRejectsIneligible(t *testing.T) {
	cases := []struct {
		name  string
		build func() []byte
	}{
		{
			name: "palette color type",
			build: func() []byte {
				return buildTestPNG(4, 3, 8, 3, 0, []pngChunkSpec{
					{"PLTE", []byte{0, 0, 0, 255, 255, 255}},
					{"IDAT", fakeIDAT(16)},
				})
			},
		},
		{
			name: "16-bit depth",
			build: func() []byte {
				return buildTestPNG(4, 3, 16, 2, 0, []pngChunkSpec{
					{"IDAT", fakeIDAT(16)},
				})
			},
		},
		{
			name: "interlaced",
			build: func() []byte {
				return buildTestPNG(4, 3, 8, 6, 1, []pngChunkSpec{
					{"IDAT", fakeIDAT(16)},
				})
			},
		},
		{
			name: "tRNS present",
			build: func() []byte {
				return buildTestPNG(4, 3, 8, 2, 0, []pngChunkSpec{
					{"tRNS", []byte{0, 0, 0}},
					{"IDAT", fakeIDAT(16)},
				})
			},
		},
		{
			name: "no IDAT",
			build: func() []byte {
				return buildTestPNG(4, 3, 8, 6, 0, nil)
			},
		},
		{
			name: "not a PNG at all",
			build: func() []byte {
				return []byte("not a png")
			},
		},
		{
			name: "truncated chunk length overruns buffer",
			build: func() []byte {
				data := buildTestPNG(4, 3, 8, 6, 0, []pngChunkSpec{
					{"IDAT", fakeIDAT(16)},
				})
				// Corrupt the IDAT chunk's declared length (the 4 bytes
				// right after the IHDR chunk, which is signature(8) +
				// length(4) + type(4) + data(13) + crc(4) = 33 bytes long)
				// to a value far larger than the remaining bytes.
				idatLenOff := 8 + 4 + 4 + 13 + 4
				binary.BigEndian.PutUint32(data[idatLenOff:idatLenOff+4], 0xffffffff)
				return data
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := parsePNGFastPath(tc.build()); ok {
				t.Fatalf("parsePNGFastPath: ok = true, want false")
			}
		})
	}
}

// TestParsePNGFastPathSingleIDATNoCopy proves the single-IDAT path returns
// a slice aliasing the original data with no copy: mutating data after
// parsing must be visible through the returned idat slice.
func TestParsePNGFastPathSingleIDATNoCopy(t *testing.T) {
	data := buildTestPNG(4, 3, 8, 6, 0, []pngChunkSpec{
		{"IDAT", fakeIDAT(16)},
	})

	info, ok := parsePNGFastPath(data)
	if !ok {
		t.Fatalf("parsePNGFastPath: ok = false, want true")
	}
	if len(info.idat) != 16 {
		t.Fatalf("len(idat) = %d, want 16", len(info.idat))
	}

	// Locate the IDAT payload's offset in data the same way buildTestPNG
	// placed it: signature(8) + IHDR chunk(4+4+13+4=25) + IDAT
	// length/type header(8).
	idatDataOff := 8 + 25 + 8
	if &data[idatDataOff] != &info.idat[0] {
		t.Fatalf("info.idat does not alias data at the expected offset: single-IDAT path made an unexpected copy")
	}

	const sentinel = 0xAB
	data[idatDataOff] = sentinel
	if info.idat[0] != sentinel {
		t.Fatalf("info.idat[0] = %#x after mutating data, want %#x (no-copy aliasing broken)", info.idat[0], sentinel)
	}
}

// TestParsePNGFastPathMultipleIDATConcatenates proves several IDAT chunks
// are concatenated, in order, into one payload.
func TestParsePNGFastPathMultipleIDATConcatenates(t *testing.T) {
	part1 := fakeIDAT(5)
	part2 := []byte{0xAA, 0xBB, 0xCC}
	part3 := fakeIDAT(4)

	data := buildTestPNG(4, 3, 8, 6, 0, []pngChunkSpec{
		{"IDAT", part1},
		{"IDAT", part2},
		{"IDAT", part3},
	})

	info, ok := parsePNGFastPath(data)
	if !ok {
		t.Fatalf("parsePNGFastPath: ok = false, want true")
	}

	want := append(append(append([]byte{}, part1...), part2...), part3...)
	if !bytes.Equal(info.idat, want) {
		t.Fatalf("idat = %v, want %v", info.idat, want)
	}
}

// TestParsePNGFastPathEmptyIDATRejected proves a stream whose only IDAT
// chunk(s) carry zero bytes is rejected rather than handed to libdeflate
// with an empty input buffer.
func TestParsePNGFastPathEmptyIDATRejected(t *testing.T) {
	data := buildTestPNG(4, 3, 8, 6, 0, []pngChunkSpec{
		{"IDAT", nil},
	})
	if _, ok := parsePNGFastPath(data); ok {
		t.Fatalf("parsePNGFastPath: ok = true, want false for an empty IDAT payload")
	}
}
