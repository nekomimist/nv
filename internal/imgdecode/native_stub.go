//go:build !native_decode || !cgo || (!linux && !windows)

package imgdecode

import (
	"image"
)

func decodeNative(_ []byte, _ string) (image.Image, error) {
	return nil, errNativeUnavailable
}

func nativeEnabled() bool {
	return false
}
