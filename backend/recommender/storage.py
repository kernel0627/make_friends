from __future__ import annotations

import sqlite3
from pathlib import Path
from typing import Iterable, Sequence


def connect(db_path: Path) -> sqlite3.Connection:
    conn = sqlite3.connect(str(db_path), timeout=30, isolation_level=None)
    conn.row_factory = sqlite3.Row
    conn.execute("PRAGMA journal_mode=WAL")
    conn.execute("PRAGMA busy_timeout=5000")
    return conn


def fetch_all(conn: sqlite3.Connection, sql: str, params: Sequence | None = None) -> list[sqlite3.Row]:
    cur = conn.execute(sql, tuple(params or ()))
    return cur.fetchall()


def fetch_one(conn: sqlite3.Connection, sql: str, params: Sequence | None = None) -> sqlite3.Row | None:
    cur = conn.execute(sql, tuple(params or ()))
    return cur.fetchone()


def execute_many(conn: sqlite3.Connection, sql: str, rows: Iterable[Sequence]) -> None:
    rows = list(rows)
    if not rows:
        return
    with conn:
        conn.executemany(sql, rows)


def load_embedding_map(conn: sqlite3.Connection, table: str, key_name: str, model_name: str) -> dict[str, list[float]]:
    rows = fetch_all(
        conn,
        f"SELECT {key_name} AS item_id, embedding_json FROM {table} WHERE model_name = ?",
        (model_name,),
    )
    result: dict[str, list[float]] = {}
    for row in rows:
        raw = row["embedding_json"] or "[]"
        try:
            import json

            result[str(row["item_id"])] = json.loads(raw)
        except Exception:
            continue
    return result


def upsert_post_embeddings(conn: sqlite3.Connection, rows: list[tuple[str, str, str, str, int]]) -> None:
    execute_many(
        conn,
        """
        INSERT INTO post_embeddings(post_id, model_name, embedding_json, content_digest, updated_at)
        VALUES (?, ?, ?, ?, ?)
        ON CONFLICT(post_id, model_name) DO UPDATE SET
          embedding_json = excluded.embedding_json,
          content_digest = excluded.content_digest,
          updated_at = excluded.updated_at
        """,
        rows,
    )


def upsert_user_embeddings(conn: sqlite3.Connection, rows: list[tuple[str, str, str, str, int]]) -> None:
    execute_many(
        conn,
        """
        INSERT INTO user_embeddings(user_id, model_name, embedding_json, profile_digest, updated_at)
        VALUES (?, ?, ?, ?, ?)
        ON CONFLICT(user_id, model_name) DO UPDATE SET
          embedding_json = excluded.embedding_json,
          profile_digest = excluded.profile_digest,
          updated_at = excluded.updated_at
        """,
        rows,
    )


def save_recommendation_model(
    conn: sqlite3.Connection,
    *,
    model_key: str,
    version: int,
    intercept: float,
    feature_json: str,
    training_stats: str,
    trained_at: int,
) -> None:
    with conn:
        conn.execute(
            """
            INSERT INTO recommendation_models(model_key, version, intercept, feature_json, training_stats, trained_at, created_at, updated_at)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(model_key) DO UPDATE SET
              version = excluded.version,
              intercept = excluded.intercept,
              feature_json = excluded.feature_json,
              training_stats = excluded.training_stats,
              trained_at = excluded.trained_at,
              updated_at = excluded.updated_at
            """,
            (model_key, version, intercept, feature_json, training_stats, trained_at, trained_at, trained_at),
        )


def load_model_version(conn: sqlite3.Connection, model_key: str) -> int:
    row = fetch_one(conn, "SELECT version FROM recommendation_models WHERE model_key = ?", (model_key,))
    if not row:
        return 0
    return int(row["version"] or 0)

