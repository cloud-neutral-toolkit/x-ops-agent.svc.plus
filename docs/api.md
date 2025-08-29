# API Reference

本项目遵循模块化架构，关键路径如下：

Sensor → Analyst → Planner → Gatekeeper → Executor → Librarian → Orchestrator

下表描述了各模块的职责、输入输出及接口定义：

| 模块 | 职责 | 输入 | 输出 | 存储 | 接口 | SLO |
|:----|:----|:----|:----|:----|:----|:----|
| Sensor | 接入信号 | OTLP、Prom、logs | OO 明细 | OO、PG | `/ingest/*` | 写入 p99<2s |
| Analyst | 异常检测 | OO 明细、PG | 聚合、analysis.findings | PG | `/analyze/run` | 10min 完成聚合 |
| Planner | 生成计划 | kb_chunk、证据 | plan.proposed | PG | `/plan/generate` | <30s 首版计划 |
| Gatekeeper | 策略评估 | plan.proposed | plan.approved | PG | `/gate/eval` | 自动评估<1s |
| Executor | 执行动作 | plan.approved | exec.step.result | OO、PG | `/adapter/exec` | 单步 15m 超时 |
| Librarian | 知识沉淀 | 日志、diff | kb_doc、kb_chunk | PG | `/kb/ingest` | 5min 内可检索 |
| Orchestrator | 状态机调度 | 各类事件 | case.updates | PG | `/case/*` | 状态原子迁移 |

所有接口遵循 CloudEvents 协议，返回 JSON 结果。示例：

```bash
curl -XPOST http://localhost:8080/plan/generate -H 'Content-Type: application/json' -d '{}'
```

响应：

```json
{"module":"planner","status":"ok"}
```

