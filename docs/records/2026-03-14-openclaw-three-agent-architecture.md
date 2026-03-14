# 三仓库专职 Agent + 专属 MCP + OpenClaw Gateway 主控方案

## 决策摘要

本次方案固定保留三个独立仓库，不新增第四个 orchestrator 仓库。

- `x-ops-agent.svc.plus` 对外身份固定为 `xops-agent`
- `x-cloud-flow.svc.plus` 对外身份固定为 `x-automation-agent`
- `x-scope-hub.svc.plus` 对外身份固定为 `x-observability-agent`

唯一主控始终是 `openclaw-gateway`。它负责 agent 注册表、入口路由和多 agent 协调；三个仓库都只负责本域推理和本域工具，不承担全局调度。

## 当前仓库定位

当前仓库是 OPS 专职 agent 的参考实现。

- 仓库：`x-ops-agent.svc.plus`
- 默认 `OPENCLAW_AGENT_ID`：`xops-agent`
- 默认角色：OPS 专职 agent
- 主责任：事故分析、根因判断、处置计划、运维执行建议

当前仓库继续保留三种运行面：

1. 真实 OpenClaw Gateway 注册能力
2. 业务 HTTP API
3. MCP HTTP 入口 `/mcp`

同时新增标准化 A2A 入口 `/a2a/v1/*`，但不吸收 automation 或 observability 逻辑。

## 三仓库总体分工

- `xops-agent`
  - 仓库：`x-ops-agent.svc.plus`
  - 领域：incident / ops / remediation
- `x-automation-agent`
  - 仓库：`x-cloud-flow.svc.plus`
  - 领域：IaC / playbook / config / deployment automation
- `x-observability-agent`
  - 仓库：`x-scope-hub.svc.plus`
  - 领域：logs / metrics / traces / topology / alert insight

边界约束固定如下：

- OPS 侧不直接输出 IaC 计划、playbook 或部署变更
- OPS 侧不伪造 logs、metrics、traces 等观测证据
- 超出本域时必须 handoff 或 `needs_input`

## Gateway 主控边界

`openclaw-gateway` 固定负责三件事：

1. agent 注册表
2. 外部请求入口路由
3. 多 agent 协调与任务分发

当前仓库不承担 orchestration，不做任意广播调度，也不实现去中心化控制平面。

## 统一接入契约

三仓库统一使用以下环境变量：

- `OPENCLAW_GATEWAY_URL`
- `OPENCLAW_GATEWAY_TOKEN`
- `OPENCLAW_GATEWAY_PASSWORD`
- `OPENCLAW_AGENT_ID`
- `OPENCLAW_AGENT_NAME`
- `OPENCLAW_AGENT_WORKSPACE`
- `OPENCLAW_AGENT_MODEL`
- `OPENCLAW_REGISTER_ON_START`
- `AI_GATEWAY_URL`
- `AI_GATEWAY_API_KEY`

三仓库统一暴露以下运行面：

- 真实注册命令：直接调用 `openclaw gateway call agents.*`
- MCP HTTP 入口：`POST /mcp`
- 业务 HTTP API：承载各自领域能力
- Codex runtime 注入面：通过 `AI_GATEWAY_URL` / `AI_GATEWAY_API_KEY` 映射到 `OPENAI_BASE_URL` / `OPENAI_API_KEY`
- A2A HTTP 入口：
  - `POST /a2a/v1/negotiate`
  - `POST /a2a/v1/tasks`
  - `GET /a2a/v1/tasks/{task_id}`

## A2A 标准最小协议

三仓库在 `/a2a/v1/*` 下统一采用同一组最小字段，不使用私有 MCP tool 名称来拼装协作协议。

请求字段：

- `from_agent_id`
- `to_agent_id`
- `request_id`
- `intent`
- `goal`
- `context`
- `artifacts`
- `constraints`

响应字段：

- `status`
- `owner_agent_id`
- `summary`
- `required_inputs`
- `result`

`status` 仅允许以下值：

- `accepted`
- `declined`
- `needs_input`
- `completed`

所有 A2A 请求必须保留完整 `request_id`，用于 gateway、agent API 和运行日志的统一追踪。

## 当前仓库接口约定

### 业务 API

- `GET /healthz`
- `POST /api/v1/cases`
- `GET /api/v1/cases/{id}`
- `POST /api/v1/analyze`
- `POST /api/v1/plan`
- `POST /api/v1/agent/run`

### MCP

- `POST /mcp`
- 工具边界只允许 OPS 域能力：
  - 健康检查
  - case create / get
  - incident analyze
  - remediation plan
  - ops-side agent execution

### A2A

当前仓库实现的是 OPS 侧 A2A 协商器：

- 收到 automation 类目标时，不在本域吞并执行，返回 handoff 到 `x-automation-agent`
- 收到 observability 取证类目标时，返回 `needs_input`，要求 `x-observability-agent` 补证据
- 收到 incident / root cause / remediation 类目标时，本域接受并输出 OPS 侧结论

## 推荐命令

```bash
make run-ops
make run-mcp
make register-openclaw
go run ./cmd/agent --mode ops --env-file .env
go run ./cmd/agent --mode register --env-file .env
```

## 验收标准

1. `go run ./cmd/agent --mode register --env-file .env` 连续执行两次，第一次 `created`，第二次 `updated`
2. `/mcp` 能通过 `initialize` 与 `tools/list`
3. `/a2a/v1/negotiate` 能对 automation / observability / ops 三类请求给出不同决策
4. 对 infra 自动化请求，不在本仓库本域执行，而是返回 `x-automation-agent` handoff
5. OpenClaw Gateway 中固定登记的 agent id 仍然是 `xops-agent`
