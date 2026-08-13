//go:build cgo && native_decode

package imgdecode

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file is the strongest correctness check for the libdeflate PNG fast
// path (native_linux.go's nv_png_fastpath_decode): differential testing
// against the untouched libpng path (decodeNativePNGLibpng), asserting the
// two decoders produce byte-identical *image.RGBA output for the same PNG
// bytes. Coverage includes every eligible color type, edge-case
// dimensions, and -- since Go's image/png encoder chooses filters
// adaptively and cannot produce color type 4 (gray+alpha) at all -- a
// hand-built PNG stream (buildRawPNG/pngEncodeFilterRow below) that
// deliberately cycles through all five PNG filter types.

// assertFastPathMatchesLibpng requires data to be fast-path eligible,
// decodes it through both decodeNativePNGFastPath and decodeNativePNGLibpng
// directly (bypassing decodeNativePNG's own fast-path-first dispatch, so
// both paths always run regardless of which one a real caller would get),
// and fails unless every byte of the two *image.RGBA results is identical.
func assertFastPathMatchesLibpng(t *testing.T, label string, data []byte) {
	t.Helper()

	if _, elig := parsePNGFastPath(data); !elig {
		t.Fatalf("%s: parsePNGFastPath reports ineligible; this fixture is meant to exercise the fast path", label)
	}

	fastImg, fastInfo, ok := decodeNativePNGFastPath(data)
	if !ok {
		t.Fatalf("%s: decodeNativePNGFastPath declined an eligible PNG", label)
	}
	fastRGBA, ok := fastImg.(*image.RGBA)
	if !ok {
		t.Fatalf("%s: fast path returned %T, want *image.RGBA", label, fastImg)
	}

	libImg, libInfo, err := decodeNativePNGLibpng(data)
	if err != nil {
		t.Fatalf("%s: decodeNativePNGLibpng failed: %v", label, err)
	}
	libRGBA, ok := libImg.(*image.RGBA)
	if !ok {
		t.Fatalf("%s: libpng path returned %T, want *image.RGBA", label, libImg)
	}

	if fastInfo != libInfo {
		t.Fatalf("%s: Info = %+v, want %+v (libpng)", label, fastInfo, libInfo)
	}

	diffRGBA(t, label, libRGBA, fastRGBA)
}

// diffRGBA fails t with a specific, localized message: the first differing
// pixel (by libpng-vs-fast-path color) rather than just "bytes differ".
func diffRGBA(t *testing.T, label string, want, got *image.RGBA) {
	t.Helper()
	if want.Bounds() != got.Bounds() {
		t.Fatalf("%s: bounds = %v, want %v", label, got.Bounds(), want.Bounds())
	}
	if want.Stride != got.Stride {
		t.Fatalf("%s: stride = %d, want %d", label, got.Stride, want.Stride)
	}
	if bytes.Equal(want.Pix, got.Pix) {
		return
	}
	b := want.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			wp := want.RGBAAt(x, y)
			gp := got.RGBAAt(x, y)
			if wp != gp {
				t.Fatalf("%s: pixel (%d,%d) = %+v (fast path), want %+v (libpng)", label, x, y, gp, wp)
			}
		}
	}
	t.Fatalf("%s: Pix bytes differ but no differing pixel found", label)
}

// TestFastPathMatchesLibpngStdlibEncoded covers the three eligible color
// types Go's own image/png encoder can produce (gray, opaque RGB, and RGBA
// with alpha values that are neither 0 nor 255), across a range of sizes
// including 1x1, 1-pixel-wide, 1-pixel-tall, and widths that don't evenly
// divide the pixel size, to catch row/edge arithmetic mistakes.
func TestFastPathMatchesLibpngStdlibEncoded(t *testing.T) {
	sizes := []struct{ w, h int }{
		{1, 1}, {1, 9}, {9, 1}, {7, 5}, {13, 11}, {64, 48},
	}

	for _, sz := range sizes {
		sz := sz
		t.Run(fmt.Sprintf("gray_%dx%d", sz.w, sz.h), func(t *testing.T) {
			img := image.NewGray(image.Rect(0, 0, sz.w, sz.h))
			for y := 0; y < sz.h; y++ {
				for x := 0; x < sz.w; x++ {
					img.SetGray(x, y, color.Gray{Y: uint8((x*37 + y*53) & 0xff)})
				}
			}
			assertFastPathMatchesLibpng(t, "gray", encodePNGForTest(t, img))
		})

		t.Run(fmt.Sprintf("rgb_opaque_%dx%d", sz.w, sz.h), func(t *testing.T) {
			img := image.NewNRGBA(image.Rect(0, 0, sz.w, sz.h))
			for y := 0; y < sz.h; y++ {
				for x := 0; x < sz.w; x++ {
					img.SetNRGBA(x, y, color.NRGBA{
						R: uint8((x*29 + y*3) & 0xff),
						G: uint8((x*7 + y*41) & 0xff),
						B: uint8((x*17 + y*11) & 0xff),
						A: 255,
					})
				}
			}
			assertFastPathMatchesLibpng(t, "rgb", encodePNGForTest(t, img))
		})

		t.Run(fmt.Sprintf("rgba_alpha_%dx%d", sz.w, sz.h), func(t *testing.T) {
			img := image.NewNRGBA(image.Rect(0, 0, sz.w, sz.h))
			for y := 0; y < sz.h; y++ {
				for x := 0; x < sz.w; x++ {
					img.SetNRGBA(x, y, color.NRGBA{
						R: uint8((x*29 + y*3) & 0xff),
						G: uint8((x*7 + y*41) & 0xff),
						B: uint8((x*17 + y*11) & 0xff),
						// (0,0) always lands on alpha 0, so the image is
						// never opaque and image/png always picks color
						// type 6 here, regardless of size -- and the
						// formula still cycles through values that are
						// neither 0 nor 255 elsewhere, exercising both the
						// fast path's a==255 shortcut and its premultiply
						// rounding.
						A: uint8((x*53 + y*97) & 0xff),
					})
				}
			}
			assertFastPathMatchesLibpng(t, "rgba", encodePNGForTest(t, img))
		})
	}
}

func encodePNGForTest(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

// TestFastPathMatchesLibpngGrayAlphaHandBuilt covers color type 4
// (gray+alpha), which Go's image/png encoder can never produce (its writer
// only ever emits color types 0, 2, 3, or 6 -- see image/png/writer.go's
// Encode), so these fixtures are hand-built with buildRawPNG instead.
func TestFastPathMatchesLibpngGrayAlphaHandBuilt(t *testing.T) {
	sizes := []struct{ w, h int }{{1, 1}, {1, 9}, {9, 1}, {7, 5}, {13, 11}}
	for _, sz := range sizes {
		sz := sz
		t.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(t *testing.T) {
			data := buildRawPNG(t, sz.w, sz.h, 4, 2,
				func(y int) []byte {
					row := make([]byte, sz.w*2)
					for x := 0; x < sz.w; x++ {
						row[x*2] = byte((x*31 + y*17) & 0xff)   // gray
						row[x*2+1] = byte((x*53 + y*97) & 0xff) // alpha
					}
					return row
				},
				func(y int) byte { return byte(y % 5) },
			)
			assertFastPathMatchesLibpng(t, "gray+alpha", data)
		})
	}
}

// TestFastPathMatchesLibpngAllFilterTypes deliberately cycles through all
// five PNG filter types (None, Sub, Up, Average, Paeth) on hand-built
// fixtures for every eligible color type. Go's image/png encoder picks
// filters adaptively per row with no way for a caller to request a
// specific one, and there is no guarantee any particular synthetic content
// makes it choose all five, so this uses buildRawPNG/pngEncodeFilterRow to
// construct the raw filtered bytes directly instead of relying on the
// stdlib encoder's heuristic.
func TestFastPathMatchesLibpngAllFilterTypes(t *testing.T) {
	const width, height = 11, 15 // >=5 rows so y%5 below cycles through every filter

	colorSpecs := []struct {
		name      string
		colorType byte
		channels  int
		rowBytes  func(y int) []byte
	}{
		{"gray", 0, 1, func(y int) []byte {
			row := make([]byte, width)
			for x := range row {
				row[x] = byte((x*41 + y*23) & 0xff)
			}
			return row
		}},
		{"rgb", 2, 3, func(y int) []byte {
			row := make([]byte, width*3)
			for x := 0; x < width; x++ {
				row[x*3] = byte((x*13 + y*29) & 0xff)
				row[x*3+1] = byte((x*37 + y*7) & 0xff)
				row[x*3+2] = byte((x*19 + y*43) & 0xff)
			}
			return row
		}},
		{"gray_alpha", 4, 2, func(y int) []byte {
			row := make([]byte, width*2)
			for x := 0; x < width; x++ {
				row[x*2] = byte((x*11 + y*31) & 0xff)
				row[x*2+1] = byte((x*59 + y*83) & 0xff)
			}
			return row
		}},
		{"rgba", 6, 4, func(y int) []byte {
			row := make([]byte, width*4)
			for x := 0; x < width; x++ {
				row[x*4] = byte((x*3 + y*61) & 0xff)
				row[x*4+1] = byte((x*67 + y*5) & 0xff)
				row[x*4+2] = byte((x*71 + y*13) & 0xff)
				row[x*4+3] = byte((x*89 + y*97) & 0xff)
			}
			return row
		}},
	}

	for _, cs := range colorSpecs {
		cs := cs
		t.Run(cs.name, func(t *testing.T) {
			data := buildRawPNG(t, width, height, cs.colorType, cs.channels, cs.rowBytes,
				func(y int) byte { return byte(y % 5) }, // None, Sub, Up, Average, Paeth, repeat
			)
			assertFastPathMatchesLibpng(t, cs.name, data)
		})
	}
}

// TestFastPathMatchesLibpngRepoFixtures runs the differential check over
// the repo's checked-in PNG fixtures. schrenshot.png is 8-bit RGBA with
// 124 IDAT chunks, exercising the multi-chunk IDAT concatenation path;
// htop.png and debian-logo.png are both 8-bit palette (color type 3) with
// a tRNS chunk, so they are expected to be correctly declined by the fast
// path rather than mis-decoded.
func TestFastPathMatchesLibpngRepoFixtures(t *testing.T) {
	cases := []struct {
		path         string
		wantEligible bool
	}{
		{filepath.Join("..", "..", "test_images", "schrenshot.png"), true},
		{filepath.Join("..", "..", "test_images", "htop.png"), false},
		{filepath.Join("..", "..", "test_images", "debian-logo.png"), false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(filepath.Base(tc.path), func(t *testing.T) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Skipf("fixture not available: %v", err)
			}
			if _, elig := parsePNGFastPath(data); elig != tc.wantEligible {
				t.Fatalf("parsePNGFastPath eligible = %v, want %v", elig, tc.wantEligible)
			}
			if !tc.wantEligible {
				return // nothing to differential-compare: the fast path never touches this file
			}
			assertFastPathMatchesLibpng(t, tc.path, data)
		})
	}
}

// TestFastPathMatchesLibpngExternalFixtures opportunistically extends the
// differential check to real-world PNGs outside the repo. It reuses
// NV_BENCH_IMAGE_DIR (see bench_test.go), the same environment variable
// used to point BenchmarkDecode at external fixtures such as this fast
// path's 02_2.png/03_31.png development images, and is skipped entirely
// when that variable is unset, so it adds no dependency for the committed
// suite or CI.
func TestFastPathMatchesLibpngExternalFixtures(t *testing.T) {
	dir := os.Getenv("NV_BENCH_IMAGE_DIR")
	if dir == "" {
		t.Skip("NV_BENCH_IMAGE_DIR not set")
	}

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.ToLower(filepath.Ext(path)) != ".png" {
			return err
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %s: %v", path, err)
			}
			if _, elig := parsePNGFastPath(data); !elig {
				t.Skipf("%s is not fast-path eligible", path)
			}
			assertFastPathMatchesLibpng(t, path, data)
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking NV_BENCH_IMAGE_DIR: %v", err)
	}
}

// buildRawPNG constructs a complete, spec-valid PNG byte stream (correct
// chunk CRCs, so it can also round-trip through Go's own image/png if
// needed) for the given color type, with each row's raw (unfiltered) bytes
// supplied by rowBytes and filter-encoded per row by filterForRow. This is
// the only way to get fixtures for color type 4 (gray+alpha, which
// image/png's encoder can never produce) and to guarantee coverage of a
// specific PNG filter type on a specific row (image/png's encoder picks
// filters adaptively with no way for a caller to choose).
func buildRawPNG(t *testing.T, width, height int, colorType byte, channels int, rowBytes func(y int) []byte, filterForRow func(y int) byte) []byte {
	t.Helper()

	var raw bytes.Buffer
	var prev []byte
	for y := 0; y < height; y++ {
		cur := rowBytes(y)
		if len(cur) != width*channels {
			t.Fatalf("rowBytes(%d) returned %d bytes, want %d", y, len(cur), width*channels)
		}
		filter := filterForRow(y)
		raw.WriteByte(filter)
		raw.Write(pngEncodeFilterRow(cur, prev, channels, filter))
		prev = cur
	}

	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}

	var out bytes.Buffer
	out.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(width))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(height))
	ihdr[8] = 8 // bit depth
	ihdr[9] = colorType
	ihdr[10] = 0 // compression method
	ihdr[11] = 0 // filter method
	ihdr[12] = 0 // interlace method: none
	writeCRCPNGChunk(&out, "IHDR", ihdr)
	writeCRCPNGChunk(&out, "IDAT", zbuf.Bytes())
	writeCRCPNGChunk(&out, "IEND", nil)
	return out.Bytes()
}

func writeCRCPNGChunk(buf *bytes.Buffer, typ string, data []byte) {
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	buf.Write(lenBuf[:])
	buf.WriteString(typ)
	buf.Write(data)

	sum := crc32.NewIEEE()
	sum.Write([]byte(typ))
	sum.Write(data)
	var crcBuf [4]byte
	binary.BigEndian.PutUint32(crcBuf[:], sum.Sum32())
	buf.Write(crcBuf[:])
}

// pngEncodeFilterRow computes the PNG filter-encoded bytes for cur (a
// row's true/unfiltered bytes) given prev (the previous row's true bytes,
// nil for the first row), implementing the exact inverse of the decode
// formulas nv_png_fastpath_decode applies in native_linux.go's cgo
// preamble, independently reimplemented here (rather than calling into
// that C code) so this test fixture generator does not share a bug with
// the code it's meant to validate.
func pngEncodeFilterRow(cur, prev []byte, channels int, filter byte) []byte {
	out := make([]byte, len(cur))
	for x := range cur {
		var a, b, c int
		if x >= channels {
			a = int(cur[x-channels])
		}
		if prev != nil {
			b = int(prev[x])
			if x >= channels {
				c = int(prev[x-channels])
			}
		}

		var pred int
		switch filter {
		case 0: // None
			pred = 0
		case 1: // Sub
			pred = a
		case 2: // Up
			pred = b
		case 3: // Average
			pred = (a + b) / 2
		case 4: // Paeth
			p := a + b - c
			pa, pb, pc := absInt(p-a), absInt(p-b), absInt(p-c)
			switch {
			case pa <= pb && pa <= pc:
				pred = a
			case pb <= pc:
				pred = b
			default:
				pred = c
			}
		default:
			panic(fmt.Sprintf("pngEncodeFilterRow: unsupported filter type %d", filter))
		}
		out[x] = byte(int(cur[x]) - pred)
	}
	return out
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
