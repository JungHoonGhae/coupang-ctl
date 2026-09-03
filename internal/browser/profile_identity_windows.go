//go:build windows

package browser

import (
	"context"
	"unsafe"

	"golang.org/x/sys/windows"
)

func installedBrowserMajorVersion(ctx context.Context, executable string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	var zero windows.Handle
	size, err := windows.GetFileVersionInfoSize(executable, &zero)
	if err != nil || size == 0 {
		return 0, ErrProfileIncompatible
	}
	buffer := make([]byte, size)
	if err := windows.GetFileVersionInfo(executable, 0, size, unsafe.Pointer(&buffer[0])); err != nil {
		return 0, ErrProfileIncompatible
	}
	var fixed *windows.VS_FIXEDFILEINFO
	fixedSize := uint32(unsafe.Sizeof(*fixed))
	if err := windows.VerQueryValue(unsafe.Pointer(&buffer[0]), `\`, unsafe.Pointer(&fixed), &fixedSize); err != nil || fixed == nil || fixedSize < uint32(unsafe.Sizeof(*fixed)) || fixed.Signature != 0xfeef04bd {
		return 0, ErrProfileIncompatible
	}
	major := int((fixed.FileVersionMS >> 16) & 0xffff)
	if major < 1 || major > 9999 {
		return 0, ErrProfileIncompatible
	}
	return major, nil
}
