#!/bin/bash
# ═══════════════════════════════════════════════════════════════════
# LLM Gateway Benchmark Runner
#
# Two modes:
#   1) Custom mode:  ./run-all.sh <gateway> <url> <pid> custom <vus> [options]
#   2) Full suite:   ./run-all.sh <gateway> <url> <pid> full
#
# ═══════════════════════════════════════════════════════════════════
#
# Custom mode - run a single test with your own parameters:
#
#   ./run-all.sh <go|node|next> <gateway_url> <gateway_pid> custom <vus> \
#       [--duration 60s] [--stream 0.7] [--latency medium] \
#       [--error-rate 0] [--preset all] [--ramp] [--label my-test]
#
#   Examples:
#     # 10 users, default settings
#     ./run-all.sh go http://server:8080 12345 custom 10
#
#     # 1000 users, all streaming, realistic latency
#     ./run-all.sh go http://server:8080 12345 custom 1000 --stream 1.0 --latency realistic
#
#     # 10000 users ramp-up, 5% error rate
#     ./run-all.sh go http://server:8080 12345 custom 10000 --ramp --error-rate 0.05 --duration 300s
#
#     # Only long text preset
#     ./run-all.sh go http://server:8080 12345 custom 50 --preset long_text --latency slow
#
#
# Full suite mode - run all 10 predefined scenarios sequentially:
#
#   ./run-all.sh go http://server:8080 12345 full
#
# ═══════════════════════════════════════════════════════════════════

set -e

GATEWAY_TYPE=${1:?Usage: $0 <go|node|next> <gateway_url> <gateway_pid> <custom|full> [args...]}
GATEWAY_URL=${2:?Provide gateway URL}
GATEWAY_PID=${3:?Provide gateway PID}
MODE=${4:?Provide mode: custom or full}

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BENCHMARK_SCRIPT="$SCRIPT_DIR/benchmark.js"
RESULTS_BASE="$SCRIPT_DIR/../results/$GATEWAY_TYPE"

mkdir -p "$RESULTS_BASE"

# ── Health check ──────────────────────────────────────────
echo "Checking gateway health..."
curl -sf "$GATEWAY_URL/health" || { echo "Gateway not responding!"; exit 1; }
echo " OK"

# ── Run a single benchmark ────────────────────────────────
run_benchmark() {
  local LABEL=$1
  local VUS=$2
  local DURATION=$3
  local STREAM=$4
  local LATENCY=$5
  local ERROR_RATE=$6
  local PRESET=$7
  local RAMP=$8

  local RESULTS_DIR="$RESULTS_BASE/$LABEL"
  mkdir -p "$RESULTS_DIR"

  echo ""
  echo "╔═══════════════════════════════════════════════════════╗"
  echo "║  $LABEL"
  echo "║  VUS=$VUS  Duration=$DURATION  Stream=${STREAM}  Latency=$LATENCY"
  echo "║  ErrorRate=$ERROR_RATE  Preset=$PRESET  Ramp=$RAMP"
  echo "╚═══════════════════════════════════════════════════════╝"

  # Start resource monitoring
  if [ -f "$SCRIPT_DIR/../monitor/collect.sh" ] && [ -d "/proc/$GATEWAY_PID" ] 2>/dev/null; then
    bash "$SCRIPT_DIR/../monitor/collect.sh" "$GATEWAY_PID" "$RESULTS_DIR" 1 &
    MONITOR_PID=$!
  else
    MONITOR_PID=""
  fi

  # Run k6
  k6 run \
    -e GATEWAY_URL="$GATEWAY_URL" \
    -e VUS="$VUS" \
    -e DURATION="$DURATION" \
    -e STREAM_RATIO="$STREAM" \
    -e LATENCY="$LATENCY" \
    -e ERROR_RATE="$ERROR_RATE" \
    -e PRESET="$PRESET" \
    -e RAMP="$RAMP" \
    -e SUMMARY_FILE="$RESULTS_DIR/summary.json" \
    --summary-export="$RESULTS_DIR/k6-summary.json" \
    "$BENCHMARK_SCRIPT" 2>&1 | tee "$RESULTS_DIR/output.txt"

  # Stop monitoring
  if [ -n "$MONITOR_PID" ]; then
    kill "$MONITOR_PID" 2>/dev/null || true
    wait "$MONITOR_PID" 2>/dev/null || true
  fi

  # Save gateway metrics snapshot
  curl -s "$GATEWAY_URL/metrics" > "$RESULTS_DIR/gateway-metrics.json" 2>/dev/null || true

  echo "Results saved to: $RESULTS_DIR"
}

# ── Custom mode ───────────────────────────────────────────
if [ "$MODE" = "custom" ]; then
  VUS=${5:?Provide VUS count for custom mode}
  shift 5

  # Defaults
  DURATION="60s"
  STREAM="0.7"
  LATENCY="medium"
  ERROR_RATE="0"
  PRESET="all"
  RAMP="false"
  LABEL=""

  # Parse optional args
  while [ $# -gt 0 ]; do
    case "$1" in
      --duration)   DURATION="$2"; shift 2 ;;
      --stream)     STREAM="$2"; shift 2 ;;
      --latency)    LATENCY="$2"; shift 2 ;;
      --error-rate) ERROR_RATE="$2"; shift 2 ;;
      --preset)     PRESET="$2"; shift 2 ;;
      --ramp)       RAMP="true"; shift ;;
      --label)      LABEL="$2"; shift 2 ;;
      *) echo "Unknown option: $1"; exit 1 ;;
    esac
  done

  # Generate label if not provided
  if [ -z "$LABEL" ]; then
    LABEL="custom-${VUS}vus-$(date +%Y%m%d-%H%M%S)"
  fi

  # Warmup
  echo "Warming up (15s, 10 VUs)..."
  k6 run --quiet \
    -e GATEWAY_URL="$GATEWAY_URL" -e VUS=10 -e DURATION=15s \
    -e STREAM_RATIO="$STREAM" -e LATENCY="$LATENCY" \
    "$BENCHMARK_SCRIPT" > /dev/null 2>&1 || true
  echo "Warmup complete."

  run_benchmark "$LABEL" "$VUS" "$DURATION" "$STREAM" "$LATENCY" "$ERROR_RATE" "$PRESET" "$RAMP"

  echo ""
  echo "Done! Results: $RESULTS_BASE/$LABEL"
  exit 0
fi

# ── Full suite mode ───────────────────────────────────────
if [ "$MODE" = "full" ]; then

  # Warmup
  echo "Warming up (30s, 10 VUs)..."
  k6 run --quiet \
    -e GATEWAY_URL="$GATEWAY_URL" -e VUS=10 -e DURATION=30s \
    -e STREAM_RATIO=0 -e LATENCY=fast \
    "$BENCHMARK_SCRIPT" > /dev/null 2>&1 || true
  echo "Warmup complete."

  # Predefined scenarios
  # Format: label vus duration stream latency error_rate preset ramp
  SCENARIOS=(
    "s01-10vus-nonstream       10   60s   0.0  fast       0     all   false"
    "s02-100vus-nonstream      100  60s   0.0  fast       0     all   false"
    "s03-500vus-nonstream      500  60s   0.0  fast       0     all   false"
    "s04-10vus-stream          10   120s  1.0  medium     0     all   false"
    "s05-100vus-stream         100  120s  1.0  medium     0     all   false"
    "s06-500vus-stream         500  120s  1.0  medium     0     all   false"
    "s07-200vus-mixed          200  180s  0.7  realistic  0.05  all   false"
    "s08-100vus-high-error     100  120s  1.0  medium     0.20  all   false"
    "s09-50vus-long-text       50   180s  1.0  slow       0     long_text false"
    "s10-500vus-ramp           500  300s  1.0  medium     0.05  all   true"
  )

  for entry in "${SCENARIOS[@]}"; do
    read -r LABEL VUS DURATION STREAM LATENCY ERROR_RATE PRESET RAMP <<< "$entry"
    run_benchmark "$LABEL" "$VUS" "$DURATION" "$STREAM" "$LATENCY" "$ERROR_RATE" "$PRESET" "$RAMP"

    echo "Cooling down (30s)..."
    sleep 30
  done

  echo ""
  echo "============================================"
  echo "Full suite complete for $GATEWAY_TYPE"
  echo "Results: $RESULTS_BASE"
  echo "============================================"

  # Parse results
  python3 "$SCRIPT_DIR/../monitor/parse.py" "$SCRIPT_DIR/../results/" 2>/dev/null || echo "(parse.py not available or failed)"
  exit 0
fi

echo "Unknown mode: $MODE (use 'custom' or 'full')"
exit 1
