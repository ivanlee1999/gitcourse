#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Load .env if exists
if [ -f "$SCRIPT_DIR/.env" ]; then
  export $(grep -v '^#' "$SCRIPT_DIR/.env" | xargs)
fi

echo "Starting backend on :8090..."
cd "$SCRIPT_DIR/backend"
GITHUB_TOKEN=$GITHUB_TOKEN GITHUB_ORG=$GITHUB_ORG PORT=8090 go run main.go > /tmp/gh-dashboard-backend.log 2>&1 &
BACKEND_PID=$!
echo "Backend PID: $BACKEND_PID"

sleep 2

echo "Starting frontend on :5173..."
cd "$SCRIPT_DIR/frontend"
npm run dev -- --host 0.0.0.0 --port 5173 > /tmp/gh-dashboard-frontend.log 2>&1 &
FRONTEND_PID=$!
echo "Frontend PID: $FRONTEND_PID"

echo ""
echo "✅ Dashboard running at http://$(hostname -I | awk '{print $1}'):5173"
echo "   Backend:  http://localhost:8090"
echo "   Logs:     /tmp/gh-dashboard-*.log"
echo ""
echo "To stop: kill $BACKEND_PID $FRONTEND_PID"
