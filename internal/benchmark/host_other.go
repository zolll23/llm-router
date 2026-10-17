//go:build !darwin

package benchmark

import "runtime"

func detectHostNative() Hardware {
	// Linux container without host-detect file — weak fingerprint only.
	return Hardware{
		Arch:   runtime.GOARCH,
		Source: "container",
		Chip:   runtime.GOOS + "/" + runtime.GOARCH,
	}
}
