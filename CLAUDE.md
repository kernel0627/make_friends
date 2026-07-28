# Project Rules

## Python 环境

所有 Python 操作（安装依赖、运行脚本、测试）一律使用 conda 的 `agent` 环境：

```bash
conda run -n agent pip install ...
conda run -n agent python ...
conda run -n agent pytest ...
```

不要用系统 python、不要新建 venv/conda 环境。

## Git 工作流

- 不直接在 main 上改代码
- 任何修改先开新 branch，测试通过后再 merge 回 main
- 动手前先说明改动范围

## 项目结构

- `backend/` — Go 后端（Gin + GORM + SQLite + Redis）
- `frontend/` — 微信小程序
- `admin-web/` — React 管理后台
- `backend/recommender/` — Python 推荐 Worker（保持独立，不动）
- `agent/` — Python Agent 服务（LangGraph，调查案件用）

## 测试命令

```bash
# Go
cd backend && go test ./... && go build ./...

# Python Agent
conda run -n agent pytest agent/tests -q

# Python Recommender（需要本地模型，一般跳过）
conda run -n agent python -m pytest backend/recommender/tests -q

# Admin web
cd admin-web && npm ci && npm run build
```

## 前端现状与待办

### 小程序端（frontend/）— 接口大量未对接

后端已有但前端完全未接入的功能：

1. **举报流程** — `POST /posts/:id/reports`, `POST /messages/:id/reports`
   - 活动详情页需要加举报入口（按钮 + 原因选择）
   - 聊天页需要长按消息举报

2. **帖子审核状态展示** — `GET /posts/:id/moderation`
   - 活动详情页：根据 `moderation_status` 显示标签（审核中/已下架/待整改）
   - 被下架时禁用报名/邀请按钮，展示原因
   - 作者视角：加"编辑"按钮触发内容修改（修改后自动回到 pending 重审）

3. **申诉流程** — `POST /moderations/:id/appeals`, `POST /credit-ledgers/:id/appeals`
   - 帖子被下架后，作者可以在详情页提交申诉
   - 信用分被扣后，用户在 credit-explain 页面可以申诉
   - 需要新增申诉表单 UI（原因 + 补充说明）

4. **编辑帖子** — `PUT /posts/:id`
   - `pages/edit/` 目录已存在但不确定功能是否完整
   - 编辑模式需复用发布页（post/index），onLoad 带 postId 参数
   - 编辑后 moderation_status 应回到 pending（后端 UpdatePost 需确认是否已实现此逻辑）

5. **结算纠纷** — 纠纷状态的 UI 展示
   - settlement 页面已有基本功能，但纠纷升级为案件后的状态跟踪缺失

**设计原则**（用户已确认）：
- 尽量复用现有页面，加条件分支，不新建页面
- 活动详情页承担审核状态展示 + 按钮禁用
- 编辑直接复用发布页面
- 如果某页面改动实在太大，删掉重写优于打补丁

### 运营端（admin-web/）— Agent 审批 UI 未对接

后端已就绪，前端需要加的：

1. **案件列表扩展**
   - 状态筛选加入 `under_review`（待审批）和 `investigating`（调查中）
   - CasesPage.jsx 的状态 select 目前只有 open/in_review/resolved

2. **Agent 调查结果展示**（案件详情侧边栏）
   - 展示 proposed decision 卡片：outcome, responsibleParty, confidence 进度条
   - proposed_actions 列表（每个 action 一行，显示类型+原因）
   - reasoning 摘要（折叠展开）
   - 使用 `GET /admin/agent-runs/:id` 获取 steps timeline

3. **审批操作**
   - 批准 / 驳回按钮 → 调用 `POST /admin/cases/:id/review-decision`
   - 批准确认弹窗（展示将执行的 actions 清单）
   - 驳回需要填 comment

4. **Agent Run 状态**
   - 调查中显示 spinner + "Agent 正在调查"
   - 失败显示错误信息

**已有后端接口**：
- `GET /admin/cases` — 列表，支持 status 筛选
- `GET /admin/cases/:id` — 详情（含 decision 如果有）
- `GET /admin/agent-runs` — run 列表
- `GET /admin/agent-runs/:id` — run 详情含 steps + decision
- `POST /admin/cases/:id/investigate` — 触发调查
- `POST /admin/cases/:id/review-decision` — 审批

## Agent 架构要点

### Safety Contract（安全合约）

Agent 绝不自动执行处罚。流程：
1. Admin 触发 investigate → Redis Stream XADD
2. Python Worker XREADGROUP 消费 → LangGraph 跑调查
3. 输出 CaseDecision(status=proposed) + proposed_actions
4. Case 状态变为 `under_review`
5. Admin 在运营后台审批 → approve 触发 action 执行 → case resolved
6. 或 reject → case 重新 open，admin 手动处理

### CaseDecision 生命周期

```
proposed → approved → executed   (正常路径)
proposed → rejected              (驳回，case 回到 open)
```

### Redis Stream 消费模式

- Stream: `agent:tasks`, Group: `agent-workers`
- XREADGROUP + XACK，未 ACK 消息 10min 后可被其他 consumer claim
- 死信：3 次重试后 ACK + 告警日志
- Worker 崩溃 → 消息留在 pending list → 下次启动时 claim 处理

### 幂等保护

- 同一 case 有 proposed decision 时，拒绝新的 investigate 请求
- CaseDecision.IdempotencyKey = `caseID:runID`，同一 run 不会重复写入

### 已知限制

- `TestPostInvitationFlow` 测试持续失败（pre-existing，moderation hold 阻止 invitation accept，和 agent 无关）
- Eval 的 `_evidence_mentioned` 虽已改进但仍有 false positive 空间
- Worker restart recovery：启动时应处理 pending list 中残留的旧消息（目前 claim_stale_messages 每 60s 检查一次，启动后最多 60s 才触发首次检查）

## 后端 / Agent 待办 Work

### P0 — 阻塞 admin-web 对接

1. **GetAdminCase 返回 decision**
   - `GET /admin/cases/:id` 当前不返回关联的 CaseDecision
   - admin-web 详情页需要 decision 数据来展示审批卡片
   - 改法：在 `admin.go` GetAdminCase 中查询该 case 最新的 CaseDecision 并放入 response payload
   - 同时返回 `agentRunId`（如有），方便前端跳转查看调查过程

2. **GetAdminCase 返回 agent run 状态**
   - Case 处于 investigating 时，前端需要展示"调查中"状态
   - response 里加 `agentRun: {id, status, startedAt}` 字段

### P1 — 健壮性与正确性

3. **Worker XREADGROUP 首次启动时检查自己的 pending list**
   - 场景：worker 上次崩溃后重启，之前 read 了消息但没 ACK
   - 当前：claim_stale_messages 只 claim 别人的超时消息，不处理自己之前的
   - 修法：启动时 XREADGROUP count=10 id=0（读自己的 pending entries）先处理掉

4. **Agent run timeout 机制**
   - 如果 LLM 调用卡住（网络超时、API 不响应），investigation 可能永远不返回
   - Worker 的 thread 没有超时机制 — Future 会无限等
   - 建议：给 `run_investigation` 加一个全局 timeout（如 5 分钟），超时后标记 failed

5. **Worker graceful shutdown 时 NACK 未完成的消息**
   - 当前 shutdown 时等 5min 然后 ACK。如果 task 没跑完就被 ACK 了，消息丢失
   - 更好的做法：shutdown 时仅 ACK 已完成的，未完成的不 ACK（让它留在 pending list 被下次 claim）

### P2 — 改进体验

6. **Eval 增加 end-to-end 回归测试 CI**
   - 目前 eval 需要手动跑（seed DB + 启动后端 + run agent）
   - 可以写一个 Makefile target 或 CI job 自动化全流程
   - 至少保证 outcome accuracy 不低于上次的 baseline

7. **Agent investigation 进度上报**
   - Worker 每完成一个 step 更新 AgentRun 的 stepCount
   - 前端可以轮询展示进度（"已完成 5/15 步"）
   - 改法：runner 里每记录一个 step 后调 `update_run(stepCount=N)`

8. **Policy YAML 缺失时的兜底**
   - `get_policy` 如果请求了不存在的 policy_id，后端返回 404
   - Agent LLM 可能 hallucinate 一个不存在的 policy_id
   - 需要在 investigate 节点的 tool call handler 里 catch 404 并给 LLM 反馈"该政策不存在"

9. **Admin-web 审批确认弹窗的 actions 展示**
   - 后端的 proposed_actions 是 JSON 数组，结构为 `[{action, targetId, amount, reason}]`
   - Admin-web 需要把这些渲染为人类可读的"将执行的操作列表"
   - 需要一个 action → 中文描述的映射（前端维护）

### P3 — 未来考虑

10. **多语言 policy 支持**
    - 当前 policy YAML 全中文。如果平台国际化需要支持英文 policy
    - 暂时不做，但 policy 目录结构预留了 locale 能力（如 `policies/zh/`, `policies/en/`）

11. **Agent 调查链路的可观测性**
    - DomainEvent 已在关键节点发出（trigger, approve, reject）
    - 缺少：LLM 调用延迟监控、token 用量追踪、verdict 置信度分布统计
    - 后续可以接 Prometheus/Grafana 或简单写 structured log
