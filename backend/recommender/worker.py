from __future__ import annotations

import logging
import time
from dataclasses import dataclass, field

import redis

from .settings import Settings
from .storage import connect
from .trainer import train_ranking_model

logger = logging.getLogger(__name__)

# How long a message may sit unacknowledged before another consumer reclaims
# it. Consumer names include the pid, so a restarted worker would otherwise
# abandon everything the previous process had read but not finished.
CLAIM_IDLE_MS = 120_000
ERROR_BACKOFF_SECONDS = 5


@dataclass
class WorkerCounters:
    exposure_count: int = 0
    click_count: int = 0
    last_train_at: float = 0.0


@dataclass
class PendingBatch:
    batches: list
    processed: bool = False
    ack_targets: list[tuple[str, str]] = field(default_factory=list)


def ensure_group(client: redis.Redis, stream: str, group: str) -> None:
    try:
        client.xgroup_create(stream, group, id="0", mkstream=True)
    except redis.ResponseError as exc:
        if "BUSYGROUP" not in str(exc):
            raise


def as_text(message: dict) -> dict[str, str]:
    result: dict[str, str] = {}
    for key, value in message.items():
        key_text = key.decode("utf-8") if isinstance(key, bytes) else str(key)
        value_text = value.decode("utf-8") if isinstance(value, bytes) else str(value)
        result[key_text] = value_text
    return result


def split_csv(raw: str) -> list[str]:
    return [item.strip() for item in (raw or "").split(",") if item.strip()]


def as_str(value) -> str:
    return value.decode("utf-8") if isinstance(value, bytes) else str(value)


def claim_stale_messages(client: redis.Redis, settings: Settings, stream: str) -> list:
    """Take over messages a previous worker read but never acknowledged.

    Consumer names are per-process (worker-<pid>), so without this every
    restart strands that process's in-flight messages in the pending list
    forever and their work is silently never done.
    """
    claimed: list = []
    cursor = "0-0"
    seen_cursors: set[str] = set()
    while True:
        try:
            result = client.xautoclaim(
                name=stream,
                groupname=settings.consumer_group,
                consumername=settings.consumer_name,
                min_idle_time=CLAIM_IDLE_MS,
                start_id=cursor,
                count=50,
            )
        except redis.ResponseError as exc:
            # XAUTOCLAIM needs Redis 6.2+; degrade rather than crash.
            logger.warning("xautoclaim unavailable on %s: %s", stream, exc)
            return claimed
        # Redis returns (next_cursor, messages) or
        # (next_cursor, messages, deleted).
        if len(result) < 2:
            return claimed
        next_cursor = as_str(result[0])
        messages = result[1] if isinstance(result[1], list) else []
        claimed.extend(messages)
        if next_cursor == "0-0":
            return claimed
        if next_cursor == cursor or next_cursor in seen_cursors:
            logger.warning("xautoclaim cursor stalled on %s at %s", stream, next_cursor)
            return claimed
        seen_cursors.add(next_cursor)
        cursor = next_cursor


def collect_work(batches, settings: Settings) -> tuple[set[str], set[str], bool, bool, list]:
    post_ids: set[str] = set()
    user_ids: set[str] = set()
    need_full_rebuild = False
    force_train = False
    ack_targets: list[tuple[str, str]] = []

    for stream_name, messages in batches:
        stream = as_str(stream_name)
        for message_id, payload in messages:
            message = as_text(payload)
            event_type = message.get("type", "")
            ack_targets.append((stream, as_str(message_id)))

            if stream == settings.jobs_stream:
                if event_type == "rebuild_all_embeddings":
                    need_full_rebuild = True
                elif event_type == "rebuild_user_profile":
                    user_ids.update(split_csv(message.get("userIds", "")))
                elif event_type == "train_ranking_model":
                    force_train = True
                continue

            if event_type == "feed_exposure":
                pass
            elif event_type == "feed_click":
                if message.get("userId"):
                    user_ids.add(message["userId"])
            elif event_type in {"post_created", "post_updated", "post_closed", "post_joined"}:
                if message.get("postId"):
                    post_ids.add(message["postId"])
                if message.get("userId"):
                    user_ids.add(message["userId"])
            elif event_type == "chat_first_message":
                if message.get("userId"):
                    user_ids.add(message["userId"])
            elif event_type == "review_written":
                user_ids.update(split_csv(message.get("targetUserIds", "")))
                if message.get("userId"):
                    user_ids.add(message["userId"])
                force_train = True

    return post_ids, user_ids, need_full_rebuild, force_train, ack_targets


def count_event_types(batches, settings: Settings) -> tuple[int, int]:
    exposures = 0
    clicks = 0
    for stream_name, messages in batches:
        if as_str(stream_name) == settings.jobs_stream:
            continue
        for _, payload in messages:
            event_type = as_text(payload).get("type", "")
            if event_type == "feed_exposure":
                exposures += 1
            elif event_type == "feed_click":
                clicks += 1
    return exposures, clicks


def process_pending_batch(
    pending: PendingBatch,
    *,
    client,
    settings: Settings,
    conn,
    embedder,
    counters: WorkerCounters,
    rebuild_post_embeddings_fn,
    rebuild_user_embeddings_fn,
    run_full_rebuild_fn,
    train_ranking_model_fn=train_ranking_model,
    now_fn=time.time,
) -> None:
    """Apply one batch once, then ACK it idempotently.

    The PendingBatch stays alive when this function raises. If business work
    failed, ``processed`` remains false and the same messages are retried. If
    only ACK failed, ``processed`` is already true, so the retry skips all
    database/model work and only repeats the idempotent acknowledgements.
    """
    if not pending.processed:
        post_ids, user_ids, need_full_rebuild, force_train, ack_targets = collect_work(
            pending.batches,
            settings,
        )
        batch_exposures, batch_clicks = count_event_types(pending.batches, settings)
        next_exposure_count = counters.exposure_count + batch_exposures
        next_click_count = counters.click_count + batch_clicks
        next_last_train_at = counters.last_train_at

        if need_full_rebuild:
            run_full_rebuild_fn(settings)
            next_last_train_at = now_fn()
            next_exposure_count = 0
            next_click_count = 0
        else:
            if post_ids:
                rebuild_post_embeddings_fn(
                    conn,
                    embedder,
                    model_name=settings.model_name,
                    post_ids=sorted(post_ids),
                    batch_size=settings.batch_size,
                )
            if user_ids:
                rebuild_user_embeddings_fn(
                    conn,
                    model_name=settings.model_name,
                    user_ids=sorted(user_ids),
                )
            now = now_fn()
            if (
                force_train
                or next_exposure_count >= settings.train_exposure_threshold
                or next_click_count >= settings.train_click_threshold
                or now - counters.last_train_at >= settings.train_interval_seconds
            ):
                train_ranking_model_fn(
                    conn,
                    model_key=settings.model_key,
                    model_name=settings.model_name,
                )
                next_last_train_at = now
                next_exposure_count = 0
                next_click_count = 0

        # Commit in-memory counters only after every database/model operation
        # for this batch completed successfully.
        counters.exposure_count = next_exposure_count
        counters.click_count = next_click_count
        counters.last_train_at = next_last_train_at
        pending.ack_targets = ack_targets
        pending.processed = True

    for stream, message_id in pending.ack_targets:
        client.xack(stream, settings.consumer_group, message_id)


def worker_loop(settings: Settings) -> None:
    if not settings.use_redis:
        raise RuntimeError("USE_REDIS must be true to run the recommender worker")

    host, port_text = settings.redis_addr.split(":")
    client = redis.Redis(host=host, port=int(port_text), password=settings.redis_password or None, decode_responses=False)
    client.ping()
    ensure_group(client, settings.events_stream, settings.consumer_group)
    ensure_group(client, settings.jobs_stream, settings.consumer_group)

    # Keep the large embedding runtime out of module import so worker
    # orchestration and retry semantics can be tested without torch.
    from .embedder import LocalSentenceEmbedder
    from .rebuild_all import (
        rebuild_post_embeddings,
        rebuild_user_embeddings,
        run_full_rebuild,
    )

    conn = connect(settings.db_path)
    embedder = LocalSentenceEmbedder(settings.model_dir, settings.preferred_device)
    counters = WorkerCounters(last_train_at=time.time())
    claimed_startup = False
    pending: PendingBatch | None = None

    try:
        while True:
            try:
                if pending is None:
                    if not claimed_startup:
                        # One complete sweep at startup so a previous crash
                        # does not leave work stranded in the pending list.
                        stale = []
                        for stream in (settings.events_stream, settings.jobs_stream):
                            messages = claim_stale_messages(client, settings, stream)
                            if messages:
                                stale.append((stream, messages))
                        claimed_startup = True
                        if stale:
                            logger.info(
                                "reclaimed %d stale stream messages",
                                sum(len(messages) for _, messages in stale),
                            )
                            batches = stale
                        else:
                            batches = None
                    else:
                        batches = None

                    if batches is None:
                        batches = client.xreadgroup(
                            groupname=settings.consumer_group,
                            consumername=settings.consumer_name,
                            streams={
                                settings.events_stream: ">",
                                settings.jobs_stream: ">",
                            },
                            count=50,
                            block=5000,
                        )

                    if not batches:
                        if (
                            time.time() - counters.last_train_at
                            >= settings.train_interval_seconds
                        ):
                            train_ranking_model(
                                conn,
                                model_key=settings.model_key,
                                model_name=settings.model_name,
                            )
                            counters.last_train_at = time.time()
                        continue

                    pending = PendingBatch(batches=batches)

                process_pending_batch(
                    pending,
                    client=client,
                    settings=settings,
                    conn=conn,
                    embedder=embedder,
                    counters=counters,
                    rebuild_post_embeddings_fn=rebuild_post_embeddings,
                    rebuild_user_embeddings_fn=rebuild_user_embeddings,
                    run_full_rebuild_fn=run_full_rebuild,
                )
                pending = None
            except KeyboardInterrupt:
                raise
            except Exception:
                # A transient Redis blip or a locked SQLite write used to kill
                # the worker outright, silently stopping all recommendation
                # updates until someone noticed and restarted it.
                logger.exception("recommender worker iteration failed; retrying")
                time.sleep(ERROR_BACKOFF_SECONDS)
    finally:
        conn.close()


def main() -> None:
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s: %(message)s")
    worker_loop(Settings.from_env())


if __name__ == "__main__":
    main()
