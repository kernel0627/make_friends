from __future__ import annotations

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[2]))

from recommender.settings import Settings  # noqa: E402
from recommender.worker import (  # noqa: E402
    PendingBatch,
    WorkerCounters,
    claim_stale_messages,
    process_pending_batch,
)


def settings() -> Settings:
    return Settings(
        db_path=Path(":memory:"),
        model_dir=Path("."),
        model_name="model",
        use_redis=True,
        redis_addr="127.0.0.1:6379",
        redis_password="",
        events_stream="events",
        jobs_stream="jobs",
        consumer_group="group",
        consumer_name="worker-test",
        preferred_device="cpu",
        train_exposure_threshold=100,
        train_click_threshold=100,
        train_interval_seconds=10_000,
        batch_size=8,
        model_key="key",
    )


class AckClient:
    def __init__(self, *, fail_first: bool = False) -> None:
        self.fail_first = fail_first
        self.acks: list[tuple[str, str, str]] = []

    def xack(self, stream: str, group: str, message_id: str) -> None:
        if self.fail_first:
            self.fail_first = False
            raise RuntimeError("temporary ack failure")
        self.acks.append((stream, group, message_id))


def no_op(*args, **kwargs) -> None:
    return None


def test_failed_batch_retries_without_counting_until_work_succeeds() -> None:
    cfg = settings()
    client = AckClient()
    counters = WorkerCounters(last_train_at=100.0)
    pending = PendingBatch(
        batches=[
            (
                cfg.events_stream,
                [
                    ("1-0", {"type": "feed_exposure"}),
                    ("2-0", {"type": "post_updated", "postId": "post-1"}),
                ],
            )
        ]
    )
    rebuild_calls = 0

    def flaky_rebuild(*args, **kwargs) -> None:
        nonlocal rebuild_calls
        rebuild_calls += 1
        if rebuild_calls == 1:
            raise RuntimeError("sqlite temporarily locked")

    kwargs = dict(
        client=client,
        settings=cfg,
        conn=object(),
        embedder=object(),
        counters=counters,
        rebuild_post_embeddings_fn=flaky_rebuild,
        rebuild_user_embeddings_fn=no_op,
        run_full_rebuild_fn=no_op,
        train_ranking_model_fn=no_op,
        now_fn=lambda: 101.0,
    )

    with pytest.raises(RuntimeError, match="sqlite temporarily locked"):
        process_pending_batch(pending, **kwargs)
    assert pending.processed is False
    assert counters.exposure_count == 0
    assert client.acks == []

    process_pending_batch(pending, **kwargs)
    assert pending.processed is True
    assert rebuild_calls == 2
    assert counters.exposure_count == 1
    assert [item[2] for item in client.acks] == ["1-0", "2-0"]


def test_ack_retry_does_not_repeat_work_or_event_counts() -> None:
    cfg = settings()
    client = AckClient(fail_first=True)
    counters = WorkerCounters(last_train_at=100.0)
    pending = PendingBatch(
        batches=[
            (
                cfg.events_stream,
                [
                    ("1-0", {"type": "feed_exposure"}),
                    ("2-0", {"type": "post_updated", "postId": "post-1"}),
                ],
            )
        ]
    )
    rebuild_calls = 0

    def rebuild(*args, **kwargs) -> None:
        nonlocal rebuild_calls
        rebuild_calls += 1

    kwargs = dict(
        client=client,
        settings=cfg,
        conn=object(),
        embedder=object(),
        counters=counters,
        rebuild_post_embeddings_fn=rebuild,
        rebuild_user_embeddings_fn=no_op,
        run_full_rebuild_fn=no_op,
        train_ranking_model_fn=no_op,
        now_fn=lambda: 101.0,
    )

    with pytest.raises(RuntimeError, match="ack failure"):
        process_pending_batch(pending, **kwargs)
    assert pending.processed is True
    assert rebuild_calls == 1
    assert counters.exposure_count == 1

    process_pending_batch(pending, **kwargs)
    assert rebuild_calls == 1
    assert counters.exposure_count == 1
    assert [item[2] for item in client.acks] == ["1-0", "2-0"]


def test_xautoclaim_follows_cursor_until_all_stale_messages_are_read() -> None:
    cfg = settings()

    class ClaimClient:
        def __init__(self) -> None:
            self.cursors: list[str] = []

        def xautoclaim(self, **kwargs):
            cursor = kwargs["start_id"]
            self.cursors.append(cursor)
            if cursor == "0-0":
                return (
                    b"50-0",
                    [(f"{index}-0", {"type": "feed_exposure"}) for index in range(50)],
                    [],
                )
            return (
                b"0-0",
                [(f"{index}-0", {"type": "feed_click"}) for index in range(50, 73)],
                [],
            )

    client = ClaimClient()
    messages = claim_stale_messages(client, cfg, cfg.events_stream)
    assert len(messages) == 73
    assert client.cursors == ["0-0", "50-0"]
