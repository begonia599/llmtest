#!/bin/bash
# Summarize fair capacity benchmark manifest.
#
# Output focus:
# 1) Same target load -> which gateway is most stable
# 2) Same hardware -> which gateway supports highest stable VUs
# 3) Hardware utilization from existing resources.csv (no rerun needed)
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

TMP_PASS=$(mktemp)
TMP_PERF=$(mktemp)
TMP_HW_RAW=$(mktemp)
TMP_HW=$(mktemp)
trap 'rm -f "$TMP_PASS" "$TMP_PERF" "$TMP_HW_RAW" "$TMP_HW"' EXIT

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
    rps_pass_sum[key]+=rps;
  }
}
END {
  for (k in total) {
    p=(k in passed)?passed[k]:0;
    rate=(p*100.0)/total[k];
    avg=(p>0)?(rps_pass_sum[k]/p):0;
    printf "%s,%d,%d,%.2f,%.2f\n", k, total[k], p, rate, avg;
  }
}' "$MANIFEST" | sort -t',' -k1,1 -k2,2n > "$TMP_PASS"

# Aggregate all-run performance by gateway+vus:
# gateway,vus,runs,avg_rps,avg_p95,avg_success,avg_interrupted
awk -F',' '
NR==1 {next}
{
  gw=$3; vus=$4; rps=$11+0; p95=$13+0; succ=$14+0; intr=$17+0;
  key=gw","vus;
  n[key]++;
  rps_sum[key]+=rps;
  p95_sum[key]+=p95;
  succ_sum[key]+=succ;
  intr_sum[key]+=intr;
}
END {
  for (k in n) {
    printf "%s,%d,%.2f,%.2f,%.2f,%.2f\n",
      k, n[k], rps_sum[k]/n[k], p95_sum[k]/n[k], succ_sum[k]/n[k], intr_sum[k]/n[k];
  }
}' "$MANIFEST" | sort -t',' -k1,1 -k2,2n > "$TMP_PERF"

# Build per-run hardware snapshots from existing resources.csv:
# gw,vus,cpu_server_avg,cpu_server_peak,cpu_core_peak,rss_peak,fd_peak,threads_peak
while IFS=',' read -r run_id server_ip gw vus rep order_idx start_time end_time exit_code result_dir rps total_requests p95 success complete interrupted intr_ratio pass; do
  if [ "$run_id" = "run_id" ]; then
    continue
  fi
  if [ -z "${result_dir:-}" ]; then
    continue
  fi

  resources_file="$result_dir/resources.csv"
  if [ ! -f "$resources_file" ]; then
    continue
  fi

  metrics=$(awk -F',' '
  NR==1 {next}
  {
    n++;
    cpu_avg_sum += $2;
    if ($2 > cpu_peak) cpu_peak = $2;
    if ($3 > core_peak) core_peak = $3;
    if ($4 > rss_peak) rss_peak = $4;
    if ($6 > fd_peak) fd_peak = $6;
    if ($7 > th_peak) th_peak = $7;
  }
  END {
    if (n > 0) {
      printf "%.2f,%.2f,%.2f,%.2f,%.0f,%.0f", cpu_avg_sum/n, cpu_peak, core_peak, rss_peak, fd_peak, th_peak;
    }
  }' "$resources_file")

  if [ -n "$metrics" ]; then
    echo "$gw,$vus,$metrics" >> "$TMP_HW_RAW"
  fi
done < "$MANIFEST"

# Aggregate hardware by gateway+vus:
# gateway,vus,runs,cpu_server_avg,cpu_server_peak,cpu_core_peak,rss_peak,fd_peak,threads_peak
if [ -s "$TMP_HW_RAW" ]; then
  awk -F',' '
  {
    key=$1","$2;
    n[key]++;
    cpu_avg_sum[key]+=$3;
    cpu_peak_sum[key]+=$4;
    core_peak_sum[key]+=$5;
    rss_peak_sum[key]+=$6;
    fd_peak_sum[key]+=$7;
    th_peak_sum[key]+=$8;
  }
  END {
    for (k in n) {
      printf "%s,%d,%.2f,%.2f,%.2f,%.2f,%.0f,%.0f\n",
        k, n[k],
        cpu_avg_sum[k]/n[k],
        cpu_peak_sum[k]/n[k],
        core_peak_sum[k]/n[k],
        rss_peak_sum[k]/n[k],
        fd_peak_sum[k]/n[k],
        th_peak_sum[k]/n[k];
    }
  }' "$TMP_HW_RAW" | sort -t',' -k1,1 -k2,2n > "$TMP_HW"
fi

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
awk -F',' '{printf "%-8s %-8s %-10s %-10s %-10s %-12s\n",$1,$2,$3,$4,$5,$6}' "$TMP_PASS"

echo ""
echo "Per-load performance (avg all runs):"
printf "%-8s %-8s %-10s %-10s %-10s %-10s %-10s\n" "Gateway" "VUS" "Runs" "AvgRPS" "AvgP95ms" "AvgSucc%" "AvgIntr%"
awk -F',' '{printf "%-8s %-8s %-10s %-10s %-10s %-10s %-10s\n",$1,$2,$3,$4,$5,$6,$7}' "$TMP_PERF"

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
  done < "$TMP_PASS"

  printf "%-8s %-16s %-16s %-20s\n" "$gw" "$max_vus" "$max_rps" "$target_rate"
done

echo ""
if [ -s "$TMP_HW" ]; then
  echo "Per-load hardware utilization (avg across runs):"
  printf "%-8s %-8s %-8s %-12s %-12s %-12s %-12s %-10s %-10s %-12s %-12s\n" \
    "Gateway" "VUS" "Runs" "CPUavg(%)" "CPUpeak(%)" "CorePeak(%)" "MemPeakMB" "FDPeak" "ThrPeak" "RPS/CPU%" "RPS/MB"

  while IFS=',' read -r gw vus runs cpu_avg cpu_peak core_peak rss_peak fd_peak th_peak; do
    avg_rps=$(awk -F',' -v g="$gw" -v v="$vus" '$1==g && $2==v {print $4; exit}' "$TMP_PERF")
    [ -z "$avg_rps" ] && avg_rps="0"
    rps_per_cpu=$(awk -v r="$avg_rps" -v c="$cpu_peak" 'BEGIN { if (c>0) printf "%.2f", r/c; else printf "0.00"; }')
    rps_per_mb=$(awk -v r="$avg_rps" -v m="$rss_peak" 'BEGIN { if (m>0) printf "%.3f", r/m; else printf "0.000"; }')

    printf "%-8s %-8s %-8s %-12s %-12s %-12s %-12s %-10s %-10s %-12s %-12s\n" \
      "$gw" "$vus" "$runs" "$cpu_avg" "$cpu_peak" "$core_peak" "$rss_peak" "$fd_peak" "$th_peak" "$rps_per_cpu" "$rps_per_mb"
  done < "$TMP_HW"
else
  echo "No resources.csv found via result_dir in manifest; hardware table skipped."
fi

echo ""
echo "Interpretation:"
echo "- Most stable at target load: highest pass_rate_at_target(%)"
echo "- Highest capacity on same hardware: highest max_stable_vus"
echo "- Hardware table uses existing resources.csv under each result_dir (no rerun needed)"
