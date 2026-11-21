#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FRONTEND_PORT="${FRONTEND_PORT:-3003}"
BACKEND_PORT="${BACKEND_PORT:-8080}"
AGENT_PORT="${AGENT_PORT:-8090}"

for cmd in go node npm; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "[ERROR] 缺少依赖命令: $cmd，请先安装 Go 1.25+ 与 Node.js 20+（或改用 docker compose -f docker-compose.deploy.yml up -d --build）。"
    exit 1
  fi
done

PIDS=()
cleanup() {
  echo ""
  echo "[stop] 正在停止本地服务..."
  for pid in "${PIDS[@]:-}"; do
    kill "$pid" 2>/dev/null || true
  done
}
trap cleanup EXIT INT TERM

echo "[1/3] 启动 yingce-agent Sidecar (:${AGENT_PORT})..."
(cd "$ROOT_DIR/yingce-agent" && YINGCE_AGENT_PORT="$AGENT_PORT" node server.mjs) &
PIDS+=($!)

echo "[2/3] 编译并启动 Go 后端 (:${BACKEND_PORT}, SQLite WAL 自动迁移)..."
(
  cd "$ROOT_DIR/backend"
  go build -o yingce-backend ./cmd/server
  CANVAS_BACKEND_ADDR=":${BACKEND_PORT}" \
  CANVAS_DATA_PATH="$ROOT_DIR/data" \
  CANVAS_DATABASE_DRIVER=sqlite \
  CANVAS_AUTO_MIGRATE=true \
  CANVAS_REGISTRATION_ENABLED=true \
  CANVAS_ALLOW_PRIVATE_UPSTREAMS=true \
  CANVAS_CORS_ORIGINS="http://localhost:${FRONTEND_PORT},http://127.0.0.1:${FRONTEND_PORT}" \
  YINGCE_AGENT_URL="http://127.0.0.1:${AGENT_PORT}" \
  ./yingce-backend
) &
PIDS+=($!)

echo "[3/3] 启动 React 前端工作台 (:${FRONTEND_PORT})..."
if [ ! -d "$ROOT_DIR/web/node_modules" ]; then
  echo "      首次运行，正在安装前端依赖 (npm install)..."
  (cd "$ROOT_DIR/web" && npm install)
fi

(
  cd "$ROOT_DIR/web"
  VITE_API_PROXY_TARGET="http://127.0.0.1:${BACKEND_PORT}" \
  npm run dev -- --port "$FRONTEND_PORT" --host 0.0.0.0
) &
PIDS+=($!)

# 等待后端就绪并初始化默认管理员账号
for _ in $(seq 1 30); do
  if curl -sf "http://127.0.0.1:${BACKEND_PORT}/api/health/ready" >/dev/null 2>&1; then
    curl -sf -X POST "http://127.0.0.1:${BACKEND_PORT}/api/v1/auth/register" \
      -H "Content-Type: application/json" \
      -d '{"email":"admin@canvas.local","password":"admin123456"}' >/dev/null 2>&1 || true
    break
  fi
  sleep 1
done

echo ""
echo "============================================================"
echo "  AI Canvas Workbench 本地部署已就绪！"
echo "  访问地址: http://localhost:${FRONTEND_PORT}"
echo "  演示管理员: admin@canvas.local / admin123456 (100 初始积分)"
echo "  按 Ctrl+C 可一键停止全部服务"
echo "============================================================"

wait
