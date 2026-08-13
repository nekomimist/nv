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

// TestContainTarget covers containTarget's "contain" (fit-inside) math in
// isolation, without cgo: the smallest size that fits entirely inside
// hint's box while preserving the source's aspect ratio, never upscaled
// past the source, and never below 1 on either axis.
func TestContainTarget(t *testing.T) {
	cases := []struct {
		name         string
		srcW, srcH   int
		hint         Hint
		wantW, wantH int
	}{
		{
			name: "landscape source in landscape box",
			srcW: 1920, srcH: 1080,
			hint:  Hint{MaxWidth: 800, MaxHeight: 600},
			wantW: 800, wantH: 450,
		},
		{
			// The bug this task fixes: a portrait source must not be
			// decoded at the full landscape box height ("cover"); it only
			// needs to be as tall as it will actually be drawn once its
			// width is fit to the box.
			name: "portrait source in landscape box",
			srcW: 1080, srcH: 1920,
			hint:  Hint{MaxWidth: 800, MaxHeight: 600},
			wantW: 338, wantH: 600,
		},
		{
			name: "landscape source in portrait box (reverse of above)",
			srcW: 1920, srcH: 1080,
			hint:  Hint{MaxWidth: 600, MaxHeight: 800},
			wantW: 600, wantH: 338,
		},
		{
			name: "hint larger than source on every axis: no reduction",
			srcW: 800, srcH: 600,
			hint:  Hint{MaxWidth: 2000, MaxHeight: 1500},
			wantW: 800, wantH: 600,
		},
		{
			name: "one axis zero: fit-width",
			srcW: 1920, srcH: 1080,
			hint:  Hint{MaxWidth: 800},
			wantW: 800, wantH: 450,
		},
		{
			name: "one axis zero: fit-height",
			srcW: 1920, srcH: 1080,
			hint:  Hint{MaxHeight: 600},
			wantW: 1067, wantH: 600,
		},
		{
			name: "both axes zero: no constraint",
			srcW: 4000, srcH: 3000,
			hint:  Hint{},
			wantW: 4000, wantH: 3000,
		},
		{
			name: "square source",
			srcW: 1000, srcH: 1000,
			hint:  Hint{MaxWidth: 300, MaxHeight: 500},
			wantW: 300, wantH: 300,
		},
		{
			name: "1x1 source, hint larger: no reduction",
			srcW: 1, srcH: 1,
			hint:  Hint{MaxWidth: 800, MaxHeight: 600},
			wantW: 1, wantH: 1,
		},
		{
			// Without ceiling division (and a floor-to-1 clamp), the
			// non-binding axis here would compute as 1*1/10000 == 0.
			name: "extreme aspect ratio would round to 0 without ceil+clamp",
			srcW: 1, srcH: 10000,
			hint:  Hint{MaxWidth: 1, MaxHeight: 1},
			wantW: 1, wantH: 1,
		},
		{
			// Same pitfall via the single-axis (fit-height) path: floor(5*3/1000) == 0.
			name: "fit-height extreme ratio would round to 0 without ceil+clamp",
			srcW: 5, srcH: 1000,
			hint:  Hint{MaxHeight: 3},
			wantW: 1, wantH: 3,
		},
		{
			// Large enough that a naive int32 product (srcW*hint or
			// srcH*hint) could overflow; containTarget must compute this
			// with int64 products and still land on the exact answer.
			name: "very large source: no overflow",
			srcW: 30000, srcH: 30000,
			hint:  Hint{MaxWidth: 15000, MaxHeight: 10000},
			wantW: 10000, wantH: 10000,
		},
		{
			// The real-world case from the bug report (4784x6278 portrait
			// WebP fit into a 2560x1440 landscape box): height is the
			// binding axis, width should land far below the 2560 hint.
			name: "real fixture aspect ratio: portrait WebP in landscape box",
			srcW: 4784, srcH: 6278,
			hint:  Hint{MaxWidth: 2560, MaxHeight: 1440},
			wantW: 1098, wantH: 1440,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotW, gotH := containTarget(tc.srcW, tc.srcH, tc.hint)
			if gotW != tc.wantW || gotH != tc.wantH {
				t.Fatalf("containTarget(%d, %d, %+v) = (%d, %d), want (%d, %d)",
					tc.srcW, tc.srcH, tc.hint, gotW, gotH, tc.wantW, tc.wantH)
			}
			if gotW > tc.srcW || gotH > tc.srcH {
				t.Fatalf("containTarget produced %dx%d, larger than source %dx%d", gotW, gotH, tc.srcW, tc.srcH)
			}
			if gotW < 1 || gotH < 1 {
				t.Fatalf("containTarget produced %dx%d, want both dimensions >= 1", gotW, gotH)
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
