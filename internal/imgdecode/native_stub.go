//go:build !native_decode || !cgo || (!linux && !windows)

package imgdecode

import (
	"image"
)

func decodeNative(_ []byte, _ string, _ Hint) (image.Image, Info, error) {
	return nil, Info{}, errNativeUnavailable
}

func nativeEnabled() bool {
	return false
}
