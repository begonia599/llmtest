#!/bin/bash
# Resource monitoring script for the gateway process
# Usage: ./collect.sh <pid> <output_dir> [interval_seconds]
#
# Collects: CPU% (normalized to total server), RSS (MB), FD count, every N seconds
# Output: CSV file at <output_dir>/resources.csv

set -e

PID=$1
OUTPUT_DIR=$2
INTERVAL=${3:-1}

if [ -z "$PID" ] || [ -z "$OUTPUT_DIR" ]; then
  echo "Usage: $0 <pid> <output_dir> [interval_seconds]"
  exit 1
fi

# Detect CPU core count
NUM_CORES=$(nproc 2>/dev/null || grep -c ^processor /proc/cpuinfo 2>/dev/null || echo 1)

mkdir -p "$OUTPUT_DIR"
CSV="$OUTPUT_DIR/resources.csv"

echo "timestamp,cpu_percent,cpu_cores_percent,rss_mb,vms_mb,fd_count,threads,num_cores" > "$CSV"

echo "Monitoring PID $PID every ${INTERVAL}s -> $CSV"
echo "CPU cores: $NUM_CORES"
echo "Press Ctrl+C to stop"

while kill -0 "$PID" 2>/dev/null; do
  TIMESTAMP=$(date +%s%3N)

  # CPU and memory from ps
  PS_OUTPUT=$(ps -p "$PID" -o %cpu=,rss=,vsz=,nlwp= 2>/dev/null || echo "0 0 0 0")
  CPU_CORES=$(echo "$PS_OUTPUT" | awk '{print $1}')
  RSS_KB=$(echo "$PS_OUTPUT" | awk '{print $2}')
  VMS_KB=$(echo "$PS_OUTPUT" | awk '{print $3}')
  THREADS=$(echo "$PS_OUTPUT" | awk '{print $4}')

  # Normalize CPU to server total (divide by core count)
  CPU_SERVER=$(echo "scale=2; $CPU_CORES / $NUM_CORES" | bc)
  RSS_MB=$(echo "scale=2; $RSS_KB / 1024" | bc)
  VMS_MB=$(echo "scale=2; $VMS_KB / 1024" | bc)

  # File descriptor count
  FD_COUNT=$(ls /proc/"$PID"/fd 2>/dev/null | wc -l || echo 0)

  echo "$TIMESTAMP,$CPU_SERVER,$CPU_CORES,$RSS_MB,$VMS_MB,$FD_COUNT,$THREADS,$NUM_CORES" >> "$CSV"

  sleep "$INTERVAL"
done

echo "Process $PID exited. Monitoring stopped."
