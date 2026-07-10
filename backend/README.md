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
$env:GOCACHE = "d:\programs\homework\.gocache"
$env:GOTMPDIR = "d:\programs\homework\.gocache\tmp"
$env:GOMODCACHE = "d:\programs\homework\.gomodcache"
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
- `WECHAT_APP_ID` / `WECHAT_APP_SECRET`：微信登录配置
- `USE_REDIS`：是否启用 Redis
- `REDIS_ADDR`：Redis 地址，默认 `127.0.0.1:6379`
- `REDIS_PASSWORD`：Redis 密码
- `WS_ENABLED`：是否启用 WebSocket

## 种子与修复命令

### 全量种子

```powershell
cd backend
$env:GOCACHE = "d:\programs\homework\.gocache"
$env:GOTMPDIR = "d:\programs\homework\.gocache\tmp"
$env:GOMODCACHE = "d:\programs\homework\.gomodcache"
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
$env:GOCACHE = "d:\programs\homework\.gocache"
$env:GOTMPDIR = "d:\programs\homework\.gocache\tmp"
$env:GOMODCACHE = "d:\programs\homework\.gomodcache"
go run ./cmd/repair-settlements
```

这个命令只回算已关闭活动的派生状态，不删数据，不改表结构。

## 测试与构建

```powershell
cd backend
$env:GOCACHE = "d:\programs\homework\.gocache"
$env:GOTMPDIR = "d:\programs\homework\.gocache\tmp"
$env:GOMODCACHE = "d:\programs\homework\.gomodcache"
go test ./...
go build ./...
```
