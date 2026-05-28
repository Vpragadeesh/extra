#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: ./convert_to_h265_cuda.sh <input_video> [output_video]

Converts a video to H.265 using NVIDIA CUDA/NVENC and shows a progress bar.

Examples:
  ./convert_to_h265_cuda.sh "Scam 2003 The Telgi Story Complete.mkv"
  ./convert_to_h265_cuda.sh "input.mkv" "output_h265.mkv"
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ $# -lt 1 || $# -gt 2 ]]; then
  usage >&2
  exit 1
fi

input="$1"
if [[ ! -f "$input" ]]; then
  echo "Input file not found: $input" >&2
  exit 1
fi

output="${2:-${input%.*}_h265.mkv}"
if [[ "$output" == "$input" ]]; then
  echo "Output file must be different from input file." >&2
  exit 1
fi

for cmd in ffmpeg ffprobe awk; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing dependency: $cmd" >&2
    exit 1
  fi
done

if ! ffmpeg -hide_banner -h encoder=hevc_nvenc >/dev/null 2>&1; then
  echo "ffmpeg does not have hevc_nvenc support on this system." >&2
  exit 1
fi

duration_secs="$(ffprobe -v error -show_entries format=duration -of default=nokey=1:noprint_wrappers=1 "$input" || true)"
if [[ -z "$duration_secs" || "$duration_secs" == "N/A" ]]; then
  echo "Could not read input duration with ffprobe." >&2
  exit 1
fi

render_progress() {
  local percent="$1"
  local speed="$2"
  local elapsed="$3"
  local width=40
  local filled=$((percent * width / 100))
  local empty=$((width - filled))
  local left right

  left="$(printf '%*s' "$filled" '' | tr ' ' '#')"
  right="$(printf '%*s' "$empty" '' | tr ' ' '-')"
  printf '\r[%s%s] %3d%%  elapsed=%s  speed=%s' "$left" "$right" "$percent" "$elapsed" "$speed"
}

to_hms() {
  local seconds="$1"
  awk -v s="$seconds" 'BEGIN {
    h=int(s/3600);
    m=int((s-h*3600)/60);
    sec=int(s-h*3600-m*60);
    printf "%02d:%02d:%02d", h, m, sec;
  }'
}

echo "Input:  $input"
echo "Output: $output"
echo "Codec:  hevc_nvenc (H.265)"

set +e
ffmpeg -hide_banner -loglevel error -y \
  -hwaccel cuda \
  -i "$input" \
  -map 0 \
  -c:v hevc_nvenc \
  -preset fast \
  -rc vbr \
  -cq 23 \
  -b:v 0 \
  -c:a copy \
  -c:s copy \
  -progress pipe:1 \
  -nostats \
  "$output" | while IFS='=' read -r key value; do
    case "$key" in
      out_time)
        current_secs="$(awk -v t="$value" 'BEGIN {
          split(t, a, ":");
          if (length(a) == 3) {
            printf "%.3f", (a[1] * 3600) + (a[2] * 60) + a[3];
          } else {
            print "0";
          }
        }')"
        percent="$(awk -v c="$current_secs" -v d="$duration_secs" 'BEGIN {
          if (d > 0) {
            p = int((c / d) * 100);
            if (p > 100) p = 100;
            print p;
          } else {
            print 0;
          }
        }')"
        elapsed="$(to_hms "$current_secs")"
        render_progress "$percent" "${speed:-0x}" "$elapsed"
        ;;
      speed)
        speed="$value"
        ;;
      progress)
        if [[ "$value" == "end" ]]; then
          render_progress 100 "${speed:-0x}" "$(to_hms "$duration_secs")"
        fi
        ;;
    esac
  done
ff_status=${PIPESTATUS[0]}
set -e

printf '\n'
if [[ $ff_status -ne 0 ]]; then
  echo "Conversion failed." >&2
  exit "$ff_status"
fi

echo "Done: $output"
