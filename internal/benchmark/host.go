package benchmark

import (
	"encoding/json"
	"os"
	"strings"
)

// Default paths checked inside the router container / local process.
var hostHardwarePaths = []string{
	"", // filled from env at runtime
	"/host/host_hardware.json",
	"/data/host_hardware.json",
	"data/host_hardware.json",
}

// DetectHost returns hardware for recommendations.
// Prefer a host-detect file written on the Docker host at compose/make time,
// then native sysctl (macOS), then a weak container fingerprint.
func DetectHost() Hardware {
	if hw, ok := loadHostHardwareFile(""); ok && hw.RAMGB > 0 {
		return hw
	}
	return detectHostNative()
}

func loadHostHardwareFile(explicit string) (Hardware, bool) {
	paths := make([]string, 0, len(hostHardwarePaths)+1)
	if explicit != "" {
		paths = append(paths, explicit)
	}
	if env := strings.TrimSpace(os.Getenv("ROUTER_HOST_HARDWARE_FILE")); env != "" {
		paths = append(paths, env)
	}
	for _, p := range hostHardwarePaths {
		if p == "" {
			continue
		}
		paths = append(paths, p)
	}
	seen := map[string]bool{}
	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var hw Hardware
		if err := json.Unmarshal(b, &hw); err != nil {
			continue
		}
		if hw.Source == "" {
			hw.Source = "host-file"
		}
		if hw.RAMGB > 0 && hw.VRAMGB <= 0 {
			hw.VRAMGB = hw.RAMGB
		}
		if hw.PreferMLX || hw.Arch == "apple_silicon" || hw.Unified {
			hw.PreferMLX = true
			hw.Unified = true
			if hw.Arch == "" {
				hw.Arch = "apple_silicon"
			}
		}
		return hw, true
	}
	return Hardware{}, false
}
