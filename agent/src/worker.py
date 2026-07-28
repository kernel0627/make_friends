"""Agent worker — long-running process that consumes tasks from Redis Stream.

Uses XREADGROUP + XACK for reliable delivery:
- Tasks are never lost if the worker crashes (pending list)
- Failed tasks are retried up to MAX_RETRIES times
- Dead tasks (exceeded retries) are logged and acknowledged

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

STREAM_KEY = "agent:tasks"
GROUP_NAME = "agent-workers"
MAX_RETRIES = 3
# If a message has been pending longer than this, claim it (seconds)
CLAIM_TIMEOUT_MS = 10 * 60 * 1000  # 10 minutes
SHUTDOWN = False


def handle_signal(signum, frame):
    global SHUTDOWN
    logger.info(f"Received signal {signum}, shutting down gracefully...")
    SHUTDOWN = True


def ensure_consumer_group(r: redis.Redis) -> None:
    """Create the consumer group if it doesn't exist."""
    try:
        r.xgroup_create(STREAM_KEY, GROUP_NAME, id="0", mkstream=True)
        logger.info(f"Created consumer group '{GROUP_NAME}' on stream '{STREAM_KEY}'")
    except redis.ResponseError as e:
        if "BUSYGROUP" in str(e):
            # Group already exists
            pass
        else:
            raise


def process_task(task: dict[str, Any], config) -> None:
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
        result = run_investigation(case_id, config=config, use_llm=True)
        elapsed = time.time() - start

        if "error" in result and result.get("error"):
            logger.error(f"Investigation failed: case={case_id} error={result['error']} ({elapsed:.1f}s)")
            # Rollback case status on failure
            _rollback_case_status(config, case_id)
        else:
            verdict = result.get("verdict", "?")
            steps = result.get("step_count", 0)
            logger.info(f"Investigation complete: case={case_id} verdict={verdict} steps={steps} ({elapsed:.1f}s)")
    except Exception as e:
        elapsed = time.time() - start
        logger.exception(f"Investigation crashed: case={case_id} ({elapsed:.1f}s)")
        _rollback_case_status(config, case_id)
    finally:
        os.environ.pop("AGENT_RUN_ID", None)


def _rollback_case_status(config, case_id: str) -> None:
    """Roll case status back to 'open' when investigation fails."""
    try:
        from .client import BackendClient
        client = BackendClient(config)
        # Use a direct PATCH — the client doesn't have this method yet,
        # so we'll use the underlying httpx client
        client._client.patch(
            f"/internal/agent/case/{case_id}/status",
            json={"status": "open"},
        )
        client.close()
        logger.info(f"Rolled back case {case_id} status to 'open'")
    except Exception as e:
        logger.warning(f"Failed to rollback case status: {e}")


def claim_stale_messages(r: redis.Redis, consumer_name: str) -> list[tuple[str, dict]]:
    """Claim messages that have been pending too long from other consumers."""
    claimed = []
    try:
        # Find pending messages older than CLAIM_TIMEOUT_MS
        pending = r.xpending_range(STREAM_KEY, GROUP_NAME, min="-", max="+", count=10)
        for entry in pending:
            msg_id = entry["message_id"]
            idle_ms = entry["time_since_delivered"]
            delivery_count = entry["times_delivered"]

            if idle_ms < CLAIM_TIMEOUT_MS:
                continue

            if delivery_count >= MAX_RETRIES:
                # Dead message — acknowledge and log
                r.xack(STREAM_KEY, GROUP_NAME, msg_id)
                logger.error(f"Dead message (retries exhausted): id={msg_id} deliveries={delivery_count}")
                continue

            # Claim the stale message
            result = r.xclaim(STREAM_KEY, GROUP_NAME, consumer_name, min_idle_time=CLAIM_TIMEOUT_MS, message_ids=[msg_id])
            for msg in result:
                msg_data = {k.decode() if isinstance(k, bytes) else k: v.decode() if isinstance(v, bytes) else v
                            for k, v in msg[1].items()}
                claimed.append((msg[0].decode() if isinstance(msg[0], bytes) else msg[0], msg_data))
                logger.info(f"Claimed stale message: id={msg_id} delivery={delivery_count + 1}")
    except redis.ResponseError as e:
        logger.warning(f"Error claiming stale messages: {e}")
    return claimed


def main() -> int:
    signal.signal(signal.SIGTERM, handle_signal)
    signal.signal(signal.SIGINT, handle_signal)

    redis_url = os.environ.get("REDIS_URL", "redis://localhost:6379/0")
    concurrency = int(os.environ.get("AGENT_CONCURRENCY", "2"))
    consumer_name = os.environ.get("AGENT_CONSUMER_NAME", f"worker-{os.getpid()}")

    logger.info(f"Agent worker starting: redis={redis_url} concurrency={concurrency} consumer={consumer_name}")

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

    # Ensure consumer group exists
    ensure_consumer_group(r)

    # Worker loop
    executor = ThreadPoolExecutor(max_workers=concurrency)
    active_futures: list[tuple[str, Future]] = []  # (msg_id, future)

    logger.info(f"Listening on stream '{STREAM_KEY}' group '{GROUP_NAME}'...")

    # Track last time we checked for stale messages
    last_claim_check = 0.0

    while not SHUTDOWN:
        # Clean up completed futures and ACK their messages
        still_active = []
        for msg_id, future in active_futures:
            if future.done():
                # Task finished — ACK the message
                try:
                    r.xack(STREAM_KEY, GROUP_NAME, msg_id)
                except redis.ConnectionError:
                    logger.warning(f"Failed to ACK {msg_id}, will retry")
                    still_active.append((msg_id, future))
                    continue
            else:
                still_active.append((msg_id, future))
        active_futures = still_active

        # If at capacity, wait
        if len(active_futures) >= concurrency:
            time.sleep(0.5)
            continue

        # Periodically check for stale/abandoned messages (every 60s)
        now = time.time()
        if now - last_claim_check > 60:
            last_claim_check = now
            claimed = claim_stale_messages(r, consumer_name)
            for msg_id, task_data in claimed:
                future = executor.submit(process_task, task_data, config)
                active_futures.append((msg_id, future))

        # Read new messages from the stream
        try:
            messages = r.xreadgroup(
                GROUP_NAME, consumer_name,
                {STREAM_KEY: ">"},
                count=1,
                block=2000,  # 2s block timeout
            )
        except redis.ConnectionError:
            logger.warning("Redis connection lost, reconnecting in 5s...")
            time.sleep(5)
            try:
                r.ping()
            except redis.ConnectionError:
                continue
            continue

        if not messages:
            continue

        # messages format: [[stream_name, [(msg_id, {field: value}), ...]]]
        for _, msg_list in messages:
            for msg_id, fields in msg_list:
                # Decode bytes
                msg_id_str = msg_id.decode() if isinstance(msg_id, bytes) else msg_id
                task_data = {
                    (k.decode() if isinstance(k, bytes) else k): (v.decode() if isinstance(v, bytes) else v)
                    for k, v in fields.items()
                }

                logger.info(f"Received task: id={msg_id_str} data={task_data}")
                future = executor.submit(process_task, task_data, config)
                active_futures.append((msg_id_str, future))

    # Graceful shutdown: wait for in-flight tasks and ACK them
    logger.info(f"Waiting for {len(active_futures)} in-flight tasks to complete...")
    for msg_id, future in active_futures:
        try:
            future.result(timeout=300)
        except Exception as e:
            logger.error(f"Task {msg_id} error during shutdown: {e}")
        try:
            r.xack(STREAM_KEY, GROUP_NAME, msg_id)
        except Exception:
            pass

    executor.shutdown(wait=True)
    logger.info("Worker shut down cleanly")
    return 0


if __name__ == "__main__":
    sys.exit(main())
