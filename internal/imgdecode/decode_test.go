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
