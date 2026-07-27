# Phase 1: Case Review Agent 后端基础

## 目标

在现有 Go 后端上补齐 Agent 调查所需的数据层和 API，同时搭建 Python Agent 服务的骨架。Phase 1 结束时：
- Go 后端有完整的 domain_event 表记录业务事件
- 案件系统支持 `investigation` 状态和 Agent 调查结果
- Go 暴露一组 Agent 专用的只读 Tool API
- Python Agent 服务骨架能跑通一个最简 investigation loop（hardcoded case → tool call → report）

## 现状分析

已有：
- `AdminCase` 模型 + CRUD（cases.go）：支持 content_report、moderation_appeal、settlement_dispute、credit_appeal
- `CaseEvent` 表：记录 case_created、decision_made、case_reopened
- `ModerationRecord` + outbox + Redis Streams 异步审核流
- `BuildCaseContext()`：聚合活动、聊天、参与、结算、信誉、审核记录
- `CaseContextJSON()`：为 Agent 预留的 JSON 接口

缺少：
- `domain_events` 表（活动修改、报名退出、邀请状态变更等关键业务事件的审计日志）
- Agent run 状态模型（agent_runs、agent_steps）
- 案件中 `investigating` 状态 + agent_run_id 关联
- Go 侧 Agent Tool API（窄接口、受控数据范围）
- Python Agent 服务骨架

## 实现步骤

### 1. 新增 `domain_events` 表 — Go 模型 + 自动写入

**文件**: `backend/internal/model/models.go`

```go
type DomainEvent struct {
    ID            uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
    EventType     string `gorm:"size:64;not null;index" json:"eventType"`
    AggregateType string `gorm:"size:32;not null;index" json:"aggregateType"` // post, user, participant, invitation, settlement
    AggregateID   string `gorm:"size:128;not null;index" json:"aggregateId"`
    ActorID       string `gorm:"size:64;not null;index" json:"actorId"`
    Payload       string `gorm:"type:text;not null;default:'{}'" json:"payload"`
    CreatedAt     int64  `gorm:"not null;index" json:"createdAt"`
}
```

EventType 枚举（第一版）：
- `post.created`, `post.updated`, `post.closed`, `post.cancelled`
- `participant.joined`, `participant.cancelled`
- `invitation.sent`, `invitation.accepted`, `invitation.rejected`
- `settlement.confirmed`, `settlement.admin_resolved`
- `credit.adjusted`, `credit.appeal_created`
- `moderation.submitted`, `moderation.decided`
- `case.created`, `case.decided`, `case.reopened`

在关键业务操作处插入 DomainEvent 写入（同事务）。

### 2. 扩展案件状态 + Agent Run 模型

**文件**: `backend/internal/model/models.go`

```go
// AdminCase 新增字段
// AgentRunID string `gorm:"size:64;not null;default:''" json:"agentRunId"`

type AgentRun struct {
    ID          string `gorm:"primaryKey;size:64" json:"id"`
    CaseID      string `gorm:"size:64;not null;index" json:"caseId"`
    Status      string `gorm:"size:24;not null;default:pending;index" json:"status"` // pending, running, completed, failed
    Model       string `gorm:"size:64;not null;default:''" json:"model"`
    StepCount   int    `gorm:"not null;default:0" json:"stepCount"`
    TokensUsed  int    `gorm:"not null;default:0" json:"tokensUsed"`
    Report      string `gorm:"type:text;not null;default:''" json:"report"` // JSON investigation report
    ErrorMsg    string `gorm:"type:text;not null;default:''" json:"errorMsg"`
    StartedAt   int64  `gorm:"not null;default:0" json:"startedAt"`
    CompletedAt int64  `gorm:"not null;default:0" json:"completedAt"`
    CreatedAt   int64  `gorm:"not null;index" json:"createdAt"`
}

type AgentStep struct {
    ID          uint64 `gorm:"primaryKey;autoIncrement" json:"id"`
    RunID       string `gorm:"size:64;not null;index" json:"runId"`
    StepIndex   int    `gorm:"not null" json:"stepIndex"`
    Action      string `gorm:"size:64;not null" json:"action"` // tool name
    Input       string `gorm:"type:text;not null;default:'{}'" json:"input"`
    Output      string `gorm:"type:text;not null;default:'{}'" json:"output"`
    Reasoning   string `gorm:"type:text;not null;default:''" json:"reasoning"`
    LatencyMs   int    `gorm:"not null;default:0" json:"latencyMs"`
    TokensUsed  int    `gorm:"not null;default:0" json:"tokensUsed"`
    CreatedAt   int64  `gorm:"not null" json:"createdAt"`
}
```

AdminCase.Status 新增 `investigating` 值。

### 3. Go 侧 Agent Tool API

新文件 `backend/internal/api/agent_tools.go`

提供内部只读接口（/internal/agent/...），用 bearer token 认证（复用现有 JWT 或简单 API key）：

| 端点 | 用途 |
|------|------|
| GET /internal/agent/cases/:id | 获取案件基本信息 |
| GET /internal/agent/cases/:id/context | 聚合上下文（复用 BuildCaseContext） |
| GET /internal/agent/posts/:id/history | 活动修改事件 |
| GET /internal/agent/posts/:id/participants | 报名/退出历史 |
| GET /internal/agent/posts/:id/chat | 聊天记录（支持时间窗、limit） |
| GET /internal/agent/users/:id/reputation | 信誉流水 |
| GET /internal/agent/domain-events | 按 aggregate 查询 domain events |
| POST /internal/agent/runs | 创建 Agent Run |
| PATCH /internal/agent/runs/:id | 更新 Run 状态/结果 |
| POST /internal/agent/runs/:id/steps | 追加 Step |

数据裁剪规则：
- 聊天最多返回 200 条
- 信誉流水按案件关联活动过滤
- 所有返回脱敏（不含密码 hash 等敏感字段）

### 4. Python Agent 服务骨架

新目录 `agent/`（项目根目录下，与 backend/ 同级）：

```
agent/
├── pyproject.toml          # 依赖: langgraph, httpx, pydantic
├── agent/__init__.py
├── agent/config.py         # 后端 URL、API key、模型配置
├── agent/client.py         # Go Agent Tool API 的 HTTP client
├── agent/tools.py          # LangGraph tool 定义（调用 client）
├── agent/state.py          # InvestigationState TypedDict
├── agent/graph.py          # LangGraph investigation graph
├── agent/runner.py         # 入口：接收 case_id，运行 graph
├── agent/trajectory.py     # 保存 trajectory JSON
└── tests/
    └── test_basic_flow.py  # 最简集成测试（mock backend）
```

第一版 graph 结构：
```
load_case → extract_claims → investigate_loop → evaluate_evidence → generate_report
```

investigate_loop 内部是 Agent 自主选择 tool 的循环（max 10 steps）。

### 5. 测试验证

- Go: `go test ./...` 通过，新增 domain_event 写入测试 + agent tool API 测试
- Python: `pytest agent/tests/ -q` 通过（mock 后端响应）
- 手动验证：启动后端 → 创建一个 case → 运行 `python -m agent.runner --case-id xxx` → 看到 trajectory 输出

## 不做的事情

- 不做前端/管理后台 UI 改动（Phase 3/4）
- 不做 RAG/向量检索（Phase 2 之后按需加）
- 不做真实 LLM 调用的完整评测（Phase 4）
- 不做推荐 Worker 任何改动
- 不拆分 router.go

## 分支策略

```
git checkout -b feat/agent-foundation
```

完成后 merge 回 main。
