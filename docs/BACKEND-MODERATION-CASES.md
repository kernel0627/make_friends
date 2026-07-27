# 后端审核与案件链路

这份文档描述当前后端已经接好的两条主线：

1. 活动内容审核
2. 统一案件处理

目标很简单：保持模块化单体，不拆微服务，不做内容版本系统，把异步任务和案件流收在同一个后端里。

## 1. 活动审核

活动创建或编辑后，后端会直接把活动置为 `moderation_status = pending`，并写入审核记录与 outbox 事件。

审核状态只保留这几个值：

- `pending`
- `approved`
- `needs_revision`
- `manual_review`
- `rejected`

当前行为：

- `POST /api/v1/posts`
- `PUT /api/v1/posts/:id`

都会自动触发审核。

审核结果落库时会比对 `content_hash` 和 `current_moderation_id`，旧任务不会覆盖新内容。

查询和管理接口：

- `GET /api/v1/posts/:id/moderation`
- `GET /api/v1/admin/moderations`
- `GET /api/v1/admin/moderations/:id`
- `POST /api/v1/admin/moderations/:id/decision`

审核期间，报名和邀请接收都会被拦住。

## 2. 统一案件

案件类型当前统一为：

- `content_report`
- `moderation_appeal`
- `settlement_dispute`
- `credit_appeal`

用户侧入口：

- `POST /api/v1/messages/:id/reports`
- `POST /api/v1/posts/:id/reports`
- `POST /api/v1/moderations/:id/appeals`
- `POST /api/v1/credit-ledgers/:id/appeals`

管理侧入口：

- `GET /api/v1/admin/cases`
- `GET /api/v1/admin/cases/:id`
- `GET /api/v1/admin/cases/:id/context`
- `POST /api/v1/admin/cases/:id/decision`
- `POST /api/v1/admin/cases/:id/reopen`

案件上下文会聚合：

- 活动信息
- 聊天消息
- 参与记录
- 履约结算
- 信誉流水
- 审核记录
- 案件事件

## 3. 可靠异步

当前链路是：

```text
写业务数据 + 审核记录 + outbox
→ 后端启动时周期投递 Redis Streams
→ Worker 消费审核任务
→ 条件更新活动和审核记录
```

启动条件：

- `USE_REDIS=true`
- `REDIS_ADDR` 可连接

服务进程会自动启动：

- outbox dispatcher
- moderation consumer

## 4. 关键表

- `posts`
- `moderation_records`
- `admin_cases`
- `case_events`
- `outbox_events`

## 5. 运行与验证

启动后端：

```bash
cd backend
USE_REDIS=false go run ./cmd/server
```

启用异步链路：

```bash
cd backend
USE_REDIS=true REDIS_ADDR=127.0.0.1:6379 go run ./cmd/server
```

测试：

```bash
cd backend
go test ./internal/api -run 'Test(ModerationLifecycleAndStaleResultIgnored|ModerationPendingBlocksJoinAndInvitationAcceptance|CreditAppealCanReverseAndRevokeCredit|ReportPostBuildsCaseContext)' -count=1
go test ./...
```
