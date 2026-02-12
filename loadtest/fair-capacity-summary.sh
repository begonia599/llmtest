#!/bin/bash
# Summarize fair capacity benchmark manifest.
#
# Output focus:
# 1) Same target load -> which gateway is most stable
# 2) Same hardware -> which gateway supports highest stable VUs
#
# Usage:
#   ./fair-capacity-summary.sh <manifest.csv> [options]
#
# Example:
#   ./fair-capacity-summary.sh ../results/fair-20260212-120000/manifest.csv \
#     --target-vus 5000 \
#     --stable-repeat-ratio 0.8

set -euo pipefail

MANIFEST=${1:?Usage: $0 <manifest.csv> [options]}
shift || true

TARGET_VUS=5000
STABLE_REPEAT_RATIO=0.8

while [ $# -gt 0 ]; do
  case "$1" in
    --target-vus) TARGET_VUS="$2"; shift 2 ;;
    --stable-repeat-ratio) STABLE_REPEAT_RATIO="$2"; shift 2 ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

if [ ! -f "$MANIFEST" ]; then
  echo "Manifest not found: $MANIFEST"
  exit 1
fi

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

# Aggregate by gateway+vus:
# gateway,vus,total_runs,pass_runs,pass_rate,avg_rps_pass
awk -F',' '
NR==1 {next}
{
  gw=$3; vus=$4; pass=$18; rps=$11+0;
  key=gw","vus;
  total[key]++;
  if (pass==1) {
    passed[key]++;
    rps_sum[key]+=rps;
  }
}
END {
  for (k in total) {
    p=(k in passed)?passed[k]:0;
    rate=(p*100.0)/total[k];
    avg=(p>0)?(rps_sum[k]/p):0;
    printf "%s,%d,%d,%.2f,%.2f\n", k, total[k], p, rate, avg;
  }
}' "$MANIFEST" | sort -t',' -k1,1 -k2,2n > "$TMP"

echo ""
echo "============================================================"
echo "Fair Capacity Summary"
echo "Manifest: $MANIFEST"
echo "Target VUS: $TARGET_VUS"
echo "Stable repeat ratio threshold: $STABLE_REPEAT_RATIO"
echo "============================================================"
echo ""

echo "Per-load pass rates:"
printf "%-8s %-8s %-10s %-10s %-10s %-12s\n" "Gateway" "VUS" "Runs" "Passes" "PassRate%" "AvgRPS(pass)"
awk -F',' '{printf "%-8s %-8s %-10s %-10s %-10s %-12s\n",$1,$2,$3,$4,$5,$6}' "$TMP"

echo ""
echo "Decision table:"
printf "%-8s %-16s %-16s %-20s\n" "Gateway" "max_stable_vus" "max_stable_rps" "pass_rate_at_target(%)"

for gw in go node next; do
  max_vus=0
  max_rps=0
  target_rate=0

  while IFS=',' read -r cgw vus runs passes pass_rate avg_rps; do
    [ "$cgw" != "$gw" ] && continue

    if [ "$vus" -eq "$TARGET_VUS" ]; then
      target_rate="$pass_rate"
    fi

    is_stable=$(awk -v rate="$pass_rate" -v thr="$STABLE_REPEAT_RATIO" 'BEGIN {print (rate/100 >= thr) ? 1 : 0}')
    if [ "$is_stable" -eq 1 ]; then
      if [ "$vus" -gt "$max_vus" ]; then
        max_vus="$vus"
        max_rps="$avg_rps"
      fi
    fi
  done < "$TMP"

  printf "%-8s %-16s %-16s %-20s\n" "$gw" "$max_vus" "$max_rps" "$target_rate"
done

echo ""
echo "Interpretation:"
echo "- Most stable at target load: highest pass_rate_at_target(%)"
echo "- Highest capacity on same hardware: highest max_stable_vus"
