# Repo Hygiene

## 这份文档解决什么问题

这份文档只负责说明两件事：

- 哪些内容是本地运行必需，但不应该上传
- 哪些文件属于正式项目文档，哪些属于冗余物或临时产物

它不替代根 `README.md`、后端说明或数据库文档。

## 当前应保留在仓库中的正式文档

- `README.md`
- `backend/README.md`
- `admin-web/README.md`
- `docs/DATABASE-OVERVIEW.md`
- `docs/REPO-HYGIENE.md`
- `backend/recommender/README.md`
- `backend-model/README.md`

说明：

- `backend/recommender/README.md` 属于推荐 worker 说明，应保留
- `backend-model/README.md` 属于模型资产自带说明，应保留

## 当前不建议上传的本地内容

这些内容应通过 `.gitignore` 排除：

### 缓存和依赖

- `.gocache/`
- `.gomodcache/`
- `.hf/`
- `node_modules/`
- `admin-web/node_modules/`
- `miniprogram_npm/`
- Python 缓存和虚拟环境目录

### 本地数据库和运行状态

- `backend/data/`
- `*.db`
- `*.sqlite*`
- `*.pid`
- `*.log`
- `*.out`
- `*.err`
- `backend/logs/`

### 构建产物和二进制

- `backend/bin/`
- `bin/`
- `dist/`
- `build/`
- `out/`
- `*.exe`

### 私有配置和本地密钥

- `project.private.config*.json`
- `backend/.env*`
- `admin-web/.env*`

### 本地模型与工具下载物

- `backend-model/` 下除 `README.md` 以外的本地模型文件

## 如何判断文件是不是冗余

本仓库里的文件按三类处理：

### 1. 应忽略但本地保留

这类文件对本地运行有用，但不应进仓库：

- 缓存
- 数据库
- 日志
- 构建产物
- 私有配置

### 2. 应保留且持续更新

这类文件是正式项目资产：

- 源码
- 启动脚本
- 正式 README / docs
- 推荐 worker 源码
- 模型说明文档

### 3. 应删除

如果后续再次出现这些内容，应直接删除或合并后删除：

- 乱码的正式 markdown
- 重复的状态汇报文档
- 阶段性清理记录、临时总结、一次性方案稿
- 无效的运行说明或过时入口说明
- 备份型文档，例如 `README-old.md`、`xxx-final-final.md`

## 当前仓库的判断结论

- 当前项目自带 markdown 只有少量正式说明文档，没有额外保留一批历史方案文档
- 需要处理的重点不是“删很多文档”，而是把已乱码的正式文档重写成可读版本
- 当前运行产物、数据库、缓存和模型下载物不应进入版本管理
