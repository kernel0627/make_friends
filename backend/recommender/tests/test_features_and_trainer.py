"""Tests for the recommender's pure-logic modules.

These deliberately avoid embedder.py and worker.py so the suite runs without
torch, sentence-transformers or a live Redis.
"""
from __future__ import annotations

import json
import sqlite3
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[2]))

from recommender.features import (  # noqa: E402
    FEATURE_NAMES,
    UNTRAINABLE_WEIGHTS,
    build_training_examples,
    city_from_address,
    clamp,
    cosine_similarity,
    default_weights,
    normalize_vector,
    parse_fixed_time_ms,
    weighted_average,
)
from recommender.trainer import train_ranking_model  # noqa: E402

DAY_MS = 24 * 60 * 60 * 1000


def make_db() -> sqlite3.Connection:
    conn = sqlite3.connect(":memory:", isolation_level=None)
    conn.row_factory = sqlite3.Row
    conn.executescript(
        """
        CREATE TABLE posts (
          id TEXT PRIMARY KEY, author_id TEXT, title TEXT, description TEXT,
          category TEXT, sub_category TEXT, time_mode TEXT, time_days INTEGER,
          fixed_time TEXT, address TEXT, max_count INTEGER, current_count INTEGER,
          status TEXT, created_at INTEGER, updated_at INTEGER
        );
        CREATE TABLE users (id TEXT PRIMARY KEY, credit_score INTEGER, rating_score REAL);
        CREATE TABLE user_tags (user_id TEXT, tag_type TEXT, tag_value TEXT, weight REAL);
        CREATE TABLE feed_exposures (
          request_id TEXT, user_id TEXT, session_id TEXT, post_id TEXT, created_at INTEGER
        );
        CREATE TABLE feed_clicks (
          request_id TEXT, user_id TEXT, session_id TEXT, post_id TEXT, created_at INTEGER
        );
        CREATE TABLE chat_messages (post_id TEXT, sender_id TEXT);
        CREATE TABLE reviews (post_id TEXT);
        CREATE TABLE activity_scores (user_id TEXT);
        CREATE TABLE post_participants (user_id TEXT, post_id TEXT);
        CREATE TABLE post_embeddings (
          post_id TEXT, model_name TEXT, embedding_json TEXT, content_digest TEXT,
          updated_at INTEGER, PRIMARY KEY (post_id, model_name)
        );
        CREATE TABLE user_embeddings (
          user_id TEXT, model_name TEXT, embedding_json TEXT, profile_digest TEXT,
          updated_at INTEGER, PRIMARY KEY (user_id, model_name)
        );
        CREATE TABLE recommendation_models (
          model_key TEXT PRIMARY KEY, version INTEGER, intercept REAL,
          feature_json TEXT, training_stats TEXT, trained_at INTEGER,
          created_at INTEGER, updated_at INTEGER
        );
        """
    )
    return conn


def seed_impressions(conn, now_ms: int, *, recent: int, old: int) -> None:
    """Create posts plus recent and stale exposures.

    post0 is busy and always clicked, post1 is quiet and never clicked, so the
    label is genuinely learnable from the engagement features. Without a
    learnable signal the trainer falls back to default weights and any
    assertion about a *trained* model passes for the wrong reason.
    """
    conn.execute(
        "INSERT INTO users(id, credit_score, rating_score) VALUES ('author', 100, 5.0)"
    )
    post_shapes = [("post0", 6, 10), ("post1", 1, 0)]
    for post_id, current_count, chat_count in post_shapes:
        conn.execute(
            "INSERT INTO posts(id, author_id, title, description, category, sub_category,"
            " time_mode, time_days, fixed_time, address, max_count, current_count, status,"
            " created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
            (post_id, "author", "t", "d", "运动", "羽毛球", "weekend", 1, "",
             "上海市某地", 8, current_count, "open", now_ms - DAY_MS, now_ms - DAY_MS),
        )
        for _ in range(chat_count):
            conn.execute("INSERT INTO chat_messages(post_id, sender_id) VALUES (?, 'author')", (post_id,))

    def add(prefix: str, count: int, created_at: int) -> None:
        for i in range(count):
            request_id = f"{prefix}_req{i}"
            post_id = f"post{i % 2}"
            conn.execute(
                "INSERT INTO feed_exposures(request_id, user_id, session_id, post_id, created_at)"
                " VALUES (?,?,?,?,?)",
                (request_id, "viewer", "sess", post_id, created_at),
            )
            if post_id == "post0":
                conn.execute(
                    "INSERT INTO feed_clicks(request_id, user_id, session_id, post_id, created_at)"
                    " VALUES (?,?,?,?,?)",
                    (request_id, "viewer", "sess", post_id, created_at + 10),
                )

    add("recent", recent, now_ms - DAY_MS)
    add("old", old, now_ms - 400 * DAY_MS)


class TestDefaultWeights:
    def test_covers_every_trainable_feature(self):
        weights = default_weights()
        for name in FEATURE_NAMES:
            assert name in weights, f"{name} missing from default weights"

    def test_includes_untrainable_features(self):
        # The Go scorer computes these live; a model that omits them silently
        # scores those signals as zero.
        weights = default_weights()
        for name in UNTRAINABLE_WEIGHTS:
            assert name in weights
            assert weights[name] != 0

    def test_has_no_unknown_feature_names(self):
        known = set(FEATURE_NAMES) | set(UNTRAINABLE_WEIGHTS)
        assert set(default_weights()) == known


class TestTrainingWindow:
    def test_ignores_impressions_outside_the_window(self):
        conn = make_db()
        now = 1_700_000_000_000
        seed_impressions(conn, now, recent=40, old=60)

        _, _, stats, _ = build_training_examples(conn, "m", now)
        assert stats["exposureCount"] == 40, "stale impressions must be excluded"

        _, _, wide_stats, _ = build_training_examples(conn, "m", now, window_days=500)
        assert wide_stats["exposureCount"] == 100

    def test_feature_row_width_matches_feature_names(self):
        conn = make_db()
        now = 1_700_000_000_000
        seed_impressions(conn, now, recent=10, old=0)
        x_rows, y_rows, _, names = build_training_examples(conn, "m", now)
        assert names == FEATURE_NAMES
        assert all(len(row) == len(FEATURE_NAMES) for row in x_rows)
        assert len(x_rows) == len(y_rows)

    def test_cutoff_exposure_keeps_its_valid_later_click(self):
        conn = make_db()
        now = 1_700_000_000_000
        cutoff = now - 30 * DAY_MS
        seed_impressions(conn, now, recent=0, old=0)
        conn.execute(
            "INSERT INTO feed_exposures(request_id, user_id, session_id, post_id, created_at)"
            " VALUES ('edge_req', 'viewer', 'sess', 'post0', ?)",
            (cutoff,),
        )
        conn.execute(
            "INSERT INTO feed_clicks(request_id, user_id, session_id, post_id, created_at)"
            " VALUES ('edge_req', 'viewer', 'sess', 'post0', ?)",
            (cutoff + DAY_MS,),
        )

        _, labels, stats, _ = build_training_examples(conn, "m", now)
        assert labels == [1]
        assert stats["exposureCount"] == 1
        assert stats["clickCount"] == 1


class TestTrainer:
    def test_falls_back_when_samples_are_insufficient(self):
        conn = make_db()
        now = 1_700_000_000_000
        seed_impressions(conn, now, recent=20, old=0)

        stats = train_ranking_model(conn, model_key="k", model_name="m", now_ms=now)
        assert stats["fallback"] is True
        assert stats["reason"] == "insufficient_samples"

        row = conn.execute("SELECT feature_json FROM recommendation_models WHERE model_key='k'").fetchone()
        assert json.loads(row["feature_json"]) == default_weights()

    def test_published_model_keeps_untrainable_weights(self):
        conn = make_db()
        now = 1_700_000_000_000
        seed_impressions(conn, now, recent=2000, old=0)

        stats = train_ranking_model(conn, model_key="k", model_name="m", now_ms=now)
        # Guard the guard: if this fell back to default weights the assertion
        # below would pass without ever exercising the trained path.
        assert stats["fallback"] is False, f"expected a trained model, got {stats}"

        row = conn.execute("SELECT feature_json FROM recommendation_models WHERE model_key='k'").fetchone()
        weights = json.loads(row["feature_json"])

        for name, value in UNTRAINABLE_WEIGHTS.items():
            assert weights.get(name) == value, (
                f"{name} must survive training; the scorer treats a missing weight as zero"
            )
        # And the learned features are still all present.
        for name in FEATURE_NAMES:
            assert name in weights

    def test_auc_is_measured_on_held_out_rows(self):
        conn = make_db()
        now = 1_700_000_000_000
        seed_impressions(conn, now, recent=2000, old=0)
        stats = train_ranking_model(conn, model_key="k", model_name="m", now_ms=now)
        assert stats["holdoutEval"] is True, f"expected held-out evaluation, got {stats}"

    def test_model_version_increments(self):
        conn = make_db()
        now = 1_700_000_000_000
        seed_impressions(conn, now, recent=20, old=0)
        train_ranking_model(conn, model_key="k", model_name="m", now_ms=now)
        train_ranking_model(conn, model_key="k", model_name="m", now_ms=now + 1)
        row = conn.execute("SELECT version FROM recommendation_models WHERE model_key='k'").fetchone()
        assert row["version"] == 2


class TestMath:
    def test_normalize_vector_returns_unit_length(self):
        out = normalize_vector([3.0, 4.0])
        assert pytest.approx(sum(v * v for v in out), rel=1e-9) == 1.0

    def test_normalize_vector_handles_degenerate_input(self):
        assert normalize_vector([]) == []
        assert normalize_vector([0.0, 0.0]) == []

    def test_cosine_similarity_bounds_and_mismatch(self):
        assert cosine_similarity([1.0, 0.0], [1.0, 0.0]) == 1.0
        assert cosine_similarity([1.0, 0.0], [-1.0, 0.0]) == -1.0
        assert cosine_similarity([1.0], [1.0, 0.0]) == 0.0
        assert cosine_similarity([], []) == 0.0

    def test_weighted_average_skips_bad_entries(self):
        merged = weighted_average([[1.0, 0.0], [0.0, 1.0], [9.9]], [1.0, 1.0, 5.0])
        assert len(merged) == 2
        assert pytest.approx(sum(v * v for v in merged), rel=1e-9) == 1.0

    def test_weighted_average_with_no_usable_weight(self):
        assert weighted_average([[1.0, 0.0]], [0.0]) == []
        assert weighted_average([], []) == []

    def test_clamp(self):
        assert clamp(5, 0, 1) == 1
        assert clamp(-5, 0, 1) == 0
        assert clamp(0.5, 0, 1) == 0.5


class TestParsing:
    def test_city_from_address(self):
        assert city_from_address("上海市徐汇区") == "上海"
        assert city_from_address("  北京朝阳  ") == "北京"
        assert city_from_address("未知小城") == ""
        assert city_from_address("") == ""

    def test_parse_fixed_time_handles_zulu_and_garbage(self):
        assert parse_fixed_time_ms("2026-05-23T19:30:00Z") > 0
        assert parse_fixed_time_ms("not-a-time") == 0
        assert parse_fixed_time_ms("") == 0
