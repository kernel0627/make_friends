# Database Overview

## 总体说明

- 数据库类型：SQLite
- 默认文件：`backend/data/app.db`
- 时间字段：统一使用 Unix 毫秒时间戳
- 当前数据库既承载小程序主业务链路，也承载推荐、信誉和管理后台相关数据

## 核心业务表

### `users`

用户主表。

关键字段：

- `id`
- `platform`
- `open_id`
- `nickname`
- `avatar_url`
- `role`
- `credit_score`
- `rating_score`
- `created_at`
- `updated_at`

### `posts`

活动帖主表。

关键字段：

- `id`
- `author_id`
- `title`
- `description`
- `category`
- `sub_category`
- `time_mode`
- `fixed_time`
- `address`
- `lat`
- `lng`
- `max_count`
- `current_count`
- `status`
- `moderation_status`
- `current_moderation_id`
- `content_hash`
- `moderation_updated_at`
- `closed_at`
- `cancelled_at`

说明：

- `status` 驱动活动是否开放、是否关闭
- `closed_at` 表示“结束活动”时间，不再拆新的结束时间字段
- `moderation_status` 驱动活动是否还能报名、邀请、进入正常曝光
- `content_hash` 用于防止旧审核结果覆盖新内容

### `post_participants`

帖子参与关系表。

关键字段：

- `post_id`
- `user_id`
- `status`
- `joined_at`
- `cancelled_at`

### `chat_messages`

群聊消息表。

关键字段：

- `id`
- `post_id`
- `sender_id`
- `content`
- `client_msg_id`
- `created_at`

### `reviews`

活动结束后的互评记录。

关键字段：

- `post_id`
- `from_user_id`
- `to_user_id`
- `rating`
- `comment`
- `created_at`
- `updated_at`

### `activity_scores`

单场活动的评分和信誉回算结果。

关键字段：

- `post_id`
- `user_id`
- `role`
- `rating_score`
- `rating_count`
- `credit_score`
- `expected_review_count`
- `completed_review_count`
- `fulfillment_status`

### `post_participant_settlements`

履约确认表。

关键字段：

- `post_id`
- `user_id`
- `participant_decision`
- `author_decision`
- `final_status`
- `participant_confirmed_at`
- `author_confirmed_at`
- `settled_at`

说明：

- 这是当前履约、评分、用户主页状态展示的核心派生来源之一
- 已关闭活动的历史脏状态可通过 `cmd/repair-settlements` 回算修复

### `credit_ledgers`

信誉流水表。

关键字段：

- `user_id`
- `post_id`
- `source_type`
- `delta`
- `status`
- `note`
- `operator_user_id`

### `moderation_records`

活动审核任务与结果表。

关键字段：

- `id`
- `post_id`
- `content_hash`
- `status`
- `matched_policies`
- `evidence`
- `decision_reason`
- `confidence`
- `model`
- `policy_version`
- `attempt_count`
- `error_message`
- `created_at`
- `finished_at`

### `outbox_events`

可靠异步投递表。

关键字段：

- `event_type`
- `aggregate_id`
- `idempotency_key`
- `payload`
- `status`
- `retry_count`
- `error_message`
- `created_at`
- `published_at`

## 管理与推荐相关表

### `admin_cases`

统一案件表，承载内容举报、审核申诉、履约争议和信用申诉。

关键字段：

- `case_type`
- `source_type`
- `source_id`
- `reporter_id`
- `status`
- `description`
- `evidence_snapshot`
- `decision`
- `decision_reason`
- `resolver_user_id`
- `resolved_at`

### `case_events`

案件操作历史表。

关键字段：

- `case_id`
- `event_type`
- `actor_id`
- `payload`
- `created_at`

### `user_tags`

用户兴趣标签，用于推荐和个人主页展示。

### `feed_exposures` / `feed_clicks`

推荐曝光和点击日志。

### `post_embeddings` / `user_embeddings`

帖子和用户向量。

### `recommendation_models`

推荐排序模型版本与参数。

## 认证相关表

### `refresh_tokens`

Refresh Token 持久化表。

### `revoked_access_tokens`

Access Token 黑名单。

## 主要关系

- 一个 `user` 可以发多个 `post`
- 一个 `post` 可以有多个 `post_participants`
- 一个 `post` 可以有多条 `chat_messages`
- 一个 `post` 关闭后，会产生 `post_participant_settlements`、`reviews`、`activity_scores`、`credit_ledgers`
- `moderation_records` 记录活动审核任务与结果，`outbox_events` 记录可靠异步投递
- `admin_cases` + `case_events` 统一承载举报、申诉和履约争议
- `user_tags`、`feed_exposures`、`feed_clicks`、embedding 表服务于推荐系统
