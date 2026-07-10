# 找个伴儿

这个仓库现在分成 4 个主要部分：

- `frontend/`：微信小程序前端工程
- `backend/`：Go 后端
- `admin-web/`：后台管理前端
- `backend/recommender/`：推荐 worker

## 小程序调试

以后请直接打开这个目录调试小程序：

```text
d:\programs\homework\frontend
```

`frontend/` 已经包含完整的小程序工程文件：

- `app.js`
- `app.json`
- `app.wxss`
- `pages/`
- `components/`
- `utils/`
- `assets/`
- `project.config.json`

小程序内部仍然使用原来的绝对路径写法：

- `/pages/...`
- `/components/...`
- `/assets/...`

不需要再从仓库根目录打开微信开发者工具。

## 后端入口

后端源码入口：

```text
backend/cmd/server/main.go
```

健康检查接口：

```text
GET /healthz
```

## 常用启动命令

```powershell
.\start-admin-system.bat
.\start-all.bat
.\status-all.bat
.\stop-all.bat
```

## 目录说明

```text
frontend/               微信小程序前端
backend/                Go 后端
admin-web/              后台管理前端
docs/                   项目文档
scripts/                启动和辅助脚本
```
