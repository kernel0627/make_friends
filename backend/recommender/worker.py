from __future__ import annotations

import time

import redis

from .embedder import LocalSentenceEmbedder
from .rebuild_all import rebuild_post_embeddings, rebuild_user_embeddings, run_full_rebuild
from .settings import Settings
from .storage import connect
from .trainer import train_ranking_model


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

    try:
        while True:
            batches = client.xreadgroup(
                groupname=settings.consumer_group,
                consumername=settings.consumer_name,
                streams={settings.events_stream: ">", settings.jobs_stream: ">"},
                count=50,
                block=5000,
            )
            post_ids: set[str] = set()
            user_ids: set[str] = set()
            need_full_rebuild = False
            force_train = False
            ack_targets: list[tuple[str, str]] = []

            if not batches:
                if time.time() - last_train_at >= settings.train_interval_seconds:
                    train_ranking_model(conn, model_key=settings.model_key, model_name=settings.model_name)
                    last_train_at = time.time()
                continue

            for stream_name, messages in batches:
                stream = stream_name.decode("utf-8") if isinstance(stream_name, bytes) else str(stream_name)
                for message_id, payload in messages:
                    message = as_text(payload)
                    event_type = message.get("type", "")
                    ack_targets.append((stream, message_id.decode("utf-8") if isinstance(message_id, bytes) else str(message_id)))

                    if stream == settings.jobs_stream:
                        if event_type == "rebuild_all_embeddings":
                            need_full_rebuild = True
                        elif event_type == "rebuild_user_profile":
                            user_ids.update(split_csv(message.get("userIds", "")))
                        elif event_type == "train_ranking_model":
                            force_train = True
                        continue

                    if event_type == "feed_exposure":
                        exposure_counter += 1
                    elif event_type == "feed_click":
                        click_counter += 1
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
                if force_train or exposure_counter >= settings.train_exposure_threshold or click_counter >= settings.train_click_threshold or time.time() - last_train_at >= settings.train_interval_seconds:
                    train_ranking_model(conn, model_key=settings.model_key, model_name=settings.model_name)
                    last_train_at = time.time()
                    exposure_counter = 0
                    click_counter = 0

            for stream, message_id in ack_targets:
                client.xack(stream, settings.consumer_group, message_id)
    finally:
        conn.close()


def main() -> None:
    worker_loop(Settings.from_env())


if __name__ == "__main__":
    main()

