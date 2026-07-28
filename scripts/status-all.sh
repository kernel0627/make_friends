#!/usr/bin/env bash
# Show status of all dev services (macOS/Linux counterpart of status-all.ps1)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PID_DIR="$REPO_ROOT/.pids"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m'

echo "=== make_friends dev services ==="
echo ""

# Check Redis
if docker ps --format '{{.Names}}' 2>/dev/null | grep -q zgbe-redis; then
  echo -e "  Redis        ${GREEN}running${NC} (docker)"
elif redis-cli ping 2>/dev/null | grep -q PONG; then
  echo -e "  Redis        ${GREEN}running${NC} (system)"
else
  echo -e "  Redis        ${RED}stopped${NC}"
fi

# Check PID-managed services
for name in backend agent admin-web; do
  pf="$PID_DIR/$name.pid"
  if [ -f "$pf" ]; then
    pid=$(cat "$pf" 2>/dev/null || true)
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      echo -e "  $name$(printf '%*s' $((12 - ${#name})) '')${GREEN}running${NC} (PID $pid)"
    else
      echo -e "  $name$(printf '%*s' $((12 - ${#name})) '')${YELLOW}stale pid${NC} ($pid)"
    fi
  else
    echo -e "  $name$(printf '%*s' $((12 - ${#name})) '')${RED}stopped${NC}"
  fi
done

# Check backend health endpoint if running
backend_pid_file="$PID_DIR/backend.pid"
if [ -f "$backend_pid_file" ]; then
  pid=$(cat "$backend_pid_file" 2>/dev/null || true)
  if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
    if curl -sf http://localhost:8080/api/v1/health >/dev/null 2>&1; then
      echo -e "  └─ health    ${GREEN}OK${NC}"
    else
      echo -e "  └─ health    ${YELLOW}not responding${NC}"
    fi
  fi
fi

echo ""
