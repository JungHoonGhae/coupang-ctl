//go:build !windows

package browserbridge

import "fmt"

func newPlatformRegistration(goos string) (platformRegistration, error) {
	if goos == "windows" {
		return nil, fmt.Errorf("Windows browser bridge registration must run on Windows")
	}
	return filePlatformRegistration{}, nil
}
