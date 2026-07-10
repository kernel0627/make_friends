from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path


ROOT_DIR = Path(__file__).resolve().parents[2]
BACKEND_DIR = ROOT_DIR / "backend"


@dataclass(frozen=True)
class Settings:
    db_path: Path
    model_dir: Path
    model_name: str
    use_redis: bool
    redis_addr: str
    redis_password: str
    events_stream: str
    jobs_stream: str
    consumer_group: str
    consumer_name: str
    preferred_device: str
    train_exposure_threshold: int
    train_click_threshold: int
    train_interval_seconds: int
    batch_size: int
    model_key: str

    @classmethod
    def from_env(cls) -> "Settings":
        backend_dir = Path(os.getenv("HW_BACKEND_DIR", str(BACKEND_DIR))).resolve()
        root_dir = backend_dir.parent
        db_path = Path(os.getenv("HW_DB_PATH", str(backend_dir / "data" / "app.db"))).resolve()
        model_dir = Path(os.getenv("HW_MODEL_DIR", str(root_dir / "backend-model"))).resolve()
        consumer_name = os.getenv("REC_CONSUMER_NAME", f"worker-{os.getpid()}")
        return cls(
            db_path=db_path,
            model_dir=model_dir,
            model_name=os.getenv("REC_MODEL_NAME", "backend-model"),
            use_redis=os.getenv("USE_REDIS", "false").strip().lower() == "true",
            redis_addr=os.getenv("REDIS_ADDR", "127.0.0.1:6379").strip(),
            redis_password=os.getenv("REDIS_PASSWORD", "").strip(),
            events_stream=os.getenv("REC_EVENTS_STREAM", "zgbe:rec:events").strip(),
            jobs_stream=os.getenv("REC_JOBS_STREAM", "zgbe:rec:jobs").strip(),
            consumer_group=os.getenv("REC_CONSUMER_GROUP", "rec-workers").strip(),
            consumer_name=consumer_name,
            preferred_device=os.getenv("REC_DEVICE", "cuda").strip(),
            train_exposure_threshold=int(os.getenv("REC_TRAIN_EXPOSURES", "200")),
            train_click_threshold=int(os.getenv("REC_TRAIN_CLICKS", "40")),
            train_interval_seconds=int(os.getenv("REC_TRAIN_INTERVAL", "300")),
            batch_size=int(os.getenv("REC_BATCH_SIZE", "32")),
            model_key=os.getenv("REC_MODEL_KEY", "lr_ranker_v1").strip(),
        )

