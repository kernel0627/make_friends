from __future__ import annotations

import argparse
import time

from .embedder import LocalSentenceEmbedder
from .features import build_post_text, build_user_embedding_sources, digest_text, load_posts, normalize_vector, safe_json, weighted_average
from .settings import Settings
from .storage import connect, load_embedding_map, upsert_post_embeddings, upsert_user_embeddings
from .trainer import train_ranking_model


def rebuild_post_embeddings(conn, embedder: LocalSentenceEmbedder, *, model_name: str, post_ids: list[str] | None = None, batch_size: int = 32) -> int:
    posts = load_posts(conn, post_ids)
    if not posts:
        return 0
    texts = [build_post_text(post) for post in posts]
    vectors = embedder.encode(texts, batch_size=batch_size)
    now_ms = int(time.time() * 1000)
    rows = []
    for post, text, vector in zip(posts, texts, vectors):
        rows.append((str(post["id"]), model_name, safe_json(normalize_vector(vector)), digest_text(text), now_ms))
    upsert_post_embeddings(conn, rows)
    return len(rows)


def rebuild_user_embeddings(conn, *, model_name: str, user_ids: list[str] | None = None) -> int:
    post_embeddings = load_embedding_map(conn, "post_embeddings", "post_id", model_name)
    sources = build_user_embedding_sources(conn)
    if user_ids is not None:
        allowed = {value.strip() for value in user_ids if value and value.strip()}
        sources = {user_id: items for user_id, items in sources.items() if user_id in allowed}
    now_ms = int(time.time() * 1000)
    rows = []
    for user_id, items in sources.items():
        vectors: list[list[float]] = []
        weights: list[float] = []
        digest_parts: list[str] = []
        for post_id, weight in items:
            vector = post_embeddings.get(post_id)
            if not vector:
                continue
            vectors.append(vector)
            weights.append(weight)
            digest_parts.append(f"{post_id}:{weight:g}")
        merged = weighted_average(vectors, weights)
        if not merged:
            continue
        rows.append((user_id, model_name, safe_json(merged), digest_text("|".join(sorted(digest_parts))), now_ms))
    upsert_user_embeddings(conn, rows)
    return len(rows)


def run_full_rebuild(settings: Settings) -> dict[str, float]:
    conn = connect(settings.db_path)
    try:
        embedder = LocalSentenceEmbedder(settings.model_dir, settings.preferred_device)
        post_count = rebuild_post_embeddings(conn, embedder, model_name=settings.model_name, batch_size=settings.batch_size)
        user_count = rebuild_user_embeddings(conn, model_name=settings.model_name)
        stats = train_ranking_model(conn, model_key=settings.model_key, model_name=settings.model_name)
        stats["postEmbeddings"] = float(post_count)
        stats["userEmbeddings"] = float(user_count)
        return stats
    finally:
        conn.close()


def main() -> None:
    parser = argparse.ArgumentParser(description="Rebuild recommendation embeddings and ranking model.")
    parser.add_argument("--device", default=None, help="Override device, e.g. cuda or cpu.")
    args = parser.parse_args()

    settings = Settings.from_env()
    if args.device:
        settings = Settings(**{**settings.__dict__, "preferred_device": args.device})
    stats = run_full_rebuild(settings)
    print(stats)


if __name__ == "__main__":
    main()

