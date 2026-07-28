"""Agent worker — long-running process that consumes tasks from Redis queue.

Usage:
    python -m agent.src.worker

Environment variables:
    REDIS_URL          — Redis connection URL (default redis://localhost:6379/0)
    AGENT_BACKEND_URL  — Go backend base URL
    AGENT_API_SECRET   — Secret token for /internal/agent/* endpoints
    AGENT_CONCURRENCY  — Max concurrent investigations (default 2)
    LLM_API_KEY        — API key for LLM provider
"""

from __future__ import annotations

import json
import logging
import os
import signal
import sys
import time
from concurrent.futures import ThreadPoolExecutor, Future
from typing import Any

import redis

from .config import load_config
from .runner import run_investigation

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    stream=sys.stderr,
)
logger = logging.getLogger(__name__)

QUEUE_KEY = "agent:tasks"
SHUTDOWN = False


def handle_signal(signum, frame):
    global SHUTDOWN
    logger.info(f"Received signal {signum}, shutting down gracefully...")
    SHUTDOWN = True


def parse_task(raw: bytes) -> dict[str, Any] | None:
    """Parse a task payload from the queue."""
    try:
        return json.loads(raw)
    except (json.JSONDecodeError, TypeError) as e:
        logger.error(f"Invalid task payload: {e}")
        return None


def process_task(task: dict[str, Any]) -> None:
    """Process a single investigation task."""
    case_id = task.get("caseId", "")
    run_id = task.get("runId", "")

    if not case_id:
        logger.error(f"Task missing caseId: {task}")
        return

    logger.info(f"Starting investigation: case={case_id} run={run_id}")
    start = time.time()

    # Set AGENT_RUN_ID so runner reuses the pre-created run
    if run_id:
        os.environ["AGENT_RUN_ID"] = run_id

    try:
        config = load_config()
        result = run_investigation(case_id, config=config, use_llm=True)
        elapsed = time.time() - start

        if "error" in result and result.get("error"):
            logger.error(f"Investigation failed: case={case_id} error={result['error']} ({elapsed:.1f}s)")
        else:
            verdict = result.get("verdict", "?")
            steps = result.get("step_count", 0)
            logger.info(f"Investigation complete: case={case_id} verdict={verdict} steps={steps} ({elapsed:.1f}s)")
    except Exception as e:
        elapsed = time.time() - start
        logger.exception(f"Investigation crashed: case={case_id} ({elapsed:.1f}s)")
    finally:
        # Clear run ID to not leak across tasks
        os.environ.pop("AGENT_RUN_ID", None)


def main() -> int:
    signal.signal(signal.SIGTERM, handle_signal)
    signal.signal(signal.SIGINT, handle_signal)

    redis_url = os.environ.get("REDIS_URL", "redis://localhost:6379/0")
    concurrency = int(os.environ.get("AGENT_CONCURRENCY", "2"))

    logger.info(f"Agent worker starting: redis={redis_url} concurrency={concurrency}")

    # Validate required config
    config = load_config()
    if not config.agent_api_secret:
        logger.error("AGENT_API_SECRET not set")
        return 1
    if not config.llm_api_key:
        logger.error("LLM_API_KEY not set")
        return 1

    # Connect to Redis
    try:
        r = redis.from_url(redis_url, decode_responses=False)
        r.ping()
        logger.info("Redis connected")
    except redis.ConnectionError as e:
        logger.error(f"Cannot connect to Redis: {e}")
        return 1

    # Worker loop with thread pool for concurrency
    executor = ThreadPoolExecutor(max_workers=concurrency)
    active_futures: list[Future] = []

    logger.info(f"Listening on queue '{QUEUE_KEY}'...")

    while not SHUTDOWN:
        # Clean up completed futures
        active_futures = [f for f in active_futures if not f.done()]

        # If at capacity, wait a bit
        if len(active_futures) >= concurrency:
            time.sleep(0.5)
            continue

        # BRPOP with 2s timeout (allows checking SHUTDOWN flag periodically)
        try:
            result = r.brpop(QUEUE_KEY, timeout=2)
        except redis.ConnectionError:
            logger.warning("Redis connection lost, reconnecting in 5s...")
            time.sleep(5)
            try:
                r.ping()
            except redis.ConnectionError:
                continue
            continue

        if result is None:
            # Timeout, no task available
            continue

        _, raw_payload = result
        task = parse_task(raw_payload)
        if task is None:
            continue

        # Submit to thread pool
        future = executor.submit(process_task, task)
        active_futures.append(future)

    # Graceful shutdown: wait for in-flight tasks
    logger.info(f"Waiting for {len(active_futures)} in-flight tasks to complete...")
    for f in active_futures:
        f.result(timeout=300)  # 5 min max wait per task

    executor.shutdown(wait=True)
    logger.info("Worker shut down cleanly")
    return 0


if __name__ == "__main__":
    sys.exit(main())
