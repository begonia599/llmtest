#!/bin/bash
# ═══════════════════════════════════════════════════════════════════
# LLM Gateway Benchmark - 一键全自动测试脚本
#
# 在 Server A（压测端）上运行，通过 SSH 远程控制 Server B（网关端）
# 自动完成：网关启停、资源监控、压测执行、结果收集
#
# 用法:
#   ./auto-bench.sh <server_b_ip> <vus> [options]
#
# 前提:
#   1. Server A 能免密 SSH 到 Server B (ssh-copy-id root@<server_b_ip>)
#   2. Server B 上已编译好三个网关 (~/llmtest 目录)
#   3. Server A 上 mock-llm 已在运行
#
# 示例:
#   # 50 VUs 测试三个网关
#   ./auto-bench.sh 152.53.164.118 50
#
#   # 500 VUs, 120 秒, 全流式
#   ./auto-bench.sh 152.53.164.118 500 --duration 120s --stream 1.0
#
#   # 限制到 2 核（CPU 0,1），并强制 Go 使用 2 个逻辑核
#   ./auto-bench.sh 152.53.164.118 1000 --cpu-set 0,1 --go-maxprocs 2
#
#   # 只测 Go 和 Node.js
#   ./auto-bench.sh 152.53.164.118 100 --only go,node
#
# ═══════════════════════════════════════════════════════════════════

set -e

# ── 参数解析 ────────────────────────────────────────────
SERVER_B=${1:?用法: $0 <server_b_ip> <vus> [options]}
VUS=${2:?请提供 VUS 数量}
shift 2

# 默认值
DURATION="60s"
STREAM="0.7"
LATENCY="medium"
ERROR_RATE="0"
PRESET="all"
RAMP="false"
CREDS=20
ONLY="go,node,next"
COOLDOWN=30
SSH_USER="root"
REMOTE_DIR="/root/llmtest"
GW_PORT=8080
CPU_SET=""
GO_MAXPROCS=""

while [ $# -gt 0 ]; do
  case "$1" in
    --duration)    DURATION="$2"; shift 2 ;;
    --stream)      STREAM="$2"; shift 2 ;;
    --latency)     LATENCY="$2"; shift 2 ;;
    --error-rate)  ERROR_RATE="$2"; shift 2 ;;
    --preset)      PRESET="$2"; shift 2 ;;
    --ramp)        RAMP="true"; shift ;;
    --creds)       CREDS="$2"; shift 2 ;;
    --only)        ONLY="$2"; shift 2 ;;
    --cooldown)    COOLDOWN="$2"; shift 2 ;;
    --ssh-user)    SSH_USER="$2"; shift 2 ;;
    --cpu-set)     CPU_SET="$2"; shift 2 ;;
    --go-maxprocs) GO_MAXPROCS="$2"; shift 2 ;;
    *) echo "未知参数: $1"; exit 1 ;;
  esac
done

GATEWAY_URL="http://${SERVER_B}:${GW_PORT}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BENCHMARK_SCRIPT="$SCRIPT_DIR/benchmark.js"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
LABEL_SUFFIX="${VUS}vus-${TIMESTAMP}"

SSH_CMD="ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 ${SSH_USER}@${SERVER_B}"

# ── 工具函数 ────────────────────────────────────────────
log() { echo "[$(date '+%H:%M:%S')] $*"; }
ssh_run() { $SSH_CMD "$@"; }
ssh_bg() { $SSH_CMD "nohup bash -c '$*' > /dev/null 2>&1 & echo \$!"; }

wait_for_health() {
  local url=$1
  local max_wait=15
  local i=0
  while [ $i -lt $max_wait ]; do
    if curl -sf "$url/health" > /dev/null 2>&1; then
      return 0
    fi
    sleep 1
    i=$((i + 1))
  done
  return 1
}

# ── SSH 连通性检查 ──────────────────────────────────────
log "检查 SSH 连接到 ${SSH_USER}@${SERVER_B}..."
if ! ssh_run "echo ok" > /dev/null 2>&1; then
  echo ""
  echo "无法 SSH 到 ${SERVER_B}，请先配置免密登录:"
  echo "  ssh-keygen -t ed25519  (如果还没有 key)"
  echo "  ssh-copy-id ${SSH_USER}@${SERVER_B}"
  exit 1
fi
log "SSH 连接正常"

# ── mock-llm 检查 ──────────────────────────────────────
log "检查本地 mock-llm..."
MOCK_PORT=9090
if ! curl -sf "http://localhost:${MOCK_PORT}/health" > /dev/null 2>&1; then
  echo "本地 mock-llm 未运行，请先启动:"
  echo "  cd ~/llmtest/mock-llm && ./mock-llm -port ${MOCK_PORT} &"
  exit 1
fi
log "mock-llm 运行中"

# ── Server B 上游连通性检查 ─────────────────────────────
LOCAL_IP=$(hostname -I | awk '{print $1}')
log "检查 Server B 能否连接到 mock-llm (${LOCAL_IP}:${MOCK_PORT})..."
if ! ssh_run "curl -sf http://${LOCAL_IP}:${MOCK_PORT}/health" > /dev/null 2>&1; then
  echo "Server B 无法访问 mock-llm (${LOCAL_IP}:${MOCK_PORT})"
  echo "请确保防火墙已开放 ${MOCK_PORT} 端口"
  exit 1
fi
log "上游连通正常"

# ── CPU 绑核工具检查（可选） ─────────────────────────────
if [ -n "$CPU_SET" ]; then
  log "检查 Server B 是否支持 taskset..."
  if ! ssh_run "command -v taskset >/dev/null 2>&1"; then
    echo "Server B 不支持 taskset，请先安装 util-linux 后重试"
    exit 1
  fi
fi

# ── 确保 Server B 的 8080 端口空闲 ─────────────────────
log "清理 Server B 上的 ${GW_PORT} 端口..."
ssh_run "fuser -k ${GW_PORT}/tcp 2>/dev/null || true"
sleep 1

# ── 解析要测试的网关 ───────────────────────────────────
IFS=',' read -ra GATEWAYS <<< "$ONLY"

echo ""
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║  LLM Gateway 自动化基准测试"
echo "║"
echo "║  Server B:   ${SERVER_B}"
echo "║  VUs:        ${VUS}"
echo "║  Duration:   ${DURATION}"
echo "║  Stream:     ${STREAM}"
echo "║  Latency:    ${LATENCY}"
echo "║  Creds:      ${CREDS}"
echo "║  Gateways:   ${ONLY}"
echo "║  CPU Set:    ${CPU_SET:-all}"
echo "║  Go P:       ${GO_MAXPROCS:-default}"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo ""

# ── 单个网关测试函数 ───────────────────────────────────
test_gateway() {
  local GW_TYPE=$1
  local LABEL="${GW_TYPE}-${LABEL_SUFFIX}"
  local RESULTS_DIR="$SCRIPT_DIR/../results/${GW_TYPE}/${LABEL}"
  local REMOTE_RESULTS="${REMOTE_DIR}/results/${GW_TYPE}/${LABEL}"

  echo ""
  log "════════════════════════════════════════════"
  log "  测试网关: ${GW_TYPE}"
  log "════════════════════════════════════════════"

  # 1. 确保端口空闲
  ssh_run "fuser -k ${GW_PORT}/tcp 2>/dev/null || true"
  sleep 2

  # 2. 启动网关
  log "[${GW_TYPE}] 启动网关..."
  local START_CMD
  local CPU_PREFIX=""
  local GO_ENV_PREFIX=""

  if [ -n "$CPU_SET" ]; then
    CPU_PREFIX="taskset -c ${CPU_SET} "
  fi
  if [ -n "$GO_MAXPROCS" ]; then
    GO_ENV_PREFIX="GOMAXPROCS=${GO_MAXPROCS} "
  fi

  case "$GW_TYPE" in
    go)
      START_CMD="cd ${REMOTE_DIR}/gateway-go && ${GO_ENV_PREFIX}${CPU_PREFIX}./gateway-go -port ${GW_PORT} -upstream http://${LOCAL_IP}:${MOCK_PORT} -creds ${CREDS}"
      ;;
    node)
      START_CMD="cd ${REMOTE_DIR}/gateway-node && UPSTREAM_URL=http://${LOCAL_IP}:${MOCK_PORT} PORT=${GW_PORT} CRED_COUNT=${CREDS} ${CPU_PREFIX}node dist/index.js"
      ;;
    next)
      START_CMD="cd ${REMOTE_DIR}/gateway-next && UPSTREAM_URL=http://${LOCAL_IP}:${MOCK_PORT} PORT=${GW_PORT} CRED_COUNT=${CREDS} ${CPU_PREFIX}node node_modules/.bin/next start -p ${GW_PORT}"
      ;;
    *)
      log "未知网关类型: ${GW_TYPE}"
      return 1
      ;;
  esac

  GW_PID=$(ssh_bg "$START_CMD")
  log "[${GW_TYPE}] 网关 PID: ${GW_PID}"

  # 3. 等待健康检查
  log "[${GW_TYPE}] 等待网关就绪..."
  if ! wait_for_health "$GATEWAY_URL"; then
    log "[${GW_TYPE}] 网关启动失败！跳过此测试"
    ssh_run "kill ${GW_PID} 2>/dev/null || true"
    return 1
  fi
  log "[${GW_TYPE}] 网关就绪"

  # 4. 获取实际进程 PID (Next.js 可能 fork 子进程)
  local ACTUAL_PID
  ACTUAL_PID=$(ssh_run "ss -tlnp | grep ':${GW_PORT} ' | grep -oP 'pid=\K[0-9]+' | head -1")
  if [ -n "$ACTUAL_PID" ]; then
    log "[${GW_TYPE}] 实际监听进程 PID: ${ACTUAL_PID}"
  else
    ACTUAL_PID=$GW_PID
  fi

  # 5. 启动远程资源监控
  log "[${GW_TYPE}] 启动资源监控..."
  ssh_run "mkdir -p ${REMOTE_RESULTS}"
  MONITOR_PID=$(ssh_bg "bash ${REMOTE_DIR}/monitor/collect.sh ${ACTUAL_PID} ${REMOTE_RESULTS} 1")
  log "[${GW_TYPE}] 监控 PID: ${MONITOR_PID}"

  # 6. 预热
  log "[${GW_TYPE}] 预热中 (15s, 10 VUs)..."
  k6 run --quiet \
    -e GATEWAY_URL="$GATEWAY_URL" -e VUS=10 -e DURATION=15s \
    -e STREAM_RATIO="$STREAM" -e LATENCY="$LATENCY" \
    "$BENCHMARK_SCRIPT" > /dev/null 2>&1 || true
  log "[${GW_TYPE}] 预热完成"

  # 7. 正式压测
  mkdir -p "$RESULTS_DIR"
  log "[${GW_TYPE}] 开始压测: ${VUS} VUs, ${DURATION}..."

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

  # 8. 保存网关 metrics
  curl -s "$GATEWAY_URL/metrics" > "$RESULTS_DIR/gateway-metrics.json" 2>/dev/null || true

  # 9. 停止监控
  log "[${GW_TYPE}] 停止资源监控..."
  ssh_run "kill ${MONITOR_PID} 2>/dev/null || true"
  sleep 2

  # 10. 拉取资源数据到本地
  log "[${GW_TYPE}] 拉取资源监控数据..."
  scp -o StrictHostKeyChecking=no "${SSH_USER}@${SERVER_B}:${REMOTE_RESULTS}/resources.csv" "$RESULTS_DIR/resources.csv" 2>/dev/null || true

  # 11. 停止网关
  log "[${GW_TYPE}] 停止网关..."
  ssh_run "kill ${GW_PID} 2>/dev/null || true; fuser -k ${GW_PORT}/tcp 2>/dev/null || true"
  sleep 2

  log "[${GW_TYPE}] 测试完成，结果: $RESULTS_DIR"
}

# ── 主流程 ─────────────────────────────────────────────
TOTAL=${#GATEWAYS[@]}
CURRENT=0

for GW in "${GATEWAYS[@]}"; do
  CURRENT=$((CURRENT + 1))
  log "进度: [${CURRENT}/${TOTAL}] ${GW}"

  test_gateway "$GW" || log "警告: ${GW} 测试失败，继续下一个"

  # 冷却（最后一个不需要）
  if [ $CURRENT -lt $TOTAL ]; then
    log "冷却 ${COOLDOWN}s..."
    sleep "$COOLDOWN"
  fi
done

# ── 汇总 ──────────────────────────────────────────────
echo ""
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║  全部测试完成!"
echo "╠═══════════════════════════════════════════════════════════════╣"

for GW in "${GATEWAYS[@]}"; do
  LABEL="${GW}-${LABEL_SUFFIX}"
  RESULTS_DIR="$SCRIPT_DIR/../results/${GW}/${LABEL}"
  if [ -f "$RESULTS_DIR/output.txt" ]; then
    echo "║"
    echo "║  ── ${GW} ──"
    # 提取关键指标
    grep -E "(RPS|Total requests|Latency avg|Latency P50|Latency P95|Success rate|SSE chunks)" "$RESULTS_DIR/output.txt" | while read -r line; do
      echo "║  $line"
    done
    # 资源摘要
    if [ -f "$RESULTS_DIR/resources.csv" ]; then
      CPU_MAX=$(tail -n +2 "$RESULTS_DIR/resources.csv" | cut -d',' -f2 | sort -n | tail -1)
      CPU_CORES_MAX=$(tail -n +2 "$RESULTS_DIR/resources.csv" | cut -d',' -f3 | sort -n | tail -1)
      MEM_MAX=$(tail -n +2 "$RESULTS_DIR/resources.csv" | cut -d',' -f4 | sort -n | tail -1)
      echo "║    峰值 CPU (服务器): ${CPU_MAX}%  峰值 CPU (核心): ${CPU_CORES_MAX}%  峰值内存: ${MEM_MAX} MB"
    fi
  fi
done

echo "║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo ""
echo "结果目录: $SCRIPT_DIR/../results/"
