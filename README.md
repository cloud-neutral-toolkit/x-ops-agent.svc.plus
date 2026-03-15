# XOpsAgent (OPS Agent + MCP Server)

当前仓库同时保留两类能力：

1. 现有的告警/闭环 API 骨架与 TimescaleDB、OpenObserve、GitOps 链路。
2. 新增的 OPS 智能体与 MCP Server，可优先通过 OpenClaw gateway 运行，失败时回退到本地嵌入式 Codex CLI。

Codex 子仓库位于 `third_party/codex`，以 Git submodule 方式接入，避免触发 Go `vendor/` 机制冲突。

核心 API 模块依次为 Sensor → Analyst → Planner → Gatekeeper → Executor →
Librarian → Orchestrator，接口细节见 [docs/api.md](docs/api.md)。

## 目录结构

```
XOpsAgent/
├── docker-compose.yml
├── configs/
│   ├── otelcol.yaml
│   └── github/
│       └── pr_template.md
├── db/
│   └── 001_schema.sql
├── cmd/agent/
│   └── main.go
├── scripts/
│   └── load_schema.sh
├── go.mod
├── README.md
└── .env.example
```

## 快速开始

### 1) 启动 TimescaleDB + OpenObserve + OTel Collector
```bash
docker compose up -d
```

OpenObserve UI: http://localhost:5080  （默认账号 admin@example.com / ComplexPass123!）

OTLP 入口：http://localhost:5080/api/default/

### 2) 初始化数据库
```bash
scripts/load_schema.sh
```

### 3) 配置环境变量并运行 Agent / OPS 智能体

创建 `.env`（或直接 export 环境变量）：

```bash
export PG_URL="postgres://postgres:postgres@127.0.0.1:5432/ops?sslmode=disable"
export LISTEN_ADDR=":8080"

# GitHub PR 所需（使用你的仓库）
export GITHUB_TOKEN="<ghp_xxx>"
export GITHUB_OWNER="your-github-user-or-org"
export GITHUB_REPO="your-gitops-repo"
export GITHUB_BASE_BRANCH="main"
export GITHUB_FILE_PATH="charts/app/values.yaml"   # 该文件需存在
export FLAG_PATH="featureFlags.recommendation_v2"  # 要切的布尔开关路径

# 可选：ArgoCD
export ARGOCD_URL="https://argocd.example.com"
export ARGOCD_TOKEN="<argocd.jwt>"
export ARGOCD_APP="your-app"
```

运行传统 API：
```bash
go run ./cmd/agent
```

运行 OPS 智能体 HTTP 服务：
```bash
go run ./cmd/agent --mode ops --env-file .env
```

运行 MCP Server（stdio）：
```bash
go run ./cmd/agent --mode mcp --env-file .env
```

注册到 OpenClaw gateway：
```bash
go run ./cmd/agent --mode register --env-file .env
```

### 4) Cloud Run 部署 (推荐方式)

项目已针对 Google Cloud Run 进行了优化，运行在 **OPS 智能体模式**下。

#### 快速部署
```bash
gcloud run deploy x-ops-agent-svc-plus \
  --source . \
  --region europe-west1 \
  --set-env-vars="OPENCLAW_GATEWAY_URL=wss://...,OPENCLAW_GATEWAY_TOKEN=...,AI_GATEWAY_URL=...,AI_GATEWAY_API_KEY=..."
```

#### 关键配置
Cloud Run 默认使用 `-mode ops` 启动。你需要配置以下环境变量（支持别名）：

| 环境变量名称 (推荐) | 原始别名 (.env) | 说明 |
| :--- | :--- | :--- |
| `OPENCLAW_GATEWAY_URL` | `remote` | OpenClaw Gateway 地址 (wss://...) |
| `OPENCLAW_GATEWAY_TOKEN` | `remote-token` | Gateway 访问令牌 |
| `AI_GATEWAY_URL` | `AI-Gateway-Url` | AI 接口 Base URL (https://api.svc.plus/v1) |
| `AI_GATEWAY_API_KEY` | `AI-Gateway-apiKey` | AI 接口访问密钥 |

### 5) 发送告警（模拟 Alertmanager Webhook）
```bash
curl -XPOST http://localhost:8080/alertmanager -H 'Content-Type: application/json' -d '{
  "status": "firing",
  "commonLabels": { "service": "checkout" },
  "alerts": [ { "labels": { "service": "checkout" }, "annotations": { "summary": "p95 latency high" } } ]
}'
```

返回类似：
```json
{"incident_id":1,"pr_url":"https://github.com/<owner>/<repo>/pull/123","verified":false}
```

### 5) Timescale 验证（演示数据）

先生成 20 分钟样本：
```sql
SELECT seed_latency('checkout', 400, 120);
REFRESH MATERIALIZED VIEW CONCURRENTLY metrics_1m;
```

PR 合并&下发（或直接再次生成“更好”的最近 5 分钟样本）：
```sql
-- 让最近 5 分钟平均值更低，模拟“生效后”好转
SELECT seed_latency('checkout', 250, 60);
REFRESH MATERIALIZED VIEW CONCURRENTLY metrics_1m;
```

Agent 的 `/alertmanager` 接口在提交 PR、（可选）等待 ArgoCD Healthy 后，会调用 `recent_latency_improved()` 对比“最近 5 分钟”与“之前 5 分钟”，降幅 ≥10% 视为成功并关闭 incident。

## OTel → OpenObserve

`configs/otelcol.yaml` 已配置 otlphttp/openobserve 导出器，开箱即上报主机指标/日志。

你也可以把应用的 OTLP 指标/日志/链路打到 `http://localhost:5080/api/default/`。

## 生产化提示

- 真实环境建议把验证指标改用 p95/p99。
- GitOps 优先，直连操作需 RBAC + 审计。
- 把“规则+RAG 计划器”放在独立 `planner/` 模块；本 PoC 仅演示“关闭开关”。
- `.env.example` 提供的是标准 env 写法；`ops/mcp/register` 模式也兼容当前仓库里已有的别名格式：
  - `remote` -> `OPENCLAW_GATEWAY_URL`
  - `remote-token` -> `OPENCLAW_GATEWAY_TOKEN`
  - `AI-Gateway-Url` -> `AI_GATEWAY_URL`
  - `AI-Gateway-apiKey` -> `AI_GATEWAY_API_KEY`

## OPS Agent API

`--mode ops` 会额外挂出以下接口：

- `GET /healthz`
- `POST /api/v1/cases`
- `GET /api/v1/cases/{id}`
- `POST /api/v1/analyze`
- `POST /api/v1/plan`
- `POST /api/v1/agent/run`
- `POST /mcp`

`/mcp` 为简化的 JSON-RPC MCP 入口；标准 stdio MCP 运行方式仍然推荐使用 `--mode mcp`。

## OpenClaw 集成

仓库通过 `openclaw gateway call` 调用远端 gateway RPC，支持：

- `agents.list`
- `agents.create`
- `agents.update`

因此可以在不直接嵌入 OpenClaw SDK 的前提下，按 `.env` 中的 gateway 地址和 token 自动完成 agent 注册或更新。

智能体运行顺序如下：

1. 优先调用 OpenClaw gateway 上的目标 agent。
2. 若 gateway 不可用，则回退到本地 `codex` CLI。
3. 若两者都不可用，则返回本地启发式分析结果。

## 常见问题

- PR 失败：检查 `GITHUB_*` 变量、`values.yaml` 路径是否存在、Token 权限（repo:contents, pull_request）。
- ArgoCD 跳过：不配置 `ARGOCD_*` 就会直接跳过等待环节。
- Timescale 没数据：先执行 `seed_latency()` 或把业务指标写入 `metrics_point`

## CI

直接用 API 触发（高级用法）

GitHub REST API v3:

curl -X POST \
  -H "Accept: application/vnd.github+json" \
  -H "Authorization: Bearer <YOUR_TOKEN>" \
  https://api.github.com/repos/<OWNER>/<REPO>/actions/workflows/build-test-release.yml/dispatches \
  -d '{"ref":"main","inputs":{"build":"true","unit":"true","integration":"false","fixtures":"false","release":"false"}}'
