from __future__ import annotations

import hashlib
import json
import math
from collections import defaultdict
from typing import Any

from .storage import fetch_all, load_embedding_map


FEATURE_NAMES = [
    "embedding_cosine",
    "category_match",
    "sub_category_match",
    "city_match",
    "author_quality",
    "author_rating_score",
    "author_credit_score",
    "author_activity_score_count",
    "post_current_count",
    "post_chat_count",
    "post_review_count",
    "post_age_hours",
    "fixed_time_distance_hours",
    "freshness",
    "interaction_heat",
    "joinability",
    "joinability_ratio",
    "viewer_clicked_same_subcategory_7d",
    "viewer_unclicked_same_subcategory_7d",
]

# How far back training reads impression data.
TRAINING_WINDOW_DAYS = 30

CHINESE_CITIES = [
    "\u4e0a\u6d77",
    "\u5317\u4eac",
    "\u5e7f\u5dde",
    "\u6df1\u5733",
    "\u676d\u5dde",
    "\u6210\u90fd",
    "\u6b66\u6c49",
    "\u5357\u4eac",
    "\u82cf\u5dde",
    "\u897f\u5b89",
    "\u91cd\u5e86",
    "\u5929\u6d25",
]


# Features the Go scorer computes at request time but that training data
# cannot express, so no weight can ever be learned for them. feed_exposures
# stores no viewer coordinates, so location_proximity is only knowable live.
#
# The scorer looks weights up by name and treats a missing name as zero, so
# these must be carried into every published model. Without this, the moment a
# real model trains, location stops influencing ranking at all.
UNTRAINABLE_WEIGHTS = {
    "location_proximity": 0.24,
}

_TRAINABLE_DEFAULT_WEIGHTS = {
    "embedding_cosine": 0.46,
    "sub_category_match": 0.18,
    "category_match": 0.08,
    "city_match": 0.06,
    "author_quality": 0.18,
    "interaction_heat": 0.06,
    "freshness": 0.08,
    "joinability": 0.06,
    "joinability_ratio": 0.06,
    "author_rating_score": 0.07,
    "author_credit_score": 0.06,
    "author_activity_score_count": 0.03,
    "post_current_count": 0.02,
    "post_chat_count": 0.03,
    "post_review_count": 0.03,
    "post_age_hours": -0.02,
    "fixed_time_distance_hours": -0.02,
    "viewer_clicked_same_subcategory_7d": 0.08,
    "viewer_unclicked_same_subcategory_7d": -0.05,
}


def default_weights() -> dict[str, float]:
    """Fallback weights, published when a model cannot be trained.

    Spans the full feature set the Go scorer evaluates: every trainable
    feature plus the untrainable ones. Previously this omitted
    ``joinability_ratio`` (so that signal was dead in the fallback model)
    while listing ``location_proximity`` — which the trainer then dropped
    from every real model it published.
    """
    missing = set(FEATURE_NAMES) - set(_TRAINABLE_DEFAULT_WEIGHTS)
    unknown = set(_TRAINABLE_DEFAULT_WEIGHTS) - set(FEATURE_NAMES)
    if missing or unknown:
        raise ValueError(
            f"default weights out of sync with FEATURE_NAMES: missing={sorted(missing)} unknown={sorted(unknown)}"
        )
    return {**_TRAINABLE_DEFAULT_WEIGHTS, **UNTRAINABLE_WEIGHTS}


def clamp(value: float, min_value: float, max_value: float) -> float:
    return max(min_value, min(max_value, value))


def saturate(value: float) -> float:
    if value <= 0:
        return 0.0
    return 1 - math.exp(-value / 5.0)


def normalize_vector(values: list[float]) -> list[float]:
    if not values:
        return []
    norm = math.sqrt(sum(v * v for v in values))
    if norm <= 0:
        return []
    return [float(v / norm) for v in values]


def weighted_average(vectors: list[list[float]], weights: list[float]) -> list[float]:
    if not vectors or not weights or len(vectors) != len(weights):
        return []
    dim = len(vectors[0])
    if dim == 0:
        return []
    total_weight = 0.0
    merged = [0.0] * dim
    for vector, weight in zip(vectors, weights):
        if len(vector) != dim or weight <= 0:
            continue
        total_weight += weight
        for idx, value in enumerate(vector):
            merged[idx] += value * weight
    if total_weight <= 0:
        return []
    merged = [value / total_weight for value in merged]
    return normalize_vector(merged)


def cosine_similarity(left: list[float], right: list[float]) -> float:
    if not left or not right or len(left) != len(right):
        return 0.0
    return clamp(sum(a * b for a, b in zip(left, right)), -1.0, 1.0)


def city_from_address(address: str) -> str:
    value = (address or "").strip()
    for city in CHINESE_CITIES:
        if city in value:
            return city
    return ""


def build_post_text(post: dict[str, Any]) -> str:
    return " ".join(
        filter(
            None,
            [
                str(post.get("title") or "").strip(),
                str(post.get("category") or "").strip(),
                str(post.get("sub_category") or "").strip(),
                str(post.get("description") or "").strip(),
                str(post.get("address") or "").strip(),
            ],
        )
    ).strip()


def digest_text(value: str) -> str:
    return hashlib.sha1(value.encode("utf-8")).hexdigest()


def safe_json(values: list[float]) -> str:
    return json.dumps([round(float(value), 8) for value in values], ensure_ascii=False)


def parse_fixed_time_ms(value: str) -> int:
    raw = (value or "").strip()
    if not raw:
        return 0
    try:
        from datetime import datetime

        if raw.endswith("Z"):
            raw = raw[:-1] + "+00:00"
        return int(datetime.fromisoformat(raw).timestamp() * 1000)
    except Exception:
        return 0


def load_posts(conn, post_ids: list[str] | None = None) -> list[dict[str, Any]]:
    sql = """
    SELECT id, author_id, title, description, category, sub_category, time_mode, time_days, fixed_time,
           address, max_count, current_count, status, created_at, updated_at
    FROM posts
    """
    params: tuple[Any, ...] = ()
    if post_ids:
        placeholders = ",".join("?" for _ in post_ids)
        sql += f" WHERE id IN ({placeholders})"
        params = tuple(post_ids)
    return [dict(row) for row in fetch_all(conn, sql, params)]


def load_users(conn) -> dict[str, dict[str, Any]]:
    return {str(row["id"]): dict(row) for row in fetch_all(conn, "SELECT id, credit_score, rating_score FROM users")}


def load_user_tag_maps(conn) -> dict[str, dict[str, dict[str, float]]]:
    rows = fetch_all(conn, "SELECT user_id, tag_type, tag_value, weight FROM user_tags")
    result: dict[str, dict[str, dict[str, float]]] = defaultdict(lambda: {"category": {}, "sub_category": {}, "city": {}})
    max_weight: dict[tuple[str, str], float] = defaultdict(float)
    for row in rows:
        key = (str(row["user_id"]), str(row["tag_type"]))
        max_weight[key] = max(max_weight[key], float(row["weight"] or 0))
    for row in rows:
        user_id = str(row["user_id"])
        tag_type = str(row["tag_type"])
        tag_value = str(row["tag_value"])
        denom = max_weight[(user_id, tag_type)] or 1.0
        result[user_id][tag_type][tag_value] = float(row["weight"] or 0) / denom
    return dict(result)


def load_recent_viewer_stats(conn, now_ms: int) -> dict[str, dict[str, dict[str, float]]]:
    cutoff = now_ms - 7 * 24 * 60 * 60 * 1000
    exposures = fetch_all(conn, "SELECT request_id, user_id, session_id, post_id FROM feed_exposures WHERE created_at >= ?", (cutoff,))
    clicks = fetch_all(conn, "SELECT request_id, user_id, session_id, post_id FROM feed_clicks WHERE created_at >= ?", (cutoff,))
    post_ids = {str(row["post_id"]) for row in exposures} | {str(row["post_id"]) for row in clicks}
    posts = {item["id"]: item for item in load_posts(conn, sorted(post_ids))}
    clicked_keys = {
        (str(row["request_id"]), str(row["post_id"]), _viewer_key(row["user_id"], row["session_id"]))
        for row in clicks
    }
    stats: dict[str, dict[str, dict[str, float]]] = defaultdict(lambda: {"clicked": defaultdict(float), "unclicked": defaultdict(float)})
    for row in clicks:
        post = posts.get(str(row["post_id"]))
        if not post:
            continue
        sub_category = str(post.get("sub_category") or "").strip()
        if sub_category:
            stats[_viewer_key(row["user_id"], row["session_id"])]["clicked"][sub_category] += 1.0
    for row in exposures:
        post = posts.get(str(row["post_id"]))
        if not post:
            continue
        sub_category = str(post.get("sub_category") or "").strip()
        if not sub_category:
            continue
        viewer = _viewer_key(row["user_id"], row["session_id"])
        if (str(row["request_id"]), str(row["post_id"]), viewer) not in clicked_keys:
            stats[viewer]["unclicked"][sub_category] += 1.0
    return stats


def load_count_maps(conn) -> tuple[dict[str, int], dict[str, int], dict[str, int]]:
    chat_counts = {str(row["post_id"]): int(row["c"] or 0) for row in fetch_all(conn, "SELECT post_id, COUNT(*) AS c FROM chat_messages GROUP BY post_id")}
    review_counts = {str(row["post_id"]): int(row["c"] or 0) for row in fetch_all(conn, "SELECT post_id, COUNT(*) AS c FROM reviews GROUP BY post_id")}
    activity_counts = {str(row["user_id"]): int(row["c"] or 0) for row in fetch_all(conn, "SELECT user_id, COUNT(*) AS c FROM activity_scores GROUP BY user_id")}
    return chat_counts, review_counts, activity_counts


def build_user_embedding_sources(conn) -> dict[str, list[tuple[str, float]]]:
    sources: dict[str, list[tuple[str, float]]] = defaultdict(list)
    for row in fetch_all(conn, "SELECT author_id AS user_id, id AS post_id FROM posts"):
        sources[str(row["user_id"])].append((str(row["post_id"]), 5.0))
    for row in fetch_all(conn, "SELECT user_id, post_id FROM post_participants"):
        sources[str(row["user_id"])].append((str(row["post_id"]), 4.0))
    for row in fetch_all(conn, "SELECT user_id, post_id FROM feed_clicks WHERE user_id IS NOT NULL AND TRIM(user_id) <> ''"):
        sources[str(row["user_id"])].append((str(row["post_id"]), 2.0))
    for row in fetch_all(conn, "SELECT sender_id AS user_id, post_id FROM chat_messages GROUP BY sender_id, post_id"):
        sources[str(row["user_id"])].append((str(row["post_id"]), 1.5))
    return dict(sources)


def build_training_examples(
    conn,
    model_name: str,
    now_ms: int,
    window_days: int = TRAINING_WINDOW_DAYS,
) -> tuple[list[list[float]], list[int], dict[str, float], list[str]]:
    # Bounded by time: this runs every few minutes, and an unbounded scan of
    # feed_exposures grows without limit, so training slows down forever as
    # the table fills. Stale impressions also describe a product that no
    # longer exists.
    cutoff = now_ms - window_days * 24 * 60 * 60 * 1000
    exposures = fetch_all(
        conn,
        "SELECT request_id, user_id, session_id, post_id, created_at FROM feed_exposures WHERE created_at >= ? ORDER BY created_at ASC",
        (cutoff,),
    )
    clicks = fetch_all(
        conn,
        "SELECT request_id, post_id, created_at FROM feed_clicks WHERE created_at >= ? ORDER BY created_at ASC",
        (cutoff,),
    )
    click_times: dict[tuple[str, str], int] = {}
    for row in clicks:
        key = (str(row["request_id"]), str(row["post_id"]))
        created_at = int(row["created_at"] or 0)
        if key not in click_times or created_at < click_times[key]:
            click_times[key] = created_at

    post_ids = sorted({str(row["post_id"]) for row in exposures})
    posts = {item["id"]: item for item in load_posts(conn, post_ids)}
    users = load_users(conn)
    tag_maps = load_user_tag_maps(conn)
    viewer_stats = load_recent_viewer_stats(conn, now_ms)
    chat_counts, review_counts, activity_counts = load_count_maps(conn)
    post_embeddings = load_embedding_map(conn, "post_embeddings", "post_id", model_name)
    user_embeddings = load_embedding_map(conn, "user_embeddings", "user_id", model_name)

    x_rows: list[list[float]] = []
    y_rows: list[int] = []
    valid_exposures = 0
    valid_clicks = 0

    for row in exposures:
        post = posts.get(str(row["post_id"]))
        if not post:
            continue
        viewer_id = str(row["user_id"] or "").strip()
        viewer_key = _viewer_key(row["user_id"], row["session_id"])
        author_id = str(post["author_id"])
        author = users.get(author_id, {"rating_score": 5.0, "credit_score": 100})
        tags = tag_maps.get(viewer_id, {"category": {}, "sub_category": {}, "city": {}})
        stats = viewer_stats.get(viewer_key, {"clicked": {}, "unclicked": {}})
        sub_category = str(post.get("sub_category") or "").strip()
        category = str(post.get("category") or "").strip()
        city = city_from_address(str(post.get("address") or ""))
        age_hours = max(0.0, (now_ms - int(post.get("created_at") or now_ms)) / 3600000.0)
        fixed_time_ms = parse_fixed_time_ms(str(post.get("fixed_time") or ""))
        fixed_distance_hours = 72.0
        if fixed_time_ms and fixed_time_ms >= now_ms:
            fixed_distance_hours = (fixed_time_ms - now_ms) / 3600000.0
        freshness = clamp(1.0 - clamp(age_hours / 168.0, 0.0, 1.0), 0.0, 1.0)
        if fixed_time_ms and now_ms <= fixed_time_ms <= now_ms + 72 * 3600000:
            freshness = clamp(freshness + 0.15 * (1.0 - clamp(fixed_distance_hours / 72.0, 0.0, 1.0)), 0.0, 1.0)
        max_count = int(post.get("max_count") or 0)
        current_count = int(post.get("current_count") or 0)
        joinability = 0.0
        if max_count > 0 and current_count < max_count:
            joinability = clamp((max_count - current_count) / max_count, 0.0, 1.0)
        chat_count = float(chat_counts.get(str(row["post_id"]), 0))
        review_count = float(review_counts.get(str(row["post_id"]), 0))
        feature_row = {
            "embedding_cosine": cosine_similarity(user_embeddings.get(viewer_id, []), post_embeddings.get(str(row["post_id"]), [])),
            "category_match": float(tags["category"].get(category, 0.0)),
            "sub_category_match": float(tags["sub_category"].get(sub_category, 0.0)),
            "city_match": float(tags["city"].get(city, 0.0)),
            "author_quality": clamp(0.6 * clamp(float(author.get("rating_score") or 5.0) / 5.0, 0.0, 1.0) + 0.4 * clamp(float(author.get("credit_score") or 100.0) / 100.0, 0.0, 1.0), 0.0, 1.0),
            "author_rating_score": clamp(float(author.get("rating_score") or 5.0) / 5.0, 0.0, 1.0),
            "author_credit_score": clamp(float(author.get("credit_score") or 100.0) / 100.0, 0.0, 1.0),
            "author_activity_score_count": clamp(float(activity_counts.get(author_id, 0)) / 6.0, 0.0, 1.0),
            "post_current_count": clamp(float(current_count) / 10.0, 0.0, 1.0),
            "post_chat_count": clamp(chat_count / 10.0, 0.0, 1.0),
            "post_review_count": clamp(review_count / 5.0, 0.0, 1.0),
            "post_age_hours": clamp(age_hours / 168.0, 0.0, 1.0),
            "fixed_time_distance_hours": clamp(fixed_distance_hours / 72.0, 0.0, 1.0),
            "freshness": freshness,
            "interaction_heat": saturate(float(current_count) + 0.3 * chat_count + 0.5 * review_count),
            "joinability": joinability,
            "joinability_ratio": joinability,
            "viewer_clicked_same_subcategory_7d": clamp(float(stats["clicked"].get(sub_category, 0.0)) / 5.0, 0.0, 1.0),
            "viewer_unclicked_same_subcategory_7d": clamp(float(stats["unclicked"].get(sub_category, 0.0)) / 8.0, 0.0, 1.0),
        }
        clicked_at = click_times.get((str(row["request_id"]), str(row["post_id"])), 0)
        label = int(clicked_at and int(row["created_at"]) <= clicked_at <= int(row["created_at"]) + 24 * 3600000)
        x_rows.append([feature_row[name] for name in FEATURE_NAMES])
        y_rows.append(label)
        valid_exposures += 1
        valid_clicks += label

    stats = {
        "exposureCount": float(valid_exposures),
        "clickCount": float(valid_clicks),
        "ctr": float(valid_clicks) / float(valid_exposures or 1),
    }
    return x_rows, y_rows, stats, FEATURE_NAMES


def _viewer_key(user_id: Any, session_id: Any) -> str:
    user_value = str(user_id or "").strip()
    if user_value:
        return f"user:{user_value}"
    return f"anon:{str(session_id or '').strip()}"
