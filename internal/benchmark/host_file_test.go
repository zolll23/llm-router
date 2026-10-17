package benchmark_test

import (
	"os"
	"path/filepath"
	"testing"

	"router-app/internal/benchmark"
)

func TestDetectHostReadsHostFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "host_hardware.json")
	content := `{
  "vram": 36,
  "ram": 36,
  "prefer_mlx": true,
  "unified": true,
  "arch": "apple_silicon",
  "chip": "Apple M5",
  "source": "host-detect"
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ROUTER_HOST_HARDWARE_FILE", path)

	hw := benchmark.DetectHost()
	if hw.RAMGB != 36 || hw.Chip != "Apple M5" || !hw.PreferMLX {
		t.Fatalf("got %+v", hw)
	}
	if hw.Source != "host-detect" {
		t.Fatalf("source=%q", hw.Source)
	}
}
