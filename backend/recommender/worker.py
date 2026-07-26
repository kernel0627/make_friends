from __future__ import annotations

import logging
import time

import redis

from .embedder import LocalSentenceEmbedder
from .rebuild_all import rebuild_post_embeddings, rebuild_user_embeddings, run_full_rebuild
from .settings import Settings
from .storage import connect
from .trainer import train_ranking_model

logger = logging.getLogger(__name__)

# How long a message may sit unacknowledged before another consumer reclaims
# it. Consumer names include the pid, so a restarted worker would otherwise
# abandon everything the previous process had read but not finished.
CLAIM_IDLE_MS = 120_000
ERROR_BACKOFF_SECONDS = 5


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
    try:
        result = client.xautoclaim(
            name=stream,
            groupname=settings.consumer_group,
            consumername=settings.consumer_name,
            min_idle_time=CLAIM_IDLE_MS,
            start_id="0-0",
            count=50,
        )
    except redis.ResponseError as exc:
        # XAUTOCLAIM needs Redis 6.2+; degrade rather than crash.
        logger.warning("xautoclaim unavailable on %s: %s", stream, exc)
        return []
    # Redis returns (next_cursor, messages) or (next_cursor, messages, deleted).
    if len(result) >= 2 and isinstance(result[1], list):
        return result[1]
    return []


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


def worker_loop(settings: Settings) -> None:
    if not settings.use_redis:
        raise RuntimeError("USE_REDIS must be true to run the recommender worker")

    host, port_text = settings.redis_addr.split(":")
    client = redis.Redis(host=host, port=int(port_text), password=settings.redis_password or None, decode_responses=False)
    client.ping()
    ensure_group(client, settings.events_stream, settings.consumer_group)
    ensure_group(client, settings.jobs_stream, settings.consumer_group)

    conn = connect(settings.db_path)
    embedder = LocalSentenceEmbedder(settings.model_dir, settings.preferred_device)
    exposure_counter = 0
    click_counter = 0
    last_train_at = time.time()
    claimed_startup = False

    try:
        while True:
            try:
                if not claimed_startup:
                    # One sweep at startup so a previous crash does not leave
                    # work stranded in the pending list.
                    stale = []
                    for stream in (settings.events_stream, settings.jobs_stream):
                        messages = claim_stale_messages(client, settings, stream)
                        if messages:
                            stale.append((stream, messages))
                    claimed_startup = True
                    if stale:
                        logger.info("reclaimed %d stale stream batches", len(stale))
                        batches = stale
                    else:
                        batches = None
                else:
                    batches = None

                if batches is None:
                    batches = client.xreadgroup(
                        groupname=settings.consumer_group,
                        consumername=settings.consumer_name,
                        streams={settings.events_stream: ">", settings.jobs_stream: ">"},
                        count=50,
                        block=5000,
                    )

                if not batches:
                    if time.time() - last_train_at >= settings.train_interval_seconds:
                        train_ranking_model(conn, model_key=settings.model_key, model_name=settings.model_name)
                        last_train_at = time.time()
                    continue

                post_ids, user_ids, need_full_rebuild, force_train, ack_targets = collect_work(batches, settings)
                batch_exposures, batch_clicks = count_event_types(batches, settings)
                exposure_counter += batch_exposures
                click_counter += batch_clicks

                if need_full_rebuild:
                    run_full_rebuild(settings)
                    last_train_at = time.time()
                    exposure_counter = 0
                    click_counter = 0
                else:
                    if post_ids:
                        rebuild_post_embeddings(conn, embedder, model_name=settings.model_name, post_ids=sorted(post_ids), batch_size=settings.batch_size)
                    if user_ids:
                        rebuild_user_embeddings(conn, model_name=settings.model_name, user_ids=sorted(user_ids))
                    if (
                        force_train
                        or exposure_counter >= settings.train_exposure_threshold
                        or click_counter >= settings.train_click_threshold
                        or time.time() - last_train_at >= settings.train_interval_seconds
                    ):
                        train_ranking_model(conn, model_key=settings.model_key, model_name=settings.model_name)
                        last_train_at = time.time()
                        exposure_counter = 0
                        click_counter = 0

                # Acknowledge only after the work landed, so a crash mid-batch
                # leaves the messages pending for the next run to reclaim.
                for stream, message_id in ack_targets:
                    client.xack(stream, settings.consumer_group, message_id)

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
