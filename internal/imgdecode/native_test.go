//go:build cgo && native_decode

package imgdecode

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// TestNativeDecodePNGAlphaIsPremultiplied proves the one thing Stage 1b
// must never get wrong silently: bytes handed to Ebiten as *image.RGBA must
// actually be premultiplied. It calls decodeNative directly (rather than
// DecodeBytes) so the test is fast, unambiguous, and unaffected by
// shouldTryNative's size gate for PNG.
func TestNativeDecodePNGAlphaIsPremultiplied(t *testing.T) {
	const w, h = 3, 1

	semiTransparent := color.NRGBA{R: 200, G: 0, B: 0, A: 128}
	opaque := color.NRGBA{R: 10, G: 20, B: 30, A: 255}
	fullyTransparent := color.NRGBA{R: 90, G: 80, B: 70, A: 0}

	src := image.NewNRGBA(image.Rect(0, 0, w, h))
	src.SetNRGBA(0, 0, semiTransparent)
	src.SetNRGBA(1, 0, opaque)
	src.SetNRGBA(2, 0, fullyTransparent)

	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatalf("png encode: %v", err)
	}

	img, _, err := decodeNative(buf.Bytes(), "alpha.png", Hint{})
	if err != nil {
		t.Fatalf("decodeNative failed: %v", err)
	}
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatalf("decoded type = %T, want *image.RGBA", img)
	}

	cases := []struct {
		name string
		x    int
		want color.NRGBA
	}{
		{"semi-transparent", 0, semiTransparent},
		{"opaque", 1, opaque},
		{"fully-transparent", 2, fullyTransparent},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertPremultiplied(t, rgba.RGBAAt(tc.x, 0), tc.want)
		})
	}
}

// assertPremultiplied checks got (raw bytes read back from a decoded
// *image.RGBA) against want, an NRGBA source pixel, by computing the
// expected premultiplied bytes independently of the native decoder via
// color.NRGBA.RGBA() -- which always returns premultiplied 16-bit channels
// per the color.Color contract -- rather than re-deriving the
// implementation's own formula, so a test using this helper can't pass
// vacuously. RGB channels allow +/-1 for libpng/WIC integer-rounding
// differences from Go's own premultiply arithmetic; alpha must match
// exactly.
func assertPremultiplied(t *testing.T, got color.RGBA, want color.NRGBA) {
	t.Helper()

	wantR16, wantG16, wantB16, wantA16 := want.RGBA()
	wantR, wantG, wantB, wantA := uint8(wantR16>>8), uint8(wantG16>>8), uint8(wantB16>>8), uint8(wantA16>>8)

	if d := absDiffU8(got.R, wantR); d > 1 {
		t.Errorf("R = %d, want %d (+/-1)", got.R, wantR)
	}
	if d := absDiffU8(got.G, wantG); d > 1 {
		t.Errorf("G = %d, want %d (+/-1)", got.G, wantG)
	}
	if d := absDiffU8(got.B, wantB); d > 1 {
		t.Errorf("B = %d, want %d (+/-1)", got.B, wantB)
	}
	if got.A != wantA {
		t.Errorf("A = %d, want %d", got.A, wantA)
	}
}

// interlacedAdam7PNGBase64 is an 8x8, color-type-6 (truecolor+alpha) PNG
// using Adam7 interlacing, exercising nv_png_decode's interlaced fallback
// path (the post-png_read_image sweep, not the fused per-row transform,
// which is deliberately skipped for interlaced images -- see the comment
// on png_set_interlace_handling in native_linux.go's cgo preamble).
//
// Go's image/png encoder cannot produce this fixture: writer.go hardcodes
// the IHDR interlace-method byte to 0 (non-interlaced) with no option to
// change it. So this was generated once, offline, from the exact pixel
// grid interlacedAdam7Pixels describes below (via image/png, non-
// interlaced) and then re-encoded to Adam7 with ImageMagick
// (`convert base.png -interlace PNG -define png:color-type=6 interlaced.png`).
// Before embedding, the re-encoded bytes were decoded with Go's own
// image/png and confirmed pixel-for-pixel identical (all four RGBA
// channels, for every pixel) to the original non-interlaced source, so
// this is a real Adam7 bitstream carrying known, verified pixel data --
// not a hand-authored approximation.
const interlacedAdam7PNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAgAAAAICAYAAAGzCI4dAAAAIGNIUk0AAHomAACAhAAA+gAAAIDoAAB1MAAA6mAAADqYAAAXcJy6UTwAAAAGYktHRAD/AP8A/6C9p5MAAABiSURBVBjTfcshEgAhDEPRb2rQaE6CQKHRPQ3H65liWMUsOzuDa/JSAiYyBT46KZf1HjKFqTZSLstUW8D00eEMMsV/sWXD/+XUgImpNpkiYKZclo/+6bhhymVxQx8dbmiq7QHDw2EjkqVEsAAAAABJRU5ErkJggg=="

// interlacedAdam7Pixels reconstructs the exact 8x8 NRGBA grid the fixture
// above encodes: a 5-color palette (spanning semi-transparent, opaque, and
// fully-transparent pixels) tiled as colors[(x+y*3)%len(colors)].
func interlacedAdam7Pixels() [8][8]color.NRGBA {
	colors := [5]color.NRGBA{
		{R: 200, G: 0, B: 0, A: 128},
		{R: 10, G: 20, B: 30, A: 255},
		{R: 90, G: 80, B: 70, A: 0},
		{R: 5, G: 250, B: 60, A: 64},
		{R: 250, G: 5, B: 250, A: 200},
	}
	var grid [8][8]color.NRGBA
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			grid[y][x] = colors[(x+y*3)%len(colors)]
		}
	}
	return grid
}

// TestNativeDecodeInterlacedPNGAlphaIsPremultiplied covers the interlaced
// fallback branch of nv_png_decode: with Adam7, the per-row user-transform
// hook is skipped (a given output row is revisited across passes, so
// premultiplying per row-callback would multiply by alpha more than once),
// and a single post-png_read_image sweep runs instead. This asserts every
// pixel of a genuinely interlaced source still comes out correctly (and
// only once) premultiplied.
func TestNativeDecodeInterlacedPNGAlphaIsPremultiplied(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString(interlacedAdam7PNGBase64)
	if err != nil {
		t.Fatalf("decoding fixture base64: %v", err)
	}

	img, _, err := decodeNative(data, "interlaced.png", Hint{})
	if err != nil {
		t.Fatalf("decodeNative failed: %v", err)
	}
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatalf("decoded type = %T, want *image.RGBA", img)
	}
	if got, want := rgba.Bounds(), image.Rect(0, 0, 8, 8); got != want {
		t.Fatalf("bounds = %v, want %v", got, want)
	}

	grid := interlacedAdam7Pixels()
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			t.Run(fmt.Sprintf("%d,%d", x, y), func(t *testing.T) {
				assertPremultiplied(t, rgba.RGBAAt(x, y), grid[y][x])
			})
		}
	}
}

// TestNativeDecodeJPEGIsOpaqueRGBA proves the JPEG path needs no
// premultiply pass: TJPF_RGBA/WIC JPEG output is always fully opaque, so
// every alpha byte in the decoded buffer must be 255.
func TestNativeDecodeJPEGIsOpaqueRGBA(t *testing.T) {
	const w, h = 8, 8

	src := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			src.SetRGBA(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 30), B: 128, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}

	img, _, err := decodeNative(buf.Bytes(), "photo.jpg", Hint{})
	if err != nil {
		t.Fatalf("decodeNative failed: %v", err)
	}
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatalf("decoded type = %T, want *image.RGBA", img)
	}

	for i := 3; i < len(rgba.Pix); i += 4 {
		if rgba.Pix[i] != 255 {
			t.Fatalf("alpha byte at Pix offset %d = %d, want 255", i, rgba.Pix[i])
		}
	}
}

// TestDecodeBytesScaledJPEGReducesResolution proves the Stage 2a scaled-
// decode capability end to end through the public API: a large synthetic
// JPEG decoded with a size hint comes back reduced, reports its true source
// dimensions, and lands on the smallest libjpeg-turbo M/8 DCT scaling
// factor that still meets the requested budget -- not merely "some smaller
// size".
func TestDecodeBytesScaledJPEGReducesResolution(t *testing.T) {
	const srcW, srcH = 2048, 2048
	// The M/8 factor family gives candidate widths of 256*M for M=1..8
	// (after excluding upscaling factors). For an 800px budget, 3/8*2048
	// = 768 (< 800, disqualified) and 1/2*2048 = 1024 (>= 800, smallest
	// qualifying), so 1024x1024 is the one deterministically correct
	// answer here, not just "a reduced size".
	hint := Hint{MaxWidth: 800, MaxHeight: 800}
	const wantW, wantH = 1024, 1024

	src := image.NewRGBA(image.Rect(0, 0, srcW, srcH))
	for y := 0; y < srcH; y += 5 {
		for x := 0; x < srcW; x += 5 {
			src.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}

	img, info, err := DecodeBytesScaled(buf.Bytes(), "large.jpg", hint)
	if err != nil {
		t.Fatalf("DecodeBytesScaled failed: %v", err)
	}

	if !info.Reduced {
		t.Fatalf("Info.Reduced = false, want true")
	}
	if info.SourceWidth != srcW || info.SourceHeight != srcH {
		t.Fatalf("Info.SourceWidth/Height = %dx%d, want %dx%d", info.SourceWidth, info.SourceHeight, srcW, srcH)
	}
	if got := img.Bounds(); got.Dx() != info.Width || got.Dy() != info.Height {
		t.Fatalf("bounds = %v, want %dx%d (Info.Width/Height)", got, info.Width, info.Height)
	}
	if info.Width != wantW || info.Height != wantH {
		t.Fatalf("Width/Height = %dx%d, want %dx%d (smallest factor still meeting the %dx%d budget)",
			info.Width, info.Height, wantW, wantH, hint.MaxWidth, hint.MaxHeight)
	}
	if info.Width < hint.MaxWidth && info.Height < hint.MaxHeight {
		t.Fatalf("decoded %dx%d meets the %dx%d budget on neither axis", info.Width, info.Height, hint.MaxWidth, hint.MaxHeight)
	}

	wantRatio := float64(srcW) / float64(srcH)
	gotRatio := float64(info.Width) / float64(info.Height)
	if diff := wantRatio - gotRatio; diff > 0.01 || diff < -0.01 {
		t.Fatalf("aspect ratio = %v, want ~%v", gotRatio, wantRatio)
	}
}

// alphaWebPBase64 is a 3x1 lossless WebP encoding the same three colors
// (semi-transparent, opaque, fully-transparent) as
// TestNativeDecodePNGAlphaIsPremultiplied's PNG fixture above, generated
// offline with libwebp's WebPEncodeLosslessRGBA and, before embedding,
// decoded with Go's own golang.org/x/image/webp and confirmed pixel-for-
// pixel identical (all four RGBA channels) to the source NRGBA values below
// -- the same verification discipline used for interlacedAdam7PNGBase64.
const alphaWebPBase64 = "UklGRjYAAABXRUJQVlA4TCoAAAAvAgAAEBcgEEiC2J9wILFgsjt/ntgEAkkk+3NNIiAoum65gOo/1CCi/wE="

// TestNativeDecodeWebPAlphaIsPremultiplied is the WebP counterpart of
// TestNativeDecodePNGAlphaIsPremultiplied above: it proves decodeNative's
// WebP path also hands back correctly bounded, correctly premultiplied
// *image.RGBA bytes, via MODE_rgbA rather than libpng's row-transform hook.
func TestNativeDecodeWebPAlphaIsPremultiplied(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString(alphaWebPBase64)
	if err != nil {
		t.Fatalf("decoding fixture base64: %v", err)
	}

	img, info, err := decodeNative(data, "alpha.webp", Hint{})
	if err != nil {
		t.Fatalf("decodeNative failed: %v", err)
	}
	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatalf("decoded type = %T, want *image.RGBA", img)
	}
	if got, want := rgba.Bounds(), image.Rect(0, 0, 3, 1); got != want {
		t.Fatalf("bounds = %v, want %v", got, want)
	}
	if info.Reduced {
		t.Fatalf("Info.Reduced = true, want false (no hint was given)")
	}

	cases := []struct {
		name string
		x    int
		want color.NRGBA
	}{
		{"semi-transparent", 0, color.NRGBA{R: 200, G: 0, B: 0, A: 128}},
		{"opaque", 1, color.NRGBA{R: 10, G: 20, B: 30, A: 255}},
		{"fully-transparent", 2, color.NRGBA{R: 90, G: 80, B: 70, A: 0}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertPremultiplied(t, rgba.RGBAAt(tc.x, 0), tc.want)
		})
	}
}

// scaledWebPBase64 is an 80x40 (2:1 aspect ratio) opaque lossless WebP,
// generated the same way as alphaWebPBase64 above, used to exercise scaled
// decode through libwebp's use_scaling option.
const scaledWebPBase64 = "UklGRkYAAABXRUJQVlA4TDkAAAAvT8AJAAmASNoffYGI/qewUJC2AVP/vncVxMD3fwII/WNQ27YN4/r/494TKAQQAAVNBWICHq/t5wIA"

// TestNativeDecodeWebPScaledReducesResolution proves the WebP native
// decoder honours a Hint smaller than the source: libwebp's scaler accepts
// arbitrary target dimensions (unlike libjpeg-turbo's fixed M/8 DCT
// factors), so webpTargetSize's covering-size computation, not a factor
// table, determines the output size. The 20x20 hint against an 80x40
// (2:1) source requires covering both axes while preserving aspect ratio:
// scale = max(20/80, 20/40) = 0.5, giving the one deterministically correct
// answer of 40x20, not merely "some smaller size".
func TestNativeDecodeWebPScaledReducesResolution(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString(scaledWebPBase64)
	if err != nil {
		t.Fatalf("decoding fixture base64: %v", err)
	}

	const srcW, srcH = 80, 40
	hint := Hint{MaxWidth: 20, MaxHeight: 20}
	const wantW, wantH = 40, 20

	img, info, err := decodeNative(data, "scaled.webp", hint)
	if err != nil {
		t.Fatalf("decodeNative failed: %v", err)
	}

	if !info.Reduced {
		t.Fatalf("Info.Reduced = false, want true")
	}
	if info.SourceWidth != srcW || info.SourceHeight != srcH {
		t.Fatalf("Info.SourceWidth/Height = %dx%d, want %dx%d", info.SourceWidth, info.SourceHeight, srcW, srcH)
	}
	if got := img.Bounds(); got.Dx() != info.Width || got.Dy() != info.Height {
		t.Fatalf("bounds = %v, want %dx%d (Info.Width/Height)", got, info.Width, info.Height)
	}
	if info.Width != wantW || info.Height != wantH {
		t.Fatalf("Width/Height = %dx%d, want %dx%d (smallest size covering the %dx%d hint on both axes)",
			info.Width, info.Height, wantW, wantH, hint.MaxWidth, hint.MaxHeight)
	}

	wantRatio := float64(srcW) / float64(srcH)
	gotRatio := float64(info.Width) / float64(info.Height)
	if diff := wantRatio - gotRatio; diff > 0.01 || diff < -0.01 {
		t.Fatalf("aspect ratio = %v, want ~%v", gotRatio, wantRatio)
	}
}

func absDiffU8(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}
