#!/usr/bin/env bash
# One-click dev startup for macOS/Linux.
# Starts: Redis → Go backend → Agent worker → Admin-web dev server
#
# Usage:
#   ./scripts/start-all.sh              # normal startup
#   ./scripts/start-all.sh --reseed     # reset + reseed database first
#   ./scripts/start-all.sh --skip-build # skip go build (use existing binary)
#
# Stop:
#   ./scripts/stop-all.sh   (or Ctrl-C if running in foreground)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BACKEND_DIR="$REPO_ROOT/backend"
ADMIN_DIR="$REPO_ROOT/admin-web"
AGENT_DIR="$REPO_ROOT/agent"
LOGS_DIR="$BACKEND_DIR/logs"
PID_DIR="$REPO_ROOT/.pids"

BACKEND_BIN="$BACKEND_DIR/bin/backend-server"
BACKEND_ADDR="${BACKEND_ADDR:-:8080}"
REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6379}"
ADMIN_PORT=5173

# Python for agent worker
AGENT_PYTHON="${AGENT_PYTHON:-$(command -v python 2>/dev/null || true)}"
# Try conda agent env
if [ -z "$AGENT_PYTHON" ] || [ ! -x "$AGENT_PYTHON" ]; then
  CONDA_AGENT="$HOME/miniforge3/envs/agent/bin/python"
  if [ -x "$CONDA_AGENT" ]; then
    AGENT_PYTHON="$CONDA_AGENT"
  fi
fi

# --- flags ---
RESEED=false
SKIP_BUILD=false
for arg in "$@"; do
  case "$arg" in
    --reseed) RESEED=true ;;
    --skip-build) SKIP_BUILD=true ;;
  esac
done

# --- helpers ---
mkdir -p "$LOGS_DIR" "$PID_DIR" "$BACKEND_DIR/bin"

cleanup() {
  echo ""
  echo "Shutting down..."
  for pf in "$PID_DIR"/*.pid; do
    [ -f "$pf" ] || continue
    pid=$(cat "$pf" 2>/dev/null || true)
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
    fi
    rm -f "$pf"
  done
  echo "Done."
}
trap cleanup EXIT INT TERM

wait_for_port() {
  local port=$1 timeout=${2:-30}
  for i in $(seq 1 "$timeout"); do
    if curl -s "http://127.0.0.1:$port/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

kill_pid_file() {
  local pf="$1"
  if [ -f "$pf" ]; then
    local pid
    pid=$(cat "$pf" 2>/dev/null || true)
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      sleep 0.5
    fi
    rm -f "$pf"
  fi
}

# --- [1/6] Redis ---
echo "[1/6] Starting Redis..."
if redis-cli -h 127.0.0.1 -p 6379 ping >/dev/null 2>&1; then
  echo "      Redis already running."
else
  cd "$BACKEND_DIR"
  docker compose up -d redis
  # wait for redis
  for i in $(seq 1 10); do
    redis-cli -h 127.0.0.1 -p 6379 ping >/dev/null 2>&1 && break
    sleep 1
  done
  echo "      Redis started."
fi

# --- [2/6] Build backend ---
if [ "$SKIP_BUILD" = true ]; then
  echo "[2/6] Skipping backend build..."
  if [ ! -x "$BACKEND_BIN" ]; then
    echo "ERROR: $BACKEND_BIN not found. Run without --skip-build first." >&2
    exit 1
  fi
else
  echo "[2/6] Building backend..."
  cd "$BACKEND_DIR"
  go build -o "$BACKEND_BIN" ./cmd/server
  echo "      Built $BACKEND_BIN"
fi

# --- [3/6] Reseed (optional) ---
if [ "$RESEED" = true ]; then
  echo "[3/6] Reseeding database..."
  cd "$BACKEND_DIR"
  go run ./cmd/seed-full -reset=true
else
  echo "[3/6] Skipping reseed..."
fi

# --- [4/6] Start backend ---
echo "[4/6] Starting backend..."
kill_pid_file "$PID_DIR/backend.pid"

export BACKEND_ADDR USE_REDIS=true REDIS_ADDR
cd "$BACKEND_DIR"
"$BACKEND_BIN" >"$LOGS_DIR/backend.out.log" 2>"$LOGS_DIR/backend.err.log" &
echo $! > "$PID_DIR/backend.pid"

if ! wait_for_port 8080 30; then
  echo "ERROR: Backend did not start within 30s. Check $LOGS_DIR/backend.err.log" >&2
  exit 1
fi
echo "      Backend healthy at http://127.0.0.1:8080/healthz (PID $(cat "$PID_DIR/backend.pid"))"

# --- [5/6] Start Agent worker ---
echo "[5/6] Starting Agent worker..."
kill_pid_file "$PID_DIR/agent.pid"

if [ -x "$AGENT_PYTHON" ]; then
  cd "$AGENT_DIR"
  "$AGENT_PYTHON" -m src.worker >"$LOGS_DIR/agent.out.log" 2>"$LOGS_DIR/agent.err.log" &
  echo $! > "$PID_DIR/agent.pid"
  echo "      Agent worker started (PID $(cat "$PID_DIR/agent.pid"))"
else
  echo "      WARNING: No Python found for agent worker. Skipping."
  echo "      Set AGENT_PYTHON or install conda env 'agent'."
fi

# --- [6/6] Start admin-web ---
echo "[6/6] Starting admin-web dev server..."
kill_pid_file "$PID_DIR/admin.pid"

cd "$ADMIN_DIR"
if [ ! -d "node_modules" ]; then
  echo "      Installing admin-web dependencies..."
  npm ci
fi
npx vite --host 127.0.0.1 --port $ADMIN_PORT --strictPort \
  >"$LOGS_DIR/admin.out.log" 2>"$LOGS_DIR/admin.err.log" &
echo $! > "$PID_DIR/admin.pid"
sleep 2
echo "      Admin-web at http://127.0.0.1:$ADMIN_PORT (PID $(cat "$PID_DIR/admin.pid"))"

# --- summary ---
echo ""
echo "═══════════════════════════════════════════"
echo " All services running"
echo "═══════════════════════════════════════════"
echo " Backend   : http://127.0.0.1:8080"
echo " Admin-web : http://127.0.0.1:$ADMIN_PORT"
echo " Redis     : $REDIS_ADDR"
echo " Agent     : worker consuming agent:tasks stream"
echo " Logs      : $LOGS_DIR/"
echo "═══════════════════════════════════════════"
echo ""
echo "Press Ctrl-C to stop all services."
echo ""

# Keep script alive so trap works
wait
