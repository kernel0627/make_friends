# 管理后台

`admin-web/` 是项目的内部管理后台，只负责管理端页面，不负责后端服务本身的启动。

## 推荐启动方式

从仓库根目录直接运行：

```powershell
.\start-admin-system.bat
```

这个官方入口会：

- 先重新构建当前后端源码
- 启动 `backend`
- 启动 `admin-web`

后端正确启动后的健康检查地址为：

```text
GET http://127.0.0.1:8080/healthz
```

## 手动启动

如果只想拆开调试，也可以手动分开启动：

```powershell
cd d:\programs\homework\backend
$env:GOCACHE = "d:\programs\homework\.gocache"
$env:GOTMPDIR = "d:\programs\homework\.gocache\tmp"
$env:GOMODCACHE = "d:\programs\homework\.gomodcache"
go run ./cmd/server
```

```powershell
cd d:\programs\homework\admin-web
npm install
npm run dev
```

## 默认演示账号

- `admin / 123456`
- `admin1 / 123456`
- `admin2 / 123456`

这些账号依赖后端种子数据或管理员补齐逻辑。

## 说明

- `start-all.bat` 是整栈入口，用于 `backend + redis + recommender`
- 管理后台本身不包含 recommender worker
- 管理后台依赖后端接口，不建议脱离后端单独理解为完整系统
