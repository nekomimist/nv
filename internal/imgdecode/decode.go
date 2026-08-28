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

	_ "github.com/gen2brain/jxl"
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

// containTarget resolves hint -- the box the decoded image will be
// displayed in -- to the smallest size a decoder must produce so that the
// image fits entirely inside that box on both axes ("contain"/fit-inside
// scaling), preserving the source's aspect ratio. This is deliberately not
// "cover" scaling: covering both axes of a box whose aspect ratio doesn't
// match the source's would over-decode the axis that isn't the visual
// bottleneck (e.g. a tall portrait image bound for a wide landscape box
// only needs to be decoded as wide as it will actually be drawn, not as
// wide as the box).
//
// A zero (or negative) Hint field means "unconstrained" on that axis --
// matching Hint's documented zero-value semantics -- so Hint{} always
// yields (srcW, srcH) unchanged, and a single populated field expresses
// fit-width ({MaxWidth: W}) or fit-height ({MaxHeight: H}) alone. The
// result never exceeds (srcW, srcH) on either axis (this function only
// ever asks a decoder to shrink, never to enlarge) and is never below 1 on
// either axis, even for extreme aspect ratios.
//
// Each backend is responsible for rounding this ideal target *up* to
// whatever concrete size its codec can actually produce (e.g. the nearest
// libjpeg-turbo DCT scaling factor, or the nearest WIC power-of-two
// division) -- containTarget only computes the shared "contain" math, not
// that codec-specific step.
//
// All arithmetic is integer cross-multiplication (no floating point), so
// results are exact regardless of image size; products are computed in
// int64 so a very large source (tens of thousands of pixels per side)
// cannot silently overflow.
func containTarget(srcW, srcH int, hint Hint) (int, int) {
	if srcW <= 0 || srcH <= 0 {
		return srcW, srcH
	}

	hasW := hint.MaxWidth > 0
	hasH := hint.MaxHeight > 0
	if !hasW && !hasH {
		return srcW, srcH
	}

	sw, sh := int64(srcW), int64(srcH)
	hw, hh := int64(hint.MaxWidth), int64(hint.MaxHeight)

	switch {
	case hasW && !hasH:
		if hw >= sw {
			return srcW, srcH
		}
		return int(hw), clampDim(ceilDiv(sh*hw, sw))
	case !hasW && hasH:
		if hh >= sh {
			return srcW, srcH
		}
		return clampDim(ceilDiv(sw*hh, sh)), int(hh)
	default: // hasW && hasH
		if hw >= sw && hh >= sh {
			return srcW, srcH
		}
		// Pick the binding (smaller-scale) axis without floating point:
		// scale_w = hw/sw, scale_h = hh/sh, and scale_w <= scale_h iff
		// hw*sh <= hh*sw (cross-multiplication; valid since sw, sh > 0).
		if hw*sh <= hh*sw {
			return int(hw), clampDim(ceilDiv(sh*hw, sw))
		}
		return clampDim(ceilDiv(sw*hh, sh)), int(hh)
	}
}

// ceilDiv returns ceil(a/b) for positive a and b, computed without
// floating point.
func ceilDiv(a, b int64) int64 {
	return (a + b - 1) / b
}

// clampDim converts v to int, clamping up to 1 so containTarget can never
// report a zero or negative dimension.
func clampDim(v int64) int {
	if v < 1 {
		return 1
	}
	return int(v)
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
// and WebP via the native decoder only). Every other path -- PNG, JPEG XL,
// other non-JPEG/WebP formats, and the registered Go decoders in general --
// ignores hint and decodes at full resolution, reporting Info.Reduced ==
// false. Callers that do not need Info can use DecodeBytes instead.
func DecodeBytesScaled(data []byte, origin string, hint Hint) (image.Image, Info, error) {
	if !shouldTryNative(data, origin) {
		img, err := decodeRegistered(data)
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

	img, err = decodeRegistered(data)
	if err != nil {
		if nativeErr != errNativeUnavailable {
			return nil, Info{}, fmt.Errorf("native decode failed: %v; Go decode failed: %w", nativeErr, err)
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
	if isWebPData(data) || hasWebPExt(origin) {
		// Unlike PNG, WebP has no small-image threshold: the pure-Go
		// golang.org/x/image/webp fallback is dramatically slower than the
		// native decoder at every size (measured ~582ms / ~51.6 MPix/s for
		// a 30MP image, versus low milliseconds natively), so there is no
		// image small enough for the registered Go path to be worth preferring.
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

func decodeRegistered(data []byte) (image.Image, error) {
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

// isWebPData reports whether data starts with a RIFF/WEBP container header:
// bytes 0-3 "RIFF", bytes 8-11 "WEBP" (bytes 4-7 are the RIFF chunk size,
// which this check does not need to validate).
func isWebPData(data []byte) bool {
	return len(data) >= 12 &&
		data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' &&
		data[8] == 'W' && data[9] == 'E' && data[10] == 'B' && data[11] == 'P'
}

func hasPNGExt(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".png"
}

func hasJPEGExt(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".jpg" || ext == ".jpeg"
}

func hasWebPExt(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".webp"
}
