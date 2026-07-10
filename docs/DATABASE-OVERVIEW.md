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
- `closed_at`
- `cancelled_at`

说明：

- `status` 驱动活动是否开放、是否关闭
- `closed_at` 表示“结束活动”时间，不再拆新的结束时间字段

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

## 管理与推荐相关表

### `admin_cases`

管理后台争议 / 工单。

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
- `user_tags`、`feed_exposures`、`feed_clicks`、embedding 表服务于推荐系统
