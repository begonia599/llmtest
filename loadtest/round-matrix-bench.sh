#!/bin/bash
# Run benchmark rounds sequentially (e.g. rounds 7-10) with one command.
#
# Usage:
#   ./round-matrix-bench.sh <server_b_ip> [options]
#
# Example:
#   ./round-matrix-bench.sh 152.53.164.118 --rounds 7-10

set -euo pipefail

SERVER_B=${1:?Usage: $0 <server_b_ip> [options]}
shift || true

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FAIR_BENCH="$SCRIPT_DIR/fair-capacity-bench.sh"
FAIR_SUMMARY="$SCRIPT_DIR/fair-capacity-summary.sh"
RESULTS_ROOT="$SCRIPT_DIR/../results"
BATCH_ID="round-batch-$(date +%Y%m%d-%H%M%S)"
BATCH_DIR="$RESULTS_ROOT/$BATCH_ID"
INDEX_CSV="$BATCH_DIR/index.csv"

ROUNDS_EXPR="7-10"
STABLE_REPEAT_RATIO="0.8"
SLEEP_BETWEEN=30
STOP_ON_FAIL=0
DRY_RUN=0

# Global overrides (optional)
O_GW=""
O_VUS_LIST=""
O_REPEATS=""
O_CREDS=""
O_DURATION=""
O_STREAM=""
O_LATENCY=""
O_ERROR_RATE=""
O_PRESET=""
O_TARGET_VUS=""
O_COOLDOWN=""
O_SSH_USER=""
O_CPU_SET=""
O_GO_MAXPROCS=""
O_SUCCESS_MIN=""
O_P95_MAX_MS=""
O_INTERRUPTED_MAX_RATIO=""

usage() {
  cat <<EOF
Usage: $0 <server_b_ip> [options]

Options:
  --rounds <expr>                  Rounds to run, e.g. 7-10 or 7,8,10
  --sleep-between <seconds>        Sleep between rounds (default: 30)
  --stable-repeat-ratio <ratio>    Summary stable ratio (default: 0.8)
  --stop-on-fail                   Stop batch when one round fails
  --dry-run                        Print commands only

  # Optional global overrides (applied to every selected round)
  --gateways <go,node,next>
  --vus-list <csv>
  --repeats <n>
  --creds <n>
  --duration <120s>
  --stream <0.5>
  --latency <fast|medium|slow|realistic>
  --error-rate <0.0>
  --preset <all|long_text|...>
  --target-vus <n>
  --cooldown <seconds>
  --ssh-user <root>
  --cpu-set <0,1>
  --go-maxprocs <n>
  --success-min <percent>
  --p95-max-ms <ms>
  --interrupted-max-ratio <percent>
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --rounds) ROUNDS_EXPR="$2"; shift 2 ;;
    --sleep-between) SLEEP_BETWEEN="$2"; shift 2 ;;
    --stable-repeat-ratio) STABLE_REPEAT_RATIO="$2"; shift 2 ;;
    --stop-on-fail) STOP_ON_FAIL=1; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    --gateways) O_GW="$2"; shift 2 ;;
    --vus-list) O_VUS_LIST="$2"; shift 2 ;;
    --repeats) O_REPEATS="$2"; shift 2 ;;
    --creds) O_CREDS="$2"; shift 2 ;;
    --duration) O_DURATION="$2"; shift 2 ;;
    --stream) O_STREAM="$2"; shift 2 ;;
    --latency) O_LATENCY="$2"; shift 2 ;;
    --error-rate) O_ERROR_RATE="$2"; shift 2 ;;
    --preset) O_PRESET="$2"; shift 2 ;;
    --target-vus) O_TARGET_VUS="$2"; shift 2 ;;
    --cooldown) O_COOLDOWN="$2"; shift 2 ;;
    --ssh-user) O_SSH_USER="$2"; shift 2 ;;
    --cpu-set) O_CPU_SET="$2"; shift 2 ;;
    --go-maxprocs) O_GO_MAXPROCS="$2"; shift 2 ;;
    --success-min) O_SUCCESS_MIN="$2"; shift 2 ;;
    --p95-max-ms) O_P95_MAX_MS="$2"; shift 2 ;;
    --interrupted-max-ratio) O_INTERRUPTED_MAX_RATIO="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *)
      echo "Unknown option: $1"
      usage
      exit 1
      ;;
  esac
done

if [ ! -x "$FAIR_BENCH" ]; then
  echo "Missing or not executable: $FAIR_BENCH"
  echo "Try: chmod +x $FAIR_BENCH"
  exit 1
fi
if [ ! -x "$FAIR_SUMMARY" ]; then
  echo "Missing or not executable: $FAIR_SUMMARY"
  echo "Try: chmod +x $FAIR_SUMMARY"
  exit 1
fi

expand_rounds() {
  local expr="$1"
  local -a parts out
  local item start end r
  IFS=',' read -r -a parts <<< "$expr"
  for item in "${parts[@]}"; do
    if [[ "$item" =~ ^([0-9]+)-([0-9]+)$ ]]; then
      start="${BASH_REMATCH[1]}"
      end="${BASH_REMATCH[2]}"
      if [ "$start" -gt "$end" ]; then
        echo "Invalid round range: $item" >&2
        return 1
      fi
      for r in $(seq "$start" "$end"); do
        out+=("$r")
      done
    elif [[ "$item" =~ ^[0-9]+$ ]]; then
      out+=("$item")
    else
      echo "Invalid round token: $item" >&2
      return 1
    fi
  done
  printf "%s\n" "${out[@]}" | awk '!seen[$0]++'
}

set_round_defaults() {
  local round="$1"
  case "$round" in
    7)
      GW="go,node,next"
      VUS_LIST="300,500,700,900,1100,1300"
      REPEATS="5"
      CREDS="1000"
      DURATION="120s"
      STREAM="0.5"
      LATENCY="medium"
      ERROR_RATE="0"
      PRESET="all"
      TARGET_VUS="1300"
      COOLDOWN="20"
      SSH_USER="root"
      CPU_SET="0,1"
      GO_MAXPROCS="2"
      SUCCESS_MIN="99.5"
      P95_MAX_MS="15000"
      INTERRUPTED_MAX_RATIO="1.0"
      ;;
    8)
      GW="go,node,next"
      VUS_LIST="300,500,700,900,1100,1300"
      REPEATS="5"
      CREDS="1000"
      DURATION="120s"
      STREAM="1.0"
      LATENCY="medium"
      ERROR_RATE="0"
      PRESET="all"
      TARGET_VUS="1300"
      COOLDOWN="20"
      SSH_USER="root"
      CPU_SET="0,1"
      GO_MAXPROCS="2"
      SUCCESS_MIN="99.5"
      P95_MAX_MS="15000"
      INTERRUPTED_MAX_RATIO="1.0"
      ;;
    9)
      GW="go,node,next"
      VUS_LIST="300,500,700,900,1100,1300"
      REPEATS="5"
      CREDS="1000"
      DURATION="120s"
      STREAM="0.0"
      LATENCY="medium"
      ERROR_RATE="0"
      PRESET="all"
      TARGET_VUS="1300"
      COOLDOWN="20"
      SSH_USER="root"
      CPU_SET="0,1"
      GO_MAXPROCS="2"
      SUCCESS_MIN="99.5"
      P95_MAX_MS="15000"
      INTERRUPTED_MAX_RATIO="1.0"
      ;;
    10)
      GW="go,node,next"
      VUS_LIST="1000,1500,2000,2500,3000,3500,4000,4500,5000"
      REPEATS="5"
      CREDS="1000"
      DURATION="120s"
      STREAM="0.5"
      LATENCY="medium"
      ERROR_RATE="0"
      PRESET="all"
      TARGET_VUS="5000"
      COOLDOWN="20"
      SSH_USER="root"
      CPU_SET="0,1"
      GO_MAXPROCS="2"
      SUCCESS_MIN="99.5"
      P95_MAX_MS="15000"
      INTERRUPTED_MAX_RATIO="1.0"
      ;;
    *)
      echo "Round $round is not predefined. Supported: 7,8,9,10"
      return 1
      ;;
  esac
}

apply_global_overrides() {
  [ -n "$O_GW" ] && GW="$O_GW"
  [ -n "$O_VUS_LIST" ] && VUS_LIST="$O_VUS_LIST"
  [ -n "$O_REPEATS" ] && REPEATS="$O_REPEATS"
  [ -n "$O_CREDS" ] && CREDS="$O_CREDS"
  [ -n "$O_DURATION" ] && DURATION="$O_DURATION"
  [ -n "$O_STREAM" ] && STREAM="$O_STREAM"
  [ -n "$O_LATENCY" ] && LATENCY="$O_LATENCY"
  [ -n "$O_ERROR_RATE" ] && ERROR_RATE="$O_ERROR_RATE"
  [ -n "$O_PRESET" ] && PRESET="$O_PRESET"
  [ -n "$O_TARGET_VUS" ] && TARGET_VUS="$O_TARGET_VUS"
  [ -n "$O_COOLDOWN" ] && COOLDOWN="$O_COOLDOWN"
  [ -n "$O_SSH_USER" ] && SSH_USER="$O_SSH_USER"
  [ -n "$O_CPU_SET" ] && CPU_SET="$O_CPU_SET"
  [ -n "$O_GO_MAXPROCS" ] && GO_MAXPROCS="$O_GO_MAXPROCS"
  [ -n "$O_SUCCESS_MIN" ] && SUCCESS_MIN="$O_SUCCESS_MIN"
  [ -n "$O_P95_MAX_MS" ] && P95_MAX_MS="$O_P95_MAX_MS"
  [ -n "$O_INTERRUPTED_MAX_RATIO" ] && INTERRUPTED_MAX_RATIO="$O_INTERRUPTED_MAX_RATIO"
}

mkdir -p "$BATCH_DIR"
echo "round,exit_code,manifest,summary_file" > "$INDEX_CSV"

mapfile -t ROUND_LIST < <(expand_rounds "$ROUNDS_EXPR")

echo "============================================================"
echo "Round Batch Runner"
echo "Batch ID:   $BATCH_ID"
echo "Server B:   $SERVER_B"
echo "Rounds:     ${ROUND_LIST[*]}"
echo "Batch dir:  $BATCH_DIR"
echo "============================================================"
echo ""

for round in "${ROUND_LIST[@]}"; do
  set_round_defaults "$round"
  apply_global_overrides

  round_log="$BATCH_DIR/round${round}.log"
  summary_file="$BATCH_DIR/round${round}-summary.txt"

  echo ">>> Round $round start"
  echo "    vus=$VUS_LIST repeats=$REPEATS stream=$STREAM duration=$DURATION target_vus=$TARGET_VUS cpu_set=${CPU_SET:-all}"

  cmd=(
    "$FAIR_BENCH" "$SERVER_B"
    --vus-list "$VUS_LIST"
    --repeats "$REPEATS"
    --gateways "$GW"
    --target-vus "$TARGET_VUS"
    --creds "$CREDS"
    --duration "$DURATION"
    --stream "$STREAM"
    --latency "$LATENCY"
    --error-rate "$ERROR_RATE"
    --preset "$PRESET"
    --cooldown "$COOLDOWN"
    --ssh-user "$SSH_USER"
    --success-min "$SUCCESS_MIN"
    --p95-max-ms "$P95_MAX_MS"
    --interrupted-max-ratio "$INTERRUPTED_MAX_RATIO"
  )
  if [ -n "$CPU_SET" ]; then
    cmd+=( --cpu-set "$CPU_SET" )
  fi
  if [ -n "$GO_MAXPROCS" ]; then
    cmd+=( --go-maxprocs "$GO_MAXPROCS" )
  fi

  if [ "$DRY_RUN" -eq 1 ]; then
    printf 'DRY RUN: '; printf '%q ' "${cmd[@]}"; echo ""
    echo "$round,0,,(dry-run)" >> "$INDEX_CSV"
    continue
  fi

  set +e
  "${cmd[@]}" 2>&1 | tee "$round_log"
  exit_code=${PIPESTATUS[0]}
  set -e

  manifest=$(grep -E '^Manifest:[[:space:]]+' "$round_log" | tail -1 | awk '{print $2}')
  if [ -z "${manifest:-}" ] || [ ! -f "$manifest" ]; then
    manifest=""
  fi

  if [ "$exit_code" -eq 0 ] && [ -n "$manifest" ]; then
    "$FAIR_SUMMARY" "$manifest" --target-vus "$TARGET_VUS" --stable-repeat-ratio "$STABLE_REPEAT_RATIO" | tee "$summary_file"
  else
    echo "Round $round failed or manifest missing, skip summary."
    : > "$summary_file"
  fi

  echo "$round,$exit_code,$manifest,$summary_file" >> "$INDEX_CSV"
  echo "<<< Round $round end (exit=$exit_code)"
  echo ""

  if [ "$exit_code" -ne 0 ] && [ "$STOP_ON_FAIL" -eq 1 ]; then
    echo "Stopping batch because --stop-on-fail is enabled."
    break
  fi

  if [ "$SLEEP_BETWEEN" -gt 0 ]; then
    sleep "$SLEEP_BETWEEN"
  fi
done

echo "Batch finished."
echo "Index: $INDEX_CSV"
