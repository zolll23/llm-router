#!/usr/bin/env bash
# Collect host hardware on the machine running Docker (macOS/Linux)
# and write JSON for the router container to consume.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${ROUTER_HOST_HARDWARE_OUT:-$ROOT/data/host_hardware.json}"
mkdir -p "$(dirname "$OUT")"

ram_gb=0
vram_gb=0
chip=""
arch="unknown"
prefer_mlx=false
unified=false
source="host-detect"

os="$(uname -s)"
case "$os" in
  Darwin)
    mem_bytes="$(sysctl -n hw.memsize 2>/dev/null || echo 0)"
    if [[ "$mem_bytes" =~ ^[0-9]+$ ]] && [[ "$mem_bytes" -gt 0 ]]; then
      ram_gb=$((mem_bytes / 1024 / 1024 / 1024))
      vram_gb=$ram_gb
    fi
    chip="$(sysctl -n machdep.cpu.brand_string 2>/dev/null || true)"
    if [[ -z "$chip" ]]; then
      chip="$(sysctl -n hw.model 2>/dev/null || true)"
    fi
    # Apple Silicon reports arm64 / brands like "Apple M*"
    machine="$(uname -m)"
    if [[ "$machine" == "arm64" ]] || echo "$chip" | grep -qi 'apple'; then
      arch="apple_silicon"
      prefer_mlx=true
      unified=true
      vram_gb=$ram_gb
    else
      arch="darwin_${machine}"
    fi
    if [[ -z "$chip" ]]; then
      chip="Apple Silicon ($machine)"
    fi
    ;;
  Linux)
    if [[ -r /proc/meminfo ]]; then
      kb="$(awk '/MemTotal:/ {print $2}' /proc/meminfo)"
      if [[ "$kb" =~ ^[0-9]+$ ]]; then
        ram_gb=$((kb / 1024 / 1024))
        vram_gb=$ram_gb
      fi
    fi
    chip="$(uname -m)"
    arch="linux"
    # NVIDIA if present
    if command -v nvidia-smi >/dev/null 2>&1; then
      if vram_mib="$(nvidia-smi --query-gpu=memory.total --format=csv,noheader,nounits 2>/dev/null | head -1 | tr -d ' ')"; then
        if [[ "$vram_mib" =~ ^[0-9]+$ ]]; then
          vram_gb=$(( (vram_mib + 512) / 1024 ))
        fi
      fi
      chip="$(nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null | head -1 | sed 's/^ *//;s/ *$//')"
      arch="nvidia"
      prefer_mlx=false
      unified=false
    fi
    ;;
  *)
    chip="$(uname -s)/$(uname -m)"
    arch="unknown"
    ;;
esac

# Fallback if detection failed on Mac CI/odd hosts
if [[ "$ram_gb" -le 0 ]]; then
  echo "warn: could not read RAM; writing incomplete host profile" >&2
fi

tmp="$(mktemp)"
cat >"$tmp" <<EOF
{
  "vram": ${vram_gb},
  "ram": ${ram_gb},
  "prefer_mlx": ${prefer_mlx},
  "unified": ${unified},
  "arch": "$(printf '%s' "$arch" | sed 's/"/\\"/g')",
  "chip": "$(printf '%s' "$chip" | sed 's/"/\\"/g')",
  "source": "${source}"
}
EOF
mv "$tmp" "$OUT"
echo "wrote host hardware -> $OUT"
cat "$OUT"
