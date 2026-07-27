# 数据库重构 + 场景驱动 Seed 计划

## 目标

把数据层从"展示用"升级为"调查用"：补事件历史、不可变快照、证据引用链，然后用场景驱动方式生成高质量数据。

---

## 一、Schema 变更（保留现有表，叠加新结构）

### 1.1 增强 `DomainEvent` — payload 结构化

当前 payload 只有 `{"title":"...","category":"..."}`。改为：

```go
type DomainEvent struct {
    // ... existing fields ...
    Payload       string // JSON: {"changedFields":["address"],"old":{"address":"海珠桥"},"new":{"address":"大学城"},"context":{"minutesBeforeStart":35}}
}
```

不改表结构（仍是 text），但规范 payload 内容格式。在 `emitDomainEvent` helper 中定义 typed payload structs。

### 1.2 新增 `ContentSnapshot` 表

```go
type ContentSnapshot struct {
    ID          string `gorm:"primaryKey;size:64"`
    PostID      string `gorm:"size:64;not null;index"`
    Title       string `gorm:"size:255;not null"`
    Description string `gorm:"type:text;not null"`
    Address     string `gorm:"size:255;not null"`
    Category    string `gorm:"size:64;not null"`
    MaxCount    int
    ContentHash string `gorm:"size:128;not null;index"`
    SnapshotAt  int64  `gorm:"not null;index"`
    CreatedAt   int64  `gorm:"not null"`
}
```

每次进入审核时自动创建。`ModerationRecord` 加一个 `SnapshotID` 字段指向它。

### 1.3 新增 `Notification` 表

```go
type Notification struct {
    ID          string `gorm:"primaryKey;size:64"`
    UserID      string `gorm:"size:64;not null;index"`
    PostID      string `gorm:"size:64;not null;index"`
    Type        string `gorm:"size:32;not null"` // "activity_changed", "reminder", "settlement_request"
    Channel     string `gorm:"size:16;not null"` // "in_app", "push", "sms"
    Status      string `gorm:"size:16;not null;index"` // "sent", "delivered", "failed", "read"
    Payload     string `gorm:"type:text;not null;default:'{}'"`
    SentAt      int64  `gorm:"not null"`
    DeliveredAt int64  `gorm:"not null;default:0"`
    ReadAt      int64  `gorm:"not null;default:0"`
    CreatedAt   int64  `gorm:"not null;index"`
}
```

### 1.4 拆分案件结构

**保留** `AdminCase` 作为主案件表（管理后台兼容），**新增**：

```go
// 原始举报/申诉（多个可合并入一个 case）
type Report struct {
    ID          string `gorm:"primaryKey;size:64"`
    CaseID      string `gorm:"size:64;not null;index"` // 关联的案件
    ReporterID  string `gorm:"size:64;not null;index"`
    TargetType  string `gorm:"size:32;not null"`       // "post", "message", "user"
    TargetID    string `gorm:"size:128;not null;index"`
    Reason      string `gorm:"type:text;not null"`
    EvidenceIDs string `gorm:"type:text;not null;default:'[]'"` // JSON array of evidence refs
    Status      string `gorm:"size:16;not null;default:pending;index"`
    CreatedAt   int64  `gorm:"not null;index"`
}

// 案件引用的证据
type CaseEvidence struct {
    ID           uint64 `gorm:"primaryKey;autoIncrement"`
    CaseID       string `gorm:"size:64;not null;index"`
    EvidenceType string `gorm:"size:32;not null"` // "domain_event", "chat_message", "content_snapshot", "credit_ledger", "notification"
    EvidenceID   string `gorm:"size:128;not null"`
    AddedBy      string `gorm:"size:64;not null"`         // "agent", "admin", "system"
    Relevance    string `gorm:"size:16;not null;default:supporting"` // "key", "supporting", "context"
    Note         string `gorm:"type:text;not null;default:''"`
    CreatedAt    int64  `gorm:"not null"`
}

// 案件裁决（支持多次裁决：初审、申诉、二审）
type CaseDecision struct {
    ID           uint64 `gorm:"primaryKey;autoIncrement"`
    CaseID       string `gorm:"size:64;not null;index"`
    DeciderID    string `gorm:"size:64;not null;index"` // admin or "agent:run_xxx"
    DecisionType string `gorm:"size:32;not null"`       // "initial", "appeal", "reopen"
    Outcome      string `gorm:"size:32;not null"`       // "upheld", "rejected", "insufficient_evidence", "escalate"
    Reasoning    string `gorm:"type:text;not null;default:''"`
    EvidenceRefs string `gorm:"type:text;not null;default:'[]'"` // JSON: evidence IDs cited
    Actions      string `gorm:"type:text;not null;default:'[]'"` // JSON: actions taken
    CreatedAt    int64  `gorm:"not null;index"`
}
```

### 1.5 增强 `CreditLedger`

```go
// 新增字段：
ReversalOfID   uint64 `gorm:"not null;default:0;index"`   // 如果是反转，指向原流水
CaseID         string `gorm:"size:64;not null;default:''"` // 关联案件
IdempotencyKey string `gorm:"size:128;not null;default:'';uniqueIndex"` // 防重复
```

### 1.6 Policy 文件（不入数据库，作为可查询的 JSON/YAML）

在 `agent/policies/` 目录下维护：

```
agent/policies/
  content_commercial.yaml
  content_off_platform.yaml
  settlement_no_show.yaml
  settlement_material_change.yaml
  credit_reversal.yaml
```

Agent 通过 `get_policy(policy_id)` 工具查询。

---

## 二、场景驱动 Seed 生成器

### 2.1 目录结构

```
backend/internal/seed/
  scenarios/           # 场景定义
    scenario.go        # Scenario struct 和 registry
    content_report.go  # 内容举报场景
    settlement.go      # 履约争议场景
    moderation.go      # 审核申诉场景
    credit.go          # 信用申诉场景
  generator.go         # 从 scenario → 写入数据库
  generator_test.go
```

### 2.2 Scenario 结构

```go
type Scenario struct {
    ID          string
    Type        string // case type
    Difficulty  string // easy / medium / hard
    
    // Ground truth (hidden from agent)
    Truth       ScenarioTruth
    
    // Timeline of events to generate
    Timeline    []TimelineEntry
    
    // What the agent should find
    RequiredEvidence []string
    Distractors      []string
}

type ScenarioTruth struct {
    Outcome          string   // "upheld", "rejected", "insufficient_evidence"
    ResponsibleParty string   // who is at fault
    PolicyRefs       []string // which policies apply
    ForbiddenClaims  []string // things the agent should NOT conclude
}

type TimelineEntry struct {
    Offset   time.Duration // relative to scenario start
    Action   string        // "create_post", "join", "send_message", "change_location", "cancel", "close", etc.
    ActorRef string        // "author", "participant_1", "reporter"
    Data     map[string]any
}
```

### 2.3 生成逻辑

Generator 接收一个 Scenario，按 timeline 顺序：
1. 创建 users（按角色 ref 映射到真实 user IDs）
2. 按 timeline 依次写入 posts、participants、messages、domain_events、notifications、snapshots
3. 最后创建 case 和 hidden label

### 2.4 第一批：10 个手写高质量场景

| # | 类型 | 难度 | 场景 |
|---|------|------|------|
| 1 | content_report | easy | 活动描述含明确广告链接 |
| 2 | content_report | hard | 帖子正常但聊天暴露传销意图 |
| 3 | content_report | medium | 恶意举报者（历史5次被驳回） |
| 4 | settlement_dispute | easy | 明确爽约（有确认消息+缺席证据） |
| 5 | settlement_dispute | hard | 组织者临时改地点+通知未送达 |
| 6 | settlement_dispute | medium | 双方说法矛盾，证据不足 |
| 7 | moderation_appeal | easy | AA 做饭被误判商业活动 |
| 8 | moderation_appeal | medium | 修改内容后申诉（需对比快照） |
| 9 | credit_appeal | easy | 组织者取消活动但参与者被扣分 |
| 10 | credit_appeal | hard | 聊天证明参加了但结算标为 no_show |

---

## 三、Agent Tool API 扩展

新增 endpoints：

- `GET /internal/agent/case/:id/reports` — 该案件关联的原始举报
- `GET /internal/agent/case/:id/evidence` — 已关联的证据列表
- `GET /internal/agent/case/:id/decisions` — 历史裁决
- `GET /internal/agent/case/:id/snapshots` — 内容快照
- `GET /internal/agent/case/:id/notifications` — 相关通知记录
- `GET /internal/agent/user/:id/reports` — 用户作为举报人的历史
- `GET /internal/agent/policy/:id` — 查询社区规则

---

## 四、Golden Case 评测格式重构

拆分为：

```
agent/tests/golden_cases/
  scenarios/           # 场景定义 YAML（truth + timeline）
  inputs/              # agent 可见输入（只有 case_id + claim）
  labels/              # 隐藏标注（expected outcome, required evidence, forbidden claims）
```

Agent 只看 `inputs/`，评测程序对比 `labels/`。

---

## 五、执行顺序

1. 新增 model structs（ContentSnapshot, Notification, Report, CaseEvidence, CaseDecision, CreditLedger 新字段）
2. 更新 db.go AutoMigrate + router.go 补丁 migrate
3. 增强 `emitDomainEvent` — typed payload structs
4. 创建 Policy 文件
5. 写 Scenario framework + Generator
6. 实现 10 个手写场景
7. 扩展 Agent Tool API
8. 重写 golden case 评测格式
9. 端到端验证：scenario → seed → agent 调查 → 评分

每一步都独立 commit，可验证。
