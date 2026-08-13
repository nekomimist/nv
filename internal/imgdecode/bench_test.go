package imgdecode

import (
	"crypto/sha1"
	"encoding/hex"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type benchImage struct {
	name   string
	data   []byte
	pixels int64
	bytes  int64
	width  int
	height int
}

func BenchmarkDecode(b *testing.B) {
	for _, tc := range benchmarkImages(b) {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(tc.bytes)
			b.ReportMetric(float64(tc.pixels)/1_000_000, "MPix/input")

			for b.Loop() {
				img, err := DecodeBytes(tc.data, tc.name)
				if err != nil {
					b.Fatalf("DecodeBytes failed: %v", err)
				}
				if img.Bounds().Empty() {
					b.Fatalf("decoded image is empty")
				}
			}
			b.ReportMetric(float64(tc.pixels*int64(b.N))/b.Elapsed().Seconds()/1_000_000, "MPix/s")
		})
	}
}

// BenchmarkDecodeScaled requests a decode at half of each fixture's native
// size, so it directly measures Stage 2a's payoff: for a JPEG served by the
// native decoder, this hint should land on the 1/2 libjpeg-turbo/WIC DCT
// scaling factor and decode substantially cheaper than BenchmarkDecode's
// full-resolution pass over the same bytes. htop.png/debian-logo.png (and
// any other PNG fixture) go through the untouched stdlib decoder either
// way -- PNG ignores the hint entirely -- so they serve as noise controls:
// their numbers here should be indistinguishable from BenchmarkDecode's.
func BenchmarkDecodeScaled(b *testing.B) {
	for _, tc := range benchmarkImages(b) {
		if tc.width <= 0 || tc.height <= 0 {
			continue
		}
		hint := Hint{MaxWidth: (tc.width + 1) / 2, MaxHeight: (tc.height + 1) / 2}

		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(tc.bytes)
			b.ReportMetric(float64(tc.pixels)/1_000_000, "MPix/input")

			for b.Loop() {
				img, info, err := DecodeBytesScaled(tc.data, tc.name, hint)
				if err != nil {
					b.Fatalf("DecodeBytesScaled failed: %v", err)
				}
				if img.Bounds().Empty() {
					b.Fatalf("decoded image is empty")
				}
				_ = info
			}
			b.ReportMetric(float64(tc.pixels*int64(b.N))/b.Elapsed().Seconds()/1_000_000, "MPix/s")
		})
	}
}

func benchmarkImages(tb testing.TB) []benchImage {
	tb.Helper()

	images := []benchImage{
		mustEncodePNG(tb, "synthetic_png_rgba_512", 512, 512),
		mustEncodePNG(tb, "synthetic_png_rgba_2048", 2048, 2048),
		mustEncodeJPEG(tb, "synthetic_jpeg_2048", 2048, 2048),
	}

	for _, path := range repoFixturePaths(tb) {
		img := readBenchFile(tb, path)
		if img.data != nil {
			images = append(images, img)
		}
	}

	if dir := os.Getenv("NV_BENCH_IMAGE_DIR"); dir != "" {
		for _, path := range externalFixturePaths(tb, dir) {
			img := readBenchFile(tb, path)
			if img.data != nil {
				images = append(images, img)
			}
		}
	}

	return images
}

func repoFixturePaths(tb testing.TB) []string {
	tb.Helper()
	candidates := []string{
		filepath.Join("..", "..", "test_images", "schrenshot.png"),
		filepath.Join("..", "..", "test_images", "htop.png"),
		filepath.Join("..", "..", "test_images", "debian-logo.png"),
	}
	var paths []string
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			paths = append(paths, path)
		}
	}
	return paths
}

func externalFixturePaths(tb testing.TB, dir string) []string {
	tb.Helper()
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".png", ".jpg", ".jpeg", ".webp":
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		tb.Fatalf("walking NV_BENCH_IMAGE_DIR: %v", err)
	}
	return paths
}

func readBenchFile(tb testing.TB, path string) benchImage {
	tb.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("reading %s: %v", path, err)
	}
	img, err := decodeStdlib(data)
	if err != nil {
		tb.Logf("skipping %s: %v", path, err)
		return benchImage{}
	}
	b := img.Bounds()
	return benchImage{
		name:   benchName(path),
		data:   data,
		pixels: int64(b.Dx()) * int64(b.Dy()),
		bytes:  int64(len(data)),
		width:  b.Dx(),
		height: b.Dy(),
	}
}

func benchName(path string) string {
	base := filepath.Base(path)
	sum := sha1.Sum([]byte(path))
	return sanitizeBenchName(base) + "_" + hex.EncodeToString(sum[:4])
}

func sanitizeBenchName(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")
	return replacer.Replace(name)
}

func mustEncodePNG(tb testing.TB, name string, w, h int) benchImage {
	tb.Helper()
	img := syntheticImage(w, h)
	var buf strings.Builder
	if err := png.Encode(stringWriter{&buf}, img); err != nil {
		tb.Fatalf("encoding synthetic PNG: %v", err)
	}
	return benchImage{name: name, data: []byte(buf.String()), pixels: int64(w) * int64(h), bytes: int64(buf.Len()), width: w, height: h}
}

func mustEncodeJPEG(tb testing.TB, name string, w, h int) benchImage {
	tb.Helper()
	img := syntheticImage(w, h)
	var buf strings.Builder
	if err := jpeg.Encode(stringWriter{&buf}, img, &jpeg.Options{Quality: 90}); err != nil {
		tb.Fatalf("encoding synthetic JPEG: %v", err)
	}
	return benchImage{name: name, data: []byte(buf.String()), pixels: int64(w) * int64(h), bytes: int64(buf.Len()), width: w, height: h}
}

func syntheticImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8((x*31 + y*7) & 0xff),
				G: uint8((x*3 + y*19) & 0xff),
				B: uint8((x*11 + y*13) & 0xff),
				A: uint8(180 + ((x + y) & 0x3f)),
			})
		}
	}
	return img
}

type stringWriter struct {
	builder *strings.Builder
}

func (w stringWriter) Write(p []byte) (int, error) {
	return w.builder.Write(p)
}
