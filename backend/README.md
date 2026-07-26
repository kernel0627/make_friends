# 找个伴儿 Backend

## 核心事实

- 源码入口：`cmd/server/main.go`
- 默认监听：`BACKEND_ADDR=:8080`
- 健康检查：`GET /healthz`
- 默认本地数据库：`data/app.db`

当前仓库中的启动脚本只是包装入口，后端服务本身的真实源码入口只有 `cmd/server/main.go`。

## 目录说明

- `cmd/server`：后端服务入口
- `cmd/seed`：基础种子数据
- `cmd/seed-admin`：补齐管理员账号
- `cmd/seed-full`：全量业务种子数据
- `cmd/repair-settlements`：回算已关闭活动的履约与评分派生状态
- `internal/api`：HTTP / WebSocket 接口
- `internal/db`：数据库初始化
- `internal/model`：数据模型
- `internal/score`：活动分、评分、信誉分回算逻辑
- `recommender/`：推荐 worker 代码

## 官方启动方式

### 1. 单独运行后端

```powershell
cd backend
$env:BACKEND_ADDR = ":8080"
$env:USE_REDIS = "false"
go run ./cmd/server
```

### 2. 管理后台联调

从仓库根目录运行：

```powershell
.\start-admin-system.bat
```

### 3. 整栈联调

从仓库根目录运行：

```powershell
.\start-all.bat
.\status-all.bat
.\stop-all.bat
```

## 常用环境变量

- `BACKEND_ADDR`：监听地址，默认 `:8080`
- `JWT_SECRET`：JWT 密钥
- `ADMIN_INIT_NICKNAME`：首次创建根管理员的昵称，默认 `admin`；冲突时需改为未占用昵称后重试
- `ADMIN_INIT_PASSWORD`：首次创建根管理员的密码；留空时生成随机密码并写入启动日志
- `WECHAT_APP_ID` / `WECHAT_APP_SECRET`：微信登录配置
- `USE_REDIS`：是否启用 Redis
- `REDIS_ADDR`：Redis 地址，默认 `127.0.0.1:6379`
- `REDIS_PASSWORD`：Redis 密码
- `REDIS_TEST_ADDR`：测试专用隔离 Redis 地址；未设置时 Redis 集成测试跳过
- `WS_ENABLED`：是否启用 WebSocket
- `TRUSTED_PROXIES`：可信代理 IP/CIDR，逗号分隔；默认忽略客户端转发头
- `RATE_LIMIT_AUTH_PER_MIN`：注册、密码登录的来源 IP 预算，默认 `60`
- `RATE_LIMIT_ACCOUNT_FAILURES_PER_MIN`：同一账号错误密码预算，默认 `10`
- `RATE_LIMIT_SESSION_PER_MIN`：微信静默登录、刷新令牌的来源 IP 预算，默认 `600`

## 种子与修复命令

### 全量种子

```powershell
cd backend
go run ./cmd/seed-full -reset=true
```

### 仅补齐管理员

```powershell
cd backend
go run ./cmd/seed-admin
```

### 回算履约派生状态

```powershell
cd backend
go run ./cmd/repair-settlements
```

这个命令只回算已关闭活动的派生状态，不删数据，不改表结构。

## 测试与构建

```powershell
cd backend
go test ./...
go build ./...
```
