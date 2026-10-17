//go:build darwin

package benchmark

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

func detectHostNative() Hardware {
	hw := Hardware{
		Arch:      "apple_silicon",
		PreferMLX: true,
		Unified:   true,
		Source:    "sysctl",
	}
	if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
		if n, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64); err == nil && n > 0 {
			hw.RAMGB = int(n / (1024 * 1024 * 1024))
			hw.VRAMGB = hw.RAMGB
		}
	}
	if out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
		hw.Chip = strings.TrimSpace(string(out))
	}
	if hw.Chip == "" {
		if out, err := exec.Command("sysctl", "-n", "hw.model").Output(); err == nil {
			hw.Chip = strings.TrimSpace(string(out))
		}
	}
	if hw.Chip == "" {
		hw.Chip = "Apple Silicon (" + runtime.GOARCH + ")"
	}
	if strings.Contains(strings.ToLower(hw.Chip), "apple") || runtime.GOARCH == "arm64" {
		hw.Arch = "apple_silicon"
		hw.PreferMLX = true
		hw.Unified = true
	}
	return hw
}
