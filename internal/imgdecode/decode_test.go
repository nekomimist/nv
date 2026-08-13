package imgdecode

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeBytesPNGMatchesBounds(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 16, 12))
	src.SetNRGBA(3, 4, color.NRGBA{R: 10, G: 20, B: 30, A: 128})

	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatalf("png encode: %v", err)
	}

	img, err := DecodeBytes(buf.Bytes(), "test.png")
	if err != nil {
		t.Fatalf("DecodeBytes failed: %v", err)
	}
	if got, want := img.Bounds(), src.Bounds(); got != want {
		t.Fatalf("bounds = %v, want %v", got, want)
	}
}

func TestDecodeFileJPEG(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 20, 10))
	src.SetRGBA(7, 3, color.RGBA{R: 200, G: 100, B: 50, A: 255})

	path := filepath.Join(t.TempDir(), "sample.jpg")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := jpeg.Encode(f, src, &jpeg.Options{Quality: 90}); err != nil {
		_ = f.Close()
		t.Fatalf("jpeg encode: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}

	img, err := DecodeFile(path)
	if err != nil {
		t.Fatalf("DecodeFile failed: %v", err)
	}
	if got, want := img.Bounds(), src.Bounds(); got != want {
		t.Fatalf("bounds = %v, want %v", got, want)
	}
}

func TestDecodeBytesInvalid(t *testing.T) {
	if _, err := DecodeBytes([]byte("not an image"), "bad.bin"); err == nil {
		t.Fatalf("expected invalid image error")
	}
}

// TestDecodeBytesScaledIgnoresHintOutsideNativeJPEG covers every decode path
// that Stage 2a leaves untouched: a zero Hint (full-resolution passthrough)
// and PNG (which has no scaled-decode capability in libpng or the stdlib
// decoder, so a hint must not change its output at all) -- including a PNG
// large enough to cross nativePNGMinPixels, so this also exercises the
// native PNG decoder's Info reporting on native_decode builds, not only the
// stdlib fallback used by the small cases and by non-native builds. Every
// case must report Info.Reduced == false with Width/Height ==
// SourceWidth/SourceHeight == the image's real dimensions, regardless of
// which decoder actually served it.
func TestDecodeBytesScaledIgnoresHintOutsideNativeJPEG(t *testing.T) {
	pngSmall := encodeTestPNG(t, 24, 18)
	pngAtNativeGate := encodeTestPNG(t, 1024, 1024) // >= nativePNGMinPixels

	cases := []struct {
		name   string
		data   []byte
		origin string
		hint   Hint
		wantW  int
		wantH  int
	}{
		{"zero hint, small PNG", pngSmall, "zero.png", Hint{}, 24, 18},
		{"small hint, small PNG (below native PNG gate)", pngSmall, "small.png", Hint{MaxWidth: 4, MaxHeight: 4}, 24, 18},
		{"small hint, large PNG (at/above native PNG gate)", pngAtNativeGate, "large.png", Hint{MaxWidth: 4, MaxHeight: 4}, 1024, 1024},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img, info, err := DecodeBytesScaled(tc.data, tc.origin, tc.hint)
			if err != nil {
				t.Fatalf("DecodeBytesScaled failed: %v", err)
			}
			if info.Reduced {
				t.Fatalf("Info.Reduced = true, want false")
			}
			want := Info{Width: tc.wantW, Height: tc.wantH, SourceWidth: tc.wantW, SourceHeight: tc.wantH}
			if info != want {
				t.Fatalf("Info = %+v, want %+v", info, want)
			}
			if got := img.Bounds(); got.Dx() != tc.wantW || got.Dy() != tc.wantH {
				t.Fatalf("bounds = %v, want %dx%d", got, tc.wantW, tc.wantH)
			}
		})
	}
}

func encodeTestPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	src := image.NewNRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}
