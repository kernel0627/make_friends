#!/usr/bin/env bash
# Stop all services started by start-all.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PID_DIR="$REPO_ROOT/.pids"

echo "Stopping services..."

for pf in "$PID_DIR"/*.pid; do
  [ -f "$pf" ] || continue
  name=$(basename "$pf" .pid)
  pid=$(cat "$pf" 2>/dev/null || true)
  if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
    kill "$pid" 2>/dev/null || true
    echo "  Stopped $name (PID $pid)"
  else
    echo "  $name not running (stale pid)"
  fi
  rm -f "$pf"
done

# Optionally stop Redis container
if docker ps --format '{{.Names}}' 2>/dev/null | grep -q zgbe-redis; then
  echo "  Stopping Redis container..."
  cd "$REPO_ROOT/backend" && docker compose stop redis
fi

echo "All stopped."
