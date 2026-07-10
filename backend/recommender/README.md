# Recommender Worker

This folder hosts the GPU recommendation worker used by the Go backend.

## Official startup path

For the normal local full-stack flow, start it from the repo root:

```powershell
.\start-all.bat
```

That script is the official entry for:

- `backend`
- `redis`
- `recommender.worker`

## Main entry points

- `python -m recommender.worker`
- `python -m recommender.rebuild_all`

## Expected environment

- SQLite database: `backend/data/app.db`
- Local embedding model: `backend-model/`
- Redis Streams:
  - `zgbe:rec:events`
  - `zgbe:rec:jobs`

## Conda setup

```powershell
conda create -n homework_env python=3.10 -y
conda run -n homework_env python -m pip install -r backend/recommender/requirements.txt
```

## Manual run

If you only want to run the worker manually, make sure Redis and the backend are already ready, then run:

```powershell
cd backend
conda run -n homework_env python -m recommender.worker
```
