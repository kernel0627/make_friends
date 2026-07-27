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
