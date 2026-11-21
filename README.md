# AI Canvas Workbench

AI 影视与多模态无限画布工作台（参考 `ddcat-ai/open-ai-canvas` 架构复刻）。

## 设计文档
- [完整开发流程方案](docs/open_ai_canvas_dev_plan.md)
- [实现级规格说明](docs/open_ai_canvas_impl_spec.md)

## 目录结构
- `backend/` — Go 业务后端（Gin + GORM + 双库 SQLite/PostgreSQL + Redis 协调层）
- `web/` — React 19 + Vite + TypeScript 前端工作区与管理后台
- `yingce-agent/` — 独立云端 Agent 运行时 Sidecar
- `plugin-packages/` — 声明式模型协议插件包与支付插件包
- `docs/` — 架构与实现规格文档

## 本地快速启动
```bash
cp .env.example .env
# 后端（默认 SQLite WAL，零外部依赖）
cd backend && go run ./cmd/server
# 前端
cd web && npm install && npm run dev
```
