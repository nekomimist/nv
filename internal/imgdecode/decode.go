package imgdecode

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

var errNativeUnavailable = errors.New("native decoder unavailable")

const nativePNGMinPixels = 1_000_000

// Hint expresses the largest size the caller needs. A zero field means "no
// limit" -- decode at full resolution on that axis.
type Hint struct {
	MaxWidth  int
	MaxHeight int
}

// Info describes what a scaled decode actually produced.
type Info struct {
	Width, Height             int  // dimensions actually produced
	SourceWidth, SourceHeight int  // full/native dimensions of the source
	Reduced                   bool // true when Width/Height < Source*
}

// DecodeFile decodes an image from a filesystem path at full resolution.
func DecodeFile(path string) (image.Image, error) {
	img, _, err := DecodeFileScaled(path, Hint{})
	return img, err
}

// DecodeFileScaled decodes an image from a filesystem path, using hint to
// request a reduced-resolution decode where the underlying decoder supports
// it. Callers that do not need Info can use DecodeFile instead.
func DecodeFileScaled(path string, hint Hint) (image.Image, Info, error) {
	if !nativeEnabled() {
		f, err := os.Open(path)
		if err != nil {
			return nil, Info{}, err
		}
		defer f.Close()

		img, _, err := image.Decode(f)
		if err != nil {
			return nil, Info{}, err
		}
		return img, infoFromImage(img), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, Info{}, err
	}
	return DecodeBytesScaled(data, path, hint)
}

// DecodeBytes decodes an image from memory at full resolution.
func DecodeBytes(data []byte, origin string) (image.Image, error) {
	img, _, err := DecodeBytesScaled(data, origin, Hint{})
	return img, err
}

// DecodeBytesScaled decodes an image from memory, using hint to request a
// reduced-resolution decode where the underlying decoder supports it (JPEG
// via the native decoder only). Every other path -- PNG, non-JPEG formats,
// and the stdlib decoder in general -- ignores hint and decodes at full
// resolution, reporting Info.Reduced == false. Callers that do not need
// Info can use DecodeBytes instead.
func DecodeBytesScaled(data []byte, origin string, hint Hint) (image.Image, Info, error) {
	if !shouldTryNative(data, origin) {
		img, err := decodeStdlib(data)
		if err != nil {
			return nil, Info{}, err
		}
		return img, infoFromImage(img), nil
	}

	img, info, err := decodeNative(data, origin, hint)
	if err == nil {
		return img, info, nil
	}
	nativeErr := err

	img, err = decodeStdlib(data)
	if err != nil {
		if nativeErr != errNativeUnavailable {
			return nil, Info{}, fmt.Errorf("native decode failed: %v; stdlib decode failed: %w", nativeErr, err)
		}
		return nil, Info{}, err
	}
	return img, infoFromImage(img), nil
}

// infoFromImage builds the Info for a decode that ignored (or had no) scale
// hint: the produced image is always the full source resolution.
func infoFromImage(img image.Image) Info {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	return Info{Width: w, Height: h, SourceWidth: w, SourceHeight: h, Reduced: false}
}

func shouldTryNative(data []byte, origin string) bool {
	if !nativeEnabled() {
		return false
	}
	if isJPEGData(data) || hasJPEGExt(origin) {
		return true
	}
	if !isPNGData(data) && !hasPNGExt(origin) {
		return false
	}

	width, height, ok := pngDimensions(data)
	if !ok {
		return true
	}
	return int64(width)*int64(height) >= nativePNGMinPixels
}

func decodeStdlib(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return img, nil
}

func isPNGData(data []byte) bool {
	return len(data) >= 8 &&
		data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' &&
		data[4] == '\r' && data[5] == '\n' && data[6] == 0x1a && data[7] == '\n'
}

func pngDimensions(data []byte) (int, int, bool) {
	if len(data) < 24 || !isPNGData(data) {
		return 0, 0, false
	}
	if string(data[12:16]) != "IHDR" {
		return 0, 0, false
	}
	width := binary.BigEndian.Uint32(data[16:20])
	height := binary.BigEndian.Uint32(data[20:24])
	maxInt := uint64(^uint(0) >> 1)
	if width == 0 || height == 0 || uint64(width) > maxInt || uint64(height) > maxInt {
		return 0, 0, false
	}
	return int(width), int(height), true
}

func isJPEGData(data []byte) bool {
	return len(data) >= 2 && data[0] == 0xff && data[1] == 0xd8
}

func hasPNGExt(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".png"
}

func hasJPEGExt(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".jpg" || ext == ".jpeg"
}
