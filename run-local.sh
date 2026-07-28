#!/usr/bin/env bash
# 本地部署启动脚本：后端 :8080 + 前端 :5173
# LLM key 放在 .env.local（已 gitignore，永不提交）。
set -euo pipefail
cd "$(dirname "$0")"

export DATABASE_URL="${DATABASE_URL:-postgres://localhost:5432/stocker?sslmode=disable}"

if [ -f .env.local ]; then
  set -a
  # shellcheck disable=SC1091
  source .env.local
  set +a
  echo "已加载 .env.local（LLM_MODEL=${LLM_MODEL:-未设置}）"
else
  echo "未找到 .env.local — 新闻将使用模板文案（复制 .env.local.example 并填入 key 可启用 LLM）"
fi

cleanup() { kill 0 2>/dev/null || true; }
trap cleanup EXIT

(cd server && go run ./cmd/server) &
(cd web && npm run dev) &

echo ""
echo "  后端  http://localhost:8080"
echo "  前端  http://localhost:5173   ← 打开这个"
echo "  Ctrl+C 同时停止两个进程"
wait
