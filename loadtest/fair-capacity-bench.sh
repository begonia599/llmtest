#!/bin/bash
# Fair capacity benchmark orchestrator (no one-sided tuning assumptions).
#
# Purpose:
# - Run Go/Node/Next with identical parameters
# - Randomize gateway execution order per round
# - Repeat each load point multiple times
# - Persist pass/fail judgments with fixed criteria
#
# Usage:
#   ./fair-capacity-bench.sh <server_b_ip> [options]
#
# Example:
#   ./fair-capacity-bench.sh 152.53.164.118 \
#     --vus-list 1000,2000,3000,4000,5000 \
#     --repeats 5 \
#     --creds 1000 \
#     --duration 120s \
#     --stream 0.5 \
#     --target-vus 5000

set -euo pipefail

SERVER_B=${1:?Usage: $0 <server_b_ip> [options]}
shift || true

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
AUTO_BENCH="$SCRIPT_DIR/auto-bench.sh"
RESULTS_ROOT="$SCRIPT_DIR/../results"
RUN_ID="fair-$(date +%Y%m%d-%H%M%S)"
RUN_DIR="$RESULTS_ROOT/$RUN_ID"
MANIFEST="$RUN_DIR/manifest.csv"

# Test matrix defaults
VUS_LIST="1000,1500,2000,2500,3000,3500,4000,4500,5000"
REPEATS=5
GATEWAYS="go,node,next"
TARGET_VUS=5000

# Shared benchmark defaults (same for all gateways)
CREDS=1000
DURATION="120s"
STREAM="0.5"
LATENCY="medium"
ERROR_RATE="0"
PRESET="all"
COOLDOWN=20
SSH_USER="root"
CPU_SET=""
GO_MAXPROCS=""

# Fixed pass criteria
SUCCESS_MIN="99.5"
P95_MAX_MS="15000"
INTERRUPTED_MAX_RATIO="1.0"

while [ $# -gt 0 ]; do
  case "$1" in
    --vus-list) VUS_LIST="$2"; shift 2 ;;
    --repeats) REPEATS="$2"; shift 2 ;;
    --gateways) GATEWAYS="$2"; shift 2 ;;
    --target-vus) TARGET_VUS="$2"; shift 2 ;;
    --creds) CREDS="$2"; shift 2 ;;
    --duration) DURATION="$2"; shift 2 ;;
    --stream) STREAM="$2"; shift 2 ;;
    --latency) LATENCY="$2"; shift 2 ;;
    --error-rate) ERROR_RATE="$2"; shift 2 ;;
    --preset) PRESET="$2"; shift 2 ;;
    --cooldown) COOLDOWN="$2"; shift 2 ;;
    --ssh-user) SSH_USER="$2"; shift 2 ;;
    --cpu-set) CPU_SET="$2"; shift 2 ;;
    --go-maxprocs) GO_MAXPROCS="$2"; shift 2 ;;
    --success-min) SUCCESS_MIN="$2"; shift 2 ;;
    --p95-max-ms) P95_MAX_MS="$2"; shift 2 ;;
    --interrupted-max-ratio) INTERRUPTED_MAX_RATIO="$2"; shift 2 ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

if [ ! -x "$AUTO_BENCH" ]; then
  echo "auto-bench.sh not found or not executable: $AUTO_BENCH"
  exit 1
fi

mkdir -p "$RUN_DIR"

cat > "$MANIFEST" <<EOF
run_id,server_ip,gateway,vus,repeat,order_index,start_time,end_time,exit_code,result_dir,rps,total_requests,latency_p95_ms,success_rate,complete_iterations,interrupted_iterations,interrupted_ratio,pass
EOF

IFS=',' read -r -a VUS_ARR <<< "$VUS_LIST"
IFS=',' read -r -a GW_ARR <<< "$GATEWAYS"

shuffle_gateways() {
  if command -v shuf >/dev/null 2>&1; then
    printf "%s\n" "$@" | shuf
  else
    # Fallback shuffle
    printf "%s\n" "$@" | awk 'BEGIN {srand()} {print rand(), $0}' | sort -k1,1n | cut -d' ' -f2-
  fi
}

extract_metric() {
  local file="$1"
  local label="$2"
  grep -E "$label" "$file" | tail -1
}

echo "===================================================================="
echo "Fair Capacity Benchmark Run"
echo "Run ID:        $RUN_ID"
echo "Server B:      $SERVER_B"
echo "Gateways:      $GATEWAYS"
echo "VUs list:      $VUS_LIST"
echo "Repeats:       $REPEATS"
echo "Shared config: creds=$CREDS duration=$DURATION stream=$STREAM latency=$LATENCY error=$ERROR_RATE preset=$PRESET"
echo "CPU config:    cpu_set=${CPU_SET:-all} go_maxprocs=${GO_MAXPROCS:-default}"
echo "Pass criteria: success>=$SUCCESS_MIN%, p95<=$P95_MAX_MS ms, interrupted_ratio<=$INTERRUPTED_MAX_RATIO%"
echo "Manifest:      $MANIFEST"
echo "===================================================================="
echo ""

for vus in "${VUS_ARR[@]}"; do
  for rep in $(seq 1 "$REPEATS"); do
    mapfile -t ORDERED_GW < <(shuffle_gateways "${GW_ARR[@]}")

    echo "[VUS=$vus][repeat=$rep] order: ${ORDERED_GW[*]}"

    idx=0
    for gw in "${ORDERED_GW[@]}"; do
      idx=$((idx + 1))
      start_ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
      log_file="$RUN_DIR/run-${gw}-${vus}-r${rep}-$(date +%Y%m%d-%H%M%S).log"

      echo "  -> [$gw] start (order=$idx)"

      cmd=(
        "$AUTO_BENCH" "$SERVER_B" "$vus"
        --creds "$CREDS"
        --duration "$DURATION"
        --stream "$STREAM"
        --latency "$LATENCY"
        --error-rate "$ERROR_RATE"
        --preset "$PRESET"
        --only "$gw"
        --cooldown "$COOLDOWN"
        --ssh-user "$SSH_USER"
      )
      if [ -n "$CPU_SET" ]; then
        cmd+=( --cpu-set "$CPU_SET" )
      fi
      if [ -n "$GO_MAXPROCS" ]; then
        cmd+=( --go-maxprocs "$GO_MAXPROCS" )
      fi

      set +e
      "${cmd[@]}" 2>&1 | tee "$log_file"
      exit_code=${PIPESTATUS[0]}
      set -e

      end_ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)

      # Pick newest matching result directory.
      result_dir=$(ls -dt "$RESULTS_ROOT/$gw/${gw}-${vus}vus-"* 2>/dev/null | head -1 || true)

      rps=""
      total_requests=""
      p95=""
      success=""
      complete_iters=""
      interrupted_iters=""
      interrupted_ratio=""
      pass="0"

      if [ -n "$result_dir" ] && [ -f "$result_dir/output.txt" ]; then
        output="$result_dir/output.txt"

        rps=$(extract_metric "$output" "RPS:" | awk '{print $2}')
        total_requests=$(extract_metric "$output" "Total requests:" | awk '{print $3}')
        p95=$(extract_metric "$output" "Latency P95:" | awk '{print $3}')
        success=$(extract_metric "$output" "Success rate:" | awk '{gsub("%","",$3); print $3}')

        run_line=$(grep -E "interrupted iterations" "$output" | tail -1 || true)
        if [ -n "$run_line" ]; then
          complete_iters=$(echo "$run_line" | awk '{for(i=1;i<=NF;i++) if($i=="complete"){print $(i-1); break}}')
          interrupted_iters=$(echo "$run_line" | awk '{for(i=1;i<=NF;i++) if($i=="interrupted"){print $(i-1); break}}')
        fi

        if [ -z "${complete_iters:-}" ]; then complete_iters="0"; fi
        if [ -z "${interrupted_iters:-}" ]; then interrupted_iters="0"; fi

        interrupted_ratio=$(awk -v c="$complete_iters" -v i="$interrupted_iters" 'BEGIN {
          total=c+i;
          if (total <= 0) { printf "%.2f", 100; }
          else { printf "%.2f", (i*100)/total; }
        }')

        ok_success=$(awk -v v="$success" -v m="$SUCCESS_MIN" 'BEGIN {print (v+0 >= m+0) ? 1 : 0}')
        ok_p95=$(awk -v v="$p95" -v m="$P95_MAX_MS" 'BEGIN {print (v+0 <= m+0) ? 1 : 0}')
        ok_interrupt=$(awk -v v="$interrupted_ratio" -v m="$INTERRUPTED_MAX_RATIO" 'BEGIN {print (v+0 <= m+0) ? 1 : 0}')

        if [ "$exit_code" -eq 0 ] && [ "$ok_success" -eq 1 ] && [ "$ok_p95" -eq 1 ] && [ "$ok_interrupt" -eq 1 ]; then
          pass="1"
        fi
      fi

      echo "  <- [$gw] done exit=$exit_code pass=$pass rps=${rps:-NA} p95=${p95:-NA} success=${success:-NA}% interrupted_ratio=${interrupted_ratio:-NA}%"
      echo "${RUN_ID},${SERVER_B},${gw},${vus},${rep},${idx},${start_ts},${end_ts},${exit_code},${result_dir},${rps},${total_requests},${p95},${success},${complete_iters},${interrupted_iters},${interrupted_ratio},${pass}" >> "$MANIFEST"
    done
  done
done

echo ""
echo "Run completed."
echo "Manifest: $MANIFEST"
echo ""
echo "Next step:"
echo "  $SCRIPT_DIR/fair-capacity-summary.sh $MANIFEST --target-vus $TARGET_VUS"
