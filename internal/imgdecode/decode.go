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

// DecodeFile decodes an image from a filesystem path.
func DecodeFile(path string) (image.Image, error) {
	if !nativeEnabled() {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		img, _, err := image.Decode(f)
		if err != nil {
			return nil, err
		}
		return img, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return DecodeBytes(data, path)
}

// DecodeBytes decodes an image from memory.
func DecodeBytes(data []byte, origin string) (image.Image, error) {
	if !shouldTryNative(data, origin) {
		return decodeStdlib(data)
	}

	img, err := decodeNative(data, origin)
	if err == nil {
		return img, nil
	}
	nativeErr := err

	img, err = decodeStdlib(data)
	if err != nil {
		if nativeErr != errNativeUnavailable {
			return nil, fmt.Errorf("native decode failed: %v; stdlib decode failed: %w", nativeErr, err)
		}
		return nil, err
	}
	return img, nil
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
